#!/usr/bin/env bash
# lint-compose-network-literals.sh
#
# Purpose: reject a literal project network name in a plugin compose fragment.
#   The project network is named by the CLI (DOCKER_NETWORK). A fragment that
#   joins "nself_network", "nself-network" or "nself" by name attaches to a
#   network that does not exist on a stack with another project name, or makes
#   compose create a second, empty network of that name. A fragment must join
#   with
#     networks:
#       - ${DOCKER_NETWORK}
#   and must not redefine the project network at the top level (D-0326).
#   Two shapes are refused for the same reason (D-0531, D-0477): a variable as
#   a network key ("${DOCKER_NETWORK}:"; compose never interpolates mapping
#   keys, so it names a network called "${DOCKER_NETWORK}", and compose 5.6
#   rejects the file) and a top-level network marked external (the fragment
#   does not own it; the CLI's main compose file declares the project network).
#   A fragment may still declare a network it owns, such as forgejo's
#   ${COMPOSE_PROJECT_NAME:-nself}_forgejo_runner_net.
#   A ${DOCKER_NETWORK:-<literal>} default is not flagged by design: the CLI
#   rewrites it to ${DOCKER_NETWORK} (internal/build/plugins_network_normalize.go).
# Inputs: fragment paths as arguments; default free/*/docker-compose.plugin.yml.
#   The default run also lints the fixtures in tests/lint/compose-network/: each
#   reject-*.yml must be flagged and each accept-*.yml must pass, so a rule that
#   stops matching fails the gate instead of reading as clean.
# Outputs: one "file:line: ..." line per offence on stderr; exit 1 if any. An
#   excepted file that still holds a literal is reported on stdout as
#   "allowed: <file> (D-0326)". An excepted file with no literal left is stale
#   and fails, so the entry gets deleted.
# Constraints: bash 3.2 compatible; POSIX awk (no GNU-only flags); read-only.
set -u

# Files allowed to keep a literal for now. Matched by path suffix, so absolute
# and relative paths both work. Delete an entry once its file is converted.
ALLOWED="
"

selftest=0
if [ "$#" -gt 0 ]; then
  files="$*"
else
  root="$(cd "$(dirname "$0")/.." && pwd)"
  files="$(ls "$root"/free/*/docker-compose.plugin.yml 2>/dev/null)"
  selftest=1
fi
if [ -z "$files" ]; then
  echo "lint-compose-network-literals: no fragments found" >&2
  echo "lint-compose-network-literals: FAILED" >&2
  exit 1
fi

# scan <file>: prints "N: message" for each literal project network name.
# Scope: inside a "networks:" mapping (service or top level; a deeper indent
# than the key is inside it, as is a "- X" item at the key indent; blank and
# comment lines do not end it), a list
# item "- X", a key "X:" or a "name: X" naming a literal; an inline
# "networks: [ ... ]" element; and "network_mode: X".
scan() {
  awk '
    BEGIN { child = -1 }
    function clean(v) {
      sub(/[ \t]+#.*$/, "", v)
      sub(/^[ \t]+/, "", v)
      sub(/[ \t\r]+$/, "", v)
      if (v ~ /^".*"$/ || v ~ /^\047.*\047$/) v = substr(v, 2, length(v) - 2)
      return v
    }
    # unqkey: a quoted mapping key ("networks": or single-quoted) read as the bare key.
    function unqkey(b,   q, r) {
      q = substr(b, 1, 1)
      if ((q != "\"" && q != "\047") || !match(b, "^" q "[^" q "]*" q "[ \t]*:")) return b
      r = substr(b, 2, index(substr(b, 2), q) - 1)
      return r substr(b, length(r) + 3)
    }
    # flowname: flag a "name:" entry of a flow mapping value ("key: { name: X }").
    function flowname(b,   t, seg) {
      t = b
      while (match(t, /[{,][ \t]*["\047]?name["\047]?[ \t]*:[ \t]*[^,}]*/)) {
        seg = substr(t, RSTART, RLENGTH)
        t = substr(t, RSTART + RLENGTH)
        sub(/^[{,][ \t]*["\047]?name["\047]?[ \t]*:/, "", seg)
        seg = clean(seg)
        if (lit(seg)) print NR ": network name is the literal \"" seg "\"; use ${DOCKER_NETWORK}"
      }
    }
    function lit(v) {
      return (v == "nself_network" || v == "nself-network" || v == "nself")
    }
    {
      line = $0
      sub(/\r$/, "", line)
      if (line ~ /^[ \t]*$/) next
      if (line ~ /^[ \t]*#/) next
      match(line, /^[ \t]*/)
      indent = RLENGTH
      body = unqkey(substr(line, indent + 1))
      # A block ends on a line at or below the key indent, except a "- X" item
      # at the key indent: a valid non-indented YAML sequence, still inside.
      if (in_net && (indent < net_indent || (indent == net_indent && body !~ /^-/))) { in_net = 0; child = -1 }

      if (body ~ /^network_mode[ \t]*:/) {
        v = body; sub(/^network_mode[ \t]*:/, "", v); v = clean(v)
        if (lit(v)) print NR ": network_mode names the literal project network \"" v "\"; use ${DOCKER_NETWORK}"
        next
      }
      if (body ~ /^networks[ \t]*:/) {
        v = body; sub(/^networks[ \t]*:/, "", v); v = clean(v)
        if (v == "") { in_net = 1; net_indent = indent; child = -1; next }
        n = split(v, tok, /[][{},: \t]+/)
        for (i = 1; i <= n; i++) {
          t = clean(tok[i])
          if (lit(t)) print NR ": inline networks list names the literal project network \"" t "\"; use ${DOCKER_NETWORK}"
        }
        next
      }
      if (!in_net) next
      if (child < 0) child = indent
      if (body ~ /^name[ \t]*:/) {
        v = body; sub(/^name[ \t]*:/, "", v); v = clean(v)
        if (lit(v)) print NR ": network name is the literal \"" v "\"; use ${DOCKER_NETWORK}"
        next
      }
      # A top-level network is a key at the child indent; external marks one
      # the fragment does not own (block "external: true" or inline map).
      if (net_indent == 0 && indent > child && body ~ /^external[ \t]*:/) {
        v = body; sub(/^external[ \t]*:/, "", v); v = clean(v)
        if (v == "true") print NR ": top-level network \"" key "\" is external; join the project network by - ${DOCKER_NETWORK} and declare only networks the plugin owns"
        next
      }
      if (indent != child) next
      if (body ~ /^-/) {
        v = body; sub(/^-[ \t]*/, "", v); v = clean(v)
        if (lit(v)) print NR ": service joins the literal network \"" v "\"; use ${DOCKER_NETWORK}"
      } else if (body ~ /^[^ \t:][^:]*:/) {
        k = body; sub(/:.*$/, "", k); k = clean(k); key = k
        if (lit(k)) print NR ": network key is the literal \"" k "\"; join by ${DOCKER_NETWORK} and do not redefine it"
        else if (k ~ /\$/) print NR ": network key \"" k "\" is a variable; compose does not interpolate keys, so join by - ${DOCKER_NETWORK} and do not redefine it"
        flowname(body)
        if (net_indent == 0 && body ~ /[{,][ \t]*["\047]?external["\047]?[ \t]*:[ \t]*["\047]?true/) print NR ": top-level network \"" k "\" is external; join the project network by - ${DOCKER_NETWORK} and declare only networks the plugin owns"
      }
    }
  ' "$1"
}

is_allowed() {
  for a in $ALLOWED; do
    case "$1" in
      "$a" | */"$a") return 0 ;;
    esac
  done
  return 1
}

bad=0
checked=0
for f in $files; do
  [ -f "$f" ] || { echo "lint-compose-network-literals: no such file: $f" >&2; bad=1; continue; }
  checked=$((checked + 1))
  # An awk error must never read as a clean file (it used to fail open).
  if ! hits="$(scan "$f")"; then
    echo "$f: awk failed" >&2
    bad=1
    continue
  fi
  if is_allowed "$f"; then
    if [ -n "$hits" ]; then
      echo "allowed: $f (D-0326)"
    else
      echo "$f: stale exception: no literal network name left; delete its entry from ALLOWED in $0" >&2
      bad=1
    fi
    continue
  fi
  [ -n "$hits" ] || continue
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    echo "$f:$line" >&2
  done <<EOT
$hits
EOT
  bad=1
done

# Positive control on the default run: the rules must still bite.
fixtures=0
if [ "$selftest" -eq 1 ]; then
  for f in "$root"/tests/lint/compose-network/*.yml; do
    [ -f "$f" ] || continue
    fixtures=$((fixtures + 1))
    if ! hits="$(scan "$f")"; then echo "$f: awk failed" >&2; bad=1; continue; fi
    case "$(basename "$f")" in
      reject-*) [ -n "$hits" ] || { echo "$f: fixture must be flagged and was not; a rule stopped matching" >&2; bad=1; } ;;
      accept-*) [ -z "$hits" ] || { echo "$f: fixture must pass and was flagged:" >&2; echo "$hits" >&2; bad=1; } ;;
      *) echo "$f: fixture name must start with reject- or accept-" >&2; bad=1 ;;
    esac
  done
  [ "$fixtures" -gt 0 ] || { echo "lint-compose-network-literals: no fixtures in tests/lint/compose-network" >&2; bad=1; }
fi

if [ "$bad" -ne 0 ]; then
  echo "lint-compose-network-literals: FAILED" >&2
  exit 1
fi
echo "lint-compose-network-literals: OK ($checked fragments, $fixtures fixtures)"
