# Walkthrough: a home lab

This page walks one real setup from start to finish: a Mac Studio on your desk runs the coordinator, two Mac Minis on the same LAN run jobs, and a small VPS you rent runs Linux jobs. Everything here is personal mode: you are the only person with access.

The page shows the order of steps and links the guide that explains each one. Commands that need real machines are shown as text, each with the test that runs it. The tables of commands, flags, codes and doctor checks live in the generated [Reference](./Reference.md#commands).

Names used below: `desk` is the Mac Studio, `mini1` and `mini2` are the Minis, `vps1` is the VPS at `203.0.113.20`. Replace them with your own.

## The desk Mac runs the coordinator

The coordinator keeps the run store, the node registry and the keys. In personal mode it starts with no extra flag. Its webhook and user API listeners stay on loopback by default; only the agent listener needs to be reachable from the Minis. The listener ports are described in [Nodes and Runners](./Nodes-and-Runners.md#team).

<!-- proven-by: TestServeModeFlagAndStart -->
```text
nself ci serve
```

A coordinator started without `--team` stays personal. Do not add `--team` here: the switch is permanent for that data directory, and a home lab used by one person does not need it.

## Adding the Minis

There are two ways to add a Mac. Use `nodes add --system` when you can reach the Mini over SSH. Use pairing when you cannot, or when you do not want the coordinator to hold an SSH key for it.

**Add over SSH.** `--system` installs the agent as a system service: a hidden agent user, one job user per slot, and a LaunchDaemon that runs the agent in pull mode. Get the host-key fingerprint from the Mini itself, not from the network.

<!-- proven-by: TestMacOSNode -->
```text
nself ci nodes add ci@mini1 --system --slots 2 --host-key <fingerprint> --agent-binary <path-to-nself-ci-agent> --project <owner/repo> --name mini1 --json
```

The bootstrap checks every account and folder it creates. Until jobs actually run through the job shim, the node is recorded as not UID-separated, and installing it grants no secret or signing rights. See [provisioning versus bootstrap](./Nodes-and-Runners.md#vps).

**Pair instead.** Run `nodes pair` on the desk Mac. It opens a short window and prints a `nself-ci-agent join` command, the coordinator fingerprint and a one-use code. Run the printed join command on the Mini, type the code there, and check that both fingerprints match before you approve.

<!-- proven-by: TestNodesPairEndToEnd -->
```text
nself ci nodes pair --label mini2 --agent-url https://desk.example:3864 --cert-ttl 30m
```

A paired Mini runs the same agent in pull mode: it only makes outbound connections to the coordinator. Never put the pairing code in a command line or a shared log.

## Labels: which jobs may run where

Give each node the labels your jobs ask for. The Minis have Xcode, so they get the `xcode` label.

<!-- proven-by: TestSeveralProjects -->
```text
nself ci nodes label mini1 xcode
nself ci nodes label mini2 xcode
```

A job that needs Xcode says so in `.nself/ci.yaml`. A runner is eligible only when it has every label the job asks for:

```yaml
jobs:
  ios:
    kind: test
    run: [xcodebuild, test]
    requires: {labels: [xcode]}
```

Check where each job would go before you push. `plan` shows the placement; `explain` lists every runner that was not chosen and why.

<!-- proven-by: TestPlanPlacementJSON -->
```text
nself ci plan --json
```

<!-- proven-by: TestScenario6ExplainListsEveryAlternative -->
```text
nself ci explain
```

The reasons (`labels.mismatch`, `tools.missing` and others) are listed in [Scheduling and Cost](./Scheduling-and-Cost.md#job-requirements-and-labels) and in the [reason table](./Reference.md#reason-codes).

## Reservations on the desk machine

If the desk Mac also takes jobs, keep part of it for yourself. Reservations are scheduler limits: CI work never uses the reserved CPU or memory. `--interactive protect` holds new work back while you are using the machine. `--on-battery` matters only for laptops; a desk machine can leave it at `allow`.

<!-- proven-by: TestNodesSetFlagsMayFollowOrPrecedeTheNodeAndKeepWhatIsNotGiven -->
```text
nself ci nodes set desk --reserve-cpu 2 --on-battery allow
nself ci nodes set desk --reserve-mem 2048
```

A reservation that would leave nothing for CI is refused. More controls are in [Nodes and Runners](./Nodes-and-Runners.md#lan).

## The personal VPS

A VPS is added the same way as a Mini, with its host key pinned. The SSH carrier forces strict host-key checking against the coordinator's own pin file, so a changed key stops the connection before any job runs. Connect as root or a user with passwordless sudo so `--system` can create the agent and job users.

<!-- proven-by: TestSystemInstallBootstrapsAndKeepsTheNodeUnseparated -->
```text
nself ci nodes add root@203.0.113.20 --system --slots 3 --host-key <fingerprint> --agent-binary <path-to-nself-ci-agent> --json
```

Never add a machine that serves your deployed apps. Deploy hosts are not eligible for CI; see [deploy hosts are ineligible](./Nodes-and-Runners.md#vps).

## Secrets on a node

A node gets no project secrets until you grant them. Grant the `project` class to the VPS if its jobs need project secrets. The `release` class needs a node dedicated to releases.

<!-- proven-by: TestGrantProjectCallsCliAndAudits -->
```text
nself ci nodes secrets grant vps1 --class project
```

Every grant and revoke is written to the audit log. The secret rules are in [Trust, Security and Privacy](./Trust-Security-and-Privacy.md#secrets).

## When a Mini sleeps

A sleeping or frozen Mini stops renewing its lease. It fences itself before the coordinator's lease expires, so it cannot finish the old job late. The coordinator marks the attempt lost and runs it again on another eligible node. If the Mini wakes and reports the old result, that result is refused. A job that failed on its own merits is not retried.

Mark machines that sleep on purpose as intermittent so the scheduler prefers always-on nodes:

<!-- proven-by: TestNodesSetJSONEnvelopeCarriesTheStoredSettings -->
```text
nself ci nodes set mini2 --reserve-mem 1024 --persistence intermittent --json
```

Lost leases and what to check are covered in [Troubleshooting](./Nodes-and-Runners.md#troubleshooting).

## Backing up the coordinator

Back up the desk Mac's data directory to a file. The archive does not contain the key-encryption key (KEK). Keep the KEK somewhere else; without it the archive cannot be restored, and a wrong KEK is refused before anything is written.

<!-- proven-by: TestBackupRestoreCommandsRoundTrip -->
```text
nself ci serve backup --data-dir <coordinator-data-dir> --out <archive> --json
nself ci serve restore --data-dir <new-data-dir> --in <archive> --kek <saved-kek-file> --json
```

Stop the coordinator before a restore. What restore keeps and resets is in [backup/restore and the KEK](./Nodes-and-Runners.md#team).

## Proven by

| Step | Test |
|---|---|
| Personal coordinator start, no `--team` | `TestServeModeFlagAndStart` |
| Add a Mac with `--system` (LaunchDaemon, users, layout) | `TestMacOSNode` |
| Pair a node and run the printed join | `TestNodesPairEndToEnd` |
| Labels and the placement they cause | `TestSeveralProjects` |
| Placement plan in JSON | `TestPlanPlacementJSON` |
| Every alternative explained | `TestScenario6ExplainListsEveryAlternative` |
| Reservations and battery policy | `TestNodesSetFlagsMayFollowOrPrecedeTheNodeAndKeepWhatIsNotGiven` |
| Add a Linux VPS with `--system` | `TestSystemInstallBootstrapsAndKeepsTheNodeUnseparated` |
| Grant project secrets to a node | `TestGrantProjectCallsCliAndAudits` |
| Release secrets need a dedicated node | `TestGrantReleaseNeedsDedicatedNode` |
| A frozen node fences and the job runs elsewhere | `TestFaultScenario9Freeze` |
| Intermittent node setting | `TestNodesSetJSONEnvelopeCarriesTheStoredSettings` |
| Backup and restore with the KEK | `TestBackupRestoreCommandsRoundTrip` |
