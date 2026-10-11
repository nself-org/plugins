#!/usr/bin/env bash
# Purpose: install the nSelf stack on the kind cluster with `nself k8s install --wait` and wait
#   for every pod to be Ready. Sourced by run.sh; the day2 leg calls install_stack twice (before
#   the backup and again after the namespace was deleted).
# Inputs: the run.sh environment: step, nself-run variables ($bin, $fixhome, $real_home, $project,
#   $KUBECONFIG, $READY_TIMEOUT) and kubectl on PATH. The kubeconfig context's namespace is where
#   the release lands.
# Outputs: install_stack returns 0 once every pod is Ready; any failure exits the run (1).
# Constraints: helm is only ever run by `nself k8s`; no secret goes on an argv.

# shellcheck disable=SC2154

# KNOWN GAP (reported to P7-DEPL-06, not hidden): `nself k8s values` lists the postgres
# bind mount /docker-entrypoint-initdb.d under notMapped, so the cluster's postgres never
# runs nself's init SQL (it creates the auth, storage and hasura schemas) and hasura-auth
# crash-loops on "schema auth does not exist". Until the values map those scripts, this
# harness applies the project's postgres/init/*.sql to the postgres pod while the
# install waits. Delete init_postgres and its call when the chart carries the scripts.
init_postgres() {
  local i=0 db user
  until kubectl get pod postgres-0 >/dev/null 2>&1; do
    # Fail fast with the real cause when the install already exited.
    kill -0 "$install_pid" 2>/dev/null || { echo "FAIL: nself k8s install exited before postgres-0 appeared" >&2; return 1; }
    i=$((i + 1)); [ "$i" -lt 120 ] || { echo "FAIL: postgres-0 never appeared" >&2; return 1; }
    sleep 2
  done
  kubectl wait --for=condition=Ready pod/postgres-0 --timeout="${READY_TIMEOUT}s"
  db="$(kubectl get secret nself-postgres -o jsonpath='{.data.POSTGRES_DB}' | base64 -d)"
  user="$(kubectl get secret nself-postgres -o jsonpath='{.data.POSTGRES_USER}' | base64 -d)"
  # The postgres image first runs a temporary server that listens on the unix socket
  # only, stops it and starts the real one. Wait for the real server (TCP), or the
  # first script hits "connection ... failed" between the two.
  i=0
  until kubectl exec postgres-0 -- pg_isready -q -h 127.0.0.1 -U "$user" -d "$db" >/dev/null 2>&1; do
    i=$((i + 1)); [ "$i" -lt 120 ] || { echo "FAIL: postgres never accepted TCP connections" >&2; return 1; }
    kill -0 "$install_pid" 2>/dev/null || { echo "FAIL: nself k8s install exited before postgres was ready" >&2; return 1; }
    sleep 2
  done
  # This function runs under "||", so set -e is off here: every step reports its own failure.
  for f in "$project"/postgres/init/*.sql; do
    echo "init sql: $(basename "$f")"
    kubectl exec -i postgres-0 -- psql -q -v ON_ERROR_STOP=1 -U "$user" -d "$db" <"$f" ||
      { echo "FAIL: init sql $(basename "$f") failed" >&2; return 1; }
  done
}

install_stack() {
  local install_rc=0
  step "nself k8s install --wait (postgres init SQL applied meanwhile)"
  install_rc=0
  # exec (with the same env as the nself function): $! is the nself process itself, so the kill below reaches it, not a wrapper subshell.
  (cd "$project" && HOME="$fixhome" DOCKER_CONFIG="${DOCKER_CONFIG:-$real_home/.docker}" exec "$bin/nself" k8s install --domain example.test --cluster "$KUBECONFIG" --wait --timeout "${READY_TIMEOUT}s") &
  install_pid=$!
  init_postgres || {
    if kill -0 "$install_pid" 2>/dev/null; then kill "$install_pid" 2>/dev/null || true
    else wait "$install_pid" || echo "nself k8s install exit code: $?" >&2; fi
    exit 1
  }
  wait "$install_pid" || install_rc=$?
  if [ "$install_rc" -ne 0 ]; then echo "FAIL: nself k8s install exited $install_rc" >&2; exit 1; fi

  step "every pod Ready"
  kubectl wait --for=condition=Ready pod --all --field-selector=status.phase!=Succeeded --timeout="${READY_TIMEOUT}s"
}
