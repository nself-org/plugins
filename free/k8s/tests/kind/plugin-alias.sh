#!/usr/bin/env bash
# Purpose: prove a dependent plugin reaches a bare compose service via plugin-* DNS.
# Inputs: kind, kubectl, helm and Docker; KIND_WORK is required scratch space.
# Outputs: exit 0 after DNS and HTTP succeed through both names; otherwise failure.
# Constraints: a dedicated throwaway cluster and namespace, always deleted on exit.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
k8s="$(cd "$here/../.." && pwd)"
# shellcheck disable=SC1091
. "$here/tools.env"
work="${KIND_WORK:?set KIND_WORK to an empty scratch directory}"
mkdir -p "$work"
export KUBECONFIG="$work/kubeconfig"
cluster="plugin-alias-$$"
cleanup() {
  rc=$?
  if [ "$rc" -ne 0 ]; then
    kubectl get pods,services -n plugin-alias -o wide >&2 || true
    kubectl describe pods -n plugin-alias >&2 || true
  fi
  kind delete cluster --name "$cluster"
  exit "$rc"
}
trap cleanup EXIT

kind create cluster --name "$cluster" --image "$KIND_NODE_IMAGE" --wait 180s
helm install aliases "$k8s/charts/nself" --namespace plugin-alias --create-namespace \
  -f "$here/fixture/plugin-alias.yaml" --wait --timeout 180s
for name in notify plugin-notify; do
  answer="$(kubectl exec -n plugin-alias deployment/dependent -- \
    sh -ec 'wget -qO- "$1"' sh "http://$name:8080")"
  [ "$answer" = notify-ok ] || { echo "FAIL: $name returned $answer" >&2; exit 1; }
done
answer="$(kubectl exec -n plugin-alias deployment/dependent -- \
  sh -ec 'wget -qO- "$PLUGIN_NOTIFY_INTERNAL_URL"')"
[ "$answer" = notify-ok ] || { echo "FAIL: dependency URL returned $answer" >&2; exit 1; }
echo 'plugin-alias: PASS, dependent PLUGIN_NOTIFY_INTERNAL_URL=http://plugin-notify:8080 returned notify-ok; bare notify also works'
