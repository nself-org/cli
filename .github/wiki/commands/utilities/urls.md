# nself urls

**Category**: Utilities

Display all service URLs and access information.

## Overview

Shows all accessible URLs for your nself services, including GraphQL endpoints, admin panels, storage, and custom services.

**Features**:
- ✅ All service URLs in one view
- ✅ Environment-aware (localhost/staging/prod domains)
- ✅ Clickable links (terminal support)
- ✅ Port information
- ✅ Authentication details
- ✅ QR codes for mobile access

## Usage

```bash
nself urls [OPTIONS] [SERVICE]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-f, --format FORMAT` | Output format (table/list/json) | table |
| `--public-only` | Show only publicly accessible URLs | false |
| `--internal-only` | Show only internal URLs | false |
| `--qr` | Generate QR codes for URLs | false |
| `-c, --copy` | Copy URL to clipboard | false |

## Arguments

| Argument | Description |
|----------|-------------|
| `SERVICE` | Show URL for specific service only (optional) |

## Examples

### Show All URLs

```bash
nself urls
```

**Output**:
```
╔═══════════════════════════════════════════════════════════╗
║                  nself Service URLs                       ║
╚═══════════════════════════════════════════════════════════╝

Core Services
──────────────────────────────────────────────────────────
GraphQL API    https://api.localhost
  → Admin Secret: admin-secret-here
  → Console: http://localhost:8080/console

Auth API       https://auth.localhost
  → Endpoint: /v1/auth

Database       postgresql://localhost:5432/myapp_db
  → User: postgres
  → Internal: postgres:5432

Optional Services
──────────────────────────────────────────────────────────
nself Admin    https://admin.localhost
  → Login: admin@localhost

MinIO Console  https://minio.localhost
  → Access Key: minioadmin
  → S3 Endpoint: https://storage.localhost

Redis          redis://localhost:6379
  → Password: (from .env)

MeiliSearch    https://search.localhost
  → Master Key: (from .env)

Monitoring
──────────────────────────────────────────────────────────
Grafana        https://grafana.localhost
  → Username: admin
  → Password: (from .env)

Prometheus     https://prometheus.localhost

Custom Services
──────────────────────────────────────────────────────────
API            https://api-custom.localhost
Worker         (no public URL)

Frontend Applications
──────────────────────────────────────────────────────────
app1           https://app1.localhost (port 3000)
app2           https://app2.localhost (port 3001)
```

### Specific Service URL

```bash
nself urls hasura
```

**Output**:
```
Hasura GraphQL Engine
─────────────────────────────────────────
Public URL:     https://api.localhost
Console:        http://localhost:8080/console
Admin Secret:   admin-secret-here
WebSocket:      wss://api.localhost/v1/graphql

GraphQL Endpoint: https://api.localhost/v1/graphql
Health Check:     https://api.localhost/healthz
Version:          https://api.localhost/v1/version
```

### JSON Format

```bash
nself urls --format json
```

**Output**:
```json
{
  "services": {
    "hasura": {
      "name": "Hasura GraphQL",
      "url": "https://api.localhost",
      "console": "http://localhost:8080/console",
      "endpoints": {
        "graphql": "https://api.localhost/v1/graphql",
        "health": "https://api.localhost/healthz"
      },
      "auth": {
        "admin_secret": "***"
      }
    },
    "auth": {
      "name": "nHost Auth",
      "url": "https://auth.localhost",
      "endpoints": {
        "signup": "https://auth.localhost/v1/signup",
        "signin": "https://auth.localhost/v1/signin",
        "token": "https://auth.localhost/v1/token"
      }
    }
  }
}
```

### Copy URL to Clipboard

```bash
nself urls hasura --copy
```

**Output**:
```
✓ Copied to clipboard: https://api.localhost
```

### Generate QR Code

```bash
nself urls --qr
```

**Output**:
```
Hasura GraphQL API
https://api.localhost

█████████████████████████████
██ ▄▄▄▄▄ █▀ █▀▀██ ▄▄▄▄▄ ██
██ █   █ █▀▀▄ ▀█ █   █ ██
██ █▄▄▄█ █ ▄▀▄ █ █▄▄▄█ ██
██▄▄▄▄▄▄▄█▀▄▀▄▀█▄▄▄▄▄▄▄██
...
```

**Scan with mobile device to access service.**

## Environment-Specific URLs

### Development (localhost)

```bash
ENV=dev nself urls
```

**Uses**: `localhost` or `.local.nself.org`

### Staging

```bash
ENV=staging nself urls
```

**Uses**: Configured staging domain
```
GraphQL API: https://api.staging.example.com
Auth API:    https://auth.staging.example.com
```

### Production

```bash
ENV=prod nself urls
```

**Uses**: Production domain
```
GraphQL API: https://api.example.com
Auth API:    https://auth.example.com
```

## Access Information

### Credentials Display

By default, sensitive information is masked:
```
Admin Secret: ***
Password:     (from .env)
Master Key:   ***
```

### Show Full Credentials (Development Only)

```bash
SHOW_CREDENTIALS=true nself urls
```

**Output**:
```
Hasura Admin Secret: admin-secret-12345
MinIO Access Key:    minioadmin
MinIO Secret Key:    minioadmin123
```

**⚠️ Never use in production or shared screens!**

## Service Categories

### Core Services (Always Available)

- **GraphQL API** (Hasura)
- **Auth API** (nHost Auth)
- **Database** (PostgreSQL - internal only)

### Optional Services (If Enabled)

- **nself Admin** - Management UI
- **MinIO Console** - Object storage UI
- **MeiliSearch** - Search API
- **Redis** - Cache (internal)

### Monitoring Services (If Enabled)

- **Grafana** - Dashboards
- **Prometheus** - Metrics API
- **Alertmanager** - Alerts

### Custom Services (If Configured)

- User-defined CS_1 through CS_10

### Frontend Applications (If Configured)

- User-defined frontend apps

## URL Testing

### Test All URLs

```bash
nself urls --test
```

**Output**:
```
Testing service URLs...

✓ https://api.localhost (200 OK)
✓ https://auth.localhost (200 OK)
✓ https://admin.localhost (200 OK)
✓ https://minio.localhost (200 OK)
✗ https://search.localhost (Connection refused)

Results: 4/5 accessible
```

### Test Specific Service

```bash
curl -I https://api.localhost/healthz
```

## Common URL Patterns

### GraphQL API

```
https://api.{domain}/v1/graphql     # GraphQL endpoint
https://api.{domain}/v1/query       # Query only
https://api.{domain}/v1/metadata    # Metadata
https://api.{domain}/healthz        # Health check
```

### Auth API

```
https://auth.{domain}/v1/signup     # User registration
https://auth.{domain}/v1/signin     # User login
https://auth.{domain}/v1/signout    # User logout
https://auth.{domain}/v1/token      # Token refresh
https://auth.{domain}/v1/user       # User info
```

### Storage (MinIO)

```
https://storage.{domain}            # S3 API endpoint
https://minio.{domain}              # Console UI
```

### Admin

```
https://admin.{domain}              # Admin dashboard
https://admin.{domain}/api/health   # Health check
```

## Troubleshooting

### URLs not accessible

**Check service status**:
```bash
nself status
```

**Check Nginx configuration**:
```bash
nself logs nginx | grep error
```

**Verify DNS/hosts**:
```bash
cat /etc/hosts | grep localhost
```

### Wrong domain shown

**Check environment**:
```bash
grep BASE_DOMAIN .env
```

**Regenerate configuration**:
```bash
nself build
nself restart nginx
```

### SSL certificate errors

**Development (self-signed)**:
Accept certificate in browser or:
```bash
# Chrome/Edge
chrome://flags/#allow-insecure-localhost

# Firefox
about:config → network.stricttransportsecurity.preloadlist → false
```

**Production**: Verify SSL certificates are configured correctly.

## Integration

### Use in Scripts

```bash
#!/bin/bash
# Get GraphQL URL programmatically
GRAPHQL_URL=$(nself urls hasura --format json | jq -r '.url')
echo "GraphQL API: $GRAPHQL_URL"

# Test endpoint
curl -X POST $GRAPHQL_URL/v1/graphql \
  -H "x-hasura-admin-secret: $HASURA_ADMIN_SECRET" \
  -d '{"query": "{ __typename }"}'
```

### Export to Environment

```bash
# Export all URLs as env vars
eval $(nself urls --format env)
echo $HASURA_GRAPHQL_URL
echo $AUTH_URL
```

### CI/CD Integration

```bash
# In .github/workflows/test.yml
- name: Get service URLs
  run: |
    nself urls --format json > service-urls.json
    echo "GRAPHQL_URL=$(jq -r '.services.hasura.url' service-urls.json)" >> $GITHUB_ENV
```

## Related Commands

- `nself status` - Check if services are running
- `nself health` - Test service health
- `nself logs` - View service logs
- `nself admin` - Open admin UI directly

## See Also

- [nself status](status.md)
- [nself health](health.md)
- [Service Configuration](../../configuration/SERVICES.md)
- [Nginx Routing](../../guides/NGINX-ROUTING.md)
