package check

import (
	"strings"
	"testing"

	"github.com/Rohan-Muslekar/agones-check/internal/manifest"
	"github.com/Rohan-Muslekar/agones-check/internal/profile"
)

const fleet = `
apiVersion: agones.dev/v1
kind: Fleet
metadata: {name: ranked, namespace: game}
spec:
  replicas: 2
  template:
    metadata:
      labels: {mode: ranked}
    spec:
      ports: [{name: default, containerPort: 7654}]
      counters:
        rooms: {count: 0, capacity: 4}
      lists:
        players: {capacity: 8}
      template:
        spec:
          containers: [{name: game, image: example/game:1}]
`

func doc(kind, body string) string {
	api := map[string]string{
		"FleetAutoscaler":      "autoscaling.agones.dev/v1",
		"GameServerAllocation": "allocation.agones.dev/v1",
		"Fleet":                "agones.dev/v1",
	}[kind]
	return "---\napiVersion: " + api + "\nkind: " + kind + "\n" + body
}

func fas(policy string) string {
	return doc("FleetAutoscaler", "metadata: {name: fas, namespace: game}\nspec:\n  fleetName: ranked\n  policy:\n"+policy)
}

func alloc(spec string) string {
	return doc("GameServerAllocation", "metadata: {generateName: m-, namespace: game}\nspec:\n"+spec)
}

func run(t *testing.T, yaml string, envs ...string) *Result {
	t.Helper()
	set, err := manifest.LoadBytes("t.yaml", []byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) == 0 {
		envs = []string{"prod"}
	}
	var ps []*profile.Profile
	for _, e := range envs {
		p, err := profile.Builtin(e)
		if err != nil {
			t.Fatal(err)
		}
		ps = append(ps, p)
	}
	res, err := Run(set, ps, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func rules(r *Result) []string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, f.Rule)
	}
	return out
}

func has(r *Result, rule string, sev Severity) *Finding {
	for i, f := range r.Findings {
		if f.Rule == rule && f.Severity == sev {
			return &r.Findings[i]
		}
	}
	return nil
}

func TestCleanSet(t *testing.T) {
	y := fleet + fas("    type: Counter\n    counter: {key: rooms, bufferSize: 4, minCapacity: 8, maxCapacity: 40}\n") +
		alloc("  selectors:\n    - matchLabels: {mode: ranked}\n      counters: {rooms: {minAvailable: 1}}\n  counters: {rooms: {action: Increment, amount: 1}}\n")
	if r := run(t, y, "prod", "dev"); len(r.Findings) != 0 {
		t.Fatalf("want no findings, got %v", r.Findings)
	}
}

func TestRules(t *testing.T) {
	cases := []struct {
		name, yaml, rule string
		sev              Severity
		msg              string
	}{
		{"fleet ref typo", fleet + doc("FleetAutoscaler", "metadata: {name: f, namespace: game}\nspec:\n  fleetName: rankd\n  policy: {type: Buffer, buffer: {bufferSize: 1, maxReplicas: 5}}\n"),
			RuleFleetRef, Error, `did you mean "ranked"`},
		{"fleet ref, no fleets in input", doc("FleetAutoscaler", "metadata: {name: f}\nspec:\n  fleetName: lobby\n  policy: {type: Buffer, buffer: {bufferSize: 1, maxReplicas: 5}}\n"),
			RuleFleetRef, Warning, "deployed elsewhere"},
		{"counter key typo", fleet + fas("    type: Counter\n    counter: {key: room, bufferSize: 4, minCapacity: 8, maxCapacity: 40}\n"),
			RuleCounterKey, Error, `did you mean "rooms"`},
		{"list key typo", fleet + fas("    type: List\n    list: {key: player, bufferSize: 4, minCapacity: 8, maxCapacity: 40}\n"),
			RuleListKey, Error, `did you mean "players"`},
		{"nested counter key in chain", fleet + fas("    type: Chain\n    chain:\n      - id: a\n        type: Counter\n        counter: {key: seats, bufferSize: 4, minCapacity: 8, maxCapacity: 40}\n"),
			RuleCounterKey, Error, "chain[0]"},
		{"capacity below one server", fleet + fas("    type: Counter\n    counter: {key: rooms, bufferSize: 1, minCapacity: 0, maxCapacity: 2}\n"),
			RuleCapacity, Warning, "even one GameServer"},
		{"two autoscalers", fleet + fas("    type: Buffer\n    buffer: {bufferSize: 1, maxReplicas: 5}\n") +
			doc("FleetAutoscaler", "metadata: {name: fas2, namespace: game}\nspec:\n  fleetName: ranked\n  policy: {type: Buffer, buffer: {bufferSize: 2, maxReplicas: 9}}\n"),
			RuleSharedFleet, Warning, "overwrite each other"},
		{"duplicate fleet", fleet + "---" + fleet, RuleDuplicate, Error, "also defined"},
		{"required selector label typo", fleet + alloc("  required:\n    matchLabels: {mode: rankd}\n"),
			RuleAllocRequired, Error, `did you mean "ranked"`},
		{"all ordered selectors dead", fleet + alloc("  selectors:\n    - matchLabels: {mode: casual}\n    - matchLabels: {mode: practice}\n"),
			RuleAllocRequired, Error, "no ordered selector"},
		{"one ordered selector dead", fleet + alloc("  selectors:\n    - matchLabels: {mode: casual}\n    - matchLabels: {mode: ranked}\n"),
			RuleAllocOptional, Warning, "spec.selectors[0]"},
		{"preferred dead", fleet + alloc("  required:\n    matchLabels: {mode: ranked}\n  preferred:\n    - matchLabels: {agones.dev/fleet: lobby}\n"),
			RuleAllocOptional, Warning, "spec.preferred[0]"},
		{"selector counter missing", fleet + alloc("  selectors:\n    - matchLabels: {mode: ranked}\n      counters: {room: {minAvailable: 1}}\n"),
			RuleAllocRequired, Error, `has no counter "room"`},
		{"selector list over capacity", fleet + alloc("  selectors:\n    - matchLabels: {mode: ranked}\n      lists: {players: {minAvailable: 9}}\n"),
			RuleAllocRequired, Error, "below minAvailable 9"},
		{"selector counter over capacity", fleet + alloc("  selectors:\n    - matchLabels: {mode: ranked}\n      counters: {rooms: {minCount: 5}}\n"),
			RuleAllocRequired, Error, "below minCount 5"},
		{"action on unknown counter", fleet + alloc("  selectors:\n    - matchLabels: {mode: ranked}\n  counters: {seats: {action: Increment, amount: 1}}\n"),
			RuleAllocKeys, Warning, `counter action for "seats" references a key absent from every matching Fleet template and standalone GameServer`},
		{"priority on unknown list", fleet + alloc("  selectors:\n    - matchLabels: {mode: ranked}\n  priorities: [{type: List, key: teams, order: Ascending}]\n"),
			RuleAllocKeys, Warning, `"teams"`},
		{"fleet priority unknown key", strings.Replace(fleet, "  replicas: 2\n", "  replicas: 2\n  priorities: [{type: Counter, key: seats, order: Ascending}]\n", 1),
			RuleFleetPriority, Warning, `"seats"`},
		{"unknown field", strings.Replace(fleet, "replicas: 2", "replica: 2", 1), RuleSchema, Error, `unknown field "spec.replica"`},
		{"upstream validation", fleet + fas("    type: Buffer\n    buffer: {bufferSize: 5, minReplicas: 1, maxReplicas: 10}\n"),
			RuleUpstream, Error, "minReplicas"},
		{"nested schedule policy", fleet + fas("    type: Schedule\n    schedule:\n      between: {start: \"2026-01-01T00:00:00Z\", end: \"2100-01-01T00:00:00Z\"}\n      activePeriod: {startCron: \"0 18 * * *\", duration: 2h}\n      policy:\n        type: Buffer\n        buffer: {bufferSize: 2, minReplicas: 5, maxReplicas: 1}\n"),
			RuleNestedPolicy, Error, "does not validate policies nested"},
		{"schedule without end", fleet + fas("    type: Schedule\n    schedule:\n      between: {start: \"2026-01-01T00:00:00Z\"}\n      activePeriod: {startCron: \"0 18 * * *\", duration: 2h}\n      policy:\n        type: Buffer\n        buffer: {bufferSize: 2, minReplicas: 2, maxReplicas: 10}\n"),
			RuleScheduleNull, Error, "spec.policy.schedule.between.end"},
		{"chain schedule without start", fleet + fas("    type: Chain\n    chain:\n      - id: weekend\n        type: Schedule\n        schedule:\n          between: {end: \"2100-01-01T00:00:00Z\"}\n          activePeriod: {startCron: \"0 18 * * *\", duration: 2h}\n          policy:\n            type: Buffer\n            buffer: {bufferSize: 2, minReplicas: 2, maxReplicas: 10}\n      - id: default\n        type: Buffer\n        buffer: {bufferSize: 2, minReplicas: 2, maxReplicas: 10}\n"),
			RuleScheduleNull, Error, "spec.policy.chain[0].schedule.between.start"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := run(t, c.yaml)
			f := has(r, c.rule, c.sev)
			if f == nil {
				t.Fatalf("want %s %s, got %v", c.sev, c.rule, r.Findings)
			}
			if !strings.Contains(f.Message+f.Field, c.msg) {
				t.Errorf("message %q (field %s) does not contain %q", f.Message, f.Field, c.msg)
			}
		})
	}
}

func TestDriftAndMatrix(t *testing.T) {
	y := fleet + fas("    type: Wasm\n    wasm:\n      from:\n        url: {url: \"https://example.com/scale.wasm\"}\n")
	r := run(t, y, "prod", "dev")
	up := has(r, RuleUpstream, Error)
	if up == nil || strings.Join(up.Profiles, ",") != "prod" {
		t.Fatalf("want upstream error in prod only, got %v", r.Findings)
	}
	d := has(r, RuleDrift, Warning)
	if d == nil || !strings.Contains(d.Message, "accepted in dev but rejected in prod") {
		t.Fatalf("want drift warning, got %v", r.Findings)
	}
	row := r.Matrix["FleetAutoscaler/game/fas"]
	if row[0] != StatusRejected || row[1] != StatusOK {
		t.Errorf("matrix row = %v, want [rejected ok]", row)
	}
	if r.Matrix["Fleet/game/ranked"][0] != StatusOK {
		t.Error("fleet should be ok")
	}
}

func TestMatrixBroken(t *testing.T) {
	r := run(t, fleet+alloc("  required:\n    matchLabels: {mode: rankd}\n"))
	if r.Matrix["GameServerAllocation/game/m-"][0] != StatusBroken {
		t.Errorf("want broken, got %v", r.Matrix)
	}
}

func TestSkipCrossIsWebhookOnly(t *testing.T) {
	y := fleet + doc("FleetAutoscaler", "metadata: {name: f, namespace: game}\nspec:\n  fleetName: rankd\n  policy: {type: Buffer, buffer: {bufferSize: 1, maxReplicas: 5}}\n")
	set, _ := manifest.LoadBytes("t.yaml", []byte(y))
	p, _ := profile.Builtin("prod")
	r, err := Run(set, []*profile.Profile{p}, Options{SkipCross: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 0 {
		t.Fatalf("webhook-only run should accept the typo, got %v", r.Findings)
	}
}

func TestOlderVersionProfile(t *testing.T) {
	// CountsAndLists was alpha in Agones 1.40, so a Counter policy that is
	// fine on 1.61 is rejected there.
	r := run(t, fleet+fas("    type: Counter\n    counter: {key: rooms, bufferSize: 4, minCapacity: 8, maxCapacity: 40}\n"), "prod", "1.40")
	if d := has(r, RuleDrift, Warning); d == nil || strings.Join(d.Profiles, ",") != "1.40" {
		t.Fatalf("want drift in 1.40, got %v", r.Findings)
	}
}

func TestNoProfiles(t *testing.T) {
	set, _ := manifest.LoadBytes("t.yaml", []byte(fleet))
	if _, err := Run(set, nil, Options{}); err == nil {
		t.Fatal("want error without profiles")
	}
}

func TestEditDistance(t *testing.T) {
	if d := editDistance("rooms", "room"); d != 1 {
		t.Errorf("got %d", d)
	}
	if s := suggest("abc", []string{"xyz"}); s != "" {
		t.Errorf("unexpected suggestion %q", s)
	}
}

func BenchmarkRun200x3(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 67; i++ {
		y := strings.ReplaceAll(fleet, "name: ranked", "name: ranked"+string(rune('a'+i%26))+strings.Repeat("x", i/26))
		sb.WriteString("---" + y)
	}
	set, err := manifest.LoadBytes("b.yaml", []byte(sb.String()))
	if err != nil {
		b.Fatal(err)
	}
	var ps []*profile.Profile
	for _, n := range []string{"prod", "dev", "beta-off"} {
		p, _ := profile.Builtin(n)
		ps = append(ps, p)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Run(set, ps, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}
