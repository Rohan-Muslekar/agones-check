# agones-check

Catch Agones misconfigurations before they reach a cluster.

[Agones](https://agones.dev) runs game servers on Kubernetes. A game backend describes its servers with several linked
resources: Fleets, FleetAutoscalers and GameServerAllocations. It deploys them to clusters whose Agones version and
feature gates differ between dev, staging and production. Agones validates each object on its own, when it is
applied to one cluster. Mistakes that span resources or environments get through and only show up at runtime:

- an autoscaler whose `fleetName` or Counter key has a typo is accepted, and never scales;
- an allocation whose selector no Fleet can satisfy is accepted, and every request returns `UnAllocated`;
- a manifest that uses an alpha feature passes in dev, and is rejected halfway through a production rollout.

`agones-check` loads a whole set of manifests, links them into a resource graph, runs **Agones' own validation code**
once per target environment, and adds the cross-resource and cross-environment checks the webhook cannot do.

```text
$ agones-check -e prod,dev deploy/
deploy/autoscaler.yaml:1 spec.policy.counter.key: ERROR AC102 [FleetAutoscaler/e2e/typo-counter] (env: all)
    Counter policy key "room" is not a counter of Fleet "arena" (it has: rooms (did you mean "rooms"?)); the autoscaler will never scale
deploy/allocation.yaml:1 spec.selectors: ERROR AC110 [GameServerAllocation/e2e/typo-] (env: all)
    none of the 1 selectors matches a Fleet or GameServer in namespace "e2e" (spec.selectors[0]: no template has labels
    matching "mode=rankd" (mode="rankd": did you mean "ranked"?)); every allocation will return UnAllocated
deploy/wasm.yaml:24: WARNING AC201 [FleetAutoscaler/default/wasm-autoscaler] (env: prod)
    accepted in dev but rejected in prod; a change that passes in one environment will fail on rollout to the other
```

## Install

```sh
go install github.com/Rohan-Muslekar/agones-check/cmd/agones-check@latest
# kubectl plugin: `kubectl agones-check ...`
make plugin
```

Requires Go 1.26. The binary links Agones 1.61's API package; nothing runs in the cluster.

## Usage

```text
agones-check [flags] PATH...           check manifests (files, directories, or - for stdin)
agones-check profile --from-cluster    print an environment profile read from the current cluster
agones-check rules                     list the rules
agones-check versions                  list the Agones versions profiles can target

  -e, --env      environment profile: file or built-in; repeat or comma-separate (default prod)
  -o             text | json | github | matrix
  --fail-on      error | warning | never (default error)
  --min-severity error | warning | info (default warning)
  --no-cross     only Agones' own per-object validation (what the webhook does)
```

Exit status: 0 no findings at the `--fail-on` level, 1 findings, 2 usage or input error.

Check a Helm chart by rendering it first: `helm template my-game ./chart | agones-check -e prod,dev -`.

### Environment profiles

An environment is an Agones version plus feature-gate overrides:

```yaml
# deploy/profiles/staging.yaml
name: staging
agonesVersion: "1.61"
featureGates:
  PlayerAllocationFilter: true
```

Built-ins: `prod` (1.61 defaults), `dev` (1.61, every alpha gate on), `beta-off` (1.61, beta gates off), and any
released version such as `1.54` (that version's default gates, from Agones' `test/upgrade/versionMap.yaml`). Checking
against the next version before an upgrade shows what will break. A profile can also be a Helm values file
(`agones.featureGates: "A=true&B=false"`), or generated from a live cluster:

```sh
agones-check profile --from-cluster --context prod-eu > deploy/profiles/prod-eu.yaml
```

`-o matrix` prints which object works where:

```text
| Object                               | prod | dev | 1.40     |
|--------------------------------------|------|-----|----------|
| FleetAutoscaler/game/practice-weekend | ok   | ok  | REJECTED |
| Fleet/game/ranked                    | ok   | ok  | REJECTED |
```

### In CI

```yaml
- uses: Rohan-Muslekar/agones-check@main
  with:
    paths: deploy/agones
    environments: deploy/profiles/prod.yaml,deploy/profiles/staging.yaml,dev
```

Findings appear as annotations on the pull request diff (`-o github`). See `examples/github-workflow/`.

## Rules
See the [validation rules guide](docs/rules.md) for invalid and corrected examples for every rule.

| Rule | Severity | What it catches | Agones' webhook |
|---|---|---|---|
| AC001 | error | unknown, mistyped (case-sensitive) or duplicate field | rejects (strict field validation) |
| AC002 | error | Agones' own `Validate()` fails with this environment's gates | rejects |
| AC003 | error | two documents define the same object; the last silently wins | accepts |
| AC004 | error | invalid policy nested in a Schedule policy | **accepts** (upstream gap) |
| AC005 | error | Schedule policy leaves `between.start` or `between.end` unset | rejects, although Agones documents both as optional (upstream bug) |
| AC101 | error / warning | `fleetName` matches no Fleet (warning if no Fleet of that namespace is in the input) | accepts |
| AC102 / AC103 | error | Counter / List policy key not defined in the Fleet template | accepts |
| AC104 | warning | capacity bounds unreachable with the Fleet's per-server capacity | accepts |
| AC105 | warning | two autoscalers resize the same Fleet | accepts |
| AC110 | error | allocation can never succeed: no Fleet satisfies the required selector (or any ordered selector) | accepts |
| AC111 | warning | one preferred or ordered selector matches no Fleet | accepts |
| AC112 | warning | allocation priorities or counter/list actions use keys no matching Fleet defines | accepts |
| AC113 | warning | Fleet priorities name a counter or list the template lacks | accepts |
| AC201 | warning | accepted in some environments, rejected in others | n/a (one cluster at a time) |

Counter and list capacities left empty in a Fleet are defaulted to 1000, as the Agones CRD schema does on the API
server.

## How it works

```text
manifests ─► loader (strict decode, CRD defaults) ─► resource graph ─┬─► cross-resource checks (AC1xx) ─┐
                                                                     │                                 ├─► report
profiles ──► gate sets per environment ─► Agones Validate() per env ─┴─► drift (AC201), matrix ────────┘
```

- `internal/manifest`: multi-document YAML loader, strict decoding into Agones' Go types, CRD defaulting.
- `internal/profile`: environments, gate stages per Agones version (embedded `versionmap.json`).
- `internal/check`: upstream validation per environment (`upstream.go`), cross-resource rules (`cross.go`),
  merging and the object × environment matrix (`engine.go`).
- `internal/report`: text, JSON, GitHub annotations, Markdown matrix.
- `internal/mutate`, `cmd/bench`, `cmd/oracle`, `scripts/e2e-runtime.sh`: evaluation.

Agones keeps feature gates in a package-level variable, so environments are checked one after another under a mutex.

## Evaluation

| Command | What it measures | Output |
|---|---|---|
| `make test` / `make cover` | unit tests | |
| `make bench` | mutation benchmark: 157 mutants of 14 scenarios, against `kubectl --dry-run=server` | `bench/results/mutation.md` |
| `make perf` | check time vs. number of objects and environments | `bench/results/perf.md` |
| `make oracle` | sends every mutant to a real cluster with `--dry-run=server`, compares with the offline baseline | `bench/results/oracle.md` |
| `make e2e` | applies seeded mistakes to a kind cluster and records what Agones actually does | `bench/results/e2e.md` |

Current results: agones-check finds 95 of 95 faulty mutants with no false alarm on 62 benign ones; `kubectl
--dry-run=server` finds 47 (prod gates) or 33 (dev gates). The offline baseline agrees with a real Agones 1.61.0
cluster on 157 of 157 mutants. On the cluster, all 8 runtime cases behave as the tool predicts.

`testdata/agones-examples/` holds the 54 example YAML files of Agones 1.61 (Apache-2.0, unchanged).
`testdata/backend/` holds three hand-written sets modelled on a game backend.

A kind cluster for `make oracle` and `make e2e`:

```sh
kind create cluster --name agones-check
helm repo add agones https://agones.dev/chart/stable
helm install agones agones/agones --version 1.61.0 -n agones-system --create-namespace
```

On rootless Podman, raise the inotify limit first or kube-proxy crashes with "too many open files". Once per machine
(persists across reboots):
`echo 'fs.inotify.max_user_instances = 512' | sudo tee /etc/sysctl.d/99-inotify.conf && sudo sysctl --system`. `make e2e` creates the `agones-sdk` service account in its own
namespace.

## Limitations

- Validation code is Agones 1.61. Older-version profiles change the gate settings, not the code, so a check that changed
  between versions is evaluated with 1.61 semantics.
- Counter and list *values* change at runtime; AC110 only uses what is knowable before deployment (keys, labels,
  capacities).
- Only Agones resources are checked. Templated values (`$CA_BUNDLE`) must be rendered before checking.

## License

Apache-2.0. See `LICENSE`.
