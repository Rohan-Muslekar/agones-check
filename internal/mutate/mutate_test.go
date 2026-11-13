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

func TestNewOperatorsApply(t *testing.T) {
	for _, tc := range []struct {
		dir  string
		fn   func(Set) []Mutant
		want int
		has  string
		lost string
	}{
		{"../../testdata/backend/casual", scheduleUnsetEnd, 1, "start:", "end:"},
		{"../../testdata/backend/arena", secondAutoscaler, 2, "name: arena-duo-matches-new", ""},
		{"../../testdata/backend/ranked", fleetPriorityKeyTypo, 1, "key: roooms", ""},
	} {
		mus := tc.fn(loadDir(t, tc.dir))
		if len(mus) != tc.want {
			t.Fatalf("%s: want %d mutants, got %d", tc.dir, tc.want, len(mus))
		}
		out := string(mus[0].Set.Render())
		if !strings.Contains(out, tc.has) {
			t.Errorf("%s (%s): missing %q", tc.dir, mus[0].Where, tc.has)
		}
		if tc.lost != "" && strings.Contains(out, tc.lost) {
			t.Errorf("%s (%s): still contains %q", tc.dir, mus[0].Where, tc.lost)
		}
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
