#!/usr/bin/env bash
# Shared by the acceptance scenario drills (P7-CI-47); sourced, never run.
#
# Purpose: cut outbound network access for the processes of a drill and read back how many packets the cut dropped, so
#   "no network after the cut" is measured, not assumed (review F7). Linux: an `nft` table that drops non-loopback OUTPUT.
#   macOS: a `pf` ruleset that blocks non-lo0 egress. Every privileged call is `sudo -n`, so a missing sudo credential
#   fails at once with a message instead of waiting for a password.
# Inputs:  CUT_WORK (a scratch directory); the commands to run inside the cut are given to cut_exec.
# Outputs: cut_apply / cut_remove / cut_exec / cut_packets / cut_info.
# Constraints: the rule matches the drill's own group id (CUT_GID), not the machine: a hosted runner's agent keeps
#   reporting the job's log while the drill cannot reach the network. cut_exec drops privileges to the calling user with
#   that group, so the drill's processes keep their files and their docker access. cut_remove is idempotent and runs
#   on every exit path (the drill traps EXIT, INT and TERM). Nothing here touches the network.

CUT_GID=${CUT_GID:-48731}
CUT_ACTIVE=0
CUT_OS=$(uname -s)
CUT_PF_TOKEN=""

cut_die() { echo "${EV_JOB:-drill}: $*" >&2; exit 2; }

# cut_preflight: sudo must work without a password; the platform tool must exist.
cut_preflight() {
	sudo -n true 2>/dev/null || cut_die "sudo -n failed: this drill cuts the network with nft (Linux) or pf (macOS) and needs a passwordless sudo"
	command -v perl >/dev/null || cut_die "perl is required to run the drill's commands under the cut group"
	case "$CUT_OS" in
	Linux) command -v nft >/dev/null || cut_die "nft is required (apt-get install nftables)" ;;
	Darwin) command -v pfctl >/dev/null || cut_die "pfctl is required" ;;
	*) cut_die "no network cut for $CUT_OS" ;;
	esac
}

# cut_exec CMD...: run CMD as the calling user with group CUT_GID (and the user's other groups). With no cut active it
# just runs CMD. Root is only used to set the group; the command never runs as root.
cut_exec() {
	if [ "$CUT_ACTIVE" -ne 1 ]; then "$@"; return; fi
	sudo -n perl -e 'my ($u, $g, $gs) = (shift, shift, shift); $( = $g; $) = "$g $gs"; $< = $u; $> = $u; die "drop failed: $!\n" if $> != $u || $< != $u; exec @ARGV or die "exec $ARGV[0]: $!\n"' \
		"$(id -u)" "$CUT_GID" "$(id -G)" "$@"
}

# cut_apply: load the rule. Returns 1 when the platform refuses.
cut_apply() {
	case "$CUT_OS" in
	Linux)
		sudo -n nft delete table inet drillcut 2>/dev/null || true
		sudo -n nft -f - <<NFT || return 1
table inet drillcut {
	chain out {
		type filter hook output priority 0; policy accept;
		meta skgid $CUT_GID oifname != "lo" counter drop
	}
}
NFT
		;;
	Darwin)
		printf 'block drop out quick on ! lo0 proto { tcp udp } from any to any group %s\n' "$CUT_GID" >"$CUT_WORK/pf.rules"
		CUT_PF_TOKEN=$(sudo -n pfctl -E 2>&1 | awk '/Token/ {print $NF}')
		sudo -n pfctl -f "$CUT_WORK/pf.rules" >/dev/null 2>&1 || return 1
		;;
	esac
	CUT_ACTIVE=1
}

# cut_remove: take the rule out again; safe to call twice.
cut_remove() {
	[ "$CUT_ACTIVE" -eq 1 ] || return 0
	CUT_ACTIVE=0
	case "$CUT_OS" in
	Linux) sudo -n nft delete table inet drillcut 2>/dev/null || true ;;
	Darwin)
		sudo -n pfctl -f /etc/pf.conf >/dev/null 2>&1 || true
		if [ -n "$CUT_PF_TOKEN" ]; then sudo -n pfctl -X "$CUT_PF_TOKEN" >/dev/null 2>&1 || sudo -n pfctl -d >/dev/null 2>&1 || true; fi
		;;
	esac
}

# cut_packets: the number of packets the rule has dropped so far (empty when unreadable).
cut_packets() {
	case "$CUT_OS" in
	Linux) sudo -n nft list table inet drillcut 2>/dev/null | sed -n 's/.*counter packets \([0-9][0-9]*\).*/\1/p' | head -n 1 ;;
	Darwin) sudo -n pfctl -s rules -v 2>/dev/null | sed -n 's/.*Packets: \([0-9][0-9]*\).*/\1/p' | head -n 1 ;;
	esac
}

# cut_info: the counter text kept in the evidence (nft table with counters; pfctl -s info and the rule's statistics).
cut_info() {
	case "$CUT_OS" in
	Linux) sudo -n nft list table inet drillcut 2>&1 | tr '\n\t' '  ' | tr -s ' ' ;;
	Darwin) { sudo -n pfctl -s info 2>&1 | grep -E 'Status|block|match' | tr '\n' ' '; sudo -n pfctl -s rules -v 2>&1 | grep -E 'Packets' | tr '\n' ' '; } | tr -s ' ' ;;
	esac
}

# cut_probe: try to reach a public address from inside the cut. Succeeds (0) when the connection FAILED, as it must.
cut_probe() {
	if cut_exec /usr/bin/env "PATH=$PATH" curl --silent --output /dev/null --connect-timeout 4 --max-time 6 https://1.1.1.1/ 2>/dev/null; then return 1; fi
	return 0
}
