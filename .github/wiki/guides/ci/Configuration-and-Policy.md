# CI configuration and policy

<!-- docmap: cap:docs.ci-core -->

Two things decide what `nself ci` does: the **configuration** in your repository, which says what to run, and the **policy** on your machine and your branches, which says what is allowed. This page covers both. The commands and codes it names are in the generated [[CI reference|guides/ci/Reference]].

## The configuration file

The file is `.nself/ci.yaml` at the repository root. It is optional. With no file, `nself ci` detects your stacks and plans the standard checks. Declare any job and detection turns off for the jobs you declare; to tune a detected check instead, use `checks:` (below).

```bash
mkdir proj && cd proj && git init -q
git config user.email docs@example.com && git config user.name docs
mkdir .nself && cat > .nself/ci.yaml <<'EOF'
version: 1
jobs:
  lint:
    kind: static
    run: [sh, -c, "echo lint"]
    timeout: 5m
  unit:
    kind: test
    run: [sh, -c, "echo test"]
    deps: [lint]
    retry:
      infra_max: 2
EOF
git add -A && git commit -q -m init
nself ci config show
nself ci plan
```

`config show` prints every effective value with its source: a flag, an environment variable, the file or a built-in default. Add `--json` for the same data as a document, where each value carries `value`, `source` and `origin`.

**Precedence is flag, then environment, then file, then default.** That holds for every key, the timeout included: `--timeout` or `NSELF_CI_TIMEOUT` overrides a job's timeout in the file.

**The file is validated strictly.** Unknown keys, wrong types, unsafe paths and unpinned images are refused with `E620`, naming the file, the line and a JSON pointer to the key. The message is built from the schema and never repeats the offending value, so a secret pasted into the wrong place is not echoed. Other rules:

- Job, service and pipeline names, `workdir`, `when.paths`, `drills_dir` and cache and artifact paths must be safe paths inside the repository.
- `container.image` must be pinned by digest.
- Environment names must be variable names. Files are read only when regular and at most one mebibyte.
- A leftover legacy `.ci.yaml` is `E621`. A git-ignored `.nself/ci.yaml` is `E623`, and the message prints the lines to add to `.gitignore`. To move from the legacy file, see [[Migrating to the new CI engine|guides/ci/Migration]].

## Jobs

A job is one command with a kind, and everything the planner needs to place and judge it.

| Key | Meaning |
|---|---|
| `kind` | `static`, `build`, `test`, `quality`, `advanced`, `release` or `drill` |
| `run` | the command as a list; it is exec'd as given, with no shell unless you write one |
| `workdir` | directory inside the repository the job runs in |
| `deps` | jobs that must finish first; a failed dependency skips the job with `dep.failed` |
| `needs_services` | disposable services the job uses; see [Services](#services) |
| `matrix.platform` | a list of platforms such as `darwin/arm64`; one planned job per platform |
| `required` | `true` by default; an optional job cannot fail the run |
| `allow_failure` | the job runs and is reported, and its failure does not fail the run |
| `idempotent` | `false` marks a job that must never be retried automatically |
| `timeout` | a duration such as `5m` |
| `retry.infra_max` | how many times an infrastructure failure may be retried |
| `priority`, `path` | `low`, `normal`, `high`; `fast` or `deep` |
| `when` | `branches`, `paths`, `triggers` and `env_present` conditions |
| `env`, `secrets` | environment values, and names of secrets the job may read |
| `isolation`, `container` | the minimum isolation level; an image pinned by digest |
| `artifacts`, `cache` | outputs to keep, and cache keys and paths |
| `requires` | runner requirements: `labels`, `tools`, `accelerators`, `cpu`, `mem_mb` |

**Every job has a reason.** `nself ci plan` shows, for each job, whether it is included or skipped and the reason code: `pipeline.excluded`, `selection.affected`, `when.paths`, `when.env_missing` and so on. `nself ci explain <job>` shows where a job would be placed and why. The full list of reason codes is generated in the [[CI reference|guides/ci/Reference]].

**Affected selection.** When a base commit is known, a job whose `when.paths` do not match the change is left out, except that a default-branch or release plan always keeps every required deep job, even one outside the named pipeline. With no base the whole plan runs, with the reason `selection.no_base`. A job that does not run always has a reason code, so a skip is never silent.

**Matrices come from your support policy.** `support:` lists the platforms you support, and a matrix is expanded from it, never invented.

### Requirements and Xcode

`requires` narrows which runners may take a job. A job that needs Xcode names the `xcode` tool and the platform, and the planner only places it on a Mac where `xcodebuild -version` actually runs. The Command Line Tools alone do not count, because they ship a stub:

```bash
mkdir mac && cd mac && git init -q
git config user.email docs@example.com && git config user.name docs
mkdir .nself && cat > .nself/ci.yaml <<'EOF'
version: 1
jobs:
  ui-test:
    kind: test
    run: [xcodebuild, test, -scheme, App, -destination, "platform=macOS"]
    matrix:
      platform: [darwin/arm64]
    requires:
      tools: [xcode]
    timeout: 20m
EOF
git add -A && git commit -q -m init
nself ci config show
```

Then, on the machine you care about, see where the job would go:

```text
nself ci plan
nself ci explain ui-test
```

On a machine that cannot satisfy the platform or the tool, the planner reports the job with an `unplaceable.*` reason such as `unplaceable.platform` or `unplaceable.tool` instead of starting it, which is the point: it waits for a Mac node. Detected Swift projects get this requirement automatically. See [[Nodes and Runners|guides/ci/Nodes-and-Runners]] for adding a Mac.

## Checks

When you declare no jobs, the catalogue plans checks per detected stack. Each check is either required, so a failure fails the run, or advisory, so it is reported but does not.

| Stack | Required by default | Advisory by default |
|---|---|---|
| Go | `go.fmt`, `go.vet`, `go.test` | `go.govulncheck`, and a coverage floor when you configure one |
| Node | `node.lint`, `node.typecheck`, `node.test`, `node.build` | |
| Rust | `rust.clippy`, `rust.test` | |
| Flutter | `flutter.analyze`, `flutter.test` | |
| Swift | `swift-build`, `swift-test`, `xcode-test` | |
| Python | `python-test` | `python-lint` (ruff), `python-types` (mypy, planned only when mypy is configured) |
| Containers | | `container.hadolint`, `container.trivy.config` |
| Any | `secrets.gitleaks` | `security.trivy.fs` |

The `container`, `release` and `strict` modes require the advisory checks as well, whatever `required:` says. The Python lint and type checks become required in the same modes and under `--container`.

A missing tool depends on the check. A missing required tool, or an unsupported Node package manager, ends the check with `E614` and a remediation line. A missing advisory tool skips the check with `advisory.tool_missing`, and unavailable Trivy data skips it with `advisory.data_offline`.

Tune a check under `checks:`: `required`, `env`, `secrets`, `coverage_floor` and, for the truth check, `rules`. Turn off an ecosystem with `detect.disable`, and point detection at subdirectories with `detect.roots`.

### Node preparation

Before a Node check runs, the job finds the lockfile in the working directory or above, and checks the declared package manager's major version against the one on `PATH`. A mismatch is `E811` and the job does not retry. If there is no `node_modules`, it runs a frozen install: `npm ci`, `pnpm install --frozen-lockfile`, `yarn --immutable` or its older form, or `bun --frozen-lockfile`, by lockfile. Caches and the pnpm store live inside the job workspace. Astro projects run `astro sync` before typecheck and build. A failed install is classified as a code failure when the lockfile is out of sync or a lifecycle script failed, and as an infrastructure failure when the registry or disk was the cause, which is retried. See [[CI troubleshooting|guides/ci/Troubleshooting]].

### The secret scan

`secrets.gitleaks` honours `.gitleaksignore`, with one rule: every entry needs a review comment on the same line or the line before.

```text
# Reviewed 2026-10-29 by alice: a documented test value, not a credential
<fingerprint>
```

An entry without one fails the check with `E812`, which names line numbers and never prints content. Add entries with `nself ci config ignore add <fingerprint> --reason <text> [--by <who>]`. A run never adds lines itself. A missing `gitleaks` is `E614`, not a skip.

### The truth check

The truth check keeps retired names, dead hosts and banned strings out of your tree. It makes no network calls. Rule files are listed under `checks.truth.rules` in `.nself/ci.yaml`, as repository-relative paths; a rule file may `include` others to a limited depth, and cycles are refused.

A rule file has `version: 1` and these parts:

- `rules[]`, each with an `id`, a `kind`, a `message` and optional `paths` globs. The kinds are `banned-literal` (a `literal`), `retired-term` (a `term`, matched case-insensitively as a whole term) and `dead-host` (a list of SHA-256 hashes, so the file does not publish the host).
- `skip[]`, globs that are never scanned.
- `allow[]`, each naming a `rule` (an id, or a glob such as `retired-*`), the `paths` it covers and a `reason` of at least eight characters.

Unknown keys, duplicate ids, bad hashes, inert globs, extra YAML documents and symlinked rule files are `E644`, naming the file and the key. A dead-host rule stores the hash of the lower-cased host. The engine hashes every address and host name token on a line and compares the whole token, so a sub-domain is a different host.

The scan covers regular files under the root except `.git`, `node_modules`, the rule files themselves and your `skip` globs. A file over four mebibytes, or with a NUL byte, is skipped and counted in the summary; a tree with nothing to scan is an error. The evidence holds one check per rule kind. Findings print `path:line`, the rule id and the message, and a dead-host finding prints only a hash prefix.

## Services

A job that needs a database or cache names it, and gets a throwaway nSelf project for that attempt.

```text
services:
  db:
    component: postgres
    version: "16"
jobs:
  integration:
    kind: test
    run: [go, test, ./...]
    needs_services: [db]
```

A name is either declared under `services:` or is the name of a component. `--service <name>` adds one to a single run. Each attempt gets its own project on random loopback ports with random credentials, and the job sees these variables, with the service name upper-cased: `NSELF_CI_SERVICE_<NAME>_HOST`, `_PORT`, `_USER`, `_PASSWORD`, `_DB` and `_URL`, the same set under `_NET_` for a container job, and `NSELF_CI_SERVICE_NETWORK`.

Services need Docker and an nSelf with the `config services` command: `E637` means no Docker, `E638` means the CLI is too old (run `nself update`), `E639` means an unknown service, a duplicate component or a bad version, and `E636` means provisioning or teardown failed. Services are torn down on pass, fail, cancel and timeout, and after a hard kill the next run sweeps them. Services need the v1.5 engine.

## Pipelines, schedules and drills

A named pipeline picks a set of jobs for a set of triggers:

```bash
mkdir sched && cd sched && git init -q
git config user.email docs@example.com && git config user.name docs
mkdir -p .nself scripts/drill/restore
printf '#!/bin/sh\necho restored\n' > scripts/drill/restore/run
chmod +x scripts/drill/restore/run
cat > .nself/ci.yaml <<'EOF'
version: 1
jobs:
  report:
    kind: quality
    run: [sh, -c, "echo report"]
    required: false
pipelines:
  nightly:
    triggers: [schedule, manual]
    schedule: "0 3 * * *"
    max_age: 36h
    drills_dir: scripts/drill
    jobs: [report]
EOF
git add -A && git commit -q -m init
nself ci plan --pipeline nightly
```

`nself ci plan --trigger schedule` plans the pipelines that list the trigger. A **drill** is a directory under `drills_dir` with an executable `run`; each becomes a job named `drill-<dir>`, in a stable order. The exit code is the result. A drill may print one `ci.evidence` document as a single line on stdout, which is validated and nested as the job's checks. A config job with the same name replaces the discovered one, which is how a drill gets its own timeout, since discovered drills use the default.

`max_age` makes a silent job visible. `nself ci evidence --pipeline <name>` reports each job's last passing run and marks a job `overdue`, with exit code 10, when it is older than `max_age`. A pipeline without `max_age` is never overdue. A scheduled pipeline on a throwaway hosted runner has no store, so there staleness shows as a missing workflow run. The org-neutral workflow for it is `free/ci/templates/scheduled-pipeline.yml`.

## Policy

Policy is not in the repository. It lives in your user configuration, in a team coordinator's configuration, and on the host. Read it with `nself ci policy show` and change it with `policy set`, `policy unset` and `policy usage`. Trust, privacy and spend keys are covered in [[CI trust, security and privacy|guides/ci/Trust-Security-and-Privacy]], scheduling keys in [[CI scheduling and cost|guides/ci/Scheduling-and-Cost]].

### Per-run values

Three flags change policy for a single run: `--prefer`, `--max-queue` and `--runner`, with `--where` for the run target. They apply to bare runs only; `nself ci plan` ignores them.

- A per-run value can only **tighten**. It is merged last and refused if it would loosen a trust or privacy key. A refusal is `E819`, shown as a run note, and the key is ignored.
- Spend and secret keys cannot be set per run.
- A key that is unset may be filled only in the scheduling area, for the runner, operating system, architecture and allowed providers, because an unset trust list means "deny all".
- `--prefer` is the exception to tighten-only: any preset in the closed set is accepted, so you can override a protected branch's preset for one run.
- `--runner` pins only runners that your dispatcher knows; before nodes are enrolled that is the local machine.
- An interactive run, one started from a real terminal with no trigger and no `NSELF_INVOKED_BY`, waits at most ten minutes for a runner by default. A scripted run does not get that cap, and a `max_queue` in your policy longer than ten minutes stands. Git Bash terminals and runs wrapped by an agent count as scripted.

The base policy digest is what evidence records, so `evidence verify` still matches a run that used per-run values.

### Branch protection, hook and audit

`nself ci policy protect` makes `nself-ci` a required check on a branch, safely, and gives you the pieces to undo it.

```text
nself ci policy protect --repo owner/repo --branch main --dry-run
nself ci policy protect --repo owner/repo --branch main
nself ci policy protect --repo owner/repo --branch main --restore <snapshot>
nself ci policy protect --hook
nself ci policy protect --audit 30
```

- **Dry run** prints the exact request body. Existing required checks keep their pinned app, the `nself-ci` check is appended (override the app with `--app-id`), every other field is replayed from the live answer, and status checks are always sent as `checks`, never as `contexts`. A protection block with an unknown field, or with required signatures on, refuses the plan. An unprotected branch gets protection that holds only the check.
- **Apply** writes a snapshot of the current protection first, to `$NSELF_CI_HOME/protect/` with mode 0600 and never overwriting a file. It sends the body from a file, never on the command line, then reads the result back and compares it, ignoring the order of checks.
- **Restore** saves what it replaces, then puts the snapshot back. A snapshot of an unprotected branch cannot be restored; removing protection is a manual step.
- If the host's plan has no branch protection, the command prints the hook and audit equivalents and exits 3 (`E643`). Other 403 responses, such as a rate limit or a token without rights, are ordinary errors.
- **Hook** installs a pre-push hook for the default branch at git's own hooks path. It runs `NSELF_V15=1 nself ci evidence verify <sha> --json` and refuses the push on any non-zero exit: missing, stale, invalid, unbound or failing evidence. It also refuses when neither `nself` nor `nself-ci` is on the path. An existing hook is never overwritten (`E642` prints the line to chain it). `git push --no-verify` is your override.
- **Audit** lists commits on the default branch, newest first, that have no green `nself-ci` status and no green check run from the pinned app, and exits 1 when there are any.

Exit codes: 0 success, 1 failure or missing commits or `E642`, 2 infrastructure, 3 `E643`.

### The workflow template

Hosted runs can produce the same check from a workflow. Copy `free/ci/templates/nself-ci.yml` to `.github/workflows/nself-ci.yml`, set the three release values it asks for (the plugin version, the release URL and its SHA-256), add setup for your own toolchain where it says so, and then run `policy protect`. Nothing is downloaded until all three values are set.

The template keeps rules that make the gate honest: the job is named exactly `nself-ci`, because branch protection matches that name; it has no `paths:` filter, no matrix and no cancel-in-progress, because a skipped or cancelled required check blocks merges or hides a red run; its actions are pinned by commit; it holds read-only repository permissions and posts no commit status; and the binary is the pinned release, never one built from the checkout under test. A check run, unlike a commit status, is also produced on fork and Dependabot pull requests, whose tokens are read-only. A pull request that edits the workflow controls its own check run, which is inherent to hosted runners, so protect `.github/workflows` with code owners as well.
