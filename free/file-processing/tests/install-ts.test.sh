#!/bin/bash
# D-0623: install.sh must fail when the TypeScript install or build fails, and
# must use pnpm (npm cannot install the link: dependency on shared/).
# Runs install_typescript_dependencies against a stub pnpm; no network.
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STUBS="$(mktemp -d "${TMPDIR:-/tmp}/fp-install-test.XXXXXX")"
trap 'rm -rf "$STUBS"' EXIT
FAILED=0

# shellcheck source=../install.sh
source "$HERE/../install.sh"
set +e # install.sh sets -e; the cases below expect non-zero returns

# stub <fail-on>: a pnpm that logs its arguments and fails on "<fail-on>" ("" = never).
stub() {
    printf '#!/bin/bash\necho "$PWD :: $*" >> "%s/calls"\n[ "$1 $2" = "%s" ] && exit 1\nexit 0\n' "$STUBS" "$1" > "$STUBS/pnpm"
    chmod +x "$STUBS/pnpm"
    : > "$STUBS/calls"
}

check() { # check <name> <want-rc> <got-rc>
    if [ "$2" != "$3" ]; then echo "FAIL $1: rc $3, want $2"; FAILED=1; else echo "ok   $1"; fi
}

run() { (PATH="$STUBS:/usr/bin:/bin"; install_typescript_dependencies) >/dev/null 2>&1; }

stub ""; run; check "all steps pass" 0 $?
grep -q ":: run build" "$STUBS/calls" || { echo "FAIL build was not run"; FAILED=1; }

stub "install --frozen-lockfile"; run; check "install failure fails the installer" 1 $?
stub "run build"; run; check "build failure fails the installer" 1 $?

rm -f "$STUBS/pnpm"; run; check "missing pnpm fails the installer" 1 $?

exit "$FAILED"
