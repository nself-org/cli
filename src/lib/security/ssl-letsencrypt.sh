#!/usr/bin/env bash

# ssl-letsencrypt.sh - Let's Encrypt SSL certificate management
# POSIX-compliant, no Bash 4+ features

# Get the directory where this script is located
SECURITY_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

LIB_ROOT="$(dirname "$SECURITY_LIB_DIR")"

# Source dependencies
source "$LIB_ROOT/utils/display.sh" 2>/dev/null || true
source "$LIB_ROOT/utils/platform-compat.sh" 2>/dev/null || true

# SSL Configuration
SSL_DIR="${SSL_DIR:-./ssl}"
CERTBOT_WEBROOT="${CERTBOT_WEBROOT:-./certbot-webroot}"
LETSENCRYPT_DIR="${LETSENCRYPT_DIR:-/etc/letsencrypt}"

# Initialize SSL directory structure
ssl::init() {
  log_info "Initializing SSL directory structure..."

  mkdir -p "$SSL_DIR"
  mkdir -p "$CERTBOT_WEBROOT/.well-known/acme-challenge"

  # Create placeholder for ACME challenges
  printf "Ready for ACME challenges\n" >"$CERTBOT_WEBROOT/.well-known/acme-challenge/.placeholder"

  log_success "SSL directory initialized: $SSL_DIR"
}

# Check if certbot is available
ssl::check_certbot() {
  if command -v certbot >/dev/null 2>&1; then
    return 0
  fi

  # Check for certbot in common locations
  local certbot_paths="/usr/bin/certbot /usr/local/bin/certbot /snap/bin/certbot"
  for path in $certbot_paths; do
    if [[ -x "$path" ]]; then
      return 0
    fi
  done

  return 1
}

# Install certbot (with guidance)
ssl::install_certbot() {
  log_info "Checking certbot installation..."

  if ssl::check_certbot; then
    local version
    version=$(certbot --version 2>&1 | head -1)
    log_success "certbot is installed: $version"
    return 0
  fi

  log_warning "certbot is not installed"
  printf "\n"
  printf "Installation instructions:\n"
  printf "\n"

  # Detect OS and provide instructions
  if [[ -f /etc/debian_version ]]; then
    printf "  Ubuntu/Debian:\n"
    printf "    sudo apt update\n"
    printf "    sudo apt install certbot\n"
  elif [[ -f /etc/redhat-release ]]; then
    printf "  RHEL/CentOS:\n"
    printf "    sudo yum install epel-release\n"
    printf "    sudo yum install certbot\n"
  elif [[ "$OSTYPE" == "darwin"* ]]; then
    printf "  macOS:\n"
    printf "    brew install certbot\n"
  else
    printf "  Visit: https://certbot.eff.org/instructions\n"
  fi

  printf "\n"
  return 1
}

# Request a new certificate
ssl::request_cert() {
  local domain="$1"
  local email="$2"
  local method="${3:-webroot}"
  local staging="${4:-false}"

  if [[ -z "$domain" ]]; then
    log_error "Domain is required"
    printf "Usage: ssl::request_cert <domain> <email> [method] [staging]\n"
    return 1
  fi

  if [[ -z "$email" ]]; then
    log_error "Email is required for Let's Encrypt registration"
    return 1
  fi

  if ! ssl::check_certbot; then
    ssl::install_certbot
    return 1
  fi

  log_info "Requesting SSL certificate for: $domain"

  # Build certbot command
  local certbot_cmd="certbot certonly"
  certbot_cmd="$certbot_cmd --non-interactive"
  certbot_cmd="$certbot_cmd --agree-tos"
  certbot_cmd="$certbot_cmd --email $email"
  certbot_cmd="$certbot_cmd -d $domain"

  # Add staging flag for testing
  if [[ "$staging" == "true" ]]; then
    certbot_cmd="$certbot_cmd --staging"
    log_warning "Using Let's Encrypt staging environment (for testing)"
  fi

  case "$method" in
    webroot)
      ssl::init
      certbot_cmd="$certbot_cmd --webroot -w $CERTBOT_WEBROOT"
      ;;
    standalone)
      certbot_cmd="$certbot_cmd --standalone"
      log_warning "Standalone mode requires ports 80/443 to be available"
      ;;
    dns)
      certbot_cmd="$certbot_cmd --manual --preferred-challenges dns"
      log_warning "DNS challenge requires manual DNS record creation"
      ;;
    *)
      log_error "Unknown method: $method (use webroot, standalone, or dns)"
      return 1
      ;;
  esac

  printf "\nRunning: %s\n\n" "$certbot_cmd"

  # SECURITY: Execute certbot via bash -c instead of raw eval
  if bash -c "$certbot_cmd"; then
    log_success "Certificate obtained successfully"
    ssl::link_cert "$domain"
    return 0
  else
    log_error "Failed to obtain certificate"
    return 1
  fi
}

# Link Let's Encrypt certificates to project SSL directory
ssl::link_cert() {
  local domain="$1"

  if [[ -z "$domain" ]]; then
    log_error "Domain is required"
    return 1
  fi

  local le_cert="$LETSENCRYPT_DIR/live/$domain/fullchain.pem"
  local le_key="$LETSENCRYPT_DIR/live/$domain/privkey.pem"

  if [[ ! -f "$le_cert" ]]; then
    log_error "Certificate not found: $le_cert"
    return 1
  fi

  mkdir -p "$SSL_DIR"

  # Copy certificates (don't symlink to avoid permission issues)
  cp "$le_cert" "$SSL_DIR/cert.pem"
  cp "$le_key" "$SSL_DIR/key.pem"

  # Set secure permissions
  chmod 644 "$SSL_DIR/cert.pem"
  chmod 600 "$SSL_DIR/key.pem"

  log_success "Certificates linked to: $SSL_DIR"
  printf "  cert.pem: SSL certificate chain\n"
  printf "  key.pem:  Private key\n"

  return 0
}

# Renew certificates
ssl::renew() {
  local force="${1:-false}"

  if ! ssl::check_certbot; then
    log_error "certbot is not installed"
    return 1
  fi

  log_info "Checking certificate renewal..."

  local certbot_cmd="certbot renew"

  if [[ "$force" == "true" ]]; then
    certbot_cmd="$certbot_cmd --force-renewal"
    log_warning "Forcing renewal (may hit rate limits)"
  fi

  if $certbot_cmd; then
    log_success "Certificate renewal check complete"

    # Re-link certificates if they were renewed
    if [[ -n "${BASE_DOMAIN:-}" ]]; then
      ssl::link_cert "$BASE_DOMAIN"
    fi

    return 0
  else
    log_error "Certificate renewal failed"
    return 1
  fi
}

# Check certificate status
ssl::status() {
  local cert_file="${1:-$SSL_DIR/cert.pem}"

  if [[ ! -f "$cert_file" ]]; then
    log_error "Certificate not found: $cert_file"
    return 1
  fi

  if ! command -v openssl >/dev/null 2>&1; then
    log_error "openssl is required for certificate inspection"
    return 1
  fi

  printf "Certificate Status: %s\n\n" "$cert_file"

  # Get certificate information
  local subject
  local issuer
  local start_date
  local end_date

  subject=$(openssl x509 -subject -noout -in "$cert_file" 2>/dev/null | cut -d'=' -f2-)
  issuer=$(openssl x509 -issuer -noout -in "$cert_file" 2>/dev/null | cut -d'=' -f2-)
  start_date=$(openssl x509 -startdate -noout -in "$cert_file" 2>/dev/null | cut -d'=' -f2)
  end_date=$(openssl x509 -enddate -noout -in "$cert_file" 2>/dev/null | cut -d'=' -f2)

  printf "  Subject: %s\n" "${subject:-N/A}"
  printf "  Issuer:  %s\n" "${issuer:-N/A}"
  printf "  Valid from: %s\n" "${start_date:-N/A}"
  printf "  Valid until: %s\n" "${end_date:-N/A}"

  # Check expiry
  if [[ -n "$end_date" ]]; then
    local expiry_epoch now_epoch
    expiry_epoch=$(date -j -f "%b %d %T %Y %Z" "$end_date" +%s 2>/dev/null || date -d "$end_date" +%s 2>/dev/null)
    now_epoch=$(date +%s)

    if [[ -n "$expiry_epoch" ]]; then
      local days_left=$(((expiry_epoch - now_epoch) / 86400))

      printf "\n"
      if [[ $days_left -lt 0 ]]; then
        printf "  ${COLOR_RED}⚠ EXPIRED %d days ago${COLOR_RESET}\n" "$((-days_left))"
      elif [[ $days_left -lt 7 ]]; then
        printf "  ${COLOR_RED}⚠ Expires in %d days - RENEW NOW${COLOR_RESET}\n" "$days_left"
      elif [[ $days_left -lt 30 ]]; then
        printf "  ${COLOR_YELLOW}⚠ Expires in %d days - consider renewing${COLOR_RESET}\n" "$days_left"
      else
        printf "  ${COLOR_GREEN}✓ Valid for %d more days${COLOR_RESET}\n" "$days_left"
      fi
    fi
  fi

  # Check if self-signed
  if echo "$issuer" | grep -qi "self-signed\|localhost\|local"; then
    printf "\n  ${COLOR_YELLOW}Note: This appears to be a self-signed certificate${COLOR_RESET}\n"
  fi

  return 0
}

# Generate self-signed certificate (for development)
ssl::generate_self_signed() {
  local domain="${1:-localhost}"
  local days="${2:-365}"

  if ! command -v openssl >/dev/null 2>&1; then
    log_error "openssl is required"
    return 1
  fi

  log_info "Generating self-signed certificate for: $domain"

  mkdir -p "$SSL_DIR"

  # Generate certificate
  openssl req -x509 -nodes -days "$days" -newkey rsa:2048 \
    -keyout "$SSL_DIR/key.pem" \
    -out "$SSL_DIR/cert.pem" \
    -subj "/CN=$domain/O=nself/C=US" \
    -addext "subjectAltName=DNS:$domain,DNS:*.$domain,DNS:localhost,IP:127.0.0.1" \
    2>/dev/null

  if [[ $? -eq 0 ]]; then
    chmod 644 "$SSL_DIR/cert.pem"
    chmod 600 "$SSL_DIR/key.pem"
    log_success "Self-signed certificate generated"
    printf "  Certificate: %s/cert.pem\n" "$SSL_DIR"
    printf "  Private key: %s/key.pem\n" "$SSL_DIR"
    printf "  Valid for: %d days\n" "$days"
    return 0
  else
    log_error "Failed to generate certificate"
    return 1
  fi
}

# Setup automatic renewal (systemd timer or cron)
ssl::setup_auto_renewal() {
  local method="${1:-cron}"

  log_info "Setting up automatic certificate renewal..."

  case "$method" in
    cron)
      ssl::setup_cron_renewal
      ;;
    systemd)
      ssl::setup_systemd_renewal
      ;;
    *)
      log_error "Unknown method: $method (use cron or systemd)"
      return 1
      ;;
  esac
}

# Setup cron-based renewal
ssl::setup_cron_renewal() {
  local cron_cmd="0 0,12 * * * certbot renew --quiet --deploy-hook 'docker compose restart nginx'"

  printf "To enable automatic renewal, add this to your crontab:\n\n"
  printf "  %s\n\n" "$cron_cmd"
  printf "Run: crontab -e\n"
  printf "And paste the line above.\n"

  # Check if already configured
  if crontab -l 2>/dev/null | grep -q "certbot renew"; then
    log_success "Certbot renewal is already in crontab"
  fi
}

# Setup systemd-based renewal
ssl::setup_systemd_renewal() {
  if [[ ! -d "/etc/systemd/system" ]]; then
    log_error "systemd not available on this system"
    return 1
  fi

  printf "Certbot typically installs its own systemd timer.\n"
  printf "Check status with:\n\n"
  printf "  systemctl status certbot.timer\n"
  printf "  systemctl list-timers | grep certbot\n"

  if systemctl is-enabled certbot.timer >/dev/null 2>&1; then
    log_success "Certbot timer is enabled"
  else
    printf "\nTo enable:\n"
    printf "  sudo systemctl enable --now certbot.timer\n"
  fi
}

# Verify certificate chain
ssl::verify_chain() {
  local cert_file="${1:-$SSL_DIR/cert.pem}"
  local key_file="${2:-$SSL_DIR/key.pem}"

  if [[ ! -f "$cert_file" ]]; then
    log_error "Certificate not found: $cert_file"
    return 1
  fi

  if [[ ! -f "$key_file" ]]; then
    log_error "Key not found: $key_file"
    return 1
  fi

  if ! command -v openssl >/dev/null 2>&1; then
    log_error "openssl is required"
    return 1
  fi

  log_info "Verifying certificate chain..."

  # Check certificate validity
  if openssl x509 -checkend 0 -noout -in "$cert_file" >/dev/null 2>&1; then
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} Certificate is not expired\n"
  else
    printf "  ${COLOR_RED}✗${COLOR_RESET} Certificate is expired\n"
    return 1
  fi

  # Check key matches certificate
  local cert_modulus key_modulus
  cert_modulus=$(openssl x509 -modulus -noout -in "$cert_file" 2>/dev/null | md5sum)
  key_modulus=$(openssl rsa -modulus -noout -in "$key_file" 2>/dev/null | md5sum)

  if [[ "$cert_modulus" == "$key_modulus" ]]; then
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} Certificate and key match\n"
  else
    printf "  ${COLOR_RED}✗${COLOR_RESET} Certificate and key do NOT match\n"
    return 1
  fi

  log_success "Certificate verification passed"
  return 0
}

# Export functions
export -f ssl::init
export -f ssl::check_certbot
export -f ssl::install_certbot
export -f ssl::request_cert
export -f ssl::link_cert
export -f ssl::renew
export -f ssl::status
export -f ssl::generate_self_signed
export -f ssl::setup_auto_renewal
export -f ssl::setup_cron_renewal
export -f ssl::setup_systemd_renewal
export -f ssl::verify_chain
