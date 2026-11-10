// Command bench runs the mutation benchmark (RQ1, RQ2) and the
// performance benchmark (RQ3) used in the technical report.
//
//	go run ./cmd/bench -out bench/results              # mutation benchmark
//	go run ./cmd/bench -perf -out bench/results        # performance
//	go run ./cmd/bench -oracle bench/results/oracle.tsv -out bench/results
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/Rohan-Muslekar/agones-check/internal/check"
	"github.com/Rohan-Muslekar/agones-check/internal/manifest"
	"github.com/Rohan-Muslekar/agones-check/internal/mutate"
	"github.com/Rohan-Muslekar/agones-check/internal/profile"
)

type scenario struct {
	Name  string   `json:"name"`
	Files []string `json:"files"`
}

// Detectors compared in the benchmark. A detector "detects" a mutant when
// it reports a finding that the unmodified scenario does not have.
var detectors = []string{
	"dry-run-prod",       // kubectl apply --dry-run=server on a prod cluster: strict schema + Agones webhook
	"dry-run-dev",        // the same against a dev cluster with alpha gates on
	"agones-check-error", // agones-check, prod, error findings only
	"agones-check",       // agones-check, prod, errors and warnings
	"agones-check-multi", // agones-check, prod+dev, errors and warnings
}

type mutantResult struct {
	ID       string          `json:"id"`
	Scenario string          `json:"scenario"`
	Operator string          `json:"operator"`
	Faulty   bool            `json:"faulty"`
	Where    string          `json:"where"`
	File     string          `json:"file"`
	Detected map[string]bool `json:"detected"`
	Rules    []string        `json:"rules"`
}

func main() {
	scen := flag.String("scenarios", "testdata/bench/scenarios.yaml", "scenario list")
	out := flag.String("out", "bench/results", "output directory")
	mutDir := flag.String("mutants", "bench/mutants", "where mutant manifests are written")
	perf := flag.Bool("perf", false, "run the performance benchmark instead")
	oracle := flag.String("oracle", "", "compare a cluster oracle TSV (from scripts/oracle.sh) with the dry-run-prod baseline")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	switch {
	case *perf:
		fail(runPerf(*out))
	case *oracle != "":
		fail(compareOracle(*oracle, *out))
	default:
		fail(runMutation(*scen, *out, *mutDir))
	}
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
}

func mustProfiles(names ...string) []*profile.Profile {
	var ps []*profile.Profile
	for _, n := range names {
		p, err := profile.Builtin(n)
		if err != nil {
			panic(err)
		}
		ps = append(ps, p)
	}
	return ps
}

var (
	prod    = mustProfiles("prod")
	dev     = mustProfiles("dev")
	prodDev = mustProfiles("prod", "dev")
)

// findingKeys runs one detector configuration and returns the keys of the
// findings it reports at the given severity.
type detectorRun struct {
	profiles []*profile.Profile
	opts     check.Options
	min      check.Severity
}

var detectorRuns = map[string]detectorRun{
	"dry-run-prod":       {prod, check.Options{SkipCross: true}, check.Error},
	"dry-run-dev":        {dev, check.Options{SkipCross: true}, check.Error},
	"agones-check-error": {prod, check.Options{}, check.Error},
	"agones-check":       {prod, check.Options{}, check.Warning},
	"agones-check-multi": {prodDev, check.Options{}, check.Warning},
}

type keySet map[string]map[string]bool // detector -> finding key -> true

func findingKey(f check.Finding) string { return f.Rule + "|" + f.Object + "|" + f.Field }

func runDetectors(set *manifest.Set) (keySet, error) {
	ks := keySet{}
	for name, d := range detectorRuns {
		res, err := check.Run(set, d.profiles, d.opts)
		if err != nil {
			return nil, err
		}
		ks[name] = map[string]bool{}
		for _, f := range res.Findings {
			if f.Severity.AtLeast(d.min) {
				ks[name][findingKey(f)] = true
			}
		}
	}
	return ks, nil
}

// detect reports, per detector, whether the mutant has a finding the
// unmodified scenario did not have.
func detect(set *manifest.Set, orig keySet) (map[string]bool, []string, error) {
	ks, err := runDetectors(set)
	if err != nil {
		return nil, nil, err
	}
	d := map[string]bool{}
	rules := map[string]bool{}
	for name, keys := range ks {
		for k := range keys {
			if !orig[name][k] {
				d[name] = true
				if name == "agones-check-multi" {
					rules[strings.SplitN(k, "|", 2)[0]] = true
				}
			}
		}
	}
	var rs []string
	for r := range rules {
		rs = append(rs, r)
	}
	sort.Strings(rs)
	return d, rs, nil
}

func readScenario(s scenario) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, p := range s.Files {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		paths := []string{p}
		if info.IsDir() {
			paths, _ = filepath.Glob(filepath.Join(p, "*.yaml"))
		}
		for _, f := range paths {
			b, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			files[f] = b
		}
	}
	return files, nil
}

func runMutation(scenFile, out, mutDir string) error {
	raw, err := os.ReadFile(scenFile)
	if err != nil {
		return err
	}
	var scens []scenario
	if err := yaml.Unmarshal(raw, &scens); err != nil {
		return err
	}
	_ = os.RemoveAll(mutDir)
	var results []mutantResult
	var cleanReport []string
	for _, s := range scens {
		files, err := readScenario(s)
		if err != nil {
			return fmt.Errorf("%s: %w", s.Name, err)
		}
		// RQ2: the unmodified scenario must be clean.
		var names []string
		for f := range files {
			names = append(names, f)
		}
		sort.Strings(names)
		orig, err := manifest.Load(names...)
		if err != nil {
			return err
		}
		res, err := check.Run(orig, prod, check.Options{})
		if err != nil {
			return err
		}
		var notes []string
		for _, f := range res.Findings {
			if f.Severity.AtLeast(check.Warning) {
				notes = append(notes, f.Rule+": "+f.Message)
			}
		}
		cleanReport = append(cleanReport, fmt.Sprintf("| %s | %d | %d | %d | %s |", s.Name, len(orig.Objects),
			res.Count(check.Error), res.Count(check.Warning)-res.Count(check.Error), strings.Join(notes, "<br>")))
		// Baseline findings of the unmodified scenario, rendered the same
		// way the mutants are so file names match.
		base, err := mutate.Parse(files)
		if err != nil {
			return err
		}
		origSet, err := manifest.LoadBytes("orig.yaml", base.Render())
		if err != nil {
			return err
		}
		origKeys, err := runDetectors(origSet)
		if err != nil {
			return err
		}

		for _, op := range mutate.Operators() {
			for i, mu := range op.Apply(base) {
				id := fmt.Sprintf("%s/%s-%d", s.Name, op.Name, i+1)
				data := mu.Set.Render()
				path := filepath.Join(mutDir, id+".yaml")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(path, data, 0o644); err != nil {
					return err
				}
				set, err := manifest.LoadBytes(path, data)
				if err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				d, rules, err := detect(set, origKeys)
				if err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				results = append(results, mutantResult{ID: id, Scenario: s.Name, Operator: op.Name, Faulty: op.Faulty,
					Where: mu.Where, File: path, Detected: d, Rules: rules})
			}
		}
	}
	js, _ := json.MarshalIndent(results, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "mutants.json"), js, 0o644); err != nil {
		return err
	}
	md := summarize(results, cleanReport, len(scens))
	if err := os.WriteFile(filepath.Join(out, "mutation.md"), []byte(md), 0o644); err != nil {
		return err
	}
	fmt.Print(md)
	return nil
}

type counts struct{ tp, fn, fp, tn int }

func (c counts) metrics() (p, r, f float64) {
	if c.tp+c.fp > 0 {
		p = float64(c.tp) / float64(c.tp+c.fp)
	}
	if c.tp+c.fn > 0 {
		r = float64(c.tp) / float64(c.tp+c.fn)
	}
	if p+r > 0 {
		f = 2 * p * r / (p + r)
	}
	return
}

func summarize(rs []mutantResult, clean []string, nScen int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Mutation benchmark\n\nGenerated %s by `go run ./cmd/bench`. %d scenarios, %d mutants.\n\n",
		time.Now().Format("2006-01-02 15:04 MST"), nScen, len(rs))
	b.WriteString("## RQ2: unmodified scenarios (prod)\n\n| Scenario | Objects | Errors | Warnings | Findings |\n|---|---|---|---|---|\n")
	b.WriteString(strings.Join(clean, "\n") + "\n\n")

	ops := mutate.Operators()
	b.WriteString("## RQ1: mutants detected, per operator\n\n| Operator | Faulty | Mutants | " + strings.Join(detectors, " | ") + " |\n|---|---|---|" + strings.Repeat("---|", len(detectors)) + "\n")
	for _, op := range ops {
		n := 0
		hits := map[string]int{}
		for _, r := range rs {
			if r.Operator != op.Name {
				continue
			}
			n++
			for _, d := range detectors {
				if r.Detected[d] {
					hits[d]++
				}
			}
		}
		if n == 0 {
			continue
		}
		cells := make([]string, len(detectors))
		for i, d := range detectors {
			cells[i] = fmt.Sprintf("%d", hits[d])
		}
		fmt.Fprintf(&b, "| %s | %v | %d | %s |\n", op.Name, op.Faulty, n, strings.Join(cells, " | "))
	}

	b.WriteString("\n## Overall precision / recall / F1\n\n| Detector | TP | FN | FP | TN | Precision | Recall | F1 |\n|---|---|---|---|---|---|---|---|\n")
	for _, d := range detectors {
		var c counts
		for _, r := range rs {
			switch {
			case r.Faulty && r.Detected[d]:
				c.tp++
			case r.Faulty:
				c.fn++
			case r.Detected[d]:
				c.fp++
			default:
				c.tn++
			}
		}
		p, rc, f := c.metrics()
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %.2f | %.2f | %.2f |\n", d, c.tp, c.fn, c.fp, c.tn, p, rc, f)
	}

	b.WriteString("\n## Missed faulty mutants (agones-check-multi)\n\n")
	missed := 0
	for _, r := range rs {
		if r.Faulty && !r.Detected["agones-check-multi"] {
			fmt.Fprintf(&b, "- %s: %s\n", r.ID, r.Where)
			missed++
		}
	}
	if missed == 0 {
		b.WriteString("None.\n")
	}
	b.WriteString("\n## False alarms on benign mutants (agones-check-multi)\n\n")
	fa := 0
	for _, r := range rs {
		if !r.Faulty && r.Detected["agones-check-multi"] {
			fmt.Fprintf(&b, "- %s: %s (rules %s)\n", r.ID, r.Where, strings.Join(r.Rules, ","))
			fa++
		}
	}
	if fa == 0 {
		b.WriteString("None.\n")
	}
	b.WriteString("\n## Operators\n\n| Operator | Ground truth on a cluster |\n|---|---|\n")
	for _, op := range ops {
		fmt.Fprintf(&b, "| %s | %s |\n", op.Name, op.Effect)
	}
	return b.String()
}

// compareOracle reads "mutant-id<TAB>accepted|rejected<TAB>message" lines
// produced by scripts/oracle.sh and checks that the offline webhook-prod
// baseline agrees with a real API server.
func compareOracle(tsv, out string) error {
	js, err := os.ReadFile(filepath.Join(out, "mutants.json"))
	if err != nil {
		return err
	}
	var rs []mutantResult
	if err := json.Unmarshal(js, &rs); err != nil {
		return err
	}
	byID := map[string]mutantResult{}
	for _, r := range rs {
		byID[r.ID] = r
	}
	f, err := os.Open(tsv)
	if err != nil {
		return err
	}
	defer f.Close()
	var agree, total int
	var b strings.Builder
	var dis []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 3)
		if len(parts) < 2 {
			continue
		}
		r, ok := byID[parts[0]]
		if !ok {
			continue
		}
		total++
		clusterRejected := parts[1] == "rejected"
		if clusterRejected == r.Detected["dry-run-prod"] {
			agree++
		} else {
			msg := ""
			if len(parts) > 2 {
				msg = parts[2]
			}
			dis = append(dis, fmt.Sprintf("- %s: cluster %s, offline baseline rejected=%v. %s", r.ID, parts[1], r.Detected["dry-run-prod"], msg))
		}
	}
	fmt.Fprintf(&b, "# Cluster oracle vs offline webhook baseline\n\nGenerated %s. `kubectl apply --dry-run=server` on a kind cluster running Agones %s with default (prod) gates.\n\n",
		time.Now().Format("2006-01-02 15:04 MST"), profile.CheckerVersion)
	fmt.Fprintf(&b, "Agreement: %d / %d mutants (%.1f%%).\n\n", agree, total, 100*float64(agree)/float64(max(total, 1)))
	if len(dis) > 0 {
		b.WriteString("Disagreements:\n\n" + strings.Join(dis, "\n") + "\n")
	}
	fmt.Print(b.String())
	return os.WriteFile(filepath.Join(out, "oracle.md"), []byte(b.String()), 0o644)
}
