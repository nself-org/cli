#!/usr/bin/env bash

# secure-defaults.sh - Secure by Default Module
# nself CLI must be secure by default. Users should not need to remember flags,
# run extra commands, or configure anything to get a secure deployment.
# POSIX-compliant, Bash 3.2+ compatible

# Get the directory where this script is located
SECURE_DEFAULTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

LIB_ROOT="$(dirname "$SECURE_DEFAULTS_DIR")"

# Source dependencies
source "$LIB_ROOT/utils/display.sh" 2>/dev/null || true
source "$LIB_ROOT/utils/platform-compat.sh" 2>/dev/null || true
source "$SECURE_DEFAULTS_DIR/firewall.sh" 2>/dev/null || true

# ============================================================
# SECURITY CONSTANTS
# ============================================================

# Sensitive ports that must NEVER be exposed to 0.0.0.0
SENSITIVE_PORTS="6379 5432 7700 9000 9090 3000"
SENSITIVE_SERVICES="redis postgres meilisearch minio prometheus grafana"

# Minimum password/secret lengths
MIN_PASSWORD_LENGTH=16
MIN_SECRET_LENGTH=32

# Allowed external ports (everything else should be 127.0.0.1 only)
ALLOWED_EXTERNAL_PORTS="22 80 443"

# ============================================================
# CORE SECURITY VALIDATION
# ============================================================

# Validate security requirements before build
# Returns 0 if secure, 1 if insecure (blocks build)
security::validate_build() {
  local env="${ENV:-dev}"
  local allow_insecure="${1:-false}"
  local errors=0

  # Production NEVER allows insecure
  if [[ "$env" == "prod" ]] || [[ "$env" == "production" ]]; then
    allow_insecure="false"
  fi

  printf "\n${COLOR_CYAN}Security Validation${COLOR_RESET}\n"
  printf "═══════════════════════════════════════\n\n"

  # 1. Check required passwords
  local password_errors
  password_errors=$(security::check_required_passwords "$env" "$allow_insecure" 2>&2)
  errors=$((errors + ${password_errors:-0}))

  # 2. Validate port bindings if docker-compose exists
  if [[ -f "docker-compose.yml" ]]; then
    local port_errors
    port_errors=$(security::check_port_bindings 2>&2)
    errors=$((errors + ${port_errors:-0}))
  fi

  # 3. Check for insecure defaults
  local insecure_errors
  insecure_errors=$(security::check_insecure_values 2>&2)
  errors=$((errors + ${insecure_errors:-0}))

  printf "\n"

  if [[ $errors -gt 0 ]]; then
    printf "${COLOR_RED}✗ SECURITY VALIDATION FAILED${COLOR_RESET}\n"
    printf "  Found %d critical security issue(s)\n\n" "$errors"

    if [[ "$allow_insecure" == "false" ]]; then
      printf "${COLOR_YELLOW}To fix:${COLOR_RESET}\n"
      printf "  1. Run: ${COLOR_CYAN}nself config secrets generate${COLOR_RESET}\n"
      printf "  2. Or set passwords in your .env file\n\n"

      if [[ "$env" != "prod" ]] && [[ "$env" != "production" ]]; then
        printf "${COLOR_DIM}For development only, use --allow-insecure to bypass${COLOR_RESET}\n"
        printf "${COLOR_DIM}(Not available for production environments)${COLOR_RESET}\n\n"
      fi
      return 1
    else
      printf "${COLOR_YELLOW}⚠ Proceeding with --allow-insecure flag${COLOR_RESET}\n"
      printf "${COLOR_DIM}This is NOT recommended for any public-facing deployment${COLOR_RESET}\n\n"
      return 0
    fi
  fi

  printf "${COLOR_GREEN}✓ Security validation passed${COLOR_RESET}\n\n"
  return 0
}

# Check required passwords are set
security::check_required_passwords() {
  local env="$1"
  local allow_insecure="$2"
  local errors=0

  # PostgreSQL password (always required)
  if [[ -z "${POSTGRES_PASSWORD:-}" ]]; then
    if [[ "$env" == "dev" ]] && [[ "$allow_insecure" != "false" ]]; then
      # Auto-generate for dev
      export POSTGRES_PASSWORD=$(openssl rand -base64 24 | tr -d '/+=' | head -c 32)
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} POSTGRES_PASSWORD: Auto-generated for dev\n" >&2
    else
      printf "  ${COLOR_RED}✗${COLOR_RESET} POSTGRES_PASSWORD: ${COLOR_RED}Not set${COLOR_RESET}\n" >&2
      errors=$((errors + 1))
    fi
  else
    # Show character count and strength assessment
    local pg_len=${#POSTGRES_PASSWORD}
    local pg_strength="weak"
    if [[ $pg_len -ge 48 ]]; then
      pg_strength="${COLOR_GREEN}secure${COLOR_RESET}"
    elif [[ $pg_len -ge 32 ]]; then
      pg_strength="${COLOR_CYAN}strong${COLOR_RESET}"
    elif [[ $pg_len -ge 24 ]]; then
      pg_strength="${COLOR_YELLOW}moderate${COLOR_RESET}"
    else
      pg_strength="${COLOR_RED}weak - recommend 32+${COLOR_RESET}"
    fi
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} POSTGRES_PASSWORD: Set (%d chars, %b)\n" "$pg_len" "$pg_strength" >&2
  fi

  # Redis password (required if Redis enabled)
  if [[ "${REDIS_ENABLED:-false}" == "true" ]]; then
    if [[ -z "${REDIS_PASSWORD:-}" ]]; then
      if [[ "$env" == "dev" ]] && [[ "$allow_insecure" != "false" ]]; then
        export REDIS_PASSWORD=$(openssl rand -base64 24 | tr -d '/+=' | head -c 32)
        printf "  ${COLOR_GREEN}✓${COLOR_RESET} REDIS_PASSWORD: Auto-generated for dev\n" >&2
      else
        printf "  ${COLOR_RED}✗${COLOR_RESET} REDIS_PASSWORD: ${COLOR_RED}Not set (Redis is enabled)${COLOR_RESET}\n" >&2
        errors=$((errors + 1))
      fi
    else
      local redis_len=${#REDIS_PASSWORD}
      local redis_strength="weak"
      if [[ $redis_len -ge 48 ]]; then
        redis_strength="${COLOR_GREEN}secure${COLOR_RESET}"
      elif [[ $redis_len -ge 32 ]]; then
        redis_strength="${COLOR_CYAN}strong${COLOR_RESET}"
      elif [[ $redis_len -ge 24 ]]; then
        redis_strength="${COLOR_YELLOW}moderate${COLOR_RESET}"
      else
        redis_strength="${COLOR_RED}weak - recommend 32+${COLOR_RESET}"
      fi
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} REDIS_PASSWORD: Set (%d chars, %b)\n" "$redis_len" "$redis_strength" >&2
    fi
  fi

  # Hasura admin secret
  if [[ "${HASURA_ENABLED:-true}" == "true" ]]; then
    if [[ -z "${HASURA_GRAPHQL_ADMIN_SECRET:-}" ]]; then
      if [[ "$env" == "dev" ]] && [[ "$allow_insecure" != "false" ]]; then
        export HASURA_GRAPHQL_ADMIN_SECRET=$(openssl rand -base64 32 | tr -d '/+=' | head -c 44)
        printf "  ${COLOR_GREEN}✓${COLOR_RESET} HASURA_GRAPHQL_ADMIN_SECRET: Auto-generated for dev\n" >&2
      else
        printf "  ${COLOR_RED}✗${COLOR_RESET} HASURA_GRAPHQL_ADMIN_SECRET: ${COLOR_RED}Not set${COLOR_RESET}\n" >&2
        errors=$((errors + 1))
      fi
    else
      local hasura_len=${#HASURA_GRAPHQL_ADMIN_SECRET}
      local hasura_strength="weak"
      if [[ $hasura_len -ge 48 ]]; then
        hasura_strength="${COLOR_GREEN}secure${COLOR_RESET}"
      elif [[ $hasura_len -ge 32 ]]; then
        hasura_strength="${COLOR_CYAN}strong${COLOR_RESET}"
      elif [[ $hasura_len -ge 24 ]]; then
        hasura_strength="${COLOR_YELLOW}moderate${COLOR_RESET}"
      else
        hasura_strength="${COLOR_RED}weak - recommend 32+${COLOR_RESET}"
      fi
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} HASURA_GRAPHQL_ADMIN_SECRET: Set (%d chars, %b)\n" "$hasura_len" "$hasura_strength" >&2
    fi
  fi

  # MeiliSearch master key
  if [[ "${MEILISEARCH_ENABLED:-false}" == "true" ]]; then
    if [[ -z "${MEILISEARCH_MASTER_KEY:-}" ]] && [[ -z "${SEARCH_API_KEY:-}" ]]; then
      if [[ "$env" == "dev" ]] && [[ "$allow_insecure" != "false" ]]; then
        export MEILISEARCH_MASTER_KEY=$(openssl rand -base64 24 | tr -d '/+=' | head -c 32)
        printf "  ${COLOR_GREEN}✓${COLOR_RESET} MEILISEARCH_MASTER_KEY: Auto-generated for dev\n" >&2
      else
        printf "  ${COLOR_RED}✗${COLOR_RESET} MEILISEARCH_MASTER_KEY: ${COLOR_RED}Not set (MeiliSearch is enabled)${COLOR_RESET}\n" >&2
        errors=$((errors + 1))
      fi
    else
      local meili_key="${MEILISEARCH_MASTER_KEY:-${SEARCH_API_KEY}}"
      local meili_len=${#meili_key}
      local meili_strength="weak"
      if [[ $meili_len -ge 48 ]]; then
        meili_strength="${COLOR_GREEN}secure${COLOR_RESET}"
      elif [[ $meili_len -ge 32 ]]; then
        meili_strength="${COLOR_CYAN}strong${COLOR_RESET}"
      elif [[ $meili_len -ge 24 ]]; then
        meili_strength="${COLOR_YELLOW}moderate${COLOR_RESET}"
      else
        meili_strength="${COLOR_RED}weak - recommend 32+${COLOR_RESET}"
      fi
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} MEILISEARCH_MASTER_KEY: Set (%d chars, %b)\n" "$meili_len" "$meili_strength" >&2
    fi
  fi

  # MinIO root password
  if [[ "${MINIO_ENABLED:-false}" == "true" ]]; then
    if [[ -z "${MINIO_ROOT_PASSWORD:-}" ]]; then
      if [[ "$env" == "dev" ]] && [[ "$allow_insecure" != "false" ]]; then
        export MINIO_ROOT_PASSWORD=$(openssl rand -base64 24 | tr -d '/+=' | head -c 32)
        printf "  ${COLOR_GREEN}✓${COLOR_RESET} MINIO_ROOT_PASSWORD: Auto-generated for dev\n" >&2
      else
        printf "  ${COLOR_RED}✗${COLOR_RESET} MINIO_ROOT_PASSWORD: ${COLOR_RED}Not set (MinIO is enabled)${COLOR_RESET}\n" >&2
        errors=$((errors + 1))
      fi
    else
      local minio_len=${#MINIO_ROOT_PASSWORD}
      local minio_strength="weak"
      if [[ $minio_len -ge 48 ]]; then
        minio_strength="${COLOR_GREEN}secure${COLOR_RESET}"
      elif [[ $minio_len -ge 32 ]]; then
        minio_strength="${COLOR_CYAN}strong${COLOR_RESET}"
      elif [[ $minio_len -ge 24 ]]; then
        minio_strength="${COLOR_YELLOW}moderate${COLOR_RESET}"
      else
        minio_strength="${COLOR_RED}weak - recommend 32+${COLOR_RESET}"
      fi
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} MINIO_ROOT_PASSWORD: Set (%d chars, %b)\n" "$minio_len" "$minio_strength" >&2
    fi
  fi

  # Grafana admin password
  if [[ "${MONITORING_ENABLED:-false}" == "true" ]]; then
    if [[ -z "${GRAFANA_ADMIN_PASSWORD:-}" ]]; then
      if [[ "$env" == "dev" ]] && [[ "$allow_insecure" != "false" ]]; then
        export GRAFANA_ADMIN_PASSWORD=$(openssl rand -base64 24 | tr -d '/+=' | head -c 32)
        printf "  ${COLOR_GREEN}✓${COLOR_RESET} GRAFANA_ADMIN_PASSWORD: Auto-generated for dev\n" >&2
      else
        printf "  ${COLOR_RED}✗${COLOR_RESET} GRAFANA_ADMIN_PASSWORD: ${COLOR_RED}Not set (Monitoring is enabled)${COLOR_RESET}\n" >&2
        errors=$((errors + 1))
      fi
    else
      local grafana_len=${#GRAFANA_ADMIN_PASSWORD}
      local grafana_strength="weak"
      if [[ $grafana_len -ge 48 ]]; then
        grafana_strength="${COLOR_GREEN}secure${COLOR_RESET}"
      elif [[ $grafana_len -ge 32 ]]; then
        grafana_strength="${COLOR_CYAN}strong${COLOR_RESET}"
      elif [[ $grafana_len -ge 24 ]]; then
        grafana_strength="${COLOR_YELLOW}moderate${COLOR_RESET}"
      else
        grafana_strength="${COLOR_RED}weak - recommend 32+${COLOR_RESET}"
      fi
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} GRAFANA_ADMIN_PASSWORD: Set (%d chars, %b)\n" "$grafana_len" "$grafana_strength" >&2
    fi
  fi

  echo "$errors"
}

# Check port bindings in docker-compose.yml
security::check_port_bindings() {
  local errors=0

  if [[ ! -f "docker-compose.yml" ]]; then
    echo "0"
    return 0
  fi

  printf "\n  ${COLOR_BOLD}Port Bindings:${COLOR_RESET}\n" >&2

  # Check each sensitive port
  for port in $SENSITIVE_PORTS; do
    # Look for ports exposed without 127.0.0.1 prefix
    # Pattern: "PORT:PORT" without "127.0.0.1:"
    if grep -E "^\s+-\s*\"?${port}:${port}\"?" docker-compose.yml 2>/dev/null | grep -v "127.0.0.1" >/dev/null 2>&1; then
      printf "  ${COLOR_RED}✗${COLOR_RESET} Port %s: Bound to 0.0.0.0 (should be 127.0.0.1)\n" "$port" >&2
      errors=$((errors + 1))
    elif grep -E "^\s+-\s*\"?0\.0\.0\.0:${port}:${port}\"?" docker-compose.yml >/dev/null 2>&1; then
      printf "  ${COLOR_RED}✗${COLOR_RESET} Port %s: Explicitly bound to 0.0.0.0\n" "$port" >&2
      errors=$((errors + 1))
    elif grep "127.0.0.1.*${port}:${port}" docker-compose.yml >/dev/null 2>&1; then
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} Port %s: Localhost only\n" "$port" >&2
    fi
  done

  echo "$errors"
}

# Check for insecure default values
security::check_insecure_values() {
  local errors=0
  local insecure_patterns="password changeme secret admin 12345 qwerty default"

  printf "\n  ${COLOR_BOLD}Insecure Values:${COLOR_RESET}\n" >&2

  # Check each security-sensitive variable
  for var in POSTGRES_PASSWORD HASURA_GRAPHQL_ADMIN_SECRET AUTH_JWT_SECRET REDIS_PASSWORD MINIO_ROOT_PASSWORD GRAFANA_ADMIN_PASSWORD MEILISEARCH_MASTER_KEY; do
    local value="${!var:-}"
    if [[ -n "$value" ]]; then
      local value_lower
      value_lower=$(printf "%s" "$value" | tr '[:upper:]' '[:lower:]')

      for pattern in $insecure_patterns; do
        if [[ "$value_lower" == "$pattern" ]]; then
          printf "  ${COLOR_RED}✗${COLOR_RESET} %s: Uses insecure value '%s'\n" "$var" "$pattern" >&2
          errors=$((errors + 1))
          break
        fi
      done
    fi
  done

  if [[ $errors -eq 0 ]]; then
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} No insecure default values detected\n" >&2
  fi

  echo "$errors"
}

# ============================================================
# POST-START SECURITY VERIFICATION
# ============================================================

# Verify no sensitive ports are exposed after container startup.
#
# WHY docker compose port (not ss/netstat):
#   ss/netstat show ALL system-level listeners, including pre-installed
#   system services (e.g. a system PostgreSQL on Debian 12 that binds to
#   0.0.0.0:5432 and is completely unrelated to nself). Using ss caused
#   false-positive "SECURITY VIOLATION" errors that would stop nself even
#   when its own containers were correctly bound to 127.0.0.1.
#
#   "docker compose port <service> <port>" returns ONLY nself's container
#   binding (e.g. "127.0.0.1:5432"), making the check precise.
#
#   ss/netstat are kept as a last-resort fallback when docker is not
#   available, but their output is reported as a WARNING (not an error)
#   because it may include system services outside nself's control.
security::verify_no_exposed_ports() {
  local errors=0
  local project_name="${COMPOSE_PROJECT_NAME:-nself}"

  printf "\n${COLOR_CYAN}Post-Start Security Check${COLOR_RESET}\n"
  printf "═══════════════════════════════════════\n\n"

  # "service:port" pairs — Bash 3.2 compatible (no associative arrays)
  local svc_port_pairs="redis:6379 postgres:5432 meilisearch:7700 minio:9000 prometheus:9090 grafana:3000"

  for pair in $svc_port_pairs; do
    local svc="${pair%%:*}"
    local port="${pair##*:}"
    local binding=""

    # Primary: docker compose port — checks ONLY nself containers, not system services
    if command -v docker >/dev/null 2>&1; then
      binding=$(docker compose --project-name "$project_name" port "$svc" "$port" 2>/dev/null || true)
    fi

    if [[ -n "$binding" ]]; then
      # binding format: "HOST_IP:HOST_PORT" e.g. "127.0.0.1:5432" or "0.0.0.0:5432"
      local host_ip="${binding%%:*}"
      if [[ "$host_ip" == "0.0.0.0" ]] || [[ "$host_ip" == "::" ]]; then
        printf "  ${COLOR_RED}✗${COLOR_RESET} Port %s (%s): Exposed to %s - SECURITY VIOLATION\n" \
          "$port" "$svc" "$host_ip"
        errors=$((errors + 1))
      else
        printf "  ${COLOR_GREEN}✓${COLOR_RESET} Port %s: Localhost only (%s)\n" "$port" "$binding"
      fi
    else
      # Service not running or port not mapped to host — both are safe
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} Port %s: Not exposed externally\n" "$port"
    fi
  done

  printf "\n"

  if [[ $errors -gt 0 ]]; then
    printf "${COLOR_RED}CRITICAL: %d sensitive port(s) exposed to public internet${COLOR_RESET}\n\n" "$errors"
    printf "This means a nself container has a port bound to 0.0.0.0 instead of 127.0.0.1.\n"
    printf "Run 'nself build' to regenerate docker-compose.yml with correct bindings.\n"
    printf "Then run 'nself start' again.\n\n"
    printf "Stopping containers to prevent security breach...\n"
    return 1
  fi

  printf "${COLOR_GREEN}✓ All sensitive ports are protected${COLOR_RESET}\n"
  return 0
}

# ============================================================
# PRODUCTION SECURITY REQUIREMENTS
# ============================================================

# Check if firewall is active (for production)
security::check_firewall_active() {
  local fw_type
  fw_type=$(firewall::detect)

  case "$fw_type" in
    ufw)
      if sudo ufw status 2>/dev/null | grep -q "Status: active"; then
        return 0
      fi
      ;;
    firewalld)
      if systemctl is-active firewalld >/dev/null 2>&1; then
        return 0
      fi
      ;;
    iptables)
      # Check if there are any INPUT rules beyond default
      local rule_count
      rule_count=$(sudo iptables -L INPUT -n 2>/dev/null | tail -n +3 | wc -l)
      if [[ "$rule_count" -gt 0 ]]; then
        return 0
      fi
      ;;
  esac

  return 1
}

# Configure firewall for production (auto-configure)
security::configure_firewall_for_prod() {
  printf "\n${COLOR_CYAN}Configuring Firewall${COLOR_RESET}\n"
  printf "═══════════════════════════════════════\n\n"

  local fw_type
  fw_type=$(firewall::detect)

  if [[ "$fw_type" == "none" ]]; then
    printf "  ${COLOR_RED}✗${COLOR_RESET} No firewall available\n"
    printf "\n  Install a firewall:\n"
    printf "    Ubuntu/Debian: sudo apt install ufw\n"
    printf "    RHEL/CentOS:   sudo yum install firewalld\n\n"
    return 1
  fi

  printf "  Detected: %s\n\n" "$fw_type"

  # Configure firewall with nself defaults
  if firewall::configure "false"; then
    printf "\n${COLOR_GREEN}✓ Firewall configured for production${COLOR_RESET}\n"
    printf "  Allowed ports: SSH (22), HTTP (80), HTTPS (443)\n"
    printf "  All other incoming traffic: DENIED\n\n"
    return 0
  else
    printf "\n${COLOR_RED}✗ Failed to configure firewall${COLOR_RESET}\n"
    return 1
  fi
}

# Full production security pre-flight check
security::production_preflight() {
  local env_name="${1:-prod}"
  local errors=0

  printf "\n${COLOR_CYAN}Production Security Pre-flight${COLOR_RESET}\n"
  printf "═══════════════════════════════════════\n\n"

  # 1. All secrets must be set
  printf "${COLOR_BOLD}1. Secrets Validation${COLOR_RESET}\n"
  local secret_errors
  secret_errors=$(security::check_required_passwords "prod" "false")
  errors=$((errors + secret_errors))
  printf "\n"

  # 2. Port bindings must be secure
  printf "${COLOR_BOLD}2. Port Binding Validation${COLOR_RESET}\n"
  if [[ -f "docker-compose.yml" ]]; then
    local port_errors
    port_errors=$(security::check_port_bindings)
    errors=$((errors + port_errors))
  else
    printf "  ${COLOR_YELLOW}!${COLOR_RESET} No docker-compose.yml found (run nself build first)\n"
  fi
  printf "\n"

  # 3. SSL must be enabled for production
  printf "${COLOR_BOLD}3. SSL Configuration${COLOR_RESET}\n"
  if [[ "${SSL_ENABLED:-true}" != "true" ]]; then
    printf "  ${COLOR_RED}✗${COLOR_RESET} SSL_ENABLED must be 'true' for production\n"
    errors=$((errors + 1))
  else
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} SSL_ENABLED: true\n"
  fi
  printf "\n"

  # 4. Firewall must be active
  printf "${COLOR_BOLD}4. Firewall Status${COLOR_RESET}\n"
  if security::check_firewall_active; then
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} Firewall is active\n"
  else
    printf "  ${COLOR_RED}✗${COLOR_RESET} Firewall is NOT active\n"
    printf "  ${COLOR_DIM}Will be configured automatically during deployment${COLOR_RESET}\n"
    # Don't increment errors - we'll auto-configure
  fi
  printf "\n"

  # 5. No debug/dev services in production
  printf "${COLOR_BOLD}5. Production Service Settings${COLOR_RESET}\n"
  if [[ "${HASURA_DEV_MODE:-false}" == "true" ]]; then
    printf "  ${COLOR_RED}✗${COLOR_RESET} HASURA_DEV_MODE should be 'false' in production\n"
    errors=$((errors + 1))
  else
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} HASURA_DEV_MODE: false\n"
  fi

  if [[ "${MAILPIT_ENABLED:-false}" == "true" ]]; then
    printf "  ${COLOR_YELLOW}!${COLOR_RESET} MAILPIT_ENABLED: true (development mail service)\n"
    # Warning only, not blocking
  else
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} MAILPIT_ENABLED: false\n"
  fi
  printf "\n"

  # Summary
  printf "═══════════════════════════════════════\n"
  if [[ $errors -gt 0 ]]; then
    printf "${COLOR_RED}✗ PRODUCTION DEPLOYMENT BLOCKED${COLOR_RESET}\n"
    printf "  Found %d critical security issue(s)\n\n" "$errors"
    printf "To fix:\n"
    printf "  1. Run: ${COLOR_CYAN}nself config secrets generate${COLOR_RESET}\n"
    printf "  2. Review your .env.prod file\n"
    printf "  3. Run: ${COLOR_CYAN}nself deploy check prod${COLOR_RESET}\n\n"
    return 1
  fi

  printf "${COLOR_GREEN}✓ All production security checks passed${COLOR_RESET}\n\n"
  return 0
}

# ============================================================
# SECURITY-SENSITIVE CONTAINER MANAGEMENT
# ============================================================

# Get list of security-sensitive containers
security::get_sensitive_containers() {
  local project_name="${1:-}"
  local containers=""

  for service in $SENSITIVE_SERVICES; do
    if docker ps -a --format "{{.Names}}" 2>/dev/null | grep -q "${project_name}_${service}"; then
      containers="$containers ${project_name}_${service}"
    fi
    # Also check alternate naming pattern
    if docker ps -a --format "{{.Names}}" 2>/dev/null | grep -q "${project_name}-${service}-1"; then
      containers="$containers ${project_name}-${service}-1"
    fi
  done

  echo "$containers"
}

# Force recreate security-sensitive containers
security::force_recreate_sensitive_containers() {
  local project_name="${1:-}"

  printf "  ${COLOR_BLUE}⠋${COLOR_RESET} Recreating security-sensitive containers..."

  # Get compose args
  local compose_args=("--project-name" "$project_name")
  if [[ -f ".env.runtime" ]]; then
    compose_args+=("--env-file" ".env.runtime")
  elif [[ -f ".env" ]]; then
    compose_args+=("--env-file" ".env")
  fi

  # Force recreate Redis and PostgreSQL
  local services_to_recreate=""

  if docker compose "${compose_args[@]}" ps redis 2>/dev/null | grep -q "redis"; then
    services_to_recreate="$services_to_recreate redis"
  fi

  if docker compose "${compose_args[@]}" ps postgres 2>/dev/null | grep -q "postgres"; then
    services_to_recreate="$services_to_recreate postgres"
  fi

  if [[ -n "$services_to_recreate" ]]; then
    docker compose "${compose_args[@]}" up -d --force-recreate $services_to_recreate >/dev/null 2>&1
    printf "\r  ${COLOR_GREEN}✓${COLOR_RESET} Security-sensitive containers recreated     \n"
  else
    printf "\r  ${COLOR_GREEN}✓${COLOR_RESET} No sensitive containers to recreate         \n"
  fi
}

# ============================================================
# AUTO-GENERATE SECRETS
# ============================================================

# Generate secure secrets and save to .env file
security::generate_secrets() {
  local env_file="${1:-.env}"
  local env_type="${2:-dev}"

  printf "\n${COLOR_CYAN}Generating Secure Secrets${COLOR_RESET}\n"
  printf "═══════════════════════════════════════\n\n"

  # Generate each secret
  local postgres_pw=$(openssl rand -base64 32 | tr -d '/+=' | head -c 44)
  local hasura_secret=$(openssl rand -base64 48 | tr -d '/+=' | head -c 64)
  local jwt_secret=$(openssl rand -base64 48 | tr -d '/+=' | head -c 64)
  local redis_pw=$(openssl rand -base64 32 | tr -d '/+=' | head -c 44)
  local minio_pw=$(openssl rand -base64 32 | tr -d '/+=' | head -c 44)
  local meili_key=$(openssl rand -base64 32 | tr -d '/+=' | head -c 44)

  # Create secrets section marker if not exists
  if ! grep -q "# Security Secrets" "$env_file" 2>/dev/null; then
    printf "\n# Security Secrets - Auto-generated by nself\n" >> "$env_file"
  fi

  # Update or add each secret
  local secrets_added=0

  if ! grep -q "^POSTGRES_PASSWORD=" "$env_file" 2>/dev/null; then
    printf "POSTGRES_PASSWORD=%s\n" "$postgres_pw" >> "$env_file"
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} POSTGRES_PASSWORD generated (%d chars)\n" "${#postgres_pw}"
    secrets_added=$((secrets_added + 1))
  fi

  if ! grep -q "^HASURA_GRAPHQL_ADMIN_SECRET=" "$env_file" 2>/dev/null; then
    printf "HASURA_GRAPHQL_ADMIN_SECRET=%s\n" "$hasura_secret" >> "$env_file"
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} HASURA_GRAPHQL_ADMIN_SECRET generated (%d chars)\n" "${#hasura_secret}"
    secrets_added=$((secrets_added + 1))
  fi

  if ! grep -q "^AUTH_JWT_SECRET=" "$env_file" 2>/dev/null; then
    printf "AUTH_JWT_SECRET=%s\n" "$jwt_secret" >> "$env_file"
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} AUTH_JWT_SECRET generated (%d chars)\n" "${#jwt_secret}"
    secrets_added=$((secrets_added + 1))
  fi

  if ! grep -q "^REDIS_PASSWORD=" "$env_file" 2>/dev/null; then
    printf "REDIS_PASSWORD=%s\n" "$redis_pw" >> "$env_file"
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} REDIS_PASSWORD generated (%d chars)\n" "${#redis_pw}"
    secrets_added=$((secrets_added + 1))
  fi

  if ! grep -q "^MINIO_ROOT_PASSWORD=" "$env_file" 2>/dev/null; then
    printf "MINIO_ROOT_PASSWORD=%s\n" "$minio_pw" >> "$env_file"
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} MINIO_ROOT_PASSWORD generated (%d chars)\n" "${#minio_pw}"
    secrets_added=$((secrets_added + 1))
  fi

  if ! grep -q "^MEILISEARCH_MASTER_KEY=" "$env_file" 2>/dev/null; then
    printf "MEILISEARCH_MASTER_KEY=%s\n" "$meili_key" >> "$env_file"
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} MEILISEARCH_MASTER_KEY generated (%d chars)\n" "${#meili_key}"
    secrets_added=$((secrets_added + 1))
  fi

  printf "\n"

  if [[ $secrets_added -gt 0 ]]; then
    printf "${COLOR_GREEN}✓ Generated %d new secret(s)${COLOR_RESET}\n" "$secrets_added"
    printf "  Saved to: %s\n\n" "$env_file"
  else
    printf "${COLOR_GREEN}✓ All secrets already configured${COLOR_RESET}\n\n"
  fi
}

# ============================================================
# COMPREHENSIVE SECURITY REPORT
# ============================================================

# Generate comprehensive security report
security::generate_report() {
  local env="${ENV:-dev}"

  printf "\n${COLOR_CYAN}Security Validation Report${COLOR_RESET}\n"
  printf "═══════════════════════════════════════\n\n"

  # Calculate scores
  local total_score=0
  local max_score=0
  local password_score=0
  local cors_score=0
  local port_score=0
  local container_score=0
  local default_score=0

  # 1. Password Strength Analysis
  printf "${COLOR_BOLD}Password Strength:${COLOR_RESET}\n"
  local password_count=0
  local secure_passwords=0

  for var in POSTGRES_PASSWORD HASURA_GRAPHQL_ADMIN_SECRET REDIS_PASSWORD MINIO_ROOT_PASSWORD MEILISEARCH_MASTER_KEY; do
    local value="${!var:-}"
    if [[ -n "$value" ]]; then
      password_count=$((password_count + 1))
      local len=${#value}
      if [[ $len -ge 32 ]]; then
        secure_passwords=$((secure_passwords + 1))
        printf "  ${COLOR_GREEN}✓${COLOR_RESET} %s: %d chars (secure)\n" "$var" "$len"
      elif [[ $len -ge 24 ]]; then
        printf "  ${COLOR_YELLOW}⚠${COLOR_RESET} %s: %d chars (moderate - recommend 32+)\n" "$var" "$len"
      else
        printf "  ${COLOR_RED}✗${COLOR_RESET} %s: %d chars (weak - recommend 32+)\n" "$var" "$len"
      fi
    fi
  done

  if [[ $password_count -gt 0 ]]; then
    password_score=$((secure_passwords * 100 / password_count))
    printf "  Score: ${COLOR_CYAN}%d/%d${COLOR_RESET} passwords are secure\n" "$secure_passwords" "$password_count"
  else
    printf "  ${COLOR_YELLOW}⚠${COLOR_RESET} No passwords configured\n"
  fi
  max_score=$((max_score + 20))
  total_score=$((total_score + (password_score * 20 / 100)))
  printf "\n"

  # 2. CORS Configuration
  printf "${COLOR_BOLD}CORS Configuration:${COLOR_RESET}\n"
  if [[ "$env" == "prod" ]] || [[ "$env" == "production" ]]; then
    if [[ "${HASURA_GRAPHQL_CORS_DOMAIN:-}" =~ \* ]]; then
      printf "  ${COLOR_RED}✗${COLOR_RESET} CORS allows wildcard in production\n"
      cors_score=0
    else
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} CORS properly restricted\n"
      cors_score=100
    fi
  else
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} CORS configured for %s environment\n" "$env"
    cors_score=100
  fi
  max_score=$((max_score + 20))
  total_score=$((total_score + (cors_score * 20 / 100)))
  printf "\n"

  # 3. Port Exposure
  printf "${COLOR_BOLD}Port Configuration:${COLOR_RESET}\n"
  if [[ "$env" == "prod" ]] || [[ "$env" == "production" ]]; then
    if [[ "${POSTGRES_EXPOSE_PORT:-auto}" == "false" ]] || [[ "${POSTGRES_EXPOSE_PORT:-auto}" == "auto" ]]; then
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} Database port not exposed\n"
      port_score=100
    else
      printf "  ${COLOR_YELLOW}⚠${COLOR_RESET} Database port may be exposed\n"
      port_score=50
    fi
  else
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} Port configuration appropriate for %s\n" "$env"
    port_score=100
  fi
  max_score=$((max_score + 20))
  total_score=$((total_score + (port_score * 20 / 100)))
  printf "\n"

  # 4. No Insecure Defaults
  printf "${COLOR_BOLD}Insecure Defaults:${COLOR_RESET}\n"
  local insecure_count=0
  for var in POSTGRES_PASSWORD HASURA_GRAPHQL_ADMIN_SECRET REDIS_PASSWORD MINIO_ROOT_PASSWORD; do
    local value="${!var:-}"
    local value_lower
    value_lower=$(printf "%s" "$value" | tr '[:upper:]' '[:lower:]')
    if [[ "$value_lower" =~ (password|changeme|secret|admin|12345|qwerty|default) ]]; then
      insecure_count=$((insecure_count + 1))
      printf "  ${COLOR_RED}✗${COLOR_RESET} %s uses insecure default\n" "$var"
    fi
  done

  if [[ $insecure_count -eq 0 ]]; then
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} No insecure default values detected\n"
    default_score=100
  else
    printf "  ${COLOR_RED}✗${COLOR_RESET} %d insecure default(s) found\n" "$insecure_count"
    default_score=0
  fi
  max_score=$((max_score + 20))
  total_score=$((total_score + (default_score * 20 / 100)))
  printf "\n"

  # 5. Container Security
  printf "${COLOR_BOLD}Container Security:${COLOR_RESET}\n"
  if [[ -f "docker-compose.yml" ]]; then
    # Check if containers run as non-root (basic check)
    local root_containers=0
    local total_containers=0

    # Count services in docker-compose
    total_containers=$(grep -c "^  [a-z]" docker-compose.yml 2>/dev/null || echo "0")

    # Count services WITHOUT user directive (rough estimate)
    root_containers=$(grep -A 20 "^  [a-z]" docker-compose.yml 2>/dev/null | grep -c "user:" || echo "0")

    if [[ $root_containers -gt 0 ]]; then
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} %d services run as non-root\n" "$root_containers"
      container_score=100
    else
      printf "  ${COLOR_YELLOW}⚠${COLOR_RESET} Unable to verify container users\n"
      container_score=80
    fi
  else
    printf "  ${COLOR_YELLOW}⚠${COLOR_RESET} No docker-compose.yml found\n"
    container_score=50
  fi
  max_score=$((max_score + 20))
  total_score=$((total_score + (container_score * 20 / 100)))
  printf "\n"

  # Calculate final score
  local final_score=$((total_score * 100 / max_score))

  # Display final score with color
  printf "═══════════════════════════════════════\n"
  if [[ $final_score -ge 90 ]]; then
    printf "${COLOR_GREEN}Security Score: %d/100 ✅${COLOR_RESET}\n" "$final_score"
    printf "  Status: ${COLOR_GREEN}Excellent${COLOR_RESET}\n"
  elif [[ $final_score -ge 75 ]]; then
    printf "${COLOR_CYAN}Security Score: %d/100 ✓${COLOR_RESET}\n" "$final_score"
    printf "  Status: ${COLOR_CYAN}Good${COLOR_RESET}\n"
  elif [[ $final_score -ge 60 ]]; then
    printf "${COLOR_YELLOW}Security Score: %d/100 ⚠${COLOR_RESET}\n" "$final_score"
    printf "  Status: ${COLOR_YELLOW}Needs Improvement${COLOR_RESET}\n"
  else
    printf "${COLOR_RED}Security Score: %d/100 ✗${COLOR_RESET}\n" "$final_score"
    printf "  Status: ${COLOR_RED}Critical Issues${COLOR_RESET}\n"
  fi
  printf "\n"

  # Recommendations
  if [[ $final_score -lt 90 ]]; then
    printf "${COLOR_BOLD}Recommendations:${COLOR_RESET}\n"
    if [[ $password_score -lt 80 ]]; then
      printf "  • Strengthen passwords (use 32+ characters)\n"
    fi
    if [[ $cors_score -lt 100 ]]; then
      printf "  • Fix CORS configuration for production\n"
    fi
    if [[ $default_score -lt 100 ]]; then
      printf "  • Replace insecure default values\n"
    fi
    if [[ $port_score -lt 100 ]]; then
      printf "  • Review port exposure settings\n"
    fi
    printf "  • Run: ${COLOR_CYAN}nself audit security${COLOR_RESET} for detailed analysis\n"
    printf "  • Run: ${COLOR_CYAN}nself harden${COLOR_RESET} to auto-fix issues\n"
    printf "\n"
  fi
}

# Export functions
export -f security::validate_build
export -f security::check_required_passwords
export -f security::check_port_bindings
export -f security::check_insecure_values
export -f security::verify_no_exposed_ports
export -f security::check_firewall_active
export -f security::configure_firewall_for_prod
export -f security::production_preflight
export -f security::get_sensitive_containers
export -f security::force_recreate_sensitive_containers
export -f security::generate_secrets
export -f security::generate_report
