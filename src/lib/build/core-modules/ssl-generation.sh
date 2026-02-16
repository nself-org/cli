#!/usr/bin/env bash

# ssl-generation.sh - SSL certificate generation module
# POSIX-compliant, no Bash 4+ features

# Generate self-signed SSL certificates
generate_ssl_certificates() {

set -euo pipefail

  local domain="${1:-localhost}"
  local output_dir="${2:-ssl/certificates}"

  # Create output directory
  mkdir -p "$output_dir/$domain"

  # Check if certificates already exist
  if [[ -f "$output_dir/$domain/fullchain.pem" ]] && [[ -f "$output_dir/$domain/privkey.pem" ]]; then
    return 0
  fi

  # Check for OpenSSL
  if ! command -v openssl >/dev/null 2>&1; then
    echo "OpenSSL not found, skipping certificate generation" >&2
    return 1
  fi

  # Generate private key
  openssl genrsa -out "$output_dir/$domain/privkey.pem" 2048 2>/dev/null

  # Generate certificate
  openssl req -new -x509 \
    -key "$output_dir/$domain/privkey.pem" \
    -out "$output_dir/$domain/fullchain.pem" \
    -days 365 \
    -subj "/C=US/ST=State/L=City/O=Organization/CN=$domain" 2>/dev/null

  # Set proper permissions
  chmod 644 "$output_dir/$domain/fullchain.pem" 2>/dev/null
  chmod 600 "$output_dir/$domain/privkey.pem" 2>/dev/null

  return 0
}

# Build list of all SSL domains from environment
# Returns space-separated list of domains to include in SSL certificate
build_ssl_domains() {
  local base_domain="${BASE_DOMAIN:-localhost}"
  local domains=()
  local subdomains=()

  # Always include localhost and 127.0.0.1
  domains+=(localhost 127.0.0.1 ::1)

  # Include BASE_DOMAIN and its wildcard
  if [[ "$base_domain" != "localhost" ]]; then
    domains+=("$base_domain" "*.$base_domain")
  fi

  # Also include local.nself.org for default development
  if [[ "$base_domain" != "local.nself.org" ]]; then
    domains+=(local.nself.org "*.local.nself.org")
  fi

  # Core service subdomains
  subdomains+=(api auth storage)

  # Optional services
  [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]] && subdomains+=(admin)
  [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]] && subdomains+=(functions)
  [[ "${MAILPIT_ENABLED:-false}" == "true" ]] && subdomains+=(mail)
  [[ "${MINIO_ENABLED:-false}" == "true" ]] && subdomains+=(minio)
  [[ "${MEILISEARCH_ENABLED:-false}" == "true" ]] && subdomains+=(search)
  [[ "${MLFLOW_ENABLED:-false}" == "true" ]] && subdomains+=(mlflow)

  # Monitoring services
  if [[ "${MONITORING_ENABLED:-false}" == "true" ]]; then
    [[ "${GRAFANA_ENABLED:-true}" == "true" ]] && subdomains+=(grafana)
    [[ "${PROMETHEUS_ENABLED:-true}" == "true" ]] && subdomains+=(prometheus)
    [[ "${ALERTMANAGER_ENABLED:-true}" == "true" ]] && subdomains+=(alertmanager)
    [[ "${TEMPO_ENABLED:-true}" == "true" ]] && subdomains+=(tempo)
    [[ "${LOKI_ENABLED:-true}" == "true" ]] && subdomains+=(loki)
  fi

  # Custom services (CS_1 through CS_10)
  for i in {1..10}; do
    local cs_var="CS_${i}"
    local cs_route_var="CS_${i}_ROUTE"
    local cs_public_var="CS_${i}_PUBLIC"

    if [[ -n "${!cs_var:-}" ]] && [[ "${!cs_public_var:-false}" == "true" ]] && [[ -n "${!cs_route_var:-}" ]]; then
      subdomains+=("${!cs_route_var}")
    fi
  done

  # Frontend apps (APP_1 through APP_10 or FRONTEND_APP_1 through FRONTEND_APP_10)
  for i in {1..10}; do
    local app_route_var="FRONTEND_APP_${i}_ROUTE"
    local alt_route_var="APP_${i}_ROUTE"

    if [[ -n "${!app_route_var:-}" ]]; then
      subdomains+=("${!app_route_var}")
    elif [[ -n "${!alt_route_var:-}" ]]; then
      subdomains+=("${!alt_route_var}")
    fi
  done

  # Add subdomains with BASE_DOMAIN suffix (e.g., "api.local.nself.org")
  for subdomain in $(printf "%s\n" "${subdomains[@]}" | sort -u); do
    # Skip if subdomain already contains a domain
    if [[ "$subdomain" != *.* ]]; then
      domains+=("${subdomain}.${base_domain}")
    else
      domains+=("$subdomain")
    fi
  done

  # Return unique domains
  printf "%s\n" "${domains[@]}" | sort -u | tr '\n' ' '
}

# Legacy function for backwards compatibility
build_localhost_subdomains() {
  build_ssl_domains
}

# Generate SSL certificates with SAN for all configured domains
generate_localhost_ssl() {
  local output_dir="${1:-ssl/certificates/localhost}"
  local base_domain="${BASE_DOMAIN:-localhost}"

  mkdir -p "$output_dir"

  # Check if already exists
  if [[ -f "$output_dir/fullchain.pem" ]] && [[ -f "$output_dir/privkey.pem" ]]; then
    return 0
  fi

  # Build dynamic list of all domains based on BASE_DOMAIN and enabled services
  local all_domains
  all_domains=$(build_ssl_domains)

  # Try mkcert first if available
  if command -v mkcert >/dev/null 2>&1; then
    # Convert space-separated string to array
    local domains_array=()
    for domain in $all_domains; do
      domains_array+=("$domain")
    done

    mkcert -cert-file "$output_dir/fullchain.pem" \
      -key-file "$output_dir/privkey.pem" \
      "${domains_array[@]}" >/dev/null 2>&1

    if [[ -f "$output_dir/fullchain.pem" ]] && [[ -f "$output_dir/privkey.pem" ]]; then
      chmod 644 "$output_dir/fullchain.pem" 2>/dev/null
      chmod 600 "$output_dir/privkey.pem" 2>/dev/null
      return 0
    fi
  fi

  # Fallback to OpenSSL self-signed if mkcert not available or failed
  # Build dynamic OpenSSL config with SAN entries
  local temp_config=$(mktemp)

  # Write config header
  cat >"$temp_config" <<EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
req_extensions = v3_req

[dn]
C = US
ST = State
L = City
O = Local Development
CN = ${base_domain}

[v3_req]
subjectAltName = @alt_names

[alt_names]
EOF

  # Add all domains as DNS entries
  local dns_index=1
  local ip_index=1
  for domain in $all_domains; do
    if [[ "$domain" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || [[ "$domain" == "::1" ]]; then
      # IP address
      printf "IP.%d = %s\n" "$ip_index" "$domain" >>"$temp_config"
      ip_index=$((ip_index + 1))
    else
      # DNS name
      printf "DNS.%d = %s\n" "$dns_index" "$domain" >>"$temp_config"
      dns_index=$((dns_index + 1))
    fi
  done

  # Generate key and certificate
  openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout "$output_dir/privkey.pem" \
    -out "$output_dir/fullchain.pem" \
    -config "$temp_config" \
    -extensions v3_req 2>/dev/null

  rm -f "$temp_config"

  # Set permissions
  chmod 644 "$output_dir/fullchain.pem" 2>/dev/null
  chmod 600 "$output_dir/privkey.pem" 2>/dev/null

  return 0
}

# Generate nself.org wildcard certificate
generate_nself_org_ssl() {
  local output_dir="${1:-ssl/certificates/nself-org}"

  mkdir -p "$output_dir"

  # Check if already exists
  if [[ -f "$output_dir/fullchain.pem" ]] && [[ -f "$output_dir/privkey.pem" ]]; then
    return 0
  fi

  # Create OpenSSL config for *.nself.org
  local temp_config=$(mktemp)
  cat >"$temp_config" <<'EOF'
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
req_extensions = v3_req

[dn]
C = US
ST = State
L = City
O = nself.org
CN = *.nself.org

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = *.nself.org
DNS.2 = nself.org
DNS.3 = *.local.nself.org
DNS.4 = local.nself.org
EOF

  # Generate key and certificate
  openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout "$output_dir/privkey.pem" \
    -out "$output_dir/fullchain.pem" \
    -config "$temp_config" \
    -extensions v3_req 2>/dev/null

  rm -f "$temp_config"

  # Set permissions
  chmod 644 "$output_dir/fullchain.pem" 2>/dev/null
  chmod 600 "$output_dir/privkey.pem" 2>/dev/null

  return 0
}

# Copy SSL certificates to nginx directory
copy_ssl_to_nginx() {
  local source_dir="${1:-ssl/certificates}"
  local nginx_dir="${2:-nginx/ssl}"

  mkdir -p "$nginx_dir"

  # Copy all certificate directories
  for cert_dir in "$source_dir"/*; do
    if [[ -d "$cert_dir" ]]; then
      local cert_name=$(basename "$cert_dir")
      mkdir -p "$nginx_dir/$cert_name"

      # Copy certificates if they exist
      if [[ -f "$cert_dir/fullchain.pem" ]]; then
        cp "$cert_dir/fullchain.pem" "$nginx_dir/$cert_name/" 2>/dev/null
      fi
      if [[ -f "$cert_dir/privkey.pem" ]]; then
        cp "$cert_dir/privkey.pem" "$nginx_dir/$cert_name/" 2>/dev/null
      fi
    fi
  done
}

# Reload nginx to pick up new SSL certificates
reload_nginx_ssl() {
  # Get project name from environment or current directory
  local project_name="${PROJECT_NAME:-$(basename "$PWD")}"
  local nginx_container="${project_name}_nginx"

  # Check if nginx container is running
  if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${nginx_container}$"; then
    # Reload nginx configuration
    if docker exec "$nginx_container" nginx -s reload 2>/dev/null; then
      return 0
    fi
  fi

  return 1
}

# Check if SSL certificates need regeneration
check_ssl_status() {
  local domain="${1:-localhost}"
  local ssl_dir="${2:-ssl/certificates}"

  # Check if certificates exist
  if [[ ! -f "$ssl_dir/$domain/fullchain.pem" ]] || [[ ! -f "$ssl_dir/$domain/privkey.pem" ]]; then
    echo "missing"
    return 1
  fi

  # Check certificate expiry (if openssl available)
  if command -v openssl >/dev/null 2>&1; then
    local expiry_date=$(openssl x509 -enddate -noout -in "$ssl_dir/$domain/fullchain.pem" 2>/dev/null | cut -d= -f2)
    if [[ -n "$expiry_date" ]]; then
      local expiry_epoch=$(date -d "$expiry_date" +%s 2>/dev/null || date -j -f "%b %d %T %Y %Z" "$expiry_date" +%s 2>/dev/null)
      local current_epoch=$(date +%s)
      local days_until_expiry=$(((expiry_epoch - current_epoch) / 86400))

      if [[ $days_until_expiry -lt 30 ]]; then
        echo "expiring"
        return 2
      fi
    fi
  fi

  echo "valid"
  return 0
}

# Main SSL setup function
setup_ssl_certificates() {
  local base_domain="${BASE_DOMAIN:-localhost}"
  local force="${1:-false}"

  # Check if SSL is needed
  local needs_ssl=false

  # Check localhost certificates
  local localhost_status=$(check_ssl_status "localhost")
  if [[ "$localhost_status" != "valid" ]] || [[ "$force" == "true" ]]; then
    needs_ssl=true
  fi

  # Check domain certificates if not localhost
  if [[ "$base_domain" != "localhost" ]]; then
    local domain_status=$(check_ssl_status "nself-org")
    if [[ "$domain_status" != "valid" ]] || [[ "$force" == "true" ]]; then
      needs_ssl=true
    fi
  fi

  if [[ "$needs_ssl" == "false" ]]; then
    return 0
  fi

  # Generate certificates
  if ! generate_localhost_ssl; then
    echo "Failed to generate localhost certificates" >&2
    return 1
  fi

  if [[ "$base_domain" != "localhost" ]]; then
    if ! generate_nself_org_ssl; then
      echo "Failed to generate nself.org certificates" >&2
      return 1
    fi
  fi

  # Copy to nginx directory
  copy_ssl_to_nginx

  # Reload nginx if it's running (silently ignore if not)
  reload_nginx_ssl 2>/dev/null || true

  return 0
}

# Export functions
export -f build_ssl_domains
export -f build_localhost_subdomains
export -f generate_ssl_certificates
export -f generate_localhost_ssl
export -f generate_nself_org_ssl
export -f copy_ssl_to_nginx
export -f reload_nginx_ssl
export -f check_ssl_status
export -f setup_ssl_certificates
