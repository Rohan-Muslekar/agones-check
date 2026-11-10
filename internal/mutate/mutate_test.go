package mutate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rohan-Muslekar/agones-check/internal/manifest"
)

func loadDir(t *testing.T, dir string) Set {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	files := map[string][]byte{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		files[p] = b
	}
	s, err := Parse(files)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOperatorsProduceValidYAML(t *testing.T) {
	base := loadDir(t, "../../testdata/backend/ranked")
	if len(base) != 3 {
		t.Fatalf("want 3 docs, got %d", len(base))
	}
	before := string(base.Render())
	total := 0
	for _, op := range Operators() {
		for _, mu := range op.Apply(base) {
			total++
			set, err := manifest.LoadBytes(op.Name, mu.Set.Render())
			if err != nil {
				t.Fatalf("%s: mutant does not parse: %v", op.Name, err)
			}
			if len(set.Objects) < 3 {
				t.Errorf("%s: lost objects", op.Name)
			}
			if string(mu.Set.Render()) == before {
				t.Errorf("%s (%s): mutant equals the original", op.Name, mu.Where)
			}
		}
	}
	if string(base.Render()) != before {
		t.Fatal("operators modified the base set")
	}
	if total < 15 {
		t.Errorf("only %d mutants from the ranked set", total)
	}
}

func TestTypo(t *testing.T) {
	if typo("rooms") != "roooms" || typo("a") != "ax" {
		t.Errorf("typo: %q %q", typo("rooms"), typo("a"))
	}
}

func TestRenameKeepsReferences(t *testing.T) {
	base := loadDir(t, "../../testdata/backend/casual")
	for _, mu := range renameFleet(base) {
		out := string(mu.Set.Render())
		if strings.Contains(mu.Where, "lobby->") && (!strings.Contains(out, "fleetName: lobby-v2") || !strings.Contains(out, "agones.dev/fleet: lobby-v2")) {
			t.Errorf("rename did not update references:\n%s", out)
		}
	}
}
