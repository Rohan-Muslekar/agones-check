package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Rohan-Muslekar/agones-check/internal/check"
	"github.com/Rohan-Muslekar/agones-check/internal/manifest"
	"github.com/Rohan-Muslekar/agones-check/internal/mutate"
)

// synthetic builds a manifest file with about n Agones objects by copying
// the three hand-written backend sets under fresh names.
func synthetic(n int) ([]byte, int, error) {
	var base mutate.Set
	for _, dir := range []string{"testdata/backend/ranked", "testdata/backend/casual", "testdata/backend/tournament"} {
		files, err := readScenario(scenario{Files: []string{dir}})
		if err != nil {
			return nil, 0, err
		}
		s, err := mutate.Parse(files)
		if err != nil {
			return nil, 0, err
		}
		base = append(base, s...)
	}
	var b bytes.Buffer
	objs := 0
	for copyN := 0; objs < n; copyN++ {
		data := string(base.Render())
		// Every name and reference gets a per-copy suffix, so copies do not
		// collide and the cross-resource graph grows with n.
		for _, name := range []string{"ranked", "lobby", "practice", "tournament"} {
			data = strings.ReplaceAll(data, "name: "+name+"\n", fmt.Sprintf("name: %s-%d\n", name, copyN))
			data = strings.ReplaceAll(data, "fleetName: "+name+"\n", fmt.Sprintf("fleetName: %s-%d\n", name, copyN))
			data = strings.ReplaceAll(data, "agones.dev/fleet: "+name+"\n", fmt.Sprintf("agones.dev/fleet: %s-%d\n", name, copyN))
		}
		for _, n := range []string{"ranked-rooms", "lobby-buffer", "practice-weekend", "tournament-teams"} {
			data = strings.ReplaceAll(data, "name: "+n+"\n", fmt.Sprintf("name: %s-%d\n", n, copyN))
		}
		b.WriteString(data)
		objs += len(base)
	}
	return b.Bytes(), objs, nil
}

func median(ds []time.Duration) time.Duration {
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	return ds[len(ds)/2]
}

func runPerf(out string) error {
	envSets := map[int][]string{1: {"prod"}, 3: {"prod", "dev", "beta-off"}, 5: {"prod", "dev", "beta-off", "1.54", "1.50"}}
	sizes := []int{50, 100, 200, 400, 800, 1600}
	const runs = 15
	var b strings.Builder
	fmt.Fprintf(&b, "# Performance benchmark\n\nGenerated %s by `go run ./cmd/bench -perf`. %s/%s, %d CPUs, %s. Median of %d runs; time includes YAML parsing. The synthetic sets are copies of the three hand-written backend sets; the 3-environment findings are the expected beta-off rejections (CountsAndLists and ScheduledAutoscaler off) and their drift warnings.\n\n",
		time.Now().Format("2006-01-02 15:04 MST"), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version(), runs)
	b.WriteString("| Objects | 1 env (ms) | 3 envs (ms) | 5 envs (ms) | Findings, prod | Findings, 3 envs |\n|---|---|---|---|---|---|\n")
	for _, n := range sizes {
		data, objs, err := synthetic(n)
		if err != nil {
			return err
		}
		row := []string{fmt.Sprint(objs)}
		findings, prodFindings := 0, 0
		for _, k := range []int{1, 3, 5} {
			ps := mustProfiles(envSets[k]...)
			var ds []time.Duration
			for i := 0; i < runs; i++ {
				start := time.Now()
				set, err := manifest.LoadBytes("synthetic.yaml", data)
				if err != nil {
					return err
				}
				res, err := check.Run(set, ps, check.Options{})
				if err != nil {
					return err
				}
				ds = append(ds, time.Since(start))
				switch k {
				case 1:
					prodFindings = res.Count(check.Warning)
				case 3:
					findings = res.Count(check.Warning)
				}
			}
			row = append(row, fmt.Sprintf("%.1f", float64(median(ds).Microseconds())/1000))
		}
		row = append(row, fmt.Sprint(prodFindings), fmt.Sprint(findings))
		fmt.Fprintf(&b, "| %s |\n", strings.Join(row, " | "))
	}
	fmt.Print(b.String())
	return os.WriteFile(filepath.Join(out, "perf.md"), []byte(b.String()), 0o644)
}
