# Real-world manifests

Ran this to get the data: `scripts/realworld.sh` (downloads the 438 files pinned in
`testdata/realworld/corpus.tsv` and writes `bench/results/realworld-findings.tsv`). The corpus is
every non-fork public GitHub file outside the Agones repository that GitHub code search returned
for `"agones.dev/v1"`, `"autoscaling.agones.dev/v1"` and `"allocation.agones.dev/v1"` in
`.yaml`/`.yml` files (searched 2026-11-15). Each directory is checked as one deployment with the
default `prod` profile, without recursing into subdirectories.

## Input

| | Count |
|---|---|
| Repositories | 152 |
| Directories | 218 |
| Directories skipped: Helm templates or invalid YAML | 26 |
| Repositories with at least one Agones object checked | 126 |
| Agones objects checked | 389 (144 FleetAutoscalers, 134 Fleets, 73 GameServerAllocations, 38 GameServers) |
| Findings | 160 (84 errors, 76 warnings) |

## Triage of every finding

Each finding was read against its file. The "cluster" rows were confirmed with
`kubectl create --dry-run=server -f <file>` on kind with Agones 1.61.0 and default gates.

| Verdict | Findings | Repos | Rules | How it was checked |
|---|---|---|---|---|
| Real mistake, the cluster rejects it | 15 | 7 | AC001 error 11, AC002 error 4 | 14 of 14 files rejected on the cluster |
| Real mistake, the cluster silently ignores the field | 12 | 7 | AC001 warning | 11 of 12 accepted with the field dropped; 1 failed for an unrelated reason (it requests multi-cluster allocation) |
| Real mistake, fails at runtime | 5 | 4 | AC110 3, AC113 2 | inspection: a standalone GameServer without the `agones.dev/fleet` label its allocation selects; a Fleet priority of type List on a key defined as a Counter |
| Correct for default gates; needs alpha gate PlayerTracking | 9 | 5 | AC002 | 4 of 4 files rejected on the default-gate cluster |
| Unrendered placeholders (`${REGION}`, `<NAME>`) | 15 | 4 | AC002 | the file as written is invalid; it is a template, not a deployable manifest |
| Cannot tell | 34 | 3 | AC101 error | 32 in one repository: Fleet names carry build numbers and autoscalers name other builds; stale reference or separately deployed Fleet |
| False alarm in context | 8 | 5 | AC003 5, AC002 2, AC110 1 | alternative variants of one object kept in one directory; a kustomize patch fragment; a Fleet defined in a file outside the corpus |
| Warning: referenced Fleet not in the input | 62 | 46 | AC101 warning | correct as stated; the Fleet is deployed elsewhere |

17 of the 126 repositories contain at least one confirmed mistake. In 11 of them the mistake
passes `kubectl apply --dry-run=server` (12 ignored allocation fields, 3 dead allocations, 1 wrong
priority type), so only a cross-resource or field-level check finds it.

## What the corpus changed in the tool

Two findings in the first run were wrong, and both are fixed in commit 98fcef0:

- Unknown fields in a GameServerAllocation were reported as rejected. The allocation API is an
  aggregated API without strict field validation, so the cluster accepts the object and drops
  the field (for example a `players` filter under `spec.selectors`, which Agones 1.61 does not
  have). AC001 now reports these as a warning that the setting has no effect.
- One FleetAutoscaler defined twice under the same name was also reported by AC105 as two
  autoscalers fighting over one Fleet. AC003 already reports the duplicate; AC105 now skips it.

The mutation benchmark and both cluster oracles are unchanged by these fixes (no mutant puts an
unknown field in an allocation or duplicates an autoscaler).
