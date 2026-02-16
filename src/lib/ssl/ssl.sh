#!/usr/bin/env bash
# ssl.sh - Core SSL certificate management functions


# Get the directory where this script is located
SSL_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

# Go up 3 levels: ssl -> lib -> src -> nself root (only if not already set)
NSELF_ROOT="${NSELF_ROOT:-$(cd "$SSL_LIB_DIR/../../.." && pwd)}"
TEMPLATES_DIR="$NSELF_ROOT/templates"
CERTS_DIR="$TEMPLATES_DIR/certs"
NSELF_BIN_DIR="${HOME}/.nself/bin"

# Source utilities
source "$SSL_LIB_DIR/../utils/display.sh" 2>/dev/null || true

# Default configuration
SSL_NSELF_ORG_DOMAIN="${SSL_NSELF_ORG_DOMAIN:-local.nself.org}"
SSL_LOCALHOST_NAMES="${SSL_LOCALHOST_NAMES:-localhost,*.localhost,127.0.0.1,::1}"
SSL_PUBLIC_WILDCARD="${SSL_PUBLIC_WILDCARD:-true}"
SSL_FALLBACK_LOCALHOST="${SSL_FALLBACK_LOCALHOST:-true}"

# Ensure required tools are available
ssl::ensure_tools() {
  local missing_tools=()

  # Check for docker
  if ! command -v docker &>/dev/null; then
    missing_tools+=("docker")
  fi

  # Check for openssl
  if ! command -v openssl &>/dev/null; then
    missing_tools+=("openssl")
  fi

  # If localhost fallback is enabled, ensure mkcert
  if [[ "$SSL_FALLBACK_LOCALHOST" == "true" ]] || [[ -z "${DNS_PROVIDER:-}" ]]; then
    if ! command -v mkcert &>/dev/null && [[ ! -f "$NSELF_BIN_DIR/mkcert" ]]; then
      log_info "Installing mkcert for local certificate generation..."
      ssl::install_mkcert
    fi
  fi

  if [[ ${#missing_tools[@]} -gt 0 ]]; then
    log_error "Missing required tools: ${missing_tools[*]}"
    log_info "Please install the missing tools and try again"
    return 1
  fi

  return 0
}

# Install mkcert binary
ssl::install_mkcert() {
  mkdir -p "$NSELF_BIN_DIR"

  local os arch mkcert_url
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  # Map architecture names
  case "$arch" in
    x86_64) arch="amd64" ;;
    aarch64 | arm64) arch="arm64" ;;
    *)
      log_error "Unsupported architecture: $arch"
      return 1
      ;;
  esac

  # Construct download URL
  local mkcert_version="v1.4.4"
  case "$os" in
    darwin)
      mkcert_url="https://github.com/FiloSottile/mkcert/releases/download/${mkcert_version}/mkcert-${mkcert_version}-${os}-${arch}"
      ;;
    linux)
      mkcert_url="https://github.com/FiloSottile/mkcert/releases/download/${mkcert_version}/mkcert-${mkcert_version}-${os}-${arch}"
      ;;
    *)
      log_error "Unsupported OS: $os"
      return 1
      ;;
  esac

  log_info "Downloading mkcert from $mkcert_url..."
  if curl -L -o "$NSELF_BIN_DIR/mkcert" "$mkcert_url"; then
    chmod +x "$NSELF_BIN_DIR/mkcert"
    export PATH="$NSELF_BIN_DIR:$PATH"
    log_success "mkcert installed successfully"
  else
    log_error "Failed to download mkcert"
    return 1
  fi
}

# Get mkcert command (either from PATH or our bin dir)
ssl::get_mkcert() {
  if command -v mkcert &>/dev/null; then
    echo "mkcert"
  elif [[ -f "$NSELF_BIN_DIR/mkcert" ]]; then
    echo "$NSELF_BIN_DIR/mkcert"
  else
    return 1
  fi
}

# Issue public wildcard certificate via Let's Encrypt
ssl::issue_public_wildcard() {
  local domain="${SSL_NSELF_ORG_DOMAIN}"
  local provider="${DNS_PROVIDER:-}"
  local output_dir="$CERTS_DIR/nself-org"

  if [[ -z "$provider" ]]; then
    log_warning "No DNS provider configured, skipping public wildcard"
    return 1
  fi

  log_info "Issuing Let's Encrypt wildcard certificate for *.$domain..."

  # Prepare acme.sh working directory
  local acme_dir="$HOME/.nself/acme"
  mkdir -p "$acme_dir"

  # Build docker command based on provider
  local docker_env_args=()
  case "$provider" in
    cloudflare)
      docker_env_args+=("-e" "CF_Token=${DNS_API_TOKEN:-}")
      docker_env_args+=("-e" "CF_Email=${CF_EMAIL:-}")
      ;;
    route53)
      docker_env_args+=("-e" "AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID:-}")
      docker_env_args+=("-e" "AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY:-}")
      ;;
    digitalocean)
      docker_env_args+=("-e" "DO_API_KEY=${DO_API_KEY:-}")
      ;;
    *)
      log_error "Unsupported DNS provider: $provider"
      return 1
      ;;
  esac

  # Run acme.sh in docker
  if docker run --rm \
    "${docker_env_args[@]}" \
    -v "$acme_dir:/acme.sh" \
    neilpang/acme.sh:latest \
    --issue --dns "dns_$provider" \
    -d "*.$domain" \
    -d "$domain" \
    --keylength ec-256 \
    --server letsencrypt; then

    # Copy certificates to our template directory
    mkdir -p "$output_dir"

    local cert_dir="$acme_dir/${domain}_ecc"
    if [[ -d "$cert_dir" ]]; then
      cp "$cert_dir/fullchain.cer" "$output_dir/fullchain.pem"
      cp "$cert_dir/${domain}.key" "$output_dir/privkey.pem"
      cp "$cert_dir/${domain}.cer" "$output_dir/cert.pem"

      # Create PFX for Windows
      openssl pkcs12 -export \
        -out "$output_dir/wildcard.pfx" \
        -inkey "$output_dir/privkey.pem" \
        -in "$output_dir/fullchain.pem" \
        -passout pass: 2>/dev/null || true

      log_success "Public wildcard certificate issued successfully"
      return 0
    fi
  fi

  log_error "Failed to issue public wildcard certificate"
  return 1
}

# Issue internal certificate for *.local.nself.org using mkcert
ssl::issue_internal_nself_org() {
  local domain="${SSL_NSELF_ORG_DOMAIN}"
  local output_dir="$CERTS_DIR/nself-org"
  local mkcert_cmd

  if ! mkcert_cmd="$(ssl::get_mkcert)"; then
    log_error "mkcert not available"
    return 1
  fi

  log_info "Generating internal certificate for *.$domain..."

  mkdir -p "$output_dir"

  # Generate certificate
  if $mkcert_cmd \
    -cert-file "$output_dir/cert.pem" \
    -key-file "$output_dir/privkey.pem" \
    "$domain" "*.$domain"; then

    # mkcert doesn't create a fullchain, so we'll use cert as fullchain
    cp "$output_dir/cert.pem" "$output_dir/fullchain.pem"

    # Create PFX for Windows
    openssl pkcs12 -export \
      -out "$output_dir/wildcard.pfx" \
      -inkey "$output_dir/privkey.pem" \
      -in "$output_dir/cert.pem" \
      -passout pass: 2>/dev/null || true

    log_success "Internal wildcard certificate generated successfully"
    log_warning "Run 'nself trust' to install the root CA in your system"
    return 0
  fi

  log_error "Failed to generate internal certificate"
  return 1
}

# Issue localhost certificate bundle using mkcert
ssl::issue_localhost_bundle() {
  local output_dir="$CERTS_DIR/localhost"
  local mkcert_cmd

  if ! mkcert_cmd="$(ssl::get_mkcert)"; then
    log_error "mkcert not available"
    return 1
  fi

  log_info "Generating localhost certificate bundle..."

  # Install mkcert root CA if not already done
  $mkcert_cmd -install 2>/dev/null || true

  mkdir -p "$output_dir"

  # Parse comma-separated names
  local names=()
  IFS=',' read -ra names <<<"$SSL_LOCALHOST_NAMES"

  # Generate certificate
  if $mkcert_cmd \
    -cert-file "$output_dir/cert.pem" \
    -key-file "$output_dir/privkey.pem" \
    "${names[@]}"; then

    # mkcert doesn't create a fullchain, so we'll use cert as fullchain
    cp "$output_dir/cert.pem" "$output_dir/fullchain.pem"

    log_success "Localhost certificate bundle generated successfully"
    return 0
  fi

  log_error "Failed to generate localhost certificate"
  return 1
}

# Copy certificates into project
ssl::copy_into_project() {
  local project_dir="${1:-.}"
  local nginx_ssl_dir="$project_dir/nginx/ssl"
  local ssl_dir="$project_dir/ssl/certificates"

  log_info "Copying certificates to project..."

  # Create target directories for both nginx and project ssl
  mkdir -p "$nginx_ssl_dir"/{localhost,nself-org,custom}
  mkdir -p "$ssl_dir"/{localhost,nself-org,custom}

  # Copy localhost certificates if they exist
  if [[ -f "$CERTS_DIR/localhost/fullchain.pem" ]]; then
    # Copy to nginx directory
    cp "$CERTS_DIR/localhost/fullchain.pem" "$nginx_ssl_dir/localhost/"
    cp "$CERTS_DIR/localhost/privkey.pem" "$nginx_ssl_dir/localhost/"
    chmod 600 "$nginx_ssl_dir/localhost/privkey.pem"

    # Copy to project ssl directory
    cp "$CERTS_DIR/localhost/fullchain.pem" "$ssl_dir/localhost/"
    cp "$CERTS_DIR/localhost/privkey.pem" "$ssl_dir/localhost/"
    chmod 600 "$ssl_dir/localhost/privkey.pem"

    log_success "Copied localhost certificates"
  fi

  # Copy nself.org certificates if they exist
  if [[ -f "$CERTS_DIR/nself-org/fullchain.pem" ]]; then
    # Copy to nginx directory
    cp "$CERTS_DIR/nself-org/fullchain.pem" "$nginx_ssl_dir/nself-org/"
    cp "$CERTS_DIR/nself-org/privkey.pem" "$nginx_ssl_dir/nself-org/"
    chmod 600 "$nginx_ssl_dir/nself-org/privkey.pem"

    # Copy to project ssl directory
    cp "$CERTS_DIR/nself-org/fullchain.pem" "$ssl_dir/nself-org/"
    cp "$CERTS_DIR/nself-org/privkey.pem" "$ssl_dir/nself-org/"
    chmod 600 "$ssl_dir/nself-org/privkey.pem"

    # Copy PFX if it exists
    if [[ -f "$CERTS_DIR/nself-org/wildcard.pfx" ]]; then
      cp "$CERTS_DIR/nself-org/wildcard.pfx" "$nginx_ssl_dir/nself-org/"
      cp "$CERTS_DIR/nself-org/wildcard.pfx" "$ssl_dir/nself-org/"
    fi

    log_success "Copied *.local.nself.org certificates"
  fi

  # Copy custom certificates if they exist
  if [[ -f "$CERTS_DIR/custom/fullchain.pem" ]]; then
    # Copy to nginx directory
    cp "$CERTS_DIR/custom/fullchain.pem" "$nginx_ssl_dir/custom/"
    cp "$CERTS_DIR/custom/privkey.pem" "$nginx_ssl_dir/custom/"
    chmod 600 "$nginx_ssl_dir/custom/privkey.pem"

    # Copy to project ssl directory
    cp "$CERTS_DIR/custom/fullchain.pem" "$ssl_dir/custom/"
    cp "$CERTS_DIR/custom/privkey.pem" "$ssl_dir/custom/"
    chmod 600 "$ssl_dir/custom/privkey.pem"

    log_success "Copied custom domain certificates"
  fi

  # Reload nginx if it's running to pick up new certificates
  ssl::reload_nginx "$project_dir" 2>/dev/null || true
}

# Reload nginx to pick up new SSL certificates
ssl::reload_nginx() {
  local project_dir="${1:-.}"

  # Try to get project name from .env file
  local project_name=""
  if [[ -f "$project_dir/.env" ]]; then
    project_name=$(grep -E "^PROJECT_NAME=" "$project_dir/.env" 2>/dev/null | cut -d= -f2 | tr -d '"' | tr -d "'")
  fi

  # Fallback to directory basename
  if [[ -z "$project_name" ]]; then
    project_name=$(basename "$(cd "$project_dir" && pwd)")
  fi

  local nginx_container="${project_name}_nginx"

  # Check if nginx container is running
  if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${nginx_container}$"; then
    # Reload nginx configuration
    if docker exec "$nginx_container" nginx -s reload >/dev/null 2>&1; then
      log_info "Reloaded nginx to pick up new SSL certificates"
      return 0
    fi
  fi

  return 1
}

# Render nginx SSL configuration snippets
ssl::render_nginx_snippets() {
  local project_dir="${1:-.}"
  local nginx_conf_dir="$project_dir/nginx/conf.d"

  log_info "Generating nginx SSL configurations..."

  mkdir -p "$nginx_conf_dir"

  # Generate SSL configuration for *.local.nself.org
  if [[ -f "$project_dir/nginx/ssl/nself-org/fullchain.pem" ]]; then
    cat >"$nginx_conf_dir/ssl-local-nself-org.conf" <<'EOF'
# Wildcard SSL for *.local.nself.org
server {
    listen 443 ssl;
    http2 on;
    server_name ~^(?<subdomain>.+)\.local\.nself\.org$;
    
    ssl_certificate     /etc/nginx/ssl/nself-org/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/nself-org/privkey.pem;
    
    # Modern SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    
    # SSL session caching
    ssl_session_timeout 1d;
    ssl_session_cache shared:SSL:10m;
    ssl_session_tickets off;
    
    # Disable HSTS in development to avoid pinning
    # add_header Strict-Transport-Security "max-age=0";
    
    # Route to appropriate backend based on subdomain
    location / {
        # Default proxy headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Return 404 for unmapped subdomains
        return 404;
    }
}

# HTTP to HTTPS redirect for *.local.nself.org
server {
    listen 80;
    server_name ~^(?<subdomain>.+)\.local\.nself\.org$;
    return 301 https://$host$request_uri;
}
EOF
    log_success "Generated SSL configuration for *.local.nself.org"
  fi

  # Generate SSL configuration for localhost
  if [[ -f "$project_dir/nginx/ssl/localhost/fullchain.pem" ]]; then
    cat >"$nginx_conf_dir/ssl-localhost.conf" <<'EOF'
# SSL for localhost and *.localhost
server {
    listen 443 ssl;
    http2 on;
    server_name localhost *.localhost;
    
    ssl_certificate     /etc/nginx/ssl/localhost/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/localhost/privkey.pem;
    
    # Modern SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    
    # SSL session caching
    ssl_session_timeout 1d;
    ssl_session_cache shared:SSL:10m;
    ssl_session_tickets off;
    
    # Default route to main application
    location / {
        proxy_pass http://hasura;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

# HTTP to HTTPS redirect for localhost
server {
    listen 80;
    server_name localhost *.localhost;
    return 301 https://$host$request_uri;
}
EOF
    log_success "Generated SSL configuration for localhost"
  fi

  # Create routes directory for subdomain-specific configs
  mkdir -p "$nginx_conf_dir/routes"

  # Create default route files for common subdomains
  local subdomains=("api" "auth" "storage" "functions" "dashboard")
  for subdomain in "${subdomains[@]}"; do
    ssl::create_route_config "$nginx_conf_dir/routes" "$subdomain"
  done
}

# Create route configuration for a subdomain
ssl::create_route_config() {
  local routes_dir="$1"
  local subdomain="$2"

  case "$subdomain" in
    api)
      cat >"$routes_dir/api.conf" <<'EOF'
proxy_pass http://hasura:8080;
EOF
      ;;
    auth)
      cat >"$routes_dir/auth.conf" <<'EOF'
proxy_pass http://auth:4000;
EOF
      ;;
    storage)
      cat >"$routes_dir/storage.conf" <<'EOF'
proxy_pass http://storage:5001;
client_max_body_size 1000m;
EOF
      ;;
    functions)
      cat >"$routes_dir/functions.conf" <<EOF
proxy_pass http://\${PROJECT_NAME:-nself}-functions:4300;
EOF
      ;;
  esac
}

# Check if certificates need regeneration for a domain
ssl::needs_regeneration() {
  local base_domain="${1:-localhost}"
  local project_ssl_dir="${2:-.}"

  # Check if we're using localhost domain
  if [[ "$base_domain" == "localhost" ]]; then
    local cert_file="$project_ssl_dir/ssl/certificates/localhost/fullchain.pem"

    # Check if certificate exists
    if [[ ! -f "$cert_file" ]]; then
      return 0 # Needs generation
    fi

    # Check if certificate includes wildcard
    local san=$(openssl x509 -in "$cert_file" -text -noout 2>/dev/null | grep -A1 "Subject Alternative Name" | tail -1)
    if [[ ! "$san" == *"*.localhost"* ]]; then
      log_info "Certificate doesn't include wildcard *.localhost"
      return 0 # Needs regeneration
    fi

    # Check expiry
    if ! openssl x509 -in "$cert_file" -checkend 86400 &>/dev/null; then
      log_info "Certificate expires within 24 hours"
      return 0 # Needs regeneration
    fi
  elif [[ "$base_domain" == "local.nself.org" ]]; then
    local cert_file="$project_ssl_dir/ssl/certificates/nself-org/fullchain.pem"

    # Check if certificate exists
    if [[ ! -f "$cert_file" ]]; then
      return 0 # Needs generation
    fi

    # Check expiry
    if ! openssl x509 -in "$cert_file" -checkend 86400 &>/dev/null; then
      log_info "Certificate expires within 24 hours"
      return 0 # Needs regeneration
    fi
  else
    # Custom domain - always needs certificate
    return 0
  fi

  return 1 # No regeneration needed
}

# Generate certificates for project with auto-detection
ssl::generate_for_project() {
  local project_dir="${1:-.}"
  local base_domain="${2:-localhost}"
  local env="${ENV:-dev}"

  # Determine certificate strategy based on environment
  local cert_strategy="mkcert" # Default for dev

  if [[ "$env" == "prod" ]] || [[ "$env" == "production" ]]; then
    # Production - use Let's Encrypt if possible
    if [[ -n "${DNS_PROVIDER:-}" ]] && [[ "$base_domain" != "localhost" ]]; then
      cert_strategy="letsencrypt"
    else
      cert_strategy="self-signed"
      log_warning "Production mode but no DNS provider configured - using self-signed certificates"
    fi
  elif [[ "$env" == "staging" ]]; then
    # Staging - use Let's Encrypt staging or mkcert
    if [[ -n "${DNS_PROVIDER:-}" ]]; then
      cert_strategy="letsencrypt-staging"
    else
      cert_strategy="mkcert"
    fi
  fi

  log_info "SSL Strategy: $cert_strategy for $base_domain (ENV=$env)"

  # Check if we need to regenerate
  if ! ssl::needs_regeneration "$base_domain" "$project_dir"; then
    log_info "SSL certificates are up to date for $base_domain"

    # Check for auto-renewal in production
    if [[ "$cert_strategy" == "letsencrypt" ]]; then
      ssl::check_renewal "$base_domain" "$project_dir"
    fi

    return 0
  fi

  log_info "Generating SSL certificates for $base_domain..."

  # Generate certificates based on strategy
  case "$cert_strategy" in
    letsencrypt)
      if ! ssl::issue_letsencrypt "$base_domain" "$project_dir"; then
        log_warning "Let's Encrypt failed, falling back to self-signed"
        ssl::issue_self_signed "$base_domain" "$project_dir"
      fi
      # Setup auto-renewal
      ssl::setup_auto_renewal "$base_domain" "$project_dir"
      ;;

    letsencrypt-staging)
      if ! ssl::issue_letsencrypt_staging "$base_domain" "$project_dir"; then
        log_warning "Let's Encrypt staging failed, falling back to mkcert"
        ssl::issue_with_mkcert "$base_domain" "$project_dir"
      fi
      ;;

    mkcert)
      # Ensure tools are available
      if ! ssl::ensure_tools; then
        return 1
      fi

      if ! ssl::issue_with_mkcert "$base_domain" "$project_dir"; then
        log_warning "mkcert failed, falling back to self-signed"
        ssl::issue_self_signed "$base_domain" "$project_dir"
      fi
      ;;

    self-signed)
      ssl::issue_self_signed "$base_domain" "$project_dir"
      ;;
  esac

  # Copy certificates to project
  ssl::copy_into_project "$project_dir"

  # Generate nginx configs
  ssl::render_nginx_snippets "$project_dir"

  log_success "SSL certificates generated and installed for $base_domain"

  # Check if root CA is installed (for dev environments)
  if [[ "$cert_strategy" == "mkcert" ]]; then
    local mkcert_cmd
    if mkcert_cmd="$(ssl::get_mkcert 2>/dev/null)"; then
      if ! $mkcert_cmd -install -check 2>/dev/null; then
        log_warning "Root CA not installed - run 'nself trust' to remove browser warnings"
      fi
    fi
  fi

  return 0
}

# Issue certificates with mkcert
ssl::issue_with_mkcert() {
  local base_domain="${1}"
  local project_dir="${2}"
  local project_name="${PROJECT_NAME:-app}"

  local mkcert_cmd
  if ! mkcert_cmd="$(ssl::get_mkcert)"; then
    log_error "mkcert not available"
    return 1
  fi

  # Install root CA if needed
  $mkcert_cmd -install 2>/dev/null || true

  # Determine certificate directory based on domain type
  local cert_dir
  if [[ "$base_domain" == "localhost" ]]; then
    cert_dir="$CERTS_DIR/localhost"
  elif [[ "$base_domain" == *"local.nself.org" ]]; then
    cert_dir="$CERTS_DIR/nself-org"
  else
    # Custom domain (production, staging, or custom dev domain)
    cert_dir="$CERTS_DIR/custom"
  fi

  mkdir -p "$cert_dir"

  # Use dynamic service discovery to collect ALL domains
  local sans=()

  # Source service discovery if available
  # Set SCRIPT_DIR if not already set
  if [[ -z "${SCRIPT_DIR:-}" ]]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/src/cli"
  fi

  if [[ -f "$SCRIPT_DIR/../lib/services/service-routes.sh" ]]; then
    source "$SCRIPT_DIR/../lib/services/service-routes.sh"

    # Collect all domains dynamically
    while IFS= read -r domain; do
      [[ -n "$domain" ]] && sans+=("$domain")
    done < <(routes::get_ssl_domains)
  else
    # Fallback to static domain list (backward compatibility)
    if [[ "$base_domain" == "localhost" ]]; then
      # Core localhost domains
      sans+=(
        "localhost"
        "127.0.0.1"
        "::1"
      )

      # Common service subdomains (explicitly listed)
      sans+=(
        "api.localhost"       # Hasura GraphQL
        "auth.localhost"      # Authentication service
        "storage.localhost"   # File storage
        "functions.localhost" # Serverless functions
        "dashboard.localhost" # Admin dashboard
        "console.localhost"   # Hasura console
        "mail.localhost"      # Mailpit
        "search.localhost"    # MeiliSearch
        "db.localhost"        # Adminer
        "queues.localhost"    # BullMQ Dashboard
      )

      # Project-specific subdomains
      if [[ -n "$project_name" ]]; then
        sans+=("${project_name}.localhost") # Main app domain

        # Common variations (chat app, admin, etc)
        [[ "$project_name" == "nchat" ]] && sans+=("chat.localhost")
        [[ "$project_name" == "admin" ]] && sans+=("admin.localhost")
      fi

      # Add wildcard as last resort
      sans+=("*.localhost")
    else
      # For custom domains
      sans+=(
        "$base_domain"
        "*.$base_domain"
        "api.$base_domain"
        "auth.$base_domain"
        "storage.$base_domain"
        "mail.$base_domain"
        "search.$base_domain"
        "db.$base_domain"
        "queues.$base_domain"
      )

      if [[ -n "$project_name" ]]; then
        sans+=("${project_name}.$base_domain")
      fi
    fi
  fi

  log_info "Generating certificate for ${#sans[@]} domains"
  log_info "Domains: ${sans[*]:0:10}$((${#sans[@]} > 10 ? "..." : ""))"

  if $mkcert_cmd \
    -cert-file "$cert_dir/fullchain.pem" \
    -key-file "$cert_dir/privkey.pem" \
    "${sans[@]}"; then

    # Copy cert as fullchain for compatibility
    cp "$cert_dir/fullchain.pem" "$cert_dir/cert.pem" 2>/dev/null || true

    # Set proper permissions
    chmod 644 "$cert_dir/fullchain.pem" "$cert_dir/cert.pem"
    chmod 600 "$cert_dir/privkey.pem"

    log_success "Certificate generated with ${#sans[@]} domains"
    return 0
  fi

  return 1
}

# Issue Let's Encrypt production certificate
ssl::issue_letsencrypt() {
  local base_domain="${1}"
  local project_dir="${2}"

  # Use existing public wildcard function with modifications
  export SSL_NSELF_ORG_DOMAIN="$base_domain"

  if ssl::issue_public_wildcard; then
    return 0
  fi

  return 1
}

# Issue Let's Encrypt staging certificate
ssl::issue_letsencrypt_staging() {
  local base_domain="${1}"
  local project_dir="${2}"

  # Similar to production but with staging server
  local provider="${DNS_PROVIDER:-}"
  if [[ -z "$provider" ]]; then
    return 1
  fi

  # Use staging server for testing
  # Implementation would be similar to ssl::issue_public_wildcard
  # but with --server letsencrypt_test

  return 1 # Placeholder for now
}

# Issue self-signed certificate
ssl::issue_self_signed() {
  local base_domain="${1}"
  local project_dir="${2}"

  local cert_dir="$CERTS_DIR/self-signed"
  mkdir -p "$cert_dir"

  # Generate self-signed certificate
  openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout "$cert_dir/privkey.pem" \
    -out "$cert_dir/fullchain.pem" \
    -subj "/C=US/ST=State/L=City/O=nself/CN=$base_domain" \
    -addext "subjectAltName=DNS:$base_domain,DNS:*.$base_domain" 2>/dev/null

  cp "$cert_dir/fullchain.pem" "$cert_dir/cert.pem"

  log_warning "Using self-signed certificate for $base_domain"
  return 0
}

# Check if certificate needs renewal (for Let's Encrypt)
ssl::check_renewal() {
  local base_domain="${1}"
  local project_dir="${2}"

  local cert_file="$project_dir/ssl/certificates/nself-org/fullchain.pem"

  if [[ -f "$cert_file" ]]; then
    # Check if certificate expires within 30 days
    if ! openssl x509 -in "$cert_file" -checkend $((30 * 24 * 60 * 60)) &>/dev/null; then
      log_info "Certificate expires within 30 days, renewing..."
      ssl::issue_letsencrypt "$base_domain" "$project_dir"
    fi
  fi
}

# Setup auto-renewal for Let's Encrypt
ssl::setup_auto_renewal() {
  local base_domain="${1}"
  local project_dir="${2}"

  # Create renewal script
  local renewal_script="$HOME/.nself/ssl-renewal.sh"

  cat >"$renewal_script" <<EOF
#!/usr/bin/env bash
# Auto-renewal script for nself SSL certificates

export BASE_DOMAIN="$base_domain"
export PROJECT_DIR="$project_dir"
export DNS_PROVIDER="${DNS_PROVIDER:-}"
export DNS_API_TOKEN="${DNS_API_TOKEN:-}"

# Source nself SSL library
source "$SSL_LIB_DIR/ssl.sh"

# Check and renew if needed
ssl::check_renewal "\$BASE_DOMAIN" "\$PROJECT_DIR"

# Restart nginx if certificate was renewed
if [[ \$? -eq 0 ]]; then
  cd "\$PROJECT_DIR" && docker compose restart nginx
fi
EOF

  chmod +x "$renewal_script"

  # Add to crontab (run daily at 2 AM)
  local cron_entry="0 2 * * * $renewal_script >> $HOME/.nself/ssl-renewal.log 2>&1"

  # Check if cron entry already exists
  if ! crontab -l 2>/dev/null | grep -q "$renewal_script"; then
    (
      crontab -l 2>/dev/null
      echo "$cron_entry"
    ) | crontab -
    log_info "Added SSL auto-renewal to crontab"
  fi
}

# Check certificate status
ssl::status() {
  echo "SSL Certificate Status:"
  echo "======================="
  echo

  # Check localhost certificates
  echo "Localhost Certificates:"
  if [[ -f "$CERTS_DIR/localhost/fullchain.pem" ]]; then
    local expiry=$(openssl x509 -in "$CERTS_DIR/localhost/fullchain.pem" -noout -enddate 2>/dev/null | cut -d= -f2)
    echo "  ✓ Certificate exists (expires: $expiry)"
  else
    echo "  ✗ No certificate found"
  fi
  echo

  # Check nself.org certificates
  echo "*.local.nself.org Certificates:"
  if [[ -f "$CERTS_DIR/nself-org/fullchain.pem" ]]; then
    local expiry=$(openssl x509 -in "$CERTS_DIR/nself-org/fullchain.pem" -noout -enddate 2>/dev/null | cut -d= -f2)
    local issuer=$(openssl x509 -in "$CERTS_DIR/nself-org/fullchain.pem" -noout -issuer 2>/dev/null | sed 's/issuer=//')
    echo "  ✓ Certificate exists"
    echo "    Issuer: $issuer"
    echo "    Expires: $expiry"

    if [[ "$issuer" == *"Let's Encrypt"* ]]; then
      echo "    Type: Public (Let's Encrypt)"
    else
      echo "    Type: Internal (mkcert)"
    fi
  else
    echo "  ✗ No certificate found"
  fi
  echo

  # Check mkcert installation
  echo "mkcert Status:"
  if mkcert_cmd="$(ssl::get_mkcert 2>/dev/null)"; then
    echo "  ✓ mkcert installed at: $mkcert_cmd"
    if $mkcert_cmd -install -check 2>/dev/null; then
      echo "  ✓ Root CA is installed in system"
    else
      echo "  ✗ Root CA not installed (run 'nself trust')"
    fi
  else
    echo "  ✗ mkcert not installed"
  fi
}

# Export functions
export -f ssl::ensure_tools
export -f ssl::install_mkcert
export -f ssl::get_mkcert
export -f ssl::issue_public_wildcard
export -f ssl::issue_internal_nself_org
export -f ssl::issue_localhost_bundle
export -f ssl::copy_into_project
export -f ssl::render_nginx_snippets
export -f ssl::create_route_config
export -f ssl::needs_regeneration
export -f ssl::generate_for_project
export -f ssl::issue_with_mkcert
export -f ssl::issue_letsencrypt
export -f ssl::issue_letsencrypt_staging
export -f ssl::issue_self_signed
export -f ssl::check_renewal
export -f ssl::setup_auto_renewal
export -f ssl::status
