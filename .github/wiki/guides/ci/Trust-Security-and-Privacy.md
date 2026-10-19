# CI trust, security and privacy

This page covers the `container` isolation level of `nself ci`: what it blocks, what it does not, and what a node must provide. The other levels (process, sandboxed container, VM) are described next to their own profiles.

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

## Secrets

A job asks for secrets by reference, never by value: `secrets: [project/NPM_TOKEN, release/ANDROID_KEYSTORE_PASSWORD, environment/staging/API_KEY, deploy/prod/DEPLOY_KEY, team/SHARED_TOKEN]`. The part after the last slash is the environment variable the job sees, and two refs may not use the same variable (`E674`). Names that would change how a process or the docker client runs (`PATH`, `HOME`, `DOCKER_*`, `LD_*`, `DYLD_*`, `GIT_*`, `NSELF_CI_*` and similar) are refused too.

| Class | Store file (`.secrets/`) | Who may get it |
|---|---|---|
| `project` | `ci.age` | trusted jobs on an eligible runner |
| `environment/<env>` | `ci-<env>.age` | trusted jobs on an eligible runner |
| `release` | `release.age` | `internal` or `owner` jobs on a dedicated-trusted node or a VM |
| `deploy/<env>` | `<env>.age` | same as `release` |
| `team` | the node's own team store | trusted jobs on an eligible runner; node-local, never granted with `nodes secrets grant` |

How it works:

- Refs are checked first: trust class of the job, the runner's facts, the runner's declared secret classes and the policy's `trust.secrets` allowlist. A refusal (`E677`) names the reasons and makes no resolver call. Untrusted jobs never get a secret.
- Values come from one place: `nself config secrets resolve`, run on the node that executes the job. A TEAM coordinator passes references only and never holds a value. Only a PERSONAL coordinator over the SSH carrier to your own node may send a `project` or `environment` value.
- On an agent node the resolver runs with `SECRETS_AGE_KEY_PATH` set to that node's own age identity, so the node decrypts only the files you granted to it: `nself ci nodes secrets grant <node> --class project` (also `environment/<env>`, `release`, `deploy/<env>`). `release` and `deploy` files go only to a dedicated-trusted node. A node whose registry entry has `secrets_eligible: false` (jobs could read its age key) is refused with `E677`. `revoke` always works; it does not unsee plaintext the node already read, so rotate the value afterwards.
- Delivery is by environment only: the process environment, `docker run -e NAME` with the value in the docker CLI's own environment (never in argv), or `export` lines on the SSH stdin of a Tart VM. No file, URL, cache key, artifact or log ever holds a value.
- Every value is registered with the output redactor before the job starts. If the installed redactor is the pass-through default, or a value cannot be registered (shorter than 6 bytes), nothing is injected and the attempt fails (`E676`) before it starts. Buffers are zeroed when the attempt ends, whether it passed, failed, was cancelled or lost.
- No `nself config secrets resolve` in the installed CLI: the job fails before it starts with `E675`.

Limits: only the byte buffers the Source owns are zeroed. Copies in Go strings (the job environment slice handed to the process, the redaction registry, the legacy-environment read) stay in the nself-ci process until it exits, and a failed availability probe is remembered until it restarts. Values live in process memory and the job's environment for the attempt. For container jobs dockerd also keeps `-e` values in the container configuration until the container is removed (`--rm` and the hygiene sweep remove it). `/proc/<pid>/environ` is readable only by that attempt's own job user.

Android release builds: until `release.age` holds the keystore secrets, `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS` and `ANDROID_KEY_PASSWORD` in your own environment still work, on your own machine only. One audit row (`secret.legacy_env`, value-free) is written first; if it cannot be written the legacy value is not used. The `legacy-env` label is kept on the Source and is not yet copied into run evidence. A remote node never uses this path.

A node granted a `release` or `deploy` file must be dedicated-trusted (operator-owned, assigned trust, accepts only `owner`, one authorized project); `nodes secrets grant` does not yet check that the project is the one the file belongs to, so grant these files only to a node dedicated to that project. Multi-line values (PEM keys) need the baseline container path today; the hardened path refuses them.
