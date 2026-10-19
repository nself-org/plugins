# CI trust, security and privacy

This page covers the `container` isolation level of `nself ci`: what it blocks, what it does not, and what a node must provide. The sandboxed container level (gVisor) is described in its own section below. The other levels (process, VM) are described next to their own profiles.

## The container level

A job at this level runs in a Docker or Podman container that the isolation profile builds from one pure function. The executor refuses any command line that is not on its allowlist, whatever the profile says.

Every container gets:

- `--cap-drop=ALL`, `--security-opt no-new-privileges` and a read-only root filesystem, with a 256 MB `/tmp`.
- A numeric user, `65532 + slot`, never root and never the agent's own UID. Two concurrent attempts that share a home never share a slot.
- A pids, memory and CPU limit.
- Network `none` unless the job asks for more. `lan` is NAT on the default bridge today; it does not block the host gateway. The network planner (P7-TRUST-08) tightens that.
- One bind mount, the attempt workspace at `/repo`. A cache restore may add read-only binds. Read-write host mounts, runtime sockets, `/var/run`, `/run` and system directories are refused.
- The labels `nself.ci.attempt`, `role=ci-ephemeral` and `coordinator_id`, so an orphan sweep can find them.
- The job environment through the runtime's stdin env file. No value reaches an argv or a file on disk.

## What it does not block

- A kernel escape. A container shares the host kernel. For code you do not trust, use the sandboxed container level (gVisor) or a VM.
- Network egress on `lan` and `internet`.
- Disk use inside the workspace. The workspace is removed after the attempt. If the job image has no shell, the cleanup helper cannot remove job-owned files and the workspace can stay on disk (tracked as debt).

## The sandboxed container level (gVisor)

`sandboxed-container` is the container level run with gVisor's `runsc` runtime. The job's syscalls go to gVisor's Sentry, a user-space kernel, and only a filtered set reaches the host kernel. `nself ci --trust untrusted -- <cmd>` needs this level, with network `none`. Without it that command exits with `E647`; it never falls back to a plain container.

### Install

1. Install gVisor from the [install guide](https://gvisor.dev/docs/user_guide/install/) (check the download hash).
2. Run `sudo runsc install`, then restart the Docker daemon. Docker must list the runtime: `docker info --format '{{json .Runtimes}}'`.
3. Podman is not supported for this level yet: its runtimes live in `containers.conf` and give the profile no table to check.

A runsc binary or a runtime entry is not availability. The profile starts a small probe container under `runsc` (`docker run --runtime=runsc --network none` of a busybox image pinned by digest, running `dmesg`) and the level is available only when the output carries gVisor's banner (`Starting gVisor`). The runsc version and the platform are reported as facts. The probe image is in the image lock as `ci-gvisor-probe`, so the daily image probe covers it.

### Platform

The probe reads the runtime entry and records `platform`. `systrap` is the default; `ptrace` works where systrap does not. `kvm` is refused where `/dev/kvm` is absent, which is every supported node today. Any other value is refused too (`isolation.platform_unsupported`).

The level is unavailable (`E678`, reason `isolation.runtime_flag_denied`) when the runsc entry carries any of `--network=host`, `--host-uds`, `--host-fifo`, `--net-raw` or `--allow-flag-override`. These open the sandbox to the host; the profile refuses them rather than stripping them, because the daemon would still pass them to runsc.

On a separated node the agent user's own runtime must be rootless, and the probe starts runsc under it. If runsc does not run rootless there, the probe fails (`isolation.probe_failed`) and the level stays unavailable.

### What it does not give you

gVisor is not a VM.

- The host kernel is shared through the Sentry's seccomp-filtered syscall set. A bug in that set, or in the Sentry, is a way out.
- There is no protection against side channels (Spectre-class attacks, cache timing).
- runsc has CVEs of its own. Keep it current; the probe reports the version it found.
- Resource exhaustion beyond the cgroup limits is not stopped.

Some toolchains do not run under gVisor because they need a syscall the Sentry lacks. That is recorded as a failed job, not worked around: there is no fallback to a plain container for a sandboxed job.

### Handoff

- `ci doctor` does not yet print the install steps; the profile exposes them as the `hint` fact on an unavailable result (`gvisor.InstallHint`).
- Podman support needs a way to read and judge its runtime table.
- Network scopes `restricted` and `internet` need the scope planner of P7-TRUST-08, as for the container level.

## What a node must provide

- Docker or Podman with the pids, memory and CPU cgroup controllers delegated. The probe reads them from `info` and reports the level unavailable (`isolation.limits_unenforceable`) when one is missing, because the runtime would start the container and silently drop the limits. Rootless Docker needs cgroup v2 with systemd delegation. Rootless Podman needs the same.
- On a node where the agent user has no docker group (a separated node), only a rootless runtime counts. A rootful socket is root-equivalent. Set `NSELF_CI_NODE_SEPARATED=1` on such a node; unset means "not separated" and a rootful runtime is accepted.
- Rootless runtimes map about 65536 UIDs, so only slots 0 to 3 run at once.

When the level is unavailable the job fails with E678 and a stable reason. It never falls back to a weaker level.

## Workspace hygiene and job UID separation

Every attempt gets its own job root, a fresh `0700` directory under the CI home (`work/<attempt>`). When the attempt ends, `Cleanup` does four things, in this order, and each step runs even if the one before failed:

1. Kills the attempt's process group (found from its marker) and any process that still carries the attempt tag, including a child that called `setsid`.
2. Removes the containers, then the networks and volumes, that carry the label `nself.ci.attempt=<attempt>`. Resources without the label, and anything of another attempt, are never touched.
3. Removes the job root with no-follow, directory-relative calls. A symlink is deleted, never followed, so a link to `$HOME` leaves `$HOME` alone. A path that escapes the root, or a different device (mount point), is refused and logged.
4. Reports `E681` (`hygiene.cleanup_failed`) if anything could not be removed.

`LeakCheck` then looks for what survived: the root, processes, TCP listeners, files outside the root owned by the job UID, and labelled resources. A check that cannot run counts as a leak, never as clean. On a leak the run hook (or the agent) drains the node and records the reason. Without `/proc` (macOS) the process and socket scans are reported as skipped; the exec marker and descendant tracking do the kill there.

`SweepOrphans` runs once when the first attempt of a process starts. It removes the roots and labelled resources of attempts that no live lease owns. It spares attempts of the running process, attempts with a live process marker and roots younger than 12 hours, because a second `nself ci` on the same machine may still own them. It leaves an attempt alone while a container of that attempt is still running (the executor's own recovery removes the containers of dead coordinators), and it does not run at all while a container runtime is installed but not answering, because a sibling's live job could not be told from an orphan.

### Which user a job runs as

`Separation` picks the host UID for the attempt's slot. Each slot has its own UID, so two concurrent attempts never share a UID and cannot read each other's workspace or `/proc/<pid>/environ`.

| Mode | When | UID |
|---|---|---|
| `container-mapped` | any container or VM level | `65532 + slot`, skipping the agent's and coordinator's UID |
| `job-shim` | system install with the per-slot job users and the agent's `job-shim` sudo rule | the UID of `nself-ci-job-<slot>` |
| `userns` | user-mode Linux where `ProbeUserns` shows an unprivileged user namespace cannot read a `0600` file the agent owns | the agent's subordinate UID plus the slot |
| `owner-exception` | no separation exists, and the job is `owner` trust, process level, from your own PERSONAL coordinator, on your own machine or non-separated node | your own UID |
| refused (`E682`) | everything else | none |

The one exception is deliberate and narrow. Evidence from such a job is `coordinator-recorded`, never `signed-remote`. A TEAM coordinator never leases to a node that has no separation. A node with fewer job users than slots runs process-level attempts one at a time (`isolation.slot_uid_shared`).

To enable `userns` on user-mode Linux, install `uidmap` (for `newuidmap`), give the agent user a range in `/etc/subuid` and `/etc/subgid`, and allow unprivileged user namespaces. Ubuntu 24.04 restricts them by default (`kernel.apparmor_restrict_unprivileged_userns=1` leaves the namespace without capabilities, so the job cannot switch to the mapped UID): the probe then reports `userns.unavailable` and the node stays non-separated until a profile or that sysctl allows it. The job runs as `unshare --user --map-users=0:<uid>:1 --map-groups=0:<uid>:1 --setuid=0 --setgid=0 -- <command>`; the probe checks that exact command. Hygiene creates no OS users and no sudoers entries; `nself ci nodes add --system` does that once.

### Known limits

- Leftover processes of a job that ran under your own UID (the local run loop and the owner exception) are found by the `NSELF_CI_EXEC_ATTEMPT` tag in their environment. A job that strips the tag, or a process that makes its environment unreadable, can evade the scan. Separated attempts are found by their job UID instead. macOS has no `/proc`, so the process and socket scans are skipped there.
- A job user's `cron` and `at` entries, SysV IPC objects and home directory are not cleaned by hygiene. `nself ci nodes add --system` is where those belong.
- The agent cannot kill or remove files that a job user owns. A separated node passes hygiene a `JobRunner`, which its job shim implements, and the kill and the removal then run as the job user. Without one, cleanup reports `E681` and the node drains rather than claiming success.
- Between a check and the action it guards (the process group kill, a volume removal, the mount snapshot) there is a few-millisecond window. Closing it needs a PID namespace or cgroup per job.
