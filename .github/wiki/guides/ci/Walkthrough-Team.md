# Walkthrough: a team

This page walks a small team that shares CI. One box runs the team coordinator. Two other boxes run jobs. Members submit runs and read results from their own machines. Each member may also keep personal nodes at home.

Names used below: `ci.example` is the coordinator box, `runner1` and `runner2` are the job boxes, `example-org/example-repo` is a team repository. Commands that need real machines are shown as text, each with the test that runs it. The command and flag tables are in the generated [Reference](./Reference.md#commands).

## A team coordinator on its own box

Start the coordinator with `--team` as soon as anyone besides you will hold a role, a token or a certificate. The switch is permanent and audited; starting the same data directory without the flag later does not make it personal again.

<!-- proven-by: TestServeModeFlagAndStart -->
```text
nself ci serve --team
```

Pick the box with care:

- Never a production box. A machine that serves deployed apps is refused as a CI node; keep the coordinator and its keys off it as well.
- Keep jobs off the coordinator. In team mode any job on the coordinator machine needs VM isolation or stronger. Two boxes are simpler: one keeps state and keys, the others run jobs.

Add `runner1` and `runner2` from the coordinator box as described in [Nodes and Runners](./Nodes-and-Runners.md#lan). The coordinator-host rule is in [Coordinator host](./Trust-Security-and-Privacy.md#coordinator-host).

## Roles and tokens

Roles are `member`, `release` and `admin`, each for one or more projects. Grant a role to a person, or issue a scoped token for a machine. A token is shown once.

<!-- proven-by: TestServeAdminRolesGrantListRevoke -->
```text
nself ci serve admin roles grant bob --role member --project example-org/example-repo --json
nself ci serve admin roles list
```

<!-- proven-by: TestServeAdminTokenIssueOnce -->
```text
nself ci serve admin token issue build-bot --role release --project example-org/example-repo --ttl 48h --json
```

Revoking a role also ends that person's tokens. Every change is in the audit log. See [roles, tokens and the audit log](./Trust-Security-and-Privacy.md#roles-tokens-and-the-audit-log).

## Members work through `--coordinator`

A member's commands go to the team coordinator when they name it with `--coordinator`. Members can list and show nodes; draining, enabling, revoking or removing a node needs `admin`.

<!-- proven-by: TestNodesRemoteMemberReadsButCannotRevoke -->
```text
nself ci nodes list --coordinator https://ci.example:3865 --json
```

<!-- proven-by: TestDetach -->
```text
nself ci --detach --json --coordinator https://ci.example:3865
```

Without the flag, commands stay on the member's own machine, even when a coordinator is configured. Adding and pairing nodes happen on the coordinator box only.

## Personal nodes stay personal

A member's home nodes belong to that member's personal coordinator. They are not part of the team pool, and a team repository's commit of record never runs on them. A project can list which pools may run it; a personal pool left off that list is excluded. `nodes pools explain` shows the effective set and why each runner is left out.

<!-- proven-by: TestScopePersonalVsTeamPool -->
```text
nself ci nodes pools explain --project example-org/example-repo
```

The pool rules are in [Auto Placement and Pools](./Auto-Placement-and-Pools.md#pools).

## The organization's GitHub credentials

The coordinator holds the organization's GitHub credentials, not each member. Write the dispatch workflow into each repository with `providers github init`.

<!-- proven-by: TestGithubInitStdout -->
```text
nself ci providers github init --stdout --agent-source source
```

Scopes and which token does what are in [credentials and scopes](./Hosted-Providers.md#credentials-and-scopes).

## Open source repositories use GitHub-hosted runners

For a public repository, a pull request from a fork is untrusted. It gets no secrets, no LAN access and no trusted cache. Its jobs need the sandboxed container level on a runner with no route to your LAN, or a disposable GitHub-hosted runner. Hosted runners are free for public repositories and nSelf never plans paid hosted minutes. A failure on the hosted runner stays a failure.

Read [Public FOSS PRs](./Trust-Security-and-Privacy.md#public-foss-prs) and [hosted capacity](./Hosted-Providers.md#hosted-capacity) before you open a public repository to your own nodes.

## The required check

Make `nself-ci` a required status check on the protected branch. Run it with `--dry-run` first: it prints the change and writes nothing.

<!-- proven-by: TestPolicyProtectDryRunChangesNothing -->
```text
nself ci policy protect --repo example-org/example-repo --branch main --dry-run
```

Without `--dry-run` it saves a snapshot of the old protection before it writes the new one.

## Own nodes satisfy the check by polling

Your nodes do not need to accept inbound webhooks. `serve watch` polls GitHub over outbound connections only, claims a commit, runs it on your nodes and posts one final `nself-ci` status. With several watchers, exactly one final result is posted per commit; if the preferred watcher dies, another takes over once its claim is dead.

<!-- proven-by: TestWatchThreeHostsExactlyOneStatus -->
```text
nself ci serve watch --once --json
```

If the branch pins the required check to a GitHub App, the watcher posts the advisory `nself-ci/watch` context instead, never `nself-ci`. See [Watch and Services](./Watch-and-Services.md#what-watch-does).

## Proven by

| Step | Test |
|---|---|
| Team mode flag, permanent switch | `TestServeModeFlagAndStart` |
| Coordinator mode rules | `TestCoordinatorMode` |
| Grant, list and revoke roles | `TestServeAdminRolesGrantListRevoke` |
| Issue a token once | `TestServeAdminTokenIssueOnce` |
| Revoking a role ends its tokens | `TestServeAdminRevokeEndsTokensToo` |
| Member reads, cannot change nodes | `TestNodesRemoteMemberReadsButCannotRevoke` |
| Add and pair stay on the coordinator box | `TestNodesRemoteAddAndPairAreLocalOnly` |
| Detached run sent to a coordinator | `TestDetach` |
| Personal pool excluded from team work | `TestScopePersonalVsTeamPool` |
| Commit of record stays on the record pool | `TestRecordPoolTransportExclusion` |
| Dispatch workflow rendering | `TestGithubInitStdout` |
| Hosted result decisions | `TestHostedDecisionMatrix` |
| Hosted capacity is never metered | `TestEconomicsGitHubHostedIsNeverMetered` |
| Required check dry run | `TestPolicyProtectDryRunChangesNothing` |
| Snapshot before applying protection | `TestPolicyProtectApplyWritesSnapshotFirst` |
| One final status from several watchers | `TestWatchThreeHostsExactlyOneStatus` |
| Takeover after a dead claim | `TestWatchKillPreferredTakeover` |
| Pinned check gets the advisory context | `TestStatusContextPinned` |
