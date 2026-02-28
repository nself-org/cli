# Admin UI Documentation

The nself Admin UI provides a comprehensive web-based interface for managing your backend stack.

## 📊 Overview

The Admin UI is a React-based dashboard that offers:
- Real-time service monitoring
- Docker container management
- Database query interface
- Log viewing and analysis
- Health checks and metrics
- Backup management
- Configuration editor

**Default URL**: http://localhost:3021

## 🚀 Quick Start

### Enable Admin UI

```bash
# Enable with temporary password
nself admin enable

# Output:
# ✅ Admin UI enabled
# 📝 Temporary password: xY3mK9pL
# 🌐 URL: http://localhost:3021
```

### Set Custom Password

```bash
# Set your own password
nself admin password MySecurePass123!

# Or generate a secure password
nself admin password --generate
```

### Access Admin UI

```bash
# Open in default browser
nself admin open

# Or navigate manually to:
# http://localhost:3021
```

## 🔧 Configuration

### Environment Variables

Add to your `.env` file:

```bash
# Admin UI Configuration
NSELF_ADMIN_ENABLED=true
NSELF_ADMIN_PORT=3021
NSELF_ADMIN_HOST=0.0.0.0

# Authentication
NSELF_ADMIN_AUTH_PROVIDER=basic
ADMIN_SECRET_KEY=your-secret-key-here
ADMIN_PASSWORD_HASH=generated-hash

# Session Settings
NSELF_ADMIN_SESSION_TIMEOUT=3600
NSELF_ADMIN_SESSION_SECURE=false

# Features
NSELF_ADMIN_ENABLE_LOGS=true
NSELF_ADMIN_ENABLE_EXEC=true
NSELF_ADMIN_ENABLE_BACKUPS=true
NSELF_ADMIN_ENABLE_METRICS=true
```

### Authentication Providers

#### Basic Authentication (Default)
```bash
NSELF_ADMIN_AUTH_PROVIDER=basic
ADMIN_USERNAME=admin
ADMIN_PASSWORD_HASH=<bcrypt-hash>
```

#### JWT Authentication
```bash
NSELF_ADMIN_AUTH_PROVIDER=jwt
NSELF_ADMIN_JWT_SECRET=your-jwt-secret
NSELF_ADMIN_JWT_ISSUER=nself-admin
```

#### OAuth2 (Coming Soon)
```bash
NSELF_ADMIN_AUTH_PROVIDER=oauth2
NSELF_ADMIN_OAUTH_CLIENT_ID=your-client-id
NSELF_ADMIN_OAUTH_CLIENT_SECRET=your-secret
NSELF_ADMIN_OAUTH_PROVIDER=github
```

## 📱 Features

### Dashboard

The main dashboard provides:
- **System Overview**: CPU, memory, disk usage
- **Service Status**: Health of all running services
- **Quick Actions**: Start, stop, restart services
- **Recent Logs**: Last 100 log entries
- **Alerts**: System warnings and errors

### Service Management

#### View Services
- List all Docker containers
- Real-time status updates
- Resource usage per container
- Network information
- Volume mounts

#### Control Services
```javascript
// Available actions
- Start/Stop/Restart
- Scale up/down
- View logs
- Execute commands
- Inspect configuration
```

### Database Management

#### Query Interface
- SQL editor with syntax highlighting
- Query history
- Export results (CSV, JSON)
- Schema browser
- Table previews

#### Database Operations
- Create/Drop databases
- Run migrations
- Backup/Restore
- View active connections
- Performance metrics

### Log Viewer

#### Features
- Real-time log streaming
- Multi-service log aggregation
- Search and filter
- Log level filtering
- Export logs
- Syntax highlighting

#### Filter Options
```javascript
{
  service: ["postgres", "hasura", "auth"],
  level: ["error", "warn", "info"],
  timeRange: "last-1h",
  search: "connection refused"
}
```

### Backup Management

#### Create Backups
- One-click backup creation
- Scheduled backups
- Selective service backup
- Compression options

#### Restore Operations
- List available backups
- Point-in-time recovery
- Verify backup integrity
- Restore to different environment

### Configuration Editor

#### Edit Settings
- Visual .env editor
- Syntax validation
- Change history
- Rollback capability
- Hot reload support

#### Service Configuration
- Docker Compose editor
- Nginx configuration
- Service-specific configs
- Template management

## 🎨 Customization

### Themes

```javascript
// themes/custom.js
export default {
  primary: '#007bff',
  secondary: '#6c757d',
  success: '#28a745',
  danger: '#dc3545',
  warning: '#ffc107',
  info: '#17a2b8',
  dark: '#343a40',
  light: '#f8f9fa'
}
```

### Custom Widgets

```javascript
// widgets/custom-metric.jsx
import { Widget } from '@nself/admin-ui';

export default function CustomMetric() {
  return (
    <Widget title="Custom Metric">
      {/* Your custom content */}
    </Widget>
  );
}
```

### API Extensions

```javascript
// api/custom-endpoint.js
export default {
  path: '/api/custom',
  method: 'GET',
  handler: async (req, res) => {
    // Custom logic
    res.json({ data: 'custom' });
  }
}
```

## 🔐 Security

### Access Control

#### Role-Based Access
```yaml
roles:
  admin:
    - all permissions
  developer:
    - view logs
    - restart services
    - query database
  viewer:
    - read-only access
```

#### IP Whitelisting
```bash
NSELF_ADMIN_IP_WHITELIST=127.0.0.1,192.168.1.0/24
```

#### 2FA Support (Coming Soon)
```bash
NSELF_ADMIN_2FA_ENABLED=true
NSELF_ADMIN_2FA_PROVIDER=totp
```

### Security Headers

```nginx
# Automatically configured
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
X-XSS-Protection: 1; mode=block
Content-Security-Policy: default-src 'self'
```

### Audit Logging

```bash
# Enable audit logs
NSELF_ADMIN_AUDIT_ENABLED=true
NSELF_ADMIN_AUDIT_PATH=/var/log/nself/audit.log

# Log format
{
  "timestamp": "2024-09-09T10:30:00Z",
  "user": "admin",
  "action": "restart_service",
  "service": "postgres",
  "ip": "192.168.1.100",
  "result": "success"
}
```

## 🔌 API Reference

### Authentication

```bash
# Login
POST /api/auth/login
{
  "username": "admin",
  "password": "password"
}

# Response
{
  "token": "jwt-token",
  "expires": 3600
}
```

### Services

```bash
# List services
GET /api/services

# Get service details
GET /api/services/{name}

# Control service
POST /api/services/{name}/action
{
  "action": "restart"
}
```

### Database

```bash
# Execute query
POST /api/database/query
{
  "sql": "SELECT * FROM users LIMIT 10"
}

# Get schema
GET /api/database/schema
```

### Logs

```bash
# Stream logs
WS /api/logs/stream?service=postgres

# Get historical logs
GET /api/logs?service=postgres&lines=100
```

## 🛠️ Admin CLI Commands

### Basic Commands

```bash
# Enable/disable admin UI
nself admin enable
nself admin disable

# Check status
nself admin status

# Set password
nself admin password [password]

# Reset to defaults
nself admin reset

# View logs
nself admin logs

# Open in browser
nself admin open
```

### Advanced Commands

```bash
# Generate API token
nself admin token generate

# List active sessions
nself admin sessions list

# Revoke session
nself admin sessions revoke [session-id]

# Export configuration
nself admin export > admin-config.json

# Import configuration
nself admin import < admin-config.json
```

## 🐛 Troubleshooting

### Cannot Access Admin UI

```bash
# Check if enabled
grep NSELF_ADMIN_ENABLED .env

# Check if running
nself status | grep admin

# Check logs
nself logs admin

# Restart admin service
nself restart admin
```

### Authentication Issues

```bash
# Reset password
nself admin password --reset

# Clear sessions
nself admin sessions clear

# Regenerate secret key
nself admin secret --regenerate
```

### Performance Issues

```bash
# Increase memory limit
NSELF_ADMIN_MEMORY_LIMIT=512M

# Enable caching
NSELF_ADMIN_CACHE_ENABLED=true

# Reduce log retention
NSELF_ADMIN_LOG_RETENTION=7d
```

## 📊 Monitoring Integration

### Prometheus Metrics

```yaml
# Exposed at /metrics
nself_admin_requests_total
nself_admin_response_time_seconds
nself_admin_active_sessions
nself_admin_api_errors_total
```

### Grafana Dashboard

Import dashboard ID: `14842` or use the provided JSON:

```bash
cp admin/grafana-dashboard.json /var/lib/grafana/dashboards/
```

## 🚀 Production Deployment

### SSL/TLS Setup

```bash
# Generate certificates
nself ssl

# Configure admin UI
NSELF_ADMIN_SSL_ENABLED=true
NSELF_ADMIN_SSL_CERT=/ssl/cert.pem
NSELF_ADMIN_SSL_KEY=/ssl/key.pem
```

### Reverse Proxy

```nginx
# Nginx configuration
location /admin {
    proxy_pass http://localhost:3100;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

### High Availability

```yaml
# Docker Swarm mode
services:
  admin:
    deploy:
      replicas: 3
      update_config:
        parallelism: 1
        delay: 10s
```

## 📚 Additional Resources

- [Video Tutorial](https://youtube.com/nself-admin-tutorial)
- [API Documentation](../architecture/API.md#admin-ui)
- [Security Guide](../guides/SECURITY.md)
- [Custom Widgets Guide](https://github.com/nself-org/cli-admin-widgets)

---

**Need help?** Check [FAQ](../getting-started/FAQ.md) or [create an issue](https://github.com/nself-org/cli/issues)