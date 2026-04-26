#!/usr/bin/env bash
set -euo pipefail

MODE="all"
COMMIT_MSG_FILE=""
HISTORY_RANGE=""

# This guard targets assistant authorship attribution language only.
# It intentionally does not block feature terms such as "AI-powered".
CONTENT_REGEX='(co-authored-by|authored-by):[[:space:]].*(chatgpt|openai|claude|codex|copilot|anthropic|gemini|bard|ai assistant|language model|llm)|(generated|written|created|authored)[[:space:]]+(by|with)[[:space:]].*(chatgpt|openai|claude|codex|copilot|anthropic|gemini|bard|ai assistant|language model|llm)|as[[:space:]]+an[[:space:]]+ai[[:space:]]+language[[:space:]]+model'
COMMIT_MSG_REGEX='co-authored-by:[[:space:]]|(generated|written|created|authored)[[:space:]]+(by|with)[[:space:]].*(chatgpt|openai|claude|codex|copilot|anthropic|gemini|bard|ai assistant|language model|llm)|as[[:space:]]+an[[:space:]]+ai[[:space:]]+language[[:space:]]+model'

TARGET_PATHS=()

usage() {
  cat <<'USAGE'
Usage:
  no-attribution-check.sh --all
  no-attribution-check.sh --staged
  no-attribution-check.sh --commit-msg <commit-msg-file>
  no-attribution-check.sh --history-range <git-range>
USAGE
}

die() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

is_exempt_path() {
  case "$1" in
    .claude/*|.codex/*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

collect_all_paths() {
  local path
  TARGET_PATHS=()
  while IFS= read -r -d '' path; do
    if is_exempt_path "$path"; then
      continue
    fi
    TARGET_PATHS+=("$path")
  done < <(git ls-files -z)
}

collect_staged_paths() {
  local path
  TARGET_PATHS=()
  while IFS= read -r -d '' path; do
    if is_exempt_path "$path"; then
      continue
    fi
    TARGET_PATHS+=("$path")
  done < <(git diff --cached --name-only --diff-filter=ACMR -z)
}

scan_file_content() {
  local scan_mode="$1"
  shift

  if [ "$#" -eq 0 ]; then
    printf 'Attribution guard: no files to scan (%s).\n' "$scan_mode"
    return 0
  fi

  if [ "$scan_mode" = "staged" ]; then
    if git grep --cached -nI -E -i "$CONTENT_REGEX" -- "$@"; then
      cat >&2 <<'ERROR_TEXT'
ERROR: assistant authorship attribution detected in staged content.
Remove assistant attribution/co-author wording before commit.
Allowed: genuine product capability text (for example, "AI-powered").
ERROR_TEXT
      return 1
    fi
  else
    if git grep -nI -E -i "$CONTENT_REGEX" -- "$@"; then
      cat >&2 <<'ERROR_TEXT'
ERROR: assistant authorship attribution detected in tracked content.
Remove assistant attribution/co-author wording.
Allowed: genuine product capability text (for example, "AI-powered").
ERROR_TEXT
      return 1
    fi
  fi

  printf 'Attribution guard: content scan passed (%s).\n' "$scan_mode"
}

scan_commit_message_file() {
  local msg_file="$1"

  [ -n "$msg_file" ] || die "--commit-msg requires a file path"
  [ -f "$msg_file" ] || die "commit message file not found: $msg_file"

  if grep -nE -i "$COMMIT_MSG_REGEX" "$msg_file"; then
    cat >&2 <<'ERROR_TEXT'
ERROR: commit message violates attribution policy.
Remove co-author trailers and assistant authorship wording from the commit message.
ERROR_TEXT
    return 1
  fi

  printf 'Attribution guard: commit message scan passed.\n'
}

scan_history_range() {
  local range="$1"
  local matches

  [ -n "$range" ] || die "--history-range requires a git range argument"

  if ! git rev-list "$range" >/dev/null 2>&1; then
    # The base SHA is not reachable in the local clone (e.g. after a force-push
    # or when the before-SHA was GC'd on the remote). Rather than hard-failing
    # the guard on a git infrastructure issue, fall back to the narrowest safe
    # scope: commits since the previous tag, or HEAD only when no tag exists.
    # The attribution check still runs — it is never silently skipped.
    printf 'Warning: base SHA not reachable (%s). Falling back to last-tag scope.\n' "$range" >&2
    local last_tag
    last_tag=$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || true)
    if [ -n "$last_tag" ]; then
      range="${last_tag}..HEAD"
    else
      range="HEAD"
    fi
    printf 'Attribution guard fallback range: %s\n' "$range"
  fi

  # Grandfather commits authored before 2026-03-30. Commits from the early
  # post-v1.0.0 period (S02-era) carried Co-Authored-By trailers from tooling
  # before this policy was enforced. Rewriting published history is not viable.
  # New commits from 2026-03-30 onward must be clean.
  GRANDFATHER_DATE="2026-03-30"

  matches="$(git log --format='%H%n%s%n%b%n---END---' --after="$GRANDFATHER_DATE" "$range" | grep -nE -i "$COMMIT_MSG_REGEX" || true)"
  if [ -n "$matches" ]; then
    printf '%s\n' "$matches"
    cat >&2 <<'ERROR_TEXT'
ERROR: commit history range violates attribution policy.
Remove co-author trailers and assistant authorship wording from commit messages.
ERROR_TEXT
    return 1
  fi

  printf 'Attribution guard: commit history scan passed (%s, grandfathered before %s).\n' "$range" "$GRANDFATHER_DATE"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --all)
      MODE="all"
      ;;
    --staged)
      MODE="staged"
      ;;
    --commit-msg)
      MODE="commit-msg"
      shift
      COMMIT_MSG_FILE="${1:-}"
      ;;
    --history-range)
      MODE="history-range"
      shift
      HISTORY_RANGE="${1:-}"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "unknown option: $1"
      ;;
  esac
  shift
done

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT"

case "$MODE" in
  all)
    collect_all_paths
    scan_file_content "all" "${TARGET_PATHS[@]}"
    ;;
  staged)
    collect_staged_paths
    scan_file_content "staged" "${TARGET_PATHS[@]}"
    ;;
  commit-msg)
    scan_commit_message_file "$COMMIT_MSG_FILE"
    ;;
  history-range)
    scan_history_range "$HISTORY_RANGE"
    ;;
  *)
    die "unsupported mode: $MODE"
    ;;
esac
