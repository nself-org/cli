# Building a Multi-Tenant SaaS Application

A comprehensive guide to building a production-ready multi-tenant SaaS application with nself.

**Time Required:** 3-4 hours
**Difficulty:** Intermediate to Advanced
**What You'll Build:** Complete SaaS platform with tenant isolation, billing, and team management

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Database Design](#database-design)
3. [Tenant Isolation](#tenant-isolation)
4. [Billing Integration](#billing-integration)
5. [Team Management](#team-management)
6. [Admin Dashboard](#admin-dashboard)
7. [Deployment](#deployment)
8. [Scaling](#scaling)

---

## Architecture Overview

### What is Multi-Tenancy?

Multi-tenancy is an architecture where a single application serves multiple customers (tenants), with each tenant's data completely isolated from others.

**Benefits:**
- Lower infrastructure costs (shared resources)
- Easier maintenance (single codebase)
- Faster onboarding (no new deployment per customer)
- Centralized updates

**Challenges:**
- Data isolation (security critical)
- Performance isolation (one tenant can't affect others)
- Customization (per-tenant configuration)
- Billing complexity (usage tracking per tenant)

### Multi-Tenancy Approaches

**1. Separate Databases** (Not using this)
- Pros: Complete isolation, easy backup
- Cons: Expensive, maintenance overhead

**2. Separate Schemas** (Not using this)
- Pros: Good isolation, moderate cost
- Cons: Schema management complexity

**3. Shared Schema with Row-Level Security** (✓ We use this)
- Pros: Cost-effective, simple maintenance
- Cons: Requires careful RLS configuration

### Our Architecture

```
┌────────────────────────────────────────────────────────┐
│                    User Browser                        │
└────────────┬───────────────────────────────────────────┘
             │
             ▼
┌────────────────────────────────────────────────────────┐
│                   Next.js Frontend                     │
│  - Authentication (nHost)                              │
│  - Tenant context in JWT                               │
│  - Role-based UI                                       │
└────────────┬───────────────────────────────────────────┘
             │
             ▼
      ┌──────────────┐
      │    Nginx     │
      │  (Routing)   │
      └──────┬───────┘
             │
    ┌────────┴────────┬──────────────┬───────────┐
    ▼                 ▼              ▼           ▼
┌─────────┐    ┌──────────┐   ┌─────────┐  ┌─────────┐
│ Hasura  │    │   Auth   │   │   API   │  │ Billing │
│ GraphQL │    │ Service  │   │ Service │  │ Service │
└────┬────┘    └────┬─────┘   └────┬────┘  └────┬────┘
     │              │              │            │
     └──────────────┴──────────────┴────────────┘
                    │
              ┌─────▼──────┐
              │ PostgreSQL │
              │   (with    │
              │    RLS)    │
              └────────────┘
```

---

## Database Design

### Core Schema

```sql
-- Enable extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================================
-- TENANTS
-- ============================================================================

CREATE TABLE tenants (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  slug TEXT UNIQUE NOT NULL,  -- Used in URLs: acme.app.com
  name TEXT NOT NULL,
  settings JSONB DEFAULT '{}',
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_tenants_slug ON tenants(slug);

COMMENT ON TABLE tenants IS 'Organizations/companies using the platform';

-- ============================================================================
-- TENANT USERS (Many-to-Many)
-- ============================================================================

CREATE TYPE tenant_role AS ENUM ('owner', 'admin', 'member', 'guest');

CREATE TABLE tenant_users (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  role tenant_role NOT NULL DEFAULT 'member',
  permissions JSONB DEFAULT '[]',  -- Custom permissions
  invited_by UUID REFERENCES auth.users(id),
  joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

  UNIQUE(tenant_id, user_id)
);

CREATE INDEX idx_tenant_users_tenant ON tenant_users(tenant_id);
CREATE INDEX idx_tenant_users_user ON tenant_users(user_id);

COMMENT ON TABLE tenant_users IS 'Users belonging to tenants with roles';

-- ============================================================================
-- INVITATIONS
-- ============================================================================

CREATE TYPE invitation_status AS ENUM ('pending', 'accepted', 'expired', 'revoked');

CREATE TABLE tenant_invitations (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  email TEXT NOT NULL,
  role tenant_role NOT NULL DEFAULT 'member',
  invited_by UUID NOT NULL REFERENCES auth.users(id),
  status invitation_status DEFAULT 'pending',
  token TEXT UNIQUE NOT NULL DEFAULT encode(gen_random_bytes(32), 'hex'),
  expires_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() + INTERVAL '7 days',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

  UNIQUE(tenant_id, email)
);

CREATE INDEX idx_invitations_token ON tenant_invitations(token);
CREATE INDEX idx_invitations_email ON tenant_invitations(email);

COMMENT ON TABLE tenant_invitations IS 'Pending user invitations';

-- ============================================================================
-- SUBSCRIPTION PLANS
-- ============================================================================

CREATE TABLE subscription_plans (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  name TEXT NOT NULL UNIQUE,
  description TEXT,
  stripe_product_id TEXT UNIQUE,
  stripe_price_id TEXT UNIQUE,
  price DECIMAL(10,2) NOT NULL,
  currency TEXT DEFAULT 'usd',
  interval TEXT NOT NULL DEFAULT 'month',  -- month, year
  features JSONB DEFAULT '[]',
  limits JSONB DEFAULT '{}',  -- {users: 10, projects: 50, storage: 1000}
  is_active BOOLEAN DEFAULT true,
  sort_order INTEGER DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_plans_active ON subscription_plans(is_active, sort_order);

COMMENT ON TABLE subscription_plans IS 'Available subscription plans';

-- Sample plans
INSERT INTO subscription_plans (name, price, interval, features, limits)
VALUES
  ('Starter', 29.00, 'month',
   '["5 team members", "10 projects", "Email support"]',
   '{"users": 5, "projects": 10, "storage_gb": 5}'),
  ('Professional', 99.00, 'month',
   '["Unlimited members", "50 projects", "Priority support", "Advanced analytics"]',
   '{"users": -1, "projects": 50, "storage_gb": 50}'),
  ('Enterprise', 299.00, 'month',
   '["Everything in Pro", "Unlimited projects", "Dedicated support", "SLA", "SSO"]',
   '{"users": -1, "projects": -1, "storage_gb": 500}');

-- ============================================================================
-- TENANT SUBSCRIPTIONS
-- ============================================================================

CREATE TYPE subscription_status AS ENUM (
  'trial', 'active', 'past_due', 'canceled', 'unpaid'
);

CREATE TABLE tenant_subscriptions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  plan_id UUID NOT NULL REFERENCES subscription_plans(id),
  stripe_subscription_id TEXT UNIQUE,
  stripe_customer_id TEXT,
  status subscription_status DEFAULT 'trial',
  trial_ends_at TIMESTAMP WITH TIME ZONE,
  current_period_start TIMESTAMP WITH TIME ZONE,
  current_period_end TIMESTAMP WITH TIME ZONE,
  cancel_at TIMESTAMP WITH TIME ZONE,
  canceled_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

  UNIQUE(tenant_id)  -- One subscription per tenant
);

CREATE INDEX idx_subscriptions_tenant ON tenant_subscriptions(tenant_id);
CREATE INDEX idx_subscriptions_status ON tenant_subscriptions(status);

COMMENT ON TABLE tenant_subscriptions IS 'Active subscriptions for tenants';

-- ============================================================================
-- BILLING USAGE
-- ============================================================================

CREATE TABLE billing_usage (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  metric TEXT NOT NULL,  -- api_calls, storage_gb, emails_sent, etc.
  quantity DECIMAL(20,4) NOT NULL DEFAULT 0,
  period_start TIMESTAMP WITH TIME ZONE NOT NULL,
  period_end TIMESTAMP WITH TIME ZONE NOT NULL,
  metadata JSONB DEFAULT '{}',
  reported_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_usage_tenant ON billing_usage(tenant_id, period_start);
CREATE INDEX idx_usage_metric ON billing_usage(metric, period_start);

COMMENT ON TABLE billing_usage IS 'Usage tracking for billing';

-- ============================================================================
-- TENANT DATA TABLES (Example: Projects)
-- ============================================================================

CREATE TABLE projects (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT,
  owner_id UUID NOT NULL REFERENCES auth.users(id),
  settings JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_projects_tenant ON projects(tenant_id);

-- ============================================================================
-- ROW LEVEL SECURITY (RLS)
-- ============================================================================

-- Enable RLS on all tenant-specific tables
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_users ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing_usage ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects ENABLE ROW LEVEL SECURITY;

-- Drop existing policies if any
DROP POLICY IF EXISTS tenant_isolation ON tenants;
DROP POLICY IF EXISTS tenant_isolation ON tenant_users;
DROP POLICY IF EXISTS tenant_isolation ON projects;

-- Tenants: Users can only see tenants they belong to
CREATE POLICY tenant_isolation ON tenants
  FOR ALL
  USING (
    id IN (
      SELECT tenant_id
      FROM tenant_users
      WHERE user_id = current_setting('hasura.user.id', true)::UUID
    )
  );

-- Tenant Users: Users can see other users in their tenants
CREATE POLICY tenant_isolation ON tenant_users
  FOR ALL
  USING (
    tenant_id IN (
      SELECT tenant_id
      FROM tenant_users
      WHERE user_id = current_setting('hasura.user.id', true)::UUID
    )
  );

-- Projects: Users can only see projects in their tenants
CREATE POLICY tenant_isolation ON projects
  FOR ALL
  USING (
    tenant_id IN (
      SELECT tenant_id
      FROM tenant_users
      WHERE user_id = current_setting('hasura.user.id', true)::UUID
    )
  );

-- Invitations: Tenant-specific
CREATE POLICY tenant_isolation ON tenant_invitations
  FOR ALL
  USING (
    tenant_id IN (
      SELECT tenant_id
      FROM tenant_users
      WHERE user_id = current_setting('hasura.user.id', true)::UUID
    )
  );

-- Subscriptions: Tenant-specific
CREATE POLICY tenant_isolation ON tenant_subscriptions
  FOR ALL
  USING (
    tenant_id IN (
      SELECT tenant_id
      FROM tenant_users
      WHERE user_id = current_setting('hasura.user.id', true)::UUID
    )
  );

-- Usage: Tenant-specific
CREATE POLICY tenant_isolation ON billing_usage
  FOR ALL
  USING (
    tenant_id IN (
      SELECT tenant_id
      FROM tenant_users
      WHERE user_id = current_setting('hasura.user.id', true)::UUID
    )
  );
```

### Understanding the Schema

**Key Tables:**

1. **tenants** - Organizations using your platform
2. **tenant_users** - Many-to-many: users can belong to multiple tenants
3. **tenant_invitations** - Pending invitations with expiry
4. **subscription_plans** - Your pricing tiers
5. **tenant_subscriptions** - Active subscriptions per tenant
6. **billing_usage** - Track usage for billing
7. **projects** - Example tenant data (your actual business data)

---

## Tenant Isolation

### How RLS Works

Row Level Security ensures users can ONLY see data from their tenants.

**Without RLS:**
```sql
-- User can see ALL projects (security risk!)
SELECT * FROM projects;
```

**With RLS:**
```sql
-- User only sees projects from their tenant
SELECT * FROM projects;
-- Automatically filtered by RLS policy
```

### Setting Tenant Context

Hasura automatically sets the user context via JWT:

```json
{
  "sub": "user-uuid",
  "https://hasura.io/jwt/claims": {
    "x-hasura-user-id": "user-uuid",
    "x-hasura-role": "user",
    "x-hasura-default-role": "user",
    "x-hasura-allowed-roles": ["user", "admin"]
  }
}
```

RLS policies use this context:

```sql
-- Access current user ID
current_setting('hasura.user.id', true)::UUID
```

### Testing Tenant Isolation

```sql
-- Temporarily set user context for testing
SET hasura.user.id = 'user-uuid-here';

-- Now queries respect RLS
SELECT * FROM projects;
-- Only shows projects from user's tenants

-- Try another user
SET hasura.user.id = 'different-user-uuid';
SELECT * FROM projects;
-- Shows different projects!
```

### Common Pitfalls

❌ **Forgetting to enable RLS:**
```sql
-- WRONG: Table without RLS
CREATE TABLE sensitive_data (...);
-- Anyone can query this!
```

✅ **Always enable RLS:**
```sql
-- RIGHT: Enable RLS
ALTER TABLE sensitive_data ENABLE ROW LEVEL SECURITY;

-- Create policy
CREATE POLICY tenant_isolation ON sensitive_data
  USING (tenant_id IN (...));
```

❌ **Admin bypass:**
```sql
-- WRONG: Admin can see everything
CREATE POLICY admin_all ON projects
  FOR ALL TO admin
  USING (true);
-- This breaks tenant isolation!
```

✅ **Admin still respects tenants:**
```sql
-- RIGHT: Admin has more permissions but same tenant isolation
CREATE POLICY admin_manage ON projects
  FOR UPDATE TO admin
  USING (tenant_id IN (...));
```

---

## Billing Integration

### Stripe Setup

**1. Install Stripe CLI:**
```bash
# macOS
brew install stripe/stripe-cli/stripe

# Login
stripe login
```

**2. Create Products in Stripe:**
```bash
# Starter plan
stripe products create \
  --name="Starter" \
  --description="Perfect for small teams"

stripe prices create \
  --product=prod_XXX \
  --unit-amount=2900 \
  --currency=usd \
  --recurring='{"interval":"month"}'
```

**3. Update database with Stripe IDs:**
```sql
UPDATE subscription_plans
SET stripe_product_id = 'prod_XXX',
    stripe_price_id = 'price_XXX'
WHERE name = 'Starter';
```

### Subscription Flow

**Create Checkout Session:**

```typescript
// api/src/billing/billing.service.ts
import Stripe from 'stripe'

const stripe = new Stripe(process.env.STRIPE_SECRET_KEY)

async function createCheckoutSession(tenantId: string, planId: string) {
  // Get plan details
  const plan = await getPlan(planId)

  // Create or get Stripe customer
  let customer = await getStripeCustomer(tenantId)
  if (!customer) {
    customer = await stripe.customers.create({
      metadata: { tenant_id: tenantId }
    })
  }

  // Create checkout session
  const session = await stripe.checkout.sessions.create({
    customer: customer.id,
    mode: 'subscription',
    payment_method_types: ['card'],
    line_items: [{
      price: plan.stripe_price_id,
      quantity: 1
    }],
    success_url: `${process.env.APP_URL}/billing/success?session_id={CHECKOUT_SESSION_ID}`,
    cancel_url: `${process.env.APP_URL}/billing/cancel`,
    metadata: {
      tenant_id: tenantId,
      plan_id: planId
    }
  })

  return session.url
}
```

**Handle Webhooks:**

```typescript
// api/src/webhooks/stripe.controller.ts
@Controller('webhooks')
export class StripeWebhookController {
  @Post('stripe')
  async handleStripeWebhook(
    @Req() req: RawBodyRequest<Request>,
    @Headers('stripe-signature') signature: string
  ) {
    const event = stripe.webhooks.constructEvent(
      req.rawBody,
      signature,
      process.env.STRIPE_WEBHOOK_SECRET
    )

    switch (event.type) {
      case 'checkout.session.completed':
        await this.handleCheckoutComplete(event.data.object)
        break

      case 'customer.subscription.created':
        await this.handleSubscriptionCreated(event.data.object)
        break

      case 'customer.subscription.updated':
        await this.handleSubscriptionUpdated(event.data.object)
        break

      case 'customer.subscription.deleted':
        await this.handleSubscriptionCanceled(event.data.object)
        break

      case 'invoice.paid':
        await this.handleInvoicePaid(event.data.object)
        break

      case 'invoice.payment_failed':
        await this.handleInvoicePaymentFailed(event.data.object)
        break
    }

    return { received: true }
  }

  private async handleSubscriptionCreated(subscription: Stripe.Subscription) {
    const tenantId = subscription.metadata.tenant_id

    await db.query(`
      INSERT INTO tenant_subscriptions (
        tenant_id,
        plan_id,
        stripe_subscription_id,
        stripe_customer_id,
        status,
        current_period_start,
        current_period_end
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
    `, [
      tenantId,
      subscription.metadata.plan_id,
      subscription.id,
      subscription.customer,
      subscription.status,
      new Date(subscription.current_period_start * 1000),
      new Date(subscription.current_period_end * 1000)
    ])
  }
}
```

### Usage-Based Billing

**Track Usage:**

```typescript
// Track API calls
async function trackApiCall(tenantId: string, endpoint: string) {
  await db.query(`
    INSERT INTO billing_usage (tenant_id, metric, quantity, period_start, period_end, metadata)
    VALUES ($1, 'api_calls', 1, date_trunc('hour', NOW()), date_trunc('hour', NOW()) + INTERVAL '1 hour', $2)
    ON CONFLICT (tenant_id, metric, period_start)
    DO UPDATE SET quantity = billing_usage.quantity + 1
  `, [tenantId, { endpoint }])
}

// Track storage
async function trackStorage(tenantId: string, bytes: number) {
  await db.query(`
    INSERT INTO billing_usage (tenant_id, metric, quantity, period_start, period_end)
    VALUES ($1, 'storage_bytes', $2, date_trunc('day', NOW()), date_trunc('day', NOW()) + INTERVAL '1 day')
    ON CONFLICT (tenant_id, metric, period_start)
    DO UPDATE SET quantity = $2
  `, [tenantId, bytes])
}
```

**Report Usage to Stripe:**

```typescript
// Report to Stripe for billing
async function reportUsageToStripe(tenantId: string) {
  const subscription = await getActiveSubscription(tenantId)

  const usage = await db.query(`
    SELECT metric, SUM(quantity) as total
    FROM billing_usage
    WHERE tenant_id = $1
      AND period_start >= $2
      AND period_start < $3
    GROUP BY metric
  `, [
    tenantId,
    subscription.current_period_start,
    subscription.current_period_end
  ])

  for (const { metric, total } of usage.rows) {
    await stripe.subscriptionItems.createUsageRecord(
      subscription.stripe_subscription_item_id,
      { quantity: total, timestamp: Math.floor(Date.now() / 1000) }
    )
  }
}
```

---

## Team Management

### Invitation Flow

**1. Send Invitation:**

```typescript
async function inviteUser(tenantId: string, email: string, role: string, invitedBy: string) {
  // Create invitation
  const invitation = await db.query(`
    INSERT INTO tenant_invitations (tenant_id, email, role, invited_by)
    VALUES ($1, $2, $3, $4)
    RETURNING *
  `, [tenantId, email, role, invitedBy])

  // Send email
  await sendEmail({
    to: email,
    subject: 'You\'ve been invited to join a team',
    template: 'invitation',
    data: {
      tenant: await getTenant(tenantId),
      inviter: await getUser(invitedBy),
      acceptUrl: `${process.env.APP_URL}/invitations/${invitation.token}`
    }
  })

  return invitation
}
```

**2. Accept Invitation:**

```typescript
async function acceptInvitation(token: string, userId: string) {
  // Get invitation
  const invitation = await db.query(`
    SELECT * FROM tenant_invitations
    WHERE token = $1
      AND status = 'pending'
      AND expires_at > NOW()
  `, [token])

  if (!invitation) {
    throw new Error('Invalid or expired invitation')
  }

  // Add user to tenant
  await db.query(`
    INSERT INTO tenant_users (tenant_id, user_id, role, invited_by)
    VALUES ($1, $2, $3, $4)
  `, [invitation.tenant_id, userId, invitation.role, invitation.invited_by])

  // Mark invitation as accepted
  await db.query(`
    UPDATE tenant_invitations
    SET status = 'accepted'
    WHERE id = $1
  `, [invitation.id])
}
```

### Permission Checking

```typescript
// Check if user has permission
async function hasPermission(
  userId: string,
  tenantId: string,
  permission: string
): Promise<boolean> {
  const result = await db.query(`
    SELECT role, permissions
    FROM tenant_users
    WHERE user_id = $1 AND tenant_id = $2
  `, [userId, tenantId])

  if (!result.rows[0]) return false

  const { role, permissions } = result.rows[0]

  // Check role-based permissions
  const rolePermissions = {
    owner: ['*'],
    admin: ['users:*', 'projects:*', 'settings:read', 'settings:write'],
    member: ['projects:read', 'projects:write'],
    guest: ['projects:read']
  }

  // Check if permission matches
  return matchesPermission(permission, [
    ...rolePermissions[role],
    ...permissions
  ])
}

function matchesPermission(required: string, allowed: string[]): boolean {
  return allowed.some(perm => {
    if (perm === '*') return true
    if (perm.endsWith(':*')) {
      const prefix = perm.slice(0, -2)
      return required.startsWith(prefix)
    }
    return perm === required
  })
}
```

---

## Admin Dashboard

### Analytics

```typescript
// Get SaaS metrics
async function getSaasMetrics() {
  const metrics = await db.query(`
    SELECT
      COUNT(DISTINCT t.id) as total_tenants,
      COUNT(DISTINCT CASE WHEN s.status = 'active' THEN t.id END) as active_tenants,
      COUNT(DISTINCT tu.user_id) as total_users,
      SUM(CASE WHEN s.status = 'active' THEN p.price ELSE 0 END) as mrr,
      AVG(EXTRACT(EPOCH FROM (NOW() - t.created_at)) / 86400) as avg_tenant_age_days
    FROM tenants t
    LEFT JOIN tenant_subscriptions s ON s.tenant_id = t.id
    LEFT JOIN subscription_plans p ON p.id = s.plan_id
    LEFT JOIN tenant_users tu ON tu.tenant_id = t.id
  `)

  return metrics.rows[0]
}
```

**Complete guide continues in Part 2...**

---

## Quick Summary

You've learned:

✅ Multi-tenant database design with RLS
✅ Complete tenant isolation
✅ Stripe billing integration
✅ Team and invitation management
✅ Permission systems
✅ Usage tracking

**Next:** Deploy your SaaS and scale to thousands of tenants!

See the [SaaS Starter Example](../examples/projects/02-saas-starter/README.md) for complete working code.

---

**Version:** 0.9.8
**Last Updated:** January 2026
