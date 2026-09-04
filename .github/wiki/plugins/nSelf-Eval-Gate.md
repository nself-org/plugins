# nSelf Eval Gate Plugin

> Eval harness and autonomy-tier gate for nSelf. **Free — MIT licensed.**

## Install

```bash
nself plugin install nself-eval-gate
```

No license key required.

## Description

Eval harness and autonomy-tier gate for nSelf. Three-mode scoring (exact, semantic via BGE-M3, rubric via LLM-as-judge), recall-quality precision/recall/fact_f1 metrics, CI integration via nself ci eval, and autonomy-tier threshold enforcement.

Category: `development`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_EVAL_GATE_DB_URL` | `(required)` | - |
| `NSELF_EVAL_GATE_PORT` | `3770` | - |
| `NSELF_EVAL_GATE_REDIS_URL` | `-` | - |
| `NSELF_EVAL_GATE_JUDGE_MODEL` | `-` | - |
| `NSELF_EVAL_GATE_JUDGE_TIMEOUT_S` | `-` | - |
| `NSELF_EVAL_GATE_EMBED_TIMEOUT_S` | `-` | - |
| `NSELF_EVAL_GATE_MAX_CONCURRENCY` | `-` | - |
| `NSELF_EVAL_GATE_CACHE_EMBED_TTL_H` | `-` | - |
| `NSELF_EVAL_GATE_CACHE_JUDGE_TTL_H` | `-` | - |
| `NSELF_EVAL_RECALL_K` | `-` | - |
| `NSELF_EVAL_SCHEMA_VERSION` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3770 | nSelf Eval Gate service port |

## Database Schema

4 table(s) added to your Postgres database:

- `np_eval_suites`
- `np_eval_tasks`
- `np_eval_runs`
- `np_eval_thresholds`

## REST API

```
POST   /eval/run
GET    /eval/runs/{id}
GET    /eval/suites
POST   /eval/validate
GET    /eval/thresholds
GET    /eval/gate/{tier}
```

## Examples

### Health check

```bash
curl http://localhost:3770/eval/run
```

## Source

[`plugins/nself-eval-gate/`](https://github.com/nself-org/plugins/tree/main/nself-eval-gate)

Manifest: [`plugins/nself-eval-gate/plugin.json`](https://github.com/nself-org/plugins/tree/main/nself-eval-gate/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
