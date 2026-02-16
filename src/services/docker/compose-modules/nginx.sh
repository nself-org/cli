#!/usr/bin/env bash
set -euo pipefail
# nginx.sh - Nginx service generation for docker-compose
# Generates environment-aware nginx configuration

generate_nginx_service() {
  # Use smart defaults
  local nginx_port="${NGINX_PORT:-80}"
  local nginx_ssl_port="${NGINX_SSL_PORT:-443}"
  local base_domain="${BASE_DOMAIN:-localhost}"

  cat <<EOF
  nginx:
    image: nginx:alpine
    container_name: \${PROJECT_NAME:-myproject}_nginx
    restart: unless-stopped
    ports:
      - "\${NGINX_PORT:-80}:80"
      - "\${NGINX_SSL_PORT:-443}:443"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/conf.d:/etc/nginx/conf.d:ro
      - ./nginx/sites:/etc/nginx/sites:ro
      - ./nginx/includes:/etc/nginx/includes:ro
      - ./ssl/certificates:/etc/nginx/ssl:ro
      - nginx_cache:/var/cache/nginx
    environment:
      - BASE_DOMAIN=\${BASE_DOMAIN:-localhost}
      - PROJECT_NAME=\${PROJECT_NAME:-myproject}
      - ENV=\${ENV:-dev}
    networks:
      - \${PROJECT_NAME:-myproject}_network
    depends_on:
EOF

  # Add dependencies based on enabled services
  if [[ "${HASURA_ENABLED:-true}" == "true" ]]; then
    echo "      - hasura"
  fi

  if [[ "${AUTH_ENABLED:-true}" == "true" ]]; then
    echo "      - auth"
  fi

  # Add custom service dependencies
  local i=1
  while [[ $i -le 10 ]]; do
    local service_var="CS_${i}"
    if [[ -n "${!service_var:-}" ]]; then
      local service_name=$(echo "${!service_var}" | cut -d: -f1)
      echo "      - ${service_name}"
    fi
    i=$((i + 1))
  done

  cat <<EOF
    healthcheck:
      test: ["CMD-SHELL", "wget --no-verbose --tries=1 -O /dev/null http://127.0.0.1/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
EOF
}

# Generate environment-aware server blocks
generate_nginx_server_blocks() {
  local current_env="${ENV:-dev}"
  local base_domain="${BASE_DOMAIN:-localhost}"

  # Get domain for current environment
  if command -v get_domain_for_env >/dev/null 2>&1; then
    base_domain=$(get_domain_for_env "$current_env")
  elif [[ "$current_env" == "prod" ]]; then
    base_domain="${PROD_DOMAIN:-${PRODUCTION_DOMAIN:-${BASE_DOMAIN:-localhost}}}"
  elif [[ "$current_env" == "staging" ]]; then
    base_domain="${STAGING_DOMAIN:-${BASE_DOMAIN:-localhost}}"
  fi

  # Generate server blocks for each service with environment-specific domains
  generate_service_server_blocks "$base_domain" "$current_env"
}

# Generate server blocks for services
generate_service_server_blocks() {
  local domain="$1"
  local env="$2"

  # Required services (always enabled)
  if [[ "${HASURA_ENABLED:-true}" == "true" ]]; then
    local api_route="${HASURA_ROUTE:-api.$domain}"
    generate_upstream_server_block "hasura" "$api_route" "8080"
  fi

  if [[ "${AUTH_ENABLED:-true}" == "true" ]]; then
    local auth_route="${AUTH_ROUTE:-auth.$domain}"
    generate_upstream_server_block "auth" "$auth_route" "4000"
  fi

  # Optional services
  if [[ "${MINIO_ENABLED:-false}" == "true" ]]; then
    local minio_route="${MINIO_ROUTE:-minio.$domain}"
    generate_upstream_server_block "minio" "$minio_route" "9001"
  fi

  if [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]]; then
    local admin_route="${NSELF_ADMIN_ROUTE:-admin.$domain}"
    generate_upstream_server_block "nself-admin" "$admin_route" "3021"
  fi

  # Custom services
  local i=1
  while [[ $i -le 10 ]]; do
    local service_var="CS_${i}"
    if [[ -n "${!service_var}" ]]; then
      local service_info="${!service_var}"
      local service_name=$(echo "$service_info" | cut -d: -f1)
      local service_port=$(echo "$service_info" | cut -d: -f3)
      local service_route_var="CS_${i}_ROUTE"
      local service_route="${!service_route_var:-${service_name}.$domain}"

      generate_upstream_server_block "$service_name" "$service_route" "$service_port"
    fi
    i=$((i + 1))
  done

  # Frontend apps (external)
  generate_frontend_server_blocks "$domain" "$env"
}

# Generate upstream server block
generate_upstream_server_block() {
  local service_name="$1"
  local route="$2"
  local port="$3"

  cat > "nginx/routes/${service_name}.conf" <<EOF
# ${service_name} service
server {
    listen 80;
    server_name ${route};

    location / {
        proxy_pass http://${service_name}:${port};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF
}

# Generate frontend server blocks (for external apps)
generate_frontend_server_blocks() {
  local domain="$1"
  local env="$2"

  local i=1
  while [[ $i -le 10 ]]; do
    local name_var="FRONTEND_APP_${i}_NAME"
    local port_var="FRONTEND_APP_${i}_PORT"
    local route_var="FRONTEND_APP_${i}_ROUTE"

    if [[ -n "${!name_var}" ]]; then
      local app_name="${!name_var}"
      local app_port="${!port_var:-3000}"
      local app_route="${!route_var:-${app_name}.$domain}"

      cat > "nginx/routes/frontend-${app_name}.conf" <<EOF
# Frontend app: ${app_name}
server {
    listen 80;
    server_name ${app_route};

    location / {
        proxy_pass http://host.docker.internal:${app_port};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF
    fi
    i=$((i + 1))
  done
}

# Export functions
export -f generate_nginx_service
export -f generate_nginx_server_blocks
export -f generate_service_server_blocks
export -f generate_upstream_server_block
export -f generate_frontend_server_blocks