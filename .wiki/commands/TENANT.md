# nself tenant - Multi-Tenant Management

**Version 0.9.0** | Complete multi-tenant platform with billing, branding, domain management, and customization

---

## Overview

The `nself tenant` command system provides comprehensive multi-tenant capabilities for SaaS platforms, resellers, and agencies managing multiple branded instances on a single nself deployment.

## Command Structure

```bash
nself tenant <subcommand> [OPTIONS]
```

### Core Tenant Management

Fundamental tenant operations:

```bash
nself tenant create <name>          # Create new tenant
nself tenant list                   # List all tenants
nself tenant show <tenant-id>       # Show tenant details
nself tenant delete <tenant-id>     # Delete tenant
nself tenant switch <tenant-id>     # Switch active tenant context
```

### Multi-Tenant Feature Commands

The `nself tenant` command acts as a parent for all multi-tenant features:

#### Billing & Subscriptions

```bash
nself tenant billing <subcommand>
```

Comprehensive billing system with:
- Subscription management (plans, upgrades, downgrades)
- Invoice generation and payment tracking
- Usage tracking and quota enforcement
- Payment method management
- Revenue analytics and reporting

**[See full billing documentation →](BILLING.md)**

#### Branding & White-Label

```bash
nself tenant branding <subcommand>
```

Complete branding control:
- Logo upload and management
- Color scheme customization
- Font selection
- Custom CSS injection
- Theme management

**[See full branding documentation →](WHITELABEL.md#branding-commands)**

#### Custom Domains

```bash
nself tenant domains <subcommand>
```

Domain configuration and SSL:
- Custom domain mapping
- DNS verification
- Automatic SSL certificates
- SSL renewal management
- Domain health monitoring

**[See full domain documentation →](WHITELABEL.md#domain-commands)**

#### Email Customization

```bash
nself tenant email <subcommand>
```

Branded email communications:
- Custom email templates
- Multi-language support
- Email branding (logo, colors)
- Template testing
- Sender configuration

**[See full email documentation →](WHITELABEL.md#email-commands)**

#### Theme Management

```bash
nself tenant themes <subcommand>
```

Visual design system:
- Pre-built themes
- Custom theme creation
- Theme activation
- Design token management
- Theme import/export

**[See full theme documentation →](WHITELABEL.md#theme-commands)**

---

## When to Use Multi-Tenant Features

### Use Multi-Tenancy When:

1. **SaaS Platform**
   - Serving multiple customers on one deployment
   - Each customer needs their own isolated data
   - Per-customer billing and subscription management
   - Example: Project management SaaS, CRM platform

2. **Agency**
   - Managing multiple client projects
   - Each client needs custom branding
   - Separate billing per client
   - Example: Digital agency managing client backends

3. **Reseller/White-Label**
   - Selling branded instances to partners
   - Each partner needs complete white-labeling
   - Per-partner custom domains
   - Example: Platform-as-a-Product reseller

4. **Enterprise with Divisions**
   - Multiple business units sharing infrastructure
   - Department-specific branding and access
   - Cost center billing
   - Example: Large corporation with multiple brands

### Don't Use Multi-Tenancy When:

1. **Single Application**
   - One product, one brand
   - No need for per-customer isolation
   - Shared user base
   - Example: Standard web app

2. **Personal Projects**
   - Individual developer use
   - No customers or clients
   - No billing requirements

---

## Quick Start - Multi-Tenant Setup

### Step 1: Enable Multi-Tenant Mode

```bash
# Add to .env
WHITELABEL_ENABLED=true
WHITELABEL_MULTI_BRAND_MODE=true
BILLING_ENABLED=true

nself build && nself restart
```

### Step 2: Create First Tenant

```bash
# Create tenant
nself tenant create "Client A"

# Setup branding
nself tenant branding set-logo ./client-a-logo.png --tenant client-a
nself tenant branding set-colors --primary "#FF0000" --tenant client-a

# Add custom domain
nself tenant domains add client-a.example.com --tenant client-a
nself tenant domains verify client-a.example.com --tenant client-a
nself tenant domains ssl client-a.example.com --auto-renew --tenant client-a

# Setup billing
nself tenant billing plans create "Basic" --price 29 --tenant client-a
```

### Step 3: Configure Tenant-Specific Settings

```bash
# Customize email templates
nself tenant email customize welcome --tenant client-a

# Create custom theme
nself tenant themes create "client-a-theme" --tenant client-a
nself tenant themes activate "client-a-theme" --tenant client-a

# View configuration
nself tenant show client-a
```

---

## Multi-Tenant Architecture

### Tenant Isolation

Each tenant gets complete isolation:

- **Database**: Table prefixing or schema separation
- **Storage**: Separate S3 buckets or folder isolation
- **Authentication**: JWT scoped to tenant
- **GraphQL**: Filtered queries per tenant
- **Branding**: Complete visual separation
- **Billing**: Independent subscriptions

### Domain Routing

Automatic tenant detection from domain:

```
client-a.app.com → Tenant A (with Client A branding)
client-b.app.com → Tenant B (with Client B branding)
admin.app.com    → Admin panel (no tenant branding)
```

### Data Isolation Strategies

**Table Prefixing (Default)**
```sql
-- Tenant A data
tenant_a_users
tenant_a_projects

-- Tenant B data
tenant_b_users
tenant_b_projects
```

**Schema Separation (Enterprise)**
```sql
-- Tenant A schema
CREATE SCHEMA tenant_a;
tenant_a.users
tenant_a.projects

-- Tenant B schema
CREATE SCHEMA tenant_b;
tenant_b.users
tenant_b.projects
```

---

## Environment Variables

### Core Multi-Tenant Settings

```bash
# Enable multi-tenant mode
WHITELABEL_ENABLED=true
WHITELABEL_MULTI_BRAND_MODE=true

# Tenant isolation strategy
TENANT_ISOLATION=table_prefix    # table_prefix, schema_separation
TENANT_ID_SOURCE=domain          # domain, subdomain, header

# Default tenant (fallback)
DEFAULT_TENANT_ID=default

# Billing
BILLING_ENABLED=true
BILLING_STRIPE_KEY=sk_live_...
```

### Advanced Configuration

```bash
# Tenant limits
MAX_TENANTS=100
MAX_USERS_PER_TENANT=1000
MAX_STORAGE_PER_TENANT_GB=100

# Feature flags per tenant
TENANT_FEATURES_SOURCE=database  # database, config, api

# Cross-tenant access
ALLOW_CROSS_TENANT_ACCESS=false
CROSS_TENANT_ADMIN_ROLE=super_admin
```

---

## Common Workflows

### Add New Tenant (Full Setup)

```bash
# 1. Create tenant
TENANT_ID=$(nself tenant create "New Client" --output-id)

# 2. Setup branding
nself tenant branding set-logo ./logo.png --tenant "$TENANT_ID"
nself tenant branding set-colors --primary "#0066cc" --tenant "$TENANT_ID"

# 3. Configure domain
nself tenant domains add client.example.com --tenant "$TENANT_ID"
nself tenant domains verify client.example.com --tenant "$TENANT_ID"
nself tenant domains ssl client.example.com --auto-renew --tenant "$TENANT_ID"

# 4. Setup billing
nself tenant billing plans assign pro --tenant "$TENANT_ID"

# 5. Customize emails
nself tenant email customize welcome --tenant "$TENANT_ID"
nself tenant email customize password-reset --tenant "$TENANT_ID"

# 6. Activate tenant
nself tenant activate "$TENANT_ID"
```

### Tenant Migration

```bash
# Export tenant configuration
nself tenant export tenant-a > tenant-a-config.json

# Import to new environment
nself tenant import tenant-a-config.json
```

### Tenant Billing Lifecycle

```bash
# Create subscription
nself tenant billing subscription create --plan pro --tenant tenant-a

# Monitor usage
nself tenant billing usage show --tenant tenant-a

# Handle upgrade
nself tenant billing subscription upgrade enterprise --tenant tenant-a

# Process payment
nself tenant billing invoice pay latest --tenant tenant-a
```

---

## Database Schema

Multi-tenant tables:

```sql
-- Core tenant management
tenants
  - id (uuid)
  - name (text)
  - slug (text)
  - status (enum: active, suspended, deleted)
  - created_at
  - settings (jsonb)

-- Tenant branding
tenant_branding
  - tenant_id
  - logo_url
  - primary_color
  - secondary_color
  - font_family
  - custom_css

-- Tenant domains
tenant_domains
  - tenant_id
  - domain (text)
  - verified (boolean)
  - ssl_cert_path
  - ssl_expires_at

-- Tenant billing
tenant_subscriptions
  - tenant_id
  - plan_id
  - status
  - current_period_end

-- Tenant users (per-tenant user data)
tenant_users
  - tenant_id
  - user_id
  - role
  - permissions
```

---

## Best Practices

### Tenant Onboarding

1. **Pre-configure templates**
   - Default branding
   - Email templates
   - Initial theme
   - Default billing plan

2. **Automated setup**
   - Script tenant creation
   - Assign default resources
   - Send welcome email
   - Provide setup checklist

3. **Progressive customization**
   - Start with defaults
   - Allow customization over time
   - Track customization completion

### Performance Optimization

1. **Caching**
   - Cache tenant settings
   - Cache branding assets
   - Cache domain routing
   - TTL: 1 hour (adjustable)

2. **Database Indexing**
   - Index tenant_id on all tables
   - Composite indexes for common queries
   - Partition large tables by tenant

3. **Asset Delivery**
   - Use CDN for logos and assets
   - Optimize image sizes
   - Lazy-load tenant resources

### Security Considerations

1. **Access Control**
   - Never expose cross-tenant data
   - Validate tenant_id on all requests
   - Audit cross-tenant access attempts

2. **Data Isolation**
   - Row-level security in PostgreSQL
   - Hasura permissions per tenant
   - Validate tenant context in functions

3. **Billing Security**
   - Encrypt payment method data
   - Webhook signature validation
   - Audit financial operations

---

## Troubleshooting

### Tenant Not Found

```bash
# List all tenants
nself tenant list

# Check tenant status
nself tenant show <tenant-id>

# Verify tenant is active
nself tenant activate <tenant-id>
```

### Branding Not Showing

```bash
# Verify branding is set
nself tenant branding show --tenant <tenant-id>

# Clear cache
nself cache clear branding

# Rebuild and restart
nself build && nself restart
```

### Domain Not Routing

```bash
# Check domain verification
nself tenant domains health <domain>

# Verify DNS records
dig <domain>

# Check nginx configuration
nself status nginx
nself logs nginx | grep <domain>
```

### Billing Issues

```bash
# Check subscription status
nself tenant billing subscription show --tenant <tenant-id>

# View usage
nself tenant billing usage show --tenant <tenant-id>

# Check payment method
nself tenant billing payment list --tenant <tenant-id>
```

---

## Related Documentation

- **[Billing Commands](BILLING.md)** - Complete billing reference
- **[White-Label & Customization](WHITELABEL.md)** - Branding and customization
- **[Architecture](../architecture/ARCHITECTURE.md)** - System design
- **[Environment Configuration](../configuration/ENVIRONMENT-VARIABLES.md)** - Configuration reference

---

## Support

For multi-tenant setup assistance:

- **Documentation**: https://docs.nself.org/multi-tenant
- **Examples**: https://docs.nself.org/multi-tenant/examples
- **Issues**: https://github.com/nself-org/cli/issues
- **Support**: support@nself.org

---

**Version**: 0.9.0
**Last Updated**: January 30, 2025
**Status**: Production Ready
