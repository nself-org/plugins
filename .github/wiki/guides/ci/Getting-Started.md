# Getting started with nself ci

<!-- docmap: cap:docs.ci-core -->

`nself ci` runs your project's checks on your own machine and records what happened as signed-off evidence. It needs no cloud account, no hosted runner and no network beyond what your own tools use. This page takes you from nothing to a first recorded run, then covers what is detected, what stays local, the history you get for free, and container mode.

Everything on this page is the v1.5 engine. Turn it on with `NSELF_V15=1` in your environment. Without it, `nself ci` runs the older gate; see [[Migrating to the new CI engine|guides/ci/Migration]]. The commands, flags and codes are listed in the generated [[CI reference|guides/ci/Reference]].

## Install

Install the plugin once, then check the machine:

```text
nself plugin install ci
NSELF_V15=1 nself ci doctor
```

`doctor` checks the tools your repository needs, the container runtime, status posting, the local store and your configuration. Each finding has an id, a one-line reason and, when it can, a fix. A warning never blocks a run. Exit codes and the full list of ids are in the [[CI reference|guides/ci/Reference]]; what to do about one is in [[CI troubleshooting|guides/ci/Troubleshooting]].

## Your first run

Make a project, commit it, and ask what `nself ci` would do before it does anything:

```bash
mkdir hello && cd hello && git init -q
git config user.email docs@example.com && git config user.name docs
printf 'module example.com/hello\n\ngo 1.22\n' > go.mod
printf 'package main\n\nfunc main() {}\n' > main.go
git add -A && git commit -q -m init
nself ci plan
nself ci config show
```

`plan` prints the job graph: each job, whether it is required, why it was chosen (`selection.no_base` here, because nothing says what changed, so everything runs) and where it would be placed. `config show` prints every effective setting with the layer it came from. With no `.nself/ci.yaml` the detected defaults apply.

A bare `nself ci` then runs the plan:

```text
NSELF_V15=1 nself ci
```

It runs the required jobs, prints one line per job, records the run in a local store, and ends with a result line. When the repository has a GitHub remote it also posts a `nself-ci` commit status; with no remote it says in one line why it did not. Add `--no-status` to never post.

To run any single command through the same machinery, put it after `--`. The command runs in a container by default, with no network. On a machine without Docker, `--isolation process` runs it on the host instead and says so:

```text
nself ci --isolation process --no-status -- cat file.txt
```

An ad hoc run is recorded like any other, as a pipeline of kind `adhoc`, and never posts a status. The flags and their limits are in [[Evidence and machine output|guides/ci/Evidence-and-Machine-Output]].

## What is detected

`nself ci` reads manifests and lockfiles, never your source, to decide which checks apply. A check is planned only when its stack is present.

| Stack | Detected from | Checks |
|---|---|---|
| Go | `go.mod` | format, vet, test, plus a vulnerability scan that is advisory |
| Node | `package.json`, with npm, pnpm, yarn or bun chosen by lockfile; workspaces are walked | lint, typecheck, test, build |
| Rust | `Cargo.toml` | clippy, test |
| Flutter and Dart | `pubspec.yaml` | analyze, test |
| Swift | `Package.swift`, or Xcode projects and workspaces with a shared scheme | `swift build` and `swift test`; `xcodebuild test` on a Mac with Xcode |
| Python | `pyproject.toml` or requirements files; uv, poetry or pip chosen by lockfile | pytest when configured or when `tests/` exists; ruff and mypy when configured |
| Dockerfiles | `Dockerfile*` | hadolint and a configuration scan, both advisory |
| Secrets | any repository | `gitleaks` |

Lockfile digests are recorded as evidence inputs. Services declared in a compose file are noticed, but a client library or an env sample never implies a service: a job gets a database only when you name one. See [[CI configuration and policy|guides/ci/Configuration-and-Policy]].

Notes on the Apple and Python stacks:

- Xcode checks need a shared scheme. Share the scheme in Xcode, or the project yields no check. The destination is the Mac itself, so iOS-only schemes need a custom job.
- With `uv.lock` the Python checks use uv; otherwise with `poetry.lock` they use poetry; otherwise pip into a `.venv` in the job root.
- Which checks are required and which are advisory is the table in [[CI configuration and policy|guides/ci/Configuration-and-Policy]].

## Local-only, and what that does and does not give you

The default run is local. Nothing leaves the machine unless a tool you run reaches out or you post a status. A drill in this repository proves the first run with the network cut off.

- **Reservations.** A run started from your terminal uses the machine freely. Background work leaves headroom for you: by default two CPUs and a quarter of memory stay free, and deep jobs wait while a laptop is on battery below a threshold. A held job shows the reason code `admission.reservation` or `admission.battery`. `--force` ignores the reservations. A run under `CI=true` or without a terminal does not reserve anything.
- **Isolation.** The `process` level runs a job as a child process in a copy of your workspace with a controlled environment, a time limit and a kill of the whole process tree on cancel. It is not a sandbox: the job shares your network, your user and your disk. The container level is stronger, and is still not a boundary against hostile code. Treat both as guard rails for your own code. The model is in [[CI trust, security and privacy|guides/ci/Trust-Security-and-Privacy]].
- **Secrets.** Job output passes a redactor before it is stored, so a stored log never holds a value the redactor knew. Evidence holds redacted excerpts only.

## History, logs and retention

Every run is stored locally, with its jobs, attempts, evidence and logs.

- `nself ci runs list [--limit N] [--json]` lists recent runs with result, duration, revision, binding and freshness.
- `nself ci runs <run>` shows one run. `nself ci logs <job|attempt> [-f] [--json] [--run <run>] [--attempt N]` prints its log, and `-f` follows a live one within about a second.
- Logs hold redacted output only, as JSON lines with a sequence number. The sequence is the order; two lines in one burst can share a timestamp.

Retention keeps the newest runs, drops old ones and caps the size of stored logs. It runs when a run starts and on `nself ci runs prune [--dry-run]`, and a run in progress is never removed. The limits are defaults you override in `retention.json` in the state directory. `runs prune` states what it removed.

```bash
mkdir keep && cd keep && git init -q
git config user.email docs@example.com && git config user.name docs
mkdir .nself && cat > .nself/ci.yaml <<'CFG'
version: 1
jobs:
  check:
    kind: test
    run: [sh, -c, "echo checked"]
CFG
git add -A && git commit -q -m init
nself ci --no-status
nself ci runs list
nself ci logs check
nself ci runs prune --dry-run
```

The state directory is `$NSELF_CI_HOME`, which defaults to a folder in your home directory.

## Container mode

`nself ci --container` runs each job in one pinned toolchain image instead of the tools on your machine, so a result does not depend on which Go or Node you have installed. The image carries its tools by checksum, and the pinned versions are in the generated table of the [[CI reference|guides/ci/Reference]]. The first run on a host pulls the whole image; after that a tool bump re-pulls one layer.

- The image reference must be pinned by digest. A tag alone is refused. `--image <ref@digest>` or `NSELF_CI_IMAGE` selects another image.
- There is no silent fallback to host tools. If the image is not available the run stops with an error that names the problem.
- Container mode needs Docker or a compatible runtime. `doctor` tells you whether it is reachable.

Flutter has no tool in the image, so Flutter checks always run with the host tools.

## Next steps

- Describe your own jobs, services and pipelines: [[CI configuration and policy|guides/ci/Configuration-and-Policy]].
- Read results as data, or start a run and come back later: [[Evidence and machine output|guides/ci/Evidence-and-Machine-Output]].
- Add a second machine: [[Nodes and Runners|guides/ci/Nodes-and-Runners]].
- Something failed: [[CI troubleshooting|guides/ci/Troubleshooting]].
