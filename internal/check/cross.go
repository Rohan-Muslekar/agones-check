package check

import (
	"fmt"
	"sort"
	"strings"

	agonesv1 "agones.dev/agones/pkg/apis/agones/v1"
	allocationv1 "agones.dev/agones/pkg/apis/allocation/v1"
	autoscalingv1 "agones.dev/agones/pkg/apis/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/Rohan-Muslekar/agones-check/internal/manifest"
)

// template is what a GameServer created from a Fleet (or a standalone
// GameServer) looks like at allocation time, as far as can be known
// before deployment.
type template struct {
	obj      *manifest.Object
	name     string
	labels   map[string]string
	counters map[string]agonesv1.CounterStatus
	lists    map[string]agonesv1.ListStatus
}

// graph indexes the loaded objects for cross-resource lookups.
type graph struct {
	set *manifest.Set
	// fleets[namespace][name]
	fleets map[string]map[string]*manifest.Object
	// templates[namespace] = every allocatable template in the namespace
	templates map[string][]template
}

func newGraph(set *manifest.Set) *graph {
	g := &graph{set: set, fleets: map[string]map[string]*manifest.Object{}, templates: map[string][]template{}}
	for _, o := range set.Objects {
		switch {
		case o.Fleet != nil:
			if g.fleets[o.Namespace] == nil {
				g.fleets[o.Namespace] = map[string]*manifest.Object{}
			}
			g.fleets[o.Namespace][o.Name] = o
			lbls := map[string]string{}
			for k, v := range o.Fleet.Spec.Template.Labels {
				lbls[k] = v
			}
			// The fleet controller adds this label to every GameServer.
			lbls[agonesv1.FleetNameLabel] = o.Name
			g.templates[o.Namespace] = append(g.templates[o.Namespace], template{
				obj: o, name: "Fleet " + o.Name, labels: lbls,
				counters: o.Fleet.Spec.Template.Spec.Counters, lists: o.Fleet.Spec.Template.Spec.Lists,
			})
		case o.GameServer != nil:
			g.templates[o.Namespace] = append(g.templates[o.Namespace], template{
				obj: o, name: "GameServer " + o.Name, labels: o.GameServer.Labels,
				counters: o.GameServer.Spec.Counters, lists: o.GameServer.Spec.Lists,
			})
		}
	}
	return g
}

// Cross runs every cross-resource check. Results do not depend on the
// environment.
func Cross(set *manifest.Set) []Finding {
	g := newGraph(set)
	var out []Finding
	out = append(out, g.duplicates()...)
	out = append(out, g.autoscalers()...)
	out = append(out, g.allocations()...)
	out = append(out, g.fleetPriorities()...)
	return out
}

func (g *graph) duplicates() []Finding {
	var out []Finding
	seen := map[string]*manifest.Object{}
	for _, o := range g.set.Objects {
		// Allocations are one-shot requests (kubectl create), and
		// generateName objects get a fresh name each time.
		if o.Kind == manifest.KindAllocation || o.Name == "" || strings.HasSuffix(o.Name, "-") {
			continue
		}
		if first, ok := seen[o.ID()]; ok {
			f := objFinding(o, RuleDuplicate, Error,
				fmt.Sprintf("%s %q is also defined at %s; keep a single definition to avoid apply-order-dependent configuration", o.Kind, o.Name, first.Source))
			f.Related = []string{first.ID()}
			out = append(out, f)
			continue
		}
		seen[o.ID()] = o
	}
	return out
}

// ---- FleetAutoscalers -------------------------------------------------

type policyRef struct {
	path   string
	policy *autoscalingv1.FleetAutoscalerPolicy
}

// policies flattens Schedule and Chain policies so nested Counter/List
// policies are checked too.
func policies(p *autoscalingv1.FleetAutoscalerPolicy, path string) []policyRef {
	out := []policyRef{{path, p}}
	if p.Schedule != nil {
		out = append(out, policies(&p.Schedule.Policy, path+".schedule.policy")...)
	}
	for i := range p.Chain {
		e := &p.Chain[i]
		out = append(out, policies(&e.FleetAutoscalerPolicy, fmt.Sprintf("%s.chain[%d]", path, i))...)
	}
	return out
}

func (g *graph) autoscalers() []Finding {
	var out []Finding
	byFleet := map[string][]*manifest.Object{}
	for _, o := range g.set.ByKind(manifest.KindFleetAutoscaler) {
		fas := o.Autoscaler
		name := fas.Spec.FleetName
		if name == "" {
			continue // upstream validation reports the empty name
		}
		fleet, ok := g.fleets[o.Namespace][name]
		if !ok {
			out = append(out, g.missingFleet(o, name))
			continue
		}
		byFleet[o.Namespace+"/"+name] = append(byFleet[o.Namespace+"/"+name], o)
		tmpl := fleet.Fleet.Spec.Template.Spec
		for _, pr := range policies(&fas.Spec.Policy, "spec.policy") {
			if c := pr.policy.Counter; c != nil && c.Key != "" {
				cs, ok := tmpl.Counters[c.Key]
				if !ok {
					f := objFinding(o, RuleCounterKey, Error, fmt.Sprintf(
						"Counter policy key %q is missing from the template for Fleet %q. Available counters: %s%s; the autoscaler will never scale",
						c.Key, name, keyList(tmpl.Counters), suggest(c.Key, keysOf(tmpl.Counters))))
					f.Field, f.Related = pr.path+".counter.key", []string{fleet.ID()}
					out = append(out, f)
				} else if f, bad := capacityCheck(o, fleet, pr.path+".counter", "counter", c.Key, cs.Capacity, c.MaxCapacity); bad {
					out = append(out, f)
				}
			}
			if l := pr.policy.List; l != nil && l.Key != "" {
				ls, ok := tmpl.Lists[l.Key]
				if !ok {
					f := objFinding(o, RuleListKey, Error, fmt.Sprintf(
						"List policy key %q is missing from the template for Fleet %q. Available lists: %s%s; the autoscaler will never scale",
						l.Key, name, keyList(tmpl.Lists), suggest(l.Key, keysOf(tmpl.Lists))))
					f.Field, f.Related = pr.path+".list.key", []string{fleet.ID()}
					out = append(out, f)
				} else if f, bad := capacityCheck(o, fleet, pr.path+".list", "list", l.Key, ls.Capacity, l.MaxCapacity); bad {
					out = append(out, f)
				}
			}
		}
	}
	keys := make([]string, 0, len(byFleet))
	for k := range byFleet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		objs := byFleet[k]
		for _, o := range objs[1:] {
			f := objFinding(o, RuleSharedFleet, Warning, fmt.Sprintf(
				"Fleet %q is also targeted by %s. Multiple autoscalers can overwrite each other's replica counts",
				o.Autoscaler.Spec.FleetName, objs[0].ID()))
			f.Related = []string{objs[0].ID()}
			out = append(out, f)
		}
	}
	return out
}

func capacityCheck(o, fleet *manifest.Object, path, kind, key string, perServer, maxCap int64) (Finding, bool) {
	var msg string
	switch {
	case perServer <= 0:
		msg = fmt.Sprintf("%s %q in Fleet %q has nonpositive per-GameServer capacity (%d). Set a positive capacity for autoscaling",
			kind, key, fleet.Name, perServer)
	case maxCap > 0 && maxCap < perServer:
		msg = fmt.Sprintf("maxCapacity %d is below the per-GameServer capacity %d of %s %q in Fleet %q; even one GameServer exceeds it",
			maxCap, perServer, kind, key, fleet.Name)
	default:
		return Finding{}, false
	}
	f := objFinding(o, RuleCapacity, Warning, msg)
	f.Field, f.Related = path, []string{fleet.ID()}
	return f, true
}

func (g *graph) missingFleet(o *manifest.Object, name string) Finding {
	inNS := g.fleets[o.Namespace]
	if len(inNS) == 0 {
		// Nothing in this input deploys Fleets to the namespace: the
		// target may live in another repository.
		f := objFinding(o, RuleFleetRef, Warning, fmt.Sprintf(
			"fleetName %q: no Fleet in namespace %q is part of this input; make sure it is deployed elsewhere", name, o.Namespace))
		f.Field = "spec.fleetName"
		return f
	}
	names := make([]string, 0, len(inNS))
	for n := range inNS {
		names = append(names, n)
	}
	f := objFinding(o, RuleFleetRef, Error, fmt.Sprintf(
		"fleetName %q matches no Fleet in the input for namespace %q%s. Include the target Fleet or correct fleetName; the autoscaler will fail with FailedGetFleet",
		name, o.Namespace, suggest(name, names)))
	f.Field = "spec.fleetName"
	return f
}

// ---- GameServerAllocations --------------------------------------------

type selectorRef struct {
	path     string
	sel      *allocationv1.GameServerSelector
	required bool
}

func (g *graph) allocations() []Finding {
	var out []Finding
	for _, o := range g.set.ByKind(manifest.KindAllocation) {
		gsa := o.Allocation
		tmpls := g.templates[o.Namespace]
		if len(tmpls) == 0 {
			continue // no Fleets for this namespace in the input; nothing to compare against
		}
		var refs []selectorRef
		if len(gsa.Spec.Selectors) > 0 {
			for i := range gsa.Spec.Selectors {
				refs = append(refs, selectorRef{fmt.Sprintf("spec.selectors[%d]", i), &gsa.Spec.Selectors[i], false})
			}
		} else {
			refs = append(refs, selectorRef{"spec.required", &gsa.Spec.Required, true})
			for i := range gsa.Spec.Preferred {
				refs = append(refs, selectorRef{fmt.Sprintf("spec.preferred[%d]", i), &gsa.Spec.Preferred[i], false})
			}
		}

		anyMatch := false
		var matched []template
		var reasons []string
		var optional []Finding
		for _, r := range refs {
			m, why := matchTemplates(r.sel, tmpls)
			if len(m) > 0 {
				anyMatch = true
				matched = append(matched, m...)
				continue
			}
			if r.required {
				f := objFinding(o, RuleAllocRequired, Error, fmt.Sprintf(
					"required selector matches no Fleet template or standalone GameServer in the input for namespace %q: %s; every allocation will return UnAllocated",
					o.Namespace, why))
				f.Field = r.path
				out = append(out, f)
				continue
			}
			reasons = append(reasons, r.path+": "+why)
			f := objFinding(o, RuleAllocOptional, Warning, fmt.Sprintf("%s matches no Fleet template or standalone GameServer in the input: %s", r.path, why))
			f.Field = r.path
			optional = append(optional, f)
		}
		if len(gsa.Spec.Selectors) > 0 && !anyMatch {
			f := objFinding(o, RuleAllocRequired, Error, fmt.Sprintf(
				"no ordered selector matches a Fleet template or standalone GameServer in the input for namespace %q (%s); every allocation will return UnAllocated",
				o.Namespace, strings.Join(reasons, "; ")))
			f.Field = "spec.selectors"
			out = append(out, f)
		} else {
			out = append(out, optional...)
		}
		if len(matched) > 0 {
			out = append(out, allocationKeys(o, matched)...)
		}
	}
	return out
}

// matchTemplates returns the templates a selector can match, or a reason
// why it matches none.
func matchTemplates(sel *allocationv1.GameServerSelector, tmpls []template) ([]template, string) {
	ls, err := metav1.LabelSelectorAsSelector(&sel.LabelSelector)
	if err != nil {
		return tmpls, "" // syntax errors are reported by upstream validation
	}
	var out []template
	var why []string
	labelMatches := 0
	for _, t := range tmpls {
		if !ls.Matches(labels.Set(t.labels)) {
			continue
		}
		labelMatches++
		if r := countersFit(sel.Counters, t); r != "" {
			why = append(why, t.name+" "+r)
			continue
		}
		if r := listsFit(sel.Lists, t); r != "" {
			why = append(why, t.name+" "+r)
			continue
		}
		out = append(out, t)
	}
	if len(out) > 0 {
		return out, ""
	}
	if labelMatches == 0 {
		return nil, fmt.Sprintf("no template has labels matching %q%s", ls.String(), labelHint(sel, tmpls))
	}
	if len(why) > 3 {
		why = append(why[:3], fmt.Sprintf("and %d more", len(why)-3))
	}
	return nil, strings.Join(why, "; ")
}

// labelHint suggests a near-miss label value, e.g. mode=rankd vs ranked.
func labelHint(sel *allocationv1.GameServerSelector, tmpls []template) string {
	for _, k := range sortedKeys(sel.MatchLabels) {
		want := sel.MatchLabels[k]
		var vals []string
		for _, t := range tmpls {
			if v, ok := t.labels[k]; ok {
				vals = append(vals, v)
			}
		}
		if len(vals) == 0 {
			continue
		}
		if h := suggest(want, vals); h != "" {
			return fmt.Sprintf(" (%s=%q: did you mean %s?)", k, want, strings.TrimSuffix(strings.TrimPrefix(h, " (did you mean "), "?)"))
		}
		if !contains(vals, want) {
			return fmt.Sprintf(" (available values for label %s: %s)", k, strings.Join(uniq(vals), ", "))
		}
	}
	return ""
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

func countersFit(sel map[string]allocationv1.CounterSelector, t template) string {
	for _, key := range sortedKeys(sel) {
		cs := sel[key]
		c, ok := t.counters[key]
		if !ok {
			return fmt.Sprintf("has no counter %q%s", key, suggest(key, keysOf(t.counters)))
		}
		// Count and available both range over [0, capacity].
		if cs.MinCount > c.Capacity {
			return fmt.Sprintf("counter %q has capacity %d, below minCount %d", key, c.Capacity, cs.MinCount)
		}
		if cs.MinAvailable > c.Capacity {
			return fmt.Sprintf("counter %q has capacity %d, below minAvailable %d", key, c.Capacity, cs.MinAvailable)
		}
	}
	return ""
}

func listsFit(sel map[string]allocationv1.ListSelector, t template) string {
	for _, key := range sortedKeys(sel) {
		lsel := sel[key]
		l, ok := t.lists[key]
		if !ok {
			return fmt.Sprintf("has no list %q%s", key, suggest(key, keysOf(t.lists)))
		}
		if lsel.MinAvailable > l.Capacity {
			return fmt.Sprintf("list %q has capacity %d, below minAvailable %d", key, l.Capacity, lsel.MinAvailable)
		}
	}
	return ""
}

func allocationKeys(o *manifest.Object, matched []template) []Finding {
	gsa := o.Allocation
	hasCounter := func(k string) bool {
		for _, t := range matched {
			if _, ok := t.counters[k]; ok {
				return true
			}
		}
		return false
	}
	hasList := func(k string) bool {
		for _, t := range matched {
			if _, ok := t.lists[k]; ok {
				return true
			}
		}
		return false
	}
	var out []Finding
	add := func(path, what, key string) {
		f := objFinding(o, RuleAllocKeys, Warning, fmt.Sprintf(
			"%s %q references a key absent from every matching Fleet template and standalone GameServer in the input", what, key))
		f.Field = path
		out = append(out, f)
	}
	for i, p := range gsa.Spec.Priorities {
		switch p.Type {
		case agonesv1.GameServerPriorityCounter:
			if !hasCounter(p.Key) {
				add(fmt.Sprintf("spec.priorities[%d]", i), "priority counter", p.Key)
			}
		case agonesv1.GameServerPriorityList:
			if !hasList(p.Key) {
				add(fmt.Sprintf("spec.priorities[%d]", i), "priority list", p.Key)
			}
		}
	}
	for _, k := range sortedKeys(gsa.Spec.Counters) {
		if !hasCounter(k) {
			add("spec.counters."+k, "counter action for", k)
		}
	}
	for _, k := range sortedKeys(gsa.Spec.Lists) {
		if !hasList(k) {
			add("spec.lists."+k, "list action for", k)
		}
	}
	return out
}

// ---- Fleet priorities -------------------------------------------------

func (g *graph) fleetPriorities() []Finding {
	var out []Finding
	for _, o := range g.set.ByKind(manifest.KindFleet) {
		spec := o.Fleet.Spec
		for i, p := range spec.Priorities {
			var ok bool
			switch p.Type {
			case agonesv1.GameServerPriorityCounter:
				_, ok = spec.Template.Spec.Counters[p.Key]
			case agonesv1.GameServerPriorityList:
				_, ok = spec.Template.Spec.Lists[p.Key]
			default:
				continue
			}
			if !ok {
				f := objFinding(o, RuleFleetPriority, Warning, fmt.Sprintf(
					"priority %s %q is missing from the Fleet template. Define the key or update the priority to use an existing key", strings.ToLower(p.Type), p.Key))
				f.Field = fmt.Sprintf("spec.priorities[%d]", i)
				out = append(out, f)
			}
		}
	}
	return out
}

// ---- helpers ----------------------------------------------------------

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func keysOf[V any](m map[string]V) []string { return sortedKeys(m) }

func keyList[V any](m map[string]V) string {
	if len(m) == 0 {
		return "none"
	}
	return strings.Join(sortedKeys(m), ", ")
}

func fmtLabels(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(m))
	for _, k := range sortedKeys(m) {
		parts = append(parts, k+"="+m[k])
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// suggest returns ` (did you mean "x"?)` when a candidate is within edit
// distance 2 of name.
func suggest(name string, candidates []string) string {
	best, bestD := "", 3
	for _, c := range candidates {
		if d := editDistance(name, c); d < bestD {
			best, bestD = c, d
		}
	}
	if best == "" || best == name {
		return ""
	}
	return fmt.Sprintf(" (did you mean %q?)", best)
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
