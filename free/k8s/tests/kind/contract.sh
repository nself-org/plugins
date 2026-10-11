#!/usr/bin/env bash
# Purpose: the machine-contract checks of tests/kind/run.sh (P7-DEPL-07): the k8s subtree
#   `nself help --json k8s` lists, the v1 envelope of `nself k8s status --json`, the
#   uninstall confirmation gate, a real uninstall that keeps the PersistentVolumeClaims and
#   the data in them (D-0376), and a reinstall with the same release name that adopts them.
#   Sourced by run.sh after the upgrade.
# Inputs: the run.sh environment: the nself function, $project, $KUBECONFIG, $READY_TIMEOUT,
#   jq and kubectl on PATH.
# Outputs: functions that return non-zero (with a FAIL line on stderr) when a check fails.
# Constraints: only `nself k8s ...` touches the release; helm is never called here. kubectl
#   only reads claims and writes one marker row through psql on stdin (no secret on argv).
#   jq reads the envelopes, so the checks do not depend on key order or spacing.

# project and KUBECONFIG are set by run.sh before this file is sourced.
# shellcheck disable=SC2154

# fail <message>: report and return 1.
fail() { echo "FAIL: $*" >&2; return 1; }

# check_help_subtree: the cli with the plugin mount lists k8s and its seven subcommands,
# every one with canon "plugin".
check_help_subtree() {
  local doc paths p missing=""
  doc="$(nself help --json k8s)"
  # Registry paths may or may not carry the leading "nself ".
  paths="$(printf '%s' "$doc" | jq -r '.data.commands[] | select(.canon == "plugin") | .path | sub("^nself "; "")')"
  echo "plugin-canon paths under k8s:"
  printf '%s\n' "$paths" | sed 's/^/  /'
  for p in "k8s" "k8s values" "k8s install" "k8s upgrade" "k8s status" "k8s uninstall" "k8s backup" "k8s restore"; do
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

# release_claims: "name=uid" of every PersistentVolumeClaim of the default release, sorted.
release_claims() {
  kubectl get pvc -l app.kubernetes.io/instance=nself \
    -o jsonpath='{range .items[*]}{.metadata.name}={.metadata.uid}{"\n"}{end}' | sort
}

# psql_marker <sql>: run sql in the postgres pod as the release's database user (stdin, -At).
psql_marker() {
  local db user
  db="$(kubectl get secret nself-postgres -o jsonpath='{.data.POSTGRES_DB}' | base64 -d)"
  user="$(kubectl get secret nself-postgres -o jsonpath='{.data.POSTGRES_USER}' | base64 -d)"
  printf '%s\n' "$1" | kubectl exec -i postgres-0 -- psql -q -At -v ON_ERROR_STOP=1 -U "$user" -d "$db"
}

# check_uninstall: uninstall --yes --json removes the release and status then reports E725,
# but every PersistentVolumeClaim (same uid) and a marker row written before survive
# (helm.sh/resource-policy: keep). A reinstall with the same release name adopts the claims
# and reads the marker back.
check_uninstall() {
  local out rc before after marker
  before="$(release_claims)"
  printf 'claims before uninstall:\n%s\n' "$before"
  [ -n "$before" ] || { fail "the release has no PersistentVolumeClaims to keep"; return 1; }
  marker="d0376-$$-$RANDOM"
  psql_marker "CREATE TABLE IF NOT EXISTS public.k8s_kind_keep (v text); INSERT INTO public.k8s_kind_keep VALUES ('$marker');" ||
    { fail "could not write the marker row"; return 1; }

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

  after="$(release_claims)"
  printf 'claims after uninstall:\n%s\n' "$after"
  [ "$after" = "$before" ] || { fail "uninstall deleted or replaced PersistentVolumeClaims"; return 1; }

  # Reinstall under the same release name: helm adopts the kept claims (same uid).
  (cd "$project" && nself k8s install --domain example.test --cluster "$KUBECONFIG" --wait --timeout "${READY_TIMEOUT}s") ||
    { fail "reinstall with the same release name failed (kept claims not adopted)"; return 1; }
  check_json_status || return 1
  after="$(release_claims)"
  [ "$after" = "$before" ] || { fail "reinstall replaced the kept PersistentVolumeClaims"; return 1; }
  kubectl wait --for=condition=Ready pod/postgres-0 --timeout="${READY_TIMEOUT}s"
  out="$(psql_marker "SELECT v FROM public.k8s_kind_keep WHERE v = '$marker';")" ||
    { fail "could not read the marker row after the reinstall"; return 1; }
  [ "$out" = "$marker" ] || fail "the marker row did not survive uninstall and reinstall (got '$out')"
}
