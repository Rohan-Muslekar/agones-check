# Mutation benchmark

Generated 2026-11-08 13:28 IST by `go run ./cmd/bench`. 14 scenarios, 157 mutants.

## RQ2: unmodified scenarios (prod)

| Scenario | Objects | Errors | Warnings | Findings |
|---|---|---|---|---|
| backend-ranked | 3 | 0 | 0 |  |
| backend-casual | 5 | 0 | 0 |  |
| backend-tournament | 3 | 0 | 0 |  |
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
| fas-fleetname-typo | true | 12 | 0 | 0 | 12 | 12 | 12 |
| fas-counter-key-typo | true | 3 | 0 | 0 | 3 | 3 | 3 |
| fas-list-key-typo | true | 2 | 0 | 0 | 2 | 2 | 2 |
| alloc-label-typo | true | 4 | 0 | 0 | 4 | 4 | 4 |
| fleet-label-drift | true | 4 | 0 | 0 | 4 | 4 | 4 |
| alloc-counter-key-typo | true | 2 | 0 | 0 | 2 | 2 | 2 |
| alloc-list-key-typo | true | 2 | 0 | 0 | 2 | 2 | 2 |
| alloc-capacity-unreachable | true | 3 | 0 | 0 | 2 | 3 | 3 |
| alpha-gated-field | true | 14 | 14 | 0 | 14 | 14 | 14 |
| unknown-field | true | 25 | 25 | 25 | 25 | 25 | 25 |
| invalid-bounds | true | 11 | 8 | 8 | 11 | 11 | 11 |
| duplicate-name | true | 13 | 0 | 0 | 13 | 13 | 13 |
| benign-rename-fleet | false | 13 | 0 | 0 | 0 | 0 | 0 |
| benign-replicas | false | 14 | 0 | 0 | 0 | 0 | 0 |
| benign-add-label | false | 14 | 0 | 0 | 0 | 0 | 0 |
| benign-raise-capacity | false | 7 | 0 | 0 | 0 | 0 | 0 |
| benign-image-tag | false | 14 | 0 | 0 | 0 | 0 | 0 |

## Overall precision / recall / F1

| Detector | TP | FN | FP | TN | Precision | Recall | F1 |
|---|---|---|---|---|---|---|---|
| dry-run-prod | 47 | 48 | 0 | 62 | 1.00 | 0.49 | 0.66 |
| dry-run-dev | 33 | 62 | 0 | 62 | 1.00 | 0.35 | 0.52 |
| agones-check-error | 94 | 1 | 0 | 62 | 1.00 | 0.99 | 0.99 |
| agones-check | 95 | 0 | 0 | 62 | 1.00 | 1.00 | 1.00 |
| agones-check-multi | 95 | 0 | 0 | 62 | 1.00 | 1.00 | 1.00 |

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
| benign-rename-fleet | none |
| benign-replicas | none |
| benign-add-label | none |
| benign-raise-capacity | none |
| benign-image-tag | none |
