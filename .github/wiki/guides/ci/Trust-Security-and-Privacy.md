# CI trust, security and privacy

This page covers the `container` isolation level of `nself ci`: what it blocks, what it does not, and what a node must provide. The sandboxed container level (gVisor) is described in its own section below. The other levels (process, VM) are described next to their own profiles.

## The container level

A job at this level runs in a Docker or Podman container that the isolation profile builds from one pure function. The executor refuses any command line that is not on its allowlist, whatever the profile says.

Every container gets:

- `--cap-drop=ALL`, `--security-opt no-new-privileges` and a read-only root filesystem, with a 256 MB `/tmp`.
- A numeric user, `65532 + slot`, never root and never the agent's own UID. On Linux, kernel leases keep slots distinct across coordinators, CI homes and OS users in the host network namespace. Leases disappear when the holder exits. Coordinators using one rootful Docker daemon must share that namespace; other platforms retain home-scoped locks.
- A pids, memory and CPU limit.
- Network `none` unless the job asks for more. `restricted` and `internet` go through a per-attempt proxy (see Network scopes). `lan` is NAT on the default bridge; it does not block the host gateway, which is why an untrusted job can never have it.
- One bind mount, the attempt workspace at `/repo`. A cache restore may add read-only binds. Read-write host mounts, runtime sockets, `/var/run`, `/run` and system directories are refused.
- The labels `nself.ci.attempt`, `role=ci-ephemeral` and `coordinator_id`, so an orphan sweep can find them.
- The job environment through the runtime's stdin env file. No value reaches an argv or a file on disk.

## What it does not block

- A kernel escape. A container shares the host kernel. For code you do not trust, use the sandboxed container level (gVisor) or a VM.
- Network egress on `lan`, and anything a job does with the hosts the proxy allows under `restricted` and `internet`.
- Disk use inside the workspace. The workspace is removed after the attempt. Files the job user created are removed by a cleanup helper that runs in a separate busybox image pinned by digest (image lock entry `ci-cleanup`), never in the job image, so an image without a shell cannot leave them behind. If the helper image cannot start (an air-gapped node that never pulled `ci-cleanup`), the cleanup is tried once in the job image, as before; if that fails too, the job log says so and the host removes what it owns; a workspace it cannot empty fails a passing run as an infra failure (`E610`).

## The sandboxed container level (gVisor)

`sandboxed-container` is the container level run with gVisor's `runsc` runtime. The job's syscalls go to gVisor's Sentry, a user-space kernel, and only a filtered set reaches the host kernel. `nself ci --trust untrusted -- <cmd>` needs this level, with network `none`. Without it that command exits with `E647`; it never falls back to a plain container.

### Install

1. Install gVisor from the [install guide](https://gvisor.dev/docs/user_guide/install/) (check the download hash).
2. Run `sudo runsc install`, then restart the Docker daemon. Docker must list the runtime: `docker info --format '{{json .Runtimes}}'`.
3. Podman is not supported for this level yet: its runtimes live in `containers.conf` and give the profile no table to check.

A runsc binary or a runtime entry is not availability. The profile starts a small probe container under `runsc` (`docker run --runtime=runsc --network none` of a busybox image pinned by digest, running `dmesg`) and the level is available only when the output has gVisor's banner as a whole line (`Starting gVisor...`, optionally behind a `[ time ]` stamp). The runtime entry must also point at a binary named `runsc`. The runsc version and the platform are reported as facts. The probe image is in the image lock as `ci-gvisor-probe`, so the daily image probe covers it.

### Platform

The probe reads the runtime entry and records `platform`. `systrap` is the default; `ptrace` is deprecated upstream and may be removed; use it only where systrap does not run on the host kernel. `kvm` is refused where `/dev/kvm` is absent. Any other value is refused too (`isolation.platform_unsupported`).

The runtime argument allow list accepts only `--platform`, `--debug-log` and `--strace` (including single-dash spellings). String values can follow `=` or occupy the next argument; the boolean `strace` accepts a bare flag or `=true`/`=false`. Unknown flags, positional arguments and malformed values make the level unavailable (`E678`, reason `isolation.runtime_flag_denied`). Host-access flags and flags that drop resource limits remain refused, even with a false value. The profile refuses arguments rather than stripping them, because the daemon would still pass them to runsc.

The entry is judged only as far as this profile can read it: an entry with a key other than `path`, `runtimeArgs` and `status` is refused, and so is an entry with no `path`. A containerd shim runtime (`runtimeType`, `options`) keeps its settings in a file the profile does not read, and `docker info` does not show those keys, so such a runtime appears as an entry without a path. `runsc install` always writes a path. The runtime table is read on every check, so a daemon reload that adds a denied argument is seen on the next check. Only the real start is remembered (for a minute, per runtime entry), and concurrent checks share one start. A change made between a check and the `docker run` is not seen; changing the daemon configuration needs root, which already owns the host.

The probe asks the local daemon: a Docker context that points at a remote host is not supported.

On a separated node the agent user's own runtime must be rootless, and the probe starts runsc under it. Rootless Docker usually needs `--ignore-cgroups` for runsc, which is refused, so the level is normally unavailable there (`isolation.runtime_flag_denied`, or `isolation.probe_failed` if the start fails).

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
- Network scopes `restricted` and `internet` use the same scope planner as the container level; the sidecar and the job network are created by the container profile, and the sandboxed job runs on that network under `runsc`.

## The macOS VM level (Tart)

On an Apple-silicon Mac the `vm` and `ephemeral-vm` levels run each attempt in a fresh clone of a VM image, through [Tart](https://tart.run). Linux has no `vm` level here. You install Tart and its network filter (softnet) yourself: nSelf does not bundle, download or redistribute either, and `ci doctor` shows the licence note that goes with them. The levels are the same profile; every attempt gets its own clone, so `ephemeral-vm` adds nothing a `vm` job lacks.

### Set up

1. Install Tart and softnet (for example `brew install cirruslabs/cli/tart`).
2. Build or pull a base image your jobs need, with Remote Login (ssh) on and a key you control in the image user's `authorized_keys`.
3. Tell nSelf in `~/.nself/ci/config.yaml`:

```yaml
ci:
  tart:
    base_image: ghcr.io/you/macos-ci@sha256:<64 hex digits>   # the digest is required
    ssh_user: admin                                            # default
    ssh_key: /Users/you/.ssh/tart_ci                           # identity file the image trusts
```

No image is shipped and a tag without a digest is refused: the level reports `isolation.tart_base_image` (E678) until the setting is right.

### What a job gets

- A clone named `nself-ci-<attempt>`. The workspace is copied in over the ssh session (a tar stream), the job runs there, and only the artifact paths the job declares come back. Nothing from the host is mounted into the VM.
- Secrets are written as `export` lines on the ssh session's stdin. They are never on the `tart` or `ssh` command line and never in a file.
- The clone is stopped and deleted on every exit path: pass, failure, cancel and timeout. A clone that cannot be deleted keeps a marker, and the next start retries.
- Orphans: each clone has a marker file under `<CI home>/tart/` that the running attempt keeps locked. If the coordinator dies (even `kill -9`), the lock goes with it, and the sweep at the next attempt start stops and deletes the clone. A VM you named `nself-ci-*` by hand has no marker and is never touched.
- Apple allows two macOS VMs at once. The profile counts free slots (`free_slots` in `ci doctor`) and never starts a third; the job waits.

### Network scopes on Tart

| Scope | How it is enforced |
|---|---|
| `lan`, `privileged` | Tart's default NAT. Owner and internal work only; netscope refuses these for untrusted jobs. |
| `none`, `restricted`, `internet` | softnet with default deny: `--net-softnet-block` lists `0.0.0.0/0` plus the deploy exclusion set, the host's on-link prefixes and its addresses, and `--net-softnet-allow` lists only the Tart gateway address (the ssh session and the proxy live there). |

`restricted` and `internet` go through the same egress proxy as the container level (public hosts only, the deploy exclusion set refused), served by the coordinator on the gateway address, and the ssh session sets `HTTP_PROXY` and `HTTPS_PROXY`. Before the job starts, a check runs inside the VM: the proxy must answer, the gateway must answer, and the public internet, the Mac's own LAN addresses and the exclusion set must not. The check also fails closed on: a guest that is not macOS, no answer from the gateway (nothing then proves the controls work), the public internet reachable by TCP (IPv4 and IPv6) or ping, any of a fixed list of host service ports open at the gateway address (22, 53, 80, 111, 443, 445, 548, 631, 3283, 5900, 8080), and, under `none` and `restricted`, public names resolving through the VM's resolver. A scope that cannot be proven fails the attempt with `E679` (`network.scope_unenforceable`), and the clone is deleted. The proxy serves only the attempt's own VM address, so a second VM on the same gateway cannot use it. If the installed Tart has no softnet block option, or no proxy can be started, every scope below `lan` is unavailable.

### Known limits

- Real-run evidence needs an Apple-silicon Mac with Tart; GitHub-hosted macOS runners do not allow nested virtualisation.
- softnet filters by destination address, not port. The gateway address is allowed, so anything on your Mac that listens on all interfaces (ssh, file sharing, screen sharing) or on the gateway, and the resolver the VM is given, are reachable from the VM on that address. The check above refuses the attempt when it finds one of the listed ports open or a public name resolving; turn those services off on a Mac that runs untrusted jobs. If the vmnet resolver cannot be closed, `none` and `restricted` stay unavailable on that Mac and `internet` and `lan` still work.
- The host-side `ssh` is `/usr/bin/ssh` (SIP strips `DYLD_*` from it), and it never receives the job's environment: env and secrets travel in the stdin script only. Resolved secret values sit in its process environment under the fixed names `NSELF_CI_VMSECRET_<n>` so the redactor still knows them.
- Host keys of an ephemeral clone are not pinned: the session is `BatchMode` with no forwarding and no agent, on a private NAT.
- IPv6 is not filtered by softnet rules; the VM image should not route it.
- A job that declares secrets needs a per-attempt secret source in its run; a run without one is refused rather than started without its secrets.
- Declared artifact paths are literal paths (no globs). Symlinks and special files in them are refused, and a collect is capped at 1 GiB, 100000 entries and 64 levels.
- The Tart gateway is `192.168.64.1` unless the Mac's vmnet network was changed; a different address makes the network check fail closed.

### Handoff

- `ci doctor` does not print the Tart facts yet; the profile exposes `tart_version`, `licence_note`, `free_slots`, `softnet_block` and `base_image_cached` as availability facts.
- The tagged integration tests (`-tags tart`) are the real-run evidence; they run on a Mac with Tart and the environment variables listed at the top of `integration_test.go`.

## Network scopes

A job runs under one scope. An undeclared scope takes the default of its trust class: `owner` and `internal` get `lan`, `collaborator` gets `internet`, `untrusted` gets `restricted`. An untrusted job that declares `lan` or `privileged` is refused (`E679`), before any resource exists.

| Scope | What the job can reach |
|---|---|
| `none` | Nothing. The container has no interface but `lo`. |
| `restricted` | Only an allowlist of package registries, through the proxy: `proxy.golang.org`, `sum.golang.org`, `registry.npmjs.org`, `registry.yarnpkg.com`, `crates.io`, `static.crates.io`, `index.crates.io`, `pypi.org`, `files.pythonhosted.org`, plus the hosts the policy key `trust.providers` names. |
| `internet` | Any public host, through the proxy. |

`restricted` serves ports 443 and 80 only; `internet` serves any port of a public host.
| `lan` | The default bridge (owner and internal jobs only). |
| `privileged` | Raw access (owner and internal jobs only). |

`restricted` and `internet` need no root and no firewall rule. For each attempt the container profile creates a Docker or Podman network with `--internal` (no route out) and `com.docker.network.bridge.inhibit_ipv4=true` (no address for the host on that bridge), with IPv6 off. A second container, the proxy sidecar, is joined to that network and to the runtime's normal bridge. It runs the static `nself-ci` binary, mounted read-only into a minimal image pinned by digest (`ci-netproxy` in `images.json`), as `nself-ci netproxy`. The job gets `HTTP_PROXY`, `HTTPS_PROXY` and `ALL_PROXY` (upper and lower case) that point at the sidecar, and `--dns 127.0.0.1`, so the job resolves no name. The proxy lives in neither the agent nor the coordinator.

The proxy takes `CONNECT` and absolute-form HTTP requests. For each request it resolves the name once, unmaps every address (an IPv4-mapped address becomes IPv4), checks all answers against the deny set, and dials the address it checked. The deny set is:

- the special-purpose IPv4 and IPv6 ranges, which include the NAT64, local-use NAT64, 6to4 and Teredo forms of an IPv4 address, so `::ffff:10.0.0.1`, `64:ff9b::a00:1` and `2002:a00:1::1` are refused like `10.0.0.1`;
- the host's own addresses and every prefix configured on its interfaces or in its routes, IPv4 and IPv6, global unicast prefixes included (a `/64` LAN of public addresses is a LAN);
- the deploy exclusion set of the host (the cached entries of its `state.db` and the never-list in `config.yaml`, read with `deployguard.LocalSet`), by address, prefix and name. Each excluded name is also resolved once when the scope is set up and its addresses join the set, so dialling the address, or another name for it, is refused too. A set that exists and cannot be read, or an entry that does not parse, makes the scope unavailable (`E679`) instead of running with less.

No environment variable loosens the set (`NSELF_ALLOW_PRIVATE_URLS` of the SDK guard has no counterpart). Each refusal is one structured event (`network.denied`, with the scope, host, port and reason) that the planner reads back when the attempt ends. The proxy never logs a path, header or body, its refusal answer names no reason, and a sidecar records at most 1000 events (the last one says so). The planner also records an event when the sidecar log could not be read back or the attempt's resources could not be removed.

A tool that ignores proxy variables fails under `restricted` and `internet`. That is intended: there is no silent bypass.

### What a runner proves

A scope is offered only where it is enforced. The probe builds the same network and sidecar a job gets, then runs one container on it that tries to connect to the host's own addresses (on a port where the probe listens), to `169.254.169.254` and to a raw public address. If any connection succeeds, or the listener sees one, `restricted` and `internet` are withheld. The result is the capability fact `network.enforced_scopes`; the scheduler derives `NoLANRoute` from it, so a runner cannot declare it. Where the network or the sidecar cannot be created the scope is unavailable (`E679`), never approximated. The sidecar runs the host's own binary in a Linux container, so `restricted` and `internet` are Linux-host features (Docker or Podman, rootless included); on macOS they report unavailable.

### Known limits

- The exclusion set is the one this host can build for itself. A team coordinator that keeps the set in its own store can replace it with `run.RegisterNetscopeExclusions`; nothing registers one yet.
- The scheduler reads the proof for the local runner. A remote node's `network.enforced_scopes` is not reported yet, so no untrusted job is placed on a remote node on the strength of it. A remote job's declared scope is checked against the class it was placed at three times: by the scheduler's admission rule, by the coordinator before it delivers the lease, and by the agent before it runs it. An untrusted job that declares `lan` or `privileged` fails there with E679 and never reaches the executor. A job that declares no scope runs as `none`, locally and on a node; the class default is not applied.
- The proxy sees the destination host and port, not the encrypted content. It does not check the TLS server name, so a shared CDN host on the allowlist can be used for traffic to another site behind it, and uploading to an allowlisted registry is still an exfiltration path. `restricted` limits where a job can talk, not what it says.
- Linux route discovery reads all routing tables through netlink, including policy routes. A public address that hairpins to a LAN service through the router cannot be learned from those tables. It still needs an observed egress address from the target network in the never-list; automatic public-NAT discovery is not implemented.
- The sidecar bind-mounts a private staged copy of this process's binary with mode `0755`; the source binary's permissions are unchanged. The copy is removed with the attempt's network resources. Restricted and internet scopes still require Linux.

## What a node must provide

- Docker or Podman with the pids, memory and CPU cgroup controllers delegated. The probe reads them from `info` and reports the level unavailable (`isolation.limits_unenforceable`) when one is missing, because the runtime would start the container and silently drop the limits. Rootless Docker needs cgroup v2 with systemd delegation. Rootless Podman needs the same.
- On a node where the agent user has no docker group (a separated node), only a rootless runtime counts. A rootful socket is root-equivalent. Set `NSELF_CI_NODE_SEPARATED=1` on such a node; unset means "not separated" and a rootful runtime is accepted.
- Rootless runtimes map about 65536 UIDs, so only slots 0 to 3 run at once.

When the level is unavailable the job waits before it fails, and never falls back to a weaker level. A slot the runtime cannot map (`isolation.slot_uid_unmapped`, for example a fifth concurrent attempt on a rootless runtime) is waited out until another attempt frees one, for up to 30 minutes (slot 0 is never waited for: no other attempt can free a better one). A missing runtime, a failed probe or limits the runtime cannot enforce are retried for about a minute. The job log says `waiting for ... isolation` and repeats it every minute. Only a refusal before the job started is waited for. After the wait the job fails with E678 and its stable reason. Other reasons (a rootful runtime, an unenforceable network scope) fail at once.

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

## When each level was last verified

A level that worked once may not work today: a runtime update or a kernel change can silently end it. So `nself ci doctor` shows, for each level, the date a passing check last proved it on this machine.

```
nself ci doctor            # lines isolation.container ... isolation.vm
nself ci doctor --json     # each of those checks has "verified_at": an RFC 3339 time, or null
```

| Level | Reads | How it is proven |
|---|---|---|
| `isolation.container` | `verified <date> on <host class>` or `unverified since never` | the container integration tests |
| `isolation.sandboxed-container` | same | the gVisor integration tests (needs `runsc`) |
| `isolation.network` | same | the network scope integration tests |
| `isolation.hygiene` | same | the workspace hygiene integration tests (Linux) |
| `isolation.vm` | same on an Apple-silicon Mac; on Linux `unavailable (no KVM host, D-0120)` | the Tart tests (needs Tart and its test image) |
| `isolation.ephemeral-vm` | Linux only: `unavailable (no KVM host, D-0120)` | not provable on Linux |

The host class is the platform of the machine that ran the check, for example `linux/amd64`.

**Where the date comes from.** The `isolation-levels` drill (`scripts/drill/isolation-levels/run`, job `drill-isolation-levels` of the `drills` pipeline) runs the integration tests of every level the host can run and records one check per level. Doctor reads the stored evidence of those runs and reports the newest passing check of each level. Doctor never writes. A level with no passing check reads `unverified since never`; that is the honest state of a machine that never ran the drill, so it is not a warning.

**Run it.**

```
nself ci --pipeline drills --job drill-isolation-levels
scripts/drill/isolation-levels/run          # same, started directly; NSELF_CI_BIN and NSELF_CI_HOME choose the binary and the store
```

A level whose prerequisite the host lacks (no running Docker or Podman, no `runsc`, no Tart) ends `error` with `reason: prerequisite.missing` in its excerpt. That is never a pass and never changes the last-verified date. A level that does not exist on the host class (for example `vm` on Linux) gets no check. `ISOLATION_DRILL_LEVELS=container,network` narrows the run to those levels. The drill runs real containers; give its job more than the default 10 minutes with a `drill-isolation-levels` job in `.nself/ci.yaml` if the host is slow. `scripts/drill/isolation-levels/selftest.sh` checks the drill script itself against fake tools.

### Handoff

- Stored evidence keeps each nested check only on the drill check's excerpt: the last line, `passed: <ids>`, lists the nested checks that passed (internal/drill writes it, `store.LastVerified` reads it). A format change must change both; a test pins them together.
- The linux `ci-isolation-linux.yml` pull-request job runs the drill against a throwaway store and fails unless doctor shows a non-null `verified_at` for `isolation.container`, `isolation.sandboxed-container` and `isolation.network`.

## Whether each level can run now

`nself ci doctor` also probes every isolation profile and prints one `isolation.<level>.availability` line per level (`isolation.container.availability`, `isolation.sandboxed-container.availability`, `isolation.vm.availability`, `isolation.ephemeral-vm.availability`). Each reads `available (<level>)` or `unavailable: <reason>`, such as `isolation.runtime_missing`, `isolation.tart_base_image` or `isolation.tart_no_free_slot`.

For an unavailable level the line carries the fix as its `remediation` (the install steps: Tart from `brew install cirruslabs/cli/tart`, gVisor from its install page then `sudo runsc install`) and prints the profile facts under it. For Tart the facts are `tart_version`, `base_image`, `base_image_cached`, `free_slots`, `running_vms`, `softnet_block` and `licence_note`: Tart and softnet are third-party tools under their own licences, you install them, nSelf neither bundles nor redistributes them. An unavailable level is not a failure: these lines always pass, because a host without Tart or gVisor is a normal state.

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

## Source trust: who made the code

Every run has a source trust class: `owner`, `internal`, `collaborator` or `untrusted`. It decides what the run may touch (secrets, network, isolation) and it is never taken from a payload field, a job input or `.nself/ci.yaml`. A coordinator that runs jobs as you only starts `owner` and `internal` work; anything below needs an isolated runner.

**A forge trigger** (a webhook or a poll) is classified from facts the forge API gave: `trust = min(actor permission, revision provenance)`. The delivery's own claims can lower the class and never raise it. `ClassifyForge` re-fetches the facts from the API first; if they cannot be re-fetched (the call failed, or no registered provider can) the result is `untrusted` with the reason `source.refetch_failed`.

| Fact | Result |
|---|---|
| Auth is `shared`, `bearer` or `none` | `untrusted` |
| Fork or other head repository, or no proof the head is this repository | `untrusted` |
| Actor has `admin` on the repository | up to `owner`; `write` or `maintain`: up to `collaborator`; anything else or unknown: `untrusted` |
| Push to a protected ref, or a sha reachable from a protected branch | `owner` is possible; any other push or a pull request: `collaborator` at most |
| Tag, API or manual trigger | `owner` only when the sha is reachable from a protected branch or tag rule; otherwise `untrusted` |
| Schedule | needs a protected ref or a reachable sha, else `untrusted` |
| `[bot]` login, Dependabot, Renovate, or a diff of lockfiles only | `untrusted` |
| A runner-demand job | `untrusted` unless the group takes protected refs only, the event is `push`, `schedule` or `workflow_dispatch`, and the head is this repository |
| Head moved since the delivery | `untrusted` |

A missing fact takes its untrusted value. A first-time contributor's run needs an approval. The repository's visibility must come from the API as `public` before a run may use hosted infrastructure; unknown or changed visibility keeps the `private-infrastructure` zone.

**A local run** is classified from git, by provenance and never by author name or email (anyone can write your email into a commit). Git runs with `--no-replace-objects`.

- `owner`: HEAD is on the `origin` default branch; or the reflogs show that every commit that branch does not contain was made here. The oldest reflog entry of a commit decides: `commit`, `commit (amend)`, a rebase step that rewrites a commit (also under `pull --rebase`), a merge commit, `cherry-pick` and `revert` count; `checkout`, `fetch`, `pull` that fast-forwards, `branch: Created from`, `rebase (finish)` and anything that fast-forwards do not. Pushing your own commits changes nothing: you still made them. A stranger's commit you fetched or checked out has an arrival entry, and a branch you fetched back from a peer keeps your `commit` entries.
- `collaborator`: HEAD is on another branch of `origin`. Pull-request namespaces (`refs/remotes/origin/pr/*`, `pull/*`, `mr/*` and any refspec that maps something other than `refs/heads/*`) do not count as branches.
- `untrusted`: anything else, including a stranger's pull request checked out with `gh pr checkout`, an unfinished merge, squash, cherry-pick, revert or rebase (the tree holds content no commit accounts for), and any case where git could not answer.
- A repository with no remote, a directory that is not a git repository, and a repository without a commit are your own directory: `owner`. A remote that is configured but never fetched is judged by the reflog like a branch with no default.

Limits, so you can judge them: the reflog expires and is read for the last 5000 entries, so an old branch (or more than 500 commits beyond the default branch) may fall to `untrusted`; run `git remote set-head origin -a` so the default branch is known when it is not `main` or `master`. A commit you rebase, amend or cherry-pick is a new commit made here, even when its content came from someone else's branch, so rebasing a stranger's pull request before you test it makes it `owner` (patch-id matching against remote commits is the planned fix, see the debt register). Uncommitted changes count as yours. A repository you extracted from an archive, or cloned from a stranger, is whatever its `origin` says it is. Run `nself ci --trust owner` when you know better, and use an isolated runner for code you did not write.

**`--trust owner`** forces `owner` and writes one audit row (`trust.override`) that names the revision, the verdict it replaced and its reasons. If the row cannot be written the run is refused (`E625`). `--trust` with a lower class always works; it never raises a class except to `owner`.

**Agents.** When `NSELF_INVOKED_BY` is set (the MCP and HTTP invokers set it; export it for a CLI agent, which is advisory because the agent controls its own environment), the class is capped by the operator policy key `trust.machine_invoked_cap` (default `owner`, no cap), with the reason `source.machine_invoked_cap`. `--trust` above the cap fails with `E687` (reason `machine_cap`). Set `untrusted` for unattended agents.

Handoff for later Tickets: `source.Classify(model.TriggerFacts) Result` and `source.ClassifyLocal(source.GitFacts) Result` are pure (`internal/trust/source`). `run.SourceTrust` is the registered `TrustClassifier`; `Decide` returns the reasons, `ClassifyForge(ctx, root, facts)` classifies a forge trigger (P7-TRUST-17 feeds it recorded deliveries; P7-TRUST-24 reads `RequiresApproval`). The `Kind`, `Auth` and `Repo` fields of the facts carry no source label: the adapters must derive them from the authenticated delivery, not from its body. `source.ApplyCap` is the one place the machine cap lowers a class.

### Run approvals

A run that needs approval waits in the `approval` admission step with the placement reason `approval.required` until a valid approval exists. A run needs approval when the source facts recorded for it say so, or, with no facts recorded, when its source trust is `untrusted` (a collaborator's pushed commit is not held; a stranger's is).

```
nself ci runs pending [--json]                    # held runs with reason keys
nself ci runs approve <run> [--sha <sha>] [--spend] [--json]
```

- The approver must hold `release` or `admin` and must not be the author of the run. The author's own approval is `E688` (`approval.self`); an unknown author is `E688` (`approval.author_unknown`); a member is `E687`. On a PERSONAL coordinator the local operator is exempt from the author rule, as for every `run.approve` decision. On a TEAM coordinator the operator may not approve a run it authored (`E688`, `approval.self`); a run with no recorded author does not stop the operator.
- An approval binds the run, its head SHA, its config digest and its head repo, as stored when the approval is recorded. Name the run by its exact id: a prefix or `latest` needs `--sha <head>` too, because it can resolve to a run someone started a moment ago. `--sha` refuses the approval unless the run is still at that head (`approval.bad_binding`), before any spend is approved.
- Any change of head SHA, config digest or head repo voids the approval and returns the run to pending; a re-run of the same SHA and digest keeps it. Validity is computed from the stored rows at every admission, so a void that has not been written yet still holds the run. `ci runs pending` writes the voids, one audit row each (`approval.void`); every approval writes one row (`approval.grant`) besides the `authorize` decision row.
- `--spend` needs `admin` and calls the spend approval once before the run approval is recorded; if it is refused, nothing is recorded. On a run that needs no trust approval, `--spend` records the spend approval only.
- A void stands: if the binding changes and later matches again, the approval stays void and the run waits for a fresh approval. When a stale approval is replaced by a new one, its void row is written first.
- The admission step reads only stored run facts inside the admission transaction (no network), so a push that lands between approval and lease returns the run to pending instead of leasing a SHA nobody approved.

Handoff: `source.Binding`, `source.VoidReasons` and `source.RequiresApproval` are the pure rules (`internal/trust/source/approval.go`); the rows live in `internal/store/trust_approvals.go` (`trust_run_facts`, `trust_approvals`). The source hook records the facts with `Store.RecordApprovalFacts` (not yet wired: see the Epic Invalidations).

## The production boundary

CI work stays off the machines that run production, and off a coordinator host that is not isolated enough. Two rules apply, and every refusal carries error code `E689` with the reason `boundary.deploy_host`, `boundary.override_minimum` or `boundary.coordinator_host`.

| Where the job would run | Rule | Reason when refused |
|---|---|---|
| A node whose host is in a project's deploy inventory | Takes no CI work at all | `boundary.deploy_host` |
| The same node, added with `nself ci nodes add --allow-deploy-host` | Takes only `owner` or `internal` jobs, at isolation `vm` or stronger, with no release jobs and no `release` or `deploy` secret class | `boundary.override_minimum` |
| The host of a TEAM coordinator | Needs isolation `vm` or stronger for every job | `boundary.coordinator_host` |
| The host of a PERSONAL coordinator | Needs isolation `vm` or stronger for `collaborator` and `untrusted` jobs; the operator's own `owner` and `internal` jobs keep their normal minimums | `boundary.coordinator_host` |

Details:

- The override needs an interactive terminal and the node id typed back. Each placement on an overridden deploy host writes a `placement.deploy_host_override` audit row, and the job itself is raised to `vm` there, not only the node's declared level.
- The deploy-host override refuses release jobs even when they request no secrets (for example, SBOM generation), as well as jobs requesting `release` or `deploy` secrets. A release job must run on a node outside the deploy inventory.
- Matching is done from the deploy inventory on the coordinator, never from what a node says about itself.
- `E689` fix: run the job on a node that is not a deploy host, or on one you overrode with owner or internal work at `vm` or stronger and no release jobs or release or deploy secrets. On a coordinator host, run collaborator and untrusted work elsewhere, or move the coordinator to a VM.

## Roles, tokens and the audit log

A TEAM coordinator knows three roles, ordered `member < release < admin`, and no per-user permissions. In PERSONAL mode the operator on the coordinator host is allowed everything (access to the data directory is admin access) and every decision is still audited.

| Role | May |
|---|---|
| `member` | view logs and artifacts; list roles and tokens |
| `release` | everything `member` may, view the audit log, approve runs it did not author, start release jobs on projects in its allowlist |
| `admin` | everything else: register and revoke runners, set node trust fields, policy, privacy zones, provider and secret authorisation, approve spend, manage roles and tokens, seal the audit log |

A grant or token names the projects it covers (`--project <id>` repeated, or `--project '*'`). An actor scoped to named projects cannot do a project-wide `release` or `admin` action. Every refusal is `E687`; the reason key says why (`role`, `project`, `no_grant`, `node_kind`, `bad_certificate`, `token_unknown`, `token_expired`, `token_revoked`, `self_approval`, `author_unknown`, `unknown_action`, `unknown_role`, `bad_request`, `audit_secret`).

### Identities

- **User certificate** from `pki.IssueUser`: `ActorFromPeer` maps it to the principal's live grant. An actor is not a snapshot: every decision re-reads the grant, the token and the certificate's denied state, so a revoked grant, token or serial, or an expired certificate, ends access on an actor resolved earlier, and a lowered role applies at once. A token keeps the role it was issued with: lowering or replacing a grant does not change the principal's tokens (revoke them with the grant). A node or coordinator certificate is refused (`node_kind`), as is a revoked, denied or expired one. A certificate with no grant has no role.
- **Scoped token**: 32 random bytes in base64url, stored only as its sha256, with a role, projects and an expiry (default 720 h, at most 8760 h). `nself ci serve admin token issue` prints it once; later commands show the id and state, never the token. `roles revoke <principal>` ends the grant and every live token issued to that principal in one step and one audit row. Send tokens over TLS only: the transport enforces that, not the role check.

Commands (run on the coordinator host; the caller is the local operator):

```
nself ci serve admin roles grant <principal> --role release --project web --project api [--self]
nself ci serve admin roles revoke <principal>
nself ci serve admin roles list [--json]
nself ci serve admin token issue <principal> --role member --project '*' [--ttl 48h] [--self] [--json]
nself ci serve admin token revoke <id>
nself ci serve admin token list [--json]
nself ci serve admin cert issue <principal> [--self] [--ttl 168h] [--json]
nself ci serve admin audit [--since 24h] [--verify] [--json]
nself ci serve admin audit seal
```

Every grant, token and user certificate records a `subject`: `self` when it was issued with `--self` to the data directory's own operator identity, otherwise the named principal. `self` is reserved: a principal cannot be named `self`, so a remote identity cannot hide among the operator's own. `roles.NonSelfPrincipals` counts the live grants, tokens and user certificates whose subject is not `self` (a certificate nobody marked counts as non-self); the node-mode check reads it.

### Issuing user certificates

On the coordinator host, `NSELF_V15=1 nself ci serve admin cert issue <principal>` issues a user mTLS certificate and prints the certificate chain, private key and root as PEM once. Redirect stdout to a private file (for example, set `umask 077` first); `--json` returns these as `cert_pem`, `key_pem` and `root_pem`. The private key is never stored by the coordinator. The TTL defaults to seven days and must be between one minute and seven days.

Issuance requires admin access and does not grant a role: grant the principal its role separately. Each successful issuance records a certificate subject and a `cert.issue` audit event before returning the credentials. `--self` is accepted only for the operator's own principal and records subject `self`; otherwise the named principal counts toward TEAM mode, including when issuing to yourself without `--self`. Use the coordinator's `NSELF_CI_HOME` for both the store and its CA keys. If subject recording or audit fails, no credential is returned; a certificate already recorded before that failure can remain counted conservatively as non-self.

### The audit log

There is one audit table. The `trust-audit` set chains it: each sealed row's `head` is the sha256 of the previous head and the canonical row (seq, time, actor, action, target, detail). `Append` inserts a row and seals every pending row in one `BEGIN IMMEDIATE` transaction, so concurrent writers, even in a second process, lose no row. Rows other packages write through the core audit API (every node mutation) are sealed by the next `Append`, by `audit seal`, or by a sealer the TEAM coordinator runs every 60 s. `audit --verify` only reads, inside one snapshot: it prints `unsealed: <n>` and exits 1 when rows are still unsealed after 60 s, when a sealed row no longer matches its hash (the row is named), when the chain link is broken, when a seq number is missing (rows are never deleted, so a gap or an AUTOINCREMENT counter above the last row means rows were removed, including the newest ones), or when an unsealed row sits below a sealed one (a cleared head). `audit seal` refuses to re-chain such a row (E609), so the damage stays visible. An `Append` or a mutation still records its row, left unsealed, so a revocation is never blocked by tampering; `--verify` keeps failing until the log is restored from backup (there is no accept-the-break command yet). An event whose actor, action, target or detail contains a registered secret value, in any encoding the redactor knows, is refused (`E687`, reason `audit_secret`) and nothing is written. P7-TRUST-26 policy writes call `audit.AppendTx` to put their row in the policy write's own transaction.

### Recovering a damaged audit chain

For E609 or a failed `audit --verify`, restore a known-good coordinator backup; there is no command to accept a break. Do not clear heads, delete rows or recompute the chain in SQL: that destroys evidence of the break.

1. Stop the coordinator and keep it stopped through recovery. Preserve a private copy of the entire damaged data directory, including `state.db`, its `-wal` and `-shm` files if present, and any off-host signed checkpoints. Keep this evidence separate from the restore destination.
2. Select a backup from before the first bad sequence reported by verification and the matching separately stored KEK. Restore on the coordinator host:

   ```sh
   NSELF_V15=1 nself ci serve restore --data-dir "$NSELF_CI_HOME" --in /secure/known-good.backup --kek /secure/original.kek
   NSELF_V15=1 nself ci serve admin audit seal
   NSELF_V15=1 nself ci serve admin audit --verify
   ```

   Set `NSELF_CI_HOME` to the restored directory for all three commands. Restore validates the archive MAC before replacing state and saves the prior database as `state.db.pre-restore-<timestamp>`; it also prints a rollback command. Seal completes pending rows from the backup before verification. If verification still fails, keep the coordinator stopped and investigate an earlier backup.
3. Compare the restored chain against an off-host checkpoint from within the backup's history when available (`audit checkpoint verify --pubkey <pinned-fingerprint> <checkpoint>`). A checkpoint newer than the backup cannot verify that older snapshot.
4. Review the recovery output before restarting. Restore rotates job-token keys, advances the incarnation and applies restart grace to leases. It preserves current revocations and deny-list entries, but other writes after the backup, including audit events, are absent from the restored history; retain the damaged copy for investigation. A restore into an empty directory marks nodes `needs-reconfirm`. Re-enable only nodes you have checked.

Without a known-good backup and its KEK, this procedure cannot recover the chain. Preserve the damaged directory for investigation; normal writes and revocations can still be recorded unsealed, but verification remains failed.

What the chain does not give you: it detects an edit that was not recomputed. Someone who can write the database and recompute every later row defeats it; the TEAM checkpoint detects that (see Signed evidence, revocation and the audit checkpoint). A PERSONAL coordinator has neither a checkpoint nor a second writer to guard against, so the chain there is a tidy record, not evidence. Someone who can write the database can also clear heads and let the next `Append` re-chain them; that counts as a database writer too. (Removing the newest rows is caught by the seq check above, but only until the counter itself is edited.)

Handoff for later Tickets: `roles.Authorize(ctx, actor, role, project)` and `roles.AuthorizeAction(ctx, actor, action, project)` are the calls a route makes; `WithAuthor(ctx, id)` carries a run's author for `run.approve`. Both write one `authorize` row per release- or admin-level decision (allowed or refused, an unknown role included) and none for member-level reads, so a mutation that writes its own row records two rows for one request. That two-row pattern is the design: the `authorize` row is the decision and exists whether or not the mutation follows (a refusal never reaches a mutation row), and the mutation row is written first and fails closed. A refused token or certificate sign-in writes a `signin.fail` row (`token:unknown`, `token:<id>:<principal>`, `user:<id>` or `certificate:unknown` as the actor, the reason key and the peer address in the detail, never the token). It is rate limited: one row per peer and kind per 30 seconds, at most 20 rows per 30 seconds across all peers, and the next row says how many were held back (`suppressed=N`). A refused `serve admin` mutation (a validation or role refusal) writes one `<action>.refused` row (`roles.grant.refused`, `roles.revoke.refused`, `token.issue.refused`, `token.revoke.refused`) with the reason key. The `serve admin` commands otherwise write only their own mutation row and do not audit the local operator's own reads. The non-author rule for `run.approve` compares principal ids (an audit label such as `token:tok-1:bob` names `bob`); the local operator is exempt only on a PERSONAL coordinator, where it is the only person; on a TEAM coordinator it is refused as `self_approval` for a run it authored.

Wiring left to the coordinator process (P7-NODE-10 and P7-TRUST-26): start `audit.Log.RunSealer` (60 s) so rows other packages write get sealed, call `audit.SetScanner(run.Redactions)` before its first `Append` or `AppendTx` (the `serve admin` commands do this themselves), and resolve an Actor per request, not per connection. `store.CertSubjectPut` has no caller until the command that issues user certificates records their subject.


## Evidence gate acceptance (E685)

The coordinator judges every evidence document against a `Gate` built from its own protected policy and the current checkout; nothing in the gate comes from the document being judged. `attest.GateAccept(ev, g)` returns every reason the evidence fails, in a fixed order; an empty list means the evidence is accepted. `attest.GateAcceptDocument(doc, g)` decodes raw bytes and judges them in one step so a registered remote verifier sees the exact bytes once; it refuses documents with duplicate JSON keys. Both are pure and fail-closed: an empty or unknown gate field never matches, unknown freshness is stale, and a zero `Gate` accepts nothing. Adhoc and drill pipelines never satisfy a gate. Self-reported evidence passes only a default-class gate whose operator policy admitted it, never a release gate.

When the gate refuses, the error carries code `E685` ("evidence rejected") and one or more of these stable reason keys. A consumer branches on them, so a key is never renamed or reused:

| Reason | Meaning |
|---|---|
| `evidence.invalid` | the document is empty or over the size limit, fails the schema, repeats a JSON key, has an unknown field, enum value or producer kind, a malformed revision, or contradicts itself (binding, counts, producer against signature kind, a `pass` the checks do not support) |
| `evidence.repo_mismatch` | the document's repository id is empty or does not match the gate's repository id |
| `evidence.revision_mismatch` | the document's revision is empty or does not match the gate's exact revision |
| `evidence.policy_mismatch` | the document's policy digest is empty or does not match the gate's policy digest |
| `evidence.jobs_mismatch` | the gate requires at least one job and the evidence omits a required job, or the gate requires no jobs (a gate that requires nothing cannot be satisfied) |
| `evidence.stale` | the gate's `Current` input digests are nil (unknown), or the evidence's freshness relative to those digests is not `current` |
| `evidence.unbound` | the evidence's binding state is not `bound` or its worktree state is not `clean` |
| `evidence.adhoc` | the evidence's pipeline kind is neither `default` nor `named` (adhoc and drill pipelines never gate) |
| `evidence.trust_below_min` | the evidence's source trust ranks below the gate's minimum trust class, or either value is unknown |
| `evidence.self_reported_not_admitted` | the evidence is self-reported and the gate is a release gate or the operator policy did not admit self-reported evidence |
| `evidence.signature_unverifiable` | the evidence is signed-remote and the signature, key or document failed verification (see the E686 signature table below for sub-reasons) |
| `evidence.selection_partial` | the gate demands full selection (release gates always do) and the evidence's selection mode is not `full` |
| `evidence.not_passing` | the evidence's overall result is not `pass`, or a required job's checks include a non-passing result |

### Producer admission

The gate treats each producer kind differently. `coordinator-recorded` evidence is accepted without further checks (the coordinator wrote it). `signed-remote` evidence is verified against the re-marshalled or raw document bytes using the node registry's pinned keys; any failure yields `evidence.signature_unverifiable`. `self-reported` evidence is accepted only when the gate is default-class and the operator-scope policy set `AllowSelfReported`; a release gate or a missing admission yields `evidence.self_reported_not_admitted`. An unknown producer kind yields `evidence.invalid`.

### Release selection

A gate whose `Class` is anything other than `default` (the constant `release` included) or that sets `FullSelection: true` demands `selection.mode: full`. Evidence produced by a partial selection (for example a skip or subset run) is refused with `evidence.selection_partial`. Full selection is the only thing `FullSelection: true` adds: a default-class gate that sets it still accepts fully selected self-reported evidence when the operator policy set `AllowSelfReported`. Only a release-class gate (any `Class` other than `default`) refuses self-reported evidence regardless of operator policy.

### Known limits

- The gate reads the coordinator's own store; evidence read from a job's worktree proves nothing about who wrote the file.
- A zero `Gate` (no repo, no revision, no policy digest, no required jobs) accepts nothing; callers must build a complete gate from protected policy.
- Freshness depends on `Gate.Current`; a nil `Current` makes every document stale. The caller sets `Current` from the live checkout and input digests at the moment of judgment.

## Signed evidence, revocation and the audit checkpoint

A node that is signing eligible signs the evidence of every job it runs. The signature is Ed25519 with the node key over `nself-ci-evidence/v1`, a newline, and the canonical JSON of the document without its `signature` and `freshness` fields (numbers kept as written, fields a newer producer added stay in). The key id is `ci-node-` plus the first 16 hex characters of the sha256 of the public key. The coordinator accepts a `signed-remote` document only when all of these hold; otherwise the gate sees `evidence.signature_unverifiable` and the log gets an `E686` reason:

| Reason | Meaning |
|---|---|
| `signature_missing`, `signature_malformed`, `key_malformed` | no usable signature or key id |
| `attempt_unknown` | the document names no attempt, or the coordinator recorded no lease holder for it |
| `revision_mismatch`, `context_mismatch` | the document's revision, or its run id, repository id, source trust, policy digest or selection mode is not what the coordinator recorded for the attempt, the document names no job or another job than the attempt's, or one of its checks belongs to another job (a node chooses every field of its own evidence and vouches for its own job only, so the coordinator compares each field it knows; the repository id is the form the run's own evidence uses, the first 8 bytes of the sha256 of the project in hex, and a producer that uses another form is refused) |
| `replay` | the same signature was already used for another attempt |
| `key_revoked`, `key_unknown`, `key_purpose`, `signature_invalid` | the key checks, all made by the CLI's `sdk/go/signing` against the keys the node registry pinned |
| `lease_holder_mismatch` | the key belongs to a node that did not hold the lease of that attempt |
| `node_not_eligible` | the node's registry record says `signing_eligible: false` |
| `ledger_unavailable` | the coordinator store could not be read (a refusal, never a pass) |

A node signs only if its agent state directory holds an empty file named `signing-eligible` (a regular file, not a symlink, not writable by group or other). Create it, or let provisioning create it, only on a node whose jobs cannot read the node key: a separate job UID, a user namespace or a container. Without the file results carry no signature and count as unsigned. The coordinator's `signing_eligible` fact decides regardless of the file. A node key that was rotated away no longer verifies old evidence; a revoked key stays revoked (the id is derived from the key).

The coordinator also checks the signature of a result frame before it settles the attempt from the frame. A frame that carries a signature the registry does not verify (unknown, revoked or wrong key, a node that did not hold the lease, a node that is not signing eligible, a replay) ends the attempt `lost`, which counts against the node and takes the infra retry; it never settles as a pass. An unsigned frame is still accepted, because a node without the marker file completes unsigned, and the release gate refuses unsigned remote evidence. A result a node reports as an infra failure, and a cancel the run did not ask for (a drain or an agent self-cancel), also end the attempt `lost` and take the infra retry; a cancel the run asked for settles as cancelled.

The runner and coordinator ids in public provenance, evidence and statuses are pseudonyms: `runner-` or `coordinator-` and 12 hex characters of an HMAC-SHA256 under a secret the operator holds. The secret is 32 random bytes in `pseudonym.key` in the CI home (mode 0600, never a link), created by the first attempt or the first `nself ci artifacts sign` and never published; a wrong-sized, group-readable or non-regular file fails the attempt or the signing instead of falling back to an unkeyed hash. A dictionary of host names cannot recover a builder id without it. Deleting the file gives every id a new pseudonym.

**Revocation.** `run.RevokeNode` (and `attest.Revoker`) revoke every evidence key a node pinned or ever used, in one transaction, and quarantine the runs those keys signed. The call returns the gates, posted statuses and promoted releases the revoked evidence supported. It writes an `evidence.revoke` audit event and one `release.quarantine` event per promoted release, and re-posts each affected status as `error` with the reason `node.revoked`. `nself ci artifacts verify` reports a quarantined release as `quarantined` and exits 1; it reads the quarantine index from the coordinator store, so on a machine without one it cannot know and reports nothing. A node revoked with `nodes revoke` (registry state `revoked`) or by `RevokeNode` no longer verifies new evidence; a key pinned for it later is refused too.

**Audit checkpoint (TEAM only).** The audit chain detects an edit that was not recomputed. A signed checkpoint detects a database writer who recomputed every row.

```
nself ci serve admin audit checkpoint export <path> [--json]
nself ci serve admin audit checkpoint verify --pubkey <fingerprint> <path> [--json]
```

`export` (admin, TEAM coordinator only; PERSONAL mode refuses with a reason) seals the log, refuses a damaged chain, signs the head of the newest row with the coordinator's `ci-audit` key (`audit-checkpoint.key`, mode 0600, created on the first export in the data directory) and prints the key's sha256 fingerprint. Copy the checkpoint file and the fingerprint off this host: the key sits in the same data directory as the database, so only a checkpoint kept elsewhere detects a rewrite by someone who can write that directory. `verify` takes the fingerprint you pinned elsewhere; it never trusts the key in the data directory, refuses to run without `--pubkey`, checks the signature, re-verifies the whole chain and then compares the head of the checkpointed row with the live one. It exits 1 when the chain was rewritten (even with every hash recomputed) or rows were removed. Rows added after the checkpoint do not break it.

Handoff for later Tickets: the coordinator calls `run.UseAttestStore(st)` at start (the first attempt also sets it), a node revocation calls `run.RevokeNode`, and the coordinator that receives a result frame builds the signed-remote document with `producer.kind: signed-remote` and `signature: {kind: runner-ed25519, key_id, value}` from the frame; the agent signs the document it sends, so that document must already carry `attempt.id`, `job`, the real `repo_id`, `run_id`, `source_trust`, `policy_digest` and checks for its own job only. The lease carries those facts as its `evidence` context, read from the coordinator's record of the attempt, and the agent repeats them unchanged (the marker file and the revoke caller are unwired: D-1300).

## Policy sources and `nself ci policy`

The effective policy of a repository is merged from, loosest first: the defaults, your operator rows (scopes `user`, `team`, `project`, kept in the CI store), the protected default branch's `.nself/ci.yaml` `policy:` block read at the fetched `origin/<default>` tip (or at an admin pin), the checked-out revision's own `policy:` block, and the flags of one run. Every later scope can only tighten. A widening in the revision, and any Spend key (`economics.*`, `burst.*`) in the branch or the revision, is ignored and listed. A `policy:` block in a pull request therefore never loosens anything.

```
nself ci policy show [--effective] [--json] [--team <id>]
nself ci policy set --scope user|team|project [--team <id>] <key> <value> [--json]
nself ci policy unset --scope user|team|project [--team <id>] <key> [--json]
nself ci policy pin <sha> | --clear [--json]
```

- `show` prints every effective key with the scope it came from, the pinned commit, the fetch state (`fresh`, `stale`, `none`), the policy digest and the ignored fields with their code (`E671`). `--effective` prints only the values. `--json` data is `{effective, sources, pinned_commit, fetch, digest, ignored: [{key, source, code, reason}]}`. A Secret key prints as `[REDACTED]`.
- `set` takes the value as JSON when it parses as JSON (`true`, `["a","b"]`), else as a plain string (`restricted`). The value goes through the key's own decoder, a string outside the key's order is refused, and the new row must merge without loosening a parent (`E670`) with the stored rows of the user, team and project it names, the protected branch's policy and the strictest defaults (an untrusted source). That keeps a stored row from making a later run in this repository fail to resolve. An unknown key is `E672`. `--scope project` and `pin` apply to the repository the command runs in (the project id comes from git; if git cannot run the command refuses rather than guess); `--scope team` needs `--team <id>` (letters, digits, `.`, `-`, `_`, at most 64).
- `pin <sha>` fixes the protected-branch commit the policy is read at. It accepts only a full commit id reachable from the fetched `origin/<default>` (else `E672`); `--clear` returns to the tip.
- Every `set`, `unset`, `pin` and `--clear` needs the `admin` role (the local operator of a PERSONAL coordinator has it) and writes its policy row and one `policy.*` audit row in one transaction: both land or neither does. The role check itself writes its own `authorize` row first, so a successful write leaves two rows and a refused one leaves only the `authorize` row (`policy.refused` for a machine-invoked refusal). The audit row names the scope, the quoted subject, the key, a short digest and the new value; a Secret key's value is never written.

### Machine-invoked callers cannot loosen trust policy

When `NSELF_INVOKED_BY` is set (the MCP and HTTP invokers always set it; you can export it for an agent that uses the CLI), `set`, `unset` and `pin` refuse, with `E687` reason `machine_invoked`, any write that loosens a `trust.*` key: raising `trust.machine_invoked_cap`, unsetting any `trust.*` key, and every `pin` or `--clear`. The refusal writes nothing to the policy and one `policy.refused` audit row. Lowering a trust key (`trust.machine_invoked_cap` from `owner` to `untrusted`) works, and so do keys outside `trust.*`. A first value for a `trust.*` list key that has no default (`trust.providers`, `trust.self_reported_gates`) counts as loosening, because an unset list means none allowed. With the variable unset the operator's writes work as before. This guard is advisory against an agent that controls its own environment, which can simply unset the variable; the plan also marks the three write commands `cli-only` on the MCP and HTTP surfaces (P7-CI-62) so a remote caller cannot reach them at all; that mapping is not part of this command.

### Known limits

- A `team` row applies to every run, plan and dispatch of the process whose environment sets `NSELF_CI_TEAM=<team id>` (the coordinator's own environment, never a job's env or a repository file; same alphabet as `--team`). A malformed value fails the resolve with `E672` rather than dropping the team's rows; `show --team <id>` overrides it. With the variable unset no team row applies.
- `pin` needs a fetched origin; with no origin there is nothing to pin against.
- The checks read the store before the write transaction opens, so two writers at the same moment can race (for example a machine write over a concurrent operator tightening). A `set` is also checked against the rows of every stored project and team (the store lists the subjects of a scope), not only the current repository's, so a user, team or project write cannot make `Resolve` fail with `E670` in another repository; the refusal names the other project.

The scheduler requires a `World.Network` proof for the effective scope even when the map is nil. A runner declaration cannot supply that proof. Placement and the network planner share the same predicate.

### Node gate registry preparation

A private registry token is a declared job secret. Built-in checks accept
`env` and `secrets` in their check configuration, using the same resolver and
trust admission as authored jobs:

```yaml
checks:
  node.lint:
    secrets: [project/NPM_TOKEN]
    env: {NSELF_CI_LOCAL_REGISTRY_INFRA: "1"}
```

Use `${NPM_TOKEN}` in `.npmrc`. The install
and the check share the resolved environment. Host registry tokens are not
forwarded implicitly; registry authentication errors remain project failures.

A local registry that the operator manages as infrastructure can opt in with
`env: {NSELF_CI_LOCAL_REGISTRY_INFRA: "1"}` on the job. Only structured
network/DNS install failures become retryable infrastructure failures. Lockfile,
lifecycle, test and registry client errors remain project failures. Without this
opt-in, a down localhost registry remains the project's setup failure.

Node preparation uses a private `mktemp` directory under the job's `TMPDIR`
(or `/tmp`), outside the copied project. Its caches stay available through the
check, including Yarn Plug'n'Play archives, then the shell removes the directory
on exit or a handled signal. An uncatchable SIGKILL or host crash can leave a
temporary directory for the host's normal temporary-file cleanup; container
scratch lives on the attempt's temporary filesystem. No `.nself-ci` directory is
created in the project by preparation.
