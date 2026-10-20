# Hosted providers

This page covers how `nself ci` works with forges you do not run yourself. The section below is the GitHub runner job: a queued GitHub Actions job that your own nodes carry through a just-in-time (JIT) runner. Other provider sections are added to this page as they ship.

## GitHub runner jobs (JIT)

When a workflow job on GitHub asks for a self-hosted runner with your jit label, the coordinator can register a one-job runner and run it as a job on one of your nodes. The runner takes one job and goes away. The job stays untrusted and sandboxed like any other job from a repository: a repository-scope runner is never trusted.

### The cap: `providers.github.jit.max_pending`

| Property | Value |
|---|---|
| Key | `providers.github.jit.max_pending` |
| Range | whole number, 1 to 50 |
| Default | 2 |
| Scope | operator only |
| Meaning | the most runner demands live for one repository at a time |

A repository cannot raise this number, because it caps the runners your hardware carries for that repository. A value outside 1 to 50 is refused (`E672`).

When a repository is at the cap, a new demand is answered `E703 runner.demand_capped`. That is not a refusal. What happens next depends on how demands reach the coordinator:

| Mode | A capped demand |
|---|---|
| Poll | The demand poll offers the job again when a slot frees. |
| Webhook | There is no poll. The job stays queued at GitHub and is not offered again until a new delivery arrives. |

If you use webhook mode and want capped jobs picked up without waiting for a new event, raise the cap or use poll mode.

### Labels

A job is accepted only when every label in its `runs-on` is one of these (case-insensitive, ASCII only):

- `self-hosted`
- your jit label (`providers.github.jit.label`, default `nself`)
- `linux`
- `x64` or `arm64` (not both)

The set must contain `self-hosted` and the jit label. A job that asks for `ubuntu-latest`, `windows`, `macos` or any other label is refused (`E708 label.not_allowed`) and is never offered again.

The label check decides whether a runner is created. It does not decide which queued job the runner takes once it is online. GitHub hands the runner any queued job whose `runs-on` is a subset of the runner's labels, and that includes a bare `runs-on: self-hosted`. So a `runs-on: self-hosted` job can land on a JIT runner. It is still untrusted and sandboxed, but do not assume the jit label alone keeps other jobs off it.

### What the job gets

- **Linux only.** The runner image is Linux (`amd64` or `arm64`).
- **No Docker inside the job.** Container runtime sockets are not mounted, so steps that need `docker` do not work.
- **A memory-only runner root.** The runner runs from a 384 MB tmpfs mounted at `/runner` (`rw,exec,nosuid,nodev`). The decoded runner credential files (`.runner`, `.credentials`, `.credentials_rsaparams`) and the runner's `_diag` logs are written there, so they live in memory and never reach the node's disk.
- **One copy of `bin/` per job.** The runner writes next to its own binaries, so each job copies the image's `bin/` (about 88 MB) into the tmpfs before it starts. Count that against the 384 MB.
- **The job can read its own JIT config.** The registration reaches the runner through its environment (`ACTIONS_RUNNER_INPUT_JITCONFIG`), and the steps run as children of the runner. A step can read its own runner's credentials. They are valid for that one runner and one job only.

### Network: exact host names

A runner job reaches only an exact-name host allowlist. For GitHub it is:

- `github.com`, `api.github.com`, `codeload.github.com`
- `objects.githubusercontent.com`, `pkg-containers.githubusercontent.com`, `ghcr.io`
- `pipelines.actions.githubusercontent.com`, `results-receiver.actions.githubusercontent.com`
- `broker.actions.githubusercontent.com`, `vstoken.actions.githubusercontent.com`, `token.actions.githubusercontent.com`

There are no wildcards, so a runner whose region answers from a numbered `*.actions.githubusercontent.com` host that is not listed cannot connect and the job fails closed. Nothing in a workflow or a demand can add a host.
