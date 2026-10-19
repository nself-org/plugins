#!/usr/bin/env bash
# Shared by the acceptance scenario drills (P7-CI-47); sourced, never run.
#
# Purpose: cut outbound network access for the processes of a drill and read back how many packets the cut dropped, so
#   "no network after the cut" is measured, not assumed (review F7). Linux: an `nft` table that drops non-loopback OUTPUT.
#   macOS: a `pf` anchor (under com.apple/, which the system ruleset evaluates) that blocks non-lo0 TCP and UDP egress
#   (pf matches a group only for those two protocols). Every privileged call is `sudo -n`, so a missing sudo credential
#   fails at once with a message instead of waiting for a password.
# Inputs:  the commands to run inside the cut are given to cut_exec.
# Outputs: cut_apply / cut_remove / cut_exec / cut_packets / cut_info.
# Constraints: the rule matches the drill's own group id (CUT_GID), not the machine: a hosted runner's agent keeps
#   reporting the job's log while the drill cannot reach the network. On Linux a second chain also blocks and counts
#   traffic that containers forward from a docker bridge to anywhere else. Only the drill's own table (Linux) or anchor
#   (macOS) is ever touched; the system's firewall rules are left as they were. While the cut is on, containers on a docker
#   bridge cannot reach beyond the host (every bridge, not only this run's): cut_preflight warns when other containers run.
#   If a run is killed with SIGKILL the cut stays: remove it with `sudo nft delete table inet drillcut` (Linux) or
#   `sudo pfctl -a com.apple/250.drillcut -F all` (macOS). cut_exec drops privileges to the calling user with
#   that group, so the drill's processes keep their files and their docker access. cut_remove is idempotent and runs
#   on every exit path (the drill traps EXIT, INT and TERM). Nothing here touches the network.

CUT_GID=${CUT_GID:-48731}
CUT_ACTIVE=0
CUT_OS=$(uname -s)
CUT_PF_TOKEN=""
CUT_PF_ANCHOR=com.apple/250.drillcut

cut_die() { echo "${EV_JOB:-drill}: $*" >&2; exit 2; }

# cut_preflight: sudo must work without a password; the platform tool must exist.
cut_preflight() {
	[ "$(id -u)" -ne 0 ] || cut_die "run this drill as your normal user, not as root: the commands under the cut must not stay root"
	sudo -n true 2>/dev/null || cut_die "sudo -n failed: this drill cuts the network with nft (Linux) or pf (macOS) and needs a passwordless sudo"
	command -v perl >/dev/null || cut_die "perl is required to run the drill's commands under the cut group"
	case "$CUT_OS" in
	Linux)
		command -v nft >/dev/null || cut_die "nft is required (apt-get install nftables)"
		if [ -n "$(docker ps -q 2>/dev/null)" ]; then echo "${EV_JOB:-drill}: warning: running containers lose outbound access (forwarding from docker bridges) while the cut is on" >&2; fi
		;;
	Darwin) command -v pfctl >/dev/null || cut_die "pfctl is required" ;;
	*) cut_die "no network cut for $CUT_OS" ;;
	esac
}

# The code that sets the group and drops root. Arguments: uid gid "other groups" command...; it checks the result in full
# (uid, egid and the whole group list) before it runs the command, and dies otherwise.
read -r -d '' CUT_PERL <<'PERL' || true
my ($u, $g, $gs) = (shift, shift, shift);
my %want = map { $_ => 1 } ($g, split " ", $gs);
$( = $g;
$) = "$g " . join(" ", $g, split(" ", $gs));
$< = $u;
$> = $u;
my ($egid, @sup) = split " ", $);
my %got = map { $_ => 1 } ($egid, @sup);
my $same = (join(" ", sort { $a <=> $b } keys %want) eq join(" ", sort { $a <=> $b } keys %got));
die "drop failed: uid $< $> egid $egid groups $) (wanted uid $u gid $g groups $gs)\n" if $> != $u || $< != $u || $egid != $g || !$same;
exec @ARGV or die "exec $ARGV[0]: $!\n";
PERL

# cut_exec CMD...: run CMD as the calling user with group CUT_GID (and up to 15 of the user's other groups: macOS allows 16
# in all). With no cut active it just runs CMD. Root is only used to set the group; the command never runs as root.
cut_exec() {
	if [ "$CUT_ACTIVE" -ne 1 ]; then "$@"; return; fi
	local groups
	groups=$(id -G | tr ' ' '\n' | head -n 15 | tr '\n' ' ')
	sudo -n perl -e "$CUT_PERL" "$(id -u)" "$CUT_GID" "$groups" "$@"
}

# cut_apply: load the rule. CUT_ACTIVE is set before anything is changed, so a signal in the middle still runs cut_remove.
# Returns 1 when the platform refuses (everything done so far is rolled back).
cut_apply() {
	CUT_ACTIVE=1
	case "$CUT_OS" in
	Linux)
		sudo -n nft delete table inet drillcut 2>/dev/null || true
		sudo -n nft -f - <<NFT || { cut_remove; return 1; }
table inet drillcut {
	chain out {
		type filter hook output priority 0; policy accept;
		meta skgid $CUT_GID oifname != "lo" counter drop
	}
	chain fwd {
		type filter hook forward priority -10; policy accept;
		iifname "docker0" oifname != "docker0" counter drop
		iifname "br-*" oifname != "br-*" counter drop
	}
}
NFT
		;;
	Darwin)
		CUT_PF_TOKEN=$(sudo -n pfctl -E 2>&1 | awk '/Token/ {print $NF}')
		printf 'block drop out quick on ! lo0 proto { tcp udp } from any to any group %s\n' "$CUT_GID" | sudo -n pfctl -a "$CUT_PF_ANCHOR" -f - >/dev/null 2>&1 || { cut_remove; return 1; }
		;;
	esac
}

# cut_remove: take the drill's own rule out again (idempotent). macOS: empty our anchor and release our pf enable token.
cut_remove() {
	[ "$CUT_ACTIVE" -eq 1 ] || return 0
	CUT_ACTIVE=0
	case "$CUT_OS" in
	Linux) sudo -n nft delete table inet drillcut 2>/dev/null || true ;;
	Darwin)
		sudo -n pfctl -a "$CUT_PF_ANCHOR" -F all >/dev/null 2>&1 || true
		if [ -n "$CUT_PF_TOKEN" ]; then sudo -n pfctl -X "$CUT_PF_TOKEN" >/dev/null 2>&1 || true; fi
		;;
	esac
}

# cut_packets: the number of packets the rules have dropped so far, all counters summed (0 when unreadable).
cut_packets() {
	case "$CUT_OS" in
	Linux) sudo -n nft list table inet drillcut 2>/dev/null | sed -n 's/.*counter packets \([0-9][0-9]*\).*/\1/p' | awk '{n += $1} END {print n + 0}' ;;
	Darwin) sudo -n pfctl -a "$CUT_PF_ANCHOR" -s rules -v 2>/dev/null | sed -n 's/.*Packets: \([0-9][0-9]*\).*/\1/p' | awk '{n += $1} END {print n + 0}' ;;
	esac
}

# cut_info: the counter text kept in the evidence (nft table with counters; pfctl -s info and the rule's statistics).
cut_info() {
	case "$CUT_OS" in
	Linux) sudo -n nft list table inet drillcut 2>&1 | tr '\n\t' '  ' | tr -s ' ' ;;
	Darwin) { sudo -n pfctl -s info 2>&1 | grep -E 'Status|block|match' | tr '\n' ' '; sudo -n pfctl -a "$CUT_PF_ANCHOR" -s rules -v 2>&1 | grep -E 'Packets' | tr '\n' ' '; } | tr -s ' ' ;;
	esac
}

# cut_probe: try to reach a public address from inside the cut. Succeeds (0) when the connection FAILED, as it must.
cut_probe() {
	if cut_exec /usr/bin/env "PATH=$PATH" curl --silent --output /dev/null --connect-timeout 4 --max-time 6 https://1.1.1.1/ 2>/dev/null; then return 1; fi
	return 0
}
