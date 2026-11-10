# Performance benchmark

Generated 2026-11-08 13:45 IST by `go run ./cmd/bench -perf`. linux/amd64, 32 CPUs, go1.26.9-X:nodwarf5. Median of 15 runs; time includes YAML parsing. The synthetic sets are copies of the three hand-written backend sets; the 3-environment findings are the expected beta-off rejections (CountsAndLists and ScheduledAutoscaler off) and their drift warnings.

| Objects | 1 env (ms) | 3 envs (ms) | 5 envs (ms) | Findings, prod | Findings, 3 envs |
|---|---|---|---|---|---|
| 55 | 12.0 | 12.9 | 13.3 | 0 | 83 |
| 110 | 24.3 | 26.1 | 29.0 | 0 | 158 |
| 209 | 44.5 | 52.8 | 50.2 | 0 | 293 |
| 407 | 80.3 | 87.0 | 92.9 | 0 | 563 |
| 803 | 159.3 | 169.3 | 176.2 | 0 | 1103 |
| 1606 | 311.1 | 331.2 | 357.6 | 0 | 2198 |

## End-to-end CLI run

`scripts/cli-timing.sh`, 2026-11-08 13:46 IST: `agones-check -e prod,dev,beta-off` on one file. 209 objects from 1 files (0 non-Agones documents skipped), 3 environments (prod, dev, beta-off): 160 errors, 133 warnings in 15.2ms

Median of 11 runs: 0.09 s wall clock, 35 MB peak RSS (includes process start).
