#!/usr/bin/env bash

# nginx.sh - Nginx configuration generation for build

# Generate nginx configuration
generate_nginx_config() {

set -euo pipefail

  local force="${1:-false}"

  # Check if nginx config already exists
  if [[ "$force" != "true" ]] && [[ -f "nginx/nginx.conf" ]]; then
    show_info "Nginx configuration already exists (use --force to regenerate)"
    return 0
  fi

  # Create nginx directories
  mkdir -p nginx/{conf.d,routes,ssl,includes,sites} 2>/dev/null || true
  mkdir -p "nginx/conf.d-${ENV:-dev}" 2>/dev/null || true

  # Generate main nginx.conf
  generate_main_nginx_conf

  # Generate default server configuration
  generate_default_server_conf

  # Generate service routes
  generate_service_routes

  # Generate SSL configuration
  generate_ssl_config

  # Generate security headers configuration
  generate_security_headers_config

  return 0
}

# Generate main nginx.conf
generate_main_nginx_conf() {
  cat >nginx/nginx.conf <<'EOF'
user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 2048;
    multi_accept on;
    use epoll;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    # Logging
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for"';

    access_log /var/log/nginx/access.log main;

    # Performance optimizations
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    client_max_body_size 100M;

    # Gzip compression
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_types text/plain text/css text/xml text/javascript
               application/json application/javascript application/xml+rss
               application/rss+xml application/atom+xml image/svg+xml
               text/x-js text/x-cross-domain-policy application/x-font-ttf
               application/x-font-opentype application/vnd.ms-fontobject
               image/x-icon;

    # Rate limiting zones
    limit_req_zone $binary_remote_addr zone=general:10m rate=10r/s;
    limit_req_zone $binary_remote_addr zone=auth:10m rate=5r/s;
    limit_req_zone $binary_remote_addr zone=api:10m rate=30r/s;

    # Cache zones
    proxy_cache_path /var/cache/nginx/cache levels=1:2 keys_zone=cache:10m
                     max_size=1g inactive=60m use_temp_path=off;

    # Include server configurations
    include /etc/nginx/conf.d/*.conf;
}
EOF
}

# Generate default server configuration
generate_default_server_conf() {
  # Use smart defaults that work for any environment
  local base_domain="${BASE_DOMAIN:-localhost}"
  local ssl_enabled="${SSL_ENABLED:-true}"

  # Detect environment for route generation
  local current_env="${ENV:-dev}"
  if command -v detect_environment >/dev/null 2>&1; then
    current_env="$(detect_environment)"
  fi

  cat >nginx/conf.d/default.conf <<EOF
# Default server configuration
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name ${base_domain} *.${base_domain};

EOF

  # Add SSL redirect if enabled
  if [[ "$ssl_enabled" == "true" ]]; then
    cat >>nginx/conf.d/default.conf <<'EOF'
    # Redirect to HTTPS
    location / {
        return 301 https://$host$request_uri;
    }

    # Allow ACME challenges for Let's Encrypt
    location ^~ /.well-known/acme-challenge/ {
        allow all;
        root /var/www/certbot;
    }
}

# SSL server configuration
server {
    listen 443 ssl default_server;
    listen [::]:443 ssl default_server;
    http2 on;
EOF
    echo "    server_name ${base_domain} *.${base_domain};" >>nginx/conf.d/default.conf
    cat >>nginx/conf.d/default.conf <<'EOF'

    # SSL configuration
    include /etc/nginx/includes/ssl.conf;

    # Security headers
    include /etc/nginx/includes/security-headers.conf;

    # Root location
    location / {
        root /usr/share/nginx/html;
        index index.html;
        try_files $uri $uri/ /index.html;
    }

    # Health check endpoint
    location /health {
        access_log off;
        return 200 "OK\n";
        add_header Content-Type text/plain;
    }

    # Include service routes
    include /etc/nginx/sites/*.conf;
}
EOF
  else
    # Non-SSL configuration
    cat >>nginx/conf.d/default.conf <<'EOF'
    # Root location
    location / {
        root /usr/share/nginx/html;
        index index.html;
        try_files $uri $uri/ /index.html;
    }

    # Health check endpoint
    location /health {
        access_log off;
        return 200 "OK\n";
        add_header Content-Type text/plain;
    }

    # Include service routes
    include /etc/nginx/sites/*.conf;
}
EOF
  fi
}

# Generate SSL configuration
generate_ssl_config() {
  local base_domain="${BASE_DOMAIN:-localhost}"
  local ssl_dir="localhost"

  if [[ "$base_domain" != "localhost" ]]; then
    ssl_dir="nself-org"
  fi

  cat >nginx/includes/ssl.conf <<EOF
# SSL certificates
ssl_certificate /etc/nginx/ssl/${ssl_dir}/fullchain.pem;
ssl_certificate_key /etc/nginx/ssl/${ssl_dir}/privkey.pem;

# SSL protocols and ciphers
ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers HIGH:!aNULL:!MD5;
ssl_prefer_server_ciphers off;

# SSL session settings
ssl_session_cache shared:SSL:10m;
ssl_session_timeout 10m;
ssl_session_tickets off;

# OCSP stapling
ssl_stapling on;
ssl_stapling_verify on;
resolver 8.8.8.8 8.8.4.4 valid=300s;
resolver_timeout 5s;
EOF
}

# Generate security headers configuration
generate_security_headers_config() {
  local csp_mode="${CSP_MODE:-strict}"
  local ssl_enabled="${SSL_ENABLED:-true}"
  local custom_domains="${CSP_CUSTOM_DOMAINS:-}"

  # Source security headers library if available
  if [[ -f "src/lib/security/headers.sh" ]]; then
    source src/lib/security/headers.sh 2>/dev/null || true
    headers::export_nginx "nginx/includes/security-headers.conf" "$ssl_enabled"
  else
    # Fallback: generate basic security headers manually
    generate_basic_security_headers "$ssl_enabled"
  fi
}

# Generate basic security headers (fallback)
generate_basic_security_headers() {
  local ssl_enabled="${1:-true}"

  cat >nginx/includes/security-headers.conf <<'EOF'
# Security Headers Configuration
# Basic security headers - for advanced configuration, use nself security headers

# Content Security Policy (CSP)
# SECURITY: Strict CSP by default - no unsafe-inline or unsafe-eval
# To allow unsafe-inline/unsafe-eval, set CSP_MODE=moderate in your .env file
# and run: nself build
add_header Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; media-src 'self'; object-src 'none'; frame-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; upgrade-insecure-requests" always;

EOF

  # Add HSTS only if SSL is enabled
  if [[ "$ssl_enabled" == "true" ]]; then
    cat >>nginx/includes/security-headers.conf <<'EOF'
# HTTP Strict Transport Security (HSTS)
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

EOF
  fi

  cat >>nginx/includes/security-headers.conf <<'EOF'
# X-Frame-Options
add_header X-Frame-Options "DENY" always;

# X-Content-Type-Options
add_header X-Content-Type-Options "nosniff" always;

# X-XSS-Protection (legacy)
add_header X-XSS-Protection "1; mode=block" always;

# Referrer-Policy
add_header Referrer-Policy "strict-origin-when-cross-origin" always;

# Permissions-Policy
add_header Permissions-Policy "camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()" always;

# X-Permitted-Cross-Domain-Policies
add_header X-Permitted-Cross-Domain-Policies "none" always;
EOF
}

# Generate service routes
generate_service_routes() {
  # Hasura route
  if [[ "${HASURA_ENABLED:-false}" == "true" ]]; then
    generate_hasura_route
  fi

  # Auth route
  if [[ "${AUTH_ENABLED:-false}" == "true" ]]; then
    generate_auth_route
  fi

  # MinIO route
  if [[ "${MINIO_ENABLED:-false}" == "true" ]]; then
    generate_minio_route
  fi

  # API routes for custom services
  generate_api_routes

  # Frontend app routes
  generate_frontend_routes
}

# Generate Hasura route
generate_hasura_route() {
  cat >nginx/sites/hasura.conf <<'EOF'
# Hasura GraphQL Engine
# gzip off: compressed JSON breaks keyword monitors (UptimeRobot, etc.)
gzip off;

location /v1/graphql {
    proxy_pass http://hasura:8080/v1/graphql;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header User-Agent $http_user_agent;
    proxy_pass_request_headers on;

    # WebSocket support
    proxy_read_timeout 86400;
}

location /v1/version {
    proxy_pass http://hasura:8080/v1/version;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header User-Agent $http_user_agent;
}

location /console {
    proxy_pass http://hasura:8080/console;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header User-Agent $http_user_agent;
}
EOF
}

# Generate Auth route
generate_auth_route() {
  cat >nginx/sites/auth.conf <<'EOF'
# Auth Service
location /auth/ {
    proxy_pass http://auth:4000/;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    # Rate limiting for auth endpoints
    limit_req zone=auth burst=5 nodelay;
}
EOF
}

# Generate MinIO route
generate_minio_route() {
  cat >nginx/sites/minio.conf <<'EOF'
# MinIO Object Storage
location /minio/ {
    proxy_pass http://minio:9000/;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    # File upload settings
    client_max_body_size 500M;
    proxy_request_buffering off;
}

# MinIO Console
location /minio-console/ {
    proxy_pass http://minio:9001/;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
EOF
}

# Generate API routes for custom services
generate_api_routes() {
  # Check for custom API services
  if [[ -n "${API_SERVICES:-}" ]]; then
    IFS=',' read -ra SERVICES <<<"$API_SERVICES"
    for service in "${SERVICES[@]}"; do
      generate_custom_api_route "$service"
    done
  fi
}

# Generate custom API route
generate_custom_api_route() {
  local service="$1"
  local service_upper=$(echo "$service" | tr '[:lower:]' '[:upper:]')
  local service_port_var="${service_upper}_PORT"
  local service_port="${!service_port_var:-3000}"

  cat >"nginx/sites/${service}.conf" <<EOF
# ${service} API Service
location /api/${service}/ {
    proxy_pass http://${service}:${service_port}/;
    proxy_http_version 1.1;
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;

    # API rate limiting
    limit_req zone=api burst=10 nodelay;
}
EOF
}

# Generate nginx upstream configuration
generate_nginx_upstream() {
  local service_name="$1"
  local service_port="$2"

  cat <<EOF
upstream ${service_name}_upstream {
    server ${service_name}:${service_port};
    keepalive 32;
}
EOF
}

# Generate nginx location block
generate_nginx_location() {
  local path="$1"
  local upstream="$2"
  local extra_config="${3:-}"

  cat <<EOF
location ${path} {
    proxy_pass http://${upstream};
    proxy_http_version 1.1;
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
    proxy_set_header Connection "";
    ${extra_config}
}
EOF
}

# Generate frontend app routes
generate_frontend_routes() {
  local port_counter=3000

  # Process Next.js apps
  if [[ -n "${NEXTJS_APPS:-}" ]]; then
    IFS=',' read -ra apps <<<"$NEXTJS_APPS"
    for app in "${apps[@]}"; do
      app=$(echo "$app" | xargs)
      local app_port=$port_counter=$((port_counter + 1))
      generate_frontend_app_route "$app" "$app_port" "nextjs"
    done
  fi

  # Process React apps
  if [[ -n "${REACT_APPS:-}" ]]; then
    IFS=',' read -ra apps <<<"$REACT_APPS"
    for app in "${apps[@]}"; do
      app=$(echo "$app" | xargs)
      local app_port=$port_counter=$((port_counter + 1))
      generate_frontend_app_route "$app" "$app_port" "react"
    done
  fi

  # Process Vue apps
  if [[ -n "${VUE_APPS:-}" ]]; then
    IFS=',' read -ra apps <<<"$VUE_APPS"
    for app in "${apps[@]}"; do
      app=$(echo "$app" | xargs)
      local app_port=$port_counter=$((port_counter + 1))
      generate_frontend_app_route "$app" "$app_port" "vue"
    done
  fi

  # Process Angular apps
  if [[ -n "${ANGULAR_APPS:-}" ]]; then
    IFS=',' read -ra apps <<<"$ANGULAR_APPS"
    for app in "${apps[@]}"; do
      app=$(echo "$app" | xargs)
      local app_port=$port_counter=$((port_counter + 1))
      generate_frontend_app_route "$app" "$app_port" "angular"
    done
  fi

  # Process Svelte apps
  if [[ -n "${SVELTE_APPS:-}" ]]; then
    IFS=',' read -ra apps <<<"$SVELTE_APPS"
    for app in "${apps[@]}"; do
      app=$(echo "$app" | xargs)
      local app_port=$port_counter=$((port_counter + 1))
      generate_frontend_app_route "$app" "$app_port" "svelte"
    done
  fi
}

# Generate individual frontend app route
generate_frontend_app_route() {
  local app_name="$1"
  local app_port="$2"
  local app_type="$3"

  cat >"nginx/conf.d/${app_name}.conf" <<EOF
# Frontend app: ${app_name} (${app_type})
# Running locally on port ${app_port}

# HTTP redirect to HTTPS
server {
    listen 80;
    server_name ${app_name}.${BASE_DOMAIN:-localhost};
    return 301 https://\$server_name\$request_uri;
}

# Main app server
server {
    listen 443 ssl;
    http2 on;
    server_name ${app_name}.${BASE_DOMAIN:-localhost};

    # SSL configuration
    ssl_certificate /etc/nginx/ssl/${BASE_DOMAIN:-localhost}/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/${BASE_DOMAIN:-localhost}/privkey.pem;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # Proxy to local development server
    location / {
        proxy_pass http://host.docker.internal:${app_port};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;

        # WebSocket support for hot reload
        proxy_read_timeout 86400;
    }
}

# API subdomain for this app (proxies to main Hasura)
server {
    listen 80;
    server_name api.${app_name}.${BASE_DOMAIN:-localhost};
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name api.${app_name}.${BASE_DOMAIN:-localhost};

    ssl_certificate /etc/nginx/ssl/${BASE_DOMAIN:-localhost}/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/${BASE_DOMAIN:-localhost}/privkey.pem;

    # Disable gzip for Hasura API responses — compressed JSON breaks keyword
    # monitors (e.g. UptimeRobot) that check for plaintext in the response body.
    # Hasura error responses are tiny (~100 bytes) so compression has no benefit.
    gzip off;

    # Proxy to Hasura with app-specific headers
    location / {
        proxy_pass http://hasura:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-App-Name "${app_name}";
        proxy_set_header X-Hasura-Role "user";

        # Add CORS headers for frontend
        add_header 'Access-Control-Allow-Origin' 'https://${app_name}.${BASE_DOMAIN:-localhost}' always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS, PUT, DELETE' always;
        add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,Authorization' always;
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range' always;

        if (\$request_method = 'OPTIONS') {
            return 204;
        }
    }
}

# Auth subdomain for this app (proxies to main auth service)
server {
    listen 80;
    server_name auth.${app_name}.${BASE_DOMAIN:-localhost};
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name auth.${app_name}.${BASE_DOMAIN:-localhost};

    ssl_certificate /etc/nginx/ssl/${BASE_DOMAIN:-localhost}/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/${BASE_DOMAIN:-localhost}/privkey.pem;

    # Proxy to Auth service
    location / {
        proxy_pass http://auth:4000;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-App-Name "${app_name}";

        # Add CORS headers
        add_header 'Access-Control-Allow-Origin' 'https://${app_name}.${BASE_DOMAIN:-localhost}' always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS, PUT, DELETE' always;
        add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,Authorization' always;

        if (\$request_method = 'OPTIONS') {
            return 204;
        }
    }
}
EOF
}

# Export functions
export -f generate_nginx_config
export -f generate_main_nginx_conf
export -f generate_default_server_conf
export -f generate_ssl_config
export -f generate_security_headers_config
export -f generate_basic_security_headers
export -f generate_service_routes
export -f generate_hasura_route
export -f generate_auth_route
export -f generate_minio_route
export -f generate_api_routes
export -f generate_custom_api_route
export -f generate_nginx_upstream
export -f generate_nginx_location
export -f generate_frontend_routes
export -f generate_frontend_app_route
