# Quick Start: B2B Platform Setup

Build a B2B platform with organization hierarchies, team management, custom branding per client, and usage tracking.

**Time Estimate**: 20-25 minutes
**Difficulty**: Intermediate
**Prerequisites**: Docker Desktop, basic understanding of multi-tenancy

> **Note:** As of v0.9.6, commands have been consolidated. This guide uses the new v1.0 command structure:
> - `nself org` → `nself tenant org`
> - `nself whitelabel` → `nself dev whitelabel`
> - `nself env` → `nself config env`

---

## What You'll Build

A complete B2B platform with:
- Organization hierarchies (parent/child accounts)
- Team management with roles & permissions
- Custom branding per organization
- Usage tracking and billing per organization
- Admin dashboard for platform management

```
B2B Architecture:
┌──────────────────────────────────────────┐
│         Platform (Your Company)          │
├──────────────────────────────────────────┤
│  ┌────────────┐  ┌────────────┐         │
│  │  Client A  │  │  Client B  │         │
│  │  (Acme)    │  │  (TechCo)  │         │
│  ├────────────┤  ├────────────┤         │
│  │ • Team 1   │  │ • Team 1   │         │
│  │ • Team 2   │  │ • Team 2   │         │
│  └────────────┘  └────────────┘         │
└──────────────────────────────────────────┘
```

---

## Step 1: Install nself (2 minutes)

```bash
curl -sSL https://install.nself.org | bash
nself version
```

---

## Step 2: Create B2B Project (3 minutes)

### Initialize with B2B template

```bash
mkdir my-b2b-platform && cd my-b2b-platform
nself init --template b2b
```

**Template includes**:
- Organization hierarchy schema
- Team & member management
- Role-based permissions
- Usage tracking tables
- Billing per organization

### Review generated schema

```bash
cat schema.dbml
```

**Key tables**:
```dbml
Table organizations {
  id uuid [pk]
  parent_id uuid [ref: > organizations.id]  // For hierarchies
  name varchar
  slug varchar [unique]
  plan varchar
  status organization_status
  settings jsonb
  created_at timestamp
}

Table organization_members {
  id uuid [pk]
  organization_id uuid [ref: > organizations.id]
  user_id uuid [ref: > users.id]
  role member_role
  permissions jsonb
  invited_by uuid
  joined_at timestamp
}

Table teams {
  id uuid [pk]
  organization_id uuid [ref: > organizations.id]
  name varchar
  description text
  settings jsonb
}

Table team_members {
  id uuid [pk]
  team_id uuid [ref: > teams.id]
  user_id uuid [ref: > users.id]
  role team_role
  added_at timestamp
}

Table organization_usage {
  id uuid [pk]
  organization_id uuid [ref: > organizations.id]
  metric_name varchar
  quantity integer
  timestamp timestamp
  metadata jsonb
}

Table organization_settings {
  organization_id uuid [pk, ref: - organizations.id]
  branding jsonb
  features jsonb
  limits jsonb
  notifications jsonb
}
```

---

## Step 3: Build and Start (2 minutes)

```bash
nself build
nself start
```

### Apply schema

```bash
nself db schema apply schema.dbml
```

---

## Step 4: Configure Organization Hierarchy (3 minutes)

### Create parent organizations (your clients)

Using Hasura console (https://api.local.nself.org):

```graphql
mutation CreateParentOrg {
  insert_organizations(objects: [
    {
      name: "Acme Corporation"
      slug: "acme"
      parent_id: null
      plan: "enterprise"
      status: "active"
      settings: {
        allowSubOrganizations: true
        maxSubOrganizations: 10
        maxMembers: 100
      }
    },
    {
      name: "TechCo Industries"
      slug: "techco"
      parent_id: null
      plan: "business"
      status: "active"
      settings: {
        allowSubOrganizations: true
        maxSubOrganizations: 5
        maxMembers: 50
      }
    }
  ]) {
    returning {
      id
      name
      slug
    }
  }
}
```

### Create child organizations (departments/divisions)

```graphql
mutation CreateChildOrgs {
  insert_organizations(objects: [
    {
      name: "Acme Engineering"
      slug: "acme-engineering"
      parent_id: "acme-org-id-here"
      plan: "team"
      status: "active"
    },
    {
      name: "Acme Marketing"
      slug: "acme-marketing"
      parent_id: "acme-org-id-here"
      plan: "team"
      status: "active"
    }
  ]) {
    returning {
      id
      name
      parent_id
    }
  }
}
```

---

## Step 5: Set Up Team Management (4 minutes)

### Create teams within organizations

```graphql
mutation CreateTeams {
  insert_teams(objects: [
    {
      organization_id: "acme-org-id"
      name: "Backend Team"
      description: "Backend development team"
      settings: {
        defaultRole: "developer"
        allowExternalMembers: false
      }
    },
    {
      organization_id: "acme-org-id"
      name: "Frontend Team"
      description: "Frontend development team"
      settings: {
        defaultRole: "developer"
        allowExternalMembers: false
      }
    }
  ]) {
    returning {
      id
      name
      organization_id
    }
  }
}
```

### Add members to organizations

```graphql
mutation AddOrganizationMembers {
  insert_organization_members(objects: [
    {
      organization_id: "acme-org-id"
      user_id: "user-id-1"
      role: "owner"
      permissions: {
        canInvite: true
        canRemove: true
        canManageTeams: true
        canManageBilling: true
      }
    },
    {
      organization_id: "acme-org-id"
      user_id: "user-id-2"
      role: "admin"
      permissions: {
        canInvite: true
        canRemove: false
        canManageTeams: true
        canManageBilling: false
      }
    },
    {
      organization_id: "acme-org-id"
      user_id: "user-id-3"
      role: "member"
      permissions: {
        canInvite: false
        canRemove: false
        canManageTeams: false
        canManageBilling: false
      }
    }
  ]) {
    returning {
      id
      role
      user {
        email
      }
    }
  }
}
```

### Add members to teams

```graphql
mutation AddTeamMembers {
  insert_team_members(objects: [
    {
      team_id: "backend-team-id"
      user_id: "user-id-2"
      role: "lead"
    },
    {
      team_id: "backend-team-id"
      user_id: "user-id-3"
      role: "member"
    }
  ]) {
    returning {
      id
      team {
        name
      }
      user {
        email
      }
    }
  }
}
```

---

## Step 6: Configure Role-Based Permissions (4 minutes)

### Define permission rules in Hasura

**Access Hasura Console**: https://api.local.nself.org

### Organizations permissions

**Table**: `organizations`

**Role: `user`** - Can only see their own organizations
```json
{
  "organization_members": {
    "user_id": {
      "_eq": "X-Hasura-User-Id"
    }
  }
}
```

**Role: `owner`** - Full access to their organization + children
```json
{
  "_or": [
    {
      "id": {
        "_eq": "X-Hasura-Organization-Id"
      }
    },
    {
      "parent_id": {
        "_eq": "X-Hasura-Organization-Id"
      }
    }
  ]
}
```

### Organization Members permissions

**Table**: `organization_members`

**Role: `owner`** - Can manage all members
```json
{
  "organization_id": {
    "_eq": "X-Hasura-Organization-Id"
  }
}
```

**Role: `admin`** - Can view all, but limited updates
```json
{
  "organization_id": {
    "_eq": "X-Hasura-Organization-Id"
  },
  "role": {
    "_nin": ["owner"]
  }
}
```

**Role: `member`** - Read-only
```json
{
  "organization_id": {
    "_eq": "X-Hasura-Organization-Id"
  }
}
```

### Teams permissions

**Table**: `teams`

**All roles** - Based on organization membership
```json
{
  "organization": {
    "organization_members": {
      "user_id": {
        "_eq": "X-Hasura-User-Id"
      }
    }
  }
}
```

### Test permissions

```graphql
# As regular user
query GetMyOrganizations {
  organizations {
    id
    name
    role: organization_members(where: {user_id: {_eq: "X-Hasura-User-Id"}}) {
      role
      permissions
    }
  }
}

# As owner
query GetOrganizationHierarchy {
  organizations(where: {parent_id: {_is_null: true}}) {
    id
    name
    child_organizations {
      id
      name
    }
    members: organization_members {
      user {
        email
      }
      role
    }
  }
}
```

---

## Step 7: Custom Branding Per Organization (3 minutes)

### Install white-label system

```bash
nself dev whitelabel init
```

### Create brand for each organization

```bash
# Brand for Acme
nself dev whitelabel branding create "Acme Corporation" \
  --tenant acme \
  --tagline "Building the future"

nself dev whitelabel branding set-colors \
  --tenant acme \
  --primary #0066cc \
  --secondary #00cc66 \
  --accent #ff6600

# Brand for TechCo
nself dev whitelabel branding create "TechCo Industries" \
  --tenant techco \
  --tagline "Technology that works"

nself dev whitelabel branding set-colors \
  --tenant techco \
  --primary #6600cc \
  --secondary #cc0066 \
  --accent #00ccff
```

### Store branding in organization settings

```graphql
mutation UpdateOrganizationBranding {
  update_organization_settings(
    where: {organization_id: {_eq: "acme-org-id"}}
    _set: {
      branding: {
        brandName: "Acme Corporation"
        tagline: "Building the future"
        colors: {
          primary: "#0066cc"
          secondary: "#00cc66"
          accent: "#ff6600"
        }
        logo: {
          main: "https://cdn.myapp.com/acme/logo-main.png"
          icon: "https://cdn.myapp.com/acme/logo-icon.png"
        }
        domain: "app.acme.com"
      }
    }
  ) {
    affected_rows
  }
}
```

### Configure custom domains

```bash
# Add custom domain for Acme
nself dev whitelabel domain add app.acme.com --tenant acme
nself dev whitelabel domain verify app.acme.com
nself dev whitelabel domain ssl app.acme.com --auto-renew

# Add custom domain for TechCo
nself dev whitelabel domain add app.techco.com --tenant techco
nself dev whitelabel domain verify app.techco.com
nself dev whitelabel domain ssl app.techco.com --auto-renew
```

---

## Step 8: Usage Tracking & Billing (3 minutes)

### Track usage per organization

**Example: Track API calls**

```graphql
mutation TrackAPICall {
  insert_organization_usage_one(object: {
    organization_id: "acme-org-id"
    metric_name: "api_calls"
    quantity: 1
    timestamp: "now()"
    metadata: {
      endpoint: "/api/v1/users"
      method: "GET"
      userId: "user-id-1"
    }
  }) {
    id
  }
}
```

**Example: Track storage**

```graphql
mutation TrackStorage {
  insert_organization_usage_one(object: {
    organization_id: "acme-org-id"
    metric_name: "storage_bytes"
    quantity: 1048576  # 1 MB
    timestamp: "now()"
    metadata: {
      fileType: "image"
      fileName: "logo.png"
    }
  }) {
    id
  }
}
```

### Query usage reports

```sql
-- Usage by organization (last 30 days)
SELECT
  o.name AS organization,
  u.metric_name,
  SUM(u.quantity) AS total_usage,
  COUNT(*) AS usage_count
FROM organization_usage u
JOIN organizations o ON u.organization_id = o.id
WHERE u.timestamp > NOW() - INTERVAL '30 days'
GROUP BY o.name, u.metric_name
ORDER BY total_usage DESC;

-- Top consumers
SELECT
  o.name AS organization,
  SUM(u.quantity) AS total_usage
FROM organization_usage u
JOIN organizations o ON u.organization_id = o.id
WHERE u.metric_name = 'api_calls'
  AND u.timestamp > NOW() - INTERVAL '7 days'
GROUP BY o.name
ORDER BY total_usage DESC
LIMIT 10;
```

### Configure billing per organization

```bash
# Install Stripe plugin
nself plugin install stripe
```

Edit `.env`:
```bash
STRIPE_API_KEY=sk_test_PLACEHOLDER_key_here
```

**Create subscriptions per organization**:

```graphql
mutation CreateSubscription {
  insert_subscriptions_one(object: {
    organization_id: "acme-org-id"
    stripe_subscription_id: "sub_xxxxx"
    plan: "enterprise"
    status: "active"
    current_period_start: "2026-01-01"
    current_period_end: "2026-02-01"
    price_amount: 9900  # $99.00
    currency: "usd"
    billing_interval: "month"
  }) {
    id
  }
}
```

---

## Step 9: Admin Dashboard Setup (2 minutes)

### Enable nself Admin

Edit `.env`:
```bash
NSELF_ADMIN_ENABLED=true
```

```bash
nself build && nself restart
```

### Access admin dashboard

Open: https://admin.local.nself.org

**Admin features**:
- View all organizations
- Manage members
- Track usage
- View billing
- Monitor health

### Create admin user

```sql
-- Add admin role to user
UPDATE users
SET role = 'admin'
WHERE email = 'admin@mycompany.com';
```

---

## Step 10: Deploy to Production (3 minutes)

### Configure production environment

```bash
nself config env create prod
```

Edit `.env.prod`:
```bash
ENV=prod
PROJECT_NAME=my-b2b-platform
BASE_DOMAIN=myplatform.com

# Production database
POSTGRES_DB=b2b_prod
POSTGRES_PASSWORD=generate-secure-password

# Live Stripe
STRIPE_API_KEY=sk_live_your_key_here

# Security
HASURA_GRAPHQL_ADMIN_SECRET=generate-secure-secret
AUTH_JWT_SECRET=generate-jwt-secret

# Multi-tenancy
TENANT_ISOLATION_ENABLED=true
TENANT_COLUMN_NAME=organization_id
```

### Deploy

```bash
nself deploy prod
```

---

## Common B2B Queries

### Get organization hierarchy

```graphql
query GetOrganizationTree {
  organizations(where: {parent_id: {_is_null: true}}) {
    id
    name
    plan
    child_organizations {
      id
      name
      plan
      members_aggregate {
        aggregate {
          count
        }
      }
    }
    members_aggregate {
      aggregate {
        count
      }
    }
  }
}
```

### Get user's organizations and teams

```graphql
query GetUserOrganizations($userId: uuid!) {
  organization_members(where: {user_id: {_eq: $userId}}) {
    role
    permissions
    organization {
      id
      name
      slug
      plan
      teams {
        id
        name
        team_members(where: {user_id: {_eq: $userId}}) {
          role
        }
      }
    }
  }
}
```

### Get organization usage summary

```graphql
query GetOrganizationUsage($orgId: uuid!, $startDate: timestamp!, $endDate: timestamp!) {
  organization_usage_aggregate(
    where: {
      organization_id: {_eq: $orgId}
      timestamp: {_gte: $startDate, _lte: $endDate}
    }
  ) {
    aggregate {
      sum {
        quantity
      }
      count
    }
    nodes {
      metric_name
      quantity
      timestamp
    }
  }
}
```

---

## Webhooks & Automation

### Organization created webhook

```javascript
// webhooks/organization-created.js
export async function onOrganizationCreated(organization) {
  // Create default branding
  await createBranding(organization.id, {
    brandName: organization.name,
    colors: getDefaultColors()
  });

  // Set up default teams
  await createDefaultTeams(organization.id, [
    { name: "General", isDefault: true },
    { name: "Admins", isDefault: false }
  ]);

  // Initialize usage tracking
  await initializeUsageMetrics(organization.id);

  // Send welcome email to owner
  await sendEmail(organization.owner_email, "organization-welcome", {
    organizationName: organization.name
  });
}
```

### Member invited webhook

```javascript
// webhooks/member-invited.js
export async function onMemberInvited(invitation) {
  const { email, organization, role, invitedBy } = invitation;

  // Send invitation email
  await sendEmail(email, "team-invitation", {
    organizationName: organization.name,
    role: role,
    inviterName: invitedBy.name,
    inviteUrl: generateInviteUrl(invitation.token)
  });

  // Log activity
  await logActivity(organization.id, {
    type: "member_invited",
    userId: invitedBy.id,
    targetEmail: email,
    role: role
  });
}
```

---

## API Integration Examples

### REST API for usage tracking

```javascript
// Track usage via REST API
const trackUsage = async (organizationId, metric, quantity) => {
  const response = await fetch('https://api.myplatform.com/v1/usage', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${apiToken}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      organization_id: organizationId,
      metric_name: metric,
      quantity: quantity,
      timestamp: new Date().toISOString()
    })
  });

  return response.json();
};

// Example usage
await trackUsage('acme-org-id', 'api_calls', 1);
await trackUsage('acme-org-id', 'storage_bytes', 1048576);
```

### GraphQL subscription for real-time updates

```graphql
subscription OrganizationActivity($orgId: uuid!) {
  organization_usage(
    where: {organization_id: {_eq: $orgId}}
    order_by: {timestamp: desc}
    limit: 10
  ) {
    id
    metric_name
    quantity
    timestamp
    metadata
  }
}
```

---

## Troubleshooting

### Permissions not working

```bash
# Check Hasura JWT configuration
nself config show hasura | grep JWT

# Verify user session has correct claims
# Session JWT should include:
# - X-Hasura-User-Id
# - X-Hasura-Organization-Id
# - X-Hasura-Role

# Test permissions
curl -X POST https://api.myplatform.com/v1/graphql \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -d '{"query": "{ organizations { id name } }"}'
```

### Usage tracking not incrementing

```bash
# Check database connection
nself db query "SELECT COUNT(*) FROM organization_usage"

# Verify trigger exists
nself db query "
  SELECT tgname
  FROM pg_trigger
  WHERE tgname LIKE '%usage%'
"

# Check recent usage
nself db query "
  SELECT * FROM organization_usage
  ORDER BY timestamp DESC
  LIMIT 10
"
```

### Custom domain not working

```bash
# Verify DNS
nslookup app.acme.com

# Check nginx config
nself config show nginx | grep acme.com

# Test SSL
curl -I https://app.acme.com

# Check certificate
nself auth ssl check app.acme.com
```

---

## Scaling B2B Platforms

### Database optimization

```sql
-- Add indexes for common queries
CREATE INDEX idx_org_members_org_id ON organization_members(organization_id);
CREATE INDEX idx_org_members_user_id ON organization_members(user_id);
CREATE INDEX idx_teams_org_id ON teams(organization_id);
CREATE INDEX idx_team_members_team_id ON team_members(team_id);
CREATE INDEX idx_usage_org_id_timestamp ON organization_usage(organization_id, timestamp DESC);

-- Partition usage table by month
CREATE TABLE organization_usage_2026_01 PARTITION OF organization_usage
FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
```

### Caching strategy

Edit `.env`:
```bash
REDIS_ENABLED=true
HASURA_GRAPHQL_REDIS_URL=redis://redis:6379
HASURA_GRAPHQL_RATE_LIMIT={"unique_params":"IP","max_reqs_per_min":100}
```

---

## Next Steps

- **[Custom Domains Guide](CUSTOM-DOMAINS.md)** - Full domain setup
- **[Stripe Integration](STRIPE-INTEGRATION.md)** - Advanced billing
- **[White-Label System](../features/WHITELABEL-SYSTEM.md)** - Complete customization
- **[Database Workflow](../guides/DATABASE-WORKFLOW.md)** - Schema management

---

## Support

- **Documentation**: https://docs.nself.org
- **GitHub**: https://github.com/nself-org/cli
- **Discord**: https://discord.gg/nself

---

**Your B2B platform is ready! Time to onboard your first client.**
