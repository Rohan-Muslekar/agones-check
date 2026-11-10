#!/usr/bin/env bash
# Runtime ground truth on a real cluster with Agones installed (kind is fine).
# For each seeded mistake it records (1) whether `kubectl --dry-run=server`
# accepts it, (2) what the cluster actually does after it is applied, and
# (3) what agones-check says. Writes bench/results/e2e.md.
#
#   KUBECONFIG=/path/to/kind.kubeconfig ./scripts/e2e-runtime.sh
set -euo pipefail
cd "$(dirname "$0")/.."
D=testdata/e2e
NS=e2e
OUT=bench/results/e2e.md
WAIT=${WAIT:-45}
go build -o bin/agones-check ./cmd/agones-check

kubectl get ns "$NS" >/dev/null 2>&1 || kubectl create ns "$NS" >/dev/null
# The Agones chart creates the SDK service account only in the namespaces
# listed in gameservers.namespaces; game server pods in $NS need it too.
kubectl -n "$NS" get sa agones-sdk >/dev/null 2>&1 || kubectl -n "$NS" create sa agones-sdk >/dev/null
kubectl -n "$NS" get rolebinding agones-sdk-access >/dev/null 2>&1 ||
  kubectl -n "$NS" create rolebinding agones-sdk-access --clusterrole=agones-sdk --serviceaccount="$NS:agones-sdk" >/dev/null
kubectl -n "$NS" delete fleetautoscalers --all --wait >/dev/null 2>&1 || true
kubectl apply -f "$D/fleet.yaml" >/dev/null
# Replace servers that failed before the service account existed.
kubectl -n "$NS" delete gs -l agones.dev/fleet=arena --field-selector status.state=Error >/dev/null 2>&1 || true
wait_ready() {
  echo "waiting for fleet arena to be Ready..."
  for _ in $(seq 1 60); do
    ready=$(kubectl -n "$NS" get fleet arena -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo 0)
    [ "${ready:-0}" -ge 2 ] && break
    sleep 3
  done
  kubectl -n "$NS" get fleet arena
}
wait_ready

dryrun() { # file verb
  if out=$(kubectl "$2" --dry-run=server -f "$1" 2>&1); then echo "accepted"; else echo "rejected: $(echo "$out" | tail -1 | cut -c1-120)"; fi
}
tool() { # files...
  local out; out=$(./bin/agones-check -e prod --fail-on never --min-severity warning "$@" | grep -E "ERROR|WARNING" | head -1 | sed -E 's/.*(ERROR|WARNING) (AC[0-9]+).*/\1 \2/' || true)
  echo "${out:-no finding}"
}
events() { # object name
  kubectl -n "$NS" get events --field-selector "involvedObject.name=$1" -o jsonpath='{range .items[*]}{.reason}: {.message}{"\n"}{end}' 2>/dev/null | sort | uniq | head -2 | tr '\n' ' ' | cut -c1-160
}

{
echo "# Runtime ground truth"
echo
echo "Generated $(date '+%Y-%m-%d %H:%M %Z') by \`scripts/e2e-runtime.sh\` against $(kubectl config current-context) (Agones $(helm list -n agones-system -o json 2>/dev/null | sed -E 's/.*\"app_version\":\"([^\"]+)\".*/\1/'), Kubernetes $(kubectl version -o json | sed -nE 's/.*"gitVersion": "([^"]+)".*/\1/p' | tail -1))."
echo
echo "| Case | kubectl --dry-run=server | What the cluster did | agones-check (prod) |"
echo "|---|---|---|---|"
} > "$OUT"

for c in fas-fleetname-typo:typo-fleet fas-counter-key-typo:typo-counter fas-list-default-capacity:list-capacity fas-nested-schedule:nested-bad fas-schedule-no-end:no-end; do
  f=${c%%:*}; name=${c##*:}
  dr=$(dryrun "$D/$f.yaml" apply)
  before=$(kubectl -n "$NS" get fleet arena -o jsonpath='{.spec.replicas}')
  kubectl apply -f "$D/$f.yaml" >/dev/null 2>&1 || true
  sleep "$WAIT"
  able=$(kubectl -n "$NS" get fleetautoscaler "$name" -o jsonpath='{.status.ableToScale}/{.status.scalingLimited}/{.status.desiredReplicas}' 2>/dev/null || echo "-")
  after=$(kubectl -n "$NS" get fleet arena -o jsonpath='{.spec.replicas}')
  ev=$(events "$name")
  t=$(tool "$D/fleet.yaml" "$D/$f.yaml")
  echo "| $f | $dr | ableToScale/limited/desired=$able; fleet replicas $before -> $after; events: ${ev:-none} | $t |" >> "$OUT"
  kubectl delete -f "$D/$f.yaml" --wait >/dev/null 2>&1 || true
  kubectl -n "$NS" scale fleet arena --replicas=2 >/dev/null
  sleep 5
done

wait_ready
for f in gsa-label-typo gsa-counter-typo gsa-control; do
  dr=$(dryrun "$D/$f.yaml" create)
  state=$(kubectl create -f "$D/$f.yaml" -o jsonpath='{.status.state}' 2>&1 | tail -1 | cut -c1-100)
  t=$(tool "$D/fleet.yaml" "$D/$f.yaml")
  echo "| $f | $dr | allocation state: $state | $t |" >> "$OUT"
done
# Release allocated servers so the fleet returns to its normal size.
kubectl -n "$NS" delete gs -l agones.dev/fleet=arena --field-selector status.state=Allocated >/dev/null 2>&1 || true

cat "$OUT"
