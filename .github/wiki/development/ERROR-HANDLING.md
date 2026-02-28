# Error Handling Best Practices

This guide covers error handling, logging, and security best practices for nself development.

## Table of Contents

- [Overview](#overview)
- [Error Handling Principles](#error-handling-principles)
- [Using the Logging System](#using-the-logging-system)
- [Audit Logging](#audit-logging)
- [Error Codes](#error-codes)
- [Security Considerations](#security-considerations)
- [Examples](#examples)

---

## Overview

nself provides a comprehensive error handling and logging system designed to:

- Prevent information disclosure
- Aid debugging without exposing sensitive data
- Provide actionable error messages
- Support compliance and audit requirements
- Enable effective troubleshooting

### Key Components

1. **Structured Logging** (`src/lib/utils/logging.sh`)
   - Multiple log levels (FATAL, ERROR, WARN, INFO, DEBUG, TRACE)
   - Automatic log rotation
   - Sensitive data sanitization
   - File and console output

2. **Audit Logging** (`src/lib/utils/audit-logging.sh`)
   - Immutable audit trail
   - Integrity checksums
   - Compliance support
   - Security event tracking

3. **Error Codes** (`src/lib/utils/error-codes.sh`)
   - Standardized error codes
   - Searchable error catalog
   - Documentation links
   - Category organization

4. **Error Messages** (`src/lib/utils/error-messages.sh`)
   - User-friendly error descriptions
   - Suggested fixes
   - Platform-specific guidance

---

## Error Handling Principles

### 1. Never Expose Sensitive Data

**❌ BAD:**
```bash
echo "Database connection failed: postgresql://user:password123@localhost:5432/db"
log_error "Auth failed with token: ${JWT_TOKEN}"
```

**✅ GOOD:**
```bash
log_error "Database connection failed" "host=localhost" "port=5432"
log_error "Authentication failed" "user=${USER}"
# Token is automatically redacted by logging system
```

### 2. Always Use Structured Logging

**❌ BAD:**
```bash
echo "ERROR: Something went wrong" >&2
```

**✅ GOOD:**
```bash
log_error "Configuration validation failed" "file=${config_file}"
```

### 3. Provide Actionable Error Messages

**❌ BAD:**
```bash
log_error "Failed"
exit 1
```

**✅ GOOD:**
```bash
error_throw "$ERR_PORT_CONFLICT" \
  "Port 5432 is already in use" \
  "conflicting_service=postgresql" \
  "suggested_port=5433"
```

### 4. Use Appropriate Log Levels

```bash
# FATAL - Critical error, application cannot continue
log_fatal "Docker daemon not accessible"

# ERROR - Operation failed but application can continue
log_error "Service health check failed" "service=postgres"

# WARN - Potentially problematic situation
log_warn "Using default configuration" "reason=.env not found"

# INFO - General informational messages
log_info "Starting service" "service=nginx"

# DEBUG - Detailed debugging information
log_debug "Loading configuration" "file=${config_file}"

# TRACE - Very detailed trace information
log_trace "Executing function" "function=validate_config"
```

### 5. Always Log Security Events

```bash
# Login attempts
audit_auth "login_success" "user=${username}" "ip=${client_ip}"
audit_auth "login_failed" "user=${username}" "reason=invalid_password"

# Configuration changes
audit_config "env_updated" "key=POSTGRES_PASSWORD" "action=rotated"

# Deployment actions
audit_deploy "deployment_started" "environment=production" "version=${version}"

# Secret operations (NEVER log the actual secret!)
audit_secret "secret_accessed" "name=${secret_name}" "purpose=deployment"
audit_secret "secret_rotated" "name=${secret_name}"
```

---

## Using the Logging System

### Basic Logging

```bash
#!/usr/bin/env bash

# Source the logging library
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/logging.sh"

# Initialize logging (optional - auto-initialized on source)
log_init

# Log messages at different levels
log_info "Application started"
log_debug "Loading configuration files"
log_warn "Configuration file not found, using defaults"
log_error "Failed to connect to database"
log_fatal "Critical system failure"
```

### Log Levels Configuration

```bash
# Set log level via environment variable
export NSELF_LOG_LEVEL=4  # DEBUG level

# Or use constants
export NSELF_LOG_LEVEL=$LOG_LEVEL_DEBUG

# Log levels (higher = more verbose):
# 0 - FATAL
# 1 - ERROR
# 2 - WARN
# 3 - INFO (default)
# 4 - DEBUG
# 5 - TRACE
```

### Log Output Configuration

```bash
# Configure log directory
export NSELF_LOG_DIR="${HOME}/.nself/logs"
export NSELF_LOG_FILE="nself.log"

# Enable/disable outputs
export NSELF_LOG_TO_FILE=true      # Write to log file
export NSELF_LOG_TO_CONSOLE=true   # Write to console

# Log rotation
export NSELF_LOG_MAX_SIZE=10485760  # 10MB
export NSELF_LOG_MAX_FILES=5        # Keep 5 rotated logs
```

### Sensitive Data Sanitization

The logging system automatically sanitizes:

- Passwords
- Tokens and API keys
- JWT tokens
- Basic auth credentials
- User paths
- Secret values

```bash
# These are automatically sanitized:
log_info "Connection string: postgresql://user:password@localhost/db"
# Logged as: "Connection string: postgresql://***REDACTED***@localhost/db"

log_debug "Auth header: Bearer eyJhbGc..."
# Logged as: "Auth header: Bearer ***JWT_REDACTED***"

log_error "Failed at /Users/john/project/file.txt"
# Logged as: "Failed at /Users/***USER***/project/file.txt"
```

### Custom Sanitization Patterns

```bash
# Add custom patterns to redact
export NSELF_LOG_REDACT_PATTERNS="password|secret|token|key|auth|ssn|creditcard"

# Disable sanitization (NOT RECOMMENDED)
export NSELF_LOG_SANITIZE=false
```

### Log Management

```bash
# View recent logs
log_tail 50

# Follow logs in real-time
log_follow

# Search logs
log_search "error"
log_search "database"

# Get log statistics
log_stats

# Clear logs
log_clear

# Export logs
log_export "/tmp/nself-logs-backup.log"
```

---

## Audit Logging

### When to Use Audit Logging

Use audit logging for **security-sensitive operations**:

- Authentication and authorization
- Configuration changes
- Deployment actions
- Secret access and rotation
- Administrative operations
- Data operations (backup, restore)
- Security policy changes

### Audit Log Categories

```bash
# Authentication/Authorization
audit_auth "login_attempt" "user=${user}" "method=password"
audit_auth "permission_check" "user=${user}" "resource=${resource}" "result=denied"

# Configuration Changes
audit_config "env_var_updated" "key=DATABASE_URL"
audit_config "service_enabled" "service=monitoring"

# Deployment Actions
audit_deploy "deployment_started" "env=production" "version=${version}"
audit_deploy "rollback_initiated" "env=production" "reason=${reason}"

# Secret Operations
audit_secret "secret_created" "name=db_password"
audit_secret "secret_accessed" "name=api_key" "service=payment"

# Security Events
audit_security "firewall_rule_added" "port=443" "protocol=tcp"
audit_security "ssl_cert_renewed" "domain=${domain}"

# Data Operations
audit_data "database_backup" "size=${size_mb}MB" "duration=${duration}s"
audit_data "database_restored" "backup_date=${date}"

# Administrative Actions
audit_admin "user_created" "username=${username}" "role=${role}"
audit_admin "service_restarted" "service=${service}" "reason=${reason}"

# System Events
audit_system "startup" "version=${VERSION}"
audit_system "shutdown" "reason=${reason}"
```

### Audit Log Format

Audit logs are stored in structured format:

```
timestamp|event_id|category|action|user|hostname|pid|details|checksum
```

Example:
```
2024-03-15 14:30:00 UTC|evt_1710513000_12345|AUTH|login_success|admin|server1|12345|ip=192.168.1.1;method=mfa|abc123def456
```

### Audit Log Management

```bash
# Query all audit logs
audit_query

# Query by category
audit_query AUTH
audit_query DEPLOY

# Query by action
audit_query "" "login_failed"

# Query by user
audit_query "" "" "admin"

# Get statistics
audit_stats

# Export audit logs
audit_export "audit-2024-03.txt" "txt"
audit_export "audit-2024-03.csv" "csv"
audit_export "audit-2024-03.json" "json"

# Verify integrity
audit_verify
```

### Compliance Features

- **Immutable logs**: Audit entries cannot be modified
- **Integrity checksums**: Each entry has SHA-256 checksum
- **Append-only**: On Linux, `chattr +a` prevents deletion
- **Restricted permissions**: 600 (owner read/write only)
- **Tamper detection**: `audit_verify` detects modifications

---

## Error Codes

### Using Error Codes

```bash
#!/usr/bin/env bash

source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/error-codes.sh"

# Throw error with code
error_throw "$ERR_CONFIG_MISSING" \
  "Configuration file not found" \
  "file=.env" \
  "location=${PWD}"

# Exit with error code
error_exit "$ERR_DOCKER_NOT_RUNNING" \
  "Docker daemon is not accessible"

# Warn with error code (non-fatal)
error_warn "$ERR_SERVICE_DEGRADED" \
  "Service is running but degraded" \
  "service=postgres" \
  "reason=high_latency"
```

### Error Code Categories

| Range | Category | Description |
|-------|----------|-------------|
| 1xx | System/General | Core system errors |
| 2xx | Configuration | Config and environment issues |
| 3xx | Docker/Container | Docker-related problems |
| 4xx | Network | Connection and network errors |
| 5xx | Authentication | Auth and permission issues |
| 6xx | Database | Database operations |
| 7xx | Deployment | Deployment and releases |
| 8xx | Service | Service lifecycle |
| 9xx | Security | Security violations |

### Common Error Codes

```bash
# Configuration Errors
ERR_CONFIG_MISSING=200          # .env file not found
ERR_CONFIG_INVALID=201          # Invalid configuration
ERR_ENV_VAR_MISSING=203         # Required variable missing
ERR_ENV_VAR_INVALID=204         # Invalid variable value

# Docker Errors
ERR_DOCKER_NOT_RUNNING=300      # Docker not running
ERR_CONTAINER_FAILED=302        # Container start failed
ERR_PORT_CONFLICT=307           # Port already in use
ERR_BUILD_FAILED=311            # Docker build failed

# Network Errors
ERR_CONNECTION_FAILED=400       # Connection failed
ERR_CONNECTION_TIMEOUT=401      # Connection timeout
ERR_SSL_ERROR=403               # SSL/TLS error

# Auth Errors
ERR_AUTH_FAILED=500             # Authentication failed
ERR_PERMISSION_INSUFFICIENT=505 # Insufficient permissions

# Database Errors
ERR_DB_CONNECTION_FAILED=600    # DB connection failed
ERR_DB_MIGRATION_FAILED=602     # Migration failed

# Deployment Errors
ERR_DEPLOY_FAILED=700           # Deployment failed
ERR_DEPLOY_HEALTH_CHECK_FAILED=703  # Health check failed
```

### Error Code Tools

```bash
# List all error codes
error_list

# List by category
error_list system
error_list docker
error_list security

# Search for errors
error_search "connection"
error_search "permission"

# Get error reference
error_reference
```

---

## Security Considerations

### 1. Information Disclosure Prevention

**Never log:**
- Passwords or password hashes
- API keys or tokens
- Private keys or certificates
- Session IDs
- Personal Identifiable Information (PII)
- Internal file paths (sanitize them)
- Database connection strings with credentials

**Safe to log:**
- Usernames (non-sensitive)
- Public hostnames
- Port numbers
- Error codes
- Timestamps
- Service names
- Sanitized file paths

### 2. Path Sanitization

```bash
# Instead of logging full paths:
log_error "File not found: /Users/alice/project/.env"

# Sanitize user paths:
log_error "File not found: /Users/***USER***/project/.env"

# Or use relative paths:
log_error "File not found: .env" "working_dir=${PWD##*/}"
```

### 3. Error Message Design

**❌ Information Leakage:**
```bash
# Reveals internal structure
log_error "Database connection failed at /var/lib/postgresql/data/pgdata"

# Reveals system details
log_error "Python 3.8.10 module 'requests' not found at /usr/lib/python3/dist-packages"

# Reveals security measures
log_error "Firewall rule #42 blocked connection from 192.168.1.100"
```

**✅ Secure Errors:**
```bash
# Generic, no internal details
log_error "Database connection failed" "service=postgres"

# No version/path details
log_error "Required dependency not found" "dependency=requests"

# No rule numbers or IPs
log_security "Connection blocked by firewall policy"
```

### 4. Audit Log Security

```bash
# NEVER do this in audit logs:
audit_secret "secret_created" "value=${secret_value}"  # ❌ LEAKS SECRET!

# Always do this:
audit_secret "secret_created" "name=${secret_name}"    # ✅ SAFE

# For password changes:
audit_auth "password_changed" "user=${user}"           # ✅ No old/new password
```

### 5. Log File Permissions

Logs are automatically secured:

```bash
# Regular logs: 600 (owner read/write only)
~/.nself/logs/nself.log

# Audit logs: 600 + append-only (Linux)
~/.nself/audit/audit.log
```

**Manual permission check:**
```bash
# Check log permissions
ls -la ~/.nself/logs/
ls -la ~/.nself/audit/

# Verify append-only on Linux
lsattr ~/.nself/audit/audit.log
# Should show: -----a--------

# Fix permissions if needed
chmod 600 ~/.nself/logs/*.log
chmod 600 ~/.nself/audit/*.log
```

---

## Examples

### Example 1: Service Startup with Logging

```bash
#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/audit-logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/error-codes.sh"

start_service() {
  local service_name="$1"

  log_info "Starting service" "service=${service_name}"
  audit_system "service_start_initiated" "service=${service_name}"

  # Check if Docker is running
  if ! docker info >/dev/null 2>&1; then
    error_throw "$ERR_DOCKER_NOT_RUNNING" \
      "Docker daemon is not accessible"
    return 1
  fi

  # Start the service
  if docker-compose up -d "$service_name" 2>/dev/null; then
    log_info "Service started successfully" "service=${service_name}"
    audit_system "service_started" "service=${service_name}"
  else
    log_error "Failed to start service" "service=${service_name}"
    audit_system "service_start_failed" "service=${service_name}"
    error_throw "$ERR_CONTAINER_FAILED" \
      "Service failed to start" \
      "service=${service_name}"
    return 1
  fi
}

start_service "postgres"
```

### Example 2: Configuration Validation

```bash
#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/audit-logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/error-codes.sh"

validate_config() {
  local config_file="${1:-.env}"

  log_debug "Validating configuration" "file=${config_file}"

  # Check if config exists
  if [[ ! -f "$config_file" ]]; then
    error_throw "$ERR_CONFIG_MISSING" \
      "Configuration file not found" \
      "file=${config_file}" \
      "expected_location=${PWD}"
    return 1
  fi

  # Check required variables
  local required_vars=("PROJECT_NAME" "POSTGRES_PASSWORD" "HASURA_GRAPHQL_ADMIN_SECRET")
  local missing_vars=()

  for var in "${required_vars[@]}"; do
    if ! grep -q "^${var}=" "$config_file"; then
      missing_vars+=("$var")
      log_warn "Required variable missing" "variable=${var}"
    fi
  done

  if [[ ${#missing_vars[@]} -gt 0 ]]; then
    error_throw "$ERR_ENV_VAR_MISSING" \
      "Required environment variables missing" \
      "count=${#missing_vars[@]}" \
      "variables=${missing_vars[*]}"
    return 1
  fi

  # Check for sensitive data in Git (security check)
  if [[ -f .git/config ]] && grep -q "POSTGRES_PASSWORD" "$config_file" 2>/dev/null; then
    log_warn "Sensitive data may be committed to Git" "file=${config_file}"
    audit_security "sensitive_data_in_git" "file=${config_file}"
  fi

  log_info "Configuration validated successfully"
  audit_config "validation_passed" "file=${config_file}"
}

validate_config ".env"
```

### Example 3: Secret Rotation

```bash
#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/audit-logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/error-codes.sh"

rotate_secret() {
  local secret_name="$1"
  local new_value="$2"

  log_info "Rotating secret" "name=${secret_name}"
  audit_secret "rotation_started" "name=${secret_name}"

  # NEVER log the actual secret value
  # log_debug "New value: ${new_value}"  # ❌ NEVER DO THIS!

  # Validate secret strength
  if [[ ${#new_value} -lt 16 ]]; then
    log_warn "Secret is too short" "name=${secret_name}" "min_length=16"
    audit_security "weak_secret_detected" "name=${secret_name}"
    error_warn "$ERR_SECURITY_POLICY_FAILED" \
      "Secret does not meet strength requirements"
  fi

  # Update secret (implementation depends on secret backend)
  if update_secret_backend "$secret_name" "$new_value"; then
    log_info "Secret rotated successfully" "name=${secret_name}"
    audit_secret "rotation_completed" "name=${secret_name}"

    # Notify dependent services (if needed)
    log_debug "Notifying dependent services" "secret=${secret_name}"
  else
    log_error "Secret rotation failed" "name=${secret_name}"
    audit_secret "rotation_failed" "name=${secret_name}"
    error_throw "$ERR_SECRET_ROTATION_FAILED" \
      "Failed to rotate secret" \
      "name=${secret_name}"
    return 1
  fi
}

# Stub for example
update_secret_backend() {
  # Implementation would go here
  return 0
}

rotate_secret "database_password" "$(openssl rand -base64 32)"
```

### Example 4: Deployment with Audit Trail

```bash
#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/audit-logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/error-codes.sh"

deploy_to_environment() {
  local environment="$1"
  local version="$2"

  log_info "Starting deployment" "env=${environment}" "version=${version}"
  audit_deploy "deployment_started" \
    "environment=${environment}" \
    "version=${version}" \
    "trigger=manual"

  # Pre-deployment validation
  log_debug "Running pre-deployment checks"
  if ! validate_deployment "$environment" "$version"; then
    log_error "Pre-deployment validation failed"
    audit_deploy "deployment_validation_failed" \
      "environment=${environment}" \
      "version=${version}"
    error_throw "$ERR_DEPLOY_VALIDATION_FAILED" \
      "Deployment validation failed"
    return 1
  fi

  # Execute deployment
  log_info "Executing deployment" "env=${environment}"
  if execute_deployment "$environment" "$version"; then
    log_info "Deployment completed successfully"
    audit_deploy "deployment_completed" \
      "environment=${environment}" \
      "version=${version}" \
      "status=success"
  else
    log_error "Deployment failed"
    audit_deploy "deployment_failed" \
      "environment=${environment}" \
      "version=${version}" \
      "status=failed"

    # Initiate rollback
    log_warn "Initiating automatic rollback"
    audit_deploy "rollback_initiated" \
      "environment=${environment}" \
      "reason=deployment_failed"

    error_throw "$ERR_DEPLOY_FAILED" \
      "Deployment failed, rollback initiated"
    return 1
  fi

  # Post-deployment health checks
  log_debug "Running post-deployment health checks"
  if ! check_deployment_health "$environment"; then
    log_error "Health checks failed after deployment"
    audit_deploy "health_check_failed" \
      "environment=${environment}" \
      "version=${version}"
    error_warn "$ERR_DEPLOY_HEALTH_CHECK_FAILED" \
      "Deployment completed but health checks failed"
  fi
}

# Stubs for example
validate_deployment() { return 0; }
execute_deployment() { return 0; }
check_deployment_health() { return 0; }

deploy_to_environment "production" "v1.2.3"
```

---

## CLI Tools

### View Logs

```bash
# Service logs
nself logs                      # Last 10 lines from all services
nself logs --more               # Last 50 lines
nself logs -f                   # Follow logs (real-time)
nself logs -e                   # Errors only
nself logs postgres             # Specific service

# System logs
log_tail 50                     # Last 50 lines
log_follow                      # Follow in real-time
log_search "error"              # Search logs
log_stats                       # Log statistics
```

### View Audit Logs

```bash
# Via nself logs command
nself logs --audit              # Show audit trail
nself logs --audit AUTH         # Authentication events only
nself logs --audit-stats        # Audit statistics
nself logs --audit-export audit.csv  # Export to CSV

# Via nself audit command
nself audit query               # All audit events
nself audit query AUTH          # By category
nself audit query DEPLOY        # Deployment events
nself audit stats               # Statistics
nself audit export audit.json json  # Export to JSON
nself audit verify              # Verify integrity
```

### Error Code Reference

```bash
# List all error codes
error_list

# List by category
error_list docker
error_list security

# Search errors
error_search "permission"
error_search "connection"

# Show reference guide
error_reference
```

---

## Best Practices Summary

### ✅ DO

- Use structured logging functions (`log_info`, `log_error`, etc.)
- Sanitize all user input and file paths
- Use error codes for consistency
- Log security events to audit trail
- Provide actionable error messages
- Set appropriate log levels
- Rotate logs automatically
- Verify audit log integrity regularly

### ❌ DON'T

- Log passwords, tokens, or secrets
- Use `echo` for error messages
- Expose internal file paths
- Log PII without sanitization
- Ignore audit logging for security events
- Modify audit logs manually
- Commit logs to version control
- Share logs publicly without review

---

## Compliance Checklist

For production deployments:

- [ ] Audit logging enabled (`NSELF_AUDIT_ENABLED=true`)
- [ ] Log rotation configured
- [ ] Audit log permissions verified (600)
- [ ] Append-only attribute set (Linux)
- [ ] Regular audit log verification scheduled
- [ ] Log retention policy documented
- [ ] Sensitive data sanitization verified
- [ ] Audit log backup strategy implemented
- [ ] Access to logs restricted
- [ ] Log export procedure documented

---

## Additional Resources

- [Logging API Reference](../reference/api/README.md)
- [Audit Logging API Reference](../reference/api/README.md)
- [Error Codes Reference](../reference/api/README.md)
- [Security Best Practices](../security/SECURITY-BEST-PRACTICES.md)
- [Compliance Guide](../security/COMPLIANCE-GUIDE.md)

---

**Last Updated:** 2024-03-15
**Version:** 1.0.0
