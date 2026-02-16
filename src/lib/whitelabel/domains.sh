#!/usr/bin/env bash
# nself Custom Domains System
# Manages custom domains, SSL certificates, DNS verification, and health checks
# Part of Sprint 14: White-Label & Customization (60pts) for v0.9.0
# POSIX-compliant, no Bash 4+ features, cross-platform compatible


# Get the directory where this script is located (namespaced to avoid clobbering caller globals)
WHITELABEL_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

_WHITELABEL_LIB_ROOT="$(dirname "$WHITELABEL_LIB_DIR")"

# Source dependencies
source "$_WHITELABEL_LIB_ROOT/utils/display.sh" 2>/dev/null || true
source "$_WHITELABEL_LIB_ROOT/utils/platform-compat.sh" 2>/dev/null || true

# Domain configuration
PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$_WHITELABEL_LIB_ROOT/../.." && pwd)}"
readonly DOMAINS_DIR="${DOMAINS_DIR:-${PROJECT_ROOT}/branding/domains}"
readonly DOMAINS_CONFIG="${DOMAINS_CONFIG:-${DOMAINS_DIR}/domains.json}"
readonly SSL_DIR="${SSL_DIR:-${DOMAINS_DIR}/ssl}"
readonly DNS_CHALLENGES_DIR="${DNS_CHALLENGES_DIR:-${DOMAINS_DIR}/dns-challenges}"

# SSL certificate providers
readonly SSL_PROVIDER="${SSL_PROVIDER:-letsencrypt}"
readonly SSL_EMAIL="${SSL_EMAIL:-admin@localhost}"

# DNS verification settings
readonly DNS_TIMEOUT=300
readonly DNS_CHECK_INTERVAL=10
readonly DNS_MAX_ATTEMPTS=30

# SSL certificate renewal thresholds (in days)
readonly SSL_RENEWAL_THRESHOLD=30
readonly SSL_RENEWAL_CRITICAL=7

# ============================================================================
# Domain System Initialization
# ============================================================================

initialize_domains_system() {
  log_info "Initializing custom domains system..."

  # Create domain directories with proper permissions
  mkdir -p "$DOMAINS_DIR"
  mkdir -p "$SSL_DIR"
  mkdir -p "$DNS_CHALLENGES_DIR"

  # Set secure permissions
  chmod 755 "$DOMAINS_DIR"
  chmod 700 "$SSL_DIR" # SSL directory must be secure
  chmod 755 "$DNS_CHALLENGES_DIR"

  # Create domains configuration file
  if [[ ! -f "$DOMAINS_CONFIG" ]]; then
    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u "+%Y-%m-%dT%H:%M:%SZ")

    cat >"$DOMAINS_CONFIG" <<EOF
{
  "version": "1.0.0",
  "domains": [],
  "defaultDomain": null,
  "sslProvider": "${SSL_PROVIDER}",
  "autoRenew": true,
  "renewalThresholdDays": ${SSL_RENEWAL_THRESHOLD},
  "createdAt": "${timestamp}",
  "updatedAt": "${timestamp}"
}
EOF
    chmod 644 "$DOMAINS_CONFIG"
  fi

  log_success "Custom domains system initialized"
  return 0
}

# ============================================================================
# Domain Management
# ============================================================================

add_custom_domain() {
  local domain="$1"
  local is_primary="${2:-false}"

  # Validate domain format
  if ! validate_domain_format "$domain"; then
    log_error "Invalid domain format: $domain"
    return 1
  fi

  log_info "Adding custom domain: $domain"

  # Initialize if needed
  [[ ! -f "$DOMAINS_CONFIG" ]] && initialize_domains_system

  # Check if domain already exists
  if domain_exists "$domain"; then
    log_warning "Domain already exists: $domain"
    return 0
  fi

  # Add domain to configuration
  if ! command -v jq >/dev/null 2>&1; then
    log_error "jq is required for domain management"
    printf "\nInstall jq:\n"
    printf "  macOS: brew install jq\n"
    printf "  Ubuntu/Debian: sudo apt install jq\n"
    printf "  RHEL/CentOS: sudo yum install jq\n"
    return 1
  fi

  local temp_file
  temp_file=$(mktemp)

  local timestamp
  timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u "+%Y-%m-%dT%H:%M:%SZ")

  local domain_entry
  domain_entry=$(
    cat <<EOF
{
  "domain": "$domain",
  "status": "pending",
  "verified": false,
  "sslEnabled": false,
  "sslIssuer": null,
  "sslExpiryDate": null,
  "sslAutoRenew": true,
  "dnsVerified": false,
  "healthStatus": "unknown",
  "lastHealthCheck": null,
  "isPrimary": $is_primary,
  "verificationToken": null,
  "createdAt": "$timestamp",
  "updatedAt": "$timestamp"
}
EOF
  )

  jq --argjson entry "$domain_entry" \
    '.domains += [$entry] | .updatedAt = $entry.updatedAt' \
    "$DOMAINS_CONFIG" >"$temp_file" && mv "$temp_file" "$DOMAINS_CONFIG"

  if [[ "$is_primary" == "true" ]]; then
    temp_file=$(mktemp)
    jq --arg domain "$domain" \
      '.defaultDomain = $domain' \
      "$DOMAINS_CONFIG" >"$temp_file" && mv "$temp_file" "$DOMAINS_CONFIG"
  fi

  log_success "Domain added: $domain"
  printf "\n${COLOR_BLUE}Next steps:${COLOR_RESET}\n"
  printf "  1. Configure DNS: Point %s to your server IP\n" "$domain"
  printf "  2. Verify domain: nself whitelabel domain verify %s\n" "$domain"
  printf "  3. Provision SSL: nself whitelabel domain ssl %s\n" "$domain"

  return 0
}

remove_custom_domain() {
  local domain="$1"
  local force="${2:-false}"

  log_info "Removing custom domain: $domain"

  if ! domain_exists "$domain"; then
    log_error "Domain not found: $domain"
    return 1
  fi

  # Check if this is the primary domain
  if is_primary_domain "$domain" && [[ "$force" != "true" ]]; then
    log_warning "This is the primary domain"
    printf "To remove it, use: --force\n"
    return 1
  fi

  # Remove SSL certificates
  local ssl_cert_dir="${SSL_DIR}/${domain}"
  if [[ -d "$ssl_cert_dir" ]]; then
    log_info "Removing SSL certificates..."
    rm -rf "$ssl_cert_dir"
  fi

  # Remove DNS challenge files
  local challenge_file="${DNS_CHALLENGES_DIR}/${domain}.txt"
  [[ -f "$challenge_file" ]] && rm -f "$challenge_file"

  # Remove auto-renewal script
  local renewal_script="${SSL_DIR}/renew-${domain}.sh"
  [[ -f "$renewal_script" ]] && rm -f "$renewal_script"

  # Remove from configuration
  if command -v jq >/dev/null 2>&1; then
    local temp_file
    temp_file=$(mktemp)
    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u "+%Y-%m-%dT%H:%M:%SZ")

    jq --arg domain "$domain" --arg timestamp "$timestamp" \
      '.domains = [.domains[] | select(.domain != $domain)] | .updatedAt = $timestamp' \
      "$DOMAINS_CONFIG" >"$temp_file" && mv "$temp_file" "$DOMAINS_CONFIG"

    # Clear default domain if it was this one
    if is_primary_domain "$domain"; then
      temp_file=$(mktemp)
      jq '.defaultDomain = null' "$DOMAINS_CONFIG" >"$temp_file" && mv "$temp_file" "$DOMAINS_CONFIG"
    fi
  fi

  log_success "Domain removed: $domain"

  return 0
}

list_custom_domains() {
  printf "${COLOR_CYAN}Custom Domains${COLOR_RESET}\n"
  printf "%s\n\n" "$(printf '%.s=' {1..70})"

  if [[ ! -f "$DOMAINS_CONFIG" ]]; then
    log_warning "No domains configured"
    printf "\nAdd a domain with:\n"
    printf "  nself whitelabel domain add <domain>\n"
    return 0
  fi

  if ! command -v jq >/dev/null 2>&1; then
    log_error "jq is required for domain management"
    return 1
  fi

  local domain_count
  domain_count=$(jq -r '.domains | length' "$DOMAINS_CONFIG")

  if [[ "$domain_count" -eq 0 ]]; then
    log_warning "No domains configured"
    return 0
  fi

  # List each domain with details
  local i=0
  while [[ $i -lt $domain_count ]]; do
    local domain status verified ssl_enabled ssl_issuer ssl_expiry health is_primary

    domain=$(jq -r ".domains[$i].domain" "$DOMAINS_CONFIG")
    status=$(jq -r ".domains[$i].status" "$DOMAINS_CONFIG")
    verified=$(jq -r ".domains[$i].verified" "$DOMAINS_CONFIG")
    ssl_enabled=$(jq -r ".domains[$i].sslEnabled" "$DOMAINS_CONFIG")
    ssl_issuer=$(jq -r ".domains[$i].sslIssuer" "$DOMAINS_CONFIG")
    ssl_expiry=$(jq -r ".domains[$i].sslExpiryDate" "$DOMAINS_CONFIG")
    health=$(jq -r ".domains[$i].healthStatus" "$DOMAINS_CONFIG")
    is_primary=$(jq -r ".domains[$i].isPrimary" "$DOMAINS_CONFIG")

    # Print domain header
    printf "${COLOR_BOLD}%s${COLOR_RESET}" "$domain"
    [[ "$is_primary" == "true" ]] && printf " ${COLOR_CYAN}(PRIMARY)${COLOR_RESET}"
    printf "\n"

    # Print status
    printf "  Status:     "
    case "$status" in
      verified) printf "${COLOR_GREEN}%s${COLOR_RESET}\n" "$status" ;;
      pending) printf "${COLOR_YELLOW}%s${COLOR_RESET}\n" "$status" ;;
      *) printf "%s\n" "$status" ;;
    esac

    # Print DNS verification
    printf "  DNS:        "
    [[ "$verified" == "true" ]] && printf "${COLOR_GREEN}✓ Verified${COLOR_RESET}\n" || printf "${COLOR_YELLOW}Pending${COLOR_RESET}\n"

    # Print SSL status
    printf "  SSL:        "
    if [[ "$ssl_enabled" == "true" ]]; then
      printf "${COLOR_GREEN}✓ Enabled${COLOR_RESET} (%s)\n" "$ssl_issuer"
      if [[ "$ssl_expiry" != "null" ]]; then
        printf "  Expires:    %s\n" "$ssl_expiry"
      fi
    else
      printf "${COLOR_YELLOW}Not configured${COLOR_RESET}\n"
    fi

    # Print health status
    printf "  Health:     "
    case "$health" in
      healthy) printf "${COLOR_GREEN}%s${COLOR_RESET}\n" "$health" ;;
      degraded) printf "${COLOR_YELLOW}%s${COLOR_RESET}\n" "$health" ;;
      unhealthy) printf "${COLOR_RED}%s${COLOR_RESET}\n" "$health" ;;
      *) printf "%s\n" "$health" ;;
    esac

    printf "\n"
    i=$((i + 1))
  done

  printf "%s\n" "$(printf '%.s=' {1..70})"
  printf "Total domains: %d\n" "$domain_count"

  return 0
}

# ============================================================================
# DNS Verification
# ============================================================================

verify_domain() {
  local domain="$1"
  local method="${2:-auto}" # auto, txt, or a-record
  local skip_wait="${3:-false}"

  log_info "Verifying domain: $domain"

  if ! domain_exists "$domain"; then
    log_error "Domain not found: $domain"
    return 1
  fi

  # Generate verification token
  local verification_token
  verification_token=$(generate_verification_token "$domain")

  # Store verification token
  local challenge_file="${DNS_CHALLENGES_DIR}/${domain}.txt"
  printf "%s" "$verification_token" >"$challenge_file"
  chmod 644 "$challenge_file"

  # Update domain with verification token
  update_domain_field "$domain" "verificationToken" "$verification_token"

  # Display verification instructions
  printf "\n${COLOR_BLUE}DNS Verification Instructions:${COLOR_RESET}\n"
  printf "%s\n\n" "$(printf '%.s-' {1..70})"

  if [[ "$method" == "txt" ]] || [[ "$method" == "auto" ]]; then
    printf "Option 1: Add TXT record (recommended):\n\n"
    printf "  Host/Name:  _nself-verification\n"
    printf "  Type:       TXT\n"
    printf "  Value:      %s\n" "$verification_token"
    printf "  TTL:        300 (or lowest available)\n\n"
  fi

  if [[ "$method" == "a-record" ]] || [[ "$method" == "auto" ]]; then
    printf "Option 2: Point domain to server:\n\n"
    printf "  Host/Name:  %s (or @)\n" "$domain"
    printf "  Type:       A\n"
    printf "  Value:      <your-server-ip>\n"
    printf "  TTL:        300 (or lowest available)\n\n"
  fi

  printf "%s\n" "$(printf '%.s-' {1..70})"

  if [[ "$skip_wait" == "true" ]]; then
    log_info "Skipping automatic verification. Verify later with:"
    printf "  nself whitelabel domain verify %s --check\n" "$domain"
    return 0
  fi

  # Attempt DNS verification
  log_info "Checking DNS propagation (this may take a few minutes)..."
  printf "\n"

  local verified=0
  local attempts=0

  while [[ $verified -eq 0 ]] && [[ $attempts -lt $DNS_MAX_ATTEMPTS ]]; do
    # Try TXT record verification first
    if verify_txt_record "$domain" "$verification_token"; then
      log_success "TXT record verified"
      verified=1
      break
    fi

    # Try A record resolution
    if check_dns_propagation "$domain"; then
      log_success "A record verified"
      verified=1
      break
    fi

    attempts=$((attempts + 1))
    if [[ $attempts -lt $DNS_MAX_ATTEMPTS ]]; then
      printf "  Attempt %d/%d - waiting %ds for DNS propagation...\n" "$attempts" "$DNS_MAX_ATTEMPTS" "$DNS_CHECK_INTERVAL"
      sleep "$DNS_CHECK_INTERVAL"
    fi
  done

  if [[ $verified -eq 1 ]]; then
    # Update domain status
    update_domain_status "$domain" "verified" "true"
    update_domain_status "$domain" "dnsVerified" "true"
    update_domain_status "$domain" "status" "verified"

    log_success "Domain verified successfully: $domain"
    printf "\n${COLOR_BLUE}Next step:${COLOR_RESET}\n"
    printf "  Provision SSL: nself whitelabel domain ssl %s\n" "$domain"
    return 0
  else
    log_warning "DNS verification timeout after $DNS_MAX_ATTEMPTS attempts"
    printf "\nDNS propagation can take up to 48 hours. You can verify later with:\n"
    printf "  nself whitelabel domain verify %s --check\n" "$domain"
    return 1
  fi
}

check_dns_propagation() {
  local domain="$1"
  local check_type="${2:-any}" # any, ipv4, ipv6

  # Try multiple DNS lookup tools
  if command -v dig >/dev/null 2>&1; then
    # dig is the most reliable
    if [[ "$check_type" == "ipv6" ]]; then
      dig +short "$domain" AAAA 2>/dev/null | grep -qE '^[0-9a-f:]+$' && return 0
    else
      dig +short "$domain" A 2>/dev/null | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' && return 0
    fi
  elif command -v nslookup >/dev/null 2>&1; then
    # nslookup fallback
    if nslookup "$domain" 2>/dev/null | grep -qE 'Address: [0-9]+\.[0-9]+'; then
      return 0
    fi
  elif command -v host >/dev/null 2>&1; then
    # host fallback
    if host "$domain" 2>/dev/null | grep -q 'has address'; then
      return 0
    fi
  fi

  return 1
}

verify_txt_record() {
  local domain="$1"
  local expected_token="$2"

  # Check for TXT record verification
  if command -v dig >/dev/null 2>&1; then
    local txt_record
    txt_record=$(dig +short "_nself-verification.${domain}" TXT 2>/dev/null | tr -d '"')
    if [[ "$txt_record" == "$expected_token" ]]; then
      return 0
    fi
  elif command -v nslookup >/dev/null 2>&1; then
    local txt_record
    txt_record=$(nslookup -type=TXT "_nself-verification.${domain}" 2>/dev/null | grep -oP '"\K[^"]+' | head -1)
    if [[ "$txt_record" == "$expected_token" ]]; then
      return 0
    fi
  elif command -v host >/dev/null 2>&1; then
    local txt_record
    txt_record=$(host -t TXT "_nself-verification.${domain}" 2>/dev/null | grep -oP '"\K[^"]+' | head -1)
    if [[ "$txt_record" == "$expected_token" ]]; then
      return 0
    fi
  fi

  return 1
}

generate_verification_token() {
  local domain="${1:-}"

  # Generate cryptographically secure random verification token
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  elif [[ -f /dev/urandom ]]; then
    # Fallback using /dev/urandom
    tr -dc 'a-f0-9' </dev/urandom 2>/dev/null | head -c 64
  else
    # Last resort: date and domain-based token (not cryptographically secure)
    local timestamp
    timestamp=$(date +%s)
    printf "%s-%s" "$domain" "$timestamp" | sha256sum 2>/dev/null | awk '{print $1}' || printf "nself-verify-%s-%s" "$domain" "$timestamp"
  fi
}

get_domain_ip() {
  local domain="$1"

  if command -v dig >/dev/null 2>&1; then
    dig +short "$domain" A 2>/dev/null | head -1
  elif command -v nslookup >/dev/null 2>&1; then
    nslookup "$domain" 2>/dev/null | grep -oE 'Address: [0-9.]+' | grep -oE '[0-9.]+$' | head -1
  elif command -v host >/dev/null 2>&1; then
    host "$domain" 2>/dev/null | grep -oE 'has address [0-9.]+' | grep -oE '[0-9.]+' | head -1
  fi
}

# ============================================================================
# SSL Certificate Management
# ============================================================================

provision_ssl() {
  local domain="$1"
  local provider="${SSL_PROVIDER}"
  local auto_renew="true"
  local force="false"
  local email="${SSL_EMAIL}"

  shift
  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --provider)
        provider="$2"
        shift 2
        ;;
      --email)
        email="$2"
        shift 2
        ;;
      --no-auto-renew)
        auto_renew="false"
        shift
        ;;
      --force)
        force="true"
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  log_info "Provisioning SSL certificate for: $domain"

  if ! domain_exists "$domain"; then
    log_error "Domain not found: $domain"
    return 1
  fi

  # Check if domain is verified
  if ! is_domain_verified "$domain" && [[ "$force" != "true" ]]; then
    log_warning "Domain not verified"
    printf "\nVerify domain first with:\n"
    printf "  nself whitelabel domain verify %s\n" "$domain"
    printf "\nOr provision anyway with --force flag\n"
    return 1
  fi

  # Check if SSL already exists
  if has_ssl_certificate "$domain" && [[ "$force" != "true" ]]; then
    log_warning "SSL certificate already exists for: $domain"
    printf "\nTo regenerate, use --force flag\n"
    printf "To renew, use: nself whitelabel domain ssl-renew %s\n" "$domain"
    return 0
  fi

  # Create SSL directory for domain
  local ssl_domain_dir="${SSL_DIR}/${domain}"
  mkdir -p "$ssl_domain_dir"
  chmod 700 "$ssl_domain_dir"

  # Provision based on provider
  case "$provider" in
    letsencrypt)
      provision_letsencrypt_ssl "$domain" "$ssl_domain_dir" "$auto_renew" "$email"
      ;;
    selfsigned)
      provision_selfsigned_ssl "$domain" "$ssl_domain_dir"
      ;;
    custom)
      provision_custom_ssl "$domain" "$ssl_domain_dir"
      return $?
      ;;
    *)
      log_error "Unknown SSL provider: $provider"
      printf "\nAvailable providers: letsencrypt, selfsigned, custom\n"
      return 1
      ;;
  esac

  local result=$?

  if [[ $result -eq 0 ]]; then
    # Update nginx configuration to use the certificate
    update_nginx_ssl_config "$domain" "$ssl_domain_dir"
  fi

  return $result
}

provision_letsencrypt_ssl() {
  local domain="$1"
  local ssl_dir="$2"
  local auto_renew="$3"
  local email="$4"

  log_info "Using Let's Encrypt for SSL..."

  # Check if certbot is available
  if ! command -v certbot >/dev/null 2>&1; then
    log_warning "certbot not found"
    printf "\nInstall certbot:\n"
    printf "  macOS:          brew install certbot\n"
    printf "  Ubuntu/Debian:  sudo apt install certbot\n"
    printf "  RHEL/CentOS:    sudo yum install certbot\n\n"
    log_info "Falling back to self-signed certificate..."
    provision_selfsigned_ssl "$domain" "$ssl_dir"
    return 0
  fi

  # Validate email
  if [[ -z "$email" ]] || [[ "$email" == "admin@localhost" ]]; then
    log_error "Valid email required for Let's Encrypt"
    printf "Set SSL_EMAIL environment variable or use --email flag\n"
    return 1
  fi

  log_info "Requesting certificate from Let's Encrypt..."
  printf "  Domain: %s\n" "$domain"
  printf "  Email:  %s\n\n" "$email"

  # Determine certbot method based on running web server
  local certbot_method="standalone"
  local webroot_path="${PROJECT_ROOT}/.well-known/acme-challenge"

  # Check if nginx is running (prefer webroot method)
  if docker ps 2>/dev/null | grep -q nginx; then
    certbot_method="webroot"
    mkdir -p "$webroot_path"
    log_info "Using webroot method (nginx is running)"
  else
    log_info "Using standalone method (port 80 must be available)"
  fi

  # Build certbot command
  local certbot_cmd="certbot certonly --non-interactive --agree-tos"
  certbot_cmd="$certbot_cmd --email $email"
  certbot_cmd="$certbot_cmd -d $domain"

  if [[ "$certbot_method" == "webroot" ]]; then
    certbot_cmd="$certbot_cmd --webroot -w $webroot_path"
  else
    certbot_cmd="$certbot_cmd --standalone"
  fi

  # Execute certbot
  if $certbot_cmd; then
    # Copy certificates from Let's Encrypt directory to our SSL directory
    local le_live_dir="/etc/letsencrypt/live/${domain}"

    if [[ -d "$le_live_dir" ]]; then
      cp "${le_live_dir}/fullchain.pem" "${ssl_dir}/cert.pem"
      cp "${le_live_dir}/privkey.pem" "${ssl_dir}/key.pem"
      cp "${le_live_dir}/chain.pem" "${ssl_dir}/chain.pem"

      # Set secure permissions
      chmod 644 "${ssl_dir}/cert.pem"
      chmod 600 "${ssl_dir}/key.pem"
      chmod 644 "${ssl_dir}/chain.pem"

      # Get expiry date from certificate
      local expiry_date
      if command -v openssl >/dev/null 2>&1; then
        local expiry_str
        expiry_str=$(openssl x509 -enddate -noout -in "${ssl_dir}/cert.pem" | cut -d'=' -f2)
        if is_macos; then
          expiry_date=$(date -j -f "%b %d %T %Y %Z" "$expiry_str" -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null)
        else
          expiry_date=$(date -d "$expiry_str" -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null)
        fi
      fi

      # Fallback to 90 days if date parsing failed
      if [[ -z "$expiry_date" ]]; then
        if is_macos; then
          expiry_date=$(date -v+90d -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null)
        else
          expiry_date=$(date -d "+90 days" -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null)
        fi
      fi

      # Update domain configuration
      update_domain_ssl_status "$domain" "true" "letsencrypt" "$expiry_date"
      update_domain_status "$domain" "sslEnabled" "true"
      update_domain_status "$domain" "sslAutoRenew" "$auto_renew"

      log_success "SSL certificate provisioned successfully"
      printf "  Issuer:     Let's Encrypt\n"
      printf "  Certificate: %s/cert.pem\n" "$ssl_dir"
      printf "  Private key: %s/key.pem\n" "$ssl_dir"
      printf "  Expires:    %s\n" "$expiry_date"

      if [[ "$auto_renew" == "true" ]]; then
        setup_ssl_auto_renewal "$domain"
      fi

      return 0
    else
      log_error "Let's Encrypt succeeded but certificates not found in $le_live_dir"
      return 1
    fi
  else
    log_error "Let's Encrypt certificate request failed"
    printf "\nCommon issues:\n"
    printf "  - Domain not pointing to this server\n"
    printf "  - Port 80 not accessible from internet\n"
    printf "  - Rate limit reached (5 failures per hour)\n\n"
    log_info "Falling back to self-signed certificate..."
    provision_selfsigned_ssl "$domain" "$ssl_dir"
    return 0
  fi
}

provision_selfsigned_ssl() {
  local domain="$1"
  local ssl_dir="$2"
  local days="${3:-365}"

  log_info "Generating self-signed certificate..."

  if ! command -v openssl >/dev/null 2>&1; then
    log_error "openssl is required for SSL certificate generation"
    return 1
  fi

  # Set restrictive umask BEFORE generating keys to ensure secure permissions
  local old_umask
  old_umask=$(umask)
  umask 077

  # Generate private key
  openssl genrsa -out "${ssl_dir}/key.pem" 2048 2>/dev/null || {
    umask "$old_umask"
    log_error "Failed to generate private key"
    return 1
  }

  # Generate certificate with SAN (Subject Alternative Name) for modern browsers
  local openssl_conf
  openssl_conf=$(mktemp)

  cat >"$openssl_conf" <<EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
req_extensions = v3_req

[dn]
CN = ${domain}
O = nself
C = US

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = ${domain}
DNS.2 = *.${domain}
EOF

  # Generate self-signed certificate
  openssl req -new -x509 -nodes -days "$days" \
    -key "${ssl_dir}/key.pem" \
    -out "${ssl_dir}/cert.pem" \
    -config "$openssl_conf" \
    -extensions v3_req 2>/dev/null || {
    rm -f "$openssl_conf"
    umask "$old_umask"
    log_error "Failed to generate self-signed certificate"
    return 1
  }

  rm -f "$openssl_conf"

  # Restore original umask after key generation
  umask "$old_umask"

  # Create chain file (empty for self-signed)
  touch "${ssl_dir}/chain.pem"

  # Ensure private key has correct permissions immediately after generation
  chmod 600 "${ssl_dir}/key.pem"
  chmod 644 "${ssl_dir}/cert.pem"
  chmod 644 "${ssl_dir}/chain.pem"

  # Calculate expiry date
  local expiry_date
  if is_macos; then
    # macOS date command
    expiry_date=$(date -v+${days}d -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null)
  else
    # GNU date command (Linux)
    expiry_date=$(date -d "+${days} days" -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null)
  fi

  # Fallback if date command failed
  if [[ -z "$expiry_date" ]]; then
    expiry_date=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  fi

  # Update domain configuration
  update_domain_ssl_status "$domain" "true" "selfsigned" "$expiry_date"
  update_domain_status "$domain" "sslEnabled" "true"

  log_success "Self-signed certificate generated"
  printf "  Certificate: %s/cert.pem\n" "$ssl_dir"
  printf "  Private key: %s/key.pem\n" "$ssl_dir"
  printf "  Valid for:   %d days\n" "$days"
  printf "  Expires:     %s\n\n" "$expiry_date"

  log_warning "Self-signed certificates are not trusted by browsers"
  printf "For production, use Let's Encrypt:\n"
  printf "  nself whitelabel domain ssl %s --provider letsencrypt\n" "$domain"

  return 0
}

provision_custom_ssl() {
  local domain="$1"
  local ssl_dir="$2"

  log_info "Custom SSL certificate setup"
  printf "\nPlace your certificate files in:\n"
  printf "  Certificate:     %s/cert.pem\n" "$ssl_dir"
  printf "  Private key:     %s/key.pem\n" "$ssl_dir"
  printf "  Chain (optional): %s/chain.pem\n\n" "$ssl_dir"

  printf "After placing files, verify with:\n"
  printf "  nself whitelabel domain ssl-verify %s\n" "$domain"

  return 0
}

# ============================================================================
# SSL Auto-Renewal
# ============================================================================

setup_ssl_auto_renewal() {
  local domain="$1"

  log_info "Setting up SSL auto-renewal for: $domain"

  # Create renewal script
  local renewal_script="${SSL_DIR}/renew-${domain}.sh"

  cat >"$renewal_script" <<'RENEWAL_SCRIPT_EOF'
#!/usr/bin/env bash
# SSL auto-renewal script for domain
# Generated by nself whitelabel


DOMAIN="DOMAIN_PLACEHOLDER"
SSL_DIR="SSL_DIR_PLACEHOLDER"
PROJECT_ROOT="PROJECT_ROOT_PLACEHOLDER"

log_info() {
  printf "[%s] INFO: %s\n" "$(date -u +"%Y-%m-%d %H:%M:%S")" "$1"
}

log_error() {
  printf "[%s] ERROR: %s\n" "$(date -u +"%Y-%m-%d %H:%M:%S")" "$1" >&2
}

# Check if certificate needs renewal (within 30 days of expiry)
if [[ -f "${SSL_DIR}/${DOMAIN}/cert.pem" ]]; then
  if command -v openssl >/dev/null 2>&1; then
    if openssl x509 -checkend 2592000 -noout -in "${SSL_DIR}/${DOMAIN}/cert.pem" 2>/dev/null; then
      log_info "Certificate for ${DOMAIN} is still valid, no renewal needed"
      exit 0
    fi
  fi
fi

log_info "Renewing SSL certificate for ${DOMAIN}..."

# Attempt renewal with certbot
if command -v certbot >/dev/null 2>&1; then
  if certbot renew --cert-name "${DOMAIN}" --quiet; then
    # Copy renewed certificates
    if [[ -d "/etc/letsencrypt/live/${DOMAIN}" ]]; then
      cp "/etc/letsencrypt/live/${DOMAIN}/fullchain.pem" "${SSL_DIR}/${DOMAIN}/cert.pem"
      cp "/etc/letsencrypt/live/${DOMAIN}/privkey.pem" "${SSL_DIR}/${DOMAIN}/key.pem"
      cp "/etc/letsencrypt/live/${DOMAIN}/chain.pem" "${SSL_DIR}/${DOMAIN}/chain.pem"

      chmod 644 "${SSL_DIR}/${DOMAIN}/cert.pem"
      chmod 600 "${SSL_DIR}/${DOMAIN}/key.pem"
      chmod 644 "${SSL_DIR}/${DOMAIN}/chain.pem"

      log_info "SSL certificate renewed successfully"

      # Reload nginx if running
      if docker ps 2>/dev/null | grep -q nginx; then
        docker exec nginx nginx -s reload 2>/dev/null && log_info "Nginx reloaded"
      fi

      exit 0
    fi
  fi
fi

log_error "SSL renewal failed for ${DOMAIN}"
exit 1
RENEWAL_SCRIPT_EOF

  # Replace placeholders
  if is_macos; then
    sed -i '' "s|DOMAIN_PLACEHOLDER|${domain}|g" "$renewal_script"
    sed -i '' "s|SSL_DIR_PLACEHOLDER|${SSL_DIR}|g" "$renewal_script"
    sed -i '' "s|PROJECT_ROOT_PLACEHOLDER|${PROJECT_ROOT}|g" "$renewal_script"
  else
    sed -i "s|DOMAIN_PLACEHOLDER|${domain}|g" "$renewal_script"
    sed -i "s|SSL_DIR_PLACEHOLDER|${SSL_DIR}|g" "$renewal_script"
    sed -i "s|PROJECT_ROOT_PLACEHOLDER|${PROJECT_ROOT}|g" "$renewal_script"
  fi

  chmod +x "$renewal_script"

  log_success "Auto-renewal configured"
  printf "\n${COLOR_BLUE}Setup automatic renewal:${COLOR_RESET}\n\n"
  printf "Option 1 - Crontab (runs twice daily):\n"
  printf "  crontab -e\n"
  printf "  Add: 0 0,12 * * * %s\n\n" "$renewal_script"

  printf "Option 2 - Systemd timer:\n"
  printf "  See: https://docs.nself.org/ssl-renewal\n\n"

  return 0
}

renew_ssl_certificate() {
  local domain="$1"
  local force="${2:-false}"

  log_info "Renewing SSL certificate for: $domain"

  if ! domain_exists "$domain"; then
    log_error "Domain not found: $domain"
    return 1
  fi

  if ! has_ssl_certificate "$domain"; then
    log_error "No SSL certificate found for: $domain"
    printf "Provision SSL first with:\n"
    printf "  nself whitelabel domain ssl %s\n" "$domain"
    return 1
  fi

  local ssl_dir="${SSL_DIR}/${domain}"
  local cert_file="${ssl_dir}/cert.pem"

  # Check if renewal is needed
  if [[ "$force" != "true" ]]; then
    if command -v openssl >/dev/null 2>&1; then
      # Check if certificate expires within 30 days
      if openssl x509 -checkend 2592000 -noout -in "$cert_file" 2>/dev/null; then
        log_info "Certificate is still valid for more than 30 days"
        printf "Use --force to renew anyway\n"
        return 0
      fi
    fi
  fi

  # Get SSL issuer to determine renewal method
  local issuer
  issuer=$(jq -r --arg domain "$domain" '.domains[] | select(.domain == $domain) | .sslIssuer' "$DOMAINS_CONFIG" 2>/dev/null)

  case "$issuer" in
    letsencrypt)
      renew_letsencrypt_certificate "$domain" "$ssl_dir"
      ;;
    selfsigned)
      log_info "Self-signed certificates cannot be renewed, generating new one..."
      provision_selfsigned_ssl "$domain" "$ssl_dir"
      ;;
    *)
      log_warning "Unknown issuer: $issuer"
      printf "Manually replace certificates in: %s\n" "$ssl_dir"
      return 1
      ;;
  esac
}

renew_letsencrypt_certificate() {
  local domain="$1"
  local ssl_dir="$2"

  if ! command -v certbot >/dev/null 2>&1; then
    log_error "certbot is required for renewal"
    return 1
  fi

  log_info "Renewing Let's Encrypt certificate..."

  if certbot renew --cert-name "$domain"; then
    # Copy renewed certificates
    local le_live_dir="/etc/letsencrypt/live/${domain}"

    if [[ -d "$le_live_dir" ]]; then
      cp "${le_live_dir}/fullchain.pem" "${ssl_dir}/cert.pem"
      cp "${le_live_dir}/privkey.pem" "${ssl_dir}/key.pem"
      cp "${le_live_dir}/chain.pem" "${ssl_dir}/chain.pem"

      chmod 644 "${ssl_dir}/cert.pem"
      chmod 600 "${ssl_dir}/key.pem"
      chmod 644 "${ssl_dir}/chain.pem"

      # Update expiry date
      local expiry_date
      if command -v openssl >/dev/null 2>&1; then
        local expiry_str
        expiry_str=$(openssl x509 -enddate -noout -in "${ssl_dir}/cert.pem" | cut -d'=' -f2)
        if is_macos; then
          expiry_date=$(date -j -f "%b %d %T %Y %Z" "$expiry_str" -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null)
        else
          expiry_date=$(date -d "$expiry_str" -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null)
        fi
      fi

      update_domain_ssl_status "$domain" "true" "letsencrypt" "$expiry_date"

      log_success "SSL certificate renewed successfully"
      printf "  Expires: %s\n" "$expiry_date"

      # Reload nginx if running
      reload_nginx_ssl

      return 0
    fi
  fi

  log_error "Certificate renewal failed"
  return 1
}

verify_ssl_certificate() {
  local domain="$1"

  log_info "Verifying SSL certificate for: $domain"

  if ! has_ssl_certificate "$domain"; then
    log_error "No SSL certificate found for: $domain"
    return 1
  fi

  local ssl_dir="${SSL_DIR}/${domain}"
  local cert_file="${ssl_dir}/cert.pem"
  local key_file="${ssl_dir}/key.pem"

  if ! command -v openssl >/dev/null 2>&1; then
    log_error "openssl is required for verification"
    return 1
  fi

  local all_checks_passed=0

  # Check 1: Certificate file readable
  printf "\n${COLOR_BLUE}1. Certificate file:${COLOR_RESET} "
  if [[ -r "$cert_file" ]]; then
    printf "${COLOR_GREEN}✓${COLOR_RESET} Readable\n"
  else
    printf "${COLOR_RED}✗${COLOR_RESET} Not readable\n"
    all_checks_passed=1
  fi

  # Check 2: Private key file readable
  printf "${COLOR_BLUE}2. Private key file:${COLOR_RESET} "
  if [[ -r "$key_file" ]]; then
    printf "${COLOR_GREEN}✓${COLOR_RESET} Readable\n"
  else
    printf "${COLOR_RED}✗${COLOR_RESET} Not readable\n"
    all_checks_passed=1
  fi

  # Check 3: Certificate validity
  printf "${COLOR_BLUE}3. Certificate validity:${COLOR_RESET} "
  if openssl x509 -checkend 0 -noout -in "$cert_file" 2>/dev/null; then
    printf "${COLOR_GREEN}✓${COLOR_RESET} Not expired\n"
  else
    printf "${COLOR_RED}✗${COLOR_RESET} Expired\n"
    all_checks_passed=1
  fi

  # Check 4: Certificate and key match
  printf "${COLOR_BLUE}4. Certificate-key match:${COLOR_RESET} "
  local cert_modulus key_modulus
  cert_modulus=$(openssl x509 -modulus -noout -in "$cert_file" 2>/dev/null | md5sum 2>/dev/null | awk '{print $1}')
  key_modulus=$(openssl rsa -modulus -noout -in "$key_file" 2>/dev/null | md5sum 2>/dev/null | awk '{print $1}')

  if [[ -n "$cert_modulus" ]] && [[ "$cert_modulus" == "$key_modulus" ]]; then
    printf "${COLOR_GREEN}✓${COLOR_RESET} Match\n"
  else
    printf "${COLOR_RED}✗${COLOR_RESET} Do not match\n"
    all_checks_passed=1
  fi

  # Check 5: Certificate details
  printf "\n${COLOR_BLUE}Certificate details:${COLOR_RESET}\n"
  local subject issuer start_date end_date
  subject=$(openssl x509 -subject -noout -in "$cert_file" 2>/dev/null | sed 's/^subject=//')
  issuer=$(openssl x509 -issuer -noout -in "$cert_file" 2>/dev/null | sed 's/^issuer=//')
  start_date=$(openssl x509 -startdate -noout -in "$cert_file" 2>/dev/null | sed 's/^notBefore=//')
  end_date=$(openssl x509 -enddate -noout -in "$cert_file" 2>/dev/null | sed 's/^notAfter=//')

  printf "  Subject:     %s\n" "$subject"
  printf "  Issuer:      %s\n" "$issuer"
  printf "  Valid from:  %s\n" "$start_date"
  printf "  Valid until: %s\n\n" "$end_date"

  if [[ $all_checks_passed -eq 0 ]]; then
    log_success "SSL certificate verification passed"
    return 0
  else
    log_error "SSL certificate verification failed"
    return 1
  fi
}

# ============================================================================
# Domain Health Checks
# ============================================================================

check_domain_health() {
  local domain="$1"

  log_info "Checking domain health: $domain"
  printf "%s\n\n" "$(printf '%.s=' {1..70})"

  local health_status="healthy"
  local issues=()

  # DNS Check
  printf "${COLOR_BLUE}1. DNS Resolution:${COLOR_RESET} "
  if check_dns_propagation "$domain"; then
    printf "${COLOR_GREEN}✓${COLOR_RESET} Resolved\n"
  else
    printf "${COLOR_RED}✗${COLOR_RESET} Not resolved\n"
    health_status="unhealthy"
    issues+=("DNS not resolving")
  fi

  # SSL Check
  printf "${COLOR_BLUE}2. SSL Certificate:${COLOR_RESET} "
  if has_ssl_certificate "$domain"; then
    if is_ssl_expired "$domain"; then
      printf "${COLOR_RED}✗${COLOR_RESET} Expired\n"
      health_status="degraded"
      issues+=("SSL certificate expired")
    else
      printf "${COLOR_GREEN}✓${COLOR_RESET} Valid\n"
    fi
  else
    printf "${COLOR_YELLOW}!${COLOR_RESET} No SSL\n"
    health_status="degraded"
    issues+=("No SSL certificate")
  fi

  # HTTP Check
  printf "${COLOR_BLUE}3. HTTP Response:${COLOR_RESET} "
  if check_http_response "$domain"; then
    printf "${COLOR_GREEN}✓${COLOR_RESET} Responding\n"
  else
    printf "${COLOR_RED}✗${COLOR_RESET} Not responding\n"
    health_status="unhealthy"
    issues+=("HTTP not responding")
  fi

  # Update health status
  local timestamp
  timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u "+%Y-%m-%dT%H:%M:%SZ")
  update_domain_health_status "$domain" "$health_status"
  update_domain_status "$domain" "lastHealthCheck" "$timestamp"

  # Summary
  printf "\n${COLOR_BLUE}Health Status:${COLOR_RESET} "
  case "$health_status" in
    healthy)
      printf "${COLOR_GREEN}Healthy${COLOR_RESET}\n"
      ;;
    degraded)
      printf "${COLOR_YELLOW}Degraded${COLOR_RESET}\n"
      ;;
    unhealthy)
      printf "${COLOR_RED}Unhealthy${COLOR_RESET}\n"
      ;;
  esac

  if [[ ${#issues[@]} -gt 0 ]]; then
    printf "\n${COLOR_BLUE}Issues Found:${COLOR_RESET}\n"
    for issue in "${issues[@]}"; do
      printf "  - %s\n" "$issue"
    done
  fi

  return 0
}

check_http_response() {
  local domain="$1"

  if command -v curl >/dev/null 2>&1; then
    local http_code
    http_code=$(curl -s -o /dev/null -w "%{http_code}" "http://${domain}" --connect-timeout 5 2>/dev/null || echo "000")
    if [[ "$http_code" =~ ^[23] ]]; then
      return 0
    fi
  elif command -v wget >/dev/null 2>&1; then
    if wget -q -O /dev/null --timeout=5 "http://${domain}" 2>/dev/null; then
      return 0
    fi
  fi

  return 1
}

# ============================================================================
# Helper Functions
# ============================================================================

validate_domain_format() {
  local domain="$1"
  # Basic domain validation regex
  if [[ "$domain" =~ ^[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]?\.[a-zA-Z]{2,}$ ]]; then
    return 0
  fi
  return 1
}

domain_exists() {
  local domain="$1"

  if [[ ! -f "$DOMAINS_CONFIG" ]]; then
    return 1
  fi

  if command -v jq >/dev/null 2>&1; then
    if jq -e --arg domain "$domain" '.domains[] | select(.domain == $domain)' "$DOMAINS_CONFIG" >/dev/null 2>&1; then
      return 0
    fi
  fi

  return 1
}

is_domain_verified() {
  local domain="$1"

  if command -v jq >/dev/null 2>&1; then
    local verified
    verified=$(jq -r --arg domain "$domain" '.domains[] | select(.domain == $domain) | .verified' "$DOMAINS_CONFIG" 2>/dev/null)
    [[ "$verified" == "true" ]] && return 0
  fi

  return 1
}

is_primary_domain() {
  local domain="$1"

  if command -v jq >/dev/null 2>&1; then
    local is_primary
    is_primary=$(jq -r --arg domain "$domain" '.domains[] | select(.domain == $domain) | .isPrimary' "$DOMAINS_CONFIG" 2>/dev/null)
    [[ "$is_primary" == "true" ]] && return 0
  fi

  return 1
}

has_ssl_certificate() {
  local domain="$1"
  local cert_file="${SSL_DIR}/${domain}/cert.pem"
  [[ -f "$cert_file" ]] && return 0
  return 1
}

is_ssl_expired() {
  local domain="$1"
  local cert_file="${SSL_DIR}/${domain}/cert.pem"

  if [[ ! -f "$cert_file" ]]; then
    return 1 # No cert means not expired (just missing)
  fi

  if command -v openssl >/dev/null 2>&1; then
    # Check if cert is currently expired
    if openssl x509 -checkend 0 -noout -in "$cert_file" 2>/dev/null; then
      return 1 # Not expired
    else
      return 0 # Expired
    fi
  fi

  return 1 # Can't check, assume not expired
}

update_domain_status() {
  local domain="$1"
  local field="$2"
  local value="$3"

  if command -v jq >/dev/null 2>&1; then
    local temp_file
    temp_file=$(mktemp)
    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u "+%Y-%m-%dT%H:%M:%SZ")

    jq --arg domain "$domain" --arg field "$field" --arg value "$value" --arg timestamp "$timestamp" \
      '(.domains[] | select(.domain == $domain) | .[$field]) = ($value | try fromjson catch $value) |
       (.domains[] | select(.domain == $domain) | .updatedAt) = $timestamp |
       .updatedAt = $timestamp' \
      "$DOMAINS_CONFIG" >"$temp_file" && mv "$temp_file" "$DOMAINS_CONFIG"
  fi
}

update_domain_field() {
  update_domain_status "$@"
}

update_domain_ssl_status() {
  local domain="$1"
  local enabled="$2"
  local issuer="$3"
  local expiry="$4"

  if command -v jq >/dev/null 2>&1; then
    local temp_file
    temp_file=$(mktemp)
    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u "+%Y-%m-%dT%H:%M:%SZ")

    jq --arg domain "$domain" --argjson enabled "$enabled" --arg issuer "$issuer" --arg expiry "$expiry" --arg timestamp "$timestamp" \
      '(.domains[] | select(.domain == $domain)) |=
       (.sslEnabled = $enabled | .sslIssuer = $issuer | .sslExpiryDate = $expiry | .updatedAt = $timestamp) |
       .updatedAt = $timestamp' \
      "$DOMAINS_CONFIG" >"$temp_file" && mv "$temp_file" "$DOMAINS_CONFIG"
  fi
}

update_domain_health_status() {
  local domain="$1"
  local status="$2"

  update_domain_status "$domain" "healthStatus" "$status"
}

update_nginx_ssl_config() {
  local domain="$1"
  local ssl_dir="$2"

  # This would update nginx configuration to use the new certificates
  # Implementation depends on nginx configuration structure
  log_info "Update nginx configuration to use SSL certificates"
  printf "  Domain: %s\n" "$domain"
  printf "  SSL directory: %s\n" "$ssl_dir"
}

reload_nginx_ssl() {
  # Reload nginx to apply new certificates
  if docker ps 2>/dev/null | grep -q nginx; then
    if docker exec nginx nginx -s reload 2>/dev/null; then
      log_success "Nginx reloaded with new certificates"
      return 0
    fi
  fi

  log_info "Nginx not running or could not be reloaded"
  return 1
}

# Export all functions
export -f initialize_domains_system
export -f add_custom_domain
export -f remove_custom_domain
export -f list_custom_domains
export -f verify_domain
export -f check_dns_propagation
export -f verify_txt_record
export -f generate_verification_token
export -f get_domain_ip
export -f provision_ssl
export -f provision_letsencrypt_ssl
export -f provision_selfsigned_ssl
export -f provision_custom_ssl
export -f setup_ssl_auto_renewal
export -f renew_ssl_certificate
export -f renew_letsencrypt_certificate
export -f verify_ssl_certificate
export -f check_domain_health
export -f check_http_response
export -f validate_domain_format
export -f domain_exists
export -f is_domain_verified
export -f is_primary_domain
export -f has_ssl_certificate
export -f is_ssl_expired
export -f update_domain_status
export -f update_domain_field
export -f update_domain_ssl_status
export -f update_domain_health_status
export -f update_nginx_ssl_config
export -f reload_nginx_ssl
