package check

import (
	"fmt"
	"strings"
	"time"

	"github.com/Rohan-Muslekar/agones-check/internal/manifest"
	"github.com/Rohan-Muslekar/agones-check/internal/profile"
)

// Options select which checks run.
type Options struct {
	// SkipCross turns off the cross-resource checks, leaving only what the
	// Agones webhook itself does. The benchmark uses it as the baseline.
	SkipCross bool
}

// Status of one object in one environment.
type Status string

const (
	StatusOK       Status = "ok"
	StatusRejected Status = "rejected" // the cluster would refuse the object
	StatusBroken   Status = "broken"   // accepted, but fails at runtime (cross-resource error)
	StatusWarning  Status = "warning"
)

// Result of checking one manifest set against a list of environments.
type Result struct {
	Profiles []string            `json:"profiles"`
	Objects  []string            `json:"objects"`
	Findings []Finding           `json:"findings"`
	Matrix   map[string][]Status `json:"matrix"` // object ID -> status per profile, in Profiles order
	Files    int                 `json:"files"`
	Skipped  int                 `json:"skippedDocuments"`
	Elapsed  time.Duration       `json:"elapsedNs"`
}

// Count returns how many findings have at least the given severity.
func (r *Result) Count(min Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity.AtLeast(min) {
			n++
		}
	}
	return n
}

// Run checks set against every profile.
func Run(set *manifest.Set, profiles []*profile.Profile, opts Options) (*Result, error) {
	start := time.Now()
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no environment profiles given")
	}
	res := &Result{Matrix: map[string][]Status{}, Files: set.Files, Skipped: set.Skipped}
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}
	res.Profiles = names
	for _, o := range set.Objects {
		res.Objects = append(res.Objects, o.ID())
	}

	var findings []Finding
	// Schema problems are the same everywhere.
	for _, o := range set.Objects {
		if o.DecodeErr != nil {
			findings = append(findings, objFinding(o, RuleSchema, Error,
				"manifest does not match the "+o.Kind+" schema: "+cleanDecodeErr(o.DecodeErr)))
		}
	}

	// Per-environment upstream validation, merged across environments.
	rejectedIn := map[string]map[string]bool{} // object -> profile -> rejected
	merged := map[string]*Finding{}
	var order []string
	for _, p := range profiles {
		fs, err := Upstream(set, p)
		if err != nil {
			return nil, err
		}
		for _, f := range fs {
			if f.Rule == RuleNestedPolicy {
				if opts.SkipCross {
					continue // not something the webhook checks
				}
			} else {
				if rejectedIn[f.Object] == nil {
					rejectedIn[f.Object] = map[string]bool{}
				}
				rejectedIn[f.Object][p.Name] = true
			}
			k := f.key()
			if m, ok := merged[k]; ok {
				m.Profiles = append(m.Profiles, p.Name)
				continue
			}
			fc := f
			merged[k] = &fc
			order = append(order, k)
		}
	}
	for _, k := range order {
		f := *merged[k]
		if len(f.Profiles) == len(profiles) {
			f.Profiles = nil
		}
		findings = append(findings, f)
	}

	// Environment drift: accepted somewhere, rejected elsewhere.
	if len(profiles) > 1 {
		for _, o := range set.Objects {
			rej := rejectedIn[o.ID()]
			if len(rej) == 0 || len(rej) == len(profiles) {
				continue
			}
			var bad, good []string
			for _, n := range names {
				if rej[n] {
					bad = append(bad, n)
				} else {
					good = append(good, n)
				}
			}
			f := objFinding(o, RuleDrift, Warning, fmt.Sprintf(
				"accepted in %s but rejected in %s; a change that passes in one environment will fail on rollout to the other",
				strings.Join(good, ", "), strings.Join(bad, ", ")))
			f.Profiles = bad
			findings = append(findings, f)
		}
	}

	if !opts.SkipCross {
		findings = append(findings, Cross(set)...)
	}
	sortFindings(findings)
	res.Findings = findings

	// Object x environment matrix.
	byObject := map[string][]Finding{}
	for _, f := range findings {
		byObject[f.Object] = append(byObject[f.Object], f)
	}
	for _, o := range set.Objects {
		row := make([]Status, len(names))
		for i, n := range names {
			row[i] = StatusOK
			for _, f := range byObject[o.ID()] {
				if !(len(f.Profiles) == 0 || contains(f.Profiles, n)) {
					continue
				}
				switch {
				case f.Rule == RuleSchema || f.Rule == RuleUpstream || f.Rule == RuleScheduleNull:
					row[i] = StatusRejected
				case f.Severity == Error && row[i] != StatusRejected:
					row[i] = StatusBroken
				case f.Severity == Warning && row[i] == StatusOK:
					row[i] = StatusWarning
				}
			}
		}
		res.Matrix[o.ID()] = row
	}
	res.Elapsed = time.Since(start)
	return res, nil
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func cleanDecodeErr(err error) string {
	s := err.Error()
	s = strings.TrimPrefix(s, "json: ")
	return s
}
