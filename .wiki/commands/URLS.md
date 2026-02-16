# nself urls - Service URLs

**Version 0.9.9** | Display all accessible URLs for your nself services

---

## Overview

The `nself urls` command displays all accessible URLs for your running services. It provides quick access to web interfaces, APIs, and admin panels with clickable links in supported terminals.

---

## Table of Contents

- [Basic Usage](#basic-usage)
- [URL Categories](#url-categories)
- [Output Formats](#output-formats)
- [Options Reference](#options-reference)
- [URL Structure](#url-structure)
- [Examples](#examples)
- [URL Configuration](#url-configuration)
- [Plugin URLs](#plugin-urls)

---

## Basic Usage

```bash
# Show all service URLs
nself urls

# Open URL in browser
nself urls --open

# Show specific service URL
nself urls hasura

# Copy URL to clipboard
nself urls grafana --copy

# Show only API endpoints
nself urls --category api
```

---

## URL Categories

### Application Root

The main application entry point:

```
https://local.nself.org                Application Root
```

### Required Services

Core infrastructure URLs:

```
https://api.local.nself.org            Hasura GraphQL Console
https://api.local.nself.org/v1/graphql GraphQL Endpoint
https://auth.local.nself.org           Authentication Service
```

### Optional Services

When enabled in configuration:

```
https://admin.local.nself.org          nself Admin Panel
https://minio.local.nself.org          MinIO Console
https://mail.local.nself.org           MailPit Web UI
https://search.local.nself.org         MeiliSearch Dashboard
https://functions.local.nself.org      Functions Runtime
https://mlflow.local.nself.org         MLflow Tracking Server
```

### Monitoring Services

When monitoring bundle is enabled:

```
https://grafana.local.nself.org        Grafana Dashboards
https://prometheus.local.nself.org     Prometheus UI
https://alertmanager.local.nself.org   Alertmanager UI
```

### Custom Services

Services defined via CS_N variables:

```
https://express-api.local.nself.org    Express API (CS_1)
https://grpc-api.local.nself.org       gRPC Service (CS_3)
https://ml-api.local.nself.org         Python ML API (CS_4)
```

### Frontend Applications

External frontend applications:

```
https://app1.local.nself.org           Frontend App 1
https://app2.local.nself.org           Frontend App 2
```

### Plugin Services

Services added by plugins:

```
https://stripe-webhooks.local.nself.org Stripe Webhook Handler
https://github-webhooks.local.nself.org GitHub Webhook Handler
```

---

## Output Formats

### Default Output

Clean, categorized display with descriptions:

```
nself URLs - myapp (development)
═══════════════════════════════════════════════════════════════

Application
  https://local.nself.org                    Application Root

Core Services
  https://api.local.nself.org                Hasura Console
  https://api.local.nself.org/v1/graphql     GraphQL API
  https://auth.local.nself.org               Authentication

Optional Services
  https://admin.local.nself.org              nself Admin
  https://minio.local.nself.org              MinIO Console
  https://mail.local.nself.org               MailPit

Monitoring
  https://grafana.local.nself.org            Grafana
  https://prometheus.local.nself.org         Prometheus
  https://alertmanager.local.nself.org       Alertmanager

Custom Services
  https://express-api.local.nself.org        Express API
  https://worker-api.local.nself.org         Worker Service

Frontend Apps
  https://app1.local.nself.org               React App
  https://app2.local.nself.org               Admin Dashboard

Total: 14 URLs
```

### JSON Output

For scripting and automation:

```bash
nself urls --json
```

```json
{
  "project": "myapp",
  "environment": "development",
  "base_domain": "local.nself.org",
  "protocol": "https",
  "urls": [
    {
      "name": "application",
      "url": "https://local.nself.org",
      "description": "Application Root",
      "category": "application",
      "service": null
    },
    {
      "name": "hasura",
      "url": "https://api.local.nself.org",
      "description": "Hasura Console",
      "category": "core",
      "service": "hasura",
      "endpoints": [
        {
          "path": "/v1/graphql",
          "description": "GraphQL API"
        },
        {
          "path": "/console",
          "description": "Hasura Console"
        }
      ]
    }
  ]
}
```

### Plain Output

Simple URL list for scripting:

```bash
nself urls --plain
```

```
https://local.nself.org
https://api.local.nself.org
https://auth.local.nself.org
https://admin.local.nself.org
https://grafana.local.nself.org
```

### Markdown Output

For documentation:

```bash
nself urls --markdown
```

```markdown
## nself URLs

### Application
- [Application Root](https://local.nself.org)

### Core Services
- [Hasura Console](https://api.local.nself.org)
- [GraphQL API](https://api.local.nself.org/v1/graphql)
- [Authentication](https://auth.local.nself.org)

### Monitoring
- [Grafana](https://grafana.local.nself.org)
- [Prometheus](https://prometheus.local.nself.org)
```

---

## Options Reference

| Option | Short | Description |
|--------|-------|-------------|
| `--json` | `-j` | Output as JSON |
| `--plain` | `-p` | Simple URL list |
| `--markdown` | `-m` | Markdown format |
| `--open` | `-o` | Open URL in browser |
| `--copy` | `-c` | Copy to clipboard |
| `--category <name>` | | Filter by category |
| `--service <name>` | `-s` | Show specific service |
| `--internal` | | Show internal URLs too |
| `--ports` | | Show port mappings |
| `--no-color` | | Disable colored output |
| `--quiet` | `-q` | URLs only, no formatting |

### Category Filters

```bash
# Core services only
nself urls --category core

# Monitoring URLs
nself urls --category monitoring

# Custom services
nself urls --category custom

# Frontend apps
nself urls --category frontend

# Plugin services
nself urls --category plugins
```

---

## URL Structure

### Domain Patterns

nself uses subdomain-based routing:

```
<service>.<base_domain>
```

Examples:
- `api.local.nself.org` - Hasura API
- `auth.local.nself.org` - Authentication
- `grafana.local.nself.org` - Grafana

### Base Domain Configuration

Set in `.env`:

```bash
# Development (default)
BASE_DOMAIN=local.nself.org

# Staging
BASE_DOMAIN=staging.example.com

# Production
BASE_DOMAIN=example.com
```

### Protocol Configuration

```bash
# HTTPS (default for production)
SSL_ENABLED=true

# HTTP (development option)
SSL_ENABLED=false
```

### Port Configuration

Development typically uses standard ports:
- HTTP: 80
- HTTPS: 443

Custom ports:

```bash
# Non-standard ports
HTTP_PORT=8080
HTTPS_PORT=8443

# URLs become:
# https://api.local.nself.org:8443
```

---

## Examples

### Open Specific Service

```bash
# Open Grafana in browser
nself urls --open grafana

# Open Hasura console
nself urls --open hasura
```

### Copy URL to Clipboard

```bash
# Copy GraphQL endpoint
nself urls hasura --copy

# Paste into Postman, curl, etc.
```

### Get GraphQL Endpoint

```bash
# Just the GraphQL URL
nself urls hasura --plain | grep graphql

# Or specific endpoint
nself urls --service hasura --endpoint graphql
```

### Integration Scripts

```bash
#!/bin/bash
# Get all API endpoints for testing

endpoints=$(nself urls --json | jq -r '.urls[] | select(.category == "core") | .url')

for url in $endpoints; do
  echo "Testing: $url"
  curl -s -o /dev/null -w "%{http_code}" "$url"
  echo ""
done
```

### Generate Documentation

```bash
# Create URLs documentation
nself urls --markdown > docs/URLS-GENERATED.md
```

### Environment-Specific URLs

```bash
# Show URLs for current environment
nself urls

# The output respects BASE_DOMAIN from .env
# Development: https://api.local.nself.org
# Staging: https://api.staging.example.com
# Production: https://api.example.com
```

---

## URL Configuration

### Service Route Configuration

Routes are defined in various places:

**Required Services** (hardcoded):
- `api.*` → Hasura
- `auth.*` → Auth service

**Optional Services** (when enabled):
- `admin.*` → nself Admin
- `minio.*` → MinIO
- `mail.*` → MailPit
- `search.*` → MeiliSearch

**Monitoring** (when enabled):
- `grafana.*` → Grafana
- `prometheus.*` → Prometheus
- `alertmanager.*` → Alertmanager

**Custom Services**:
```bash
# CS_1 route comes from service name
CS_1=my-api:express-js:8001
# Creates: https://my-api.local.nself.org
```

### Custom Route Overrides

Override default routes in `.env`:

```bash
# Custom route for Hasura
HASURA_ROUTE=graphql
# Creates: https://graphql.local.nself.org

# Custom route for custom service
CS_1_ROUTE=backend
# Creates: https://backend.local.nself.org
```

### Frontend App Routes

```bash
# Frontend app routing
FRONTEND_APP_1_NAME=webapp
FRONTEND_APP_1_ROUTE=app
FRONTEND_APP_1_PORT=3000
# Creates: https://app.local.nself.org → localhost:3000
```

---

## Plugin URLs

Plugins can add their own URLs:

### Viewing Plugin URLs

```bash
# Show plugin URLs
nself urls --category plugins
```

### Stripe Plugin URLs

When Stripe plugin is installed:

```
https://stripe-webhooks.local.nself.org   Stripe Webhook Endpoint
```

### GitHub Plugin URLs

When GitHub plugin is installed:

```
https://github-webhooks.local.nself.org   GitHub Webhook Endpoint
https://github-auth.local.nself.org       GitHub OAuth Callback
```

### Plugin URL Configuration

Plugins define routes in their `plugin.json`:

```json
{
  "routes": [
    {
      "subdomain": "stripe-webhooks",
      "target": "stripe-webhook-service",
      "port": 8001,
      "description": "Stripe Webhook Endpoint"
    }
  ]
}
```

---

## Internal URLs

### Show Internal URLs

```bash
# Include internal/container URLs
nself urls --internal
```

```
External URLs
  https://api.local.nself.org              Hasura Console

Internal URLs (Docker network)
  http://hasura:8080                       Hasura (internal)
  http://postgres:5432                     PostgreSQL (internal)
  http://redis:6379                        Redis (internal)
```

### Internal Service Discovery

Services communicate via Docker network:

```bash
# From within a container:
curl http://hasura:8080/healthz
curl http://auth:4000/healthz
```

---

## Troubleshooting

### URLs Not Accessible

```bash
# Check if services are running
nself status

# Verify nginx is routing correctly
docker logs myapp_nginx

# Check SSL certificates
openssl s_client -connect local.nself.org:443
```

### Wrong Domain

```bash
# Check BASE_DOMAIN in .env
grep BASE_DOMAIN .env

# Rebuild nginx config
nself build && nself start
```

### SSL Certificate Errors

```bash
# Trust development certificate (macOS)
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain \
  ./ssl/cert.pem

# Or use HTTP for development
SSL_ENABLED=false nself build
```

### Port Conflicts

```bash
# Check what's using ports
lsof -i :80
lsof -i :443

# Use custom ports
HTTP_PORT=8080 HTTPS_PORT=8443 nself build
```

### Missing URLs

```bash
# Service might not be enabled
grep ENABLED .env

# Enable and rebuild
echo "REDIS_ENABLED=true" >> .env
nself build && nself start
```

---

## Quick Reference

### Get Specific URLs Quickly

```bash
# GraphQL endpoint
nself urls hasura --plain | head -1

# Grafana URL
nself urls grafana --plain

# All monitoring URLs
nself urls --category monitoring --plain
```

### Common URL Patterns

| Service | URL Pattern |
|---------|-------------|
| Application | `https://<base_domain>` |
| Hasura | `https://api.<base_domain>` |
| Auth | `https://auth.<base_domain>` |
| Grafana | `https://grafana.<base_domain>` |
| Custom (CS_1) | `https://<service_name>.<base_domain>` |

---

## Related Commands

- [status](STATUS.md) - Check service status
- [start](START.md) - Start services
- [build](BUILD.md) - Generate configuration
- [logs](LOGS.md) - View service logs

---

*Last Updated: January 2026 | Version 0.9.9*
