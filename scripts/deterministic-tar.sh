#!/usr/bin/env bash
# deterministic-tar.sh
# The ONE tarball recipe for this repo. Both scripts/build-tarballs.sh and
# .github/workflows/release-tarballs.yml call this instead of invoking `tar`
# directly, so a source tarball or a per-platform binary tarball built here,
# built on a laptop, or built by CI always produces byte-identical output for
# byte-identical input. That is a hard requirement: registry.json stores a
# single sha256 per plugin and `nself plugin install` refuses the install on
# any mismatch (internal/plugin/security.go verifyChecksum) — so if two build
# sites can disagree on the bytes, the checksum is worthless.
#
# What made plain `tar -czf` non-reproducible, both fixed here:
#   1. mtime — tar preserves each member's on-disk mtime, and gzip stamps the
#      wall-clock build time into its header. Two builds of the identical
#      source tree, seconds apart, produced different bytes.
#   2. member order — scripts/build-tarballs.sh (bash glob "*/ ") and the old
#      inline workflow step (`find | sort`) could walk directories in
#      different orders, changing tar's member sequence and therefore the
#      output bytes even when every member's content was identical.
#
# Usage:
#   deterministic-tar.sh <output.tar.gz> <tar-args...>
#
# <tar-args...> are passed through to `tar -cf -` after the fixed
# determinism flags, so both plain forms work:
#   deterministic-tar.sh dist/foo-1.0.1.tar.gz free/foo
#   deterministic-tar.sh dist/foo-1.0.1-darwin-arm64.tar.gz -C "$stage" foo
#
# Requires GNU tar. macOS ships bsdtar as `/usr/bin/tar`, which does not
# support --sort/--owner/--group/--numeric-owner/--pax-option the same way
# (some are silently ignored, some error) — there is no equivalent bsdtar
# invocation that reliably reproduces GNU tar's byte layout, and CI runs GNU
# tar (ubuntu-latest), so a "works on my Mac" bsdtar path would silently
# diverge from what CI actually publishes. Policy: require GNU tar
# everywhere. On macOS: `brew install gnu-tar` (provides the `gtar` binary)
# and this script picks it up automatically. On Linux, `tar` already is GNU
# tar.

set -euo pipefail

if [ "$#" -lt 2 ]; then
  printf "usage: %s <output.tar.gz> <tar-args...>\n" "$0" >&2
  exit 1
fi

OUT="$1"
shift

# Locate a GNU tar binary: prefer `tar` if it identifies as GNU tar (true on
# every Linux CI runner), else fall back to `gtar` (Homebrew's GNU tar on
# macOS). Fail loudly and tell the operator exactly how to fix it rather than
# silently building non-reproducible archives with bsdtar.
TAR_BIN=""
if tar --version 2>/dev/null | grep -q "GNU tar"; then
  TAR_BIN="tar"
elif command -v gtar >/dev/null 2>&1 && gtar --version 2>/dev/null | grep -q "GNU tar"; then
  TAR_BIN="gtar"
else
  printf "[deterministic-tar] ERROR: GNU tar is required to build reproducible tarballs.\n" >&2
  printf "[deterministic-tar]   macOS: brew install gnu-tar   (installs GNU tar as 'gtar')\n" >&2
  printf "[deterministic-tar]   Linux: GNU tar ships as 'tar' by default — check your PATH.\n" >&2
  printf "[deterministic-tar] Refusing to fall back to bsdtar: it cannot reproduce GNU tar's\n" >&2
  printf "[deterministic-tar] byte layout, and CI (ubuntu-latest) always uses GNU tar — a\n" >&2
  printf "[deterministic-tar] bsdtar-built tarball would not match the checksum CI records.\n" >&2
  exit 1
fi

mkdir -p "$(dirname "$OUT")"

# Fixed epoch chosen to match the same normalization already shipped for paid
# plugins (nself-org/web@51518c38, ping_api's streamScopedPluginTarball):
# every entry's mtime pinned to the Unix epoch. --owner/--group/--numeric-owner
# zero out uid/gid so the building machine's user account never leaks into
# the archive or its bytes. --pax-option strips the PAX extended-header
# atime/ctime fields GNU tar otherwise emits for some filesystems, which are
# themselves wall-clock noise. --sort=name makes the member order a pure
# function of the file tree, independent of the underlying filesystem's
# directory-entry order or which shell mechanism (glob vs find) enumerated it.
"$TAR_BIN" --sort=name \
  --mtime='UTC 1970-01-01' \
  --owner=0 --group=0 --numeric-owner \
  --pax-option=exthdr.name=%d/PaxHeaders/%f,delete=atime,delete=ctime \
  -cf - "$@" | gzip -n -9 > "$OUT"
