#!/usr/bin/env bash

# nginx-validator.sh - Nginx configuration validation and auto-fix utilities

# Source display utilities
UTILS_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

source "$UTILS_DIR/display.sh" 2>/dev/null || true

# Validate nginx configuration files
nginx::validate_config() {
  local config_dir="${1:-nginx/conf.d}"
  local errors_found=false
  local warnings=()

  # Check if config directory exists
  if [[ ! -d "$config_dir" ]]; then
    log_debug "Nginx config directory $config_dir does not exist"
    return 0
  fi

  # Check for common misconfigurations
  for conf_file in "$config_dir"/*.conf; do
    [[ -f "$conf_file" ]] || continue

    local filename=$(basename "$conf_file")
    log_debug "Validating $filename"

    # Check for rate limiting directives in wrong context
    if grep -q "limit_req_zone" "$conf_file" 2>/dev/null; then
      log_error "Rate limiting directive 'limit_req_zone' found in $filename"
      log_info "This directive must be in http context (nginx.conf), not in server blocks"
      errors_found=true

      # Auto-fix if enabled
      if [[ "${AUTO_FIX:-true}" == "true" ]]; then
        nginx::fix_rate_limiting "$conf_file"
      fi
    fi

    # Check for deprecated http2 directive
    if grep -q "listen.*http2;" "$conf_file" 2>/dev/null; then
      warnings+=("$filename uses deprecated 'listen ... http2' syntax")

      # Auto-fix if enabled
      if [[ "${AUTO_FIX:-true}" == "true" ]]; then
        nginx::fix_http2_directive "$conf_file"
      fi
    fi

    # Check for missing SSL certificates
    if grep -q "ssl_certificate" "$conf_file" 2>/dev/null; then
      local cert_path=$(grep "ssl_certificate " "$conf_file" | head -1 | awk '{print $2}' | tr -d ';')
      local cert_path_cleaned=${cert_path#/etc/nginx/}

      if [[ ! -f "$cert_path_cleaned" ]] && [[ ! -f "ssl/${cert_path_cleaned#ssl/}" ]]; then
        log_error "SSL certificate not found: $cert_path"
        errors_found=true
      fi
    fi

    # Check for upstream references to non-existent services
    if grep -q "proxy_pass.*http://" "$conf_file" 2>/dev/null; then
      while IFS= read -r line; do
        local upstream=$(echo "$line" | sed -n 's/.*proxy_pass.*http:\/\/\([^:\/]*\).*/\1/p')
        if [[ -n "$upstream" ]] && [[ "$upstream" != "host.docker.internal" ]]; then
          # Check if service exists in docker-compose
          if [[ -f "docker-compose.yml" ]]; then
            if ! grep -q "^\s*${upstream}:" docker-compose.yml 2>/dev/null &&
              ! grep -q "^\s*${upstream}:" docker-compose.custom.yml 2>/dev/null; then
              log_warning "Upstream service '$upstream' referenced in $filename may not exist"
              warnings+=("$filename references potentially missing service: $upstream")
            fi
          fi
        fi
      done < <(grep "proxy_pass.*http://" "$conf_file")
    fi
  done

  # Display warnings if any
  if [[ ${#warnings[@]} -gt 0 ]]; then
    log_warning "Nginx configuration warnings found:"
    for warning in "${warnings[@]}"; do
      echo "  • $warning"
    done
  fi

  if [[ "$errors_found" == "true" ]]; then
    return 1
  fi

  return 0
}

# Fix rate limiting directives
nginx::fix_rate_limiting() {
  local conf_file="$1"
  local filename=$(basename "$conf_file")

  log_info "Removing rate limiting directives from $filename"

  # Remove rate limiting zone definitions (these belong in http context)
  sed -i.bak '/limit_req_zone/d' "$conf_file"

  # Comment out rate limiting usage (can be re-enabled when zones are properly defined)
  sed -i.bak 's/^\s*limit_req /    # limit_req /' "$conf_file"
  sed -i.bak 's/^\s*limit_req_status/    # limit_req_status/' "$conf_file"

  # Remove backup files
  rm -f "${conf_file}.bak"

  log_success "Fixed rate limiting configuration in $filename"
}

# Fix deprecated http2 directive
nginx::fix_http2_directive() {
  local conf_file="$1"
  local filename=$(basename "$conf_file")

  log_info "Updating deprecated http2 syntax in $filename"

  # Replace "listen 443 ssl http2;" with "listen 443 ssl;" followed by "http2 on;"
  # Use perl for more reliable multi-line replacement
  perl -i -pe 's/^(\s*listen\s+443\s+ssl)\s+http2\s*;/$1;\n$1;\n    http2 on;/g' "$conf_file"

  # Remove duplicate listen lines
  awk '!seen[$0]++' "$conf_file" >"${conf_file}.tmp" && mv "${conf_file}.tmp" "$conf_file"

  log_success "Updated http2 directive in $filename"
}

# Generate fallback nginx configuration
nginx::generate_fallback_config() {
  local config_dir="${1:-nginx/conf.d}"

  log_info "Generating fallback nginx configuration"

  mkdir -p "$config_dir"

  cat >"$config_dir/default-fallback.conf" <<'EOF'
# Fallback configuration - minimal working setup
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;

    location / {
        return 200 "nself nginx is running (fallback mode)\n";
        add_header Content-Type text/plain;
    }

    location /health {
        access_log off;
        return 200 "healthy\n";
        add_header Content-Type text/plain;
    }
}

# HTTPS fallback (if certificates exist)
server {
    listen 443 ssl default_server;
    http2 on;
    listen [::]:443 ssl default_server;
    server_name _;

    # Try to use any available certificate
    ssl_certificate /etc/nginx/ssl/localhost/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/localhost/privkey.pem;

    # Fallback to self-signed if needed
    ssl_certificate /etc/nginx/ssl/fallback/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/fallback/key.pem;

    location / {
        return 200 "nself nginx is running with SSL (fallback mode)\n";
        add_header Content-Type text/plain;
    }

    location /health {
        access_log off;
        return 200 "healthy\n";
        add_header Content-Type text/plain;
    }
}
EOF

  log_success "Fallback configuration created"
}

# Validate nginx configuration in container
nginx::validate_in_container() {
  local project="${PROJECT_NAME:-nself}"
  local container="${project}_nginx"

  # Check if container exists
  if ! docker ps -a --format '{{.Names}}' | grep -q "^${container}$"; then
    log_debug "Nginx container $container does not exist"
    return 0
  fi

  # Test configuration inside container
  log_info "Validating nginx configuration in container"

  if docker exec "$container" nginx -t 2>&1 | grep -q "syntax is ok"; then
    log_success "Nginx configuration is valid"
    return 0
  else
    log_error "Nginx configuration validation failed"

    # Show specific errors
    docker exec "$container" nginx -t 2>&1 | grep -E "emerg|error|warn" | while IFS= read -r line; do
      echo "  • $line"
    done

    return 1
  fi
}

# Auto-fix common nginx issues
nginx::auto_fix() {
  local config_dir="${1:-nginx/conf.d}"
  local fixed_issues=0

  log_info "Running nginx auto-fix"

  # Fix rate limiting issues
  if grep -r "limit_req_zone" "$config_dir" 2>/dev/null; then
    for conf_file in "$config_dir"/*.conf; do
      [[ -f "$conf_file" ]] || continue
      if grep -q "limit_req_zone" "$conf_file"; then
        nginx::fix_rate_limiting "$conf_file"
        ((fixed_issues++))
      fi
    done
  fi

  # Fix deprecated http2 syntax
  if grep -r "listen.*http2;" "$config_dir" 2>/dev/null; then
    for conf_file in "$config_dir"/*.conf; do
      [[ -f "$conf_file" ]] || continue
      if grep -q "listen.*http2;" "$conf_file"; then
        nginx::fix_http2_directive "$conf_file"
        ((fixed_issues++))
      fi
    done
  fi

  # Remove references to non-existent services
  if [[ -f "docker-compose.yml" ]]; then
    for conf_file in "$config_dir"/*.conf; do
      [[ -f "$conf_file" ]] || continue
      local filename=$(basename "$conf_file")

      # Check if this is a custom service config referencing non-existent containers
      if [[ "$filename" == "custom-services.conf" ]]; then
        # Check if custom services are actually defined
        if ! grep -q "^\s*actions:" docker-compose*.yml 2>/dev/null &&
          ! grep -q "^\s*realtime:" docker-compose*.yml 2>/dev/null; then
          log_warning "Disabling $filename - referenced services not found"
          mv "$conf_file" "${conf_file}.disabled"
          ((fixed_issues++))
        fi
      fi
    done
  fi

  if [[ $fixed_issues -gt 0 ]]; then
    log_success "Fixed $fixed_issues nginx configuration issues"
  else
    log_info "No nginx issues found to fix"
  fi

  return 0
}

# Check nginx health
nginx::health_check() {
  local project="${PROJECT_NAME:-nself}"
  local container="${project}_nginx"

  # Check if container is running
  if ! docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
    log_error "Nginx container is not running"
    return 1
  fi

  # Check if nginx process is running
  if ! docker exec "$container" pgrep nginx >/dev/null 2>&1; then
    log_error "Nginx process is not running in container"
    return 1
  fi

  # Check if nginx is responding
  if docker exec "$container" curl -f http://localhost/health >/dev/null 2>&1; then
    log_success "Nginx is healthy and responding"
    return 0
  else
    log_error "Nginx is not responding to health checks"
    return 1
  fi
}

# Export functions
export -f nginx::validate_config
export -f nginx::fix_rate_limiting
export -f nginx::fix_http2_directive
export -f nginx::generate_fallback_config
export -f nginx::validate_in_container
export -f nginx::auto_fix
export -f nginx::health_check
