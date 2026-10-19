#!/bin/sh
# apt-snapshot.sh STAMP: point apt at the dated snapshot.debian.org archive of STAMP (YYYYMMDDTHHMMSSZ),
# so two builds from the same base digest install the same package bytes (debt D-0994). STAMP comes
# from the build ARG APT_SNAPSHOT, which the Dockerfile takes from pins/apt.yaml; the drift test keeps
# them equal. Used only inside the image build, before the first `apt-get update`.
# Env: APT_DIR (default /etc/apt) is where sources.list.d and apt.conf.d are written (tests set it).
# Integrity: plain http on purpose (the slim base has no ca-certificates before the first install);
# apt verifies the signed InRelease files against the debian-archive-keyring already in the base image.
set -eu
stamp=${1:-}
case "$stamp" in
[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]T[0-9][0-9][0-9][0-9][0-9][0-9]Z) ;;
*)
	echo "apt-snapshot: expected a timestamp like 20261005T000000Z, got '$stamp'" >&2
	exit 2
	;;
esac
dir=${APT_DIR:-/etc/apt}
mkdir -p "$dir/sources.list.d" "$dir/apt.conf.d"
rm -f "$dir/sources.list" "$dir"/sources.list.d/*
cat >"$dir/sources.list.d/debian.sources" <<EOF
Types: deb
URIs: http://snapshot.debian.org/archive/debian/$stamp/
Suites: trixie trixie-updates
Components: main
Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg

Types: deb
URIs: http://snapshot.debian.org/archive/debian-security/$stamp/
Suites: trixie-security
Components: main
Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg
EOF
# A dated Release file is past its Valid-Until by design; the signature check stays on.
cat >"$dir/apt.conf.d/99snapshot" <<EOF
Acquire::Check-Valid-Until "false";
Acquire::Retries "5";
EOF
