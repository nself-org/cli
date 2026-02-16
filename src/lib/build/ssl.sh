#!/usr/bin/env bash

# ssl.sh - SSL certificate generation for build

# Source platform compatibility functions
source "$(dirname "${BASH_SOURCE[0]}")/../utils/platform-compat.sh" 2>/dev/null || true

set -euo pipefail


# Generate SSL certificates
generate_ssl_certificates() {
  local force="${1:-false}"
  local ssl_was_new=false

  # Check if certificates already exist
  if [[ "$force" != "true" ]]; then
    if [[ -f "ssl/certificates/localhost/fullchain.pem" ]]; then
      show_info "SSL certificates already exist (use --force to regenerate)"
      return 0
    fi
  else
    # Force regeneration requested
    export SSL_REGENERATED="true"
  fi

  # Mark as new if no SSL directory exists
  [[ ! -d "ssl" ]] && ssl_was_new=true && export SSL_REGENERATED="true"

  # Create certificate directories
  mkdir -p ssl/certificates/{localhost,nself-org} 2>/dev/null || true
  mkdir -p nginx/ssl/{localhost,nself-org} 2>/dev/null || true

  # Determine SSL method
  local ssl_method="self-signed"
  if command_exists mkcert; then
    ssl_method="mkcert"
  fi

  # Generate certificates based on method
  case "$ssl_method" in
    mkcert)
      generate_mkcert_certificates
      ;;
    self-signed)
      generate_self_signed_certificates
      ;;
    *)
      show_error "Unknown SSL method: $ssl_method"
      return 1
      ;;
  esac

  # Copy certificates to nginx directory
  copy_certificates_to_nginx

  return 0
}

# Generate certificates using mkcert
generate_mkcert_certificates() {
  local project_name="${PROJECT_NAME:-myproject}"
  local base_domain="${BASE_DOMAIN:-localhost}"

  # Build list of domains
  local domains=()

  if [[ "$base_domain" == "localhost" ]]; then
    # For localhost, add explicit subdomains (no wildcard - RFC 6761 issue)
    domains+=(
      "localhost"
      "${project_name}.localhost"
      "api.localhost"
      "auth.localhost"
      "storage.localhost"
      "hasura.localhost"
      "admin.localhost"
      "functions.localhost"
      "mail.localhost"
      "minio.localhost"
      "search.localhost"
      "mlflow.localhost"
      "grafana.localhost"
      "prometheus.localhost"
      "alertmanager.localhost"
      "tempo.localhost"
      "loki.localhost"
    )

    # Add custom subdomains if configured
    if [[ -n "${CUSTOM_SUBDOMAINS:-}" ]]; then
      IFS=',' read -ra CUSTOM <<<"$CUSTOM_SUBDOMAINS"
      for subdomain in "${CUSTOM[@]}"; do
        domains+=("${subdomain}.localhost")
      done
    fi

    # Generate localhost certificates
    mkcert -cert-file ssl/certificates/localhost/fullchain.pem \
      -key-file ssl/certificates/localhost/privkey.pem \
      "${domains[@]}" >/dev/null 2>&1

  else
    # For custom domains
    domains+=(
      "${base_domain}"
      "*.${base_domain}"
      "api.${base_domain}"
      "auth.${base_domain}"
      "storage.${base_domain}"
      "hasura.${base_domain}"
    )

    # Generate domain certificates
    mkcert -cert-file ssl/certificates/nself-org/fullchain.pem \
      -key-file ssl/certificates/nself-org/privkey.pem \
      "${domains[@]}" >/dev/null 2>&1

    # Also generate localhost as fallback (no wildcard for .localhost TLD)
    mkcert -cert-file ssl/certificates/localhost/fullchain.pem \
      -key-file ssl/certificates/localhost/privkey.pem \
      "localhost" "api.localhost" "auth.localhost" "storage.localhost" >/dev/null 2>&1
  fi

  show_info "Generated trusted SSL certificates with mkcert"
}

# Generate self-signed certificates
generate_self_signed_certificates() {
  local base_domain="${BASE_DOMAIN:-localhost}"

  # Generate config for certificate with SANs
  local ssl_config=$(mktemp)
  cat >"$ssl_config" <<EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
req_extensions = v3_req

[dn]
C=US
ST=State
L=City
O=nself
CN=${base_domain}

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = ${base_domain}
DNS.2 = *.${base_domain}
DNS.3 = localhost
DNS.4 = *.localhost
IP.1 = 127.0.0.1
IP.2 = ::1
EOF

  # Generate certificates
  if [[ "$base_domain" == "localhost" ]]; then
    openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
      -keyout ssl/certificates/localhost/privkey.pem \
      -out ssl/certificates/localhost/fullchain.pem \
      -config "$ssl_config" \
      -extensions v3_req >/dev/null 2>&1
  else
    # Generate for custom domain
    openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
      -keyout ssl/certificates/nself-org/privkey.pem \
      -out ssl/certificates/nself-org/fullchain.pem \
      -config "$ssl_config" \
      -extensions v3_req >/dev/null 2>&1

    # Also generate localhost certificates
    # Create a copy of config for localhost
    cp "$ssl_config" "${ssl_config}.localhost"
    safe_sed_inline "${ssl_config}.localhost" "s/CN=${base_domain}/CN=localhost/"
    openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
      -keyout ssl/certificates/localhost/privkey.pem \
      -out ssl/certificates/localhost/fullchain.pem \
      -config "${ssl_config}.localhost" \
      -extensions v3_req >/dev/null 2>&1
  fi

  rm -f "$ssl_config" "${ssl_config}.localhost"
  show_info "Generated self-signed SSL certificates"
}

# Copy certificates to nginx directory
copy_certificates_to_nginx() {
  # Ensure nginx SSL directories exist
  mkdir -p nginx/ssl/{localhost,nself-org} 2>/dev/null || true

  # Copy localhost certificates
  if [[ -f ssl/certificates/localhost/fullchain.pem ]]; then
    cp ssl/certificates/localhost/*.pem nginx/ssl/localhost/ 2>/dev/null || true
  fi

  # Copy domain certificates
  if [[ -f ssl/certificates/nself-org/fullchain.pem ]]; then
    cp ssl/certificates/nself-org/*.pem nginx/ssl/nself-org/ 2>/dev/null || true
  fi
}

# Trust certificates (for development)
trust_certificates() {
  if ! command_exists mkcert; then
    return 0
  fi

  # Install CA if not already installed
  if ! mkcert -install 2>/dev/null; then
    show_warning "Could not install mkcert CA (may need sudo)"
  fi
}

# Check SSL certificate validity
check_certificate_validity() {
  local cert_file="${1:-ssl/certificates/localhost/fullchain.pem}"

  if [[ ! -f "$cert_file" ]]; then
    return 1
  fi

  # Check if certificate is valid
  if openssl x509 -noout -checkend 86400 -in "$cert_file" >/dev/null 2>&1; then
    return 0
  else
    show_warning "SSL certificate will expire soon"
    return 1
  fi
}

# Get certificate info
get_certificate_info() {
  local cert_file="${1:-ssl/certificates/localhost/fullchain.pem}"

  if [[ ! -f "$cert_file" ]]; then
    echo "No certificate found"
    return 1
  fi

  # Get certificate details
  openssl x509 -noout -text -in "$cert_file" 2>/dev/null | grep -E "(Subject|DNS|Not After)" || echo "Invalid certificate"
}

# Helper function for self-signed certs (compatibility alias)
create_self_signed_cert() {
  generate_self_signed_certificates "$@"
}

# Get SSL certificate path
get_ssl_cert_path() {
  local base_domain="${BASE_DOMAIN:-localhost}"

  if [[ "$base_domain" == "localhost" ]]; then
    echo "ssl/certificates/localhost/fullchain.pem"
  else
    echo "ssl/certificates/nself-org/fullchain.pem"
  fi
}

# Export functions
export -f generate_ssl_certificates
export -f generate_mkcert_certificates
export -f generate_self_signed_certificates
export -f copy_certificates_to_nginx
export -f trust_certificates
export -f check_certificate_validity
export -f get_certificate_info
export -f create_self_signed_cert
export -f get_ssl_cert_path
