# nself Tutorials

Comprehensive step-by-step tutorials for common use cases with nself.

---

## Quick Start Tutorials

Get up and running quickly with complete, ready-to-use setups.

### [Build a SaaS in 15 Minutes](QUICK-START-SAAS.md)

**Time**: 15-20 minutes | **Difficulty**: Beginner

Build a production-ready SaaS platform with:
- Multi-tenant architecture with database isolation
- Stripe billing integration (subscriptions & usage-based)
- White-label customization per tenant
- Authentication & authorization
- Production deployment

**Perfect for**: SaaS startups, B2C applications, subscription platforms

---

### [B2B Platform Setup](QUICK-START-B2B.md)

**Time**: 20-25 minutes | **Difficulty**: Intermediate

Build a B2B platform with:
- Organization hierarchies (parent/child accounts)
- Team management with roles & permissions
- Custom branding per organization
- Usage tracking and billing per organization
- Admin dashboard for platform management

**Perfect for**: B2B SaaS, enterprise platforms, team collaboration tools

---

### [Marketplace/Platform Setup](QUICK-START-MARKETPLACE.md)

**Time**: 25-30 minutes | **Difficulty**: Intermediate-Advanced

Build a multi-vendor marketplace with:
- Multi-vendor architecture with isolation
- Vendor onboarding and verification
- Product/listing management per vendor
- Separate billing and payouts per vendor
- Platform fee collection (commission)
- White-label storefront per vendor

**Perfect for**: Marketplaces, multi-vendor platforms, e-commerce platforms

---

### [Agency Reseller Setup](QUICK-START-AGENCY.md)

**Time**: 20-25 minutes | **Difficulty**: Intermediate

Build an agency platform with:
- Isolated client environments
- Custom branding per client
- Centralized billing and invoicing
- Project management and tracking
- Client portal access
- Usage tracking across all clients

**Perfect for**: Digital agencies, consulting firms, resellers, managed service providers

---

## Integration Guides

Deep-dive guides for specific integrations and features.

### [Complete Stripe Integration](STRIPE-INTEGRATION.md)

**Time**: 30-40 minutes | **Difficulty**: Intermediate

Complete guide covering:
- Stripe account setup and API keys
- Plugin installation and configuration
- Webhook configuration (development & production)
- Subscription billing (monthly, annual, custom)
- Usage-based billing (metered, tiered)
- One-time payments and invoices
- Stripe Connect for marketplaces
- Testing with test cards
- Production deployment checklist
- Troubleshooting common issues

**Topics covered**:
- Payment intents
- Customer management
- Subscription lifecycle
- Proration handling
- Refunds and disputes
- Webhook security
- Error handling

---

### [Custom Domains Setup](CUSTOM-DOMAINS.md)

**Time**: 15-30 minutes (+ DNS propagation) | **Difficulty**: Intermediate

Complete guide covering:
- Single domain configuration
- Multi-tenant domain management
- SSL certificate management (Let's Encrypt, self-signed, custom)
- DNS configuration (Cloudflare, GoDaddy, Namecheap, Route 53)
- Wildcard domains (*.yourdomain.com)
- Domain verification methods
- Production deployment
- Troubleshooting DNS and SSL issues

**Topics covered**:
- A records and CNAME records
- DNS propagation
- Certificate issuance and renewal
- HTTPS enforcement
- Custom SSL certificates
- Wildcard SSL certificates

---

## By Use Case

### SaaS Applications
- ✅ [Quick Start: SaaS](QUICK-START-SAAS.md) - General SaaS setup
- ✅ [Stripe Integration](STRIPE-INTEGRATION.md) - Billing and subscriptions
- ✅ [Custom Domains](CUSTOM-DOMAINS.md) - Custom domain setup

### B2B Platforms
- ✅ [Quick Start: B2B](QUICK-START-B2B.md) - B2B platform setup
- ✅ [Custom Domains](CUSTOM-DOMAINS.md) - Organization domains
- ✅ [Stripe Integration](STRIPE-INTEGRATION.md) - B2B billing

### Marketplaces
- ✅ [Quick Start: Marketplace](QUICK-START-MARKETPLACE.md) - Multi-vendor setup
- ✅ [Stripe Integration](STRIPE-INTEGRATION.md) - Stripe Connect for payouts
- ✅ [Custom Domains](CUSTOM-DOMAINS.md) - Vendor storefronts

### Agencies
- ✅ [Quick Start: Agency](QUICK-START-AGENCY.md) - Agency platform
- ✅ [Custom Domains](CUSTOM-DOMAINS.md) - Client domains
- ✅ [Stripe Integration](STRIPE-INTEGRATION.md) - Client billing

---

## Tutorial Features

Each tutorial includes:

### Step-by-Step Instructions
Every tutorial provides clear, numbered steps with exact commands to run.

### Time Estimates
Know how long each section will take before you start.

### Code Examples
- GraphQL mutations and queries
- API endpoint implementations
- Frontend integration code
- Database queries

### Diagrams
ASCII art diagrams showing architecture and data flow.

### Common Pitfalls
Learn from common mistakes and avoid them.

### Troubleshooting
Dedicated troubleshooting sections for each tutorial.

### Next Steps
Links to related tutorials and advanced topics.

---

## Difficulty Levels

### Beginner
- Basic understanding of web development
- Comfortable with command line
- No prior nself experience required

**Recommended tutorials**:
- [Quick Start: SaaS](QUICK-START-SAAS.md)

### Intermediate
- Experience with databases and APIs
- Understanding of multi-tenancy concepts
- Basic DevOps knowledge helpful

**Recommended tutorials**:
- [Quick Start: B2B](QUICK-START-B2B.md)
- [Quick Start: Agency](QUICK-START-AGENCY.md)
- [Stripe Integration](STRIPE-INTEGRATION.md)
- [Custom Domains](CUSTOM-DOMAINS.md)

### Intermediate-Advanced
- Strong understanding of distributed systems
- Experience with payment processing
- Comfortable with advanced configurations

**Recommended tutorials**:
- [Quick Start: Marketplace](QUICK-START-MARKETPLACE.md)

---

## Prerequisites

### Required for All Tutorials
- **Docker Desktop** (v4.0+) - [Install](https://docs.docker.com/get-docker/)
- **nself** installed - `curl -sSL https://install.nself.org | bash`
- **4GB RAM** minimum
- **10GB disk space**

### Optional (Per Tutorial)
- **Stripe account** - For billing tutorials
- **Domain name** - For custom domain tutorials
- **Production server** - For deployment sections

---

## Getting Help

### Documentation
- **[Main Documentation](../README.md)** - Complete docs
- **[Commands Reference](../commands/COMMANDS.md)** - All CLI commands
- **[Services Guide](../services/SERVICES.md)** - Service configurations

### Community
- **[GitHub Issues](https://github.com/nself-org/cli/issues)** - Report bugs
- **[GitHub Discussions](https://github.com/nself-org/cli/discussions)** - Ask questions
- **[Discord](https://discord.gg/nself)** - Live chat

### Support
- **Email**: support@nself.org
- **Website**: https://nself.org

---

## Contributing

Found an issue in a tutorial? Have a suggestion?

1. **Open an issue**: [GitHub Issues](https://github.com/nself-org/cli/issues)
2. **Submit a PR**: [Contributing Guide](../contributing/CONTRIBUTING.md)
3. **Request a tutorial**: [Discussions](https://github.com/nself-org/cli/discussions)

---

## Tutorial Roadmap

### Coming Soon

- [ ] **Quick Start: E-commerce** - Full e-commerce platform
- [ ] **Quick Start: Mobile Backend** - Mobile app backend
- [ ] **Quick Start: API-First** - API-only setup
- [ ] **Advanced: Multi-Region** - Geographic distribution
- [ ] **Advanced: High Availability** - HA setup
- [ ] **Integration: SendGrid** - Email service
- [ ] **Integration: Twilio** - SMS and voice
- [ ] **Integration: Auth0** - Advanced authentication
- [ ] **Integration: Algolia** - Advanced search
- [ ] **Integration: AWS S3** - Cloud storage
- [ ] **Deployment: AWS** - AWS deployment
- [ ] **Deployment: DigitalOcean** - DO deployment
- [ ] **Deployment: Kubernetes** - K8s deployment

**Want a specific tutorial?** [Request it here](https://github.com/nself-org/cli/discussions)

---

## Tutorial Index

| Tutorial | Time | Difficulty | Topics |
|----------|------|------------|--------|
| [SaaS](QUICK-START-SAAS.md) | 15-20 min | Beginner | Multi-tenancy, Billing, White-label |
| [B2B](QUICK-START-B2B.md) | 20-25 min | Intermediate | Organizations, Teams, Permissions |
| [Marketplace](QUICK-START-MARKETPLACE.md) | 25-30 min | Intermediate-Advanced | Multi-vendor, Payouts, Commissions |
| [Agency](QUICK-START-AGENCY.md) | 20-25 min | Intermediate | Client management, Isolation, Billing |
| [Stripe](STRIPE-INTEGRATION.md) | 30-40 min | Intermediate | Payments, Subscriptions, Webhooks |
| [Custom Domains](CUSTOM-DOMAINS.md) | 15-30 min | Intermediate | DNS, SSL, Multi-tenant domains |

---

## Quick Links

**New to nself?** Start here:
1. [Installation Guide](../getting-started/Installation.md)
2. [Quick Start Guide](../getting-started/Quick-Start.md)
3. [Choose a tutorial above](#quick-start-tutorials)

**Already using nself?** Skip to:
- [Integration Guides](#integration-guides)
- [Advanced Topics](../guides/)
- [API Reference](../reference/api/)

---

**Last Updated**: January 30, 2026
**Total Tutorials**: 6
**Total Pages**: ~150+

*More tutorials added regularly. Star the repo to stay updated!*
