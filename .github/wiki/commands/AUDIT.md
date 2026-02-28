# nself audit

Audit log management and querying.

## Description

The `audit` command provides tools for managing and querying audit logs in your nself application. Audit logs track important security and compliance events such as user logins, data access, configuration changes, and administrative actions.

**Use Cases:**
- Security monitoring and incident investigation
- Compliance reporting (SOC 2, HIPAA, GDPR)
- User activity tracking
- Forensic analysis
- Troubleshooting authentication issues

---

## Usage

```bash
nself audit <command> [options]
```

---

## Commands

### `init` - Initialize Audit Logging

Initialize the audit logging system in your database.

```bash
nself audit init
```

**What it does:**
- Creates audit log tables in PostgreSQL
- Sets up database triggers for automatic logging
- Configures indexes for efficient querying
- Initializes retention policies

**Example:**
```bash
nself audit init
```

**Output:**
```
✓ Audit logging initialized
```

---

### `query` - Query Audit Logs

Query audit logs with optional filters and limits.

```bash
nself audit query [filters] [limit]
```

**Parameters:**
- `filters` (optional) - JSON filter object (default: `{}`)
- `limit` (optional) - Maximum number of results (default: 100)

**Filter Fields:**
- `event_type` - Type of event (e.g., "user.login", "user.logout", "data.access")
- `actor_id` - User ID who performed the action
- `resource_type` - Type of resource accessed
- `resource_id` - ID of resource accessed
- `timestamp_from` - Start timestamp (ISO 8601)
- `timestamp_to` - End timestamp (ISO 8601)
- `ip_address` - IP address of actor
- `status` - Event status (success, failure)

**Examples:**

```bash
# Query all recent audit logs
nself audit query

# Query user login events
nself audit query '{"event_type":"user.login"}' 50

# Query actions by specific user
nself audit query '{"actor_id":"<user-uuid>"}'

# Query failed authentication attempts
nself audit query '{"event_type":"user.login","status":"failure"}'

# Query events in date range
nself audit query '{"timestamp_from":"2026-01-01T00:00:00Z","timestamp_to":"2026-01-31T23:59:59Z"}'

# Query data access events
nself audit query '{"event_type":"data.access","resource_type":"users"}' 100

# Query by IP address
nself audit query '{"ip_address":"192.168.1.100"}'
```

**Output Format:**

The query results are returned as formatted JSON:

```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "event_type": "user.login",
    "actor_id": "123e4567-e89b-12d3-a456-426614174000",
    "actor_email": "user@example.com",
    "resource_type": null,
    "resource_id": null,
    "action": "login",
    "status": "success",
    "ip_address": "192.168.1.100",
    "user_agent": "Mozilla/5.0...",
    "metadata": {
      "provider": "email",
      "mfa_enabled": true
    },
    "timestamp": "2026-01-30T12:34:56.789Z",
    "tenant_id": "tenant-uuid"
  }
]
```

---

## Event Types

### Authentication Events

| Event Type | Description |
|------------|-------------|
| `user.login` | User logged in |
| `user.logout` | User logged out |
| `user.signup` | New user registered |
| `user.password_reset` | Password reset requested |
| `user.mfa_enabled` | MFA enabled |
| `user.mfa_disabled` | MFA disabled |
| `auth.failed_attempt` | Failed login attempt |

### Data Events

| Event Type | Description |
|------------|-------------|
| `data.access` | Data accessed/read |
| `data.create` | New record created |
| `data.update` | Record updated |
| `data.delete` | Record deleted |
| `data.export` | Data exported |

### Administrative Events

| Event Type | Description |
|------------|-------------|
| `admin.user_created` | Admin created user |
| `admin.user_deleted` | Admin deleted user |
| `admin.role_changed` | User role changed |
| `admin.config_updated` | Configuration changed |
| `admin.service_restarted` | Service restarted |

### System Events

| Event Type | Description |
|------------|-------------|
| `system.started` | System started |
| `system.stopped` | System stopped |
| `system.error` | System error occurred |
| `system.backup` | Backup created |
| `system.restore` | Backup restored |

---

## Options

| Option | Description |
|--------|-------------|
| `-h, --help` | Show help message |

---

## Database Schema

### Audit Logs Table

```sql
CREATE TABLE audit_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type VARCHAR(100) NOT NULL,
  actor_id UUID REFERENCES users(id),
  actor_email VARCHAR(255),
  resource_type VARCHAR(100),
  resource_id UUID,
  action VARCHAR(100) NOT NULL,
  status VARCHAR(20) NOT NULL,
  ip_address INET,
  user_agent TEXT,
  metadata JSONB,
  timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  tenant_id UUID REFERENCES tenants(id),

  -- Indexes
  INDEX idx_audit_event_type (event_type),
  INDEX idx_audit_actor_id (actor_id),
  INDEX idx_audit_timestamp (timestamp),
  INDEX idx_audit_tenant_id (tenant_id),
  INDEX idx_audit_status (status),
  INDEX idx_audit_metadata (metadata) USING GIN
);
```

---

## Integration Examples

### Track Custom Events

Use the audit logging library to track custom events in your application:

```typescript
// TypeScript example
import { auditLog } from '@nself/audit';

// Log a custom event
await auditLog({
  event_type: 'payment.processed',
  actor_id: userId,
  resource_type: 'payment',
  resource_id: paymentId,
  action: 'process',
  status: 'success',
  metadata: {
    amount: 99.99,
    currency: 'USD',
    payment_method: 'card'
  }
});
```

### Automated Triggers

Audit logging can be automated with database triggers:

```sql
-- Auto-log user updates
CREATE TRIGGER audit_user_updates
AFTER UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION log_audit_event('data.update', 'users');
```

---

## Compliance Use Cases

### SOC 2 Compliance

```bash
# Query all administrative actions
nself audit query '{"event_type":"admin.*"}' 1000

# Query failed login attempts
nself audit query '{"event_type":"auth.failed_attempt"}' 500

# Query data access logs
nself audit query '{"event_type":"data.*"}' 5000
```

### GDPR Right to Access

```bash
# Export all actions by a specific user
nself audit query '{"actor_id":"<user-uuid>"}' 10000 > user_audit.json
```

### HIPAA Audit Trail

```bash
# Query medical record access
nself audit query '{"resource_type":"medical_records","event_type":"data.access"}' 1000
```

---

## Retention and Archival

### Default Retention

By default, audit logs are retained for:
- **Development:** 30 days
- **Staging:** 90 days
- **Production:** 365 days (1 year)

### Configure Retention

Set retention period in environment variables:

```bash
AUDIT_RETENTION_DAYS=730  # 2 years
```

### Archive Old Logs

```bash
# Export logs older than 1 year
nself audit query '{"timestamp_to":"2025-01-30T00:00:00Z"}' 100000 > archive_2025.json
```

---

## Performance Considerations

### Indexing

Audit logs are automatically indexed on:
- `event_type` - Fast event type filtering
- `actor_id` - Fast user activity queries
- `timestamp` - Fast time range queries
- `tenant_id` - Fast tenant isolation
- `metadata` - JSONB GIN index for metadata queries

### Query Optimization

```bash
# Use specific filters instead of broad queries
# Good:
nself audit query '{"event_type":"user.login","actor_id":"<uuid>"}' 100

# Avoid:
nself audit query '{}' 100000
```

### Partitioning

For high-volume applications, consider time-based partitioning:

```sql
-- Partition by month
CREATE TABLE audit_logs_2026_01 PARTITION OF audit_logs
FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
```

---

## Troubleshooting

### Audit Logs Not Recording

```bash
# Reinitialize audit system
nself audit init

# Check database triggers
psql -d $POSTGRES_DB -c "\d+ users" | grep TRIGGER
```

### Query Timeout

```bash
# Use more specific filters
nself audit query '{"event_type":"user.login"}' 50

# Add timestamp filter
nself audit query '{"timestamp_from":"2026-01-30T00:00:00Z"}' 100
```

### Large Result Sets

```bash
# Use pagination with limit
nself audit query '{"event_type":"data.access"}' 100

# Export to file
nself audit query '{"event_type":"data.access"}' 10000 > audit_export.json
```

---

## Related Commands

- **[auth](AUTH.md)** - Authentication management
- **[security](SECURITY.md)** - Security tools
- **[db](DB.md)** - Database management
- **[tenant](TENANT.md)** - Multi-tenancy management

---

## Security Notes

- Audit logs are **immutable** - cannot be modified after creation
- Only administrators can query audit logs
- Audit logs include **tenant isolation** for multi-tenant applications
- IP addresses and user agents are captured for forensics
- Sensitive data should NOT be stored in audit log metadata

---

**Version:** v0.6.0+
**Category:** Security & Compliance
**Related Documentation:**
- [Security System](../security/SECURITY-SYSTEM.md)
- [Compliance Guide](../security/COMPLIANCE-GUIDE.md)
- [Audit Trail Architecture](../architecture/ARCHITECTURE.md)
