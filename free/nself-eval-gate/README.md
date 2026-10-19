# nself-eval-gate

Eval harness and autonomy-tier gate for nSelf. Three-mode scoring (exact, semantic via BGE-M3, rubric via LLM-as-judge), recall-quality precision/recall/fact_f1 metrics, CI integration via the `nself eval` command, and autonomy-tier threshold enforcement.

## Details

- **Category:** development
- **Tier:** pro
- **Language:** go
- **Port:** 3770
- **License:** MIT

## Configuration

| Env var | Required | Description |
|---|---|---|
| `NSELF_EVAL_GATE_DB_URL` | Yes | — |
| `NSELF_EVAL_GATE_PORT` | No | — |
| `NSELF_EVAL_GATE_REDIS_URL` | No | — |
| `NSELF_EVAL_GATE_JUDGE_MODEL` | No | — |
| `NSELF_EVAL_GATE_JUDGE_TIMEOUT_S` | No | — |
| `NSELF_EVAL_GATE_EMBED_TIMEOUT_S` | No | — |
| `NSELF_EVAL_GATE_MAX_CONCURRENCY` | No | — |
| `NSELF_EVAL_GATE_CACHE_EMBED_TTL_H` | No | — |
| `NSELF_EVAL_GATE_CACHE_JUDGE_TTL_H` | No | — |
| `NSELF_EVAL_RECALL_K` | No | — |
| `NSELF_EVAL_SCHEMA_VERSION` | No | — |

## API

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/eval/run` | plugin-jwt |  |
| `GET` | `/eval/runs/{id}` | plugin-jwt |  |
| `GET` | `/eval/suites` | plugin-jwt |  |
| `POST` | `/eval/validate` | plugin-jwt |  |
| `GET` | `/eval/thresholds` | plugin-jwt |  |
| `GET` | `/eval/gate/{tier}` | plugin-jwt |  |

## CLI: `nself eval`

The plugin ships a second binary, `nself-eval` (`cmd/eval/`), declared in `plugin.json` as the `commands` block (`command: eval`, subcommand `gate`). It is the port of core `nself ci eval [gate]`: same flags, same output, same exit contract. The HTTP service is unchanged.

```bash
nself eval --suite recall-quality-v1                    # run a suite, poll to completion
nself eval --suite recall-quality-v1 --output table     # json | table | summary
nself eval --validate-only --suite recall-quality-v1    # schema check, no run
nself eval --dry-run --suite recall-quality-v1
nself eval gate --tier semi-auto                        # supervised | semi-auto | full-auto
```

| Exit | Meaning |
|---|---|
| 0 | suite passed / tier cleared |
| 1 | below threshold / tier blocked (also flag parse errors, as cobra did) |
| 2 | validation error, invalid tier, or an infra error on `gate` |
| 3 | precondition not met (BGE-M3 or AI gateway down; not a regression) |

Notes:

- The binary receives the argv that followed `nself ci eval` in core. The nself host handles the global flags `--json`, `--no-deprecation-warnings` and `--no-monorepo`; the binary accepts and ignores them.
- The run writes `{repo}/.nself-ci/artifacts/eval-results.json`. Progress lines go to stderr, the formatted result to stdout.
- `--help` prints core's text verbatim, including the `nself ci eval` wording, until core is deleted (P7-SHIP-09).
- The new ci engine (`free/ci`) has no eval stage. A project may run the gate as a generic job in `.nself/ci.yaml`: `nself eval gate --tier full-auto`.
- v1.4 keeps core `nself ci eval` and the legacy gate stage; the v1.5 route from `nself ci eval` to `nself eval` is a breakouts row (P7-CI-45).
- Parity: `testdata/eval-parity/` holds goldens captured from a core build (`cases.json`, `routes.json`, `<case>.{stdout,stderr,exit}`); `TestEvalParity` replays every case against `internal/cli`. To recapture, run core `nself ci eval <args>` for each case in `cases.json` against a server that serves `routes.json` (POST `/eval/run` and `/eval/validate` are keyed by suite slug and by empty or non-empty content) and replace the server URL and repo path with `{{URL}}` and `{{REPO}}`.

## Dependencies

Optional: `plugin-retrieval`, `nself-ai-gateway`

## Install

```bash
nself plugin install nself-eval-gate
```
