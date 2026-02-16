#!/usr/bin/env bash

# auto-fix.sh - Automatic fixes applied during start
# Bash 3.2 compatible, cross-platform

# Source platform compatibility utilities (namespaced to avoid clobbering caller's SCRIPT_DIR)
_START_AUTOFIX_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$_START_AUTOFIX_DIR/../utils/platform-compat.sh" 2>/dev/null || {
  # Fallback definition if not found
  safe_sed_inline() {
    local file="$1"
    shift
    if [[ "$OSTYPE" == "darwin"* ]]; then
      sed -i '' "$@" "$file"
    else
      sed -i "$@" "$file"
    fi
  }
}

# Fix nginx config environment variable syntax
fix_nginx_configs() {
  local project_dir="${1:-.}"
  local verbose="${2:-false}"

  # Add health check location to default.conf if missing
  local default_conf="$project_dir/nginx/conf.d/default.conf"
  if [[ -f "$default_conf" ]]; then
    # Check if health location exists in HTTP block
    if ! grep -q "location /health" "$default_conf"; then
      [ "$verbose" = "true" ] && echo "  Adding health check endpoint to nginx"
      # Insert health check before the catch-all redirect
      safe_sed_inline "$default_conf" '/location \/ {/i\
\    # Health check endpoint (do not redirect)\
\    location /health {\
\        access_log off;\
\        return 200 "healthy\\n";\
\        add_header Content-Type text/plain;\
\    }\
\'
    fi
  fi

  # Fix nginx.conf and all site configs - include conf.d directory
  for config_file in "$project_dir"/nginx/nginx.conf "$project_dir"/nginx/sites/*.conf "$project_dir"/nginx/conf.d/*.conf; do
    if [[ -f "$config_file" ]]; then
      [ "$verbose" = "true" ] && echo "  Fixing nginx config: $(basename "$config_file")"

      # Get values from environment or use defaults
      local client_max_body_size="${NGINX_MAX_BODY_SIZE:-100M}"
      local proxy_read_timeout="${NGINX_PROXY_READ_TIMEOUT:-60}"
      local proxy_connect_timeout="${NGINX_PROXY_CONNECT_TIMEOUT:-60}"
      local proxy_send_timeout="${NGINX_PROXY_SEND_TIMEOUT:-60}"
      local base_domain="${BASE_DOMAIN:-localhost}"
      local ssl_cert_name="${SSL_CERT_NAME:-localhost}"
      local project_name="${PROJECT_NAME:-nself}"

      # Fix ALL bash-style variables with comprehensive replacement
      safe_sed_inline "$config_file" \
        -e "s/\${NGINX_MAX_BODY_SIZE:-[^}]*}/${client_max_body_size}/g" \
        -e "s/\${CLIENT_MAX_BODY_SIZE:-[^}]*}/${client_max_body_size}/g" \
        -e "s/\${PROXY_READ_TIMEOUT:-[^}]*}/${proxy_read_timeout}/g" \
        -e "s/\${NGINX_PROXY_READ_TIMEOUT:-[^}]*}/${proxy_read_timeout}/g" \
        -e "s/\${PROXY_CONNECT_TIMEOUT:-[^}]*}/${proxy_connect_timeout}/g" \
        -e "s/\${NGINX_PROXY_CONNECT_TIMEOUT:-[^}]*}/${proxy_connect_timeout}/g" \
        -e "s/\${PROXY_SEND_TIMEOUT:-[^}]*}/${proxy_send_timeout}/g" \
        -e "s/\${NGINX_PROXY_SEND_TIMEOUT:-[^}]*}/${proxy_send_timeout}/g" \
        -e "s/\${BASE_DOMAIN:-[^}]*}/${base_domain}/g" \
        -e "s/\${SSL_CERT_NAME:-[^}]*}/${ssl_cert_name}/g" \
        -e "s/\${PROJECT_NAME:-[^}]*}/${project_name}/g"
    fi
  done
}

# Generate missing lock files for services
generate_service_lock_files() {
  local project_dir="${1:-.}"
  local verbose="${2:-false}"

  # Check functions service - create minimal structure if needed
  if [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]]; then
    local functions_dir="$project_dir/functions"

    # Create functions directory if it doesn't exist
    if [[ ! -d "$functions_dir" ]]; then
      [ "$verbose" = "true" ] && echo "  Creating functions directory structure"
      mkdir -p "$functions_dir"

      # Create minimal package.json if missing
      if [[ ! -f "$functions_dir/package.json" ]]; then
        echo '{"name":"functions","version":"1.0.0","dependencies":{}}' >"$functions_dir/package.json"
      fi

      # Create lock file
      if [[ ! -f "$functions_dir/package-lock.json" ]]; then
        echo '{"lockfileVersion": 2, "requires": true, "packages": {}}' >"$functions_dir/package-lock.json"
      fi
    elif [[ -f "$functions_dir/package.json" ]] && [[ ! -f "$functions_dir/package-lock.json" ]]; then
      [ "$verbose" = "true" ] && echo "  Generating package-lock.json for functions service"
      echo '{"lockfileVersion": 2, "requires": true, "packages": {}}' >"$functions_dir/package-lock.json"
    fi
  fi

  # Fix Python FastAPI exception handlers if needed
  for i in {1..10}; do
    local cs_var="CS_$i"
    local cs_value=$(eval "echo \${$cs_var:-}")
    if [[ -n "$cs_value" ]]; then
      local service_name=$(echo "$cs_value" | cut -d: -f1)
      local template_type=$(echo "$cs_value" | cut -d: -f2)

      # Check if it's a Python FastAPI service
      if [[ "$template_type" == *"fastapi"* ]] || [[ "$template_type" == *"py"* ]]; then
        local main_file="$project_dir/services/$service_name/main.py"
        if [[ -f "$main_file" ]]; then
          # Check for incorrect HTTPException return in exception handlers
          if grep -q "return HTTPException" "$main_file"; then
            [ "$verbose" = "true" ] && echo "  Fixing FastAPI exception handlers in $service_name"

            # Add import if missing (ensure it goes on a new line)
            if ! grep -q "from fastapi.responses import JSONResponse" "$main_file"; then
              # Use a more robust approach to add the import
              safe_sed_inline "$main_file" 's/^from fastapi import.*$/&\
from fastapi.responses import JSONResponse/'
            fi

            # Fix exception handlers
            safe_sed_inline "$main_file" \
              -e 's/return HTTPException(/return JSONResponse(/g' \
              -e 's/detail=/content=/g'
          fi
        fi
      fi
    fi
  done

  # Check all custom services
  for i in {1..10}; do
    local cs_var="CS_$i"
    local cs_value=$(eval "echo \${$cs_var:-}")
    if [[ -n "$cs_value" ]]; then
      local service_name=$(echo "$cs_value" | cut -d: -f1)
      local service_dir="$project_dir/services/$service_name"

      if [[ -d "$service_dir" ]]; then
        # Node.js services
        if [[ -f "$service_dir/package.json" ]] && [[ ! -f "$service_dir/package-lock.json" ]]; then
          [ "$verbose" = "true" ] && echo "  Generating package-lock.json for $service_name"
          echo '{"lockfileVersion": 2, "requires": true, "packages": {}}' >"$service_dir/package-lock.json"
        fi

        # Go services
        if [[ -f "$service_dir/go.mod" ]] && [[ ! -f "$service_dir/go.sum" ]]; then
          [ "$verbose" = "true" ] && echo "  Creating go.sum for $service_name"
          touch "$service_dir/go.sum"
        fi

        # Python services
        if [[ -f "$service_dir/requirements.txt" ]] && [[ ! -f "$service_dir/requirements.lock" ]]; then
          [ "$verbose" = "true" ] && echo "  Creating requirements.lock for $service_name"
          touch "$service_dir/requirements.lock"
        fi
      fi
    fi
  done
}

# Fix JWT configuration comprehensively
fix_jwt_configuration() {
  local env_file="${1:-.env.runtime}"
  local verbose="${2:-false}"

  # Ensure JWT type is set
  if ! grep -q "^AUTH_JWT_TYPE=" "$env_file" 2>/dev/null; then
    [ "$verbose" = "true" ] && echo "  Setting AUTH_JWT_TYPE=HS256"
    echo "AUTH_JWT_TYPE=HS256" >>"$env_file"
  fi

  # Get or generate JWT secret
  local jwt_secret=""
  if grep -q "^AUTH_JWT_SECRET=" "$env_file" 2>/dev/null; then
    jwt_secret=$(grep "^AUTH_JWT_SECRET=" "$env_file" | cut -d= -f2-)
  elif grep -q "^AUTH_JWT_KEY=" "$env_file" 2>/dev/null; then
    jwt_secret=$(grep "^AUTH_JWT_KEY=" "$env_file" | cut -d= -f2-)
  elif grep -q "^HASURA_JWT_KEY=" "$env_file" 2>/dev/null; then
    jwt_secret=$(grep "^HASURA_JWT_KEY=" "$env_file" | cut -d= -f2-)
  fi

  # Generate secure JWT secret if none exists or too short
  if [[ -z "$jwt_secret" ]] || [[ ${#jwt_secret} -lt 32 ]]; then
    # Try openssl first (most portable)
    jwt_secret=$(openssl rand -base64 32 2>/dev/null | tr -d '\n' | head -c 32)

    # Fallback to /dev/urandom (Linux, macOS, WSL)
    if [[ -z "$jwt_secret" ]] && [[ -r /dev/urandom ]]; then
      jwt_secret=$(tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 32)
    fi

    # Last resort: use $RANDOM (weak but better than hardcoded)
    if [[ -z "$jwt_secret" ]]; then
      jwt_secret=""
      local chars="ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
      for ((i=0; i<32; i++)); do
        jwt_secret="${jwt_secret}${chars:RANDOM%${#chars}:1}"
      done
    fi
    [ "$verbose" = "true" ] && echo "  Generated JWT secret"
  fi

  # Ensure all JWT-related variables are set
  if ! grep -q "^AUTH_JWT_SECRET=" "$env_file" 2>/dev/null; then
    echo "AUTH_JWT_SECRET=$jwt_secret" >>"$env_file"
  fi
  if ! grep -q "^AUTH_JWT_KEY=" "$env_file" 2>/dev/null; then
    echo "AUTH_JWT_KEY=$jwt_secret" >>"$env_file"
  fi
  if ! grep -q "^HASURA_JWT_KEY=" "$env_file" 2>/dev/null; then
    echo "HASURA_JWT_KEY=$jwt_secret" >>"$env_file"
  fi

  # Fix Hasura JWT secret format - MUST be single-quoted to preserve JSON
  local jwt_type=$(grep "^AUTH_JWT_TYPE=" "$env_file" 2>/dev/null | cut -d= -f2- || echo "HS256")
  local hasura_jwt_secret="'{\"type\":\"${jwt_type}\",\"key\":\"${jwt_secret}\"}'"

  # Update or add HASURA_GRAPHQL_JWT_SECRET with proper quoting
  if grep -q "^HASURA_GRAPHQL_JWT_SECRET=" "$env_file" 2>/dev/null; then
    safe_sed_inline "$env_file" "/^HASURA_GRAPHQL_JWT_SECRET=/d"
  fi
  echo "HASURA_GRAPHQL_JWT_SECRET=${hasura_jwt_secret}" >>"$env_file"
}

# Fix MeiliSearch environment mapping
fix_meilisearch_env() {
  local env_file="${1:-.env.runtime}"
  local verbose="${2:-false}"

  local env_value=$(grep "^ENV=" "$env_file" 2>/dev/null | cut -d= -f2-)
  local meili_env="development"

  case "$env_value" in
    dev | development)
      meili_env="development"
      ;;
    prod | production)
      meili_env="production"
      ;;
    staging | stage | test)
      meili_env="development"
      ;;
    *)
      meili_env="development"
      ;;
  esac

  # Update or add MEILI_ENV
  if grep -q "^MEILI_ENV=" "$env_file" 2>/dev/null; then
    if [[ "$(grep "^MEILI_ENV=" "$env_file" | cut -d= -f2-)" != "$meili_env" ]]; then
      [ "$verbose" = "true" ] && echo "  Fixing MEILI_ENV: $meili_env"
      safe_sed_inline "$env_file" "s/^MEILI_ENV=.*/MEILI_ENV=$meili_env/"
    fi
  else
    [ "$verbose" = "true" ] && echo "  Setting MEILI_ENV=$meili_env"
    echo "MEILI_ENV=$meili_env" >>"$env_file"
  fi
}

# Ensure custom service ports are set as environment variables
fix_custom_service_ports() {
  local env_file="${1:-.env.runtime}"
  local verbose="${2:-false}"

  for i in {1..10}; do
    local cs_var="CS_$i"
    if grep -q "^$cs_var=" "$env_file" 2>/dev/null; then
      local cs_value=$(grep "^$cs_var=" "$env_file" | cut -d= -f2-)
      if [[ -n "$cs_value" ]]; then
        local service_name=$(echo "$cs_value" | cut -d: -f1)
        local port=$(echo "$cs_value" | cut -d: -f3)

        # Ensure PORT environment variable is set for the service
        local port_var=$(echo "$service_name" | tr '[:lower:]' '[:upper:]' | tr '-' '_')_PORT

        if ! grep -q "^$port_var=" "$env_file" 2>/dev/null; then
          [ "$verbose" = "true" ] && echo "  Setting $port_var=$port"
          echo "$port_var=$port" >>"$env_file"
        fi
      fi
    fi
  done
}

# Auto-fix common issues before starting services
apply_start_auto_fixes() {
  local project_name="${1:-nself}"
  local env_file="${2:-.env}"
  local verbose="${3:-false}"

  # Fix 1: MLFlow port 5000 conflict on macOS
  if [[ "$(uname)" == "Darwin" ]]; then
    # Check if port 5000 is in use (likely Control Center)
    if lsof -i :5000 >/dev/null 2>&1; then
      # Check if MLFLOW_PORT is still set to 5000
      if grep -q "MLFLOW_PORT=5000" "$env_file" 2>/dev/null; then
        [ "$verbose" = "true" ] && echo "Auto-fixing: MLFlow port conflict (5000 -> 5005)"

        # Update env file
        safe_sed_inline "$env_file" 's/MLFLOW_PORT=5000/MLFLOW_PORT=5005/g'

        # Also update docker-compose if it exists
        if [[ -f "docker-compose.yml" ]]; then
          safe_sed_inline "docker-compose.yml" 's/\${MLFLOW_PORT:-5000}/\${MLFLOW_PORT:-5005}/g'
          safe_sed_inline "docker-compose.yml" 's/localhost:5000/localhost:5005/g'
        fi
      fi
    fi
  fi

  # Fix 2: Create monitoring config files if missing
  if [[ "${GRAFANA_ENABLED:-false}" == "true" ]] || [[ "${LOKI_ENABLED:-false}" == "true" ]]; then
    if [[ ! -f "monitoring/loki/local-config.yaml" ]]; then
      [ "$verbose" = "true" ] && echo "Auto-fixing: Creating missing monitoring configs"

      # Source monitoring setup if available
      if command -v setup_monitoring_configs >/dev/null 2>&1; then
        setup_monitoring_configs >/dev/null 2>&1
      fi
    fi
  fi

  # Fix 3: Ensure DATABASE_URL is set if missing
  if ! grep -q "^DATABASE_URL=" "$env_file" 2>/dev/null; then
    local db_name="${POSTGRES_DB:-${PROJECT_NAME}}"
    # Sanitize database name (replace hyphens with underscores)
    local safe_db_name=$(echo "$db_name" | tr '-' '_')
    local db_user="${POSTGRES_USER:-postgres}"
    local db_pass="${POSTGRES_PASSWORD:-postgres}"
    local db_host="${POSTGRES_HOST:-postgres}"
    local db_port="${POSTGRES_PORT:-5432}"

    [ "$verbose" = "true" ] && echo "Auto-fixing: Adding DATABASE_URL to env file"
    echo "DATABASE_URL=postgres://${db_user}:${db_pass}@${db_host}:${db_port}/${safe_db_name}" >>"$env_file"
  fi

  # Fix 4: Comprehensive JWT configuration fix
  fix_jwt_configuration "$env_file" "$verbose"

  # Fix 4.5: Add Hasura endpoint for Auth service
  if ! grep -q "^HASURA_GRAPHQL_GRAPHQL_URL=" "$env_file" 2>/dev/null; then
    [ "$verbose" = "true" ] && echo "  Setting HASURA_GRAPHQL_GRAPHQL_URL for auth service"
    echo "HASURA_GRAPHQL_GRAPHQL_URL=http://hasura:8080/v1/graphql" >>"$env_file"
  fi

  # Fix 5: MeiliSearch environment mapping
  if grep -q "MEILISEARCH_ENABLED=true" "$env_file" 2>/dev/null; then
    fix_meilisearch_env "$env_file" "$verbose"
  fi

  # Fix 6: Fix nginx configs
  fix_nginx_configs "." "$verbose"

  # Fix 7: Generate missing lock files
  generate_service_lock_files "." "$verbose"

  # Fix 8: Ensure custom service ports are set
  fix_custom_service_ports "$env_file" "$verbose"

  # Fix 9: Resolve known port conflicts
  fix_port_conflicts "$env_file" "$verbose"

  # Fix 10: Ensure auth service can connect to postgres
  fix_auth_database_connection "$env_file" "$verbose"

  # Fix 11: Fix functions service requirements
  if [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]]; then
    fix_functions_requirements "${PROJECT_NAME:-nself}" "$verbose"
  fi

  # Fix 12: Comprehensive auth database URL fixes
  fix_auth_database_urls "$env_file" "$verbose"

  return 0
}

# Fix auth service database connection issues
fix_auth_database_connection() {
  local env_file="${1:-.env.runtime}"
  local verbose="${2:-false}"

  # Auth service sometimes needs explicit postgres connection vars
  if [[ "${AUTH_ENABLED:-true}" == "true" ]]; then
    local db_name="${POSTGRES_DB:-demo_db}"
    local db_user="${POSTGRES_USER:-postgres}"
    local db_pass="${POSTGRES_PASSWORD:-postgres}"

    # Ensure AUTH_DATABASE_URL is set
    if ! grep -q "^AUTH_DATABASE_URL=" "$env_file" 2>/dev/null; then
      [ "$verbose" = "true" ] && echo "  Setting AUTH_DATABASE_URL for auth service"
      echo "AUTH_DATABASE_URL=postgresql://${db_user}:${db_pass}@postgres:5432/${db_name}" >>"$env_file"
    fi

    # Also set individual AUTH_POSTGRES variables as fallback
    if ! grep -q "^AUTH_POSTGRES_HOST=" "$env_file" 2>/dev/null; then
      [ "$verbose" = "true" ] && echo "  Setting AUTH_POSTGRES connection variables"
      echo "AUTH_POSTGRES_HOST=postgres" >>"$env_file"
      echo "AUTH_POSTGRES_PORT=5432" >>"$env_file"
      echo "AUTH_POSTGRES_USER=${db_user}" >>"$env_file"
      echo "AUTH_POSTGRES_PASSWORD=${db_pass}" >>"$env_file"
      echo "AUTH_POSTGRES_DATABASE=${db_name}" >>"$env_file"
    fi
  fi
}

# Fix known port conflicts between services
fix_port_conflicts() {
  local env_file="${1:-.env.runtime}"
  local verbose="${2:-false}"

  # Check for Loki/nself-admin port conflict (Loki defaults to 3100, admin to 3021)
  local loki_port="${LOKI_PORT:-3100}"
  local nself_admin_port="${NSELF_ADMIN_PORT:-3021}"

  if [[ "$loki_port" == "$nself_admin_port" ]] && [[ "${LOKI_ENABLED:-false}" == "true" ]] && [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]]; then
    [ "$verbose" = "true" ] && echo "  Resolving port conflict between Loki and nself-admin (both on $loki_port)"

    # Move nself-admin to alternate port
    if ! grep -q "^NSELF_ADMIN_PORT=" "$env_file" 2>/dev/null; then
      echo "NSELF_ADMIN_PORT=3022" >>"$env_file"
      [ "$verbose" = "true" ] && echo "    Moving nself-admin to port 3022"
    else
      safe_sed_inline "$env_file" "s/^NSELF_ADMIN_PORT=.*/NSELF_ADMIN_PORT=3022/"
    fi
  fi

  # Check for other common conflicts
  # Tempo/Grafana conflict (both can default to 3000)
  local grafana_port="${GRAFANA_PORT:-3000}"
  local tempo_port="${TEMPO_PORT:-3200}" # Tempo actually defaults to 3200, but check anyway

  # Add more conflict resolutions as needed
}

# Fix functions service requirements
fix_functions_requirements() {
  local project_name="${1:-nself}"
  local verbose="${2:-false}"

  # Create functions directory if missing
  if [[ ! -d "functions" ]]; then
    [ "$verbose" = "true" ] && echo "  Creating functions directory"
    mkdir -p functions
  fi

  # Create package.json if missing
  if [[ ! -f "functions/package.json" ]]; then
    [ "$verbose" = "true" ] && echo "  Creating functions/package.json"
    cat >functions/package.json <<'EOF'
{
  "name": "functions",
  "version": "1.0.0",
  "description": "Serverless functions",
  "main": "index.js",
  "scripts": {
    "start": "node index.js"
  },
  "dependencies": {}
}
EOF
  fi

  # Create package-lock.json if missing
  if [[ ! -f "functions/package-lock.json" ]]; then
    [ "$verbose" = "true" ] && echo "  Creating functions/package-lock.json"
    cat >functions/package-lock.json <<'EOF'
{
  "name": "functions",
  "version": "1.0.0",
  "lockfileVersion": 2,
  "requires": true,
  "packages": {
    "": {
      "name": "functions",
      "version": "1.0.0",
      "dependencies": {}
    }
  }
}
EOF
  fi

  # Create yarn.lock as alternative
  if [[ ! -f "functions/yarn.lock" ]]; then
    [ "$verbose" = "true" ] && echo "  Creating functions/yarn.lock"
    echo "# yarn lockfile v1" >functions/yarn.lock
  fi

  return 0
}

# Fix auth service database URLs to ensure they use 'postgres' hostname
fix_auth_database_urls() {
  local env_file="${1:-.env.runtime}"
  local verbose="${2:-false}"

  # Ensure AUTH_DATABASE_URL uses postgres hostname
  local auth_db_url=$(grep "^AUTH_DATABASE_URL=" "$env_file" 2>/dev/null | cut -d= -f2-)
  if [[ -n "$auth_db_url" ]] && [[ "$auth_db_url" == *"localhost"* || "$auth_db_url" == *"127.0.0.1"* || "$auth_db_url" == *"::1"* ]]; then
    [ "$verbose" = "true" ] && echo "  Fixing AUTH_DATABASE_URL to use postgres hostname"
    local fixed_url=$(echo "$auth_db_url" | sed 's/@localhost/@postgres/g' | sed 's/@127\.0\.0\.1/@postgres/g' | sed 's/@\[::1\]/@postgres/g' | sed 's/@::1/@postgres/g')
    safe_sed_inline "$env_file" "s|^AUTH_DATABASE_URL=.*|AUTH_DATABASE_URL=$fixed_url|"
  fi

  # Also ensure POSTGRES_HOST is set correctly
  if ! grep -q "^POSTGRES_HOST=" "$env_file" 2>/dev/null; then
    echo "POSTGRES_HOST=postgres" >>"$env_file"
  elif grep -q "^POSTGRES_HOST=localhost\|^POSTGRES_HOST=127.0.0.1\|^POSTGRES_HOST=::1" "$env_file" 2>/dev/null; then
    [ "$verbose" = "true" ] && echo "  Fixing POSTGRES_HOST to postgres"
    safe_sed_inline "$env_file" "s/^POSTGRES_HOST=.*/POSTGRES_HOST=postgres/"
  fi

  # Ensure AUTH_POSTGRES_* variables are set
  local postgres_user=$(grep "^POSTGRES_USER=" "$env_file" 2>/dev/null | cut -d= -f2- || echo "postgres")
  local postgres_password=$(grep "^POSTGRES_PASSWORD=" "$env_file" 2>/dev/null | cut -d= -f2- || echo "postgres")
  local postgres_db=$(grep "^POSTGRES_DB=" "$env_file" 2>/dev/null | cut -d= -f2- || echo "nself_db")

  if ! grep -q "^AUTH_POSTGRES_HOST=" "$env_file" 2>/dev/null; then
    echo "AUTH_POSTGRES_HOST=postgres" >>"$env_file"
  fi
  if ! grep -q "^AUTH_POSTGRES_PORT=" "$env_file" 2>/dev/null; then
    echo "AUTH_POSTGRES_PORT=5432" >>"$env_file"
  fi
  if ! grep -q "^AUTH_POSTGRES_USER=" "$env_file" 2>/dev/null; then
    echo "AUTH_POSTGRES_USER=$postgres_user" >>"$env_file"
  fi
  if ! grep -q "^AUTH_POSTGRES_PASSWORD=" "$env_file" 2>/dev/null; then
    echo "AUTH_POSTGRES_PASSWORD=$postgres_password" >>"$env_file"
  fi
  if ! grep -q "^AUTH_POSTGRES_DATABASE=" "$env_file" 2>/dev/null; then
    echo "AUTH_POSTGRES_DATABASE=$postgres_db" >>"$env_file"
  fi

  return 0
}

# Monitor and auto-heal unhealthy services
monitor_and_heal_services() {
  local project_name="${1:-nself}"
  local max_attempts="${2:-3}"
  local verbose="${3:-false}"

  local attempt=0
  local all_healthy=false

  while [[ $attempt -lt $max_attempts ]] && [[ "$all_healthy" == "false" ]]; do
    attempt=$((attempt + 1))

    # Wait a bit for services to stabilize
    sleep 5

    # Get unhealthy services
    local unhealthy_services=$(docker compose ps --format json 2>/dev/null |
      jq -r 'select(.Health == "unhealthy" or .State == "restarting") | .Service' 2>/dev/null)

    if [[ -z "$unhealthy_services" ]]; then
      all_healthy=true
      [ "$verbose" = "true" ] && echo "  All services are healthy"
    else
      [ "$verbose" = "true" ] && echo "  Attempting to heal unhealthy services (attempt $attempt/$max_attempts)"

      # Try to restart unhealthy services
      for service in $unhealthy_services; do
        [ "$verbose" = "true" ] && echo "    Restarting $service..."
        docker compose restart "$service" >/dev/null 2>&1
      done
    fi
  done

  return 0
}

# Export functions
export -f apply_start_auto_fixes
export -f fix_nginx_configs
export -f generate_service_lock_files
export -f fix_jwt_configuration
export -f fix_meilisearch_env
export -f fix_custom_service_ports
export -f fix_functions_requirements
export -f fix_auth_database_urls
export -f fix_auth_database_connection
export -f fix_port_conflicts
export -f monitor_and_heal_services
