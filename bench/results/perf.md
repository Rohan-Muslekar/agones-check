# Performance benchmark

Generated 2026-11-16 07:58 IST by `go run ./cmd/bench -perf`. linux/amd64, 32 CPUs, go1.26.10-X:nodwarf5. Median of 15 runs; time includes YAML parsing. The synthetic sets are copies of the three hand-written backend sets; the 3-environment findings are the expected beta-off rejections (CountsAndLists and ScheduledAutoscaler off) and their drift warnings.

| Objects | 1 env (ms) | 3 envs (ms) | 5 envs (ms) | Findings, prod | Findings, 3 envs |
|---|---|---|---|---|---|
| 55 | 13.0 | 13.3 | 13.9 | 0 | 83 |
| 110 | 22.1 | 23.8 | 27.4 | 0 | 158 |
| 209 | 42.2 | 45.6 | 49.5 | 0 | 293 |
| 407 | 79.9 | 88.2 | 93.9 | 0 | 563 |
| 803 | 154.4 | 165.6 | 177.6 | 0 | 1103 |
| 1606 | 308.9 | 334.4 | 350.6 | 0 | 2198 |

## End-to-end CLI run

`scripts/cli-timing.sh`, 2026-11-16 07:58 IST: `agones-check -e prod,dev,beta-off` on one file. 209 objects from 1 files (0 non-Agones documents skipped), 3 environments (prod, dev, beta-off): 160 errors, 133 warnings in 7ms

Median of 11 runs: 0.07 s wall clock, 34 MB peak RSS (includes process start).
