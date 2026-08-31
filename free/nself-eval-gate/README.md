# nself-eval-gate

Eval harness and autonomy-tier gate for nSelf. Three-mode scoring (exact, semantic via BGE-M3, rubric via LLM-as-judge), recall-quality precision/recall/fact_f1 metrics, CI integration via nself ci eval, and autonomy-tier threshold enforcement.

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

## Dependencies

Optional: `plugin-retrieval`, `nself-ai-gateway`

## Install

```bash
nself plugin install nself-eval-gate
```
