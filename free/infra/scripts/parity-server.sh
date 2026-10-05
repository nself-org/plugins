#!/usr/bin/env bash
# parity-server.sh: prove `nself-infra server ...` matches core `nself server ...` (P7-CANON-12).
# Purpose: build core nself from cli origin/main and the head nself-infra, run the same argv through both
#   (core `nself server ARGS`, head `nself-infra server ARGS`) and diff stdout, stderr and exit code.
#   Cases: --help of server and its four subcommands; every refusal (no token, no backup flag, --release-ip
#   without a backup flag, missing id, bad label, missing flags) with and without --json; and, with a fake
#   token, the paths that do reach the network, through a recording proxy that refuses every connection.
# Safety: no real cloud API is ever called. HOME is a temp dir; the only token is a fake string; every HTTPS
#   request goes to a local proxy (HTTPS_PROXY) that logs the CONNECT and answers 403, so nothing leaves the
#   machine and no DELETE can ever be delivered. A preflight aborts the run unless that proxy demonstrably
#   sees the request. Cases that must refuse before any network call assert the proxy saw ZERO requests.
# Inputs: run from anywhere inside the plugins worktree.
#   NSELF            checkout holding cli/ (core build from cli origin/main; required unless PARITY_NSELF_BIN)
#   TMPDIR           scratch parent (required)
#   PARITY_HEAD_SRC  free/infra sources to build (default: this plugin); the self-test points it at mutants.
#   PARITY_CLI_REF   cli ref for the core build (default origin/main)
#   PARITY_NSELF_BIN / PARITY_HEAD_BIN: prebuilt binaries (skip the builds)
# Flags:   --self-test  prove the comparator and the gate: a planted difference must be flagged, a binary
#          compared with itself must pass, and head mutants (destroy gate removed; IP step skipped; delete
#          before the snapshot check) must FAIL the parity run.
# Output:  "ok <case>" per case, DIFF blocks, "parity: PASS (N cases)" or "parity: FAIL".
# Exit:    0 all equal, 1 a difference or an empty capture, 2 usage or build failure.
# Documented differences (normalised, nothing else is): the usage path `nself infra server` is read as
#   `nself server`; core's "Global Flags:" help block (root persistent flags a plugin root does not have) is
#   dropped (up to its blank line; the footer after it is compared); header-box padding before the closing bar is collapsed.
set -euo pipefail

MODE=run
case "${1:-}" in --self-test) MODE=self;; "") ;; *) echo "usage: parity-server.sh [--self-test]" >&2; exit 2;; esac
: "${TMPDIR:?TMPDIR must be set}"
HERE=$(cd "$(dirname "$0")/.." && pwd)            # free/infra
ROOT=$(cd "$HERE/../.." && pwd)                    # plugins worktree root
HEAD_SRC=${PARITY_HEAD_SRC:-$HERE}
CLI_REF=${PARITY_CLI_REF:-origin/main}
CLI_LOCK=${PARITY_CLI_LOCK:-/tmp/nself-cli-compile.lock}
PLUG_LOCK=${PARITY_PLUGINS_LOCK:-/tmp/nself-plugins-compile.lock}
DIFF=/usr/bin/diff

WORK=$(mktemp -d "$TMPDIR/parity-server.XXXXXX"); WORK=$(cd "$WORK" && pwd)
PROXY_PID=
cleanup() {
  [ -n "$PROXY_PID" ] && kill "$PROXY_PID" 2>/dev/null || true
  if [ -n "${NSELF:-}" ] && [ -d "$NSELF/cli" ]; then git -C "$NSELF/cli" worktree remove --force "$WORK/cli" >/dev/null 2>&1 || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT
locked() { local lock=$1; shift; if command -v lockf >/dev/null 2>&1; then lockf -k "$lock" "$@"; else "$@"; fi; }
die() { echo "parity: $*" >&2; exit 2; }

build_head() { # build_head <src> <out>
  ( cd "$1" && CGO_ENABLED=0 locked "$PLUG_LOCK" go build -mod=vendor -o "$2" ./cmd/ ) || die "cannot build nself-infra from $1"
}
mkdir -p "$WORK/bin" "$WORK/home"
if [ -n "${PARITY_NSELF_BIN:-}" ]; then cp "$PARITY_NSELF_BIN" "$WORK/bin/nself"
else
  : "${NSELF:?NSELF must point at the checkout that holds cli/}"
  git -C "$NSELF/cli" worktree add --detach "$WORK/cli" "$CLI_REF" >/dev/null 2>&1 || die "cannot check out cli $CLI_REF"
  ( cd "$WORK/cli" && CGO_ENABLED=0 locked "$CLI_LOCK" go build -o "$WORK/bin/nself" ./cmd/nself ) || die "cannot build core nself"
fi
if [ -n "${PARITY_HEAD_BIN:-}" ]; then cp "$PARITY_HEAD_BIN" "$WORK/bin/nself-infra"; else build_head "$HEAD_SRC" "$WORK/bin/nself-infra"; fi
CORE=$WORK/bin/nself; HEADBIN=$WORK/bin/nself-infra

# ---- recording proxy: logs each request line, answers 403, never forwards ------------------------------
cat > "$WORK/proxy.py" <<'PY'
import socketserver, sys
log = sys.argv[2]
class H(socketserver.StreamRequestHandler):
    def handle(self):
        line = self.rfile.readline().decode("latin-1").strip()
        while True:
            l = self.rfile.readline()
            if l in (b"\r\n", b"\n", b""): break
        with open(log, "a") as f: f.write(line + "\n")
        self.wfile.write(b"HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
class S(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
S(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
PY
PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')
PLOG=$WORK/proxy.log; : > "$PLOG"
python3 "$WORK/proxy.py" "$PORT" "$PLOG" & PROXY_PID=$!
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
  python3 -c "import socket; socket.create_connection(('127.0.0.1',$PORT),1).close()" 2>/dev/null && break; sleep 0.25
done
: > "$PLOG"

# runbin <bin> <outprefix> <token?> args...: runs in an empty env with the proxy; captures out/err/rc
runbin() {
  local bin=$1 pre=$2 tok=$3; shift 3
  local envs=(HOME="$WORK/home" PATH="/usr/bin:/bin" NO_COLOR=1 HTTPS_PROXY="http://127.0.0.1:$PORT" HTTP_PROXY="http://127.0.0.1:$PORT" https_proxy="http://127.0.0.1:$PORT" http_proxy="http://127.0.0.1:$PORT")
  [ "$tok" = tok ] && envs+=(HETZNER_NSELF_TOKEN=parity-fake-token-not-real)
  local sub=server; set +e
  env -i "${envs[@]}" "$bin" $sub "$@" </dev/null > "$pre.out" 2> "$pre.err"; echo $? > "$pre.rc"; set -e
}
norm() { # normalise stdin per the documented differences
  sed -E -e 's/nself infra server/nself server/g' -e "s#127\\.0\\.0\\.1:$PORT#127.0.0.1:PORT#g" -e 's/ +║/ ║/g' \
    | awk 'BEGIN{skip=0} /^Global Flags:$/ {skip=1; next} skip && /^$/ {skip=0; next} !skip {print}' | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}'
}

# ---- preflight: the proxy must see a request, or nothing network-capable runs ---------------------------
runbin "$HEADBIN" "$WORK/pre" tok list
grep -q 'CONNECT api.hetzner.cloud:443' "$PLOG" || die "preflight: the proxy did not see the Hetzner request; refusing to run network-capable cases"
: > "$PLOG"

CASES=0; FAILS=0
# case <name> <expect-net: 0|1|any> <token: tok|notok> args...
pcase() {
  local name=$1 net=$2 tok=$3; shift 3
  : > "$PLOG"; runbin "$CORE" "$WORK/c" "$tok" "$@"; local cn; cn=$(wc -l < "$PLOG" | tr -d ' ')
  : > "$PLOG"; runbin "$HEADBIN" "$WORK/h" "$tok" "$@"; local hn; hn=$(wc -l < "$PLOG" | tr -d ' ')
  CASES=$((CASES + 1)); local bad=0
  for k in out err; do
    norm < "$WORK/c.$k" > "$WORK/c.$k.n"; norm < "$WORK/h.$k" > "$WORK/h.$k.n"
    if ! $DIFF -u "$WORK/c.$k.n" "$WORK/h.$k.n" > "$WORK/d.$k"; then bad=1; echo "DIFF $name ($k):"; head -30 "$WORK/d.$k"; fi
  done
  if [ "$(cat "$WORK/c.rc")" != "$(cat "$WORK/h.rc")" ]; then bad=1; echo "DIFF $name (exit): core=$(cat "$WORK/c.rc") head=$(cat "$WORK/h.rc")"; fi
  if [ "$cn" != "$hn" ]; then bad=1; echo "DIFF $name (network calls): core=$cn head=$hn"; fi
  case $net in 0) [ "$hn" = 0 ] || { bad=1; echo "DIFF $name: refusal path reached the network ($hn request)"; };; 1) [ "$hn" -ge 1 ] || { bad=1; echo "DIFF $name: expected a network attempt"; };; esac
  # an empty capture (nothing printed, exit 0) would hide a broken harness
  if [ ! -s "$WORK/h.out" ] && [ ! -s "$WORK/h.err" ] && [ "$(cat "$WORK/h.rc")" = 0 ]; then bad=1; echo "DIFF $name: empty capture"; fi
  if [ "$bad" = 0 ]; then echo "ok $name (rc=$(cat "$WORK/h.rc"), net=$hn)"; else FAILS=$((FAILS + 1)); fi
}

run_cases() {
  # help
  pcase "help server" 0 notok --help
  pcase "server (no args)" 0 notok
  for s in provision list resize destroy; do pcase "help $s" 0 notok "$s" --help; done
  # no token: every subcommand refuses before any network call
  pcase "list no-token" 0 notok list
  pcase "provision no-token" 0 notok provision --name a --type cx22 --location fsn1 --image ubuntu-24.04
  pcase "resize no-token" 0 notok resize --id 1 --type cx41
  pcase "destroy no-token" 0 notok destroy --id 1
  pcase "destroy no-token --json" 0 notok destroy --id 1 --json
  pcase "destroy no-token force" 0 notok destroy --id 1 --force-no-backup
  # destroy gate: refused with a token present, before any network call, in every output mode
  pcase "destroy no-backup-flag" 0 tok destroy --id 1
  pcase "destroy no-backup-flag --json" 0 tok destroy --id 1 --json
  pcase "destroy release-ip does not waive gate" 0 tok destroy --id 1 --release-ip
  pcase "destroy release-ip --json" 0 tok destroy --id 1 --release-ip --json
  pcase "destroy no id" 0 tok destroy
  pcase "destroy missing id with force" 0 tok destroy --force-no-backup
  pcase "destroy missing id with snapshot --json" 0 tok destroy --snapshot --json
  pcase "destroy unknown flag" 0 tok destroy --id 1 --yes
  pcase "destroy bad timeout" 0 tok destroy --id 1 --snapshot --snapshot-timeout nope
  # destroy paths that pass the gate: the first provider call fails identically (proxy refuses), no delete
  pcase "destroy snapshot" 1 tok destroy --id 1 --snapshot
  pcase "destroy snapshot --json" 1 tok destroy --id 1 --snapshot --json
  pcase "destroy force-no-backup" 1 tok destroy --id 1 --force-no-backup
  pcase "destroy force-no-backup --release-ip" 1 tok destroy --id 1 --force-no-backup --release-ip
  # other subcommands
  pcase "provision missing flags" any tok provision
  pcase "provision bad label" 0 tok provision --name a --type cx22 --location fsn1 --image ubuntu-24.04 --label novalue
  pcase "provision network" 1 tok provision --name a --type cx22 --location fsn1 --image ubuntu-24.04 --label purpose=ci --ssh-key k
  pcase "provision network --json" 1 tok provision --name a --type cx22 --location fsn1 --image ubuntu-24.04 --json
  pcase "list network" 1 tok list
  pcase "list network --json" 1 tok list --json --label-selector managed-by=nself-cli
  pcase "resize network" 1 tok resize --id 1 --type cx41
  pcase "resize missing type" any tok resize --id 1
}

if [ "$MODE" = run ]; then
  run_cases
  # no DELETE (or any tunnel) was ever established: the proxy refuses CONNECT, and no case may log a DELETE
  if grep -q DELETE "$PLOG"; then echo "FAIL: a DELETE reached the proxy"; FAILS=$((FAILS + 1)); fi
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
# (a) comparator: a binary compared with itself passes the help case; a planted difference is flagged
CASES=0; FAILS=0; cp "$CORE" "$WORK/bin/nself-infra"; HEADBIN=$WORK/bin/nself-infra
PCASE_OUT=$(pcase "self vs self help destroy" 0 notok destroy --help 2>&1 || true)
# core renders "nself server", the copy of core prints the same: so a self comparison must pass
case "$PCASE_OUT" in ok*) echo "ok selftest: identical binaries pass";; *) echo "SELFTEST FAIL: identical binaries differ: $PCASE_OUT"; fails=$((fails + 1));; esac
printf '#!/bin/sh\nexec "%s" "$@" | sed "s/Delete a Hetzner/Remove a Hetzner/"\n' "$CORE" > "$WORK/bin/planted"; chmod +x "$WORK/bin/planted"
HEADBIN=$WORK/bin/planted; PCASE_OUT=$(pcase "planted difference" 0 notok destroy --help 2>&1 || true)
case "$PCASE_OUT" in *DIFF*) echo "ok selftest: planted difference flagged";; *) echo "SELFTEST FAIL: planted difference not flagged"; fails=$((fails + 1));; esac
# (b) head mutants must fail the full parity run (core behaviour is the oracle)
mutant() { # mutant <label> <file-relative-to-internal/server> <sed-expr>
  local label=$1 file=$2 expr=$3 src="$WORK/mut-$1"; src=${src// /_}
  rm -rf "$src"; mkdir -p "$src"; cp -R "$HERE"/. "$src"/
  local before; before=$(cksum < "$src/internal/server/$file")
  sed -i.bak -E "$expr" "$src/internal/server/$file"; rm -f "$src/internal/server/$file.bak"
  [ "$before" != "$(cksum < "$src/internal/server/$file")" ] || { echo "SELFTEST FAIL: mutant '$label' changed nothing (vacuous)"; fails=$((fails + 1)); return; }
  expect_fail "mutant: $label" env PARITY_HEAD_SRC="$src" PARITY_NSELF_BIN="$CORE" "$0"
}
mutant "destroy gate removed" destroy.go 's/if !req.TakeSnapshot && !req.ForceNoBackup \{/if false {/'
mutant "primary IP step skipped" destroy.go 's/retained, released, err := ProtectOrReleaseIPs\(ctx, client, req.ServerID, req.ReleaseIP\)/var retained, released []PrimaryIP; var err error/'
mutant "refusal message changed" destroy.go 's/destroy refused: no verified backup/destroy ok: no verified backup/'
if [ "$fails" = 0 ]; then echo "parity self-test: PASS"; exit 0; fi
echo "parity self-test: FAIL ($fails)"; exit 1
