# SaaS Starter - Multi-Tenant Application Template

A complete, production-ready SaaS application template with multi-tenancy, billing, team management, and enterprise features.

**Perfect for:** SaaS products, B2B applications, multi-tenant platforms

---

## Features

### Core Features

- ✅ **Multi-Tenancy** - Complete tenant isolation with RLS
- ✅ **Stripe Billing** - Subscriptions, invoices, usage tracking
- ✅ **Team Management** - Invite users, manage roles
- ✅ **Role-Based Access Control** - Granular permissions
- ✅ **Admin Dashboard** - Tenant management, analytics
- ✅ **API Rate Limiting** - Per-tenant quotas
- ✅ **Audit Logging** - Track all actions
- ✅ **Email Notifications** - Invites, billing updates

### Authentication

- Email/password signup
- OAuth (Google, GitHub)
- Email verification
- Password reset
- MFA (optional)
- Session management

### Billing Features

- Multiple subscription plans
- Usage-based billing
- Invoice management
- Payment method management
- Proration handling
- Billing portal
- Webhook handling

### Team Features

- User invitations
- Role management (Owner, Admin, Member)
- Permission sets
- Team settings
- User onboarding

### Admin Features

- Tenant analytics
- Revenue dashboard
- User management
- Feature flags
- System health monitoring

---

## Architecture

```
┌─────────────────────────────────────────┐
│         Next.js Frontend                │
│  (Customer App + Admin Dashboard)       │
└────────────┬────────────────────────────┘
             │
             ▼
      ┌──────────────┐
      │    Nginx     │
      └──────┬───────┘
             │
    ┌────────┴────────┬─────────────┬──────────────┐
    ▼                 ▼             ▼              ▼
┌─────────┐    ┌──────────┐   ┌─────────┐   ┌──────────┐
│ Hasura  │    │   Auth   │   │  API    │   │  Stripe  │
│ GraphQL │    │ Service  │   │ Service │   │ Webhooks │
└────┬────┘    └────┬─────┘   └────┬────┘   └────┬─────┘
     │              │              │              │
     └──────────────┴──────────────┴──────────────┘
                    │
              ┌─────▼──────┐
              │ PostgreSQL │
              │ (Multi-    │
              │  Tenant)   │
              └────────────┘
```

---

## Quick Start

### Prerequisites

- nself v0.9.9+
- Node.js 18+
- Stripe account (for billing)
- Docker & Docker Compose

### 1. Setup Project

```bash
# Clone example
cd examples/02-saas-starter/

# Copy environment
cp .env.example .env

# Edit configuration
nano .env
```

**Required Configuration:**

```bash
# Project
PROJECT_NAME=my-saas
BASE_DOMAIN=localhost

# Stripe (get from https://dashboard.stripe.com)
STRIPE_SECRET_KEY=sk_test_...
STRIPE_PUBLISHABLE_KEY=pk_test_...
STRIPE_WEBHOOK_SECRET=whsec_...

# Database
POSTGRES_PASSWORD=your-secure-password

# Hasura
HASURA_GRAPHQL_ADMIN_SECRET=your-admin-secret

# Auth
AUTH_JWT_SECRET=your-jwt-secret-min-32-chars
```

### 2. Initialize Infrastructure

```bash
# Initialize nself
nself init

# Build infrastructure
nself build

# Start services
nself start
```

### 3. Setup Database

```bash
# Apply multi-tenant schema
nself db execute --file database/schema.sql

# Create initial tenants/plans
nself db execute --file database/seeds/plans.sql
nself db execute --file database/seeds/sample-tenant.sql
```

### 4. Configure Hasura

```bash
# Apply metadata
nself service hasura metadata apply --dir hasura/metadata/

# Open console
nself admin hasura
```

### 5. Start Frontend

```bash
cd frontend/

# Install dependencies
npm install

# Start development server
npm run dev
```

### 6. Configure Stripe

```bash
# Setup products and prices in Stripe
npm run stripe:setup

# Start webhook listener (development)
stripe listen --forward-to http://localhost:8001/webhooks/stripe
```

### 7. Access Application

- **Customer App:** http://localhost:3000
- **Admin Dashboard:** http://localhost:3000/admin
- **Hasura Console:** http://api.localhost
- **API Docs:** http://localhost:8001/docs

---

## Project Structure

```
02-saas-starter/
├── .env.example
├── README.md
├── TUTORIAL.md
├── DEPLOYMENT.md
│
├── database/
│   ├── schema.sql                # Multi-tenant schema
│   ├── functions/                # Database functions
│   │   ├── tenants.sql          # Tenant management
│   │   ├── permissions.sql      # Permission helpers
│   │   └── billing.sql          # Billing calculations
│   ├── migrations/              # Schema migrations
│   └── seeds/
│       ├── plans.sql            # Subscription plans
│       └── sample-tenant.sql    # Demo tenant
│
├── hasura/
│   ├── metadata/                # Hasura configuration
│   └── migrations/              # Hasura migrations
│
├── api/                         # Custom API service (NestJS)
│   ├── src/
│   │   ├── auth/               # Auth modules
│   │   ├── billing/            # Stripe integration
│   │   ├── tenants/            # Tenant management
│   │   ├── webhooks/           # Webhook handlers
│   │   └── common/             # Shared utilities
│   ├── Dockerfile
│   └── package.json
│
├── frontend/                    # Next.js application
│   ├── src/
│   │   ├── app/
│   │   │   ├── (auth)/         # Auth pages
│   │   │   ├── (dashboard)/    # Customer dashboard
│   │   │   ├── (admin)/        # Admin dashboard
│   │   │   └── api/            # API routes
│   │   ├── components/
│   │   │   ├── ui/             # UI components
│   │   │   ├── billing/        # Billing components
│   │   │   ├── teams/          # Team components
│   │   │   └── admin/          # Admin components
│   │   ├── lib/
│   │   │   ├── graphql/        # GraphQL queries
│   │   │   ├── stripe/         # Stripe helpers
│   │   │   └── hooks/          # Custom hooks
│   │   └── types/              # TypeScript types
│   ├── public/
│   └── package.json
│
└── docs/
    ├── MULTI-TENANCY.md         # Multi-tenancy guide
    ├── BILLING.md               # Billing integration
    ├── PERMISSIONS.md           # RBAC guide
    └── API.md                   # API documentation
```

---

## Multi-Tenancy Architecture

### Tenant Isolation

Every table uses Row Level Security (RLS) with tenant_id:

```sql
-- Example: projects table
CREATE TABLE projects (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  name TEXT NOT NULL,
  ...
);

-- RLS Policy
CREATE POLICY tenant_isolation ON projects
  USING (tenant_id = current_setting('app.tenant_id')::UUID);
```

### Tenant Context

Hasura sets tenant context via JWT claims:

```json
{
  "sub": "user-uuid",
  "https://hasura.io/jwt/claims": {
    "x-hasura-tenant-id": "tenant-uuid",
    "x-hasura-role": "user",
    "x-hasura-user-id": "user-uuid"
  }
}
```

### Database Schema

**Core Tables:**

- `tenants` - Tenant organizations
- `tenant_users` - Users belonging to tenants
- `tenant_invitations` - Pending invitations
- `subscription_plans` - Available plans
- `tenant_subscriptions` - Active subscriptions
- `billing_usage` - Usage tracking
- `audit_logs` - Action history

**See:** `docs/MULTI-TENANCY.md` for complete guide

---

## Billing Integration

### Subscription Flow

1. **User Signs Up** → Creates tenant
2. **Select Plan** → Shows pricing page
3. **Add Payment** → Stripe Checkout
4. **Create Subscription** → Via webhook
5. **Activate Features** → Based on plan

### Plans

```typescript
// Example plans
const plans = [
  {
    name: 'Starter',
    price: 29,
    features: ['5 team members', '10 projects', 'Basic support']
  },
  {
    name: 'Professional',
    price: 99,
    features: ['Unlimited members', '50 projects', 'Priority support']
  },
  {
    name: 'Enterprise',
    price: 299,
    features: ['Everything', 'Dedicated support', 'SLA']
  }
]
```

### Usage Tracking

```typescript
// Track API calls, storage, etc.
await trackUsage({
  tenant_id: tenant.id,
  metric: 'api_calls',
  quantity: 1,
  metadata: { endpoint: '/api/projects' }
})
```

### Invoicing

Automatic invoice generation via Stripe webhooks:
- `invoice.created`
- `invoice.paid`
- `invoice.payment_failed`

**See:** `docs/BILLING.md` for complete guide

---

## Team Management

### Roles

| Role | Permissions |
|------|-------------|
| **Owner** | Full access, billing, delete tenant |
| **Admin** | Manage users, projects, settings |
| **Member** | View and edit projects |
| **Guest** | Read-only access |

### Invitations

```typescript
// Send invitation
const invitation = await inviteUser({
  tenant_id: tenant.id,
  email: 'user@example.com',
  role: 'member'
})

// Email sent with magic link
// User clicks → Creates account → Joins tenant
```

### Permission Checking

```typescript
// Check permission
const canEdit = await checkPermission({
  user_id: user.id,
  tenant_id: tenant.id,
  permission: 'projects:write'
})
```

**See:** `docs/PERMISSIONS.md` for complete guide

---

## Admin Dashboard

### Features

- **Analytics:**
  - MRR (Monthly Recurring Revenue)
  - Churn rate
  - Active users
  - Popular features

- **Tenant Management:**
  - View all tenants
  - Impersonate users (debugging)
  - Modify subscriptions
  - Feature flags

- **System Health:**
  - Service status
  - Database metrics
  - Error tracking
  - Performance stats

### Access

```bash
# Admin users have is_admin flag
UPDATE auth.users
SET metadata = jsonb_set(metadata, '{is_admin}', 'true')
WHERE email = 'admin@yourcompany.com';
```

Access: http://localhost:3000/admin

---

## API Documentation

### Custom Endpoints

**Tenant Management:**
- `POST /api/tenants` - Create tenant
- `GET /api/tenants/:id` - Get tenant
- `PATCH /api/tenants/:id` - Update tenant
- `DELETE /api/tenants/:id` - Delete tenant

**Billing:**
- `POST /api/billing/checkout` - Create checkout session
- `POST /api/billing/portal` - Open billing portal
- `GET /api/billing/invoices` - List invoices
- `POST /api/billing/usage` - Track usage

**Team:**
- `POST /api/teams/invite` - Invite user
- `POST /api/teams/accept` - Accept invitation
- `DELETE /api/teams/remove` - Remove member
- `PATCH /api/teams/role` - Update role

**See:** `docs/API.md` for complete reference

---

## Testing

### Unit Tests

```bash
# Backend tests
cd api/
npm test

# Frontend tests
cd frontend/
npm test
```

### Integration Tests

```bash
# Database tests
nself db test

# API tests
cd api/
npm run test:e2e
```

### Billing Tests

```bash
# Use Stripe test mode
# Test cards: https://stripe.com/docs/testing

# Test successful payment
4242 4242 4242 4242

# Test payment decline
4000 0000 0000 0002
```

---

## Deployment

### Production Checklist

- [ ] Change all secrets in `.env`
- [ ] Setup Stripe production keys
- [ ] Configure domain and SSL
- [ ] Setup monitoring
- [ ] Enable backups
- [ ] Configure email provider (replace MailPit)
- [ ] Test billing flow end-to-end
- [ ] Setup error tracking (Sentry)
- [ ] Load test application
- [ ] Verify RLS policies
- [ ] Review permissions
- [ ] Create admin users

### Deploy

```bash
# Build for production
ENV=production nself build

# Deploy to server
nself deploy push production

# Run migrations
nself deploy exec production "nself db migrate apply"

# Setup monitoring
nself deploy exec production "nself monitor setup"

# Configure SSL
nself deploy exec production "nself auth ssl cert --domain yourdomain.com"
```

**See:** `DEPLOYMENT.md` for complete guide

---

## Customization

### Adding Features

1. **Database Table:**
   ```sql
   CREATE TABLE your_table (
     id UUID PRIMARY KEY,
     tenant_id UUID NOT NULL REFERENCES tenants(id),
     ...
   );
   ```

2. **RLS Policy:**
   ```sql
   CREATE POLICY tenant_isolation ON your_table
     USING (tenant_id = current_setting('app.tenant_id')::UUID);
   ```

3. **Hasura Metadata:**
   - Track table
   - Add relationships
   - Configure permissions

4. **Frontend:**
   - Add GraphQL queries
   - Create components
   - Add routes

### Changing Plans

Edit `database/seeds/plans.sql`:

```sql
INSERT INTO subscription_plans (name, stripe_price_id, price, features)
VALUES ('Your Plan', 'price_...', 49, '{"feature1", "feature2"}');
```

### Custom Permissions

Add to `database/functions/permissions.sql`:

```sql
CREATE OR REPLACE FUNCTION can_manage_projects(user_uuid UUID, tenant_uuid UUID)
RETURNS BOOLEAN AS $$
  -- Your logic
$$ LANGUAGE plpgsql;
```

---

## Monitoring

### Metrics Tracked

- Tenant count
- Active subscriptions
- MRR
- API usage per tenant
- Error rates
- Response times
- Database performance

### Dashboards

Access Grafana: http://grafana.localhost

Pre-configured dashboards:
- SaaS Overview
- Tenant Analytics
- Billing Metrics
- System Health

---

## Troubleshooting

### Common Issues

**Issue:** Users can see other tenants' data
**Solution:** Verify RLS policies are enabled

```sql
-- Check RLS
SELECT tablename, rowsecurity
FROM pg_tables
WHERE schemaname = 'public';

-- All should have rowsecurity = true
```

**Issue:** Stripe webhooks not working
**Solution:** Verify webhook secret

```bash
# Test webhook locally
stripe listen --forward-to http://localhost:8001/webhooks/stripe

# Check logs
nself logs api | grep stripe
```

**Issue:** Invitations not sending
**Solution:** Check email configuration

```bash
# View mail logs
nself logs mailpit

# Test SMTP
nself service email test --to user@example.com
```

---

## Resources

- **Tutorial:** TUTORIAL.md
- **Multi-Tenancy Guide:** [docs/MULTI-TENANCY.md](../../../architecture/MULTI-TENANCY.md)
- **Billing Guide:** [docs/BILLING.md](../../../guides/BILLING-AND-USAGE.md)
- **API Reference:** [docs/API.md](../../../architecture/API.md)

---

## Support

- **Issues:** [GitHub Issues](https://github.com/acamarata/nself/issues)
- **Discussions:** [GitHub Discussions](https://github.com/acamarata/nself/discussions)

---

## License

MIT License - See LICENSE

---

**Version:** 0.9.8
**Difficulty:** Intermediate
**Time to Complete:** 2-4 hours
