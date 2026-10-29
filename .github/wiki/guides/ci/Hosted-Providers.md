# Hosted providers

This guide covers the forges and hosted runners that `nself ci` can dispatch to: GitHub and GitLab. It explains how to connect them, how their runs are trusted, what capacity and spend they use, how requests reach your coordinator, and what happens when a provider goes down.

Command names and doctor check ids are not repeated here. They come from the generated [CI reference](Reference.md#commands). Read the commands there first, and check [the doctor table](Reference.md#doctor-checks) when a check fails.

## GitHub

### Credentials and scopes

The coordinator never stores a token value. Policy names the environment variable that holds it:

```text
nself ci policy set --scope user providers.github.token_env <ENV_NAME>
```

`nself ci providers test <id>` reads the token and reports what it can do. The provider id comes from `nself ci providers list`. A classic token reports its scopes. A fine-grained token sends no scopes header, so the probe reports the scopes as unknown and checks the access it can see through the API. No credential value appears in any output of the providers commands, the doctor section or the Forgejo alias.

### `providers github init`

`nself ci providers github init` renders the dispatch workflow from the template and writes it to `.github/workflows/nself-ci-dispatch.yml` in your repository:

```text
nself ci providers github init --stdout --agent-source source
nself ci providers github init --dir <repo> --agent-source source
nself ci providers github init --check --dir <repo> --agent-source source
```

- `--stdout` prints the file and writes nothing.
- `--check` reports drift between the rendering and the default branch copy and the untrusted ref copy. It exits 1 on drift.
- `--agent-source` chooses `source` or `release`. Without the flag, the value comes from `providers.github.agent_source` in policy.
- `--json` returns the same result as one JSON envelope.

The command never pushes. For each ref that is out of date, it prints the exact commands that would create or update it, and leaves running them to you. A file without the generated marker is never overwritten.

### The untrusted ref and why it exists

The dispatch workflow exists in two places: the default branch and a separate untrusted ref. Both copies must equal the rendering. `--check` fails when either one drifts.

The ref matters because of source trust. A run is classed by facts the forge reports, and a push to a ref that is not protected does not get owner trust. The table in [Trust, security and privacy](Trust-Security-and-Privacy.md#source-trust-who-made-the-code) lists each fact and its result. A missing fact takes its untrusted value.

### Standard labels only

Hosted jobs request standard labels only. The label set is generated from the template. It is not repeated here, so the template stays the only source.

### What a self-reported result proves

A GitHub run reports its result through the API. The coordinator records that report with the provider's attestation. Read [the attestation fields](Evidence-and-Machine-Output.md#providerattestation) for the exact shape.

The attestation proves what the provider reported for a run and a commit. It does not prove that the code ran on a node you control, and it does not prove the runner image beyond what the record states. Treat it as the provider's account of the run.

### JIT runners on your nodes

Your own Linux nodes can take GitHub jobs through just-in-time (JIT) runners. Each runner registers for one job and is removed when the job ends.

Limits that matter:

- A JIT job is classed untrusted unless the organization runner group meets the protected-group rule below. An untrusted job gets no secrets and no protected-branch rights.
- The node must be a Linux gVisor node. See [the sandboxed container level](Trust-Security-and-Privacy.md#the-sandboxed-container-level-gvisor).
- A repository-level runner is never proven restricted, because a runner group governs organization runners only. Repository runners run every workflow of their repository, fork pull requests included.

### Runner groups

A runner group counts as protected only when the API shows all of these:

- the group allows no public repositories,
- the group is restricted to workflows,
- the group lists at least one workflow,
- every listed workflow is pinned to a concrete protected branch.

An unreadable reply, or a reply that leaves out a field, is recorded as not verified. It is never read as verified.

This rule describes the group. It does not make a workflow safe that checks out fork code. A `pull_request_target` or `workflow_run` workflow still runs fork code on the runner, and the trust rules decide that from the event, the head repository and the fork flag.

### Fork approval

The fork pull request approval policy is read from the API and stored as a provider fact. A first-time contributor's run needs an approval before it starts. Fork pull requests always run on GitHub-hosted runners, whatever your repository variable says. The [workflow below](#required-check-with-your-own-nodes) applies this rule.

### Webhooks and per-repo secrets

Webhook deliveries are verified with a per-repository secret. Policy maps each `owner/repo` to the name of the environment variable that holds its secret:

```text
nself ci policy set --scope user providers.github.webhook_secrets '{"owner/repo":"<ENV_NAME>"}'
```

Ingress is covered in [Ingress: poll or webhook](#ingress-poll-or-webhook).

### Reconciliation

A reconcile pass is one counted, budgeted session over one repository. Every request goes through the provider REST client.

- A `304 Not Modified` reply costs nothing against the hourly ledger.
- A quota error records a retry time once and ends the pass.
- While the retry time is in the future, the coordinator makes no requests at all.

## GitLab

### Status in this release

GitLab support is tested against fixtures. The pipeline, status and runner paths have fixture tests, but no live GitLab run is covered by the test suite. Check the output of `nself ci providers test gitlab` against your own instance before you rely on it.

### Pipeline API and project token

The include file is what a GitLab pipeline loads. Render it with:

```text
nself ci providers gitlab init --stdout
nself ci providers gitlab init --dir <repo>
nself ci providers gitlab init --check --dir <repo>
```

The coordinator reaches the GitLab pipeline API with the project token. Policy names the environment variable that holds it under `providers.gitlab.token_env`. `providers test` reads that token's own record, with its role and scopes. Roles are reported by GitLab's access levels.

For runners, `providers.gitlab.runner_token_env` names the runner token. `nself ci providers test <id>` checks its scopes. A runner token without `create_runner` fails the probe.

### Untrusted ref for GitLab

`providers gitlab init --untrusted-dir <dir>` writes the two generated files of the untrusted ref into a directory. Add `--inputs` to write the inputs variant. Nothing is pushed. The check compares the default branch copy and both untrusted ref files with the rendering, and reports any missing file.

### Statuses

Commit statuses go through the GitLab status adapter. Status reporting is covered by fixture tests only, as described above.

### Signing secrets and secret tokens

GitLab webhooks can be verified in two ways.

| Method | Key | Check |
|---|---|---|
| Signing secret (Standard Webhooks) | `providers.gitlab.webhook_signing_secrets` | HMAC over the delivery, with a `whsec_` secret |
| Secret token | the token key next to it | `X-Gitlab-Token`, compared in constant time |

A project listed under the signing key is a signing project. For that project the bare token is never accepted, so a copied token cannot replace the signature. Use signing secrets where your GitLab version supports them.

### Runners

GitLab runners use the runner token named above. The token needs `create_runner`.

## Hosted capacity

### Included capacity and cost class

Included capacity from a hosted provider is a cost class. The classes, and how the scheduler weighs them, are in [CI scheduling and cost](Scheduling-and-Cost.md#cost-classes).

### The SCHED ceiling rule

A provider's allowance is bounded by its declared ceiling. The allowance is the declared ceiling, minus what the provider reported as used, minus what the local ledger spent since the newest day the provider's report covers. Providers report with a delay, so the local ledger fills the gap.

Paid placement is off by default. It needs `economics.paid_enabled` and a day or month budget above zero. The full rule, including rates and approval, is in [CI scheduling and cost](Scheduling-and-Cost.md).

### Usage import and confidence

`nself ci providers usage import` reads the provider's billing data into the ledger. Run it against a repository root:

```text
nself ci providers usage import --repo <dir>
```

The ledger shows every figure with a confidence label and its source. The labels and what each one means are in [the ledger table](Scheduling-and-Cost.md#the-ledger-and-its-confidence). Read `nself ci policy usage` for the current ledger.

### Outage behaviour

Each provider has a health breaker that moves between up, degraded and down.

- It opens to down after a run of consecutive failures inside a short window.
- While a probe is in progress, a down provider reads as degraded. Placement may send work elsewhere during that time.
- A quota answer (429) says nothing about the provider's health, so it does not open the breaker.
- Server errors count only as a repeated run, not as one error.
- A successful probe ends the outage. Placement does not treat that probe as a success until the provider is back up.

What happens to queued work after that is placement's decision. See [CI scheduling and cost](Scheduling-and-Cost.md).

## Schedules

Scheduled runs need a protected ref or a reachable sha. Without one, a scheduled run is classed untrusted, as the trust table in [Trust, security and privacy](Trust-Security-and-Privacy.md#source-trust-who-made-the-code) shows.

## Authorization

A provider can run work for a project only after an admin authorizes it. The project is a repository path.

```text
nself ci providers authorize <id> --project <repository path>
nself ci providers revoke <id> --project <repository path>
nself ci providers list
nself ci providers show <id>
```

- `authorize` and `revoke` are admin-only and audited.
- After `revoke`, the next admission for that project fails with the reason `privacy.provider_not_allowlisted`.
- `list` and `show` only read. They write nothing.
- A `.nself/ci.yaml` in a revision cannot authorize a provider. Authorization comes from the operator's store.

`--allow-private` on `authorize` writes the privacy zone first. It then prints the SCHED ceiling command for you to review. It never runs that command. Zones and their rules are in [Trust, security and privacy](Trust-Security-and-Privacy.md#network-scopes).

## Doctor

Provider checks are not part of the default doctor run. Add the flag:

```text
nself ci doctor --providers
nself ci doctor --providers --json --repo <dir>
```

The provider check ids are in [the doctor table](Reference.md#doctor-checks). `nself ci doctor` has no provider subcommand. Use the flag.

`nself ci forgejo` still runs for now, but it is an alias. It prints a deprecation line that names `nself ci doctor --providers` and states the release where the alias is removed. Move your scripts to the flag before then.

## Ingress: poll or webhook

Poll is the default. The coordinator asks the forge for changes over outbound connections only, so no inbound port needs to be open.

Webhook mode is optional. It needs all of these:

- `nself ci serve` reachable through a TLS reverse proxy.
- The proxy forwards only `/webhook`. It must not forward anything else.
- A per-repository secret for each repository. See [Webhooks and per-repo secrets](#webhooks-and-per-repo-secrets).
- The agent and user API listeners stay unexposed. The proxy never reaches them.

The loopback bind stays the default. `nself ci serve` listens on loopback unless you pass `--addr` with an address you have chosen on purpose.

```text
nself ci serve --addr <host:port>
```

## Required check with your own nodes

The required `nself-ci` check is pinned to GitHub Actions. A status posted from your own coordinator does not satisfy it, however good the local result is. Only a job that runs in GitHub Actions and reports the `nself-ci` name satisfies the check.

Your own nodes can still run that job. The `nself-ci` job runs on JIT runners, and a repository variable chooses where it runs:

- `runs-on` is `[self-hosted, nself]` when the variable selects your nodes.
- Otherwise it is `ubuntu-latest`.
- Fork pull requests always use GitHub-hosted runners, whatever the variable says.
- A JIT job is untrusted unless the protected-runner-group rule holds, so the node must be a Linux gVisor node.

<!-- snippet: nself-ci-own-nodes -->
```yaml
name: nself-ci

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  nself-ci:
    name: nself-ci
    runs-on: ${{ (vars.NSELF_CI_RUNNER == 'nself' && !github.event.pull_request.head.repo.fork) && fromJSON('["self-hosted","nself"]') || 'ubuntu-latest' }}
    steps:
      - uses: actions/checkout@v4
      - name: Run the nself-ci gate
        run: nself ci run
```

Without a node that meets the rule above, keep the check on GitHub-hosted runners. Your own nodes still give a fast gate before you push. Run `nself ci run` locally for that, and treat it as a pre-push check, not as the required status.
