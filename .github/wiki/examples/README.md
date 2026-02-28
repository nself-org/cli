# nself v0.9.0 - Configuration Examples

This directory contains comprehensive example configurations for nself v0.9.0, showcasing billing, white-label, and multi-tenant features.

## Quick Reference

| File | Use Case | Complexity | Best For |
|------|----------|------------|----------|
| `quick-start-saas.env` | Get started fast | ⭐ Simple | MVPs, prototypes, learning |
| `billing-config-example.env` | Billing & payments | ⭐⭐ Moderate | Monetizing your app |
| `whitelabel-config-example.env` | Custom branding | ⭐⭐ Moderate | Agencies, white-label products |
| `multi-tenant-example.env` | Multi-tenancy setup | ⭐⭐⭐ Advanced | SaaS applications |
| `enterprise-example.env` | Full production setup | ⭐⭐⭐⭐ Expert | Enterprise deployments |

## Example Files

### 1. quick-start-saas.env

**Perfect for beginners and MVPs**

Minimal configuration to get a SaaS application running quickly with:
- Multi-tenancy (organization-based)
- Billing & subscriptions (Stripe test mode)
- Basic white-labeling (logo + colors)
- Essential services (database, auth, storage, email)

**How to use:**
```bash
cp src/examples/quick-start-saas.env .env
# Edit .env and fill in your Stripe test keys
nself build && nself start
```

**Time to running app:** ~5 minutes

---

### 2. billing-config-example.env

**Complete billing and monetization reference**

Comprehensive configuration for:
- Stripe payment processing
- Usage tracking and metering
- Subscription plans and pricing
- Invoice generation
- Quota enforcement
- Tax handling
- Payment recovery (dunning)
- Customer portal

**Key sections:**
- API configuration (test and live keys)
- Usage tracking and aggregation
- Quotas and limits
- Invoice customization
- Webhook setup
- Tax compliance
- Customer portal settings

**Use this when:** You need to monetize your application with subscriptions or usage-based pricing.

---

### 3. whitelabel-config-example.env

**Complete white-label and branding reference**

Comprehensive configuration for:
- Brand identity (name, logo, colors)
- Custom domains per feature
- Email template customization
- Custom themes and CSS
- Documentation and resources
- SEO and marketing settings
- Multi-brand mode (resellers)

**White-label tiers:**
- **Basic:** Logo and colors only
- **Professional:** + Custom domains, email templates
- **Enterprise:** + Full rebrand, source code customization

**Use this when:** Building white-label products, agency solutions, or rebranded deployments.

---

### 4. multi-tenant-example.env

**Complete multi-tenancy reference**

Comprehensive configuration for:
- Tenant isolation (schema, database, or row-level)
- Organization management
- Team collaboration
- Resource quotas per tenant
- Custom domains per tenant
- Tenant-specific branding
- Hierarchical tenancy (sub-organizations)
- Data portability and export

**Architecture options:**
- **Schema:** Each tenant gets own PostgreSQL schema (recommended for most SaaS)
- **Database:** Each tenant gets own database (maximum isolation)
- **Row-level:** All tenants share tables with RLS (unlimited scale)
- **Hybrid:** Combination of approaches

**Use this when:** Building SaaS applications serving multiple organizations or customers.

---

### 5. enterprise-example.env

**Production-ready enterprise configuration**

Full-featured production setup with:
- All features enabled (billing, multi-tenant, white-label)
- High availability and scaling
- Advanced monitoring (Prometheus, Grafana, Loki, Tempo)
- Security hardening (SSL, firewall, audit logging)
- Compliance features (SOC2, GDPR, HIPAA)
- Backup and disaster recovery
- Performance optimization
- Multiple custom services and frontend apps

**Includes:**
- 25+ Docker services
- Enterprise SSO (Google, Microsoft, Okta, SAML)
- Database replication
- Auto-scaling
- CDN configuration
- Secrets management (Vault)
- Full observability stack

**Use this when:** Deploying to production with enterprise requirements.

---

## How to Use These Examples

### Method 1: Copy entire file
```bash
# Start with the example closest to your needs
cp src/examples/quick-start-saas.env .env

# Customize for your project
nano .env

# Build and start
nself build && nself start
```

### Method 2: Copy specific sections
```bash
# Keep your existing .env
nano .env

# Copy relevant sections from examples
# Example: Add billing configuration from billing-config-example.env
```

### Method 3: Reference while configuring
```bash
# Use examples as documentation
# Open example files to see all available options and comments
less src/examples/billing-config-example.env
```

## Configuration Workflow

### For MVPs and Prototypes
```bash
1. Start with: quick-start-saas.env
2. Fill in required fields (marked with ⚠️)
3. Run: nself build && nself start
4. Iterate and add features as needed
```

### For Production Deployments
```bash
1. Start with: enterprise-example.env
2. Review all security settings
3. Replace all <CHANGE_ME_*> placeholders
4. Store secrets in .secrets file (never commit!)
5. Test in staging environment first
6. Deploy to production
```

### For Specific Features
```bash
# Adding billing to existing project
1. Open: billing-config-example.env
2. Copy billing section to your .env
3. Get Stripe API keys from dashboard
4. Configure webhook endpoint
5. Rebuild: nself build

# Adding white-label to existing project
1. Open: whitelabel-config-example.env
2. Copy white-label section to your .env
3. Upload logos/assets to CDN
4. Set custom domains in DNS
5. Rebuild: nself build
```

## Common Configuration Patterns

### Pattern 1: Simple SaaS (B2B)
```env
MULTI_TENANT_ENABLED=true
MULTI_TENANT_ARCHITECTURE=schema
BILLING_ENABLED=true
BILLING_DEFAULT_PLAN=free
WHITELABEL_ENABLED=true
WHITELABEL_TIER=basic
```

### Pattern 2: Freemium with Metered Billing
```env
BILLING_ENABLED=true
BILLING_USAGE_TRACKING_ENABLED=true
BILLING_METERED_ENABLED=true
BILLING_QUOTA_ENFORCEMENT=true
BILLING_DEFAULT_PLAN=free
```

### Pattern 3: White-Label Platform (Agencies)
```env
WHITELABEL_ENABLED=true
WHITELABEL_TIER=enterprise
WHITELABEL_MULTI_BRAND_MODE=true
MULTI_TENANT_ENABLED=true
MULTI_TENANT_BRANDING_ENABLED=true
```

### Pattern 4: Enterprise SaaS
```env
MULTI_TENANT_ENABLED=true
MULTI_TENANT_HIERARCHICAL_ENABLED=true
MULTI_TENANT_CUSTOM_DOMAINS_ENABLED=true
MULTI_TENANT_SSO_ENFORCEMENT_ENABLED=true
BILLING_ENABLED=true
BILLING_BILLING_ENTITY=organization
COMPLIANCE_SOC2=true
COMPLIANCE_GDPR=true
```

## Environment-Specific Configurations

### Development
```env
ENV=dev
BASE_DOMAIN=local.nself.org
BILLING_TEST_MODE=true
STRIPE_PUBLISHABLE_KEY=pk_test_...
STRIPE_SECRET_KEY=sk_test_PLACEHOLDER...
HASURA_GRAPHQL_ENABLE_CONSOLE=true
AUTH_DISABLE_SIGNUP=false
MONITORING_ENABLED=false  # Optional, lighter resource usage
```

### Staging
```env
ENV=staging
BASE_DOMAIN=staging.yourdomain.com
BILLING_TEST_MODE=true  # Still use test Stripe keys
SSL_ENABLED=true
SSL_PROVIDER=letsencrypt
MONITORING_ENABLED=true
AUTO_SCALING_ENABLED=false
BACKUP_ENABLED=true
```

### Production
```env
ENV=production
BASE_DOMAIN=yourdomain.com
BILLING_TEST_MODE=false
STRIPE_PUBLISHABLE_KEY=pk_live_...
STRIPE_SECRET_KEY=sk_live_...
SSL_ENABLED=true
HASURA_GRAPHQL_ENABLE_CONSOLE=false
HASURA_GRAPHQL_DISABLE_INTROSPECTION=true
MONITORING_ENABLED=true
AUTO_SCALING_ENABLED=true
BACKUP_ENABLED=true
SECURITY_AUDIT_LOGGING=true
```

## Security Best Practices

### Required for Production

1. **Strong Secrets**
   ```bash
   # Generate strong random secrets
   openssl rand -hex 32  # For passwords, JWT keys, etc.
   ```

2. **Never Commit Secrets**
   ```bash
   # Use .secrets file (gitignored)
   echo "STRIPE_SECRET_KEY=sk_live_..." >> .secrets
   echo "POSTGRES_PASSWORD=..." >> .secrets
   ```

3. **Enable SSL/TLS**
   ```env
   SSL_ENABLED=true
   SSL_PROVIDER=letsencrypt
   SSL_MIN_VERSION=TLSv1.2
   ```

4. **Audit Logging**
   ```env
   SECURITY_AUDIT_LOGGING=true
   MULTI_TENANT_AUDIT_LOGGING=true
   BILLING_EVENT_LOGGING=true
   ```

5. **Regular Backups**
   ```env
   BACKUP_ENABLED=true
   BACKUP_SCHEDULE=0 2 * * *  # Daily at 2am
   BACKUP_RETENTION_DAYS=30
   BACKUP_ENCRYPTION=true
   ```

## Troubleshooting

### "Service won't start"
```bash
# Check Docker logs
docker logs <container_name>

# Common issues:
# - Port already in use (change port in .env)
# - Missing required variable
# - Invalid credentials
```

### "Stripe webhook not working"
```bash
# Verify webhook secret matches Stripe dashboard
# Check webhook URL is publicly accessible HTTPS
# Test with Stripe CLI:
stripe listen --forward-to localhost:1337/webhooks/stripe
```

### "Multi-tenancy not working"
```bash
# Verify tenant identification source matches your setup
# Subdomain: Ensure wildcard DNS configured (*.app.domain.com)
# Domain: Ensure DNS points to your server
# Check Docker logs for tenant resolution errors
```

### "Quotas not enforcing"
```bash
# Ensure quota enforcement is enabled
BILLING_QUOTA_ENFORCEMENT=true
BILLING_QUOTA_CHECK_MODE=realtime

# Check usage tracking is enabled
BILLING_USAGE_TRACKING_ENABLED=true
```

## Getting Help

- **Documentation:** https://docs.nself.org
- **Examples:** https://github.com/nself-org/cli/tree/main/examples
- **Community:** https://discord.gg/nself
- **Support:** support@nself.org

## Contributing

Found an issue or have a suggestion for these examples?

1. Open an issue: https://github.com/nself-org/cli/issues
2. Submit a PR: https://github.com/nself-org/cli/pulls

## License

These examples are part of the nself project and licensed under the same terms.

---

**Last Updated:** January 30, 2025
**nself Version:** v0.9.0
**Status:** ✅ Ready for use
