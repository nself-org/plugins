#!/usr/bin/env bash
# lint-compose-password-urls.sh
#
# Purpose: reject a raw secret variable inside a URL in a plugin compose
#   fragment. docker compose substitutes a variable verbatim and cannot encode
#   it, so ":${POSTGRES_PASSWORD}@" breaks for a password holding / @ : ? # %.
#   The CLI writes percent-encoded twins (<NAME>_URLENC, cli P7-PROD-31); a
#   fragment must reference the nested form
#     ${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}
#   so an older compose.env without the twin still renders the raw value.
#   A secret variable is one whose name holds PASS, PASSWORD, PASSWD, SECRET or
#   TOKEN (any case). A line fails when, after the nested forms are removed,
#     1. any "://" URL on it (every one, not just the first) still names a
#        secret variable anywhere: userinfo, path or query, or
#     2. a secret variable is followed by "@" in the same word (the userinfo
#        tail of a URL whose "://" sits on the line above).
# Inputs: fragment paths as arguments; default free/*/docker-compose.plugin.yml.
# Outputs: one "file:line: ..." line per offence on stderr; exit 1 if any.
# Constraints: bash 3.2 compatible; sed/grep only; never prints a password.
set -u

if [ "$#" -gt 0 ]; then
  files="$*"
else
  root="$(cd "$(dirname "$0")/.." && pwd)"
  files="$(ls "$root"/free/*/docker-compose.plugin.yml 2>/dev/null)"
fi

secret='\$\{?[A-Za-z0-9_]*(PASS|SECRET|TOKEN)'
bad=0
checked=0
for f in $files; do
  [ -f "$f" ] || { echo "lint-compose-password-urls: no such file: $f" >&2; bad=1; continue; }
  checked=$((checked + 1))
  # Lines naming a secret variable, comments dropped, prefixed "N:".
  hits="$(grep -niE "$secret" "$f" | grep -vE '^[0-9]+:[[:space:]]*#' || true)"
  [ -n "$hits" ] || continue
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    n="${line%%:*}"
    # The nested safe form is allowed; strip it before looking for raw uses.
    # (No sed back-references: BSD and GNU sed disagree on them.)
    rest="${line#*:}"
    for name in $(printf '%s\n' "$rest" | grep -oE '[A-Za-z0-9_]+_URLENC:-' | sed 's/_URLENC:-$//'); do
      safe='${'"${name}"'_URLENC:-${'"${name}"'}}'
      rest="${rest//"$safe"/}"
    done
    urls="$(printf '%s\n' "$rest" | grep -oE '://[^[:space:]"'"'"']*' || true)"
    if printf '%s\n' "$urls" | grep -qiE "$secret" ||
       printf '%s\n' "$rest" | grep -qiE "$secret"'[^[:space:]@]*@'; then
      echo "$f:$n: secret variable used raw inside a URL; use \${NAME_URLENC:-\${NAME}}" >&2
      bad=1
    fi
  done <<EOT
$hits
EOT
done

if [ "$bad" -ne 0 ]; then
  echo "lint-compose-password-urls: FAILED" >&2
  exit 1
fi
echo "lint-compose-password-urls: OK ($checked fragments)"
