#!/bin/sh
# fetch-verified.sh NAME ARCH OUTFILE: download the pinned artifact of NAME for ARCH and
# refuse to continue unless its sha256 matches the pin. The URL and checksum come from the
# build ARGs NAME_URL_ARCH and NAME_SHA256_ARCH (upper-case, '-' as '_'), which the Dockerfile
# takes from pins/*.yaml; the drift test keeps them equal. Used only inside the image build.
set -eu
name=$1
arch=$2
out=$3
up() { printf '%s' "$1" | tr 'a-z-' 'A-Z_'; }
n=$(up "$name")
a=$(up "$arch")
eval "url=\${${n}_URL_${a}:-}"
eval "sha=\${${n}_SHA256_${a}:-}"
if [ -z "$url" ] || [ -z "$sha" ]; then
	echo "fetch-verified: no pin for $name/$arch" >&2
	exit 1
fi
curl --fail --silent --show-error --location --retry 5 --proto '=https' --tlsv1.2 --output "$out" "$url"
echo "$sha  $out" | sha256sum -c -
