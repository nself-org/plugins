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

# The chart carries the project's postgres/init scripts (D-0308): `nself k8s values` maps the
# /docker-entrypoint-initdb.d bind mount, so the cluster's postgres runs nself's init SQL
# (auth, storage and hasura schemas) on its own and hasura-auth starts without a hand-applied step.
install_stack() {
  local install_rc=0
  step "nself k8s install --wait"
  (cd "$project" && HOME="$fixhome" DOCKER_CONFIG="${DOCKER_CONFIG:-$real_home/.docker}" "$bin/nself" k8s install --domain example.test --cluster "$KUBECONFIG" --wait --timeout "${READY_TIMEOUT}s") || install_rc=$?
  if [ "$install_rc" -ne 0 ]; then echo "FAIL: nself k8s install exited $install_rc" >&2; exit 1; fi

  step "every pod Ready"
  kubectl wait --for=condition=Ready pod --all --field-selector=status.phase!=Succeeded --timeout="${READY_TIMEOUT}s"
}
