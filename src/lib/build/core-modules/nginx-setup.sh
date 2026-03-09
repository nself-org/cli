#!/usr/bin/env bash

# nginx-setup.sh - Nginx configuration generation module
# POSIX-compliant, no Bash 4+ features

# Generate main nginx.conf
generate_nginx_conf() {

set -euo pipefail

  local project_name="${1:-nself}"
  local base_domain="${2:-localhost}"

  # Ensure nginx directory exists
  mkdir -p nginx

  # Generate nginx.conf
  cat >nginx/nginx.conf <<'EOF'
user nginx;
worker_processes auto;
pid /var/run/nginx.pid;

events {
    worker_connections 1024;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    # Gzip
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;

    # Logging
    access_log /var/log/nginx/access.log;
    error_log /var/log/nginx/error.log warn;

    # Include all server configurations
    include /etc/nginx/conf.d/*.conf;
}
EOF

  return 0
}

# Generate default server block
generate_default_conf() {
  local base_domain="${1:-localhost}"
  local project_name="${2:-nself}"
  local ssl_path="${3:-/etc/nginx/ssl/localhost}"

  mkdir -p nginx/conf.d

  cat >nginx/conf.d/default.conf <<EOF
server {
    listen 80 default_server;
    server_name _;

    # Health check endpoint (available on HTTP)
    location /health {
        access_log off;
        return 200 "healthy\\n";
        add_header Content-Type text/plain;
    }

    # Redirect everything else to HTTPS
    location / {
        return 301 https://\$host\$request_uri;
    }
}

server {
    listen 443 ssl default_server;
    http2 on;
    server_name ${base_domain};

    ssl_certificate ${ssl_path}/fullchain.pem;
    ssl_certificate_key ${ssl_path}/privkey.pem;

    location / {
        root /usr/share/nginx/html;
        index index.html;
    }

    # Health check also available on HTTPS
    location /health {
        access_log off;
        return 200 "healthy\\n";
        add_header Content-Type text/plain;
    }
}
EOF

  return 0
}

# Generate service-specific nginx configs
generate_service_configs() {
  local base_domain="${1:-localhost}"
  local hasura_enabled="${HASURA_ENABLED:-false}"
  local auth_enabled="${AUTH_ENABLED:-false}"
  local storage_enabled="${STORAGE_ENABLED:-false}"
  local mailpit_enabled="${MAILPIT_ENABLED:-false}"

  mkdir -p nginx/conf.d

  # Hasura configuration
  if [[ "$hasura_enabled" == "true" ]]; then
    generate_hasura_nginx_conf "$base_domain"
  fi

  # Auth configuration
  if [[ "$auth_enabled" == "true" ]]; then
    generate_auth_nginx_conf "$base_domain"
  fi

  # Storage configuration
  if [[ "$storage_enabled" == "true" ]]; then
    generate_storage_nginx_conf "$base_domain"
  fi

  # Mailpit configuration
  if [[ "$mailpit_enabled" == "true" ]]; then
    generate_mailpit_nginx_conf "$base_domain"
  fi

  return 0
}

# Generate Hasura nginx config
generate_hasura_nginx_conf() {
  local base_domain="${1:-localhost}"
  local hasura_route="${HASURA_ROUTE:-api.${base_domain}}"
  local ssl_path="/etc/nginx/ssl/localhost"

  cat >nginx/conf.d/hasura.conf <<EOF
upstream hasura {
    server hasura:8080;
}

server {
    listen 80;
    server_name ${hasura_route};
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name ${hasura_route};

    ssl_certificate ${ssl_path}/fullchain.pem;
    ssl_certificate_key ${ssl_path}/privkey.pem;

    # Disable gzip for Hasura API responses — compressed JSON breaks keyword
    # monitors (e.g. UptimeRobot) that check for plaintext in the response body.
    # Hasura error responses are tiny (~100 bytes) so compression has no benefit.
    gzip off;

    location / {
        proxy_pass http://hasura:8080;
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

# Generate Auth nginx config
generate_auth_nginx_conf() {
  local base_domain="${1:-localhost}"
  local auth_route="${AUTH_ROUTE:-auth.${base_domain}}"
  local ssl_path="/etc/nginx/ssl/localhost"

  cat >nginx/conf.d/auth.conf <<EOF
upstream auth {
    server auth:4000;
}

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

    location / {
        proxy_pass http://auth;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF
}

# Generate Storage nginx config
generate_storage_nginx_conf() {
  local base_domain="${1:-localhost}"
  local storage_route="${STORAGE_ROUTE:-storage.${base_domain}}"
  local ssl_path="/etc/nginx/ssl/localhost"

  cat >nginx/conf.d/storage.conf <<EOF
upstream storage {
    server storage:5000;
}

server {
    listen 80;
    server_name ${storage_route};
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name ${storage_route};

    ssl_certificate ${ssl_path}/fullchain.pem;
    ssl_certificate_key ${ssl_path}/privkey.pem;

    client_max_body_size 100M;

    location / {
        proxy_pass http://storage;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF
}

# Generate Mailpit nginx config
generate_mailpit_nginx_conf() {
  local base_domain="${1:-localhost}"
  local mailpit_route="${MAILPIT_ROUTE:-mail.${base_domain}}"
  local ssl_path="/etc/nginx/ssl/localhost"

  cat >nginx/conf.d/mailpit.conf <<EOF
upstream mailpit {
    server mailpit:8025;
}

server {
    listen 80;
    server_name ${mailpit_route};
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name ${mailpit_route};

    ssl_certificate ${ssl_path}/fullchain.pem;
    ssl_certificate_key ${ssl_path}/privkey.pem;

    location / {
        proxy_pass http://mailpit;
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

# Main nginx setup function
setup_nginx() {
  local project_name="${PROJECT_NAME:-nself}"
  local base_domain="${BASE_DOMAIN:-localhost}"

  # Generate main nginx.conf
  generate_nginx_conf "$project_name" "$base_domain"

  # Generate default server block
  generate_default_conf "$base_domain" "$project_name"

  # Generate service-specific configs
  generate_service_configs "$base_domain"

  return 0
}

# Export functions
export -f generate_nginx_conf
export -f generate_default_conf
export -f generate_service_configs
export -f generate_hasura_nginx_conf
export -f generate_auth_nginx_conf
export -f generate_storage_nginx_conf
export -f generate_mailpit_nginx_conf
export -f setup_nginx
