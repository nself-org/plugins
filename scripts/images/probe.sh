#!/usr/bin/env bash
# probe.sh [--file images.json] [--fixtures]: anonymous availability probe for every image in the lock (ADR 0030).
# For each entry: pull the image index anonymously (empty DOCKER_CONFIG, no credential), require the bytes to
# hash to the pinned digest and the index to list every platform the entry names.
# Bounded: PROBE_TIMEOUT s per call (60), PROBE_ATTEMPTS (3) with PROBE_BACKOFF s growth (5), PROBE_DEADLINE s
# for the whole run (900). A rate limit, 5xx, timeout or network error is TRANSIENT, never a verdict.
# Exit: 0 all ok (or no entries), 1 at least one definite failure (gone, unpullable, wrong digest, missing
# platform), 3 only transient trouble (inconclusive: callers warn, they do not block), 2 usage.
# PROBE_INSPECT=<cmd> replaces the registry call (cmd REF prints the raw index); used by the fixtures.
set -u
. "$(dirname "$0")/lib.sh"
img_need jq
file=$IMG_ROOT/images.json; fixtures=0
while [ $# -gt 0 ]; do
  case $1 in
    --file) file=$2; shift 2 ;;
    --fixtures) fixtures=1; shift ;;
    *) echo "usage: probe.sh [--file images.json] [--fixtures]" >&2; exit 2 ;;
  esac
done
T=${PROBE_TIMEOUT:-60}; A=${PROBE_ATTEMPTS:-3}; B=${PROBE_BACKOFF:-5}; DL=${PROBE_DEADLINE:-900}
sha256_stdin() { if command -v sha256sum >/dev/null 2>&1; then sha256sum | cut -d' ' -f1; else shasum -a 256 | cut -d' ' -f1; fi; }

registry_inspect() { env -u BUILDX_BUILDER DOCKER_CONFIG=$anon docker buildx imagetools inspect --raw "$1"; }

# run_bounded SECS OUT ERR CMD...: run CMD with a hard time limit (no coreutils timeout on macOS).
run_bounded() {
  local secs=$1 o=$2 e=$3 pid w rc; shift 3
  "$@" >"$o" 2>"$e" & pid=$!
  ( sleep "$secs"; kill -TERM "$pid" 2>/dev/null; sleep 2; kill -KILL "$pid" 2>/dev/null ) >/dev/null 2>&1 & w=$!
  wait "$pid"; rc=$?
  pkill -P "$w" 2>/dev/null; kill "$w" 2>/dev/null; wait "$w" 2>/dev/null
  [ "$rc" -lt 128 ] || { echo "timed out after ${secs}s" >> "$e"; rc=124; }
  return "$rc"
}

# is_transient FILE: the error text looks like rate limiting, an outage or a network problem.
is_transient() {
  grep -Eiq 'toomanyrequests|too many requests|rate limit|(status|http|code|error)[: ]+(429|5[0-9][0-9])|bad gateway|service unavailable|gateway time-?out|timed out|timeout|deadline exceeded|temporar|connection (reset|refused)|no such host|network is unreachable|tls handshake|unexpected eof|^eof|context canceled|try again' "$1"
}
# is_definite FILE: the registry answered and the image is not there for an anonymous caller.
is_definite() {
  grep -Eiq 'not found|unknown flag|no builder|unknown command|is not a docker command|usage: +docker|manifest unknown|name unknown|no such manifest|denied|unauthorized|authentication required|requested access|does not exist' "$1"
}

# probe_one NAME IMAGE PLATFORMS-CSV prints "ok|FAIL|TRANSIENT <name> <detail>" and returns 0/1/3.
probe_one() {
  local name=$1 ref=$2 want=$3 n=1 rc digest got p have missing
  local o=$work/out e=$work/err
  digest=${ref##*@sha256:}
  while :; do
    if [ "$SECONDS" -ge "$deadline" ]; then echo "TRANSIENT $name run deadline (${DL}s) reached before this entry"; return 3; fi
    run_bounded "$T" "$o" "$e" ${PROBE_INSPECT:-registry_inspect} "$ref"; rc=$?
    [ $rc = 0 ] && break
    if is_transient "$e" || { ! is_definite "$e"; }; then
      if [ "$n" -lt "$A" ]; then sleep $((B * n * n)); n=$((n + 1)); continue; fi
      echo "TRANSIENT $name $ref: $(tr '\n' ' ' < "$e" | cut -c1-160)"; return 3
    fi
    echo "FAIL $name $ref: unpullable anonymously: $(tr '\n' ' ' < "$e" | cut -c1-160)"; return 1
  done
  got=$(sha256_stdin < "$o")
  if [ "$got" != "$digest" ]; then echo "FAIL $name $ref: registry served digest $got, lock pins $digest"; return 1; fi
  if ! jq -e '(.manifests | type) == "array"' "$o" >/dev/null 2>&1; then echo "FAIL $name $ref: not an image index (no manifests list)"; return 1; fi
  missing=""
  for p in $(printf '%s' "$want" | tr ',' ' '); do
    jq -e --arg p "$p" 'any(.manifests[]; ((.platform.os // "") + "/" + (.platform.architecture // "")) == $p)' "$o" >/dev/null 2>&1 || missing="$missing $p"
  done
  if [ -n "$missing" ]; then echo "FAIL $name $ref: missing platform(s):$missing"; return 1; fi
  echo "ok $name $ref"; return 0
}

run_probe() {
  local p rows name image plats rc fail=0 trans=0 total=0
  [ -f "$file" ] || { echo "probe: $file not found" >&2; return 2; }
  jq -e '.schema == "nself.plugins.images/v1" and (.images | type == "object")' "$file" >/dev/null || { echo "probe: $file is not an images v1 document" >&2; return 2; }
  rows=$(jq -r '.images | to_entries[] | [.key, .value.image, (.value.platforms | join(","))] | @tsv' "$file")
  work=$(img_tmp); anon=$work/docker-config; mkdir -p "$anon"
  # Anonymous = no auth entries and no credential helper. Only the buildx plugin is linked in.
  for p in "${DOCKER_CONFIG:-$HOME/.docker}/cli-plugins" /usr/local/lib/docker/cli-plugins /usr/libexec/docker/cli-plugins /usr/lib/docker/cli-plugins; do
    [ -d "$p" ] && { ln -s "$p" "$anon/cli-plugins"; break; }
  done
  deadline=$((SECONDS + DL))
  while IFS="$(printf '\t')" read -r name image plats; do
    [ -n "$name" ] || continue
    total=$((total + 1)); probe_one "$name" "$image" "$plats"; rc=$?
    case $rc in 1) fail=$((fail + 1)) ;; 3) trans=$((trans + 1)) ;; esac
  done <<ROWS
$rows
ROWS
  rm -rf "$work"
  echo "probe: $total entries, $fail failed, $trans inconclusive"
  [ "$fail" = 0 ] || return 1
  [ "$trans" = 0 ] || return 3
  return 0
}

if [ "$fixtures" = 1 ]; then
  t=$(img_tmp); trap 'rm -rf "$t"' EXIT; rc=0; mkdir -p "$t/reg"
  # Stub registry: reg/<digest> holds the bytes served for that digest; reg/<digest>.err the error to print.
  cat > "$t/inspect.sh" <<'STUB'
#!/usr/bin/env bash
d=${1##*@sha256:}; r=$(dirname "$0")/reg
[ -f "$r/$d.sleep" ] && sleep "$(cat "$r/$d.sleep")"
[ -f "$r/$d.err" ] && { cat "$r/$d.err" >&2; exit 1; }
[ -f "$r/$d" ] && { cat "$r/$d"; exit 0; }
echo "pull access denied, repository does not exist or may require authorization" >&2; exit 1
STUB
  chmod +x "$t/inspect.sh"
  idx() { printf '{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[%s]}' "$1"; }
  amd='{"mediaType":"x","digest":"sha256:1","size":1,"platform":{"architecture":"amd64","os":"linux"}}'
  arm='{"mediaType":"x","digest":"sha256:2","size":1,"platform":{"architecture":"arm64","os":"linux"}}'
  unk='{"mediaType":"x","digest":"sha256:3","size":1,"platform":{"architecture":"unknown","os":"unknown"}}'
  put() { local d; d=$(printf '%s' "$2" | sha256_stdin); printf '%s' "$2" > "$t/reg/$d"; eval "$1=$d"; }
  put good "$(idx "$amd,$arm,$unk")"; put noarm "$(idx "$amd")"; put single '{"schemaVersion":2,"config":{}}'
  put wrong "$(idx "$amd,$arm")"; mv "$t/reg/$wrong" "$t/reg/$(printf 'e%.0s' $(seq 64))"
  rate=$(printf 'f%.0s' $(seq 64)); printf 'toomanyrequests: You have reached your pull rate limit\n' > "$t/reg/$rate.err"
  slow=$(printf '9%.0s' $(seq 64)); echo 30 > "$t/reg/$slow.sleep"
  gone=$(printf '0%.0s' $(seq 64))
  wrongref=$(printf 'e%.0s' $(seq 64))
  mkl() { # name digest platforms-json ...; builds t/lock.json from triples
    local j='{}'; while [ $# -gt 0 ]; do j=$(printf '%s' "$j" | jq --arg n "$1" --arg d "$2" --argjson p "$3" '.[$n] = {kind: "upstream", image: ("example/" + $n + ":1@sha256:" + $d), platforms: $p, owner: "t"}'); shift 3; done
    printf '%s' "$j" | jq '{schema: "nself.plugins.images/v1", _generated: "x", images: .}' > "$t/lock.json"
  }
  both='["linux/amd64","linux/arm64"]'
  run() { PROBE_INSPECT="$t/inspect.sh" PROBE_ATTEMPTS=2 PROBE_BACKOFF=0 PROBE_TIMEOUT=2 bash "$0" --file "$t/lock.json" >"$t/out" 2>&1; echo $?; }
  want() { # id want-rc
    local c; c=$(run); if [ "$c" = "$2" ]; then echo "pass  $1 (rc=$c)"; else echo "FAIL  $1: rc=$c want $2"; sed 's/^/        /' "$t/out"; rc=1; fi
  }
  mkl good "$good" "$both"; want "multi-arch index passes" 0
  mkl noarm "$noarm" "$both"; want "missing arm64 platform fails" 1; grep -q 'missing platform(s): linux/arm64' "$t/out" || { echo "FAIL  missing platform not named"; rc=1; }
  mkl gone "$gone" "$both"; want "unpullable ref fails" 1
  mkl single "$single" "$both"; want "a single manifest (not an index) fails" 1
  mkl wrong "$wrongref" "$both"; want "bytes that do not hash to the pinned digest fail" 1
  mkl rate "$rate" "$both"; want "a rate limit alone is inconclusive, not a failure" 3
  mkl slow "$slow" "$both"; want "a hung registry call is cut off and inconclusive" 3
  mkl good "$good" "$both" noarm "$noarm" "$both"; want "one definite failure among good entries fails" 1
  mkl rate "$rate" "$both" noarm "$noarm" "$both"; want "a failure outranks a transient" 1
  mkl good "$good" "$both" rate "$rate" "$both"; want "transient plus ok stays inconclusive" 3
  printf '%s' '{"schema":"nself.plugins.images/v1","images":{}}' > "$t/lock.json"; want "an empty lock probes nothing and passes" 0
  printf '%s' '{"nope":1}' > "$t/lock.json"; c=$(run); [ "$c" = 2 ] && echo "pass  a malformed lock is a usage error (rc=2)" || { echo "FAIL  malformed lock rc=$c"; rc=1; }
  # Live, anonymous: needs the network. PROBE_FIXTURES_OFFLINE=1 skips these (stubs above still run).
  if [ "${PROBE_FIXTURES_OFFLINE:-0}" = 1 ]; then echo "skip  live cases (PROBE_FIXTURES_OFFLINE=1)"; exit $rc; fi
  live=sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc
  printf '{"schema":"nself.plugins.images/v1","images":{"alpine":{"kind":"upstream","image":"alpine:3.20@%s","platforms":["linux/amd64","linux/arm64"],"owner":"t"}}}' "$live" > "$t/lock.json"
  c=$(PROBE_TIMEOUT=60 bash "$0" --file "$t/lock.json" >"$t/out" 2>&1; echo $?)
  [ "$c" = 0 ] && echo "pass  live public multi-arch ref passes anonymously" || { echo "FAIL  live ref rc=$c"; cat "$t/out"; rc=1; }
  sed -i.bak 's/"linux\/arm64"/"windows\/amd64"/' "$t/lock.json"; rm -f "$t/lock.json.bak"
  c=$(PROBE_TIMEOUT=60 bash "$0" --file "$t/lock.json" >"$t/out" 2>&1; echo $?)
  [ "$c" = 1 ] && grep -q 'missing platform' "$t/out" && echo "pass  live ref missing a platform fails" || { echo "FAIL  live missing-platform rc=$c"; cat "$t/out"; rc=1; }
  printf '{"schema":"nself.plugins.images/v1","images":{"ghost":{"kind":"plugin","image":"nself/nself-no-such-image-p761:1@sha256:%s","platforms":["linux/amd64"],"owner":"t"}}}' "$gone" > "$t/lock.json"
  c=$(PROBE_TIMEOUT=60 bash "$0" --file "$t/lock.json" >"$t/out" 2>&1; echo $?)
  [ "$c" = 1 ] && echo "pass  live unpullable ref fails anonymously" || { echo "FAIL  live unpullable rc=$c"; cat "$t/out"; rc=1; }
  exit $rc
fi

run_probe
