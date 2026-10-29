#!/usr/bin/env bash
# lib.sh: shared helpers of the hardware conformance legs (P7-AUTO-25). Sourced, never run.
# Purpose: print one `ci-conformance/v1` JSON line per leg and exit 0 (pass), 1 (fail) or 3 (skip).
# Inputs (environment, all optional until a leg needs them):
#   CI_CONF_BIN     the ci binary (default nself-ci)
#   CI_CONF_OUT     evidence directory (default: a folder under TMPDIR)
#   CI_CONF_MAC     "local" when this machine is the macOS arm64 host
#   CI_CONF_LINUX   node id of a linux amd64 node
#   CI_CONF_NODE1, CI_CONF_NODE2   node ids of two real nodes; CI_CONF_VPS a personal VPS node id
#   CI_CONF_COORD   a coordinator pool id; CI_CONF_EXTVOL an external volume whose root is 0775
# Outputs: the leg line on stdout; commands' output goes to the evidence file.
# Constraints: bash 3.2; a leg never changes the host outside a temp HOME and the sample repo.
set -u

now_ms() { echo $(( $(date +%s) * 1000 )); }

# jesc escapes a string for a JSON value (control characters dropped).
jesc() { printf '%s' "$1" | tr -d '\000-\037' | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }

conf_begin() {
  CONF_LEG=$1
  CONF_START=$(now_ms)
  : "${TMPDIR:?TMPDIR must be set}"
  CONF_DIR=$(mktemp -d "${CI_CONF_OUT:-$TMPDIR}/ci-conf-$1.XXXXXX") || exit 2
  CONF_EVID="$CONF_DIR/evidence.log"
  CONF_HOME="$CONF_DIR/home"
  mkdir -p "$CONF_HOME"
  : > "$CONF_EVID"
  BIN=${CI_CONF_BIN:-nself-ci}
}

conf_emit() { # status pass reason
  printf '{"schema":"ci-conformance/v1","leg":"%s","kind":"hardware","status":"%s","pass":%s,"reason":"%s","evidence_ref":"%s","duration_ms":%s}\n' \
    "$CONF_LEG" "$1" "$2" "$(jesc "$3")" "$(jesc "$CONF_EVID")" "$(( $(now_ms) - CONF_START ))"
}
conf_pass() { conf_emit pass true "${1:-ok}"; exit 0; }
conf_fail() { conf_emit fail false "$1"; exit 1; }
conf_skip() { conf_emit skip false "$1"; exit 3; }

# conf_need NAME...: skip unless every named variable is set and non-empty.
conf_need() {
  local v
  for v in "$@"; do
    eval "[ -n \"\${$v:-}\" ]" || conf_skip "missing host: $v is not set"
  done
}

# conf_bin: skip unless the ci binary runs.
conf_bin() { command -v "$BIN" >/dev/null 2>&1 || conf_skip "missing binary: $BIN"; }

# conf_timeout SECONDS CMD...: run with a deadline; exit 124 on a hang.
conf_timeout() {
  local secs=$1 pid waited=0 rc
  shift
  "$@" & pid=$!
  while kill -0 "$pid" 2>/dev/null; do
    if [ "$waited" -ge "$secs" ]; then kill "$pid" 2>/dev/null; wait "$pid" 2>/dev/null; return 124; fi
    sleep 1; waited=$((waited + 1))
  done
  wait "$pid"; rc=$?
  return $rc
}

# conf_ci ARGS...: run the binary with a temp HOME, appending output to the evidence file.
conf_ci() { HOME="$CONF_HOME" "$BIN" "$@" >>"$CONF_EVID" 2>&1; }

# conf_sample KIND DIR: write a tiny repository of KIND go, node or gonode.
conf_sample() {
  local kind=$1 dir=$2
  mkdir -p "$dir" && git -C "$dir" init -q || return 1
  case "$kind" in
    go|gonode)
      printf 'module sample\n\ngo 1.22\n' > "$dir/go.mod"
      printf 'package sample\n\nfunc One() int { return 1 }\n' > "$dir/sample.go"
      printf 'package sample\n\nimport "testing"\n\nfunc TestOne(t *testing.T) {\n\tif One() != 1 {\n\t\tt.Fatal("one")\n\t}\n}\n' > "$dir/sample_test.go" ;;
  esac
  case "$kind" in
    node|gonode)
      printf '{"name":"sample","version":"0.0.0","scripts":{"test":"node -e 0"}}\n' > "$dir/package.json" ;;
  esac
  git -C "$dir" add . && git -C "$dir" -c user.name=conf -c user.email=conf@localhost commit -q -m sample
}

# conf_table WHERE DIR: the gate table of DIR on WHERE with timings removed, one row per line.
conf_table() {
  (cd "$2" && HOME="$CONF_HOME" conf_timeout 600 "$BIN" --where "$1" --check --no-status --json 2>>"$CONF_EVID") \
    | grep -E '"(gate|job|status)"' | sed -E 's/"(duration|elapsed|started|finished)[a-z_]*":[^,}]*,?//g' | sort
}
