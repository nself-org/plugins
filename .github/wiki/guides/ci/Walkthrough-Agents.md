# Walkthrough: agents

This page walks a coding agent through CI. The agent starts a run, waits for it, and reads the evidence. It can do this through the CLI, through MCP tools or through HTTP routes. All three return the same JSON.

Commands that need a real run are shown as text, each with the test that runs it. The full list of MCP tools and HTTP routes, with each command's side effect and confirmation rule, is the generated machine-surface table at the top of the [CI plugin page](../../plugins/CI.md). The command and flag tables are in the generated [Reference](./Reference.md#commands).

## The CLI loop

Start the run in the background with `--detach --json`. The reply is one JSON document with the run id and the exact commands to wait for it, follow it, read its evidence, cancel it and read its logs.

<!-- proven-by: TestDetach -->
```text
nself ci --detach --json
```

Wait for the result, then read the evidence. `runs wait` returns when the run ends; `--timeout` bounds the wait.

<!-- proven-by: TestRunsWait -->
```text
nself ci runs wait <run-id> --json
```

<!-- proven-by: TestBareJSONIsOneEvidenceEnvelope -->
```text
nself ci evidence <run-id> --json
```

To watch progress instead of waiting, follow the run. Each line is one JSON event, and a reader that reconnects can resume from the last sequence number it saw.

<!-- proven-by: TestFollowReplayFromSeq -->
```text
nself ci runs follow <run-id> --json
```

The document shapes are in [Evidence and Machine Output](./Evidence-and-Machine-Output.md).

## The MCP loop

Start the MCP server with `nself mcp` inside the project directory. The agent needs only three tools for the loop:

1. `nself_ci` starts a run. Pass `{"args": ["--detach"]}`.
2. `nself_ci_runs_wait` waits for it. Pass `{"args": ["<run-id>"]}`.
3. `nself_ci_evidence` returns the evidence. Pass `{"args": ["<run-id>"]}`.

Every other tool is listed in the machine-surface table. Commands that only make sense in a terminal, that stream, or that ask questions have no tool.

## Confirmations

A bare run executes code, so it needs a confirmation. The first `nself_ci` call does not run anything. It returns a request with a `confirm` value. The agent shows the request to its user and, once approved, calls `nself_ci` again with the same arguments plus that `confirm` value. Only the second call starts the run. Destructive commands, such as revoking a node, work the same way. The confirmation column of the machine-surface table says which commands need it.

```text
first call:  {"args": ["--detach"]}
reply:       {"kind": "request", "confirm": "<64 hex characters>", ...}
second call: {"args": ["--detach"], "confirm": "<the same value>"}
```

## The HTTP routes

`nself admin serve` exposes the same commands as HTTP routes under `/v1/commands/`, for example `/v1/commands/ci/runs/wait`. Send a POST with a bearer token and a JSON body such as `{"args": ["<run-id>"]}`. The reply carries the same data as the CLI's `--json` output. Confirmations use the same `confirm` round trip.

`runs follow` has no MCP tool. Over HTTP, `/v1/commands/ci/runs/follow` answers with an NDJSON stream: one JSON event per line, flushed as the run moves.

## One server per project

An MCP or HTTP server works on the project directory it was started in. Start one server per project, each from its own checkout.

## No commit status from agents by default

A run started through MCP or HTTP is a machine-invoked run: the server marks it with `NSELF_INVOKED_BY`. A CLI agent should export that variable itself. A machine-invoked run posts no commit status, and prints one line saying so. Add `--post-status` when the agent's result should count as the commit's status.

<!-- proven-by: TestMachineDefaults -->
```text
nself ci --detach --json --post-status
```

## Capping the trust of agent runs

The policy key `trust.machine_invoked_cap` caps the source trust class of every machine-invoked run. The default is `owner`, which is no cap. For agents that run unattended, set it to `untrusted`. A machine-invoked caller can lower the cap but never raise it again.

<!-- proven-by: TestPolicyMachineInvokedCap -->
```text
nself ci policy set --scope user trust.machine_invoked_cap untrusted
```

For a CLI agent the cap is advisory, because the agent controls its own environment. See [machine-invoked runs](./Trust-Security-and-Privacy.md#machine-invoked-runs).

## Code an agent wrote

Run code that an agent wrote, and that nobody has read yet, as untrusted. `--trust untrusted` needs the sandboxed container level; it never falls back to a weaker level, never gets service containers and never gets internet access. If the host cannot provide the sandbox, nothing runs.

<!-- proven-by: TestResolveLevelAndNetwork -->
```text
nself ci --trust untrusted --detach --json -- go test ./...
```

The isolation levels are in [Trust, Security and Privacy](./Trust-Security-and-Privacy.md#the-sandboxed-container-level-gvisor).

## Proven by

| Step | Test |
|---|---|
| Detached run and the handle's commands | `TestDetach` |
| Detached run outlives its parent | `TestDetachSurvivesParent` |
| Wait for a run | `TestRunsWait` |
| Evidence as one JSON document | `TestBareJSONIsOneEvidenceEnvelope` |
| Follow replays from a sequence number | `TestFollowReplayFromSeq` |
| MCP tools, HTTP routes, same output on CLI, MCP and HTTP, confirmation round trip | `scripts/ci/ci-machine-surface.sh` |
| No status from machine-invoked runs unless `--post-status` | `TestMachineDefaults` |
| Trust cap key, lower but never raise | `TestPolicyMachineInvokedCap` |
| `--trust untrusted` needs the sandbox | `TestResolveLevelAndNetwork` |
