# Quick Start: Agency Reseller Setup

Build an agency platform to manage multiple client projects with isolated environments, custom branding, aggregated billing, and white-label solutions.

**Time Estimate**: 20-25 minutes
**Difficulty**: Intermediate
**Prerequisites**: Docker Desktop, multiple client projects

---

## What You'll Build

An agency management platform with:
- Isolated client environments
- Custom branding per client
- Centralized billing and invoicing
- Project management and tracking
- Client portal access
- Usage tracking across all clients
- White-label agency branding

```
Agency Architecture:
┌─────────────────────────────────────────────┐
│         Your Agency (Parent Account)        │
├─────────────────────────────────────────────┤
│  Client 1       Client 2       Client 3     │
│  ┌─────────┐   ┌─────────┐   ┌─────────┐  │
│  │Project A│   │Project B│   │Project C│  │
│  │         │   │         │   │         │  │
│  │Custom   │   │Custom   │   │Custom   │  │
│  │Branding │   │Branding │   │Branding │  │
│  └─────────┘   └─────────┘   └─────────┘  │
└─────────────────────────────────────────────┘
          ↓
    Aggregated Billing
    Agency Dashboard
    Unified Analytics
```

---

## Step 1: Install nself (2 minutes)

```bash
curl -sSL https://install.nself.org | bash
nself version
```

---

## Step 2: Create Agency Master Project (3 minutes)

### Initialize agency master

```bash
mkdir agency-master && cd agency-master
nself init --template agency
```

**Prompts**:
```
Project name: agency-master
Environment: dev
Base domain: agency.local.nself.org
Agency name: Your Agency Name
Enable monitoring? Yes
```

### Review agency schema

```bash
cat schema.dbml
```

**Key tables**:
```dbml
Table agency_info {
  id uuid [pk]
  name varchar
  slug varchar [unique]
  owner_id uuid [ref: > users.id]
  settings jsonb
  created_at timestamp
}

Table clients {
  id uuid [pk]
  agency_id uuid [ref: > agency_info.id]
  name varchar
  slug varchar [unique]
  contact_email varchar
  status client_status  // active, suspended, cancelled
  billing_type billing_type  // monthly, project, hourly
  settings jsonb
  created_at timestamp
}

Table client_projects {
  id uuid [pk]
  client_id uuid [ref: > clients.id]
  name varchar
  description text
  status project_status  // planning, active, maintenance, completed
  deployment_url varchar
  repository_url varchar
  settings jsonb
  started_at timestamp
  completed_at timestamp
}

Table client_billing {
  id uuid [pk]
  client_id uuid [ref: > clients.id]
  billing_period_start timestamp
  billing_period_end timestamp
  amount_due decimal
  amount_paid decimal
  status invoice_status  // draft, sent, paid, overdue
  stripe_invoice_id varchar
  due_date timestamp
  paid_at timestamp
}

Table client_usage {
  id uuid [pk]
  client_id uuid [ref: > clients.id]
  project_id uuid [ref: > client_projects.id]
  metric_name varchar
  quantity integer
  timestamp timestamp
  metadata jsonb
}

Table client_credentials {
  id uuid [pk]
  client_id uuid [ref: > clients.id]
  project_id uuid [ref: > client_projects.id]
  service_name varchar
  encrypted_data text
  created_at timestamp
}
```

---

## Step 3: Build Agency Master (2 minutes)

```bash
nself build
nself start
nself db schema apply schema.dbml
```

### Access agency dashboard

Open: https://admin.agency.local.nself.org

---

## Step 4: Configure Agency Branding (3 minutes)

### Initialize white-label system

```bash
nself whitelabel init
```

### Create agency master brand

```bash
nself whitelabel branding create "Your Agency" \
  --tagline "Digital Solutions That Scale"

nself whitelabel branding set-colors \
  --primary #1a1a1a \
  --secondary #ff6600 \
  --accent #0066cc

nself whitelabel logo upload ./agency-logo.png --type main
```

### Configure agency domain

```bash
nself whitelabel domain add agency.yourdomain.com
nself whitelabel domain verify agency.yourdomain.com
nself whitelabel domain ssl agency.yourdomain.com --auto-renew
```

---

## Step 5: Add Your First Client (4 minutes)

### Create client record

```graphql
mutation CreateClient {
  insert_clients_one(object: {
    agency_id: "your-agency-id"
    name: "Acme Corporation"
    slug: "acme-corp"
    contact_email: "contact@acme.com"
    status: "active"
    billing_type: "monthly"
    settings: {
      branding: {
        colors: {
          primary: "#0066cc"
          secondary: "#00cc66"
        }
      }
      features: {
        customDomain: true
        sslEnabled: true
        backupsEnabled: true
      }
      billing: {
        monthlyRate: 2500
        currency: "usd"
        paymentTerms: "net-30"
      }
      notifications: {
        email: "contact@acme.com"
        alerts: true
        reports: true
      }
    }
  }) {
    id
    name
    slug
  }
}
```

### Create isolated project environment

```bash
# Create separate directory for client
mkdir ~/clients/acme-corp && cd ~/clients/acme-corp

# Initialize client project
nself init --client acme-corp --parent-agency agency-master
```

**This creates isolated environment**:
```
~/clients/acme-corp/
├── .env                    # Client-specific config
├── .env.secrets            # Isolated secrets
├── schema.dbml             # Client's database schema
└── docker-compose.yml      # Generated after build
```

### Configure client project

Edit `~/clients/acme-corp/.env`:

```bash
# Project info
PROJECT_NAME=acme-corp
CLIENT_ID=client-id-from-database
AGENCY_ID=your-agency-id

# Domain (client subdomain or custom)
BASE_DOMAIN=acme.youragency.com
# Or: BASE_DOMAIN=app.acme.com (client's custom domain)

# Database (isolated)
POSTGRES_DB=acme_corp_db
POSTGRES_USER=acme_user
POSTGRES_PASSWORD=generate-unique-password

# Client branding
CLIENT_BRAND_PRIMARY=#0066cc
CLIENT_BRAND_SECONDARY=#00cc66
CLIENT_LOGO_URL=https://cdn.youragency.com/clients/acme/logo.png

# Features
REDIS_ENABLED=true
MINIO_ENABLED=true
MONITORING_ENABLED=false  # Enable per client needs
```

### Build and start client project

```bash
cd ~/clients/acme-corp
nself build
nself start
```

---

## Step 6: Configure Client Billing (4 minutes)

### Install Stripe plugin

```bash
cd ~/agency-master
nself plugin install stripe
```

Edit `.env`:
```bash
STRIPE_API_KEY=sk_test_PLACEHOLDER_key
STRIPE_WEBHOOK_SECRET=whsec_your_secret
```

### Create Stripe customer for client

```graphql
mutation UpdateClientWithStripe {
  update_clients_by_pk(
    pk_columns: {id: "client-id"}
    _set: {
      stripe_customer_id: "cus_xxxxx"
    }
  ) {
    id
    stripe_customer_id
  }
}
```

### Set up recurring billing

```javascript
// API endpoint: /api/agency/billing/setup-recurring
import Stripe from 'stripe';
const stripe = new Stripe(process.env.STRIPE_API_KEY);

export async function setupClientBilling(clientId) {
  const client = await getClient(clientId);

  // Create Stripe customer
  const customer = await stripe.customers.create({
    email: client.contact_email,
    name: client.name,
    metadata: {
      client_id: clientId,
      agency_id: client.agency_id
    }
  });

  // Create subscription
  const subscription = await stripe.subscriptions.create({
    customer: customer.id,
    items: [{
      price_data: {
        currency: 'usd',
        product_data: {
          name: `${client.name} - Monthly Service`
        },
        recurring: {
          interval: 'month'
        },
        unit_amount: client.settings.billing.monthlyRate * 100  // $2500 -> 250000 cents
      }
    }],
    metadata: {
      client_id: clientId
    }
  });

  // Update client record
  await updateClient(clientId, {
    stripe_customer_id: customer.id,
    stripe_subscription_id: subscription.id
  });

  return subscription;
}
```

### Create invoices

```graphql
mutation CreateInvoice {
  insert_client_billing_one(object: {
    client_id: "client-id"
    billing_period_start: "2026-01-01"
    billing_period_end: "2026-01-31"
    amount_due: 2500.00
    status: "draft"
    due_date: "2026-02-15"
  }) {
    id
    amount_due
    status
  }
}
```

---

## Step 7: Client Portal Access (3 minutes)

### Create client portal user

```graphql
mutation CreateClientUser {
  insert_users_one(object: {
    email: "contact@acme.com"
    role: "client"
    metadata: {
      client_id: "client-id"
      permissions: ["view_projects", "view_billing", "view_analytics"]
    }
  }) {
    id
    email
  }
}
```

### Configure client portal permissions

**Hasura permissions for `client` role**:

**Table: `client_projects`**
```json
{
  "client": {
    "id": {
      "_eq": "X-Hasura-Client-Id"
    }
  }
}
```

**Table: `client_billing`**
```json
{
  "client_id": {
    "_eq": "X-Hasura-Client-Id"
  }
}
```

**Table: `client_usage`**
```json
{
  "client_id": {
    "_eq": "X-Hasura-Client-Id"
  }
}
```

### Client portal queries

```graphql
# Client dashboard
query ClientDashboard($clientId: uuid!) {
  client: clients_by_pk(id: $clientId) {
    id
    name
    status

    # Projects
    projects {
      id
      name
      status
      deployment_url
    }

    # Billing
    billing(order_by: {billing_period_start: desc}, limit: 6) {
      id
      billing_period_start
      billing_period_end
      amount_due
      amount_paid
      status
      due_date
    }

    # Usage
    usage_aggregate(where: {timestamp: {_gte: "2026-01-01"}}) {
      aggregate {
        sum {
          quantity
        }
      }
      nodes {
        metric_name
        quantity
      }
    }
  }
}
```

---

## Step 8: Track Client Usage (3 minutes)

### Track project usage

```graphql
mutation TrackUsage {
  insert_client_usage(objects: [
    {
      client_id: "client-id"
      project_id: "project-id"
      metric_name: "api_requests"
      quantity: 1000
      timestamp: "now()"
      metadata: {
        endpoint: "/api/v1/users"
        source: "web_app"
      }
    },
    {
      client_id: "client-id"
      project_id: "project-id"
      metric_name: "storage_gb"
      quantity: 5
      timestamp: "now()"
      metadata: {
        type: "database"
      }
    }
  ]) {
    returning {
      id
      metric_name
      quantity
    }
  }
}
```

### Generate usage reports

```sql
-- Client usage summary (current month)
SELECT
  c.name AS client,
  p.name AS project,
  u.metric_name,
  SUM(u.quantity) AS total_usage
FROM client_usage u
JOIN clients c ON u.client_id = c.id
JOIN client_projects p ON u.project_id = p.id
WHERE u.timestamp >= DATE_TRUNC('month', CURRENT_DATE)
  AND u.timestamp < DATE_TRUNC('month', CURRENT_DATE) + INTERVAL '1 month'
GROUP BY c.name, p.name, u.metric_name
ORDER BY c.name, total_usage DESC;

-- Agency-wide usage
SELECT
  DATE(timestamp) AS date,
  metric_name,
  SUM(quantity) AS total
FROM client_usage
WHERE timestamp > NOW() - INTERVAL '30 days'
GROUP BY DATE(timestamp), metric_name
ORDER BY date DESC, total DESC;
```

---

## Step 9: Manage Multiple Clients (2 minutes)

### CLI helper for client management

Create `~/agency-cli.sh`:

```bash
#!/bin/bash

# Quick switch between client projects
client_switch() {
  local client_slug=$1
  cd ~/clients/$client_slug
  echo "Switched to $client_slug"
  nself status
}

# Start all client projects
clients_start_all() {
  for dir in ~/clients/*; do
    if [ -d "$dir" ]; then
      echo "Starting $(basename $dir)..."
      cd $dir && nself start
    fi
  done
}

# Stop all client projects
clients_stop_all() {
  for dir in ~/clients/*; do
    if [ -d "$dir" ]; then
      echo "Stopping $(basename $dir)..."
      cd $dir && nself stop
    fi
  done
}

# Get status of all clients
clients_status() {
  for dir in ~/clients/*; do
    if [ -d "$dir" ]; then
      echo "=== $(basename $dir) ==="
      cd $dir && nself status
      echo ""
    fi
  done
}

# Backup all client databases
clients_backup_all() {
  local backup_dir=~/agency-backups/$(date +%Y-%m-%d)
  mkdir -p $backup_dir

  for dir in ~/clients/*; do
    if [ -d "$dir" ]; then
      client=$(basename $dir)
      echo "Backing up $client..."
      cd $dir && nself db backup --output $backup_dir/$client.sql
    fi
  done
}

# Update all clients to latest nself
clients_update_all() {
  for dir in ~/clients/*; do
    if [ -d "$dir" ]; then
      echo "Updating $(basename $dir)..."
      cd $dir && nself build && nself restart
    fi
  done
}

# Usage
case "$1" in
  switch) client_switch "$2" ;;
  start-all) clients_start_all ;;
  stop-all) clients_stop_all ;;
  status) clients_status ;;
  backup-all) clients_backup_all ;;
  update-all) clients_update_all ;;
  *) echo "Usage: agency-cli {switch|start-all|stop-all|status|backup-all|update-all}" ;;
esac
```

Make executable:
```bash
chmod +x ~/agency-cli.sh
```

### Use agency CLI

```bash
# Switch to client
~/agency-cli.sh switch acme-corp

# Get status of all clients
~/agency-cli.sh status

# Backup all clients
~/agency-cli.sh backup-all

# Update all clients
~/agency-cli.sh update-all
```

---

## Step 10: Agency Dashboard & Analytics (2 minutes)

### Query agency-wide metrics

```graphql
query AgencyDashboard($agencyId: uuid!) {
  agency: agency_info_by_pk(id: $agencyId) {
    id
    name

    # Clients
    clients_aggregate {
      aggregate {
        count
      }
    }

    active_clients: clients_aggregate(where: {status: {_eq: "active"}}) {
      aggregate {
        count
      }
    }

    # Projects
    clients {
      projects_aggregate {
        aggregate {
          count
        }
      }
    }

    # Revenue
    clients {
      billing_aggregate(
        where: {
          status: {_eq: "paid"}
          paid_at: {_gte: "2026-01-01"}
        }
      ) {
        aggregate {
          sum {
            amount_paid
          }
        }
      }
    }

    # Recent invoices
    clients {
      billing(
        order_by: {billing_period_start: desc}
        limit: 10
      ) {
        id
        client {
          name
        }
        amount_due
        status
        due_date
      }
    }
  }
}
```

### Generate client reports

```sql
-- Monthly revenue report
SELECT
  DATE_TRUNC('month', paid_at) AS month,
  COUNT(*) AS invoices_paid,
  SUM(amount_paid) AS revenue
FROM client_billing
WHERE status = 'paid'
  AND paid_at >= DATE_TRUNC('year', CURRENT_DATE)
GROUP BY DATE_TRUNC('month', paid_at)
ORDER BY month DESC;

-- Client profitability
SELECT
  c.name AS client,
  COUNT(DISTINCT p.id) AS projects,
  SUM(b.amount_paid) AS revenue,
  SUM(u.quantity * CASE
    WHEN u.metric_name = 'storage_gb' THEN 0.10
    WHEN u.metric_name = 'api_requests' THEN 0.001
    ELSE 0
  END) AS estimated_costs,
  SUM(b.amount_paid) - SUM(u.quantity * CASE
    WHEN u.metric_name = 'storage_gb' THEN 0.10
    WHEN u.metric_name = 'api_requests' THEN 0.001
    ELSE 0
  END) AS profit
FROM clients c
LEFT JOIN client_projects p ON c.id = p.client_id
LEFT JOIN client_billing b ON c.id = b.client_id AND b.status = 'paid'
LEFT JOIN client_usage u ON c.id = u.client_id
WHERE b.paid_at >= DATE_TRUNC('year', CURRENT_DATE)
GROUP BY c.name
ORDER BY profit DESC;
```

---

## Client Onboarding Checklist

### For each new client:

```bash
# 1. Create client record in database
# Use GraphQL mutation

# 2. Create isolated project directory
mkdir ~/clients/<client-slug>
cd ~/clients/<client-slug>

# 3. Initialize project
nself init --client <client-slug> --parent-agency agency-master

# 4. Configure client environment
# Edit .env with client-specific settings

# 5. Set up client branding
nself whitelabel branding create "<Client Name>" --tenant <client-slug>
nself whitelabel branding set-colors --tenant <client-slug> --primary <color>

# 6. Configure custom domain (if needed)
nself whitelabel domain add app.<client-domain>.com --tenant <client-slug>
nself whitelabel domain verify app.<client-domain>.com
nself whitelabel domain ssl app.<client-domain>.com --auto-renew

# 7. Build and start
nself build
nself start

# 8. Apply client schema
nself db schema apply schema.dbml

# 9. Set up billing
# Create Stripe customer and subscription

# 10. Create client portal access
# Create user with 'client' role

# 11. Send welcome email
# Use email template: client-welcome
```

---

## Common Agency Queries

### Get all clients with status

```graphql
query GetAllClients($agencyId: uuid!) {
  clients(
    where: {agency_id: {_eq: $agencyId}}
    order_by: {created_at: desc}
  ) {
    id
    name
    slug
    status
    billing_type
    projects_aggregate {
      aggregate {
        count
      }
    }
    settings
  }
}
```

### Get overdue invoices

```graphql
query GetOverdueInvoices($agencyId: uuid!) {
  client_billing(
    where: {
      client: {agency_id: {_eq: $agencyId}}
      status: {_in: ["sent", "overdue"]}
      due_date: {_lt: "now()"}
    }
    order_by: {due_date: asc}
  ) {
    id
    client {
      name
      contact_email
    }
    amount_due
    due_date
    billing_period_start
    billing_period_end
  }
}
```

---

## Troubleshooting

### Client project won't start

```bash
# Check if ports are in use
cd ~/clients/<client-slug>
nself doctor

# View logs
nself logs

# Check Docker
docker ps | grep <client-slug>
```

### Database connection issues

```bash
# Verify database is running
nself status postgres

# Check database name
nself config show | grep POSTGRES_DB

# Test connection
nself db query "SELECT 1"
```

### Billing sync issues

```bash
# Sync Stripe data
cd ~/agency-master
nself plugin stripe sync

# Check webhook events
nself plugin stripe webhook list
```

---

## Scaling Your Agency

### Server organization

**Recommended setup for larger agencies**:

```
Production Server 1: Agency Master + Small Clients (1-5)
Production Server 2: Medium Clients (6-15)
Production Server 3: Large Clients (16+)
```

**Load balancing**:
```bash
# Configure nginx upstream for multi-server
upstream client_acme {
  server server1.youragency.com:443;
}

upstream client_techco {
  server server2.youragency.com:443;
}
```

---

## Next Steps

- **[SaaS Quick Start](QUICK-START-SAAS.md)** - SaaS features for clients
- **[B2B Setup](QUICK-START-B2B.md)** - B2B client projects
- **[Stripe Integration](STRIPE-INTEGRATION.md)** - Advanced billing
- **[Custom Domains](CUSTOM-DOMAINS.md)** - Domain management

---

## Support

- **Documentation**: https://docs.nself.org
- **GitHub**: https://github.com/nself-org/cli
- **Discord**: https://discord.gg/nself

---

**Your agency platform is ready! Time to onboard your first client.**
