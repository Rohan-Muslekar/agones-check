package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cli(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestCleanExitZero(t *testing.T) {
	code, out, _ := cli("-e", "prod,dev", "../../testdata/backend")
	if code != 0 || !strings.Contains(out, "0 errors, 0 warnings") {
		t.Fatalf("code=%d out=%s", code, out)
	}
}

func TestFindingsExitOne(t *testing.T) {
	dir := t.TempDir()
	b, _ := os.ReadFile("../../testdata/backend/ranked/fleet.yaml")
	bad := strings.Replace(string(b), "key: rooms\n      bufferSize", "key: room\n      bufferSize", 1)
	p := filepath.Join(dir, "bad.yaml")
	_ = os.WriteFile(p, []byte(bad), 0o644)
	code, out, _ := cli(p)
	if code != 1 || !strings.Contains(out, "AC102") {
		t.Fatalf("code=%d out=%s", code, out)
	}
	if code, _, _ := cli("--fail-on", "never", p); code != 0 {
		t.Errorf("--fail-on never: code %d", code)
	}
}

func TestGitHubWarning(t *testing.T) {
	const path = "../../testdata/agones-examples/fleetautoscaler.yaml"
	const annotation = `::warning file=` + path + `,line=24,title=AC101 FleetAutoscaler/default/fleet-autoscaler-example (env%3A all)::fleetName "fleet-example": no Fleet in namespace "default" is part of this input; make sure it is deployed elsewhere`
	for _, tc := range []struct {
		failOn string
		code   int
	}{
		{"error", 0},
		{"warning", 1},
		{"never", 0},
	} {
		t.Run(tc.failOn, func(t *testing.T) {
			args := []string{"-o", "github"}
			if tc.failOn != "error" {
				args = append(args, "--fail-on", tc.failOn)
			}
			code, out, stderr := cli(append(args, path)...)
			if code != tc.code || stderr != "" {
				t.Fatalf("exit = %d, want %d; stderr = %q", code, tc.code, stderr)
			}
			var annotations []string
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "::") {
					annotations = append(annotations, line)
				}
			}
			if len(annotations) != 1 || annotations[0] != annotation {
				t.Errorf("annotations = %q, want [%q]", annotations, annotation)
			}
		})
	}
}

func TestProfileFromClusterErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		helm   string
		stderr string
	}{
		{"missing Helm", "", "helm get values: exec: \"helm\""},
		{"Helm stderr", "echo 'release not found' >&2; exit 1", "helm get values: release not found\n"},
		{"Helm exit status", "exit 7", "helm get values: exit status 7\n"},
		{"invalid values", "echo '{'", "error converting YAML to JSON"},
		{"invalid discovered version", `case "$1" in
	get) echo '{}' ;;
	list) echo '[{"app_version":"99.99.99"}]' ;;
esac`, `profile "test": unknown Agones version "99.99.99"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			if tc.helm != "" {
				if err := os.WriteFile(filepath.Join(dir, "helm"), []byte("#!/bin/sh\n"+tc.helm+"\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			code, out, stderr := cli("profile", "--from-cluster", "--name", "test")
			if code != 2 || out != "" || !strings.HasPrefix(stderr, "agones-check profile: ") || !strings.Contains(stderr, tc.stderr) {
				t.Fatalf("exit = %d, stdout = %q, stderr = %q; want exit 2, empty stdout and stderr containing %q", code, out, stderr, tc.stderr)
			}
		})
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"-e", "nope", "../../testdata/backend"},
		{"-e", "prod,prod", "../../testdata/backend"},
		{"-o", "xml", "../../testdata/backend"},
		{"--fail-on", "sometimes", "../../testdata/backend"},
		{"does-not-exist.yaml"},
	} {
		if code, _, _ := cli(args...); code != 2 {
			t.Errorf("%v: code %d, want 2", args, code)
		}
	}
}

func TestProfileFile(t *testing.T) {
	code, out, errs := cli("-e", "../../examples/profiles/staging.yaml,../../examples/profiles/prod-1.59.yaml", "-o", "matrix", "../../testdata/backend/ranked")
	if code != 0 || !strings.Contains(out, "| Object | staging | prod-1.59 |") {
		t.Fatalf("code=%d out=%s err=%s", code, out, errs)
	}
}

func TestSubcommands(t *testing.T) {
	if code, out, _ := cli("rules"); code != 0 || !strings.Contains(out, "AC110") {
		t.Errorf("rules: %d %s", code, out)
	}
	if code, out, _ := cli("versions"); code != 0 || !strings.Contains(out, "1.61.0") {
		t.Errorf("versions: %d %s", code, out)
	}
	if code, out, _ := cli("version"); code != 0 || !strings.Contains(out, "agones-check") {
		t.Errorf("version: %d %s", code, out)
	}
	if code, _, _ := cli("profile"); code != 2 {
		t.Errorf("profile without --from-cluster should fail")
	}
}

func TestStdin(t *testing.T) {
	f, _ := os.Open("../../testdata/backend/casual/fleets.yaml")
	defer f.Close()
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()
	if code, out, _ := cli("-"); code != 0 || !strings.Contains(out, "2 objects") {
		t.Errorf("stdin: %d %s", code, out)
	}
}
