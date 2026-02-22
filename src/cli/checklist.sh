#!/usr/bin/env bash
# checklist.sh - Production readiness checklist
# Part of nself v0.9.9 - Security Hardening

set -euo pipefail

# Get script directory (use namespaced variable to avoid clobbering caller globals)
_CHECKLIST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$_CHECKLIST_DIR/../.." && pwd)"

# Source utilities
source "$_CHECKLIST_DIR/../lib/utils/cli-output.sh"
source "$_CHECKLIST_DIR/../lib/utils/env.sh"
source "$_CHECKLIST_DIR/../lib/hooks/pre-command.sh"
source "$_CHECKLIST_DIR/../lib/hooks/post-command.sh"

# Colors
COLOR_GREEN="\033[0;32m"
COLOR_YELLOW="\033[0;33m"
COLOR_RED="\033[0;31m"
COLOR_RESET="\033[0m"

# =============================================================================
# HELP TEXT
# =============================================================================

show_help() {
  cat <<'EOF'
nself checklist - Production readiness verification

USAGE:
  nself checklist [options]

OPTIONS:
  --fix                          Auto-fix issues where possible
  --verbose                      Show detailed output
  --json                         Output in JSON format
  -h, --help                     Show this help message

CHECKS PERFORMED:
  ✓ SSL certificates valid and not expiring
  ✓ Backups configured and recent
  ✓ Monitoring active with alerts
  ✓ Resource limits defined
  ✓ Secrets properly configured
  ✓ Firewall enabled
  ✓ Log rotation configured
  ✓ Health endpoints responding
  ✓ Database properly tuned
  ✓ Security headers configured

EXAMPLES:
  nself checklist                         # Run all checks
  nself checklist --fix                   # Auto-fix issues
  nself checklist --verbose               # Detailed output
  nself checklist --json                  # JSON format

For more information: https://docs.nself.org/production
EOF
}

# =============================================================================
# INDIVIDUAL CHECKS
# =============================================================================

check_ssl_certificates() {
  local auto_fix="$1"
  load_env_with_priority

  if [[ ! -d "./ssl/certificates" ]]; then
    echo "SSL directory not found"
    return 1
  fi

  if [[ ! -f "./ssl/certificates/cert.pem" ]]; then
    echo "SSL certificate not found"
    [[ "$auto_fix" == "true" ]] && {
      mkdir -p ./ssl/certificates
      echo "Created SSL directory (generate certificate with 'nself auth ssl generate')"
      return 2
    }
    return 1
  fi

  # Check expiry
  local expiry_date=$(openssl x509 -in ./ssl/certificates/cert.pem -noout -enddate 2>/dev/null | cut -d'=' -f2)
  if [[ -z "$expiry_date" ]]; then
    echo "Unable to read certificate expiry"
    return 1
  fi

  local expiry_epoch=$(date -d "$expiry_date" +%s 2>/dev/null || date -jf "%b %d %T %Y %Z" "$expiry_date" +%s 2>/dev/null || echo "0")
  if [[ "$expiry_epoch" == "0" ]]; then
    echo "Unable to parse certificate expiry date"
    return 2
  fi

  local now_epoch=$(date +%s)
  local days_until_expiry=$(((expiry_epoch - now_epoch) / 86400))

  if [[ $days_until_expiry -lt 0 ]]; then
    echo "SSL certificate has expired!"
    return 1
  elif [[ $days_until_expiry -lt 30 ]]; then
    echo "SSL certificate expires in $days_until_expiry days"
    return 2
  fi

  echo "SSL certificate valid (expires in $days_until_expiry days)"
  return 0
}

check_backups() {
  local auto_fix="$1"

  # Check if backups are configured
  if [[ ! -d "./backups" ]]; then
    echo "Backup directory not found"
    [[ "$auto_fix" == "true" ]] && {
      mkdir -p ./backups
      echo "Created backup directory"
      return 2
    }
    return 1
  fi

  # Check for recent backup
  local latest_backup=$(ls -1t ./backups/*.tar.gz 2>/dev/null | head -1 || echo "")
  if [[ -z "$latest_backup" ]]; then
    echo "No backups found - run 'nself backup create'"
    return 2
  fi

  # Check backup age
  local backup_time=$(stat -f %m "$latest_backup" 2>/dev/null || stat -c %Y "$latest_backup" 2>/dev/null || echo "0")
  local backup_age_seconds=$(( $(date +%s) - backup_time ))
  local backup_age_days=$((backup_age_seconds / 86400))

  if [[ $backup_age_days -gt 7 ]]; then
    echo "Latest backup is $backup_age_days days old (should be < 7 days)"
    return 2
  fi

  # Check if automated backups are scheduled
  if command -v crontab >/dev/null 2>&1; then
    if ! crontab -l 2>/dev/null | grep -q "nself.*backup"; then
      echo "No automated backup schedule found - run 'nself backup schedule create daily'"
      return 2
    fi
  fi

  echo "Backups configured (latest: $backup_age_days day(s) old)"
  return 0
}

check_monitoring() {
  local auto_fix="$1"
  load_env_with_priority

  if [[ "${MONITORING_ENABLED:-false}" != "true" ]]; then
    echo "Monitoring not enabled (set MONITORING_ENABLED=true in .env)"
    return 2
  fi

  # Check if Prometheus is running
  local project_name="${PROJECT_NAME:-nself}"
  if ! docker ps --format "{{.Names}}" 2>/dev/null | grep -q "${project_name}_prometheus"; then
    echo "Prometheus not running - start with 'nself start'"
    return 1
  fi

  # Check if alerts are configured
  if [[ ! -f "./monitoring/prometheus/alert-rules.yml" ]]; then
    echo "Alert rules not found"
    [[ "$auto_fix" == "true" ]] && {
      bash "$ROOT_DIR/src/lib/monitoring/alert-rules.sh" rules >/dev/null 2>&1
      echo "Generated default alert rules"
      return 2
    }
    return 2
  fi

  echo "Monitoring active with alert rules configured"
  return 0
}

check_resource_limits() {
  local auto_fix="$1"

  # Check if docker-compose has resource limits
  if [[ ! -f "./docker-compose.yml" ]]; then
    echo "docker-compose.yml not found - run 'nself build'"
    return 1
  fi

  if ! grep -q "resources:" ./docker-compose.yml 2>/dev/null; then
    echo "No resource limits defined (set DOCKER_RESOURCE_LIMITS=true)"
    return 2
  fi

  # Check system resources meet minimum requirements
  if [[ -f "$ROOT_DIR/src/lib/docker/resources.sh" ]]; then
    if ! bash "$ROOT_DIR/src/lib/docker/resources.sh" check >/dev/null 2>&1; then
      echo "System resources below minimum requirements"
      return 2
    fi
  fi

  echo "Resource limits configured"
  return 0
}

check_secrets() {
  local auto_fix="$1"
  load_env_with_priority

  local issues=()

  # Check for default/weak passwords
  [[ "${POSTGRES_PASSWORD:-postgres}" == "postgres" ]] && issues+=("PostgreSQL using default password")
  [[ "${HASURA_ADMIN_SECRET:-}" == "myadminsecret" ]] && issues+=("Hasura using default admin secret")
  [[ -z "${HASURA_ADMIN_SECRET:-}" ]] && issues+=("Hasura admin secret not set")

  # Check password strength (minimum 16 characters for production)
  if [[ -n "${POSTGRES_PASSWORD:-}" ]] && [[ ${#POSTGRES_PASSWORD} -lt 16 ]]; then
    issues+=("PostgreSQL password too short (minimum 16 characters)")
  fi

  if [[ -n "${HASURA_ADMIN_SECRET:-}" ]] && [[ ${#HASURA_ADMIN_SECRET} -lt 32 ]]; then
    issues+=("Hasura admin secret too short (minimum 32 characters)")
  fi

  if [[ ${#issues[@]} -gt 0 ]]; then
    echo "Security issues found:"
    for issue in "${issues[@]}"; do
      echo "  - $issue"
    done
    return 1
  fi

  echo "Secrets properly configured"
  return 0
}

check_firewall() {
  local auto_fix="$1"

  # Check if ufw is installed (Linux)
  if command -v ufw >/dev/null 2>&1; then
    if ! sudo ufw status 2>/dev/null | grep -q "Status: active"; then
      echo "Firewall (ufw) not active - enable with 'sudo ufw enable'"
      return 2
    fi
    echo "Firewall (ufw) active"
    return 0
  elif command -v firewall-cmd >/dev/null 2>&1; then
    if ! sudo firewall-cmd --state 2>/dev/null | grep -q "running"; then
      echo "Firewall (firewalld) not running"
      return 2
    fi
    echo "Firewall (firewalld) active"
    return 0
  elif [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS - check pf firewall
    if ! sudo pfctl -s info >/dev/null 2>&1; then
      echo "Firewall not active (macOS pf) - consider enabling in System Preferences"
      return 2
    fi
    echo "Firewall configured (macOS)"
    return 0
  else
    echo "No firewall detected (recommended for production)"
    return 2
  fi
}

check_log_rotation() {
  local auto_fix="$1"

  if [[ ! -d "./.nself/logrotate.d" ]]; then
    echo "Logrotate configurations not found"
    [[ "$auto_fix" == "true" ]] && {
      bash "$ROOT_DIR/src/lib/logging/logrotate.sh" generate >/dev/null 2>&1
      echo "Generated logrotate configurations"
      return 2
    }
    return 2
  fi

  # Check if log directory exists
  if [[ ! -d "./logs" ]]; then
    echo "Log directory not found"
    [[ "$auto_fix" == "true" ]] && {
      bash "$ROOT_DIR/src/lib/logging/logrotate.sh" setup >/dev/null 2>&1
      echo "Created log directory structure"
      return 2
    }
    return 2
  fi

  echo "Log rotation configured"
  return 0
}

check_health_endpoints() {
  local auto_fix="$1"
  load_env_with_priority

  local base_domain="${BASE_DOMAIN:-local.nself.org}"
  local protocol="https"

  # Use http for local development
  if [[ "$base_domain" == "localhost" ]] || [[ "$base_domain" == "local.nself.org" ]]; then
    protocol="http"
  fi

  local failed=0
  local endpoints=(
    "api.${base_domain}/healthz"
    "auth.${base_domain}/healthz"
  )

  for endpoint in "${endpoints[@]}"; do
    if ! curl -sf --max-time 5 "${protocol}://$endpoint" >/dev/null 2>&1; then
      echo "Health endpoint not responding: ${protocol}://$endpoint"
      failed=$((failed + 1))
    fi
  done

  if [[ $failed -gt 0 ]]; then
    echo "$failed health endpoint(s) not responding - services may not be running"
    return 1
  fi

  echo "All health endpoints responding"
  return 0
}

check_database_tuning() {
  local auto_fix="$1"
  load_env_with_priority

  local warnings=()

  # Check if PgBouncer is enabled
  if [[ "${PGBOUNCER_ENABLED:-false}" != "true" ]]; then
    warnings+=("PgBouncer not enabled (recommended for production)")
  fi

  # Check PostgreSQL settings
  local project_name="${PROJECT_NAME:-nself}"
  if docker ps --format "{{.Names}}" 2>/dev/null | grep -q "${project_name}_postgres"; then
    local max_connections=$(docker exec "${project_name}_postgres" psql -U "${POSTGRES_USER:-postgres}" -t -c "SHOW max_connections;" 2>/dev/null | tr -d ' ' || echo "0")
    if [[ -n "$max_connections" ]] && [[ $max_connections -lt 100 ]]; then
      warnings+=("PostgreSQL max_connections low: $max_connections (recommended: >= 100)")
    fi
  else
    echo "PostgreSQL not running"
    return 1
  fi

  if [[ ${#warnings[@]} -gt 0 ]]; then
    for warning in "${warnings[@]}"; do
      echo "$warning"
    done
    return 2
  fi

  echo "Database properly configured"
  return 0
}

check_security_headers() {
  local auto_fix="$1"
  load_env_with_priority

  local base_domain="${BASE_DOMAIN:-local.nself.org}"
  local protocol="https"

  if [[ "$base_domain" == "localhost" ]] || [[ "$base_domain" == "local.nself.org" ]]; then
    protocol="http"
  fi

  # Try to get headers
  local headers=$(curl -sI --max-time 5 "${protocol}://${base_domain}" 2>/dev/null || echo "")

  if [[ -z "$headers" ]]; then
    echo "Unable to check security headers (service may not be running)"
    return 2
  fi

  local missing=()
  echo "$headers" | grep -qi "X-Frame-Options" || missing+=("X-Frame-Options")
  echo "$headers" | grep -qi "X-Content-Type-Options" || missing+=("X-Content-Type-Options")
  echo "$headers" | grep -qi "Strict-Transport-Security" || missing+=("HSTS")

  if [[ ${#missing[@]} -gt 0 ]]; then
    echo "Missing security headers: ${missing[*]}"
    return 2
  fi

  echo "Security headers configured"
  return 0
}

# =============================================================================
# MAIN CHECKLIST COMMAND
# =============================================================================

cmd_checklist() {
  local auto_fix="${AUTO_FIX:-false}"
  local verbose="${VERBOSE:-false}"
  local json_mode="${JSON_OUTPUT:-false}"

  if [[ "$json_mode" != "true" ]]; then
    cli_section "Production Readiness Checklist"
    printf "\n"
  fi

  local checks=(
    "check_ssl_certificates"
    "check_backups"
    "check_monitoring"
    "check_resource_limits"
    "check_secrets"
    "check_firewall"
    "check_log_rotation"
    "check_health_endpoints"
    "check_database_tuning"
    "check_security_headers"
  )

  local passed=0
  local failed=0
  local warnings=0
  local results=()

  for check in "${checks[@]}"; do
    local result=$($check "$auto_fix" 2>&1 || echo "")
    local exit_code=$?

    local status=""
    local color=""

    case $exit_code in
      0)
        passed=$((passed + 1))
        status="PASS"
        color="$COLOR_GREEN"
        ;;
      1)
        failed=$((failed + 1))
        status="FAIL"
        color="$COLOR_RED"
        ;;
      2)
        warnings=$((warnings + 1))
        status="WARN"
        color="$COLOR_YELLOW"
        ;;
    esac

    results+=("$status|$check|$result")

    if [[ "$json_mode" != "true" ]]; then
      local check_name=$(echo "$check" | sed 's/check_//' | tr '_' ' ')
      printf "${color}%-6s${COLOR_RESET} %s\n" "[$status]" "$check_name"
      if [[ "$verbose" == "true" ]] || [[ $exit_code -ne 0 ]]; then
        echo "$result" | sed 's/^/         /'
      fi
    fi
  done

  # Summary
  if [[ "$json_mode" == "true" ]]; then
    printf '{"total": %d, "passed": %d, "failed": %d, "warnings": %d, "checks": [' \
      "${#checks[@]}" "$passed" "$failed" "$warnings"

    local first=true
    for result in "${results[@]}"; do
      [[ "$first" != "true" ]] && printf ","
      first=false

      local status=$(echo "$result" | cut -d'|' -f1)
      local check=$(echo "$result" | cut -d'|' -f2)
      local message=$(echo "$result" | cut -d'|' -f3-)

      printf '{"check": "%s", "status": "%s", "message": "%s"}' \
        "$check" "$status" "$message"
    done

    printf ']}\n'
  else
    printf "\n"
    cli_section "Summary"
    printf "\n"
    printf "  ${COLOR_GREEN}✓ Passed:${COLOR_RESET}   %d / %d\n" "$passed" "${#checks[@]}"
    printf "  ${COLOR_YELLOW}⚠ Warnings:${COLOR_RESET} %d\n" "$warnings"
    printf "  ${COLOR_RED}✗ Failed:${COLOR_RESET}   %d\n" "$failed"
    printf "\n"

    if [[ $failed -eq 0 ]] && [[ $warnings -eq 0 ]]; then
      cli_success "All production checks passed! Ready for deployment."
    elif [[ $failed -eq 0 ]]; then
      cli_warning "$warnings warning(s) found. Review before deploying to production."
    else
      cli_error "$failed critical issue(s) found. Fix these before deploying!"
      return 1
    fi
  fi

  [[ $failed -eq 0 ]]
}

# =============================================================================
# MAIN
# =============================================================================

main() {
  local show_help_flag=false

  # Parse flags
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --fix)
        AUTO_FIX=true
        shift
        ;;
      --verbose)
        VERBOSE=true
        shift
        ;;
      --json)
        JSON_OUTPUT=true
        shift
        ;;
      -h | --help)
        show_help_flag=true
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  if [[ "$show_help_flag" == "true" ]]; then
    show_help
    return 0
  fi

  cmd_checklist
}

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_help
      exit 0
    fi
  done
  pre_command "checklist" || exit $?
  main "$@"
  exit_code=$?
  post_command "checklist" $exit_code
  exit $exit_code
fi
