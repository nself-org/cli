# PayPal Pro Plugin

> Advanced PayPal integration with full webhook processing, dispute automation, and recurring subscription management. **Pro plugin, requires license.**

## Tier required

| Tier | Monthly | Annual | Includes this plugin? |
|------|---------|--------|----------------------|
| Free | $0 | $0 | No |
| Basic | $0.99/mo | $9.99/yr | Yes |
| Pro | $1.99/mo | $19.99/yr | Yes |
| Elite | $4.99/mo | $49.99/yr | Yes |
| Business | $9.99/mo | $99.99/yr | Yes |
| Business+ | $49.99/mo | $499.99/yr | Yes |
| Enterprise | $99.99/mo | $999.99/yr | Yes |

**Minimum tier:** Basic (this is a `tier: pro` plugin per F07-PRICING-TIERS).

## Bundle membership

Not currently in a named bundle. Purchase any tier subscription (Basic and up) for access.

Or get all bundles + all apps via **ɳSelf+** ($49.99/yr).

## Install

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install paypal-pro
nself build
```

The license is validated against `ping.nself.org/license/validate`. Tier is checked server-side; insufficient tier returns an error.

## Description

PayPal Pro extends the standard `paypal` plugin (port 3071) with full webhook signature validation, automated dispute response workflows, and recurring subscription lifecycle management. Incoming PayPal webhooks are verified, persisted, and fanned out to downstream tables so your application has a complete local replica of payment activity without polling the PayPal REST API.

The plugin tracks orders, captures, refunds, disputes, subscriptions, plans, payouts, and invoices. With 14 synced tables, you can query all PayPal data directly via Hasura GraphQL for custom dashboards, billing portals, and reconciliation reports.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `PAYPAL_PORT` | `3071` | PayPal pro service port |
| `PAYPAL_CLIENT_ID` | — | PayPal REST API client ID |
| `PAYPAL_CLIENT_SECRET` | — | PayPal REST API client secret |
| `PAYPAL_MODE` | `sandbox` | Mode: `sandbox` or `live` |
| `PAYPAL_WEBHOOK_ID` | — | PayPal webhook ID for signature verification |

## Ports

| Port | Purpose |
|------|---------|
| 3071 | PayPal pro REST API and webhook receiver |

## Database Schema

14 tables prefixed `np_paypal_`:

- `np_paypal_orders`, order records
- `np_paypal_captures`, payment captures
- `np_paypal_authorizations`, payment authorizations
- `np_paypal_refunds`, refund records
- `np_paypal_disputes`, dispute and chargeback records
- `np_paypal_subscriptions`, subscription state
- `np_paypal_subscription_plans`, subscription plan definitions
- `np_paypal_products`, product catalog
- `np_paypal_payouts`, payout records
- `np_paypal_invoices`, invoice history
- `np_paypal_payers`, payer (customer) records
- `np_paypal_balances`, account balance snapshots
- `np_paypal_transactions`, transaction log
- `np_paypal_webhook_events`, raw incoming webhook payloads

## Nginx Routes

| Route | Target |
|-------|--------|
| `/paypal/webhook` | PayPal webhook receiver |
| `/paypal/orders/` | Order creation and capture |
| `/paypal/subscriptions/` | Subscription management |

## Examples

### Create a PayPal order

```bash
curl -X POST http://localhost/paypal/orders \
  -H "Content-Type: application/json" \
  -d '{"amount": "29.99", "currency": "USD", "description": "Pro subscription"}'
```

### Query subscriptions via Hasura

```graphql
query ActiveSubscriptions {
  np_paypal_subscriptions(where: {status: {_eq: "ACTIVE"}}) {
    id
    payer_id
    plan_id
    next_billing_time
  }
}
```

## Source

Source-available (license required to run): [`bundles/paid/paypal/`](https://github.com/nself-org/bundles/tree/main/paid/paypal)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- [plugin-stripe-pro](plugin-stripe-pro), Stripe billing integration
- [plugin-shopify-pro](plugin-shopify-pro), Shopify store sync
- [plugin-donorbox-pro](plugin-donorbox-pro), Donorbox donation sync
- [Plugin-Overview](Plugin-Overview), full plugin index
- [Plugin-Licensing](Plugin-Licensing), tier comparison

← [Plugin-Overview](Plugin-Overview) | [Home](Home) →
