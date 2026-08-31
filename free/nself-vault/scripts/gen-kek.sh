#!/usr/bin/env bash
# gen-kek.sh — generate a 32-byte KEK (Key Encryption Key) for nself-vault.
#
# Output: 32 random bytes encoded as 64-character lowercase hex on stdout.
# Idempotent only when --out=<path> is given AND the target file already exists.
#
# Usage:
#   ./gen-kek.sh                # print KEK hex to stdout
#   ./gen-kek.sh --out=v2.key   # write to file (skip if exists)
#   ./gen-kek.sh --version=2    # print as KEK env var assignment (VAULT_KEK_V2=...)
#
# Source preference:
#   1. /dev/urandom (POSIX, never blocks)
#   2. openssl rand (fallback when /dev/urandom unavailable)
#
# WARNING: KEK material is highly sensitive. Never log, never commit, never
# transmit over insecure channels. Treat output like a root password.

set -euo pipefail

OUT_FILE=""
VERSION=""

for arg in "$@"; do
    case "$arg" in
        --out=*)
            OUT_FILE="${arg#--out=}"
            ;;
        --version=*)
            VERSION="${arg#--version=}"
            ;;
        --help|-h)
            sed -n '2,18p' "$0"
            exit 0
            ;;
        *)
            echo "gen-kek.sh: unknown argument: $arg" >&2
            exit 2
            ;;
    esac
done

# Idempotency: if --out is given and file exists, exit success without overwriting.
if [ -n "$OUT_FILE" ] && [ -e "$OUT_FILE" ]; then
    echo "gen-kek.sh: $OUT_FILE already exists — skipping (idempotent)" >&2
    exit 0
fi

# Generate 32 random bytes, hex-encode them.
generate_kek_hex() {
    if [ -r /dev/urandom ]; then
        # head -c reads exactly 32 bytes from /dev/urandom; xxd -p emits hex on one line.
        head -c 32 /dev/urandom | xxd -p -c 32
        return 0
    fi
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex 32
        return 0
    fi
    echo "gen-kek.sh: no entropy source available (need /dev/urandom or openssl)" >&2
    return 1
}

KEK_HEX="$(generate_kek_hex)"

# Sanity check: must be exactly 64 hex characters.
if [ "${#KEK_HEX}" -ne 64 ]; then
    echo "gen-kek.sh: generated KEK has wrong length (${#KEK_HEX}, want 64)" >&2
    exit 1
fi
if ! printf '%s' "$KEK_HEX" | grep -Eq '^[0-9a-f]{64}$'; then
    echo "gen-kek.sh: generated KEK is not valid lowercase hex" >&2
    exit 1
fi

# Format output.
if [ -n "$VERSION" ]; then
    LINE="VAULT_KEK_V${VERSION}=${KEK_HEX}"
else
    LINE="$KEK_HEX"
fi

if [ -n "$OUT_FILE" ]; then
    # Write with restrictive permissions (owner read/write only).
    OLD_UMASK="$(umask)"
    umask 077
    printf '%s\n' "$LINE" > "$OUT_FILE"
    umask "$OLD_UMASK"
    echo "gen-kek.sh: wrote $OUT_FILE (mode 0600)" >&2
else
    printf '%s\n' "$LINE"
fi
