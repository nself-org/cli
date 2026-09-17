# Stripe Pro Plugin

> Advanced Stripe integration with full webhook coverage, customer portal, and 24-table Hasura GraphQL access. **Pro plugin, requires license.**

## Tier required

| Tier | Monthly | Annual | Includes this plugin? |
|------|---------|--------|----------------------|
| Free | $0 | $0 | No |
| Any bundle | $0.99/mo | $9.99/yr | If stripe-pro is in that bundle |
| ɳSelf+ | $3.99/mo | $39.99/yr | Yes — all bundles + all apps |

**Minimum tier:** Any bundle subscription that includes stripe-pro, or ɳSelf+.

## Bundle membership

Not currently in a named bundle. Requires ɳSelf+ ($3.99/mo or $39.99/yr) for standalone access.

Or get all bundles + all apps via **ɳSelf+** ($3.99/mo · $39.99/yr).

## Install

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install stripe-pro
nself build
```

The license is validated against `ping.nself.org/license/validate`. Tier is checked server-side; insufficient tier returns an error.

## Description

Stripe Pro extends the standard `stripe` plugin (port 3740) with full webhook coverage, the Stripe customer portal, checkout session API, and 24 synced tables. Webhooks are signature-verified, persisted, and fanned out to downstream tables so your application has a complete local replica of billing activity without hitting the Stripe API.

The plugin tracks customers, subscriptions, invoices, payment intents, products, prices, refunds, disputes, coupons, trials, tax rates, and more. With 24 synced tables, you can query all billing data directly via Hasura GraphQL for custom dashboards, billing portals, and reconciliation reports.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `STRIPE_PORT` | `3740` | Stripe pro service port |
| `STRIPE_API_KEY` | — | Stripe secret key (`sk_live_...` or `sk_test_...`) |
| `STRIPE_WEBHOOK_SECRET` | — | Webhook endpoint signing secret |
| `STRIPE_PORTAL_RETURN_URL` | — | Customer portal return URL |

## Ports

| Port | Purpose |
|------|---------|
| 3740 | Stripe pro REST API and webhook receiver |

## Database Schema

24 tables prefixed `np_stripe_`. Key tables:

- `np_stripe_customers`, customer records
- `np_stripe_subscriptions`, subscription state
- `np_stripe_invoices`, invoice history
- `np_stripe_payment_intents`, payment records
- `np_stripe_products`, product catalog
- `np_stripe_prices`, pricing tiers
- `np_stripe_refunds`, refund records
- `np_stripe_disputes`, dispute and chargeback records
- `np_stripe_coupons`, discount coupons
- `np_stripe_tax_rates`, tax rate definitions
- `np_stripe_webhook_events`, raw incoming webhook payloads

Plus 13 more tables for trials, payment methods, charges, transfers, and balance transactions.

## Nginx Routes

| Route | Target |
|-------|--------|
| `/stripe/webhook` | Stripe event webhook receiver |
| `/stripe/portal/session` | Customer portal session creation |
| `/stripe/checkout/session` | Checkout session creation |

## Examples

### Create a checkout session

```bash
curl -X POST http://localhost/stripe/checkout/session \
  -H "Content-Type: application/json" \
  -d '{"price_id": "price_1234", "customer_id": "cus_5678", "success_url": "https://example.com/success"}'
```

### Query active subscriptions via Hasura

```graphql
query ActiveSubscriptions {
  np_stripe_subscriptions(where: {status: {_eq: "active"}}) {
    id
    customer_id
    current_period_end
    items
  }
}
```

### Open the customer portal

```bash
curl -X POST http://localhost/stripe/portal/session \
  -H "Content-Type: application/json" \
  -d '{"customer_id": "cus_5678"}'
```

## Source

Source-available (license required to run): [`bundles/paid/stripe/`](https://github.com/nself-org/bundles/tree/main/paid/stripe)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- [plugin-paypal-pro](plugin-paypal-pro), PayPal payment sync
- [plugin-shopify-pro](plugin-shopify-pro), Shopify store sync
- [plugin-donorbox-pro](plugin-donorbox-pro), Donorbox donation sync
- [Plugin-Overview](Plugin-Overview), full plugin index
- [Plugin-Licensing](Plugin-Licensing), tier comparison

← [Plugin-Overview](Plugin-Overview) | [Home](Home) →
