#!/usr/bin/env bash

# mlflow-setup.sh - Setup MLflow with PostgreSQL support

# Generate MLflow Dockerfile with psycopg2
generate_mlflow_dockerfile() {

set -euo pipefail

  local mlflow_dir="${1:-mlflow}"

  # Only generate if MLflow is enabled
  if [[ "${MLFLOW_ENABLED:-false}" != "true" ]]; then
    return 0
  fi

  # Create MLflow directory
  mkdir -p "$mlflow_dir"

  # Generate Dockerfile with psycopg2 support
  cat >"$mlflow_dir/Dockerfile" <<'DOCKERFILE'
FROM ghcr.io/mlflow/mlflow:latest

# Install PostgreSQL adapter
RUN pip install --no-cache-dir psycopg2-binary

# Ensure artifacts directory exists
RUN mkdir -p /mlflow/artifacts

# Set working directory
WORKDIR /mlflow

# The actual command is provided by docker-compose.yml
DOCKERFILE

  return 0
}

# Export function
export -f generate_mlflow_dockerfile
