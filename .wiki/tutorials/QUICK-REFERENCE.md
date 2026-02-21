# Quick Reference Card

One-page reference for all nself tutorials with common commands and patterns.

---

## Tutorial Quick Links

| Tutorial | Time | Command |
|----------|------|---------|
| [SaaS](QUICK-START-SAAS.md) | 15 min | `nself init --template saas` |
| [B2B](QUICK-START-B2B.md) | 20 min | `nself init --template b2b` |
| [Marketplace](QUICK-START-MARKETPLACE.md) | 25 min | `nself init --template marketplace` |
| [Agency](QUICK-START-AGENCY.md) | 20 min | `nself init --template agency` |
| [Stripe](STRIPE-INTEGRATION.md) | 30 min | `nself plugin install stripe` |
| [Domains](CUSTOM-DOMAINS.md) | 15 min | `nself whitelabel domain add` |

---

## Common Commands

### Project Setup
```bash
# Install nself
curl -sSL https://install.nself.org | bash

# Create project
mkdir myapp && cd myapp
nself init

# Build and start
nself build
nself start

# Check status
nself status
nself urls
```

### Database
```bash
# Apply schema
nself db schema apply schema.dbml

# Run migration
nself db migrate up

# Database shell
nself db shell

# Run query
nself db query "SELECT * FROM users"

# Backup
nself db backup
```

### Stripe Integration
```bash
# Install plugin
nself plugin install stripe

# Configure in .env
STRIPE_API_KEY=sk_test_PLACEHOLDER
STRIPE_PUBLISHABLE_KEY=pk_test_xxxxx

# Sync data
nself plugin stripe sync

# View customers
nself plugin stripe customers list

# View subscriptions
nself plugin stripe subscriptions list
```

### Custom Domains
```bash
# Add domain
nself whitelabel domain add app.myapp.com

# Verify domain
nself whitelabel domain verify app.myapp.com

# Generate SSL
nself ssl generate myapp.com --letsencrypt

# Check SSL
nself ssl check myapp.com

# Enable auto-renewal
nself ssl enable-auto-renew myapp.com
```

### White-Label
```bash
# Initialize
nself whitelabel init

# Create brand
nself whitelabel branding create "My Brand"

# Set colors
nself whitelabel branding set-colors \
  --primary #0066cc \
  --secondary #00cc66

# Upload logo
nself whitelabel logo upload ./logo.png --type main

# Customize email
nself whitelabel email edit welcome
```

---

## GraphQL Patterns

### Create Organization
```graphql
mutation CreateOrganization {
  insert_organizations_one(object: {
    name: "Acme Corp"
    slug: "acme"
    status: "active"
  }) {
    id
    name
  }
}
```

### Create Subscription
```graphql
mutation CreateSubscription {
  insert_subscriptions_one(object: {
    user_id: "user-id"
    stripe_subscription_id: "sub_xxxxx"
    plan: "pro"
    status: "active"
    price_amount: 2999
  }) {
    id
  }
}
```

### Track Usage
```graphql
mutation TrackUsage {
  insert_usage_one(object: {
    organization_id: "org-id"
    metric_name: "api_calls"
    quantity: 1000
  }) {
    id
  }
}
```

### Query Dashboard
```graphql
query Dashboard($userId: uuid!) {
  organizations(where: {members: {user_id: {_eq: $userId}}}) {
    id
    name
    subscription {
      plan
      status
    }
  }
}
```

---

## Environment Variables

### Core Settings
```bash
PROJECT_NAME=myapp
ENV=dev
BASE_DOMAIN=local.nself.org

# Database
POSTGRES_DB=myapp_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=secure-password

# Hasura
HASURA_GRAPHQL_ADMIN_SECRET=admin-secret
HASURA_GRAPHQL_JWT_SECRET=jwt-secret
```

### Stripe
```bash
STRIPE_API_KEY=sk_test_PLACEHOLDER
STRIPE_PUBLISHABLE_KEY=pk_test_xxxxx
STRIPE_WEBHOOK_SECRET=whsec_xxxxx
```

### Services
```bash
# Enable optional services
REDIS_ENABLED=true
MINIO_ENABLED=true
MONITORING_ENABLED=true
NSELF_ADMIN_ENABLED=true

# Enable email
MAILPIT_ENABLED=true

# Enable search
MEILISEARCH_ENABLED=true
```

### Multi-Tenancy
```bash
TENANT_ISOLATION_ENABLED=true
TENANT_COLUMN_NAME=organization_id
```

---

## DNS Records

### Single Domain
```
Type    Name    Value               TTL
A       @       your-server-ip      300
A       *       your-server-ip      300
```

### Specific Subdomains
```
Type    Name    Value               TTL
A       api     your-server-ip      300
A       auth    your-server-ip      300
A       admin   your-server-ip      300
```

### Wildcard
```
Type    Name    Value               TTL
A       *       your-server-ip      300
```

### Domain Verification
```
Type    Name                Value                   TTL
TXT     _nself-verify      "verification-token"     300
```

---

## Stripe Test Cards

### Successful Payments
```
4242 4242 4242 4242  # Visa
5555 5555 5555 4444  # Mastercard
```

### Failed Payments
```
4000 0000 0000 0002  # Declined
4000 0000 0000 9995  # Insufficient funds
```

**Expiry**: Any future date (12/34)
**CVC**: Any 3 digits (123)

---

## Docker Commands

```bash
# View running containers
docker ps

# View all containers
docker ps -a

# View logs
docker logs <container-name>

# Follow logs
docker logs -f <container-name>

# Restart container
docker restart <container-name>

# Stop all
docker stop $(docker ps -q)

# Clean up
docker system prune -a
```

---

## Troubleshooting Quick Fixes

### Services not starting
```bash
nself doctor
nself logs
nself restart
```

### Port conflicts
```bash
lsof -i :5432  # Check port
AUTO_FIX=true nself build
```

### Database issues
```bash
nself db query "SELECT 1"  # Test connection
nself restart postgres
```

### SSL issues
```bash
nself ssl check myapp.com
nself ssl renew myapp.com
```

### Stripe sync issues
```bash
nself plugin stripe sync --verbose
nself plugin stripe webhook list
```

### DNS not resolving
```bash
dig app.myapp.com A
nslookup app.myapp.com
# Wait for propagation (5 min - 48 hours)
```

---

## File Locations

```
project/
├── .env                        # Main config
├── .env.secrets                # Sensitive data
├── schema.dbml                 # Database schema
├── docker-compose.yml          # Generated
├── nginx/
│   ├── nginx.conf
│   └── sites/
│       └── *.conf
├── postgres/
│   ├── init/
│   └── migrations/
├── ssl/
│   ├── cert.pem
│   └── key.pem
├── branding/                   # White-label
│   ├── config.json
│   ├── logos/
│   └── themes/
└── nself/
    ├── migrations/
    ├── seeds/
    └── backups/
```

---

## URLs by Environment

### Development
```
https://api.local.nself.org      (GraphQL)
https://auth.local.nself.org     (Auth)
https://admin.local.nself.org    (Admin)
```

### Production
```
https://api.myapp.com            (GraphQL)
https://auth.myapp.com           (Auth)
https://admin.myapp.com          (Admin)
```

---

## Time Estimates

| Task | Time |
|------|------|
| Install nself | 2 min |
| Create project | 3 min |
| Build & start | 2 min (+ 2-5 min first time) |
| Apply schema | 2 min |
| Configure Stripe | 5 min |
| Add custom domain | 10 min (+ DNS propagation) |
| Deploy to production | 5-10 min |

---

## Support Resources

- **Docs**: https://docs.nself.org
- **GitHub**: https://github.com/nself-org/cli
- **Discord**: https://discord.gg/nself
- **Email**: support@nself.org

---

**Print this page for quick reference while following tutorials!**
