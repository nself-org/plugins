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
#   A secret variable is one whose name holds PASS (PASSWORD, PASSWD), PWD,
#   SECRET or TOKEN, any case. KEY is left out: the CLI writes no _URLENC twin
#   for an API key, and it matches names such as KEYCLOAK_HOST. A line fails
#   when, after the nested forms are removed,
#     1. any "://" URL on it (every one, not just the first) still names a
#        secret variable anywhere: userinfo, path or query,
#     2. a secret variable is followed by "@" in the same word (the userinfo
#        tail of a URL whose "://" sits on the line above), or
#     3. it continues a URL split across lines: the line after a "://" line
#        that ends in a backslash stuck to URL text (any word count, a trailing
#        " #..." comment ignored), or a one-word line below a "://" line and
#        indented deeper than the key or list item holding that scalar. A
#        flow-sequence item boundary (a comma, a closing bracket) ends the URL.
# Inputs: fragment paths as arguments; default free/*/docker-compose.plugin.yml.
# Outputs: one "file:line: ..." line per offence on stderr; exit 1 if any.
# Constraints: bash 3.2 compatible; sed/grep/awk only; never prints a password.
set -u

if [ "$#" -gt 0 ]; then
  files="$*"
else
  root="$(cd "$(dirname "$0")/.." && pwd)"
  files="$(ls "$root"/free/*/docker-compose.plugin.yml 2>/dev/null)"
fi

secret='\$\{?[A-Za-z0-9_]*(PASS|PWD|SECRET|TOKEN)'
bad=0
checked=0
for f in $files; do
  [ -f "$f" ] || { echo "lint-compose-password-urls: no such file: $f" >&2; bad=1; continue; }
  checked=$((checked + 1))
  # Lines naming a secret variable, comments dropped, prefixed "N:".
  hits="$(grep -niE "$secret" "$f" | grep -vE '^[0-9]+:[[:space:]]*#' || true)"
  [ -n "$hits" ] || continue
  # Line numbers of continuation lines inside a multi-line URL: the line after one that ends in a backslash stuck to
  # URL text (any word count), and a one-word line below a "://" line. A trailing " #..." comment is not text of
  # the line; a flow-sequence item boundary (a comma, or the closing bracket) ends the URL.
  cont=" $(awk '
    /^[[:space:]]*(#|$)/ { next }
    { line = $0; sub(/[[:space:]]+#.*$/, "", line)
      match(line, /^[[:space:]]*/); ind = RLENGTH
      key = (line ~ /^[[:space:]]*(- |[A-Za-z0-9_.-]+:([[:space:]]|$))/)
      n = split(line, w, " ") }
    url && ind <= kind { url = 0 }
    chain || (url && n == 1) { printf "%d ", NR }
    !url && key { kind = ind }
    line ~ /:\/\// { url = 1 }
    { bs = (line ~ /[^[:space:]]\\$/)
      chain = bs && (chain || w[n] ~ /:\/\//)
      q = line; gsub(/"[^"]*"/, "", q); gsub(/\047[^\047]*\047/, "", q)
      t = q; o = gsub(/\[/, "", t); t = q; c = gsub(/\]/, "", t)
      was = depth; depth += o - c; if (depth < 0) depth = 0
      if ((depth > 0 && line ~ /,$/) || (was > 0 && depth == 0)) url = 0 }
  ' "$f")"
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
    split=no
    case "$cont" in *" $n "*) split=yes ;; esac
    if printf '%s\n' "$urls" | grep -qiE "$secret" ||
       printf '%s\n' "$rest" | grep -qiE "$secret"'[^[:space:]@]*@' ||
       { [ "$split" = yes ] && printf '%s\n' "$rest" | grep -qiE "$secret"; }; then
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
