# Walkthrough: several projects on one fleet

This page walks one fleet that serves two projects. Project A and project B share a Mac and a VPS. A second VPS belongs to project A only. Every project keeps its own queue share, cache and statistics.

Names used below: `mac1` has Xcode, `vps-shared` serves both projects, `vps-a` serves project A only. `~/src/project-a` and `~/src/project-b` are the two checkouts. Commands that need real machines are shown as text, each with the test that runs it. The command and flag tables are in the generated [Reference](./Reference.md#commands).

## One pool for several projects

A node runs jobs only for the projects it is authorized for. Authorize by checkout directory or by repository id; a directory is turned into the same repository id that evidence uses.

<!-- proven-by: TestSeveralProjects -->
```text
nself ci nodes projects add mac1 ~/src/project-a ~/src/project-b
nself ci nodes projects add vps-shared ~/src/project-a ~/src/project-b
nself ci nodes projects list vps-shared
```

To take a project off a node, remove it. `--all` removes every project from that node.

<!-- proven-by: TestSeveralProjects -->
```text
nself ci nodes projects remove vps-shared <repo-id>
```

## A VPS for one project

Authorize the dedicated VPS for project A only. Project B's jobs never land there, even when it is idle.

<!-- proven-by: TestSeveralProjects -->
```text
nself ci nodes projects add vps-a ~/src/project-a
nself ci nodes projects list vps-a
```

A personal node is never chosen for a private repository it is not authorized for. Adding the project makes it eligible.

## Pinning a project with labels

Labels describe a node; a project can require extra labels for all its jobs. Give `vps-a` a label, then require that label at project scope for project A. Project A's jobs then need it on top of their own `requires.labels`.

<!-- proven-by: TestSeveralProjects -->
```text
nself ci nodes label mac1 xcode
nself ci nodes label vps-a project=a
nself ci policy set --scope project sched.overrides.labels '["project=a"]'
```

Run the `policy set` line inside project A's checkout. After it, all of A's jobs go to `vps-a`. A job of A that also needs `xcode` waits: `mac1` has Xcode but not `project=a`, and `vps-a` has the reverse. `explain` names the override as the reason.

<!-- proven-by: TestScenario6ExplainListsEveryAlternative -->
```text
nself ci explain
```

How overrides combine with job requirements is in [Scheduling and Cost](./Scheduling-and-Cost.md#overrides).

## Fairness and the queue

When the fleet is busy, the scheduler shares capacity between projects and never admits more than the fleet can hold. Interactive runs start before queued background work. `runs queue` shows each waiting run with its position, its band and the expected wait.

<!-- proven-by: TestRunsQueueListsPositionBandAndWait -->
```text
nself ci runs queue
```

The fairness rules and presets are in [Scheduling and Cost](./Scheduling-and-Cost.md#how-a-runner-is-chosen).

## Cache and artifacts stay separate

Each cache entry lives in a namespace made of the project, the trust class of the source and whether the ref is protected. The same key in another project's namespace neither blocks nor reveals the entry. Artifacts are stored per run, and runs belong to one project. Nothing needs to be set up for this. See [namespaces](./Cache-and-Artifacts.md#namespaces-per-trust-and-per-pr).

## Statistics per project

`runs stats` summarises runs, queue time and failures. `--project` limits it to one project; `--since` sets the window. A caller of a team coordinator sees only the projects it has a role on.

<!-- proven-by: TestStatsReadWindowAndProject -->
```text
nself ci runs stats --since 168h --project <owner/repo> --json
```

The fields are described in [CI health](./Scheduling-and-Cost.md#ci-health).

## Proven by

| Step | Test |
|---|---|
| Authorize, list and remove projects on a node; per-project VPS; label override | `TestSeveralProjects` |
| A personal node needs authorization for a private repo | `TestPersonalNodePrivateRepoOptIn` |
| Every alternative explained | `TestScenario6ExplainListsEveryAlternative` |
| Fair share under saturation | `TestScenario10SaturationStaysWithinCapacityAndFair` |
| Interactive before background | `TestScenario10InteractiveStartsBeforeQueuedBackground` |
| Queue position, band and wait | `TestRunsQueueListsPositionBandAndWait` |
| Cache namespaces do not leak | `TestSameKeyStringInAnotherNamespaceNeitherBlocksNorRevealsTheTrustedEntry` |
| Stats window and project filter | `TestStatsReadWindowAndProject` |
| Stats scoped to the caller's projects | `TestStatsScopedCallerSeesOnlyAllowedProjects` |
