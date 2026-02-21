# Quick Start: Marketplace/Platform Setup

Build a multi-vendor marketplace with vendor isolation, separate billing, white-label storefronts, and platform fees.

**Time Estimate**: 25-30 minutes
**Difficulty**: Intermediate-Advanced
**Prerequisites**: Docker Desktop, Stripe account, understanding of marketplaces

---

## What You'll Build

A complete marketplace platform with:
- Multi-vendor architecture with isolation
- Vendor onboarding and verification
- Product/listing management per vendor
- Separate billing and payouts per vendor
- Platform fee collection (commission)
- White-label storefront per vendor
- Order management and fulfillment

```
Marketplace Architecture:
┌────────────────────────────────────────────────┐
│           Platform (Your Marketplace)          │
├────────────────────────────────────────────────┤
│  Vendor A          Vendor B          Vendor C  │
│  ┌──────────┐     ┌──────────┐     ┌────────┐ │
│  │Products  │     │Products  │     │Products│ │
│  │Orders    │     │Orders    │     │Orders  │ │
│  │Payouts   │     │Payouts   │     │Payouts │ │
│  └──────────┘     └──────────┘     └────────┘ │
└────────────────────────────────────────────────┘
         ↓                  ↓                ↓
    Customers → Order → Platform → Vendor Payout
                        (15% fee)
```

---

## Step 1: Install nself (2 minutes)

```bash
curl -sSL https://install.nself.org | bash
nself version
```

---

## Step 2: Create Marketplace Project (3 minutes)

### Initialize with marketplace template

```bash
mkdir my-marketplace && cd my-marketplace
nself init --template marketplace
```

**Template includes**:
- Vendor management
- Product/listing catalogs
- Order processing
- Payment splits
- Commission tracking
- Payout management

### Review marketplace schema

```bash
cat schema.dbml
```

**Key tables**:
```dbml
Table vendors {
  id uuid [pk]
  user_id uuid [ref: > users.id]
  business_name varchar
  slug varchar [unique]
  status vendor_status  // pending, verified, active, suspended
  stripe_account_id varchar [unique]  // Stripe Connect account
  commission_rate decimal [default: 15.0]
  settings jsonb
  verified_at timestamp
  created_at timestamp
}

Table vendor_products {
  id uuid [pk]
  vendor_id uuid [ref: > vendors.id]
  name varchar
  description text
  price decimal
  currency varchar [default: 'usd']
  inventory_count integer
  status product_status  // draft, active, archived
  images jsonb
  metadata jsonb
  created_at timestamp
}

Table orders {
  id uuid [pk]
  customer_id uuid [ref: > users.id]
  vendor_id uuid [ref: > vendors.id]
  order_number varchar [unique]
  status order_status  // pending, processing, shipped, delivered, cancelled
  subtotal decimal
  platform_fee decimal
  vendor_amount decimal
  total_amount decimal
  stripe_payment_intent_id varchar
  created_at timestamp
}

Table order_items {
  id uuid [pk]
  order_id uuid [ref: > orders.id]
  product_id uuid [ref: > vendor_products.id]
  quantity integer
  unit_price decimal
  total_price decimal
}

Table vendor_payouts {
  id uuid [pk]
  vendor_id uuid [ref: > vendors.id]
  amount decimal
  currency varchar
  status payout_status  // pending, processing, paid, failed
  stripe_payout_id varchar
  period_start timestamp
  period_end timestamp
  orders_count integer
  created_at timestamp
}

Table platform_fees {
  id uuid [pk]
  order_id uuid [ref: > orders.id]
  vendor_id uuid [ref: > vendors.id]
  fee_amount decimal
  fee_percentage decimal
  collected_at timestamp
}
```

---

## Step 3: Build and Start (2 minutes)

```bash
nself build
nself start
nself db schema apply schema.dbml
```

---

## Step 4: Configure Stripe Connect (5 minutes)

### Enable Stripe Connect for marketplace

```bash
nself plugin install stripe
```

### Configure Stripe for Connect

Edit `.env`:
```bash
# Stripe API Keys
STRIPE_API_KEY=sk_test_PLACEHOLDER_key_here
STRIPE_PUBLISHABLE_KEY=pk_test_your_key_here

# Stripe Connect (Platform/Marketplace)
STRIPE_CONNECT_ENABLED=true
STRIPE_CONNECT_CLIENT_ID=ca_your_client_id_here

# Platform fee settings
PLATFORM_FEE_PERCENTAGE=15.0  # 15% commission
PLATFORM_FEE_FIXED=0.50       # $0.50 per transaction
```

### Get Stripe Connect Client ID

1. Go to [Stripe Dashboard](https://dashboard.stripe.com)
2. Navigate to **Settings** → **Connect**
3. Enable **Connect** if not already enabled
4. Copy **Client ID** (starts with `ca_`)
5. Add to `.env`

### Rebuild with Stripe Connect

```bash
nself build && nself restart
```

---

## Step 5: Create Vendor Onboarding Flow (4 minutes)

### Create vendor signup mutation

```graphql
mutation CreateVendor {
  insert_vendors_one(object: {
    user_id: "user-id-here"
    business_name: "Awesome Shop"
    slug: "awesome-shop"
    status: "pending"
    settings: {
      notifications: {
        email: "vendor@example.com"
        newOrders: true
        lowInventory: true
      }
      shipping: {
        enabled: true
        rates: []
      }
    }
  }) {
    id
    business_name
    slug
    status
  }
}
```

### Connect vendor to Stripe

**Step 1: Generate Connect onboarding link**

```javascript
// API endpoint: /api/vendor/stripe-connect
import Stripe from 'stripe';
const stripe = new Stripe(process.env.STRIPE_API_KEY);

export async function createStripeConnectAccount(vendorId, email) {
  // Create Stripe Connect account
  const account = await stripe.accounts.create({
    type: 'express',  // or 'standard'
    email: email,
    capabilities: {
      card_payments: { requested: true },
      transfers: { requested: true }
    },
    business_type: 'individual',  // or 'company'
    metadata: {
      vendor_id: vendorId
    }
  });

  // Create account link for onboarding
  const accountLink = await stripe.accountLinks.create({
    account: account.id,
    refresh_url: `https://mymarketplace.com/vendor/stripe/refresh`,
    return_url: `https://mymarketplace.com/vendor/stripe/complete`,
    type: 'account_onboarding'
  });

  // Save account ID
  await updateVendor(vendorId, {
    stripe_account_id: account.id
  });

  return accountLink.url;  // Redirect vendor here
}
```

**Step 2: Update vendor with Stripe account**

```graphql
mutation UpdateVendorStripeAccount {
  update_vendors_by_pk(
    pk_columns: {id: "vendor-id"}
    _set: {
      stripe_account_id: "acct_xxxxx"
      status: "verified"
      verified_at: "now()"
    }
  ) {
    id
    stripe_account_id
    status
  }
}
```

---

## Step 6: Product Management (3 minutes)

### Create products for vendor

```graphql
mutation CreateProducts {
  insert_vendor_products(objects: [
    {
      vendor_id: "vendor-id"
      name: "Organic Coffee Beans"
      description: "Premium organic coffee beans from Colombia"
      price: 24.99
      currency: "usd"
      inventory_count: 100
      status: "active"
      images: {
        main: "https://cdn.mymarketplace.com/products/coffee-1.jpg"
        gallery: [
          "https://cdn.mymarketplace.com/products/coffee-2.jpg",
          "https://cdn.mymarketplace.com/products/coffee-3.jpg"
        ]
      }
      metadata: {
        sku: "COFFEE-ORG-001"
        weight: "1lb"
        category: "Food & Beverages"
        tags: ["organic", "coffee", "fair-trade"]
      }
    },
    {
      vendor_id: "vendor-id"
      name: "Ceramic Mug Set"
      description: "Handcrafted ceramic mugs, set of 4"
      price: 39.99
      currency: "usd"
      inventory_count: 50
      status: "active"
      images: {
        main: "https://cdn.mymarketplace.com/products/mug-1.jpg"
      }
      metadata: {
        sku: "MUG-CER-004"
        category: "Home & Kitchen"
        tags: ["handmade", "ceramic", "mugs"]
      }
    }
  ]) {
    returning {
      id
      name
      price
      vendor {
        business_name
      }
    }
  }
}
```

### Query products by vendor

```graphql
query GetVendorProducts($vendorId: uuid!) {
  vendor_products(
    where: {
      vendor_id: {_eq: $vendorId}
      status: {_eq: "active"}
    }
    order_by: {created_at: desc}
  ) {
    id
    name
    description
    price
    currency
    inventory_count
    images
    metadata
  }
}
```

### Search all marketplace products

```graphql
query SearchProducts($search: String!) {
  vendor_products(
    where: {
      _or: [
        {name: {_ilike: $search}},
        {description: {_ilike: $search}}
      ]
      status: {_eq: "active"}
      inventory_count: {_gt: 0}
    }
  ) {
    id
    name
    price
    vendor {
      business_name
      slug
    }
  }
}
```

---

## Step 7: Order Processing with Payment Splits (5 minutes)

### Create order with automatic fee split

```javascript
// API endpoint: /api/orders/create
import Stripe from 'stripe';
const stripe = new Stripe(process.env.STRIPE_API_KEY);

export async function createOrder(cart, customerId) {
  const { vendorId, items, totalAmount } = cart;

  // Get vendor
  const vendor = await getVendor(vendorId);

  // Calculate fees
  const platformFeePercentage = vendor.commission_rate || 15.0;
  const platformFeeFixed = 0.50;
  const platformFee = (totalAmount * platformFeePercentage / 100) + platformFeeFixed;
  const vendorAmount = totalAmount - platformFee;

  // Create Stripe Payment Intent with destination charge
  const paymentIntent = await stripe.paymentIntents.create({
    amount: Math.round(totalAmount * 100),  // cents
    currency: 'usd',
    customer: customerId,
    application_fee_amount: Math.round(platformFee * 100),  // Platform fee
    transfer_data: {
      destination: vendor.stripe_account_id  // Vendor's Stripe Connect account
    },
    metadata: {
      vendor_id: vendorId,
      platform_fee: platformFee,
      vendor_amount: vendorAmount
    }
  });

  // Create order in database
  const order = await createOrderInDB({
    customer_id: customerId,
    vendor_id: vendorId,
    order_number: generateOrderNumber(),
    status: 'pending',
    subtotal: totalAmount,
    platform_fee: platformFee,
    vendor_amount: vendorAmount,
    total_amount: totalAmount,
    stripe_payment_intent_id: paymentIntent.id
  });

  // Create order items
  await createOrderItems(order.id, items);

  // Record platform fee
  await recordPlatformFee({
    order_id: order.id,
    vendor_id: vendorId,
    fee_amount: platformFee,
    fee_percentage: platformFeePercentage
  });

  return {
    order,
    clientSecret: paymentIntent.client_secret
  };
}
```

### Create order mutation

```graphql
mutation CreateOrder {
  insert_orders_one(object: {
    customer_id: "customer-id"
    vendor_id: "vendor-id"
    order_number: "MKT-2026-00001"
    status: "pending"
    subtotal: 64.98
    platform_fee: 10.25  # 15% + $0.50
    vendor_amount: 54.73
    total_amount: 64.98
    stripe_payment_intent_id: "pi_xxxxx"
    order_items: {
      data: [
        {
          product_id: "product-1-id"
          quantity: 1
          unit_price: 24.99
          total_price: 24.99
        },
        {
          product_id: "product-2-id"
          quantity: 1
          unit_price: 39.99
          total_price: 39.99
        }
      ]
    }
  }) {
    id
    order_number
    total_amount
    platform_fee
    vendor_amount
  }
}
```

### Update order status

```graphql
mutation UpdateOrderStatus {
  update_orders_by_pk(
    pk_columns: {id: "order-id"}
    _set: {
      status: "processing"  # or: shipped, delivered
    }
  ) {
    id
    status
  }
}
```

---

## Step 8: Vendor Payout Management (4 minutes)

### Calculate vendor payouts

```sql
-- Get vendor earnings for payout period
SELECT
  v.id AS vendor_id,
  v.business_name,
  COUNT(o.id) AS orders_count,
  SUM(o.vendor_amount) AS payout_amount,
  SUM(o.platform_fee) AS platform_fees_collected
FROM vendors v
JOIN orders o ON v.id = o.vendor_id
WHERE o.status IN ('delivered', 'completed')
  AND o.created_at >= '2026-01-01'
  AND o.created_at < '2026-02-01'
  AND v.stripe_account_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM vendor_payouts p
    WHERE p.vendor_id = v.id
      AND p.period_start = '2026-01-01'
      AND p.period_end = '2026-02-01'
  )
GROUP BY v.id, v.business_name;
```

### Create payout records

```graphql
mutation CreatePayouts {
  insert_vendor_payouts(objects: [
    {
      vendor_id: "vendor-1-id"
      amount: 543.21
      currency: "usd"
      status: "pending"
      period_start: "2026-01-01"
      period_end: "2026-02-01"
      orders_count: 15
    },
    {
      vendor_id: "vendor-2-id"
      amount: 1234.56
      currency: "usd"
      status: "pending"
      period_start: "2026-01-01"
      period_end: "2026-02-01"
      orders_count: 32
    }
  ]) {
    returning {
      id
      vendor {
        business_name
      }
      amount
      status
    }
  }
}
```

### Process payouts via Stripe

```javascript
// API endpoint: /api/payouts/process
export async function processVendorPayouts() {
  const pendingPayouts = await getPendingPayouts();

  for (const payout of pendingPayouts) {
    try {
      // Create Stripe payout to vendor's Connect account
      const stripePayout = await stripe.payouts.create(
        {
          amount: Math.round(payout.amount * 100),  // cents
          currency: payout.currency
        },
        {
          stripeAccount: payout.vendor.stripe_account_id  // Send to vendor
        }
      );

      // Update payout record
      await updatePayout(payout.id, {
        stripe_payout_id: stripePayout.id,
        status: 'processing'
      });

      console.log(`Payout ${payout.id} created: ${stripePayout.id}`);
    } catch (error) {
      console.error(`Payout failed for ${payout.id}:`, error);

      await updatePayout(payout.id, {
        status: 'failed',
        error_message: error.message
      });
    }
  }
}
```

---

## Step 9: White-Label Vendor Storefronts (3 minutes)

### Initialize white-label system

```bash
nself whitelabel init
```

### Create brand per vendor

```bash
# Vendor 1: Awesome Shop
nself whitelabel branding create "Awesome Shop" \
  --tenant awesome-shop \
  --tagline "Quality products you'll love"

nself whitelabel branding set-colors \
  --tenant awesome-shop \
  --primary #ff6600 \
  --secondary #0066cc

# Vendor 2: Craft Corner
nself whitelabel branding create "Craft Corner" \
  --tenant craft-corner \
  --tagline "Handmade with love"

nself whitelabel branding set-colors \
  --tenant craft-corner \
  --primary #9933ff \
  --secondary #33cc99
```

### Configure custom domains per vendor

```bash
# Vendor 1
nself whitelabel domain add shop.awesomeshop.com --tenant awesome-shop
nself whitelabel domain verify shop.awesomeshop.com
nself whitelabel domain ssl shop.awesomeshop.com --auto-renew

# Vendor 2
nself whitelabel domain add store.craftcorner.com --tenant craft-corner
nself whitelabel domain verify store.craftcorner.com
nself whitelabel domain ssl store.craftcorner.com --auto-renew
```

### Store branding in vendor settings

```graphql
mutation UpdateVendorBranding {
  update_vendors_by_pk(
    pk_columns: {id: "vendor-id"}
    _set: {
      settings: {
        branding: {
          logo: "https://cdn.mymarketplace.com/vendors/awesome-shop/logo.png"
          colors: {
            primary: "#ff6600"
            secondary: "#0066cc"
          }
          domain: "shop.awesomeshop.com"
        }
      }
    }
  ) {
    id
    settings
  }
}
```

---

## Step 10: Platform Analytics & Monitoring (2 minutes)

### Enable monitoring

Edit `.env`:
```bash
MONITORING_ENABLED=true
GRAFANA_ADMIN_PASSWORD=admin
```

```bash
nself build && nself restart
```

### Access Grafana

Open: https://grafana.local.nself.org

**Default dashboards**:
- Platform revenue
- Vendor performance
- Order metrics
- Payout tracking

### Custom marketplace queries

```sql
-- Platform revenue (last 30 days)
SELECT
  DATE(created_at) AS date,
  COUNT(*) AS orders,
  SUM(total_amount) AS total_revenue,
  SUM(platform_fee) AS platform_earnings,
  SUM(vendor_amount) AS vendor_earnings
FROM orders
WHERE created_at > NOW() - INTERVAL '30 days'
  AND status NOT IN ('cancelled', 'refunded')
GROUP BY DATE(created_at)
ORDER BY date DESC;

-- Top vendors
SELECT
  v.business_name,
  COUNT(o.id) AS orders,
  SUM(o.total_amount) AS total_sales,
  AVG(o.vendor_amount) AS avg_vendor_amount
FROM vendors v
JOIN orders o ON v.id = o.vendor_id
WHERE o.created_at > NOW() - INTERVAL '30 days'
GROUP BY v.business_name
ORDER BY total_sales DESC
LIMIT 10;

-- Product performance
SELECT
  p.name,
  v.business_name AS vendor,
  COUNT(oi.id) AS units_sold,
  SUM(oi.total_price) AS revenue
FROM vendor_products p
JOIN vendors v ON p.vendor_id = v.id
JOIN order_items oi ON p.id = oi.product_id
JOIN orders o ON oi.order_id = o.id
WHERE o.created_at > NOW() - INTERVAL '30 days'
GROUP BY p.name, v.business_name
ORDER BY revenue DESC
LIMIT 20;
```

---

## Common Marketplace Queries

### Get vendor dashboard data

```graphql
query VendorDashboard($vendorId: uuid!) {
  vendor: vendors_by_pk(id: $vendorId) {
    id
    business_name
    commission_rate

    # Product stats
    products_aggregate {
      aggregate {
        count
        sum {
          inventory_count
        }
      }
    }

    # Order stats
    orders_aggregate(where: {status: {_neq: "cancelled"}}) {
      aggregate {
        count
        sum {
          vendor_amount
          platform_fee
        }
      }
    }

    # Recent orders
    orders(order_by: {created_at: desc}, limit: 10) {
      id
      order_number
      status
      total_amount
      vendor_amount
      created_at
      customer {
        email
      }
    }

    # Pending payout
    payouts_aggregate(where: {status: {_eq: "pending"}}) {
      aggregate {
        sum {
          amount
        }
      }
    }
  }
}
```

### Search marketplace

```graphql
query MarketplaceSearch($search: String!, $category: String, $minPrice: numeric, $maxPrice: numeric) {
  vendor_products(
    where: {
      _and: [
        {
          _or: [
            {name: {_ilike: $search}},
            {description: {_ilike: $search}},
            {metadata: {_contains: {tags: [$search]}}}
          ]
        },
        {status: {_eq: "active"}},
        {inventory_count: {_gt: 0}},
        {metadata: {_contains: {category: $category}}},
        {price: {_gte: $minPrice, _lte: $maxPrice}}
      ]
    }
    order_by: {created_at: desc}
  ) {
    id
    name
    description
    price
    images
    vendor {
      business_name
      slug
    }
  }
}
```

---

## Stripe Connect Webhooks

### Configure webhook endpoint

```javascript
// api/webhooks/stripe-connect.js
import Stripe from 'stripe';
const stripe = new Stripe(process.env.STRIPE_API_KEY);

export async function handleStripeConnectWebhook(req) {
  const sig = req.headers['stripe-signature'];
  const event = stripe.webhooks.constructEvent(
    req.body,
    sig,
    process.env.STRIPE_WEBHOOK_SECRET
  );

  switch (event.type) {
    case 'account.updated':
      // Vendor account updated
      await handleAccountUpdated(event.data.object);
      break;

    case 'payout.paid':
      // Payout completed
      await handlePayoutPaid(event.data.object);
      break;

    case 'payout.failed':
      // Payout failed
      await handlePayoutFailed(event.data.object);
      break;

    case 'payment_intent.succeeded':
      // Order paid
      await handlePaymentSucceeded(event.data.object);
      break;
  }

  return { received: true };
}
```

---

## Troubleshooting

### Stripe Connect account not verified

```bash
# Check account status
curl https://api.stripe.com/v1/accounts/acct_xxxxx \
  -u sk_test_PLACEHOLDER:

# Re-send onboarding link
nself plugin stripe connect --vendor-id vendor-id --refresh-link
```

### Payment split failing

```bash
# Verify vendor's Stripe account is active
nself plugin stripe connect --vendor-id vendor-id --status

# Check platform fee calculation
nself db query "
  SELECT
    order_number,
    total_amount,
    platform_fee,
    vendor_amount,
    (platform_fee + vendor_amount) AS sum_check
  FROM orders
  WHERE id = 'order-id'
"
```

### Payout failed

```bash
# Check payout status
nself db query "
  SELECT * FROM vendor_payouts
  WHERE status = 'failed'
  ORDER BY created_at DESC
"

# Retry payout
nself plugin stripe payout retry payout-id
```

---

## Next Steps

- **[Stripe Integration Guide](STRIPE-INTEGRATION.md)** - Advanced Stripe features
- **[Custom Domains](CUSTOM-DOMAINS.md)** - Vendor domain setup
- **[White-Label System](../features/WHITELABEL-SYSTEM.md)** - Complete branding
- **[SaaS Quick Start](QUICK-START-SAAS.md)** - Additional SaaS features

---

## Support

- **Documentation**: https://docs.nself.org
- **GitHub**: https://github.com/nself-org/cli
- **Discord**: https://discord.gg/nself

---

**Your marketplace is ready! Time to onboard your first vendors.**
