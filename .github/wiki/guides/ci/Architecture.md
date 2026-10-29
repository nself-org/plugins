# CI architecture

<!-- docmap: cap:docs.ci-core -->

This page explains how `nself ci` is built: the path a run takes, the layers of the code, the local store that holds state, and the rules that keep each part replaceable. Read it to reason about behaviour. To use the tool, start with [[Getting started with nself ci|guides/ci/Getting-Started]].

## The path of a run

A run is a pure plan followed by an audited execution. Each step has one owner and one record.

1. **Detect.** Manifests and lockfiles become facts: which stacks exist, which tools they need, which services a compose file declares. Detection reads files and runs nothing.
2. **Plan.** The configuration, the facts, the check catalogue and the revision context produce a pipeline: a validated graph of jobs. Every job in it, and every job left out, carries a reason code. The planner is a pure function, which is why `nself ci plan` can explain a run without starting one.
3. **Place.** Each job is matched to a runner: this machine, a node you enrolled, or a hosted provider. Placement is covered in [[CI scheduling and cost|guides/ci/Scheduling-and-Cost]] and [[Nodes and Runners|guides/ci/Nodes-and-Runners]].
4. **Admit and lease.** The coordinator commits the job's demand against the host in one store transaction. If the host is full the job waits; it is never half-started.
5. **Execute.** The executor starts the job, enforces its timeout, passes its output through the redactor and kills the whole process tree on cancel.
6. **Record.** The result of each check, the inputs, the binding to the commit and the runner become one evidence document. [[Evidence and machine output|guides/ci/Evidence-and-Machine-Output]] describes it.

A job moves through a closed set of states: queued, leased, running, finalizing, then passed, failed, cancelled or lost, with retryable and needs-operator for the cases that need a second attempt or a person. The transitions are a table; nothing else moves a job.

## Layers

The code under `free/ci` is split into layers, and a test fails the build when a package imports a layer it may not. The layer file is [layers.yaml](../../../../free/ci/architecture/layers.yaml).

| Layer | Holds | Rule the test enforces |
|---|---|---|
| kernel | the data model, the schemas and the error and reason registries | imports no other package of the engine, and no `os` or `net` |
| pure | planning, detection, scoring, diagnosis, trust rules | imports no `os` and no `net` |
| execution | the process and container executor, the store | the only code that starts a process, and the only code that touches SQL |
| domain | everything that joins them: config, runs, reports, evidence, history, doctor, nodes, scheduling | may use the layers above; no package imports `cmd` |

Two consequences matter in practice:

- A pure package cannot touch the file system or the network, so planning and scoring are testable without a machine.
- There is exactly one place that starts a process or a container, the executor, and exactly one that speaks SQL, the store. Provisioning, ad hoc runs, drills and services all go through the executor, so redaction, timeouts and isolation are applied once and cannot be skipped by a new feature.

## State and the store

The engine keeps its state in one SQLite database named `state.db` in the state directory (`$NSELF_CI_HOME`). SQLite is a deliberate choice here. CI state is a local file owned by one user on one machine, not an application backend, so it does not use a database server. The driver is pure Go, built without cgo, so the same binary runs on every supported platform.

**Local disk only.** Put the state directory on a local disk. SQLite in WAL mode depends on file locking and shared memory that network file systems do not provide reliably. Do not point `NSELF_CI_HOME` at an NFS or SMB share, and do not share one directory between machines. Several machines share work through a coordinator, not through a file.

How the store is used:

- **WAL, one writer.** The database runs in write-ahead-log mode with one writer connection and a busy timeout of five seconds. Readers never block the writer. Foreign keys are enforced.
- **Aggregate interface.** Callers do not write SQL. They call operations such as register a coordinator, admit and lease a job, record evidence, transition a job, request a cancel, back up and restore. Each operation is one transaction.
- **Compare-and-set transitions.** A state change names the state it expects. If another writer got there first, the change is refused with an error rather than overwritten. This is why two processes can share a store safely.
- **Expand-only migrations.** Schema changes only add. Each package registers its own migration set, and a version table records what ran. A stored minimum reader version tells an older binary that it must not open a newer database: it stops with a clear error and asks you to upgrade, instead of misreading rows. A downgrade therefore fails closed.
- **At-most-once for releases.** A release job, and any job marked `idempotent: false`, is never retried on its own. After a failure it waits in `needs-operator`, and a retry is allowed only after an operator action has been written to the audit table. A reused idempotency key is refused.
- **Audit sealing.** Security-relevant actions append rows to an audit table. Sealing chains the rows with hashes, so a deleted or edited row breaks the chain. The store holds no secret values in audit rows. Verification is `nself ci serve admin audit --verify`.
- **Host admission.** Admission is a list of steps run inside the lease transaction: the host's reservations, the demand of running jobs and any budget. Each step can refuse. Because the check and the lease commit together, two coordinators cannot both take the last slot.
- **Backup and restore.** A backup is a consistent snapshot of the database written while it is in use. A restore replaces the database and fences any coordinator that was running against the old one, so a stale process cannot write into the restored state.

A coordinator is one process holding an identity in the store. A coordinator that stopped without finishing leaves a stale identity. On the next start the engine ends its in-flight attempts as lost or cancelled, closes runs that have no live coordinator, and sweeps containers it left behind.

## What lives outside the store

| Data | Where | Why |
|---|---|---|
| Logs | files under the state directory, one folder per attempt | large and append-only; redacted before they are written |
| Detached run streams | files under the state directory | the event stream of a run started with `--detach` |
| Branch-protection snapshots | files under the state directory, mode 0600 | the rollback for a policy change |
| Cache and artifacts | a content-addressed store | see [[Cache and artifacts|guides/ci/Cache-and-Artifacts]] |
| Configuration | `.nself/ci.yaml` in the repository | reviewed with the code it governs |

## Extension seams

Features plug in through hooks the run engine calls at fixed points, rather than by editing the engine. Examples are the policy resolver, the evidence builder, the report and status hooks, the services hook and the isolation level. A hook that panics is caught and the attempt ends with an error instead of taking the process down. This is how trust, scheduling and caching were added without changing how a job is planned or recorded.

## Design rules worth knowing

- **A pass is a claim with a proof.** A result is recorded as evidence that says exactly which inputs it covers, and it is bound to a commit only when the tree matched that commit. A dirty tree, or a tree that changed during the run, is recorded as unbound, and an unbound run posts no commit status.
- **Nothing hidden skips.** A job that does not run has a reason code. A required job that cannot run fails the run; it does not vanish.
- **Failures have a class.** Code, infrastructure, timeout, cancelled and lost are different, and only infrastructure failures are retried automatically. See [[CI troubleshooting|guides/ci/Troubleshooting]].
- **One config, one schema.** `.nself/ci.yaml` is validated against a generated schema, and the same document describes the plan, the evidence and the events.
- **Generated reference.** Every table of commands, codes and versions is generated from the code, so the [[CI reference|guides/ci/Reference]] cannot drift from the binary.
