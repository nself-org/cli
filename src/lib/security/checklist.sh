#!/usr/bin/env bash

# checklist.sh - Production security checklist and audit
# POSIX-compliant, no Bash 4+ features

# Get the directory where this script is located
SECURITY_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

LIB_ROOT="$(dirname "$SECURITY_LIB_DIR")"

# Source dependencies
source "$LIB_ROOT/utils/display.sh" 2>/dev/null || true
source "$LIB_ROOT/utils/platform-compat.sh" 2>/dev/null || true

# Security check results
SECURITY_PASSED=0
SECURITY_WARNINGS=0
SECURITY_FAILED=0

# Reset security counters
security::reset_counters() {
  SECURITY_PASSED=0
  SECURITY_WARNINGS=0
  SECURITY_FAILED=0
}

# Run full security audit
security::audit() {
  local env_name="${1:-}"
  local verbose="${2:-false}"

  security::reset_counters

  printf "${COLOR_CYAN}═══════════════════════════════════════════════════${COLOR_RESET}\n"
  printf "${COLOR_CYAN}       Production Security Checklist${COLOR_RESET}\n"
  printf "${COLOR_CYAN}═══════════════════════════════════════════════════${COLOR_RESET}\n\n"

  # Environment checks
  printf "${COLOR_YELLOW}Environment Configuration${COLOR_RESET}\n"
  security::check_env_settings "$verbose"
  printf "\n"

  # Secrets checks
  printf "${COLOR_YELLOW}Secrets Management${COLOR_RESET}\n"
  security::check_secrets "$verbose"
  printf "\n"

  # SSL/TLS checks
  printf "${COLOR_YELLOW}SSL/TLS Configuration${COLOR_RESET}\n"
  security::check_ssl "$verbose"
  printf "\n"

  # Docker security
  printf "${COLOR_YELLOW}Docker Security${COLOR_RESET}\n"
  security::check_docker_security "$verbose"
  printf "\n"

  # Network security
  printf "${COLOR_YELLOW}Network Security${COLOR_RESET}\n"
  security::check_network_security "$verbose"
  printf "\n"

  # File permissions
  printf "${COLOR_YELLOW}File Permissions${COLOR_RESET}\n"
  security::check_file_permissions "$verbose"
  printf "\n"

  # Summary
  security::print_summary

  # Return status based on failures
  if [[ $SECURITY_FAILED -gt 0 ]]; then
    return 1
  elif [[ $SECURITY_WARNINGS -gt 0 ]]; then
    return 2
  else
    return 0
  fi
}

# Check environment settings
security::check_env_settings() {
  local verbose="${1:-false}"

  # Check DEBUG mode
  local debug_mode
  debug_mode=$(grep "^DEBUG=" .env 2>/dev/null | cut -d'=' -f2)
  if [[ "$debug_mode" == "true" ]]; then
    security::fail "DEBUG mode is enabled (should be false in production)"
  else
    security::pass "DEBUG mode is disabled"
  fi

  # Check ENV setting
  local env_setting
  env_setting=$(grep "^ENV=" .env 2>/dev/null | cut -d'=' -f2)
  if [[ "$env_setting" == "production" ]] || [[ "$env_setting" == "prod" ]]; then
    security::pass "Environment is set to production"
  elif [[ "$env_setting" == "staging" ]]; then
    security::warn "Environment is set to staging (not production)"
  else
    security::warn "Environment is not set to production: ${env_setting:-<not set>}"
  fi

  # Check LOG_LEVEL
  local log_level
  log_level=$(grep "^LOG_LEVEL=" .env 2>/dev/null | cut -d'=' -f2)
  if [[ "$log_level" == "debug" ]]; then
    security::warn "LOG_LEVEL is set to debug (consider warning or error for production)"
  else
    security::pass "LOG_LEVEL is appropriate: ${log_level:-info}"
  fi

  # Check Hasura dev mode
  local hasura_dev
  hasura_dev=$(grep "^HASURA_GRAPHQL_DEV_MODE=" .env 2>/dev/null | cut -d'=' -f2)
  if [[ "$hasura_dev" == "true" ]]; then
    security::fail "Hasura dev mode is enabled (should be false in production)"
  else
    security::pass "Hasura dev mode is disabled"
  fi

  # Check Hasura console
  local hasura_console
  hasura_console=$(grep "^HASURA_GRAPHQL_ENABLE_CONSOLE=" .env 2>/dev/null | cut -d'=' -f2)
  if [[ "$hasura_console" == "true" ]]; then
    security::warn "Hasura console is enabled (consider disabling in production)"
  else
    security::pass "Hasura console is disabled"
  fi
}

# Check secrets configuration
security::check_secrets() {
  local verbose="${1:-false}"

  # Check if secrets file exists with proper permissions
  if [[ -f ".env.secrets" ]]; then
    local perms
    perms=$(safe_stat_perms ".env.secrets" 2>/dev/null)
    if [[ "$perms" == "600" ]]; then
      security::pass ".env.secrets has correct permissions (600)"
    else
      security::fail ".env.secrets has insecure permissions: $perms (should be 600)"
    fi
  else
    security::warn ".env.secrets file not found"
  fi

  # Check for default/weak passwords in .env
  local weak_passwords="password|secret|admin|123456|changeme|default"

  # Check POSTGRES_PASSWORD
  local pg_pass
  pg_pass=$(grep "^POSTGRES_PASSWORD=" .env .env.secrets 2>/dev/null | tail -1 | cut -d'=' -f2)
  if [[ -z "$pg_pass" ]]; then
    security::fail "POSTGRES_PASSWORD is not set"
  elif echo "$pg_pass" | grep -qiE "^($weak_passwords)$"; then
    security::fail "POSTGRES_PASSWORD is weak or default"
  elif [[ ${#pg_pass} -lt 16 ]]; then
    security::warn "POSTGRES_PASSWORD is less than 16 characters"
  else
    security::pass "POSTGRES_PASSWORD appears strong"
  fi

  # Check HASURA_GRAPHQL_ADMIN_SECRET
  local hasura_secret
  hasura_secret=$(grep "^HASURA_GRAPHQL_ADMIN_SECRET=" .env .env.secrets 2>/dev/null | tail -1 | cut -d'=' -f2)
  if [[ -z "$hasura_secret" ]]; then
    security::fail "HASURA_GRAPHQL_ADMIN_SECRET is not set"
  elif echo "$hasura_secret" | grep -qiE "^($weak_passwords|hasura)"; then
    security::fail "HASURA_GRAPHQL_ADMIN_SECRET is weak or default"
  elif [[ ${#hasura_secret} -lt 32 ]]; then
    security::warn "HASURA_GRAPHQL_ADMIN_SECRET is less than 32 characters"
  else
    security::pass "HASURA_GRAPHQL_ADMIN_SECRET appears strong"
  fi

  # Check JWT_SECRET
  local jwt_secret
  jwt_secret=$(grep "^JWT_SECRET=" .env .env.secrets 2>/dev/null | tail -1 | cut -d'=' -f2)
  if [[ -z "$jwt_secret" ]]; then
    security::fail "JWT_SECRET is not set"
  elif [[ ${#jwt_secret} -lt 32 ]]; then
    security::fail "JWT_SECRET is less than 32 characters (should be at least 32)"
  else
    security::pass "JWT_SECRET appears strong"
  fi

  # Check for secrets in .env (should be in .env.secrets)
  local env_secrets
  env_secrets=$(grep -E "^(.*PASSWORD|.*SECRET|.*KEY|.*TOKEN)=" .env 2>/dev/null | grep -v "^#" | wc -l)
  if [[ $env_secrets -gt 0 ]]; then
    security::warn "Found $env_secrets secret(s) in .env (consider moving to .env.secrets)"
  else
    security::pass "No secrets found in .env file"
  fi
}

# Check SSL/TLS configuration
security::check_ssl() {
  local verbose="${1:-false}"

  # Check SSL enabled
  local ssl_enabled
  ssl_enabled=$(grep "^SSL_ENABLED=" .env 2>/dev/null | cut -d'=' -f2)
  if [[ "$ssl_enabled" != "true" ]]; then
    security::fail "SSL is not enabled"
  else
    security::pass "SSL is enabled"
  fi

  # Check SSL provider
  local ssl_provider
  ssl_provider=$(grep "^SSL_PROVIDER=" .env 2>/dev/null | cut -d'=' -f2)
  if [[ "$ssl_provider" == "self-signed" ]]; then
    security::warn "Using self-signed SSL certificates (use letsencrypt for production)"
  elif [[ "$ssl_provider" == "letsencrypt" ]]; then
    security::pass "Using Let's Encrypt SSL certificates"
  elif [[ -n "$ssl_provider" ]]; then
    security::pass "Using custom SSL provider: $ssl_provider"
  else
    security::warn "SSL_PROVIDER not specified"
  fi

  # Check SSL certificate files
  if [[ -d "ssl" ]]; then
    if [[ -f "ssl/cert.pem" ]] && [[ -f "ssl/key.pem" ]]; then
      # Check key permissions
      local key_perms
      key_perms=$(safe_stat_perms "ssl/key.pem" 2>/dev/null)
      if [[ "$key_perms" == "600" ]] || [[ "$key_perms" == "400" ]]; then
        security::pass "SSL private key has correct permissions"
      else
        security::fail "SSL private key has insecure permissions: $key_perms"
      fi

      # Check certificate expiry (if openssl available)
      if command -v openssl >/dev/null 2>&1; then
        local expiry
        expiry=$(openssl x509 -enddate -noout -in "ssl/cert.pem" 2>/dev/null | cut -d'=' -f2)
        if [[ -n "$expiry" ]]; then
          local expiry_epoch
          local now_epoch
          expiry_epoch=$(date -j -f "%b %d %T %Y %Z" "$expiry" +%s 2>/dev/null || date -d "$expiry" +%s 2>/dev/null)
          now_epoch=$(date +%s)

          if [[ -n "$expiry_epoch" ]]; then
            local days_left=$(((expiry_epoch - now_epoch) / 86400))
            if [[ $days_left -lt 0 ]]; then
              security::fail "SSL certificate has expired"
            elif [[ $days_left -lt 7 ]]; then
              security::fail "SSL certificate expires in $days_left days"
            elif [[ $days_left -lt 30 ]]; then
              security::warn "SSL certificate expires in $days_left days"
            else
              security::pass "SSL certificate valid for $days_left days"
            fi
          fi
        fi
      fi
    else
      security::warn "SSL certificate files not found"
    fi
  fi
}

# Check Docker security settings
security::check_docker_security() {
  local verbose="${1:-false}"

  if [[ ! -f "docker-compose.yml" ]]; then
    security::warn "docker-compose.yml not found"
    return
  fi

  # Check for privileged containers
  local privileged
  privileged=$(grep -c "privileged: true" docker-compose.yml 2>/dev/null || echo 0)
  if [[ $privileged -gt 0 ]]; then
    security::warn "Found $privileged privileged container(s)"
  else
    security::pass "No privileged containers"
  fi

  # Check for host network mode
  local host_network
  host_network=$(grep -c "network_mode: host" docker-compose.yml 2>/dev/null || echo 0)
  if [[ $host_network -gt 0 ]]; then
    security::warn "Found $host_network container(s) using host network"
  else
    security::pass "No containers using host network"
  fi

  # Check for exposed ports on 0.0.0.0
  local exposed_all
  exposed_all=$(grep -E '^\s+- "0\.0\.0\.0:' docker-compose.yml 2>/dev/null | wc -l)
  if [[ $exposed_all -gt 0 ]]; then
    security::warn "Found $exposed_all port(s) exposed on 0.0.0.0 (consider 127.0.0.1)"
  else
    security::pass "No ports exposed on 0.0.0.0"
  fi

  # Check for resource limits
  local has_limits
  has_limits=$(grep -c "mem_limit\|cpus:" docker-compose.yml 2>/dev/null || echo 0)
  if [[ $has_limits -eq 0 ]]; then
    security::warn "No resource limits defined for containers"
  else
    security::pass "Resource limits are configured"
  fi
}

# Check network security
security::check_network_security() {
  local verbose="${1:-false}"

  # Check nginx security headers
  if [[ -d "nginx" ]]; then
    # Check for security headers include
    if [[ -f "nginx/includes/security-headers.conf" ]]; then
      security::pass "Security headers configuration file exists"
    else
      security::warn "Security headers configuration file not found"
    fi

    # Check for Content-Security-Policy (CSP)
    if grep -rq "Content-Security-Policy" nginx/ 2>/dev/null; then
      security::pass "Content-Security-Policy (CSP) header configured"

      # Check CSP mode/strictness - strict is the default since V098-P1-013
      if grep -rq "unsafe-eval" nginx/ 2>/dev/null; then
        security::warn "CSP allows unsafe-eval (set CSP_MODE=strict to harden)"
      elif grep -rq "unsafe-inline" nginx/ 2>/dev/null; then
        security::warn "CSP allows unsafe-inline (set CSP_MODE=strict to harden)"
      else
        security::pass "CSP uses strict mode (no unsafe-inline/unsafe-eval)"
      fi
    else
      security::fail "Content-Security-Policy (CSP) header not configured - CRITICAL"
    fi

    # Check for HSTS (only for SSL-enabled sites)
    local ssl_enabled
    ssl_enabled=$(grep "^SSL_ENABLED=" .env 2>/dev/null | cut -d'=' -f2)
    if [[ "$ssl_enabled" == "true" ]]; then
      if grep -rq "Strict-Transport-Security" nginx/ 2>/dev/null; then
        security::pass "HSTS header configured"

        # Check HSTS max-age
        local hsts_age
        hsts_age=$(grep -r "Strict-Transport-Security" nginx/ 2>/dev/null | grep -o "max-age=[0-9]*" | cut -d= -f2 | head -1)
        if [[ -n "$hsts_age" ]] && [[ $hsts_age -ge 31536000 ]]; then
          security::pass "HSTS max-age is adequate (>= 1 year)"
        elif [[ -n "$hsts_age" ]]; then
          security::warn "HSTS max-age is less than 1 year ($hsts_age seconds)"
        fi

        # Check for includeSubDomains
        if grep -rq "includeSubDomains" nginx/ 2>/dev/null; then
          security::pass "HSTS includes subdomains"
        else
          security::warn "HSTS does not include subdomains (consider adding)"
        fi
      else
        security::warn "HSTS header not found (recommended for HTTPS sites)"
      fi
    fi

    # Check for X-XSS-Protection
    if grep -rq "X-XSS-Protection" nginx/ 2>/dev/null; then
      security::pass "X-XSS-Protection header configured"
    else
      security::warn "X-XSS-Protection header not found"
    fi

    # Check for X-Frame-Options
    if grep -rq "X-Frame-Options" nginx/ 2>/dev/null; then
      security::pass "X-Frame-Options header configured"

      # Check value (DENY is most secure)
      if grep -rq 'X-Frame-Options.*"DENY"' nginx/ 2>/dev/null; then
        security::pass "X-Frame-Options set to DENY (most secure)"
      elif grep -rq 'X-Frame-Options.*"SAMEORIGIN"' nginx/ 2>/dev/null; then
        security::pass "X-Frame-Options set to SAMEORIGIN"
      fi
    else
      security::warn "X-Frame-Options header not found"
    fi

    # Check for X-Content-Type-Options
    if grep -rq "X-Content-Type-Options" nginx/ 2>/dev/null; then
      security::pass "X-Content-Type-Options header configured"
    else
      security::warn "X-Content-Type-Options header not found"
    fi

    # Check for Referrer-Policy
    if grep -rq "Referrer-Policy" nginx/ 2>/dev/null; then
      security::pass "Referrer-Policy header configured"
    else
      security::warn "Referrer-Policy header not found (recommended)"
    fi

    # Check for Permissions-Policy
    if grep -rq "Permissions-Policy" nginx/ 2>/dev/null; then
      security::pass "Permissions-Policy header configured"
    else
      security::info "Permissions-Policy header not configured (optional)"
    fi
  else
    security::warn "nginx directory not found"
  fi

  # Check for rate limiting
  if grep -rq "limit_req" nginx/ 2>/dev/null; then
    security::pass "Rate limiting is configured"
  else
    security::warn "Rate limiting not configured in nginx"
  fi
}

# Check file permissions
security::check_file_permissions() {
  local verbose="${1:-false}"

  # Check .env permissions
  if [[ -f ".env" ]]; then
    local env_perms
    env_perms=$(safe_stat_perms ".env" 2>/dev/null)
    if [[ "$env_perms" == "600" ]] || [[ "$env_perms" == "640" ]]; then
      security::pass ".env has secure permissions ($env_perms)"
    else
      security::warn ".env has permissions $env_perms (recommend 600 or 640)"
    fi
  fi

  # Check for world-readable sensitive files
  local sensitive_files=".env .env.secrets .env.local ssl/key.pem"
  for file in $sensitive_files; do
    if [[ -f "$file" ]]; then
      local perms
      perms=$(safe_stat_perms "$file" 2>/dev/null)
      # Check if world-readable (ends in 4, 5, 6, or 7 for "other" permissions)
      local other_perms="${perms: -1}"
      if [[ "$other_perms" -ge 4 ]]; then
        security::fail "$file is world-readable (permissions: $perms)"
      fi
    fi
  done

  # Check script permissions
  if [[ -d "src/cli" ]]; then
    local world_writable
    world_writable=$(find src/cli -type f -perm -002 2>/dev/null | wc -l)
    if [[ $world_writable -gt 0 ]]; then
      security::warn "Found $world_writable world-writable script(s) in src/cli"
    else
      security::pass "No world-writable scripts found"
    fi
  fi
}

# Helper functions for recording results
security::pass() {
  local message="$1"
  printf "  ${COLOR_GREEN}✓${COLOR_RESET} %s\n" "$message"
  SECURITY_PASSED=$((SECURITY_PASSED + 1))
}

security::warn() {
  local message="$1"
  printf "  ${COLOR_YELLOW}⚠${COLOR_RESET} %s\n" "$message"
  SECURITY_WARNINGS=$((SECURITY_WARNINGS + 1))
}

security::fail() {
  local message="$1"
  printf "  ${COLOR_RED}✗${COLOR_RESET} %s\n" "$message"
  SECURITY_FAILED=$((SECURITY_FAILED + 1))
}

security::info() {
  local message="$1"
  printf "  ${COLOR_BLUE}ℹ${COLOR_RESET} %s\n" "$message"
}

# Print security audit summary
security::print_summary() {
  local total=$((SECURITY_PASSED + SECURITY_WARNINGS + SECURITY_FAILED))

  printf "${COLOR_CYAN}═══════════════════════════════════════════════════${COLOR_RESET}\n"
  printf "${COLOR_CYAN}                    Summary${COLOR_RESET}\n"
  printf "${COLOR_CYAN}═══════════════════════════════════════════════════${COLOR_RESET}\n"

  printf "  ${COLOR_GREEN}Passed:${COLOR_RESET}   %d\n" "$SECURITY_PASSED"
  printf "  ${COLOR_YELLOW}Warnings:${COLOR_RESET} %d\n" "$SECURITY_WARNINGS"
  printf "  ${COLOR_RED}Failed:${COLOR_RESET}   %d\n" "$SECURITY_FAILED"
  printf "  ${COLOR_CYAN}Total:${COLOR_RESET}    %d checks\n" "$total"
  printf "\n"

  if [[ $SECURITY_FAILED -gt 0 ]]; then
    printf "${COLOR_RED}⚠ Security audit FAILED - address critical issues before deploying${COLOR_RESET}\n"
  elif [[ $SECURITY_WARNINGS -gt 0 ]]; then
    printf "${COLOR_YELLOW}⚠ Security audit passed with warnings - review before deploying${COLOR_RESET}\n"
  else
    printf "${COLOR_GREEN}✓ Security audit PASSED - ready for production${COLOR_RESET}\n"
  fi
}

# Quick security check (subset of full audit)
security::quick_check() {
  security::reset_counters

  printf "Running quick security check...\n\n"

  # Only critical checks
  security::check_env_settings "false"
  security::check_secrets "false"

  printf "\n"

  local total=$((SECURITY_PASSED + SECURITY_WARNINGS + SECURITY_FAILED))
  printf "Quick check: %d passed, %d warnings, %d failed\n" \
    "$SECURITY_PASSED" "$SECURITY_WARNINGS" "$SECURITY_FAILED"

  [[ $SECURITY_FAILED -eq 0 ]]
}

# Export functions
export -f security::audit
export -f security::reset_counters
export -f security::check_env_settings
export -f security::check_secrets
export -f security::check_ssl
export -f security::check_docker_security
export -f security::check_network_security
export -f security::check_file_permissions
export -f security::pass
export -f security::warn
export -f security::fail
export -f security::print_summary
export -f security::quick_check
