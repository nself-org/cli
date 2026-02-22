#!/usr/bin/env bash
# nginx-generator.sh - Generate individual nginx configurations for all services

# Don't use strict mode for library files that get sourced
# set -euo pipefail

# Get the directory where this script is located
if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
  NGINX_GEN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  # Go up 3 levels: services -> lib -> src -> nself root (only if not already set)
  NSELF_ROOT="${NSELF_ROOT:-$(cd "$NGINX_GEN_DIR/../../.." && pwd)}"
else
  # Fallback - try to find nself root dynamically
  if command -v nself >/dev/null 2>&1; then
    NSELF_BIN="$(which nself)"
    if [[ -L "$NSELF_BIN" ]]; then
      NSELF_BIN="$(readlink -f "$NSELF_BIN" 2>/dev/null || readlink "$NSELF_BIN")"
    fi
    NSELF_ROOT="${NSELF_ROOT:-$(cd "$(dirname "$NSELF_BIN")/.." && pwd)}"
    NGINX_GEN_DIR="$NSELF_ROOT/src/lib/services"
  else
    # Last resort - use pwd
    NGINX_GEN_DIR="$(pwd)/src/lib/services"
    NSELF_ROOT="${NSELF_ROOT:-$(pwd)}"
  fi
fi

# Source dependencies
source "$NGINX_GEN_DIR/../utils/display.sh" 2>/dev/null || true
source "$NGINX_GEN_DIR/service-routes.sh" 2>/dev/null || true

# Get SSL certificate path for a domain
get_ssl_cert_path() {
  local domain="${1:-localhost}"

  # For api.*.localhost domains, use api-localhost certificates if they exist
  if [[ "$domain" == "api."*".localhost" ]] && [[ -d "ssl/certificates/api-localhost" ]]; then
    echo "/etc/nginx/ssl/api-localhost"
  # For api.localhost, use api-localhost certificates if they exist
  elif [[ "$domain" == "api.localhost" ]] && [[ -d "ssl/certificates/api-localhost" ]]; then
    echo "/etc/nginx/ssl/api-localhost"
  # For localhost and *.localhost domains, use localhost certificates
  elif [[ "$domain" == "localhost" ]] || [[ "$domain" == *".localhost" ]]; then
    echo "/etc/nginx/ssl/localhost"
  # For local.nself.org and *.local.nself.org, use nself-org certificates
  elif [[ "$domain" == *"local.nself.org" ]]; then
    echo "/etc/nginx/ssl/nself-org"
  # For custom domains (production/staging), use custom certificates
  elif [[ "$domain" != *".localhost" ]]; then
    echo "/etc/nginx/ssl/custom"
  # Default to localhost for development
  else
    echo "/etc/nginx/ssl/localhost"
  fi
}

# Generate nginx config for a backend service
nginx::generate_service_config() {
  local service_name="$1"
  local output_dir="$2"
  local base_domain="${BASE_DOMAIN:-localhost}"

  # Get service configuration
  local config
  config=$(routes::get_service_config "$service_name")
  [[ -z "$config" ]] && return 1

  # Parse configuration
  local route container_name internal_port upstream_name
  local needs_websocket="false"
  local max_body_size=""

  while IFS='=' read -r key value; do
    case "$key" in
      route) route="$value" ;;
      container_name) container_name="$value" ;;
      internal_port) internal_port="$value" ;;
      upstream_name) upstream_name="$value" ;;
      needs_websocket) needs_websocket="$value" ;;
      max_body_size) max_body_size="$value" ;;
    esac
  done <<<"$config"

  # Skip if essential info is missing
  [[ -z "$route" || -z "$container_name" || -z "$internal_port" ]] && return 1

  # Set default upstream_name if not provided
  [[ -z "$upstream_name" ]] && upstream_name="${container_name//-/_}"

  # Determine SSL certificate path based on domain
  local ssl_path
  ssl_path=$(get_ssl_cert_path "$route")

  # Generate the nginx configuration
  cat >"$output_dir/${service_name}.conf" <<EOF
# ${service_name} service proxy configuration
upstream ${upstream_name} {
    server ${container_name}:${internal_port};
}

server {
    listen 80;
    server_name ${route};
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name ${route};
    
    ssl_certificate ${ssl_path}/fullchain.pem;
    ssl_certificate_key ${ssl_path}/privkey.pem;
EOF

  # Add max body size if specified
  if [[ -n "$max_body_size" ]]; then
    echo "    client_max_body_size ${max_body_size};" >>"$output_dir/${service_name}.conf"
  fi

  # Add location block with service-specific settings
  cat >>"$output_dir/${service_name}.conf" <<EOF
    
    location / {
        proxy_pass http://${upstream_name};
        proxy_http_version 1.1;
EOF

  # Add WebSocket support if needed
  if [[ "$needs_websocket" == "true" ]]; then
    cat >>"$output_dir/${service_name}.conf" <<'EOF'
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
EOF
  fi

  # Add common proxy headers
  cat >>"$output_dir/${service_name}.conf" <<'EOF'
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header User-Agent $http_user_agent;
        proxy_pass_request_headers on;

        # Security headers
        proxy_set_header X-Forwarded-Host $server_name;
        proxy_set_header X-Forwarded-Port $server_port;
    }
    
    # Health check endpoint
    location /health {
        access_log off;
        proxy_pass http://${upstream_name}/health;
        proxy_connect_timeout 1s;
        proxy_send_timeout 1s;
        proxy_read_timeout 1s;
    }
}
EOF

  log_info "Generated nginx config for $service_name at $route"
  return 0
}

# Generate nginx config for frontend applications
nginx::generate_frontend_config() {
  local app_name="$1"
  local route="$2"
  local port="$3"
  local output_dir="$4"

  # Determine SSL certificate path
  local ssl_path
  ssl_path=$(get_ssl_cert_path "$route")

  # Generate the nginx configuration
  cat >"$output_dir/${app_name}.conf" <<EOF
# Frontend application: ${app_name}
server {
    listen 80;
    server_name ${route};
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name ${route};
    
    ssl_certificate ${ssl_path}/fullchain.pem;
    ssl_certificate_key ${ssl_path}/privkey.pem;
    
    # Frontend app: ${app_name}
    # Development port: ${port}
    
    location / {
        proxy_pass http://host.docker.internal:${port};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header User-Agent \$http_user_agent;
        proxy_pass_request_headers on;

        # CORS headers for frontend apps
        add_header Access-Control-Allow-Origin \$http_origin always;
        add_header Access-Control-Allow-Credentials true always;
    }
    
    # Health check endpoint
    location /health {
        access_log off;
        return 200 "OK";
        add_header Content-Type text/plain;
    }
    
    # Static assets optimization
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        expires 1y;
        add_header Cache-Control "public, immutable";
        proxy_pass http://host.docker.internal:${port};
    }
}
EOF

  log_info "Generated nginx config for frontend app $app_name at $route"
  return 0
}

# Generate nginx config for frontend app GraphQL API endpoints
nginx::generate_frontend_auth_config() {
  local app_name="$1"
  local auth_route="$2"
  local output_dir="$3"

  # Determine SSL certificate path
  local ssl_path
  ssl_path=$(get_ssl_cert_path "$auth_route")

  # Default to localhost cert if not found
  if [[ -z "$ssl_path" ]] || [[ ! -f "$ssl_path/fullchain.pem" ]]; then
    ssl_path="/etc/nginx/ssl/localhost"
  fi

  # Generate the nginx configuration for per-app auth endpoint
  # Use app prefix to avoid conflicts
  local config_name="${app_name}-auth"
  cat >"$output_dir/${config_name}.conf" <<EOF
# Per-app auth endpoint for ${app_name}
server {
    listen 80;
    server_name ${auth_route};
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name ${auth_route};

    ssl_certificate ${ssl_path}/fullchain.pem;
    ssl_certificate_key ${ssl_path}/privkey.pem;

    # Auth proxy for app: ${app_name}
    # Routes to shared auth service but preserves Host header

    location / {
        proxy_pass http://auth:4000;
        proxy_http_version 1.1;

        # Critical: preserve Host header for per-app behavior
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-Host \$host;

        # WebSocket support for auth
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";

        # CORS headers for auth service
        # In development, allow all origins with credentials for flexibility
        # In production, this should be restricted to specific origins
        if (\$http_origin ~* "^https?://(localhost|127\.0\.0\.1|.*\.localhost|.*\.${BASE_DOMAIN})(:[0-9]+)?$") {
            add_header Access-Control-Allow-Origin \$http_origin always;
            add_header Access-Control-Allow-Credentials true always;
        }
        add_header Access-Control-Allow-Methods "GET, POST, PUT, DELETE, OPTIONS" always;
        add_header Access-Control-Allow-Headers "Content-Type, Authorization, X-Requested-With" always;
        add_header Access-Control-Max-Age 86400 always;

        # Handle preflight requests
        if (\$request_method = 'OPTIONS') {
            return 204;
        }
    }

    # Health check endpoint
    location /health {
        access_log off;
        proxy_pass http://auth:4000/healthz;
    }
}
EOF

  return 0
}

nginx::generate_frontend_api_config() {
  local app_name="$1"
  local api_route="$2"
  local port="$3"
  local output_dir="$4"

  # Determine SSL certificate path
  local ssl_path
  ssl_path=$(get_ssl_cert_path "$api_route")

  # Generate the nginx configuration for API endpoint
  cat >"$output_dir/${app_name}-api.conf" <<EOF
# Frontend API endpoint: ${app_name}
server {
    listen 80;
    server_name ${api_route};
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name ${api_route};

    ssl_certificate ${ssl_path}/fullchain.pem;
    ssl_certificate_key ${ssl_path}/privkey.pem;

    # GraphQL API endpoint for: ${app_name}
    # Proxies to shared Hasura instance with app context

    location / {
        # Proxy to shared Hasura instance
        proxy_pass http://hasura:8080;
        proxy_http_version 1.1;

        # Preserve Host header for per-app context
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-Host \$host;

        # WebSocket support for subscriptions
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";

        # Add app context header for multi-tenant support
        proxy_set_header X-App-Name "${app_name}";
        proxy_set_header X-App-Route "${api_route}";

        # CORS headers for GraphQL
        add_header Access-Control-Allow-Origin \$http_origin always;
        add_header Access-Control-Allow-Methods "GET, POST, OPTIONS" always;
        add_header Access-Control-Allow-Headers "Content-Type, Authorization, x-hasura-admin-secret" always;
        add_header Access-Control-Allow-Credentials true always;

        # Handle preflight requests
        if (\$request_method = 'OPTIONS') {
            return 204;
        }
    }

    # Health check endpoint
    location /health {
        access_log off;
        proxy_pass http://hasura/healthz;
    }
}
EOF

  log_info "Generated nginx API config for $app_name at $api_route"
  return 0
}

# Generate nginx config for custom services
nginx::generate_custom_service_config() {
  local service_name="$1"
  local service_type="$2"
  local route="$3"
  local port="$4"
  local container_name="$5"
  local output_dir="$6"

  # Determine SSL certificate path
  local ssl_path
  ssl_path=$(get_ssl_cert_path "$route")

  # Create upstream name (replace hyphens with underscores for nginx)
  local upstream_name="${service_name//-/_}"

  # Generate the nginx configuration
  cat >"$output_dir/cs-${service_name}.conf" <<EOF
# Custom service: ${service_name} (${service_type})
upstream ${upstream_name} {
    server ${container_name}:${port};
}

server {
    listen 80;
    server_name ${route};
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name ${route};
    
    ssl_certificate ${ssl_path}/fullchain.pem;
    ssl_certificate_key ${ssl_path}/privkey.pem;
    
    # Custom ${service_type} service: ${service_name}
    
    location / {
        proxy_pass http://${upstream_name};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
    
    # Health check endpoint
    location /health {
        access_log off;
        return 200 "OK - ${service_name}";
        add_header Content-Type text/plain;
    }
}
EOF

  log_info "Generated nginx config for custom service $service_name ($service_type) at $route"
  return 0
}

# Remove wildcard catch-all configs that conflict with specific routing
nginx::remove_wildcard_conflicts() {
  local nginx_dir="$1"

  # Check for problematic wildcard configs
  local conflicts=()

  if [[ -f "$nginx_dir/conf.d/ssl-localhost.conf" ]]; then
    if grep -q "server_name \*\.localhost" "$nginx_dir/conf.d/ssl-localhost.conf" 2>/dev/null; then
      conflicts+=("ssl-localhost.conf")
    fi
  fi

  if [[ -f "$nginx_dir/conf.d/ssl-local-nself-org.conf" ]]; then
    if grep -q "server_name \*\.local\.nself\.org" "$nginx_dir/conf.d/ssl-local-nself-org.conf" 2>/dev/null; then
      conflicts+=("ssl-local-nself-org.conf")
    fi
  fi

  # Disable conflicting configs by renaming them
  if [[ ${#conflicts[@]} -gt 0 ]]; then
    for conflict in "${conflicts[@]}"; do
      if [[ -f "$nginx_dir/conf.d/$conflict" ]]; then
        mv "$nginx_dir/conf.d/$conflict" "$nginx_dir/conf.d/$conflict.disabled"
        log_warning "Disabled conflicting wildcard config: $conflict"
      fi
    done
  fi
}

# Generate all nginx configurations
nginx::generate_all_configs() {
  local project_dir="${1:-.}"
  local nginx_dir="$project_dir/nginx"
  local conf_dir="$nginx_dir/conf.d"

  # Create nginx directories
  mkdir -p "$conf_dir"

  # Remove conflicting wildcard configs
  nginx::remove_wildcard_conflicts "$nginx_dir"

  local configs_generated=0

  # Generate configs for enabled backend services
  while IFS= read -r service; do
    if nginx::generate_service_config "$service" "$conf_dir"; then
      configs_generated=$((configs_generated + 1))
    fi
  done < <(routes::get_enabled_services)

  # Generate configs for frontend applications
  local current_app=""
  local api_route=""
  while IFS= read -r line; do
    if [[ "$line" == "---" ]]; then
      current_app=""
      api_route=""
      continue
    fi

    IFS='=' read -r key value <<<"$line"
    case "$key" in
      app_name) current_app="$value" ;;
      route) app_route="$value" ;;
      api_route) api_route="$value" ;;
      port)
        app_port="$value"
        if [[ -n "$current_app" ]]; then
          # Generate main frontend config
          if nginx::generate_frontend_config "$current_app" "$app_route" "$app_port" "$conf_dir"; then
            configs_generated=$((configs_generated + 1))
          fi

          # Generate API config if remote schema URL is defined
          if [[ -n "$api_route" ]]; then
            if nginx::generate_frontend_api_config "$current_app" "$api_route" "$app_port" "$conf_dir"; then
              configs_generated=$((configs_generated + 1))
            fi

            # Generate per-app auth config ONLY if this app has a remote schema
            # Extract base domain from app route (e.g., app1.localhost -> auth.app1.localhost)
            local app_domain="${app_route#*.}"  # Get everything after first dot
            local app_prefix="${app_route%%.*}" # Get everything before first dot
            local auth_route="auth.${app_prefix}.${app_domain}"

            # Ensure we don't duplicate if it's already the main auth route
            if [[ "$auth_route" != "${AUTH_ROUTE:-auth.localhost}" ]]; then
              if nginx::generate_frontend_auth_config "$current_app" "$auth_route" "$conf_dir"; then
                configs_generated=$((configs_generated + 1))
              fi
            fi
          fi
        fi
        ;;
    esac
  done < <(routes::get_frontend_apps)

  # Generate configs for custom services
  local current_app=""
  local cs_name="" cs_type="" cs_route="" cs_port="" cs_container=""
  while IFS= read -r line; do
    if [[ "$line" == "---" ]]; then
      if [[ -n "$cs_name" && -n "$cs_type" && -n "$cs_route" && -n "$cs_port" && -n "$cs_container" ]]; then
        if nginx::generate_custom_service_config "$cs_name" "$cs_type" "$cs_route" "$cs_port" "$cs_container" "$conf_dir"; then
          configs_generated=$((configs_generated + 1))
        fi
      fi
      cs_name="" cs_type="" cs_route="" cs_port="" cs_container=""
      continue
    fi

    IFS='=' read -r key value <<<"$line"
    case "$key" in
      service_name) cs_name="$value" ;;
      service_type) cs_type="$value" ;;
      route) cs_route="$value" ;;
      internal_port) cs_port="$value" ;;
      container_name) cs_container="$value" ;;
    esac
  done < <(routes::get_custom_services)

  # Handle last custom service if not ended with ---
  if [[ -n "$cs_name" && -n "$cs_type" && -n "$cs_route" && -n "$cs_port" && -n "$cs_container" ]]; then
    if nginx::generate_custom_service_config "$cs_name" "$cs_type" "$cs_route" "$cs_port" "$cs_container" "$conf_dir"; then
      configs_generated=$((configs_generated + 1))
    fi
  fi

  echo $configs_generated
  return 0
}

# Validate nginx configuration
nginx::validate_config() {
  local nginx_dir="${1:-.}/nginx"

  # Test nginx config using docker
  if command -v docker >/dev/null 2>&1; then
    # Use a simple syntax check that doesn't require full nginx setup
    if docker run --rm -v "$nginx_dir:/tmp/nginx:ro" nginx:alpine sh -c 'nginx -t -c /tmp/nginx/nginx.conf -p /tmp' 2>/dev/null; then
      return 0
    else
      # Try a more lenient validation - check each conf file individually
      local has_errors=false

      for conf_file in "$nginx_dir"/conf.d/*.conf; do
        [[ ! -f "$conf_file" ]] && continue

        if ! docker run --rm -v "$conf_file:/tmp/test.conf:ro" nginx:alpine sh -c 'nginx -t -c /tmp/test.conf -p /tmp' 2>/dev/null; then
          log_warning "Potential issue in $(basename "$conf_file")"
          has_errors=true
        fi
      done

      [[ "$has_errors" == "true" ]] && return 1 || return 0
    fi
  else
    log_warning "Docker not available - skipping nginx config validation"
    return 0
  fi
}

# Export functions
export -f nginx::generate_service_config
export -f nginx::generate_frontend_config
export -f nginx::generate_frontend_auth_config
export -f nginx::generate_frontend_api_config
export -f nginx::generate_custom_service_config
export -f nginx::remove_wildcard_conflicts
export -f nginx::generate_all_configs
export -f nginx::validate_config
