# Responsibilities

ENGR5510G Foundations of Software Engineering, final project (applied track), Fall 2026.
Team: Rohan Muslekar (101006689), Ankit Suman (101041764), Emmanuel Adegboye (101038549).

The tool, tests and evaluation are implemented and passing (`make test`, `make bench`, `make perf`). This file says
who owns which part (same split as Section IV of the proposal), and lists the polishing work left for each of us
before the demo. Nothing below blocks the report: every result in the report is already reproducible from this repo.

## Deadlines

| What | When (Toronto time) |
|---|---|
| Final report (10–15 pages, IEEE CS conference format) | **Mon Nov 9, 2026, 23:59** (late = −20%/day) |
| Presentation + demo | Nov 16 / 23 / 30 (our slot: TBD) |

## Ownership

| Area | Code | Owner | Status |
|---|---|---|---|
| Manifest loader, strict decoding, CRD defaults | `internal/manifest` | Rohan | done, tested |
| Environment profiles, gate stages per Agones version, `profile --from-cluster` | `internal/profile`, `cmd/agones-check` | Rohan | done, tested |
| Upstream `Validate()` per environment, drift, matrix | `internal/check/upstream.go`, `engine.go` | Rohan | done, tested |
| Cross-resource checks (references, keys, capacity, selectors, nested policies) | `internal/check/cross.go` | Ankit | done, tested |
| CLI, output formats, kubectl plugin | `cmd/agones-check`, `internal/report`, `Makefile` | Ankit | done, tested |
| Mutation operators and benchmark (RQ1, RQ2) | `internal/mutate`, `cmd/bench` | Emmanuel | done, results in `bench/results/mutation.md` |
| Performance benchmark (RQ3) | `cmd/bench/perf.go`, `BenchmarkRun200x3` | Emmanuel | done, results in `bench/results/perf.md` |
| Cluster oracle and runtime e2e (kind) | `cmd/oracle`, `scripts/e2e-runtime.sh` | Emmanuel | done, results in `bench/results/oracle.md` (157/157) and `e2e.md` (8 cases) |
| GitHub Action and CI | `action.yml`, `.github/workflows/ci.yml` | Emmanuel | done; first run happens when the repo is pushed |

Note on the cluster runs: the kind cluster needs `fs.inotify.max_user_instances` above 128 or kube-proxy crashes
with "too many open files". Fix once per machine (survives reboots):
`echo 'fs.inotify.max_user_instances = 512' | sudo tee /etc/sysctl.d/99-inotify.conf && sudo sysctl --system`.

## Report: section owners

A complete draft exists (12 pages, IEEE CS conference template `\documentclass[conference,compsoc]{IEEEtran}`), with every
number checked against `bench/results/`. Rohan uploads the LaTeX source to a shared Overleaf project. Each person does an
edit pass on the sections they own below (wording, flow, anything they would explain differently); numbers should not
change unless the results are regenerated. Rohan merges, does the final read, and submits.

| Section | Rubric criterion | Owner |
|---|---|---|
| Abstract, I Introduction | Problem / project analysis & motivation | Rohan |
| II Background and related work | Background & related work | Ankit |
| III Analysis of the current system (Agones validation) | Problem / project analysis | Rohan |
| IV Design of agones-check | Proposed solution / design | Ankit |
| V Implementation and development process | Methodology / implementation | Rohan |
| VI Evaluation (RQ1–RQ4), VII Discussion, threats | Evaluation, results & discussion | Emmanuel |
| VIII Conclusion, references, final proofread | Writing | all, Rohan final pass |

## Tasks by person

Everything here is open work. Tick a box when done and mention it in the PR. Tasks marked **(report)** need to be
finished before Nov 9; the others are for the demo and the repo.

### Rohan

- [ ] **(report)** Push the repo, add Ankit and Emmanuel as collaborators, tag `report-2026-11-09`.
- [ ] **(report)** Upload `Project/report/latex/` to a shared Overleaf project and share it with both.
- [ ] **(report)** Merge the section edits, do the final read, export the PDF, submit on Canvas.
- [ ] Own sections I, III and V of the report (see the table above) and the profile / drift / matrix code.
- [ ] Optional: re-run `make oracle` and `make e2e` on a fresh kind cluster before the demo.

### Ankit (cross-resource rules, CLI, plugin, background and design sections)

- [ ] **(report)** Edit pass on report sections II (Background) and IV (Design): wording, flow, and check that every
      rule in `internal/check/cross.go` matches Table 3. Do not change numbers.
- [ ] **(report)** File the three upstream Agones issues at `github.com/agones-dev/agones/issues` and put the links in
      the README and in report section VII.2. The text for each is in the table below; repro files are in `testdata/`.
- [ ] Review every finding message in `internal/check/cross.go` for wording; keep the "did you mean" hints.
- [ ] Raise CLI test coverage (`cmd/agones-check` is 65.8%): add tests for `profile --from-cluster` error paths and
      for `-o github` output with warnings. Check with `go test -cover ./cmd/agones-check`.
- [ ] Write a krew plugin manifest (`plugins/agones-check.yaml`) so it installs with `kubectl krew`.
- [ ] Add a "Rules" page to the README that lists AC001..AC201 with one bad and one good example each.

| Upstream issue | Repro | Rule |
|---|---|---|
| `SchedulePolicy.ValidateSchedulePolicy` never validates its nested `policy`, so impossible bounds are accepted | `testdata/e2e/fas-nested-schedule.yaml` | AC004 |
| A Schedule policy without `between.start` or `between.end` is rejected with `must be of type string: "null"`, although the docs say "If not set, the policy is always eligible" (`Between` uses `metav1.Time` without `omitempty`) | `testdata/e2e/fas-schedule-no-end.yaml` | AC005 |
| `examples/listfleetautoscaler.yaml` sets `maxCapacity: 100` while `examples/fleet.yaml` leaves the list capacity empty (CRD default 1000), so the autoscaler removes servers | `testdata/agones-examples/` | AC104 |

### Emmanuel (benchmarks, oracle, e2e, CI, evaluation section)

- [ ] **(report)** Edit pass on report sections VI (Evaluation) and VII (Discussion): re-run `make bench perf` and
      check that every number in the tables still matches `bench/results/`. If a number changes, tell Rohan.
- [ ] **(report)** After the push, confirm `ci.yml` is green, including the `action` job that runs `action.yml` on
      this repo's own test data. Fix the workflow if it is not.
- [ ] Add at least four more mutation scenarios to `testdata/bench/scenarios.yaml` (for example a second game with
      two Fleets) and a new operator in `internal/mutate` if one is missing; re-run `make bench` and `make oracle`.
- [ ] Run the oracle against a second Agones version (for example 1.59) on kind and add the result to
      `bench/results/oracle.md`.
- [ ] Build the demo repository: one pull request that changes a counter key, with the action blocking it and
      annotating the diff; then apply the same change to kind and show the autoscaler stuck.
- [ ] Draft the slides (15-20 min): problem, design, live demo, results tables, limitations.

### Everyone

- [ ] Read the whole report once before submission and the slides before the talk.
- [ ] Rehearse the demo end to end on the presenter's laptop.

## How to continue

```sh
git clone git@github.com:Rohan-Muslekar/agones-check.git && cd agones-check
make test          # unit tests
make bench perf    # regenerate bench/results/
make help          # every target
```

Conventions: Conventional Commits (`feat:`, `fix:`, `test:`, `docs:` …), one branch per task, PR into `main`,
CI must be green before merge.
