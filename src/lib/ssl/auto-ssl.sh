#!/usr/bin/env bash
# auto-ssl.sh - Fully automatic SSL management


# Get the directory where this script is located
SSL_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$SSL_LIB_DIR/../utils/display.sh" 2>/dev/null || true
source "$SSL_LIB_DIR/ssl.sh" 2>/dev/null || true

# Auto-detect all domains from nginx configuration
auto_detect_domains() {
  local domains=()
  local base_domain="${BASE_DOMAIN:-local.nself.org}"

  # Always include base domain
  domains+=("$base_domain")
  domains+=("*.$base_domain")

  # Scan nginx configs for server_name directives
  if [[ -d "nginx" ]]; then
    local detected_domains=$(find nginx -name "*.conf" -exec grep -h "server_name" {} \; 2>/dev/null |
      sed 's/.*server_name[[:space:]]*\([^;]*\);.*/\1/' |
      tr ' ' '\n' |
      grep -v "^$" |
      sort -u)

    while IFS= read -r domain; do
      if [[ -n "$domain" ]] && [[ "$domain" != "_" ]]; then
        domains+=("$domain")
      fi
    done <<<"$detected_domains"
  fi

  # Remove duplicates and wildcards that are covered
  printf "%s\n" "${domains[@]}" | sort -u
}

# Automatically set up SSL for all detected domains
auto_setup_ssl() {
  log_info "🔒 Setting up automatic SSL..."

  # Detect all domains
  local domains=($(auto_detect_domains))
  log_debug "Detected domains: ${domains[*]}"

  # Ensure SSL tools are available
  if ! ssl::ensure_tools; then
    log_warning "SSL tools not available, skipping SSL setup"
    return 1
  fi

  # Set up SSL certificates
  local ssl_success=false

  # Try public wildcard first if DNS provider configured
  if [[ -n "${DNS_PROVIDER:-}" ]] && [[ -n "${DNS_API_TOKEN:-}" ]]; then
    log_info "🌐 Attempting public wildcard certificate..."
    if ssl::issue_public_wildcard; then
      ssl_success=true
      log_success "✓ Public wildcard certificate issued"
    else
      log_warning "⚠ Public wildcard failed, falling back to local certificates"
    fi
  fi

  # Fall back to local certificates if needed
  if [[ "$ssl_success" != "true" ]]; then
    log_info "🏠 Generating local certificates..."
    if ssl::issue_internal_nself_org && ssl::issue_localhost_bundle; then
      ssl_success=true
      log_success "✓ Local certificates generated"
    fi
  fi

  if [[ "$ssl_success" == "true" ]]; then
    # Copy certificates to project
    ssl::copy_into_project "."
    ssl::render_nginx_snippets "."

    # Set up automatic renewal monitoring
    setup_automatic_renewal

    # Trust certificates if not already trusted
    check_and_install_trust

    return 0
  else
    log_error "✗ SSL setup failed"
    return 1
  fi
}

# Set up automatic renewal with safer 7-day margin
setup_automatic_renewal() {
  log_debug "Setting up automatic SSL renewal..."

  # Create enhanced renewal script
  local renewal_script="/tmp/nself-ssl-auto-renewal.sh"
  cat >"$renewal_script" <<'EOF'
#!/usr/bin/env bash
# nself automatic SSL renewal script
# Runs daily to check and renew certificates 7+ days before expiry

LOG_FILE="/var/log/nself-ssl-auto.log"
PROJECT_DIR="$(pwd)"

log() {
  echo "$(date '+%Y-%m-%d %H:%M:%S') $*" | tee -a "$LOG_FILE"
}

# Change to project directory
cd "$PROJECT_DIR" || exit 1

# Check if this is an nself project
if [[ ! -f "docker-compose.yml" ]] || [[ ! -f ".env.local" ]]; then
  log "INFO: Not an nself project, skipping SSL check"
  exit 0
fi

# Load environment
set -a
source .env.local 2>/dev/null || source .env 2>/dev/null || true
set +a

# Function to check certificate expiry
check_cert_expiry() {
  local cert_path="$1"
  local threshold_days="${2:-7}"  # 7 days safety margin
  
  if [[ ! -f "$cert_path" ]]; then
    return 1  # Needs renewal if doesn't exist
  fi
  
  local expiry_date=$(openssl x509 -in "$cert_path" -noout -enddate 2>/dev/null | cut -d= -f2)
  if [[ -z "$expiry_date" ]]; then
    return 1  # Needs renewal if can't read
  fi
  
  local expiry_epoch=$(date -d "$expiry_date" +%s 2>/dev/null || date -j -f "%b %d %T %Y %Z" "$expiry_date" +%s 2>/dev/null || echo "0")
  local now_epoch=$(date +%s)
  local days_until_expiry=$(((expiry_epoch - now_epoch) / 86400))
  
  if [[ $days_until_expiry -lt $threshold_days ]]; then
    log "WARN: Certificate expires in $days_until_expiry days (threshold: $threshold_days)"
    return 0  # Needs renewal
  else
    log "INFO: Certificate valid for $days_until_expiry more days"
    return 1  # No renewal needed
  fi
}

# Check and renew if needed
RENEWED=false

# Check main certificate
if check_cert_expiry "nginx/ssl/cert.pem" 7; then
  log "INFO: Certificate renewal needed, attempting renewal..."
  
  # Only renew public certificates automatically
  if [[ -n "${DNS_PROVIDER:-}" ]]; then
    if nself ssl renew >> "$LOG_FILE" 2>&1; then
      log "SUCCESS: Certificate renewed successfully"
      RENEWED=true
      
      # Restart nginx to apply new certificate
      if docker compose ps nginx | grep -q "Up"; then
        log "INFO: Restarting nginx to apply new certificate..."
        docker compose restart nginx >> "$LOG_FILE" 2>&1
        log "SUCCESS: Nginx restarted"
      fi
    else
      log "ERROR: Certificate renewal failed"
    fi
  else
    log "INFO: Local certificates detected, no auto-renewal needed"
  fi
fi

# Health check after renewal
if [[ "$RENEWED" == "true" ]]; then
  sleep 5
  if curl -s -k "https://$(echo $BASE_DOMAIN | head -1)" > /dev/null; then
    log "SUCCESS: SSL renewal and restart completed successfully"
  else
    log "WARN: SSL may not be working properly after renewal"
  fi
fi

log "INFO: Daily SSL check completed"
EOF

  chmod +x "$renewal_script"

  # Add to crontab if not already present
  local cron_line="0 3 * * * $renewal_script"
  if ! crontab -l 2>/dev/null | grep -q "nself-ssl-auto-renewal"; then
    (
      crontab -l 2>/dev/null || true
      echo "$cron_line"
    ) | crontab -
    log_success "✓ Automatic SSL renewal scheduled (daily at 3 AM)"
    log_info "  • Checks certificates daily"
    log_info "  • Renews 7+ days before expiry"
    log_info "  • Logs to /var/log/nself-ssl-auto.log"
  fi
}

# Check and install trust if needed
check_and_install_trust() {
  local mkcert_cmd
  if mkcert_cmd="$(ssl::get_mkcert 2>/dev/null)"; then
    if ! $mkcert_cmd -install -check 2>/dev/null; then
      log_info "🔐 Installing SSL root certificate for trusted HTTPS..."
      if $mkcert_cmd -install 2>/dev/null; then
        log_success "✓ SSL root certificate installed - browsers will show green locks!"
      else
        log_warning "⚠ Could not auto-install root certificate"
        log_info "  Run 'nself trust' manually to install for green browser locks"
      fi
    else
      log_debug "SSL root certificate already trusted"
    fi
  fi
}

# Auto-detect and configure microservice domains
auto_detect_microservices() {
  local services=()
  local base_domain="${BASE_DOMAIN:-local.nself.org}"

  # Check docker-compose for custom services
  if [[ -f "docker-compose.yml" ]]; then
    # Extract service names that might need SSL
    local custom_services=$(grep -E "^[[:space:]]*[a-zA-Z][a-zA-Z0-9_-]*:" docker-compose.yml |
      sed 's/^[[:space:]]*//' | sed 's/:.*//' |
      grep -v -E "^(postgres|redis|mailpit)$")

    while IFS= read -r service; do
      if [[ -n "$service" ]]; then
        services+=("$service.$base_domain")
      fi
    done <<<"$custom_services"
  fi

  # Check for NestJS services
  if [[ "${NESTJS_ENABLED:-false}" == "true" ]] && [[ -n "${NESTJS_SERVICES:-}" ]]; then
    IFS=',' read -ra nestjs_services <<<"${NESTJS_SERVICES}"
    for service in "${nestjs_services[@]}"; do
      services+=("$service.$base_domain")
    done
  fi

  # Check for Go services
  if [[ "${GOLANG_ENABLED:-false}" == "true" ]] && [[ -n "${GOLANG_SERVICES:-}" ]]; then
    IFS=',' read -ra go_services <<<"${GOLANG_SERVICES}"
    for service in "${go_services[@]}"; do
      services+=("$service.$base_domain")
    done
  fi

  # Check for Python services
  if [[ "${PYTHON_ENABLED:-false}" == "true" ]] && [[ -n "${PYTHON_SERVICES:-}" ]]; then
    IFS=',' read -ra python_services <<<"${PYTHON_SERVICES}"
    for service in "${python_services[@]}"; do
      services+=("$service.$base_domain")
    done
  fi

  printf "%s\n" "${services[@]}" | sort -u
}

# Main auto-SSL function called during build
auto_ssl_build_integration() {
  log_info "🔄 Checking SSL configuration..."

  # Skip if SSL is explicitly disabled
  if [[ "${SSL_ENABLED:-true}" == "false" ]]; then
    log_info "SSL disabled in configuration, skipping"
    return 0
  fi

  # Check if certificates exist and are valid
  local needs_ssl=false

  if [[ ! -f "nginx/ssl/cert.pem" ]] || [[ ! -f "nginx/ssl/privkey.pem" ]]; then
    needs_ssl=true
    log_debug "SSL certificates not found"
  else
    # Check if certificates expire within 7 days
    local expiry_date=$(openssl x509 -in "nginx/ssl/cert.pem" -noout -enddate 2>/dev/null | cut -d= -f2)
    if [[ -n "$expiry_date" ]]; then
      local expiry_epoch=$(date -d "$expiry_date" +%s 2>/dev/null || date -j -f "%b %d %T %Y %Z" "$expiry_date" +%s 2>/dev/null || echo "0")
      local now_epoch=$(date +%s)
      local days_until_expiry=$(((expiry_epoch - now_epoch) / 86400))

      if [[ $days_until_expiry -lt 7 ]]; then
        needs_ssl=true
        log_debug "SSL certificates expire in $days_until_expiry days"
      fi
    fi
  fi

  # Set up SSL if needed
  if [[ "$needs_ssl" == "true" ]]; then
    if auto_setup_ssl; then
      log_success "✓ SSL configured automatically"
    else
      log_warning "⚠ SSL setup had issues, but build can continue"
      log_info "  Run 'nself ssl bootstrap' manually if needed"
    fi
  else
    log_debug "SSL certificates are current"

    # Still set up auto-renewal if not already done
    setup_automatic_renewal
    check_and_install_trust
  fi

  return 0
}

# Export functions
export -f auto_detect_domains
export -f auto_setup_ssl
export -f setup_automatic_renewal
export -f check_and_install_trust
export -f auto_detect_microservices
export -f auto_ssl_build_integration
