#!/usr/bin/env bash
# Shared by the acceptance scenario drills (P7-CI-47); sourced, never run.
#
# Purpose: cut outbound network access for the processes of a drill and read back how many packets the cut dropped, so
#   "no network after the cut" is measured, not assumed (review F7). Linux: an `nft` table that drops non-loopback, non-bridge OUTPUT
#   of the drill's group (a published container port is reached over the docker bridge), DNS (port 53, even to the local stub resolver), forwarding out of docker bridges and, where
#   the kernel supports `socket cgroupv2`, non-loopback output of docker.service and containerd.service. macOS: a `pf`
#   anchor (under com.apple/, which the system ruleset evaluates) that blocks non-lo0 TCP and UDP of the drill's group;
#   DNS through mDNSResponder and ICMP are not measured there. Every privileged call is `sudo -n`, so a missing sudo
#   credential fails at once with a message instead of waiting for a password.
# Inputs:  CUT_WORK (a scratch directory for the watchdog's stop file); commands to run inside the cut go to cut_exec.
# Outputs: cut_preflight / cut_apply / cut_remove / cut_exec / cut_selftest / cut_probe_ip / cut_probe_dns / cut_packets /
#          cut_info / CUT_NOTE (what the counters do and do not cover).
# Constraints: the rule matches the drill's own group id (CUT_GID, unique per run), not the machine: a hosted runner's agent
#   keeps reporting the job's log while the drill cannot reach the network. The table (Linux) and anchor (macOS) names carry
#   the run's pid, so two runs never touch each other's rules; the system's firewall is left as it was. cut_exec drops
#   privileges to the calling user with that group (Linux: setpriv with no_new_privs, so sudo and setuid binaries inside
#   the cut fail; macOS: perl, which cannot offer that). While the cut is on, containers on any docker bridge cannot reach
#   beyond the host: cut_preflight refuses while other containers run (DRILL_CUT_ALLOW_SHARED_HOST=1 overrides). cut_remove is idempotent and runs on every exit path
#   the shell can see (the drill traps EXIT, INT, TERM and HUP); for SIGKILL a detached root watchdog removes the cut when
#   the drill's pid is gone. By hand: `sudo nft list tables | grep drillcut` / `sudo pfctl -s Anchors`. Nothing here
#   touches the network.

CUT_ID=$$
CUT_GID=${CUT_GID:-$((40000 + CUT_ID % 20000))}
CUT_ACTIVE=0
CUT_OS=$(uname -s)
CUT_PF_TOKEN=""
CUT_TABLE=drillcut_$CUT_ID
CUT_PF_ANCHOR=com.apple/250.drillcut.$CUT_ID
CUT_DOCKERD=0
CUT_NOTE=""
# shellcheck disable=SC2034 # read by the drills
CUT_ERR=""
case "$CUT_GID" in '' | *[!0-9]*) echo "CUT_GID must be a number" >&2; exit 2 ;; esac

cut_die() { echo "${EV_JOB:-drill}: $*" >&2; exit 2; }

# cut_preflight: sudo must work without a password; the platform tools must exist; refuse a group id the host already uses.
cut_preflight() {
	[ "$(id -u)" -ne 0 ] || cut_die "run this drill as your normal user, not as root: the commands under the cut must not stay root"
	sudo -n true 2>/dev/null || cut_die "sudo -n failed: this drill cuts the network with nft (Linux) or pf (macOS) and needs a passwordless sudo"
	[ -x /usr/bin/perl ] || cut_die "/usr/bin/perl is required"
	case "$CUT_OS" in
	Linux)
		command -v nft >/dev/null || cut_die "nft is required (apt-get install nftables)"
		command -v setpriv >/dev/null || cut_die "setpriv (util-linux) is required"
		! getent group "$CUT_GID" >/dev/null 2>&1 || cut_die "group id $CUT_GID is already in use; set CUT_GID to a free number"
		# The cut drops forwarding out of every docker bridge and the docker daemon's own egress, so it would break other
		# containers on a shared box. Refuse unless the caller says this host is the drill's own (hosted runner, throwaway VM).
		if [ -n "$(docker ps -q 2>/dev/null)" ] && [ "${DRILL_CUT_ALLOW_SHARED_HOST:-}" != 1 ]; then
			cut_die "containers are running ($(docker ps --format "{{.ID}} {{.Image}} {{.Names}} {{.Status}}" 2>/dev/null | tr "\n" ";")); the cut would cut their network too. Stop them, or set DRILL_CUT_ALLOW_SHARED_HOST=1 on a host that is only for this drill"
		fi
		;;
	Darwin) command -v pfctl >/dev/null || cut_die "pfctl is required" ;;
	*) cut_die "no network cut for $CUT_OS" ;;
	esac
}

# The macOS code that sets the effective group and drops root. Arguments: uid gid command...; it checks uid and egid before it
# runs the command and dies otherwise. The user's other groups stay as sudo left them (setgroups does not take effect here).
read -r -d '' CUT_PERL <<'PERL' || true
my ($u, $g) = (shift, shift);
$( = $g;
$) = "$g $g";
$< = $u;
$> = $u;
my $egid = (split " ", $))[0];
die "drop failed: uid $< $> egid $egid (wanted uid $u egid $g)\n" if $> != $u || $< != $u || $egid != $g;
exec { $ARGV[0] } @ARGV or die "exec $ARGV[0]: $!\n";
PERL

# cut_exec CMD...: run CMD as the calling user with group CUT_GID (Linux: and the user's other groups, so docker still works).
# With no cut active it just runs CMD. Root is only used to set the group; the command never runs as root.
cut_exec() {
	if [ "$CUT_ACTIVE" -ne 1 ]; then "$@"; return; fi
	if [ "$CUT_OS" = Linux ]; then
		sudo -n setpriv --reuid "$(id -u)" --regid "$CUT_GID" --groups "$(id -G | tr -s ' ' ',')" --no-new-privs -- "$@"
	else
		sudo -n /usr/bin/perl -e "$CUT_PERL" "$(id -u)" "$CUT_GID" "$@"
	fi
}

# cut_selftest: the commands under the cut run as this user with group CUT_GID (and, on Linux, cannot become root again).
cut_selftest() {
	local u g
	u=$(cut_exec /usr/bin/id -u 2>&1 | tail -n 1)
	[ "$u" = "$(id -u)" ] || { echo "uid under the cut is '$u', not $(id -u)"; return 1; }
	g=$(cut_exec /usr/bin/id -g 2>&1 | tail -n 1)
	[ "$g" = "$CUT_GID" ] || { echo "gid under the cut is '$g', not $CUT_GID"; return 1; }
	if [ "$CUT_OS" = Linux ] && cut_exec /usr/bin/sudo -n true >/dev/null 2>&1; then echo "sudo still works under the cut"; return 1; fi
	return 0
}

# cut_apply: load the rules. CUT_ACTIVE is set before anything is changed, so a signal in the middle still runs cut_remove.
# Returns 1 when the platform refuses (everything done so far is rolled back).
cut_apply() {
	CUT_ACTIVE=1
	case "$CUT_OS" in
	Linux)
		sudo -n nft -f - 2>"$CUT_WORK/nft.err" <<NFT || { CUT_ERR=$(tr '\n' ' ' <"$CUT_WORK/nft.err" | cut -c1-300); cut_remove; return 1; }
table inet $CUT_TABLE {
	chain out {
		type filter hook output priority 0; policy accept;
		meta skgid $CUT_GID udp dport 53 counter drop
		meta skgid $CUT_GID tcp dport 53 counter drop
		meta skgid $CUT_GID oifname "docker0" accept
		meta skgid $CUT_GID oifname "br-*" accept
		meta skgid $CUT_GID oifname != "lo" counter drop
	}
	chain bridge_out {
		type filter hook forward priority -10; policy accept;
		iifname "docker0" oifname != "docker0" counter drop
		iifname "br-*" oifname != "br-*" counter drop
	}
}
NFT
		# The docker daemon makes its own connections (outside the drill's group); count and drop those too where nft can.
		if sudo -n nft -f - >/dev/null 2>&1 <<NFT; then CUT_DOCKERD=1; fi
add chain inet $CUT_TABLE docker_daemon { type filter hook output priority 0; policy accept; }
add rule inet $CUT_TABLE docker_daemon socket cgroupv2 level 2 "system.slice/docker.service" oifname { "lo", "docker0" } accept
add rule inet $CUT_TABLE docker_daemon socket cgroupv2 level 2 "system.slice/docker.service" oifname "br-*" accept
add rule inet $CUT_TABLE docker_daemon socket cgroupv2 level 2 "system.slice/docker.service" counter drop
add rule inet $CUT_TABLE docker_daemon socket cgroupv2 level 2 "system.slice/containerd.service" oifname { "lo", "docker0" } accept
add rule inet $CUT_TABLE docker_daemon socket cgroupv2 level 2 "system.slice/containerd.service" oifname "br-*" accept
add rule inet $CUT_TABLE docker_daemon socket cgroupv2 level 2 "system.slice/containerd.service" counter drop
NFT
		# shellcheck disable=SC2034 # read by the drills
		CUT_NOTE="counts non-loopback output and DNS (port 53) of the drill's group, forwarding out of docker bridges, and docker.service/containerd.service output: $([ $CUT_DOCKERD -eq 1 ] && echo measured || echo 'not measured here (nft lacks socket cgroupv2)')"
		;;
	Darwin)
		CUT_PF_TOKEN=$(sudo -n pfctl -E 2>&1 | awk '/Token/ {print $NF}')
		printf 'block drop out quick on ! lo0 proto { tcp udp } from any to any group %s\n' "$CUT_GID" | sudo -n pfctl -a "$CUT_PF_ANCHOR" -f - >/dev/null 2>"$CUT_WORK/pf.err" || { CUT_ERR=$(tr '\n' ' ' <"$CUT_WORK/pf.err" | cut -c1-300); cut_remove; return 1; }
		# shellcheck disable=SC2034 # read by the drills
		CUT_NOTE="counts TCP and UDP of the drill's group; DNS through mDNSResponder and ICMP are not measured"
		;;
	esac
	cut_watch
}

# cut_cleanup_cmd: the command (as root) that removes the cut; the watchdog and cut_remove both run it.
cut_cleanup_cmd() {
	case "$CUT_OS" in
	Linux) echo "nft delete table inet $CUT_TABLE" ;;
	Darwin) echo "pfctl -a $CUT_PF_ANCHOR -F all; [ -z '$CUT_PF_TOKEN' ] || pfctl -X '$CUT_PF_TOKEN'" ;;
	esac
}

# cut_watch: a detached root process that removes the cut when this shell's pid is gone, however it died (SIGKILL included).
cut_watch() {
	CUT_STOP=${CUT_WORK:?CUT_WORK is not set}/cut.stop
	rm -f "$CUT_STOP"
	sudo -n /usr/bin/perl -e 'use POSIX qw(setsid); my ($pid, $stop) = (shift, shift);
		defined(my $k = fork) or exit 1; exit 0 if $k; setsid();
		open STDIN, "</dev/null"; open STDOUT, ">/dev/null"; open STDERR, ">&STDOUT";
		sleep 1 while kill(0, $pid) && !-e $stop;
		system("/bin/sh", "-c", $ARGV[0]);' "$CUT_ID" "$CUT_STOP" "$(cut_cleanup_cmd)" >/dev/null 2>&1 || true
}

# cut_remove: take the drill's own rules out again (idempotent) and release the watchdog.
cut_remove() {
	[ "$CUT_ACTIVE" -eq 1 ] || return 0
	CUT_ACTIVE=0
	sudo -n /bin/sh -c "$(cut_cleanup_cmd)" >/dev/null 2>&1 || true
	[ -z "${CUT_STOP:-}" ] || : >"$CUT_STOP" 2>/dev/null || true
}

# cut_packets: the number of packets the rules have dropped so far, all counters summed (0 when unreadable).
cut_packets() {
	case "$CUT_OS" in
	Linux) sudo -n nft list table inet "$CUT_TABLE" 2>/dev/null | sed -n 's/.*counter packets \([0-9][0-9]*\).*/\1/p' | awk '{n += $1} END {print n + 0}' ;;
	Darwin) sudo -n pfctl -a "$CUT_PF_ANCHOR" -s rules -v 2>/dev/null | sed -n 's/.*Packets: \([0-9][0-9]*\).*/\1/p' | awk '{n += $1} END {print n + 0}' ;;
	esac
}

# cut_info: the counter text kept in the evidence (nft table with counters; pfctl -s info and the rule's statistics).
cut_info() {
	case "$CUT_OS" in
	Linux) sudo -n nft list table inet "$CUT_TABLE" 2>&1 | grep counter | sed -E 's/^[[:space:]]+//; s/ counter packets ([0-9]+) bytes [0-9]+ drop/ => \1/; s/meta skgid [0-9]+ //' | tr '\n' ';' ;;
	Darwin) { sudo -n pfctl -s info 2>&1 | grep -E 'Status|block|match' | tr '\n' ' '; sudo -n pfctl -a "$CUT_PF_ANCHOR" -s rules -v 2>&1 | grep -E 'Packets' | tr '\n' ' '; } | tr -s ' ' ;;
	esac
}

# cut_probe_ip / cut_probe_dns: try to reach a public address / resolve a public name from inside the cut. Succeed (0) when it
# FAILED, as it must. The DNS probe needs getent (Linux only).
cut_probe_ip() {
	if cut_exec /usr/bin/env "PATH=$PATH" curl --silent --output /dev/null --connect-timeout 4 --max-time 6 https://1.1.1.1/ 2>/dev/null; then return 1; fi
	return 0
}
cut_probe_dns() {
	command -v getent >/dev/null 2>&1 || return 0
	if cut_exec /usr/bin/env "PATH=$PATH" getent hosts example.org >/dev/null 2>&1; then return 1; fi
	return 0
}
