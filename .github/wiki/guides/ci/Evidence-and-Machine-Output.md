# Evidence and machine output

<!-- docmap: cap:docs.ci-core -->

Every run of `nself ci` leaves a record you can read, verify and hand to another program. This page covers that record, the machine-readable output of the commands, how to start a run and collect it later, and ad hoc commands. For the first run, see [[Getting started with nself ci|guides/ci/Getting-Started]].

## Evidence

A run's evidence is one JSON document of schema `ci.evidence/v1`. It states which revision was checked, which inputs the result covers, which runner produced it, what each check concluded and how the document itself was produced. The tables at the end of this page are generated from the schema and list every field.

Read it with:

- `nself ci evidence <run> --json`, the evidence of one run.
- `nself ci evidence --pipeline <name>`, the last passing run of each job of a scheduled pipeline, with `overdue` marked when it is older than the pipeline's `max_age`.
- `nself ci evidence verify <revision> [--json]`, a yes or no for a commit under the policy in force.

### Binding and freshness

**Binding** says whether the result is about a commit. It is `bound` when the working tree matched the commit throughout the run. It is `unbound`, with a reason, when the tree was dirty or changed during the run. Hidden edits count: the tree is read again when the run completes. Tracked files marked `skip-worktree` or `assume-unchanged` unbind the run, except a `skip-worktree` path that is absent because of a sparse checkout. Ignored files such as `.env` or `go.work` stay outside the binding, and jobs still see them, which is a known limitation.

**Freshness** says whether the evidence still describes the present: `current`, `stale` or `unknown`. It is computed when you read the evidence, never stored, and a document's own claim is never trusted. Evidence goes stale when the inputs it covers have changed, when it was made under another policy, or when the working tree has uncommitted changes. It is `unknown` when the inputs cannot be read, and unknown is never treated as current.

### Verify

`evidence verify <revision>` returns a verdict you can act on in a script or a hook. It serves stored evidence only when that evidence is a finished run, with a full selection, of the default pipeline, for this repository, under the policy in force. Anything else is rerun in a temporary worktree with hooks, file-system monitors and LFS smudging turned off, and always with a full selection. An interrupted or errored rerun exits 2. The verdict is the exit code: 0 for current, bound, passing evidence; 1 when the evidence fails; 10 when it is stale, which includes another policy; 11 when it is invalid, which includes another repository; and 12 when it is unbound. Evidence from ad hoc commands and drills never gates a commit. The whole exit-code table is generated in the [[CI reference|guides/ci/Reference]].

`--file <path>` verifies a document you were given. A file alone proves nothing about where it came from, so a file with no local record of its run is refused with exit 11 when it would otherwise pass. A document signed by a remote runner goes through the registered verifier, and its repository and policy digest are checked.

## Machine output

Add `--json` to any command and it prints one envelope on stdout:

```text
{ "schema_version": "1", "command": "ci runs wait", "data": { ... } }
{ "schema_version": "1", "command": "ci plan", "error": { "code": "E620", "message": "...", "exit_code": 1, "class": "user" } }
```

`data` is the result, and `error` replaces it on failure with a stable code, the exit code and a class. Programs should read the code and the exit code, not the message.

A run also emits a stream of events with schema `ci.events/v1`, one JSON object per line, in order: `queued`, `planned`, `leased`, `started`, `log`, `check_result`, `retrying`, `cancelled`, `needs_operator`, `finished` and `evidence`. Each carries a sequence number, a time, the run and, when it applies, the job. `--stream` prints it live. One writer produces the stream; log records are best effort and are dropped after a bounded wait rather than slowing a job, with no gap in the sequence. A failed stream flushes its error line first.

Usage errors exit 1 across the tool. The full table of exit codes and error codes is generated in the [[CI reference|guides/ci/Reference]].

## Start a run and come back later

A foreground run holds your terminal. An agent, a script or an editor often cannot wait, so a run can be started in the background and collected later. The handle is a document of schema `ci.run-handle/v1`, which is experimental until the next major version.

```text
NSELF_V15=1 nself ci --detach --json [flags] [repo] [-- <cmd>]
```

`--detach` starts the run in a new session, detached from your terminal, and prints the handle once the run is queued. The handle gives the run id and argument lists for the next steps: `wait`, `follow`, `evidence`, `cancel` and `logs`. If the run cannot be queued within thirty seconds, the command fails with `E648` and a reason (`spawn`, `no_queued_event` or `child_exit`); it ends the child and shows a redacted tail of what the child printed.

Then choose how to collect it:

- **Poll.** `nself ci runs wait <run> [--timeout d] [--json]` waits up to the timeout, and exits 0 when the run is over or 10 while it is still running. When it exits 0, read `data.state` for the outcome: `passed`, `failed`, `cancelled` or `lost`. The timeout defaults to four minutes and is shortened to fit `NSELF_INVOKE_DEADLINE_MS` when a caller sets one, so a tool with its own time limit never hangs. Exit 10 means call it again.
- **Stream.** `nself ci runs follow <run> [--from-seq n]` prints the events of the run as NDJSON. It replays what happened and tails the run until the `result` line. A run with no stored stream is rebuilt from the store in the same order, so the sequence never depends on when you looked. A run that died without a result ends with an `E648` error line and exit 2.
- **Read the result.** `nself ci evidence <run> --json`.
- **Stop it.** `nself ci runs cancel <run>`.

A complete example, which starts a run, waits for it with a bounded loop, then reads the events and the evidence:

```bash
mkdir async && cd async && git init -q
git config user.email docs@example.com && git config user.name docs
mkdir .nself && cat > .nself/ci.yaml <<'CFG'
version: 1
jobs:
  show:
    kind: test
    run: [sh, -c, "echo hello"]
CFG
git add -A && git commit -q -m init
nself ci --detach --json --no-status > handle.json
run=$(sed -n 's/.*"run_id": "\(.*\)",/\1/p' handle.json)
rc=10
while [ "$rc" -eq 10 ]; do
  rc=0
  nself ci runs wait "$run" --timeout 30s --json > wait.json || rc=$?
done
[ "$rc" -eq 0 ]
grep -q '"state": "passed"' wait.json
nself ci runs follow "$run" > events.ndjson
grep -q '"type":"finished"' events.ndjson
nself ci evidence "$run" --json > evidence.json
grep -q '"result": "pass"' evidence.json
```

A few limits to know. Streams and pid files of runs that no longer exist are removed by `runs prune` and by every `--detach`. A system that kills every process in a control group when the caller exits can still kill the detached child; start the run from a context that outlives the caller if you rely on it. `--coordinator` is read from the arguments or `NSELF_CI_COORDINATOR`.

## Ad hoc commands

`nself ci [flags] -- <command> [args]` runs one command through the same machinery: a real run with a store entry, a log and an evidence document, but a pipeline of kind `adhoc` that never gates anything and never posts a status. It is the safe way to let a person or an agent run a command with a record.

| Flag | Meaning |
|---|---|
| `--isolation container\|process\|sandboxed-container` | the isolation level; `container` is the default and needs Docker |
| `--image <ref@digest>` | the container image, pinned by digest |
| `--network none\|restricted\|internet\|lan` | the network scope; none by default, or the service network when `--service` is given |
| `--service <name>` | a disposable service the command can reach |
| `--trust owner\|collaborator\|internal\|untrusted` | the trust class of the command's source |

The command is exec'd as given. The workspace is mounted read-only, scratch space is a temporary file system, and the environment is only a language setting, a home and a temporary directory.

**What isolation does and does not do.** A container reduces what a command can touch, and it is not a boundary against hostile code. `--isolation process` runs on the host with the host network and a writable copy of the workspace, prints a warning and records the network scope as `lan`. An `untrusted` source needs the sandboxed level, and a host without it refuses the command with `E647` and runs nothing. `--network internet` is refused until the network planner for the level allows it, and is never widened silently. Your model of risk is in [[CI trust, security and privacy|guides/ci/Trust-Security-and-Privacy]].

**What it records.** The evidence names the isolation level actually used, the requested trust class, the redacted command line and the network scope. Redaction of the command line goes by the shape and name of an argument, so a secret passed as a bare positional argument is not recognised. Do not pass secrets that way.

## Evidence fields

The tables below are generated from the schema. Change the model, then regenerate.

<!-- BEGIN GENERATED:evidence-fields -->
#### Evidence document (root)

| Field | Type | Required | Values |
|---|---|---|---|
| `schema` | string | yes |  |
| `revision` | string | yes |  |
| `repo_id` | string | yes |  |
| `run_id` | string | yes |  |
| `pipeline` | `PipelineRef` | yes |  |
| `job` | string or null | no |  |
| `attempt` | `AttemptRef` or null | no |  |
| `trigger` | string | yes | `push`, `pull_request`, `tag`, `manual`, `schedule`, `api`, `local` |
| `source_trust` | string | yes | `owner`, `collaborator`, `internal`, `untrusted` |
| `policy_digest` | string | yes |  |
| `input_digests` | `InputDigests` | yes |  |
| `worktree_state` | string | yes | `clean`, `dirty` |
| `tracked_changes_digest` | string or null | yes |  |
| `binding` | `Binding` | yes |  |
| `runner` | `RunnerRef` | yes |  |
| `producer` | `Producer` | yes |  |
| `timings` | `Timings` | yes |  |
| `tool_versions` | map of string | yes |  |
| `checks` | list of `Check` | yes |  |
| `artifacts` | list of `Artifact` | yes |  |
| `retries` | list of `Retry` | yes |  |
| `selection` | `Selection` | yes |  |
| `placement_ref` | string or null | no |  |
| `result` | string | yes | `pass`, `fail`, `error`, `cancelled` |
| `counts` | `Counts` | yes |  |
| `freshness` | `Freshness` | yes |  |
| `signature` | `Signature` | yes |  |
| `provider_attestation` | `ProviderAttestation` or null | no |  |
| `attempts` | list of `AttemptEvidence` | no |  |
| `errors` | list of `EvidenceError` | no |  |

#### `PipelineRef`

| Field | Type | Required | Values |
|---|---|---|---|
| `id` | string | yes |  |
| `name` | string | yes |  |
| `kind` | string | yes | `default`, `named`, `drill`, `adhoc` |

#### `AttemptRef`

| Field | Type | Required | Values |
|---|---|---|---|
| `id` | string | yes |  |
| `n` | integer | yes |  |

#### `InputDigests`

| Field | Type | Required | Values |
|---|---|---|---|
| `config` | string | yes |  |
| `lockfiles` | list of `DigestRef` | yes |  |
| `generated` | list of `DigestRef` | yes |  |

#### `Binding`

| Field | Type | Required | Values |
|---|---|---|---|
| `state` | string | yes | `bound`, `unbound` |
| `reason` | string or null | yes | `dirty`, `sha_mismatch` |

#### `RunnerRef`

| Field | Type | Required | Values |
|---|---|---|---|
| `id` | string | yes |  |
| `provider` | string | yes |  |
| `platform` | string | yes |  |
| `isolation` | string | yes | `process`, `container`, `sandboxed-container`, `vm`, `ephemeral-vm`, `hosted-disposable` |

#### `Producer`

| Field | Type | Required | Values |
|---|---|---|---|
| `kind` | string | yes | `coordinator-recorded`, `signed-remote`, `self-reported` |
| `version` | string | yes |  |

#### `Timings`

| Field | Type | Required | Values |
|---|---|---|---|
| `queued_at` | string or null | yes |  |
| `started_at` | string or null | yes |  |
| `finished_at` | string or null | yes |  |
| `duration_ms` | integer or null | yes |  |

#### `Check`

| Field | Type | Required | Values |
|---|---|---|---|
| `id` | string | yes |  |
| `job_id` | string | yes |  |
| `kind` | string | yes |  |
| `tool` | string | yes |  |
| `tool_version` | string or null | yes |  |
| `required` | boolean | yes |  |
| `substantive` | boolean | yes |  |
| `allow_failure` | boolean | no |  |
| `result` | string | yes | `pass`, `fail`, `skip`, `error` |
| `reason` | string | no | `pipeline.excluded`, `selection.no_base`, `selection.full_required`, `selection.affected`, `when.branch`, `when.paths`, `when.trigger`, `when.env_missing`, `dep.failed`, `fail_fast.cancelled`, `unplaceable.platform`, `unplaceable.docker`, `unplaceable.tool`, `unplaceable.service`, `unplaceable.label`, `unplaceable.resources`, `admission.battery`, `admission.reservation`, `advisory.tool_missing`, `advisory.data_offline` |
| `failure_class` | string | no | `code`, `infra`, `timeout`, `cancelled`, `lost` |
| `findings` | integer | yes |  |
| `excerpt` | string | yes |  |
| `log_ref` | string or null | yes |  |
| `exit` | integer or null | no |  |
| `diagnosis` | string | no |  |
| `remediation` | string | no |  |
| `flaky` | `Flaky` or null | no |  |

#### `Artifact`

| Field | Type | Required | Values |
|---|---|---|---|
| `path` | string | yes |  |
| `sha256` | string | yes |  |
| `size` | integer | yes |  |
| `committed` | boolean | yes |  |

#### `Retry`

| Field | Type | Required | Values |
|---|---|---|---|
| `job_id` | string | yes |  |
| `attempt` | integer | yes |  |
| `class` | string | yes | `code`, `infra`, `timeout`, `cancelled`, `lost` |
| `diagnosis` | string | no |  |
| `excerpt` | string | no |  |

#### `Selection`

| Field | Type | Required | Values |
|---|---|---|---|
| `mode` | string | yes | `full`, `affected` |
| `reason` | string | yes | `pipeline.excluded`, `selection.no_base`, `selection.full_required`, `selection.affected`, `when.branch`, `when.paths`, `when.trigger`, `when.env_missing`, `dep.failed`, `fail_fast.cancelled`, `unplaceable.platform`, `unplaceable.docker`, `unplaceable.tool`, `unplaceable.service`, `unplaceable.label`, `unplaceable.resources`, `admission.battery`, `admission.reservation`, `advisory.tool_missing`, `advisory.data_offline` |
| `digest` | string | yes |  |

#### `Counts`

| Field | Type | Required | Values |
|---|---|---|---|
| `pass` | integer | yes |  |
| `fail` | integer | yes |  |
| `skip` | integer | yes |  |
| `error` | integer | yes |  |
| `total` | integer | yes |  |

#### `Freshness`

| Field | Type | Required | Values |
|---|---|---|---|
| `state` | string | yes | `current`, `stale`, `unknown` |
| `reason` | string or null | yes |  |

#### `Signature`

| Field | Type | Required | Values |
|---|---|---|---|
| `kind` | string | yes | `coordinator-recorded`, `runner-ed25519` |
| `key_id` | string or null | no |  |
| `value` | string or null | no |  |

#### `ProviderAttestation`

| Field | Type | Required | Values |
|---|---|---|---|
| `provider` | string | yes |  |
| `run_id` | string | yes |  |
| `run_attempt` | integer | yes |  |
| `ref` | string | yes |  |
| `workflow_path` | string | yes |  |
| `workflow_sha` | string | yes |  |
| `workflow_digest` | string | yes |  |
| `template_version` | string | yes |  |
| `nonce` | string | yes |  |
| `artifact_id` | string | yes |  |
| `artifact_sha256` | string | yes |  |
| `verified_at` | string | yes |  |

#### `AttemptEvidence`

| Field | Type | Required | Values |
|---|---|---|---|
| `job_id` | string | yes |  |
| `attempt` | integer | yes |  |
| `runner` | `AttemptRunner` | yes |  |
| `timings` | `AttemptTimings` or null | no |  |

#### `EvidenceError`

| Field | Type | Required | Values |
|---|---|---|---|
| `code` | string | yes |  |
| `job_id` | string | no |  |
| `attempt` | integer | no |  |
| `detail` | string | no |  |

#### `DigestRef`

| Field | Type | Required | Values |
|---|---|---|---|
| `path` | string | yes |  |
| `sha256` | string | yes |  |

#### `Flaky`

| Field | Type | Required | Values |
|---|---|---|---|
| `source` | string | yes |  |
| `passes` | integer | yes |  |
| `fails` | integer | yes |  |

#### `AttemptRunner`

| Field | Type | Required | Values |
|---|---|---|---|
| `kind` | string | yes |  |
| `tier` | string | yes |  |
| `pool` | string | yes |  |
| `rank` | integer | yes |  |

#### `AttemptTimings`

| Field | Type | Required | Values |
|---|---|---|---|
| `queue_ms` | integer or null | no |  |
| `transfer_ms` | integer or null | no |  |
| `run_ms` | integer or null | no |  |
<!-- END GENERATED:evidence-fields -->
