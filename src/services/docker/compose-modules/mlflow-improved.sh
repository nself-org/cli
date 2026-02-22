#!/usr/bin/env bash
set -euo pipefail
# mlflow-improved.sh - Improved MLflow service generation

# Generate improved MLflow service with proper database handling
generate_mlflow_service_improved() {
  local enabled="${MLFLOW_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  cat <<EOF

  # MLflow - Machine Learning Lifecycle Platform
  mlflow:
    image: ghcr.io/mlflow/mlflow:latest
    container_name: \${PROJECT_NAME}_mlflow
    restart: unless-stopped
    networks:
      - \${DOCKER_NETWORK}
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      MLFLOW_BACKEND_STORE_URI: postgresql://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD}@postgres:5432/mlflow
      MLFLOW_DEFAULT_ARTIFACT_ROOT: /mlflow/artifacts
      MLFLOW_HOST: 0.0.0.0
      MLFLOW_PORT: \${MLFLOW_PORT:-5005}
      POSTGRES_USER: \${POSTGRES_USER:-postgres}
      POSTGRES_PASSWORD: \${POSTGRES_PASSWORD}
      POSTGRES_DB: mlflow
    entrypoint: ["/bin/bash", "-c"]
    command:
      - |
        set -e
        echo "Waiting for PostgreSQL to be ready..."
        until pg_isready -h postgres -p 5432 -U \$\${POSTGRES_USER}; do
          echo "Waiting for database..."
          sleep 2
        done

        echo "Ensuring MLflow database exists..."
        PGPASSWORD=\$\${POSTGRES_PASSWORD} psql -h postgres -U \$\${POSTGRES_USER} -tc "SELECT 1 FROM pg_database WHERE datname = 'mlflow'" | grep -q 1 || \
          PGPASSWORD=\$\${POSTGRES_PASSWORD} psql -h postgres -U \$\${POSTGRES_USER} -c "CREATE DATABASE mlflow"

        echo "Running MLflow database migrations..."
        mlflow db upgrade \$\${MLFLOW_BACKEND_STORE_URI}

        echo "Starting MLflow server..."
        exec mlflow server \
          --backend-store-uri \$\${MLFLOW_BACKEND_STORE_URI} \
          --default-artifact-root \$\${MLFLOW_DEFAULT_ARTIFACT_ROOT} \
          --host \$\${MLFLOW_HOST} \
          --port \$\${MLFLOW_PORT} \
          --serve-artifacts
    volumes:
      - mlflow_data:/mlflow/artifacts
    # SECURITY: Bound to 127.0.0.1 only - access via nginx proxy
    ports:
      - "127.0.0.1:\${MLFLOW_PORT:-5005}:\${MLFLOW_PORT:-5005}"
    healthcheck:
      test: ["CMD", "python", "-c", "import urllib.request; urllib.request.urlopen('http://localhost:\${MLFLOW_PORT:-5005}/health')"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 120s
EOF
}

# Export function
export -f generate_mlflow_service_improved