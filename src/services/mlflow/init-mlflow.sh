#!/usr/bin/env bash

# MLflow Initialization Script
# Creates database and S3 bucket for MLflow when enabled

set -euo pipefail

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../../lib/utils/colors.sh"
source "${SCRIPT_DIR}/../../lib/utils/env.sh"

# Load environment with priority
load_env_with_priority

# Initialize MLflow database in PostgreSQL
init_mlflow_database() {
  local max_retries=30
  local retry_count=0
  
  echo_info "Initializing MLflow database..."
  
  # Wait for PostgreSQL to be ready
  while [ $retry_count -lt $max_retries ]; do
    if docker exec "${PROJECT_NAME}-postgres-1" pg_isready -U "${POSTGRES_USER}" >/dev/null 2>&1; then
      break
    fi
    echo_warning "Waiting for PostgreSQL... (attempt $((retry_count + 1))/$max_retries)"
    sleep 2
    retry_count=$((retry_count + 1))
  done
  
  if [ $retry_count -eq $max_retries ]; then
    echo_error "PostgreSQL failed to start after $max_retries attempts"
    return 1
  fi
  
  # Create MLflow database if it doesn't exist
  echo_info "Creating MLflow database '${MLFLOW_DB_NAME}'..."
  docker exec "${PROJECT_NAME}-postgres-1" psql -U "${POSTGRES_USER}" -tc "SELECT 1 FROM pg_database WHERE datname = '${MLFLOW_DB_NAME}'" | grep -q 1 || \
    docker exec "${PROJECT_NAME}-postgres-1" psql -U "${POSTGRES_USER}" -c "CREATE DATABASE ${MLFLOW_DB_NAME};"
  
  # Create extensions
  echo_info "Setting up PostgreSQL extensions for MLflow..."
  docker exec "${PROJECT_NAME}-postgres-1" psql -U "${POSTGRES_USER}" -d "${MLFLOW_DB_NAME}" -c "CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\";"
  
  echo_success "MLflow database initialized successfully"
}

# Initialize MinIO bucket for MLflow artifacts
init_mlflow_storage() {
  local max_retries=30
  local retry_count=0
  
  echo_info "Initializing MLflow artifact storage..."
  
  # Wait for MinIO to be ready
  while [ $retry_count -lt $max_retries ]; do
    if docker exec "${PROJECT_NAME}-minio-1" mc alias set local http://localhost:9000 "${MINIO_ROOT_USER}" "${MINIO_ROOT_PASSWORD}" >/dev/null 2>&1; then
      break
    fi
    echo_warning "Waiting for MinIO... (attempt $((retry_count + 1))/$max_retries)"
    sleep 2
    retry_count=$((retry_count + 1))
  done
  
  if [ $retry_count -eq $max_retries ]; then
    echo_error "MinIO failed to start after $max_retries attempts"
    return 1
  fi
  
  # Create MLflow artifacts bucket if it doesn't exist
  echo_info "Creating MLflow artifacts bucket '${MLFLOW_ARTIFACTS_BUCKET}'..."
  docker exec "${PROJECT_NAME}-minio-1" mc mb --ignore-existing "local/${MLFLOW_ARTIFACTS_BUCKET}"
  
  # Set bucket policy to allow MLflow access
  docker exec "${PROJECT_NAME}-minio-1" mc anonymous set download "local/${MLFLOW_ARTIFACTS_BUCKET}"
  
  echo_success "MLflow artifact storage initialized successfully"
}

# Wait for MLflow to be healthy
wait_for_mlflow() {
  local max_retries=30
  local retry_count=0
  
  echo_info "Waiting for MLflow to be ready..."
  
  while [ $retry_count -lt $max_retries ]; do
    if curl -s -o /dev/null -w "%{http_code}" "http://localhost:${MLFLOW_PORT}/health" | grep -q "200"; then
      echo_success "MLflow is ready!"
      return 0
    fi
    echo_warning "Waiting for MLflow... (attempt $((retry_count + 1))/$max_retries)"
    sleep 2
    retry_count=$((retry_count + 1))
  done
  
  echo_error "MLflow failed to become ready after $max_retries attempts"
  return 1
}

# Main initialization
main() {
  if [[ "${MLFLOW_ENABLED:-false}" != "true" ]]; then
    echo_info "MLflow is not enabled. Skipping initialization."
    exit 0
  fi
  
  echo_info "Starting MLflow initialization..."
  
  # Initialize database
  if ! init_mlflow_database; then
    echo_error "Failed to initialize MLflow database"
    exit 1
  fi
  
  # Initialize storage
  if ! init_mlflow_storage; then
    echo_error "Failed to initialize MLflow storage"
    exit 1
  fi
  
  # Wait for MLflow service
  if ! wait_for_mlflow; then
    echo_warning "MLflow service is not responding, but initialization completed"
  fi
  
  echo_success "MLflow initialization completed successfully!"
  echo_info "Access MLflow at: https://${MLFLOW_ROUTE}"
  
  if [[ "${MLFLOW_AUTH_ENABLED:-false}" == "true" ]]; then
    echo_info "Authentication enabled - Username: ${MLFLOW_AUTH_USERNAME}"
  fi
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi