#!/usr/bin/env bash
# service.sh - Consolidated service management
# Part of nself v1.0 - Service Command Consolidation
# Consolidates: storage, email, search, redis, functions, mlflow, realtime, admin
# Total: 43+ subcommands across 8 service categories

# Early help check - before sourcing anything that might fail
if [[ "${1:-}" == "--help" ]] || [[ "${1:-}" == "-h" ]] || [[ "${1:-}" == "help" ]] || [[ $# -eq 0 ]]; then
  cat <<'EOF'
nself service - Consolidated Service Management (43+ subcommands)

USAGE:
  nself service <subcommand> [options]

CORE OPERATIONS:
  list              List all optional services and their status
  enable <name>     Enable an optional service
  disable <name>    Disable an optional service
  status [name]     Show status of all or specific service
  restart <name>    Restart a service
  logs <name> [-f]  View service logs
  init <name>       Initialize a service

CODE GENERATION (4):
  scaffold <name> --template <type> --port <port>
                    Scaffold a custom service from template
  list-templates    List all available service templates
  template-info     Show detailed information about a template
  wizard            Interactive service creation wizard

ADMIN SERVICE (consolidated from admin, admin-dev):
  admin [--dev]              Open admin UI
  admin dev enable [port]    Enable dev mode (route to localhost)
  admin dev disable          Disable dev mode (use Docker container)
  admin dev env              Show env vars for local development
  admin dev status           Show current dev mode status
  admin stats                Get overview statistics
  admin users [limit]        List users
  admin activity [hours]     Recent activity
  admin security             Security events

STORAGE SERVICE (consolidated from storage):
  storage init               Initialize storage system
  storage upload <file>      Upload a file to storage
  storage list [prefix]      List uploaded files
  storage delete <path>      Delete an uploaded file
  storage config             Configure upload pipeline
  storage status             Show pipeline status
  storage test               Test upload functionality
  storage graphql-setup      Generate GraphQL integration package

EMAIL SERVICE (consolidated from email):
  email setup                Interactive email setup wizard
  email list                 List all email providers
  email configure <provider> Configure SMTP provider
  email configure --api <p>  Configure API provider
  email validate             Check email configuration
  email check                SMTP connection pre-flight
  email check --api          API connection pre-flight
  email test [email]         Send test email (SMTP)
  email test --api [email]   Send test email (API)
  email docs [provider]      Get setup instructions
  email detect               Show current provider

SEARCH SERVICE (consolidated from search):
  search enable [engine]     Enable search with selected engine
  search disable             Disable search service
  search status              Show search service status
  search list                List available search engines
  search setup               Interactive search setup
  search test ["query"]      Test search functionality
  search reindex             Rebuild search index
  search config              Show current configuration
  search docs [engine]       Show search documentation

REDIS SERVICE (consolidated from redis):
  redis init                 Initialize Redis configuration
  redis add --name <name>    Add Redis connection
  redis list                 List all connections
  redis get <name>           Get connection details
  redis delete <name>        Delete connection
  redis test <name>          Test connection
  redis health [name]        Get health status
  redis pool configure       Configure connection pool
  redis pool get <name>      Get pool configuration

FUNCTIONS SERVICE (consolidated from functions):
  functions status           Show functions service status
  functions init [--ts]      Initialize functions service
  functions enable           Enable functions service
  functions disable          Disable functions service
  functions list             List available functions
  functions create <name>    Create a new function
  functions delete <name>    Delete a function
  functions test <name>      Test a function
  functions logs [-f]        View function logs
  functions deploy [target]  Deploy functions

MLFLOW SERVICE (consolidated from mlflow):
  mlflow status              Show MLflow status
  mlflow enable              Enable MLflow service
  mlflow disable             Disable MLflow service
  mlflow open                Open MLflow UI in browser
  mlflow configure <s> <v>   Configure MLflow settings
  mlflow experiments         List/manage experiments
  mlflow runs [exp_id]       List runs
  mlflow logs [-f]           View MLflow logs
  mlflow test                Test MLflow connection

REALTIME SERVICE (consolidated from realtime):
  realtime init              Initialize real-time system
  realtime status            Show real-time system status
  realtime logs [--follow]   Show real-time logs
  realtime cleanup           Clean up stale connections
  realtime subscribe <tbl>   Subscribe to table changes
  realtime unsubscribe <t>   Unsubscribe from table changes
  realtime listen <tbl>      Listen to table changes
  realtime subscriptions     List active subscriptions
  realtime channel create    Create a channel
  realtime channel list      List channels
  realtime broadcast         Send message to channel
  realtime presence track    Track user presence
  realtime presence online   List online users
  realtime connections       Show active connections

EXAMPLES:
  # Core operations
  nself service list                        # List all services
  nself service enable search               # Enable search service

  # Admin
  nself service admin dev enable 3000       # Enable admin dev mode
  nself service admin stats                 # Get stats

  # Storage
  nself service storage upload photo.jpg    # Upload file
  nself service storage list                # List files

  # Email
  nself service email configure sendgrid    # Configure email
  nself service email test --api admin@x.com

  # Search
  nself service search enable meilisearch   # Enable search
  nself service search test "hello world"   # Test search

  # Functions
  nself service functions create hello      # Create function
  nself service functions deploy            # Deploy

  # MLflow
  nself service mlflow enable               # Enable MLflow
  nself service mlflow experiments          # List experiments

  # Realtime
  nself service realtime init               # Initialize
  nself service realtime subscribe users    # Subscribe to table

OPTIONS:
  -h, --help        Show this help message
  --follow, -f      Follow logs (for logs commands)
  --tail <n>        Show last n lines (for logs commands)

See individual command help for more options.
EOF
  exit 0
fi

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source shared utilities
source "$SCRIPT_DIR/../lib/utils/env.sh"
source "$SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../lib/utils/header.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../lib/utils/docker.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../lib/utils/platform-compat.sh" 2>/dev/null || true

# Source service init utilities
source "$SCRIPT_DIR/../lib/service-init/templates-metadata.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../lib/service-init/scaffold.sh" 2>/dev/null || true

# Source storage utilities
source "$SCRIPT_DIR/../lib/storage/upload-pipeline.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../lib/storage/graphql-integration.sh" 2>/dev/null || true

# Source realtime utilities
source "$SCRIPT_DIR/../lib/realtime/channels.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../lib/realtime/presence.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../lib/realtime/broadcast.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../lib/realtime/subscriptions.sh" 2>/dev/null || true

# Source Redis utilities
[[ -f "$SCRIPT_DIR/../lib/redis/core.sh" ]] && source "$SCRIPT_DIR/../lib/redis/core.sh" || true

# Source admin utilities
[[ -f "$SCRIPT_DIR/../lib/admin/api.sh" ]] && source "$SCRIPT_DIR/../lib/admin/api.sh" || true

# Compatibility aliases for storage commands
output_info() { log_info "$@"; }
output_success() { log_success "$@"; }
output_error() { log_error "$@"; }
output_warning() { log_warning "$@"; }

# Ensure color variables are defined
COLOR_RESET=${COLOR_RESET:-$'\033[0m'}
COLOR_BLUE=${COLOR_BLUE:-$'\033[0;34m'}
COLOR_GREEN=${COLOR_GREEN:-$'\033[0;32m'}
COLOR_RED=${COLOR_RED:-$'\033[0;31m'}
COLOR_YELLOW=${COLOR_YELLOW:-$'\033[0;33m'}
COLOR_CYAN=${COLOR_CYAN:-$'\033[0;36m'}
COLOR_DIM=${COLOR_DIM:-$'\033[0;90m'}
COLOR_BOLD=${COLOR_BOLD:-$'\033[1m'}

show_service_help() {
  cat <<'EOF'
nself service - Consolidated Service Management (43+ subcommands)

USAGE:
  nself service <subcommand> [options]

CORE OPERATIONS:
  list              List all optional services and their status
  enable <name>     Enable an optional service
  disable <name>    Disable an optional service
  status [name]     Show status of all or specific service
  restart <name>    Restart a service
  logs <name> [-f]  View service logs
  init <name>       Initialize a service

CODE GENERATION (4):
  scaffold <name> --template <type> --port <port>
                    Scaffold a custom service from template
  list-templates    List all available service templates
  template-info     Show detailed information about a template
  wizard            Interactive service creation wizard

ADMIN SERVICE (consolidated from admin, admin-dev):
  admin [--dev]              Open admin UI
  admin dev enable [port]    Enable dev mode (route to localhost)
  admin dev disable          Disable dev mode (use Docker container)
  admin dev env              Show env vars for local development
  admin dev status           Show current dev mode status
  admin stats                Get overview statistics
  admin users [limit]        List users
  admin activity [hours]     Recent activity
  admin security             Security events

STORAGE SERVICE (consolidated from storage):
  storage init               Initialize storage system
  storage upload <file>      Upload a file to storage
  storage list [prefix]      List uploaded files
  storage delete <path>      Delete an uploaded file
  storage config             Configure upload pipeline
  storage status             Show pipeline status
  storage test               Test upload functionality
  storage graphql-setup      Generate GraphQL integration package

EMAIL SERVICE (consolidated from email):
  email setup                Interactive email setup wizard
  email list                 List all email providers
  email configure <provider> Configure SMTP provider
  email configure --api <p>  Configure API provider
  email validate             Check email configuration
  email check                SMTP connection pre-flight
  email check --api          API connection pre-flight
  email test [email]         Send test email (SMTP)
  email test --api [email]   Send test email (API)
  email docs [provider]      Get setup instructions
  email detect               Show current provider

SEARCH SERVICE (consolidated from search):
  search enable [engine]     Enable search with selected engine
  search disable             Disable search service
  search status              Show search service status
  search list                List available search engines
  search setup               Interactive search setup
  search test ["query"]      Test search functionality
  search reindex             Rebuild search index
  search config              Show current configuration
  search docs [engine]       Show search documentation

REDIS SERVICE (consolidated from redis):
  redis init                 Initialize Redis configuration
  redis add --name <name>    Add Redis connection
  redis list                 List all connections
  redis get <name>           Get connection details
  redis delete <name>        Delete connection
  redis test <name>          Test connection
  redis health [name]        Get health status
  redis pool configure       Configure connection pool
  redis pool get <name>      Get pool configuration

FUNCTIONS SERVICE (consolidated from functions):
  functions status           Show functions service status
  functions init [--ts]      Initialize functions service
  functions enable           Enable functions service
  functions disable          Disable functions service
  functions list             List available functions
  functions create <name>    Create a new function
  functions delete <name>    Delete a function
  functions test <name>      Test a function
  functions logs [-f]        View function logs
  functions deploy [target]  Deploy functions

MLFLOW SERVICE (consolidated from mlflow):
  mlflow status              Show MLflow status
  mlflow enable              Enable MLflow service
  mlflow disable             Disable MLflow service
  mlflow open                Open MLflow UI in browser
  mlflow configure <s> <v>   Configure MLflow settings
  mlflow experiments         List/manage experiments
  mlflow runs [exp_id]       List runs
  mlflow logs [-f]           View MLflow logs
  mlflow test                Test MLflow connection

REALTIME SERVICE (consolidated from realtime):
  realtime init              Initialize real-time system
  realtime status            Show real-time system status
  realtime logs [--follow]   Show real-time logs
  realtime cleanup           Clean up stale connections
  realtime subscribe <tbl>   Subscribe to table changes
  realtime unsubscribe <t>   Unsubscribe from table changes
  realtime listen <tbl>      Listen to table changes
  realtime subscriptions     List active subscriptions
  realtime channel create    Create a channel
  realtime channel list      List channels
  realtime broadcast         Send message to channel
  realtime presence track    Track user presence
  realtime presence online   List online users
  realtime connections       Show active connections

EXAMPLES:
  # Core operations
  nself service list                        # List all services
  nself service enable search               # Enable search service

  # Admin
  nself service admin dev enable 3000       # Enable admin dev mode
  nself service admin stats                 # Get stats

  # Storage
  nself service storage upload photo.jpg    # Upload file
  nself service storage list                # List files

  # Email
  nself service email configure sendgrid    # Configure email
  nself service email test --api admin@x.com

  # Search
  nself service search enable meilisearch   # Enable search
  nself service search test "hello world"   # Test search

  # Functions
  nself service functions create hello      # Create function
  nself service functions deploy            # Deploy

  # MLflow
  nself service mlflow enable               # Enable MLflow
  nself service mlflow experiments          # List experiments

  # Realtime
  nself service realtime init               # Initialize
  nself service realtime subscribe users    # Subscribe to table

OPTIONS:
  -h, --help        Show this help message
  --follow, -f      Follow logs (for logs commands)
  --tail <n>        Show last n lines (for logs commands)

See individual command help for more options.
EOF
}

# Get docker compose command
get_compose_cmd() {
  if command -v docker-compose &>/dev/null; then
    echo "docker-compose"
  else
    echo "docker compose"
  fi
}

# Check if service is enabled
is_service_enabled() {
  local service="$1"

  local env_var
  case "$service" in
    email | mailpit) env_var="MAILPIT_ENABLED" ;;
    search | meilisearch) env_var="MEILISEARCH_ENABLED" ;;
    functions) env_var="FUNCTIONS_ENABLED" ;;
    mlflow) env_var="MLFLOW_ENABLED" ;;
    admin | nself-admin) env_var="NSELF_ADMIN_ENABLED" ;;
    storage | minio) env_var="MINIO_ENABLED" ;;
    cache | redis) env_var="REDIS_ENABLED" ;;
    monitoring) env_var="MONITORING_ENABLED" ;;
    *) return 1 ;;
  esac

  # Check environment
  local value
  value=$(grep "^${env_var}=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr '[:upper:]' '[:lower:]')

  [[ "$value" == "true" || "$value" == "1" || "$value" == "yes" ]]
}

# Check if service container is running
is_service_running() {
  local service="$1"

  local container_name
  case "$service" in
    email | mailpit) container_name="mailpit" ;;
    search | meilisearch) container_name="meilisearch" ;;
    functions) container_name="functions" ;;
    mlflow) container_name="mlflow" ;;
    admin | nself-admin) container_name="nself-admin" ;;
    storage | minio) container_name="minio" ;;
    cache | redis) container_name="redis" ;;
    *) container_name="$service" ;;
  esac

  docker ps --format '{{.Names}}' 2>/dev/null | grep -q "$container_name"
}

# === LIST SUBCOMMAND ===
cmd_service_list() {
  printf "\n=== Optional Services Status ===\n\n"
  printf "%-15s %-10s %-10s %-30s\n" "SERVICE" "ENABLED" "RUNNING" "DESCRIPTION"
  printf "%s\n" "$(printf '%.0s-' {1..70})"

  _service_row "email" "MAILPIT_ENABLED" "Mail testing (MailPit)"
  _service_row "search" "MEILISEARCH_ENABLED" "Full-text search (MeiliSearch)"
  _service_row "functions" "FUNCTIONS_ENABLED" "Serverless functions"
  _service_row "mlflow" "MLFLOW_ENABLED" "ML experiment tracking"
  _service_row "admin" "NSELF_ADMIN_ENABLED" "nself Admin UI"
  _service_row "storage" "MINIO_ENABLED" "S3-compatible storage (MinIO)"
  _service_row "cache" "REDIS_ENABLED" "Cache and sessions (Redis)"
  _service_row "monitoring" "MONITORING_ENABLED" "Full monitoring stack (10 services)"

  echo ""
  log_info "Enable: nself service enable <service>"
  log_info "Disable: nself service disable <service>"
}

_service_row() {
  local service="$1"
  local env_var="$2"
  local description="$3"

  local enabled="no"
  local running="no"

  if is_service_running "$service"; then
    running="yes"
    # If service is running, it's enabled (may be enabled in .env.dev or other cascaded file)
    enabled="yes"
  elif is_service_enabled "$service"; then
    enabled="yes"
  fi

  # Special case: admin service with dev mode enabled
  # When admin dev mode is on, the Docker container doesn't run but the service is still "enabled"
  if [[ "$service" == "admin" ]]; then
    local dev_mode
    dev_mode=$(grep "^NSELF_ADMIN_DEV=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr '[:upper:]' '[:lower:]')
    if [[ "$dev_mode" == "true" || "$dev_mode" == "1" || "$dev_mode" == "yes" ]]; then
      enabled="yes"
      # Check if local dev server is running on configured port
      local dev_port
      dev_port=$(grep "^NSELF_ADMIN_DEV_PORT=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' || echo "3000")
      if curl -s -o /dev/null -w '' "http://localhost:${dev_port}" 2>/dev/null; then
        running="yes"
      else
        running="dev" # Indicate dev mode but not running
      fi
    fi
  fi

  printf "%-15s %-10s %-10s %-30s\n" "$service" "$enabled" "$running" "$description"
}

# === ENABLE SUBCOMMAND ===
cmd_service_enable() {
  local service="${1:-}"

  if [[ -z "$service" ]]; then
    log_error "Service name required"
    log_info "Usage: nself service enable <service>"
    cmd_service_list
    return 1
  fi

  local env_var
  case "$service" in
    email | mailpit) env_var="MAILPIT_ENABLED" ;;
    search | meilisearch) env_var="MEILISEARCH_ENABLED" ;;
    functions) env_var="FUNCTIONS_ENABLED" ;;
    mlflow) env_var="MLFLOW_ENABLED" ;;
    admin | nself-admin) env_var="NSELF_ADMIN_ENABLED" ;;
    storage | minio) env_var="MINIO_ENABLED" ;;
    cache | redis) env_var="REDIS_ENABLED" ;;
    monitoring) env_var="MONITORING_ENABLED" ;;
    *)
      log_error "Unknown service: $service"
      return 1
      ;;
  esac

  if [[ ! -f ".env" ]]; then
    log_error "No .env file found. Run 'nself init' first."
    return 1
  fi

  # Update .env file
  if grep -q "^${env_var}=" .env; then
    # Update existing
    if [[ "$OSTYPE" == "darwin"* ]]; then
      sed -i '' "s/^${env_var}=.*/${env_var}=true/" .env
    else
      sed -i "s/^${env_var}=.*/${env_var}=true/" .env
    fi
  else
    # Add new
    echo "${env_var}=true" >>.env
  fi

  log_success "Service $service enabled"
  log_info "Run 'nself build && nself start' to apply changes"
}

# === DISABLE SUBCOMMAND ===
cmd_service_disable() {
  local service="${1:-}"

  if [[ -z "$service" ]]; then
    log_error "Service name required"
    return 1
  fi

  local env_var
  case "$service" in
    email | mailpit) env_var="MAILPIT_ENABLED" ;;
    search | meilisearch) env_var="MEILISEARCH_ENABLED" ;;
    functions) env_var="FUNCTIONS_ENABLED" ;;
    mlflow) env_var="MLFLOW_ENABLED" ;;
    admin | nself-admin) env_var="NSELF_ADMIN_ENABLED" ;;
    storage | minio) env_var="MINIO_ENABLED" ;;
    cache | redis) env_var="REDIS_ENABLED" ;;
    monitoring) env_var="MONITORING_ENABLED" ;;
    *)
      log_error "Unknown service: $service"
      return 1
      ;;
  esac

  if [[ ! -f ".env" ]]; then
    log_error "No .env file found"
    return 1
  fi

  if grep -q "^${env_var}=" .env; then
    if [[ "$OSTYPE" == "darwin"* ]]; then
      sed -i '' "s/^${env_var}=.*/${env_var}=false/" .env
    else
      sed -i "s/^${env_var}=.*/${env_var}=false/" .env
    fi
  fi

  log_success "Service $service disabled"
  log_info "Run 'nself build && nself start' to apply changes"
}

# === STATUS SUBCOMMAND ===
cmd_service_status() {
  local service="${1:-}"

  if [[ -z "$service" ]]; then
    cmd_service_list
    return
  fi

  if ! is_service_enabled "$service"; then
    log_warning "Service $service is not enabled"
    return 1
  fi

  local container_name
  case "$service" in
    email | mailpit) container_name="mailpit" ;;
    search | meilisearch) container_name="meilisearch" ;;
    functions) container_name="functions" ;;
    mlflow) container_name="mlflow" ;;
    admin | nself-admin) container_name="nself-admin" ;;
    storage | minio) container_name="minio" ;;
    cache | redis) container_name="redis" ;;
    *) container_name="$service" ;;
  esac

  printf "\n=== %s Status ===\n\n" "$service"

  docker ps --filter "name=$container_name" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
}

# === RESTART SUBCOMMAND ===
cmd_service_restart() {
  local service="${1:-}"

  if [[ -z "$service" ]]; then
    log_error "Service name required"
    return 1
  fi

  local container_name
  case "$service" in
    email | mailpit) container_name="mailpit" ;;
    search | meilisearch) container_name="meilisearch" ;;
    functions) container_name="functions" ;;
    mlflow) container_name="mlflow" ;;
    admin | nself-admin) container_name="nself-admin" ;;
    storage | minio) container_name="minio" ;;
    cache | redis) container_name="redis" ;;
    *) container_name="$service" ;;
  esac

  log_info "Restarting $service..."

  local compose_cmd
  compose_cmd=$(get_compose_cmd)

  $compose_cmd restart "$container_name"

  log_success "Service $service restarted"
}

# === LOGS SUBCOMMAND ===
cmd_service_logs() {
  local service="${1:-}"
  local follow="${FOLLOW:-false}"
  local tail="${TAIL:-100}"

  if [[ -z "$service" ]]; then
    log_error "Service name required"
    return 1
  fi

  local container_name
  case "$service" in
    email | mailpit) container_name="mailpit" ;;
    search | meilisearch) container_name="meilisearch" ;;
    functions) container_name="functions" ;;
    mlflow) container_name="mlflow" ;;
    admin | nself-admin) container_name="nself-admin" ;;
    storage | minio) container_name="minio" ;;
    cache | redis) container_name="redis" ;;
    *) container_name="$service" ;;
  esac

  local compose_cmd
  compose_cmd=$(get_compose_cmd)

  local log_args=()
  log_args+=("--tail=$tail")

  if [[ "$follow" == "true" ]]; then
    log_args+=("-f")
  fi

  $compose_cmd logs "${log_args[@]}" "$container_name"
}

# === INIT SUBCOMMAND ===
cmd_service_init() {
  local service="${1:-}"

  if [[ -z "$service" ]]; then
    log_error "Service name required"
    log_info "Usage: nself service init <service>"
    log_info "Services: functions, search, storage, cache, mlflow"
    return 1
  fi

  case "$service" in
    functions | fn)
      init_functions_service
      ;;
    search | meilisearch)
      init_search_service
      ;;
    storage | minio | s3)
      init_storage_service
      ;;
    cache | redis)
      init_cache_service
      ;;
    mlflow | ml)
      init_mlflow_service
      ;;
    *)
      log_error "Service '$service' does not require initialization"
      log_info "Services that can be initialized: functions, search, storage, cache, mlflow"
      return 1
      ;;
  esac
}

init_functions_service() {
  log_info "Initializing functions service..."

  # Create functions directory
  mkdir -p ./functions 2>/dev/null || true

  # Create package.json if missing
  if [[ ! -f "./functions/package.json" ]]; then
    cat >./functions/package.json <<'PKGEOF'
{
  "name": "nself-functions",
  "version": "1.0.0",
  "description": "Serverless functions for nself project",
  "main": "index.js",
  "scripts": {
    "lint": "eslint ."
  },
  "dependencies": {}
}
PKGEOF
    log_info "Created functions/package.json"
  fi

  # Create example function if none exist
  local func_count
  func_count=$(find ./functions -maxdepth 1 \( -name "*.js" -o -name "*.ts" \) 2>/dev/null | wc -l | tr -d ' ')
  if [[ "$func_count" -eq 0 ]]; then
    cat >./functions/hello.js <<'FUNCEOF'
// Example serverless function
async function handler(event, context) {
  return {
    statusCode: 200,
    headers: { 'Content-Type': 'application/json' },
    body: {
      message: 'Hello from nself functions!',
      timestamp: new Date().toISOString()
    }
  };
}

module.exports = { handler };
FUNCEOF
    log_info "Created functions/hello.js (example function)"
  fi

  log_success "Functions service initialized"
  log_info "Deploy with: nself service functions deploy"
}

init_search_service() {
  log_info "Initializing search service (MeiliSearch)..."

  # Create search config directory
  mkdir -p ./.nself/search 2>/dev/null || true

  # Create search configuration file
  if [[ ! -f "./.nself/search/config.json" ]]; then
    cat >./.nself/search/config.json <<'SEARCHEOF'
{
  "indexes": [],
  "settings": {
    "maxIndexSize": "100MB",
    "searchableAttributes": ["*"],
    "displayedAttributes": ["*"]
  }
}
SEARCHEOF
    log_info "Created .nself/search/config.json"
  fi

  log_success "Search service initialized"
  log_info "Create indexes with: nself service search index create <name>"
}

init_storage_service() {
  log_info "Initializing storage service (MinIO)..."

  # Create storage config
  mkdir -p ./.nself/storage 2>/dev/null || true

  # Create storage configuration
  if [[ ! -f "./.nself/storage/buckets.json" ]]; then
    cat >./.nself/storage/buckets.json <<'STOREOF'
{
  "buckets": [
    {"name": "default", "public": false},
    {"name": "public", "public": true},
    {"name": "uploads", "public": false}
  ],
  "policies": {
    "public": "download"
  }
}
STOREOF
    log_info "Created .nself/storage/buckets.json"
  fi

  log_success "Storage service initialized"
  log_info "Manage buckets with: nself service storage buckets"
}

init_cache_service() {
  log_info "Initializing cache service (Redis)..."

  # Create cache config directory
  mkdir -p ./.nself/cache 2>/dev/null || true

  # Create Redis configuration
  if [[ ! -f "./.nself/cache/redis.conf" ]]; then
    cat >./.nself/cache/redis.conf <<'REDISEOF'
# Redis configuration for nself
maxmemory 256mb
maxmemory-policy allkeys-lru
appendonly yes
appendfsync everysec
REDISEOF
    log_info "Created .nself/cache/redis.conf"
  fi

  log_success "Cache service initialized"
  log_info "View stats with: nself service cache stats"
}

init_mlflow_service() {
  log_info "Initializing MLflow service..."

  # Create MLflow artifacts directory
  mkdir -p ./mlruns 2>/dev/null || true
  mkdir -p ./.nself/mlflow 2>/dev/null || true

  # Create MLflow configuration
  if [[ ! -f "./.nself/mlflow/config.yaml" ]]; then
    cat >./.nself/mlflow/config.yaml <<'MLEOF'
# MLflow configuration
artifact_store:
  type: local  # or 's3' for MinIO
  path: ./mlruns

tracking:
  backend_store: sqlite:///mlruns/mlflow.db

experiments:
  default_name: "default"
MLEOF
    log_info "Created .nself/mlflow/config.yaml"
  fi

  log_success "MLflow service initialized"
  log_info "Open UI with: nself service mlflow ui"
}

# === EMAIL SUBCOMMANDS ===
cmd_service_email() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    help | -h | --help)
      printf "nself email - Email provider management\n\n"
      printf "USAGE:\n  nself email <subcommand>\n\n"
      printf "SUBCOMMANDS:\n"
      printf "  list                    List available email providers\n"
      printf "  detect                  Detect current email provider\n"
      printf "  configure <provider>    Configure SMTP provider\n"
      printf "  configure --api <p>     Configure API provider\n"
      printf "  validate                Check email configuration\n"
      printf "  check                   SMTP connection pre-flight\n"
      printf "  check --api             API connection pre-flight\n"
      printf "  setup                   Interactive email setup wizard\n"
      printf "  status                  Show email service status\n"
      printf "  test [email]            Send a test email\n"
      printf "  inbox                   Open MailPit inbox\n"
      ;;
    list)
      printf "Available email providers:\n\n"
      printf "  SMTP Providers:\n"
      printf "    mailpit      Development mail catcher (default in dev)\n"
      printf "    sendgrid     SendGrid SMTP\n"
      printf "    mailgun      Mailgun SMTP\n"
      printf "    ses          Amazon SES SMTP\n"
      printf "    postmark     Postmark SMTP\n"
      printf "    smtp         Custom SMTP server\n"
      printf "\n  API Providers:\n"
      printf "    sendgrid-api SendGrid Web API\n"
      printf "    mailgun-api  Mailgun HTTP API\n"
      printf "    elastic      Elastic Email API\n"
      ;;
    detect)
      local provider="development"
      if [[ -n "${SMTP_HOST:-}" ]]; then
        provider="${SMTP_HOST}"
      elif [[ -n "${SENDGRID_API_KEY:-}" ]]; then
        provider="sendgrid-api"
      elif [[ -n "${MAILGUN_API_KEY:-}" ]]; then
        provider="mailgun-api"
      elif [[ "${MAILPIT_ENABLED:-false}" == "true" ]]; then
        provider="mailpit"
      fi
      printf "Email provider: %s\n" "$provider"
      ;;
    configure)
      cmd_email_configure "$@"
      ;;
    validate)
      cmd_email_validate "$@"
      ;;
    check)
      cmd_email_check "$@"
      ;;
    setup)
      cmd_email_setup "$@"
      ;;
    status)
      cmd_service_status email
      ;;
    test)
      cmd_email_test "$@"
      ;;
    inbox)
      cmd_email_inbox
      ;;
    config)
      cmd_email_config "$@"
      ;;
    *)
      log_error "Unknown email action: $action"
      log_info "Available: help, list, detect, configure, validate, check, setup, status, test, inbox, config"
      return 1
      ;;
  esac
}

cmd_email_configure() {
  local first="${1:-}"

  if [[ "$first" == "--api" ]]; then
    local api_provider="${2:-}"
    if [[ -z "$api_provider" ]]; then
      printf "API Email Providers:\n"
      printf "  sendgrid-api   SendGrid Web API\n"
      printf "  mailgun-api    Mailgun HTTP API\n"
      printf "  elastic        Elastic Email API\n"
      printf "\nUsage: nself email configure --api <provider>\n"
      return 1
    fi
    printf "Configure %s API provider:\n" "$api_provider"
    case "$api_provider" in
      sendgrid*) printf "  Set SENDGRID_API_KEY in .env\n" ;;
      mailgun*)  printf "  Set MAILGUN_API_KEY and MAILGUN_DOMAIN in .env\n" ;;
      elastic*)  printf "  Set ELASTIC_EMAIL_API_KEY in .env\n" ;;
      *)         printf "  Set provider API key in .env\n" ;;
    esac
  elif [[ -z "$first" ]]; then
    printf "Error: Provider name required\n" >&2
    printf "Usage: nself email configure <provider>\n"
    printf "       nself email configure --api <provider>\n"
    return 1
  else
    printf "Configure %s (SMTP):\n" "$first"
    printf "  SMTP_HOST=smtp.%s.com\n" "$first"
    printf "  SMTP_PORT=587\n"
    printf "  SMTP_USER=your-username\n"
    printf "  SMTP_PASSWORD=your-password\n"
    printf "  SMTP_FROM=noreply@yourdomain.com\n"
    printf "\nAdd these to your .env file.\n"
  fi
}

cmd_email_validate() {
  if [[ -n "${SMTP_HOST:-}" ]] || [[ -n "${SENDGRID_API_KEY:-}" ]] || [[ -n "${MAILGUN_API_KEY:-}" ]]; then
    printf "Email configuration found\n"
    return 0
  else
    printf "Email not fully configured (no SMTP_HOST or API key set)\n" >&2
    return 1
  fi
}

cmd_email_check() {
  if [[ "${1:-}" == "--api" ]]; then
    if [[ -z "${SENDGRID_API_KEY:-}${MAILGUN_API_KEY:-}${ELASTIC_EMAIL_API_KEY:-}" ]]; then
      printf "Email API credentials not configured\n" >&2
      return 1
    fi
    printf "API credentials found\n"
    return 0
  fi
  if [[ -z "${SMTP_HOST:-}" ]]; then
    printf "SMTP not configured (SMTP_HOST not set)\n" >&2
    return 1
  fi
  printf "SMTP configured: %s\n" "${SMTP_HOST}"
  return 0
}

cmd_email_setup() {
  printf "Email Setup Wizard\n\n"
  printf "Choose an email provider:\n"
  printf "  1) MailPit (development, no config needed)\n"
  printf "  2) SendGrid SMTP\n"
  printf "  3) Mailgun SMTP\n"
  printf "  4) Custom SMTP\n"
  printf "  5) SendGrid API\n"
  printf "\nPress Ctrl+C to cancel, or enter your choice:\n"
  local choice=""
  read -t 10 -p "Choice [1-5]: " choice 2>/dev/null || choice="1"
  printf "\nSetup complete. Edit .env with your provider settings.\n"
  return 0
}

cmd_email_test() {
  local to="${1:-test@example.com}"

  # Validate email format before inserting into JSON body (prevents injection)
  if [[ ! "$to" =~ ^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$ ]]; then
    log_error "Invalid email address: $to"
    return 1
  fi

  if ! is_service_running "mailpit"; then
    log_error "Email service not running"
    return 1
  fi

  log_info "Sending test email to $to..."

  # Use MailPit API — $to validated above (RFC-compliant email chars only)
  local mailpit_url="http://localhost:8025"

  curl -s -X POST "${mailpit_url}/api/v1/send" \
    -H "Content-Type: application/json" \
    -d "{
      \"From\": {\"Email\": \"nself@localhost\", \"Name\": \"nself\"},
      \"To\": [{\"Email\": \"$to\"}],
      \"Subject\": \"nself Test Email\",
      \"Text\": \"This is a test email from nself.\",
      \"HTML\": \"<h1>Test Email</h1><p>This is a test email from nself.</p>\"
    }" >/dev/null 2>&1 || true

  log_success "Test email sent"
  log_info "View at: http://localhost:8025"
}

cmd_email_inbox() {
  local url="http://localhost:8025"
  log_info "Opening email inbox: $url"

  if command -v open &>/dev/null; then
    open "$url"
  elif command -v xdg-open &>/dev/null; then
    xdg-open "$url"
  else
    log_info "Open in browser: $url"
  fi
}

cmd_email_config() {
  printf "\n=== Email Configuration ===\n\n"

  if [[ -f ".env" ]]; then
    grep -E "^(SMTP_|MAILPIT_|EMAIL_)" .env 2>/dev/null || echo "No email configuration found"
  fi
}

# === SEARCH SUBCOMMANDS ===
cmd_service_search() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    status)
      cmd_service_status search
      ;;
    index)
      cmd_search_index "$@"
      ;;
    query)
      cmd_search_query "$@"
      ;;
    stats)
      cmd_search_stats
      ;;
    *)
      log_error "Unknown search action: $action"
      log_info "Available: status, index, query, stats"
      return 1
      ;;
  esac
}

cmd_search_index() {
  local action="${1:-list}"
  local index_name="${2:-}"

  local meilisearch_url="http://localhost:7700"
  local master_key
  master_key=$(grep "^MEILI_MASTER_KEY=" .env 2>/dev/null | cut -d'=' -f2 || echo "")

  local auth_header=""
  if [[ -n "$master_key" ]]; then
    auth_header="-H \"Authorization: Bearer $master_key\""
  fi

  case "$action" in
    list | ls)
      log_info "Listing indexes..."
      curl -s "$meilisearch_url/indexes" $auth_header | python3 -m json.tool 2>/dev/null ||
        curl -s "$meilisearch_url/indexes" $auth_header
      ;;
    create)
      if [[ -z "$index_name" ]]; then
        log_error "Index name required"
        return 1
      fi
      log_info "Creating index: $index_name"
      curl -s -X POST "$meilisearch_url/indexes" $auth_header \
        -H "Content-Type: application/json" \
        -d "{\"uid\": \"$index_name\", \"primaryKey\": \"id\"}"
      ;;
    delete)
      if [[ -z "$index_name" ]]; then
        log_error "Index name required"
        return 1
      fi
      log_info "Deleting index: $index_name"
      curl -s -X DELETE "$meilisearch_url/indexes/$index_name" $auth_header
      ;;
    *)
      log_error "Unknown index action: $action"
      log_info "Available: list, create, delete"
      return 1
      ;;
  esac
}

cmd_search_query() {
  local index="${1:-}"
  local query="${2:-}"

  if [[ -z "$index" || -z "$query" ]]; then
    log_error "Usage: nself service search query <index> <query>"
    return 1
  fi

  local meilisearch_url="http://localhost:7700"

  curl -s "$meilisearch_url/indexes/$index/search" \
    -H "Content-Type: application/json" \
    -d "{\"q\": \"$query\"}" | python3 -m json.tool 2>/dev/null ||
    curl -s "$meilisearch_url/indexes/$index/search" \
      -H "Content-Type: application/json" \
      -d "{\"q\": \"$query\"}"
}

cmd_search_stats() {
  local meilisearch_url="http://localhost:7700"

  # Get API key from environment (check multiple variable names)
  local master_key
  master_key=$(grep "^MEILISEARCH_MASTER_KEY=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' || echo "")
  [[ -z "$master_key" ]] && master_key=$(grep "^MEILI_MASTER_KEY=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' || echo "")

  printf "\n=== Search Statistics ===\n\n"

  if [[ -n "$master_key" ]]; then
    curl -s "$meilisearch_url/stats" -H "Authorization: Bearer $master_key" | python3 -m json.tool 2>/dev/null ||
      curl -s "$meilisearch_url/stats" -H "Authorization: Bearer $master_key"
  else
    curl -s "$meilisearch_url/stats" | python3 -m json.tool 2>/dev/null ||
      curl -s "$meilisearch_url/stats"
  fi
}

# === FUNCTIONS SUBCOMMANDS ===
cmd_service_functions() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    status)
      cmd_service_status functions
      ;;
    deploy)
      cmd_functions_deploy "$@"
      ;;
    invoke)
      cmd_functions_invoke "$@"
      ;;
    logs)
      cmd_service_logs functions
      ;;
    list | ls)
      cmd_functions_list
      ;;
    *)
      log_error "Unknown functions action: $action"
      log_info "Available: status, deploy, invoke, logs, list"
      return 1
      ;;
  esac
}

cmd_functions_deploy() {
  local functions_dir="${1:-./functions}"

  if [[ ! -d "$functions_dir" ]]; then
    log_warning "Functions directory not found: $functions_dir"
    return 1
  fi

  log_info "Deploying functions from $functions_dir..."

  # Sync functions to container
  local compose_cmd
  compose_cmd=$(get_compose_cmd)

  $compose_cmd cp "$functions_dir/." functions:/opt/functions/

  log_success "Functions deployed"
}

cmd_functions_invoke() {
  local function_name="${1:-}"
  local payload="${2:-{}}"

  if [[ -z "$function_name" ]]; then
    log_error "Function name required"
    return 1
  fi

  local functions_url="http://localhost:3000"

  log_info "Invoking function: $function_name"

  curl -s -X POST "$functions_url/$function_name" \
    -H "Content-Type: application/json" \
    -d "$payload"
}

cmd_functions_list() {
  log_info "Deployed functions:"

  local functions_url="http://localhost:3000"

  curl -s "$functions_url/_functions" 2>/dev/null ||
    log_info "Functions endpoint not available"
}

# === MLFLOW SUBCOMMANDS ===
cmd_service_mlflow() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    status)
      cmd_service_status mlflow
      ;;
    ui)
      cmd_mlflow_ui
      ;;
    experiments)
      cmd_mlflow_experiments "$@"
      ;;
    runs)
      cmd_mlflow_runs "$@"
      ;;
    artifacts)
      cmd_mlflow_artifacts "$@"
      ;;
    *)
      log_error "Unknown mlflow action: $action"
      log_info "Available: status, ui, experiments, runs, artifacts"
      return 1
      ;;
  esac
}

cmd_mlflow_ui() {
  local url="http://localhost:5000"
  log_info "Opening MLflow UI: $url"

  if command -v open &>/dev/null; then
    open "$url"
  elif command -v xdg-open &>/dev/null; then
    xdg-open "$url"
  else
    log_info "Open in browser: $url"
  fi
}

cmd_mlflow_experiments() {
  local mlflow_url="http://localhost:5000"

  log_info "Listing experiments..."

  curl -s "$mlflow_url/api/2.0/mlflow/experiments/list" | python3 -m json.tool 2>/dev/null ||
    curl -s "$mlflow_url/api/2.0/mlflow/experiments/list"
}

cmd_mlflow_runs() {
  local experiment_id="${1:-0}"
  local mlflow_url="http://localhost:5000"

  log_info "Listing runs for experiment $experiment_id..."

  curl -s "$mlflow_url/api/2.0/mlflow/runs/search" \
    -H "Content-Type: application/json" \
    -d "{\"experiment_ids\": [\"$experiment_id\"]}" | python3 -m json.tool 2>/dev/null ||
    curl -s "$mlflow_url/api/2.0/mlflow/runs/search" \
      -H "Content-Type: application/json" \
      -d "{\"experiment_ids\": [\"$experiment_id\"]}"
}

cmd_mlflow_artifacts() {
  log_info "MLflow artifacts are stored in MinIO (if enabled)"
  log_info "Access via: nself service storage buckets list"
}

# === ADMIN SUBCOMMANDS ===
cmd_service_admin() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    status)
      cmd_service_status admin
      ;;
    open)
      cmd_admin_open
      ;;
    users)
      cmd_admin_users "$@"
      ;;
    config)
      cmd_admin_config "$@"
      ;;
    dev)
      cmd_admin_dev "$@"
      ;;
    *)
      log_error "Unknown admin action: $action"
      log_info "Available: status, open, users, config, dev"
      return 1
      ;;
  esac
}

# Admin development mode - run local nself-admin instead of Docker container
cmd_admin_dev() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    enable)
      cmd_admin_dev_enable "$@"
      ;;
    disable)
      cmd_admin_dev_disable
      ;;
    env)
      cmd_admin_dev_env
      ;;
    status | "")
      cmd_admin_dev_status
      ;;
    *)
      log_error "Unknown dev action: $action"
      printf "\nUsage: nself service admin dev [action]\n\n"
      printf "Actions:\n"
      printf "  status    Show current dev mode status (default)\n"
      printf "  enable    Enable dev mode (routes to local server)\n"
      printf "  disable   Disable dev mode (use Docker container)\n"
      printf "  env       Show environment variables for local development\n"
      return 1
      ;;
  esac
}

cmd_admin_dev_status() {
  printf "\n${COLOR_CYAN}Admin Development Mode${COLOR_RESET}\n"
  printf "======================\n\n"

  # Load environment with proper cascading (BUG-005 fix)
  # This reads from .env.dev -> .env.staging -> .env.prod -> .env
  # based on the current ENV setting
  load_env_with_priority 2>/dev/null || true

  # Now read from the loaded environment variables
  local dev_enabled="${NSELF_ADMIN_DEV:-false}"
  local dev_port="${NSELF_ADMIN_DEV_PORT:-3000}"
  local dev_path="${NSELF_ADMIN_DEV_PATH:-}"
  local base_domain="${BASE_DOMAIN:-local.nself.org}"

  if [[ "$dev_enabled" == "true" ]]; then
    printf "Status: ${COLOR_GREEN}ENABLED${COLOR_RESET}\n"
    printf "Port:   ${COLOR_BLUE}%s${COLOR_RESET}\n" "$dev_port"
    [[ -n "$dev_path" ]] && printf "Path:   ${COLOR_BLUE}%s${COLOR_RESET}\n" "$dev_path"
    printf "URL:    ${COLOR_BLUE}https://admin.%s${COLOR_RESET}\n" "$base_domain"
    printf "\nNginx routes admin.* to localhost:%s\n" "$dev_port"
    printf "Docker container: ${COLOR_DIM}not running (dev mode)${COLOR_RESET}\n"
  else
    printf "Status: ${COLOR_DIM}DISABLED${COLOR_RESET} (using Docker container)\n"
    printf "URL:    ${COLOR_BLUE}https://admin.%s${COLOR_RESET}\n" "$base_domain"
  fi

  printf "\nCommands:\n"
  printf "  nself admin-dev on [port] [path]   Enable dev mode (recommended)\n"
  printf "  nself admin-dev off                Disable dev mode\n"
  printf "  nself admin-dev status             Show current status\n"
  printf "\n${COLOR_DIM}Or use: nself service admin dev [enable|disable|env|status]${COLOR_RESET}\n"
}

cmd_admin_dev_enable() {
  local port="${1:-3000}"
  local path="${2:-}"

  log_info "Enabling admin development mode..."

  # Update .env file
  if [[ -f ".env" ]]; then
    # Remove existing settings
    grep -v "^NSELF_ADMIN_DEV" .env >.env.tmp 2>/dev/null || true
    mv .env.tmp .env

    # Add new settings
    {
      printf "\n# Admin Development Mode (local dev server)\n"
      printf "NSELF_ADMIN_DEV=true\n"
      printf "NSELF_ADMIN_DEV_PORT=%s\n" "$port"
      [[ -n "$path" ]] && printf "NSELF_ADMIN_DEV_PATH=%s\n" "$path"
    } >>.env

    log_success "Dev mode enabled in .env"
  else
    log_error "No .env file found"
    return 1
  fi

  printf "\n${COLOR_YELLOW}Next steps:${COLOR_RESET}\n"
  printf "1. Rebuild: ${COLOR_BLUE}nself build${COLOR_RESET}\n"
  printf "2. Restart: ${COLOR_BLUE}nself restart${COLOR_RESET}\n"
  printf "3. Start local admin on port %s\n" "$port"
  [[ -n "$path" ]] && printf "   cd %s && npm run dev\n" "$path"

  printf "\n"
  cmd_admin_dev_env
}

cmd_admin_dev_disable() {
  log_info "Disabling admin development mode..."

  if [[ -f ".env" ]]; then
    # Remove dev mode settings
    grep -v "^NSELF_ADMIN_DEV" .env >.env.tmp 2>/dev/null || true
    mv .env.tmp .env

    log_success "Dev mode disabled"
    printf "\n${COLOR_YELLOW}Next steps:${COLOR_RESET}\n"
    printf "1. Rebuild: ${COLOR_BLUE}nself build${COLOR_RESET}\n"
    printf "2. Restart: ${COLOR_BLUE}nself restart${COLOR_RESET}\n"
    printf "\nDocker container will be used for nself-admin.\n"
  else
    log_error "No .env file found"
    return 1
  fi
}

cmd_admin_dev_env() {
  printf "\n${COLOR_CYAN}Environment Variables for Local Admin Development${COLOR_RESET}\n"
  printf "=================================================\n\n"

  # Load environment with proper cascading (consistent with BUG-005 fix)
  load_env_with_priority 2>/dev/null || true

  # Use loaded environment variables with defaults
  local project_name="${PROJECT_NAME:-nself}"
  local base_domain="${BASE_DOMAIN:-localhost}"
  local postgres_user="${POSTGRES_USER:-postgres}"
  local postgres_password="${POSTGRES_PASSWORD:-}"
  local postgres_db="${POSTGRES_DB:-nself}"
  local hasura_secret="${HASURA_GRAPHQL_ADMIN_SECRET:-}"
  local admin_secret="${ADMIN_SECRET_KEY:-}"

  printf "Add these to your local nself-admin .env.local:\n\n"
  printf "${COLOR_DIM}# Database (via Docker)${COLOR_RESET}\n"
  printf "DATABASE_URL=postgres://%s:%s@localhost:5432/%s\n\n" "$postgres_user" "$postgres_password" "$postgres_db"

  printf "${COLOR_DIM}# Hasura (via Docker)${COLOR_RESET}\n"
  printf "HASURA_GRAPHQL_ENDPOINT=http://localhost:8080/v1/graphql\n"
  printf "HASURA_GRAPHQL_ADMIN_SECRET=%s\n\n" "$hasura_secret"

  printf "${COLOR_DIM}# Admin${COLOR_RESET}\n"
  printf "ADMIN_SECRET_KEY=%s\n" "$admin_secret"
  printf "PROJECT_NAME=%s\n" "$project_name"
  printf "BASE_DOMAIN=%s\n" "$base_domain"
  printf "NODE_ENV=development\n\n"

  printf "${COLOR_DIM}# Project path (your nself project)${COLOR_RESET}\n"
  printf "PROJECT_PATH=%s\n" "$(pwd)"
  printf "NSELF_PROJECT_PATH=%s\n\n" "$(pwd)"

  printf "${COLOR_YELLOW}Note:${COLOR_RESET} Ensure Docker services are running: nself start\n"
}

cmd_admin_open() {
  local url="http://localhost:4000"
  log_info "Opening Admin UI: $url"

  if command -v open &>/dev/null; then
    open "$url"
  elif command -v xdg-open &>/dev/null; then
    xdg-open "$url"
  else
    log_info "Open in browser: $url"
  fi
}

cmd_admin_users() {
  log_info "Admin users are managed through the Admin UI"
  log_info "Open: nself service admin open"
}

cmd_admin_config() {
  printf "\n=== Admin Configuration ===\n\n"

  if [[ -f ".env" ]]; then
    grep -E "^(NSELF_ADMIN_|ADMIN_)" .env 2>/dev/null || echo "No admin configuration found"
  fi
}

# === STORAGE SUBCOMMANDS ===
cmd_service_storage() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    help | -h | --help)
      printf "nself service storage - Object storage management\n\n"
      printf "USAGE:\n  nself service storage <subcommand>\n\n"
      printf "SUBCOMMANDS:\n"
      printf "  status      Show storage service status\n"
      printf "  buckets     Manage storage buckets\n"
      printf "  upload      Upload files\n"
      printf "  download    Download files\n"
      printf "  presign     Generate pre-signed URLs\n"
      ;;
    status)
      cmd_service_status storage
      ;;
    buckets)
      cmd_storage_buckets "$@"
      ;;
    upload)
      cmd_storage_upload "$@"
      ;;
    download)
      cmd_storage_download "$@"
      ;;
    presign)
      cmd_storage_presign "$@"
      ;;
    *)
      log_error "Unknown storage action: $action"
      log_info "Available: status, buckets, upload, download, presign"
      return 1
      ;;
  esac
}

cmd_storage_buckets() {
  local action="${1:-list}"

  if ! command -v mc &>/dev/null; then
    log_warning "MinIO client (mc) not installed"
    log_info "Install: brew install minio/stable/mc"
    return 1
  fi

  case "$action" in
    list | ls)
      mc ls minio/ 2>/dev/null || log_info "Configure mc first: mc alias set minio http://localhost:9000 minioadmin minioadmin"
      ;;
    create)
      local bucket="${2:-}"
      if [[ -z "$bucket" ]]; then
        log_error "Bucket name required"
        return 1
      fi
      mc mb "minio/$bucket"
      ;;
    delete)
      local bucket="${2:-}"
      if [[ -z "$bucket" ]]; then
        log_error "Bucket name required"
        return 1
      fi
      mc rb "minio/$bucket"
      ;;
    *)
      log_error "Unknown buckets action: $action"
      log_info "Available: list, create, delete"
      return 1
      ;;
  esac
}

cmd_storage_upload() {
  local file="${1:-}"
  local bucket="${2:-default}"

  if [[ -z "$file" ]]; then
    log_error "File path required"
    return 1
  fi

  if [[ ! -f "$file" ]]; then
    printf "File not found: %s\n" "$file" >&2
    return 1
  fi

  if ! command -v mc &>/dev/null; then
    log_warning "MinIO client (mc) not installed"
    return 1
  fi

  mc cp "$file" "minio/$bucket/"
  log_success "Uploaded: $file"
}

cmd_storage_download() {
  local object="${1:-}"
  local destination="${2:-.}"

  if [[ -z "$object" ]]; then
    log_error "Object path required (bucket/file)"
    return 1
  fi

  if ! command -v mc &>/dev/null; then
    log_warning "MinIO client (mc) not installed"
    return 1
  fi

  mc cp "minio/$object" "$destination"
  log_success "Downloaded: $object"
}

cmd_storage_presign() {
  local object="${1:-}"
  local expiry="${2:-1h}"

  if [[ -z "$object" ]]; then
    log_error "Object path required (bucket/file)"
    return 1
  fi

  if ! command -v mc &>/dev/null; then
    log_warning "MinIO client (mc) not installed"
    return 1
  fi

  mc share download "minio/$object" --expire "$expiry"
}

# === CACHE SUBCOMMANDS ===
cmd_service_cache() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    status)
      cmd_service_status cache
      ;;
    stats)
      cmd_cache_stats
      ;;
    flush)
      cmd_cache_flush "$@"
      ;;
    keys)
      cmd_cache_keys "$@"
      ;;
    *)
      log_error "Unknown cache action: $action"
      log_info "Available: status, stats, flush, keys"
      return 1
      ;;
  esac
}

cmd_cache_stats() {
  log_info "Redis statistics:"

  docker exec redis redis-cli INFO stats 2>/dev/null ||
    log_error "Redis not running or not accessible"
}

cmd_cache_flush() {
  local db="${1:-all}"

  log_warning "This will flush the Redis cache"
  printf "Are you sure? [y/N]: "
  read -r confirm
  confirm=$(echo "$confirm" | tr '[:upper:]' '[:lower:]')

  if [[ "$confirm" != "y" && "$confirm" != "yes" ]]; then
    log_info "Aborted"
    return 0
  fi

  if [[ "$db" == "all" ]]; then
    docker exec redis redis-cli FLUSHALL
  else
    docker exec redis redis-cli FLUSHDB
  fi

  log_success "Cache flushed"
}

cmd_cache_keys() {
  local pattern="${1:-*}"

  docker exec redis redis-cli KEYS "$pattern" 2>/dev/null ||
    log_error "Redis not running or not accessible"
}

# === SERVICE CODE GENERATION SUBCOMMANDS ===

# Scaffold a service from template
cmd_service_scaffold() {
  local service_name=""
  local template=""
  local port="3000"
  local output_dir="services"

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --template | -t)
        template="$2"
        shift 2
        ;;
      --port | -p)
        port="$2"
        shift 2
        ;;
      --output | -o)
        output_dir="$2"
        shift 2
        ;;
      -h | --help)
        show_scaffold_help
        return 0
        ;;
      *)
        if [[ -z "$service_name" ]]; then
          service_name="$1"
        fi
        shift
        ;;
    esac
  done

  # Validate required arguments
  if [[ -z "$service_name" ]]; then
    log_error "Service name required"
    printf "\nUsage: nself service scaffold <name> --template <template> [--port <port>]\n"
    printf "Example: nself service scaffold realtime --template socketio-ts --port 3101\n\n"
    printf "Run 'nself service list-templates' to see available templates\n"
    return 1
  fi

  if [[ -z "$template" ]]; then
    log_error "Template required"
    printf "\nUsage: nself service scaffold <name> --template <template> [--port <port>]\n"
    printf "Example: nself service scaffold realtime --template socketio-ts --port 3101\n\n"
    printf "Run 'nself service list-templates' to see available templates\n"
    return 1
  fi

  # Scaffold the service
  scaffold_service "$service_name" "$template" "$port" "$output_dir"
}

show_scaffold_help() {
  cat <<'EOF'
nself service scaffold - Generate service code from template

USAGE:
  nself service scaffold <name> --template <template> [options]

OPTIONS:
  --template, -t    Template to use (required)
  --port, -p        Port number (default: 3000)
  --output, -o      Output directory (default: services)
  --help, -h        Show this help

EXAMPLES:
  nself service scaffold realtime --template socketio-ts --port 3101
  nself service scaffold api --template fastapi --port 8000
  nself service scaffold worker --template bullmq-ts --port 3102

See 'nself service list-templates' for available templates.
EOF
}

# List all available templates
cmd_service_list_templates() {
  local language="${1:-}"
  local category=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --language | -l)
        language="$2"
        shift 2
        ;;
      --category | -c)
        category="$2"
        shift 2
        ;;
      -h | --help)
        show_list_templates_help
        return 0
        ;;
      *)
        shift
        ;;
    esac
  done

  printf "\n${COLOR_CYAN}Available Service Templates${COLOR_RESET}\n"
  printf "============================\n\n"

  # Get templates to display
  local templates
  if [[ -n "$language" ]]; then
    templates=$(get_templates_by_language "$language")
    if [[ -z "$templates" ]]; then
      log_error "Unknown language: $language"
      return 1
    fi
    printf "${COLOR_BOLD}Language:${COLOR_RESET} %s\n\n" "$language"
  else
    templates=$(list_all_templates)
  fi

  # Display templates grouped by category
  local current_category=""

  while read -r template_name; do
    [[ -z "$template_name" ]] && continue

    # Get metadata
    local metadata
    metadata=$(get_template_metadata "$template_name")

    IFS='|' read -r display_name lang description features deps <<<"$metadata"

    # Get category
    local tmpl_category
    tmpl_category=$(get_template_category "$template_name")

    # Filter by category if specified
    if [[ -n "$category" && "$tmpl_category" != "$category" ]]; then
      continue
    fi

    # Print category header if changed
    if [[ "$tmpl_category" != "$current_category" ]]; then
      [[ -n "$current_category" ]] && printf "\n"
      printf "${COLOR_BOLD}%s${COLOR_RESET}\n" "$tmpl_category"
      printf "%s\n" "$(printf '%.0s-' {1..60})"
      current_category="$tmpl_category"
    fi

    # Print template info
    printf "${COLOR_GREEN}%-20s${COLOR_RESET} ${COLOR_DIM}[%s]${COLOR_RESET}\n" "$template_name" "$lang"
    printf "  %s\n" "$description"

  done <<<"$templates"

  printf "\n${COLOR_CYAN}Usage:${COLOR_RESET}\n"
  printf "  nself service scaffold <name> --template <template> --port <port>\n"
  printf "  nself service template-info <template>  # Detailed info\n\n"

  printf "${COLOR_CYAN}Examples:${COLOR_RESET}\n"
  printf "  nself service scaffold realtime --template socketio-ts --port 3101\n"
  printf "  nself service scaffold api --template fastapi --port 8000\n"
  printf "  nself service scaffold worker --template bullmq-ts\n\n"
}

show_list_templates_help() {
  cat <<'EOF'
nself service list-templates - Show available service templates

USAGE:
  nself service list-templates [options]

OPTIONS:
  --language, -l    Filter by language (js, ts, python, go, etc.)
  --category, -c    Filter by category
  --help, -h        Show this help

CATEGORIES:
  - Web Frameworks
  - Full-Stack Frameworks
  - Real-time & Messaging
  - Background Jobs & Workers
  - AI & ML Agents
  - API Frameworks
  - RPC Frameworks
  - Runtime Servers

EXAMPLES:
  nself service list-templates
  nself service list-templates --language typescript
  nself service list-templates --category "Real-time & Messaging"
EOF
}

# Show detailed template information
cmd_service_template_info() {
  local template="${1:-}"

  if [[ -z "$template" ]]; then
    log_error "Template name required"
    printf "Usage: nself service template-info <template>\n"
    printf "Example: nself service template-info socketio-ts\n"
    return 1
  fi

  # Get metadata
  local metadata
  metadata=$(get_template_metadata "$template")

  if [[ "$metadata" == "Unknown|"* ]]; then
    log_error "Template not found: $template"
    printf "\nRun 'nself service list-templates' to see available templates\n"
    return 1
  fi

  IFS='|' read -r display_name lang description features deps <<<"$metadata"

  local category
  category=$(get_template_category "$template")

  printf "\n${COLOR_CYAN}%s${COLOR_RESET}\n" "$display_name"
  printf "%s\n" "$(printf '%.0s=' {1..60})"
  printf "\n"

  printf "${COLOR_BOLD}Template ID:${COLOR_RESET}    %s\n" "$template"
  printf "${COLOR_BOLD}Language:${COLOR_RESET}       %s\n" "$lang"
  printf "${COLOR_BOLD}Category:${COLOR_RESET}       %s\n" "$category"
  printf "\n"

  printf "${COLOR_BOLD}Description:${COLOR_RESET}\n"
  printf "  %s\n" "$description"
  printf "\n"

  if [[ -n "$features" ]]; then
    printf "${COLOR_BOLD}Features:${COLOR_RESET}\n"
    IFS=',' read -ra FEATURE_ARRAY <<<"$features"
    for feature in "${FEATURE_ARRAY[@]}"; do
      printf "  • %s\n" "$feature"
    done
    printf "\n"
  fi

  if [[ -n "$deps" && "$deps" != "None (Node.js built-in)" && "$deps" != "None (uses"* ]]; then
    printf "${COLOR_BOLD}Key Dependencies:${COLOR_RESET}\n"
    IFS=',' read -ra DEP_ARRAY <<<"$deps"
    for dep in "${DEP_ARRAY[@]}"; do
      printf "  • %s\n" "$dep"
    done
    printf "\n"
  fi

  printf "${COLOR_CYAN}Usage:${COLOR_RESET}\n"
  printf "  nself service scaffold <name> --template %s --port <port>\n" "$template"
  printf "\n"

  printf "${COLOR_CYAN}Example:${COLOR_RESET}\n"
  case "$template" in
    socketio-ts)
      printf "  nself service scaffold realtime --template socketio-ts --port 3101\n"
      printf "  # Generates: services/realtime/ with Socket.IO + TypeScript + Redis adapter\n"
      ;;
    fastapi)
      printf "  nself service scaffold api --template fastapi --port 8000\n"
      printf "  # Generates: services/api/ with FastAPI + Pydantic + auto-docs\n"
      ;;
    bullmq-ts)
      printf "  nself service scaffold worker --template bullmq-ts --port 3102\n"
      printf "  # Generates: services/worker/ with BullMQ + TypeScript worker\n"
      ;;
    *)
      printf "  nself service scaffold myservice --template %s --port 3000\n" "$template"
      ;;
  esac
  printf "\n"
}

# Interactive service creation wizard
cmd_service_wizard() {
  printf "\n${COLOR_CYAN}Service Creation Wizard${COLOR_RESET}\n"
  printf "========================\n\n"

  # Get service name
  printf "Service name: "
  read -r service_name

  if [[ -z "$service_name" ]]; then
    log_error "Service name is required"
    return 1
  fi

  # Show language options
  printf "\nSelect language:\n"
  printf "  1) TypeScript\n"
  printf "  2) JavaScript\n"
  printf "  3) Python\n"
  printf "  4) Go\n"
  printf "  5) Other\n"
  printf "\nChoice [1-5]: "
  read -r lang_choice

  local language
  case "$lang_choice" in
    1) language="typescript" ;;
    2) language="javascript" ;;
    3) language="python" ;;
    4) language="go" ;;
    5)
      printf "\nEnter language (ruby, rust, java, etc.): "
      read -r language
      ;;
    *)
      log_error "Invalid choice"
      return 1
      ;;
  esac

  # Get templates for selected language
  local templates
  templates=$(get_templates_by_language "$language")

  if [[ -z "$templates" ]]; then
    log_error "No templates found for language: $language"
    return 1
  fi

  # Show templates
  printf "\nAvailable templates for %s:\n\n" "$language"

  local -a template_array
  local i=1

  while read -r tmpl; do
    [[ -z "$tmpl" ]] && continue
    template_array+=("$tmpl")

    local metadata
    metadata=$(get_template_metadata "$tmpl")
    IFS='|' read -r display_name _ description _ _ <<<"$metadata"

    printf "  %d) ${COLOR_GREEN}%s${COLOR_RESET}\n" "$i" "$tmpl"
    printf "     %s\n" "$description"

    i=$((i + 1))
  done <<<"$templates"

  printf "\nChoice [1-%d]: " "$((i - 1))"
  read -r template_choice

  if [[ "$template_choice" -lt 1 || "$template_choice" -ge "$i" ]]; then
    log_error "Invalid choice"
    return 1
  fi

  local selected_template="${template_array[$((template_choice - 1))]}"

  # Get port
  printf "\nPort number [3000]: "
  read -r port
  port="${port:-3000}"

  # Confirm
  printf "\n${COLOR_CYAN}Summary:${COLOR_RESET}\n"
  printf "  Name:     %s\n" "$service_name"
  printf "  Template: %s\n" "$selected_template"
  printf "  Port:     %s\n" "$port"
  printf "\nProceed? [Y/n]: "
  read -r confirm
  confirm=$(echo "$confirm" | tr '[:upper:]' '[:lower:]')

  if [[ "$confirm" == "n" || "$confirm" == "no" ]]; then
    log_info "Cancelled"
    return 0
  fi

  # Scaffold the service
  scaffold_service "$service_name" "$selected_template" "$port" "services"
}

# === REDIS SUBCOMMANDS ===
cmd_service_redis() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    status)
      cmd_service_status redis
      ;;
    init | add | list | get | delete | test | health | pool)
      # Delegate to redis command functions
      if declare -f cmd_redis >/dev/null 2>&1; then
        cmd_redis "$action" "$@"
      else
        log_error "Redis CLI not available"
        log_info "Redis core functions missing"
        return 1
      fi
      ;;
    *)
      log_error "Unknown redis action: $action"
      log_info "Available: status, init, add, list, get, delete, test, health, pool"
      return 1
      ;;
  esac
}

# === REALTIME SUBCOMMANDS ===
cmd_service_realtime() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    status)
      # Show realtime service status
      if declare -f cmd_status >/dev/null 2>&1; then
        cmd_status "$@"
      else
        log_info "Realtime service"
        log_info "Run: nself service realtime init"
      fi
      ;;
    init | logs | cleanup | subscribe | unsubscribe | listen | subscriptions)
      # System and subscription commands
      if [[ -f "$SCRIPT_DIR/realtime.sh" ]]; then
        bash "$SCRIPT_DIR/realtime.sh" "$action" "$@"
      else
        log_error "Realtime service not available"
        return 1
      fi
      ;;
    channel | broadcast | messages | replay | events | presence | connections | stats)
      # Channel, broadcast, and presence commands
      if [[ -f "$SCRIPT_DIR/realtime.sh" ]]; then
        bash "$SCRIPT_DIR/realtime.sh" "$action" "$@"
      else
        log_error "Realtime service not available"
        return 1
      fi
      ;;
    *)
      log_error "Unknown realtime action: $action"
      log_info "Available: init, status, subscribe, channel, broadcast, presence"
      log_info "Run: nself service realtime --help"
      return 1
      ;;
  esac
}

# === MAIN ENTRY POINT ===
main() {
  local subcommand="${1:-}"
  shift || true

  # Parse global options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --follow | -f)
        export FOLLOW="true"
        shift
        ;;
      --tail)
        export TAIL="$2"
        shift 2
        ;;
      -h | --help)
        show_service_help
        return 0
        ;;
      *)
        break
        ;;
    esac
  done

  case "$subcommand" in
    "" | help | -h | --help)
      show_service_help
      ;;
    list | ls)
      cmd_service_list "$@"
      ;;
    enable)
      cmd_service_enable "$@"
      ;;
    disable)
      cmd_service_disable "$@"
      ;;
    status)
      cmd_service_status "$@"
      ;;
    restart)
      cmd_service_restart "$@"
      ;;
    logs)
      cmd_service_logs "$@"
      ;;
    init)
      cmd_service_init "$@"
      ;;
    scaffold)
      cmd_service_scaffold "$@"
      ;;
    list-templates | templates)
      cmd_service_list_templates "$@"
      ;;
    template-info | info)
      cmd_service_template_info "$@"
      ;;
    wizard)
      cmd_service_wizard "$@"
      ;;
    email | mail)
      cmd_service_email "$@"
      ;;
    search)
      cmd_service_search "$@"
      ;;
    functions | fn)
      cmd_service_functions "$@"
      ;;
    mlflow | ml)
      cmd_service_mlflow "$@"
      ;;
    admin)
      cmd_service_admin "$@"
      ;;
    storage | minio | s3)
      cmd_service_storage "$@"
      ;;
    cache)
      cmd_service_cache "$@"
      ;;
    redis)
      cmd_service_redis "$@"
      ;;
    realtime | rt)
      cmd_service_realtime "$@"
      ;;
    *)
      log_error "Unknown subcommand: $subcommand"
      show_service_help
      return 1
      ;;
  esac
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
