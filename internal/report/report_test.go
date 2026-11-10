package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Rohan-Muslekar/agones-check/internal/check"
	"github.com/Rohan-Muslekar/agones-check/internal/manifest"
)

func sample() *check.Result {
	return &check.Result{
		Profiles: []string{"prod", "dev"},
		Objects:  []string{"FleetAutoscaler/game/fas", "Fleet/game/ranked"},
		Matrix: map[string][]check.Status{
			"FleetAutoscaler/game/fas": {check.StatusRejected, check.StatusOK},
			"Fleet/game/ranked":        {check.StatusOK, check.StatusWarning},
		},
		Findings: []check.Finding{
			{Rule: check.RuleUpstream, Severity: check.Error, Object: "FleetAutoscaler/game/fas",
				Source: manifest.Source{File: "a.yaml", Line: 3}, Message: "Forbidden: feature, with comma", Profiles: []string{"prod"}},
			{Rule: check.RuleFleetPriority, Severity: check.Warning, Object: "Fleet/game/ranked",
				Source: manifest.Source{File: "a.yaml", Line: 9}, Message: "line1\nline2"},
		},
	}
}

func TestText(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "text", sample(), check.Warning); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"a.yaml:3: ERROR AC002", "(env: prod)", "(env: all)", "1 errors, 1 warnings"} {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q:\n%s", want, out)
		}
	}
	b.Reset()
	_ = Write(&b, "text", sample(), check.Error)
	if strings.Contains(b.String(), "AC113") {
		t.Error("min severity error should hide warnings")
	}
}

func TestJSON(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "json", sample(), check.Warning); err != nil {
		t.Fatal(err)
	}
	var r check.Result
	if err := json.Unmarshal(b.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 2 || r.Matrix["Fleet/game/ranked"][1] != check.StatusWarning {
		t.Errorf("round trip lost data: %+v", r)
	}
}

func TestGitHub(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "github", sample(), check.Warning); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "::error file=a.yaml,line=3,title=AC002 FleetAutoscaler/game/fas (env%3A prod)::Forbidden: feature, with comma") {
		t.Errorf("bad error annotation:\n%s", out)
	}
	if !strings.Contains(out, "::warning file=a.yaml,line=9") || !strings.Contains(out, "line1%0Aline2") {
		t.Errorf("bad warning annotation:\n%s", out)
	}
}

func TestMatrix(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "matrix", sample(), check.Warning); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "| FleetAutoscaler/game/fas | REJECTED | ok |") {
		t.Errorf("matrix:\n%s", b.String())
	}
}

func TestUnknownFormat(t *testing.T) {
	if err := Write(&bytes.Buffer{}, "xml", sample(), check.Warning); err == nil {
		t.Error("want error")
	}
}
