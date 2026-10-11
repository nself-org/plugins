#!/usr/bin/env bash
# parity-server.sh: prove `nself-infra server ...` matches core `nself server ...` (P7-CANON-12).
# Purpose: build core nself (cli origin/main) and the head nself-infra, BOTH with
#   -ldflags "-X <module>/internal/server.hetznerAPIBaseURL=http://127.0.0.1:PORT/v1" (the var seam core's own tests
#   use), point them at scripts/parity-fixture.py (a scripted loopback stand-in for the Hetzner API) and run the same
#   argv through both (core `nself server ARGS`, head `nself-infra server ARGS`). For every case it diffs stdout,
#   stderr, exit code AND the full recorded request sequence (method, path, query, body). Cases: help of server and its
#   four subcommands; every refusal with and without --json; destroy success (snapshot and force-no-backup, with and
#   without --release-ip, a server with no IPs) and every destroy failure path (snapshot start failure, snapshot action
#   error, verify timeout, image poll failure, primary IP read failure, primary IP write failure, unknown server, delete
#   failure); provision, list and resize success and failure paths including the disk-shrink refusal.
# Safety: no real cloud API is ever called. The base URL is baked in as a loopback http URL, HTTPS_PROXY is a dead local
#   port (so even a binary that ignored the seam could not reach the internet), HOME is a temp dir and the only token is a
#   fake string. A preflight aborts unless BOTH binaries demonstrably hit the fixture. Cases that must not delete assert
#   the head's request log holds no DELETE (a refusal path must additionally hold NO request at all).
# Inputs: run from anywhere inside the plugins worktree.
#   NSELF            checkout holding cli/ (core build from cli origin/main; required)
#   TMPDIR           scratch parent (required)
#   PARITY_HEAD_SRC  free/infra sources to build (default: this plugin); the self-test points it at mutants.
#   PARITY_CLI_REF   cli ref for the core build (default origin/main)
#   PARITY_PORT / PARITY_NSELF_BIN: reuse a port and a core binary already built for that port (self-test children)
# Flags:   --self-test  prove the comparator and the gate: a planted difference must be flagged, a binary compared with
#          itself must pass, and eight head mutants (destroy gate removed; primary IP step skipped; refusal text changed;
#          snapshot timeout treated as available; snapshot action error ignored; snapshot failure ignored; primary IP
#          write failure ignored; IPs released by default) must FAIL the parity run; for --json, the real head must pass,
#          and an ok envelope where core failed, an extra request and a data difference must each be flagged.
# Output:  "ok <case>" per case, DIFF blocks, "parity: PASS (N cases)" or "parity: FAIL".
# Exit:    0 all equal, 1 a difference or an empty capture, 2 usage or build failure.
# Documented differences (normalised, nothing else is): the usage path `nself infra server` is read as `nself server`;
#   core's "Global Flags:" help block (root persistent flags a plugin root does not have) is dropped up to its blank
#   line; header-box padding before the closing bar is collapsed; the fixture port is masked. Added with the v1
#   envelope (P7-SURF-29) and --user-data-file (P7-SCHED-19, plugins#160), D-0416:
#   - help cases only: the `--json` flag line (core: "output as JSON", plugin: the v1 envelope text) is dropped from
#     both sides, the plugin-only `--user-data-file` flag line is dropped, and the run of spaces between a flag and its
#     description is collapsed to one (the plugin-only flag widens pflag's column, which shifts every other line).
#   - `--json` cases do NOT diff stdout with core. scripts/parity-envelope.py requires the head's stdout to be exactly
#     one v1 envelope; core exit 0 <=> envelope ok and head exit 0; core exit 1 <=> error envelope, head exit equal to
#     the envelope's exit_code, which must match its class (user 1, infra 2, auth 3, destructive_blocked 4; see
#     cmd/errcodes.go) and not be 0. On success core's legacy JSON must equal the envelope data under the SURF-29
#     mapping (provision: data.server; list: data.servers; resize: data.action; destroy: SnapshotID/RetainedIPs/
#     ReleasedIPs = snapshot_id/retained_ips/released_ips, null = []; the extra data keys user_data, count, server_id,
#     target_type must be present and nothing else). Error text is not compared (core: "Error: ..." on stderr; head: the
#     envelope message), so stderr of a --json case is compared only when core succeeded.
#   - NOT relaxed for any case, --json included: the full recorded request sequence is byte-exact (no extra call, no
#     extra DELETE), and the per-case expectations (refuse: no request; nodelete: no DELETE; delete: DELETE last).
set -euo pipefail

MODE=run
case "${1:-}" in --self-test) MODE=self;; "") ;; *) echo "usage: parity-server.sh [--self-test]" >&2; exit 2;; esac
: "${TMPDIR:?TMPDIR must be set}"
HERE=$(cd "$(dirname "$0")/.." && pwd)            # free/infra
HEAD_SRC=${PARITY_HEAD_SRC:-$HERE}
CLI_REF=${PARITY_CLI_REF:-origin/main}
CLI_LOCK=${PARITY_CLI_LOCK:-/tmp/nself-cli-compile.lock}
PLUG_LOCK=${PARITY_PLUGINS_LOCK:-/tmp/nself-plugins-compile.lock}
DIFF=/usr/bin/diff
CORE_MOD=github.com/nself-org/cli/internal/server
HEAD_MOD=github.com/nself-org/nself-infra/internal/server

WORK=$(mktemp -d "$TMPDIR/parity-server.XXXXXX"); WORK=$(cd "$WORK" && pwd)
FIX_PID=
cleanup() {
  [ -n "$FIX_PID" ] && kill "$FIX_PID" 2>/dev/null || true
  if [ -n "${NSELF:-}" ] && [ -d "$NSELF/cli" ]; then git -C "$NSELF/cli" worktree remove --force "$WORK/cli" >/dev/null 2>&1 || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT
locked() { local lock=$1; shift; if command -v lockf >/dev/null 2>&1; then lockf -k "$lock" "$@"; else "$@"; fi; }
die() { echo "parity: $*" >&2; exit 2; }

PORT=${PARITY_PORT:-$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')}
BASEURL="http://127.0.0.1:$PORT/v1"
FLOG=$WORK/fixture.log; : > "$FLOG"
mkdir -p "$WORK/bin" "$WORK/home"

build_head() { # build_head <src> <out>
  ( cd "$1" && CGO_ENABLED=0 locked "$PLUG_LOCK" go build -mod=vendor -ldflags "-X $HEAD_MOD.hetznerAPIBaseURL=$BASEURL" -o "$2" ./cmd/ ) || die "cannot build nself-infra from $1"
}
if [ -n "${PARITY_NSELF_BIN:-}" ]; then cp "$PARITY_NSELF_BIN" "$WORK/bin/nself"
else
  : "${NSELF:?NSELF must point at the checkout that holds cli/}"
  git -C "$NSELF/cli" worktree add --detach "$WORK/cli" "$CLI_REF" >/dev/null 2>&1 || die "cannot check out cli $CLI_REF"
  ( cd "$WORK/cli" && CGO_ENABLED=0 locked "$CLI_LOCK" go build -ldflags "-X $CORE_MOD.hetznerAPIBaseURL=$BASEURL" -o "$WORK/bin/nself" ./cmd/nself ) || die "cannot build core nself"
fi
build_head "$HEAD_SRC" "$WORK/bin/nself-infra"
CORE=$WORK/bin/nself; HEADBIN=$WORK/bin/nself-infra

start_fixture() {
  python3 "$HERE/scripts/parity-fixture.py" "$PORT" "$FLOG" & FIX_PID=$!
  for _ in $(seq 1 40); do
    python3 -c "import socket; socket.create_connection(('127.0.0.1',$PORT),1).close()" 2>/dev/null && return 0; sleep 0.25
  done
  die "fixture did not start"
}
start_fixture

# runbin <bin> <outprefix> <token: tok|notok> args...: empty env, fake token, dead HTTPS proxy; captures out/err/rc
runbin() {
  local bin=$1 pre=$2 tok=$3; shift 3
  local envs=(HOME="$WORK/home" PATH="/usr/bin:/bin" NO_COLOR=1 HTTPS_PROXY="http://127.0.0.1:1" https_proxy="http://127.0.0.1:1")
  [ "$tok" = tok ] && envs+=(HETZNER_NSELF_TOKEN=parity-fake-token-not-real)
  set +e
  env -i "${envs[@]}" "$bin" server "$@" </dev/null > "$pre.out" 2> "$pre.err"; echo $? > "$pre.rc"; set -e
}
norm() { # normalise stdin per the documented differences; NORM_HELP=1 (help cases only) adds the plugin-only flag lines
  sed -E -e 's/nself infra server/nself server/g' -e "s#127\\.0\\.0\\.1:$PORT#127.0.0.1:PORT#g" -e 's/ +║/ ║/g' \
    | awk 'BEGIN{skip=0} /^Global Flags:$/ {skip=1; next} skip && /^$/ {skip=0; next} !skip {print}' | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}' \
    | if [ "${NORM_HELP:-0}" = 1 ]; then
        # drop exactly the --json and --user-data-file flag lines, then undo the column padding they change
        grep -v -E '^ +--(json|user-data-file) ' | sed -E 's/^( +(-[A-Za-z], )?--[a-z][a-z-]*( [A-Za-z]+)?) {2,}/\1 /' || true
      else cat; fi
}

# ---- preflight: both binaries must hit the fixture, or nothing network-capable runs -----------------------
for b in "$CORE" "$HEADBIN"; do
  : > "$FLOG"; runbin "$b" "$WORK/pre" tok list
  grep -q '^GET /v1/servers' "$FLOG" || die "preflight: $(basename "$b") did not reach the fixture; refusing to run"
done
: > "$FLOG"

CASES=0; FAILS=0
# pcase <name> <expect: refuse|nodelete|delete|any> <token: tok|notok> args...
#   refuse   the head must send NO request at all (a gate or validation refusal)
#   nodelete the head may send requests but never a DELETE
#   delete   the head's last request must be the DELETE (a success path)
pcase() {
  local name=$1 expect=$2 tok=$3; shift 3
  : > "$FLOG"; runbin "$CORE" "$WORK/c" "$tok" "$@"; cp "$FLOG" "$WORK/c.req"
  : > "$FLOG"; runbin "$HEADBIN" "$WORK/h" "$tok" "$@"; cp "$FLOG" "$WORK/h.req"
  CASES=$((CASES + 1)); local bad=0 k a js=0 hp=0
  for a in "$@"; do [ "$a" = --json ] && js=1; [ "$a" = --help ] && hp=1; done
  if [ "$js" = 1 ]; then
    # --json: the head prints one v1 envelope, core its legacy JSON; check the contract and the SURF-29 mapping
    if ! python3 "$HERE/scripts/parity-envelope.py" "$1" "$WORK/c.out" "$WORK/h.out" "$(cat "$WORK/c.rc")" "$(cat "$WORK/h.rc")" > "$WORK/d.env" 2>&1; then
      bad=1; echo "DIFF $name (envelope):"; head -20 "$WORK/d.env"
    fi
    # stderr is compared when core succeeded; a core failure prints "Error: ..." there and the head's envelope replaces it
    if [ "$(cat "$WORK/c.rc")" = 0 ]; then
      norm < "$WORK/c.err" > "$WORK/c.err.n"; norm < "$WORK/h.err" > "$WORK/h.err.n"
      if ! $DIFF -u "$WORK/c.err.n" "$WORK/h.err.n" > "$WORK/d.err"; then bad=1; echo "DIFF $name (err):"; head -30 "$WORK/d.err"; fi
    fi
  else
    for k in out err; do
      NORM_HELP=$hp norm < "$WORK/c.$k" > "$WORK/c.$k.n"; NORM_HELP=$hp norm < "$WORK/h.$k" > "$WORK/h.$k.n"
      if ! $DIFF -u "$WORK/c.$k.n" "$WORK/h.$k.n" > "$WORK/d.$k"; then bad=1; echo "DIFF $name ($k):"; head -30 "$WORK/d.$k"; fi
    done
    if [ "$(cat "$WORK/c.rc")" != "$(cat "$WORK/h.rc")" ]; then bad=1; echo "DIFF $name (exit): core=$(cat "$WORK/c.rc") head=$(cat "$WORK/h.rc")"; fi
  fi
  # the request sequence is byte-exact for every case: it is the safety property (no extra DELETE, no extra call)
  if ! $DIFF -u "$WORK/c.req" "$WORK/h.req" > "$WORK/d.req"; then bad=1; echo "DIFF $name (request sequence):"; head -30 "$WORK/d.req"; fi
  case $expect in
    refuse) [ ! -s "$WORK/h.req" ] || { bad=1; echo "DIFF $name: refusal path sent a request: $(head -1 "$WORK/h.req")"; };;
    nodelete) ! grep -q '^DELETE ' "$WORK/h.req" || { bad=1; echo "DIFF $name: a DELETE was sent on a must-not-delete path"; };;
    delete) [ "$(tail -1 "$WORK/h.req" | cut -d' ' -f1)" = DELETE ] || { bad=1; echo "DIFF $name: success path did not end in DELETE"; };;
  esac
  # an empty capture (nothing printed, exit 0) would hide a broken harness
  if [ ! -s "$WORK/h.out" ] && [ ! -s "$WORK/h.err" ] && [ "$(cat "$WORK/h.rc")" = 0 ]; then bad=1; echo "DIFF $name: empty capture"; fi
  if [ "$bad" = 0 ]; then echo "ok $name (rc=$(cat "$WORK/h.rc"), requests=$(wc -l < "$WORK/h.req" | tr -d ' '))"; else FAILS=$((FAILS + 1)); fi
}

run_cases() {
  pcase "help server" refuse notok --help
  pcase "server (no args)" refuse notok
  for s in provision list resize destroy; do pcase "help $s" refuse notok "$s" --help; done
  pcase "list no-token" refuse notok list
  pcase "provision no-token" refuse notok provision --name a --type cx22 --location fsn1 --image ubuntu-24.04
  pcase "resize no-token" refuse notok resize --id 1 --type cx41
  pcase "destroy no-token" refuse notok destroy --id 1
  pcase "destroy no-token --json" refuse notok destroy --id 1 --json
  pcase "destroy no-token force" refuse notok destroy --id 1 --force-no-backup
  # destroy gate and validation refusals: no request at all
  pcase "destroy no-backup-flag" refuse tok destroy --id 1
  pcase "destroy no-backup-flag --json" refuse tok destroy --id 1 --json
  pcase "destroy release-ip does not waive gate" refuse tok destroy --id 1 --release-ip
  pcase "destroy release-ip --json" refuse tok destroy --id 1 --release-ip --json
  pcase "destroy no id" refuse tok destroy
  pcase "destroy missing id with force" refuse tok destroy --force-no-backup
  pcase "destroy missing id with snapshot --json" refuse tok destroy --snapshot --json
  pcase "destroy unknown flag" refuse tok destroy --id 1 --yes
  pcase "destroy bad timeout" refuse tok destroy --id 1 --snapshot --snapshot-timeout nope
  # destroy success paths: snapshot, IP protect, DELETE last
  pcase "destroy snapshot success" delete tok destroy --id 1 --snapshot
  pcase "destroy snapshot success --json" delete tok destroy --id 1 --snapshot --json
  pcase "destroy force-no-backup success" delete tok destroy --id 1 --force-no-backup
  pcase "destroy force-no-backup --json" delete tok destroy --id 1 --force-no-backup --json
  pcase "destroy release-ip success" delete tok destroy --id 1 --force-no-backup --release-ip
  pcase "destroy snapshot release-ip" delete tok destroy --id 1 --snapshot --release-ip
  pcase "destroy server without IPs" delete tok destroy --id 10 --force-no-backup
  # destroy failure paths: no DELETE, identical text and request sequence
  pcase "destroy snapshot start fails" nodelete tok destroy --id 2 --snapshot
  pcase "destroy snapshot action error" nodelete tok destroy --id 3 --snapshot
  pcase "destroy snapshot verify timeout" nodelete tok destroy --id 4 --snapshot --snapshot-timeout 1s
  pcase "destroy snapshot verify timeout --json" nodelete tok destroy --id 4 --snapshot --snapshot-timeout 1s --json
  pcase "destroy IP read fails (force)" nodelete tok destroy --id 5 --force-no-backup
  pcase "destroy IP write fails (force)" nodelete tok destroy --id 6 --force-no-backup
  pcase "destroy IP write fails after snapshot" nodelete tok destroy --id 6 --snapshot
  pcase "destroy unknown server (force)" nodelete tok destroy --id 7 --force-no-backup
  pcase "destroy unknown server (snapshot)" nodelete tok destroy --id 7 --snapshot
  pcase "destroy image poll fails" nodelete tok destroy --id 8 --snapshot
  pcase "destroy delete fails" any tok destroy --id 9 --force-no-backup
  # provision, list, resize
  pcase "provision missing flags" refuse tok provision
  pcase "provision bad label" refuse tok provision --name a --type cx22 --location fsn1 --image ubuntu-24.04 --label novalue
  pcase "provision success" nodelete tok provision --name a --type cx22 --location fsn1 --image ubuntu-24.04 --label purpose=ci --ssh-key k
  pcase "provision success --json" nodelete tok provision --name a --type cx22 --location fsn1 --image ubuntu-24.04 --json
  pcase "provision rejected" nodelete tok provision --name fail --type cx22 --location fsn1 --image ubuntu-24.04
  pcase "list" nodelete tok list
  pcase "list --json selector" nodelete tok list --json --label-selector managed-by=nself-cli
  pcase "resize upgrade" nodelete tok resize --id 1 --type cx41
  pcase "resize upgrade-disk --json" nodelete tok resize --id 1 --type cx41 --upgrade-disk --json
  pcase "resize disk shrink refused" nodelete tok resize --id 1 --type cx11
  pcase "resize unknown type" nodelete tok resize --id 1 --type nosuch
  pcase "resize unknown server" nodelete tok resize --id 7 --type cx41
  pcase "resize missing type" refuse tok resize --id 1
}

if [ "$MODE" = run ]; then
  run_cases
  [ "$CASES" -gt 0 ] || { echo "parity: FAIL (no cases ran)"; exit 1; }
  if [ "$FAILS" = 0 ]; then echo "parity: PASS ($CASES cases)"; exit 0; fi
  echo "parity: FAIL ($FAILS of $CASES cases differ)"; exit 1
fi

# ---- self-test ------------------------------------------------------------------------------------------
fails=0
expect_fail() { # expect_fail <label> <cmd...>: the parity run must exit 1
  local label=$1; shift
  if "$@" > "$WORK/st.out" 2>&1; then echo "SELFTEST FAIL: $label was NOT detected"; fails=$((fails + 1)); else echo "ok selftest: $label detected"; fi
}
# (a) comparator: core against itself passes; a planted difference is flagged (head slot is replaced temporarily)
cp "$HEADBIN" "$WORK/bin/nself-infra.real"; cp "$CORE" "$WORK/bin/nself-infra"
OUTA=$(pcase "self vs self destroy" delete tok destroy --id 1 --snapshot 2>&1 || true)
case "$OUTA" in ok*) echo "ok selftest: identical binaries pass";; *) echo "SELFTEST FAIL: identical binaries differ: $OUTA"; fails=$((fails + 1));; esac
printf '#!/bin/sh\nexec "%s" "$@" | sed "s/Server destroyed/Server removed/"\n' "$CORE" > "$WORK/bin/nself-infra"; chmod +x "$WORK/bin/nself-infra"
OUTB=$(pcase "planted text difference" delete tok destroy --id 1 --snapshot 2>&1 || true)
case "$OUTB" in *DIFF*) echo "ok selftest: planted output difference flagged";; *) echo "SELFTEST FAIL: planted difference not flagged"; fails=$((fails + 1));; esac
printf '#!/bin/sh\nexec "%s" "$@" | sed "s/NOT deleted/deleted/"\n' "$CORE" > "$WORK/bin/nself-infra"
OUTC=$(pcase "planted failure text" nodelete tok destroy --id 2 --snapshot 2>&1 || true)
case "$OUTC" in *DIFF*) echo "ok selftest: planted failure-text difference flagged";; *) echo "SELFTEST FAIL: planted failure-text difference not flagged"; fails=$((fails + 1));; esac
# (a2) --json cases: the head's real binary passes, and each planted --json difference is flagged
cp "$WORK/bin/nself-infra.real" "$WORK/bin/nself-infra"
OUTD=$(pcase "json control" nodelete tok list --json 2>&1 || true)
case "$OUTD" in ok*) echo "ok selftest: real head passes a --json case";; *) echo "SELFTEST FAIL: real head fails a --json control: $OUTD"; fails=$((fails + 1));; esac
printf '#!/bin/sh\nprintf "%%s\\n" '"'"'{"schema_version":"1","command":"infra server destroy","data":{}}'"'"'\nexit 0\n' > "$WORK/bin/nself-infra"
OUTE=$(pcase "planted ok envelope" refuse tok destroy --id 1 --json 2>&1 || true)
case "$OUTE" in *"DIFF planted ok envelope (envelope)"*) echo "ok selftest: ok envelope where core failed flagged";; *) echo "SELFTEST FAIL: ok envelope where core failed not flagged: $OUTE"; fails=$((fails + 1));; esac
printf '#!/bin/sh\n"%s" "$@"; rc=$?\n/usr/bin/curl -s -o /dev/null "%s/servers"\nexit $rc\n' "$WORK/bin/nself-infra.real" "$BASEURL" > "$WORK/bin/nself-infra"
OUTF=$(pcase "planted extra request" nodelete tok list --json 2>&1 || true)
case "$OUTF" in *"DIFF planted extra request (request sequence)"*) echo "ok selftest: extra --json request flagged";; *) echo "SELFTEST FAIL: extra --json request not flagged: $OUTF"; fails=$((fails + 1));; esac
printf '#!/bin/sh\n"%s" "$@" | sed "s/srv-1/srv-X/"\n' "$WORK/bin/nself-infra.real" > "$WORK/bin/nself-infra"
OUTG=$(pcase "planted data difference" nodelete tok list --json 2>&1 || true)
case "$OUTG" in *"DIFF planted data difference (envelope)"*) echo "ok selftest: --json data difference flagged";; *) echo "SELFTEST FAIL: --json data difference not flagged: $OUTG"; fails=$((fails + 1));; esac
cp "$WORK/bin/nself-infra.real" "$WORK/bin/nself-infra"
# (b) head mutants must fail the full parity run (core behaviour is the oracle); children reuse this core and port
kill "$FIX_PID" 2>/dev/null || true; wait "$FIX_PID" 2>/dev/null || true; FIX_PID=
mutant() { # mutant <label> <file under internal/server> <sed -E expr>
  local label=$1 file=$2 expr=$3 src="$WORK/mut-${1// /_}"
  rm -rf "$src"; mkdir -p "$src"; cp -R "$HERE"/. "$src"/
  local before; before=$(cksum < "$src/internal/server/$file")
  sed -i.bak -E "$expr" "$src/internal/server/$file"; rm -f "$src/internal/server/$file.bak"
  [ "$before" != "$(cksum < "$src/internal/server/$file")" ] || { echo "SELFTEST FAIL: mutant '$label' changed nothing (vacuous)"; fails=$((fails + 1)); return; }
  expect_fail "mutant: $label" env PARITY_HEAD_SRC="$src" PARITY_PORT="$PORT" PARITY_NSELF_BIN="$CORE" "$0"
}
mutant "destroy gate removed" destroy.go 's/if !req.TakeSnapshot && !req.ForceNoBackup \{/if false {/'
mutant "primary IP step skipped" destroy.go 's/retained, released, err := ProtectOrReleaseIPs\(ctx, client, req.ServerID, req.ReleaseIP\)/var retained, released []PrimaryIP; var err error/'
mutant "refusal message changed" destroy.go 's/destroy refused: no verified backup/destroy ok: no verified backup/'
mutant "snapshot timeout treated as available" snapshot.go 's/if time.Now\(\).After\(deadline\) \{/if time.Now().After(deadline) {\n\t\t\treturn img, nil\n\t\t}\n\t\tif false {/'
mutant "snapshot action error ignored" snapshot.go 's/if act.Status == "error" \{/if false {/'
mutant "snapshot failure ignored" destroy.go 's/return nil, fmt.Errorf\("destroy: pre-destroy snapshot failed, server NOT deleted: %w", err\)/img = \&Image{}/'
mutant "primary IP write failure ignored" primaryip.go 's/return nil, nil, fmt.Errorf\("set auto_delete=%v on IP %s: %w", wantAutoDelete, ip.IP, err\)/_ = err/'
mutant "IPs released by default" primaryip.go 's/wantAutoDelete := release/wantAutoDelete := !release/'
if [ "$fails" = 0 ]; then echo "parity self-test: PASS"; exit 0; fi
echo "parity self-test: FAIL ($fails)"; exit 1
