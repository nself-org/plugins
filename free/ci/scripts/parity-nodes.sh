#!/usr/bin/env bash
# parity-nodes.sh: prove `nself-ci nodes verify|provision` equals core `nself runner verify|provision` (P7-CANON-10).
# Runs core (a cli origin/main build) and the head nself-ci against two disposable local sshd containers and
# diffs stdout, stderr, exit code, the ssh argv sent (every remote script byte for byte) and the sorted commands
# the containers' stub sudo/curl/apt-get/tar/dpkg/ldd recorded. "nself runner " is normalised to "nself ci nodes ";
# the "--" the sdk puts before the ssh destination is dropped from the argv log (the one intended argv change).
# Hosts: linuxserver/openssh-server pinned by digest, key auth, a key pair made in $TMPDIR, published on 127.0.0.1
#   only, removed on exit. Nothing else is contacted: the stubs are first on the ssh PATH (asserted before every
#   case) and the GitHub names resolve to the container itself. Provision never runs on this machine: every case
#   names a container host or is refused before any host work, and a blocking sudo/apt-get/curl leads the local PATH.
# Inputs: run from a plugins worktree. NSELF (checkout holding cli/; core is built from cli origin/main) and
#   TMPDIR are required. PARITY_HEAD_SRC (head free/ci sources, default this tree; negative proofs point it at a
#   mutated copy), PARITY_CLI_REF, PARITY_NSELF_BIN (prebuilt core), PARITY_ONLY (regex of case labels; zero cases
#   fails), PARITY_KEEP (keep the scratch dir). Flags: --self-test (the comparator flags a planted difference),
#   --capture-help (rewrite internal/nodes/provision/help/*.txt from the core build).
# Exit: 0 all equal, 1 a difference or an empty capture, 2 usage, build or docker failure.
# Not compared: core `provision --json` prints core's E402 error (the plugin a plain one); a destination that
#   passes core's legacy check but not the sdk allowlist is refused before ssh (unit tests).
set -euo pipefail
MODE=run
case "${1:-}" in --self-test) MODE=self;; --capture-help) MODE=help;; "") ;; *) echo "usage: parity-nodes.sh [--self-test|--capture-help]" >&2; exit 2;; esac
: "${TMPDIR:?TMPDIR must be set}"
IMAGE="lscr.io/linuxserver/openssh-server@sha256:46f115de7c251558297e7e87566fc3fc08544b63e55502b5cf294454db5d29d1"
HERE=$(cd "$(dirname "$0")/.." && pwd); HEAD_SRC=${PARITY_HEAD_SRC:-$HERE}; CLI_REF=${PARITY_CLI_REF:-origin/main}
CLI_LOCK=${PARITY_CLI_LOCK:-/tmp/nself-cli-compile.lock}; PLUG_LOCK=${PARITY_PLUGINS_LOCK:-/tmp/nself-plugins-compile.lock}
REAL_SSH=$(command -v ssh) || { echo "parity-nodes: ssh is required" >&2; exit 2; }
WORK=$(mktemp -d "$TMPDIR/parity-nodes.XXXXXX"); WORK=$(cd "$WORK" && pwd); NODES=""
cleanup() {
  for n in $NODES; do docker rm -f "$n" >/dev/null 2>&1 || true; done
  [ -d "${NSELF:-/nonexistent}/cli" ] && git -C "$NSELF/cli" worktree remove --force "$WORK/cli" >/dev/null 2>&1 || true
  if [ -n "${PARITY_KEEP:-}" ]; then echo "parity-nodes: kept $WORK" >&2; else rm -rf "$WORK"; fi
}
trap cleanup EXIT
die() { echo "parity-nodes: $*" >&2; exit 2; }
locked() { local lock=$1; shift; if command -v lockf >/dev/null 2>&1; then lockf -k "$lock" "$@"; else "$@"; fi; }

# ---- builds ---------------------------------------------------------------------------------------------
mkdir -p "$WORK/head" "$WORK/core" "$WORK/shim" "$WORK/blocklocal" "$WORK/cwd" "$WORK/home/.ssh" "$WORK/o" "$WORK/stubs"
chmod 700 "$WORK/home/.ssh"
( cd "$HEAD_SRC" && CGO_ENABLED=0 locked "$PLUG_LOCK" go build -o "$WORK/head/nself-ci" ./cmd/ ) || die "cannot build nself-ci from $HEAD_SRC"
if [ -n "${PARITY_NSELF_BIN:-}" ]; then cp "$PARITY_NSELF_BIN" "$WORK/core/nself"; else
  : "${NSELF:?NSELF must point at the checkout that holds cli/}"
  git -C "$NSELF/cli" worktree add --detach "$WORK/cli" "$CLI_REF" >/dev/null 2>&1 || die "cannot check out cli $CLI_REF"
  ( cd "$WORK/cli" && CGO_ENABLED=0 locked "$CLI_LOCK" go build -o "$WORK/core/nself" ./cmd/nself ) || die "cannot build core nself"
fi
CORE=$WORK/core/nself; HEADBIN=$WORK/head/nself-ci
if [ "$MODE" = help ]; then
  for c in provision verify; do
    "$CORE" runner "$c" --help | sed 's/nself runner /nself ci nodes /g' > "$HERE/internal/nodes/provision/help/$c.txt"
    [ -s "$HERE/internal/nodes/provision/help/$c.txt" ] || die "empty help for $c"
  done; echo "captured help for provision and verify"; exit 0
fi

# ---- comparator -----------------------------------------------------------------------------------------
norm() { sed -E -e "s#$WORK#<W>#g" -e 's/\[127\.0\.0\.1\]:[0-9]+/[127.0.0.1]:PORT/g' -e 's/port [0-9]+/port N/g' -e 's/nself runner /nself ci nodes /g'; }
CASES=0; FAILS=0; ENVV=""; STATE=bare; WANT_RC=""; FAIL_PATS=()
# capture <dir> cmd args...: stdout, stderr, rc, ssh argv log and the nodes' stub logs, normalised
capture() {
  local dir=$1 rc=0 n; shift; mkdir -p "$dir"
  reset_nodes; : > "$WORK/ssh.log"; rm -f "$WORK/home/.ssh/known_hosts"
  ( cd "$WORK/cwd" && env -u GITHUB_RUNNER_TOKEN -u NSELF_DEPLOY_KEY_PATH -u NSELF_DEPLOY_SSH_KEY HOME="$WORK/home" \
      PATH="$WORK/shim:$WORK/blocklocal:$PATH" NO_COLOR=1 $ENVV "$@" >"$dir/out" 2>"$dir/err" ) || rc=$?
  echo "$rc" > "$dir/rc"; norm < "$dir/out" > "$dir/out.n"; norm < "$dir/err" > "$dir/err.n"
  { grep -vx -e '--' "$WORK/ssh.log" || true; } | norm > "$dir/ssh.n"
  # sorted: the stages of a pipeline in one remote script (curl | sudo tee) log in either order
  { for n in $NODES; do echo "== node"; docker exec "$n" sh -c 'sort /tmp/stub.log 2>/dev/null || true'; done; } | norm > "$dir/stub.n"
}
# pair <label> <1: stdout must be non-empty, 0: stderr> args...: core `runner CMD ARGS` against head `nodes CMD ARGS`;
# the first word of the label is the subcommand
pair() {
  local label=$1 ne=$2 cmd=${1%% *} id; shift 2
  if [ -n "${PARITY_ONLY:-}" ] && ! [[ $label =~ $PARITY_ONLY ]]; then return 0; fi
  id=$(printf '%s' "$label" | tr -c 'A-Za-z0-9\n' '_'); CASES=$((CASES + 1))
  capture "$WORK/o/$id/a" "$CORE" runner "$cmd" "$@"; capture "$WORK/o/$id/b" "$HEADBIN" nodes "$cmd" "$@"
  verdict "$label" "$id" "$ne"; FAIL_PATS=()
}
verdict() {
  local label=$1 id=$2 ne=$3 bad=0 ext o=$WORK/o/$2
  for ext in out.n err.n rc ssh.n stub.n; do
    cmp -s "$o/a/$ext" "$o/b/$ext" || { bad=1; echo "DIFF $label ($ext):"; diff -u "$o/a/$ext" "$o/b/$ext" | head -40 || true; }
  done
  if [ "$ne" = 1 ] && ! [ -s "$o/a/out.n" ]; then bad=1; echo "EMPTY $label: nothing on stdout (vacuous)"; fi
  if [ "$ne" = 0 ] && ! [ -s "$o/a/err.n" ]; then bad=1; echo "EMPTY $label: nothing on stderr (vacuous)"; fi
  if [ -n "$WANT_RC" ] && [ "$(cat "$o/a/rc")" != "$WANT_RC" ]; then
    bad=1; echo "RC $label: exit code $(cat "$o/a/rc"), expected $WANT_RC (the case did not exercise what it claims)"; fi
  if [ $bad -eq 0 ]; then echo "ok $label"; else FAILS=$((FAILS + 1)); fi
}
if [ "$MODE" = self ]; then # the comparator needs no containers
  reset_nodes() { :; }; printf '#!/bin/sh\necho "$@"\n' > "$WORK/sa"; printf '#!/bin/sh\necho "$@" changed\n' > "$WORK/sb"; chmod +x "$WORK/sa" "$WORK/sb"
  capture "$WORK/o/s1/a" "$WORK/sa" x; capture "$WORK/o/s1/b" "$WORK/sa" x; capture "$WORK/o/s2/b" "$WORK/sb" x; cp -R "$WORK/o/s1/a" "$WORK/o/s2/a"
  [ "$(verdict "self identical" s1 1 | head -1)" = "ok self identical" ] || { echo "parity-nodes self-test: FAIL (identical sides differ)"; exit 1; }
  verdict "self planted" s2 1 | awk '/^DIFF self planted/{f=1} END{exit f?0:1}' || { echo "parity-nodes self-test: FAIL (difference not reported)"; exit 1; }
  WANT_RC=9; verdict "self rc" s1 1 | awk '/^RC self rc/{f=1} END{exit f?0:1}' || { echo "parity-nodes self-test: FAIL (wrong exit code not reported)"; exit 1; }
  echo "parity-nodes self-test: PASS (identical sides equal, planted difference and wrong exit code reported)"; exit 0
fi

# ---- fixture: ssh shim, stubs, containers ---------------------------------------------------------------
# the shim records the argv it is given, then adds -F (OpenSSH reads the passwd home, not $HOME, for ssh config)
# and a known_hosts under the scratch HOME; both sides go through the same shim
printf '#!/bin/sh\n{ for a in "$@"; do printf "%%s\\n" "$a"; done; echo ---; } >> "%s"\nexec "%s" -F "%s" -o "UserKnownHostsFile=%s" "$@"\n' \
  "$WORK/ssh.log" "$REAL_SSH" "$WORK/home/.ssh/config" "$WORK/home/.ssh/known_hosts" > "$WORK/shim/ssh"
for b in sudo apt-get curl; do printf '#!/bin/sh\necho "LOCAL %s BLOCKED" >&2\nexit 99\n' "$b" > "$WORK/blocklocal/$b"; done
chmod +x "$WORK/shim/ssh" "$WORK/blocklocal"/*
command -v docker >/dev/null 2>&1 || die "docker is required"
docker image inspect "$IMAGE" >/dev/null 2>&1 || docker pull -q "$IMAGE" >/dev/null || die "cannot get $IMAGE"
ssh-keygen -q -t ed25519 -N '' -f "$WORK/key" -C parity-nodes; KEY=$WORK/key
MAN=$HERE/internal/nodes/provision/manifest.yaml # dependency names, read from the embedded manifest
BINS=$(awk '/^ *binary:/{gsub(/"/,"",$2); if ($2!="") print $2}' "$MAN" | sort -u | tr '\n' ' ')
PKGS=$(awk '/^ *apt_package:/{p=$2} /^ *binary:/{gsub(/"/,"",$2); if ($2=="") print p}' "$MAN" | tr '\n' ' ')
[ -n "$BINS" ] && [ -n "$PKGS" ] || die "could not read dependencies from $MAN"
# mkstub <name> <exit code of an injected failure> <body>: log argv, fail when an argv contains a /tmp/stub.fail line
FAILCHK='if [ -f /tmp/stub.fail ]; then while IFS= read -r p; do [ -n "$p" ] && case "$*" in *"$p"*) echo "stub: injected failure for $p" >&2; exit CODE;; esac; done < /tmp/stub.fail; fi'
mkstub() { printf '#!/bin/sh\necho "%s $*" >> /tmp/stub.log\n%s\n%s\n' "$1" "${FAILCHK/CODE/$2}" "$3" > "$WORK/stubs/$1"; }
mkstub sudo 100 'case "$1" in tee) cat >/dev/null;; grep) exit 1;; mkdir) for l in "$@"; do :; done; case "$l" in /opt/*) exec mkdir "$@";; esac;; esac'
mkstub apt-get 100 :; mkstub tar 100 :
mkstub curl 22 'o=; p=; for a in "$@"; do [ "$p" = -o ] && o=$a; p=$a; done; [ -n "$o" ] && : > "$o"; case "$*" in *releases/latest*) echo "  \"tag_name\": \"v9.9.9\",";; esac'
mkstub dpkg 1 '[ "$1" = -s ] && [ -f /tmp/dpkg.installed ] && grep -qxF "$2" /tmp/dpkg.installed'
mkstub ldd 1 'case "$1" in *broken*) echo "	libparity.so.1 => not found";; *) echo "	libc.so => /lib/libc.so";; esac'
chmod +x "$WORK/stubs"/*
start_node() { # start_node <name>: sets NODE_PORT (not in a subshell: NODES must keep the name for the trap)
  local name=$1 port i; NODES="$NODES $name"
  # the --add-host lines point the GitHub names at the container itself: a missing stub still could not reach them
  docker run -d --name "$name" --add-host api.github.com:127.0.0.1 --add-host github.com:127.0.0.1 --add-host cli.github.com:127.0.0.1 \
    --add-host objects.githubusercontent.com:127.0.0.1 -e USER_NAME=ci -e PUBLIC_KEY="$(cat "$KEY.pub")" -e SUDO_ACCESS=false \
    -e PASSWORD_ACCESS=false -p 127.0.0.1::2222 "$IMAGE" >/dev/null || die "cannot start $name"
  port=$(docker port "$name" 2222/tcp | head -1 | sed 's/.*://')
  for i in $(seq 1 60); do
    "$REAL_SSH" -i "$KEY" -p "$port" -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
      -o ConnectTimeout=2 ci@127.0.0.1 true >/dev/null 2>&1 && { NODE_PORT=$port; return 0; }; sleep 1
  done; die "$name did not accept ssh"
}
start_node "nself-parity-nodes-a-$$"; PA=$NODE_PORT; start_node "nself-parity-nodes-b-$$"; PB=$NODE_PORT
printf 'Host node-a\n  HostName 127.0.0.1\n  Port %s\nHost node-b\n  HostName 127.0.0.1\n  Port %s\nHost down\n  HostName 127.0.0.1\n  Port 1\n' \
  "$PA" "$PB" > "$WORK/home/.ssh/config"; chmod 600 "$WORK/home/.ssh/config" "$KEY"
# the stubs live in /usr/local/sbin, first in the ssh PATH; no dependency stub is ever put there
for n in $NODES; do
  docker exec -u root "$n" sh -c 'mkdir -p /usr/local/sbin /usr/local/bin'
  for s in sudo apt-get tar curl dpkg; do docker cp "$WORK/stubs/$s" "$n:/usr/local/sbin/$s" >/dev/null; done
  docker cp "$WORK/stubs/ldd" "$n:/tmp/ldd.stub" >/dev/null; docker exec -u root "$n" sh -c 'chmod 755 /usr/local/sbin/*'
done
on_nodes() { local n; for n in $NODES; do docker exec -u root "$n" sh -c "$1"; done; }
dep_stubs() { # shell text that makes every dependency present
  local b p s=""
  for b in $BINS; do s="$s printf '#!/bin/sh\nexit 0\n' > /usr/local/bin/$b; chmod 755 /usr/local/bin/$b; echo $b >> /tmp/depstubs;"; done
  for p in $PKGS; do s="$s echo $p >> /tmp/dpkg.installed;"; done; echo "$s"
}
chrome_at() { echo "f=/home/tester/.cache/ms-playwright/$1; mkdir -p \$(dirname \$f) && printf '#!/bin/sh\nexit 0\n' > \$f && chmod 755 \$f;"; }
apply_state() { # apply_state <name>: bare, deps, workdir, symlink, chrome_ok, chrome_bad, config, drift (A only)
  local ldd="cp /tmp/ldd.stub /usr/local/bin/ldd; chmod 755 /usr/local/bin/ldd;" wd="mkdir -p /opt/actions-runner/_work;"
  case $1 in
    bare) ;;
    deps) on_nodes "$(dep_stubs)";;
    workdir) on_nodes "$(dep_stubs) $wd";;
    symlink) on_nodes "$(dep_stubs) mkdir -p /opt/real-work && ln -s /opt/real-work /opt/actions-runner/_work";;
    chrome_ok) on_nodes "$(dep_stubs) $wd $ldd $(chrome_at chromium-1/chrome-linux/chrome)";;
    chrome_bad) on_nodes "$(dep_stubs) $wd $ldd $(chrome_at chromium_headless_shell-9/chrome-linux/headless_shell) $(chrome_at chromium-broken-2/chrome-linux/chrome)";;
    config) on_nodes 'mkdir -p /opt/actions-runner/runner-1 && : > /opt/actions-runner/runner-1/config.sh';;
    drift) set -- $NODES; docker exec -u root "$1" sh -c "$(dep_stubs)";;
    *) die "unknown state $1";;
  esac
}
reset_nodes() { # clean slate, the case's state and failure patterns; then prove the stubs are what ssh finds first
  local al t got p
  on_nodes 'for f in $(cat /tmp/depstubs 2>/dev/null); do rm -f "/usr/local/bin/$f"; done
    rm -f /tmp/depstubs /tmp/stub.log /tmp/stub.fail /tmp/dpkg.installed /usr/local/bin/ldd; rm -rf /opt/actions-runner /opt/ar2 /home/tester
    mkdir -p /opt/actions-runner /opt/ar2 && chown ci:ci /opt/actions-runner /opt/ar2; : > /tmp/stub.log && chmod 666 /tmp/stub.log'
  apply_state "$STATE"
  for al in node-a node-b; do for t in sudo curl apt-get tar dpkg; do
    got=$("$REAL_SSH" -F "$WORK/home/.ssh/config" -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -o LogLevel=ERROR \
      -o BatchMode=yes -i "$KEY" "ci@$al" "command -v $t" </dev/null 2>&1 || true)
    [ "$got" = "/usr/local/sbin/$t" ] || die "stub $t is not first on the PATH of $al (got: $got)"
  done; done
  for p in ${FAIL_PATS[@]+"${FAIL_PATS[@]}"}; do on_nodes "echo '$p' >> /tmp/stub.fail"; done
}

# ---- cases: STATE=<node state> WANT_RC=<expected core exit code> V|P <label rest> args --------------------
A=ci@node-a; B=ci@node-b; D=ci@down; TOK=FAKE-RUNNER-TOKEN-parity-0000; URL=https://github.com/nself-org/example; KO=(--ssh-key "$KEY")
V() { pair "verify $1" 1 --host "$A" "${KO[@]}" "${@:2}"; }                                    # verify against node A
P() { pair "provision $1" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK" "${@:2}"; } # provision node A
STATE=bare WANT_RC=1 V "bare host: missing dependencies fail"
STATE=bare WANT_RC=1 V "bare host --json" --json
STATE=deps WANT_RC=0 V "all dependencies, work dir absent (warn)"
STATE=workdir WANT_RC=0 V "real work dir, no chromium (warn)"
STATE=chrome_ok WANT_RC=0 V "chromium resolves its libraries"
STATE=symlink WANT_RC=1 V "work dir is a symlink"
STATE=chrome_bad WANT_RC=1 V "chromium missing libraries"
STATE=chrome_bad WANT_RC=1 V "chromium missing libraries --json" --json
STATE=bare WANT_RC=1 pair "verify unreachable host" 1 --host "$D" "${KO[@]}"
STATE=bare WANT_RC=1 pair "verify unreachable host --json" 1 --host "$D" "${KO[@]}" --json
STATE=workdir WANT_RC=1 pair "verify reachable plus unreachable" 1 --host "$A" --host "$D" "${KO[@]}"
STATE=drift WANT_RC=1 pair "verify drift between two hosts" 1 --host "$A" --host "$B" "${KO[@]}"
STATE=drift WANT_RC=1 pair "verify drift --json" 1 --host "$A,$B" "${KO[@]}" --json
STATE=workdir WANT_RC=0 pair "verify two identical hosts, no drift" 1 --host "$A" --host "$B" "${KO[@]}"
STATE=deps WANT_RC=0 ENVV="NSELF_DEPLOY_KEY_PATH=$KEY" pair "verify key from NSELF_DEPLOY_KEY_PATH" 1 --host "$A"
STATE=deps WANT_RC=0 ENVV="NSELF_DEPLOY_SSH_KEY=$KEY" pair "verify key from NSELF_DEPLOY_SSH_KEY" 1 --host "$A"
# destination checks that run before ssh; --json shows the message the text matrix hides
export WANT_RC=1 # (the cases below run no ssh)
for h in "" "-oProxyCommand=x" "ci@a b" "$(printf 'ci@a\tb')"; do pair "verify bad host [$h] --json" 1 --host "$h" "${KO[@]}" --json; done
pair "verify empty host" 1 --host "" "${KO[@]}"
pair "verify missing key file --json" 1 --host "$A" --ssh-key "$WORK/no-such-key" --json
# this machine: both run the read-only checks locally; nothing is installed
WANT_RC= pair "verify default host is this machine" 1
WANT_RC= pair "verify --host local" 1 --host local
pair "verify local plus unreachable" 1 --host local --host "$D" "${KO[@]}"
# provision: success, options, a failure at every step (the stubs exit with the injected status)
WANT_RC=0; STATE=bare
P "one instance"
P "two instances, labels, install root" --instances 2 --install-root /opt/ar2 --labels gpu,fast --labels extra
pair "provision --flag=value forms" 1 --host="$A" --ssh-key="$KEY" --github-url="$URL" --token="$TOK" --instances=1
P "instances 0 runs one" --instances 0
ENVV="GITHUB_RUNNER_TOKEN=$TOK" pair "provision token from GITHUB_RUNNER_TOKEN" 1 --host "$A" "${KO[@]}" --github-url "$URL"
ENVV="GITHUB_RUNNER_TOKEN=FAKE-ENV-OTHER-0000" P "--token overrides the env var"
STATE=config P "with config.sh already present"
STATE=symlink WANT_RC=1 P "refuses a symlinked work dir"
export WANT_RC=100
FAIL_PATS=(apt-get); P "fails at install-packages"
FAIL_PATS=(useradd); P "fails at create-runner-user"
FAIL_PATS=(visudo); P "fails at grant-passwordless-sudo"
FAIL_PATS=(config.sh); P "fails at runner registration" --instances 2
FAIL_PATS=("svc.sh start"); P "fails at service start"
FAIL_PATS=(actions-runner-linux); WANT_RC=22 P "fails at the runner download"
WANT_RC=255 pair "provision unreachable host" 1 --host "$D" "${KO[@]}" --github-url "$URL" --token "$TOK"
# (--host "" would mean this machine, as in core: deliberately not run) refusals before any host work:
export WANT_RC=1
pair "provision without --github-url" 0
pair "provision without --github-url, two hosts" 0 --host a --host b
pair "provision without a token" 0 --github-url "$URL"
pair "provision with two hosts" 0 --github-url "$URL" --token "$TOK" --host a --host b
pair "provision with two hosts as CSV" 0 --github-url "$URL" --token "$TOK" --host a,b
pair "provision with local plus a host" 0 --github-url "$URL" --token "$TOK" --host local --host "$A"
pair "provision bad --instances" 0 --instances x --github-url "$URL" --token "$TOK" --host "$A"
# flag errors, help, positional arguments
for c in provision verify; do
  WANT_RC=1; for a in --bogus --bogus=1 -x --host ---x; do pair "$c flag error $a" 0 "$a"; done
  pair "$c --help after a bad flag" 0 --bogus --help
  WANT_RC=0; pair "$c --help" 1 --help; pair "$c -h" 1 -h
done
STATE=deps WANT_RC=0 pair "provision --help=false does not print help" 1 --help=false --github-url "$URL" --token "$TOK" --host "$A" "${KO[@]}"
STATE=deps WANT_RC=0 V "--help=false does not print help" --help=false
STATE=deps WANT_RC=0 V "ignores positional arguments" extra more
STATE=deps WANT_RC=0 V "-- ends the flags" -- --bogus
WANT_RC=1 pair "verify bad --json value" 0 --json=maybe

[ "$CASES" -gt 0 ] || { echo "parity-nodes: FAIL (no case ran)"; exit 1; }
if [ $FAILS -eq 0 ]; then echo "parity-nodes: PASS ($CASES cases)"; exit 0; fi
echo "parity-nodes: FAIL ($FAILS of $CASES cases differ)"; exit 1
