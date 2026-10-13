#!/usr/bin/env bash
# sign-key-formats-test.sh: scripts/release/sign.sh must accept NSELF_RELEASE_SIGNING_KEY_FREE as a raw PKCS8 PEM, as a PEM whose
# newlines were collapsed to spaces, and as the base64 of the PEM (the vault form, P7-PROD-59), and must still refuse garbage and a
# key id that does not match the key. A throwaway key is generated here; no real key is read, nothing is uploaded, no key text is printed.
# Run from any directory: bash scripts/release/sign-key-formats-test.sh   (set OPENSSL to an OpenSSL 3 binary if needed)
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
O="${OPENSSL:-openssl}"
"$O" version 2>/dev/null | grep -q '^OpenSSL ' || { echo "FAIL: $O is not OpenSSL (set OPENSSL)"; exit 1; }
T="$(mktemp -d "${TMPDIR:-/tmp}/signfmt.XXXXXX")"; trap 'rm -rf "$T"' EXIT
"$O" genpkey -algorithm ed25519 -out "$T/k.pem" 2>/dev/null
ID="plugins-$("$O" pkey -in "$T/k.pem" -pubout -outform DER | tail -c 32 | { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-16)"
printf 'hello\n' > "$T/x.txt"; tar -cf "$T/x.tar" -C "$T" x.txt
PEM="$(cat "$T/k.pem")"
fails=0
run() { # run <label> <key text> <key id> -> prints the signature JSON, or FAILED
  NSELF_RELEASE_SIGNING_KEY_FREE="$2" NSELF_RELEASE_SIGNING_KEY_FREE_ID="$3" OPENSSL="$O" bash "$HERE/sign.sh" demo 1.0.0 "$T/x.tar" 2>/dev/null || echo FAILED
}
ref="$(run raw "$PEM" "$ID")"
case "$ref" in '{"key_id":"'"$ID"'"'*) ;; *) echo "FAIL raw PEM: $ref"; exit 1 ;; esac
check_same() { local got; got="$(run "$1" "$2" "$ID")"; if [ "$got" = "$ref" ]; then echo "ok   $1"; else echo "FAIL $1 (signature differs or signing failed)"; fails=$((fails+1)); fi; }
check_same "collapsed newlines (spaces)" "$(printf '%s' "$PEM" | tr '\n' ' ')"
check_same "base64 of the PEM" "$(printf '%s\n' "$PEM" | "$O" base64 -A)"
check_same "base64 of the PEM, wrapped" "$(printf '%s\n' "$PEM" | "$O" base64)"
check_fail() { local got; got="$(run "$1" "$2" "$3")"; if [ "$got" = FAILED ]; then echo "ok   refuses $1"; else echo "FAIL $1 was accepted"; fails=$((fails+1)); fi; }
check_fail "garbage" "not a key at all" "$ID"
check_fail "wrong key id" "$PEM" "plugins-0000000000000000"
check_fail "base64 of garbage" "$(printf 'nothing' | "$O" base64 -A)" "$ID"
[ "$fails" = 0 ] && echo "PASS sign-key-formats" || { echo "FAILED: $fails case(s)"; exit 1; }
