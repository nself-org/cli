# nself tenant

**Category**: Multi-Tenancy Commands

Manage multi-tenant applications, organizations, billing, and subscriptions.

## Overview

All multi-tenancy operations use `nself tenant <subcommand>` for managing tenants, organizations, billing, subscriptions, and tenant-specific configurations.

**Features**:
- ✅ Multi-tenant database isolation
- ✅ Organization management
- ✅ Subscription & billing
- ✅ Tenant-specific configuration
- ✅ Usage tracking & quotas
- ✅ Custom domains per tenant

## Command Categories

### Tenant Management (10 subcommands)
- create, delete, update, list, activate, deactivate, migrate, export, import, clone

### Organization Management (15 subcommands)
- org create, delete, update, list, members, invite, remove, roles, transfer, merge, suspend, reactivate

### Billing & Subscriptions (15 subcommands)
- billing setup, plans, subscribe, cancel, upgrade, downgrade, invoice, payment-method, usage, quota, trial, coupon

### Domain Management (5 subcommands)
- domain add, remove, verify, list, ssl

### Data Isolation (5 subcommands)
- isolate, schema, migrate-tenant, backup-tenant, restore-tenant

## Subcommands Reference

### Tenant Management

| Subcommand | Description | Use Case |
|------------|-------------|----------|
| `create` | Create new tenant | Onboard customer |
| `delete` | Delete tenant | Off-board customer |
| `update` | Update tenant | Change settings |
| `list` | List all tenants | Admin overview |
| `activate` | Activate tenant | Enable tenant |
| `deactivate` | Deactivate tenant | Temporary disable |
| `migrate` | Migrate tenant data | Move between regions |
| `export` | Export tenant data | Backup or transfer |
| `import` | Import tenant data | Restore or onboard |
| `clone` | Clone tenant | Create test copy |

### Organization Management

| Subcommand | Description |
|------------|-------------|
| `org create` | Create organization |
| `org delete` | Delete organization |
| `org update` | Update organization |
| `org list` | List organizations |
| `org members` | List organization members |
| `org invite` | Invite member |
| `org remove` | Remove member |
| `org roles` | Manage member roles |
| `org transfer` | Transfer ownership |
| `org merge` | Merge organizations |
| `org suspend` | Suspend organization |
| `org reactivate` | Reactivate organization |
| `org settings` | Organization settings |
| `org quota` | Set organization quotas |
| `org usage` | View usage statistics |

### Billing & Subscriptions

| Subcommand | Description |
|------------|-------------|
| `billing setup` | Setup billing |
| `billing plans` | List pricing plans |
| `billing subscribe` | Subscribe to plan |
| `billing cancel` | Cancel subscription |
| `billing upgrade` | Upgrade plan |
| `billing downgrade` | Downgrade plan |
| `billing invoice` | Generate invoice |
| `billing payment-method` | Manage payment methods |
| `billing usage` | View usage |
| `billing quota` | View/set quotas |
| `billing trial` | Start/end trial |
| `billing coupon` | Apply coupon |
| `billing history` | Billing history |
| `billing export` | Export billing data |
| `billing webhook` | Billing webhooks |

## Quick Start

### Create Tenant

```bash
nself tenant create --name "Acme Corp" --email admin@acme.com
```

**Output**:
```
Creating tenant...
✓ Tenant created successfully

Name: Acme Corp
ID: tenant_abc123
Email: admin@acme.com
Status: active
Created: 2026-02-13 14:30:00

Database schema: acme_corp
Subdomain: acme-corp.app.com
Admin URL: https://acme-corp.app.com/admin
```

### Create Organization

```bash
nself tenant org create --name "Engineering Team" --tenant tenant_abc123
```

### Setup Billing

```bash
nself tenant billing setup --tenant tenant_abc123 \
  --provider stripe \
  --plan pro-monthly
```

### Add Custom Domain

```bash
nself tenant domain add tenant_abc123 app.acme.com
```

## Tenant Management

### nself tenant create

Create new tenant with isolated environment.

**Usage**:
```bash
nself tenant create [OPTIONS]
```

**Options**:
- `--name NAME` - Tenant name (required)
- `--email EMAIL` - Admin email (required)
- `--plan PLAN` - Subscription plan
- `--trial` - Start with trial period
- `--domain DOMAIN` - Custom domain
- `--schema SCHEMA` - Database schema name
- `--template TEMPLATE` - Use tenant template

**Isolation Strategy**:
- **Schema** (default) - Separate PostgreSQL schema per tenant
- **Database** - Separate database per tenant
- **Cluster** - Separate cluster per tenant (enterprise)

**Examples**:
```bash
# Basic tenant
nself tenant create --name "Acme Corp" --email admin@acme.com

# With trial period
nself tenant create --name "Acme Corp" --email admin@acme.com --trial --plan pro

# With custom domain
nself tenant create --name "Acme Corp" --email admin@acme.com --domain app.acme.com

# From template
nself tenant create --name "Acme Corp" --email admin@acme.com --template saas-starter
```

### nself tenant list

List all tenants with filtering.

**Usage**:
```bash
nself tenant list [OPTIONS]
```

**Options**:
- `--status STATUS` - Filter by status (active/inactive/trial)
- `--plan PLAN` - Filter by plan
- `--format FORMAT` - Output format (table/json/csv)
- `--limit N` - Limit results

**Examples**:
```bash
# List all tenants
nself tenant list

# Active tenants only
nself tenant list --status active

# Export to CSV
nself tenant list --format csv > tenants.csv
```

### nself tenant delete

Delete tenant and all associated data.

**Usage**:
```bash
nself tenant delete <tenant_id> [OPTIONS]
```

**Options**:
- `--backup` - Create backup before deletion
- `--force` - Skip confirmation
- `--keep-data DAYS` - Soft delete with retention period

**Examples**:
```bash
# Delete with backup
nself tenant delete tenant_abc123 --backup

# Soft delete (30 day retention)
nself tenant delete tenant_abc123 --keep-data 30
```

**Warning**: This permanently deletes:
- Tenant database schema
- User data
- File uploads
- Billing history

## Organization Management

### nself tenant org create

Create organization within tenant.

**Usage**:
```bash
nself tenant org create [OPTIONS]
```

**Options**:
- `--name NAME` - Organization name
- `--tenant TENANT_ID` - Parent tenant
- `--owner EMAIL` - Organization owner
- `--members EMAILS` - Initial members (comma-separated)

**Examples**:
```bash
# Create organization
nself tenant org create --name "Engineering" --tenant tenant_abc123

# With initial members
nself tenant org create --name "Engineering" \
  --tenant tenant_abc123 \
  --members "user1@acme.com,user2@acme.com"
```

### nself tenant org members

Manage organization members.

**Usage**:
```bash
nself tenant org members <action> <org_id> [OPTIONS]
```

**Actions**:
- `list` - List members
- `invite` - Invite new member
- `remove` - Remove member
- `update-role` - Update member role

**Examples**:
```bash
# List members
nself tenant org members list org_123

# Invite member
nself tenant org invite org_123 user@example.com --role member

# Update role
nself tenant org update-role org_123 user@example.com admin

# Remove member
nself tenant org remove org_123 user@example.com
```

### Organization Roles

Default organization roles:

- **owner** - Full control, billing access
- **admin** - Administrative access (no billing)
- **member** - Standard member access
- **viewer** - Read-only access

## Billing & Subscriptions

### nself tenant billing setup

Setup billing for tenant.

**Usage**:
```bash
nself tenant billing setup <tenant_id> [OPTIONS]
```

**Options**:
- `--provider PROVIDER` - Billing provider (stripe/paddle/chargebee)
- `--plan PLAN` - Initial plan
- `--payment-method TOKEN` - Payment method token

**Supported Providers**:
- **Stripe** (recommended)
- **Paddle**
- **Chargebee**
- **Recurly**

**Examples**:
```bash
# Setup with Stripe
nself tenant billing setup tenant_abc123 \
  --provider stripe \
  --plan pro-monthly

# With payment method
nself tenant billing setup tenant_abc123 \
  --provider stripe \
  --plan pro-monthly \
  --payment-method pm_abc123
```

### nself tenant billing plans

Manage pricing plans.

**Usage**:
```bash
nself tenant billing plans <action> [OPTIONS]
```

**Actions**:
- `list` - List available plans
- `create` - Create new plan
- `update` - Update plan
- `archive` - Archive plan

**Plan Structure**:
```json
{
  "id": "pro-monthly",
  "name": "Pro (Monthly)",
  "price": 49,
  "currency": "USD",
  "interval": "month",
  "features": {
    "users": 10,
    "storage": "100GB",
    "api_calls": 100000
  }
}
```

**Examples**:
```bash
# List plans
nself tenant billing plans list

# Create plan
nself tenant billing plans create \
  --id enterprise \
  --name "Enterprise" \
  --price 499 \
  --interval month
```

### nself tenant billing subscribe

Subscribe tenant to plan.

**Usage**:
```bash
nself tenant billing subscribe <tenant_id> <plan_id> [OPTIONS]
```

**Options**:
- `--trial-days N` - Trial period in days
- `--coupon CODE` - Apply coupon code
- `--prorate` - Prorate charges

**Examples**:
```bash
# Subscribe to plan
nself tenant billing subscribe tenant_abc123 pro-monthly

# With trial
nself tenant billing subscribe tenant_abc123 pro-monthly --trial-days 14

# With coupon
nself tenant billing subscribe tenant_abc123 pro-monthly --coupon SAVE20
```

### nself tenant billing usage

Track usage and quotas.

**Usage**:
```bash
nself tenant billing usage <tenant_id> [OPTIONS]
```

**Tracked Metrics**:
- Active users
- Storage used
- API calls
- Database queries
- Bandwidth

**Examples**:
```bash
# Current usage
nself tenant billing usage tenant_abc123

# Usage history
nself tenant billing usage tenant_abc123 --since "2026-01-01"

# Export usage report
nself tenant billing usage tenant_abc123 --export usage-report.csv
```

**Output**:
```
Usage Report - Acme Corp (tenant_abc123)

Plan: Pro (Monthly)
Period: 2026-02-01 to 2026-02-28

Metric          Used        Quota       % Used
────────────────────────────────────────────────
Users           7           10          70%
Storage         45 GB       100 GB      45%
API Calls       75,234      100,000     75%
Bandwidth       12 GB       Unlimited   -

Status: Within quota limits
```

## Domain Management

### nself tenant domain add

Add custom domain to tenant.

**Usage**:
```bash
nself tenant domain add <tenant_id> <domain> [OPTIONS]
```

**Options**:
- `--ssl` - Auto-provision SSL certificate
- `--verify-later` - Skip immediate verification

**Process**:
1. Add domain to tenant
2. Verify domain ownership (DNS TXT record)
3. Provision SSL certificate
4. Configure routing

**Examples**:
```bash
# Add custom domain
nself tenant domain add tenant_abc123 app.acme.com

# With SSL
nself tenant domain add tenant_abc123 app.acme.com --ssl
```

**Output**:
```
Adding custom domain: app.acme.com

1. Verify Domain Ownership
   Add this DNS TXT record:

   Type:  TXT
   Name:  _nself-verify.app.acme.com
   Value: tenant-verify-abc123def456

2. Point Domain to nself
   Add this DNS A record:

   Type:  A
   Name:  app.acme.com
   Value: 5.75.235.42

3. Wait for DNS Propagation (up to 48 hours)

4. Verify domain
   nself tenant domain verify tenant_abc123 app.acme.com

5. SSL certificate will be auto-provisioned after verification
```

## Data Isolation

### nself tenant isolate

Configure tenant data isolation.

**Usage**:
```bash
nself tenant isolate <tenant_id> [OPTIONS]
```

**Isolation Levels**:
- **schema** - Separate PostgreSQL schema (default, cost-effective)
- **database** - Separate database (better isolation)
- **cluster** - Separate database cluster (maximum isolation)

**Examples**:
```bash
# Use schema isolation (default)
nself tenant isolate tenant_abc123 --level schema

# Upgrade to database isolation
nself tenant isolate tenant_abc123 --level database
```

### nself tenant backup-tenant

Backup specific tenant data.

**Usage**:
```bash
nself tenant backup-tenant <tenant_id> [OPTIONS]
```

**Options**:
- `--include-files` - Include file uploads
- `--compress` - Compress backup

**Examples**:
```bash
# Backup tenant data
nself tenant backup-tenant tenant_abc123

# Full backup with files
nself tenant backup-tenant tenant_abc123 --include-files --compress
```

## Multi-Tenancy Patterns

### Shared Database, Separate Schema (Default)

```
┌──────────────────────────────────┐
│        PostgreSQL Database        │
├──────────────────────────────────┤
│  Schema: tenant_acme             │
│  - users, posts, etc.            │
├──────────────────────────────────┤
│  Schema: tenant_globex           │
│  - users, posts, etc.            │
├──────────────────────────────────┤
│  Schema: tenant_initech          │
│  - users, posts, etc.            │
└──────────────────────────────────┘
```

**Pros**: Cost-effective, easy management
**Cons**: Less isolation

### Separate Database

```
┌───────────────────┐  ┌───────────────────┐
│  Database: acme   │  │ Database: globex  │
│  - users          │  │ - users           │
│  - posts          │  │ - posts           │
└───────────────────┘  └───────────────────┘
```

**Pros**: Better isolation, easier backups
**Cons**: Higher resource usage

## Best Practices

### 1. Use Schema Isolation for Most Cases

```bash
# Default schema isolation is sufficient for most SaaS apps
nself tenant create --name "New Customer" --email admin@customer.com
```

### 2. Enforce Quotas

```bash
# Set quotas per plan
nself tenant billing quota set --plan pro \
  --users 10 \
  --storage 100GB \
  --api-calls 100000
```

### 3. Regular Usage Audits

```bash
# Monthly usage audit
nself tenant billing usage --all --export monthly-usage-$(date +%Y%m).csv
```

### 4. Backup Before Major Changes

```bash
# Backup before tenant migration
nself tenant backup-tenant tenant_abc123
nself tenant migrate tenant_abc123 --to-region eu-west
```

## Related Commands

- `nself db migrate` - Run tenant-specific migrations
- `nself auth users` - Manage tenant users
- `nself config` - Tenant configuration

## See Also

- [Multi-Tenancy Guide](../../guides/MULTI-TENANCY.md)
- [Billing Integration](../../guides/BILLING.md)
- [Custom Domains](../../guides/CUSTOM-DOMAINS.md)
- [Data Isolation](../../guides/DATA-ISOLATION.md)
