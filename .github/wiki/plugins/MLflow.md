# MLflow Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install mlflow` |
| **Requirements** | nSelf >=1.0.0 |

<!-- END GENERATED:catalog -->

> Experiment tracking and model registry for teams running ML workloads on nSelf.

## Install

```bash
nself plugin install mlflow
```

No license key required.

## Description

MLflow experiment tracking and model registry

This plugin runs as its own container in your nSelf stack (rebuild with `nself build && nself start` after install).

Category: `data`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | *(required)* | Required. |
| `MLFLOW_DEFAULT_ARTIFACT_ROOT` | *(see plugin.json)* | Optional. |
| `MLFLOW_BACKEND_STORE_URI` | *(see plugin.json)* | Optional. |
| `MLFLOW_TRACKING_USERNAME` | *(see plugin.json)* | Optional. |
| `MLFLOW_TRACKING_PASSWORD` | — | Optional. |

## Ports

| Port | Purpose |
|------|---------|
| 5000 | MLflow service |

## REST API

```
MLflow's own tracking-server REST API (runs, experiments, model registry) at the configured port
```

## Examples

### Check health

```bash
curl http://localhost:5000/health
```

## Source

[`plugins/mlflow/`](https://github.com/nself-org/plugins/tree/main/mlflow)

Manifest: [`plugins/mlflow/plugin.json`](https://github.com/nself-org/plugins/tree/main/mlflow/plugin.json)

## See Also

- [[Model]] — manage local Ollama models

← [[Home]] →
