# Mutation benchmark

Generated 2026-11-12 23:53 EST by `go run ./cmd/bench`. 18 scenarios, 261 mutants.

## RQ2: unmodified scenarios (prod)

| Scenario | Objects | Errors | Warnings | Findings |
|---|---|---|---|---|
| backend-ranked | 3 | 0 | 0 |  |
| backend-casual | 5 | 0 | 0 |  |
| backend-tournament | 3 | 0 | 0 |  |
| backend-arena | 5 | 0 | 0 |  |
| backend-party | 3 | 0 | 0 |  |
| backend-events | 3 | 0 | 0 |  |
| backend-lab | 4 | 0 | 0 |  |
| example-counter | 3 | 0 | 0 |  |
| example-list | 2 | 0 | 1 | AC104: maxCapacity 100 is below the per-GameServer capacity 1000 of list "players" in Fleet "fleet-example"; even one GameServer exceeds it |
| example-buffer | 2 | 0 | 0 |  |
| example-schedule | 2 | 0 | 0 |  |
| example-chain | 2 | 0 | 0 |  |
| example-webhook | 2 | 0 | 0 |  |
| example-simple-game-server | 2 | 0 | 0 |  |
| example-xonotic | 2 | 0 | 0 |  |
| example-supertuxkart | 2 | 0 | 0 |  |
| example-cpp | 1 | 0 | 0 |  |
| example-gameserver | 1 | 0 | 0 |  |

## RQ1: mutants detected, per operator

| Operator | Faulty | Mutants | dry-run-prod | dry-run-dev | agones-check-error | agones-check | agones-check-multi |
|---|---|---|---|---|---|---|---|
| fas-fleetname-typo | true | 17 | 0 | 0 | 17 | 17 | 17 |
| fas-counter-key-typo | true | 5 | 0 | 0 | 5 | 5 | 5 |
| fas-list-key-typo | true | 3 | 0 | 0 | 3 | 3 | 3 |
| alloc-label-typo | true | 8 | 0 | 0 | 8 | 8 | 8 |
| fleet-label-drift | true | 9 | 0 | 0 | 6 | 9 | 9 |
| alloc-counter-key-typo | true | 5 | 0 | 0 | 3 | 5 | 5 |
| alloc-list-key-typo | true | 3 | 0 | 0 | 3 | 3 | 3 |
| alloc-capacity-unreachable | true | 7 | 0 | 0 | 3 | 7 | 7 |
| alpha-gated-field | true | 19 | 19 | 0 | 19 | 19 | 19 |
| unknown-field | true | 35 | 35 | 35 | 35 | 35 | 35 |
| invalid-bounds | true | 16 | 12 | 12 | 16 | 16 | 16 |
| duplicate-name | true | 18 | 0 | 0 | 18 | 18 | 18 |
| fas-schedule-unset-end | true | 4 | 4 | 4 | 4 | 4 | 4 |
| fas-second-autoscaler | true | 17 | 0 | 0 | 0 | 17 | 17 |
| fleet-priority-key-typo | true | 8 | 0 | 0 | 0 | 8 | 8 |
| benign-rename-fleet | false | 18 | 0 | 0 | 0 | 0 | 0 |
| benign-replicas | false | 19 | 0 | 0 | 0 | 0 | 0 |
| benign-add-label | false | 19 | 0 | 0 | 0 | 0 | 0 |
| benign-raise-capacity | false | 12 | 0 | 0 | 0 | 0 | 0 |
| benign-image-tag | false | 19 | 0 | 0 | 0 | 0 | 0 |

## Overall precision / recall / F1

| Detector | TP | FN | FP | TN | Precision | Recall | F1 |
|---|---|---|---|---|---|---|---|
| dry-run-prod | 70 | 104 | 0 | 87 | 1.00 | 0.40 | 0.57 |
| dry-run-dev | 51 | 123 | 0 | 87 | 1.00 | 0.29 | 0.45 |
| agones-check-error | 140 | 34 | 0 | 87 | 1.00 | 0.80 | 0.89 |
| agones-check | 174 | 0 | 0 | 87 | 1.00 | 1.00 | 1.00 |
| agones-check-multi | 174 | 0 | 0 | 87 | 1.00 | 1.00 | 1.00 |

## Missed faulty mutants (agones-check-multi)

None.

## False alarms on benign mutants (agones-check-multi)

None.

## Operators

| Operator | Ground truth on a cluster |
|---|---|
| fas-fleetname-typo | runtime: FleetAutoscaler event FailedGetFleet, never scales |
| fas-counter-key-typo | runtime: autoscaler error, Counter key does not exist |
| fas-list-key-typo | runtime: autoscaler error, List key does not exist |
| alloc-label-typo | runtime: every allocation returns UnAllocated |
| fleet-label-drift | runtime: allocations that select the old label return UnAllocated |
| alloc-counter-key-typo | runtime: every allocation returns UnAllocated |
| alloc-list-key-typo | runtime: every allocation returns UnAllocated |
| alloc-capacity-unreachable | runtime: allocations return UnAllocated unless capacity is raised at runtime |
| alpha-gated-field | apply: rejected where PlayerTracking is off (prod), accepted in dev |
| unknown-field | apply: rejected by strict field validation |
| invalid-bounds | apply: rejected by the Agones webhook at top level; accepted when nested in a Schedule policy, which the webhook does not validate |
| duplicate-name | apply: second document silently replaces the first |
| fas-schedule-unset-end | apply: rejected; the mutating webhook writes between.end as null, although Agones documents it as optional |
| fas-second-autoscaler | runtime: two autoscalers write the same Fleet's replicas, which flip between their targets |
| fleet-priority-key-typo | runtime: accepted; the unknown priority key is ignored and scale-down falls back to the default order |
| benign-rename-fleet | none |
| benign-replicas | none |
| benign-add-label | none |
| benign-raise-capacity | none |
| benign-image-tag | none |
