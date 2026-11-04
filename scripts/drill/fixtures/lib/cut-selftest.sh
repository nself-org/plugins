#!/usr/bin/env bash
# cut-selftest.sh: fixture test for cut_count_chains in cut.sh (D-1541).
# Purpose: prove the no-egress-after-cut check of scenario-01-local-only still fails when the JOB emits a packet, and
#   no longer fails for the docker/containerd daemons' background packets. Planted `nft list table` listings stand in for
#   the kernel: one with host-daemon packets only (must not move the job count), one with a packet in the job's group
#   chain (must), one with a packet forwarded off a docker bridge (must).
# Inputs: none. No sudo, no nft, no network.
# Outputs: "ok:"/"FAIL:" per case; exit 1 on any failure.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
CUT_WORK=$(mktemp -d "${TMPDIR:-/tmp}/cut-selftest.XXXXXX") || exit 1
trap 'rm -rf "$CUT_WORK"' EXIT
# shellcheck source=/dev/null
. "$here/cut.sh"
fail=0

# listing OUT BRIDGE DAEMON: a table listing with those packet counts in the three counted chains.
listing() {
	printf 'table inet drillcut_1 {\n\tchain out {\n\t\ttype filter hook output priority filter; policy accept;\n'
	printf '\t\tmeta skgid 40001 udp dport 53 counter packets 0 bytes 0 drop\n\t\tmeta skgid 40001 oifname != "lo" counter packets %s bytes 90 drop\n\t}\n' "$1"
	printf '\tchain bridge_out {\n\t\tiifname "docker0" oifname != "docker0" counter packets %s bytes 90 drop\n\t}\n' "$2"
	printf '\tchain docker_daemon {\n\t\tsocket cgroupv2 level 2 "system.slice/docker.service" counter packets %s bytes 90 drop\n\t}\n}\n' "$3"
}
# verdict BEFORE AFTER: the scenario's assertion (equal job counts = pass).
verdict() { if [ "$1" = "$2" ]; then echo pass; else echo fail; fi; }
check() {
	if [ "$2" = "$3" ]; then echo "ok: $1 ($3)"; else echo "FAIL: $1: want $2, got $3" >&2; fail=1; fi
}

before=$(listing 3 0 0 | cut_count_chains out bridge_out)
check job-count-before 3 "$before"
check host-count-only-daemon 2 "$(listing 3 0 2 | cut_count_chains docker_daemon)"
check host-background-passes pass "$(verdict "$before" "$(listing 3 0 2 | cut_count_chains out bridge_out)")"
check planted-job-packet-fails fail "$(verdict "$before" "$(listing 4 0 0 | cut_count_chains out bridge_out)")"
check planted-job-container-packet-fails fail "$(verdict "$before" "$(listing 3 1 0 | cut_count_chains out bridge_out)")"
check planted-job-packet-with-host-noise-fails fail "$(verdict "$before" "$(listing 4 0 2 | cut_count_chains out bridge_out)")"
exit "$fail"
