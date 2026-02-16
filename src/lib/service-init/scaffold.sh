#!/usr/bin/env bash

# scaffold.sh - Service scaffolding engine

# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$SCRIPT_DIR/../utils/platform-compat.sh" 2>/dev/null || true
source "$SCRIPT_DIR/templates-metadata.sh" 2>/dev/null || true

# Scaffold a service from template
scaffold_service() {
  local service_name="$1"
  local template="$2"
  local port="${3:-3000}"
  local output_dir="${4:-services}"

  # Validate inputs
  if [[ -z "$service_name" ]]; then
    printf "Error: Service name required\n" >&2
    return 1
  fi

  if [[ -z "$template" ]]; then
    printf "Error: Template name required\n" >&2
    return 1
  fi

  # Get template path
  local template_path
  template_path=$(get_template_path "$template")

  if [[ ! -d "$template_path" ]]; then
    printf "Error: Template not found: %s\n" "$template" >&2
    printf "Run 'nself service list-templates' to see available templates\n" >&2
    return 1
  fi

  # Create output directory
  local service_dir="${output_dir}/${service_name}"

  if [[ -d "$service_dir" ]]; then
    printf "Error: Service directory already exists: %s\n" "$service_dir" >&2
    printf "Use a different name or remove the existing directory\n" >&2
    return 1
  fi

  mkdir -p "$service_dir" 2>/dev/null || {
    printf "Error: Failed to create directory: %s\n" "$service_dir" >&2
    return 1
  }

  # Load environment variables for substitution
  load_env_vars

  # Copy and process template files
  copy_template_files "$template_path" "$service_dir" "$service_name" "$port"

  # Post-processing based on template type
  post_process_template "$service_dir" "$template"

  printf "✓ Service scaffolded: %s\n" "$service_dir"
  printf "  Template: %s\n" "$template"
  printf "  Port: %s\n" "$port"
  printf "\nNext steps:\n"
  printf "  1. Review generated code in %s/\n" "$service_dir"
  printf "  2. Customize the service for your needs\n"
  printf "  3. Add to .env: CS_N=%s:%s:%s\n" "$service_name" "$template" "$port"
  printf "  4. Build and start: nself build && nself start\n"

  return 0
}

# Get template path
get_template_path() {
  local template="$1"

  # Try to find NSELF_ROOT
  local nself_root="${NSELF_ROOT:-}"

  if [[ -z "$nself_root" ]]; then
    # Try to detect from script location
    nself_root="$(cd "$SCRIPT_DIR/../../.." && pwd)"
  fi

  local base_path="$nself_root/src/templates/services"

  # Map template name to path
  case "$template" in
    socketio-js | socketio-ts | express-js | express-ts | fastify-js | fastify-ts | hono-js | hono-ts | nest-js | nest-ts | bullmq-js | bullmq-ts | temporal-js | temporal-ts | trpc | bun | deno | node-js | node-ts)
      printf "%s/js/%s" "$base_path" "$template"
      ;;
    fastapi | flask | django-rest | celery | ray | agent-*)
      printf "%s/py/%s" "$base_path" "$template"
      ;;
    gin | fiber | echo | grpc)
      printf "%s/go/%s" "$base_path" "$template"
      ;;
    rails | sinatra)
      printf "%s/ruby/%s" "$base_path" "$template"
      ;;
    actix-web)
      printf "%s/rust/%s" "$base_path" "$template"
      ;;
    spring-boot)
      printf "%s/java/%s" "$base_path" "$template"
      ;;
    aspnet)
      printf "%s/csharp/%s" "$base_path" "$template"
      ;;
    laravel)
      printf "%s/php/%s" "$base_path" "$template"
      ;;
    phoenix)
      printf "%s/elixir/%s" "$base_path" "$template"
      ;;
    ktor)
      printf "%s/kotlin/%s" "$base_path" "$template"
      ;;
    vapor)
      printf "%s/swift/%s" "$base_path" "$template"
      ;;
    oatpp)
      printf "%s/cpp/%s" "$base_path" "$template"
      ;;
    lapis)
      printf "%s/lua/%s" "$base_path" "$template"
      ;;
    zap)
      printf "%s/zig/%s" "$base_path" "$template"
      ;;
    *)
      printf ""
      ;;
  esac
}

# Load environment variables for substitution
load_env_vars() {
  # Load from .env if exists
  if [[ -f ".env" ]]; then
    # Source .env (compatible with Bash 3.2)
    while IFS='=' read -r key value; do
      # Skip comments and empty lines
      [[ "$key" =~ ^#.*$ ]] && continue
      [[ -z "$key" ]] && continue

      # Remove quotes from value
      value=$(echo "$value" | sed 's/^["'\'']//' | sed 's/["'\'']$//')

      # Export variable
      export "$key=$value"
    done <.env
  fi

  # Set defaults if not set
  export PROJECT_NAME="${PROJECT_NAME:-nself}"
  export BASE_DOMAIN="${BASE_DOMAIN:-localhost}"
  export ENV="${ENV:-development}"
  export POSTGRES_USER="${POSTGRES_USER:-postgres}"
  export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-}"
  export POSTGRES_DB="${POSTGRES_DB:-nself}"
  export HASURA_GRAPHQL_ENDPOINT="${HASURA_GRAPHQL_ENDPOINT:-http://localhost:8080/v1/graphql}"
  export HASURA_GRAPHQL_ADMIN_SECRET="${HASURA_GRAPHQL_ADMIN_SECRET:-}"
  export REDIS_URL="${REDIS_URL:-redis://localhost:6379}"
  export MINIO_ENDPOINT="${MINIO_ENDPOINT:-localhost:9000}"
  export MEILISEARCH_URL="${MEILISEARCH_URL:-http://localhost:7700}"
}

# Copy and process template files
copy_template_files() {
  local template_path="$1"
  local service_dir="$2"
  local service_name="$3"
  local port="$4"

  # Export variables for substitution
  export SERVICE_NAME="$service_name"
  export PORT="$port"
  export SERVICE_NAME_UPPER=$(echo "$service_name" | tr '[:lower:]' '[:upper:]')
  export SERVICE_NAME_LOWER=$(echo "$service_name" | tr '[:upper:]' '[:lower:]')

  # Process all files recursively
  process_directory "$template_path" "$service_dir" "$service_name"
}

# Process directory recursively
process_directory() {
  local src_dir="$1"
  local dest_dir="$2"
  local service_name="$3"

  # Create destination directory
  mkdir -p "$dest_dir" 2>/dev/null || true

  # Process each item in source directory
  for item in "$src_dir"/*; do
    [[ ! -e "$item" ]] && continue

    local basename
    basename=$(basename "$item")

    # Skip hidden files
    [[ "$basename" =~ ^\. ]] && continue

    # Process filename (replace {{SERVICE_NAME}} placeholders)
    local dest_name
    dest_name=$(echo "$basename" | sed "s/{{SERVICE_NAME}}/${service_name}/g")

    # Remove .template extension
    dest_name="${dest_name%.template}"

    local dest_path="$dest_dir/$dest_name"

    if [[ -d "$item" ]]; then
      # Recursively process subdirectories
      process_directory "$item" "$dest_path" "$service_name"
    elif [[ -f "$item" ]]; then
      # Process file
      process_file "$item" "$dest_path"
    fi
  done
}

# Process a single file
process_file() {
  local src_file="$1"
  local dest_file="$2"

  # Check if file is binary
  if file "$src_file" | grep -q "text"; then
    # Text file - process placeholders
    substitute_placeholders "$src_file" "$dest_file"
  else
    # Binary file - copy as-is
    cp "$src_file" "$dest_file"
  fi

  # Preserve executable permissions
  if [[ -x "$src_file" ]]; then
    chmod +x "$dest_file"
  fi
}

# Substitute placeholders in file
substitute_placeholders() {
  local src_file="$1"
  local dest_file="$2"

  # Read file and substitute variables
  local content
  content=$(cat "$src_file")

  # Replace common placeholders
  content=$(echo "$content" | sed "s/{{SERVICE_NAME}}/${SERVICE_NAME}/g")
  content=$(echo "$content" | sed "s/{{SERVICE_NAME_UPPER}}/${SERVICE_NAME_UPPER}/g")
  content=$(echo "$content" | sed "s/{{SERVICE_NAME_LOWER}}/${SERVICE_NAME_LOWER}/g")
  content=$(echo "$content" | sed "s/{{PROJECT_NAME}}/${PROJECT_NAME}/g")
  content=$(echo "$content" | sed "s/{{BASE_DOMAIN}}/${BASE_DOMAIN}/g")
  content=$(echo "$content" | sed "s/{{PORT}}/${PORT}/g")
  content=$(echo "$content" | sed "s/{{ENV}}/${ENV}/g")
  content=$(echo "$content" | sed "s|{{POSTGRES_USER}}|${POSTGRES_USER}|g")
  content=$(echo "$content" | sed "s|{{POSTGRES_PASSWORD}}|${POSTGRES_PASSWORD}|g")
  content=$(echo "$content" | sed "s|{{POSTGRES_DB}}|${POSTGRES_DB}|g")
  content=$(echo "$content" | sed "s|{{HASURA_GRAPHQL_ENDPOINT}}|${HASURA_GRAPHQL_ENDPOINT}|g")
  content=$(echo "$content" | sed "s|{{HASURA_GRAPHQL_ADMIN_SECRET}}|${HASURA_GRAPHQL_ADMIN_SECRET}|g")
  content=$(echo "$content" | sed "s|{{REDIS_URL}}|${REDIS_URL}|g")
  content=$(echo "$content" | sed "s|{{MINIO_ENDPOINT}}|${MINIO_ENDPOINT}|g")
  content=$(echo "$content" | sed "s|{{MEILISEARCH_URL}}|${MEILISEARCH_URL}|g")

  # Write to destination
  printf "%s\n" "$content" >"$dest_file"
}

# Post-process template based on type
post_process_template() {
  local service_dir="$1"
  local template="$2"

  case "$template" in
    socketio-ts | socketio-js)
      # Add Redis adapter configuration hint
      if [[ -f "$service_dir/package.json" ]]; then
        printf "\nℹ️  Socket.IO Redis adapter:\n"
        printf "   To enable multi-instance support, install:\n"
        printf "   npm install @socket.io/redis-adapter ioredis\n"
      fi
      ;;
    bullmq-ts | bullmq-js | celery)
      printf "\nℹ️  Worker service created.\n"
      printf "   Make sure Redis is enabled in .env: REDIS_ENABLED=true\n"
      ;;
    *-ts)
      # TypeScript services - check for tsconfig
      if [[ -f "$service_dir/tsconfig.json" ]]; then
        printf "\nℹ️  TypeScript configured. Install dependencies:\n"
        printf "   cd %s && npm install\n" "$service_dir"
      fi
      ;;
  esac
}

# Export functions
export -f scaffold_service
export -f get_template_path
export -f load_env_vars
export -f copy_template_files
export -f process_directory
export -f process_file
export -f substitute_placeholders
export -f post_process_template
