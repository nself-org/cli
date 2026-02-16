#!/usr/bin/env bash


# Schema-related fixes for auth, storage, and other services

LAST_FIX_DESCRIPTION=""

set -euo pipefail


get_last_fix_description() {
  echo "$LAST_FIX_DESCRIPTION"
}

# Fix missing database schemas
fix_missing_schemas() {
  local project_name="${PROJECT_NAME:-nself}"
  local service_name="${1:-}"

  # Ensure postgres is running first
  if ! docker exec "${project_name}_postgres" pg_isready -U postgres >/dev/null 2>&1; then
    docker compose restart postgres >/dev/null 2>&1
    sleep 5
  fi

  # Get the database name from environment or use default
  local db_name="${POSTGRES_DB:-postgres}"

  # Create schemas in the correct database
  docker exec "${project_name}_postgres" psql -U postgres -d "$db_name" -c "CREATE SCHEMA IF NOT EXISTS auth;" >/dev/null 2>&1
  docker exec "${project_name}_postgres" psql -U postgres -d "$db_name" -c "CREATE SCHEMA IF NOT EXISTS storage;" >/dev/null 2>&1
  docker exec "${project_name}_postgres" psql -U postgres -d "$db_name" -c "CREATE SCHEMA IF NOT EXISTS public;" >/dev/null 2>&1

  # Grant comprehensive permissions
  docker exec "${project_name}_postgres" psql -U postgres -d "$db_name" -c "
        -- Grant schema permissions
        GRANT ALL ON SCHEMA auth TO postgres;
        GRANT ALL ON SCHEMA storage TO postgres;
        GRANT ALL ON SCHEMA public TO postgres;
        
        -- Grant table permissions for existing tables
        GRANT ALL ON ALL TABLES IN SCHEMA auth TO postgres;
        GRANT ALL ON ALL TABLES IN SCHEMA storage TO postgres;
        GRANT ALL ON ALL TABLES IN SCHEMA public TO postgres;
        
        -- Grant sequence permissions
        GRANT ALL ON ALL SEQUENCES IN SCHEMA auth TO postgres;
        GRANT ALL ON ALL SEQUENCES IN SCHEMA storage TO postgres;
        GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO postgres;
        
        -- Set default privileges for future objects
        ALTER DEFAULT PRIVILEGES IN SCHEMA auth GRANT ALL ON TABLES TO postgres;
        ALTER DEFAULT PRIVILEGES IN SCHEMA storage GRANT ALL ON TABLES TO postgres;
        ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO postgres;
        ALTER DEFAULT PRIVILEGES IN SCHEMA auth GRANT ALL ON SEQUENCES TO postgres;
        ALTER DEFAULT PRIVILEGES IN SCHEMA storage GRANT ALL ON SEQUENCES TO postgres;
        ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO postgres;
    " >/dev/null 2>&1

  # If a specific service was passed, restart it completely
  if [[ -n "$service_name" ]]; then
    local clean_service="${service_name#${project_name}_}"

    # Stop and remove the container completely
    docker compose stop "$clean_service" >/dev/null 2>&1
    docker compose rm -f "$clean_service" >/dev/null 2>&1

    # Wait a moment for cleanup
    sleep 2

    # Start fresh
    docker compose up -d "$clean_service" >/dev/null 2>&1

    LAST_FIX_DESCRIPTION="Created schemas and restarted $clean_service with fresh state"
  else
    LAST_FIX_DESCRIPTION="Created database schemas (auth, storage, public)"
  fi

  return 0
}

# Fix for missing node modules in bullmq workers
fix_missing_node_modules() {
  local service_name="$1"
  local project_name="${PROJECT_NAME:-nself}"

  # Check if it's a bullmq worker
  if [[ "$service_name" == *"bull"* ]] || [[ "$service_name" == *"worker"* ]]; then
    # Stop the service
    docker compose stop "$service_name" >/dev/null 2>&1
    docker compose rm -f "$service_name" >/dev/null 2>&1

    # Check if we have a package.json for this service
    local service_path=""
    if [[ -d "services/node/$service_name" ]]; then
      service_path="services/node/$service_name"
    elif [[ -d "services/bullmq/$service_name" ]]; then
      service_path="services/bullmq/$service_name"
    elif [[ -d "workers/$service_name" ]]; then
      service_path="workers/$service_name"
    fi

    if [[ -n "$service_path" ]] && [[ -f "$service_path/package.json" ]]; then
      # Ensure package.json has bullmq dependency
      if ! grep -q '"bullmq"' "$service_path/package.json"; then
        # Add bullmq to dependencies
        local temp_file=$(mktemp)
        jq '.dependencies.bullmq = "^5.0.0" | .dependencies.ioredis = "^5.3.2"' "$service_path/package.json" >"$temp_file"
        mv "$temp_file" "$service_path/package.json"
      fi

      # Rebuild the service
      docker compose build "$service_name" >/dev/null 2>&1
    else
      # Create a basic package.json if missing
      mkdir -p "services/bullmq/$service_name"
      cat >"services/bullmq/$service_name/package.json" <<'EOF'
{
  "name": "bullmq-worker",
  "version": "1.0.0",
  "main": "src/index.js",
  "scripts": {
    "start": "node src/index.js"
  },
  "dependencies": {
    "bullmq": "^5.0.0",
    "ioredis": "^5.3.2"
  }
}
EOF

      # Create a basic worker if missing
      mkdir -p "services/bullmq/$service_name/src"
      if [[ ! -f "services/bullmq/$service_name/src/index.js" ]]; then
        cat >"services/bullmq/$service_name/src/index.js" <<'EOF'
const { Worker } = require('bullmq');
const Redis = require('ioredis');

const connection = new Redis(process.env.REDIS_URL || 'redis://redis:6379');

const worker = new Worker('default', async job => {
    console.log(`Processing job ${job.id}`);
    // Add your job processing logic here
    return { success: true };
}, { connection });

worker.on('completed', job => {
    console.log(`Job ${job.id} completed`);
});

worker.on('failed', (job, err) => {
    console.error(`Job ${job.id} failed:`, err);
});

console.log('Worker started');
EOF
      fi

      # Rebuild
      nself build --force >/dev/null 2>&1
    fi

    LAST_FIX_DESCRIPTION="Installed missing node modules for $service_name"
  else
    # For other node services
    docker compose exec "$service_name" npm install >/dev/null 2>&1 || {
      # If exec fails, rebuild
      docker compose build "$service_name" >/dev/null 2>&1
    }
    LAST_FIX_DESCRIPTION="Installed dependencies for $service_name"
  fi

  return 0
}

# Fix nginx upstream issues
fix_nginx_upstream() {
  local project_name="${PROJECT_NAME:-nself}"

  # Nginx depends on other services being up
  # Start dependent services first
  local auth_running=$(docker ps -q -f name="${project_name}_auth" | wc -l)

  if [[ $auth_running -eq 0 ]]; then
    # Start auth service first
    docker compose up -d auth >/dev/null 2>&1
    sleep 5
  fi

  # Restart nginx with proper network resolution
  docker compose restart nginx >/dev/null 2>&1

  LAST_FIX_DESCRIPTION="Restarted nginx with dependencies"
  return 0
}

# Fix missing healthcheck tools
fix_missing_healthcheck_tools() {
  local service_name="$1"
  local project_name="${PROJECT_NAME:-nself}"

  # For services missing curl/wget, we need to rebuild with healthcheck tools
  # or change the healthcheck method

  # Check if it's a Node.js service
  if docker exec "${project_name}_${service_name}" which node >/dev/null 2>&1; then
    # Update the healthcheck to use node instead of curl
    # This requires updating the docker-compose.yml

    # For now, just rebuild with proper base image
    docker compose stop "$service_name" >/dev/null 2>&1
    docker compose rm -f "$service_name" >/dev/null 2>&1

    # Rebuild the service (this should use a Dockerfile with curl installed)
    docker compose build --no-cache "$service_name" >/dev/null 2>&1

    LAST_FIX_DESCRIPTION="Rebuilt $service_name with healthcheck tools"
  else
    # For other services, just restart
    docker compose restart "$service_name" >/dev/null 2>&1
    LAST_FIX_DESCRIPTION="Restarted $service_name"
  fi

  return 0
}
