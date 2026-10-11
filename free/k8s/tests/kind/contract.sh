#!/usr/bin/env bash
# Purpose: the machine-contract checks of tests/kind/run.sh (P7-DEPL-07): the k8s subtree
#   `nself help --json k8s` lists, the v1 envelope of `nself k8s status --json`, the
#   uninstall confirmation gate, and a real uninstall. Sourced by run.sh after the upgrade.
# Inputs: the run.sh environment: the nself function, $project, $KUBECONFIG, jq on PATH.
# Outputs: functions that return non-zero (with a FAIL line on stderr) when a check fails.
# Constraints: only `nself k8s ...` touches the release; helm is never called here. jq reads
#   the envelopes, so the checks do not depend on key order or spacing.

# project and KUBECONFIG are set by run.sh before this file is sourced.
# shellcheck disable=SC2154

# fail <message>: report and return 1.
fail() { echo "FAIL: $*" >&2; return 1; }

# check_help_subtree: the cli with the plugin mount lists k8s and its five subcommands,
# every one with canon "plugin".
check_help_subtree() {
  local doc paths p missing=""
  doc="$(nself help --json k8s)"
  # Registry paths may or may not carry the leading "nself ".
  paths="$(printf '%s' "$doc" | jq -r '.data.commands[] | select(.canon == "plugin") | .path | sub("^nself "; "")')"
  echo "plugin-canon paths under k8s:"
  printf '%s\n' "$paths" | sed 's/^/  /'
  for p in "k8s" "k8s values" "k8s install" "k8s upgrade" "k8s status" "k8s uninstall"; do
    printf '%s\n' "$paths" | grep -qxF "$p" || missing="$missing [$p]"
  done
  [ -z "$missing" ] || fail "help --json k8s does not list with canon plugin:$missing"
}

# envelope_ok <json> <jq filter>: the document is a v1 envelope and the filter is true.
envelope_ok() {
  printf '%s' "$1" | jq -e '.schema_version == "1" and ('"$2"')' >/dev/null
}

# check_json_status: status --json is a v1 envelope whose data is the five-field summary.
check_json_status() {
  local out
  out="$(cd "$project" && nself k8s status --json --cluster "$KUBECONFIG")"
  printf '%s\n' "$out"
  envelope_ok "$out" '.command == "k8s status" and (has("error") | not) and .data.status == "deployed" and (.data | keys | length) == 5' ||
    fail "status --json is not a v1 envelope with data.status deployed"
}

# check_uninstall_gate: uninstall without --yes and without a terminal exits 4, in both
# modes, and the release survives.
check_uninstall_gate() {
  local out rc
  set +e
  out="$(cd "$project" && nself k8s uninstall --cluster "$KUBECONFIG" </dev/null 2>&1)"
  rc=$?
  set -e
  echo "$out"
  [ "$rc" -eq 4 ] || fail "uninstall without --yes exited $rc, want 4"
  set +e
  out="$(cd "$project" && nself k8s uninstall --json --cluster "$KUBECONFIG" </dev/null 2>/dev/null)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  [ "$rc" -eq 4 ] || fail "uninstall --json without --yes exited $rc, want 4"
  envelope_ok "$out" '.command == "k8s uninstall" and .error.code == "E403" and .error.class == "destructive_blocked" and .error.exit_code == 4' ||
    fail "the refusal is not an E403 envelope"
  check_json_status
}

# check_uninstall: uninstall --yes --json removes the release; status then reports E725.
check_uninstall() {
  local out rc
  out="$(cd "$project" && nself k8s uninstall --yes --json --cluster "$KUBECONFIG" 2>/dev/null)"
  printf '%s\n' "$out"
  envelope_ok "$out" '.command == "k8s uninstall" and .data.uninstalled == true' || fail "uninstall --yes --json did not report uninstalled"
  set +e
  out="$(cd "$project" && nself k8s status --json --cluster "$KUBECONFIG" 2>/dev/null)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  [ "$rc" -eq 1 ] || fail "status after uninstall exited $rc, want 1"
  envelope_ok "$out" '.error.code == "E725"' || fail "the release is still there: status did not report E725"
}
