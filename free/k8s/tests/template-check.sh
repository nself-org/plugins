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

# The install-time overlay (tests/overlay/install.yaml: domain, plugins.install,
# license.secretName, two plugin services) must render, validate, and apply the
# domain rewrite, the initdb Secret and mount, and the licence secretKeyRef.
g=testdata/full/golden
out="$(helm template nself charts/nself -f $g/values.yaml -f $g/secrets.yaml -f tests/overlay/install.yaml)" || { echo "FAIL: helm template with the install overlay" >&2; fail=1; out=""; }
if [ -n "$out" ]; then
  printf '%s\n' "$out" | kubeconform -strict -summary -kubernetes-version "$KUBERNETES_VERSION" - || fail=1
  expect() { printf '%s\n' "$out" | grep -qF -- "$1" || { echo "FAIL: overlay render lacks: $1" >&2; fail=1; }; }
  expect "- host: api.shop.example.org"
  expect "name: nself-postgres-initdb"
  expect "mountPath: /docker-entrypoint-initdb.d"
  expect "secretName: nself-postgres-initdb"
  expect "name: fake-licence-secret"
  # No Ingress host or TLS host may keep the generated base domain.
  if printf '%s\n' "$out" | grep -E '^ +(- )?host: [^ ]*example\.test$|^ +- [a-z-]+\.example\.test$'; then
    echo "FAIL: an ingress host kept the base domain" >&2; fail=1
  fi
fi

# plugins.install of an absent plugin fails with the run-this message.
if msg="$(helm template nself charts/nself -f $g/values.yaml -f $g/secrets.yaml --set 'plugins.install[0]=ghost' 2>&1)"; then
  echo "FAIL: plugins.install of an absent plugin rendered" >&2; fail=1
elif ! printf '%s' "$msg" | grep -q "plugin ghost is not in the generated values: run nself plugin install ghost, then nself k8s values"; then
  echo "FAIL: absent plugin gave the wrong message: $msg" >&2; fail=1
fi

# An initdb name without content in secrets.yaml fails, naming the file.
if msg="$(helm template nself charts/nself -f $g/values.yaml -f $g/secrets.yaml --set 'initdb.postgres=null' 2>&1)"; then
  echo "FAIL: initdb names without secrets content rendered" >&2; fail=1
elif ! printf '%s' "$msg" | grep -q "has no initdb content for 00-extensions.sql of service postgres"; then
  echo "FAIL: missing initdb content gave the wrong message: $msg" >&2; fail=1
fi

# A render without secrets.yaml must fail loudly, never emit empty Secrets.
if helm template nself charts/nself -f testdata/full/golden/values.yaml >/dev/null 2>&1; then
  echo "FAIL: render without secrets.yaml succeeded" >&2
  fail=1
fi

[ "$checked" -eq 2 ] || fail=1
[ "$fail" -eq 0 ] && echo "template-check: ok"
exit "$fail"
