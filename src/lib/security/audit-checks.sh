#!/usr/bin/env bash
# audit-checks.sh - Security audit functions
# Detects security issues in nself deployments

set -euo pipefail

# Get directory paths
SECURITY_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$SECURITY_LIB_DIR/../../.." && pwd)"

# Source dependencies
if [[ -f "$NSELF_ROOT/src/lib/utils/cli-output.sh" ]]; then
  source "$NSELF_ROOT/src/lib/utils/cli-output.sh"
fi

# Check for weak secrets
check_weak_secrets() {
  local env_file="${1:-.env}"
  local issues=0

  if [[ ! -f "$env_file" ]]; then
    return 0
  fi

  # Weak patterns to detect
  local weak_patterns=(
    "minioadmin" "changeme" "admin" "password123"
    "secret" "dev-password" "test" "demo" "postgres"
    "hasura" "admin-secret"
  )

  for pattern in "${weak_patterns[@]}"; do
    if grep -qi "$pattern" "$env_file" 2>/dev/null; then
      cli_error "Weak secret pattern detected: $pattern"
      issues=$((issues + 1))
    fi
  done

  # Check secret lengths
  while IFS= read -r line; do
    if [[ "$line" =~ ^([A-Z_]+)=(.+)$ ]]; then
      local key="${BASH_REMATCH[1]}"
      local value="${BASH_REMATCH[2]}"

      if [[ "$key" =~ (PASSWORD|SECRET|KEY) && ! "$key" =~ (PUBLIC|ENABLED) ]]; then
        if [[ ${#value} -lt 24 ]]; then
          cli_warning "Secret too short (<24 chars): $key"
          issues=$((issues + 1))
        fi
      fi
    fi
  done < "$env_file"

  return $issues
}

# Check CORS security
check_cors_security() {
  local env="${ENV:-dev}"
  local issues=0

  if [[ "$env" == "production" ]]; then
    if grep -q 'CORS.*\*' .env 2>/dev/null; then
      cli_error "Wildcard CORS (*) in production"
      issues=$((issues + 1))
    fi
  fi

  return $issues
}

# Check exposed ports
check_exposed_ports() {
  local env="${ENV:-dev}"
  local issues=0

  if [[ "$env" == "production" ]]; then
    if [[ "${POSTGRES_EXPOSE_PORT:-false}" == "true" ]]; then
      cli_warning "Database port exposed in production"
      issues=$((issues + 1))
    fi
  fi

  return $issues
}

# Check container users
check_container_users() {
  local compose_file="${1:-docker-compose.yml}"
  local issues=0

  if [[ ! -f "$compose_file" ]]; then
    return 0
  fi

  # Services that should run as non-root
  local services=("postgres" "hasura" "auth" "redis" "minio" "grafana" "prometheus")

  for service in "${services[@]}"; do
    if grep -A 20 "^  $service:" "$compose_file" | grep -q "user:"; then
      cli_success "$service: running as non-root"
    else
      cli_warning "$service: running as root"
      issues=$((issues + 1))
    fi
  done

  return $issues
}

# Security audit runner
run_security_audit() {
  local issues=0

  cli_header "Security Audit"
  printf "Environment: %s\n\n" "${ENV:-dev}"

  cli_info "Checking secrets..."
  check_weak_secrets || ((issues+=$?))

  cli_info "Checking CORS configuration..."
  check_cors_security || ((issues+=$?))

  cli_info "Checking port exposure..."
  check_exposed_ports || ((issues+=$?))

  cli_info "Checking container users..."
  check_container_users || ((issues+=$?))

  echo ""
  if [[ $issues -eq 0 ]]; then
    cli_success "All security checks passed!"
  else
    cli_error "Found $issues security issue(s)"
    cli_info "Fix automatically: nself harden"
  fi

  return $issues
}

# Export functions
export -f check_weak_secrets
export -f check_cors_security
export -f check_exposed_ports
export -f check_container_users
export -f run_security_audit
