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

func TestWarningFailOn(t *testing.T) {
	code, _, _ := cli("--fail-on", "warning", "../../testdata/agones-examples/fleetautoscaler.yaml")
	if code != 1 {
		t.Errorf("want exit 1 on warning, got %d", code)
	}
	code, _, _ = cli("../../testdata/agones-examples/fleetautoscaler.yaml")
	if code != 0 {
		t.Errorf("warning alone should pass with default --fail-on, got %d", code)
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
