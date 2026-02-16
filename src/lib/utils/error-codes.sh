#!/usr/bin/env bash

# error-codes.sh - Comprehensive error code catalog for nself
# Provides standardized error codes for searchability and documentation
# Cross-platform compatible (Bash 3.2+)

# Prevent double-sourcing
[[ "${ERROR_CODES_SOURCED:-}" == "1" ]] && return 0

set -euo pipefail

export ERROR_CODES_SOURCED=1

# Source dependencies (namespaced to avoid clobbering caller's SCRIPT_DIR)
_ERROR_CODES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${_ERROR_CODES_DIR}/logging.sh" 2>/dev/null || true
source "${_ERROR_CODES_DIR}/error-messages.sh" 2>/dev/null || true
source "${_ERROR_CODES_DIR}/cli-output.sh" 2>/dev/null || true

# =============================================================================
# ERROR CODE DEFINITIONS
# Format: ERR_CATEGORY_SPECIFIC_NAME
# Categories: INIT, CONFIG, DOCKER, NETWORK, AUTH, DEPLOY, DB, BUILD, RUNTIME
# =============================================================================

# General/System Errors (1xx)
export ERR_UNKNOWN=100
export ERR_INVALID_ARGS=101
export ERR_NOT_IMPLEMENTED=102
export ERR_PERMISSION_DENIED=103
export ERR_FILE_NOT_FOUND=104
export ERR_DIR_NOT_FOUND=105
export ERR_COMMAND_NOT_FOUND=106
export ERR_TIMEOUT=107
export ERR_INTERRUPTED=108
export ERR_INSUFFICIENT_RESOURCES=109

# Configuration Errors (2xx)
export ERR_CONFIG_MISSING=200
export ERR_CONFIG_INVALID=201
export ERR_CONFIG_PARSE_FAILED=202
export ERR_ENV_VAR_MISSING=203
export ERR_ENV_VAR_INVALID=204
export ERR_CONFIG_VALIDATION_FAILED=205
export ERR_CONFIG_CONFLICT=206
export ERR_CONFIG_READONLY=207
export ERR_CONFIG_CORRUPT=208
export ERR_CONFIG_VERSION_MISMATCH=209

# Docker/Container Errors (3xx)
export ERR_DOCKER_NOT_RUNNING=300
export ERR_DOCKER_NOT_INSTALLED=301
export ERR_CONTAINER_FAILED=302
export ERR_CONTAINER_NOT_FOUND=303
export ERR_CONTAINER_UNHEALTHY=304
export ERR_IMAGE_NOT_FOUND=305
export ERR_IMAGE_PULL_FAILED=306
export ERR_PORT_CONFLICT=307
export ERR_VOLUME_ERROR=308
export ERR_NETWORK_ERROR=309
export ERR_COMPOSE_ERROR=310
export ERR_BUILD_FAILED=311
export ERR_DOCKERFILE_ERROR=312

# Network/Connectivity Errors (4xx)
export ERR_CONNECTION_FAILED=400
export ERR_CONNECTION_TIMEOUT=401
export ERR_DNS_FAILED=402
export ERR_SSL_ERROR=403
export ERR_CERT_INVALID=404
export ERR_FIREWALL_BLOCKED=405
export ERR_PROXY_ERROR=406
export ERR_NETWORK_UNREACHABLE=407
export ERR_HOST_UNREACHABLE=408
export ERR_PORT_CLOSED=409

# Authentication/Authorization Errors (5xx)
export ERR_AUTH_FAILED=500
export ERR_AUTH_REQUIRED=501
export ERR_AUTH_EXPIRED=502
export ERR_AUTH_INVALID_TOKEN=503
export ERR_AUTH_INVALID_CREDENTIALS=504
export ERR_PERMISSION_INSUFFICIENT=505
export ERR_ROLE_INVALID=506
export ERR_MFA_REQUIRED=507
export ERR_MFA_FAILED=508
export ERR_SESSION_EXPIRED=509

# Database Errors (6xx)
export ERR_DB_CONNECTION_FAILED=600
export ERR_DB_QUERY_FAILED=601
export ERR_DB_MIGRATION_FAILED=602
export ERR_DB_BACKUP_FAILED=603
export ERR_DB_RESTORE_FAILED=604
export ERR_DB_CONSTRAINT_VIOLATION=605
export ERR_DB_DEADLOCK=606
export ERR_DB_TIMEOUT=607
export ERR_DB_DISK_FULL=608
export ERR_DB_CORRUPT=609

# Deployment Errors (7xx)
export ERR_DEPLOY_FAILED=700
export ERR_DEPLOY_VALIDATION_FAILED=701
export ERR_DEPLOY_ROLLBACK_FAILED=702
export ERR_DEPLOY_HEALTH_CHECK_FAILED=703
export ERR_DEPLOY_RESOURCE_LIMIT=704
export ERR_DEPLOY_VERSION_CONFLICT=705
export ERR_DEPLOY_LOCK_FAILED=706
export ERR_DEPLOY_ABORTED=707
export ERR_DEPLOY_PARTIAL=708
export ERR_DEPLOY_CLEANUP_FAILED=709

# Service-Specific Errors (8xx)
export ERR_SERVICE_UNAVAILABLE=800
export ERR_SERVICE_DEGRADED=801
export ERR_SERVICE_DEPENDENCY_FAILED=802
export ERR_SERVICE_CONFIG_INVALID=803
export ERR_SERVICE_INIT_FAILED=804
export ERR_SERVICE_SHUTDOWN_FAILED=805
export ERR_SERVICE_RESTART_FAILED=806
export ERR_SERVICE_NOT_ENABLED=807
export ERR_SERVICE_VERSION_INCOMPATIBLE=808
export ERR_SERVICE_QUOTA_EXCEEDED=809

# Security Errors (9xx)
export ERR_SECURITY_VIOLATION=900
export ERR_SECURITY_POLICY_FAILED=901
export ERR_SECRET_NOT_FOUND=902
export ERR_SECRET_DECRYPTION_FAILED=903
export ERR_SECRET_ROTATION_FAILED=904
export ERR_ENCRYPTION_FAILED=905
export ERR_SIGNATURE_INVALID=906
export ERR_AUDIT_LOG_FAILED=907
export ERR_SECURITY_SCAN_FAILED=908
export ERR_VULNERABILITY_DETECTED=909

# =============================================================================
# ERROR CODE METADATA
# Provides descriptions and documentation links for each error
# =============================================================================

# Get error description
error_get_description() {
  local code="$1"

  case "$code" in
    # General/System (1xx)
    "$ERR_UNKNOWN") printf "Unknown error occurred" ;;
    "$ERR_INVALID_ARGS") printf "Invalid command arguments" ;;
    "$ERR_NOT_IMPLEMENTED") printf "Feature not yet implemented" ;;
    "$ERR_PERMISSION_DENIED") printf "Permission denied" ;;
    "$ERR_FILE_NOT_FOUND") printf "Required file not found" ;;
    "$ERR_DIR_NOT_FOUND") printf "Required directory not found" ;;
    "$ERR_COMMAND_NOT_FOUND") printf "Required command not found" ;;
    "$ERR_TIMEOUT") printf "Operation timed out" ;;
    "$ERR_INTERRUPTED") printf "Operation was interrupted" ;;
    "$ERR_INSUFFICIENT_RESOURCES") printf "Insufficient system resources" ;;

    # Configuration (2xx)
    "$ERR_CONFIG_MISSING") printf "Configuration file missing" ;;
    "$ERR_CONFIG_INVALID") printf "Invalid configuration" ;;
    "$ERR_CONFIG_PARSE_FAILED") printf "Failed to parse configuration" ;;
    "$ERR_ENV_VAR_MISSING") printf "Required environment variable missing" ;;
    "$ERR_ENV_VAR_INVALID") printf "Invalid environment variable value" ;;
    "$ERR_CONFIG_VALIDATION_FAILED") printf "Configuration validation failed" ;;
    "$ERR_CONFIG_CONFLICT") printf "Configuration conflict detected" ;;
    "$ERR_CONFIG_READONLY") printf "Configuration is read-only" ;;
    "$ERR_CONFIG_CORRUPT") printf "Configuration file is corrupt" ;;
    "$ERR_CONFIG_VERSION_MISMATCH") printf "Configuration version mismatch" ;;

    # Docker/Container (3xx)
    "$ERR_DOCKER_NOT_RUNNING") printf "Docker is not running" ;;
    "$ERR_DOCKER_NOT_INSTALLED") printf "Docker is not installed" ;;
    "$ERR_CONTAINER_FAILED") printf "Container failed to start" ;;
    "$ERR_CONTAINER_NOT_FOUND") printf "Container not found" ;;
    "$ERR_CONTAINER_UNHEALTHY") printf "Container is unhealthy" ;;
    "$ERR_IMAGE_NOT_FOUND") printf "Docker image not found" ;;
    "$ERR_IMAGE_PULL_FAILED") printf "Failed to pull Docker image" ;;
    "$ERR_PORT_CONFLICT") printf "Port is already in use" ;;
    "$ERR_VOLUME_ERROR") printf "Docker volume error" ;;
    "$ERR_NETWORK_ERROR") printf "Docker network error" ;;
    "$ERR_COMPOSE_ERROR") printf "Docker Compose error" ;;
    "$ERR_BUILD_FAILED") printf "Docker build failed" ;;
    "$ERR_DOCKERFILE_ERROR") printf "Dockerfile syntax error" ;;

    # Network (4xx)
    "$ERR_CONNECTION_FAILED") printf "Network connection failed" ;;
    "$ERR_CONNECTION_TIMEOUT") printf "Connection timeout" ;;
    "$ERR_DNS_FAILED") printf "DNS resolution failed" ;;
    "$ERR_SSL_ERROR") printf "SSL/TLS error" ;;
    "$ERR_CERT_INVALID") printf "Invalid SSL certificate" ;;
    "$ERR_FIREWALL_BLOCKED") printf "Blocked by firewall" ;;
    "$ERR_PROXY_ERROR") printf "Proxy configuration error" ;;
    "$ERR_NETWORK_UNREACHABLE") printf "Network unreachable" ;;
    "$ERR_HOST_UNREACHABLE") printf "Host unreachable" ;;
    "$ERR_PORT_CLOSED") printf "Port is closed" ;;

    # Authentication (5xx)
    "$ERR_AUTH_FAILED") printf "Authentication failed" ;;
    "$ERR_AUTH_REQUIRED") printf "Authentication required" ;;
    "$ERR_AUTH_EXPIRED") printf "Authentication expired" ;;
    "$ERR_AUTH_INVALID_TOKEN") printf "Invalid authentication token" ;;
    "$ERR_AUTH_INVALID_CREDENTIALS") printf "Invalid credentials" ;;
    "$ERR_PERMISSION_INSUFFICIENT") printf "Insufficient permissions" ;;
    "$ERR_ROLE_INVALID") printf "Invalid user role" ;;
    "$ERR_MFA_REQUIRED") printf "Multi-factor authentication required" ;;
    "$ERR_MFA_FAILED") printf "Multi-factor authentication failed" ;;
    "$ERR_SESSION_EXPIRED") printf "Session expired" ;;

    # Database (6xx)
    "$ERR_DB_CONNECTION_FAILED") printf "Database connection failed" ;;
    "$ERR_DB_QUERY_FAILED") printf "Database query failed" ;;
    "$ERR_DB_MIGRATION_FAILED") printf "Database migration failed" ;;
    "$ERR_DB_BACKUP_FAILED") printf "Database backup failed" ;;
    "$ERR_DB_RESTORE_FAILED") printf "Database restore failed" ;;
    "$ERR_DB_CONSTRAINT_VIOLATION") printf "Database constraint violation" ;;
    "$ERR_DB_DEADLOCK") printf "Database deadlock detected" ;;
    "$ERR_DB_TIMEOUT") printf "Database operation timeout" ;;
    "$ERR_DB_DISK_FULL") printf "Database disk full" ;;
    "$ERR_DB_CORRUPT") printf "Database corruption detected" ;;

    # Deployment (7xx)
    "$ERR_DEPLOY_FAILED") printf "Deployment failed" ;;
    "$ERR_DEPLOY_VALIDATION_FAILED") printf "Deployment validation failed" ;;
    "$ERR_DEPLOY_ROLLBACK_FAILED") printf "Deployment rollback failed" ;;
    "$ERR_DEPLOY_HEALTH_CHECK_FAILED") printf "Deployment health check failed" ;;
    "$ERR_DEPLOY_RESOURCE_LIMIT") printf "Deployment resource limit exceeded" ;;
    "$ERR_DEPLOY_VERSION_CONFLICT") printf "Deployment version conflict" ;;
    "$ERR_DEPLOY_LOCK_FAILED") printf "Failed to acquire deployment lock" ;;
    "$ERR_DEPLOY_ABORTED") printf "Deployment aborted" ;;
    "$ERR_DEPLOY_PARTIAL") printf "Partial deployment completed" ;;
    "$ERR_DEPLOY_CLEANUP_FAILED") printf "Deployment cleanup failed" ;;

    # Service (8xx)
    "$ERR_SERVICE_UNAVAILABLE") printf "Service unavailable" ;;
    "$ERR_SERVICE_DEGRADED") printf "Service is degraded" ;;
    "$ERR_SERVICE_DEPENDENCY_FAILED") printf "Service dependency failed" ;;
    "$ERR_SERVICE_CONFIG_INVALID") printf "Service configuration invalid" ;;
    "$ERR_SERVICE_INIT_FAILED") printf "Service initialization failed" ;;
    "$ERR_SERVICE_SHUTDOWN_FAILED") printf "Service shutdown failed" ;;
    "$ERR_SERVICE_RESTART_FAILED") printf "Service restart failed" ;;
    "$ERR_SERVICE_NOT_ENABLED") printf "Service not enabled" ;;
    "$ERR_SERVICE_VERSION_INCOMPATIBLE") printf "Service version incompatible" ;;
    "$ERR_SERVICE_QUOTA_EXCEEDED") printf "Service quota exceeded" ;;

    # Security (9xx)
    "$ERR_SECURITY_VIOLATION") printf "Security policy violation" ;;
    "$ERR_SECURITY_POLICY_FAILED") printf "Security policy check failed" ;;
    "$ERR_SECRET_NOT_FOUND") printf "Secret not found" ;;
    "$ERR_SECRET_DECRYPTION_FAILED") printf "Secret decryption failed" ;;
    "$ERR_SECRET_ROTATION_FAILED") printf "Secret rotation failed" ;;
    "$ERR_ENCRYPTION_FAILED") printf "Encryption failed" ;;
    "$ERR_SIGNATURE_INVALID") printf "Invalid signature" ;;
    "$ERR_AUDIT_LOG_FAILED") printf "Audit logging failed" ;;
    "$ERR_SECURITY_SCAN_FAILED") printf "Security scan failed" ;;
    "$ERR_VULNERABILITY_DETECTED") printf "Security vulnerability detected" ;;

    *) printf "Unknown error code: %s" "$code" ;;
  esac
}

# Get documentation link for error code
error_get_doc_link() {
  local code="$1"
  local base_url="https://docs.nself.org/errors"

  # Determine category from code range
  local category=""
  if [[ $code -ge 100 && $code -lt 200 ]]; then
    category="system"
  elif [[ $code -ge 200 && $code -lt 300 ]]; then
    category="configuration"
  elif [[ $code -ge 300 && $code -lt 400 ]]; then
    category="docker"
  elif [[ $code -ge 400 && $code -lt 500 ]]; then
    category="network"
  elif [[ $code -ge 500 && $code -lt 600 ]]; then
    category="auth"
  elif [[ $code -ge 600 && $code -lt 700 ]]; then
    category="database"
  elif [[ $code -ge 700 && $code -lt 800 ]]; then
    category="deployment"
  elif [[ $code -ge 800 && $code -lt 900 ]]; then
    category="service"
  elif [[ $code -ge 900 && $code -lt 1000 ]]; then
    category="security"
  else
    category="unknown"
  fi

  printf "%s/%s#error-%s" "$base_url" "$category" "$code"
}

# =============================================================================
# ERROR HANDLING FUNCTIONS
# =============================================================================

# Throw error with code
# Args: error_code, message, [context...]
error_throw() {
  local code="$1"
  local message="$2"
  shift 2
  local context=("$@")

  # Get error description
  local description
  description=$(error_get_description "$code")

  # Get documentation link
  local doc_link
  doc_link=$(error_get_doc_link "$code")

  # Log error with code
  log_with_code "$LOG_LEVEL_ERROR" "$code" "$message" "${context[@]}"

  # Display formatted error
  cli_error "[ERR-${code}] ${message}"
  printf "\n"
  cli_info "Description: ${description}"
  cli_info "Documentation: ${doc_link}"
  printf "\n"

  return "$code"
}

# Exit with error code and message
# Args: error_code, message, [context...]
error_exit() {
  local code="$1"
  local message="$2"
  shift 2
  local context=("$@")

  error_throw "$code" "$message" "${context[@]}"

  # Map error codes to shell exit codes (0-255)
  local exit_code=$((code % 256))
  [[ $exit_code -eq 0 ]] && exit_code=1

  exit "$exit_code"
}

# Warn with error code (non-fatal)
# Args: error_code, message, [context...]
error_warn() {
  local code="$1"
  local message="$2"
  shift 2
  local context=("$@")

  local description
  description=$(error_get_description "$code")

  log_with_code "$LOG_LEVEL_WARN" "$code" "$message" "${context[@]}"

  cli_warning "[WARN-${code}] ${message}"
  printf "\n"
  cli_info "Description: ${description}"
  printf "\n"
}

# =============================================================================
# ERROR SEARCH AND REFERENCE
# =============================================================================

# Search error codes by keyword
error_search() {
  local keyword="$1"
  local keyword_lower
  keyword_lower=$(printf "%s" "$keyword" | tr '[:upper:]' '[:lower:]')

  printf "Searching for errors matching: %s\n\n" "$keyword"

  # Get all error code variables
  local codes
  codes=$(env | grep '^ERR_' | cut -d= -f2 | sort -n)

  local found=0
  for code in $codes; do
    local description
    description=$(error_get_description "$code")
    local description_lower
    description_lower=$(printf "%s" "$description" | tr '[:upper:]' '[:lower:]')

    # Check if keyword matches
    if printf "%s" "$description_lower" | grep -q "$keyword_lower"; then
      printf "%s - %s\n" "$code" "$description"
      found=$((found + 1))
    fi
  done

  if [[ $found -eq 0 ]]; then
    printf "No errors found matching: %s\n" "$keyword"
  else
    printf "\nFound %s matching error(s)\n" "$found"
  fi
}

# List all error codes
error_list() {
  local category="${1:-all}"

  printf "nself Error Code Reference\n"
  printf "==========================\n\n"

  # Determine range based on category
  local min=0
  local max=1000
  case "$category" in
    system) min=100; max=199 ;;
    config|configuration) min=200; max=299 ;;
    docker) min=300; max=399 ;;
    network) min=400; max=499 ;;
    auth) min=500; max=599 ;;
    database|db) min=600; max=699 ;;
    deploy|deployment) min=700; max=799 ;;
    service) min=800; max=899 ;;
    security) min=900; max=999 ;;
  esac

  # Get all error codes in range
  local codes
  codes=$(env | grep '^ERR_' | cut -d= -f2 | sort -n | awk -v min="$min" -v max="$max" '$1 >= min && $1 <= max')

  if [[ -z "$codes" ]]; then
    printf "No errors in category: %s\n" "$category"
    return
  fi

  for code in $codes; do
    local description
    description=$(error_get_description "$code")
    printf "%s - %s\n" "$code" "$description"
  done
}

# Show error code reference
error_reference() {
  cat <<'EOF'
nself Error Code Reference
==========================

Error codes are organized by category:

1xx - System/General Errors
    100-109: Core system errors

2xx - Configuration Errors
    200-209: Configuration and environment issues

3xx - Docker/Container Errors
    300-312: Docker and container-related issues

4xx - Network/Connectivity Errors
    400-409: Network and connection problems

5xx - Authentication/Authorization Errors
    500-509: Auth and permission issues

6xx - Database Errors
    600-609: Database operations and failures

7xx - Deployment Errors
    700-709: Deployment and release issues

8xx - Service-Specific Errors
    800-809: Service lifecycle and health

9xx - Security Errors
    900-909: Security and compliance issues

Usage:
  nself error list [category]     - List all errors in category
  nself error search <keyword>    - Search for errors by keyword
  nself error info <code>         - Get detailed error information

Documentation: https://docs.nself.org/errors

EOF
}

# =============================================================================
# EXPORT FUNCTIONS
# =============================================================================

export -f error_get_description
export -f error_get_doc_link
export -f error_throw
export -f error_exit
export -f error_warn
export -f error_search
export -f error_list
export -f error_reference
