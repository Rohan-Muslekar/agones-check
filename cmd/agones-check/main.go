// Command agones-check checks Agones manifests for cross-resource and
// cross-environment mistakes before they are deployed.
//
// Installed as kubectl-agones_check it also works as `kubectl agones-check`.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/Rohan-Muslekar/agones-check/internal/check"
	"github.com/Rohan-Muslekar/agones-check/internal/manifest"
	"github.com/Rohan-Muslekar/agones-check/internal/profile"
	"github.com/Rohan-Muslekar/agones-check/internal/report"
)

var version = "0.1.0"

const usage = `agones-check: catch Agones misconfigurations before they reach a cluster.

Usage:
  agones-check [flags] PATH...           check manifests (files, directories, or - for stdin)
  agones-check profile --from-cluster    print an environment profile read from the current cluster
  agones-check rules                     list the rules
  agones-check versions                  list the Agones versions profiles can target
  agones-check version

Environments (-e) are profile files or built-ins:
  prod      Agones %[1]s defaults (beta gates on, alpha off)
  dev       Agones %[1]s with every alpha gate on
  beta-off  Agones %[1]s with every beta gate off
  1.NN      default gates of Agones 1.NN (for example 1.54), to preview upgrades

Exit status: 0 no findings at --fail-on level, 1 findings, 2 usage or input error.

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "rules":
			return rules(stdout)
		case "versions":
			fmt.Fprintln(stdout, strings.Join(profile.Versions(), "\n"))
			return 0
		case "version", "--version":
			fmt.Fprintf(stdout, "agones-check %s (Agones validation %s)\n", version, profile.CheckerVersion)
			return 0
		case "profile":
			return profileCmd(args[1:], stdout, stderr)
		}
	}
	fs := flag.NewFlagSet("agones-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var envs multiFlag
	fs.Var(&envs, "e", "environment profile: file or built-in name; repeat or comma-separate (default prod)")
	fs.Var(&envs, "env", "same as -e")
	format := fs.String("o", "text", "output format: "+strings.Join(report.Formats, ", "))
	failOn := fs.String("fail-on", "error", "exit 1 when a finding of this severity or worse exists: error, warning, never")
	minSev := fs.String("min-severity", "warning", "lowest severity to print: error, warning, info")
	noCross := fs.Bool("no-cross", false, "only run Agones' own per-object validation (what the webhook does)")
	fs.Usage = func() {
		fmt.Fprintf(stderr, usage, profile.CheckerVersion)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}
	if len(envs) == 0 {
		envs = []string{"prod"}
	}
	var profiles []*profile.Profile
	seen := map[string]bool{}
	for _, e := range envs {
		p, err := profile.Resolve(e)
		if err != nil {
			fmt.Fprintln(stderr, "agones-check:", err)
			return 2
		}
		if seen[p.Name] {
			fmt.Fprintf(stderr, "agones-check: environment %q given twice\n", p.Name)
			return 2
		}
		seen[p.Name] = true
		profiles = append(profiles, p)
	}
	set, err := manifest.Load(fs.Args()...)
	if err != nil {
		fmt.Fprintln(stderr, "agones-check:", err)
		return 2
	}
	res, err := check.Run(set, profiles, check.Options{SkipCross: *noCross})
	if err != nil {
		fmt.Fprintln(stderr, "agones-check:", err)
		return 2
	}
	if err := report.Write(stdout, *format, res, check.Severity(*minSev)); err != nil {
		fmt.Fprintln(stderr, "agones-check:", err)
		return 2
	}
	switch *failOn {
	case "never":
		return 0
	case "error", "warning":
		if res.Count(check.Severity(*failOn)) > 0 {
			return 1
		}
		return 0
	}
	fmt.Fprintf(stderr, "agones-check: --fail-on must be error, warning or never\n")
	return 2
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			*m = append(*m, s)
		}
	}
	return nil
}

func rules(w io.Writer) int {
	ids := make([]string, 0, len(check.RuleInfo))
	for id := range check.RuleInfo {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		fmt.Fprintf(w, "%s  %s\n", id, check.RuleInfo[id])
	}
	return 0
}

// profileCmd builds a profile from the Agones Helm release in the current
// kube context, so CI can check against what is really installed.
func profileCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("profile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fromCluster := fs.Bool("from-cluster", false, "read the Agones Helm release from the current cluster")
	name := fs.String("name", "", "profile name (default: the kube context name)")
	ns := fs.String("namespace", "agones-system", "namespace of the Agones Helm release")
	release := fs.String("release", "agones", "Agones Helm release name")
	kubeContext := fs.String("context", "", "kube context to use")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*fromCluster {
		fmt.Fprintln(stderr, "agones-check profile: only --from-cluster is supported")
		return 2
	}
	helmArgs := func(a ...string) []string {
		if *kubeContext != "" {
			a = append(a, "--kube-context", *kubeContext)
		}
		return a
	}
	out, err := exec.Command("helm", helmArgs("get", "values", *release, "-n", *ns, "-a", "-o", "json")...).Output()
	if err != nil {
		fmt.Fprintln(stderr, "agones-check profile: helm get values:", exitMsg(err))
		return 2
	}
	p, err := profile.Parse(out)
	if err != nil {
		fmt.Fprintln(stderr, "agones-check profile:", err)
		return 2
	}
	// The chart version is the Agones version.
	if lst, err := exec.Command("helm", helmArgs("list", "-n", *ns, "-o", "json", "--filter", "^"+*release+"$")...).Output(); err == nil {
		var rels []struct {
			AppVersion string `json:"app_version"`
		}
		if json.Unmarshal(lst, &rels) == nil && len(rels) == 1 && rels[0].AppVersion != "" {
			p.AgonesVersion = rels[0].AppVersion
		}
	}
	p.Name = *name
	if p.Name == "" {
		ctx := *kubeContext
		if ctx == "" {
			if b, err := exec.Command("kubectl", "config", "current-context").Output(); err == nil {
				ctx = strings.TrimSpace(string(b))
			}
		}
		p.Name = ctx
	}
	if err := p.Validate(); err != nil {
		fmt.Fprintln(stderr, "agones-check profile:", err)
		return 2
	}
	b, _ := yaml.Marshal(p)
	stdout.Write(b)
	return 0
}

func exitMsg(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return strings.TrimSpace(string(ee.Stderr))
	}
	return err.Error()
}
