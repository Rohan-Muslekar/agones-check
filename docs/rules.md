# Validation rules

Run `agones-check rules` for the rule IDs and `agones-check -o json PATH...` to inspect finding fields and severities. The checker runs Agones 1.61 validation with the selected environment's gate settings. Cross-resource checks use the manifests supplied in the same invocation. They can compare labels, keys and capacities, while GameServer readiness and counter or list values can change at runtime.

The examples below are edits to complete manifests. Field paths are relative to the resource named in the example. Use the default `prod` profile unless a row specifies other profiles. Each correction clears the named rule.

## Manifest and admission checks

| Rule | What it checks and why it matters | Invalid example | Corrected example |
| --- | --- | --- | --- |
| **AC001: Manifest schema** (error) | Strict decoding detects unknown, mis-cased and duplicate fields, as well as values incompatible with the Agones API types. A manifest with a mistyped field may be rejected or fail to express the intended configuration. | In the [runtime Fleet](../testdata/e2e/fleet.yaml), replace `spec.replicas: 2` with `spec.replica: 2`. | Use `spec.replicas: 2`. |
| **AC002: Agones validation** (error) | Runs Agones' defaulting and validation for each object under the profile's gates. Invalid values or disabled features can block a rollout. | With the runtime Fleet and [Buffer autoscaler](../testdata/e2e/fas-fleetname-typo.yaml), first set `fleetName: arena`, then set `spec.policy.buffer: {bufferSize: 2, minReplicas: 1, maxReplicas: 4}`. | Set `minReplicas: 2` in the Buffer policy so the minimum can accommodate the buffer. |
| **AC003: Duplicate object** (error) | Finds repeated kind, namespace and name combinations. Multiple definitions make the applied configuration depend on apply order. The checker skips allocation requests and objects with an empty name or a name ending in a hyphen. | Put two copies of the runtime Fleet into separate YAML documents in one input. Both define `Fleet/e2e/arena`. | Keep one document for `Fleet/e2e/arena`. |
| **AC004: Invalid nested Schedule policy** (error) | Validates the policy embedded in every Schedule, including Schedules inside a Chain. Agones 1.61 admission can accept an invalid nested policy whose behavior emerges when the schedule is active. | Check the runtime Fleet and [nested Schedule fixture](../testdata/e2e/fas-nested-schedule.yaml), whose nested Buffer has `minReplicas: 5` and `maxReplicas: 1`. | Set the nested Buffer's `maxReplicas: 6`. |
| **AC005: Missing Schedule bound** (error) | Flags an unset `between.start` or `between.end` in a Schedule. Agones 1.61's admission flow can serialize an unset bound as null, which the API server rejects. | Check the runtime Fleet and [Schedule without an end](../testdata/e2e/fas-schedule-no-end.yaml). | Add `end: "2100-01-01T00:00:00Z"` under `spec.policy.schedule.between`. |

## FleetAutoscaler checks

Use the [runtime Fleet](../testdata/e2e/fleet.yaml) with each autoscaler below. Its name is `arena` in namespace `e2e`, with a `rooms` counter and a `players` list. Counter and List checks also cover policies nested in Schedule or Chain policies.

| Rule | What it checks and why it matters | Invalid example | Corrected example |
| --- | --- | --- | --- |
| **AC101: Fleet reference** (error or warning) | Resolves `spec.fleetName` to a Fleet in the same namespace. A missing target prevents scaling if the Fleet is also absent from the cluster. The checker warns if the input contains no Fleets in that namespace and reports an error if it contains other Fleets there. | Use the [Fleet name typo fixture](../testdata/e2e/fas-fleetname-typo.yaml) with `fleetName: arenaa`. | Set `fleetName: arena`. |
| **AC102: Counter policy key** (error) | Checks that the target Fleet template defines the policy's counter key. The policy needs that counter to calculate capacity and scale. | Use the [Counter key typo fixture](../testdata/e2e/fas-counter-key-typo.yaml) with `spec.policy.counter.key: room`. | Set `key: rooms`. |
| **AC103: List policy key** (error) | Checks that the target Fleet template defines the policy's list key. The policy needs that list to calculate capacity and scale. | In the [List autoscaler fixture](../testdata/e2e/fas-list-default-capacity.yaml), set `spec.policy.list.key: player`. Use a Fleet list capacity of 10 for this pair. | Set `key: players`. |
| **AC104: Incompatible capacity** (warning) | Flags a per-GameServer capacity of zero or less, or a positive `maxCapacity` smaller than one server's capacity. These settings prevent the policy from using capacity as intended. The loader defaults missing or null capacities in retained entries to 1000. | Use the unmodified runtime Fleet and List autoscaler fixture. The Fleet's null `players` capacity defaults to 1000, while the policy sets `maxCapacity: 100`. | Set the Fleet's `spec.template.spec.lists.players.capacity: 10`. |
| **AC105: Shared autoscaler target** (warning) | Flags additional autoscalers targeting the same Fleet. Each can write a different desired replica count. | Correct the Fleet name typo fixture to use `fleetName: arena`, then add a copy with a different `metadata.name` and the same target. | Keep one autoscaler for `e2e/arena`. |

## Allocation and Fleet priority checks

The allocation examples use the [runtime Fleet](../testdata/e2e/fleet.yaml) and edits to the [allocation selector fixture](../testdata/e2e/gsa-label-typo.yaml). Allocation rules compare the candidates in the same namespace, including standalone GameServers. They skip namespaces with no candidates in the input.

| Rule | What it checks and why it matters | Invalid example | Corrected example |
| --- | --- | --- | --- |
| **AC110: No eligible allocation candidate** (error) | Flags a legacy required selector with no match or an ordered selector list in which every entry has no match. A candidate must satisfy label selectors, define the requested counter and list keys, and have capacities large enough for the selector minima. With no eligible candidate in the cluster, the allocation returns `UnAllocated`. | Use the allocation selector fixture with `spec.selectors: [{matchLabels: {mode: rankd}}]`. | Change `mode: rankd` to `mode: ranked`. |
| **AC111: Unmatched optional selector** (warning) | Flags an unmatched legacy preferred selector or an unmatched ordered selector when another ordered selector matches. The intended preference or fallback has no candidate in the input. | Set `spec.selectors: [{matchLabels: {mode: rankd}}, {matchLabels: {mode: ranked}}]`. | Keep only `{matchLabels: {mode: ranked}}` in the selector list. |
| **AC112: Allocation priority or action key** (warning) | Flags priority and action keys that are absent from every candidate matched by the allocation's selectors. The allocation cannot use that key on those GameServers. | With the selector corrected to `mode: ranked`, set `spec.counters: {room: {action: Increment, amount: 1}}`. | Set `spec.counters: {rooms: {action: Increment, amount: 1}}`. |
| **AC113: Fleet priority key** (warning) | Checks that each Counter or List priority refers to a key defined in the Fleet template. A missing key prevents that priority from influencing scale-down ordering. | Add `spec.priorities: [{type: List, key: player, order: Ascending}]` to the runtime Fleet. | Change `key: player` to `key: players`. |

## Environment checks

| Rule | What it checks and why it matters | Invalid example | Corrected example |
| --- | --- | --- | --- |
| **AC201: Environment drift** (warning) | Reports when an object passes admission-oriented validation in at least one requested profile and fails it in another. This can expose a feature-gate difference before a rollout. | Check the [Wasm autoscaler](../testdata/agones-examples/wasmfleetautoscaler.yaml) and its [target Fleet](../testdata/agones-examples/simple-game-server/fleet.yaml) with `-e prod,dev`. The Wasm policy is enabled in `dev` and rejected in `prod`. | Replace the autoscaler's entire `spec.policy` with `{type: Buffer, buffer: {bufferSize: 1, minReplicas: 1, maxReplicas: 4}}` and use the same profiles. |
