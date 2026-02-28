# Logging and Error Handling Integration Guide

This guide shows how to integrate the nself logging and error handling system into your scripts and custom services.

## Quick Start

### 1. Basic Integration

```bash
#!/usr/bin/env bash
set -euo pipefail

# Source the libraries you need
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../lib/utils/logging.sh"
source "${SCRIPT_DIR}/../lib/utils/audit-logging.sh"
source "${SCRIPT_DIR}/../lib/utils/error-codes.sh"

# Your script logic
main() {
  log_info "Script started"

  # Your code here
  if ! perform_operation; then
    error_throw "$ERR_UNKNOWN" "Operation failed"
    return 1
  fi

  log_info "Script completed successfully"
}

perform_operation() {
  # Implementation
  return 0
}

main "$@"
```

### 2. Environment Setup

Add to your `.env.local`:

```bash
# Logging configuration
NSELF_LOG_LEVEL=3              # INFO (0=FATAL, 1=ERROR, 2=WARN, 3=INFO, 4=DEBUG, 5=TRACE)
NSELF_LOG_DIR="${HOME}/.nself/logs"
NSELF_LOG_FILE="nself.log"
NSELF_LOG_TO_FILE=true
NSELF_LOG_TO_CONSOLE=true

# Log rotation
NSELF_LOG_MAX_SIZE=10485760    # 10MB
NSELF_LOG_MAX_FILES=5

# Audit logging
NSELF_AUDIT_ENABLED=true
NSELF_AUDIT_DIR="${HOME}/.nself/audit"
NSELF_AUDIT_FILE="audit.log"

# Security
NSELF_LOG_SANITIZE=true
```

---

## Common Integration Patterns

### Pattern 1: CLI Command with Full Logging

```bash
#!/usr/bin/env bash
set -euo pipefail

# cli-command.sh - Example CLI command with proper logging

CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${CLI_SCRIPT_DIR}/../lib/utils/logging.sh"
source "${CLI_SCRIPT_DIR}/../lib/utils/audit-logging.sh"
source "${CLI_SCRIPT_DIR}/../lib/utils/error-codes.sh"
source "${CLI_SCRIPT_DIR}/../lib/utils/display.sh"

show_help() {
  cat <<'HELP'
Usage: nself example [options]

Options:
  --verbose       Enable debug logging
  -h, --help      Show this help

Examples:
  nself example --verbose
HELP
}

cmd_example() {
  local verbose=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --verbose)
        verbose=true
        export NSELF_LOG_LEVEL=$LOG_LEVEL_DEBUG
        shift
        ;;
      -h|--help)
        show_help
        return 0
        ;;
      *)
        log_error "Unknown option: $1"
        return 1
        ;;
    esac
  done

  # Show command header
  show_command_header "nself example" "Example command with logging"

  # Log command execution
  log_info "Command started" "verbose=${verbose}"
  audit_admin "command_executed" "command=example" "verbose=${verbose}"

  # Do work
  if [[ "$verbose" == "true" ]]; then
    log_debug "Debug mode enabled"
  fi

  # Simulate work
  log_info "Processing..."
  sleep 1

  # Success
  log_success "Command completed successfully"
  audit_admin "command_completed" "command=example" "status=success"
}

export -f cmd_example
[[ "${BASH_SOURCE[0]}" == "${0}" ]] && cmd_example "$@"
```

### Pattern 2: Service Initialization

```bash
#!/usr/bin/env bash
set -euo pipefail

# service-init.sh - Initialize a service with proper error handling

source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/audit-logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/error-codes.sh"

init_service() {
  local service_name="$1"
  local config_file="${2:-.env}"

  log_info "Initializing service" "service=${service_name}"
  audit_system "service_init_started" "service=${service_name}"

  # Check prerequisites
  if ! check_prerequisites; then
    error_throw "$ERR_COMMAND_NOT_FOUND" \
      "Required commands not found" \
      "service=${service_name}"
    return 1
  fi

  # Load configuration
  if [[ ! -f "$config_file" ]]; then
    log_warn "Configuration file not found, using defaults" \
      "file=${config_file}"
    audit_config "using_defaults" "reason=config_not_found"
  else
    log_debug "Loading configuration" "file=${config_file}"
    source "$config_file"
  fi

  # Create required directories
  local dirs=("logs" "data" "tmp")
  for dir in "${dirs[@]}"; do
    if [[ ! -d "$dir" ]]; then
      log_debug "Creating directory" "dir=${dir}"
      mkdir -p "$dir" || {
        error_throw "$ERR_PERMISSION_DENIED" \
          "Failed to create directory" \
          "dir=${dir}"
        return 1
      }
    fi
  done

  # Initialize service-specific components
  if ! init_service_components "$service_name"; then
    error_throw "$ERR_SERVICE_INIT_FAILED" \
      "Service initialization failed" \
      "service=${service_name}"
    return 1
  fi

  log_info "Service initialized successfully" "service=${service_name}"
  audit_system "service_init_completed" "service=${service_name}"
}

check_prerequisites() {
  local required_commands=("docker" "docker-compose")

  for cmd in "${required_commands[@]}"; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      log_error "Required command not found" "command=${cmd}"
      return 1
    fi
    log_debug "Found required command" "command=${cmd}"
  done

  return 0
}

init_service_components() {
  local service_name="$1"
  # Service-specific initialization
  log_debug "Initializing components" "service=${service_name}"
  return 0
}

init_service "example-service"
```

### Pattern 3: Database Operations

```bash
#!/usr/bin/env bash
set -euo pipefail

# database-ops.sh - Database operations with audit logging

source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/audit-logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/error-codes.sh"

backup_database() {
  local database="$1"
  local backup_dir="${2:-./backups}"
  local timestamp
  timestamp=$(date +%Y%m%d_%H%M%S)
  local backup_file="${backup_dir}/${database}_${timestamp}.sql"

  log_info "Starting database backup" "database=${database}"
  audit_data "backup_started" "database=${database}"

  # Create backup directory
  if [[ ! -d "$backup_dir" ]]; then
    mkdir -p "$backup_dir" || {
      error_throw "$ERR_PERMISSION_DENIED" \
        "Failed to create backup directory" \
        "dir=${backup_dir}"
      return 1
    }
  fi

  # Perform backup
  local start_time
  start_time=$(date +%s)

  if pg_dump -h localhost -U postgres "$database" > "$backup_file" 2>/dev/null; then
    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))
    local size
    size=$(du -h "$backup_file" | cut -f1)

    log_info "Backup completed" \
      "database=${database}" \
      "size=${size}" \
      "duration=${duration}s"

    audit_data "backup_completed" \
      "database=${database}" \
      "file=${backup_file}" \
      "size=${size}" \
      "duration=${duration}s"

    return 0
  else
    log_error "Backup failed" "database=${database}"
    audit_data "backup_failed" "database=${database}"

    error_throw "$ERR_DB_BACKUP_FAILED" \
      "Database backup failed" \
      "database=${database}"
    return 1
  fi
}

restore_database() {
  local database="$1"
  local backup_file="$2"

  # CRITICAL: Audit before destructive operation
  log_warn "Initiating database restore (destructive operation)" \
    "database=${database}"
  audit_data "restore_initiated" \
    "database=${database}" \
    "backup=${backup_file}"

  # Verify backup file exists
  if [[ ! -f "$backup_file" ]]; then
    error_throw "$ERR_FILE_NOT_FOUND" \
      "Backup file not found" \
      "file=${backup_file}"
    return 1
  fi

  # Confirm with user
  read -p "Are you sure you want to restore ${database}? (yes/no): " -r confirm
  if [[ "$confirm" != "yes" ]]; then
    log_info "Restore cancelled by user"
    audit_data "restore_cancelled" "database=${database}" "reason=user_declined"
    return 0
  fi

  # Perform restore
  if psql -h localhost -U postgres "$database" < "$backup_file" 2>/dev/null; then
    log_info "Restore completed" "database=${database}"
    audit_data "restore_completed" \
      "database=${database}" \
      "backup=${backup_file}"
    return 0
  else
    log_error "Restore failed" "database=${database}"
    audit_data "restore_failed" "database=${database}"

    error_throw "$ERR_DB_RESTORE_FAILED" \
      "Database restore failed" \
      "database=${database}"
    return 1
  fi
}

# Example usage
backup_database "myapp"
```

### Pattern 4: Secret Management

```bash
#!/usr/bin/env bash
set -euo pipefail

# secret-manager.sh - Secure secret management

source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/audit-logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/error-codes.sh"

create_secret() {
  local secret_name="$1"
  local secret_value="$2"

  # CRITICAL: Never log the secret value
  log_info "Creating secret" "name=${secret_name}"
  audit_secret "secret_create_initiated" "name=${secret_name}"

  # Validate secret name
  if [[ ! "$secret_name" =~ ^[a-zA-Z0-9_-]+$ ]]; then
    error_throw "$ERR_INVALID_ARGS" \
      "Invalid secret name format" \
      "name=${secret_name}"
    return 1
  fi

  # Validate secret strength
  if [[ ${#secret_value} -lt 16 ]]; then
    log_warn "Secret is weak" "name=${secret_name}" "min_length=16"
    audit_security "weak_secret_detected" \
      "name=${secret_name}" \
      "length=${#secret_value}"

    error_warn "$ERR_SECURITY_POLICY_FAILED" \
      "Secret does not meet minimum strength requirements"
  fi

  # Store secret (example: write to encrypted file)
  local secret_dir="${HOME}/.nself/secrets"
  local secret_file="${secret_dir}/${secret_name}.enc"

  mkdir -p "$secret_dir"
  chmod 700 "$secret_dir"

  # Encrypt and store (simplified example)
  if echo "$secret_value" | openssl enc -aes-256-cbc -salt -out "$secret_file" 2>/dev/null; then
    chmod 600 "$secret_file"

    log_info "Secret created successfully" "name=${secret_name}"
    audit_secret "secret_created" "name=${secret_name}"
    return 0
  else
    log_error "Failed to create secret" "name=${secret_name}"
    audit_secret "secret_create_failed" "name=${secret_name}"

    error_throw "$ERR_ENCRYPTION_FAILED" \
      "Failed to encrypt secret" \
      "name=${secret_name}"
    return 1
  fi
}

access_secret() {
  local secret_name="$1"
  local purpose="${2:-unknown}"

  # IMPORTANT: Audit every access
  log_debug "Secret accessed" "name=${secret_name}"
  audit_secret "secret_accessed" \
    "name=${secret_name}" \
    "purpose=${purpose}"

  local secret_dir="${HOME}/.nself/secrets"
  local secret_file="${secret_dir}/${secret_name}.enc"

  if [[ ! -f "$secret_file" ]]; then
    error_throw "$ERR_SECRET_NOT_FOUND" \
      "Secret not found" \
      "name=${secret_name}"
    return 1
  fi

  # Decrypt and return (to stdout, not logs!)
  if ! openssl enc -aes-256-cbc -d -in "$secret_file" 2>/dev/null; then
    log_error "Failed to decrypt secret" "name=${secret_name}"
    audit_secret "secret_decryption_failed" "name=${secret_name}"

    error_throw "$ERR_SECRET_DECRYPTION_FAILED" \
      "Failed to decrypt secret" \
      "name=${secret_name}"
    return 1
  fi
}

rotate_secret() {
  local secret_name="$1"
  local new_value="$2"

  log_info "Rotating secret" "name=${secret_name}"
  audit_secret "rotation_started" "name=${secret_name}"

  # Backup old secret
  local backup_name="${secret_name}.backup.$(date +%s)"
  if create_secret "$backup_name" "$(access_secret "$secret_name" "rotation")"; then
    log_debug "Old secret backed up" "backup=${backup_name}"
  fi

  # Create new secret
  if create_secret "$secret_name" "$new_value"; then
    log_info "Secret rotated successfully" "name=${secret_name}"
    audit_secret "rotation_completed" "name=${secret_name}"
    return 0
  else
    log_error "Secret rotation failed" "name=${secret_name}"
    audit_secret "rotation_failed" "name=${secret_name}"

    error_throw "$ERR_SECRET_ROTATION_FAILED" \
      "Failed to rotate secret" \
      "name=${secret_name}"
    return 1
  fi
}

delete_secret() {
  local secret_name="$1"

  # CRITICAL: Audit deletions
  log_warn "Deleting secret" "name=${secret_name}"
  audit_secret "deletion_initiated" "name=${secret_name}"

  local secret_dir="${HOME}/.nself/secrets"
  local secret_file="${secret_dir}/${secret_name}.enc"

  if [[ -f "$secret_file" ]]; then
    # Secure deletion
    if command -v shred >/dev/null 2>&1; then
      shred -u "$secret_file" 2>/dev/null
    else
      rm -f "$secret_file"
    fi

    log_info "Secret deleted" "name=${secret_name}"
    audit_secret "deletion_completed" "name=${secret_name}"
  else
    log_warn "Secret not found" "name=${secret_name}"
    audit_secret "deletion_failed" "name=${secret_name}" "reason=not_found"
  fi
}

# Example usage (commented out)
# create_secret "api_key" "$(openssl rand -base64 32)"
```

### Pattern 5: Deployment Script

```bash
#!/usr/bin/env bash
set -euo pipefail

# deploy.sh - Production deployment with comprehensive logging

source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/audit-logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/error-codes.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/display.sh"

deploy() {
  local environment="$1"
  local version="$2"
  local dry_run="${3:-false}"

  show_command_header "nself deploy" "Deploy to ${environment}"

  log_info "Deployment initiated" \
    "env=${environment}" \
    "version=${version}" \
    "dry_run=${dry_run}"

  audit_deploy "deployment_initiated" \
    "environment=${environment}" \
    "version=${version}" \
    "dry_run=${dry_run}"

  # Pre-deployment checks
  log_info "Running pre-deployment checks"

  if ! pre_deployment_checks "$environment" "$version"; then
    error_exit "$ERR_DEPLOY_VALIDATION_FAILED" \
      "Pre-deployment validation failed"
  fi

  # Create deployment lock
  if ! acquire_deployment_lock "$environment"; then
    error_exit "$ERR_DEPLOY_LOCK_FAILED" \
      "Another deployment is in progress" \
      "environment=${environment}"
  fi

  # Perform deployment
  local deployment_id="deploy_${environment}_$(date +%s)"

  log_info "Starting deployment" "id=${deployment_id}"
  audit_deploy "deployment_started" \
    "id=${deployment_id}" \
    "environment=${environment}" \
    "version=${version}"

  if [[ "$dry_run" == "true" ]]; then
    log_info "Dry run mode - simulating deployment"
    audit_deploy "deployment_dry_run" "id=${deployment_id}"
    release_deployment_lock "$environment"
    return 0
  fi

  # Execute deployment steps
  local steps=("build" "test" "deploy" "verify")
  for step in "${steps[@]}"; do
    log_info "Executing step: ${step}"

    if ! execute_deployment_step "$step" "$environment" "$version"; then
      log_error "Deployment step failed" "step=${step}"
      audit_deploy "deployment_step_failed" \
        "id=${deployment_id}" \
        "step=${step}"

      # Initiate rollback
      initiate_rollback "$environment" "$deployment_id"

      release_deployment_lock "$environment"

      error_exit "$ERR_DEPLOY_FAILED" \
        "Deployment failed at step: ${step}"
    fi

    log_success "Step completed: ${step}"
  done

  # Post-deployment verification
  log_info "Running post-deployment verification"

  if ! verify_deployment "$environment" "$version"; then
    log_error "Deployment verification failed"
    audit_deploy "deployment_verification_failed" \
      "id=${deployment_id}"

    initiate_rollback "$environment" "$deployment_id"
    release_deployment_lock "$environment"

    error_exit "$ERR_DEPLOY_HEALTH_CHECK_FAILED" \
      "Deployment verification failed"
  fi

  # Success
  log_success "Deployment completed successfully"
  audit_deploy "deployment_completed" \
    "id=${deployment_id}" \
    "environment=${environment}" \
    "version=${version}" \
    "status=success"

  release_deployment_lock "$environment"
}

pre_deployment_checks() {
  local environment="$1"
  local version="$2"

  log_debug "Checking environment status" "env=${environment}"
  log_debug "Validating version" "version=${version}"

  # Add actual checks here
  return 0
}

acquire_deployment_lock() {
  local environment="$1"
  local lock_file="/tmp/nself_deploy_${environment}.lock"

  if [[ -f "$lock_file" ]]; then
    log_error "Deployment lock exists" "file=${lock_file}"
    return 1
  fi

  echo "$$" > "$lock_file"
  log_debug "Deployment lock acquired" "pid=$$"
  return 0
}

release_deployment_lock() {
  local environment="$1"
  local lock_file="/tmp/nself_deploy_${environment}.lock"

  rm -f "$lock_file"
  log_debug "Deployment lock released"
}

execute_deployment_step() {
  local step="$1"
  local environment="$2"
  local version="$3"

  log_debug "Executing deployment step" \
    "step=${step}" \
    "env=${environment}" \
    "version=${version}"

  # Actual implementation would go here
  sleep 1  # Simulate work

  return 0
}

verify_deployment() {
  local environment="$1"
  local version="$2"

  log_debug "Verifying deployment" \
    "env=${environment}" \
    "version=${version}"

  # Actual verification would go here
  return 0
}

initiate_rollback() {
  local environment="$1"
  local deployment_id="$2"

  log_warn "Initiating rollback" \
    "env=${environment}" \
    "deployment_id=${deployment_id}"

  audit_deploy "rollback_initiated" \
    "environment=${environment}" \
    "deployment_id=${deployment_id}" \
    "reason=deployment_failed"

  # Actual rollback would go here
}

# Example usage
deploy "staging" "v1.2.3" "false"
```

---

## Testing Your Integration

### 1. Test Logging Levels

```bash
#!/usr/bin/env bash

source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/logging.sh"

# Test all log levels
log_fatal "This is a fatal error"
log_error "This is an error"
log_warn "This is a warning"
log_info "This is info"
log_debug "This is debug"
log_trace "This is trace"

# Change log level
export NSELF_LOG_LEVEL=$LOG_LEVEL_DEBUG
log_debug "Now debug is visible"
```

### 2. Test Error Codes

```bash
#!/usr/bin/env bash

source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/error-codes.sh"

# Test error throwing
if ! some_operation; then
  error_throw "$ERR_UNKNOWN" "Operation failed"
fi

# Test error warning
error_warn "$ERR_SERVICE_DEGRADED" "Service is slow"

# Test error search
error_search "connection"
```

### 3. Test Audit Logging

```bash
#!/usr/bin/env bash

source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/audit-logging.sh"

# Generate test audit events
audit_auth "test_login" "user=testuser"
audit_config "test_change" "key=TEST_VAR"
audit_deploy "test_deploy" "env=test"

# Query audit logs
audit_query
audit_stats
audit_verify
```

---

## Troubleshooting

### Logs Not Appearing

```bash
# Check if logging is enabled
echo "NSELF_LOG_TO_FILE: ${NSELF_LOG_TO_FILE}"
echo "NSELF_LOG_TO_CONSOLE: ${NSELF_LOG_TO_CONSOLE}"

# Check log level
echo "NSELF_LOG_LEVEL: ${NSELF_LOG_LEVEL}"

# Check log file location
echo "Log file: ${NSELF_LOG_DIR}/${NSELF_LOG_FILE}"
ls -la "${NSELF_LOG_DIR}/${NSELF_LOG_FILE}"
```

### Audit Logs Not Working

```bash
# Check if audit logging is enabled
echo "NSELF_AUDIT_ENABLED: ${NSELF_AUDIT_ENABLED}"

# Check audit file location
echo "Audit file: ${NSELF_AUDIT_DIR}/${NSELF_AUDIT_FILE}"
ls -la "${NSELF_AUDIT_DIR}/${NSELF_AUDIT_FILE}"

# Verify permissions
stat "${NSELF_AUDIT_DIR}/${NSELF_AUDIT_FILE}"
```

### Permission Issues

```bash
# Fix log directory permissions
chmod 700 "${NSELF_LOG_DIR}"
chmod 600 "${NSELF_LOG_DIR}"/*.log

# Fix audit directory permissions
chmod 700 "${NSELF_AUDIT_DIR}"
chmod 600 "${NSELF_AUDIT_DIR}"/*.log
```

---

## Migration Guide

### Migrating from Echo/Printf

**Before:**
```bash
echo "ERROR: Something failed" >&2
printf "WARNING: %s\n" "$message" >&2
```

**After:**
```bash
log_error "Something failed"
log_warn "$message"
```

### Migrating from Custom Logging

**Before:**
```bash
log() {
  echo "[$(date)] $1" >> /var/log/myapp.log
}

log "Message here"
```

**After:**
```bash
source "path/to/logging.sh"
log_info "Message here"
```

### Adding Error Codes

**Before:**
```bash
echo "Error: Port 5432 in use" >&2
exit 1
```

**After:**
```bash
source "path/to/error-codes.sh"
error_exit "$ERR_PORT_CONFLICT" \
  "Port 5432 is already in use" \
  "port=5432"
```

---

## Performance Considerations

### 1. Log Level Impact

- **TRACE/DEBUG**: High overhead, use only for development
- **INFO**: Moderate overhead, good for production
- **WARN/ERROR**: Low overhead, always safe

### 2. Conditional Debug Logging

```bash
# Good: Only evaluate if debug is enabled
if [[ $NSELF_LOG_LEVEL -ge $LOG_LEVEL_DEBUG ]]; then
  expensive_debug_data=$(expensive_operation)
  log_debug "Debug info" "data=${expensive_debug_data}"
fi

# Bad: Always evaluates expensive operation
log_debug "Debug info" "data=$(expensive_operation)"
```

### 3. Audit Log Overhead

- Minimal for normal operations
- Append-only, no read overhead
- Automatic rotation prevents unbounded growth

---

## Additional Resources

- [Error Handling Best Practices](ERROR-HANDLING.md)
- [Logging API Reference](../reference/api/README.md)
- [Audit Logging API Reference](../reference/api/README.md)
- [Error Codes Reference](../reference/api/README.md)

---

**Last Updated:** 2024-03-15
**Version:** 1.0.0
