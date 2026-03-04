#!/usr/bin/env bash

# shared.sh - Shared nginx container management for multi-project routing
# Manages a single nginx container that routes traffic to all registered nself projects
# Bash 3.2 compatible — printf only, parallel arrays, no Bash 4+ features

# Prevent double-sourcing
[[ "${NGINX_SHARED_SOURCED:-}" == "1" ]] && return 0
export NGINX_SHARED_SOURCED=1

set -euo pipefail

# Resolve lib directory for sourcing siblings
_NGINX_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source display utilities if available
if [[ -f "$_NGINX_LIB_DIR/../utils/display.sh" ]]; then
  . "$_NGINX_LIB_DIR/../utils/display.sh"
fi

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

SHARED_NGINX_DIR="$HOME/.nself/nginx"
SHARED_NGINX_COMPOSE="$SHARED_NGINX_DIR/docker-compose.yml"
SHARED_NGINX_CONF="$SHARED_NGINX_DIR/nginx.conf"
SHARED_SSL_DIR="$HOME/.nself/ssl"
SHARED_NGINX_CONTAINER="nself-shared-nginx"

# ---------------------------------------------------------------------------
# shared::container_name — Return the canonical container name
# ---------------------------------------------------------------------------
shared::container_name() {
  printf '%s\n' "$SHARED_NGINX_CONTAINER"
}

# ---------------------------------------------------------------------------
# shared::is_running — Check if the shared nginx container is running
# Returns 0 if running, 1 otherwise
# ---------------------------------------------------------------------------
shared::is_running() {
  local state
  state="$(docker inspect --format '{{.State.Running}}' "$SHARED_NGINX_CONTAINER" 2>/dev/null)" || return 1
  if [[ "$state" == "true" ]]; then
    return 0
  fi
  return 1
}

# ---------------------------------------------------------------------------
# shared::generate_compose — Generate docker-compose.yml for shared nginx
# Args: $1 = registry file path
# ---------------------------------------------------------------------------
shared::generate_compose() {
  local registry_file="${1:-}"

  if [[ -z "$registry_file" ]]; then
    log_error "Registry file path required"
    return 1
  fi

  if [[ ! -f "$registry_file" ]]; then
    log_error "Registry file not found: $registry_file"
    return 1
  fi

  # Source registry functions
  if [[ -f "$_NGINX_LIB_DIR/registry.sh" ]]; then
    . "$_NGINX_LIB_DIR/registry.sh"
  fi

  # Ensure directories exist
  mkdir -p "$SHARED_NGINX_DIR"
  mkdir -p "$SHARED_SSL_DIR"

  local compose_file="$SHARED_NGINX_COMPOSE"

  # Write docker-compose.yml header
  printf 'services:\n' > "$compose_file"
  printf '  nginx:\n' >> "$compose_file"
  printf '    image: nginx:alpine\n' >> "$compose_file"
  printf '    container_name: %s\n' "$SHARED_NGINX_CONTAINER" >> "$compose_file"
  printf '    restart: unless-stopped\n' >> "$compose_file"
  printf '    ports:\n' >> "$compose_file"
  printf '      - "127.0.0.1:80:80"\n' >> "$compose_file"
  printf '      - "127.0.0.1:443:443"\n' >> "$compose_file"
  printf '    volumes:\n' >> "$compose_file"
  printf '      - %s:/etc/nginx/nginx.conf:ro\n' "$SHARED_NGINX_CONF" >> "$compose_file"
  printf '      - %s:/etc/nginx/ssl:ro\n' "$SHARED_SSL_DIR" >> "$compose_file"

  # Read registered projects from JSON registry and mount their nginx/sites/ directories
  local project_entries
  project_entries="$(jq -r '.projects[]? | "\(.name)|\(.path)"' "$registry_file" 2>/dev/null)" || true

  if [[ -n "$project_entries" ]]; then
    printf '%s\n' "$project_entries" | while IFS='|' read -r project_name project_path; do
      if [[ -z "$project_name" ]] || [[ -z "$project_path" ]]; then
        continue
      fi

      local sites_dir="$project_path/nginx/sites"
      if [[ -d "$sites_dir" ]]; then
        printf '      - %s:/etc/nginx/conf.d/%s:ro\n' "$sites_dir" "$project_name" >> "$compose_file"
      else
        log_debug "Skipping volume mount for $project_name: $sites_dir does not exist"
      fi
    done
  fi

  # Write healthcheck
  printf '    healthcheck:\n' >> "$compose_file"
  printf '      test: ["CMD-SHELL", "wget --no-verbose --tries=1 -O /dev/null http://127.0.0.1/ 2>/dev/null || exit 1"]\n' >> "$compose_file"
  printf '      interval: 30s\n' >> "$compose_file"
  printf '      timeout: 5s\n' >> "$compose_file"
  printf '      retries: 3\n' >> "$compose_file"
  printf '      start_period: 10s\n' >> "$compose_file"

  log_debug "Generated shared nginx compose at $compose_file"
  return 0
}

# ---------------------------------------------------------------------------
# shared::generate_main_conf — Generate the main nginx.conf
# ---------------------------------------------------------------------------
shared::generate_main_conf() {
  mkdir -p "$SHARED_NGINX_DIR"

  local conf_file="$SHARED_NGINX_CONF"

  printf 'worker_processes auto;\n' > "$conf_file"
  printf '\n' >> "$conf_file"
  printf 'events {\n' >> "$conf_file"
  printf '    worker_connections 1024;\n' >> "$conf_file"
  printf '}\n' >> "$conf_file"
  printf '\n' >> "$conf_file"
  printf 'http {\n' >> "$conf_file"
  printf '    include /etc/nginx/mime.types;\n' >> "$conf_file"
  printf '    default_type application/octet-stream;\n' >> "$conf_file"
  printf '    sendfile on;\n' >> "$conf_file"
  printf '    keepalive_timeout 65;\n' >> "$conf_file"
  printf '\n' >> "$conf_file"
  printf '    include /etc/nginx/conf.d/*/*.conf;\n' >> "$conf_file"
  printf '}\n' >> "$conf_file"

  log_debug "Generated shared nginx.conf at $conf_file"
  return 0
}

# ---------------------------------------------------------------------------
# shared::start — Generate configs and start the shared nginx container
# Args: $1 = registry file path (optional, defaults to ~/.nself/nginx/registry.json)
# ---------------------------------------------------------------------------
shared::start() {
  local registry_file="${1:-$HOME/.nself/nginx/registry.json}"

  if [[ ! -f "$registry_file" ]]; then
    log_error "No registry file found at $registry_file"
    log_info "Register a project first with: nself nginx register"
    return 1
  fi

  log_info "Starting shared nginx container..."

  # Start the container (caller is responsible for generating configs first)
  if ! docker compose -f "$SHARED_NGINX_COMPOSE" -p nself-shared up -d 2>/dev/null; then
    # Fallback for older docker-compose
    if command -v docker-compose >/dev/null 2>&1; then
      docker-compose -f "$SHARED_NGINX_COMPOSE" -p nself-shared up -d
    else
      log_error "Failed to start shared nginx container"
      return 1
    fi
  fi

  # Verify the container is running
  local attempts=0
  local max_attempts=10
  while [[ $attempts -lt $max_attempts ]]; do
    if shared::is_running; then
      log_success "Shared nginx container is running"
      return 0
    fi
    attempts=$((attempts + 1))
    sleep 1
  done

  log_error "Shared nginx container failed to start within ${max_attempts}s"
  return 1
}

# ---------------------------------------------------------------------------
# shared::stop — Stop the shared nginx container
# ---------------------------------------------------------------------------
shared::stop() {
  if ! shared::is_running; then
    log_info "Shared nginx container is not running"
    return 0
  fi

  log_info "Stopping shared nginx container..."

  if ! docker compose -f "$SHARED_NGINX_COMPOSE" -p nself-shared down 2>/dev/null; then
    if command -v docker-compose >/dev/null 2>&1; then
      docker-compose -f "$SHARED_NGINX_COMPOSE" -p nself-shared down
    else
      # Direct stop as last resort
      docker stop "$SHARED_NGINX_CONTAINER" 2>/dev/null || true
      docker rm "$SHARED_NGINX_CONTAINER" 2>/dev/null || true
    fi
  fi

  log_success "Shared nginx container stopped"
  return 0
}

# ---------------------------------------------------------------------------
# shared::reload — Reload nginx configuration without restart
# ---------------------------------------------------------------------------
shared::reload() {
  if ! shared::is_running; then
    log_error "Shared nginx container is not running"
    return 1
  fi

  log_info "Reloading shared nginx configuration..."

  if docker exec "$SHARED_NGINX_CONTAINER" nginx -s reload 2>/dev/null; then
    log_success "Nginx configuration reloaded"
    return 0
  else
    log_error "Failed to reload nginx configuration"
    return 1
  fi
}

# ---------------------------------------------------------------------------
# shared::logs — Show container logs
# Args: $1 = number of tail lines (optional, defaults to 100)
# ---------------------------------------------------------------------------
shared::logs() {
  local tail_lines="${1:-100}"

  if ! docker ps -a --format '{{.Names}}' | grep -q "^${SHARED_NGINX_CONTAINER}$" 2>/dev/null; then
    log_error "Shared nginx container does not exist"
    return 1
  fi

  docker logs --tail "$tail_lines" "$SHARED_NGINX_CONTAINER"
}
