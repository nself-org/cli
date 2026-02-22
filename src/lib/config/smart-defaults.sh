#!/usr/bin/env bash


# Smart Defaults System for nself
# Provides default values for all configuration options
# Priority: .env > .env.local > defaults

# Generate a secure random password
generate_password() {
  local length="${1:-16}"
  if command -v openssl >/dev/null 2>&1; then
    local bytes=$(( (length * 4 / 3) + 16 ))
    openssl rand -base64 "$bytes" | tr -d "=+/\n" | head -c "$length"
  else
    LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c "$length"
  fi
}

# Apply smart defaults for any missing environment variables
apply_smart_defaults() {
  # Core Settings
  : ${ENV:=dev}
  : ${PROJECT_NAME:=myproject}
  : ${PROJECT_DESCRIPTION:=""}
  : ${BASE_DOMAIN:=local.nself.org}
  : ${ADMIN_EMAIL:=""}
  : ${DB_ENV_SEEDS:=true}

  # PostgreSQL
  : ${POSTGRES_VERSION:=16-alpine}
  : ${POSTGRES_HOST:=postgres}
  # CRITICAL: Internal port must always be 5432 for container-to-container communication
  # External port can be different for host access
  : ${POSTGRES_INTERNAL_PORT:=5432}
  : ${POSTGRES_PORT:=${POSTGRES_EXTERNAL_PORT:-5432}} # External port for host access
  : ${POSTGRES_DB:=nhost}
  : ${POSTGRES_USER:=postgres}
  : ${POSTGRES_PASSWORD:=postgres-dev-password}

  # Construct database URL - ALWAYS use internal port 5432 for service-to-service communication
  : ${HASURA_GRAPHQL_DATABASE_URL:=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}}
  : ${POSTGRES_EXTENSIONS:=uuid-ossp}

  # Hasura
  : ${HASURA_VERSION:=v2.44.0}
  : ${HASURA_GRAPHQL_ADMIN_SECRET:=hasura-admin-secret-dev}

  # JWT Configuration - Support both new simple format and legacy JSON format
  : ${HASURA_JWT_KEY:=development-secret-key-minimum-32-characters-long}
  : ${JWT_KEY:=$HASURA_JWT_KEY}
  : ${HASURA_JWT_TYPE:=HS256}

  # If HASURA_GRAPHQL_JWT_SECRET is not set, construct it from the simple variables
  if [[ -z "${HASURA_GRAPHQL_JWT_SECRET:-}" ]]; then
    HASURA_GRAPHQL_JWT_SECRET="{\"type\":\"${HASURA_JWT_TYPE}\",\"key\":\"${HASURA_JWT_KEY}\"}"
  fi

  # Set console/dev mode based on ENV
  if [[ "$ENV" == "prod" ]]; then
    : ${HASURA_GRAPHQL_ENABLE_CONSOLE:=false}
    : ${HASURA_GRAPHQL_DEV_MODE:=false}
  else
    : ${HASURA_GRAPHQL_ENABLE_CONSOLE:=true}
    : ${HASURA_GRAPHQL_DEV_MODE:=true}
  fi

  : ${HASURA_GRAPHQL_ENABLE_TELEMETRY:=false}
  : ${HASURA_GRAPHQL_CORS_DOMAIN:=*}
  : ${HASURA_ROUTE:=api.${BASE_DOMAIN}}

  # Auth
  : ${AUTH_VERSION:=0.36.0}
  : ${AUTH_HOST:=auth}
  : ${AUTH_PORT:=4000}
  : ${AUTH_CLIENT_URL:=http://localhost:3000}
  : ${AUTH_JWT_REFRESH_TOKEN_EXPIRES_IN:=2592000}
  : ${AUTH_JWT_ACCESS_TOKEN_EXPIRES_IN:=900}
  : ${AUTH_WEBAUTHN_ENABLED:=false}
  : ${AUTH_ROUTE:=auth.${BASE_DOMAIN}}

  # Email (Development defaults to MailPit)
  : ${AUTH_SMTP_HOST:=mailpit}
  : ${AUTH_SMTP_PORT:=1025}
  : ${AUTH_SMTP_USER:=""}
  : ${AUTH_SMTP_PASS:=""}
  : ${AUTH_SMTP_SECURE:=false}
  : ${AUTH_SMTP_SENDER:=noreply@${BASE_DOMAIN}}

  # Storage
  : ${STORAGE_VERSION:=0.6.1}
  : ${STORAGE_ROUTE:=storage.${BASE_DOMAIN}}
  : ${STORAGE_CONSOLE_ROUTE:=storage-console.${BASE_DOMAIN}}
  : ${MINIO_VERSION:=latest}
  : ${MINIO_PORT:=9000}
  : ${MINIO_ROOT_USER:=minioadmin}
  : ${MINIO_ROOT_PASSWORD:=minioadmin}
  : ${S3_ACCESS_KEY:=storage-access-key-dev}
  : ${S3_SECRET_KEY:=storage-secret-key-dev}
  : ${S3_BUCKET:=nhost}
  : ${S3_REGION:=us-east-1}

  # Nginx
  : ${NGINX_VERSION:=alpine}
  : ${NGINX_HTTP_PORT:=80}
  : ${NGINX_HTTPS_PORT:=443}
  : ${NGINX_CLIENT_MAX_BODY_SIZE:=100M}
  : ${NGINX_GZIP_ENABLED:=true}
  : ${NGINX_RATE_LIMIT:=""}

  # SSL
  : ${SSL_MODE:=local}

  # Service Enable Flags (core services default to true for backward compatibility)
  : ${POSTGRES_ENABLED:=true}
  : ${HASURA_ENABLED:=true}
  : ${AUTH_ENABLED:=true}
  : ${STORAGE_ENABLED:=true}
  : ${NSELF_ADMIN_ENABLED:=false}

  # Map deprecated variable names for backward compatibility
  if [[ "${NADMIN_ENABLED:-}" == "true" ]]; then
    NSELF_ADMIN_ENABLED=true
  fi

  # Map STORAGE_ENABLED to MINIO for backward compatibility
  if [[ "$STORAGE_ENABLED" == "true" ]]; then
    MINIO_ENABLED=true
  elif [[ "${MINIO_ENABLED:-}" == "true" ]]; then
    STORAGE_ENABLED=true
  fi

  # Optional Services (all disabled by default)
  : ${MONITORING_ENABLED:=false}
  : ${MAILPIT_ENABLED:=false}
  : ${MEILISEARCH_ENABLED:=false}
  : ${FUNCTIONS_ENABLED:=false}
  : ${FUNCTIONS_ROUTE:=functions.${BASE_DOMAIN}}
  : ${MLFLOW_ENABLED:=false}
  : ${MLFLOW_USERNAME:=admin}
  : ${MLFLOW_PASSWORD:=${ADMIN_PASSWORD:-$(generate_password 16)}}
  : ${DASHBOARD_ENABLED:=false}
  : ${DASHBOARD_VERSION:=latest}
  : ${DASHBOARD_ROUTE:=dashboard.${BASE_DOMAIN}}
  : ${REDIS_ENABLED:=false}
  : ${REDIS_VERSION:=7-alpine}
  : ${REDIS_PORT:=6379}
  : ${REDIS_PASSWORD:=""}

  # MLflow - ML Experiment Tracking
  : ${MLFLOW_ENABLED:=false}
  : ${MLFLOW_VERSION:=2.9.2}
  : ${MLFLOW_PORT:=5000}
  : ${MLFLOW_ROUTE:=mlflow.${BASE_DOMAIN}}
  : ${MLFLOW_DB_NAME:=mlflow}
  : ${MLFLOW_ARTIFACTS_BUCKET:=mlflow-artifacts}
  : ${MLFLOW_AUTH_ENABLED:=false}
  : ${MLFLOW_AUTH_USERNAME:=admin}
  : ${MLFLOW_AUTH_PASSWORD:=mlflow-admin-password}

  # Search Services Configuration
  : ${SEARCH_ENABLED:=false}
  : ${SEARCH_ENGINE:=meilisearch} # meilisearch, typesense, elasticsearch, opensearch, zinc, sonic

  # Meilisearch (Default - Best for most use cases)
  : ${MEILISEARCH_VERSION:=v1.6}
  : ${MEILISEARCH_PORT:=7700}
  : ${MEILISEARCH_MASTER_KEY:=meilisearch-master-key-minimum-16-chars}
  : ${MEILISEARCH_ROUTE:=search.${BASE_DOMAIN}}
  : ${MEILISEARCH_ENV:=development}

  # Typesense (High-performance alternative)
  : ${TYPESENSE_VERSION:=26.0}
  : ${TYPESENSE_PORT:=8108}
  : ${TYPESENSE_API_KEY:=typesense-api-key-minimum-32-chars}
  : ${TYPESENSE_ROUTE:=search.${BASE_DOMAIN}}

  # Elasticsearch (Industry standard, resource heavy)
  : ${ELASTICSEARCH_VERSION:=8.11.3}
  : ${ELASTICSEARCH_PORT:=9200}
  : ${ELASTICSEARCH_PASSWORD:=elasticsearch-password}
  : ${ELASTICSEARCH_ROUTE:=search.${BASE_DOMAIN}}
  : ${ELASTICSEARCH_MEMORY:=1Gi}

  # OpenSearch (AWS fork of Elasticsearch)
  : ${OPENSEARCH_VERSION:=2.11.1}
  : ${OPENSEARCH_PORT:=9200}
  : ${OPENSEARCH_PASSWORD:=opensearch-password}
  : ${OPENSEARCH_ROUTE:=search.${BASE_DOMAIN}}
  : ${OPENSEARCH_MEMORY:=1Gi}

  # Zinc (Lightweight Elasticsearch alternative in Go)
  : ${ZINC_VERSION:=0.4.9}
  : ${ZINC_PORT:=4080}
  : ${ZINC_ADMIN_USER:=admin}
  : ${ZINC_ADMIN_PASSWORD:=zinc-admin-password}
  : ${ZINC_ROUTE:=search.${BASE_DOMAIN}}

  # Sonic (Ultra-lightweight, schema-less)
  : ${SONIC_VERSION:=1.4.8}
  : ${SONIC_PORT:=1491}
  : ${SONIC_PASSWORD:=sonic-password}
  : ${SONIC_ROUTE:=search.${BASE_DOMAIN}}

  # Email Provider
  : ${EMAIL_PROVIDER:=mailpit}
  : ${MAILPIT_SMTP_PORT:=1025}
  : ${MAILPIT_UI_PORT:=8025}
  : ${MAILPIT_ROUTE:=mail.${BASE_DOMAIN}}
  : ${EMAIL_FROM:=noreply@${BASE_DOMAIN}}

  # Microservices (all disabled by default)
  : ${SERVICES_ENABLED:=false}
  : ${NESTJS_ENABLED:=false}
  : ${NESTJS_SERVICES:=""}
  : ${NESTJS_USE_TYPESCRIPT:=true}
  : ${NESTJS_PORT_START:=3100}
  : ${BULLMQ_ENABLED:=false}
  : ${BULLMQ_WORKERS:=""}
  : ${BULLMQ_DASHBOARD_ENABLED:=false}
  : ${BULLMQ_DASHBOARD_PORT:=4200}
  : ${BULLMQ_DASHBOARD_ROUTE:=queues.${BASE_DOMAIN}}
  : ${GOLANG_ENABLED:=false}
  : ${GOLANG_SERVICES:=""}
  : ${GOLANG_PORT_START:=3200}
  : ${PYTHON_ENABLED:=false}
  : ${PYTHON_SERVICES:=""}
  : ${PYTHON_FRAMEWORK:=fastapi}
  : ${PYTHON_PORT_START:=3300}
  : ${NESTJS_RUN_ENABLED:=false}
  : ${NESTJS_RUN_PORT:=3400}

  # Advanced/Internal
  : ${HASURA_METADATA_DATABASE_URL:=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}}
  : ${DOCKER_NETWORK:=${PROJECT_NAME}_network}
  : ${HASURA_PORT:=8080}
  : ${HASURA_CONSOLE_PORT:=9695}
  : ${FUNCTIONS_PORT:=4300}
  : ${MLFLOW_PORT:=5000}
  : ${DASHBOARD_PORT:=4500}
  : ${STORAGE_PORT:=5001}
  : ${S3_ENDPOINT:=http://minio:${MINIO_PORT}}
  : ${FILES_ROUTE:=files.${BASE_DOMAIN}}
  : ${MAIL_ROUTE:=mail.${BASE_DOMAIN}}

  # Export all variables
  export ENV PROJECT_NAME PROJECT_DESCRIPTION BASE_DOMAIN ADMIN_EMAIL DB_ENV_SEEDS
  export POSTGRES_ENABLED HASURA_ENABLED AUTH_ENABLED STORAGE_ENABLED NSELF_ADMIN_ENABLED MINIO_ENABLED
  export POSTGRES_VERSION POSTGRES_HOST POSTGRES_PORT POSTGRES_DB POSTGRES_USER POSTGRES_PASSWORD POSTGRES_EXTENSIONS
  export HASURA_VERSION HASURA_GRAPHQL_ADMIN_SECRET HASURA_GRAPHQL_JWT_SECRET
  export HASURA_JWT_KEY HASURA_JWT_TYPE
  export HASURA_GRAPHQL_ENABLE_CONSOLE HASURA_GRAPHQL_DEV_MODE HASURA_GRAPHQL_ENABLE_TELEMETRY
  export HASURA_GRAPHQL_CORS_DOMAIN HASURA_ROUTE
  export AUTH_VERSION AUTH_HOST AUTH_PORT AUTH_CLIENT_URL
  export AUTH_JWT_REFRESH_TOKEN_EXPIRES_IN AUTH_JWT_ACCESS_TOKEN_EXPIRES_IN
  export AUTH_WEBAUTHN_ENABLED AUTH_ROUTE
  export AUTH_SMTP_HOST AUTH_SMTP_PORT AUTH_SMTP_USER AUTH_SMTP_PASS AUTH_SMTP_SECURE AUTH_SMTP_SENDER
  export STORAGE_VERSION STORAGE_ROUTE STORAGE_CONSOLE_ROUTE
  export MINIO_VERSION MINIO_PORT MINIO_ROOT_USER MINIO_ROOT_PASSWORD
  export S3_ACCESS_KEY S3_SECRET_KEY S3_BUCKET S3_REGION
  export NGINX_VERSION NGINX_HTTP_PORT NGINX_HTTPS_PORT NGINX_CLIENT_MAX_BODY_SIZE NGINX_GZIP_ENABLED NGINX_RATE_LIMIT
  export SSL_MODE
  export MONITORING_ENABLED MAILPIT_ENABLED MEILISEARCH_ENABLED
  export FUNCTIONS_ENABLED FUNCTIONS_ROUTE
  export DASHBOARD_ENABLED DASHBOARD_VERSION DASHBOARD_ROUTE
  export REDIS_ENABLED REDIS_VERSION REDIS_PORT REDIS_PASSWORD
  export MLFLOW_ENABLED MLFLOW_VERSION MLFLOW_PORT MLFLOW_ROUTE
  export MLFLOW_DB_NAME MLFLOW_ARTIFACTS_BUCKET MLFLOW_AUTH_ENABLED
  export MLFLOW_AUTH_USERNAME MLFLOW_AUTH_PASSWORD
  export SEARCH_ENABLED SEARCH_ENGINE
  export MEILISEARCH_VERSION MEILISEARCH_PORT MEILISEARCH_MASTER_KEY MEILISEARCH_ROUTE MEILISEARCH_ENV
  export TYPESENSE_VERSION TYPESENSE_PORT TYPESENSE_API_KEY TYPESENSE_ROUTE
  export ELASTICSEARCH_VERSION ELASTICSEARCH_PORT ELASTICSEARCH_PASSWORD ELASTICSEARCH_ROUTE ELASTICSEARCH_MEMORY
  export OPENSEARCH_VERSION OPENSEARCH_PORT OPENSEARCH_PASSWORD OPENSEARCH_ROUTE OPENSEARCH_MEMORY
  export ZINC_VERSION ZINC_PORT ZINC_ADMIN_USER ZINC_ADMIN_PASSWORD ZINC_ROUTE
  export SONIC_VERSION SONIC_PORT SONIC_PASSWORD SONIC_ROUTE
  export EMAIL_PROVIDER MAILPIT_SMTP_PORT MAILPIT_UI_PORT MAILPIT_ROUTE EMAIL_FROM
  export SERVICES_ENABLED
  export NESTJS_ENABLED NESTJS_SERVICES NESTJS_USE_TYPESCRIPT NESTJS_PORT_START
  export BULLMQ_ENABLED BULLMQ_WORKERS BULLMQ_DASHBOARD_ENABLED BULLMQ_DASHBOARD_PORT BULLMQ_DASHBOARD_ROUTE
  export GOLANG_ENABLED GOLANG_SERVICES GOLANG_PORT_START
  export PYTHON_ENABLED PYTHON_SERVICES PYTHON_FRAMEWORK PYTHON_PORT_START
  export NESTJS_RUN_ENABLED NESTJS_RUN_PORT
  export HASURA_METADATA_DATABASE_URL DOCKER_NETWORK
  export HASURA_PORT HASURA_CONSOLE_PORT FUNCTIONS_PORT DASHBOARD_PORT STORAGE_PORT
  export S3_ENDPOINT FILES_ROUTE MAIL_ROUTE
}

# Load environment files with proper priority
load_env_with_defaults() {
  # Source safe env parser if available
  local _sd_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  if command -v safe_source_env >/dev/null 2>&1; then
    : # Already available
  elif [[ -f "$_sd_script_dir/../utils/env.sh" ]]; then
    source "$_sd_script_dir/../utils/env.sh"
  fi

  # Use safe_source_env if available, fallback to source for .env files
  _sd_load_env() {
    local file="$1"
    [[ ! -f "$file" ]] && return 1
    if command -v safe_source_env >/dev/null 2>&1; then
      safe_source_env "$file"
    else
      set -a
      source "$file" 2>/dev/null
      set +a
    fi
  }

  # Load .env.dev first (team defaults)
  _sd_load_env ".env.dev"

  # Normalize ENV after loading
  case "${ENV:-dev}" in
    development | develop | devel)
      export ENV="dev"
      ;;
    production | prod)
      export ENV="prod"
      ;;
    staging | stage)
      export ENV="staging"
      ;;
  esac

  # Load environment-specific files
  if [[ "${ENV:-}" == "staging" ]]; then
    _sd_load_env ".env.staging"
  elif [[ "${ENV:-}" == "prod" ]]; then
    _sd_load_env ".env.prod"
    _sd_load_env ".env.secrets"
  fi

  # Load .env.local if it exists (alternative to .env)
  _sd_load_env ".env.local"

  # Load local overrides last (HIGHEST PRIORITY)
  _sd_load_env ".env"

  # Apply smart defaults for any missing values
  apply_smart_defaults

  # Re-construct JWT secret if using simple format
  if [[ -z "${HASURA_GRAPHQL_JWT_SECRET:-}" ]] && [[ -n "${HASURA_JWT_KEY:-}" ]]; then
    : ${HASURA_JWT_TYPE:=HS256}
    HASURA_GRAPHQL_JWT_SECRET="{\"type\":\"${HASURA_JWT_TYPE}\",\"key\":\"${HASURA_JWT_KEY}\"}"
  fi

  # Re-apply computed values that depend on other vars
  : ${HASURA_ROUTE:=api.${BASE_DOMAIN}}
  : ${AUTH_ROUTE:=auth.${BASE_DOMAIN}}
  : ${STORAGE_ROUTE:=storage.${BASE_DOMAIN}}
  : ${STORAGE_CONSOLE_ROUTE:=storage-console.${BASE_DOMAIN}}
  : ${FUNCTIONS_ROUTE:=functions.${BASE_DOMAIN}}
  : ${DASHBOARD_ROUTE:=dashboard.${BASE_DOMAIN}}
  : ${MAILPIT_ROUTE:=mail.${BASE_DOMAIN}}
  : ${BULLMQ_DASHBOARD_ROUTE:=queues.${BASE_DOMAIN}}
  : ${MLFLOW_ROUTE:=mlflow.${BASE_DOMAIN}}
  : ${AUTH_SMTP_SENDER:=noreply@${BASE_DOMAIN}}
  : ${EMAIL_FROM:=noreply@${BASE_DOMAIN}}
  : ${FILES_ROUTE:=files.${BASE_DOMAIN}}
  : ${MAIL_ROUTE:=mail.${BASE_DOMAIN}}
  : ${DOCKER_NETWORK:=${PROJECT_NAME}_network}
  : ${HASURA_METADATA_DATABASE_URL:=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}}
  : ${S3_ENDPOINT:=http://minio:${MINIO_PORT}}

  # Export computed values
  export HASURA_ROUTE AUTH_ROUTE STORAGE_ROUTE STORAGE_CONSOLE_ROUTE
  export FUNCTIONS_ROUTE DASHBOARD_ROUTE MAILPIT_ROUTE BULLMQ_DASHBOARD_ROUTE MLFLOW_ROUTE
  export AUTH_SMTP_SENDER EMAIL_FROM FILES_ROUTE MAIL_ROUTE
  export DOCKER_NETWORK HASURA_METADATA_DATABASE_URL S3_ENDPOINT
  export HASURA_GRAPHQL_JWT_SECRET HASURA_JWT_KEY HASURA_JWT_TYPE
}

export -f apply_smart_defaults
export -f load_env_with_defaults
