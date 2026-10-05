#!/usr/bin/env bash
# parity-nodes.sh: prove `nself-ci nodes verify|provision` equals core `nself runner verify|provision` (P7-CANON-10).
# Purpose: run core (a cli origin/main build) and the head nself-ci against disposable local sshd containers and
#   diff stdout, stderr, exit code, the exact ssh argv sent (so every remote script is byte-compared) and the
#   command sequence the container's stubbed sudo/curl/apt-get/dpkg/ldd recorded. Spelling "nself runner" is
#   normalised to "nself ci nodes" in text; the "--" the sdk puts before the ssh destination is dropped from the
#   argv log (the one intended argv difference). Provision never runs against this machine: every provision
#   case names a container host or is refused before any host work, and a blocking sudo/curl/apt-get on the
#   local PATH makes a stray local run fail harmlessly.
# Hosts:   two containers of linuxserver/openssh-server pinned by digest (below), key auth with a key pair
#   generated in $TMPDIR, published on 127.0.0.1 only, removed on exit (trap). No other host is contacted: the
#   container's sudo, curl, apt-get, tar, dpkg, ldd and the dependency binaries are recording stubs.
# Inputs:  run from the root of the plugins worktree (HEAD = the change under test).
#   NSELF            checkout holding cli/ (the core build comes from cli origin/main; required)
#   TMPDIR           scratch parent (required)
#   PARITY_HEAD_SRC  directory of the head free/ci sources (default: free/ci of this worktree); the negative
#                    proofs point it at a mutated copy.
#   PARITY_CLI_REF   cli ref for the core build (default origin/main)
#   PARITY_NSELF_BIN prebuilt core nself (skips that build)
#   PARITY_ONLY      extended regex: run only the cases whose label matches (negative proofs; a run of
#                    zero cases fails)
#   PARITY_KEEP      set to keep the scratch directory (captures under o/) for debugging
# Flags:   --self-test     prove the comparator: a planted difference must be reported, identical sides pass.
#          --capture-help  rewrite internal/nodes/provision/help/*.txt from the core build, then exit.
# Output:  one "ok <case>" line per case; "DIFF" blocks; "parity-nodes: PASS (N cases)" or FAIL.
# Exit:    0 all cases equal, 1 a difference or an empty capture, 2 usage, build or docker failure.
# Documented deltas (not compared): core `provision --json` prints core's E402 error, the plugin a plain
#   "does not support --json" error; a destination that passes core's legacy check but not the sdk allowlist
#   is refused by the plugin before ssh (unit tests); the plugin's ssh runs with the sdk environment allowlist.
set -euo pipefail

MODE=run
case "${1:-}" in
  --self-test) MODE=self;; --capture-help) MODE=help;; "") ;;
  *) echo "usage: parity-nodes.sh [--self-test|--capture-help]" >&2; exit 2;;
esac
: "${TMPDIR:?TMPDIR must be set}"
IMAGE="lscr.io/linuxserver/openssh-server@sha256:46f115de7c251558297e7e87566fc3fc08544b63e55502b5cf294454db5d29d1"
HERE=$(cd "$(dirname "$0")/.." && pwd)            # free/ci
HEAD_SRC=${PARITY_HEAD_SRC:-$HERE}
CLI_REF=${PARITY_CLI_REF:-origin/main}
CLI_LOCK=${PARITY_CLI_LOCK:-/tmp/nself-cli-compile.lock}
PLUG_LOCK=${PARITY_PLUGINS_LOCK:-/tmp/nself-plugins-compile.lock}
REAL_SSH=$(command -v ssh) || { echo "parity-nodes: ssh is required" >&2; exit 2; }

WORK=$(mktemp -d "$TMPDIR/parity-nodes.XXXXXX"); WORK=$(cd "$WORK" && pwd)
NODES=""
cleanup() {
  for n in $NODES; do docker rm -f "$n" >/dev/null 2>&1 || true; done
  if [ -n "${NSELF:-}" ] && [ -d "${NSELF}/cli" ]; then git -C "$NSELF/cli" worktree remove --force "$WORK/cli" >/dev/null 2>&1 || true; fi
  if [ -n "${PARITY_KEEP:-}" ]; then echo "parity-nodes: kept $WORK" >&2; else rm -rf "$WORK"; fi
}
trap cleanup EXIT
die() { echo "parity-nodes: $*" >&2; exit 2; }
locked() { local lock=$1; shift; if command -v lockf >/dev/null 2>&1; then lockf -k "$lock" "$@"; else "$@"; fi; }

# ---- builds -------------------------------------------------------------------------------------
mkdir -p "$WORK/head" "$WORK/core"
( cd "$HEAD_SRC" && CGO_ENABLED=0 locked "$PLUG_LOCK" go build -o "$WORK/head/nself-ci" ./cmd/ ) || die "cannot build nself-ci from $HEAD_SRC"
if [ -n "${PARITY_NSELF_BIN:-}" ]; then cp "$PARITY_NSELF_BIN" "$WORK/core/nself"
else
  : "${NSELF:?NSELF must point at the checkout that holds cli/}"
  git -C "$NSELF/cli" worktree add --detach "$WORK/cli" "$CLI_REF" >/dev/null 2>&1 || die "cannot check out cli $CLI_REF"
  ( cd "$WORK/cli" && CGO_ENABLED=0 locked "$CLI_LOCK" go build -o "$WORK/core/nself" ./cmd/nself ) || die "cannot build core nself"
fi
CORE=$WORK/core/nself; HEADBIN=$WORK/head/nself-ci

if [ "$MODE" = help ]; then
  for c in provision verify; do
    "$CORE" runner "$c" --help | sed 's/nself runner /nself ci nodes /g' > "$HERE/internal/nodes/provision/help/$c.txt"
    [ -s "$HERE/internal/nodes/provision/help/$c.txt" ] || die "empty help for $c"
  done
  echo "captured help for provision and verify"; exit 0
fi

# ---- comparator ---------------------------------------------------------------------------------
norm() {
  sed -E -e "s#$WORK#<W>#g" -e 's/\[127\.0\.0\.1\]:[0-9]+/[127.0.0.1]:PORT/g' -e 's/port [0-9]+/port N/g' \
    -e 's/nself runner /nself ci nodes /g'
}
CASES=0; FAILS=0
# capture <dir> <env...> -- cmd args: stdout, stderr, rc, ssh argv log, container stub logs
capture() {
  local dir=$1; shift; mkdir -p "$dir"
  local envs=(); while [ "$1" != "--" ]; do envs+=("$1"); shift; done; shift
  reset_nodes; : > "$WORK/ssh.log"; rm -f "$WORK/home/.ssh/known_hosts"
  local rc=0
  ( cd "$WORK/cwd" && env -u GITHUB_RUNNER_TOKEN -u NSELF_DEPLOY_KEY_PATH -u NSELF_DEPLOY_SSH_KEY \
      HOME="$WORK/home" PATH="$WORK/shim:$WORK/blocklocal:$PATH" NO_COLOR=1 ${envs[@]+"${envs[@]}"} \
      "$@" >"$dir/out" 2>"$dir/err" ) || rc=$?
  echo "$rc" > "$dir/rc"
  norm < "$dir/out" > "$dir/out.n"; norm < "$dir/err" > "$dir/err.n"
  { grep -vx -e '--' "$WORK/ssh.log" || true; } | norm > "$dir/ssh.n"
  # sorted: the stages of a pipeline in one remote script (curl | sudo tee) log in either order
  { for n in $NODES; do echo "== node"; docker exec "$n" sh -c 'sort /tmp/stub.log 2>/dev/null || true'; done; } | norm > "$dir/stub.n"
}
ENVV=(); STATE=st_bare; WANT_RC=""; FAIL_PATS=()
# pair <label> <nonempty: 1 stdout, 0 stderr> args...: core `runner ARGS` against head `nodes ARGS`
pair() {
  local label=$1 ne=$2; shift 2
  if [ -n "${PARITY_ONLY:-}" ] && ! [[ $label =~ $PARITY_ONLY ]]; then return 0; fi
  local cmd=${label%% *}   # the first word of the label is the subcommand: provision or verify
  local id; id=$(printf '%s' "$label" | tr -c 'A-Za-z0-9\n' '_')
  CASES=$((CASES + 1))
  $STATE
  capture "$WORK/o/$id/a" ${ENVV[@]+"${ENVV[@]}"} -- "$CORE" runner "$cmd" "$@"
  capture "$WORK/o/$id/b" ${ENVV[@]+"${ENVV[@]}"} -- "$HEADBIN" nodes "$cmd" "$@"
  verdict "$label" "$id" "$ne"
  FAIL_PATS=()
}
verdict() {
  local label=$1 id=$2 ne=$3 bad=0 ext
  for ext in out.n err.n rc ssh.n stub.n; do
    if ! cmp -s "$WORK/o/$id/a/$ext" "$WORK/o/$id/b/$ext"; then
      bad=1; echo "DIFF $label ($ext):"; diff -u "$WORK/o/$id/a/$ext" "$WORK/o/$id/b/$ext" | head -40 || true
    fi
  done
  if [ "$ne" = 1 ] && ! [ -s "$WORK/o/$id/a/out.n" ]; then bad=1; echo "EMPTY $label: nothing on stdout (vacuous)"; fi
  if [ "$ne" = 0 ] && ! [ -s "$WORK/o/$id/a/err.n" ]; then bad=1; echo "EMPTY $label: nothing on stderr (vacuous)"; fi
  if [ -n "$WANT_RC" ] && [ "$(cat "$WORK/o/$id/a/rc")" != "$WANT_RC" ]; then
    bad=1; echo "RC $label: exit code $(cat "$WORK/o/$id/a/rc"), expected $WANT_RC (the case did not exercise what it claims)"; fi
  if [ $bad -eq 0 ]; then echo "ok $label"; else FAILS=$((FAILS + 1)); fi
}

# ---- fixture: shims and containers --------------------------------------------------------------
mkdir -p "$WORK/shim" "$WORK/blocklocal" "$WORK/cwd" "$WORK/home/.ssh" "$WORK/o"
chmod 700 "$WORK/home/.ssh"
# the shim records the argv it was given, then adds -F (OpenSSH reads the passwd home, not $HOME, for
# ~/.ssh/config) and a known_hosts file under the scratch HOME; both sides go through the same shim
cat > "$WORK/shim/ssh" <<SHIM
#!/bin/sh
{ for a in "\$@"; do printf '%s\n' "\$a"; done; echo '---'; } >> "$WORK/ssh.log"
exec "$REAL_SSH" -F "$WORK/home/.ssh/config" -o "UserKnownHostsFile=$WORK/home/.ssh/known_hosts" "\$@"
SHIM
chmod +x "$WORK/shim/ssh"
# a provision that reached this machine would run sudo, apt-get or curl: block them (and log the attempt)
for b in sudo apt-get curl; do
  printf '#!/bin/sh\necho "LOCAL %s BLOCKED" >&2\nexit 99\n' "$b" > "$WORK/blocklocal/$b"; chmod +x "$WORK/blocklocal/$b"
done
# a stub self-test needs no containers
if [ "$MODE" = self ]; then
  reset_nodes() { :; }
  mkdir -p "$WORK/shim"; printf '#!/bin/sh\necho "$@"\n' > "$WORK/st-a"; printf '#!/bin/sh\necho "$@" changed\n' > "$WORK/st-b"; chmod +x "$WORK/st-a" "$WORK/st-b"
  STATE=:; CASES=0; FAILS=0
  capture "$WORK/o/s1/a" -- "$WORK/st-a" x; capture "$WORK/o/s1/b" -- "$WORK/st-a" x; verdict "self identical" s1 1
  [ $FAILS -eq 0 ] || { echo "parity-nodes self-test: FAIL (identical sides reported a difference)"; exit 1; }
  capture "$WORK/o/s2/a" -- "$WORK/st-a" x; capture "$WORK/o/s2/b" -- "$WORK/st-b" x
  out=$(verdict "self planted" s2 1 2>&1 || true)
  printf '%s\n' "$out" | grep -q '^DIFF self planted' || { echo "parity-nodes self-test: FAIL (planted difference not reported)"; exit 1; }
  WANT_RC=9; out=$(verdict "self rc" s1 1 2>&1 || true)
  printf '%s\n' "$out" | grep -q '^RC self rc' || { echo "parity-nodes self-test: FAIL (unexpected exit code not reported)"; exit 1; }
  echo "parity-nodes self-test: PASS (identical sides equal, planted difference and wrong exit code reported)"; exit 0
fi

command -v docker >/dev/null 2>&1 || die "docker is required"
docker image inspect "$IMAGE" >/dev/null 2>&1 || docker pull -q "$IMAGE" >/dev/null || die "cannot get $IMAGE"
ssh-keygen -q -t ed25519 -N '' -f "$WORK/key" -C parity-nodes
KEY=$WORK/key
chmod 600 "$KEY"

# manifest facts, read from the embedded manifest so the stubs follow it
MAN=$HERE/internal/nodes/provision/manifest.yaml
BINS=$(awk '/^ *- name:/{b=""} /^ *binary:/{gsub(/"/,"",$2); if ($2!="") print $2}' "$MAN" | sort -u | tr '\n' ' ')
PKGS=$(awk '/^ *apt_package:/{p=$2} /^ *binary:/{gsub(/"/,"",$2); if ($2=="") print p}' "$MAN" | tr '\n' ' ')
[ -n "$BINS" ] && [ -n "$PKGS" ] || die "could not read dependencies from $MAN"

# stubs, copied into each container
mkdir -p "$WORK/stubs"
cat > "$WORK/stubs/sudo" <<'STUB'
#!/bin/sh
echo "sudo $*" >> /tmp/stub.log
if [ -f /tmp/stub.fail ]; then
  while IFS= read -r pat; do
    [ -n "$pat" ] || continue
    case "$*" in *"$pat"*) echo "stub: injected failure for $pat" >&2; exit 100;; esac
  done < /tmp/stub.fail
fi
case "$1" in
  tee) cat >/dev/null; exit 0;;
  grep) exit 1;;
  mkdir) for last in "$@"; do :; done; case "$last" in /opt/*) exec mkdir "$@";; esac; exit 0;;
esac
exit 0
STUB
for t in apt-get tar; do
  printf '#!/bin/sh\necho "%s $*" >> /tmp/stub.log\nif [ -f /tmp/stub.fail ] && grep -qxF "%s" /tmp/stub.fail; then echo "stub: injected failure for %s" >&2; exit 100; fi\nexit 0\n' "$t" "$t" "$t" > "$WORK/stubs/$t"
done
cat > "$WORK/stubs/curl" <<'STUB'
#!/bin/sh
echo "curl $*" >> /tmp/stub.log
if [ -f /tmp/stub.fail ]; then
  while IFS= read -r pat; do
    [ -n "$pat" ] || continue
    case "$*" in *"$pat"*) echo "stub: injected failure for $pat" >&2; exit 22;; esac
  done < /tmp/stub.fail
fi
out=""; prev=""
for a in "$@"; do [ "$prev" = "-o" ] && out=$a; prev=$a; done
[ -n "$out" ] && : > "$out"
case "$*" in *releases/latest*) echo '  "tag_name": "v9.9.9",';; esac
exit 0
STUB
cat > "$WORK/stubs/dpkg" <<'STUB'
#!/bin/sh
echo "dpkg $*" >> /tmp/stub.log
[ "$1" = "-s" ] && [ -f /tmp/dpkg.installed ] && grep -qxF "$2" /tmp/dpkg.installed && exit 0
exit 1
STUB
cat > "$WORK/stubs/ldd" <<'STUB'
#!/bin/sh
echo "ldd $*" >> /tmp/stub.log
case "$1" in *broken*) echo "	libparity.so.1 => not found"; echo "	libparity2.so.1 => not found";; *) echo "	libc.so => /lib/libc.so";; esac
STUB
chmod +x "$WORK/stubs"/*

start_node() { # start_node <name>: sets NODE_PORT (not a subshell: NODES must record the name for the trap)
  local name=$1 port i
  NODES="$NODES $name"
  # the --add-host lines point the GitHub names at the container itself: a stub that went missing could
  # not reach the real hosts (defence in depth; the stubs are the first line)
  docker run -d --name "$name" --add-host api.github.com:127.0.0.1 --add-host github.com:127.0.0.1 \
    --add-host cli.github.com:127.0.0.1 --add-host objects.githubusercontent.com:127.0.0.1 -e USER_NAME=ci -e PUBLIC_KEY="$(cat "$KEY.pub")" -e SUDO_ACCESS=false \
    -e PASSWORD_ACCESS=false -p 127.0.0.1::2222 "$IMAGE" >/dev/null || die "cannot start $name"
  port=$(docker port "$name" 2222/tcp | head -1 | sed 's/.*://')
  for i in $(seq 1 60); do
    "$REAL_SSH" -i "$KEY" -p "$port" -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
      -o LogLevel=ERROR -o ConnectTimeout=2 ci@127.0.0.1 true >/dev/null 2>&1 && { NODE_PORT=$port; return 0; }
    sleep 1
  done
  die "$name did not accept ssh"
}
SUF=$$
start_node "nself-parity-nodes-a-$SUF"; PA=$NODE_PORT
start_node "nself-parity-nodes-b-$SUF"; PB=$NODE_PORT
cat > "$WORK/home/.ssh/config" <<CFG
Host node-a
  HostName 127.0.0.1
  Port $PA
Host node-b
  HostName 127.0.0.1
  Port $PB
Host down
  HostName 127.0.0.1
  Port 1
CFG
chmod 600 "$WORK/home/.ssh/config"
# /usr/local/sbin comes before /usr/local/bin and /usr/bin in the ssh PATH, and no dependency stub is ever
# placed there, so the recording stubs always win over the real tools (reset_nodes asserts it every case)
for n in $NODES; do
  docker exec -u root "$n" sh -c 'mkdir -p /usr/local/sbin /usr/local/bin'
  for s in sudo apt-get tar curl dpkg; do docker cp "$WORK/stubs/$s" "$n:/usr/local/sbin/$s" >/dev/null; done
  docker cp "$WORK/stubs/ldd" "$n:/tmp/ldd.stub" >/dev/null
  docker exec -u root "$n" sh -c 'chmod 755 /usr/local/sbin/*'
done

# reset_nodes: clean slate on every node, then apply the case's state function (run by pair via $STATE)
reset_nodes() {
  local n
  for n in $NODES; do
    docker exec -u root "$n" sh -c '
      for f in $(cat /tmp/depstubs 2>/dev/null); do rm -f "/usr/local/bin/$f"; done
      rm -f /tmp/depstubs /tmp/stub.log /tmp/stub.fail /tmp/dpkg.installed /usr/local/bin/ldd
      rm -rf /opt/actions-runner /opt/ar2 /home/tester
      mkdir -p /opt/actions-runner /opt/ar2 && chown ci:ci /opt/actions-runner /opt/ar2
      : > /tmp/stub.log && chmod 666 /tmp/stub.log'
  done
  [ -z "${NODE_STATE:-}" ] || $NODE_STATE
  local al t got # a case must never run a real sudo, curl, apt-get, tar or dpkg
  for al in node-a node-b; do
    for t in sudo curl apt-get tar dpkg; do
      got=$("$REAL_SSH" -F "$WORK/home/.ssh/config" -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
        -o LogLevel=ERROR -o BatchMode=yes -i "$KEY" "ci@$al" "command -v $t" </dev/null 2>&1 || true)
      [ "$got" = "/usr/local/sbin/$t" ] || die "stub $t is not first on the PATH of $al (got: $got)"
    done
  done
  local p; for p in ${FAIL_PATS[@]+"${FAIL_PATS[@]}"}; do on_nodes "echo '$p' >> /tmp/stub.fail"; done
}
on_nodes() { local n; for n in $NODES; do docker exec -u root "$n" sh -c "$1"; done; }
st_bare() { NODE_STATE=; }
st_deps() { # every dependency present
  NODE_STATE=deps_apply
}
deps_apply() {
  local b p script=""
  for b in $BINS; do script="$script printf '#!/bin/sh\nexit 0\n' > /usr/local/bin/$b; chmod 755 /usr/local/bin/$b; echo $b >> /tmp/depstubs;"; done
  for p in $PKGS; do script="$script echo $p >> /tmp/dpkg.installed;"; done
  on_nodes "$script"
}
st_workdir() { NODE_STATE=workdir_apply; }
workdir_apply() { deps_apply; on_nodes 'mkdir -p /opt/actions-runner/_work'; }
st_symlink() { NODE_STATE=symlink_apply; }
symlink_apply() { deps_apply; on_nodes 'mkdir -p /opt/real-work && ln -s /opt/real-work /opt/actions-runner/_work'; }
st_chromium_ok() { NODE_STATE=chromium_ok_apply; }
chromium_ok_apply() {
  workdir_apply
  on_nodes 'cp /tmp/ldd.stub /usr/local/bin/ldd; chmod 755 /usr/local/bin/ldd; mkdir -p /home/tester/.cache/ms-playwright/chromium-1/chrome-linux && printf "#!/bin/sh\nexit 0\n" > /home/tester/.cache/ms-playwright/chromium-1/chrome-linux/chrome && chmod 755 /home/tester/.cache/ms-playwright/chromium-1/chrome-linux/chrome'
}
st_chromium_broken() { NODE_STATE=chromium_broken_apply; }
chromium_broken_apply() {
  workdir_apply
  on_nodes 'cp /tmp/ldd.stub /usr/local/bin/ldd; chmod 755 /usr/local/bin/ldd; mkdir -p /home/tester/.cache/ms-playwright/chromium_headless_shell-9/chrome-linux && printf "#!/bin/sh\nexit 0\n" > /home/tester/.cache/ms-playwright/chromium_headless_shell-9/chrome-linux/headless_shell && chmod 755 /home/tester/.cache/ms-playwright/chromium_headless_shell-9/chrome-linux/headless_shell; mkdir -p /home/tester/.cache/ms-playwright/chromium-broken-2/chrome-linux && printf "#!/bin/sh\nexit 0\n" > /home/tester/.cache/ms-playwright/chromium-broken-2/chrome-linux/chrome && chmod 755 /home/tester/.cache/ms-playwright/chromium-broken-2/chrome-linux/chrome'
}
st_config_present() { NODE_STATE=config_apply; }
config_apply() { on_nodes 'mkdir -p /opt/actions-runner/runner-1 && : > /opt/actions-runner/runner-1/config.sh'; }
# a state only on node A: A has every dependency, B stays bare (drift)
st_drift() { NODE_STATE=drift_apply; }
drift_apply() {
  local n first=1 b p script=""
  for b in $BINS; do script="$script printf '#!/bin/sh\nexit 0\n' > /usr/local/bin/$b; chmod 755 /usr/local/bin/$b; echo $b >> /tmp/depstubs;"; done
  for p in $PKGS; do script="$script echo $p >> /tmp/dpkg.installed;"; done
  for n in $NODES; do [ $first -eq 1 ] && docker exec -u root "$n" sh -c "$script"; first=0; done
}
fail_on() { FAIL_PATS=("$@"); } # the stubs refuse a command containing a pattern, for the next case only

A=ci@node-a; B=ci@node-b; D=ci@down; TOK=FAKE-RUNNER-TOKEN-parity-0000
URL=https://github.com/nself-org/example
KO=(--ssh-key "$KEY")

# ---- verify: dependencies, work dir, chromium, hosts, output modes ------------------------------
WANT_RC=1; STATE=st_bare
pair "verify bare host: missing dependencies fail" 1 --host "$A" "${KO[@]}"
pair "verify bare host --json" 1 --host "$A" "${KO[@]}" --json
WANT_RC=0
STATE=st_deps;     pair "verify all dependencies, work dir absent (warn)" 1 --host "$A" "${KO[@]}"
STATE=st_workdir;  pair "verify real work dir, no chromium (warn)" 1 --host "$A" "${KO[@]}"
STATE=st_chromium_ok; pair "verify chromium resolves its libraries" 1 --host "$A" "${KO[@]}"
WANT_RC=1
STATE=st_symlink;  pair "verify work dir is a symlink" 1 --host "$A" "${KO[@]}"
STATE=st_chromium_broken; pair "verify chromium missing libraries" 1 --host "$A" "${KO[@]}"
STATE=st_chromium_broken; pair "verify chromium missing libraries --json" 1 --host "$A" "${KO[@]}" --json
STATE=st_bare;     pair "verify unreachable host" 1 --host "$D" "${KO[@]}"
STATE=st_bare;     pair "verify unreachable host --json" 1 --host "$D" "${KO[@]}" --json
STATE=st_workdir;  pair "verify reachable plus unreachable" 1 --host "$A" --host "$D" "${KO[@]}"
STATE=st_drift;    pair "verify drift between two hosts" 1 --host "$A" --host "$B" "${KO[@]}"
STATE=st_drift;    pair "verify drift --json" 1 --host "$A,$B" "${KO[@]}" --json
WANT_RC=0
STATE=st_workdir;  pair "verify two identical hosts, no drift" 1 --host "$A" --host "$B" "${KO[@]}"
ENVV=("NSELF_DEPLOY_KEY_PATH=$KEY"); STATE=st_deps
pair "verify key from NSELF_DEPLOY_KEY_PATH" 1 --host "$A"
ENVV=("NSELF_DEPLOY_SSH_KEY=$KEY")
pair "verify key from NSELF_DEPLOY_SSH_KEY" 1 --host "$A"
ENVV=()
# destination checks that run before ssh (the legacy core messages)
WANT_RC=1; STATE=st_bare
pair "verify empty host" 1 --host "" "${KO[@]}"
pair "verify empty host --json" 1 --host "" "${KO[@]}" --json
pair "verify host that looks like an option" 1 --host "-oProxyCommand=x" "${KO[@]}"
pair "verify host that looks like an option --json" 1 --host "-oProxyCommand=x" "${KO[@]}" --json
pair "verify host with whitespace --json" 1 --host "ci@a b" "${KO[@]}" --json
pair "verify host with a control character --json" 1 --host "$(printf 'ci@a\tb')" "${KO[@]}" --json
pair "verify missing key file --json" 1 --host "$A" --ssh-key "$WORK/no-such-key" --json
# this machine (core and the plugin both run read-only checks locally; nothing is installed)
WANT_RC=
pair "verify default host is this machine" 1
pair "verify --host local" 1 --host local
WANT_RC=1; pair "verify local plus unreachable" 1 --host local --host "$D" "${KO[@]}"

# ---- provision: success, options, failures at every step ----------------------------------------
WANT_RC=0; STATE=st_bare
pair "provision one instance" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK"
pair "provision two instances, labels, install root" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK" \
  --instances 2 --install-root /opt/ar2 --labels gpu,fast --labels extra
pair "provision --flag=value forms" 1 --host="$A" --ssh-key="$KEY" --github-url="$URL" --token="$TOK" --instances=1
pair "provision instances 0 runs one" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK" --instances 0
ENVV=("GITHUB_RUNNER_TOKEN=$TOK")
pair "provision token from GITHUB_RUNNER_TOKEN" 1 --host "$A" "${KO[@]}" --github-url "$URL"
ENVV=("GITHUB_RUNNER_TOKEN=FAKE-ENV-OTHER-0000")
pair "provision --token overrides the env var" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK"
ENVV=()
STATE=st_config_present
pair "provision with config.sh already present" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK"
WANT_RC=1
STATE=st_symlink
pair "provision refuses a symlinked work dir" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK"
STATE=st_bare; WANT_RC=100
fail_on "apt-get"; pair "provision fails at install-packages" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK"
fail_on "useradd"; pair "provision fails at create-runner-user" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK"
fail_on "visudo"; pair "provision fails at grant-passwordless-sudo" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK"
fail_on "config.sh"; pair "provision fails at runner registration" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK" --instances 2
fail_on "svc.sh start"; pair "provision fails at service start" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK"
WANT_RC=22
fail_on "actions-runner-linux"; pair "provision fails at the runner download" 1 --host "$A" "${KO[@]}" --github-url "$URL" --token "$TOK"
STATE=st_bare; WANT_RC=255
pair "provision unreachable host" 1 --host "$D" "${KO[@]}" --github-url "$URL" --token "$TOK"
# (a provision with --host "" would mean this machine, as in core: it is deliberately not run here)

# ---- provision refusals before any host work (no --host would mean this machine: none of these get there)
WANT_RC=1; STATE=st_bare
pair "provision without --github-url" 0
pair "provision without --github-url, two hosts" 0 --host a --host b
pair "provision without a token" 0 --github-url "$URL"
pair "provision with two hosts" 0 --github-url "$URL" --token "$TOK" --host a --host b
pair "provision with two hosts as CSV" 0 --github-url "$URL" --token "$TOK" --host a,b
pair "provision with local plus a host" 0 --github-url "$URL" --token "$TOK" --host local --host "$A"
pair "provision bad --instances" 0 --instances x --github-url "$URL" --token "$TOK" --host "$A"

# ---- flag errors, help, positional arguments ---------------------------------------------------
for c in provision verify; do
  WANT_RC=1
  pair "$c unknown flag" 0 --bogus
  pair "$c unknown flag with value" 0 --bogus=1
  pair "$c unknown shorthand" 0 -x
  pair "$c flag needs an argument" 0 --host
  pair "$c bad flag syntax" 0 ---x
  pair "$c --help after a bad flag" 0 --bogus --help
  WANT_RC=0
  pair "$c --help" 1 --help
  pair "$c -h" 1 -h
done
STATE=st_deps
pair "provision --help=false does not print help" 1 --help=false --github-url "$URL" --token "$TOK" --host "$A" "${KO[@]}"
pair "verify --help=false does not print help" 1 --help=false --host "$A" "${KO[@]}"
STATE=st_deps; WANT_RC=0
pair "verify ignores positional arguments" 1 extra --host "$A" "${KO[@]}" more
pair "verify -- ends the flags" 1 --host "$A" "${KO[@]}" -- --bogus
WANT_RC=1
pair "verify bad --json value" 0 --json=maybe

[ "$CASES" -gt 0 ] || { echo "parity-nodes: FAIL (no case ran)"; exit 1; }
if [ $FAILS -eq 0 ]; then echo "parity-nodes: PASS ($CASES cases)"; exit 0; fi
echo "parity-nodes: FAIL ($FAILS of $CASES cases differ)"; exit 1
