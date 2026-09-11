#!/usr/bin/env bash
# build-tarballs.sh
# Build tarballs for all free plugins without uploading. Outputs to dist/.
# Run build-and-upload-tarballs.sh to also upload.
#
# Usage: ./scripts/build-tarballs.sh [VERSION]
#   VERSION defaults to the registry.json top-level version field (e.g. 1.0.0)
#
# Every plugin gets its source tarball. A plugin that also declares a
# binaryName (it provides an `nself <cmd>`) additionally gets five
# per-platform tarballs, built with the SAME Go cross-compile + tar recipe
# .github/workflows/release-tarballs.yml uses to build the real release
# assets — see build_platform_tarballs() below. Set
# NSELF_SKIP_PLATFORM_TARBALLS=1 to build source tarballs only (faster local
# iteration when you don't need the platform checksums); CI's
# tarball-checksum-gate.yml never sets it, since a platform tarball's
# checksum is exactly what it exists to verify.
#
# This file is also sourceable: tarball-checksum-gate.yml sources it and
# calls build_platform_tarballs() directly per plugin, so the Go-build
# orchestration (which command maps to which cmd/ subdirectory, which
# platforms exist) is defined in exactly one place, not copy-pasted into the
# workflow YAML a second time.
#
# Requirements: jq, sha256sum (or shasum on macOS), scripts/deterministic-tar.sh
#   (GNU tar). Platform tarballs additionally require a Go toolchain matching
#   go.mod's directive for each plugin (CGO_ENABLED=0 cross-compile).
# Output: dist/<name>-<version>.tar.gz(.sha256)
#         dist/<name>-<version>-<platform>.tar.gz(.sha256)  [binaryName plugins]

# NOTE: no `set -euo pipefail` at sourcing time — see the execution guard at
# the bottom of this file. Functions below assume it (via `main`'s own
# `set -euo pipefail`), consistent with how this script always ran standalone.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DETERMINISTIC_TAR="${SCRIPT_DIR}/deterministic-tar.sh"

# The five platform strings nself plugin install requests, exactly as
# internal/plugin/arch.go's PlatformArch() returns them. binaryPluginDownloadURL()
# builds the asset filename from the same strings — a mismatch here is an
# install failure there.
PLATFORMS="darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64"

log()  { printf "[build-tarballs] %s\n" "$*"; }
err()  { printf "[build-tarballs] ERROR: %s\n" "$*" >&2; }
warn() { printf "[build-tarballs] WARN: %s\n" "$*" >&2; }

# sha256 helper — macOS uses shasum -a 256, Linux uses sha256sum
sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

# build_platform_tarballs builds the five per-platform tarballs for one
# plugin, when it declares a binaryName, into dist_dir. No-op (returns 0) for
# a plugin with no binaryName — its package is source and works everywhere.
#
# Ported from .github/workflows/release-tarballs.yml's build_plugin() step,
# which is the proven, already-running-in-CI version of this logic; kept
# byte-for-byte equivalent in command/package resolution so a local build and
# a CI build can never disagree about which commands get compiled or from
# which cmd/ subdirectory.
#
# Args: plugin_dir plugin_name version dist_dir
build_platform_tarballs() {
  local plugin_dir="$1" plugin_name="$2" version="$3" dist_dir="$4"

  local bin_name
  bin_name="$(jq -r '.binaryName // .implementation.binaryName // ""' "${plugin_dir}/plugin.json")"
  if [ -z "$bin_name" ]; then
    return 0
  fi

  # A plugin may provide more than one command; cliCommands lists them and
  # the CLI publishes one binary per entry, so every one has to be built or
  # the command it names is dead on arrival.
  local commands
  commands="$(jq -r 'if (.cliCommands // []) | length > 0 then (.cliCommands[].name) else "" end' "${plugin_dir}/plugin.json")"
  if [ -z "$commands" ]; then
    commands="${bin_name#nself-}"
  fi

  local dist_dir_abs
  dist_dir_abs="$(cd "$dist_dir" && pwd)"

  local platform
  for platform in $PLATFORMS; do
    local goos="${platform%%-*}"
    local goarch="${platform##*-}"
    local stage="${dist_dir}/stage-${plugin_name}-${platform}"
    local exe=""
    [ "$goos" = "windows" ] && exe=".exe"

    rm -rf "$stage"
    mkdir -p "${stage}/${plugin_name}"
    cp -R "${plugin_dir}/." "${stage}/${plugin_name}/"

    local failed=0
    local command
    for command in $commands; do
      # A plugin providing several commands keeps each under cmd/<command>/;
      # one providing a single command may put it at cmd/ directly.
      local pkg="./cmd/${command}/"
      [ -d "${plugin_dir}/cmd/${command}" ] || pkg="./cmd/"

      if ! (cd "${plugin_dir}" && CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
            go build -trimpath -ldflags="-s -w" \
            -o "${dist_dir_abs}/$(basename "$stage")/${plugin_name}/nself-${command}${exe}" "$pkg"); then
        err "$plugin_name failed to build nself-${command} for $platform"
        failed=1
        break
      fi
    done

    if [ "$failed" -ne 0 ]; then
      rm -rf "$stage"
      return 1
    fi

    local ptar="${dist_dir}/${plugin_name}-${version}-${platform}.tar.gz"
    # Same shared recipe as the source tarball — the per-platform binary
    # archives had the identical mtime/ordering non-determinism (compiled
    # binary mtimes, per-run staging-dir enumeration order) that the source
    # tarball did before deterministic-tar.sh.
    "$DETERMINISTIC_TAR" "$ptar" -C "$stage" "${plugin_name}"
    local psha
    psha="$(sha256_file "$ptar")"
    printf "%s  %s\n" "$psha" "$(basename "$ptar")" > "${ptar}.sha256"
    rm -rf "$stage"
    log "  ${platform} sha256: ${psha}"
  done
}

# main runs the full build: every plugin's source tarball, plus (unless
# NSELF_SKIP_PLATFORM_TARBALLS=1) every binaryName plugin's five platform
# tarballs. Only invoked when this file is executed directly — see the guard
# below — so tarball-checksum-gate.yml can source it for build_platform_tarballs
# alone without also running this whole pass.
main() {
  set -euo pipefail

  local plugins_dir="${PLUGINS_DIR:-free}"
  local dist_dir="${DIST_DIR:-dist}"
  local errors=0

  local version
  if [ -n "${1:-}" ]; then
    version="$1"
  else
    if ! command -v jq >/dev/null 2>&1; then
      err "jq not found; pass VERSION as first argument"
      exit 1
    fi
    version="$(jq -r '.version' registry.json 2>/dev/null || printf '')"
    if [ -z "$version" ]; then
      err "Cannot read version from registry.json; pass VERSION as first argument"
      exit 1
    fi
  fi

  mkdir -p "$dist_dir"

  local built=0
  # Walk free/ plugin directories in a fixed, filesystem-order-independent
  # sequence (matches the traversal .github/workflows/release-tarballs.yml
  # uses) — member order inside each individual tarball is already pinned by
  # deterministic-tar.sh's --sort=name, this just makes the build log/exit
  # order predictable too.
  while IFS= read -r plugin_dir; do
    local plugin_name tarball_name checksum_name tarball_path checksum_path
    plugin_name="$(basename "$plugin_dir")"
    tarball_name="${plugin_name}-${version}.tar.gz"
    checksum_name="${tarball_name}.sha256"
    tarball_path="${dist_dir}/${tarball_name}"
    checksum_path="${dist_dir}/${checksum_name}"

    # Verify plugin.json exists (skip non-plugin dirs)
    if [ ! -f "${plugin_dir}/plugin.json" ]; then
      warn "Skipping $plugin_name (no plugin.json)"
      continue
    fi

    log "Building ${tarball_name} ..."
    # No trailing slash on plugin_dir: deterministic-tar.sh's --sort=name
    # already makes member order stable regardless, but a consistent
    # no-trailing-slash argument keeps this script and the workflow's
    # `find -type d` output (which never has one) textually identical inputs.
    if ! "$DETERMINISTIC_TAR" "$tarball_path" "${plugin_dir%/}"; then
      err "Failed to build tarball for $plugin_name"
      errors=$((errors + 1))
      continue
    fi

    local sha256
    sha256="$(sha256_file "$tarball_path")"
    printf "%s  %s\n" "$sha256" "$tarball_name" > "$checksum_path"
    log "  sha256: ${sha256}"
    built=$((built + 1))

    if [ "${NSELF_SKIP_PLATFORM_TARBALLS:-}" != "1" ]; then
      if ! build_platform_tarballs "$plugin_dir" "$plugin_name" "$version" "$dist_dir"; then
        err "Failed to build one or more platform tarballs for $plugin_name"
        errors=$((errors + 1))
      fi
    fi
  done < <(find "$plugins_dir" -maxdepth 1 -mindepth 1 -type d | sort)

  log "Built ${built} plugin(s) in ${dist_dir}/"

  if [ "$errors" -gt 0 ]; then
    log "Completed with $errors error(s)."
    exit 1
  fi

  log "Done."
}

# Only run main when executed directly (./build-tarballs.sh or bash
# build-tarballs.sh), not when sourced — tarball-checksum-gate.yml sources
# this file to reuse build_platform_tarballs()/sha256_file() without
# triggering a full build of every plugin.
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
  main "$@"
fi
