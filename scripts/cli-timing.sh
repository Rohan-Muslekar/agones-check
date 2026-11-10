#!/usr/bin/env bash
# End-to-end CLI time (process start, YAML parsing, 3 environments) on a
# ~200-object file built from the hand-written backend sets. Appends to
# bench/results/perf.md.
set -euo pipefail
cd "$(dirname "$0")/.."
go build -o bin/agones-check ./cmd/agones-check
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for i in $(seq 0 18); do
  for f in testdata/backend/*/*.yaml; do
    printf '\n---\n'
    sed -E "s/^(\s*(name|fleetName|agones.dev\/fleet): )(ranked-rooms|lobby-buffer|practice-weekend|tournament-teams|ranked|lobby|practice|tournament)$/\1\3-$i/" "$f"
  done
done > "$tmp/synth.yaml"
summary=$(./bin/agones-check -e prod,dev,beta-off --fail-on never "$tmp/synth.yaml" | tail -1)
times=$(for _ in $(seq 1 11); do /usr/bin/time -f "%e %M" ./bin/agones-check -e prod,dev,beta-off --fail-on never "$tmp/synth.yaml" 2>&1 >/dev/null; done | sort -n)
med=$(echo "$times" | sed -n 6p)
{
  echo
  echo "## End-to-end CLI run"
  echo
  echo "\`scripts/cli-timing.sh\`, $(date '+%Y-%m-%d %H:%M %Z'): \`agones-check -e prod,dev,beta-off\` on one file. $summary"
  echo
  echo "Median of 11 runs: $(echo "$med" | cut -d' ' -f1) s wall clock, $(( $(echo "$med" | cut -d' ' -f2) / 1024 )) MB peak RSS (includes process start)."
} >> bench/results/perf.md
tail -5 bench/results/perf.md
