# kubeconform baseline

Generated 2026-11-16 08:01 IST by `scripts/kubeconform-baseline.py`. kubeconform v0.6.7, `-strict -ignore-missing-schemas`, schemas converted from the Agones 1.61.0 CRDs of the current cluster with unknown fields rejected. GameServerAllocation has no CRD, so kubeconform skips it.

Faulty mutants rejected: 38 of 174. Benign mutants rejected (false alarms): 0 of 87.

| Operator | Faulty | Mutants | kubeconform | dry-run-prod | agones-check |
|---|---|---|---|---|---|
| fas-fleetname-typo | true | 17 | 0 | 0 | 17 |
| fas-counter-key-typo | true | 5 | 0 | 0 | 5 |
| alloc-label-typo | true | 8 | 0 | 0 | 8 |
| fleet-label-drift | true | 9 | 0 | 0 | 9 |
| alloc-counter-key-typo | true | 5 | 0 | 0 | 5 |
| alloc-capacity-unreachable | true | 7 | 0 | 0 | 7 |
| alpha-gated-field | true | 19 | 0 | 19 | 19 |
| unknown-field | true | 35 | 35 | 35 | 35 |
| invalid-bounds | true | 16 | 3 | 12 | 16 |
| duplicate-name | true | 18 | 0 | 0 | 18 |
| fas-second-autoscaler | true | 17 | 0 | 0 | 17 |
| fleet-priority-key-typo | true | 8 | 0 | 0 | 8 |
| benign-rename-fleet | false | 18 | 0 | 0 | 0 |
| benign-replicas | false | 19 | 0 | 0 | 0 |
| benign-add-label | false | 19 | 0 | 0 | 0 |
| benign-raise-capacity | false | 12 | 0 | 0 | 0 |
| benign-image-tag | false | 19 | 0 | 0 | 0 |
| fas-schedule-unset-end | true | 4 | 0 | 4 | 4 |
| fas-list-key-typo | true | 3 | 0 | 0 | 3 |
| alloc-list-key-typo | true | 3 | 0 | 0 | 3 |
