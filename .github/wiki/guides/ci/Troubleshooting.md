# CI troubleshooting

<!-- docmap: cap:docs.ci-core -->

Start here when a run is red, slow, stuck or surprising. The order that finds most problems is: ask `doctor` about the machine, ask `explain` about the plan, read the log, then reproduce the job. Codes such as `E614` are listed with their summaries and fixes in the generated [[CI reference|guides/ci/Reference]]; this page tells you which one to look for and what to do next.

## Ask the machine: doctor

```text
NSELF_V15=1 nself ci doctor [--json] [--repo dir]
```

`doctor` runs a set of read-only checks and prints one line per finding with an id, a verdict (`PASS`, `WARN` or `FAIL`), a reason and, when it knows one, a fix. It covers the toolchain for each detected stack, whether the container runtime is reachable, whether status posting is possible, whether the local store opens and is the right version, whether `.nself/ci.yaml` validates, a leftover legacy `.ci.yaml`, a git-ignored config, and drift between the tools you have and the pinned toolchain image. Nodes, providers and isolation add their own sections when they are in use.

- Exit 0 means clean, 10 means at least one failure, 12 means warnings only, 1 is a usage error and 2 is a bad `--repo`.
- A section that fails to run, panics or takes too long is reported as its own finding. One broken section never hides the rest.
- `doctor` never creates the local store. A missing store passes with "created on first run".
- The table of ids is generated: see the doctor section of the [[CI reference|guides/ci/Reference]]. A warning about drift in a tool version is information: run with `--container` for the pinned version, or install it.
- Known limits: the toolchain requirement is matched on the first word of a job's command, so a job run through `make` or `sh -c` does not mark a toolchain as required; a job that needs a service does not by itself mark Docker as needed.

## Ask the plan: explain

If a job did not run, or ran where you did not expect, do not guess:

```text
nself ci plan [--pipeline name] [--trigger t] [--base sha]
nself ci explain <job>
```

`plan` lists every job with its state and a reason code. `explain` shows the requirements of a job, the runner it chose, the score components that led to that choice and the alternatives. Both only read; neither starts anything.

| You see | It usually means |
|---|---|
| `selection.no_base` | nothing said what changed, so everything runs; pass `--base` or run from a branch with a known base |
| `pipeline.excluded` | the job is not in the pipeline you named |
| `when.paths`, `when.branch`, `when.trigger` | a condition in `when` was not met |
| `when.env_missing` | the variable named in `when.env_present` is not set |
| `dep.failed` | a job it depends on failed, so it was skipped |
| `unplaceable.*` | no runner can take it: wrong platform, a missing tool, no Docker or a missing label |
| `admission.reservation`, `admission.battery` | the machine is protecting itself; see [[Getting started with nself ci|guides/ci/Getting-Started]] |
| `advisory.tool_missing`, `advisory.data_offline` | an advisory check skipped because its tool or data was unavailable |

## Read the log

```text
nself ci logs <job|attempt> [--run <run>] [--attempt N] [-f] [--json]
```

Without `--attempt` it shows the highest attempt of the job. After `--confirm-flaky` that is the passing rerun, not the failing attempt the evidence points to, so pass `--attempt` when you want the original. With `--json` you get NDJSON log records and a final result record. No stored log for that job or attempt is `E633`, and a stored log that cannot be read is `E632`; check that the state directory is writable and not full. A log can be missing when the disk was full or the folder unwritable; the job is unaffected, and `ci logs` is where you see it.

Logs hold redacted output, so a value that matches a known secret shape shows as a mask. Lines carry a sequence number; use it for order, since lines written in one burst can share a timestamp.

## Failure or infrastructure

A failed check carries a **failure class**, and the class decides what happens next.

| Class | Meaning | Retried automatically |
|---|---|---|
| `code` | your code or tests failed | no |
| `infra` | the machine, the network or the tool installation failed | yes, up to the budget |
| `timeout` | the job ran out of time | no |
| `cancelled` | someone stopped it | no |
| `lost` | the runner went away | by recovery, not by retry |

The classifier looks for infrastructure evidence in a fixed order: the executor died without an exit code, a container hit a resource limit, the executor reported its own error, or the last lines of output match a known infrastructure signature, and only when no test-failure marker such as `FAIL`, `not ok`, `assertion` or `panic` is in the tail. Anything else is `code`. **A plain exit 137 or 127 is a code failure**: a program chose it. An unknown job failure is `code`. An error on the executor side that cannot be proven is `infra`, but it is never retried.

The evidence for a failed check names the class, the exit code, a `diagnosis` that matches a signature, a `remediation`, a redacted excerpt and where the log is stored.

**Retries.** An infrastructure failure is retried up to `retry.infra_max`, bounded by the plan's own limit. Release jobs and jobs marked `idempotent: false` run once and wait for an operator instead. Each retry records why the previous attempt failed.

**Install failures.** For Node, a failed frozen install prints a bounded report and is classified from the manager's own error lines: an out-of-sync lockfile, a failing lifecycle script or a test failure is `code`; a registry error, a full disk or a killed child is `infra`. A package manager that does not match the declared one is `E811` and is not retried. A tool that is not installed at all is `E614`.

### Flaky checks

A check that fails once and passes on identical inputs is flaky, and you can measure it without changing a result:

```text
nself ci --confirm-flaky 3
nself ci runs --flaky [--limit N] [--json]
```

`--confirm-flaky N` reruns a job that failed with a code failure up to N more times, stopping at the first pass. N is at most ten. Each rerun is authorised by an audit row naming the failed attempt and is recorded with the diagnosis `confirm-flaky`. The original result stays **failed**: confirmation tells you the check is flaky, it never turns red into green. `runs --flaky` lists checks that both passed and failed on identical inputs (same repository, revision, tracked digest, pipeline, policy, source trust and input digests) among recent runs. Dirty, unbound and ad hoc runs are ignored. The finished run's evidence carries the same marker in the `flaky` field.

## Reproduce a job

```text
nself ci runs reproduce <job> [--run <run|latest>] [--check] [--shell] [--json] [--repo dir]
```

Reproduce takes the newest stored run that holds the job, rebuilds its inputs from the evidence (revision, trigger, pipeline, isolation), and prints a **drift report** before doing anything. The drift kinds are a tool whose version changed, a tool that is missing, and a platform that differs.

- A platform that differs is refused (`E634`) before anything runs, and the message names both platforms.
- If the original evidence was unbound, only the recorded revision is reproduced and you get a warning (`E635`): uncommitted changes are never reproduced.
- `--check` stops after the report.
- `--shell` prepares the worktree and runs nothing, and keeps it for you. Remove it with `git worktree remove` when you finish.
- Otherwise it makes a temporary worktree at the recorded revision, runs the one job through the normal engine, and appends a link line to the state directory under `reproductions/`. The line records both outcomes and `reproduced: true` when the result and the failure class match. The temporary worktree is removed on error, panic and cancel, and a cancelled reproduction writes no link.

Services appear in the report only after the run, not before.

```bash
mkdir broken && cd broken && git init -q
git config user.email docs@example.com && git config user.name docs
mkdir .nself && cat > .nself/ci.yaml <<'CFG'
version: 1
jobs:
  build:
    kind: test
    run: [sh, -c, "echo building; echo boom >&2; exit 3"]
CFG
git add -A && git commit -q -m init
rc=0
nself ci --no-status || rc=$?
[ "$rc" -ne 0 ]
nself ci logs build
nself ci runs list
nself ci runs reproduce build --check
```

## Common problems

| Symptom | Look at |
|---|---|
| `E614` before any job runs | a required tool is not on `PATH`; the message names it. Install it or run with `--container`. |
| `E620` on a config you did not touch | the file is validated strictly; the message names the line and key. See [[CI configuration and policy|guides/ci/Configuration-and-Policy]]. |
| `E812` from the secret scan | a `.gitleaksignore` entry has no review comment; add one with `nself ci config ignore add`. |
| A required job is `unplaceable.platform` | it needs a platform you have no node for; see [[Nodes and Runners|guides/ci/Nodes-and-Runners]] |
| `E637` from a service job | no container runtime is reachable; start Docker, Colima or the service |
| `E646` from an ad hoc command | the default is a container and none is available; use `--isolation process` knowingly |
| The run is red but every check passed | a required job ended in error, or a required job was skipped; read `nself ci plan` and the evidence `result` |
| `E606` opening the store | the store was written by a newer binary; upgrade the plugin |
| A run stays `running` after a crash | start any run; recovery closes runs with no live coordinator, or run `nself ci runs wait <run>` |
| `nself ci runs wait` exits 10 | the run is still going; call it again |
| A detached run produced `E648` | the run could not be queued; the message ends with a redacted tail of what it printed |
| Disk use grows | `nself ci runs prune --dry-run`, then without `--dry-run`; see retention in [[Getting started with nself ci|guides/ci/Getting-Started]] |

If you are stuck on a node rather than on a job, the lost-lease, drain and enrollment problems are in [[Nodes and Runners|guides/ci/Nodes-and-Runners]]. For trust and secrets questions, see [[CI trust, security and privacy|guides/ci/Trust-Security-and-Privacy]].
