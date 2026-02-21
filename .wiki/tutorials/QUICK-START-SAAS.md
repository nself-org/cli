# Quick Start: Build a SaaS in 15 Minutes

Complete step-by-step guide to launch a production-ready SaaS platform with billing, multi-tenancy, and white-label capabilities.

**Time Estimate**: 15-20 minutes
**Difficulty**: Beginner
**Prerequisites**: Docker Desktop installed

> **Note:** As of v0.9.6, commands have been consolidated. This guide uses the new v1.0 command structure where applicable:
> - `nself env` → `nself config env`
> - `nself scale` → `nself perf scale`
> - Other commands referenced use their consolidated forms

---

## What You'll Build

By the end of this tutorial, you'll have:
- Multi-tenant SaaS backend with database
- Stripe billing integration (subscriptions & usage-based)
- White-label customization per tenant
- Authentication & authorization
- Production-ready deployment

```
Timeline:
[0-5 min]   Install nself & initialize project
[5-10 min]  Configure billing & multi-tenancy
[10-15 min] Deploy to production
[15-20 min] Test & verify
```

---

## Step 1: Install nself (2 minutes)

### Install via curl

```bash
curl -sSL https://install.nself.org | bash
```

### Verify installation

```bash
nself version
# Expected: nself v0.4.8 or later
```

### Alternative: Manual installation

```bash
git clone https://github.com/nself-org/cli.git ~/.nself
echo 'export PATH="$PATH:$HOME/.nself/bin"' >> ~/.bashrc
source ~/.bashrc
```

---

## Step 2: Create Project with SaaS Template (3 minutes)

### Initialize with SaaS template

```bash
mkdir my-saas && cd my-saas
nself init --template saas
```

**Answer the prompts**:
```
Project name: my-saas
Environment: dev
Base domain: local.nself.org (default for local)
Enable monitoring? Yes
```

### What gets created

```
my-saas/
├── .env                 # Configuration
├── .env.secrets         # Auto-generated secrets
├── schema.dbml          # SaaS database schema (auto-generated)
└── nself/
```

### Review the generated schema

```bash
cat schema.dbml
```

**Pre-configured SaaS schema includes**:
- `organizations` - Tenant organizations
- `organization_members` - Team members
- `subscriptions` - Billing subscriptions
- `subscription_usage` - Usage tracking
- `invoices` - Invoice history
- `payment_methods` - Saved payment methods
- `billing_events` - Billing event log

---

## Step 3: Build and Start (2 minutes)

### Generate configurations

```bash
nself build
```

**What happens**:
1. Generates `docker-compose.yml` (25+ services)
2. Creates nginx configurations
3. Sets up SSL certificates
4. Initializes database schema
5. Configures Hasura GraphQL

### Start all services

```bash
nself start
```

**First start downloads Docker images (2-5 minutes)**

### Check status

```bash
nself status
```

**Expected output**:
```
Service Status:
  postgres     ✓ healthy
  hasura       ✓ healthy
  auth         ✓ healthy
  nginx        ✓ healthy
  redis        ✓ healthy
  minio        ✓ healthy
  monitoring   ✓ 10/10 services healthy
```

### View all URLs

```bash
nself urls
```

**Key URLs**:
- API: https://api.local.nself.org
- Auth: https://auth.local.nself.org
- Admin: https://admin.local.nself.org
- Grafana: https://grafana.local.nself.org

---

## Step 4: Apply Database Schema (2 minutes)

### Apply the SaaS schema

```bash
nself db schema apply schema.dbml
```

**This command**:
1. Creates SQL migration from DBML
2. Runs migration
3. Generates sample data
4. Seeds test organizations

### Verify tables

```bash
nself db shell
```

```sql
-- List all tables
\dt

-- Check organizations table
SELECT * FROM organizations;

-- Check subscriptions table
SELECT * FROM subscriptions;
```

Exit with `\q`

---

## Step 5: Configure Stripe Billing (3 minutes)

### Get your Stripe keys

1. Go to [Stripe Dashboard](https://dashboard.stripe.com)
2. Create account (free)
3. Get test API keys:
   - Publishable key: `pk_test_...`
   - Secret key: `sk_test_PLACEHOLDER...`

### Install Stripe plugin

```bash
nself plugin install stripe
```

### Configure Stripe

Edit `.env` and add:

```bash
# Stripe Configuration
STRIPE_API_KEY=sk_test_PLACEHOLDER_secret_key_here
STRIPE_PUBLISHABLE_KEY=pk_test_your_publishable_key_here
STRIPE_WEBHOOK_SECRET=whsec_your_webhook_secret_here
```

### Create Stripe products

```bash
# Create a subscription product
nself plugin stripe products create \
  --name "Pro Plan" \
  --price 29.99 \
  --interval month

# Create usage-based product
nself plugin stripe products create \
  --name "API Calls" \
  --price 0.01 \
  --type usage
```

### Sync Stripe data

```bash
nself plugin stripe sync
```

**Syncs**:
- Customers
- Products & Prices
- Subscriptions
- Invoices
- Payment methods

---

## Step 6: Enable Multi-Tenancy (2 minutes)

### Configure tenant isolation

Edit `.env`:

```bash
# Multi-Tenancy Settings
HASURA_GRAPHQL_JWT_SECRET='{"type":"HS256","key":"your-jwt-secret-here"}'
HASURA_GRAPHQL_ENABLE_CONSOLE=true
HASURA_GRAPHQL_ENABLE_TELEMETRY=false

# Tenant isolation
TENANT_ISOLATION_ENABLED=true
TENANT_COLUMN_NAME=organization_id
```

### Set up tenant permissions

```bash
# Access Hasura console
open https://api.local.nself.org
```

**Configure row-level security**:
1. Go to Data → organizations → Permissions
2. For "user" role:
   - Select: `{"organization_members":{"user_id":{"_eq":"X-Hasura-User-Id"}}}`
   - Insert: `{"owner_id":{"_eq":"X-Hasura-User-Id"}}`
   - Update: Same as Select
   - Delete: Same as Select

**This ensures users only see their organization's data**

---

## Step 7: Configure White-Label Branding (2 minutes)

### Initialize white-label system

```bash
nself whitelabel init
```

### Create default brand

```bash
nself whitelabel branding create "My SaaS" \
  --tagline "Your tagline here"
```

### Set brand colors

```bash
nself whitelabel branding set-colors \
  --primary #0066cc \
  --secondary #00cc66 \
  --accent #ff6600
```

### Upload logo (optional)

```bash
nself whitelabel logo upload ./logo.png --type main
```

### Customize email templates

```bash
# List available templates
nself whitelabel email list

# Customize welcome email
nself whitelabel email edit welcome
```

**Available templates**:
- `welcome` - Welcome new users
- `password-reset` - Password reset
- `verify-email` - Email verification
- `invite` - Team invitations
- `subscription-created` - New subscription
- `subscription-cancelled` - Cancellation
- `invoice-paid` - Payment confirmation
- `payment-failed` - Failed payment

---

## Step 8: Deploy to Production (5 minutes)

### Create production environment

```bash
nself config env create prod
```

### Configure production server

Edit `.env.prod`:

```bash
ENV=prod
PROJECT_NAME=my-saas
BASE_DOMAIN=myapp.com

# Production database
POSTGRES_DB=myapp_prod
POSTGRES_USER=postgres
POSTGRES_PASSWORD=generate-secure-password-here

# Production Stripe (LIVE keys)
STRIPE_API_KEY=sk_live_your_live_key_here
STRIPE_PUBLISHABLE_KEY=pk_live_your_live_key_here

# Security
HASURA_GRAPHQL_ADMIN_SECRET=generate-secure-secret-here
AUTH_JWT_SECRET=generate-jwt-secret-here

# Disable dev features
HASURA_GRAPHQL_ENABLE_CONSOLE=false
HASURA_GRAPHQL_DEV_MODE=false
```

### Set up production server

**Requirements**:
- Ubuntu 20.04+ or Debian 11+
- 2GB RAM minimum
- Docker & Docker Compose installed
- Domain with DNS configured

**DNS Configuration**:
```
A     @              -> your-server-ip
A     *              -> your-server-ip
CNAME api            -> myapp.com
CNAME auth           -> myapp.com
CNAME admin          -> myapp.com
```

### Configure server.json

Create `.environments/prod/server.json`:

```json
{
  "host": "your-server-ip",
  "user": "root",
  "port": 22,
  "path": "/var/www/my-saas",
  "sshKey": "~/.ssh/id_rsa"
}
```

### Deploy to production

```bash
nself deploy prod
```

**Deployment process**:
1. Connects via SSH
2. Uploads project files
3. Installs dependencies
4. Runs migrations
5. Starts services
6. Provisions SSL certificates

### Verify deployment

```bash
nself deploy status prod
```

---

## Step 9: Test Your SaaS (3 minutes)

### Create test organization

Using Hasura console (https://api.myapp.com):

```graphql
mutation CreateOrganization {
  insert_organizations_one(object: {
    name: "Acme Corp"
    slug: "acme-corp"
    plan: "pro"
    status: "active"
  }) {
    id
    name
    slug
  }
}
```

### Create subscription

```graphql
mutation CreateSubscription {
  insert_subscriptions_one(object: {
    organization_id: "org-id-from-above"
    stripe_subscription_id: "sub_xxxxx"
    plan: "pro"
    status: "active"
    current_period_start: "2026-01-01"
    current_period_end: "2026-02-01"
    price_amount: 2999
    currency: "usd"
  }) {
    id
    status
  }
}
```

### Track usage

```graphql
mutation TrackUsage {
  insert_subscription_usage_one(object: {
    subscription_id: "sub-id-from-above"
    metric_name: "api_calls"
    quantity: 1000
    timestamp: "now()"
  }) {
    id
    quantity
  }
}
```

### Query revenue

```graphql
query GetRevenue {
  subscriptions_aggregate(where: {status: {_eq: "active"}}) {
    aggregate {
      sum {
        price_amount
      }
      count
    }
  }
}
```

---

## Common Tasks

### Check billing status

```bash
# Subscription stats
nself plugin stripe subscriptions stats

# Recent invoices
nself plugin stripe invoices list --limit 10

# MRR (Monthly Recurring Revenue)
nself db query "SELECT SUM(price_amount/100) as mrr FROM subscriptions WHERE status = 'active'"
```

### Monitor usage

```bash
# Usage by organization
nself db query "
  SELECT o.name, SUM(u.quantity) as total_usage
  FROM subscription_usage u
  JOIN subscriptions s ON u.subscription_id = s.id
  JOIN organizations o ON s.organization_id = o.id
  WHERE u.timestamp > NOW() - INTERVAL '30 days'
  GROUP BY o.name
  ORDER BY total_usage DESC
"
```

### View logs

```bash
# All logs
nself logs

# Specific service
nself logs hasura
nself logs auth

# Follow logs in real-time
nself logs -f
```

---

## Webhook Setup (Important!)

### Stripe webhooks

1. **Go to Stripe Dashboard > Webhooks**
2. **Add endpoint**: `https://myapp.com/webhooks/stripe`
3. **Select events**:
   - `customer.created`, `customer.updated`
   - `subscription.created`, `subscription.updated`, `subscription.deleted`
   - `invoice.paid`, `invoice.payment_failed`
   - `payment_intent.succeeded`, `payment_intent.payment_failed`

4. **Copy webhook secret** to `.env`:
```bash
STRIPE_WEBHOOK_SECRET=whsec_your_webhook_secret_here
```

5. **Rebuild and restart**:
```bash
nself build && nself restart
```

### Test webhooks

```bash
# List recent webhook events
nself plugin stripe webhook list

# Check webhook health
nself plugin stripe webhook stats

# Retry failed webhook
nself plugin stripe webhook retry evt_xxxxx
```

---

## Architecture Diagram

```
                           ┌─────────────────────┐
                           │   Load Balancer     │
                           │   (nginx + SSL)     │
                           └──────────┬──────────┘
                                      │
                    ┌─────────────────┼─────────────────┐
                    │                 │                 │
            ┌───────▼────────┐ ┌─────▼──────┐ ┌───────▼────────┐
            │  GraphQL API   │ │    Auth    │ │   Admin UI     │
            │   (Hasura)     │ │  (nHost)   │ │   (nself)      │
            └───────┬────────┘ └─────┬──────┘ └───────┬────────┘
                    │                │                 │
            ┌───────▼────────────────▼─────────────────▼────────┐
            │              PostgreSQL Database                   │
            │  ┌──────────────────────────────────────────────┐ │
            │  │ Organizations │ Subscriptions │ Usage        │ │
            │  │ Members      │ Invoices     │ Events        │ │
            │  └──────────────────────────────────────────────┘ │
            └────────────────────────────────────────────────────┘
                    │                                    │
            ┌───────▼────────┐                  ┌───────▼────────┐
            │  Stripe API    │                  │  MinIO Storage │
            │  (Billing)     │                  │  (Files/Logos) │
            └────────────────┘                  └────────────────┘
```

---

## Troubleshooting

### Services not starting

```bash
# Diagnose issues
nself doctor

# Check Docker
docker ps

# View logs
nself logs postgres
nself logs hasura
```

### Database connection errors

```bash
# Verify database is running
nself status postgres

# Check connection
nself db query "SELECT 1"

# Restart database
nself restart postgres
```

### Stripe sync issues

```bash
# Verify API key
curl -u sk_test_PLACEHOLDER: https://api.stripe.com/v1/customers?limit=1

# Check plugin status
nself plugin status stripe

# Verbose sync
nself plugin stripe sync --verbose
```

### SSL certificate issues

```bash
# Check certificate
nself ssl check myapp.com

# Renew certificate
nself ssl renew myapp.com

# Check nginx config
nself config validate nginx
```

### Port conflicts

```bash
# Check what's using ports
lsof -i :5432  # PostgreSQL
lsof -i :8080  # Hasura
lsof -i :443   # nginx

# Auto-fix port conflicts
AUTO_FIX=true nself build
```

---

## Next Steps

### 1. Customize Your SaaS

- **[Database Workflow Guide](../guides/DATABASE-WORKFLOW.md)** - Extend your schema
- **[Custom Services](../services/SERVICES_CUSTOM.md)** - Add microservices
- **[White-Label System](../features/WHITELABEL-SYSTEM.md)** - Full customization guide

### 2. Add Features

```bash
# Enable search
nself service enable meilisearch

# Enable email
nself service enable mailpit

# Enable functions
nself service enable functions
```

### 3. Implement Frontend

**Suggested stack**:
- React + TypeScript
- Apollo Client (GraphQL)
- TailwindCSS
- nHost SDK (authentication)

**Quick setup**:
```bash
npx create-react-app frontend --template typescript
cd frontend
npm install @apollo/client @nhost/react graphql
```

### 4. Production Checklist

- [ ] Use live Stripe keys
- [ ] Configure production database backups
- [ ] Set up monitoring alerts
- [ ] Configure custom domain SSL
- [ ] Enable rate limiting
- [ ] Configure CORS
- [ ] Set up CDN
- [ ] Configure backup schedule
- [ ] Document API endpoints
- [ ] Set up error tracking

---

## Cost Estimate

**Development (Local)**:
- nself: Free
- Docker: Free
- Stripe: Free (test mode)
- **Total**: $0/month

**Production (Small)**:
- VPS (2GB RAM): $10-20/month
- Domain: $12/year
- SSL: Free (Let's Encrypt)
- Stripe: 2.9% + $0.30 per transaction
- **Total**: ~$15/month + transaction fees

**Production (Medium)**:
- VPS (4GB RAM): $20-40/month
- Domain: $12/year
- SSL: Free
- Stripe: 2.9% + $0.30 per transaction
- **Total**: ~$25-40/month + transaction fees

---

## Scaling Your SaaS

### Horizontal Scaling

```bash
# Scale specific services
nself perf scale hasura --replicas 3
nself perf scale auth --replicas 2

# Enable load balancing
LOAD_BALANCER_ENABLED=true nself build
```

### Vertical Scaling

Edit `.env`:
```bash
# Increase PostgreSQL resources
POSTGRES_SHARED_BUFFERS=256MB
POSTGRES_MAX_CONNECTIONS=200
POSTGRES_WORK_MEM=8MB

# Increase Hasura resources
HASURA_GRAPHQL_MAX_CACHE_SIZE=1000
HASURA_GRAPHQL_CONNECTIONS_PER_READ_REPLICA=5
```

### Database Optimization

```bash
# Analyze query performance
nself db analyze

# Add indexes
nself db query "CREATE INDEX idx_org_members ON organization_members(organization_id)"

# Configure connection pooling
PGBOUNCER_ENABLED=true nself build
```

---

## Related Tutorials

- **[Quick Start: B2B Platform](QUICK-START-B2B.md)** - B2B-specific setup
- **[Quick Start: Marketplace](QUICK-START-MARKETPLACE.md)** - Multi-vendor platforms
- **[Quick Start: Agency](QUICK-START-AGENCY.md)** - Agency/reseller setup
- **[Stripe Integration](STRIPE-INTEGRATION.md)** - Complete Stripe guide
- **[Custom Domains](CUSTOM-DOMAINS.md)** - Domain configuration

---

## Support

- **Documentation**: https://docs.nself.org
- **GitHub Issues**: https://github.com/nself-org/cli/issues
- **Discord**: https://discord.gg/nself
- **Email**: support@nself.org

---

**Congratulations! You've built a production-ready SaaS in 15 minutes.**

*Time to customize and launch your product!*
