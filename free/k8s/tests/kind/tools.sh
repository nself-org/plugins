#!/usr/bin/env bash
# Purpose: install the pinned kind, kubectl and helm binaries for run.sh.
# Inputs: tools.env (versions and sha256 per os/arch); $1 = destination bin dir;
#   optional TOOLS_CACHE (a directory that keeps verified downloads between runs).
# Outputs: kind, kubectl and helm in the destination dir; exit 1 on a checksum
#   mismatch or an unsupported platform. A binary is never run before its sum matches.
# Constraints: downloads come only from the project hosts below; nothing is
#   installed outside the destination and the cache.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
. "$here/tools.env"

dest="${1:?usage: tools.sh <bin dir>}"
cache="${TOOLS_CACHE:-$dest/.cache}"
mkdir -p "$dest" "$cache"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "tools.sh: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac
case "$os" in linux | darwin) ;; *) echo "tools.sh: unsupported os $os" >&2; exit 1 ;; esac

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# fetch <file in cache> <url> <expected sha256>: download once, verify always.
fetch() {
  local name="$1" url="$2" want="$3" got
  if [ -z "$want" ]; then echo "tools.sh: no pinned sum for $name on $os/$arch" >&2; exit 1; fi
  if [ ! -f "$cache/$name" ] || [ "$(sha256_of "$cache/$name")" != "$want" ]; then
    curl -fsSL --retry 3 --retry-delay 2 -o "$cache/$name.part" "$url"
    mv "$cache/$name.part" "$cache/$name"
  fi
  got="$(sha256_of "$cache/$name")"
  if [ "$got" != "$want" ]; then
    echo "tools.sh: checksum mismatch for $name: got $got, want $want" >&2
    rm -f "$cache/$name"
    exit 1
  fi
}

pin() { local v="$1_${os}_${arch}"; printf '%s' "${!v:-}"; }

fetch "kind-$KIND_VERSION-$os-$arch" "https://kind.sigs.k8s.io/dl/$KIND_VERSION/kind-$os-$arch" "$(pin KIND_SHA256)"
install -m 0755 "$cache/kind-$KIND_VERSION-$os-$arch" "$dest/kind"

fetch "kubectl-$KUBECTL_VERSION-$os-$arch" "https://dl.k8s.io/release/$KUBECTL_VERSION/bin/$os/$arch/kubectl" "$(pin KUBECTL_SHA256)"
install -m 0755 "$cache/kubectl-$KUBECTL_VERSION-$os-$arch" "$dest/kubectl"

tgz="helm-$HELM_VERSION-$os-$arch.tar.gz"
fetch "$tgz" "https://get.helm.sh/$tgz" "$(pin HELM_SHA256)"
tar -xzf "$cache/$tgz" -C "$dest" --strip-components=1 "$os-$arch/helm"
chmod 0755 "$dest/helm"

"$dest/kind" version
"$dest/kubectl" version --client
"$dest/helm" version --short
