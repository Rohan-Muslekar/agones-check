// Package check runs Agones' own per-object validation once per
// environment profile and adds the cross-resource checks that the Agones
// admission webhook cannot do.
package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Rohan-Muslekar/agones-check/internal/manifest"
)

// Severity of a finding.
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Info    Severity = "info"
)

func (s Severity) rank() int {
	switch s {
	case Error:
		return 2
	case Warning:
		return 1
	}
	return 0
}

// AtLeast reports whether s is as severe as min.
func (s Severity) AtLeast(min Severity) bool { return s.rank() >= min.rank() }

// Rule identifiers. AC0xx are per-object, AC1xx cross-resource, AC2xx
// cross-environment.
const (
	RuleSchema        = "AC001" // document does not decode (unknown or mistyped field)
	RuleUpstream      = "AC002" // Agones' own Validate() rejects the object in this environment
	RuleDuplicate     = "AC003" // two objects with the same kind/namespace/name
	RuleNestedPolicy  = "AC004" // policy nested in a Schedule policy is invalid; the webhook does not look at it
	RuleScheduleNull  = "AC005" // Schedule policy leaves between.start/end unset; the defaulted object is rejected
	RuleFleetRef      = "AC101" // FleetAutoscaler.fleetName does not name a Fleet
	RuleCounterKey    = "AC102" // Counter policy key not defined in the Fleet template
	RuleListKey       = "AC103" // List policy key not defined in the Fleet template
	RuleCapacity      = "AC104" // Counter/List policy capacity cannot be met by the Fleet
	RuleSharedFleet   = "AC105" // two FleetAutoscalers target the same Fleet
	RuleAllocRequired = "AC110" // required/only selector of an allocation matches no Fleet
	RuleAllocOptional = "AC111" // a preferred/ordered selector matches no Fleet
	RuleAllocKeys     = "AC112" // allocation priorities/actions use a key no matching Fleet defines
	RuleFleetPriority = "AC113" // Fleet priorities name a counter/list the template lacks
	RuleDrift         = "AC201" // valid in some environments, rejected in others
)

// RuleInfo describes a rule for docs and SARIF-style output.
var RuleInfo = map[string]string{
	RuleSchema:        "Manifest has an unknown or mistyped field (kubectl strict field validation rejects it)",
	RuleUpstream:      "Agones admission validation rejects the object in this environment",
	RuleDuplicate:     "Duplicate object: a later document silently replaces an earlier one",
	RuleNestedPolicy:  "Policy nested inside a Schedule policy is invalid (Agones' webhook does not validate it)",
	RuleScheduleNull:  "Schedule policy leaves between.start or between.end unset; Agones' mutating webhook writes null and the API server rejects it",
	RuleFleetRef:      "FleetAutoscaler references a Fleet that does not exist",
	RuleCounterKey:    "Counter autoscaling policy uses a counter the Fleet does not define",
	RuleListKey:       "List autoscaling policy uses a list the Fleet does not define",
	RuleCapacity:      "Autoscaling capacity bounds cannot be met with the Fleet's per-server capacity",
	RuleSharedFleet:   "Several FleetAutoscalers resize the same Fleet and will fight",
	RuleAllocRequired: "GameServerAllocation can never succeed: no Fleet satisfies its selector",
	RuleAllocOptional: "A preferred or ordered allocation selector matches no Fleet",
	RuleAllocKeys:     "Allocation priorities or counter/list actions use keys no matching Fleet defines",
	RuleFleetPriority: "Fleet priorities name a counter or list its template does not define",
	RuleDrift:         "Object is accepted in some environments and rejected in others",
}

// Finding is one problem.
type Finding struct {
	Rule     string          `json:"rule"`
	Severity Severity        `json:"severity"`
	Object   string          `json:"object"`
	Source   manifest.Source `json:"source"`
	Message  string          `json:"message"`
	Field    string          `json:"field,omitempty"`
	Related  []string        `json:"related,omitempty"`
	// Profiles lists the environments the finding applies to. Empty means
	// every environment.
	Profiles []string `json:"profiles,omitempty"`
}

func (f Finding) key() string {
	return strings.Join([]string{f.Object, f.Rule, f.Field, f.Message}, "|")
}

func (f Finding) String() string {
	env := "all environments"
	if len(f.Profiles) > 0 {
		env = strings.Join(f.Profiles, ",")
	}
	return fmt.Sprintf("%s %s %s [%s] %s (%s)", f.Source, f.Severity, f.Rule, f.Object, f.Message, env)
}

func sortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Source.File != b.Source.File {
			return a.Source.File < b.Source.File
		}
		if a.Source.Line != b.Source.Line {
			return a.Source.Line < b.Source.Line
		}
		if a.Severity.rank() != b.Severity.rank() {
			return a.Severity.rank() > b.Severity.rank()
		}
		return a.Rule < b.Rule
	})
}

func objFinding(o *manifest.Object, rule string, sev Severity, msg string) Finding {
	return Finding{Rule: rule, Severity: sev, Object: o.ID(), Source: o.Source, Message: msg}
}
