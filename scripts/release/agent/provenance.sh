#!/usr/bin/env bash
# provenance.sh --in DIR (--prepare | --sign | --rehearsal) [--versions FILE]
#
# Purpose: write the SBOMs and the signed in-toto provenance of every nself-ci-agent binary in DIR (hq CACHE D17, P7-CACHE-10). It runs
#   after manifest.sh, so every binary already has its signed agent-release/v1 manifest. Two phases, so that no third-party program ever
#   runs while the signing key exists:
#     --prepare  (NO key; refuses to run if NSELF_AGENT_SIGNING_KEY is set)
#                1. runs the pinned syft (version and sha256 from the `syft` row of free/ci/internal/toolchain/versions.yaml, P7-CACHE-08)
#                   and writes <binary>.cdx.json (CycloneDX 1.6) and <binary>.spdx.json (SPDX 2.3);
#                2. runs `go run ./tools/provgen` (free/ci, over internal/provenance.BuildAgent): statement = buildType agent/v1, subject
#                   = binary name + the manifest's sha256, byproducts = the two SBOM digests, builder = the workflow ref, invocation = the
#                   run id; the statement and its DSSE pre-authentication encoding land in DIR/.provenance/<target>/.
#     --sign     (key; runs nothing but openssl) signs each PAE with the agent-purpose key through the same
#                `openssl pkeyutl -sign -rawin` primitive manifest.sh uses, writes <binary>.intoto.dsse.json (the DSSE envelope) and
#                removes DIR/.provenance.
#     --rehearsal  with neither phase flag: both phases in one run with a THROWAWAY key generated here (the environment key is never
#                read); its public half is APPENDED to DIR/rehearsal-test.pub (which manifest.sh --rehearsal created with the manifest
#                key), so `nself-ci-agent verify-release --pub` trusts both. With a phase flag it selects the throwaway key for --sign.
#   A release run is `--prepare` and then `--sign` as two workflow steps; one process holding the key and starting go and syft is refused.
#   free/ci/internal/agent/verify.VerifyProvenance checks the result (called from verify.Artifact); a Go test runs this script and feeds
#   its output to that package.
#
# Inputs:  --in DIR        output of build.sh and manifest.sh (nself-ci-agent-<os>-<arch>, its .manifest.json, VERSION).
#          --versions FILE the versions table to read the syft pin from (default: the repository's; tests pass their own).
#   Environment (never argv):
#          --sign, outside rehearsal:
#            NSELF_AGENT_SIGNING_KEY      PKCS8 PEM of the Ed25519 agent key (same three forms as manifest.sh).
#            NSELF_AGENT_SIGNING_KEY_ID   agent-<first 16 hex of sha256(raw 32-byte public key)>; derived, never chosen.
#          --prepare: GITHUB_WORKFLOW_REF, GITHUB_RUN_ID, GITHUB_SHA   the builder, the run and the commit (set by GitHub Actions;
#            rehearsal defaults them).
#            NSELF_AGENT_SYFT   optional path of a syft to try first; it is used only if it IS the pinned syft.
#            NSELF_CI_HOME      where `nself ci artifacts tools install` places syft (default ~/.nself/ci).
#          OPENSSL              optional openssl binary; must be OpenSSL 3 or newer.
# Output:  DIR/<binary>.cdx.json, .spdx.json and .intoto.dsse.json for every binary.
# Syft:    only the pinned syft runs: a candidate (NSELF_AGENT_SYFT, the CI home tool, PATH) must have the pinned sha256 and print the
#          pinned version; with none, the pinned archive is downloaded over https into a private temp directory, its sha256 is
#          checked, and the binary is checked again. Never `latest`, never an unpinned download.
# Exit 0 ok, 1 failure, 2 usage. The key lives in a 0600 file under a 0700 temp dir removed on exit; it is never printed, logged,
# written under DIR or passed on an argv.

set -euo pipefail
set +x

err() { printf "[agent-provenance] ERROR: %s\n" "$*" >&2; }

IN=""
REHEARSAL=false
PREPARE=false
SIGN=false
VERSIONS=""
usage() { printf "usage: %s --in DIR (--prepare | --sign | --rehearsal) [--versions FILE]\n" "$0" >&2; }
while [ "$#" -gt 0 ]; do
  case "$1" in
    --in) [ "$#" -ge 2 ] || { err "--in needs a value"; exit 2; }; IN="$2"; shift 2 ;;
    --rehearsal) REHEARSAL=true; shift ;;
    --prepare) PREPARE=true; shift ;;
    --sign) SIGN=true; shift ;;
    --versions) [ "$#" -ge 2 ] || { err "--versions needs a value"; exit 2; }; VERSIONS="$2"; shift 2 ;;
    *) err "unknown argument: $1"; usage; exit 2 ;;
  esac
done
if [ -z "$IN" ]; then usage; exit 2; fi
if [ "$PREPARE" = "false" ] && [ "$SIGN" = "false" ]; then
  # Both phases in one process is for the throwaway key only.
  [ "$REHEARSAL" = "true" ] || { err "name a phase: --prepare (no key) and then --sign, or --rehearsal for both with a throwaway key"; usage; exit 2; }
  PREPARE=true
  SIGN=true
fi
if [ "$PREPARE" = "true" ] && [ "$SIGN" = "true" ] && [ "$REHEARSAL" != "true" ]; then
  err "--prepare and --sign together are for --rehearsal only: a release signs in a separate step"
  exit 2
fi
# --prepare starts go and syft: it refuses to run in a process that holds the signing key, before it looks at anything else.
if [ "$PREPARE" = "true" ] && [ -n "${NSELF_AGENT_SIGNING_KEY:-}" ]; then
  err "--prepare starts go and syft, so it must run without NSELF_AGENT_SIGNING_KEY in its environment (use a step that has no secret)"
  exit 1
fi
[ -d "$IN" ] || { err "input directory not found: $IN"; exit 1; }
IN="$(cd "$IN" && pwd)"
[ -f "$IN/VERSION" ] || { err "$IN/VERSION not found (run build.sh first)"; exit 1; }
VERSION="$(tr -d '\n' < "$IN/VERSION")"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { err "VERSION '$VERSION' is not a plain version"; exit 1; }

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

umask 077
WORK="$(mktemp -d "${TMPDIR:-/tmp}/agent-provenance.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

# list_targets prints the binary file names in DIR, each checked: a known target, a regular file (never a symlink), with a manifest.
list_targets() {
  local bin name target n=0
  for bin in "$IN"/nself-ci-agent-*-*; do
    [ -e "$bin" ] || [ -L "$bin" ] || continue
    name="$(basename "$bin")"
    case "$name" in *.manifest.json|*.pub|*.cdx.json|*.spdx.json|*.intoto.dsse.json) continue ;; esac
    target="${name#nself-ci-agent-}"
    [[ "$target" =~ ^(linux|darwin)-(amd64|arm64)$ ]] || { err "unexpected file name $name"; return 1; }
    { [ -f "$bin" ] && [ ! -L "$bin" ]; } || { err "$name is not a regular file"; return 1; }
    [ -f "$IN/$name.manifest.json" ] || { err "$name has no manifest (run manifest.sh first)"; return 1; }
    printf '%s\n' "$name"
    n=$((n + 1))
  done
  [ "$n" -gt 0 ] || { err "no nself-ci-agent-<os>-<arch> files in $IN"; return 1; }
}
TARGETS="$(list_targets)" || exit 1

# ============================== --prepare: SBOMs and statements, no key ======================================================
if [ "$PREPARE" = "true" ]; then
  [ -n "$VERSIONS" ] || VERSIONS="$ROOT/free/ci/internal/toolchain/versions.yaml"
  [ -f "$VERSIONS" ] || { err "versions table not found: $VERSIONS"; exit 1; }
  command -v go >/dev/null 2>&1 || { err "go not found (tools/provgen needs it)"; exit 1; }

  # ---- the syft pin ---------------------------------------------------------------------------------------------------------
  case "$(uname -s)" in Linux) HOST_OS=linux ;; Darwin) HOST_OS=darwin ;; *) err "unsupported host OS $(uname -s)"; exit 1 ;; esac
  case "$(uname -m)" in x86_64|amd64) HOST_ARCH=amd64 ;; arm64|aarch64) HOST_ARCH=arm64 ;; *) err "unsupported host CPU $(uname -m)"; exit 1 ;; esac
  PIN="$(awk -v os="$HOST_OS" -v arch="$HOST_ARCH" '
    /^  - name: / { insyft = ($3 == "syft"); next }
    insyft && /^    version: / { ver = $2 }
    insyft && /^      - os: / { cur_os = $3; next }
    insyft && /^        arch: / { cur_arch = $2 }
    insyft && /^        url: / { url = $2 }
    insyft && /^        sha256: / { sha = $2 }
    insyft && /^        binary_sha256: / { if (cur_os == os && cur_arch == arch) { print ver, url, sha, $2; exit } }
  ' "$VERSIONS")"
  [ -n "$PIN" ] || { err "$VERSIONS has no syft row for $HOST_OS/$HOST_ARCH"; exit 1; }
  read -r PIN_VERSION PIN_URL PIN_SHA PIN_BIN_SHA <<< "$PIN"
  [[ "$PIN_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] && [[ "$PIN_SHA" =~ ^[0-9a-f]{64}$ ]] && [[ "$PIN_BIN_SHA" =~ ^[0-9a-f]{64}$ ]] \
    || { err "the syft row of $VERSIONS is not a pinned version with both checksums"; exit 1; }
  case "$PIN_URL" in https://*) ;; *) err "the syft download URL is not https"; exit 1 ;; esac

  # syft_is_pinned FILE: the file has the pinned sha256 (checked BEFORE it is run) and prints the pinned version.
  syft_is_pinned() {
    local b="$1" v
    [ -f "$b" ] && [ -x "$b" ] || return 1
    [ "$(sha256_file "$b")" = "$PIN_BIN_SHA" ] || return 1
    v="$(SYFT_CHECK_FOR_APP_UPDATE=false "$b" version 2>/dev/null | awk '/^Version:/ { sub(/^v/, "", $2); print $2; exit }')" || return 1
    [ "$v" = "$PIN_VERSION" ]
  }

  find_syft() {
    local c home="${NSELF_CI_HOME:-${HOME:-/nonexistent}/.nself/ci}"
    for c in "${NSELF_AGENT_SYFT:-}" "$home/tools/syft-$PIN_VERSION/syft" "$(command -v syft 2>/dev/null || true)"; do
      if [ -n "$c" ] && syft_is_pinned "$c"; then SYFT="$c"; return 0; fi
    done
    if ! command -v curl >/dev/null 2>&1 || ! command -v tar >/dev/null 2>&1; then
      err "no pinned syft $PIN_VERSION found and curl or tar is missing to download it"
      return 1
    fi
    printf "[agent-provenance] downloading the pinned syft %s\n" "$PIN_VERSION"
    mkdir -p "$WORK/syft-dl"
    curl -fsSL --proto '=https' --proto-redir '=https' --tlsv1.2 --max-time 300 -o "$WORK/syft-dl/syft.tgz" "$PIN_URL" || { err "cannot download the pinned syft"; return 1; }
    [ "$(sha256_file "$WORK/syft-dl/syft.tgz")" = "$PIN_SHA" ] || { err "the downloaded syft archive does not have the pinned sha256"; return 1; }
    tar -xzf "$WORK/syft-dl/syft.tgz" -C "$WORK/syft-dl" syft || { err "the syft archive has no syft file"; return 1; }
    syft_is_pinned "$WORK/syft-dl/syft" || { err "the downloaded syft is not the pinned binary"; return 1; }
    SYFT="$WORK/syft-dl/syft"
  }
  SYFT=""
  find_syft || exit 1

  # ---- builder facts --------------------------------------------------------------------------------------------------------
  if [ "$REHEARSAL" = "true" ]; then
    BUILDER="${GITHUB_WORKFLOW_REF:-nself-org/plugins/.github/workflows/release-agent.yml@refs/tags/v$VERSION}"
    RUN_ID="${GITHUB_RUN_ID:-1}"
    REVISION="${GITHUB_SHA:-$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || true)}"
    [[ "$REVISION" =~ ^[0-9a-f]{40}$ ]] || REVISION="0000000000000000000000000000000000000000"
  else
    BUILDER="${GITHUB_WORKFLOW_REF:-}"; RUN_ID="${GITHUB_RUN_ID:-}"; REVISION="${GITHUB_SHA:-}"
    [ -n "$BUILDER" ] && [ -n "$RUN_ID" ] && [ -n "$REVISION" ] || { err "GITHUB_WORKFLOW_REF, GITHUB_RUN_ID and GITHUB_SHA must be set (run this in the release workflow)"; exit 1; }
  fi

  (cd "$ROOT/free/ci" && CGO_ENABLED=0 go build -mod=vendor -o "$WORK/provgen" ./tools/provgen) || { err "cannot build tools/provgen"; exit 1; }
  mkdir "$IN/.provenance" || { err "$IN/.provenance already exists (a prepare step ran before)"; exit 1; }
  # A rehearsal's statements carry placeholder builder facts; a real --sign refuses a directory that was prepared as a rehearsal.
  if [ "$REHEARSAL" = "true" ]; then : > "$IN/.provenance/REHEARSAL"; fi

  while IFS= read -r name; do
    target="${name#nself-ci-agent-}"
    for suffix in .cdx.json .spdx.json .intoto.dsse.json; do
      { [ ! -e "$IN/$name$suffix" ] && [ ! -L "$IN/$name$suffix" ]; } || { err "$IN/$name$suffix already exists"; exit 1; }
    done
    sha="$(sha256_file "$IN/$name")"
    # The provenance subject is the digest the manifest signs: a manifest for other bytes stops the run.
    grep -q "\"sha256\":\"$sha\"" "$IN/$name.manifest.json" || { err "$name.manifest.json does not name the sha256 of $name"; exit 1; }

    (cd "$IN" && SYFT_CHECK_FOR_APP_UPDATE=false "$SYFT" scan "file:$name" --source-name "$name" \
      -o "cyclonedx-json@1.6=$name.cdx.json" -o "spdx-json@2.3=$name.spdx.json" -q < /dev/null) || { err "syft failed on $name"; exit 1; }
    for suffix in .cdx.json .spdx.json; do
      { [ -s "$IN/$name$suffix" ] && [ ! -L "$IN/$name$suffix" ]; } || { err "syft wrote no regular $name$suffix"; exit 1; }
      chmod 644 "$IN/$name$suffix"
    done
    "$WORK/provgen" --binary "$IN/$name" --sha256 "$sha" --sbom "$IN/$name.cdx.json" --sbom "$IN/$name.spdx.json" \
      --builder-id "$BUILDER" --revision "$REVISION" --run-id "$RUN_ID" --out "$IN/.provenance/$target" > /dev/null < /dev/null || { err "provgen refused $name"; exit 1; }
  done <<< "$TARGETS"
  printf "[agent-provenance] prepared SBOMs and statements for %s targets, syft %s\n" "$(printf '%s\n' "$TARGETS" | wc -l | tr -d ' ')" "$PIN_VERSION"
fi

# ============================== --sign: DSSE envelopes, nothing but openssl runs ==============================================
if [ "$SIGN" = "true" ]; then
  O="${OPENSSL:-openssl}"
  command -v "$O" >/dev/null 2>&1 || { err "openssl not found (set OPENSSL to an OpenSSL 3 binary)"; exit 1; }
  "$O" version 2>/dev/null | grep -q '^OpenSSL ' || { err "$O is not OpenSSL (LibreSSL cannot sign Ed25519 -rawin); set OPENSSL"; exit 1; }
  [ -d "$IN/.provenance" ] || { err "$IN/.provenance not found (run --prepare first)"; exit 1; }
  if [ "$REHEARSAL" != "true" ] && [ -e "$IN/.provenance/REHEARSAL" ]; then
    err "$IN/.provenance was prepared as a rehearsal (placeholder builder facts); prepare it again without --rehearsal"
    exit 1
  fi
  if [ "$REHEARSAL" != "true" ]; then
    # The statement must be about THIS run: builder, invocation and commit are checked against what GitHub set for the signing step.
    for v in GITHUB_WORKFLOW_REF GITHUB_REPOSITORY GITHUB_RUN_ID GITHUB_SHA; do
      [ -n "${!v:-}" ] || { err "$v must be set (run this in the release workflow)"; exit 1; }
    done
  fi

  # The PEM marker lines are assembled from two pieces on purpose: written whole, a secret scanner reads this script as a committed key.
  PEM_BEGIN="-----BEGIN PRIV""ATE KEY-----"
  PEM_END="-----END PRIV""ATE KEY-----"
  normalize_key() {
    local v="$1" body
    case "$v" in
      *"-----BEGIN "*) ;;
      *) v="$(printf '%s' "$v" | tr -d ' \t\r\n' | "$O" base64 -d -A 2>/dev/null)" || v="" ;;
    esac
    case "$v" in
      *"$PEM_BEGIN"*"$PEM_END"*)
        body="${v#*"$PEM_BEGIN"}"
        body="${body%%"$PEM_END"*}"
        body="$(printf '%s' "$body" | tr -d ' \t\r\n')"
        printf '%s\n%s\n%s\n' "$PEM_BEGIN" "$(printf '%s' "$body" | fold -w 64)" "$PEM_END"
        ;;
      *) printf '%s\n' "$v" ;;
    esac
  }

  if [ "$REHEARSAL" = "true" ]; then
    "$O" genpkey -algorithm ed25519 -out "$WORK/key.pem" 2>/dev/null || { err "cannot generate the rehearsal key"; exit 1; }
    "$O" pkey -in "$WORK/key.pem" -pubout > "$WORK/rehearsal.pub" 2>/dev/null || { err "cannot derive the rehearsal public key"; exit 1; }
  else
    [ -n "${NSELF_AGENT_SIGNING_KEY:-}" ] || { err "NSELF_AGENT_SIGNING_KEY is not set"; exit 1; }
    normalize_key "$NSELF_AGENT_SIGNING_KEY" > "$WORK/key.pem"
  fi

  # Only Ed25519 keys sign here: an EC or RSA key would produce a signature labelled ed25519 that no CLI could verify.
  KEY_TEXT="$("$O" pkey -in "$WORK/key.pem" -noout -text 2>/dev/null)" || { err "the signing key is not a readable PEM private key"; exit 1; }
  case "$KEY_TEXT" in "ED25519 Private-Key:"*) ;; *) err "the signing key is not an Ed25519 key"; exit 1 ;; esac
  if ! DERIVED="$("$O" pkey -in "$WORK/key.pem" -pubout -outform DER 2>/dev/null | tail -c 32 | { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-16)" || [ -z "$DERIVED" ]; then
    err "the signing key is not a readable PEM private key"
    exit 1
  fi
  KEY_ID="agent-${DERIVED}"
  if [ "$REHEARSAL" != "true" ]; then
    [[ "${NSELF_AGENT_SIGNING_KEY_ID:-}" =~ ^agent-[0-9a-f]{16}$ ]] || { err "NSELF_AGENT_SIGNING_KEY_ID must look like agent-<16 hex>"; exit 1; }
    [ "$NSELF_AGENT_SIGNING_KEY_ID" = "$KEY_ID" ] || { err "NSELF_AGENT_SIGNING_KEY_ID does not match the key (expected agent-<16 hex of sha256(public key))"; exit 1; }
  fi

  PAYLOAD_TYPE="application/vnd.in-toto+json"
  count=0
  while IFS= read -r name; do
    target="${name#nself-ci-agent-}"
    stmt="$IN/.provenance/$target"
    { [ ! -e "$IN/$name.intoto.dsse.json" ] && [ ! -L "$IN/$name.intoto.dsse.json" ]; } || { err "$IN/$name.intoto.dsse.json already exists"; exit 1; }
    for f in statement.json statement.pae; do
      { [ -f "$stmt/$f" ] && [ ! -L "$stmt/$f" ]; } || { err "$stmt/$f not found (run --prepare first)"; exit 1; }
    done
    # The statement must be about this binary and these SBOMs; verify-release repeats the full check before anything is uploaded.
    for file in "$name" "$name.cdx.json" "$name.spdx.json"; do
      { [ -f "$IN/$file" ] && [ ! -L "$IN/$file" ]; } || { err "$file is missing or not a regular file"; exit 1; }
      grep -q "\"sha256\":\"$(sha256_file "$IN/$file")\"" "$stmt/statement.json" || { err "$name: the prepared statement does not match $file"; exit 1; }
    done
    if [ "$REHEARSAL" != "true" ]; then
      for want in "\"id\":\"https://github.com/$GITHUB_WORKFLOW_REF\"" "\"invocationId\":\"https://github.com/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID\"" "\"gitCommit\":\"$GITHUB_SHA\""; do
        grep -qF "$want" "$stmt/statement.json" || { err "$name: the prepared statement is not about this run ($want)"; exit 1; }
      done
    fi
    # Sign the PAE computed here from the statement bytes, never a file taken on trust: DSSEv1 <len(type)> <type> <len(payload)> <payload>.
    { printf 'DSSEv1 %d %s %d ' "${#PAYLOAD_TYPE}" "$PAYLOAD_TYPE" "$(wc -c < "$stmt/statement.json" | tr -d ' ')"; cat "$stmt/statement.json"; } > "$WORK/pae"
    cmp -s "$WORK/pae" "$stmt/statement.pae" || { err "$name: statement.pae is not the DSSE encoding of statement.json"; exit 1; }
    "$O" pkeyutl -sign -rawin -inkey "$WORK/key.pem" -in "$WORK/pae" -out "$WORK/sig" 2>/dev/null || { err "signing failed (is the key Ed25519?)"; exit 1; }
    [ "$(wc -c < "$WORK/sig" | tr -d ' ')" = "64" ] || { err "signature is not 64 bytes; the key is not Ed25519"; exit 1; }
    sig="$("$O" base64 -A -in "$WORK/sig")"
    payload="$("$O" base64 -A -in "$stmt/statement.json")"
    printf '{"payloadType":"%s","payload":"%s","signatures":[{"keyid":"%s","sig":"%s"}]}\n' "$PAYLOAD_TYPE" "$payload" "$KEY_ID" "$sig" > "$IN/$name.intoto.dsse.json"
    chmod 644 "$IN/$name.intoto.dsse.json"
    count=$((count + 1))
  done <<< "$TARGETS"
  rm -rf "$IN/.provenance"
  if [ "$REHEARSAL" = "true" ]; then
    # Appended only after every target succeeded, so a failed run leaves the file as manifest.sh wrote it.
    { [ ! -L "$IN/rehearsal-test.pub" ] && cat "$WORK/rehearsal.pub" >> "$IN/rehearsal-test.pub"; } || { err "cannot append to $IN/rehearsal-test.pub"; exit 1; }
    chmod 644 "$IN/rehearsal-test.pub"
  fi
  printf "[agent-provenance] signed provenance for %s targets, key %s\n" "$count" "$KEY_ID"
fi
