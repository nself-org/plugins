# CI trust, security and privacy

This page covers the `container` isolation level of `nself ci`: what it blocks, what it does not, and what a node must provide. The sandboxed container level (gVisor) is described in its own section below. The other levels (process, VM) are described next to their own profiles.

## The container level

A job at this level runs in a Docker or Podman container that the isolation profile builds from one pure function. The executor refuses any command line that is not on its allowlist, whatever the profile says.

Every container gets:

- `--cap-drop=ALL`, `--security-opt no-new-privileges` and a read-only root filesystem, with a 256 MB `/tmp`.
- A numeric user, `65532 + slot`, never root and never the agent's own UID. Two concurrent attempts that share a home never share a slot.
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

The level is unavailable (`E678`, reason `isolation.runtime_flag_denied`) when the runsc entry carries any of `--network=host`, `--host-uds`, `--host-fifo`, `--net-raw`, `--allow-flag-override`, `--ignore-cgroups`, `--rootless`, `--file-access=shared`, `--nvproxy`, `--nvproxy-docker`, `--tpuproxy`, `--profile`, `--fsgofer-host-uds` or a `--TESTONLY-*` flag. The first group opens the sandbox to the host; `--ignore-cgroups` would drop the pids, memory and CPU limits. The profile refuses them rather than stripping them, because the daemon would still pass them to runsc. A flag is matched by presence in every spelling the Go flag package accepts, so `--net-raw=false` is refused as well.

The deny list is not a complete list of unsafe flags. The entry is judged only as far as this profile can read it: an entry with a key other than `path`, `runtimeArgs` and `status` is refused, and so is an entry with no `path`. A containerd shim runtime (`runtimeType`, `options`) keeps its settings in a file the profile does not read, and `docker info` does not show those keys, so such a runtime appears as an entry without a path. `runsc install` always writes a path. The runtime table is read on every check, so a daemon reload that adds a denied argument is seen on the next check. Only the real start is remembered (for a minute, per runtime entry), and concurrent checks share one start. A change made between a check and the `docker run` is not seen; changing the daemon configuration needs root, which already owns the host.

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

`restricted` and `internet` go through the same egress proxy as the container level (public hosts only, the deploy exclusion set refused), served by the coordinator on the gateway address, and the ssh session sets `HTTP_PROXY` and `HTTPS_PROXY`. Before the job starts, a check runs inside the VM: the proxy must answer, the gateway must answer, and the public internet, the Mac's own LAN addresses and the exclusion set must not. A scope that cannot be proven fails the attempt with `E679` (`network.scope_unenforceable`), and the clone is deleted. If the installed Tart has no softnet block option, or no proxy can be started, every scope below `lan` is unavailable.

### Known limits

- Real-run evidence needs an Apple-silicon Mac with Tart; GitHub-hosted macOS runners do not allow nested virtualisation.
- softnet filters by destination address, not port. The gateway address is allowed, so anything on your Mac that listens on all interfaces (ssh, file sharing, screen sharing) is reachable from the VM on that address. Turn those services off on a Mac that runs untrusted jobs, or keep untrusted work on a Mac that offers none of them.
- Host keys of an ephemeral clone are not pinned: the session is `BatchMode` with no forwarding and no agent, on a private NAT.
- IPv6 is not filtered by softnet rules; the VM image should not route it.
- A job that declares secrets needs a per-attempt secret source in its run; a run without one is refused rather than started without its secrets.
- Declared artifact paths are literal paths (no globs). Symlinks and special files in them are refused.
- The Tart gateway is `192.168.64.1` unless the Mac's vmnet network was changed; a different address makes the network check fail closed.

### Handoff

- `ci doctor` does not print the Tart facts yet; the profile exposes `tart_version`, `licence_note`, `free_slots`, `softnet_block` and `base_image_cached` as availability facts.
- The tagged integration tests (`-tags tart`) are the real-run evidence; they run on a Mac with Tart and the environment variables listed at the top of `integration_test.go`.

## Network scopes

A job runs under one scope. An undeclared scope takes the default of its trust class: `owner` and `internal` get `lan`, `collaborator` gets `internet`, `untrusted` gets `restricted`. An untrusted job that declares `lan` or `privileged` is refused (`E680`), before any resource exists.

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
- The scheduler reads the proof for the local runner. A remote node's `network.enforced_scopes` is not reported yet, so no untrusted job is placed on a remote node on the strength of it. The agent does not yet pass the lease's network scope into the executor (container jobs there run with `none`), and `netscope.Effective`, which refuses `lan` and `privileged` for an untrusted job, is called by the scheduler's admission rules only.
- The proxy sees the destination host and port, not the encrypted content. It does not check the TLS server name, so a shared CDN host on the allowlist can be used for traffic to another site behind it, and uploading to an allowlisted registry is still an exfiltration path. `restricted` limits where a job can talk, not what it says.
- The host's other routing tables (policy routing for a VPN) and a public address that hairpins to a LAN service through the router are not in the on-link set. Put such addresses in the never-list.
- The sidecar bind-mounts this process's own binary, so the file must be readable and executable by other users (mode `0755`), or the scope is unavailable (`E679`).

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
nself ci serve admin audit [--since 24h] [--verify] [--json]
nself ci serve admin audit seal
```

Every grant, token and user certificate records a `subject`: `self` when it was issued with `--self` to the data directory's own operator identity, otherwise the named principal. `self` is reserved: a principal cannot be named `self`, so a remote identity cannot hide among the operator's own. `roles.NonSelfPrincipals` counts the live grants, tokens and user certificates whose subject is not `self` (a certificate nobody marked counts as non-self); the node-mode check reads it.

### The audit log

There is one audit table. The `trust-audit` set chains it: each sealed row's `head` is the sha256 of the previous head and the canonical row (seq, time, actor, action, target, detail). `Append` inserts a row and seals every pending row in one `BEGIN IMMEDIATE` transaction, so concurrent writers, even in a second process, lose no row. Rows other packages write through the core audit API (every node mutation) are sealed by the next `Append`, by `audit seal`, or by a sealer the TEAM coordinator runs every 60 s. `audit --verify` only reads, inside one snapshot: it prints `unsealed: <n>` and exits 1 when rows are still unsealed after 60 s, when a sealed row no longer matches its hash (the row is named), when the chain link is broken, when a seq number is missing (rows are never deleted, so a gap or an AUTOINCREMENT counter above the last row means rows were removed, including the newest ones), or when an unsealed row sits below a sealed one (a cleared head). `audit seal` refuses to re-chain such a row (E609), so the damage stays visible. An `Append` or a mutation still records its row, left unsealed, so a revocation is never blocked by tampering; `--verify` keeps failing until the log is restored from backup (there is no accept-the-break command yet). An event whose actor, action, target or detail contains a registered secret value, in any encoding the redactor knows, is refused (`E687`, reason `audit_secret`) and nothing is written. P7-TRUST-26 policy writes call `audit.AppendTx` to put their row in the policy write's own transaction.

What the chain does not give you: it detects an edit that was not recomputed. Someone who can write the database and recompute every later row defeats it; the TEAM checkpoint detects that (see Signed evidence, revocation and the audit checkpoint). A PERSONAL coordinator has neither a checkpoint nor a second writer to guard against, so the chain there is a tidy record, not evidence. Someone who can write the database can also clear heads and let the next `Append` re-chain them; that counts as a database writer too. (Removing the newest rows is caught by the seq check above, but only until the counter itself is edited.)

Handoff for later Tickets: `roles.Authorize(ctx, actor, role, project)` and `roles.AuthorizeAction(ctx, actor, action, project)` are the calls a route makes; `WithAuthor(ctx, id)` carries a run's author for `run.approve`. Both write one `authorize` row per release- or admin-level decision (allowed or refused, an unknown role included) and none for member-level reads, so a mutation that writes its own row records two rows for one request. The `serve admin` commands write only their own mutation row and do not audit failed validation, failed token or certificate sign-in, or the local operator's own reads. The non-author rule for `run.approve` compares principal ids (an audit label such as `token:tok-1:bob` names `bob`); the local operator is exempt, being the only person of a PERSONAL coordinator.

Wiring left to the coordinator process (P7-NODE-10 and P7-TRUST-26): start `audit.Log.RunSealer` (60 s) so rows other packages write get sealed, call `audit.SetScanner(run.Redactions)` before its first `Append` or `AppendTx` (the `serve admin` commands do this themselves), and resolve an Actor per request, not per connection. `store.CertSubjectPut` has no caller until the command that issues user certificates records their subject.


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

**Revocation.** `run.RevokeNode` (and `attest.Revoker`) revoke every evidence key a node pinned or ever used, in one transaction, and quarantine the runs those keys signed. The call returns the gates, posted statuses and promoted releases the revoked evidence supported. It writes an `evidence.revoke` audit event and one `release.quarantine` event per promoted release, and re-posts each affected status as `error` with the reason `node.revoked`. `nself ci artifacts verify` reports a quarantined release as `quarantined` and exits 1; it reads the quarantine index from the coordinator store, so on a machine without one it cannot know and reports nothing. A node revoked with `nodes revoke` (registry state `revoked`) or by `RevokeNode` no longer verifies new evidence; a key pinned for it later is refused too.

**Audit checkpoint (TEAM only).** The audit chain detects an edit that was not recomputed. A signed checkpoint detects a database writer who recomputed every row.

```
nself ci serve admin audit checkpoint export <path> [--json]
nself ci serve admin audit checkpoint verify --pubkey <fingerprint> <path> [--json]
```

`export` (admin, TEAM coordinator only; PERSONAL mode refuses with a reason) seals the log, refuses a damaged chain, signs the head of the newest row with the coordinator's `ci-audit` key (`audit-checkpoint.key`, mode 0600, created on the first export in the data directory) and prints the key's sha256 fingerprint. Copy the checkpoint file and the fingerprint off this host: the key sits in the same data directory as the database, so only a checkpoint kept elsewhere detects a rewrite by someone who can write that directory. `verify` takes the fingerprint you pinned elsewhere; it never trusts the key in the data directory, refuses to run without `--pubkey`, checks the signature, re-verifies the whole chain and then compares the head of the checkpointed row with the live one. It exits 1 when the chain was rewritten (even with every hash recomputed) or rows were removed. Rows added after the checkpoint do not break it.

Handoff for later Tickets: the coordinator calls `run.UseAttestStore(st)` at start (the first attempt also sets it), a node revocation calls `run.RevokeNode`, and the coordinator that receives a result frame builds the signed-remote document with `producer.kind: signed-remote` and `signature: {kind: runner-ed25519, key_id, value}` from the frame; the agent signs the document it sends, so that document must already carry `attempt.id`, `job`, the real `repo_id`, `run_id`, `source_trust`, `policy_digest` and checks for its own job only (the agent's base document does not yet: D-1299; the marker file and the revoke caller are unwired: D-1300).

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

- A `team` row applies only where a caller names the team (`show --team`); no run path supplies one yet, so a TEAM coordinator's team rows are inert until the coordinator wiring (P7-NODE) passes the team id.
- `pin` needs a fetched origin; with no origin there is nothing to pin against.
- The checks read the store before the write transaction opens, so two writers at the same moment can race (for example a machine write over a concurrent operator tightening). The rows of other projects are not listed by the store, so a user-scope write is not checked against them; `policy show` in that repository reports a conflict as `E670`.
