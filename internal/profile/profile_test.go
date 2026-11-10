package profile

import "testing"

func TestStage(t *testing.T) {
	cases := []struct {
		version, gate string
		want          GateStage
	}{
		{"1.61", "CountsAndLists", StageBeta},
		{"1.61", "WasmAutoscaler", StageAlpha},
		{"1.61", "PortRanges", StageStable},
		{"1.40", "CountsAndLists", StageAlpha},
		{"1.40", "PortRanges", StageMissing},
		{"1.53", "WasmAutoscaler", StageMissing},
		{"1.54", "WasmAutoscaler", StageAlpha},
		{"1.61", "ProcessorAllocator", StageDev},
	}
	for _, c := range cases {
		if got := Stage(c.version, c.gate); got != c.want {
			t.Errorf("Stage(%s, %s) = %s, want %s", c.version, c.gate, got, c.want)
		}
	}
}

func TestEffective(t *testing.T) {
	p := &Profile{Name: "old", AgonesVersion: "1.40"}
	e := p.Effective()
	if e["CountsAndLists"] || !e["DisableResyncOnSDKServer"] || e["ScheduledAutoscaler"] {
		t.Errorf("1.40 gates wrong: %v", e)
	}
	p = &Profile{Name: "x", AgonesVersion: "1.61", FeatureGates: map[string]bool{"WasmAutoscaler": true}}
	if !p.Effective()["WasmAutoscaler"] || !p.Effective()["CountsAndLists"] {
		t.Error("override or beta default lost")
	}
}

func TestValidate(t *testing.T) {
	bad := []*Profile{
		{Name: "a", AgonesVersion: "9.99"},
		{Name: "b", AgonesVersion: "1.61", FeatureGates: map[string]bool{"NoSuchGate": true}},
		{Name: "c", AgonesVersion: "1.53", FeatureGates: map[string]bool{"WasmAutoscaler": true}},
		{Name: "d", AgonesVersion: "1.61", FeatureGates: map[string]bool{"PortRanges": false}},
	}
	for _, p := range bad {
		if p.Validate() == nil {
			t.Errorf("profile %s: want validation error", p.Name)
		}
	}
	ok := &Profile{Name: "ok", AgonesVersion: "v1.61.0", FeatureGates: map[string]bool{"CountsAndLists": false}}
	if err := ok.Validate(); err != nil {
		t.Error(err)
	}
}

func TestBuiltin(t *testing.T) {
	for _, n := range []string{"prod", "dev", "beta-off", "1.54", "1.40.0"} {
		if _, err := Builtin(n); err != nil {
			t.Errorf("Builtin(%s): %v", n, err)
		}
	}
	if _, err := Builtin("staging"); err == nil {
		t.Error("unknown builtin should fail")
	}
}

func TestParse(t *testing.T) {
	p, err := Parse([]byte("name: staging\nagonesVersion: \"1.60\"\nfeatureGates:\n  PlayerTracking: true\n"))
	if err != nil || p.Name != "staging" || !p.FeatureGates["PlayerTracking"] {
		t.Fatalf("plain profile: %+v %v", p, err)
	}
	helm := []byte("agones:\n  featureGates: \"WasmAutoscaler=true&CountsAndLists=false\"\n  image:\n    tag: 1.59.0\n  controller:\n    replicas: 2\n")
	p, err = Parse(helm)
	if err != nil {
		t.Fatal(err)
	}
	if p.AgonesVersion != "1.59.0" || !p.FeatureGates["WasmAutoscaler"] || p.FeatureGates["CountsAndLists"] {
		t.Errorf("helm values: %+v", p)
	}
	if _, err := ParseGateString("A=maybe"); err == nil {
		t.Error("want bool parse error")
	}
}

func TestLoadFile(t *testing.T) {
	p, err := Load("../../examples/profiles/staging.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "staging" {
		t.Errorf("name = %q", p.Name)
	}
}
