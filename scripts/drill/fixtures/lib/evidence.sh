#!/usr/bin/env bash
# Shared by the acceptance scenario drills (P7-CI-47); sourced, never run.
#
# Purpose: record one check per scenario step and print ONE ci.evidence/v1 document on the last line of stdout, the
#   way scripts/drill/README.md describes (a drill's exit code is its result; the nested document gives every step its own
#   check). Modelled on scripts/drill/toolchain-pins/run.
# Inputs:  EV_JOB (the drill directory name), EV_SCRIPT (path of the drill's `run`, hashed as the config digest),
#          EV_ROOT (the repository under test), EV_WORK (a private scratch directory), EV_TOOLS (JSON object of tool versions).
# Outputs: ev_pass / ev_fail record a check; ev_finish prints the document and returns 0 when no check failed.
# Constraints: bash 3.2 compatible (macOS); jq is the only JSON writer; no secret ever enters an excerpt.

EV_TOOLS=${EV_TOOLS:-"{}"}

ev_sha() { sha256sum "$@" 2>/dev/null || shasum -a 256 "$@"; }
say() { printf '%s: %s\n' "${EV_JOB:-drill}" "$*" >&2; }

# ev_snip FILE: the line that says why a command died (a Go crash or panic first), then the last two lines; one line, short.
ev_snip() {
	{
		grep -m1 -E 'fatal error|^panic|SIG[A-Z]+|^runtime:|^E[0-9]{3}' "$1" 2>/dev/null | cut -c1-160
		tail -n 2 "$1" 2>/dev/null | cut -c1-100
	} | tr '\n' ' '
}

ev_begin() { # ev_begin JOB SCRIPT ROOT WORK
	EV_JOB=$1 EV_SCRIPT=$2 EV_ROOT=$3 EV_WORK=$4
	EV_CHECKS=$EV_WORK/checks.ndjson
	: >"$EV_CHECKS"
	EV_STARTED=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	EV_T0=$(date +%s)
}

# ev_check ID RESULT EXCERPT [FINDINGS]: ID is the step name; the check id is <job-prefix>.<ID>.
ev_check() {
	local id=$1 result=$2 excerpt=$3 findings=${4:-0}
	say "$result $id: $(printf '%s' "$excerpt" | head -n 1 | cut -c1-200)"
	jq -nc --arg id "$EV_JOB.$id" --arg job "$EV_JOB" --arg result "$result" --argjson findings "$findings" --arg excerpt "$excerpt" '
		{id: $id, job_id: $job, kind: "drill", tool: "bash", tool_version: null, required: true, substantive: true,
		 result: $result, findings: $findings, excerpt: $excerpt[0:4000], log_ref: null}' >>"$EV_CHECKS"
}
ev_pass() { ev_check "$1" pass "$2" 0; }
ev_fail() { ev_check "$1" fail "$2" 1; }

# ev_expect ID DESCRIPTION CONDITION-EXIT-CODE [DETAIL]: pass when the exit code given is 0.
ev_expect() {
	if [ "$3" -eq 0 ]; then ev_pass "$1" "$2"; else ev_fail "$1" "$2 -- ${4:-failed}"; fi
}

ev_finish() {
	local finished revision first repo_id dirty trigger platform cfg
	finished=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	revision=$(git -C "$EV_ROOT" rev-parse HEAD 2>/dev/null || true)
	[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || revision=0000000000000000000000000000000000000000
	first=$(git -C "$EV_ROOT" rev-list --max-parents=0 HEAD 2>/dev/null | tail -1 || true)
	repo_id="local:$(printf '%s' "${first:-none}" | ev_sha | cut -d' ' -f1)"
	dirty=false
	[ -n "$(git -C "$EV_ROOT" status --porcelain --untracked-files=no 2>/dev/null || true)" ] && dirty=true
	case "${GITHUB_EVENT_NAME:-}" in schedule | push | pull_request) trigger=$GITHUB_EVENT_NAME ;; workflow_dispatch) trigger=manual ;; *) trigger=local ;; esac
	platform=$(uname -s | tr '[:upper:]' '[:lower:]')/$(uname -m | sed -e s/x86_64/amd64/ -e s/aarch64/arm64/)
	cfg=$(ev_sha "$EV_SCRIPT" | cut -d' ' -f1)
	jq -sc --arg revision "$revision" --arg repo "$repo_id" --arg run "${GITHUB_RUN_ID:-$EV_JOB-$EV_T0}" --arg trigger "$trigger" \
		--arg cfg "$cfg" --argjson dirty "$dirty" --arg platform "$platform" --arg host "$(hostname)" --arg job "$EV_JOB" \
		--arg provider "$([ -n "${GITHUB_RUN_ID:-}" ] && echo github || echo local)" --argjson tools "$EV_TOOLS" \
		--arg started "$EV_STARTED" --arg finished "$finished" --argjson ms "$((($(date +%s) - EV_T0) * 1000))" '
		. as $checks
		| ($checks | map(select(.result == "fail")) | length) as $fail
		| {
		  schema: "ci.evidence/v1", revision: $revision, repo_id: $repo, run_id: $run,
		  pipeline: {id: "drills", name: "drills", kind: "drill"}, job: $job,
		  trigger: $trigger, source_trust: "owner", policy_digest: "none",
		  input_digests: {config: $cfg, generated: [], lockfiles: []},
		  worktree_state: (if $dirty then "dirty" else "clean" end), tracked_changes_digest: null,
		  binding: (if $dirty then {state: "unbound", reason: "dirty"} else {state: "bound", reason: null} end),
		  runner: {id: $host, provider: $provider, platform: $platform, isolation: "process"},
		  producer: {kind: "self-reported", version: ($job + "/1")},
		  timings: {queued_at: null, started_at: $started, finished_at: $finished, duration_ms: $ms},
		  tool_versions: $tools,
		  checks: $checks, artifacts: [], retries: [],
		  selection: {mode: "full", reason: "selection.full_required", digest: "none"},
		  result: (if $fail > 0 then "fail" else "pass" end),
		  counts: {pass: ($checks | map(select(.result == "pass")) | length), fail: $fail, skip: 0, error: 0, total: ($checks | length)},
		  freshness: (if $dirty then {state: "unknown", reason: null} else {state: "current", reason: null} end),
		  signature: {kind: "coordinator-recorded"}
		}' "$EV_CHECKS" | tee "$EV_WORK/evidence.json"
	[ "$(jq -r .result "$EV_WORK/evidence.json")" = pass ]
}
