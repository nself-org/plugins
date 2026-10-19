#!/usr/bin/env bash
# Shared by the acceptance scenario drills (P7-CI-47); sourced, never run.
#
# Purpose: get the three binaries a scenario needs without the installed cli or the served plugin: the ci plugin built
#   from THIS checkout (so the hosted run tests the code under review), a gitleaks on PATH, and for scenario 1 an
#   `nself` built from nself-org/cli origin/main (readiness R7; the served-bytes install leg is P7-CI-99's).
# Inputs:  EV_ROOT (the repository under test), a scratch directory, optional NSELF_DRILL_CI_BIN, NSELF_DRILL_CLI_BIN,
#          NSELF_DRILL_CLI_SRC (a cli checkout to build instead of cloning). A drill job sees none of these (the exec
#          allowlist drops NSELF_*); they are for running a `run` script by hand.
# Outputs: tools_build_plugin, tools_gitleaks_dir and tools_cli print a path on stdout; failures exit the drill with 2.
# Constraints: pinned download with sha256 verification only; plugin built CGO_ENABLED=0 with the vendored modules.

# Pins: keep equal to the gitleaks row of free/ci/toolchain/versions.yaml (checked by tools_pin_drift). The darwin
# digests come from the release's own gitleaks_8.30.1_checksums.txt; the toolchain pins list linux only.
GITLEAKS_VERSION=8.30.1
gitleaks_sha() {
	case "$1" in
	linux_x64) echo 551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb ;;
	linux_arm64) echo e4a487ee7ccd7d3a7f7ec08657610aa3606637dab924210b3aee62570fb4b080 ;;
	darwin_arm64) echo b40ab0ae55c505963e365f271a8d3846efbc170aa17f2607f13df610a9aeb6a5 ;;
	darwin_x64) echo dfe101a4db2255fc85120ac7f3d25e4342c3c20cf749f2c20a18081af1952709 ;;
	esac
}

tools_die() { echo "$EV_JOB: $*" >&2; exit 2; }

# tools_pin_drift: the pin above must equal the toolchain row; prints a note when it does not.
tools_pin_drift() {
	local row
	row=$(awk '/^  - name: gitleaks$/ {f=1} f && /^    version:/ {print $2; exit}' "$EV_ROOT/free/ci/toolchain/versions.yaml" 2>/dev/null || true)
	[ -z "$row" ] || [ "$row" = "$GITLEAKS_VERSION" ] || echo "gitleaks pin $GITLEAKS_VERSION differs from free/ci/toolchain/versions.yaml ($row)"
}

# tools_build_plugin DEST: build free/ci from this checkout (or take NSELF_DRILL_CI_BIN, or the binary the workflow built
# into .drill-bin/nself-ci from the same checkout, which saves a cold compile inside the job's time limit).
tools_build_plugin() {
	if [ -n "${NSELF_DRILL_CI_BIN:-}" ]; then cp "$NSELF_DRILL_CI_BIN" "$1"
	elif [ -n "${CI:-}" ] && [ -x "$EV_ROOT/.drill-bin/nself-ci" ]; then cp "$EV_ROOT/.drill-bin/nself-ci" "$1"; else
		command -v go >/dev/null || tools_die "go is required to build the ci plugin"
		(cd "$EV_ROOT/free/ci" && CGO_ENABLED=0 go build -mod=vendor -o "$1" ./cmd) >&2 || tools_die "building free/ci failed"
	fi
	chmod +x "$1"
}

# tools_gitleaks_dir WORK: a directory holding gitleaks (the one on PATH, or the pinned download).
tools_gitleaks_dir() {
	local bin os arch key tmp
	if bin=$(command -v gitleaks 2>/dev/null); then dirname "$bin"; return; fi
	os=$(uname -s | tr '[:upper:]' '[:lower:]')
	case "$(uname -m)" in x86_64 | amd64) arch=x64 ;; arm64 | aarch64) arch=arm64 ;; *) tools_die "no gitleaks pin for $(uname -m)" ;; esac
	key=${os}_${arch}
	[ -n "$(gitleaks_sha "$key")" ] || tools_die "no gitleaks pin for $key"
	tmp=$1/gitleaks-dl
	mkdir -p "$tmp/bin"
	curl --fail --silent --show-error --location --proto '=https' --retry 3 --max-time 120 \
		--output "$tmp/gl.tgz" "https://github.com/gitleaks/gitleaks/releases/download/v$GITLEAKS_VERSION/gitleaks_${GITLEAKS_VERSION}_$key.tar.gz" ||
		tools_die "cannot download gitleaks"
	[ "$(ev_sha "$tmp/gl.tgz" | cut -d' ' -f1)" = "$(gitleaks_sha "$key")" ] || tools_die "gitleaks download does not match its pinned sha256"
	tar -xzf "$tmp/gl.tgz" -C "$tmp/bin" gitleaks || tools_die "cannot unpack gitleaks"
	echo "$tmp/bin"
}

# tools_cli WORK: an `nself` built from nself-org/cli origin/main (or NSELF_DRILL_CLI_BIN / a checkout in NSELF_DRILL_CLI_SRC,
# or a checkout the workflow left at .drill-bin/nself).
tools_cli() {
	local src out=$1/nself-cli
	if [ -n "${NSELF_DRILL_CLI_BIN:-}" ]; then echo "$NSELF_DRILL_CLI_BIN"; return; fi
	if [ -n "${CI:-}" ] && [ -x "$EV_ROOT/.drill-bin/nself" ]; then echo "$EV_ROOT/.drill-bin/nself"; return; fi
	command -v go >/dev/null || tools_die "go is required to build the cli"
	src=${NSELF_DRILL_CLI_SRC:-}
	if [ -z "$src" ]; then
		src=$1/cli-src
		git clone --quiet --depth 1 --branch main https://github.com/nself-org/cli.git "$src" >&2 || tools_die "cannot clone nself-org/cli"
	fi
	(cd "$src" && go build -o "$out" ./cmd/nself) >&2 || tools_die "building the cli failed"
	echo "$out"
}
