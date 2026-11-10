# Runtime ground truth

Generated 2026-11-08 13:40 IST by `scripts/e2e-runtime.sh` against kind-agones-check (Agones 1.61.0, Kubernetes v1.35.8).

| Case | kubectl --dry-run=server | What the cluster did | agones-check (prod) |
|---|---|---|---|
| fas-fleetname-typo | accepted | ableToScale/limited/desired=//; fleet replicas 2 -> 2; events: FailedGetFleet: could not fetch fleet: arenaa  | ERROR AC101 |
| fas-counter-key-typo | accepted | ableToScale/limited/desired=//; fleet replicas 2 -> 2; events: FleetAutoscaler: Error calculating desired fleet size on FleetAutoscaler typo-counter. Error: agones.dev/agones/pkg/fleetautoscalers: cannot apply CounterPolicy | ERROR AC102 |
| fas-list-default-capacity | accepted | ableToScale/limited/desired=true/false/1; fleet replicas 2 -> 1; events: AutoScalingFleet: Scaling fleet arena from 2 to 1  | WARNING AC104 |
| fas-nested-schedule | accepted | ableToScale/limited/desired=true/true/1; fleet replicas 2 -> 1; events: AutoScalingFleet: Scaling fleet arena from 2 to 1 ScalingLimited: Scaling fleet arena was limited to maximum size of 1  | ERROR AC004 |
| fas-schedule-no-end | rejected: The FleetAutoscaler "no-end" is invalid: spec.policy.schedule.between.end: Invalid value: "null": spec.policy.schedule.b | ableToScale/limited/desired=-; fleet replicas 2 -> 2; events: none | ERROR AC005 |
| gsa-label-typo | accepted | allocation state: UnAllocated | ERROR AC110 |
| gsa-counter-typo | accepted | allocation state: UnAllocated | ERROR AC110 |
| gsa-control | accepted | allocation state: Allocated | no finding |
