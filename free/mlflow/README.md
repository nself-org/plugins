# mlflow

MLflow experiment tracking and model registry — backed by PostgreSQL, integrated into your nSelf stack.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install mlflow
nself build
nself start
```

## Overview

The `mlflow` plugin brings the full [MLflow](https://mlflow.org) experiment tracking and model registry into your nSelf stack. It runs the MLflow tracking server as a Docker service, stores all experiment metadata in PostgreSQL via `--backend-store-uri`, and persists artifacts to a configurable local or S3-compatible path.

This is a **config-type plugin** — it orchestrates a Docker service rather than shipping a separate Go binary. `nself build` injects the MLflow tracking server into your compose stack.

MLflow UI is available at port 5000 after `nself start`. Access it at `http://127.0.0.1:5000` on the host, or through your configured Nginx route.

## Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string — used as MLflow backend store |
| `MLFLOW_DEFAULT_ARTIFACT_ROOT` | No | `/mlflow/artifacts` | Local or S3 path for run artifacts (model files, plots, etc.) |
| `MLFLOW_BACKEND_STORE_URI` | No | derived from `DATABASE_URL` | Override MLflow backend URI directly (e.g. `sqlite:///mlflow.db` for local) |
| `MLFLOW_TRACKING_USERNAME` | No | — | Username for MLflow tracking server HTTP auth |
| `MLFLOW_TRACKING_PASSWORD` | No | — | Password for MLflow tracking server HTTP auth |
| `MLFLOW_PORT` | No | `5000` | Port the MLflow UI and API serve on |
| `MLFLOW_WORKERS` | No | `4` | Gunicorn worker count for the tracking server |
| `MLFLOW_SERVE_ARTIFACTS` | No | `true` | Serve artifacts through the tracking server |

## Usage

```bash
# List and manage MLflow experiments
nself plugin run mlflow experiments

# List and manage experiment runs
nself plugin run mlflow runs

# Browse the model registry
nself plugin run mlflow models

# View and download run artifacts
nself plugin run mlflow artifacts
```

## Python SDK Integration

Set the tracking URI to point at your nSelf instance:

```python
import mlflow

mlflow.set_tracking_uri("http://127.0.0.1:5000")
mlflow.set_experiment("my-experiment")

with mlflow.start_run():
    mlflow.log_param("learning_rate", 0.001)
    mlflow.log_metric("accuracy", 0.94)
    mlflow.sklearn.log_model(model, "model")
```

## Nginx Route

To proxy the MLflow UI through Nginx:

```bash
# In your .env.local
MLFLOW_ROUTE=/mlflow
# Then: nself build — Nginx is configured to proxy /mlflow → port 5000
```

## Artifact Storage

Artifacts are stored at `MLFLOW_DEFAULT_ARTIFACT_ROOT`. For production, point this at an S3-compatible bucket:

```bash
MLFLOW_DEFAULT_ARTIFACT_ROOT=s3://my-bucket/mlflow-artifacts
AWS_ACCESS_KEY_ID=...
AWS_SECRET_ACCESS_KEY=...
```

## Port

MLflow UI and API bind to `127.0.0.1:5000`. Access locally or through your configured Nginx route.

## Database

MLflow stores experiment metadata (runs, params, metrics, tags) in PostgreSQL automatically. It manages its own schema — no `np_` tables are created by this plugin.

## Multi-App

The `mlflow` plugin does not support multi-app isolation (`multiApp.supported: false`). It provides a single shared MLflow instance per nSelf deployment. Separate nSelf deployments give separate MLflow instances.

## See also

- [plugin-jobs](plugin-jobs.md) — schedule training runs as background jobs
- [nSelf CLI: nself plugin](cmd-plugin.md) — plugin management
- [MLflow documentation](https://mlflow.org/docs/latest/index.html) — upstream docs
