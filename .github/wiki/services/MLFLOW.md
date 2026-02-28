# MLflow - ML Experiment Tracking

## Overview

MLflow is an open-source machine learning lifecycle management platform integrated into nself as an optional service. It provides experiment tracking, model versioning, artifact storage, and model serving capabilities. When enabled, MLflow uses PostgreSQL as its backend store and MinIO as its artifact store, giving you a fully self-hosted ML operations platform that integrates with the rest of your nself stack.

MLflow is one of the 7 optional services in nself. Enable it with `MLFLOW_ENABLED=true` in your `.env` file. The service includes the MLflow Tracking Server with its web UI, connected to your existing PostgreSQL database for metadata and MinIO for artifact storage.

## Features

### Current Capabilities

- **Experiment Tracking** - Log parameters, metrics, and artifacts for every training run
- **Model Registry** - Version, stage, and manage ML models with lifecycle tracking
- **Artifact Storage** - Store model files, datasets, and outputs in MinIO
- **Web Dashboard** - Browser-based UI for comparing experiments and viewing results
- **REST API** - Programmatic access for logging, querying, and model management
- **Run Comparison** - Side-by-side comparison of metrics across training runs
- **Search and Filter** - Query experiments by parameters, metrics, or tags
- **Model Serving** - Deploy registered models as REST endpoints
- **Multi-User Support** - Shared tracking server for team collaboration
- **Framework Agnostic** - Works with PyTorch, TensorFlow, scikit-learn, XGBoost, and more

### Integration Points

| Service | Integration | Purpose |
|---------|------------|---------|
| PostgreSQL | Backend store | Experiment metadata, run parameters, metrics |
| MinIO | Artifact store | Model files, datasets, plots, checkpoints |
| Hasura | GraphQL access | Query experiment data through GraphQL API |
| Custom Services (CS_N) | MLflow SDK | Log experiments from training scripts |
| Functions | Model inference | Trigger predictions via serverless endpoints |
| Monitoring | Prometheus metrics | Tracking server health and usage metrics |

## Configuration

### Basic Setup

Enable MLflow in your `.env` file:

```bash
# MLflow Configuration
MLFLOW_ENABLED=true
```

MLflow requires MinIO for artifact storage. If MinIO is not already enabled, nself will automatically enable it when MLflow is active.

### Complete Configuration Reference

```bash
# Required
MLFLOW_ENABLED=true

# Version
MLFLOW_VERSION=2.9.2                 # MLflow version (default: 2.9.2)

# Port Configuration
MLFLOW_PORT=5000                     # Web UI and API port (default: 5000)

# Route Configuration
MLFLOW_ROUTE=mlflow                  # Creates mlflow.yourdomain.com

# Database Configuration
MLFLOW_DB_NAME=mlflow                # PostgreSQL database name (default: mlflow)
MLFLOW_DB_USER=mlflow                # Database user (default: mlflow)
MLFLOW_DB_PASSWORD=                  # Database password (auto-generated if not set)

# Artifact Storage
MLFLOW_ARTIFACTS_BUCKET=mlflow-artifacts  # MinIO bucket name (default: mlflow-artifacts)
MLFLOW_ARTIFACTS_ROOT=s3://mlflow-artifacts  # Artifact root URI

# Server Settings
MLFLOW_WORKERS=2                     # Gunicorn workers (default: 2)
MLFLOW_DEFAULT_ARTIFACT_ROOT=        # Override artifact root (auto-configured)
MLFLOW_SERVE_ARTIFACTS=true          # Proxy artifact access through server (default: true)

# Authentication (optional)
MLFLOW_AUTH_ENABLED=false            # Enable basic auth (default: false)
MLFLOW_AUTH_USERNAME=admin           # Basic auth username
MLFLOW_AUTH_PASSWORD=                # Basic auth password (auto-generated if not set)

# Garbage Collection
MLFLOW_GC_ENABLED=true               # Enable automatic garbage collection (default: true)
MLFLOW_GC_SCHEDULE=0 3 * * 0        # GC cron schedule (default: weekly Sunday 3 AM)
```

### Environment-Specific Configurations

#### Development

```bash
MLFLOW_ENABLED=true
MLFLOW_WORKERS=1
MLFLOW_AUTH_ENABLED=false
MINIO_ENABLED=true
```

#### Staging

```bash
MLFLOW_ENABLED=true
MLFLOW_WORKERS=2
MLFLOW_AUTH_ENABLED=true
MLFLOW_AUTH_USERNAME=ml-team
MLFLOW_AUTH_PASSWORD=staging-password
MINIO_ENABLED=true
```

#### Production

```bash
MLFLOW_ENABLED=true
MLFLOW_WORKERS=4
MLFLOW_AUTH_ENABLED=true
MLFLOW_AUTH_USERNAME=ml-team
MLFLOW_AUTH_PASSWORD=strong-production-password
MLFLOW_GC_ENABLED=true
MINIO_ENABLED=true
```

### Database Setup

nself automatically creates the MLflow database during `nself build`. The database is created within your existing PostgreSQL instance:

```sql
-- Automatically executed during build
CREATE DATABASE mlflow;
CREATE USER mlflow WITH PASSWORD 'auto-generated';
GRANT ALL PRIVILEGES ON DATABASE mlflow TO mlflow;
```

## Access

### Web Dashboard

**Local Development:**
- URL: `https://mlflow.local.nself.org`
- No authentication by default

**Production:**
- URL: `https://mlflow.<your-domain>`
- Basic auth when `MLFLOW_AUTH_ENABLED=true`

**Direct Access:**
- URL: `http://localhost:5000`

### Tracking API

**Within Docker Network:**
- URI: `http://mlflow:5000`

**From Host Machine:**
- URI: `http://localhost:5000`

## Usage

### CLI Commands

MLflow is managed through the `nself service mlflow` command group:

```bash
# Check MLflow status
nself service mlflow status

# View MLflow connection info
nself service mlflow info

# List experiments
nself service mlflow experiments

# List registered models
nself service mlflow models

# Open MLflow web UI
nself service mlflow open

# Run garbage collection manually
nself service mlflow gc

# View MLflow logs
nself service mlflow logs

# Export experiment data
nself service mlflow export --experiment "my-experiment" --output ./export/
```

### General Service Commands

```bash
# View MLflow container logs
nself logs mlflow

# Restart MLflow
nself restart mlflow

# Execute a command inside the MLflow container
nself exec mlflow mlflow --version

# Check all service URLs
nself urls
```

### Tracking Experiments

#### Python (MLflow SDK)

```python
import mlflow
import os

# Set the tracking URI to your nself MLflow instance
mlflow.set_tracking_uri(os.environ.get('MLFLOW_TRACKING_URI', 'http://mlflow:5000'))

# Set the experiment
mlflow.set_experiment('my-classification-model')

# Start a training run
with mlflow.start_run(run_name='random-forest-v1'):
    # Log parameters
    mlflow.log_param('n_estimators', 100)
    mlflow.log_param('max_depth', 10)
    mlflow.log_param('learning_rate', 0.01)

    # Train your model
    model = train_model(params)

    # Log metrics
    mlflow.log_metric('accuracy', 0.95)
    mlflow.log_metric('f1_score', 0.93)
    mlflow.log_metric('auc', 0.97)

    # Log the model artifact
    mlflow.sklearn.log_model(model, 'model')

    # Log additional artifacts
    mlflow.log_artifact('confusion_matrix.png')
    mlflow.log_artifact('feature_importance.csv')
```

#### Tracking with PyTorch

```python
import mlflow
import mlflow.pytorch
import torch

mlflow.set_tracking_uri('http://mlflow:5000')
mlflow.set_experiment('pytorch-image-classifier')

with mlflow.start_run():
    mlflow.log_params({
        'epochs': 50,
        'batch_size': 32,
        'optimizer': 'Adam',
        'learning_rate': 0.001,
    })

    for epoch in range(50):
        train_loss = train_one_epoch(model, dataloader, optimizer)
        val_accuracy = evaluate(model, val_dataloader)

        mlflow.log_metric('train_loss', train_loss, step=epoch)
        mlflow.log_metric('val_accuracy', val_accuracy, step=epoch)

    # Log the trained PyTorch model
    mlflow.pytorch.log_model(model, 'model')
```

#### Tracking with TensorFlow/Keras

```python
import mlflow
import mlflow.tensorflow

mlflow.set_tracking_uri('http://mlflow:5000')
mlflow.set_experiment('keras-text-classifier')

mlflow.tensorflow.autolog()

with mlflow.start_run():
    model = build_keras_model()
    history = model.fit(
        X_train, y_train,
        validation_data=(X_val, y_val),
        epochs=20,
        batch_size=64,
    )
    # Parameters and metrics are automatically logged by autolog
```

### Model Registry

#### Registering a Model

```python
import mlflow

mlflow.set_tracking_uri('http://mlflow:5000')

# Register a model from a run
result = mlflow.register_model(
    model_uri='runs:/abc123def456/model',
    name='production-classifier'
)

# Transition model stage
client = mlflow.MlflowClient()
client.transition_model_version_stage(
    name='production-classifier',
    version=1,
    stage='Production'
)
```

#### Loading a Registered Model

```python
import mlflow

mlflow.set_tracking_uri('http://mlflow:5000')

# Load the latest production model
model = mlflow.pyfunc.load_model('models:/production-classifier/Production')

# Make predictions
predictions = model.predict(input_data)
```

### REST API

MLflow exposes a REST API for programmatic access:

```bash
# List all experiments
curl http://localhost:5000/api/2.0/mlflow/experiments/list

# Search runs with filters
curl -X POST http://localhost:5000/api/2.0/mlflow/runs/search \
  -H 'Content-Type: application/json' \
  -d '{
    "experiment_ids": ["1"],
    "filter": "metrics.accuracy > 0.9",
    "max_results": 10,
    "order_by": ["metrics.accuracy DESC"]
  }'

# Get a specific run
curl http://localhost:5000/api/2.0/mlflow/runs/get?run_id=abc123

# List registered models
curl http://localhost:5000/api/2.0/mlflow/registered-models/list

# Get model version details
curl http://localhost:5000/api/2.0/mlflow/model-versions/get?name=my-model&version=1
```

### Using MLflow with nself Custom Services

A common pattern is running ML training scripts as nself custom services:

```bash
# .env configuration
CS_1=ml-trainer:python-fastapi:8010
MLFLOW_ENABLED=true
MINIO_ENABLED=true
```

The custom service can then track experiments:

```python
# services/ml_trainer/app.py
from fastapi import FastAPI, BackgroundTasks
import mlflow

app = FastAPI()
mlflow.set_tracking_uri('http://mlflow:5000')

@app.post('/train')
async def start_training(config: TrainingConfig, background_tasks: BackgroundTasks):
    background_tasks.add_task(run_training, config)
    return {'status': 'Training started'}

async def run_training(config):
    mlflow.set_experiment(config.experiment_name)
    with mlflow.start_run():
        mlflow.log_params(config.dict())
        model = train(config)
        mlflow.log_metric('accuracy', evaluate(model))
        mlflow.sklearn.log_model(model, 'model')
```

## Network and Routing

| Access Point | Address | Purpose |
|-------------|---------|---------|
| Web UI (Browser) | `https://mlflow.local.nself.org` | Dashboard and experiment viewer |
| Tracking API (Docker) | `http://mlflow:5000` | SDK tracking and API access |
| Tracking API (Host) | `http://localhost:5000` | Local development access |

## Resource Requirements

| Resource | Minimum | Recommended | Production |
|----------|---------|-------------|------------|
| CPU | 0.25 cores | 0.5 cores | 1-2 cores |
| Memory | 256MB | 512MB | 2-4GB |
| Storage | 500MB | 5GB | 50GB+ |
| Network | Low | Medium | Medium |

Storage requirements scale with the number of experiments and the size of logged artifacts. Large model files and datasets stored as artifacts are the primary storage consumers, held in MinIO rather than the MLflow container itself.

## Monitoring

When the monitoring bundle is enabled, MLflow server health is tracked.

### Available Metrics

- MLflow server response times and status codes (via nginx access logs)
- PostgreSQL query performance for the mlflow database
- MinIO storage usage for the mlflow-artifacts bucket
- Container resource consumption (CPU, memory) via cAdvisor

### Grafana Dashboard

```bash
# Access Grafana
# URL: https://grafana.local.nself.org
# Navigate to: Dashboards > MLflow Overview
```

### Health Checks

```bash
# Check MLflow health
nself health mlflow

# Docker health check (built into compose config)
# Uses: curl -f http://localhost:5000/health
# Interval: 30s
# Timeout: 10s
# Retries: 3
```

## Security

### Authentication

When `MLFLOW_AUTH_ENABLED=true`, MLflow requires HTTP Basic Authentication for all UI and API access:

```bash
# Configure credentials
MLFLOW_AUTH_ENABLED=true
MLFLOW_AUTH_USERNAME=ml-team
MLFLOW_AUTH_PASSWORD=secure-password
```

Client configuration with authentication:

```python
import os
os.environ['MLFLOW_TRACKING_USERNAME'] = 'ml-team'
os.environ['MLFLOW_TRACKING_PASSWORD'] = 'secure-password'

import mlflow
mlflow.set_tracking_uri('http://mlflow:5000')
```

### Network Isolation

- MLflow is only accessible within the Docker network by default
- External access is routed through nginx with HTTPS
- Restrict production access by IP via nginx configuration

### Best Practices

1. Enable authentication in staging and production environments
2. Store MLflow credentials in `.secrets` (never in `.env.dev`)
3. Use separate MinIO buckets for MLflow artifacts and application storage
4. Regularly run garbage collection to clean up deleted experiments
5. Back up the mlflow database as part of your regular backup schedule
6. Use experiment and run tags for organization and access control
7. Avoid logging sensitive data (credentials, PII) as parameters or tags

## Troubleshooting

### MLflow not starting

```bash
# Check MLflow logs
nself logs mlflow

# Verify MLflow is enabled
grep MLFLOW_ENABLED .env

# Check for port conflicts
lsof -i :5000

# Verify PostgreSQL is running (required dependency)
nself status postgres

# Verify MinIO is running (required for artifacts)
nself status minio

# Run diagnostics
nself doctor
```

### Cannot access web dashboard

```bash
# Verify nginx routing
nself urls

# Check MLflow is running
nself status

# Test direct access
curl -s http://localhost:5000/

# Rebuild nginx configuration
nself build --force && nself restart nginx
```

### Experiment logging failures

```bash
# Verify tracking URI is correct
# Inside Docker: http://mlflow:5000
# From host: http://localhost:5000

# Check MLflow server logs
nself logs mlflow --follow

# Verify database connectivity
nself exec mlflow python -c "
import mlflow
mlflow.set_tracking_uri('http://localhost:5000')
print(mlflow.search_experiments())
"

# Check MinIO connectivity for artifacts
nself exec mlflow curl -s http://minio:9000/minio/health/live
```

### Artifact storage errors

```bash
# Verify MinIO is running
nself status minio

# Check the artifacts bucket exists
nself exec minio mc ls local/mlflow-artifacts

# Verify MinIO credentials
grep MINIO_ROOT .env

# Check artifact configuration
grep MLFLOW_ARTIFACTS .env
```

### Database connection errors

```bash
# Verify PostgreSQL is running
nself status postgres

# Check if the mlflow database exists
nself exec postgres psql -U postgres -c "\l" | grep mlflow

# Manually create if missing
nself exec postgres psql -U postgres -c "CREATE DATABASE mlflow;"

# Check connection string
grep MLFLOW_DB .env
```

### High memory usage

```bash
# Check container resource usage
docker stats $(docker ps -q --filter name=mlflow)

# Increase worker count for better throughput
MLFLOW_WORKERS=4

# Run garbage collection to clean up old runs
nself service mlflow gc
```

## Data Persistence

MLflow data is stored in two locations:

1. **Metadata** (experiments, runs, parameters, metrics) - PostgreSQL database `mlflow`
2. **Artifacts** (model files, plots, datasets) - MinIO bucket `mlflow-artifacts`

Both are backed by Docker volumes that persist across container restarts.

### Backup

```bash
# Include MLflow in full backup
nself backup create --include mlflow

# Backup the MLflow database separately
nself db dump --database mlflow --output mlflow-backup.sql

# Backup artifacts via MinIO
nself exec minio mc mirror local/mlflow-artifacts /tmp/mlflow-artifacts-backup
```

### Restore

```bash
# Restore from full backup
nself backup restore --include mlflow --from backup-2026-01-15.tar.gz

# Restore database separately
nself db restore --database mlflow --input mlflow-backup.sql
```

## Related Documentation

- [Optional Services Overview](SERVICES_OPTIONAL.md) - All optional services
- [Services Overview](SERVICES.md) - Complete service listing
- [MinIO Documentation](MINIO.md) - Artifact storage backend
- [Redis Documentation](REDIS.md) - Caching for model inference
- [Environment Variables](../configuration/ENVIRONMENT-VARIABLES.md) - Full configuration reference
- [Custom Services](SERVICES_CUSTOM.md) - Building ML training services
- [Troubleshooting](../troubleshooting/README.md) - Common issues and solutions
