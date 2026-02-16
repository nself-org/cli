#!/usr/bin/env bash
set -euo pipefail

# services.sh - Generate services directory structure

# Source utility functions
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../../lib/utils/env.sh"
source "$SCRIPT_DIR/../../lib/utils/display.sh"
source "$SCRIPT_DIR/../../lib/utils/platform-compat.sh" 2>/dev/null || true

# Load environment safely (without executing JSON values)
if [ -f ".env.local" ]; then
  load_env_with_priority ".env.local"
else
  log_error "No .env.local file found."
  exit 1
fi

TEMPLATES_DIR="$SCRIPT_DIR/../../templates/services"

# Create base services directory
mkdir -p services

# Helper function to convert kebab-case to camelCase
to_camel_case() {
  echo "$1" | sed -E 's/-([a-z])/\U\1/g'
}

# Helper function to convert kebab-case to PascalCase
to_pascal_case() {
  echo "$1" | sed -r 's/(^|-)([a-z])/\U\2/g'
}

# Generate NestJS services
if [[ "$NESTJS_ENABLED" == "true" ]]; then
  log_info "Generating NestJS services..."

  mkdir -p services/nest

  # Parse services list
  IFS=',' read -ra NEST_SERVICES <<<"$NESTJS_SERVICES"
  PORT_COUNTER=0

  for service in "${NEST_SERVICES[@]}"; do
    service=$(echo "$service" | xargs) # Trim whitespace
    SERVICE_PORT=$((NESTJS_PORT_START + PORT_COUNTER))
    SERVICE_NAME_CAMEL=$(to_camel_case "$service")

    log_info "Creating NestJS service: $service"

    # Create service directory
    mkdir -p "services/nest/$service"

    # Generate package.json
    if [[ "$NESTJS_USE_TYPESCRIPT" == "true" ]]; then
      cp "$TEMPLATES_DIR/nest/package.json.template" "services/nest/$service/package.json"
    else
      cp "$TEMPLATES_DIR/nest/package-js.json.template" "services/nest/$service/package.json"
    fi

    # Replace template variables
    # Use platform-safe sed
    if command -v safe_sed_inline >/dev/null 2>&1; then
      safe_sed_inline "services/nest/$service/package.json" "s/\${SERVICE_NAME}/$service/g"
      safe_sed_inline "services/nest/$service/package.json" "s/\${PROJECT_NAME}/$PROJECT_NAME/g"
    else
      sed -i.bak "s/\${SERVICE_NAME}/$service/g" "services/nest/$service/package.json"
      sed -i.bak "s/\${PROJECT_NAME}/$PROJECT_NAME/g" "services/nest/$service/package.json"
      rm -f "services/nest/$service/package.json.bak"
    fi
    rm -f "services/nest/$service/package.json.bak"

    # Generate source files
    mkdir -p "services/nest/$service/src"

    if [[ "$NESTJS_USE_TYPESCRIPT" == "true" ]]; then
      # TypeScript files - use specialized templates if available
      if [[ "$service" == "actions-api" ]] || [[ "$service" == "weather-actions" ]]; then
        # Use weather actions template for weather-specific services
        for template in main.ts app.module.ts app.controller.ts app.service.ts weather-actions.controller.ts weather.service.ts; do
          if [[ -f "$TEMPLATES_DIR/nest/$template.template" ]]; then
            cp "$TEMPLATES_DIR/nest/$template.template" "services/nest/$service/src/$template"
            # Use platform-safe sed
            if command -v safe_sed_inline >/dev/null 2>&1; then
              safe_sed_inline "services/nest/$service/src/$template" "s/\${SERVICE_NAME}/$service/g"
              safe_sed_inline "services/nest/$service/src/$template" "s/\${SERVICE_PORT}/$SERVICE_PORT/g"
              safe_sed_inline "services/nest/$service/src/$template" "s/\${PROJECT_NAME}/$PROJECT_NAME/g"
            else
              sed -i.bak "s/\${SERVICE_NAME}/$service/g" "services/nest/$service/src/$template"
              sed -i.bak "s/\${SERVICE_PORT}/$SERVICE_PORT/g" "services/nest/$service/src/$template"
              sed -i.bak "s/\${PROJECT_NAME}/$PROJECT_NAME/g" "services/nest/$service/src/$template"
              rm -f "services/nest/$service/src/$template.bak"
            fi
            rm -f "services/nest/$service/src/$template.bak"
          fi
        done
      else
        # Use generic templates
        for template in main.ts app.module.ts app.controller.ts app.service.ts; do
          cp "$TEMPLATES_DIR/nest/$template.template" "services/nest/$service/src/$template"
          sed -i.bak "s/\${SERVICE_NAME}/$service/g" "services/nest/$service/src/$template"
          sed -i.bak "s/\${SERVICE_PORT}/$SERVICE_PORT/g" "services/nest/$service/src/$template"
          sed -i.bak "s/\${PROJECT_NAME}/$PROJECT_NAME/g" "services/nest/$service/src/$template"
          rm -f "services/nest/$service/src/$template.bak"
        done
      fi

      # Create tsconfig.json
      cat >"services/nest/$service/tsconfig.json" <<EOF
{
  "compilerOptions": {
    "module": "commonjs",
    "declaration": true,
    "removeComments": true,
    "emitDecoratorMetadata": true,
    "experimentalDecorators": true,
    "allowSyntheticDefaultImports": true,
    "target": "es2017",
    "sourceMap": true,
    "outDir": "./dist",
    "baseUrl": "./",
    "incremental": true,
    "skipLibCheck": true,
    "strict": true,
    "noImplicitReturns": true,
    "noFallthroughCasesInSwitch": true
  }
}
EOF

      BUILD_STEP="RUN npm run build"
      START_COMMAND="npm run start:prod"
    else
      # JavaScript files
      for template in main.js; do
        cp "$TEMPLATES_DIR/nest/$template.template" "services/nest/$service/src/$template"
        sed -i.bak "s/\${SERVICE_NAME}/$service/g" "services/nest/$service/src/$template"
        sed -i.bak "s/\${SERVICE_PORT}/$SERVICE_PORT/g" "services/nest/$service/src/$template"
        rm -f "services/nest/$service/src/$template.bak"
      done

      BUILD_STEP="# No build step needed for JavaScript"
      START_COMMAND="node src/main.js"
    fi

    # Generate Dockerfile (use | as delimiter to avoid issues with /)
    cp "$TEMPLATES_DIR/nest/Dockerfile.template" "services/nest/$service/Dockerfile"
    sed -i.bak "s|\${SERVICE_PORT}|$SERVICE_PORT|g" "services/nest/$service/Dockerfile"
    sed -i.bak "s|\${BUILD_STEP}|$BUILD_STEP|g" "services/nest/$service/Dockerfile"
    sed -i.bak "s|\${START_COMMAND}|$START_COMMAND|g" "services/nest/$service/Dockerfile"
    rm -f "services/nest/$service/Dockerfile.bak"

    # Generate package-lock.json
    log_info "  Generating package-lock.json for $service..."
    (cd "services/nest/$service" && npm install --package-lock-only >/dev/null 2>&1) || {
      # If npm install fails, create a basic package-lock.json
      cat >"services/nest/$service/package-lock.json" <<EOF
{
  "name": "${PROJECT_NAME}-${service}",
  "version": "1.0.0",
  "lockfileVersion": 2,
  "requires": true,
  "packages": {}
}
EOF
    }

    PORT_COUNTER=$((PORT_COUNTER + 1))
  done

  # Create shared utilities
  mkdir -p services/nest/shared
  cat >services/nest/shared/hasura.ts <<EOF
// Shared Hasura utilities
export const HASURA_ENDPOINT = process.env.HASURA_ENDPOINT || 'http://hasura:8080/v1/graphql';
export const HASURA_ADMIN_SECRET = process.env.HASURA_ADMIN_SECRET;

export const hasuraQuery = async (query: string, variables?: any) => {
  const response = await fetch(HASURA_ENDPOINT, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Hasura-Admin-Secret': HASURA_ADMIN_SECRET,
    },
    body: JSON.stringify({ query, variables }),
  });
  
  return response.json();
};
EOF
fi

# Generate BullMQ workers
if [[ "$BULLMQ_ENABLED" == "true" ]]; then
  log_info "Generating BullMQ workers..."

  mkdir -p services/bullmq

  # Parse workers list
  IFS=',' read -ra BULLMQ_WORKER_LIST <<<"$BULLMQ_WORKERS"

  for worker in "${BULLMQ_WORKER_LIST[@]}"; do
    worker=$(echo "$worker" | xargs) # Trim whitespace
    WORKER_NAME_CAMEL=$(to_camel_case "$worker")

    log_info "Creating BullMQ worker: $worker"

    # Create worker directory
    mkdir -p "services/bullmq/$worker"

    # Generate package.json
    cp "$TEMPLATES_DIR/bullmq/package.json.template" "services/bullmq/$worker/package.json"
    sed -i.bak "s/\${WORKER_NAME}/$worker/g" "services/bullmq/$worker/package.json"
    sed -i.bak "s/\${PROJECT_NAME}/$PROJECT_NAME/g" "services/bullmq/$worker/package.json"
    rm -f "services/bullmq/$worker/package.json.bak"

    # Generate worker file - use specialized templates if available
    if [[ "$worker" == "weather-processor" ]] && [[ -f "$TEMPLATES_DIR/bullmq/weather-processor.worker.ts.template" ]]; then
      cp "$TEMPLATES_DIR/bullmq/weather-processor.worker.ts.template" "services/bullmq/$worker/worker.ts"
    elif [[ "$worker" == "currency-processor" ]] && [[ -f "$TEMPLATES_DIR/bullmq/currency-processor.worker.ts.template" ]]; then
      cp "$TEMPLATES_DIR/bullmq/currency-processor.worker.ts.template" "services/bullmq/$worker/worker.ts"
    else
      cp "$TEMPLATES_DIR/bullmq/worker.ts.template" "services/bullmq/$worker/worker.ts"
      sed -i.bak "s/\${WORKER_NAME}/$worker/g" "services/bullmq/$worker/worker.ts"
      sed -i.bak "s/\${WORKER_NAME_CAMEL}/$WORKER_NAME_CAMEL/g" "services/bullmq/$worker/worker.ts"
      rm -f "services/bullmq/$worker/worker.ts.bak"
    fi

    # Generate Dockerfile
    cp "$TEMPLATES_DIR/bullmq/Dockerfile.template" "services/bullmq/$worker/Dockerfile"
    sed -i.bak "s/\${BULLMQ_DASHBOARD_PORT}/$BULLMQ_DASHBOARD_PORT/g" "services/bullmq/$worker/Dockerfile"
    rm -f "services/bullmq/$worker/Dockerfile.bak"

    # Create TypeScript config
    cat >"services/bullmq/$worker/tsconfig.json" <<EOF
{
  "compilerOptions": {
    "target": "es2018",
    "module": "commonjs",
    "lib": ["es2018"],
    "outDir": "./dist",
    "rootDir": "./",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true
  },
  "include": ["**/*"],
  "exclude": ["node_modules", "dist"]
}
EOF

    # Generate package-lock.json
    log_info "  Generating package-lock.json for $worker..."
    (cd "services/bullmq/$worker" && npm install --package-lock-only >/dev/null 2>&1) || {
      # If npm install fails, create a basic package-lock.json
      cat >"services/bullmq/$worker/package-lock.json" <<EOF
{
  "name": "${PROJECT_NAME}-${worker}",
  "version": "1.0.0",
  "lockfileVersion": 2,
  "requires": true,
  "packages": {}
}
EOF
    }
  done
fi

# Generate GoLang services
if [[ "$GOLANG_ENABLED" == "true" ]]; then
  log_info "Generating GoLang services..."

  mkdir -p services/go

  # Parse services list
  IFS=',' read -ra GO_SERVICES <<<"$GOLANG_SERVICES"
  PORT_COUNTER=0

  for service in "${GO_SERVICES[@]}"; do
    service=$(echo "$service" | xargs) # Trim whitespace
    SERVICE_PORT=$((GOLANG_PORT_START + PORT_COUNTER))
    SERVICE_NAME_CAMEL=$(to_camel_case "$service")

    log_info "Creating GoLang service: $service"

    # Create service directory
    mkdir -p "services/go/$service"

    # Generate main.go - use specialized templates if available
    if [[ "$service" == "currency-fetcher" ]] && [[ -f "$TEMPLATES_DIR/go/currency-fetcher.go.template" ]]; then
      cp "$TEMPLATES_DIR/go/currency-fetcher.go.template" "services/go/$service/main.go"
      sed -i.bak "s/\${SERVICE_NAME}/$service/g" "services/go/$service/main.go"
      sed -i.bak "s/\${SERVICE_PORT}/$SERVICE_PORT/g" "services/go/$service/main.go"
      sed -i.bak "s/\${PROJECT_NAME}/$PROJECT_NAME/g" "services/go/$service/main.go"
      rm -f "services/go/$service/main.go.bak"
    else
      cp "$TEMPLATES_DIR/go/main.go.template" "services/go/$service/main.go"
      sed -i.bak "s/\${SERVICE_NAME}/$service/g" "services/go/$service/main.go"
      sed -i.bak "s/\${SERVICE_NAME_CAMEL}/$SERVICE_NAME_CAMEL/g" "services/go/$service/main.go"
      sed -i.bak "s/\${SERVICE_PORT}/$SERVICE_PORT/g" "services/go/$service/main.go"
      sed -i.bak "s/\${PROJECT_NAME}/$PROJECT_NAME/g" "services/go/$service/main.go"
      rm -f "services/go/$service/main.go.bak"
    fi

    # Generate go.mod
    cp "$TEMPLATES_DIR/go/go.mod.template" "services/go/$service/go.mod"
    sed -i.bak "s/\${SERVICE_NAME}/$service/g" "services/go/$service/go.mod"
    rm -f "services/go/$service/go.mod.bak"

    # Generate Dockerfile
    cp "$TEMPLATES_DIR/go/Dockerfile.template" "services/go/$service/Dockerfile"
    sed -i.bak "s/\${SERVICE_PORT}/$SERVICE_PORT/g" "services/go/$service/Dockerfile"
    sed -i.bak "s/\${SERVICE_NAME}/$service/g" "services/go/$service/Dockerfile"
    rm -f "services/go/$service/Dockerfile.bak"

    # Generate go.sum with proper dependencies
    if command -v go &>/dev/null; then
      log_info "  Generating go.sum for $service..."
      (cd "services/go/$service" && go mod download && go mod tidy 2>/dev/null) || {
        # If go mod tidy fails, use template go.sum
        if [[ -f "$TEMPLATES_DIR/go/go.sum.template" ]]; then
          cp "$TEMPLATES_DIR/go/go.sum.template" "services/go/$service/go.sum"
        else
          touch "services/go/$service/go.sum"
        fi
      }
    else
      # Use template go.sum if go is not installed
      if [[ -f "$TEMPLATES_DIR/go/go.sum.template" ]]; then
        cp "$TEMPLATES_DIR/go/go.sum.template" "services/go/$service/go.sum"
        log_info "  Using template go.sum for $service"
      else
        touch "services/go/$service/go.sum"
        echo_warning "  Go not installed - go.sum will be empty (will be generated during Docker build)"
      fi
    fi

    PORT_COUNTER=$((PORT_COUNTER + 1))
  done

  # Create shared utilities
  mkdir -p services/go/shared
  cat >services/go/shared/config.go <<EOF
package shared

import "os"

func GetEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

var (
    HasuraEndpoint   = GetEnv("HASURA_ENDPOINT", "http://hasura:8080/v1/graphql")
    HasuraAdminSecret = GetEnv("HASURA_ADMIN_SECRET", "")
    DatabaseURL      = GetEnv("DATABASE_URL", "")
)
EOF
fi

# Generate Python services
if [[ "$PYTHON_ENABLED" == "true" ]]; then
  log_info "Generating Python services..."

  mkdir -p services/py

  # Parse services list
  IFS=',' read -ra PY_SERVICES <<<"$PYTHON_SERVICES"
  PORT_COUNTER=0

  for service in "${PY_SERVICES[@]}"; do
    service=$(echo "$service" | xargs) # Trim whitespace
    SERVICE_PORT=$((PYTHON_PORT_START + PORT_COUNTER))

    log_info "Creating Python service: $service"

    # Create service directory
    mkdir -p "services/py/$service"

    # Generate main.py - use specialized templates if available
    if [[ "$service" == "data-analyzer" ]] && [[ -f "$TEMPLATES_DIR/py/data-analyzer.py.template" ]]; then
      cp "$TEMPLATES_DIR/py/data-analyzer.py.template" "services/py/$service/main.py"
      sed -i.bak "s/\${SERVICE_NAME}/$service/g" "services/py/$service/main.py"
      sed -i.bak "s/\${SERVICE_PORT}/$SERVICE_PORT/g" "services/py/$service/main.py"
      sed -i.bak "s/\${PROJECT_NAME}/$PROJECT_NAME/g" "services/py/$service/main.py"
      rm -f "services/py/$service/main.py.bak"
    else
      cp "$TEMPLATES_DIR/py/main.py.template" "services/py/$service/main.py"
      sed -i.bak "s/\${SERVICE_NAME}/$service/g" "services/py/$service/main.py"
      sed -i.bak "s/\${SERVICE_PORT}/$SERVICE_PORT/g" "services/py/$service/main.py"
      sed -i.bak "s/\${PROJECT_NAME}/$PROJECT_NAME/g" "services/py/$service/main.py"
      rm -f "services/py/$service/main.py.bak"
    fi

    # Generate requirements.txt
    cp "$TEMPLATES_DIR/py/requirements.txt.template" "services/py/$service/requirements.txt"

    # Generate Dockerfile
    cp "$TEMPLATES_DIR/py/Dockerfile.template" "services/py/$service/Dockerfile"
    sed -i.bak "s/\${SERVICE_PORT}/$SERVICE_PORT/g" "services/py/$service/Dockerfile"
    rm -f "services/py/$service/Dockerfile.bak"

    PORT_COUNTER=$((PORT_COUNTER + 1))
  done

  # Create shared utilities
  mkdir -p services/py/shared
  cat >services/py/shared/config.py <<EOF
"""Shared configuration and utilities for Python services."""
import os

# Environment variables
HASURA_ENDPOINT = os.getenv('HASURA_ENDPOINT', 'http://hasura:8080/v1/graphql')
HASURA_ADMIN_SECRET = os.getenv('HASURA_ADMIN_SECRET', '')
DATABASE_URL = os.getenv('DATABASE_URL', '')
REDIS_HOST = os.getenv('REDIS_HOST', 'redis')
REDIS_PORT = int(os.getenv('REDIS_PORT', '6379'))

# Common functions
def get_env(key: str, default: str = '') -> str:
    return os.getenv(key, default)
EOF

  cat >services/py/shared/hasura.py <<EOF
"""Hasura GraphQL client utilities."""
import httpx
from typing import Dict, Any, Optional
from .config import HASURA_ENDPOINT, HASURA_ADMIN_SECRET

async def hasura_query(query: str, variables: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    """Execute a GraphQL query against Hasura."""
    async with httpx.AsyncClient() as client:
        response = await client.post(
            HASURA_ENDPOINT,
            json={'query': query, 'variables': variables or {}},
            headers={'X-Hasura-Admin-Secret': HASURA_ADMIN_SECRET}
        )
        return response.json()
EOF
fi

# Generate shared environment file
log_info "Creating shared environment configuration..."
cp "$TEMPLATES_DIR/shared/.env.template" "services/.env"

# Replace template variables
sed -i.bak "s/\${PROJECT_NAME}/$PROJECT_NAME/g" services/.env
sed -i.bak "s/\${ENVIRONMENT}/$ENVIRONMENT/g" services/.env
sed -i.bak "s/\${HASURA_GRAPHQL_DATABASE_URL}/$HASURA_GRAPHQL_DATABASE_URL/g" services/.env
sed -i.bak "s/\${POSTGRES_HOST}/$POSTGRES_HOST/g" services/.env
sed -i.bak "s/\${POSTGRES_PORT}/$POSTGRES_PORT/g" services/.env
sed -i.bak "s/\${POSTGRES_DB}/$POSTGRES_DB/g" services/.env
sed -i.bak "s/\${POSTGRES_USER}/$POSTGRES_USER/g" services/.env
sed -i.bak "s/\${POSTGRES_PASSWORD}/$POSTGRES_PASSWORD/g" services/.env
sed -i.bak "s/\${HASURA_GRAPHQL_ADMIN_SECRET}/$HASURA_GRAPHQL_ADMIN_SECRET/g" services/.env
sed -i.bak "s/\${REDIS_HOST}/${REDIS_HOST:-redis}/g" services/.env
sed -i.bak "s/\${REDIS_PORT}/${REDIS_PORT:-6379}/g" services/.env
sed -i.bak "s/\${REDIS_PASSWORD}/${REDIS_PASSWORD:-}/g" services/.env
sed -i.bak "s/\${SERVICES_LOG_LEVEL}/$SERVICES_LOG_LEVEL/g" services/.env
sed -i.bak "s/\${BASE_DOMAIN}/$BASE_DOMAIN/g" services/.env
rm -f services/.env.bak

# Note: Services are now integrated into main docker-compose.yml
log_success "Services directory structure created successfully!"

# Display service information
log_info "Created services:"
if [[ "$NESTJS_ENABLED" == "true" ]]; then
  log_info "  NestJS services in services/nest/"
fi
if [[ "$BULLMQ_ENABLED" == "true" ]]; then
  log_info "  BullMQ workers in services/bullmq/"
fi
if [[ "$GOLANG_ENABLED" == "true" ]]; then
  log_info "  GoLang services in services/go/"
fi
if [[ "$PYTHON_ENABLED" == "true" ]]; then
  log_info "  Python services in services/py/"
fi

log_info "To start services:"
log_info "  cd services && docker compose up -d"
