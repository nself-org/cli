#!/usr/bin/env bash
# build-warnings.sh - Security preflight for staging/prod builds
# Called at end of nself build when NSELF_ENV=staging or prod
# Bash 3.2+ compatible

# Prevent double-sourcing
[[ "${BUILD_WARNINGS_SOURCED:-}" == "1" ]] && return 0
export BUILD_WARNINGS_SOURCED=1

# ── Output helpers ───────────────────────────────────────────────────────────
_bw_warn() { printf "  \033[0;33m⚠\033[0m  %s\n" "$1"; }
_bw_fail() { printf "  \033[0;31m✗\033[0m  %s\n" "$1"; }
_bw_ok()   { printf "  \033[0;32m✓\033[0m  %s\n" "$1"; }
_bw_fix()  { printf "     → %s\n" "$1"; }

# ============================================================================
# Individual check functions
# ============================================================================

# Check 1: fail2ban
_bw_check_fail2ban() {
  if [[ "$(uname -s)" == "Darwin" ]]; then return 0; fi
  if ! command -v fail2ban-client >/dev/null 2>&1; then
    _bw_warn "fail2ban is not installed"
    _bw_fix "Install fail2ban: apt install fail2ban"
    return 1
  fi
  return 0
}

# Check 2: sshd_config
_bw_check_sshd() {
  if [[ "$(uname -s)" == "Darwin" ]]; then return 0; fi
  local sshd_config="${SSHD_CONFIG_PATH:-/etc/ssh/sshd_config}"
  if [[ ! -f "$sshd_config" ]]; then return 0; fi

  local issues=0

  local pw_auth
  pw_auth=$(grep -iE '^[[:space:]]*PasswordAuthentication[[:space:]]' "$sshd_config" | tail -1 | awk '{print $2}' | tr '[:upper:]' '[:lower:]' || true)
  if [[ "${pw_auth:-yes}" == "yes" ]]; then
    _bw_warn "sshd: PasswordAuthentication yes (allows brute-force)"
    _bw_fix "Set 'PasswordAuthentication no' in $sshd_config"
    issues=$((issues + 1))
  fi

  local root_login
  root_login=$(grep -iE '^[[:space:]]*PermitRootLogin[[:space:]]' "$sshd_config" | tail -1 | awk '{print $2}' | tr '[:upper:]' '[:lower:]' || true)
  if [[ "${root_login:-yes}" == "yes" ]]; then
    _bw_warn "sshd: PermitRootLogin yes (root SSH access is dangerous)"
    _bw_fix "Set 'PermitRootLogin no' in $sshd_config"
    issues=$((issues + 1))
  fi

  return $issues
}

# Check 3: Docker daemon iptables
_bw_check_docker_iptables() {
  if [[ "$(uname -s)" == "Darwin" ]]; then return 0; fi
  local daemon_json="/etc/docker/daemon.json"
  if [[ ! -f "$daemon_json" ]]; then return 0; fi

  if grep -q '"iptables"[[:space:]]*:[[:space:]]*false' "$daemon_json" 2>/dev/null; then
    _bw_warn "Docker iptables bypass detected — UFW rules may not apply to Docker ports"
    _bw_fix "Remove '\"iptables\": false' from $daemon_json and restart Docker"
    return 1
  fi
  return 0
}

# Check 4: Port bindings in docker-compose.yml
_bw_check_port_bindings() {
  local dc_file="${1:-docker-compose.yml}"
  if [[ ! -f "$dc_file" ]]; then return 0; fi

  local violations=0
  local in_service=""
  local in_ports=false

  while IFS= read -r line; do
    if printf '%s' "$line" | grep -qE '^[[:space:]]{2}[a-z]'; then
      in_service=$(printf '%s' "$line" | sed 's/[[:space:]]*\([a-z_-]*\):.*/\1/')
      in_ports=false
    fi
    if printf '%s' "$line" | grep -qE '^[[:space:]]*(ports|published):'; then
      in_ports=true
    fi
    if [[ "$in_ports" == "true" ]] && printf '%s' "$line" | grep -qE '0\.0\.0\.0:'; then
      if [[ "$in_service" != "nginx" ]]; then
        local port
        port=$(printf '%s' "$line" | grep -oE '[0-9]+:[0-9]+' | head -1)
        _bw_fail "Service '$in_service' port $port is exposed on 0.0.0.0 (bypasses nginx)"
        _bw_fix "Run nself build to regenerate docker-compose with correct bindings"
        violations=$((violations + 1))
      fi
    fi
    if [[ "$in_ports" == "true" ]] && ! printf '%s' "$line" | grep -qE '(ports|published|-)'; then
      in_ports=false
    fi
  done < "$dc_file"

  return $violations
}

# Check 5: HASURA_GRAPHQL_ENABLE_CONSOLE in production
_bw_check_hasura_console() {
  local env_file="${1:-.env}"
  local console_val
  console_val=$(grep "^HASURA_GRAPHQL_ENABLE_CONSOLE=" "$env_file" 2>/dev/null | tail -1 | cut -d'=' -f2- | tr -d '"' | tr -d "'" || true)
  if [[ "$console_val" == "true" ]]; then
    _bw_warn "HASURA_GRAPHQL_ENABLE_CONSOLE=true in production"
    _bw_fix "Set HASURA_GRAPHQL_ENABLE_CONSOLE=false in your .env.prod"
    return 1
  fi
  return 0
}

# Check 6: DEBUG flag in production
_bw_check_debug() {
  local env_file="${1:-.env}"
  local debug_val
  debug_val=$(grep "^DEBUG=" "$env_file" 2>/dev/null | tail -1 | cut -d'=' -f2- | tr -d '"' | tr -d "'" || true)
  if [[ "$debug_val" == "true" ]]; then
    _bw_warn "DEBUG=true in production"
    _bw_fix "Set DEBUG=false in your .env.prod"
    return 1
  fi
  return 0
}

# Check 7: JWT secret length
_bw_check_jwt_secret() {
  local jwt_secret="${HASURA_GRAPHQL_JWT_SECRET:-${JWT_SECRET:-}}"
  # Strip surrounding JSON if present
  local secret_val
  secret_val=$(printf '%s' "$jwt_secret" | grep -o '"key"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4 || printf '%s' "$jwt_secret")
  if [[ -z "$secret_val" ]]; then
    secret_val="$jwt_secret"
  fi

  if [[ -z "$secret_val" ]]; then
    _bw_fail "JWT secret is not set (HASURA_GRAPHQL_JWT_SECRET)"
    _bw_fix "Generate with: openssl rand -hex 32"
    return 1
  fi

  if [[ "${#secret_val}" -lt 32 ]]; then
    _bw_fail "JWT secret is too short (${#secret_val} chars, minimum 32 required)"
    _bw_fix "Generate with: openssl rand -hex 32"
    return 1
  fi
  return 0
}

# Check 8: Hasura admin secret length
_bw_check_admin_secret() {
  local admin_secret="${HASURA_GRAPHQL_ADMIN_SECRET:-}"
  if [[ -z "$admin_secret" ]]; then
    _bw_fail "HASURA_GRAPHQL_ADMIN_SECRET is not set"
    _bw_fix "Generate with: openssl rand -hex 16"
    return 1
  fi
  if [[ "${#admin_secret}" -lt 16 ]]; then
    _bw_fail "HASURA_GRAPHQL_ADMIN_SECRET is too short (${#admin_secret} chars, minimum 16 required)"
    _bw_fix "Generate with: openssl rand -hex 16"
    return 1
  fi
  return 0
}

# ============================================================================
# MAIN: run_build_security_warnings
# Called at the end of nself build for staging/prod
# ============================================================================

run_build_security_warnings() {
  local env="${NSELF_ENV:-${ENV:-dev}}"
  local dc_file="${1:-docker-compose.yml}"
  local env_file="${2:-.env}"

  # Dev/local: skip all checks
  case "$env" in
    dev|local|development|test)
      return 0
      ;;
  esac

  # Only run for staging/prod
  case "$env" in
    staging|prod|production) ;;
    *) return 0 ;;
  esac

  local warning_count=0
  local error_count=0

  printf "\n\033[0;33mSecurity Preflight\033[0m (%s)\n" "$env"

  # Run checks
  _bw_check_fail2ban    || warning_count=$((warning_count + 1))
  _bw_check_sshd        || warning_count=$((warning_count + 1))
  _bw_check_docker_iptables || warning_count=$((warning_count + 1))
  _bw_check_port_bindings "$dc_file" || error_count=$((error_count + 1))
  _bw_check_hasura_console "$env_file" || warning_count=$((warning_count + 1))
  _bw_check_debug "$env_file" || warning_count=$((warning_count + 1))
  _bw_check_jwt_secret   || error_count=$((error_count + 1))
  _bw_check_admin_secret || error_count=$((error_count + 1))

  local total=$((warning_count + error_count))
  if [[ $total -gt 0 ]]; then
    printf "\n  Security: %d warning(s), %d error(s) — run 'nself security audit' for details\n\n" \
      "$warning_count" "$error_count"
    if [[ $error_count -gt 0 ]]; then
      return 1
    fi
  else
    _bw_ok "Security preflight passed"
    printf "\n"
  fi

  return 0
}

export -f run_build_security_warnings
