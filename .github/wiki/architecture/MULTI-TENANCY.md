# Multi-Tenancy Architecture Guide

> **nself v0.8.0** - Comprehensive Multi-Tenancy System

## Table of Contents

1. [Overview](#overview)
2. [Core Concepts](#core-concepts)
3. [Database Architecture](#database-architecture)
4. [CLI Usage](#cli-usage)
5. [Tenant Routing](#tenant-routing)
6. [Data Isolation](#data-isolation)
7. [Security Considerations](#security-considerations)
8. [Development Workflow](#development-workflow)
9. [Production Deployment](#production-deployment)
10. [Migration Guide](#migration-guide)
11. [Advanced Topics](#advanced-topics)

---

## Overview

### What is Multi-Tenancy in nself?

nself's multi-tenancy system enables a single infrastructure to serve multiple isolated tenants (customers, organizations, or business units). Each tenant gets:

- **Isolated data** - Complete data separation using PostgreSQL Row-Level Security (RLS)
- **Independent schemas** - Optional dedicated PostgreSQL schemas per tenant
- **Custom branding** - Per-tenant settings and configurations
- **Resource quotas** - Enforced limits on users, storage, and API requests
- **Custom domains** - Tenant-specific subdomains or fully custom domains

### Use Cases

#### 1. SaaS Platforms
```
yourapp.com
├── acme.yourapp.com     → Acme Corp tenant
├── techco.yourapp.com   → TechCo tenant
└── startup.yourapp.com  → Startup Inc tenant
```

Each tenant gets isolated data, users, and configuration while sharing the same infrastructure.

#### 2. B2B Applications
Multi-department enterprise applications where each department is a tenant:
- **Finance Department** - Access to financial data only
- **HR Department** - Access to employee data only
- **Sales Department** - Access to CRM data only

#### 3. Reseller/White-Label Platforms
Single codebase serving multiple branded instances:
```
customer1.com → Tenant 1 (custom domain)
customer2.com → Tenant 2 (custom domain)
partner3.yourapp.com → Tenant 3 (subdomain)
```

### Architecture Approach

nself uses a **hybrid multi-tenancy model**:

```
┌─────────────────────────────────────────────────────────────┐
│                     Shared Infrastructure                    │
│  ┌──────────┐  ┌─────────┐  ┌────────┐  ┌──────────┐      │
│  │PostgreSQL│  │ Hasura  │  │ Redis  │  │  Nginx   │      │
│  └──────────┘  └─────────┘  └────────┘  └──────────┘      │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    Tenant Isolation Layer                    │
│  • Row-Level Security (RLS) - All shared tables             │
│  • Schema-per-tenant - Optional dedicated schemas           │
│  • Redis namespaces - Per-tenant cache isolation            │
│  • MinIO buckets - Per-tenant storage isolation             │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│  Tenant A    │  │  Tenant B    │  │  Tenant C    │
│  acme.app.com│  │  tech.app.com│  │  custom.com  │
└──────────────┘  └──────────────┘  └──────────────┘
```

**Key Features:**

1. **Shared Schema with RLS** (default)
   - All tenants share the same tables
   - PostgreSQL RLS enforces data isolation
   - Most efficient for databases with many tenants

2. **Schema-per-Tenant** (optional)
   - Each tenant gets a dedicated PostgreSQL schema
   - Complete schema isolation
   - Better for compliance requirements (GDPR, HIPAA)

3. **Hybrid Approach** (recommended)
   - Core tables use shared schema with RLS
   - Tenant-specific tables use dedicated schemas
   - Balance between isolation and efficiency

---

## Core Concepts

### 1. Tenants

A **tenant** is an isolated workspace within your nself infrastructure.

```sql
-- Tenant structure
{
  "id": "uuid",
  "slug": "acme",                    -- URL-friendly identifier
  "name": "Acme Corporation",        -- Display name
  "status": "active",                -- active | suspended | deleted
  "plan_id": "pro",                  -- Subscription plan
  "owner_user_id": "uuid",           -- Primary owner
  "max_users": 50,                   -- User limit
  "max_storage_gb": 100,             -- Storage quota
  "max_api_requests_per_month": 100000,
  "settings": {},                    -- Custom settings (JSONB)
  "metadata": {}                     -- Additional metadata
}
```

### 2. Tenant Identification

nself supports **four methods** for identifying tenants (in priority order):

#### Priority 1: X-Tenant-ID Header (Direct)
```bash
curl https://api.yourapp.com/v1/users \
  -H "X-Tenant-ID: 550e8400-e29b-41d4-a716-446655440000"
```
**Use case:** Internal service-to-service communication

#### Priority 2: X-Tenant-Slug Header
```bash
curl https://api.yourapp.com/v1/users \
  -H "X-Tenant-Slug: acme"
```
**Use case:** API clients with known tenant slug

#### Priority 3: Custom Domain
```bash
curl https://acme.example.com/v1/users
# Domain lookup: acme.example.com → Tenant ID
```
**Use case:** White-label deployments

#### Priority 4: Subdomain
```bash
curl https://acme.yourapp.com/v1/users
# Subdomain extraction: acme.yourapp.com → "acme" slug → Tenant ID
```
**Use case:** SaaS multi-tenant applications (most common)

### 3. Tenant Lifecycle

```
┌──────────┐   init    ┌────────┐  suspend  ┌───────────┐
│          │ ───────→  │ Active │ ────────→ │ Suspended │
│  Create  │           └────────┘           └───────────┘
│          │               ↑ activate            │
└──────────┘               └─────────────────────┘
     │                              │
     │ delete                       │ delete
     ↓                              ↓
┌───────────────────────────────────────────────┐
│                   Deleted                      │
│  (Schema dropped, data purged)                │
└───────────────────────────────────────────────┘
```

**States:**

- **Active** - Fully operational, users can access
- **Suspended** - Temporarily disabled, no access allowed
- **Deleted** - Soft-deleted initially, then purged (30-day retention)

### 4. Tenant Isolation Strategies

#### Strategy 1: Row-Level Security (RLS)
**How it works:**
```sql
-- Every table has a tenant_id column
CREATE TABLE users (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  email TEXT,
  ...
);

-- RLS policy ensures users only see their tenant's data
CREATE POLICY user_isolation ON users
  FOR ALL
  USING (tenant_id = tenants.current_tenant_id());
```

**Pros:**
- Efficient for large numbers of tenants
- Simple schema management
- Easy to query across tenants (admin/analytics)

**Cons:**
- Risk of misconfiguration exposing data
- All tenants share same indexes
- Harder to restore single tenant from backup

#### Strategy 2: Schema-per-Tenant
**How it works:**
```
PostgreSQL Database: myapp_db
├── tenant_550e8400_e29b_41d4_a716_446655440000/
│   ├── users
│   ├── products
│   └── orders
├── tenant_660f9511_f3ac_52e5_b827_557766551111/
│   ├── users
│   ├── products
│   └── orders
└── tenants/ (shared schema)
    ├── tenants
    ├── tenant_domains
    └── tenant_members
```

**Pros:**
- Complete schema isolation
- Easy to backup/restore single tenant
- Better for compliance (GDPR per-tenant deletion)

**Cons:**
- Schema proliferation (many tenants = many schemas)
- More complex migrations
- Harder to aggregate cross-tenant analytics

#### Strategy 3: Hybrid (Recommended)
```
Shared Schema (with RLS):
├── tenants.tenants
├── tenants.tenant_members
├── auth.users (with tenant_id + RLS)
├── auth.sessions (with tenant_id + RLS)
└── public shared tables

Per-Tenant Schemas:
├── tenant_<uuid>.custom_tables
├── tenant_<uuid>.tenant_specific_data
└── tenant_<uuid>.uploaded_files_metadata
```

**Best of both worlds:**
- Core tables shared (efficient)
- Tenant data isolated in dedicated schemas
- Configurable per-tenant

---

## Database Architecture

### Schema Structure

```
PostgreSQL Database
│
├── tenants schema (Tenant Management)
│   ├── tenants                    -- Tenant registry
│   ├── tenant_schemas             -- Schema tracking
│   ├── tenant_domains             -- Custom domains
│   ├── tenant_members             -- User-tenant membership
│   └── tenant_settings            -- Per-tenant settings
│
├── auth schema (Authentication - RLS Enabled)
│   ├── users (tenant_id)          -- Tenant-isolated users
│   ├── sessions (tenant_id)       -- Tenant-isolated sessions
│   ├── refresh_tokens (tenant_id) -- Tenant-isolated tokens
│   ├── mfa_factors (tenant_id)    -- Tenant-isolated MFA
│   └── api_keys (tenant_id)       -- Tenant-isolated API keys
│
├── organizations schema (Enterprise Structure)
│   ├── organizations              -- Multi-tenant organizations
│   ├── org_members                -- Organization membership
│   ├── teams                      -- Teams within orgs
│   ├── team_members               -- Team membership
│   └── org_tenants                -- Org to tenant mapping
│
├── metrics schema (Observability - RLS Enabled)
│   ├── metrics (tenant_id)        -- Tenant-isolated metrics
│   ├── log_entries (tenant_id)    -- Tenant-isolated logs
│   └── traces (tenant_id)         -- Tenant-isolated traces
│
└── tenant_<uuid> schemas (Per-Tenant Data)
    └── [Custom tenant tables]
```

### Core Tables

#### tenants.tenants
```sql
CREATE TABLE tenants.tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',

    -- Resource Quotas
    plan_id TEXT DEFAULT 'free',
    max_users INTEGER DEFAULT 5,
    max_storage_gb INTEGER DEFAULT 1,
    max_api_requests_per_month INTEGER DEFAULT 10000,

    -- Metadata
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    suspended_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,

    -- Ownership
    owner_user_id UUID NOT NULL,

    -- Flexible configuration
    settings JSONB DEFAULT '{}'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb
);
```

#### tenants.tenant_domains
```sql
CREATE TABLE tenants.tenant_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants.tenants(id) ON DELETE CASCADE,
    domain TEXT UNIQUE NOT NULL,
    is_primary BOOLEAN DEFAULT false,
    is_verified BOOLEAN DEFAULT false,
    verification_token TEXT,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

#### tenants.tenant_members
```sql
CREATE TABLE tenants.tenant_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants.tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    -- Roles: owner, admin, member, guest

    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    invited_by UUID,

    UNIQUE (tenant_id, user_id)
);
```

#### tenants.tenant_settings
```sql
CREATE TABLE tenants.tenant_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants.tenants(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (tenant_id, key)
);
```

### Row-Level Security Policies

#### Tenant Context Functions

```sql
-- Get current tenant ID from Hasura session variable
CREATE OR REPLACE FUNCTION tenants.current_tenant_id()
RETURNS UUID AS $$
BEGIN
    RETURN current_setting('hasura.user.x-hasura-tenant-id', true)::uuid;
EXCEPTION
    WHEN OTHERS THEN
        RETURN NULL;
END;
$$ LANGUAGE plpgsql STABLE;

-- Get current user ID from session
CREATE OR REPLACE FUNCTION tenants.current_user_id()
RETURNS UUID AS $$
BEGIN
    RETURN current_setting('hasura.user.x-hasura-user-id', true)::uuid;
EXCEPTION
    WHEN OTHERS THEN
        RETURN NULL;
END;
$$ LANGUAGE plpgsql STABLE;

-- Check if user is member of tenant
CREATE OR REPLACE FUNCTION tenants.is_tenant_member(p_tenant_id UUID, p_user_id UUID)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN EXISTS (
        SELECT 1 FROM tenants.tenant_members
        WHERE tenant_id = p_tenant_id
        AND user_id = p_user_id
    );
END;
$$ LANGUAGE plpgsql STABLE;

-- Get user's role in tenant
CREATE OR REPLACE FUNCTION tenants.get_user_tenant_role(p_tenant_id UUID, p_user_id UUID)
RETURNS TEXT AS $$
DECLARE
    v_role TEXT;
BEGIN
    SELECT role INTO v_role
    FROM tenants.tenant_members
    WHERE tenant_id = p_tenant_id
    AND user_id = p_user_id;

    RETURN v_role;
END;
$$ LANGUAGE plpgsql STABLE;
```

#### RLS Policy Examples

**Users can only see tenants they belong to:**
```sql
CREATE POLICY tenant_member_select ON tenants.tenants
    FOR SELECT
    USING (
        id = tenants.current_tenant_id()
        OR
        tenants.is_tenant_member(id, tenants.current_user_id())
    );
```

**Only owners can update tenant:**
```sql
CREATE POLICY tenant_owner_update ON tenants.tenants
    FOR UPDATE
    USING (
        owner_user_id = tenants.current_user_id()
        OR
        tenants.get_user_tenant_role(id, tenants.current_user_id()) = 'owner'
    );
```

**Tenant members can view domains:**
```sql
CREATE POLICY tenant_domains_select ON tenants.tenant_domains
    FOR SELECT
    USING (
        tenants.is_tenant_member(tenant_id, tenants.current_user_id())
    );
```

**Admins/owners can manage domains:**
```sql
CREATE POLICY tenant_domains_manage ON tenants.tenant_domains
    FOR ALL
    USING (
        tenants.get_user_tenant_role(tenant_id, tenants.current_user_id()) IN ('owner', 'admin')
    );
```

### Cross-Tenant Query Prevention

RLS ensures that even with SQL injection or compromised queries, data cannot leak:

```sql
-- This query returns NOTHING if current_tenant_id() != tenant_id
SELECT * FROM auth.users;

-- Even with malicious WHERE clause, RLS policy applies
SELECT * FROM auth.users WHERE 1=1 OR tenant_id != current_tenant_id();
-- Still filtered by: WHERE tenant_id = current_tenant_id()
```

### Quota Enforcement Functions

```sql
-- Check storage quota
CREATE OR REPLACE FUNCTION tenants.check_storage_quota(p_tenant_id UUID)
RETURNS BOOLEAN AS $$
DECLARE
    v_current_size BIGINT;
    v_max_size BIGINT;
BEGIN
    v_current_size := tenants.get_tenant_database_size(p_tenant_id);

    SELECT max_storage_gb * 1073741824 INTO v_max_size
    FROM tenants.tenants
    WHERE id = p_tenant_id;

    RETURN v_current_size < v_max_size;
END;
$$ LANGUAGE plpgsql;

-- Check API request quota
CREATE OR REPLACE FUNCTION tenants.check_api_quota(p_tenant_id UUID)
RETURNS BOOLEAN AS $$
DECLARE
    v_current_count INTEGER;
    v_max_count INTEGER;
BEGIN
    v_current_count := tenants.get_tenant_api_requests(p_tenant_id);

    SELECT max_api_requests_per_month INTO v_max_count
    FROM tenants.tenants
    WHERE id = p_tenant_id;

    RETURN v_current_count < v_max_count;
END;
$$ LANGUAGE plpgsql;
```

### Triggers for Quota Enforcement

```sql
-- Prevent user creation if tenant at limit
CREATE OR REPLACE FUNCTION tenants.check_user_limit()
RETURNS TRIGGER AS $$
DECLARE
    v_tenant_id UUID;
    v_max_users INTEGER;
    v_current_users INTEGER;
BEGIN
    v_tenant_id := NEW.tenant_id;

    SELECT max_users INTO v_max_users
    FROM tenants.tenants
    WHERE id = v_tenant_id;

    SELECT COUNT(*) INTO v_current_users
    FROM auth.users
    WHERE tenant_id = v_tenant_id;

    IF v_current_users >= v_max_users THEN
        RAISE EXCEPTION 'Tenant has reached maximum user limit (%)', v_max_users;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER enforce_user_limit
    BEFORE INSERT ON auth.users
    FOR EACH ROW
    EXECUTE FUNCTION tenants.check_user_limit();
```

---

## CLI Usage

### Initialization

Initialize the multi-tenancy system (run once):

```bash
# Initialize multi-tenancy tables and RLS policies
nself tenant init

# Creates:
# - tenants schema and tables
# - RLS policies on all tenant-aware tables
# - Default tenant for existing data
# - Helper functions and triggers
```

### Tenant Management

#### Create Tenant

```bash
# Basic creation (auto-generates slug)
nself tenant create "Acme Corporation"

# With custom slug and plan
nself tenant create "Acme Corp" --slug acme --plan pro

# With specific owner
nself tenant create "TechCo" --slug techco --owner <user-uuid> --plan enterprise
```

**Output:**
```
✓ Tenant created: acme (ID: 550e8400-e29b-41d4-a716-446655440000)
  Owner: user-uuid-here
  Plan: pro
  URL: https://acme.yourapp.com
```

#### List Tenants

```bash
# Table format
nself tenant list

# JSON format (for scripting)
nself tenant list --json
```

**Output:**
```
ID                                   SLUG        NAME              STATUS    PLAN
─────────────────────────────────────────────────────────────────────────────────
550e8400-e29b-41d4-a716-446655440000 acme        Acme Corporation  active    pro
660f9511-f3ac-52e5-b827-557766551111 techco      TechCo Inc        active    enterprise
770c0622-g4bd-63f6-c938-668877662222 startup     Startup LLC       suspended free
```

#### Show Tenant Details

```bash
# By ID or slug
nself tenant show acme
nself tenant show 550e8400-e29b-41d4-a716-446655440000
```

**Output:**
```
Tenant Details:
  ID: 550e8400-e29b-41d4-a716-446655440000
  Slug: acme
  Name: Acme Corporation
  Status: active
  Plan: pro
  Owner: user-uuid-here
  Members: 15
  Max Users: 50
  Max Storage: 100 GB
  Max API Requests: 100,000/month
  Created: 2025-01-15 10:30:00
```

#### Suspend/Activate Tenant

```bash
# Suspend (disable access)
nself tenant suspend acme

# Activate (re-enable)
nself tenant activate acme
```

#### Delete Tenant

```bash
# Soft delete with confirmation
nself tenant delete acme

# Prompt:
# "Are you sure you want to delete tenant 'acme'? This cannot be undone. (yes/no):"
```

**What happens:**
1. Tenant schema dropped (if using schema-per-tenant)
2. Tenant status set to 'deleted'
3. All related data cascade-deleted
4. Custom domains removed
5. User memberships removed

#### Tenant Statistics

```bash
nself tenant stats
```

**Output:**
```
Tenant Statistics

Total Tenants: 45
Active: 42
Suspended: 2
Deleted: 1

Tenants by Plan:
  free: 20
  pro: 15
  enterprise: 10
```

### Member Management

#### Add Member to Tenant

```bash
# Add as member (default role)
nself tenant member add acme <user-uuid>

# Add with specific role
nself tenant member add acme <user-uuid> admin

# Roles: owner, admin, member, guest
```

#### Remove Member

```bash
nself tenant member remove acme <user-uuid>
```

#### List Members

```bash
nself tenant member list acme
```

**Output:**
```
USER_ID                              ROLE    JOINED_AT
──────────────────────────────────────────────────────
user-uuid-1                          owner   2025-01-15
user-uuid-2                          admin   2025-01-16
user-uuid-3                          member  2025-01-20
```

### Domain Management

#### Add Custom Domain

```bash
nself tenant domain add acme acme.example.com
```

**Output:**
```
✓ Domain added: acme.example.com
  Verification token: a3f5c9e7d2b4a6f8e9c7d5b3a1f4c6e8
  Add this TXT record to your DNS:
    nself-verify=a3f5c9e7d2b4a6f8e9c7d5b3a1f4c6e8
```

#### Verify Domain

After adding DNS TXT record:

```bash
nself tenant domain verify acme acme.example.com
```

**Output:**
```
✓ Domain verified: acme.example.com
  SSL certificate will be generated automatically
```

#### Remove Domain

```bash
nself tenant domain remove acme acme.example.com
```

#### List Domains

```bash
nself tenant domain list acme
```

**Output:**
```
DOMAIN              PRIMARY  VERIFIED  VERIFIED_AT           CREATED_AT
────────────────────────────────────────────────────────────────────────
acme.example.com    true     true      2025-01-20 15:30:00   2025-01-20
api.acme.com        false    true      2025-01-21 09:00:00   2025-01-21
```

### Settings Management

#### Set Setting

```bash
# Simple value
nself tenant setting set acme branding.logo_url "https://cdn.acme.com/logo.png"

# Nested JSON
nself tenant setting set acme features.enable_api true
nself tenant setting set acme limits.custom_quota 500000
```

#### Get Setting

```bash
nself tenant setting get acme branding.logo_url
```

**Output:**
```
"https://cdn.acme.com/logo.png"
```

#### List Settings

```bash
nself tenant setting list acme
```

**Output:**
```
KEY                      VALUE                                    UPDATED_AT
────────────────────────────────────────────────────────────────────────────
branding.logo_url        "https://cdn.acme.com/logo.png"         2025-01-20
branding.primary_color   "#FF6B35"                                2025-01-20
features.enable_api      true                                     2025-01-21
limits.custom_quota      500000                                   2025-01-22
```

---

## Tenant Routing

### Subdomain-Based Routing

**Most common for SaaS applications:**

```
Request: https://acme.yourapp.com/api/users
         ↓
Nginx extracts subdomain: "acme"
         ↓
Lua script queries database:
  SELECT id FROM tenants.tenants WHERE slug = 'acme'
         ↓
Tenant ID added to headers:
  X-Hasura-Tenant-Id: 550e8400-e29b-41d4-a716-446655440000
  X-Tenant-Id: 550e8400-e29b-41d4-a716-446655440000
         ↓
Proxied to Hasura with tenant context
         ↓
PostgreSQL RLS enforces isolation
```

### Custom Domain Routing

**For white-label deployments:**

```
Request: https://acme.example.com/api/users
         ↓
Nginx Lua script queries database:
  SELECT tenant_id FROM tenants.tenant_domains
  WHERE domain = 'acme.example.com' AND is_verified = true
         ↓
Tenant ID resolved and added to headers
         ↓
Proxied with tenant context
```

### JWT-Based Identification

**For API clients:**

```bash
# JWT claims include tenant_id
{
  "sub": "user-uuid",
  "https://hasura.io/jwt/claims": {
    "x-hasura-user-id": "user-uuid",
    "x-hasura-tenant-id": "550e8400-e29b-41d4-a716-446655440000",
    "x-hasura-role": "user"
  }
}
```

Hasura automatically sets PostgreSQL session variables:
```sql
SET hasura.user.x-hasura-tenant-id = '550e8400-e29b-41d4-a716-446655440000';
```

### Nginx Configuration

#### Tenant Routing Config

```nginx
# Map to extract tenant from subdomain
map $host $tenant_slug {
    default "";

    # Pattern: subdomain.base-domain.com → subdomain
    ~^(?<tenant>[^.]+)\..+$ $tenant;
}

# Tenant resolution priority:
# 1. X-Tenant-ID header (direct specification)
# 2. X-Tenant-Slug header
# 3. Custom domain lookup (PostgreSQL)
# 4. Subdomain extraction
```

#### Lua Tenant Resolver

```lua
-- tenant_resolver.lua
local tenant_resolver = {}

function tenant_resolver.resolve()
    local headers = ngx.req.get_headers()
    local host = headers["Host"]

    -- Priority 1: X-Tenant-ID header
    if headers["X-Tenant-ID"] then
        return headers["X-Tenant-ID"]
    end

    -- Priority 2: X-Tenant-Slug header
    if headers["X-Tenant-Slug"] then
        return tenant_resolver.resolve_from_slug(headers["X-Tenant-Slug"])
    end

    -- Priority 3: Custom domain
    local tenant_id = tenant_resolver.resolve_from_domain(host)
    if tenant_id then
        return tenant_id
    end

    -- Priority 4: Subdomain
    local subdomain = host:match("^([^.]+)%.")
    if subdomain then
        return tenant_resolver.resolve_from_slug(subdomain)
    end

    return nil
end

return tenant_resolver
```

#### Nginx Location Config

```nginx
location /v1/ {
    # Resolve tenant using Lua
    set $tenant_id '';
    access_by_lua_block {
        local resolver = require("tenant_resolver")
        local tenant_id = resolver.resolve()
        if tenant_id then
            ngx.var.tenant_id = tenant_id
        end
    }

    # Pass tenant ID to backend
    proxy_set_header X-Hasura-Tenant-Id $tenant_id;
    proxy_set_header X-Tenant-Id $tenant_id;

    # Proxy to Hasura
    proxy_pass http://hasura:8080;
}
```

### SSL Certificate Management

#### Subdomain Wildcard Certificate

```bash
# Development (mkcert)
mkcert "*.yourapp.com"

# Production (Let's Encrypt)
nself ssl letsencrypt --domain "*.yourapp.com"
```

#### Custom Domain Certificates

```bash
# Per-tenant custom domain SSL
nself tenant domain add acme acme.example.com
nself tenant domain verify acme acme.example.com

# Automatically generates SSL certificate via Let's Encrypt
nself ssl letsencrypt --domain acme.example.com --tenant acme
```

---

## Data Isolation

### PostgreSQL RLS Enforcement

All tenant-aware tables have RLS enabled:

```sql
-- Enable RLS on table
ALTER TABLE auth.users ENABLE ROW LEVEL SECURITY;

-- Users can only see users in their tenant
CREATE POLICY user_tenant_isolation ON auth.users
    FOR SELECT
    USING (tenant_id = tenants.current_tenant_id());
```

**Benefits:**
- Enforced at database level (cannot be bypassed)
- Applies to all queries (even raw SQL)
- Works with Hasura GraphQL automatically

**How it works:**
1. User authenticates and gets JWT
2. JWT includes `x-hasura-tenant-id` claim
3. Hasura sets PostgreSQL session variable:
   ```sql
   SET hasura.user.x-hasura-tenant-id = 'tenant-uuid';
   ```
4. All queries filtered by RLS policy:
   ```sql
   SELECT * FROM users;
   -- Automatically becomes:
   SELECT * FROM users WHERE tenant_id = current_setting('hasura.user.x-hasura-tenant-id');
   ```

### Redis Namespace Isolation

```javascript
// Redis key pattern: tenant:{tenant_id}:{key}
const cacheKey = `tenant:${tenantId}:user:${userId}`;
await redis.set(cacheKey, userData);

// Example:
// tenant:550e8400-e29b-41d4-a716-446655440000:user:user-123
```

**Benefits:**
- Prevents cache collisions between tenants
- Easy to flush all cache for specific tenant
- Supports tenant-specific cache policies

**Flushing tenant cache:**
```bash
# Flush all cache for tenant
redis-cli --scan --pattern "tenant:550e8400-*" | xargs redis-cli del
```

### MinIO Bucket Isolation

```javascript
// Bucket naming: tenant-{tenant_id}
const bucketName = `tenant-${tenantId}`;

// Example:
// tenant-550e8400-e29b-41d4-a716-446655440000
```

**Benefits:**
- Complete storage isolation
- Per-tenant storage quotas
- Per-tenant backup/restore

**Bucket policies:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {"AWS": ["arn:aws:iam:::user/tenant-550e8400"]},
      "Action": ["s3:GetObject", "s3:PutObject"],
      "Resource": ["arn:aws:s3:::tenant-550e8400-*/*"]
    }
  ]
}
```

### Log and Metric Isolation

#### Logs
```sql
-- Logs table with tenant_id
ALTER TABLE logs.log_entries ADD COLUMN tenant_id UUID;

-- RLS policy
CREATE POLICY logs_tenant_isolation ON logs.log_entries
    FOR ALL
    USING (tenant_id = tenants.current_tenant_id());
```

#### Metrics
```sql
-- Metrics with tenant context
INSERT INTO metrics.metrics (tenant_id, metric_name, value)
VALUES (current_tenant_id(), 'api.request.count', 1);

-- Query tenant-specific metrics
SELECT * FROM metrics.metrics
WHERE tenant_id = current_tenant_id()
AND metric_name LIKE 'api.request%';
```

#### Tracing
```sql
-- Distributed traces with tenant context
ALTER TABLE tracing.traces ADD COLUMN tenant_id UUID;

-- RLS isolation
CREATE POLICY traces_tenant_isolation ON tracing.traces
    FOR ALL
    USING (tenant_id = tenants.current_tenant_id());
```

---

## Security Considerations

### 1. Preventing Cross-Tenant Data Leaks

#### Database Level
- **Always use RLS** - Never rely on application-level filtering
- **Test RLS policies** - Verify no data leakage with test queries
- **Audit RLS changes** - Track all policy modifications

```sql
-- Test: User in tenant A cannot see tenant B data
SET hasura.user.x-hasura-tenant-id = 'tenant-a-uuid';
SELECT COUNT(*) FROM auth.users WHERE tenant_id = 'tenant-b-uuid';
-- Must return: 0
```

#### Application Level
- **Validate tenant context** - Always verify tenant_id matches user's tenant
- **Avoid hardcoded tenant IDs** - Use session variables
- **Log tenant context** - Include tenant_id in all logs

```javascript
// ❌ BAD: Using tenant_id from request body (can be manipulated)
const tenantId = req.body.tenant_id;

// ✅ GOOD: Using tenant_id from JWT claims (verified)
const tenantId = req.user.tenant_id;
```

### 2. Tenant Impersonation Prevention

#### JWT Security
```javascript
// JWT must include tenant_id in claims
{
  "sub": "user-uuid",
  "https://hasura.io/jwt/claims": {
    "x-hasura-user-id": "user-uuid",
    "x-hasura-tenant-id": "550e8400-e29b-41d4-a716-446655440000",
    "x-hasura-role": "user",
    "x-hasura-allowed-roles": ["user"]
  }
}
```

**Verification:**
- JWT signed with secret key (cannot be forged)
- Tenant ID embedded in claims (cannot be changed)
- Hasura validates JWT before setting session variables

#### Admin Access
```sql
-- Super admin role can switch tenants
CREATE POLICY admin_cross_tenant_access ON tenants.tenants
    FOR SELECT
    USING (
        tenants.is_tenant_member(id, tenants.current_user_id())
        OR
        tenants.current_user_role() = 'super_admin'
    );
```

### 3. Audit Logging per Tenant

```sql
-- Audit log table
CREATE TABLE audit.log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id UUID,
    changes JSONB,
    ip_address INET,
    user_agent TEXT,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Trigger for auditing
CREATE OR REPLACE FUNCTION audit.log_changes()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO audit.log (tenant_id, user_id, action, resource_type, resource_id, changes)
    VALUES (
        tenants.current_tenant_id(),
        tenants.current_user_id(),
        TG_OP,
        TG_TABLE_NAME,
        NEW.id,
        jsonb_build_object('old', row_to_json(OLD), 'new', row_to_json(NEW))
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

### 4. Compliance (GDPR, HIPAA)

#### GDPR: Right to Deletion

```bash
# Delete all data for tenant (schema-per-tenant)
nself tenant delete acme --purge-data

# What happens:
# 1. Drop tenant schema: DROP SCHEMA tenant_550e8400 CASCADE;
# 2. Delete from shared tables: DELETE FROM auth.users WHERE tenant_id = ...
# 3. Delete from storage: aws s3 rm s3://tenant-550e8400 --recursive
# 4. Clear cache: redis-cli --scan --pattern "tenant:550e8400:*" | xargs redis-cli del
```

#### GDPR: Data Export

```bash
# Export all tenant data
nself tenant export acme --output acme-data-export.tar.gz

# Includes:
# - PostgreSQL dump (tenant schema)
# - MinIO objects (tenant bucket)
# - Audit logs (tenant-specific)
# - Settings and metadata
```

#### HIPAA: Encryption at Rest

```sql
-- PostgreSQL encryption
ALTER TABLE tenant_550e8400.patient_records
  SET (encrypted = true);

-- MinIO server-side encryption
mc admin config set minio encryption \
  kms_master_key=tenant-550e8400-key
```

### 5. Rate Limiting per Tenant

```nginx
# Nginx rate limiting by tenant
limit_req_zone $tenant_id zone=tenant_limit:10m rate=100r/s;

location /api/ {
    limit_req zone=tenant_limit burst=50 nodelay;
    proxy_pass http://backend;
}
```

### 6. DDoS Protection per Tenant

```javascript
// Check tenant API quota before processing
async function checkTenantQuota(tenantId) {
  const count = await db.query(`
    SELECT tenants.get_tenant_api_requests($1) as count,
           max_api_requests_per_month as max
    FROM tenants.tenants WHERE id = $1
  `, [tenantId]);

  if (count.count >= count.max) {
    throw new Error('API quota exceeded for this tenant');
  }
}
```

---

## Development Workflow

### Local Multi-Tenant Development

#### 1. Initialize Multi-Tenancy

```bash
# Start infrastructure
nself init
nself build
nself start

# Initialize multi-tenancy
nself tenant init
```

#### 2. Create Test Tenants

```bash
# Create multiple tenants for testing
nself tenant create "Test Tenant A" --slug test-a --plan free
nself tenant create "Test Tenant B" --slug test-b --plan pro
nself tenant create "Test Tenant C" --slug test-c --plan enterprise
```

#### 3. Add Test Users

```bash
# Create users for each tenant
# (Assuming you have user creation endpoint)

# Tenant A user
curl -X POST http://localhost:8080/v1/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query": "mutation { signUp(email: \"usera@testa.com\", password: \"password123\") { id } }"
  }'

# Update user's tenant
psql -d myapp_db -c "
  UPDATE auth.users
  SET tenant_id = (SELECT id FROM tenants.tenants WHERE slug = 'test-a')
  WHERE email = 'usera@testa.com';
"
```

#### 4. Test Tenant Isolation

```bash
# Terminal 1: Tenant A requests
export TENANT_ID=$(psql -t -c "SELECT id FROM tenants.tenants WHERE slug='test-a'")

curl http://localhost:8080/v1/graphql \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ users { id email } }"}'

# Terminal 2: Tenant B requests
export TENANT_ID=$(psql -t -c "SELECT id FROM tenants.tenants WHERE slug='test-b'")

curl http://localhost:8080/v1/graphql \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ users { id email } }"}'
```

#### 5. Test Subdomain Routing

Update `/etc/hosts`:
```
127.0.0.1 test-a.local.nself.org
127.0.0.1 test-b.local.nself.org
127.0.0.1 test-c.local.nself.org
```

Test subdomain access:
```bash
curl https://test-a.local.nself.org/v1/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ users { id email } }"}'
```

### Testing Tenant Isolation

#### Test Script

```bash
#!/bin/bash
# test-tenant-isolation.sh

set -e

echo "Testing tenant data isolation..."

TENANT_A=$(psql -t -c "SELECT id FROM tenants.tenants WHERE slug='test-a'" | tr -d ' ')
TENANT_B=$(psql -t -c "SELECT id FROM tenants.tenants WHERE slug='test-b'" | tr -d ' ')

# Test 1: Tenant A cannot see Tenant B users
echo "Test 1: Cross-tenant user visibility"
COUNT=$(psql -t -c "
  SET hasura.user.x-hasura-tenant-id = '$TENANT_A';
  SELECT COUNT(*) FROM auth.users WHERE tenant_id = '$TENANT_B';
" | tr -d ' ')

if [ "$COUNT" -eq "0" ]; then
  echo "✓ PASS: Tenant A cannot see Tenant B users"
else
  echo "✗ FAIL: Tenant isolation breach! Tenant A can see $COUNT users from Tenant B"
  exit 1
fi

# Test 2: Tenant A can see own users
echo "Test 2: Own tenant user visibility"
COUNT=$(psql -t -c "
  SET hasura.user.x-hasura-tenant-id = '$TENANT_A';
  SELECT COUNT(*) FROM auth.users WHERE tenant_id = '$TENANT_A';
" | tr -d ' ')

if [ "$COUNT" -gt "0" ]; then
  echo "✓ PASS: Tenant A can see own users ($COUNT)"
else
  echo "✗ FAIL: Tenant cannot see own users"
  exit 1
fi

echo "✓ All tenant isolation tests passed"
```

### Debugging Tenant-Specific Issues

#### Check Current Tenant Context

```sql
-- In PostgreSQL session
SELECT tenants.current_tenant_id();
SELECT tenants.current_user_id();
```

#### View Tenant Members

```sql
SELECT
  u.email,
  tm.role,
  tm.joined_at
FROM tenants.tenant_members tm
JOIN auth.users u ON tm.user_id = u.id
WHERE tm.tenant_id = 'your-tenant-uuid';
```

#### Check RLS Policies

```sql
-- View all RLS policies on a table
SELECT
  schemaname,
  tablename,
  policyname,
  permissive,
  roles,
  cmd,
  qual
FROM pg_policies
WHERE tablename = 'users';
```

#### Test Query with Different Tenant Context

```sql
-- Test as Tenant A
SET hasura.user.x-hasura-tenant-id = 'tenant-a-uuid';
SELECT * FROM auth.users; -- Should only see Tenant A users

-- Test as Tenant B
SET hasura.user.x-hasura-tenant-id = 'tenant-b-uuid';
SELECT * FROM auth.users; -- Should only see Tenant B users
```

---

## Production Deployment

### Multi-Tenant Production Setup

#### 1. Environment Configuration

```bash
# .env.prod
ENV=prod
BASE_DOMAIN=yourapp.com

# Enable multi-tenancy
MULTI_TENANCY_ENABLED=true

# Tenant quotas (default for new tenants)
TENANT_DEFAULT_MAX_USERS=10
TENANT_DEFAULT_MAX_STORAGE_GB=5
TENANT_DEFAULT_MAX_API_REQUESTS=50000

# Tenant routing
TENANT_ROUTING_METHOD=subdomain  # subdomain | custom_domain | both
TENANT_SUBDOMAIN_WILDCARD=true
TENANT_REQUIRE_DOMAIN_VERIFICATION=true

# SSL
SSL_PROVIDER=letsencrypt
LETSENCRYPT_EMAIL=admin@yourapp.com
```

#### 2. DNS Configuration

**Wildcard subdomain for tenants:**
```
# DNS Records
*.yourapp.com  A  203.0.113.10  (your server IP)
yourapp.com    A  203.0.113.10
```

**Custom domain CNAME:**
```
# Tenant's DNS (for custom domains)
acme.example.com  CNAME  tenant-proxy.yourapp.com
```

#### 3. SSL Certificate Setup

```bash
# Wildcard certificate for all subdomains
nself ssl letsencrypt --domain "*.yourapp.com" --domain "yourapp.com"

# Auto-renewal cron job
crontab -e
# Add: 0 0 * * * /usr/local/bin/nself ssl renew
```

#### 4. PostgreSQL Optimization

```sql
-- Connection pooling per tenant
ALTER SYSTEM SET max_connections = 500;
ALTER SYSTEM SET shared_buffers = '2GB';

-- Optimize for RLS queries
ALTER SYSTEM SET enable_partitionwise_join = on;
CREATE INDEX CONCURRENTLY idx_users_tenant_id ON auth.users(tenant_id);
CREATE INDEX CONCURRENTLY idx_sessions_tenant_id ON auth.sessions(tenant_id);

-- Statistics for query optimization
ALTER TABLE auth.users ALTER COLUMN tenant_id SET STATISTICS 1000;
ANALYZE auth.users;
```

### Scaling Considerations

#### Database Scaling

**Vertical Scaling:**
```bash
# Increase PostgreSQL resources
docker-compose.yml:
  postgres:
    deploy:
      resources:
        limits:
          cpus: '4'
          memory: 8G
        reservations:
          cpus: '2'
          memory: 4G
```

**Connection Pooling:**
```bash
# Enable PgBouncer for connection pooling
PGBOUNCER_ENABLED=true
PGBOUNCER_POOL_MODE=transaction
PGBOUNCER_MAX_CLIENT_CONN=10000
PGBOUNCER_DEFAULT_POOL_SIZE=25
```

**Read Replicas:**
```bash
# Route read queries to replicas
POSTGRES_READ_REPLICA_1=replica1.yourapp.com:5432
POSTGRES_READ_REPLICA_2=replica2.yourapp.com:5432

# Hasura configuration
HASURA_GRAPHQL_READ_REPLICA_URLS=postgresql://replica1,postgresql://replica2
```

#### Application Scaling

**Horizontal Scaling:**
```yaml
# docker-compose.prod.yml
services:
  hasura:
    deploy:
      replicas: 5
      resources:
        limits:
          cpus: '2'
          memory: 4G
```

**Load Balancing:**
```nginx
upstream hasura_backend {
    least_conn;
    server hasura-1:8080;
    server hasura-2:8080;
    server hasura-3:8080;
    server hasura-4:8080;
    server hasura-5:8080;
}

server {
    location /v1/ {
        proxy_pass http://hasura_backend;
    }
}
```

#### Caching Strategy

```javascript
// Redis cache with tenant namespace
const cacheKey = `tenant:${tenantId}:query:${queryHash}`;

// Cache tenant metadata (rarely changes)
const tenantData = await cache.get(`tenant:${tenantId}:metadata`);
if (!tenantData) {
  tenantData = await db.getTenant(tenantId);
  await cache.set(`tenant:${tenantId}:metadata`, tenantData, 3600); // 1 hour TTL
}

// Cache per-tenant API quotas
const quotaKey = `tenant:${tenantId}:quota:api:${month}`;
await cache.incr(quotaKey);
await cache.expire(quotaKey, 2592000); // 30 days
```

### Performance Optimization

#### Index Strategy

```sql
-- Critical indexes for multi-tenant queries
CREATE INDEX CONCURRENTLY idx_users_tenant_email
  ON auth.users(tenant_id, email);

CREATE INDEX CONCURRENTLY idx_sessions_tenant_user
  ON auth.sessions(tenant_id, user_id);

CREATE INDEX CONCURRENTLY idx_tenant_members_lookup
  ON tenants.tenant_members(user_id, tenant_id);

-- Partial indexes for active tenants
CREATE INDEX CONCURRENTLY idx_active_tenants
  ON tenants.tenants(id) WHERE status = 'active';
```

#### Query Optimization

```sql
-- Use tenant_id in all WHERE clauses
-- ❌ Slow (scans all rows)
SELECT * FROM users WHERE email = 'user@example.com';

-- ✅ Fast (uses tenant + email index)
SELECT * FROM users
WHERE tenant_id = current_tenant_id()
AND email = 'user@example.com';
```

#### Materialized Views

```sql
-- Pre-aggregate tenant statistics
CREATE MATERIALIZED VIEW tenants.tenant_stats AS
SELECT
  t.id,
  t.slug,
  COUNT(DISTINCT u.id) as user_count,
  COUNT(DISTINCT s.id) as session_count,
  SUM(pg_total_relation_size(quote_ident('tenant_' || replace(t.id::text, '-', '_')))) as storage_bytes
FROM tenants.tenants t
LEFT JOIN auth.users u ON u.tenant_id = t.id
LEFT JOIN auth.sessions s ON s.tenant_id = t.id
WHERE t.status = 'active'
GROUP BY t.id;

-- Refresh hourly
CREATE INDEX ON tenants.tenant_stats(id);
REFRESH MATERIALIZED VIEW CONCURRENTLY tenants.tenant_stats;
```

### Monitoring Tenant Health

#### Metrics to Track

```sql
-- Tenant health dashboard query
SELECT
  t.slug,
  t.status,
  t.plan_id,
  COUNT(DISTINCT u.id) as users,
  t.max_users,
  COUNT(DISTINCT s.id) as active_sessions,
  tenants.get_tenant_database_size(t.id) / 1073741824 as storage_gb,
  t.max_storage_gb,
  tenants.get_tenant_api_requests(t.id) as api_requests_this_month,
  t.max_api_requests_per_month
FROM tenants.tenants t
LEFT JOIN auth.users u ON u.tenant_id = t.id
LEFT JOIN auth.sessions s ON s.tenant_id = t.id
  AND s.created_at > NOW() - INTERVAL '1 hour'
WHERE t.status = 'active'
GROUP BY t.id;
```

#### Alerting Rules

```yaml
# Prometheus alerting rules
groups:
  - name: tenant_quotas
    rules:
      # Alert when tenant near user limit
      - alert: TenantNearUserLimit
        expr: tenant_user_count / tenant_max_users > 0.9
        for: 5m
        annotations:
          summary: "Tenant {{ $labels.tenant_slug }} near user limit"

      # Alert when tenant near storage limit
      - alert: TenantNearStorageLimit
        expr: tenant_storage_gb / tenant_max_storage_gb > 0.9
        for: 10m
        annotations:
          summary: "Tenant {{ $labels.tenant_slug }} near storage limit"

      # Alert when tenant exceeds API quota
      - alert: TenantExceededAPIQuota
        expr: tenant_api_requests > tenant_max_api_requests
        annotations:
          summary: "Tenant {{ $labels.tenant_slug }} exceeded API quota"
```

---

## Migration Guide

### Converting Single-Tenant to Multi-Tenant

#### Phase 1: Preparation

1. **Backup existing data:**
```bash
nself db backup --output pre-migration-backup.sql
```

2. **Review current schema:**
```bash
# List all tables that need tenant_id column
psql -d myapp_db -c "
  SELECT table_schema, table_name
  FROM information_schema.tables
  WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
  AND table_type = 'BASE TABLE';
"
```

3. **Plan tenant structure:**
```
Existing users:
├── All existing users → Default tenant
├── Segment by organization → Multiple tenants
└── Manual assignment → Custom tenants
```

#### Phase 2: Initialize Multi-Tenancy

```bash
# Run multi-tenancy migrations
nself tenant init

# Creates:
# - tenants schema
# - Default tenant
# - Adds tenant_id to core tables
# - Enables RLS
```

#### Phase 3: Migrate Existing Data

**Strategy 1: Single Default Tenant (Simple)**
```sql
-- All existing users go to default tenant
UPDATE auth.users
SET tenant_id = (SELECT id FROM tenants.tenants WHERE slug = 'default')
WHERE tenant_id IS NULL;
```

**Strategy 2: Organization-Based (Advanced)**
```sql
-- Create tenants from existing organizations
INSERT INTO tenants.tenants (slug, name, owner_user_id, plan_id)
SELECT
  lower(replace(org_name, ' ', '-')),
  org_name,
  org_owner_id,
  'pro'
FROM legacy_organizations;

-- Assign users to tenants based on organization
UPDATE auth.users u
SET tenant_id = t.id
FROM legacy_user_organizations luo
JOIN tenants.tenants t ON t.slug = lower(replace(luo.org_name, ' ', '-'))
WHERE u.id = luo.user_id;
```

#### Phase 4: Add tenant_id to Custom Tables

```sql
-- Add tenant_id column to your tables
ALTER TABLE products ADD COLUMN tenant_id UUID;
ALTER TABLE orders ADD COLUMN tenant_id UUID;
ALTER TABLE invoices ADD COLUMN tenant_id UUID;

-- Backfill tenant_id based on user ownership
UPDATE products p
SET tenant_id = u.tenant_id
FROM auth.users u
WHERE p.created_by_user_id = u.id;

-- Make tenant_id NOT NULL after backfill
ALTER TABLE products ALTER COLUMN tenant_id SET NOT NULL;

-- Add foreign key constraint
ALTER TABLE products
  ADD CONSTRAINT fk_products_tenant
  FOREIGN KEY (tenant_id) REFERENCES tenants.tenants(id) ON DELETE CASCADE;

-- Add index
CREATE INDEX idx_products_tenant ON products(tenant_id);
```

#### Phase 5: Enable RLS on Custom Tables

```sql
-- Enable RLS
ALTER TABLE products ENABLE ROW LEVEL SECURITY;

-- Create isolation policy
CREATE POLICY products_tenant_isolation ON products
  FOR ALL
  USING (tenant_id = tenants.current_tenant_id());

-- Grant access to Hasura
GRANT SELECT, INSERT, UPDATE, DELETE ON products TO hasura;
```

#### Phase 6: Update Application Code

**Before (single-tenant):**
```javascript
// Query all users
const users = await db.query('SELECT * FROM users');
```

**After (multi-tenant):**
```javascript
// Query only tenant's users (RLS enforces this automatically)
const users = await db.query('SELECT * FROM users');
// RLS adds: WHERE tenant_id = current_tenant_id()

// Or explicitly pass tenant context
const users = await db.query('SELECT * FROM users WHERE tenant_id = $1', [tenantId]);
```

#### Phase 7: Testing

```bash
# Test tenant isolation
./test-tenant-isolation.sh

# Verify no data leakage
psql -c "
  SET hasura.user.x-hasura-tenant-id = 'tenant-a-uuid';
  SELECT COUNT(*) FROM products WHERE tenant_id = 'tenant-b-uuid';
  -- Should return: 0
"
```

#### Phase 8: Gradual Rollout

1. **Enable multi-tenancy flag:**
```bash
# .env
MULTI_TENANCY_ENABLED=true
```

2. **Test with subset of users:**
```sql
-- Create pilot tenants
nself tenant create "Pilot Tenant 1" --slug pilot1
nself tenant create "Pilot Tenant 2" --slug pilot2

-- Migrate pilot users
UPDATE auth.users
SET tenant_id = (SELECT id FROM tenants.tenants WHERE slug = 'pilot1')
WHERE email IN ('user1@example.com', 'user2@example.com');
```

3. **Monitor and iterate:**
```bash
# Watch tenant metrics
nself tenant stats --watch

# Check for errors
nself logs | grep -i "tenant\|rls"
```

4. **Full rollout:**
```bash
# Migrate all remaining users
UPDATE auth.users
SET tenant_id = (SELECT id FROM tenants.tenants WHERE slug = 'default')
WHERE tenant_id IS NULL;
```

### Adding Multi-Tenancy to Existing App

If your app is already running and you want to add multi-tenancy:

#### Minimal Disruption Approach

1. **Add tenant_id columns with defaults:**
```sql
-- Add nullable tenant_id
ALTER TABLE users ADD COLUMN tenant_id UUID;

-- Create default tenant
INSERT INTO tenants.tenants (slug, name, owner_user_id, plan_id)
VALUES ('default', 'Default Tenant', 'admin-user-uuid', 'enterprise');

-- Set default for new rows
ALTER TABLE users
  ALTER COLUMN tenant_id
  SET DEFAULT (SELECT id FROM tenants.tenants WHERE slug = 'default');

-- Backfill existing rows
UPDATE users
SET tenant_id = (SELECT id FROM tenants.tenants WHERE slug = 'default')
WHERE tenant_id IS NULL;

-- Make NOT NULL after backfill
ALTER TABLE users ALTER COLUMN tenant_id SET NOT NULL;
```

2. **Enable RLS gradually:**
```sql
-- Enable RLS but create permissive policy initially
ALTER TABLE users ENABLE ROW LEVEL SECURITY;

-- Allow all access initially (no disruption)
CREATE POLICY users_allow_all ON users FOR ALL USING (true);

-- Later, switch to tenant isolation
DROP POLICY users_allow_all ON users;
CREATE POLICY users_tenant_isolation ON users
  FOR ALL
  USING (tenant_id = tenants.current_tenant_id());
```

3. **Update application gradually:**
```javascript
// Phase 1: Pass tenant_id explicitly everywhere
function getUsers(tenantId) {
  return db.query('SELECT * FROM users WHERE tenant_id = $1', [tenantId]);
}

// Phase 2: Rely on RLS (remove explicit filters)
function getUsers() {
  return db.query('SELECT * FROM users');
  // RLS automatically filters by tenant
}
```

---

## Advanced Topics

### Multi-Organization Tenancy

For enterprise customers with multiple organizations:

```
Enterprise Customer
├── Organization A (Tenant A)
│   ├── Team 1
│   ├── Team 2
│   └── Users: 50
├── Organization B (Tenant B)
│   ├── Team 1
│   └── Users: 30
└── Organization C (Tenant C)
    └── Users: 20
```

#### Schema

```sql
-- Organizations can have multiple tenants
CREATE TABLE organizations.org_tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations.organizations(id),
    tenant_id UUID NOT NULL REFERENCES tenants.tenants(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, tenant_id)
);

-- Users can belong to multiple organizations
CREATE TABLE organizations.org_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations.organizations(id),
    user_id UUID NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    UNIQUE (org_id, user_id)
);
```

#### Cross-Organization Queries

```sql
-- Get all tenants for an organization
SELECT t.*
FROM tenants.tenants t
JOIN organizations.org_tenants ot ON t.id = ot.tenant_id
WHERE ot.org_id = 'org-uuid';

-- Get all users across organization's tenants
SELECT u.*
FROM auth.users u
JOIN tenants.tenants t ON u.tenant_id = t.id
JOIN organizations.org_tenants ot ON t.id = ot.tenant_id
WHERE ot.org_id = 'org-uuid';
```

### Tenant Data Export/Import

#### Export Tenant Data

```bash
#!/bin/bash
# export-tenant.sh

TENANT_ID=$1
TENANT_SLUG=$(psql -t -c "SELECT slug FROM tenants.tenants WHERE id='$TENANT_ID'" | tr -d ' ')
OUTPUT_DIR="./tenant-exports/$TENANT_SLUG"

mkdir -p "$OUTPUT_DIR"

# Export tenant schema
pg_dump -d myapp_db -n "tenant_$(echo $TENANT_ID | tr -d '-')" > "$OUTPUT_DIR/schema.sql"

# Export tenant rows from shared tables
psql -d myapp_db -c "
  COPY (SELECT * FROM auth.users WHERE tenant_id='$TENANT_ID')
  TO STDOUT CSV HEADER
" > "$OUTPUT_DIR/users.csv"

# Export tenant settings
psql -d myapp_db -c "
  COPY (SELECT * FROM tenants.tenant_settings WHERE tenant_id='$TENANT_ID')
  TO STDOUT CSV HEADER
" > "$OUTPUT_DIR/settings.csv"

# Export MinIO bucket
mc mirror minio/tenant-$TENANT_ID "$OUTPUT_DIR/storage/"

# Create archive
tar -czf "$TENANT_SLUG-export-$(date +%Y%m%d).tar.gz" -C "$OUTPUT_DIR" .

echo "✓ Tenant data exported to $TENANT_SLUG-export-$(date +%Y%m%d).tar.gz"
```

#### Import Tenant Data

```bash
#!/bin/bash
# import-tenant.sh

ARCHIVE=$1
TENANT_SLUG=$2

# Extract archive
TEMP_DIR=$(mktemp -d)
tar -xzf "$ARCHIVE" -C "$TEMP_DIR"

# Create new tenant
TENANT_ID=$(nself tenant create "$TENANT_SLUG" --json | jq -r '.id')

# Import schema
psql -d myapp_db < "$TEMP_DIR/schema.sql"

# Import users (update tenant_id)
psql -d myapp_db -c "
  COPY auth.users FROM STDIN CSV HEADER;
" < "$TEMP_DIR/users.csv"

# Update tenant_id references
psql -d myapp_db -c "
  UPDATE auth.users SET tenant_id='$TENANT_ID'
  WHERE tenant_id=(SELECT tenant_id FROM auth.users LIMIT 1);
"

# Import storage
mc mirror "$TEMP_DIR/storage/" minio/tenant-$TENANT_ID

echo "✓ Tenant imported as $TENANT_SLUG (ID: $TENANT_ID)"
```

### Tenant-Specific Customization

#### Custom Business Logic per Tenant

```javascript
// Tenant-specific configuration
const tenantConfig = await db.query(`
  SELECT settings FROM tenants.tenants WHERE id = $1
`, [tenantId]);

const features = tenantConfig.settings.features || {};

// Feature flags
if (features.enable_custom_workflow) {
  await executeCustomWorkflow();
} else {
  await executeDefaultWorkflow();
}

// Tenant-specific integrations
if (tenantConfig.settings.integrations?.slack?.enabled) {
  await notifySlack(tenantConfig.settings.integrations.slack.webhook_url);
}
```

#### Custom GraphQL Schema per Tenant

```javascript
// Hasura remote schema per tenant
const remoteSchemas = {
  'tenant-a': 'https://tenant-a.api.example.com/graphql',
  'tenant-b': 'https://tenant-b.api.example.com/graphql',
};

// Add remote schema dynamically
const schemaUrl = remoteSchemas[tenantSlug];
if (schemaUrl) {
  await hasura.addRemoteSchema({
    name: `tenant_${tenantSlug}`,
    url: schemaUrl,
  });
}
```

### Performance Benchmarks

#### RLS Overhead

```
Query: SELECT * FROM users WHERE id = 'user-uuid'

Without RLS: 0.8ms
With RLS:    1.2ms
Overhead:    +50% (acceptable for security)

Query: SELECT * FROM users WHERE tenant_id = 'tenant-uuid' LIMIT 100

Without RLS: 15ms
With RLS:    18ms
Overhead:    +20% (with proper indexes)
```

**Optimization:**
```sql
-- Add composite indexes
CREATE INDEX idx_users_tenant_id ON users(tenant_id, id);

-- Analyze frequently
ANALYZE users;

-- Increase statistics target
ALTER TABLE users ALTER COLUMN tenant_id SET STATISTICS 1000;
```

### Troubleshooting

#### Issue: RLS Policy Not Working

**Symptom:** Users can see data from other tenants

**Debug:**
```sql
-- Check if RLS is enabled
SELECT schemaname, tablename, rowsecurity
FROM pg_tables
WHERE tablename = 'users';

-- Check policies
SELECT * FROM pg_policies WHERE tablename = 'users';

-- Test policy
SET hasura.user.x-hasura-tenant-id = 'tenant-a-uuid';
SELECT * FROM users WHERE tenant_id = 'tenant-b-uuid';
-- Should return: 0 rows
```

**Fix:**
```sql
-- Enable RLS if disabled
ALTER TABLE users ENABLE ROW LEVEL SECURITY;

-- Recreate policy if missing
DROP POLICY IF EXISTS users_tenant_isolation ON users;
CREATE POLICY users_tenant_isolation ON users
  FOR ALL
  USING (tenant_id = tenants.current_tenant_id());
```

#### Issue: Slow Queries with Many Tenants

**Symptom:** Queries slow when tenant count > 1000

**Debug:**
```sql
EXPLAIN ANALYZE SELECT * FROM users WHERE tenant_id = current_tenant_id();
```

**Fix:**
```sql
-- Add index on tenant_id
CREATE INDEX CONCURRENTLY idx_users_tenant ON users(tenant_id);

-- Use partial index for active tenants
CREATE INDEX CONCURRENTLY idx_active_tenant_users
  ON users(tenant_id, id)
  WHERE tenant_id IN (SELECT id FROM tenants.tenants WHERE status = 'active');

-- Increase work_mem for large result sets
SET work_mem = '256MB';
```

#### Issue: Tenant Context Not Set

**Symptom:** `tenants.current_tenant_id()` returns NULL

**Debug:**
```sql
-- Check session variable
SHOW hasura.user.x-hasura-tenant-id;
```

**Fix:**
1. Verify JWT includes tenant_id claim
2. Check Hasura JWT configuration
3. Ensure nginx passes X-Tenant-ID header

---

## Summary

nself's multi-tenancy system provides:

✅ **Complete data isolation** via PostgreSQL RLS
✅ **Flexible tenant identification** (subdomain, custom domain, JWT, header)
✅ **Resource quotas** (users, storage, API requests)
✅ **Custom domains** with SSL support
✅ **Organization hierarchy** for enterprise use cases
✅ **Audit logging** per tenant
✅ **GDPR compliance** with per-tenant data deletion
✅ **Production-ready** with monitoring and scaling support

**Next Steps:**

1. Initialize multi-tenancy: `nself tenant init`
2. Create your first tenant: `nself tenant create "My Tenant"`
3. Configure routing (subdomain or custom domain)
4. Test tenant isolation thoroughly
5. Deploy to production with monitoring

For more information:
- [Database Architecture](ARCHITECTURE.md)
- [Security Best Practices](../guides/SECURITY.md)
- [Production Deployment](../deployment/PRODUCTION-DEPLOYMENT.md)
- [API Documentation](API.md)

---

**Version:** nself v0.8.0
**Last Updated:** January 2026
**Status:** Production Ready
