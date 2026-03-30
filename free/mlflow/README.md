# mlflow

MLflow experiment tracking and model registry for nself.

## Overview

The `mlflow` plugin integrates MLflow into your nself stack, providing experiment tracking, run management, model registry, and artifact storage backed by PostgreSQL.

## Installation

```bash
nself plugin install mlflow
```

## Configuration

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `MLFLOW_DEFAULT_ARTIFACT_ROOT` | No | Artifact storage path (default: `/mlflow/artifacts`) |
| `MLFLOW_BACKEND_STORE_URI` | No | MLflow backend store URI |
| `MLFLOW_TRACKING_USERNAME` | No | Username for MLflow tracking server auth |
| `MLFLOW_TRACKING_PASSWORD` | No | Password for MLflow tracking server auth |

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

## Access

MLflow UI is available at port 5000 after starting the plugin.

## License

MIT
