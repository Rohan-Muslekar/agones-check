// Command oracle sends every benchmark mutant to a real cluster with
// `kubectl --dry-run=server` and records whether the API server (strict
// field validation plus the Agones webhooks) accepts it. The output is the
// TSV that `go run ./cmd/bench -oracle` compares with the offline
// dry-run-prod baseline.
//
//	KUBECONFIG=... go run ./cmd/oracle -mutants bench/mutants > bench/results/oracle.tsv
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

func main() {
	dir := flag.String("mutants", "bench/mutants", "directory written by go run ./cmd/bench")
	kubeContext := flag.String("context", "", "kube context (default: current)")
	flag.Parse()

	var files []string
	_ = filepath.WalkDir(*dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".yaml") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	nss := map[string]bool{}
	for _, f := range files {
		docs, _ := split(f)
		for _, d := range docs {
			nss[d.ns] = true
		}
	}
	for ns := range nss {
		_ = kubectl(*kubeContext, nil, "create", "namespace", ns) // already exists is fine
	}
	for _, f := range files {
		id := strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(f), filepath.ToSlash(*dir)+"/"), ".yaml")
		docs, err := split(f)
		if err != nil {
			fmt.Printf("%s\terror\t%v\n", id, err)
			continue
		}
		verdict, msg := "accepted", ""
		for _, d := range docs {
			verb := []string{"apply", "--dry-run=server", "-f", "-"}
			if d.kind == "GameServerAllocation" || d.generated {
				// Allocations are create-only requests, and apply cannot
				// target an object that only has metadata.generateName.
				verb = []string{"create", "--dry-run=server", "-f", "-"}
			}
			if err := kubectl(*kubeContext, d.data, verb...); err != nil {
				verdict, msg = "rejected", oneLine(err.Error())
				break
			}
		}
		fmt.Printf("%s\t%s\t%s\n", id, verdict, msg)
		fmt.Fprintf(os.Stderr, "%s %s\n", verdict, id)
	}
}

type doc struct {
	kind, ns  string
	generated bool // metadata.generateName without metadata.name
	data      []byte
}

func split(path string) ([]doc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []doc
	for _, chunk := range bytes.Split(b, []byte("\n---")) {
		chunk = bytes.TrimPrefix(bytes.TrimSpace(chunk), []byte("---"))
		var h struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Namespace    string `json:"namespace"`
				Name         string `json:"name"`
				GenerateName string `json:"generateName"`
			} `json:"metadata"`
		}
		if err := yaml.Unmarshal(chunk, &h); err != nil || h.Kind == "" {
			continue
		}
		ns := h.Metadata.Namespace
		if ns == "" {
			ns = "default"
		}
		generated := h.Metadata.Name == "" && h.Metadata.GenerateName != ""
		out = append(out, doc{kind: h.Kind, ns: ns, generated: generated, data: chunk})
	}
	return out, nil
}

func kubectl(ctx string, stdin []byte, args ...string) error {
	if ctx != "" {
		args = append([]string{"--context", ctx}, args...)
	}
	cmd := exec.Command("kubectl", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
