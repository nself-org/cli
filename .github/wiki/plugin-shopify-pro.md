# Shopify Pro Plugin

> Advanced Shopify integration with full webhook coverage, multi-location inventory, and Hasura GraphQL access. **Pro plugin, requires license.**

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
nself plugin install shopify-pro
nself build
```

The license is validated against `ping.nself.org/license/validate`. Tier is checked server-side; insufficient tier returns an error.

## Description

Shopify Pro extends the standard `shopify` plugin (port 3072) with full webhook coverage, multi-location inventory tracking, customer segments, and order risk scoring. Webhooks are HMAC-verified, persisted, and fanned out to downstream tables so your application has a complete local replica of store activity without polling the Shopify Admin API.

The plugin tracks products, variants, collections, customers, orders, line items, fulfillments, and inventory across locations. With 9 synced tables, you can query Shopify data directly via Hasura GraphQL for custom storefronts, analytics dashboards, and fulfillment automation.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `SHOPIFY_PORT` | `3072` | Shopify pro service port |
| `SHOPIFY_SHOP_DOMAIN` | — | Your Shopify store domain (e.g., `mystore.myshopify.com`) |
| `SHOPIFY_ACCESS_TOKEN` | — | Shopify Admin API access token |
| `SHOPIFY_WEBHOOK_SECRET` | — | Shopify webhook HMAC secret |
| `SHOPIFY_API_VERSION` | `2024-01` | Shopify API version |

## Ports

| Port | Purpose |
|------|---------|
| 3072 | Shopify pro REST API and webhook receiver |

## Database Schema

9 tables prefixed `np_shopify_`:

- `np_shopify_products`, product catalog
- `np_shopify_variants`, product variants
- `np_shopify_collections`, product collections
- `np_shopify_customers`, customer records
- `np_shopify_orders`, order records
- `np_shopify_line_items`, order line items
- `np_shopify_fulfillments`, fulfillment records
- `np_shopify_inventory`, inventory levels per location
- `np_shopify_webhook_events`, raw incoming webhook payloads

## Nginx Routes

| Route | Target |
|-------|--------|
| `/shopify/webhook` | Shopify webhook receiver |
| `/shopify/products/` | Product queries and updates |
| `/shopify/orders/` | Order queries and fulfillment |

## Examples

### Query orders via Hasura

```graphql
query RecentOrders {
  np_shopify_orders(order_by: {created_at: desc}, limit: 20) {
    id
    name
    total_price
    financial_status
    fulfillment_status
  }
}
```

### Trigger a fulfillment

```bash
curl -X POST http://localhost/shopify/orders/12345/fulfillments \
  -H "Content-Type: application/json" \
  -d '{"location_id": 67890, "tracking_number": "1Z999AA10123456784"}'
```

## Source

Source-available (license required to run): [`bundles/paid/shopify/`](https://github.com/nself-org/bundles/tree/main/paid/shopify)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- [plugin-stripe-pro](plugin-stripe-pro), Stripe billing integration
- [plugin-paypal-pro](plugin-paypal-pro), PayPal payment sync
- [plugin-donorbox-pro](plugin-donorbox-pro), Donorbox donation sync
- [Plugin-Overview](Plugin-Overview), full plugin index
- [Plugin-Licensing](Plugin-Licensing), tier comparison

← [Plugin-Overview](Plugin-Overview) | [Home](Home) →
