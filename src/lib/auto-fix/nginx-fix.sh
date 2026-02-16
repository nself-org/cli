#!/usr/bin/env bash

# nginx-fix.sh - Auto-fix nginx configuration issues

# Fix nginx restart loop
fix_nginx_restart_loop() {

set -euo pipefail

  local container_name="${1:-${PROJECT_NAME}_nginx}"
  local verbose="${2:-false}"

  [[ "$verbose" == "true" ]] && log_info "Analyzing nginx restart loop..."

  # Get the error from nginx logs
  local nginx_error=$(docker logs "$container_name" 2>&1 | grep -E "emerg|error" | tail -5)

  if [[ -z "$nginx_error" ]]; then
    [[ "$verbose" == "true" ]] && log_warning "No specific nginx error found"
    return 1
  fi

  local fixed=false

  # Fix: Rate limiting directive in wrong context
  if echo "$nginx_error" | grep -q "limit_req_zone.*directive is not allowed here"; then
    log_info "Fixing rate limiting configuration..."

    # Find and fix the problematic config files
    for conf_file in nginx/conf.d/*.conf; do
      [[ -f "$conf_file" ]] || continue

      if grep -q "limit_req_zone" "$conf_file" 2>/dev/null; then
        # Remove limit_req_zone directives (they belong in http context)
        sed -i.bak '/limit_req_zone/d' "$conf_file"
        sed -i.bak 's/^\s*limit_req /    # limit_req /' "$conf_file"
        sed -i.bak 's/^\s*limit_req_status/    # limit_req_status/' "$conf_file"
        rm -f "${conf_file}.bak"

        log_success "Fixed rate limiting in $(basename "$conf_file")"
        fixed=true
      fi
    done
  fi

  # Fix: Upstream host not found
  if echo "$nginx_error" | grep -q "host not found in upstream"; then
    # Extract the missing service name
    local missing_service=$(echo "$nginx_error" | sed -n 's/.*host not found in upstream "\([^"]*\)".*/\1/p' | head -1)

    if [[ -n "$missing_service" ]]; then
      log_info "Service '$missing_service' not found, checking configuration..."

      # Special handling for storage service - wait for it to be ready
      if [[ "$missing_service" == "storage:5001" ]] || [[ "$missing_service" == "storage" ]]; then
        log_info "Storage service not ready, waiting for startup..."

        # Wait up to 30 seconds for storage service to be ready
        local waited=0
        local max_wait=30
        while [[ $waited -lt $max_wait ]]; do
          if docker ps --format '{{.Names}}' | grep -q "${PROJECT_NAME}_storage"; then
            # Check if storage container is actually running (not restarting)
            local storage_status=$(docker ps --filter "name=${PROJECT_NAME}_storage" --format '{{.Status}}')
            if ! echo "$storage_status" | grep -q "Restarting"; then
              log_success "Storage service is now running"
              fixed=true
              break
            fi
          fi
          sleep 2
          waited=$((waited + 2))
        done

        if [[ "$fixed" != "true" ]]; then
          log_warning "Storage service still not ready after ${max_wait}s"
        fi
      else
        # Find config files referencing this service
        for conf_file in nginx/conf.d/*.conf; do
          [[ -f "$conf_file" ]] || continue

          if grep -q "proxy_pass.*$missing_service" "$conf_file" 2>/dev/null; then
            local filename=$(basename "$conf_file")

            # Check if this is a custom service that's not running
            if [[ "$filename" == "custom-services.conf" ]]; then
              log_warning "Disabling $filename - referenced services not running"
              mv "$conf_file" "${conf_file}.disabled"
              fixed=true
            else
              # Comment out the problematic proxy_pass
              sed -i.bak "s|proxy_pass.*$missing_service|# & # Service not found|" "$conf_file"
              rm -f "${conf_file}.bak"
              log_warning "Commented out references to '$missing_service' in $filename"
              fixed=true
            fi
          fi
        done
      fi
    fi
  fi

  # Fix: SSL certificate missing
  if echo "$nginx_error" | grep -q "cannot load certificate.*No such file or directory"; then
    local missing_cert=$(echo "$nginx_error" | sed -n 's/.*cannot load certificate "\([^"]*\)".*/\1/p' | head -1)

    if [[ -n "$missing_cert" ]]; then
      log_warning "SSL certificate missing: $missing_cert"

      # Check if we can use a fallback certificate
      local cert_dir=$(dirname "$missing_cert")
      local fallback_cert="/etc/nginx/ssl/localhost/fullchain.pem"

      # Update configs to use fallback certificate
      for conf_file in nginx/conf.d/*.conf; do
        [[ -f "$conf_file" ]] || continue

        if grep -q "$missing_cert" "$conf_file" 2>/dev/null; then
          sed -i.bak "s|$missing_cert|$fallback_cert|g" "$conf_file"
          sed -i.bak "s|${missing_cert%.pem}.key|/etc/nginx/ssl/localhost/privkey.pem|g" "$conf_file"
          rm -f "${conf_file}.bak"

          log_info "Updated $(basename "$conf_file") to use fallback certificate"
          fixed=true
        fi
      done
    fi
  fi

  # Fix: Deprecated http2 directive
  if echo "$nginx_error" | grep -q 'the "listen ... http2" directive is deprecated'; then
    log_info "Fixing deprecated http2 syntax..."

    for conf_file in nginx/conf.d/*.conf; do
      [[ -f "$conf_file" ]] || continue

      if grep -q "listen.*http2;" "$conf_file" 2>/dev/null; then
        # Replace deprecated syntax
        perl -i -pe 's/^(\s*listen\s+443\s+ssl)\s+http2\s*;/$1;\n    http2 on;/g' "$conf_file"
        log_success "Fixed http2 syntax in $(basename "$conf_file")"
        fixed=true
      fi
    done
  fi

  if [[ "$fixed" == "true" ]]; then
    # Restart nginx to apply fixes
    log_info "Restarting nginx with fixed configuration..."
    docker restart "$container_name" >/dev/null 2>&1

    # Wait and check if it's running
    sleep 3
    if docker ps --format '{{.Names}}' | grep -q "^${container_name}$"; then
      # Check if still restarting
      local status=$(docker ps --filter "name=${container_name}" --format '{{.Status}}')
      if echo "$status" | grep -q "Restarting"; then
        log_error "Nginx still restarting after fix attempt"
        return 1
      else
        log_success "Nginx is now running successfully"
        return 0
      fi
    else
      log_error "Nginx failed to start after fix"
      return 1
    fi
  else
    [[ "$verbose" == "true" ]] && log_warning "No automatic fix available for this nginx error"
    return 1
  fi
}

# Generate minimal fallback nginx config
generate_nginx_fallback() {
  local config_dir="${1:-nginx/conf.d}"

  log_info "Generating fallback nginx configuration..."

  # Backup existing configs
  if [[ -d "$config_dir" ]]; then
    mkdir -p "$config_dir/.backup"
    cp "$config_dir"/*.conf "$config_dir/.backup/" 2>/dev/null || true
  else
    mkdir -p "$config_dir"
  fi

  # Create minimal working config
  cat >"$config_dir/00-fallback.conf" <<'EOF'
# Minimal fallback configuration
server {
    listen 80 default_server;
    server_name _;

    location / {
        return 200 "nself nginx is running (fallback mode)\n";
        add_header Content-Type text/plain;
    }

    location /health {
        access_log off;
        return 200 "OK\n";
        add_header Content-Type text/plain;
    }
}
EOF

  # Disable all other configs temporarily
  for conf_file in "$config_dir"/*.conf; do
    [[ -f "$conf_file" ]] || continue
    [[ "$(basename "$conf_file")" == "00-fallback.conf" ]] && continue

    mv "$conf_file" "${conf_file}.disabled" 2>/dev/null || true
  done

  log_success "Fallback configuration activated"
  log_info "Run 'nself build' to regenerate full configuration"
}

# Export functions
export -f fix_nginx_restart_loop
export -f generate_nginx_fallback
