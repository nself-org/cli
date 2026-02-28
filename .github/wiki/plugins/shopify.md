# Shopify Plugin

Sync Shopify store data including products, orders, customers, and inventory.

## Installation

```bash
nself plugin install shopify
```

## Configuration

### Required

```bash
SHOPIFY_STORE=your-store                    # Store name (without .myshopify.com)
SHOPIFY_ACCESS_TOKEN=shpat_xxxxxxxxxxxx     # Admin API access token
```

### Optional

```bash
SHOPIFY_API_VERSION=2024-01                 # API version (default: 2024-01)
SHOPIFY_WEBHOOK_SECRET=xxxxxxxxxxxx         # Webhook HMAC verification
SHOPIFY_SYNC_INTERVAL=3600                  # Sync interval in seconds
```

### Creating Access Token

1. Go to Shopify Admin > Settings > Apps and sales channels
2. Click "Develop apps" > "Create an app"
3. Configure Admin API scopes:
   - `read_products`, `write_products`
   - `read_customers`
   - `read_orders`
   - `read_inventory`
4. Install app and copy the access token

## Usage

### Sync Data

```bash
# Full sync
nself plugin shopify sync

# Products only
nself plugin shopify sync --products-only

# Initial sync (runs on install)
nself plugin shopify sync --initial
```

### Products

```bash
# List products
nself plugin shopify products list

# Filter by vendor
nself plugin shopify products list --vendor "My Brand"

# Filter by status
nself plugin shopify products list --status active

# Low stock items
nself plugin shopify products low-stock

# Statistics
nself plugin shopify products stats
```

### Orders

```bash
# List orders
nself plugin shopify orders list

# Pending orders
nself plugin shopify orders pending

# Unfulfilled orders
nself plugin shopify orders unfulfilled

# Order details
nself plugin shopify orders show 123456

# Statistics
nself plugin shopify orders stats
```

### Customers

```bash
# List customers
nself plugin shopify customers list

# Top customers by spending
nself plugin shopify customers top

# Customer details
nself plugin shopify customers show 123456

# Statistics
nself plugin shopify customers stats
```

### Webhooks

```bash
# List events
nself plugin shopify webhook list

# Filter by topic
nself plugin shopify webhook list --topic orders/create

# Pending events
nself plugin shopify webhook pending

# Retry event
nself plugin shopify webhook retry <event-id>
```

## Webhook Setup

### Shopify Admin

1. Go to Settings > Notifications > Webhooks
2. Create webhooks for each topic:
   - URL: `https://your-domain.com/webhooks/shopify`
   - Format: JSON
3. Topics to subscribe:
   - `orders/create`, `orders/updated`, `orders/paid`, `orders/fulfilled`
   - `products/create`, `products/update`, `products/delete`
   - `customers/create`, `customers/update`
   - `inventory_levels/update`

### Shopify CLI

```bash
shopify app webhook forward --topic orders/create \
  --path /webhooks/shopify --port 443
```

## Database Schema

### Tables

| Table | Description |
|-------|-------------|
| `shopify_shops` | Store metadata |
| `shopify_products` | Product catalog |
| `shopify_variants` | Product variants |
| `shopify_collections` | Product collections |
| `shopify_customers` | Customer data |
| `shopify_orders` | Order history |
| `shopify_order_items` | Line items |
| `shopify_inventory` | Inventory levels |
| `shopify_webhook_events` | Webhook log |

### Views

| View | Description |
|------|-------------|
| `shopify_sales_overview` | Daily sales summary |
| `shopify_top_products` | Best sellers |
| `shopify_low_inventory` | Low stock alerts |
| `shopify_customer_value` | Customer lifetime value |

### Example Queries

```sql
-- Daily sales (last 7 days)
SELECT * FROM shopify_sales_overview
WHERE order_date > CURRENT_DATE - 7;

-- Top products by revenue
SELECT * FROM shopify_top_products
LIMIT 20;

-- Low inventory alerts
SELECT * FROM shopify_low_inventory;

-- Customer segments
SELECT
  CASE
    WHEN total_spent >= 1000 THEN 'VIP'
    WHEN total_spent >= 500 THEN 'High Value'
    WHEN total_spent >= 100 THEN 'Regular'
    ELSE 'New'
  END AS segment,
  COUNT(*) AS customers,
  SUM(total_spent) AS total_revenue
FROM shopify_customers
WHERE orders_count > 0
GROUP BY segment
ORDER BY total_revenue DESC;
```

## Environment Handling

### Development

```bash
ENV=dev
SHOPIFY_STORE=your-store-dev       # Use development store
SHOPIFY_ACCESS_TOKEN=shpat_dev
```

Use a Shopify development store for dev/staging.

### Production

```bash
ENV=prod
SHOPIFY_STORE=your-store           # Production store
SHOPIFY_ACCESS_TOKEN=shpat_live
```

## Uninstall

```bash
# Remove plugin and data
nself plugin remove shopify

# Keep database tables
nself plugin remove shopify --keep-data
```

## Troubleshooting

### Authentication Errors

```bash
# Test API access
curl -H "X-Shopify-Access-Token: shpat_xxx" \
  "https://your-store.myshopify.com/admin/api/2024-01/shop.json"
```

### Rate Limiting

Shopify has a 2 requests/second limit. The plugin:
- Uses 0.5s delay between requests
- Handles pagination automatically

### Webhook Issues

```bash
# Verify HMAC signature
# Check SHOPIFY_WEBHOOK_SECRET matches Shopify settings

# Check nginx logs
docker logs <project>_nginx | grep webhook
```

### Missing Products/Orders

```bash
# Force full resync
nself plugin shopify sync --full

# Check sync timestamp
nself plugin shopify status
```

## Related

- [Plugin Command](../commands/PLUGIN.md)
- [Database Command](../commands/DB.md)
- [Shopify API Docs](https://shopify.dev/docs/api/admin-rest)
