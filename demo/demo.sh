#!/usr/bin/env bash
# Live demo for the agones-check presentation (about 2.5 minutes).
# Run it from anywhere (it changes into demo/ itself). Press Enter to move to the next step.
#
# Needs: agones-check on PATH (or AGONES_CHECK=/path/to/binary), and for step 3
# the kind cluster "agones-check" with Agones 1.61.0 running.
#   go install github.com/Rohan-Muslekar/agones-check/cmd/agones-check@v0.1.0
#   KIND_EXPERIMENTAL_PROVIDER=podman kind get kubeconfig --name agones-check > kubeconfig
set -u
cd "$(dirname "$0")"
AC=${AGONES_CHECK:-agones-check}
export KUBECONFIG="$PWD/kubeconfig"

kubectl get ns game >/dev/null 2>&1 || kubectl create ns game >/dev/null 2>&1 || true

step() { printf '\n\033[1;36m# %s\033[0m\n' "$1"; read -r -p "$ $2"; echo; eval "$2"; }

step "1. The ranked game mode as it ships today: clean in prod and dev" \
  "$AC -e prod,dev good"

step "2. Three one-word mistakes (see: diff -r good broken)" \
  "diff -r good broken | grep '^[<>]'"

step "3. What the cluster says today: kubectl dry run against prod" \
  "kubectl apply --dry-run=server -f broken/fleet.yaml; kubectl create --dry-run=server -f broken/allocation.yaml -o name"

step "4. agones-check, offline, both environments" \
  "$AC -e prod,dev broken"

step "5. Which object works where, including the older 1.40 release" \
  "$AC -e prod,dev,1.40 -o matrix broken"

step "6. The same findings as GitHub pull-request annotations" \
  "$AC -o github broken 2>/dev/null | grep '^::'"
