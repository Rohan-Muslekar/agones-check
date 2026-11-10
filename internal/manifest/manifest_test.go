package manifest

import (
	"strings"
	"testing"
)

const fleetYAML = `# leading comment
---
apiVersion: agones.dev/v1
kind: Fleet
metadata:
  name: lobby
spec:
  replicas: 2
  template:
    metadata:
      labels: {mode: casual}
    spec:
      ports: [{name: default, containerPort: 7654}]
      counters:
        rooms: {count: 0}
      lists:
        players:
          capacity:
        teams: {capacity: 4}
      template:
        spec:
          containers: [{name: game, image: example/game:1}]
---
apiVersion: v1
kind: Service
metadata: {name: not-agones}
---
apiVersion: allocation.agones.dev/v1
kind: GameServerAllocation
metadata:
  generateName: lobby-
  namespace: game
spec:
  selectors:
    - matchLabels: {mode: casual}
`

func TestLoadBytes(t *testing.T) {
	set, err := LoadBytes("f.yaml", []byte(fleetYAML))
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Objects) != 2 || set.Skipped != 1 {
		t.Fatalf("objects=%d skipped=%d, want 2 and 1", len(set.Objects), set.Skipped)
	}
	f := set.Objects[0]
	if f.Kind != KindFleet || f.Name != "lobby" || f.Namespace != DefaultNamespace {
		t.Errorf("fleet id = %s", f.ID())
	}
	if f.Source.Line != 3 {
		t.Errorf("fleet line = %d, want 3", f.Source.Line)
	}
	if f.DecodeErr != nil {
		t.Errorf("unexpected decode error: %v", f.DecodeErr)
	}
	a := set.Objects[1]
	if a.Kind != KindAllocation || a.Name != "lobby-" || a.Namespace != "game" {
		t.Errorf("allocation id = %s", a.ID())
	}
	if a.Source.Line != 28 {
		t.Errorf("allocation line = %d, want 28", a.Source.Line)
	}
}

func TestCRDDefaultCapacity(t *testing.T) {
	set, err := LoadBytes("f.yaml", []byte(fleetYAML))
	if err != nil {
		t.Fatal(err)
	}
	spec := set.Objects[0].Fleet.Spec.Template.Spec
	if got := spec.Counters["rooms"].Capacity; got != CRDDefaultCapacity {
		t.Errorf("missing counter capacity = %d, want %d", got, CRDDefaultCapacity)
	}
	if got := spec.Lists["players"].Capacity; got != CRDDefaultCapacity {
		t.Errorf("null list capacity = %d, want %d", got, CRDDefaultCapacity)
	}
	if got := spec.Lists["teams"].Capacity; got != 4 {
		t.Errorf("explicit list capacity = %d, want 4", got)
	}
}

func TestStrictDecode(t *testing.T) {
	cases := map[string]string{
		"unknown field":   "replica: 2",
		"case mismatch":   "Replicas: 2",
		"duplicate field": "replicas: 2\n  replicas: 3",
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			doc := "apiVersion: agones.dev/v1\nkind: Fleet\nmetadata: {name: x}\nspec:\n  " + line + "\n"
			set, err := LoadBytes("f.yaml", []byte(doc))
			if err != nil {
				// sigs.k8s.io/yaml rejects duplicate keys while converting.
				if name == "duplicate field" && strings.Contains(err.Error(), "already set") {
					return
				}
				t.Fatal(err)
			}
			if set.Objects[0].DecodeErr == nil {
				t.Fatalf("%s: want a decode error", name)
			}
		})
	}
}

func TestInvalidYAML(t *testing.T) {
	if _, err := LoadBytes("bad.yaml", []byte("kind: Fleet\n  bad: [")); err == nil {
		t.Fatal("want error for invalid YAML")
	}
}

func TestLoadDirectory(t *testing.T) {
	set, err := Load("../../testdata/backend")
	if err != nil {
		t.Fatal(err)
	}
	if set.Files != 7 || len(set.Objects) != 11 {
		t.Fatalf("files=%d objects=%d, want 7 and 11", set.Files, len(set.Objects))
	}
}
