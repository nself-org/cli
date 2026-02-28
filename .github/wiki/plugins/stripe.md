# Stripe Plugin

Sync Stripe billing data including customers, subscriptions, invoices, and payments.

## Installation

```bash
nself plugin install stripe
```

## Configuration

### Required

```bash
STRIPE_API_KEY=sk_test_PLACEHOLDER
```

### Optional

```bash
STRIPE_WEBHOOK_SECRET=whsec_xxxxxxxxxxxx   # Webhook signature verification
STRIPE_SYNC_INTERVAL=3600                   # Sync interval (default: 1 hour)
```

### API Key Types

- **Test Mode**: Use `sk_test_PLACEHOLDER*` keys for development
- **Live Mode**: Use `sk_live_*` keys for production

The plugin auto-detects test vs live mode from the key prefix.

## Usage

### Sync Data

```bash
# Full sync
nself plugin stripe sync

# Sync specific resources
nself plugin stripe sync --customers
nself plugin stripe sync --subscriptions
```

### Customers

```bash
# List customers
nself plugin stripe customers list

# Filter by email
nself plugin stripe customers list --email "user@example.com"

# Show customer details
nself plugin stripe customers show cus_xxxxx
```

### Subscriptions

```bash
# List subscriptions
nself plugin stripe subscriptions list

# Active only
nself plugin stripe subscriptions list --status active

# Statistics
nself plugin stripe subscriptions stats
```

### Invoices

```bash
# List invoices
nself plugin stripe invoices list

# Paid invoices
nself plugin stripe invoices list --status paid

# Show invoice details
nself plugin stripe invoices show in_xxxxx
```

### Webhooks

```bash
# List webhook events
nself plugin stripe webhook list

# Pending events
nself plugin stripe webhook pending

# Retry failed event
nself plugin stripe webhook retry evt_xxxxx

# Event statistics
nself plugin stripe webhook stats
```

## Webhook Setup

### Stripe Dashboard

1. Go to [Stripe Dashboard > Webhooks](https://dashboard.stripe.com/webhooks)
2. Add endpoint: `https://your-domain.com/webhooks/stripe`
3. Select events to subscribe:
   - `customer.created`, `customer.updated`
   - `subscription.created`, `subscription.updated`, `subscription.deleted`
   - `invoice.paid`, `invoice.payment_failed`
   - `payment_intent.succeeded`, `payment_intent.payment_failed`
4. Copy signing secret to `STRIPE_WEBHOOK_SECRET`

### Stripe CLI (Development)

```bash
stripe listen --forward-to localhost/webhooks/stripe
```

## Database Schema

### Tables

| Table | Description |
|-------|-------------|
| `stripe_customers` | Customer records |
| `stripe_products` | Product catalog |
| `stripe_prices` | Pricing tiers |
| `stripe_subscriptions` | Active subscriptions |
| `stripe_invoices` | Invoice history |
| `stripe_payment_intents` | Payment attempts |
| `stripe_payment_methods` | Saved payment methods |
| `stripe_webhook_events` | Webhook event log |

### Views

| View | Description |
|------|-------------|
| `stripe_revenue_by_month` | Monthly revenue breakdown |
| `stripe_subscription_mrr` | Monthly recurring revenue |

### Example Queries

```sql
-- Active subscriptions by plan
SELECT
  p.name AS plan,
  COUNT(*) AS subscribers,
  SUM(s.quantity * pr.unit_amount / 100) AS mrr
FROM stripe_subscriptions s
JOIN stripe_prices pr ON s.price_id = pr.id
JOIN stripe_products p ON pr.product_id = p.id
WHERE s.status = 'active'
GROUP BY p.name
ORDER BY mrr DESC;

-- Recent payments
SELECT
  c.email,
  i.amount_paid / 100 AS amount,
  i.status,
  i.created_at
FROM stripe_invoices i
JOIN stripe_customers c ON i.customer_id = c.id
WHERE i.created_at > NOW() - INTERVAL '7 days'
ORDER BY i.created_at DESC;
```

## Environment Handling

### Development (Test Mode)

```bash
ENV=dev
STRIPE_API_KEY=sk_test_PLACEHOLDER
```

Uses test customers, no real charges.

### Production (Live Mode)

```bash
ENV=prod
STRIPE_API_KEY=sk_live_xxxxx
```

Real customer data and charges.

## Uninstall

```bash
# Remove plugin and data
nself plugin remove stripe

# Keep database tables
nself plugin remove stripe --keep-data
```

## Troubleshooting

### Sync Failures

```bash
# Check API key
curl -u sk_test_PLACEHOLDER: https://api.stripe.com/v1/customers?limit=1

# Verbose sync
nself plugin stripe sync --verbose
```

### Webhook Issues

```bash
# Verify signature
echo "Check STRIPE_WEBHOOK_SECRET matches Stripe dashboard"

# List recent events
nself plugin stripe webhook list --limit 10
```

## Related

- [Plugin Command](../commands/PLUGIN.md)
- [Database Command](../commands/DB.md)
- [Stripe API Docs](https://stripe.com/docs/api)
