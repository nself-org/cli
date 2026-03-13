#!/usr/bin/env bash
# audit.sh - Security Audit System
# Part of nself v0.9.6+ - Security First Implementation
#
# Provides production readiness security auditing for nself projects

set -euo pipefail

# Get script directory
SECURITY_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_ROOT="$(dirname "$SECURITY_LIB_DIR")"

# Source dependencies
source "$LIB_ROOT/utils/display.sh" 2>/dev/null || true
source "$LIB_ROOT/utils/platform-compat.sh" 2>/dev/null || true

# ============================================================================
# Production Readiness Checklist
# ============================================================================

# Check SSL/TLS configuration
audit_ssl_configuration() {
  local issues=0
  local env_file="${1:-.env}"

  printf "  ${COLOR_BOLD}SSL/TLS Configuration${COLOR_RESET}\n\n"

  # Check if SSL is enabled
  local ssl_enabled
  ssl_enabled=$(grep "^SSL_ENABLED=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d "'")

  if [[ "$ssl_enabled" != "true" ]]; then
    printf "    ${COLOR_RED}✗${COLOR_RESET} SSL not enabled\n"
    printf "      → Run: nself auth ssl generate\n"
    issues=$((issues + 1))
  else
    printf "    ${COLOR_GREEN}✓${COLOR_RESET} SSL enabled\n"

    # Check certificate validity if files exist
    if [[ -f "ssl/cert.pem" ]]; then
      if command -v openssl >/dev/null 2>&1; then
        local expiry
        expiry=$(openssl x509 -in ssl/cert.pem -noout -enddate 2>/dev/null | cut -d'=' -f2)

        if [[ -n "$expiry" ]]; then
          printf "    ${COLOR_GREEN}✓${COLOR_RESET} Certificate valid until: %s\n" "$expiry"
        fi
      fi
    fi
  fi

  printf "\n"
  return $issues
}

# Check authentication configuration
audit_auth_configuration() {
  local issues=0
  local env_file="${1:-.env}"

  printf "  ${COLOR_BOLD}Authentication Configuration${COLOR_RESET}\n\n"

  # Check if admin secret is set
  local admin_secret
  admin_secret=$(grep "^HASURA_GRAPHQL_ADMIN_SECRET=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d "'")

  if [[ -z "$admin_secret" ]]; then
    printf "    ${COLOR_RED}✗${COLOR_RESET} Hasura admin secret not set\n"
    printf "      → Set HASURA_GRAPHQL_ADMIN_SECRET in .env\n"
    issues=$((issues + 1))
  elif [[ ${#admin_secret} -lt 32 ]]; then
    printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} Admin secret too short (%d chars)\n" "${#admin_secret}"
    printf "      → Use at least 32 characters\n"
    issues=$((issues + 1))
  else
    printf "    ${COLOR_GREEN}✓${COLOR_RESET} Admin secret configured\n"
  fi

  # Check JWT secret
  local jwt_secret
  jwt_secret=$(grep "^JWT_SECRET=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d "'")

  if [[ -z "$jwt_secret" ]]; then
    printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} JWT secret not set\n"
    printf "      → Set JWT_SECRET in .env\n"
    issues=$((issues + 1))
  elif [[ ${#jwt_secret} -lt 64 ]]; then
    printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} JWT secret too short (%d chars)\n" "${#jwt_secret}"
    printf "      → Use at least 64 characters\n"
    issues=$((issues + 1))
  else
    printf "    ${COLOR_GREEN}✓${COLOR_RESET} JWT secret configured\n"
  fi

  # Check console disabled in production
  local env
  env=$(grep "^ENV=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d "'")

  if [[ "$env" == "production" ]] || [[ "$env" == "prod" ]]; then
    local console_enabled
    console_enabled=$(grep "^HASURA_GRAPHQL_ENABLE_CONSOLE=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d "'")

    if [[ "$console_enabled" == "true" ]]; then
      printf "    ${COLOR_RED}✗${COLOR_RESET} Hasura console enabled in production\n"
      printf "      → Set HASURA_GRAPHQL_ENABLE_CONSOLE=false\n"
      issues=$((issues + 1))
    else
      printf "    ${COLOR_GREEN}✓${COLOR_RESET} Hasura console disabled in production\n"
    fi
  fi

  printf "\n"
  return $issues
}

# Check monitoring configuration
audit_monitoring() {
  local issues=0
  local env_file="${1:-.env}"

  printf "  ${COLOR_BOLD}Monitoring & Observability${COLOR_RESET}\n\n"

  local monitoring_enabled
  monitoring_enabled=$(grep "^MONITORING_ENABLED=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d "'")

  if [[ "$monitoring_enabled" != "true" ]]; then
    printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} Monitoring not enabled\n"
    printf "      → Set MONITORING_ENABLED=true for production visibility\n"
    issues=$((issues + 1))
  else
    printf "    ${COLOR_GREEN}✓${COLOR_RESET} Monitoring enabled\n"

    # Check Grafana password
    local grafana_password
    grafana_password=$(grep "^GRAFANA_ADMIN_PASSWORD=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d "'")

    if [[ -n "$grafana_password" ]] && [[ ${#grafana_password} -ge 16 ]]; then
      printf "    ${COLOR_GREEN}✓${COLOR_RESET} Grafana admin password configured\n"
    else
      printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} Grafana admin password weak or missing\n"
      issues=$((issues + 1))
    fi
  fi

  printf "\n"
  return $issues
}

# Check backup configuration
audit_backup() {
  local issues=0

  printf "  ${COLOR_BOLD}Backup & Recovery${COLOR_RESET}\n\n"

  # Check if backup directory exists
  if [[ -d "_backup" ]]; then
    printf "    ${COLOR_GREEN}✓${COLOR_RESET} Backup directory exists\n"

    # Check for recent backups
    if command -v find >/dev/null 2>&1; then
      local recent_backup
      recent_backup=$(find _backup -type f -name "*.sql" -mtime -7 2>/dev/null | head -n 1)

      if [[ -n "$recent_backup" ]]; then
        printf "    ${COLOR_GREEN}✓${COLOR_RESET} Recent backup found (< 7 days)\n"
      else
        printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} No recent backups (< 7 days)\n"
        printf "      → Run: nself backup create\n"
        issues=$((issues + 1))
      fi
    fi
  else
    printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} No backup directory\n"
    printf "      → Run: nself backup create\n"
    issues=$((issues + 1))
  fi

  printf "\n"
  return $issues
}

# Check firewall and network security
audit_network_security() {
  local issues=0
  local env_file="${1:-.env}"

  printf "  ${COLOR_BOLD}Network Security${COLOR_RESET}\n\n"

  # Check CORS configuration
  local cors_origin
  cors_origin=$(grep "^HASURA_GRAPHQL_CORS_DOMAIN=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d "'")

  if [[ "$cors_origin" == "*" ]]; then
    printf "    ${COLOR_RED}✗${COLOR_RESET} CORS allows all origins (*)\n"
    printf "      → Restrict to specific domains\n"
    issues=$((issues + 1))
  elif [[ -z "$cors_origin" ]]; then
    printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} CORS not configured\n"
    printf "      → Set HASURA_GRAPHQL_CORS_DOMAIN\n"
    issues=$((issues + 1))
  else
    printf "    ${COLOR_GREEN}✓${COLOR_RESET} CORS restricted to: %s\n" "$cors_origin"
  fi

  # Check for exposed admin ports
  local exposed_ports=()

  if grep -q "3021.*:.*3021" docker-compose.yml 2>/dev/null; then
    exposed_ports+=("3021 (admin)")
  fi

  if grep -q "9001.*:.*9001" docker-compose.yml 2>/dev/null; then
    exposed_ports+=("9001 (MinIO)")
  fi

  if [[ ${#exposed_ports[@]} -gt 0 ]]; then
    printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} Admin ports exposed: %s\n" "${exposed_ports[*]}"
    printf "      → Consider restricting access via firewall\n"
    issues=$((issues + 1))
  else
    printf "    ${COLOR_GREEN}✓${COLOR_RESET} No unnecessary exposed ports\n"
  fi

  printf "\n"
  return $issues
}

# Check database security
audit_database_security() {
  local issues=0
  local env_file="${1:-.env}"

  printf "  ${COLOR_BOLD}Database Security${COLOR_RESET}\n\n"

  # Check PostgreSQL password
  local postgres_password
  postgres_password=$(grep "^POSTGRES_PASSWORD=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d "'")

  if [[ -z "$postgres_password" ]]; then
    printf "    ${COLOR_RED}✗${COLOR_RESET} PostgreSQL password not set\n"
    printf "      → Set POSTGRES_PASSWORD in .env\n"
    issues=$((issues + 1))
  elif [[ ${#postgres_password} -lt 16 ]]; then
    printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} PostgreSQL password too short (%d chars)\n" "${#postgres_password}"
    printf "      → Use at least 16 characters\n"
    issues=$((issues + 1))
  else
    printf "    ${COLOR_GREEN}✓${COLOR_RESET} PostgreSQL password configured\n"
  fi

  # Check if PostgreSQL port is exposed
  local postgres_port_mapping
  postgres_port_mapping=$(grep -A 5 "postgres:" docker-compose.yml 2>/dev/null | grep "ports:" -A 1 | grep "5432" || true)

  if [[ -n "$postgres_port_mapping" ]]; then
    printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} PostgreSQL port exposed externally\n"
    printf "      → Consider removing port mapping for security\n"
    issues=$((issues + 1))
  else
    printf "    ${COLOR_GREEN}✓${COLOR_RESET} PostgreSQL not exposed externally\n"
  fi

  printf "\n"
  return $issues
}

# Check compliance readiness
audit_compliance() {
  local issues=0

  printf "  ${COLOR_BOLD}Compliance Readiness${COLOR_RESET}\n\n"

  # Check for audit logging
  if [[ -f "logs/audit.log" ]] || [[ -d "logs" ]]; then
    printf "    ${COLOR_GREEN}✓${COLOR_RESET} Audit logging directory exists\n"
  else
    printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} No audit logging directory\n"
    printf "      → Create logs directory for audit trails\n"
    issues=$((issues + 1))
  fi

  # Check for data retention policy
  if [[ -f "DATA_RETENTION_POLICY.md" ]] || grep -q "data.*retention" README.md 2>/dev/null; then
    printf "    ${COLOR_GREEN}✓${COLOR_RESET} Data retention policy documented\n"
  else
    printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} No documented data retention policy\n"
    printf "      → Document data retention for compliance\n"
    issues=$((issues + 1))
  fi

  # Check for privacy policy
  if [[ -f "PRIVACY.md" ]] || grep -q "privacy" README.md 2>/dev/null; then
    printf "    ${COLOR_GREEN}✓${COLOR_RESET} Privacy documentation exists\n"
  else
    printf "    ${COLOR_YELLOW}⚠${COLOR_RESET} No privacy documentation\n"
    printf "      → Add privacy policy for GDPR/compliance\n"
    issues=$((issues + 1))
  fi

  printf "\n"
  return $issues
}

# ============================================================================
# Main Audit Function
# ============================================================================

security_audit_comprehensive() {
  local env_file="${1:-.env}"
  local no_docker="${2:-false}"
  local format="${3:-}"

  if [[ ! -f "$env_file" ]]; then
    printf "${COLOR_RED}Error:${COLOR_RESET} Environment file not found: %s\n" "$env_file"
    return 1
  fi

  # JSON format output
  if [[ "$format" == "json" ]]; then
    local issues_json="[]"
    # Check for missing JWT secret
    local jwt_secret
    jwt_secret=$(grep "^JWT_SECRET=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d "'" || true)
    if [[ -z "$jwt_secret" ]]; then
      issues_json='[{"severity":"warning","check":"jwt_secret","message":"JWT_SECRET not set"}]'
    fi
    # Check admin secret
    local admin_secret
    admin_secret=$(grep "^HASURA_GRAPHQL_ADMIN_SECRET=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d "'" || true)
    if [[ -z "$admin_secret" ]] || [[ ${#admin_secret} -lt 32 ]]; then
      issues_json=$(printf '%s' "$issues_json" | sed 's/\]$/,{"severity":"warning","check":"admin_secret","message":"HASURA_GRAPHQL_ADMIN_SECRET too short"}]/')
      issues_json="${issues_json/,\]/\]}"
    fi
    printf '{"status":"complete","env_file":"%s","issues":%s}\n' "$env_file" "$issues_json"
    return 0
  fi

  local total_issues=0

  printf "\n"
  printf "${COLOR_BOLD}${COLOR_CYAN}╔═══════════════════════════════════════════════════════════════════════════╗${COLOR_RESET}\n"
  printf "${COLOR_BOLD}${COLOR_CYAN}║                                                                           ║${COLOR_RESET}\n"
  printf "${COLOR_BOLD}${COLOR_CYAN}║              nself SECURITY AUDIT                                        ║${COLOR_RESET}\n"
  printf "${COLOR_BOLD}${COLOR_CYAN}║              Production Readiness Check                                  ║${COLOR_RESET}\n"
  printf "${COLOR_BOLD}${COLOR_CYAN}║                                                                           ║${COLOR_RESET}\n"
  printf "${COLOR_BOLD}${COLOR_CYAN}╚═══════════════════════════════════════════════════════════════════════════╝${COLOR_RESET}\n"
  printf "\n"
  printf "  Audit started: %s\n" "$(date '+%Y-%m-%d %H:%M:%S')"
  printf "  Environment: %s\n" "$env_file"
  printf "\n"

  # Run all audit checks
  audit_ssl_configuration "$env_file" || total_issues=$((total_issues + $?))
  audit_auth_configuration "$env_file" || total_issues=$((total_issues + $?))
  audit_monitoring "$env_file" || total_issues=$((total_issues + $?))
  audit_backup || total_issues=$((total_issues + $?))
  audit_network_security "$env_file" || total_issues=$((total_issues + $?))
  audit_database_security "$env_file" || total_issues=$((total_issues + $?))
  audit_compliance || total_issues=$((total_issues + $?))

  # Final summary
  printf "\n"
  printf "${COLOR_BOLD}${COLOR_CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${COLOR_RESET}\n"
  printf "${COLOR_BOLD}AUDIT SUMMARY${COLOR_RESET}\n"
  printf "${COLOR_BOLD}${COLOR_CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${COLOR_RESET}\n"
  printf "\n"

  if [[ $total_issues -eq 0 ]]; then
    printf "  ${COLOR_GREEN}${COLOR_BOLD}✓ PASSED - Production Ready${COLOR_RESET}\n"
    printf "  All security checks passed.\n"
    printf "\n"
    return 0
  elif [[ $total_issues -lt 5 ]]; then
    printf "  ${COLOR_YELLOW}⚠ WARNINGS - %d issues found${COLOR_RESET}\n" "$total_issues"
    printf "  Address warnings before deploying to production.\n"
    printf "\n"
    return 1
  else
    printf "  ${COLOR_RED}✗ FAILED - %d issues found${COLOR_RESET}\n" "$total_issues"
    printf "  ${COLOR_RED}${COLOR_BOLD}NOT READY FOR PRODUCTION${COLOR_RESET}\n"
    printf "  Fix critical issues immediately.\n"
    printf "\n"
    return 1
  fi
}

# Generate security report
security_generate_report() {
  local output_file="${1:-security-report.txt}"
  local env_file="${2:-.env}"

  printf "Generating security report...\n"

  # Redirect all output to file
  {
    security_audit_comprehensive "$env_file"
  } >"$output_file" 2>&1

  printf "${COLOR_GREEN}✓${COLOR_RESET} Security report generated: %s\n" "$output_file"
  printf "  View report: cat %s\n" "$output_file"
}

# Export functions
export -f audit_ssl_configuration
export -f audit_auth_configuration
export -f audit_monitoring
export -f audit_backup
export -f audit_network_security
export -f audit_database_security
export -f audit_compliance
export -f security_audit_comprehensive
export -f security_generate_report
