#!/usr/bin/env bash
# parity-with-core.sh: prove the nself-ci binary is a drop-in for core `nself ci` (P7-CANON-09).
# Purpose: two legs, both diffing stdout, stderr and exit code (binary and temp paths normalised):
#   leg 1  core `nself ci ARGS` (cli origin/main build; its proxy runs the plugins origin/main binary
#          found first on PATH) versus the head `nself-ci ARGS` with the same core argv: --help of ci,
#          build, forgejo, serve; the gate on a clean and a planted-secret fixture with --no-status
#          --check (and --no-gitleaks, --verbose); forgejo with a recording stub docker (unreachable and
#          healthy server); serve refusing to start without a secret.
#   leg 2  deployed 1.4.x: the base cli with the plugins origin/main nself-ci first on PATH versus the
#          same base cli with the head nself-ci first on PATH, across the buildCIArgs flag matrix (each
#          of --check, --no-gitleaks, --no-status, --sha, --owner, --repo, -v, --filesystem, a repo-root
#          argument, no argument) plus `build` and `run --help` / `run` on both fixtures.
# Inputs:  run from the root of the plugins worktree (HEAD = the change under test).
#   NSELF            checkout holding cli/ (the core build comes from cli origin/main; required)
#   TMPDIR           scratch parent (required)
#   PARITY_HEAD_SRC  directory of the head free/ci sources (default: free/ci of this worktree); the
#                    negative proofs point it at a mutated copy.
#   PARITY_BASE_REF  plugins ref for the "deployed" binary (default origin/main)
#   PARITY_CLI_REF   cli ref for the core build (default origin/main)
#   PARITY_NSELF_BIN / PARITY_BASE_BIN: prebuilt core nself / plugins base nself-ci (skip those builds)
# Flags:   --self-test  prove the comparator itself: it must flag a planted difference, and a clean
#                       comparison of a binary against itself must pass.
# Output:  one "ok <leg> <case>" line per case; "DIFF" blocks; "parity: PASS (N cases)" or FAIL.
# Exit:    0 all cases equal, 1 a difference or an empty capture, 2 usage or build failure.
# Documented core-framing rule: the gate prints core's section header and "Error: gate failed" when
# --check is given (the one core flag the 1.4.x argv never sends); core argv `--no-status` without
# --check and `build` framing cannot be reproduced without changing the frozen proxy argv output.
set -euo pipefail

SELF_TEST=0
case "${1:-}" in --self-test) SELF_TEST=1;; "") ;; *) echo "usage: parity-with-core.sh [--self-test]" >&2; exit 2;; esac
: "${TMPDIR:?TMPDIR must be set}"
HERE=$(cd "$(dirname "$0")/.." && pwd)            # free/ci
ROOT=$(cd "$HERE/../.." && pwd)                    # plugins worktree root
HEAD_SRC=${PARITY_HEAD_SRC:-$HERE}
BASE_REF=${PARITY_BASE_REF:-origin/main}
CLI_REF=${PARITY_CLI_REF:-origin/main}
CLI_LOCK=${PARITY_CLI_LOCK:-/tmp/nself-cli-compile.lock}
PLUG_LOCK=${PARITY_PLUGINS_LOCK:-/tmp/nself-plugins-compile.lock}

WORK=$(mktemp -d "$TMPDIR/parity.XXXXXX")
WORK=$(cd "$WORK" && pwd)
SRV_PID=
cleanup() {
  [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null || true
  git -C "$ROOT" worktree remove --force "$WORK/plugins-base" >/dev/null 2>&1 || true
  if [ -n "${NSELF:-}" ] && [ -d "$NSELF/cli" ]; then git -C "$NSELF/cli" worktree remove --force "$WORK/cli" >/dev/null 2>&1 || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT

locked() { # locked <lockfile> cmd...: serialise heavy builds across agents when lockf exists
  local lock=$1; shift
  if command -v lockf >/dev/null 2>&1; then lockf -k "$lock" "$@"; else "$@"; fi
}
die() { echo "parity: $*" >&2; exit 2; }

# ---- builds -------------------------------------------------------------------------------------
build_head() { # build_head <src> <out>
  ( cd "$1" && CGO_ENABLED=0 locked "$PLUG_LOCK" go build -o "$2" ./cmd/ ) || die "cannot build nself-ci from $1"
}
mkdir -p "$WORK/head" "$WORK/base" "$WORK/core"
build_head "$HEAD_SRC" "$WORK/head/nself-ci"
if [ -n "${PARITY_BASE_BIN:-}" ]; then cp "$PARITY_BASE_BIN" "$WORK/base/nself-ci"
else
  git -C "$ROOT" worktree add --detach "$WORK/plugins-base" "$BASE_REF" >/dev/null 2>&1 || die "cannot check out $BASE_REF"
  build_head "$WORK/plugins-base/free/ci" "$WORK/base/nself-ci"
fi
if [ -n "${PARITY_NSELF_BIN:-}" ]; then cp "$PARITY_NSELF_BIN" "$WORK/core/nself"
else
  : "${NSELF:?NSELF must point at the checkout that holds cli/}"
  git -C "$NSELF/cli" worktree add --detach "$WORK/cli" "$CLI_REF" >/dev/null 2>&1 || die "cannot check out cli $CLI_REF"
  ( cd "$WORK/cli" && CGO_ENABLED=0 locked "$CLI_LOCK" go build -o "$WORK/core/nself" ./cmd/nself ) || die "cannot build core nself"
fi
CORE=$WORK/core/nself

# ---- fixtures -----------------------------------------------------------------------------------
mkfx() { # mkfx <dir> <module>
  mkdir -p "$1"; ( cd "$1"
    printf 'module example.com/%s\n\ngo 1.21\n' "$2" > go.mod
    printf 'package main\n\nfunc main() {}\n' > main.go
    printf 'package main\n\nimport "testing"\n\nfunc TestX(t *testing.T) {}\n' > main_test.go )
}
mkfx "$WORK/fx/clean" clean
mkfx "$WORK/fx/secret" secret
# the planted secret is assembled at run time so this source never carries a token-shaped string
printf 'package main\n\nvar tok = "%s%s"\n' "ghp_" "R8sT1uV2wX3yZ4aB5cD6eF7gH8iJ9kL0mN1o" > "$WORK/fx/secret/secret.go"
for r in clean secret; do ( cd "$WORK/fx/$r" && git init -q . && git add -A && git -c user.email=p@x -c user.name=p commit -qm init ); done
command -v gitleaks >/dev/null 2>&1 || die "gitleaks is required (the fixtures exercise the secret scan)"

# stub docker: records argv, answers inspect with "running" (a missing stub would hide the docker path)
mkdir -p "$WORK/stub"
cat > "$WORK/stub/docker" <<STUB
#!/bin/sh
echo "docker \$*" >> "$WORK/docker.log"
case " \${STUB_FAIL:-} " in *" \$4 "*) exit 1;; esac
[ "\$1" = inspect ] && echo running
exit 0
STUB
chmod +x "$WORK/stub/docker"
# healthy Forgejo: a static /-/health document
mkdir -p "$WORK/srv/-"; printf '{"healthy": true}\n' > "$WORK/srv/-/health"
PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')
( cd "$WORK/srv" && exec python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1 ) & SRV_PID=$!
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
  python3 -c "import urllib.request,sys; urllib.request.urlopen('http://127.0.0.1:$PORT/-/health', timeout=1)" 2>/dev/null && break; sleep 0.25
done

# ---- comparator ---------------------------------------------------------------------------------
norm() { # normalise volatile text; reads stdin
  sed -E \
    -e "s#$WORK#<W>#g" \
    -e 's/\([0-9][0-9a-zµ.]*\)/(T)/g' \
    -e 's/[0-9]{1,2}:[0-9]{2}(AM|PM)/TIME/g' \
    -e 's/in [0-9.]+(µs|ms|s)/in DUR/g' \
    -e "s#127\\.0\\.0\\.1:$PORT#127.0.0.1:PORT#g"
}
CASES=0; FAILS=0
# capture <name> <dir> <env-PATH-prefix> <cwd> cmd args...: stdout, stderr, rc into dir
capture() {
  local name=$1 dir=$2 pathpre=$3 cwd=$4; shift 4
  mkdir -p "$dir"; : > "$WORK/docker.log"
  local rc=0
  ( cd "$cwd" && PATH="$pathpre:$PATH" NO_COLOR=1 NSELF_CI_REPO= NSELF_CI_SHA= NSELF_CI_SKIP_STATUS= GITHUB_WEBHOOK_SECRET= "$@" >"$dir/$name.out" 2>"$dir/$name.err" ) || rc=$?
  echo "$rc" > "$dir/$name.rc"
  norm < "$dir/$name.out" > "$dir/$name.out.n"; norm < "$dir/$name.err" > "$dir/$name.err.n"
  norm < "$WORK/docker.log" > "$dir/$name.docker.n"
}
# compare <leg> <label> <expect-nonempty: 1 = stdout must be non-empty, 0 = stderr must be> <A-cmd-spec> -- <B-cmd-spec>: cmd spec = "PATHPREFIX|CWD|argv..."
compare() {
  local leg=$1 label=$2 nonempty=$3 aspec=$4 bspec=$5 a b
  local id; id=$(printf '%s' "$leg-$label" | tr -c 'A-Za-z0-9\n' '_')
  CASES=$((CASES + 1))
  run_spec "$aspec" "$WORK/o/$id/a"; run_spec "$bspec" "$WORK/o/$id/b"
  local bad=0
  for ext in out.n err.n rc docker.n; do
    if ! cmp -s "$WORK/o/$id/a/x.$ext" "$WORK/o/$id/b/x.$ext"; then
      bad=1; echo "DIFF $leg $label ($ext):"; diff -u "$WORK/o/$id/a/x.$ext" "$WORK/o/$id/b/x.$ext" | head -40 || true
    fi
  done
  if [ "$nonempty" = 1 ] && ! [ -s "$WORK/o/$id/a/x.out.n" ]; then bad=1; echo "EMPTY $leg $label: nothing was captured on stdout (vacuous)"; fi
  if [ "$nonempty" = 0 ] && ! [ -s "$WORK/o/$id/a/x.err.n" ]; then bad=1; echo "EMPTY $leg $label: nothing was captured on stderr (vacuous)"; fi
  if [ -n "${WANT_RC:-}" ] && [ "$(cat "$WORK/o/$id/a/x.rc")" != "$WANT_RC" ]; then bad=1; echo "RC $leg $label: exit code $(cat "$WORK/o/$id/a/x.rc"), expected $WANT_RC (the case did not exercise what it claims)"; fi
  if [ $bad -eq 0 ]; then echo "ok $leg $label"; else FAILS=$((FAILS + 1)); fi
}
run_spec() { # run_spec "PATHPREFIX|CWD|exe|args..." <outdir>   (args split on the unit separator 0x1f)
  local spec=$1 dir=$2 pre cwd rest
  pre=${spec%%|*}; rest=${spec#*|}; cwd=${rest%%|*}; rest=${rest#*|}
  local IFS=$'\x1f'; set -- $rest; unset IFS
  capture x "$dir" "$pre" "$cwd" "$@"
}
US=$'\x1f'
spec() { # spec <pathprefix> <cwd> <exe> args...
  local pre=$1 cwd=$2; shift 2; local s="$pre|$cwd|$1"; shift
  local a; for a in "$@"; do s="$s$US$a"; done; printf '%s' "$s"
}

HEADB=$WORK/head; BASEB=$WORK/base; STUB=$WORK/stub
FXC=$WORK/fx/clean; FXS=$WORK/fx/secret

if [ $SELF_TEST -eq 1 ]; then
  # same command both sides must be equal; a planted difference must be reported
  compare self "identical" 1 "$(spec "$BASEB" "$WORK" "$CORE" ci --help)" "$(spec "$BASEB" "$WORK" "$CORE" ci --help)"
  [ $FAILS -eq 0 ] || { echo "parity self-test: FAIL (identical sides reported a difference)"; exit 1; }
  FAILS=0
  out=$(compare self "planted" 1 "$(spec "$BASEB" "$WORK" "$CORE" ci --help)" "$(spec "$BASEB" "$WORK" "$CORE" ci build --help)" 2>&1 || true)
  printf '%s\n' "$out" | grep -q '^DIFF self planted' || { echo "parity self-test: FAIL (planted difference not reported)"; exit 1; }
  echo "parity self-test: PASS (identical sides equal, planted difference reported)"; exit 0
fi

# ---- leg 1: core nself ci vs head nself-ci with core argv ------------------------------------------
c1() { # c1 <label> <nonempty> <core ci args...> -- <head args...> : split on literal --
  local label=$1 ne=$2; shift 2; local ca=() ha=() seen=0 a
  for a in "$@"; do if [ "$a" = "--" ] && [ $seen -eq 0 ]; then seen=1; else if [ $seen -eq 0 ]; then ca+=("$a"); else ha+=("$a"); fi; fi; done
  compare leg1 "$label" "$ne" "$(spec "$BASEB" "$WORK" "$CORE" ci "${ca[@]}")" "$(spec "$HEADB" "$WORK" "$HEADB/nself-ci" "${ha[@]}")"
}
c1 "ci --help" 1 --help -- --help
c1 "ci build --help" 1 build --help -- build --help
c1 "ci forgejo --help" 1 forgejo --help -- forgejo --help
c1 "ci serve --help" 1 serve --help -- serve --help
for fx in clean secret; do
  FX=$WORK/fx/$fx; WANT_RC=0; [ "$fx" = secret ] && WANT_RC=1
  c1 "gate $fx --no-status --check" 1 --no-status --check "$FX" -- --no-status --check "$FX"
  c1 "gate $fx --check" 1 --check "$FX" -- --check "$FX"
  WANT_RC=0 c1 "gate $fx --no-gitleaks --check" 1 --no-gitleaks --check "$FX" -- --no-gitleaks --check "$FX"
  c1 "gate $fx --verbose --check" 1 --verbose --check "$FX" -- --verbose --check "$FX"
  c1 "gate $fx root-first" 1 "$FX" --check -- "$FX" --check
done
WANT_RC=0
# gate from the fixture directory with no repo-root argument
compare leg1 "gate clean cwd" 1 "$(spec "$BASEB" "$FXC" "$CORE" ci --check)" "$(spec "$HEADB" "$FXC" "$HEADB/nself-ci" --check)"
WANT_RC=0
# forgejo: recording stub docker, unreachable server and a healthy server
compare leg1 "forgejo unreachable stub" 1 "$(spec "$STUB" "$WORK" "$CORE" ci forgejo --url http://127.0.0.1:1 --runner rn)" "$(spec "$STUB" "$WORK" "$HEADB/nself-ci" forgejo --url http://127.0.0.1:1 --runner rn)"
compare leg1 "forgejo healthy stub" 1 "$(spec "$STUB" "$WORK" "$CORE" ci forgejo --url "http://127.0.0.1:$PORT")" "$(spec "$STUB" "$WORK" "$HEADB/nself-ci" forgejo --url "http://127.0.0.1:$PORT")"
compare leg1 "forgejo healthy named runner" 1 "$(spec "$STUB" "$WORK" "$CORE" ci forgejo --url "http://127.0.0.1:$PORT/" --runner my_runner)" "$(spec "$STUB" "$WORK" "$HEADB/nself-ci" forgejo --url "http://127.0.0.1:$PORT/" --runner my_runner)"
# the first runner-name candidate is missing, then both are: the probe order must match core
export STUB_FAIL=nself_forgejo_runner
compare leg1 "forgejo second candidate" 1 "$(spec "$STUB" "$WORK" "$CORE" ci forgejo --url http://127.0.0.1:1)" "$(spec "$STUB" "$WORK" "$HEADB/nself-ci" forgejo --url http://127.0.0.1:1)"
export STUB_FAIL="nself_forgejo_runner app_forgejo_runner"
compare leg1 "forgejo no candidate" 1 "$(spec "$STUB" "$WORK" "$CORE" ci forgejo --url http://127.0.0.1:1)" "$(spec "$STUB" "$WORK" "$HEADB/nself-ci" forgejo --url http://127.0.0.1:1)"
unset STUB_FAIL
# serve refuses to start without a secret (fail-closed default)
WANT_RC=1
compare leg1 "serve refuses without secret" 0 "$(spec "$BASEB" "$WORK" "$CORE" ci serve --addr 127.0.0.1:0 --workdir "$WORK/wd")" "$(spec "$HEADB" "$WORK" "$HEADB/nself-ci" serve --addr 127.0.0.1:0 --workdir "$WORK/wd")"

# ---- leg 2: deployed 1.4.x: base cli, plugins base binary vs head binary ---------------------------
WANT_RC=
c2() { # c2 <label> <cwd> <core ci args...>
  local label=$1 cwd=$2; shift 2
  compare leg2 "$label" "${C2NE:-1}" "$(spec "$BASEB" "$cwd" "$CORE" ci "$@")" "$(spec "$HEADB" "$cwd" "$CORE" ci "$@")"
}
for fx in clean secret; do
  FX=$WORK/fx/$fx; WANT_RC=0; [ "$fx" = secret ] && WANT_RC=1
  c2 "$fx --check" "$WORK" --check "$FX"
  c2 "$fx --no-status" "$WORK" --no-status "$FX"
  WANT_RC=0 c2 "$fx --no-gitleaks --check" "$WORK" --no-gitleaks --check "$FX"
  c2 "$fx -v --check" "$WORK" -v --check "$FX"
  c2 "$fx --filesystem --check" "$WORK" --filesystem --check "$FX"
  c2 "$fx --sha --no-status" "$WORK" --sha abc1234 --no-status "$FX"
  c2 "$fx --owner --repo --no-status" "$WORK" --owner nself-org --repo cli --no-status "$FX"
  WANT_RC=0 c2 "$fx all flags" "$WORK" --check --no-status --no-gitleaks --filesystem -v --sha abc1234 --owner o --repo r "$FX"
  c2 "$fx cwd no arg" "$FX" --check
  WANT_RC=1 c2 "$fx build" "$WORK" build --artifact android "$FX"
  WANT_RC=1 C2NE=0 c2 "$fx build --upload no tag" "$WORK" build --upload "$FX"
  WANT_RC=0 compare leg2 "$fx direct run" 1 "$(spec "$BASEB" "$WORK" "$BASEB/nself-ci" run "$FX")" "$(spec "$HEADB" "$WORK" "$HEADB/nself-ci" run "$FX")"
done
WANT_RC=0 compare leg2 "run --help" 0 "$(spec "$BASEB" "$WORK" "$BASEB/nself-ci" run --help)" "$(spec "$HEADB" "$WORK" "$HEADB/nself-ci" run --help)"
WANT_RC=0 compare leg2 "direct legacy argv" 1 "$(spec "$BASEB" "$WORK" "$BASEB/nself-ci" --no-status --no-gitleaks -v "$FXC")" "$(spec "$HEADB" "$WORK" "$HEADB/nself-ci" --no-status --no-gitleaks -v "$FXC")"

[ "$CASES" -gt 0 ] || { echo "parity: FAIL (no case ran)"; exit 1; }
if [ $FAILS -eq 0 ]; then echo "parity: PASS ($CASES cases)"; exit 0; fi
echo "parity: FAIL ($FAILS of $CASES cases differ)"; exit 1
