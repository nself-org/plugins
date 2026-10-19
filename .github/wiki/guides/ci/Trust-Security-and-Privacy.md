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

What the chain does not give you: it detects an edit that was not recomputed. Someone who can write the database and recompute every later row defeats it; the TEAM checkpoint (P7-TRUST-20) detects that. A PERSONAL coordinator has neither a checkpoint nor a second writer to guard against, so the chain there is a tidy record, not evidence. Someone who can write the database can also clear heads and let the next `Append` re-chain them; that counts as a database writer too. (Removing the newest rows is caught by the seq check above, but only until the counter itself is edited.)

Handoff for later Tickets: `roles.Authorize(ctx, actor, role, project)` and `roles.AuthorizeAction(ctx, actor, action, project)` are the calls a route makes; `WithAuthor(ctx, id)` carries a run's author for `run.approve`. Both write one `authorize` row per release- or admin-level decision (allowed or refused, an unknown role included) and none for member-level reads, so a mutation that writes its own row records two rows for one request. The `serve admin` commands write only their own mutation row and do not audit failed validation, failed token or certificate sign-in, or the local operator's own reads. The non-author rule for `run.approve` compares principal ids (an audit label such as `token:tok-1:bob` names `bob`); the local operator is exempt, being the only person of a PERSONAL coordinator.

Wiring left to the coordinator process (P7-NODE-10 and P7-TRUST-26): start `audit.Log.RunSealer` (60 s) so rows other packages write get sealed, call `audit.SetScanner(run.Redactions)` before its first `Append` or `AppendTx` (the `serve admin` commands do this themselves), and resolve an Actor per request, not per connection. `store.CertSubjectPut` has no caller until the command that issues user certificates records their subject.


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
