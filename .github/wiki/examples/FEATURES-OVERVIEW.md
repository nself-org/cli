# nself v0.9.0 - Features Overview

Quick reference guide for the three major features introduced in v0.9.0.

> **Command Structure Note:** v0.9.6+ uses consolidated v1.0 commands. Examples below use the new command structure.

## 1. Billing & Monetization

**Purpose:** Monetize your application with subscriptions, usage-based pricing, and payment processing.

### Key Capabilities
- ✅ Stripe integration (test and production)
- ✅ Subscription management (monthly, annual, trials)
- ✅ Usage tracking and metering
- ✅ Quota enforcement (soft and hard limits)
- ✅ Invoice generation (PDF, automatic emails)
- ✅ Tax handling (Stripe Tax, manual rates)
- ✅ Payment recovery (dunning, retry logic)
- ✅ Customer portal (self-service)
- ✅ Webhooks (event notifications)
- ✅ Analytics (MRR, churn, LTV)

### Configuration File
**billing-config-example.env** (640 lines, comprehensive)

### Quick Start
```env
BILLING_ENABLED=true
BILLING_STRIPE_ENABLED=true
STRIPE_PUBLISHABLE_KEY=pk_test_...
STRIPE_SECRET_KEY=sk_test_PLACEHOLDER...
BILLING_DEFAULT_PLAN=free
BILLING_TRIAL_PERIOD_DAYS=14
```

### Use Cases
- Freemium SaaS with paid tiers
- Usage-based pricing (API calls, storage, etc.)
- Subscription box services
- Metered billing platforms
- Enterprise seat-based pricing

---

## 2. White-Label & Branding

**Purpose:** Rebrand the entire platform with your company identity or enable multi-brand deployments.

### Key Capabilities
- ✅ Brand identity (name, logo, colors, fonts)
- ✅ Custom domains (per service)
- ✅ Email template customization
- ✅ Custom themes and CSS
- ✅ SEO and meta tags
- ✅ Documentation rebranding
- ✅ Multi-brand mode (resellers)
- ✅ Source code rebranding (enterprise)
- ✅ Custom CLI name (enterprise)
- ✅ Remove nself branding

### Configuration File
**whitelabel-config-example.env** (700 lines, comprehensive)

### White-Label Tiers

| Tier | Features | Best For |
|------|----------|----------|
| **Basic** | Logo, colors | Simple branding |
| **Professional** | + Custom domains, email templates | Agencies, white-label |
| **Enterprise** | + Full rebrand, source code access | Resellers, private label |

### Quick Start
```env
WHITELABEL_ENABLED=true
WHITELABEL_TIER=professional
WHITELABEL_BRAND_NAME=Acme Platform
WHITELABEL_LOGO_URL=https://cdn.acme.com/logo.svg
WHITELABEL_PRIMARY_COLOR=#007bff
WHITELABEL_DOMAIN=app.acme.com
WHITELABEL_SHOW_NSELF_BRANDING=false
```

### Use Cases
- Agency white-label products
- Partner/reseller programs
- Enterprise private deployments
- Multi-brand platforms
- Custom OEM solutions

---

## 3. Multi-Tenancy

**Purpose:** Serve multiple organizations/customers from a single deployment with data isolation.

### Key Capabilities
- ✅ Tenant isolation (schema, database, or row-level)
- ✅ Organization management
- ✅ Team collaboration (roles, permissions)
- ✅ Resource quotas per tenant
- ✅ Custom domains per tenant
- ✅ Tenant-specific branding
- ✅ Hierarchical tenancy (sub-orgs)
- ✅ Usage tracking per tenant
- ✅ Billing per tenant
- ✅ Data export and portability

### Configuration File
**multi-tenant-example.env** (717 lines, comprehensive)

### Architecture Options

| Architecture | Isolation | Scalability | Complexity | Best For |
|--------------|-----------|-------------|------------|----------|
| **Schema** | Good | High | Medium | Most SaaS (10-10k tenants) |
| **Database** | Maximum | Medium | High | Enterprise, compliance (10-1k tenants) |
| **Row-level** | Moderate | Unlimited | Low | Simple apps, many tenants |
| **Hybrid** | Flexible | High | High | Complex hierarchies |

### Quick Start
```env
MULTI_TENANT_ENABLED=true
MULTI_TENANT_ARCHITECTURE=schema
MULTI_TENANT_IDENTIFIER_SOURCE=subdomain
MULTI_TENANT_ORGANIZATIONS_ENABLED=true
MULTI_TENANT_TEAMS_ENABLED=true
MULTI_TENANT_QUOTAS_ENABLED=true
MULTI_TENANT_DEFAULT_STORAGE_QUOTA=1073741824  # 1GB
```

### Use Cases
- B2B SaaS platforms
- Enterprise team collaboration tools
- Multi-organization project management
- Agency client management
- Marketplace platforms

---

## Combined Features

### Scenario 1: SaaS Platform
```env
# Multi-tenant + Billing
MULTI_TENANT_ENABLED=true
BILLING_ENABLED=true
BILLING_BILLING_ENTITY=organization
MULTI_TENANT_BILLING_ENABLED=true
```

**Result:** Each organization has its own subscription and quotas.

### Scenario 2: White-Label Reseller Platform
```env
# Multi-tenant + White-label
MULTI_TENANT_ENABLED=true
WHITELABEL_ENABLED=true
WHITELABEL_TIER=enterprise
WHITELABEL_MULTI_BRAND_MODE=true
MULTI_TENANT_BRANDING_ENABLED=true
```

**Result:** Each tenant can have their own branding and custom domain.

### Scenario 3: Enterprise SaaS with Full Features
```env
# All three features
MULTI_TENANT_ENABLED=true
BILLING_ENABLED=true
WHITELABEL_ENABLED=true
MULTI_TENANT_CUSTOM_DOMAINS_ENABLED=true
BILLING_BILLING_ENTITY=organization
WHITELABEL_TIER=professional
```

**Result:** Full-featured enterprise platform with billing, branding, and multi-tenancy.

---

## Feature Comparison Matrix

| Feature | Billing | White-Label | Multi-Tenant |
|---------|---------|-------------|--------------|
| **Purpose** | Monetization | Branding | Data Isolation |
| **Primary Users** | All SaaS | Agencies, Resellers | B2B SaaS |
| **Complexity** | Medium | Low-Medium | High |
| **Setup Time** | ~30 min | ~15 min | ~1 hour |
| **External Deps** | Stripe | CDN (optional) | None |
| **Production Ready** | ✅ Yes | ✅ Yes | ✅ Yes |

---

## Example Configurations

### 1. Quick Start (MVP)
**File:** `quick-start-saas.env` (397 lines)
- Multi-tenant: Basic
- Billing: Test mode
- White-label: Basic branding
- Time to launch: ~5 minutes

### 2. Production SaaS
**File:** `enterprise-example.env` (676 lines)
- Multi-tenant: Full features
- Billing: Production Stripe
- White-label: Professional
- Monitoring: Full stack
- Security: Hardened
- Time to launch: ~2 hours (including setup)

---

## Getting Started

### Step 1: Choose Your Path

**Path A: Learning / MVP**
→ Start with `quick-start-saas.env`

**Path B: Production SaaS**
→ Start with `enterprise-example.env`

**Path C: Specific Feature**
→ Reference individual feature files:
- `billing-config-example.env`
- `whitelabel-config-example.env`
- `multi-tenant-example.env`

### Step 2: Configure

```bash
# Copy example to .env
cp src/examples/quick-start-saas.env .env

# Edit configuration
nano .env

# Replace placeholders:
# - Stripe API keys
# - Database passwords
# - Brand name and colors
```

### Step 3: Build and Start

```bash
# Generate configuration
nself build

# Start services
nself start

# Check status
nself status

# View URLs
nself urls
```

### Step 4: Verify

```bash
# Access your application
open http://app.local.nself.org

# Create first user (becomes admin)
# Create first organization (becomes first tenant)
# Test billing (test mode)
```

---

## Resources

- **Documentation:** https://docs.nself.org
- **Examples Directory:** `/src/examples/`
- **Configuration Reference:** `src/examples/README.md`
- **Community:** https://discord.gg/nself
- **Support:** support@nself.org

---

## Version Information

- **nself Version:** v0.9.0
- **Release Date:** February 2026 (planned)
- **Status:** In Development
- **Documentation Updated:** January 30, 2025

---

## Next Steps

After getting familiar with these features:

1. **Customize your configuration** based on your specific needs
2. **Test in development** with test Stripe keys
3. **Deploy to staging** for team testing
4. **Configure production** with production Stripe keys
5. **Launch** and iterate based on user feedback

---

**Need help?** Check `src/examples/README.md` for detailed guides and troubleshooting.
