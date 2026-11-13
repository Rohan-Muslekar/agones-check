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
  for f in testdata/backend/{ranked,casual,tournament}/*.yaml; do
    printf '\n---\n'
    sed -E "s/^([[:space:]]*(name|fleetName|agones.dev\/fleet): )(ranked-rooms|lobby-buffer|practice-weekend|tournament-teams|ranked|lobby|practice|tournament)$/\1\3-$i/" "$f"
  done
done > "$tmp/synth.yaml"
summary=$(./bin/agones-check -e prod,dev,beta-off --fail-on never "$tmp/synth.yaml" | tail -1)
# Prints "<seconds> <peak RSS in KB>" for one run. GNU time (Linux) has -f;
# BSD time (macOS) only has -l, which reports RSS in bytes.
measure() {
  if /usr/bin/time -f "%e %M" true >/dev/null 2>&1; then
    /usr/bin/time -f "%e %M" "$@" 2>&1 >/dev/null
  else
    /usr/bin/time -l "$@" 2>&1 >/dev/null |
      awk '/ real /{s=$1} /maximum resident set size/{k=int($1/1024)} END{print s, k}'
  fi
}
times=$(for _ in $(seq 1 11); do measure ./bin/agones-check -e prod,dev,beta-off --fail-on never "$tmp/synth.yaml"; done | sort -n)
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
