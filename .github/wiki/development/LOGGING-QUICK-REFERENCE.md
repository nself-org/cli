# Logging & Error Handling - Quick Reference

One-page reference for nself logging, audit trails, and error handling.

---

## Setup

```bash
# Import libraries
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/audit-logging.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils/error-codes.sh"

# Configure (optional - has good defaults)
export NSELF_LOG_LEVEL=3              # INFO
export NSELF_AUDIT_ENABLED=true
```

---

## Logging Functions

```bash
# Log levels (0=FATAL to 5=TRACE)
log_fatal "Critical error"           # Level 0
log_error "Error message"            # Level 1
log_warn "Warning message"           # Level 2
log_info "Info message"              # Level 3 (default)
log_debug "Debug message"            # Level 4
log_trace "Trace message"            # Level 5

# With context
log_error "Connection failed" "host=db.example.com" "port=5432"
```

---

## Audit Logging

```bash
# By category
audit_auth "login_success" "user=admin"
audit_config "env_updated" "key=DATABASE_URL"
audit_deploy "deployment_started" "env=production"
audit_secret "secret_rotated" "name=api_key"  # NEVER log actual secret!
audit_security "firewall_rule_added" "port=443"
audit_data "database_backup" "size=10GB"
audit_admin "user_created" "username=john"
audit_system "service_started" "service=nginx"

# Generic
audit_log "CATEGORY" "action" "detail1" "detail2"
```

---

## Error Handling

```bash
# Throw error (continues execution)
error_throw "$ERR_PORT_CONFLICT" \
  "Port 5432 is in use" \
  "port=5432"

# Exit with error
error_exit "$ERR_DOCKER_NOT_RUNNING" \
  "Docker is not accessible"

# Warning (non-fatal)
error_warn "$ERR_SERVICE_DEGRADED" \
  "Service is slow" \
  "latency=2000ms"
```

---

## Common Error Codes

```bash
# Configuration (2xx)
ERR_CONFIG_MISSING=200
ERR_ENV_VAR_MISSING=203

# Docker (3xx)
ERR_DOCKER_NOT_RUNNING=300
ERR_CONTAINER_FAILED=302
ERR_PORT_CONFLICT=307
ERR_BUILD_FAILED=311

# Network (4xx)
ERR_CONNECTION_FAILED=400
ERR_SSL_ERROR=403

# Auth (5xx)
ERR_AUTH_FAILED=500
ERR_PERMISSION_INSUFFICIENT=505

# Database (6xx)
ERR_DB_CONNECTION_FAILED=600
ERR_DB_MIGRATION_FAILED=602

# Deployment (7xx)
ERR_DEPLOY_FAILED=700
ERR_DEPLOY_HEALTH_CHECK_FAILED=703

# Service (8xx)
ERR_SERVICE_UNAVAILABLE=800
ERR_SERVICE_INIT_FAILED=804

# Security (9xx)
ERR_SECURITY_VIOLATION=900
ERR_SECRET_NOT_FOUND=902
```

---

## CLI Commands

```bash
# View service logs
nself logs                        # Last 10 lines
nself logs -f                     # Follow (real-time)
nself logs -e                     # Errors only
nself logs postgres               # Specific service

# View audit logs
nself logs --audit                # All audit events
nself logs --audit AUTH           # Auth events only
nself logs --audit-stats          # Statistics
nself logs --audit-export audit.csv  # Export

# Manage audit logs
nself audit query                 # Query all
nself audit query AUTH            # By category
nself audit stats                 # Statistics
nself audit export file.json json # Export
nself audit verify                # Check integrity

# Error reference
error_list                        # All errors
error_list docker                 # By category
error_search "connection"         # Search
```

---

## Configuration

```bash
# Logging
NSELF_LOG_LEVEL=3                    # 0-5, default: 3 (INFO)
NSELF_LOG_DIR="${HOME}/.nself/logs"
NSELF_LOG_FILE="nself.log"
NSELF_LOG_TO_FILE=true
NSELF_LOG_TO_CONSOLE=true
NSELF_LOG_MAX_SIZE=10485760          # 10MB
NSELF_LOG_MAX_FILES=5
NSELF_LOG_SANITIZE=true              # ALWAYS true in production!

# Audit
NSELF_AUDIT_ENABLED=true             # REQUIRED for compliance
NSELF_AUDIT_DIR="${HOME}/.nself/audit"
NSELF_AUDIT_FILE="audit.log"
```

---

## Security Rules

### ❌ NEVER Log

- Passwords
- API keys or tokens
- JWT tokens
- Private keys
- Session IDs
- Credit card numbers
- SSN or PII

### ✅ Safe to Log

- Usernames
- Public hostnames
- Port numbers
- Error codes
- Timestamps
- Service names
- Sanitized paths

### Example

```bash
# ❌ WRONG - Exposes password
log_info "Connection: postgresql://user:password123@localhost/db"

# ✅ RIGHT - Automatically sanitized
log_info "Connection: postgresql://user:password123@localhost/db"
# Logged as: "Connection: postgresql://***REDACTED***@localhost/db"

# ✅ BETTER - Don't include sensitive data
log_info "Database connection" "host=localhost" "database=myapp"
```

---

## Common Patterns

### Service Startup

```bash
log_info "Starting service" "service=${service_name}"
audit_system "service_start" "service=${service_name}"

if ! start_service "$service_name"; then
  error_exit "$ERR_SERVICE_INIT_FAILED" \
    "Failed to start service" \
    "service=${service_name}"
fi

log_info "Service started successfully"
```

### Configuration Changes

```bash
local old_value=$(get_config "$key")
set_config "$key" "$new_value"

audit_config "config_updated" \
  "key=${key}" \
  "old_value=${old_value}" \
  "new_value=${new_value}"
```

### Secret Operations

```bash
# NEVER log the actual secret value!
log_info "Rotating secret" "name=${secret_name}"
audit_secret "rotation_started" "name=${secret_name}"

rotate_secret_backend "$secret_name" "$new_value"

audit_secret "rotation_completed" "name=${secret_name}"
```

### Deployment

```bash
log_info "Deployment started" "env=${env}" "version=${version}"
audit_deploy "deployment_started" \
  "environment=${env}" \
  "version=${version}"

if deploy_to_environment "$env" "$version"; then
  audit_deploy "deployment_completed" "status=success"
else
  audit_deploy "deployment_failed" "status=failed"
  error_exit "$ERR_DEPLOY_FAILED" "Deployment failed"
fi
```

---

## File Locations

```
Source:
  src/lib/utils/logging.sh
  src/lib/utils/audit-logging.sh
  src/lib/utils/error-codes.sh

Runtime:
  ~/.nself/logs/nself.log          (regular logs)
  ~/.nself/audit/audit.log         (audit trail)

Docs:
  docs/development/ERROR-HANDLING.md
  docs/development/LOGGING-INTEGRATION-GUIDE.md
```

---

## Troubleshooting

```bash
# Logs not appearing?
echo "Log level: $NSELF_LOG_LEVEL"
echo "Log file: ${NSELF_LOG_DIR}/${NSELF_LOG_FILE}"
ls -la "${NSELF_LOG_DIR}/${NSELF_LOG_FILE}"

# Audit not working?
echo "Audit enabled: $NSELF_AUDIT_ENABLED"
ls -la "${NSELF_AUDIT_DIR}/${NSELF_AUDIT_FILE}"

# Permission issues?
chmod 700 "${NSELF_LOG_DIR}"
chmod 600 "${NSELF_LOG_DIR}"/*.log
```

---

## Best Practices Checklist

- [ ] Use `log_*` functions, not `echo`
- [ ] Set appropriate log levels
- [ ] Log security events to audit trail
- [ ] Use error codes for consistency
- [ ] Never log passwords/secrets
- [ ] Sanitize file paths and user data
- [ ] Provide actionable error messages
- [ ] Verify audit logs regularly (`audit_verify`)

---

**Full Documentation:** [ERROR-HANDLING.md](ERROR-HANDLING.md) | [LOGGING-INTEGRATION-GUIDE.md](LOGGING-INTEGRATION-GUIDE.md)
