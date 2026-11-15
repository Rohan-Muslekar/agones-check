#!/usr/bin/env bash
# Runs agones-check on public Agones manifests from GitHub, pinned to the
# commits in testdata/realworld/corpus.tsv (repo<TAB>commit<TAB>path; found
# with GitHub code search for "agones.dev/v1", "autoscaling.agones.dev/v1"
# and "allocation.agones.dev/v1" in .yaml/.yml files, forks and Agones itself
# excluded). Each directory is checked as one deployment, without recursing
# into subdirectories. Writes bench/results/realworld-findings.tsv.
set -euo pipefail
cd "$(dirname "$0")/.."
go build -o bin/agones-check ./cmd/agones-check
work=${WORK:-$(mktemp -d)}
while IFS=$'\t' read -r repo sha path; do
  dst="$work/files/${repo//\//_}/$path"
  [ -s "$dst" ] && continue
  mkdir -p "$(dirname "$dst")"
  curl -fsSL "https://raw.githubusercontent.com/$repo/$sha/${path// /%20}" -o "$dst" || echo "download failed: $repo $path" >&2
done < testdata/realworld/corpus.tsv
out=bench/results/realworld-findings.tsv
printf 'directory\tstatus\trule\tseverity\tfile\tline\tmessage\n' > "$out"
find "$work/files" -type f \( -name '*.yaml' -o -name '*.yml' \) -printf '%h\n' | sort -u | while read -r d; do
  mapfile -d '' fs < <(find "$d" -maxdepth 1 -type f \( -name '*.yaml' -o -name '*.yml' \) -print0)
  rel=${d#"$work/files/"}
  if ! json=$(./bin/agones-check --fail-on never -o json "${fs[@]}" 2>/dev/null); then
    printf '%s\tunparsable (Helm template or invalid YAML)\t\t\t\t\t\n' "$rel" >> "$out"; continue
  fi
  printf '%s' "$json" | python3 -c '
import json, sys
rel, work = sys.argv[1], sys.argv[2] + "/files/"
j = json.load(sys.stdin)
n = len(j.get("objects") or [])
fs = j.get("findings") or []
if not fs:
    print("%s\tclean (%d objects)\t\t\t\t\t" % (rel, n))
for f in fs:
    s = f["source"]
    print("\t".join([rel, "%d objects" % n, f["rule"], f["severity"], s["file"].replace(work, ""), str(s["line"]), f["message"]]))
' "$rel" "$work" >> "$out"
done
echo "wrote $out"
