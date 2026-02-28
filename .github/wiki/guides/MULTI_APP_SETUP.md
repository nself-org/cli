# Multi-App Setup Guide for nself

## Overview

nself now supports multiple frontend applications sharing a single backend infrastructure. This allows you to build complex systems with:
- One database (PostgreSQL)
- One auth service (Hasura Auth)
- One storage service (MinIO/Hasura Storage)
- Multiple frontend apps with their own routes and APIs

## Key Concepts

### Shared Backend Services
All apps share the same core services:
- **Database**: Single PostgreSQL instance with schema-based separation
- **Auth**: One auth service handling all apps with per-app redirect URLs
- **Storage**: Shared S3-compatible storage
- **Cache**: Single Redis instance

### Per-App Features
When you define a `REMOTE_SCHEMA_URL` for an app, you automatically get:
- **Per-app API route**: `api.app1.localhost` → Your app's GraphQL endpoint
- **Per-app auth route**: `auth.app1.localhost` → Proxies to shared auth service
- **Table namespacing**: Using `TABLE_PREFIX` to separate data (e.g., `app1_users`)

## Configuration

### Basic Multi-App Setup

```bash
# .env or .env.dev

# Core services
POSTGRES_ENABLED=true
HASURA_ENABLED=true
AUTH_ENABLED=true
STORAGE_ENABLED=true

# Define multiple frontend apps
FRONTEND_APP_COUNT=3

# App 1: Admin Portal
FRONTEND_APP_1_DISPLAY_NAME="Admin Portal"
FRONTEND_APP_1_SYSTEM_NAME=admin
FRONTEND_APP_1_TABLE_PREFIX=adm_       # Tables: adm_users, adm_posts, etc.
FRONTEND_APP_1_PORT=3001
FRONTEND_APP_1_ROUTE=admin.localhost
FRONTEND_APP_1_REMOTE_SCHEMA_NAME=admin_schema
FRONTEND_APP_1_REMOTE_SCHEMA_URL=api.admin  # Creates api.admin.localhost

# App 2: Customer Portal
FRONTEND_APP_2_DISPLAY_NAME="Customer Portal"
FRONTEND_APP_2_SYSTEM_NAME=customer
FRONTEND_APP_2_TABLE_PREFIX=cust_      # Tables: cust_users, cust_orders, etc.
FRONTEND_APP_2_PORT=3002
FRONTEND_APP_2_ROUTE=customer.localhost
FRONTEND_APP_2_REMOTE_SCHEMA_NAME=customer_schema
FRONTEND_APP_2_REMOTE_SCHEMA_URL=api.customer  # Creates api.customer.localhost

# App 3: Mobile App (uses main API, no remote schema)
FRONTEND_APP_3_DISPLAY_NAME="Mobile App"
FRONTEND_APP_3_SYSTEM_NAME=mobile
FRONTEND_APP_3_TABLE_PREFIX=mob_       # Tables: mob_sessions, mob_devices, etc.
FRONTEND_APP_3_PORT=3003
FRONTEND_APP_3_ROUTE=mobile.localhost
# No REMOTE_SCHEMA_URL = uses main Hasura API at api.localhost
```

### Custom Service APIs

For apps with remote schemas, you need to provide the actual API service:

```bash
# Define custom services for per-app APIs
CS_1=node-ts:admin-api:4001:api.admin
CS_2=node-ts:customer-api:4002:api.customer

# CS_N format: type:name:port:route[:internal]
# - type: Service template (node-ts, python, go, etc.)
# - name: Service name
# - port: Internal port
# - route: External route subdomain
# - internal: Optional, set to "true" for internal-only services
```

## Generated Routes

With the above configuration, nself automatically creates:

### Admin App
- Frontend: `https://admin.localhost`
- API: `https://api.admin.localhost/graphql`
- Auth: `https://auth.admin.localhost`

### Customer App
- Frontend: `https://customer.localhost`
- API: `https://api.customer.localhost/graphql`
- Auth: `https://auth.customer.localhost`

### Mobile App
- Frontend: `https://mobile.localhost`
- Main API: `https://api.localhost/graphql` (shared)
- Main Auth: `https://auth.localhost` (shared)

## Database Schema Organization

### Schema Separation
Each app can have its own schema based on `TABLE_PREFIX`:

```sql
-- Core schemas (always created)
auth.*          -- Shared authentication tables
storage.*       -- Shared file storage metadata
public.*        -- Shared/common tables

-- App-specific schemas (from TABLE_PREFIX)
adm.*           -- Admin app tables
cust.*          -- Customer app tables
mob.*           -- Mobile app tables
```

### Table Naming Convention
When using `TABLE_PREFIX`, tables are namespaced:

```sql
-- Without prefix (shared tables)
auth.users      -- All users across all apps
public.settings -- Global settings

-- With prefix (app-specific tables)
adm.users       -- Admin-specific user extensions
cust.orders     -- Customer orders
mob.sessions    -- Mobile app sessions
```

## Authentication Flow

### Shared Auth with App Isolation
1. All apps use the same auth service
2. Auth service accepts redirect URLs for all configured apps
3. Each app can have its own auth subdomain (`auth.app1.localhost`)
4. The auth subdomain proxies to the shared auth service while preserving the Host header

### Redirect URLs Configuration
nself automatically configures allowed redirect URLs:

```javascript
// Automatically generated for auth service
AUTH_ALLOWED_REDIRECT_URLS = [
  "http://admin.localhost/*",
  "https://admin.localhost/*",
  "http://auth.admin.localhost/*",
  "https://auth.admin.localhost/*",
  "http://api.admin.localhost/*",
  "https://api.admin.localhost/*",
  // ... same for all other apps
]
```

## SSL Certificates

SSL certificates are automatically generated for all domains:

```bash
# Run after configuration
nself ssl    # Generate certificates
nself trust  # Trust them in your system

# Certificates include:
# - *.localhost (wildcard)
# - app1.localhost, auth.app1.localhost, api.app1.localhost
# - app2.localhost, auth.app2.localhost, api.app2.localhost
# - All other configured routes
```

## Nginx Routing

nself automatically generates nginx configurations for:

1. **Frontend routes**: `app.localhost` → Container at port 3000+
2. **API routes**: `api.app.localhost` → Custom service
3. **Auth routes**: `auth.app.localhost` → Shared auth service
4. **Core services**: Storage, mail, database admin, etc.

## Advanced Configuration

### Isolated Auth Tables

For complete app isolation, enable per-app auth tables:

```bash
FRONTEND_APP_1_ISOLATE_AUTH=true  # Creates adm.users instead of using auth.users
```

### Custom Remote Schema Headers

Apps automatically send identifying headers:

```javascript
// Remote schema receives:
headers: {
  "X-App-Name": "admin"  // From FRONTEND_APP_1_SYSTEM_NAME
}
```

### Multiple Environments

Use environment cascading for different stages:

```bash
# .env.dev - Development settings (committed)
FRONTEND_APP_COUNT=3

# .env.staging - Staging overrides
FRONTEND_APP_1_ROUTE=admin.staging.example.com

# .env.prod - Production settings
FRONTEND_APP_1_ROUTE=admin.example.com
BASE_DOMAIN=example.com
```

## Commands

```bash
# Initialize project
nself init

# Build configuration
nself build

# Start services
nself start

# Check status
nself status

# View all URLs
nself urls

# Database management
nself db

# View logs
nself logs [service]
```

## Best Practices

1. **Use TABLE_PREFIX** for data isolation between apps
2. **Define REMOTE_SCHEMA_URL** only when the app needs its own API
3. **Share the auth service** - don't duplicate authentication logic
4. **Use schemas** instead of separate databases for multi-tenancy
5. **Test locally** with `.localhost` domains before production
6. **Version control** your `.env.dev` file for team collaboration
7. **Keep secrets** in `.env.secrets` (never commit)

## Troubleshooting

### SSL Certificate Errors
```bash
# Regenerate certificates with all domains
nself ssl --force
nself trust
```

### Auth Redirect Issues
```bash
# Check current configuration
nself config | grep AUTH_ALLOWED_REDIRECT_URLS

# Rebuild if needed
nself build --force
```

### Service Discovery
```bash
# View all routes
nself urls

# Check nginx configuration
cat nginx/conf.d/*.conf
```

### Database Schema Issues
```bash
# Connect to database
nself db

# Check schemas
\dn

# Check tables in schema
\dt adm.*
```

## Example: E-commerce Platform

```bash
# Three apps sharing one backend
FRONTEND_APP_COUNT=3

# Customer storefront
FRONTEND_APP_1_SYSTEM_NAME=store
FRONTEND_APP_1_TABLE_PREFIX=store_
FRONTEND_APP_1_ROUTE=shop.example.com
FRONTEND_APP_1_REMOTE_SCHEMA_URL=api.shop

# Vendor dashboard
FRONTEND_APP_2_SYSTEM_NAME=vendor
FRONTEND_APP_2_TABLE_PREFIX=vendor_
FRONTEND_APP_2_ROUTE=vendor.example.com
FRONTEND_APP_2_REMOTE_SCHEMA_URL=api.vendor

# Admin panel
FRONTEND_APP_3_SYSTEM_NAME=admin
FRONTEND_APP_3_TABLE_PREFIX=admin_
FRONTEND_APP_3_ROUTE=admin.example.com
FRONTEND_APP_3_REMOTE_SCHEMA_URL=api.admin

# API services for each app
CS_1=node-ts:store-api:4001:api.shop
CS_2=node-ts:vendor-api:4002:api.vendor
CS_3=node-ts:admin-api:4003:api.admin
```

This creates a complete e-commerce platform with:
- Shared user authentication
- Shared product catalog (in public schema)
- App-specific tables (store_carts, vendor_products, admin_logs)
- Per-app GraphQL APIs with custom business logic
- Single database, optimized resource usage