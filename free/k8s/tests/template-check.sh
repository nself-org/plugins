#!/usr/bin/env bash
# Purpose: render charts/nself with the values nself-k8s generates for each
# fixture (testdata/*/golden/{values,secrets}.yaml, which the Go tests prove
# equal to the generator output) through `helm lint` and `helm template`, and
# validate every manifest with kubeconform -strict.
# Inputs: tests/tools.env (pinned helm and kubeconform images); docker, or
#   HELM_BIN / KUBECONFORM_BIN pointing at local binaries.
# Outputs: exit 0 when every fixture renders and validates; 1 otherwise.
# Also checks that the chart carries no typed image reference and that a
# missing secrets.yaml fails the render instead of producing empty secrets.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
k8s="$(cd "$here/.." && pwd)"
# shellcheck disable=SC1091
. "$here/tools.env"

helm() {
  if [ -n "${HELM_BIN:-}" ]; then "$HELM_BIN" "$@"; return; fi
  docker run --rm -v "$k8s:/work:ro" -w /work "$HELM_IMAGE" "$@"
}
kubeconform() {
  if [ -n "${KUBECONFORM_BIN:-}" ]; then "$KUBECONFORM_BIN" "$@"; return; fi
  docker run --rm -i "$KUBECONFORM_IMAGE" "$@"
}

fail=0

# No hand-typed image reference in the chart.
if grep -rnE '(^|[^a-zA-Z.-])(image|repository):[[:space:]]*"?[a-z0-9./-]+[:@]' "$k8s/charts/nself"; then
  echo "FAIL: chart contains a typed image reference" >&2
  fail=1
fi

helm lint charts/nself -f charts/nself/ci/default-values.yaml >/dev/null || fail=1

checked=0
for fx in full interp; do
  g="testdata/$fx/golden"
  out="$(helm template nself charts/nself -f "$g/values.yaml" -f "$g/secrets.yaml")" || { echo "FAIL: helm template $fx" >&2; fail=1; continue; }
  n="$(printf '%s\n' "$out" | grep -c '^kind: ')"
  if [ "$n" -lt 3 ]; then echo "FAIL: $fx rendered only $n objects" >&2; fail=1; continue; fi
  printf '%s\n' "$out" | kubeconform -strict -summary -kubernetes-version "$KUBERNETES_VERSION" - || fail=1
  echo "$fx: $n objects rendered"
  checked=$((checked + 1))
done

# A render without secrets.yaml must fail loudly, never emit empty Secrets.
if helm template nself charts/nself -f testdata/full/golden/values.yaml >/dev/null 2>&1; then
  echo "FAIL: render without secrets.yaml succeeded" >&2
  fail=1
fi

[ "$checked" -eq 2 ] || fail=1
[ "$fail" -eq 0 ] && echo "template-check: ok"
exit "$fail"
