#!/usr/bin/env bash
# fragment-urlenc-smoke.sh
#
# Purpose: prove the free plugin compose fragments build correct connection
#   URLs under reserved-character passwords (cli P7-PROD-31 / plugins
#   P7-PROD-33). `docker compose config` renders every fragment that uses a
#   ${X_URLENC:-${X}} password; the smoke checks that
#     1. with the encoded twins set, every URL's password decodes to the
#        original (a double-encode or a raw substitution fails here),
#     2. with only the raw variables set (an older CLI), each render equals the
#        render of the same fragment at the base ref,
#     3. the lint rejects the base ref's raw-password fragments.
# Inputs: env SMOKE_BASE (default origin/main) names the baseline ref;
#   SMOKE_TMPDIR (default $TMPDIR or /tmp) holds scratch copies.
# Outputs: "ok:"/"FAIL:" lines, final summary; exit 1 on any failure.
# Constraints: docker compose config only (no containers, no network),
#   deterministic, python3 for URL decoding, bash 3.2 compatible.
set -u

root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root" || exit 1
base="${SMOKE_BASE:-origin/main}"
tmp="$(mktemp -d "${SMOKE_TMPDIR:-${TMPDIR:-/tmp}}/urlenc-smoke.XXXXXX")" || exit 1
trap 'rm -rf "$tmp"' EXIT
fail=0
note_fail() { echo "FAIL: $*" >&2; fail=1; }

command -v docker >/dev/null 2>&1 || { echo "FAIL: docker not found" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "FAIL: python3 not found" >&2; exit 1; }

# Values every fragment needs to render; none is a password.
export POSTGRES_USER=nself POSTGRES_DB=nself_db
export DOCKER_NETWORK=smoke_network NSELF_PLUGIN_DIR=/plugins

# encode PW -> the exact bytes the CLI writes to <NAME>_URLENC
# (Go url.UserPassword: everything but unreserved and $&+,;= is escaped).
encode() {
  python3 -c 'import sys,urllib.parse; sys.stdout.write(urllib.parse.quote(sys.argv[1], safe="$&+,;="))' "$1"
}

# Every fragment that puts a Postgres/Redis password in a URL (a raw one is
# still selected, so a regression fails the checks below instead of dropping out).
frags="$(grep -lE '^[^#]*://[^ ]*\$\{(POSTGRES|REDIS)_PASSWORD' free/*/docker-compose.plugin.yml)"
nfrags="$(printf '%s\n' "$frags" | grep -c .)"
[ "$nfrags" -gt 0 ] || { echo "FAIL: no fragment builds a password URL" >&2; exit 1; }

render() { # render FILE -> JSON on stdout (compose project name fixed)
  docker compose -p smoke -f "$1" config --no-consistency --no-path-resolution --format json 2>/dev/null
}

# --- 1. encoded twins set: every URL password decodes to the original --------
cat > "$tmp/check.py" <<'PY'
import json, sys, urllib.parse
doc, pg, rd = json.load(open(sys.argv[1])), sys.argv[2], sys.argv[3]
want = {"postgres": pg, "redis": rd}
seen = 0
def walk(v):
    global seen
    if isinstance(v, dict):
        for x in v.values(): walk(x)
    elif isinstance(v, list):
        for x in v: walk(x)
    elif isinstance(v, str) and "://" in v:
        u = urllib.parse.urlsplit(v)
        if u.password is None and u.username is None: return
        host = (u.hostname or "")
        if host not in want:
            print("unexpected URL host %r" % host); sys.exit(1)
        got = urllib.parse.unquote(u.password or "")
        if got != want[host]:
            print("password for %s did not round-trip" % host); sys.exit(1)
        if host == "postgres" and u.username != sys.argv[4]:
            print("user changed"); sys.exit(1)
        seen += 1
walk(doc)
if seen == 0:
    print("no credential URL rendered"); sys.exit(1)
print(seen)
PY

pws=('p/@ss:w?rd#1%' 'a+b c' '%40%2F-already pct' '@:/?# %+' 'plain' '100%')
for pw in "${pws[@]}"; do
  enc="$(encode "$pw")"
  rpw='r/@:?#%+ '"$pw"
  renc="$(encode "$rpw")"
  bad=0
  for f in $frags; do
    out="$tmp/enc.json"
    POSTGRES_PASSWORD="$pw" POSTGRES_PASSWORD_URLENC="$enc" \
    REDIS_PASSWORD="$rpw" REDIS_PASSWORD_URLENC="$renc" render "$f" > "$out"
    if [ ! -s "$out" ]; then note_fail "$f: compose config failed (encoded)"; bad=1; continue; fi
    if ! msg="$(python3 "$tmp/check.py" "$out" "$pw" "$rpw" "$POSTGRES_USER")"; then
      note_fail "$f: $msg"; bad=1
    fi
  done
  [ "$bad" -eq 0 ] && echo "ok: $nfrags fragments round-trip test password '$pw'"
done

# --- 2. raw only: render equals the base ref's render ------------------------
mkdir -p "$tmp/old" "$tmp/new"
bad=0
for f in $frags; do
  d="$(basename "$(dirname "$f")")"
  mkdir -p "$tmp/old/$d" "$tmp/new/$d"
  if ! git show "$base:$f" > "$tmp/old/$d/docker-compose.plugin.yml" 2>/dev/null; then
    echo "skip: $f is new at $base"; continue
  fi
  cp "$f" "$tmp/new/$d/docker-compose.plugin.yml"
  for pw in 'plain-pw' 'p/@ss:w?rd#1%'; do
    export POSTGRES_PASSWORD="$pw" REDIS_PASSWORD="$pw"
    unset POSTGRES_PASSWORD_URLENC REDIS_PASSWORD_URLENC
    render "$tmp/old/$d/docker-compose.plugin.yml" > "$tmp/old.json"
    render "$tmp/new/$d/docker-compose.plugin.yml" > "$tmp/new.json"
    if [ ! -s "$tmp/old.json" ] || [ ! -s "$tmp/new.json" ]; then
      note_fail "$f: render failed (raw only)"; bad=1
    elif ! cmp -s "$tmp/old.json" "$tmp/new.json"; then
      note_fail "$f: raw-only render differs from $base"; bad=1
    fi
  done
  unset POSTGRES_PASSWORD REDIS_PASSWORD
done
[ "$bad" -eq 0 ] && echo "ok: raw-only renders equal $base for $nfrags fragments"

# --- 3. the lint rejects the base ref's raw form -----------------------------
n=0
for f in $frags; do
  d="$(basename "$(dirname "$f")")"
  [ -s "$tmp/old/$d/docker-compose.plugin.yml" ] || continue
  if bash scripts/lint-compose-password-urls.sh "$tmp/old/$d/docker-compose.plugin.yml" >/dev/null 2>&1; then
    note_fail "$f: lint accepted the base ref's raw password URL"
  else
    n=$((n + 1))
  fi
done
echo "ok: lint rejects $n base-ref fragments"

if [ "$fail" -ne 0 ]; then echo "fragment-urlenc-smoke: FAILED" >&2; exit 1; fi
echo "fragment-urlenc-smoke: OK ($nfrags fragments)"
