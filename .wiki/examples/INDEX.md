# nself v0.9.0 Examples - Complete Index

This directory contains 7 comprehensive configuration examples totaling **3,821 lines** of thoroughly documented configuration options for nself v0.9.0.

## Files in This Directory

| File | Lines | Size | Purpose |
|------|-------|------|---------|
| `quick-start-saas.env` | 397 | 13K | Get SaaS running in 5 minutes |
| `billing-config-example.env` | 640 | 21K | Complete billing reference |
| `whitelabel-config-example.env` | 700 | 22K | Complete white-label reference |
| `multi-tenant-example.env` | 717 | 23K | Complete multi-tenancy reference |
| `enterprise-example.env` | 676 | 19K | Production-ready enterprise config |
| `README.md` | 389 | 9.4K | User guide and documentation |
| `FEATURES-OVERVIEW.md` | 302 | 7.4K | Quick feature reference |
| **Total** | **3,821** | **115K** | |

## Quick Navigation

### I Want To...

**Get started quickly with a SaaS MVP**
→ Read: `quick-start-saas.env`
→ Time: 5 minutes

**Add billing to my existing app**
→ Read: `billing-config-example.env`
→ Copy: Billing section to your .env

**White-label my platform**
→ Read: `whitelabel-config-example.env`
→ Copy: White-label section to your .env

**Build a multi-tenant SaaS**
→ Read: `multi-tenant-example.env`
→ Copy: Multi-tenant section to your .env

**Deploy to production**
→ Read: `enterprise-example.env`
→ Review: All security and performance settings

**Understand the features**
→ Read: `FEATURES-OVERVIEW.md`
→ Then: Choose relevant example file

**Learn best practices**
→ Read: `README.md`
→ Reference: Common patterns section

## Configuration Coverage

### Billing Features (640 lines)
- ✅ Stripe integration (test and live)
- ✅ Subscription management
- ✅ Usage tracking and metering
- ✅ Quota enforcement
- ✅ Invoice generation
- ✅ Tax handling
- ✅ Payment recovery
- ✅ Customer portal
- ✅ Webhooks
- ✅ Analytics

### White-Label Features (700 lines)
- ✅ Brand identity
- ✅ Custom domains
- ✅ Email templates
- ✅ Custom themes
- ✅ SEO settings
- ✅ Multi-brand mode
- ✅ Source code rebrand
- ✅ Custom CLI name
- ✅ Asset management
- ✅ Localization

### Multi-Tenant Features (717 lines)
- ✅ Tenant isolation (3 architectures)
- ✅ Organization management
- ✅ Team collaboration
- ✅ Resource quotas
- ✅ Custom domains per tenant
- ✅ Tenant branding
- ✅ Hierarchical tenancy
- ✅ Data portability
- ✅ Audit logging
- ✅ Performance optimization

### Enterprise Features (676 lines)
- ✅ All above features combined
- ✅ High availability
- ✅ Full monitoring stack
- ✅ Security hardening
- ✅ Compliance features
- ✅ Backup/disaster recovery
- ✅ Performance tuning
- ✅ Auto-scaling
- ✅ Multiple environments
- ✅ Production-ready

## Usage Patterns

### Pattern 1: Learning (Development)
```bash
cp src/examples/quick-start-saas.env .env
# Edit with your Stripe test keys
nself build && nself start
```

### Pattern 2: Feature Addition
```bash
# Keep your .env, add specific feature
nano .env
# Copy relevant section from billing-config-example.env
nself build && nself start
```

### Pattern 3: Production Deployment
```bash
cp src/examples/enterprise-example.env .env
# Review ALL settings
# Replace ALL <CHANGE_ME_*> placeholders
# Store secrets in .secrets file
nself build && nself start
```

## What Each File Teaches You

### quick-start-saas.env
**Learn:** How to get started with minimal configuration
- Basic multi-tenancy setup
- Test mode billing
- Simple white-labeling
- Essential services only

### billing-config-example.env
**Learn:** Every billing and monetization option
- 15+ billing patterns
- Stripe integration details
- Usage tracking strategies
- Quota enforcement approaches
- Invoice customization
- Tax compliance

### whitelabel-config-example.env
**Learn:** Complete branding and customization
- 3 white-label tiers
- Custom domain setup
- Email template branding
- Theme customization
- Multi-brand deployment
- Reseller configuration

### multi-tenant-example.env
**Learn:** Enterprise multi-tenancy architecture
- 4 isolation architectures
- Tenant identification methods
- Organization hierarchies
- Resource quota strategies
- Security and compliance
- Performance optimization

### enterprise-example.env
**Learn:** Production deployment best practices
- All features integrated
- Security hardening
- High availability setup
- Monitoring configuration
- Backup strategies
- Compliance settings

### README.md
**Learn:** How to use the examples effectively
- Configuration workflows
- Common patterns
- Troubleshooting guides
- Security best practices
- Environment-specific configs

### FEATURES-OVERVIEW.md
**Learn:** Quick feature comparison
- Feature capabilities
- Use cases
- Quick start snippets
- Combined scenarios
- Getting started guide

## File Organization

```
examples/
├── INDEX.md                           # This file - start here
├── README.md                          # User guide and documentation
├── FEATURES-OVERVIEW.md               # Quick feature reference
├── quick-start-saas.env              # Minimal SaaS template (MVP)
├── billing-config-example.env        # Complete billing reference
├── whitelabel-config-example.env     # Complete white-label reference
├── multi-tenant-example.env          # Complete multi-tenant reference
└── enterprise-example.env            # Production enterprise config
```

## Reading Order

### For Beginners
1. Start: `FEATURES-OVERVIEW.md` (understand what's possible)
2. Then: `quick-start-saas.env` (get hands-on quickly)
3. Finally: `README.md` (learn best practices)

### For Feature Addition
1. Start: `README.md` (understand configuration patterns)
2. Then: Relevant feature file (billing/whitelabel/multi-tenant)
3. Finally: `FEATURES-OVERVIEW.md` (see how features combine)

### For Production Deployment
1. Start: `enterprise-example.env` (see production config)
2. Then: `README.md` (security and best practices)
3. Finally: Individual feature files (deep dive as needed)

## Key Statistics

- **Total Documentation:** 3,821 lines
- **Total File Size:** 115 KB
- **Number of Examples:** 7 files
- **Configuration Options:** 500+ documented variables
- **Use Cases Covered:** 15+ scenarios
- **Architectures Explained:** 4 multi-tenant, 3 white-label tiers
- **Security Patterns:** 10+ hardening techniques
- **Performance Tips:** 20+ optimization strategies

## Additional Resources

### Within This Repository
- `/docs/` - Full documentation
- `.claude/CLAUDE.md` - Development notes
- `/src/` - Source code with inline docs

### External Resources
- **Documentation:** https://docs.nself.org
- **Community:** https://discord.gg/nself
- **GitHub:** https://github.com/nself-org/cli
- **Support:** support@nself.org

### Stripe Resources
- **Dashboard:** https://dashboard.stripe.com
- **API Docs:** https://stripe.com/docs/api
- **Webhooks:** https://stripe.com/docs/webhooks
- **Testing:** https://stripe.com/docs/testing

## Version History

- **v0.9.0** (Feb 2026) - Initial release of billing, white-label, multi-tenant
- Examples created: January 30, 2025
- Last updated: January 30, 2025

## Contributing

Found a typo or have a suggestion?
- **Issues:** https://github.com/nself-org/cli/issues
- **Pull Requests:** https://github.com/nself-org/cli/pulls

## License

These examples are part of the nself project and licensed under the same terms.

---

**Ready to start?** Pick a file from the table above and dive in!
