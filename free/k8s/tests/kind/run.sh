#!/usr/bin/env bash
# Purpose: prove `nself k8s values|install|upgrade|status|uninstall` on a real kind cluster. The same
#   script is the body of .github/workflows/k8s-kind.yml and the local pre-merge run.
# Inputs (env, all optional):
#   CLI_REF      nself-org/cli commit to build (default CLI_REF_DEFAULT from tools.env)
#   CLI_DIR      an existing cli checkout to build instead of cloning CLI_REF
#   LEG          install (values, install, upgrade, contract) or day2 (backup, delete the
#                namespace, reinstall, restore; P7-DEPL-08); anything else exits 3
#   SOURCE       branch (the plugin built from this checkout) or registry (the plugin `nself add k8s`
#                installs from the served registry, P7-DEPL-31; registry.sh holds the assertions)
#   READY_TIMEOUT seconds for every pod to be Ready (default 600)
#   KIND_WORK    work directory (default: a new mktemp dir, removed on exit)
#   TOOLS_CACHE  directory that caches the pinned tool downloads
#   KEEP_CLUSTER=1 keeps the kind cluster for debugging
# Outputs: leg day2: exit 0 when the restore verified the fingerprints and an independent
#   query agrees (day2.sh). Leg install: exit 0 when the stack is Ready, Hasura answers {__typename} and the machine
#   contract holds (contract.sh, needs jq: the help --json subtree with canon plugin, the
#   status --json envelope, the uninstall gate that exits 4 without --yes, and a real
#   uninstall --yes --json); 1 on any failure (cluster state is dumped first); 3 for an
#   unknown source or leg. Source registry first asserts, before any cluster exists, that the
#   installed plugin is the served row at the checkout's version with the chart-era commands, and
#   fails naming the served version and the missing commands; it never falls back to a branch build.
# Constraints: helm is only ever run by `nself k8s`; the script never calls it. The cli
#   sha is printed. No secret goes on any argv: the admin secret reaches curl on stdin.
#   Source branch lays the plugin out as `nself add` does: bin/nself-k8s plus k8s/plugin.json under
#   ~/.nself/plugins; source registry starts that dir empty and lets `nself add k8s` fill it (HOME
#   is a fixture dir; DOCKER_CONFIG keeps the real docker context). The registry URL is the
#   NSELF_PLUGIN_REGISTRY override or the default of the cli that was built.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
k8s="$(cd "$here/../.." && pwd)"
# shellcheck disable=SC1091
. "$here/tools.env"
# shellcheck disable=SC1091
. "$here/registry.sh"

LEG="${LEG:-install}"
SOURCE="${SOURCE:-branch}"
CLI_REF="${CLI_REF:-$CLI_REF_DEFAULT}"
READY_TIMEOUT="${READY_TIMEOUT:-600}"
if { [ "$SOURCE" != "branch" ] && [ "$SOURCE" != "registry" ]; } || { [ "$LEG" != "install" ] && [ "$LEG" != "day2" ]; }; then
  echo "run.sh: unknown source '$SOURCE' or leg '$LEG' (source branch|registry, leg install|day2)" >&2
  exit 3
fi

command -v jq >/dev/null || { echo "run.sh: jq is required (contract.sh reads the envelopes with it)" >&2; exit 2; }

real_home="$HOME"
made_work=0
if [ -z "${KIND_WORK:-}" ]; then
  KIND_WORK="$(mktemp -d "${TMPDIR:-/tmp}/nself-k8s-kind.XXXXXX")"
  made_work=1
fi
mkdir -p "$KIND_WORK"
bin="$KIND_WORK/bin"
fixhome="$KIND_WORK/home"
project="$KIND_WORK/project"
# A caller-supplied KIND_WORK may hold an earlier run: start from a fresh fixture, or the
# "refuses without generated values" check would see stale values.
rm -rf "$project" "$fixhome"
export KUBECONFIG="$KIND_WORK/kubeconfig"
cluster="nself-k8s-$$"
pf_pid=""
cluster_up=0
mkdir -p "$bin" "$fixhome" "$project"

dump() {
  echo "---- cluster state ----" >&2
  "$bin/kubectl" get nodes,pods,svc,ingress -A -o wide >&2 || true
  "$bin/kubectl" describe pods >&2 || true
  for p in $("$bin/kubectl" get pods -o name 2>/dev/null); do
    echo "---- logs $p ----" >&2
    "$bin/kubectl" logs "$p" --all-containers --tail=40 >&2 || true
  done
}
cleanup() {
  rc=$?
  if [ "$rc" -ne 0 ] && [ "$cluster_up" = 1 ]; then dump; fi
  [ -n "$pf_pid" ] && kill "$pf_pid" 2>/dev/null || true
  if [ "${KEEP_CLUSTER:-0}" != "1" ]; then "$bin/kind" delete cluster --name "$cluster" >/dev/null 2>&1 || true; fi
  if [ "$made_work" = 1 ] && [ "${KEEP_CLUSTER:-0}" != "1" ]; then rm -rf "$KIND_WORK"; fi
  exit "$rc"
}
trap cleanup EXIT

step() { echo; echo "==> $*"; }

step "tools (pinned, checksum-verified)"
TOOLS_CACHE="${TOOLS_CACHE:-$KIND_WORK/cache}" bash "$here/tools.sh" "$bin"
export PATH="$bin:$PATH"

step "build nself from nself-org/cli at $CLI_REF"
if [ -z "${CLI_DIR:-}" ]; then
  CLI_DIR="$KIND_WORK/cli"
  mkdir -p "$CLI_DIR"
  git -C "$CLI_DIR" init -q
  git -C "$CLI_DIR" fetch -q --depth 1 https://github.com/nself-org/cli "$CLI_REF"
  git -C "$CLI_DIR" checkout -q FETCH_HEAD
fi
echo "cli sha: $(git -C "$CLI_DIR" rev-parse HEAD)"
# cli pins a newer Go than the runner image; let the go command fetch that toolchain.
(cd "$CLI_DIR" && GOTOOLCHAIN="${GOTOOLCHAIN_CLI:-auto}" CGO_ENABLED=0 go build -o "$bin/nself" ./cmd/nself)

plug="$fixhome/.nself/plugins"

# nself runs with a fixture HOME (its plugin dir); docker keeps the real config.
nself() { HOME="$fixhome" DOCKER_CONFIG="${DOCKER_CONFIG:-$real_home/.docker}" "$bin/nself" "$@"; }

if [ "$SOURCE" = "registry" ]; then
  # Only the checkout's plugin.json version is read from free/k8s; nothing is built from it.
  checkout_version="$(jq -r '.version' "$k8s/plugin.json")"
  # Both the fetch below and the cli read this registry; the cli falls back to another host
  # only when this one fails.
  NSELF_PLUGIN_REGISTRY="$(registry_url "$CLI_DIR")"
  export NSELF_PLUGIN_REGISTRY
  step "registry leg: the served k8s row at $NSELF_PLUGIN_REGISTRY (checkout $checkout_version)"
  registry_fetch "$NSELF_PLUGIN_REGISTRY" "$KIND_WORK/registry.json"
  served_version="$(registry_served_version "$KIND_WORK/registry.json" k8s)"
  case "$(uname -m)" in x86_64 | amd64) plat_arch=amd64 ;; *) plat_arch=arm64 ;; esac
  plat="$(uname -s | tr '[:upper:]' '[:lower:]')-$plat_arch"
  echo "served k8s: $served_version; platform checksum ($plat): $(registry_platform_checksum "$KIND_WORK/registry.json" k8s "$plat")"
  assert_served_matches_checkout "$served_version" "$checkout_version"

  step "nself add k8s into an empty plugin dir"
  assert_empty_plugin_dir "$plug"
  cp "$here/fixture/project.env" "$project/.env"
  (cd "$project" && nself add k8s </dev/null)
  test -f "$plug/k8s/plugin.json" || { echo "FAIL: registry leg: nself add k8s did not install $plug/k8s/plugin.json" >&2; exit 1; }
  assert_installed_version "$plug/k8s/plugin.json" "$served_version" "$checkout_version"
  nself help --json k8s >"$KIND_WORK/help-k8s.json"
  assert_chart_era "$KIND_WORK/help-k8s.json" "$served_version"
  echo "registry leg: cli $(git -C "$CLI_DIR" rev-parse HEAD) proved served k8s $served_version"
else
  step "lay out the k8s plugin as nself add does"
  mkdir -p "$plug/bin" "$plug/k8s"
  (cd "$k8s" && CGO_ENABLED=0 go build -o "$plug/bin/nself-k8s" ./cmd)
  cp "$k8s/plugin.json" "$plug/k8s/plugin.json"
fi

step "fixture project: nself build"
cp "$here/fixture/project.env" "$project/.env"
(cd "$project" && nself build)

step "install refuses without generated values"
set +e
msg="$(cd "$project" && nself k8s install --domain example.test --cluster "$KUBECONFIG" 2>&1)"
rc=$?
set -e
echo "$msg"
if [ "$rc" -eq 0 ] || ! printf '%s' "$msg" | grep -q "run nself k8s values"; then
  echo "FAIL: install without values must exit non-zero with 'run nself k8s values' (rc=$rc)" >&2
  exit 1
fi

step "nself k8s values"
(cd "$project" && nself k8s values)
test -s "$project/.nself/generated/k8s/values.yaml"
test -s "$project/.nself/generated/k8s/secrets.yaml"

step "kind cluster"
kind create cluster --name "$cluster" --image "$KIND_NODE_IMAGE" --kubeconfig "$KUBECONFIG" --wait 180s
cluster_up=1
if [ "$LEG" = "day2" ]; then
  # A dedicated namespace that the day2 leg deletes and recreates; helm and kubectl use the
  # kubeconfig context's namespace, so every later command lands there.
  kubectl create namespace nself-day2
  kubectl config set-context --current --namespace nself-day2
fi

# shellcheck disable=SC1091
. "$here/stack.sh"
install_stack

if [ "$LEG" = "day2" ]; then
  # shellcheck disable=SC1091
  . "$here/day2.sh"
  run_day2
  echo
  echo "k8s-kind: ok, day2, source $SOURCE (cli $(git -C "$CLI_DIR" rev-parse HEAD))"
  exit 0
fi

query_hasura() {
  local port=18080 secret out rc deadline
  secret="$(kubectl get secret nself-hasura -o jsonpath='{.data.HASURA_GRAPHQL_ADMIN_SECRET}' | base64 -d)"
  kubectl port-forward svc/hasura "$port:8080" >"$KIND_WORK/port-forward.log" 2>&1 &
  pf_pid=$!
  deadline=$((SECONDS + 120))
  while :; do
    set +e
    out="$(printf 'header = "x-hasura-admin-secret: %s"\n' "$secret" |
      curl -s --max-time 20 -K - -H 'Content-Type: application/json' \
        -d '{"query":"{__typename}"}' "http://127.0.0.1:$port/v1/graphql")"
    rc=$?
    set -e
    # Retry only while the port-forward is not listening yet (connection refused).
    if [ "$rc" -eq 7 ] && [ "$SECONDS" -lt "$deadline" ]; then sleep 2; continue; fi
    break
  done
  kill "$pf_pid" 2>/dev/null || true
  pf_pid=""
  if [ "$rc" -ne 0 ] || [ "$out" != '{"data":{"__typename":"query_root"}}' ]; then
    echo "FAIL: GraphQL answer was rc=$rc body=$out" >&2
    return 1
  fi
  echo "graphql: $out"
}

step "GraphQL through kubectl port-forward"
query_hasura

step "nself k8s upgrade --wait, then the same checks"
(cd "$project" && nself k8s upgrade --domain example.test --cluster "$KUBECONFIG" --wait --timeout "${READY_TIMEOUT}s")
status="$(cd "$project" && nself k8s status --cluster "$KUBECONFIG")"
printf '%s\n' "$status" | grep -q '"version":2' || { echo "FAIL: release is not at revision 2" >&2; exit 1; }
kubectl wait --for=condition=Ready pod --all --field-selector=status.phase!=Succeeded --timeout="${READY_TIMEOUT}s"
query_hasura

# shellcheck disable=SC1091
. "$here/contract.sh"

step "nself help --json k8s lists the subtree with canon plugin"
check_help_subtree

step "nself k8s status --json is a v1 envelope"
check_json_status

step "nself k8s uninstall refuses without --yes (exit 4), the release survives"
check_uninstall_gate

step "nself k8s uninstall --yes --json removes the release"
check_uninstall

echo
echo "k8s-kind: ok, source $SOURCE (cli $(git -C "$CLI_DIR" rev-parse HEAD))"
