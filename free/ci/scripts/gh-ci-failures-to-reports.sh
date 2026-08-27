#!/usr/bin/env bash
# gh-ci-failures-to-reports — bridge GitHub-hosted Actions failures into nSentry MD reports.
# Lists failed runs across an org's repos, dedups to one report per (repo, workflow, sha),
# and writes timestamped MD reports to --out (default /opt/nself-ops/errors). Run on the
# sentry box (gh authed via a token) on a cron, or locally. Idempotent via a seen-manifest.
#
# Each report embeds which jobs and steps failed, a tail of the failing log, and the first
# error line, both as prose and as machine-readable frontmatter. Without that a reader has
# only a run URL and must round-trip to `gh run view --log-failed` before it can triage
# anything, which is the cost the bridge exists to remove. Only NEW reports pay for the
# extra lookups; the seen-manifest means an unchanged failure is never re-fetched.
#
# Usage: gh-ci-failures-to-reports.sh [--org nself-org] [--out DIR] [--per-repo N] [--repos "a b c"]
#        [--log-lines N] [--no-logs]
set -euo pipefail
ORG="${GH_ORG:-nself-org}"
OUT="${NSENTRY_REMOTE_DIR:-/opt/nself-ops/errors}"
PER=10
REPOS=""
LOG_LINES="${NSENTRY_LOG_LINES:-40}"
WANT_LOGS=1
while [ $# -gt 0 ]; do case "$1" in
  --org) ORG="$2"; shift 2;; --out) OUT="$2"; shift 2;;
  --per-repo) PER="$2"; shift 2;; --repos) REPOS="$2"; shift 2;;
  --log-lines) LOG_LINES="$2"; shift 2;; --no-logs) WANT_LOGS=0; shift;; *) shift;; esac; done
mkdir -p "$OUT"; SEEN="$OUT/.gh-seen"; touch "$SEEN"
[ -n "$REPOS" ] || REPOS=$(gh repo list "$ORG" --no-archived --limit 100 --json name -q '.[].name')
ts(){ date -u +%Y%m%d-%H%M%S; }

# yqs — quote an arbitrary string for a YAML double-quoted scalar. Log lines
# routinely contain colons, quotes and backslashes; an unquoted one silently
# corrupts the document and breaks every downstream parse of the report.
yqs(){ printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' | tr -d '\000-\010\013\014\016-\037'; }

# first_error — the single most useful line from a failing log.
#
# Two passes, because the obvious one-liner picks the wrong line. Warnings are
# skipped first: a Rust build emits `cargo:warning=Compiler family detection
# failed due to error: ...` long before the real failure, and it matches every
# error pattern. Only if nothing else matches do we fall back to including
# warnings, which still beats reporting nothing.
#
# The generic "##[error]Process completed with exit code 1" ends almost every
# failure and says nothing, so it is never preferred over a real message.
# Strips the job/step/timestamp columns `gh` prefixes to each log line.
_err_pat='(FAIL|FAILED|Error:|error:|ERROR|Exception|panic:|Cannot |No such )'
_err_clean(){ sed -e 's/^[^	]*	[^	]*	//' -e 's/^[0-9][0-9-]*T[0-9:.]*Z[[:space:]]*//' | cut -c1-300; }
first_error(){
  local hit
  hit=$(grep -aE "$_err_pat" "$1" 2>/dev/null \
        | grep -avE 'warning|##\[error\]Process completed' | head -1)
  [ -n "$hit" ] || hit=$(grep -m1 -aE "$_err_pat" "$1" 2>/dev/null)
  printf '%s' "$hit" | _err_clean
}

for r in $REPOS; do
  # one entry per workflow (latest failed run), so we don't spam per-run
  gh run list --repo "$ORG/$r" --status failure --limit "$PER" \
    --json workflowName,headSha,displayTitle,url,createdAt,event,databaseId \
    -q '.[] | [.workflowName,.headSha,.displayTitle,.url,.createdAt,.event,.databaseId] | @tsv' 2>/dev/null | \
  awk -F'\t' '!seen[$1]++' | while IFS=$'\t' read -r wf sha title url created event runid; do
    [ -z "$wf" ] && continue
    key="ghci:$r:$wf:${sha:0:8}"
    grep -qxF "$key" "$SEEN" && continue
    h=$(printf '%s' "$key" | md5sum 2>/dev/null | cut -c1-6 || printf '%s' "$key" | md5 | cut -c1-6)
    f="$OUT/$(ts)-$h-ci-$r.md"

    # ── Which jobs and steps actually failed ────────────────────────────────
    # A workflow name alone does not say what broke. "Accessibility Tests"
    # failing at "Install dependencies" is a lockfile problem, not an a11y one,
    # and that distinction is invisible without this.
    jobs_tsv=""
    if [ -n "${runid:-}" ]; then
      jobs_tsv=$(gh run view "$runid" --repo "$ORG/$r" --json jobs \
        -q '.jobs[] | select(.conclusion=="failure" or .conclusion=="cancelled" or .conclusion=="timed_out")
             | . as $j | ($j.steps[]? | select(.conclusion=="failure" or .conclusion=="cancelled" or .conclusion=="timed_out")
             | [$j.name, .name, .conclusion] | @tsv)' 2>/dev/null || true)
      # A job can fail with no failed step (runner/setup failure). Still name it.
      [ -n "$jobs_tsv" ] || jobs_tsv=$(gh run view "$runid" --repo "$ORG/$r" --json jobs \
        -q '.jobs[] | select(.conclusion=="failure" or .conclusion=="cancelled" or .conclusion=="timed_out")
             | [.name, "(job-level, no failing step)", .conclusion] | @tsv' 2>/dev/null || true)
    fi

    # ── Log tail ────────────────────────────────────────────────────────────
    # Bounded on both ends: head -c caps a runaway log before it is buffered,
    # tail keeps the part that says why. Logs expire and private-repo runs can
    # 404, so every failure path here is non-fatal and yields an empty excerpt.
    logf=""; errline=""
    if [ "$WANT_LOGS" = "1" ] && [ -n "${runid:-}" ]; then
      logf=$(mktemp)
      gh run view "$runid" --repo "$ORG/$r" --log-failed 2>/dev/null \
        | head -c 2000000 | tail -n "$LOG_LINES" > "$logf" || : > "$logf"
      errline=$(first_error "$logf")
    fi

    { echo "---"; echo "id: $key"; echo "created_at: $(date -u +%FT%TZ)";
      echo "title: \"CI failed: $ORG/$r — $wf\""; echo "severity: high"; echo "source: github-actions";
      echo "repo: $ORG/$r"; echo "workflow: \"$wf\""; echo "sha: $sha";
      echo "run_id: ${runid:-null}";
      if [ -n "$jobs_tsv" ]; then
        echo "jobs_failed:"
        printf '%s\n' "$jobs_tsv" | cut -f1 | sort -u | while IFS= read -r j; do
          [ -n "$j" ] && echo "  - \"$(yqs "$j")\""; done
        echo "steps:"
        printf '%s\n' "$jobs_tsv" | while IFS=$'\t' read -r j s c; do
          [ -n "$j" ] || continue
          echo "  - job: \"$(yqs "$j")\""
          echo "    step: \"$(yqs "$s")\""
          echo "    conclusion: \"$(yqs "$c")\""; done
      else
        echo "jobs_failed: []"; echo "steps: []"
      fi
      if [ -n "$errline" ]; then echo "first_error_line: \"$(yqs "$errline")\""
      else echo "first_error_line: null"; fi
      echo "---"; echo;
      echo "# CI failed: $ORG/$r — $wf"; echo;
      echo "- **Commit:** \`${sha:0:8}\` · **Event:** $event · **When:** $created";
      echo "- **Title:** $title"; echo "- **Run:** $url"; echo;
      if [ -n "$errline" ]; then
        echo "**First error:** \`$(printf '%s' "$errline" | sed 's/`/'"'"'/g')\`"; echo;
      fi
      if [ -n "$jobs_tsv" ]; then
        echo "## What failed"; echo;
        echo "| Job | Step | Result |"; echo "|---|---|---|";
        printf '%s\n' "$jobs_tsv" | while IFS=$'\t' read -r j s c; do
          [ -n "$j" ] && echo "| $j | $s | $c |"; done
        echo;
      fi
      if [ -n "$logf" ] && [ -s "$logf" ]; then
        echo "## Log tail (last $LOG_LINES lines of the failing step)"; echo;
        echo '```'; cat "$logf"; echo '```'; echo;
      fi
      echo "Routed by nSentry (GitHub-Actions bridge) → your .claude/inbox. Fix or migrate this workflow to the self-hosted runner."; } > "$f"
    [ -n "$logf" ] && rm -f "$logf"
    echo "$key" >> "$SEEN"; echo "WROTE $f"
  done
done
echo "gh-ci bridge done → $OUT (new reports listed above; deduped via .gh-seen)"
