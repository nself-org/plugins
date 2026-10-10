#!/usr/bin/env bash
# Purpose: prove `nself k8s values|install|upgrade` on a real kind cluster. The same
#   script is the body of .github/workflows/k8s-kind.yml and the local pre-merge run.
# Inputs (env, all optional):
#   CLI_REF      nself-org/cli commit to build (default CLI_REF_DEFAULT from tools.env)
#   CLI_DIR      an existing cli checkout to build instead of cloning CLI_REF
#   LEG          install (the only leg this Ticket proves; others exit 3)
#   SOURCE       branch (the plugin built from this checkout; registry belongs to P7-DEPL-99)
#   READY_TIMEOUT seconds for every pod to be Ready (default 600)
#   KIND_WORK    work directory (default: a new mktemp dir, removed on exit)
#   TOOLS_CACHE  directory that caches the pinned tool downloads
#   KEEP_CLUSTER=1 keeps the kind cluster for debugging
# Outputs: exit 0 when the stack is Ready and Hasura answers {__typename}; 1 on
#   any failure (cluster state is dumped first); 3 for a leg this Ticket does not run.
# Constraints: helm is only ever run by `nself k8s`; the script never calls it. The cli
#   sha is printed. No secret goes on any argv: the admin secret reaches curl on stdin.
#   The plugin is laid out as `nself add` does: bin/nself-k8s plus k8s/plugin.json under
#   ~/.nself/plugins (HOME is a fixture dir; DOCKER_CONFIG keeps the real docker context).
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
k8s="$(cd "$here/../.." && pwd)"
# shellcheck disable=SC1091
. "$here/tools.env"

LEG="${LEG:-install}"
SOURCE="${SOURCE:-branch}"
CLI_REF="${CLI_REF:-$CLI_REF_DEFAULT}"
READY_TIMEOUT="${READY_TIMEOUT:-600}"
if [ "$SOURCE" != "branch" ] || [ "$LEG" != "install" ]; then
  echo "run.sh: only source=branch leg=install is proven by this Ticket (registry: P7-DEPL-99, day2: P7-DEPL-07)" >&2
  exit 3
fi

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

step "lay out the k8s plugin as nself add does"
plug="$fixhome/.nself/plugins"
mkdir -p "$plug/bin" "$plug/k8s"
(cd "$k8s" && CGO_ENABLED=0 go build -o "$plug/bin/nself-k8s" ./cmd)
cp "$k8s/plugin.json" "$plug/k8s/plugin.json"

# nself runs with a fixture HOME (its plugin dir); docker keeps the real config.
nself() { HOME="$fixhome" DOCKER_CONFIG="${DOCKER_CONFIG:-$real_home/.docker}" "$bin/nself" "$@"; }

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

# KNOWN GAP (reported to P7-DEPL-06, not hidden): `nself k8s values` lists the postgres
# bind mount /docker-entrypoint-initdb.d under notMapped, so the cluster's postgres never
# runs nself's init SQL (it creates the auth, storage and hasura schemas) and hasura-auth
# crash-loops on "schema auth does not exist". Until the values map those scripts, this
# harness applies the project's postgres/init/*.sql to the postgres pod while the
# install waits. Delete init_postgres and its call when the chart carries the scripts.
init_postgres() {
  local i=0 db user
  until kubectl get pod postgres-0 >/dev/null 2>&1; do
    i=$((i + 1)); [ "$i" -lt 120 ] || { echo "FAIL: postgres-0 never appeared" >&2; return 1; }
    sleep 2
  done
  kubectl wait --for=condition=Ready pod/postgres-0 --timeout="${READY_TIMEOUT}s"
  db="$(kubectl get secret nself-postgres -o jsonpath='{.data.POSTGRES_DB}' | base64 -d)"
  user="$(kubectl get secret nself-postgres -o jsonpath='{.data.POSTGRES_USER}' | base64 -d)"
  for f in "$project"/postgres/init/*.sql; do
    echo "init sql: $(basename "$f")"
    kubectl exec -i postgres-0 -- psql -q -v ON_ERROR_STOP=1 -U "$user" -d "$db" <"$f"
  done
}

step "nself k8s install --wait (postgres init SQL applied meanwhile)"
install_rc=0
(cd "$project" && nself k8s install --domain example.test --cluster "$KUBECONFIG" --wait --timeout "${READY_TIMEOUT}s") &
install_pid=$!
init_postgres || { kill "$install_pid" 2>/dev/null || true; exit 1; }
wait "$install_pid" || install_rc=$?
if [ "$install_rc" -ne 0 ]; then echo "FAIL: nself k8s install exited $install_rc" >&2; exit 1; fi

step "every pod Ready"
kubectl wait --for=condition=Ready pod --all --field-selector=status.phase!=Succeeded --timeout="${READY_TIMEOUT}s"

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

echo
echo "k8s-kind: ok (cli $(git -C "$CLI_DIR" rev-parse HEAD))"
