// Package mutate seeds known misconfigurations into valid manifest sets
// to measure what agones-check and Agones' own validation catch.
//
// Every operator works on generic YAML documents, so a mutant is exactly
// what a developer could have typed. An operator yields one mutant per
// place it applies.
package mutate

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

// Doc is one YAML document of a manifest set.
type Doc struct {
	File string
	Obj  map[string]any
}

// Kind of the document.
func (d Doc) Kind() string { s, _ := d.Obj["kind"].(string); return s }

// Name of the document.
func (d Doc) Name() string { return str(get(d.Obj, "metadata", "name")) }

// Set is a parsed scenario.
type Set []Doc

// Parse splits files into documents.
func Parse(files map[string][]byte) (Set, error) {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var out Set
	for _, n := range names {
		for _, chunk := range bytes.Split(files[n], []byte("\n---")) {
			var m map[string]any
			if err := yaml.Unmarshal(chunk, &m); err != nil {
				return nil, fmt.Errorf("%s: %w", n, err)
			}
			if len(m) == 0 {
				continue
			}
			out = append(out, Doc{File: n, Obj: m})
		}
	}
	return out, nil
}

// Render writes the set back as one multi-document YAML file.
func (s Set) Render() []byte {
	var b bytes.Buffer
	for _, d := range s {
		y, _ := yaml.Marshal(d.Obj)
		b.WriteString("---\n")
		b.Write(y)
	}
	return b.Bytes()
}

func (s Set) clone() Set {
	out := make(Set, len(s))
	for i, d := range s {
		out[i] = Doc{File: d.File, Obj: deepCopy(d.Obj).(map[string]any)}
	}
	return out
}

// Mutant is one seeded change.
type Mutant struct {
	Operator string
	Where    string // human description of the site
	Set      Set
}

// Operator describes a mutation family.
type Operator struct {
	Name string
	// Faulty is the ground truth: true when the change breaks the
	// deployment (rejected at apply, or fails at runtime), false for
	// benign edits used to measure false alarms.
	Faulty bool
	// Effect is what the cluster does with the mutant.
	Effect string
	Apply  func(Set) []Mutant
}

// Operators returns every operator, faulty ones first.
func Operators() []Operator {
	return []Operator{
		{"fas-fleetname-typo", true, "runtime: FleetAutoscaler event FailedGetFleet, never scales", fasFleetNameTypo},
		{"fas-counter-key-typo", true, "runtime: autoscaler error, Counter key does not exist", policyKeyTypo("counter")},
		{"fas-list-key-typo", true, "runtime: autoscaler error, List key does not exist", policyKeyTypo("list")},
		{"alloc-label-typo", true, "runtime: every allocation returns UnAllocated", allocLabelTypo},
		{"fleet-label-drift", true, "runtime: allocations that select the old label return UnAllocated", fleetLabelDrift},
		{"alloc-counter-key-typo", true, "runtime: every allocation returns UnAllocated", allocKeyTypo("counters")},
		{"alloc-list-key-typo", true, "runtime: every allocation returns UnAllocated", allocKeyTypo("lists")},
		{"alloc-capacity-unreachable", true, "runtime: allocations return UnAllocated unless capacity is raised at runtime", allocCapacity},
		{"alpha-gated-field", true, "apply: rejected where PlayerTracking is off (prod), accepted in dev", alphaField},
		{"unknown-field", true, "apply: rejected by strict field validation", unknownField},
		{"invalid-bounds", true, "apply: rejected by the Agones webhook at top level; accepted when nested in a Schedule policy, which the webhook does not validate", invalidBounds},
		{"duplicate-name", true, "apply: second document silently replaces the first", duplicateName},
		{"benign-rename-fleet", false, "none", renameFleet},
		{"benign-replicas", false, "none", replicas},
		{"benign-add-label", false, "none", addLabel},
		{"benign-raise-capacity", false, "none", raiseCapacity},
		{"benign-image-tag", false, "none", imageTag},
	}
}

// ---- faulty operators ---------------------------------------------------

func typo(s string) string {
	if len(s) < 2 {
		return s + "x"
	}
	// Double a character in the middle: "rooms" -> "roooms".
	i := len(s) / 2
	return s[:i] + s[i:i+1] + s[i:]
}

func each(s Set, kind string, fn func(c Set, d Doc) (string, bool)) []Mutant {
	var out []Mutant
	for i, d := range s {
		if d.Kind() != kind {
			continue
		}
		c := s.clone()
		if where, ok := fn(c, c[i]); ok {
			out = append(out, Mutant{Where: where, Set: c})
		}
	}
	return out
}

func fasFleetNameTypo(s Set) []Mutant {
	return each(s, "FleetAutoscaler", func(c Set, d Doc) (string, bool) {
		spec := m(d.Obj["spec"])
		name := str(spec["fleetName"])
		if name == "" {
			return "", false
		}
		spec["fleetName"] = typo(name)
		return fmt.Sprintf("%s fleetName %s->%s", d.Name(), name, spec["fleetName"]), true
	})
}

// policyMaps returns every policy map of a FleetAutoscaler, including
// those nested in schedule and chain policies.
func policyMaps(p map[string]any) []map[string]any {
	if p == nil {
		return nil
	}
	out := []map[string]any{p}
	if sch := m(p["schedule"]); sch != nil {
		out = append(out, policyMaps(m(sch["policy"]))...)
	}
	if ch, ok := p["chain"].([]any); ok {
		for _, e := range ch {
			out = append(out, policyMaps(m(e))...)
		}
	}
	return out
}

func policyKeyTypo(kind string) func(Set) []Mutant {
	return func(s Set) []Mutant {
		return each(s, "FleetAutoscaler", func(c Set, d Doc) (string, bool) {
			for _, p := range policyMaps(m(get(d.Obj, "spec", "policy"))) {
				if pol := m(p[kind]); pol != nil && str(pol["key"]) != "" {
					old := str(pol["key"])
					pol["key"] = typo(old)
					return fmt.Sprintf("%s %s key %s->%s", d.Name(), kind, old, pol["key"]), true
				}
			}
			return "", false
		})
	}
}

// selectors returns the selector maps of an allocation.
func selectors(d Doc) []map[string]any {
	spec := m(d.Obj["spec"])
	var out []map[string]any
	if r := m(spec["required"]); r != nil {
		out = append(out, r)
	}
	for _, key := range []string{"preferred", "selectors"} {
		if xs, ok := spec[key].([]any); ok {
			for _, x := range xs {
				out = append(out, m(x))
			}
		}
	}
	return out
}

func allocLabelTypo(s Set) []Mutant {
	return each(s, "GameServerAllocation", func(c Set, d Doc) (string, bool) {
		// Change the value of one label key in every selector that uses it,
		// as if the label had been renamed in the allocation code.
		sels := selectors(d)
		var key, old string
		for _, sel := range sels {
			ml := m(sel["matchLabels"])
			for _, k := range sortedKeys(ml) {
				if k != "agones.dev/fleet" {
					key, old = k, str(ml[k])
					break
				}
			}
			if key != "" {
				break
			}
		}
		if key == "" {
			return "", false
		}
		for _, sel := range sels {
			if ml := m(sel["matchLabels"]); ml != nil && str(ml[key]) == old {
				ml[key] = typo(old)
			}
		}
		return fmt.Sprintf("%s matchLabels %s=%s->%s", d.Name(), key, old, typo(old)), true
	})
}

func fleetLabelDrift(s Set) []Mutant {
	// Labels some allocation in the set selects on.
	used := map[string]string{}
	for _, d := range s {
		if d.Kind() != "GameServerAllocation" {
			continue
		}
		for _, sel := range selectors(d) {
			for k, v := range m(sel["matchLabels"]) {
				used[k] = str(v)
			}
		}
	}
	return each(s, "Fleet", func(c Set, d Doc) (string, bool) {
		lbls := m(get(d.Obj, "spec", "template", "metadata", "labels"))
		for _, k := range sortedKeys(lbls) {
			if v, ok := used[k]; ok && str(lbls[k]) == v {
				lbls[k] = v + "-v2"
				return fmt.Sprintf("%s template label %s=%s->%s-v2", d.Name(), k, v, v), true
			}
		}
		return "", false
	})
}

func allocKeyTypo(section string) func(Set) []Mutant {
	return func(s Set) []Mutant {
		return each(s, "GameServerAllocation", func(c Set, d Doc) (string, bool) {
			var key string
			for _, sel := range selectors(d) {
				if ks := sortedKeys(m(sel[section])); len(ks) > 0 {
					key = ks[0]
				}
				if key != "" {
					break
				}
			}
			if key == "" {
				return "", false
			}
			for _, sel := range selectors(d) {
				if sec := m(sel[section]); sec != nil {
					if v, ok := sec[key]; ok {
						delete(sec, key)
						sec[typo(key)] = v
					}
				}
			}
			return fmt.Sprintf("%s selector %s key %s->%s", d.Name(), section, key, typo(key)), true
		})
	}
}

func allocCapacity(s Set) []Mutant {
	// Largest capacity of each counter/list over the Fleets in the set.
	caps := map[string]int64{}
	for _, d := range s {
		if d.Kind() != "Fleet" {
			continue
		}
		for _, sec := range []string{"counters", "lists"} {
			for k, v := range m(get(d.Obj, "spec", "template", "spec", sec)) {
				c := toInt(m(v)["capacity"], 1000)
				if c > caps[sec+"/"+k] {
					caps[sec+"/"+k] = c
				}
			}
		}
	}
	return each(s, "GameServerAllocation", func(c Set, d Doc) (string, bool) {
		for _, sel := range selectors(d) {
			for _, sec := range []string{"counters", "lists"} {
				for _, k := range sortedKeys(m(sel[sec])) {
					cp, ok := caps[sec+"/"+k]
					if !ok {
						continue
					}
					entry := m(m(sel[sec])[k])
					entry["minAvailable"] = cp + 1
					return fmt.Sprintf("%s selector %s %s minAvailable->%d (capacity %d)", d.Name(), sec, k, cp+1, cp), true
				}
			}
		}
		return "", false
	})
}

func alphaField(s Set) []Mutant {
	return each(s, "Fleet", func(c Set, d Doc) (string, bool) {
		spec := m(get(d.Obj, "spec", "template", "spec"))
		if spec == nil {
			return "", false
		}
		spec["players"] = map[string]any{"initialCapacity": int64(10)}
		return d.Name() + " add players.initialCapacity (PlayerTracking, alpha)", true
	})
}

func unknownField(s Set) []Mutant {
	var out []Mutant
	out = append(out, each(s, "Fleet", func(c Set, d Doc) (string, bool) {
		spec := m(d.Obj["spec"])
		v, ok := spec["replicas"]
		if !ok {
			return "", false
		}
		delete(spec, "replicas")
		spec["replica"] = v
		return d.Name() + " replicas->replica", true
	})...)
	out = append(out, each(s, "FleetAutoscaler", func(c Set, d Doc) (string, bool) {
		for _, p := range policyMaps(m(get(d.Obj, "spec", "policy"))) {
			for _, sec := range []string{"buffer", "counter", "list"} {
				if pol := m(p[sec]); pol != nil {
					if v, ok := pol["bufferSize"]; ok {
						delete(pol, "bufferSize")
						pol["buffersize"] = v
						return d.Name() + " " + sec + ".bufferSize->buffersize", true
					}
				}
			}
		}
		return "", false
	})...)
	return out
}

func invalidBounds(s Set) []Mutant {
	return each(s, "FleetAutoscaler", func(c Set, d Doc) (string, bool) {
		for _, p := range policyMaps(m(get(d.Obj, "spec", "policy"))) {
			if b := m(p["buffer"]); b != nil {
				b["maxReplicas"] = toInt(b["minReplicas"], 1) - 1
				return d.Name() + " buffer maxReplicas below minReplicas", true
			}
			for _, sec := range []string{"counter", "list"} {
				if pol := m(p[sec]); pol != nil {
					pol["maxCapacity"] = toInt(pol["minCapacity"], 1) - 1
					return d.Name() + " " + sec + " maxCapacity below minCapacity", true
				}
			}
		}
		return "", false
	})
}

func duplicateName(s Set) []Mutant {
	var out []Mutant
	for i, d := range s {
		if d.Kind() != "Fleet" || d.Name() == "" {
			continue
		}
		c := s.clone()
		dup := Doc{File: d.File, Obj: deepCopy(d.Obj).(map[string]any)}
		if r, ok := get(dup.Obj, "spec", "replicas").(float64); ok {
			m(dup.Obj["spec"])["replicas"] = r + 1
		} else {
			m(dup.Obj["spec"])["replicas"] = int64(7)
		}
		c = append(c[:i+1], append(Set{dup}, c[i+1:]...)...)
		out = append(out, Mutant{Where: d.Name() + " defined twice (copy-paste without rename)", Set: c})
	}
	return out
}

// ---- benign operators ---------------------------------------------------

func renameFleet(s Set) []Mutant {
	var out []Mutant
	for i, d := range s {
		if d.Kind() != "Fleet" || d.Name() == "" {
			continue
		}
		old, nw := d.Name(), d.Name()+"-v2"
		c := s.clone()
		m(c[i].Obj["metadata"])["name"] = nw
		for _, o := range c {
			switch o.Kind() {
			case "FleetAutoscaler":
				if spec := m(o.Obj["spec"]); str(spec["fleetName"]) == old {
					spec["fleetName"] = nw
				}
			case "GameServerAllocation":
				for _, sel := range selectors(o) {
					if ml := m(sel["matchLabels"]); ml != nil && str(ml["agones.dev/fleet"]) == old {
						ml["agones.dev/fleet"] = nw
					}
				}
			}
		}
		out = append(out, Mutant{Where: fmt.Sprintf("rename Fleet %s->%s and every reference", old, nw), Set: c})
	}
	return out
}

func replicas(s Set) []Mutant {
	return each(s, "Fleet", func(c Set, d Doc) (string, bool) {
		m(d.Obj["spec"])["replicas"] = int64(9)
		return d.Name() + " replicas->9", true
	})
}

func addLabel(s Set) []Mutant {
	return each(s, "Fleet", func(c Set, d Doc) (string, bool) {
		tm := m(get(d.Obj, "spec", "template"))
		if tm == nil {
			return "", false
		}
		md := m(tm["metadata"])
		if md == nil {
			md = map[string]any{}
			tm["metadata"] = md
		}
		lbls := m(md["labels"])
		if lbls == nil {
			lbls = map[string]any{}
			md["labels"] = lbls
		}
		lbls["team"] = "backend"
		return d.Name() + " add template label team=backend", true
	})
}

func raiseCapacity(s Set) []Mutant {
	return each(s, "Fleet", func(c Set, d Doc) (string, bool) {
		for _, sec := range []string{"counters", "lists"} {
			secs := m(get(d.Obj, "spec", "template", "spec", sec))
			for _, k := range sortedKeys(secs) {
				e := m(secs[k])
				if e == nil {
					e = map[string]any{}
					secs[k] = e
				}
				e["capacity"] = toInt(e["capacity"], 1000) + 10
				return fmt.Sprintf("%s %s %s capacity +10", d.Name(), sec, k), true
			}
		}
		return "", false
	})
}

func imageTag(s Set) []Mutant {
	return each(s, "Fleet", func(c Set, d Doc) (string, bool) {
		cs, ok := get(d.Obj, "spec", "template", "spec", "template", "spec", "containers").([]any)
		if !ok || len(cs) == 0 {
			return "", false
		}
		c0 := m(cs[0])
		c0["image"] = str(c0["image"]) + "-rc1"
		return d.Name() + " image tag bump", true
	})
}

// ---- helpers --------------------------------------------------------------

func m(v any) map[string]any { x, _ := v.(map[string]any); return x }

func str(v any) string { s, _ := v.(string); return s }

func get(obj map[string]any, path ...string) any {
	var cur any = obj
	for _, p := range path {
		mm := m(cur)
		if mm == nil {
			return nil
		}
		cur = mm[p]
	}
	return cur
}

func toInt(v any, def int64) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	}
	return def
}

func sortedKeys(mm map[string]any) []string {
	out := make([]string, 0, len(mm))
	for k := range mm {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func deepCopy(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = deepCopy(vv)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, vv := range x {
			out[i] = deepCopy(vv)
		}
		return out
	}
	return v
}

// Describe is a one-line summary used in reports.
func (mu Mutant) Describe() string { return mu.Operator + ": " + strings.TrimSpace(mu.Where) }
