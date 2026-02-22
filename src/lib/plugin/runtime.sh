#!/usr/bin/env bash
# runtime.sh - Plugin runtime management (start/stop/logs)
# POSIX-compliant, no Bash 4+ features

# ============================================================================
# Plugin & Runtime Directories
# ============================================================================

PLUGIN_DIR="${PLUGIN_DIR:-${NSELF_PLUGIN_DIR:-$HOME/.nself/plugins}}"

set -euo pipefail

PLUGIN_RUNTIME_DIR="${NSELF_PLUGIN_RUNTIME:-$HOME/.nself/runtime}"
PLUGIN_LOGS_DIR="$PLUGIN_RUNTIME_DIR/logs"
PLUGIN_PIDS_DIR="$PLUGIN_RUNTIME_DIR/pids"
PLUGIN_STATES_DIR="$PLUGIN_RUNTIME_DIR/states"

# ============================================================================
# Setup & Prerequisites
# ============================================================================

# Ensure runtime directories exist
ensure_runtime_dirs() {
  mkdir -p "$PLUGIN_LOGS_DIR"
  mkdir -p "$PLUGIN_PIDS_DIR"
  mkdir -p "$PLUGIN_STATES_DIR"
}

# ============================================================================
# Lifecycle State Management
# ============================================================================
# States: starting, running, stopping, stopped, failed

# Set plugin state
set_plugin_state() {
  local plugin_name="$1"
  local state="$2"
  local state_file="$PLUGIN_STATES_DIR/${plugin_name}.state"

  ensure_runtime_dirs
  printf "%s" "$state" > "$state_file"
}

# Get plugin state
get_plugin_state() {
  local plugin_name="$1"
  local state_file="$PLUGIN_STATES_DIR/${plugin_name}.state"

  if [[ -f "$state_file" ]]; then
    cat "$state_file"
  else
    printf "stopped"
  fi
}

# Clear plugin state
clear_plugin_state() {
  local plugin_name="$1"
  local state_file="$PLUGIN_STATES_DIR/${plugin_name}.state"
  rm -f "$state_file"
}

# Setup shared utilities (one-time)
setup_shared_utilities() {
  local shared_link="$HOME/.nself/shared"
  local shared_target="$HOME/.nself/plugins/_shared"

  # Check if symlink already exists
  if [[ -L "$shared_link" ]]; then
    return 0
  fi

  # Check if _shared directory exists
  if [[ ! -d "$shared_target" ]]; then
    log_warning "Shared utilities not found at $shared_target"
    printf "Install plugins first with: nself plugin install <name>\n"
    return 1
  fi

  # Check if _shared is built
  if [[ ! -d "$shared_target/dist" ]]; then
    log_info "Building shared utilities..."

    # Try to build from source repo if available
    if [[ -d "$HOME/Sites/nself-plugins/shared" ]]; then
      (cd "$HOME/Sites/nself-plugins/shared" && pnpm install --silent && pnpm build --silent)
      cp -r "$HOME/Sites/nself-plugins/shared/dist" "$shared_target/"
    else
      log_error "Shared utilities not built and source not found"
      printf "\nRun: cd ~/Sites/nself-plugins/shared && pnpm install && pnpm build\n"
      return 1
    fi
  fi

  # Create symlink
  ln -s "$shared_target" "$shared_link"
  log_success "Shared utilities ready"
}

# Get DATABASE_URL from project .env
get_database_url() {
  local db_url=""

  # Use project directory (where nself command was run)
  local project_dir="${NSELF_PROJECT_DIR:-$(pwd)}"

  # Try to load from project .env files
  for env_file in "$project_dir/.env.dev" "$project_dir/.env.local" "$project_dir/.env"; do
    if [[ -f "$env_file" ]]; then
      db_url=$(grep "^DATABASE_URL=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'")
      if [[ -n "$db_url" ]]; then
        break
      fi

      # Try building from POSTGRES_* variables
      local pg_user=$(grep "^POSTGRES_USER=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'")
      local pg_pass=$(grep "^POSTGRES_PASSWORD=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'")
      local pg_db=$(grep "^POSTGRES_DB=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'")
      local pg_host=$(grep "^POSTGRES_HOST=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'")
      local pg_port=$(grep "^POSTGRES_PORT=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'")

      pg_host="${pg_host:-localhost}"
      pg_port="${pg_port:-5432}"

      if [[ -n "$pg_user" ]] && [[ -n "$pg_pass" ]] && [[ -n "$pg_db" ]]; then
        db_url="postgresql://${pg_user}:${pg_pass}@${pg_host}:${pg_port}/${pg_db}"
        break
      fi
    fi
  done

  if [[ -z "$db_url" ]]; then
    log_error "Could not find DATABASE_URL in .env files"
    return 1
  fi

  printf '%s' "$db_url"
}

# Get or generate encryption key
get_encryption_key() {
  local key_file="$HOME/.nself/encryption.key"

  if [[ ! -f "$key_file" ]]; then
    log_info "Generating encryption key..."
    openssl rand -base64 32 > "$key_file"
    chmod 600 "$key_file"
  fi

  cat "$key_file"
}

# Get MinIO configuration from project .env (if enabled)
get_minio_config() {
  local project_dir="${NSELF_PROJECT_DIR:-$(pwd)}"
  local minio_enabled=""
  local minio_port=""
  local minio_user=""
  local minio_pass=""
  local minio_bucket=""

  # Try to load from project .env files
  for env_file in "$project_dir/.env.dev" "$project_dir/.env.local" "$project_dir/.env"; do
    if [[ -f "$env_file" ]]; then
      minio_enabled=$(grep "^MINIO_ENABLED=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d ' ')

      if [[ "$minio_enabled" == "true" ]]; then
        minio_port=$(grep "^MINIO_PORT=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d ' ')
        minio_user=$(grep "^MINIO_ROOT_USER=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d ' ')
        minio_pass=$(grep "^MINIO_ROOT_PASSWORD=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d ' ')

        # Try to find a default bucket (prefer MINIO_BUCKET_RAW or first bucket found)
        minio_bucket=$(grep "^MINIO_BUCKET_RAW=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d ' ')
        if [[ -z "$minio_bucket" ]]; then
          minio_bucket=$(grep "^MINIO_BUCKET_" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d ' ')
        fi

        break
      fi
    fi
  done

  # Return empty if MinIO not enabled
  if [[ "$minio_enabled" != "true" ]]; then
    return 0
  fi

  # Set defaults
  minio_port="${minio_port:-9000}"
  minio_user="${minio_user:-minioadmin}"
  minio_pass="${minio_pass:-minioadmin}"
  minio_bucket="${minio_bucket:-default}"

  # Output as key=value pairs (will be parsed by caller)
  printf "FILE_STORAGE_PROVIDER=minio\n"
  printf "FILE_STORAGE_ENDPOINT=http://127.0.0.1:%s\n" "$minio_port"
  printf "FILE_STORAGE_BUCKET=%s\n" "$minio_bucket"
  printf "FILE_STORAGE_ACCESS_KEY=%s\n" "$minio_user"
  printf "FILE_STORAGE_SECRET_KEY=%s\n" "$minio_pass"
}

# Get Redis configuration from project .env (if enabled)
get_redis_config() {
  local project_dir="${NSELF_PROJECT_DIR:-$(pwd)}"
  local redis_enabled=""
  local redis_host=""
  local redis_port=""
  local redis_pass=""

  # Try to load from project .env files
  for env_file in "$project_dir/.env.dev" "$project_dir/.env.local" "$project_dir/.env"; do
    if [[ -f "$env_file" ]]; then
      redis_enabled=$(grep "^REDIS_ENABLED=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d ' ')

      if [[ "$redis_enabled" == "true" ]]; then
        redis_host=$(grep "^REDIS_HOST=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d ' ')
        redis_port=$(grep "^REDIS_PORT=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d ' ')
        redis_pass=$(grep "^REDIS_PASSWORD=" "$env_file" | head -1 | cut -d= -f2- | tr -d '"' | tr -d "'" | tr -d ' ')
        break
      fi
    fi
  done

  # Return empty if Redis not enabled
  if [[ "$redis_enabled" != "true" ]]; then
    return 0
  fi

  # Set defaults
  redis_host="${redis_host:-127.0.0.1}"
  redis_port="${redis_port:-6379}"

  # Build Redis URL
  local redis_url="redis://"
  if [[ -n "$redis_pass" ]]; then
    redis_url="${redis_url}:${redis_pass}@"
  fi
  redis_url="${redis_url}${redis_host}:${redis_port}"

  printf "JOBS_REDIS_URL=%s\n" "$redis_url"
  printf "REDIS_URL=%s\n" "$redis_url"
}

# ============================================================================
# Plugin Process Management
# ============================================================================

# Check if plugin is running
is_plugin_running() {
  local plugin_name="$1"
  local pid_file="$PLUGIN_PIDS_DIR/${plugin_name}.pid"

  if [[ ! -f "$pid_file" ]]; then
    return 1
  fi

  local pid=$(cat "$pid_file")
  if kill -0 "$pid" 2>/dev/null; then
    return 0
  else
    # PID file exists but process is dead
    rm -f "$pid_file"
    return 1
  fi
}

# Get plugin PID
get_plugin_pid() {
  local plugin_name="$1"
  local pid_file="$PLUGIN_PIDS_DIR/${plugin_name}.pid"

  if [[ -f "$pid_file" ]]; then
    cat "$pid_file"
  fi
}

# Prepare plugin for startup
prepare_plugin() {
  local plugin_name="$1"
  local plugin_dir="$PLUGIN_DIR/$plugin_name/ts"

  if [[ ! -d "$plugin_dir" ]]; then
    log_error "Plugin '$plugin_name' not installed"
    return 1
  fi

  # Install dependencies if needed
  if [[ ! -d "$plugin_dir/node_modules" ]]; then
    log_info "Installing dependencies for $plugin_name..."
    local install_output=$(mktemp)
    (cd "$plugin_dir" && pnpm install 2>&1) > "$install_output" &
    local install_pid=$!

    # Show spinner while installing
    local spin_chars="⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
    local i=0
    while kill -0 $install_pid 2>/dev/null; do
      local char="${spin_chars:$((i % ${#spin_chars})):1}"
      printf "\r${COLOR_BLUE}%s${COLOR_RESET} Installing dependencies for $plugin_name..." "$char"
      i=$((i + 1))
      sleep 0.1
    done
    wait $install_pid
    local install_result=$?
    rm -f "$install_output"

    if [[ $install_result -ne 0 ]]; then
      printf "\r${COLOR_RED}✗${COLOR_RESET} Failed to install dependencies for $plugin_name\n"
      return 1
    else
      printf "\r${COLOR_GREEN}✓${COLOR_RESET} Dependencies installed for $plugin_name         \n"
    fi
  fi

  # Build if needed
  if [[ ! -d "$plugin_dir/dist" ]]; then
    log_info "Building $plugin_name..."
    local build_output=$(mktemp)
    (cd "$plugin_dir" && pnpm build 2>&1) > "$build_output" &
    local build_pid=$!

    # Show spinner while building
    local spin_chars="⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
    local i=0
    while kill -0 $build_pid 2>/dev/null; do
      local char="${spin_chars:$((i % ${#spin_chars})):1}"
      printf "\r${COLOR_BLUE}%s${COLOR_RESET} Building $plugin_name..." "$char"
      i=$((i + 1))
      sleep 0.1
    done
    wait $build_pid
    local build_result=$?
    rm -f "$build_output"

    if [[ $build_result -ne 0 ]]; then
      printf "\r${COLOR_RED}✗${COLOR_RESET} Failed to build $plugin_name                    \n"
      return 1
    else
      printf "\r${COLOR_GREEN}✓${COLOR_RESET} Build completed for $plugin_name               \n"
    fi
  fi

  return 0
}

# Create plugin .env file
create_plugin_env() {
  local plugin_name="$1"
  local plugin_dir="$PLUGIN_DIR/$plugin_name/ts"
  local port="${2:-}"

  # Get plugin manifest for default port
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"
  if [[ -z "$port" ]] && [[ -f "$manifest" ]]; then
    if command -v jq >/dev/null 2>&1; then
      port=$(jq -r '.port // 3000' "$manifest" 2>/dev/null)
    else
      port=$(grep '"port"' "$manifest" | head -1 | sed 's/[^0-9]//g')
    fi
  fi
  port="${port:-3000}"

  local env_file="$plugin_dir/.env"

  # Don't overwrite existing .env
  if [[ -f "$env_file" ]]; then
    return 0
  fi

  local db_url
  db_url=$(get_database_url) || return 1
  if [[ -z "$db_url" ]]; then
    log_error "Cannot create .env for $plugin_name: DATABASE_URL is empty"
    log_info "Ensure .env files exist in your project directory with POSTGRES_* or DATABASE_URL"
    return 1
  fi

  # Validate DATABASE_URL format
  if [[ ! "$db_url" =~ ^postgresql:// ]]; then
    log_error "Invalid DATABASE_URL format for $plugin_name: $db_url"
    log_info "Expected: postgresql://user:pass@host:port/database"
    return 1
  fi
  local encryption_key
  encryption_key=$(get_encryption_key) || return 1

  # Get optional service configurations
  local minio_config=$(get_minio_config)
  local redis_config=$(get_redis_config)

  # Start building .env file
  cat > "$env_file" <<EOF
# Auto-generated by nself plugin start
# Edit as needed for plugin-specific configuration

# Core Configuration
DATABASE_URL=$db_url
ENCRYPTION_KEY=$encryption_key
PORT=$port
LOG_LEVEL=info
EOF

  # Add MinIO configuration if enabled
  if [[ -n "$minio_config" ]]; then
    printf "\n# File Storage (MinIO)\n" >> "$env_file"
    printf "%s\n" "$minio_config" >> "$env_file"
  fi

  # Add Redis configuration if enabled
  if [[ -n "$redis_config" ]]; then
    printf "\n# Cache & Jobs (Redis)\n" >> "$env_file"
    printf "%s\n" "$redis_config" >> "$env_file"
  fi

  log_success "Created .env for $plugin_name"
}

# Validate required environment variables for plugin
validate_plugin_env() {
  local plugin_name="$1"
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"

  if [[ ! -f "$manifest" ]]; then
    return 0  # No manifest, skip validation
  fi

  # Check if plugin specifies required env vars
  local required_env=""
  if command -v jq >/dev/null 2>&1; then
    required_env=$(jq -r '.required_env // [] | .[]' "$manifest" 2>/dev/null)
  else
    # Fallback: parse JSON manually for required_env array
    required_env=$(grep -A10 '"required_env"' "$manifest" | grep -o '"[A-Z_]*"' | tr -d '"' | grep -v "required_env")
  fi

  if [[ -z "$required_env" ]]; then
    return 0  # No required env vars
  fi

  # Check each required variable
  local env_file="$PLUGIN_DIR/$plugin_name/ts/.env"
  local missing_vars=""
  for var in $required_env; do
    # Check if var exists in .env file
    if [[ -f "$env_file" ]]; then
      local value=$(grep "^${var}=" "$env_file" | cut -d= -f2-)
      if [[ -z "$value" ]]; then
        missing_vars="$missing_vars $var"
      fi
    else
      missing_vars="$missing_vars $var"
    fi
  done

  if [[ -n "$missing_vars" ]]; then
    log_error "Missing required environment variables for $plugin_name:$missing_vars"
    log_info "Add these variables to $env_file or your project's .env file"
    return 1
  fi

  return 0
}

# Start a plugin
start_plugin() {
  local plugin_name="$1"
  local port="${2:-}"

  ensure_runtime_dirs

  if is_plugin_running "$plugin_name"; then
    log_warning "Plugin '$plugin_name' is already running (PID: $(get_plugin_pid "$plugin_name"))"
    printf "  Use 'nself plugin restart %s' to restart it\n" "$plugin_name"
    return 0
  fi

  # Setup shared utilities if needed
  if [[ ! -L "$HOME/.nself/shared" ]]; then
    setup_shared_utilities || return 1
  fi

  # Prepare plugin (install deps, build)
  prepare_plugin "$plugin_name" || return 1

  # Create .env if needed
  create_plugin_env "$plugin_name" "$port" || return 1

  # Validate required environment variables
  validate_plugin_env "$plugin_name" || return 1

  # Check port availability before starting
  local plugin_dir="$PLUGIN_DIR/$plugin_name/ts"
  local env_file="$plugin_dir/.env"
  if [[ -f "$env_file" ]]; then
    local check_port=$(grep "^PORT=" "$env_file" | cut -d= -f2)
    if [[ -n "$check_port" ]] && command -v lsof >/dev/null 2>&1; then
      if lsof -ti ":$check_port" >/dev/null 2>&1; then
        local occupant=$(lsof -ti ":$check_port" | head -1)
        log_error "Port $check_port already in use by PID $occupant"
        log_info "Stop the conflicting process: kill $occupant"
        log_info "Or use a different port in plugin.json"
        return 1
      fi
    fi
  fi

  local plugin_dir="$PLUGIN_DIR/$plugin_name/ts"
  local log_file="$PLUGIN_LOGS_DIR/${plugin_name}.log"
  local pid_file="$PLUGIN_PIDS_DIR/${plugin_name}.pid"

  log_info "Starting $plugin_name..."
  set_plugin_state "$plugin_name" "starting"

  # Start plugin in background
  (
    cd "$plugin_dir" && \
    pnpm start > "$log_file" 2>&1 &
    echo $! > "$pid_file"
  )

  sleep 0.5

  if is_plugin_running "$plugin_name"; then
    set_plugin_state "$plugin_name" "running"
    log_success "$plugin_name started (PID: $(get_plugin_pid "$plugin_name"))"
    printf "Logs: nself plugin logs %s\n" "$plugin_name"
    return 0
  else
    set_plugin_state "$plugin_name" "failed"
    log_error "$plugin_name failed to start (check logs: $log_file)"
    return 1
  fi
}

# Stop a plugin
stop_plugin() {
  local plugin_name="$1"
  local force="${2:-false}"

  if ! is_plugin_running "$plugin_name"; then
    log_warning "Plugin '$plugin_name' is not running"
    printf "  Use 'nself plugin start %s' to start it\n" "$plugin_name"
    return 0
  fi

  local pid=$(get_plugin_pid "$plugin_name")

  if [[ "$force" == "true" ]]; then
    log_info "Force-stopping $plugin_name (PID: $pid)..."
  else
    log_info "Stopping $plugin_name (PID: $pid)..."
  fi

  set_plugin_state "$plugin_name" "stopping"

  if [[ "$force" == "true" ]]; then
    # Force mode: immediate SIGKILL
    pkill -9 -P "$pid" 2>/dev/null || true
    kill -9 "$pid" 2>/dev/null || true
  else
    # Graceful mode: Try SIGTERM first
    pkill -P "$pid" 2>/dev/null || true

    if kill "$pid" 2>/dev/null; then
      # Wait for graceful shutdown
      local timeout=5
      while kill -0 "$pid" 2>/dev/null && ((timeout > 0)); do
        sleep 1
        timeout=$((timeout - 1))
      done

      # Force kill if still running (process and any remaining children)
      if kill -0 "$pid" 2>/dev/null; then
        pkill -9 -P "$pid" 2>/dev/null || true
        kill -9 "$pid" 2>/dev/null
      fi
    fi
  fi

  # Clean up any process still holding the plugin's port
  local env_file="$PLUGIN_DIR/$plugin_name/ts/.env"
  if [[ -f "$env_file" ]]; then
    local port=$(grep "^PORT=" "$env_file" | cut -d= -f2)
    if [[ -n "$port" ]] && command -v lsof >/dev/null 2>&1; then
      local port_pids=$(lsof -ti ":$port" 2>/dev/null || true)
      if [[ -n "$port_pids" ]]; then
        kill -9 $port_pids 2>/dev/null || true
      fi
    fi
  fi

  rm -f "$PLUGIN_PIDS_DIR/${plugin_name}.pid"
  set_plugin_state "$plugin_name" "stopped"
  log_success "$plugin_name stopped"
  return 0
}

# Restart a plugin
restart_plugin() {
  local plugin_name="$1"
  stop_plugin "$plugin_name"

  # Wait for port to be fully released
  local env_file="$PLUGIN_DIR/$plugin_name/ts/.env"
  if [[ -f "$env_file" ]]; then
    local port=$(grep "^PORT=" "$env_file" | cut -d= -f2)
    if [[ -n "$port" ]] && command -v lsof >/dev/null 2>&1; then
      local wait_count=0
      while lsof -ti ":$port" >/dev/null 2>&1 && [[ $wait_count -lt 10 ]]; do
        sleep 0.5
        wait_count=$((wait_count + 1))
      done
    else
      sleep 1
    fi
  else
    sleep 1
  fi

  start_plugin "$plugin_name"
}

# ============================================================================
# Dependency Management
# ============================================================================

# Get plugin dependencies from manifest
get_plugin_dependencies() {
  local plugin_name="$1"
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"

  if [[ ! -f "$manifest" ]]; then
    return 0
  fi

  # Extract plugin dependencies (not npm dependencies)
  if command -v jq >/dev/null 2>&1; then
    # Check if dependencies is an object or array
    local dep_type=$(jq -r '.dependencies | type' "$manifest" 2>/dev/null)

    if [[ "$dep_type" == "object" ]]; then
      # New format: dependencies.plugins[] for plugin dependencies
      # dependencies.npm[] is for npm packages (not used for topological sort)
      jq -r '.dependencies.plugins[]?' "$manifest" 2>/dev/null || true
    elif [[ "$dep_type" == "array" ]]; then
      # Old format: flat array of plugin names (backward compatibility)
      jq -r '.dependencies[]?' "$manifest" 2>/dev/null || true
    fi
    # If dependencies is null or other type, return nothing (no plugin dependencies)
  else
    # Fallback: parse JSON manually
    # Look for dependencies.plugins field
    grep -A10 '"plugins"' "$manifest" | grep -o '"[a-z-]*"' | tr -d '"' | grep -v "plugins" || true
  fi
}

# Topological sort for plugin dependencies
# Returns plugins in dependency order (dependencies first)
topological_sort_plugins() {
  local -a all_plugins=()
  local -a sorted=()
  local -a visiting=()
  local -a visited=()

  # Collect all plugin names
  for plugin_dir in "$PLUGIN_DIR"/*/; do
    if [[ -f "$plugin_dir/plugin.json" ]]; then
      local name=$(basename "$plugin_dir")
      [[ "$name" == "_shared" ]] && continue
      all_plugins+=("$name")
    fi
  done

  # DFS visit function
  visit_plugin() {
    local plugin="$1"

    # Check if already visited
    if [[ ${#visited[@]} -gt 0 ]]; then
      local p
      for p in "${visited[@]}"; do
        if [[ "$p" == "$plugin" ]]; then
          return 0
        fi
      done
    fi

    # Check for circular dependency
    if [[ ${#visiting[@]} -gt 0 ]]; then
      local p
      for p in "${visiting[@]}"; do
        if [[ "$p" == "$plugin" ]]; then
          log_warning "Circular dependency detected involving $plugin"
          printf "  Check 'dependencies' field in plugin.json files\n"
          return 1
        fi
      done
    fi

    visiting+=("$plugin")

    # Visit dependencies first
    local deps=$(get_plugin_dependencies "$plugin")
    for dep in $deps; do
      # Check if dependency is installed
      local dep_installed=false
      if [[ ${#all_plugins[@]} -gt 0 ]]; then
        local p
        for p in "${all_plugins[@]}"; do
          if [[ "$p" == "$dep" ]]; then
            dep_installed=true
            break
          fi
        done
      fi

      if [[ "$dep_installed" == "true" ]]; then
        visit_plugin "$dep"
      else
        log_warning "Plugin $plugin depends on $dep which is not installed"
        printf "  Install it with: nself plugin install %s\n" "$dep"
      fi
    done

    # Remove from visiting, add to visited
    local new_visiting=()
    if [[ ${#visiting[@]} -gt 0 ]]; then
      local p
      for p in "${visiting[@]}"; do
        [[ "$p" != "$plugin" ]] && new_visiting+=("$p")
      done
    fi
    if [[ ${#new_visiting[@]} -gt 0 ]]; then
      visiting=("${new_visiting[@]}")
    else
      visiting=()
    fi

    visited+=("$plugin")
    sorted+=("$plugin")
  }

  # Visit all plugins
  if [[ ${#all_plugins[@]} -gt 0 ]]; then
    for plugin in "${all_plugins[@]}"; do
      visit_plugin "$plugin"
    done
  fi

  # Output sorted list
  if [[ ${#sorted[@]} -gt 0 ]]; then
    printf "%s\n" "${sorted[@]}"
  fi
}

# ============================================================================
# Batch Operations
# ============================================================================

# Start plugins (respecting dependencies)
# Usage: start_all_plugins [project_list_file]
#   project_list_file: path to a file with one plugin name per line.
#                      If provided, only those plugins are started.
#                      If omitted, all globally installed plugins are started.
start_all_plugins() {
  local project_list_file="${1:-}"
  ensure_runtime_dirs

  local count=0
  local failed=0

  # Get plugins in dependency order (full global list)
  local sorted_plugins
  sorted_plugins=$(topological_sort_plugins)

  if [[ -z "$sorted_plugins" ]]; then
    return 0
  fi

  # If a project list file is given, filter to only those plugins
  if [[ -n "$project_list_file" ]] && [[ -f "$project_list_file" ]]; then
    local filtered=""
    while IFS= read -r name; do
      [[ -z "$name" ]] && continue
      if grep -qx "$name" "$project_list_file" 2>/dev/null; then
        filtered="${filtered}${name}
"
      fi
    done <<< "$sorted_plugins"
    sorted_plugins="$filtered"
  fi

  if [[ -z "$sorted_plugins" ]]; then
    return 0
  fi

  # Start plugins in order
  while IFS= read -r name; do
    [[ -z "$name" ]] && continue
    if start_plugin "$name"; then
      count=$((count + 1))
    else
      failed=$((failed + 1))
    fi
  done <<< "$sorted_plugins"

  if [[ $count -eq 0 ]] && [[ $failed -eq 0 ]]; then
    return 0
  fi

  printf "\n"
  if [[ $count -gt 0 ]]; then
    log_success "Started $count plugin(s)"
  fi
  if [[ $failed -gt 0 ]]; then
    # Soft warning — plugin failures never look like project failures
    printf "  %d plugin(s) failed to start (logs: %s/)\n" "$failed" "$PLUGIN_LOGS_DIR"
  fi
}

# Stop all running plugins
stop_all_plugins() {
  local force="${1:-false}"
  local count=0

  for pid_file in "$PLUGIN_PIDS_DIR"/*.pid; do
    [[ -f "$pid_file" ]] || continue
    local name=$(basename "$pid_file" .pid)
    if stop_plugin "$name" "$force"; then
      count=$((count + 1))
    fi
  done

  if [[ $count -eq 0 ]]; then
    log_info "No plugins were running"
  else
    log_success "Stopped $count plugins"
  fi
}

# List all installed plugins with detailed status
list_all_plugins() {
  printf "\n=== Installed Plugins ===\n\n"

  # Source display utilities for colors
  if [[ -f "$(dirname "${BASH_SOURCE[0]}")/../utils/display.sh" ]]; then
    source "$(dirname "${BASH_SOURCE[0]}")/../utils/display.sh" 2>/dev/null || true
  fi

  # Print header
  printf "%-25s %-12s %-8s %-8s %-s\n" "PLUGIN" "STATE" "PID" "PORT" "DESCRIPTION"
  printf "%-25s %-12s %-8s %-8s %-s\n" "------" "-----" "---" "----" "-----------"

  local total=0
  local running=0
  local stopped=0
  local failed=0

  for plugin_dir in "$PLUGIN_DIR"/*/; do
    [[ -d "$plugin_dir" ]] || continue
    local name=$(basename "$plugin_dir")

    # Skip shared utilities
    [[ "$name" == "_shared" ]] && continue

    # Check if it has plugin.json
    if [[ ! -f "$plugin_dir/plugin.json" ]]; then
      continue
    fi

    total=$((total + 1))

    # Get state
    local state=$(get_plugin_state "$name")

    # Get PID if running
    local pid=""
    if is_plugin_running "$name"; then
      pid=$(get_plugin_pid "$name")
      state="running"
      running=$((running + 1))
    elif [[ "$state" == "running" ]]; then
      # State says running but no PID - it crashed
      state="failed"
      failed=$((failed + 1))
    elif [[ "$state" == "stopped" ]] || [[ "$state" == "failed" ]]; then
      if [[ "$state" == "failed" ]]; then
        failed=$((failed + 1))
      else
        stopped=$((stopped + 1))
      fi
    else
      # Default to stopped if no state file
      state="stopped"
      stopped=$((stopped + 1))
    fi

    # Get port from .env
    local port=""
    local env_file="$plugin_dir/ts/.env"
    if [[ -f "$env_file" ]]; then
      port=$(grep "^PORT=" "$env_file" | cut -d= -f2)
    fi

    # Get description from plugin.json
    local desc=""
    if command -v jq >/dev/null 2>&1; then
      desc=$(jq -r '.description // ""' "$plugin_dir/plugin.json" 2>/dev/null)
    else
      desc=$(grep '"description"' "$plugin_dir/plugin.json" | head -1 | sed 's/.*"description"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    fi

    # Truncate description if too long
    if [[ ${#desc} -gt 40 ]]; then
      desc="${desc:0:37}..."
    fi

    # Color-code state
    local state_display="$state"
    case "$state" in
      running)
        state_display="${COLOR_GREEN}●${COLOR_RESET} running"
        ;;
      stopped)
        state_display="${COLOR_DIM}○${COLOR_RESET} stopped"
        ;;
      starting)
        state_display="${COLOR_YELLOW}◐${COLOR_RESET} starting"
        ;;
      stopping)
        state_display="${COLOR_YELLOW}◐${COLOR_RESET} stopping"
        ;;
      failed)
        state_display="${COLOR_RED}✗${COLOR_RESET} failed"
        ;;
    esac

    # Print row
    printf "%-25s %-20s %-8s %-8s %-s\n" "$name" "$state_display" "$pid" "$port" "$desc"
  done

  # Print summary
  printf "\n"
  if [[ $total -eq 0 ]]; then
    log_info "No plugins installed"
    printf "\nInstall plugins with: nself plugin install <name>\n"
  else
    printf "Total: %d installed" "$total"
    if [[ $running -gt 0 ]]; then
      printf " | ${COLOR_GREEN}%d running${COLOR_RESET}" "$running"
    fi
    if [[ $stopped -gt 0 ]]; then
      printf " | ${COLOR_DIM}%d stopped${COLOR_RESET}" "$stopped"
    fi
    if [[ $failed -gt 0 ]]; then
      printf " | ${COLOR_RED}%d failed${COLOR_RESET}" "$failed"
    fi
    printf "\n"
  fi
}

# List running plugins (legacy, kept for compatibility)
list_running_plugins() {
  printf "\n=== Running Plugins ===\n\n"

  local count=0
  for pid_file in "$PLUGIN_PIDS_DIR"/*.pid; do
    [[ -f "$pid_file" ]] || continue
    local name=$(basename "$pid_file" .pid)

    if is_plugin_running "$name"; then
      local pid=$(get_plugin_pid "$name")

      # Get port from .env
      local port=""
      local env_file="$PLUGIN_DIR/$name/ts/.env"
      if [[ -f "$env_file" ]]; then
        port=$(grep "^PORT=" "$env_file" | cut -d= -f2)
      fi

      printf "%-20s PID: %-8s Port: %s\n" "$name" "$pid" "$port"
      count=$((count + 1))
    fi
  done

  if [[ $count -eq 0 ]]; then
    log_info "No plugins currently running"
    printf "\nStart plugins with: nself plugin start <name>\n"
  else
    printf "\nTotal: %d running\n" "$count"
  fi
}

# ============================================================================
# Health Checks
# ============================================================================

# Check plugin health
check_plugin_health() {
  local plugin_name="$1"

  if ! is_plugin_running "$plugin_name"; then
    printf "❌ %s - Not running\n" "$plugin_name"
    return 1
  fi

  # Get port
  local env_file="$PLUGIN_DIR/$plugin_name/ts/.env"
  local port=""
  if [[ -f "$env_file" ]]; then
    port=$(grep "^PORT=" "$env_file" | cut -d= -f2)
  fi

  if [[ -z "$port" ]]; then
    printf "⚠️  %s - Running but port unknown\n" "$plugin_name"
    return 1
  fi

  # Try health endpoint
  if command -v curl >/dev/null 2>&1; then
    if curl -sf "http://localhost:$port/health" >/dev/null 2>&1; then
      printf "✅ %s - Healthy (port %s)\n" "$plugin_name" "$port"
      return 0
    else
      printf "⚠️  %s - Running but not responding (port %s)\n" "$plugin_name" "$port"
      return 1
    fi
  else
    printf "⚠️  %s - Running (port %s, curl not available for health check)\n" "$plugin_name" "$port"
    return 0
  fi
}

# Health check all running plugins
health_check_all() {
  printf "\n=== Plugin Health Check ===\n\n"

  local count=0
  local healthy=0

  for pid_file in "$PLUGIN_PIDS_DIR"/*.pid; do
    [[ -f "$pid_file" ]] || continue
    local name=$(basename "$pid_file" .pid)

    if is_plugin_running "$name"; then
      count=$((count + 1))
      if check_plugin_health "$name"; then
        healthy=$((healthy + 1))
      fi
    fi
  done

  if [[ $count -eq 0 ]]; then
    log_info "No plugins running"
  else
    printf "\n%d/%d plugins healthy\n" "$healthy" "$count"
  fi
}

# ============================================================================
# Logs
# ============================================================================

# Show plugin logs
show_plugin_logs() {
  local plugin_name="$1"
  local follow="${2:-false}"

  local log_file="$PLUGIN_LOGS_DIR/${plugin_name}.log"

  if [[ ! -f "$log_file" ]]; then
    log_error "No logs found for $plugin_name"
    printf "Log file: %s\n" "$log_file"
    return 1
  fi

  if [[ "$follow" == "true" ]]; then
    tail -f "$log_file"
  else
    tail -50 "$log_file"
  fi
}

# Export functions
export -f ensure_runtime_dirs
export -f set_plugin_state
export -f get_plugin_state
export -f clear_plugin_state
export -f setup_shared_utilities
export -f get_database_url
export -f get_encryption_key
export -f get_minio_config
export -f get_redis_config
export -f is_plugin_running
export -f get_plugin_pid
export -f prepare_plugin
export -f create_plugin_env
export -f validate_plugin_env
export -f get_plugin_dependencies
export -f topological_sort_plugins
export -f start_plugin
export -f stop_plugin
export -f restart_plugin
export -f start_all_plugins
export -f stop_all_plugins
export -f list_all_plugins
export -f list_running_plugins
export -f check_plugin_health
export -f health_check_all
export -f show_plugin_logs
