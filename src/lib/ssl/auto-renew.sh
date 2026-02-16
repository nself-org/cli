#!/usr/bin/env bash
# auto-renew.sh - Automatic SSL certificate renewal


# Get the directory where this script is located
SSL_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$SSL_LIB_DIR/../utils/display.sh" 2>/dev/null || true

# Check if certificate needs renewal
ssl::needs_renewal() {
  local cert_path="${1:-nginx/ssl/cert.pem}"
  local days_before_expiry="${2:-30}" # Renew 30 days before expiry (industry standard)

  # Check if certificate exists
  if [[ ! -f "$cert_path" ]]; then
    return 0 # Needs renewal if doesn't exist
  fi

  # Get certificate expiry date
  local expiry_date=$(openssl x509 -in "$cert_path" -noout -enddate 2>/dev/null | cut -d= -f2)
  if [[ -z "$expiry_date" ]]; then
    return 0 # Needs renewal if can't read
  fi

  # Calculate days until expiry
  local expiry_epoch=$(date -d "$expiry_date" +%s 2>/dev/null || date -j -f "%b %d %T %Y %Z" "$expiry_date" +%s 2>/dev/null || echo "0")
  local now_epoch=$(date +%s)
  local days_until_expiry=$(((expiry_epoch - now_epoch) / 86400))

  # Check if renewal is needed
  if [[ $days_until_expiry -lt $days_before_expiry ]]; then
    log_info "Certificate expires in $days_until_expiry days (threshold: $days_before_expiry)"
    return 0
  else
    log_debug "Certificate valid for $days_until_expiry more days"
    return 1
  fi
}

# Auto-renew certificates if needed
ssl::auto_renew() {
  local project_dir="${1:-.}"
  local force="${2:-false}"

  # Change to project directory
  cd "$project_dir"

  # Load environment
  if [[ -f ".env.local" ]]; then
    set -a
    source .env.local
    set +a
  elif [[ -f ".env" ]]; then
    set -a
    source .env
    set +a
  fi

  # Check if we have public certificates configured
  if [[ -z "${DNS_PROVIDER:-}" ]]; then
    log_debug "No DNS provider configured, using local certificates (no auto-renewal needed)"
    return 0
  fi

  # Check if renewal is needed
  if [[ "$force" == "true" ]] || ssl::needs_renewal "nginx/ssl/cert.pem"; then
    log_info "Starting automatic certificate renewal..."

    # Run renewal
    if nself ssl renew; then
      log_success "Certificate renewed successfully"

      # Restart nginx to apply new certificate
      if docker compose ps nginx &>/dev/null; then
        log_info "Restarting nginx to apply new certificate..."
        docker compose restart nginx
        log_success "Nginx restarted with new certificate"
      fi

      return 0
    else
      log_error "Certificate renewal failed"
      return 1
    fi
  else
    log_debug "Certificate renewal not needed"
    return 0
  fi
}

# Schedule automatic renewal
ssl::schedule_renewal() {
  local schedule="${1:-daily}" # daily, weekly, monthly
  local project_dir="${2:-$(pwd)}"

  # Create renewal script
  local renewal_script="/tmp/nself-ssl-renewal.sh"
  cat >"$renewal_script" <<EOF
#!/usr/bin/env bash
# Auto-generated SSL renewal script
cd "$project_dir"
/usr/local/bin/nself ssl auto-renew >> /var/log/nself-ssl-renewal.log 2>&1
EOF
  chmod +x "$renewal_script"

  # Determine cron schedule
  local cron_expr=""
  case "$schedule" in
    hourly)
      cron_expr="0 * * * *"
      ;;
    daily)
      cron_expr="0 3 * * *" # 3 AM daily
      ;;
    weekly)
      cron_expr="0 3 * * 0" # 3 AM Sunday
      ;;
    monthly)
      cron_expr="0 3 1 * *" # 3 AM first day of month
      ;;
    *)
      cron_expr="$schedule" # Custom cron expression
      ;;
  esac

  # Add to crontab
  (
    crontab -l 2>/dev/null | grep -v "nself-ssl-renewal"
    echo "$cron_expr $renewal_script"
  ) | crontab -

  log_success "Scheduled SSL auto-renewal: $schedule ($cron_expr)"
  log_info "Renewal script: $renewal_script"
  log_info "Log file: /var/log/nself-ssl-renewal.log"
}

# Unschedule automatic renewal
ssl::unschedule_renewal() {
  crontab -l 2>/dev/null | grep -v "nself-ssl-renewal" | crontab - || true
  rm -f /tmp/nself-ssl-renewal.sh
  log_success "Unscheduled SSL auto-renewal"
}

# Export functions
export -f ssl::needs_renewal
export -f ssl::auto_renew
export -f ssl::schedule_renewal
export -f ssl::unschedule_renewal
