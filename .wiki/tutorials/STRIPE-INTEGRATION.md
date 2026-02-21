# Complete Stripe Integration Guide

Comprehensive guide to integrating Stripe for billing, subscriptions, usage-based pricing, and payment processing with nself.

**Time Estimate**: 30-40 minutes
**Difficulty**: Intermediate
**Prerequisites**: Stripe account, nself project initialized

---

## Table of Contents

1. [Stripe Account Setup](#stripe-account-setup)
2. [Plugin Installation](#plugin-installation)
3. [Webhook Configuration](#webhook-configuration)
4. [Subscription Billing](#subscription-billing)
5. [Usage-Based Billing](#usage-based-billing)
6. [One-Time Payments](#one-time-payments)
7. [Stripe Connect (Marketplaces)](#stripe-connect-marketplaces)
8. [Testing](#testing)
9. [Production Deployment](#production-deployment)
10. [Troubleshooting](#troubleshooting)

---

## Stripe Account Setup

### Step 1: Create Stripe Account (5 minutes)

1. **Go to** [stripe.com](https://stripe.com)
2. **Click** "Sign up"
3. **Enter** business information
4. **Complete** email verification

### Step 2: Get API Keys (2 minutes)

**Test Mode Keys** (for development):

1. Go to [Dashboard > Developers > API keys](https://dashboard.stripe.com/test/apikeys)
2. Copy **Publishable key** (starts with `pk_test_`)
3. Reveal and copy **Secret key** (starts with `sk_test_PLACEHOLDER`)

**Live Mode Keys** (for production):

1. **Complete** business verification first
2. Toggle to **Live mode** in dashboard
3. Go to API keys
4. Copy **Publishable key** (`pk_live_`)
5. Reveal and copy **Secret key** (`sk_live_`)

### Step 3: Business Settings (3 minutes)

**Configure**:
- Business name and address
- Support email and phone
- Statement descriptor (appears on customer statements)
- Tax settings (if applicable)

Go to: [Dashboard > Settings > Business settings](https://dashboard.stripe.com/settings/business)

---

## Plugin Installation

### Install Stripe Plugin (1 minute)

```bash
cd your-project
nself plugin install stripe
```

### Configure Environment Variables (2 minutes)

Edit `.env`:

```bash
# Stripe Configuration
STRIPE_API_KEY=sk_test_PLACEHOLDER_secret_key_here
STRIPE_PUBLISHABLE_KEY=pk_test_your_publishable_key_here
STRIPE_WEBHOOK_SECRET=whsec_your_webhook_secret_here  # Get this from webhook setup

# Optional Settings
STRIPE_API_VERSION=2024-12-18  # Latest API version
STRIPE_MAX_NETWORK_RETRIES=2
STRIPE_TIMEOUT=30000  # 30 seconds

# Currency (default: usd)
STRIPE_CURRENCY=usd

# Sync Settings
STRIPE_SYNC_INTERVAL=3600  # Sync every hour (in seconds)
STRIPE_AUTO_SYNC=true
```

### Rebuild and Restart (1 minute)

```bash
nself build
nself restart
```

### Verify Installation

```bash
# Check plugin status
nself plugin status stripe

# Test API connection
curl -u sk_test_PLACEHOLDER: https://api.stripe.com/v1/customers?limit=1
```

---

## Webhook Configuration

### Why Webhooks?

Webhooks notify your application of Stripe events in real-time:
- Payment succeeded/failed
- Subscription created/updated/cancelled
- Invoice paid/payment failed
- Customer updated

### Step 1: Create Webhook Endpoint (3 minutes)

**For Development (local testing)**:

```bash
# Install Stripe CLI
brew install stripe/stripe-cli/stripe

# Login
stripe login

# Forward webhooks to local
stripe listen --forward-to http://localhost/webhooks/stripe
```

**Output**:
```
> Ready! Your webhook signing secret is whsec_xxxxx
```

Copy the signing secret to `.env`:
```bash
STRIPE_WEBHOOK_SECRET=whsec_xxxxx
```

### Step 2: Configure Production Webhook (5 minutes)

**When deploying to production**:

1. Go to [Dashboard > Developers > Webhooks](https://dashboard.stripe.com/test/webhooks)
2. Click **"Add endpoint"**
3. Enter endpoint URL: `https://yourdomain.com/webhooks/stripe`
4. Select **API version**: Latest
5. **Select events to listen to**:

**Recommended events**:
```
✓ customer.created
✓ customer.updated
✓ customer.deleted
✓ customer.subscription.created
✓ customer.subscription.updated
✓ customer.subscription.deleted
✓ customer.subscription.trial_will_end
✓ invoice.created
✓ invoice.finalized
✓ invoice.paid
✓ invoice.payment_failed
✓ payment_intent.created
✓ payment_intent.succeeded
✓ payment_intent.payment_failed
✓ payment_method.attached
✓ payment_method.detached
✓ charge.succeeded
✓ charge.failed
✓ charge.refunded
```

6. Click **"Add endpoint"**
7. Copy **Signing secret** to `.env.prod`

### Step 3: Test Webhook (2 minutes)

```bash
# Trigger test event
stripe trigger payment_intent.succeeded

# Check webhook events
nself plugin stripe webhook list

# View event details
nself plugin stripe webhook show evt_xxxxx
```

---

## Subscription Billing

### Scenario: SaaS with Monthly Plans

#### Step 1: Create Products in Stripe (5 minutes)

**Option A: Via Stripe Dashboard**

1. Go to [Dashboard > Products](https://dashboard.stripe.com/test/products)
2. Click **"Add product"**
3. Enter product details:
   - Name: "Pro Plan"
   - Description: "Professional features"
   - Pricing: $29.99/month
   - Billing period: Monthly
   - Currency: USD
4. Click **"Save product"**
5. Repeat for other tiers (Basic, Enterprise)

**Option B: Via nself CLI**

```bash
# Create Pro Plan
nself plugin stripe products create \
  --name "Pro Plan" \
  --description "Professional features" \
  --price 29.99 \
  --interval month \
  --currency usd

# Create Basic Plan
nself plugin stripe products create \
  --name "Basic Plan" \
  --price 9.99 \
  --interval month

# Create Enterprise Plan
nself plugin stripe products create \
  --name "Enterprise Plan" \
  --price 99.99 \
  --interval month
```

#### Step 2: Sync Products to Database (1 minute)

```bash
nself plugin stripe sync --products
```

**Verify**:
```sql
SELECT * FROM stripe_products;
SELECT * FROM stripe_prices;
```

#### Step 3: Create Subscription (3 minutes)

**Frontend: Collect payment method**

```javascript
// client/src/checkout.js
import { loadStripe } from '@stripe/stripe-js';

const stripe = await loadStripe(process.env.REACT_APP_STRIPE_PUBLISHABLE_KEY);

// Create payment method
const { error, paymentMethod } = await stripe.createPaymentMethod({
  type: 'card',
  card: cardElement,
  billing_details: {
    name: customerName,
    email: customerEmail
  }
});

if (error) {
  console.error(error);
} else {
  // Send to backend
  await createSubscription(paymentMethod.id, priceId);
}
```

**Backend: Create subscription**

```javascript
// api/subscriptions/create.js
import Stripe from 'stripe';
const stripe = new Stripe(process.env.STRIPE_API_KEY);

export async function createSubscription(customerId, priceId, paymentMethodId) {
  // Attach payment method to customer
  await stripe.paymentMethods.attach(paymentMethodId, {
    customer: customerId
  });

  // Set as default payment method
  await stripe.customers.update(customerId, {
    invoice_settings: {
      default_payment_method: paymentMethodId
    }
  });

  // Create subscription
  const subscription = await stripe.subscriptions.create({
    customer: customerId,
    items: [{ price: priceId }],
    payment_behavior: 'default_incomplete',
    payment_settings: {
      save_default_payment_method: 'on_subscription'
    },
    expand: ['latest_invoice.payment_intent']
  });

  // Save to database
  await saveSubscriptionToDB(subscription);

  return subscription;
}
```

**GraphQL Mutation**:

```graphql
mutation CreateSubscription {
  insert_subscriptions_one(object: {
    user_id: "user-id"
    stripe_subscription_id: "sub_xxxxx"
    stripe_customer_id: "cus_xxxxx"
    status: "active"
    plan: "pro"
    current_period_start: "2026-01-01"
    current_period_end: "2026-02-01"
    price_id: "price_xxxxx"
    price_amount: 2999  # $29.99
    currency: "usd"
  }) {
    id
    status
  }
}
```

#### Step 4: Handle Subscription Changes (2 minutes)

**Upgrade/Downgrade**:

```javascript
export async function changeSubscription(subscriptionId, newPriceId) {
  const subscription = await stripe.subscriptions.retrieve(subscriptionId);

  // Update subscription
  const updatedSubscription = await stripe.subscriptions.update(subscriptionId, {
    items: [{
      id: subscription.items.data[0].id,
      price: newPriceId
    }],
    proration_behavior: 'create_prorations'  // Prorate the difference
  });

  // Update database
  await updateSubscriptionInDB(updatedSubscription);

  return updatedSubscription;
}
```

**Cancel Subscription**:

```javascript
export async function cancelSubscription(subscriptionId, immediate = false) {
  const options = immediate ? {} : { at_period_end: true };

  const subscription = await stripe.subscriptions.cancel(subscriptionId, options);

  // Update database
  await updateSubscriptionInDB(subscription);

  return subscription;
}
```

---

## Usage-Based Billing

### Scenario: API Usage Pricing

**$0.01 per API call, billed monthly**

#### Step 1: Create Usage-Based Product (3 minutes)

**Via Stripe Dashboard**:
1. Create product: "API Calls"
2. Pricing model: **Usage-based**
3. Unit price: $0.01
4. Billing period: Monthly
5. Usage aggregation: Sum

**Via CLI**:
```bash
nself plugin stripe products create \
  --name "API Calls" \
  --type usage \
  --price 0.01 \
  --interval month \
  --usage-type metered \
  --aggregate sum
```

#### Step 2: Track Usage (2 minutes)

**After each API call**:

```javascript
// api/middleware/track-usage.js
export async function trackAPIUsage(userId, subscriptionId) {
  // Get subscription item ID
  const subscription = await getSubscription(subscriptionId);
  const usageItemId = subscription.items.data.find(
    item => item.price.product.name === 'API Calls'
  ).id;

  // Report usage to Stripe
  await stripe.subscriptionItems.createUsageRecord(usageItemId, {
    quantity: 1,  // 1 API call
    timestamp: Math.floor(Date.now() / 1000),
    action: 'increment'
  });

  // Also log in database
  await logUsageInDB(userId, subscriptionId, 'api_calls', 1);
}
```

**Batch reporting** (more efficient):

```javascript
// Cron job: Every hour
export async function reportBatchUsage() {
  // Get usage counts from database
  const usageCounts = await getHourlyUsageCounts();

  for (const { subscriptionItemId, quantity } of usageCounts) {
    await stripe.subscriptionItems.createUsageRecord(subscriptionItemId, {
      quantity: quantity,
      timestamp: Math.floor(Date.now() / 1000),
      action: 'increment'
    });
  }
}
```

#### Step 3: Query Usage (1 minute)

```javascript
export async function getUsageSummary(subscriptionItemId) {
  const usageRecords = await stripe.subscriptionItems.listUsageRecordSummaries(
    subscriptionItemId,
    {
      limit: 10
    }
  );

  return usageRecords.data;
}
```

---

## One-Time Payments

### Scenario: Product Purchases

#### Step 1: Create Payment Intent (3 minutes)

```javascript
// api/payments/create-intent.js
export async function createPaymentIntent(amount, currency, customerId) {
  const paymentIntent = await stripe.paymentIntents.create({
    amount: Math.round(amount * 100),  // cents
    currency: currency,
    customer: customerId,
    payment_method_types: ['card'],
    metadata: {
      product_id: 'product-id',
      user_id: 'user-id'
    }
  });

  return paymentIntent;
}
```

#### Step 2: Frontend Payment Form (5 minutes)

```javascript
// client/src/PaymentForm.jsx
import { CardElement, useStripe, useElements } from '@stripe/react-stripe-js';

export function PaymentForm({ amount, onSuccess }) {
  const stripe = useStripe();
  const elements = useElements();

  const handleSubmit = async (event) => {
    event.preventDefault();

    if (!stripe || !elements) return;

    // Create payment intent on backend
    const { clientSecret } = await fetch('/api/payments/create-intent', {
      method: 'POST',
      body: JSON.stringify({ amount: amount })
    }).then(r => r.json());

    // Confirm payment
    const { error, paymentIntent } = await stripe.confirmCardPayment(
      clientSecret,
      {
        payment_method: {
          card: elements.getElement(CardElement)
        }
      }
    );

    if (error) {
      console.error(error);
    } else if (paymentIntent.status === 'succeeded') {
      onSuccess(paymentIntent);
    }
  };

  return (
    <form onSubmit={handleSubmit}>
      <CardElement />
      <button type="submit" disabled={!stripe}>
        Pay ${amount}
      </button>
    </form>
  );
}
```

#### Step 3: Handle Webhook (2 minutes)

```javascript
// api/webhooks/stripe.js
export async function handlePaymentIntentSucceeded(paymentIntent) {
  // Save payment to database
  await savePaymentToDB({
    stripe_payment_intent_id: paymentIntent.id,
    user_id: paymentIntent.metadata.user_id,
    amount: paymentIntent.amount / 100,
    currency: paymentIntent.currency,
    status: 'succeeded'
  });

  // Fulfill order
  await fulfillOrder(paymentIntent.metadata.product_id);

  // Send receipt email
  await sendReceiptEmail(paymentIntent.customer);
}
```

---

## Stripe Connect (Marketplaces)

### For multi-vendor platforms

#### Step 1: Enable Connect (3 minutes)

1. Go to [Dashboard > Connect > Settings](https://dashboard.stripe.com/test/connect/accounts/overview)
2. Click **"Get started"**
3. Choose **Account type**: Express (recommended) or Standard
4. Copy **Client ID** (starts with `ca_`)

Edit `.env`:
```bash
STRIPE_CONNECT_ENABLED=true
STRIPE_CONNECT_CLIENT_ID=ca_your_client_id_here
PLATFORM_FEE_PERCENTAGE=15.0
```

#### Step 2: Onboard Vendors (5 minutes)

```javascript
// api/vendors/create-connect-account.js
export async function createConnectAccount(vendorEmail) {
  // Create Connect account
  const account = await stripe.accounts.create({
    type: 'express',
    email: vendorEmail,
    capabilities: {
      card_payments: { requested: true },
      transfers: { requested: true }
    }
  });

  // Create onboarding link
  const accountLink = await stripe.accountLinks.create({
    account: account.id,
    refresh_url: 'https://yoursite.com/vendor/connect/refresh',
    return_url: 'https://yoursite.com/vendor/connect/complete',
    type: 'account_onboarding'
  });

  return {
    accountId: account.id,
    onboardingUrl: accountLink.url
  };
}
```

#### Step 3: Process Split Payments (3 minutes)

```javascript
// api/orders/create-with-split.js
export async function createOrderWithSplit(vendorAccountId, amount, platformFee) {
  const vendorAmount = amount - platformFee;

  // Create payment with destination
  const paymentIntent = await stripe.paymentIntents.create({
    amount: Math.round(amount * 100),
    currency: 'usd',
    application_fee_amount: Math.round(platformFee * 100),  // Platform fee
    transfer_data: {
      destination: vendorAccountId  // Vendor receives vendorAmount
    },
    metadata: {
      vendor_account_id: vendorAccountId,
      platform_fee: platformFee
    }
  });

  return paymentIntent;
}
```

#### Step 4: Payout to Vendors (2 minutes)

**Automatic** (Stripe handles):
- Vendors receive payouts based on their payout schedule
- Default: 2-day rolling basis

**Manual** (you control):
```javascript
export async function payoutToVendor(vendorAccountId, amount) {
  const payout = await stripe.payouts.create(
    {
      amount: Math.round(amount * 100),
      currency: 'usd'
    },
    {
      stripeAccount: vendorAccountId  // Vendor's Connect account
    }
  );

  return payout;
}
```

---

## Testing

### Test Mode vs Live Mode

**Test Mode**:
- Use `sk_test_PLACEHOLDER` and `pk_test_` keys
- No real charges
- Test card numbers work

**Live Mode**:
- Use `sk_live_` and `pk_live_` keys
- Real charges
- Real credit cards only

### Test Card Numbers

**Successful payments**:
```
4242 4242 4242 4242  # Visa
5555 5555 5555 4444  # Mastercard
3782 822463 10005    # American Express
```

**Failed payments**:
```
4000 0000 0000 0002  # Card declined
4000 0000 0000 9995  # Insufficient funds
4000 0000 0000 0069  # Charge expired
```

**3D Secure**:
```
4000 0027 6000 3184  # 3DS required, succeeds
4000 0082 6000 3178  # 3DS required, fails
```

**Expiry**: Any future date (e.g., 12/34)
**CVC**: Any 3 digits (e.g., 123)

### Testing Webhooks Locally

```bash
# Start Stripe CLI listener
stripe listen --forward-to http://localhost/webhooks/stripe

# In another terminal, trigger events
stripe trigger payment_intent.succeeded
stripe trigger customer.subscription.created
stripe trigger invoice.payment_failed
```

### Verify Integration

```bash
# Check Stripe data sync
nself plugin stripe sync

# View customers
nself plugin stripe customers list

# View subscriptions
nself plugin stripe subscriptions list

# View invoices
nself plugin stripe invoices list --status paid

# Check webhooks
nself plugin stripe webhook stats
```

---

## Production Deployment

### Pre-Launch Checklist

- [ ] Business verification complete in Stripe
- [ ] Live API keys obtained
- [ ] Webhook endpoint configured with live keys
- [ ] SSL certificate active on domain
- [ ] Test all payment flows with live keys in test environment
- [ ] Error handling implemented
- [ ] Email receipts configured
- [ ] Refund policy documented
- [ ] Terms of service updated
- [ ] Privacy policy includes payment processing

### Switch to Live Mode (5 minutes)

1. **Complete business verification**
   - [Dashboard > Settings > Business settings](https://dashboard.stripe.com/settings/business)

2. **Get live API keys**
   - Toggle to Live mode
   - Copy keys

3. **Update production environment**

Edit `.env.prod`:
```bash
# REMOVE test keys
# STRIPE_API_KEY=sk_test_PLACEHOLDER
# STRIPE_PUBLISHABLE_KEY=pk_test_xxxxx

# ADD live keys
STRIPE_API_KEY=sk_live_your_live_key_here
STRIPE_PUBLISHABLE_KEY=pk_live_your_live_key_here
```

4. **Configure live webhook**
   - Create endpoint in Live mode
   - Copy signing secret to `.env.prod`

5. **Deploy**
```bash
nself deploy prod
```

6. **Test with real card**
   - Use your own card
   - Make small test purchase
   - Immediately refund

### Monitoring

**Stripe Dashboard**:
- Monitor payments
- Track failed charges
- View customer activity

**nself Monitoring**:
```bash
# View webhook events
nself plugin stripe webhook list --live

# Check failed payments
nself plugin stripe payments list --status failed

# View subscription churn
nself plugin stripe subscriptions stats
```

---

## Troubleshooting

### Common Issues

#### "No such customer"

**Cause**: Customer ID not found in Stripe

**Fix**:
```bash
# Verify customer exists
curl -u sk_test_PLACEHOLDER: https://api.stripe.com/v1/customers/cus_xxxxx

# Create customer if missing
nself plugin stripe customers create --email user@example.com
```

#### "This customer has no attached payment source"

**Cause**: No payment method saved

**Fix**:
```javascript
// Attach payment method
await stripe.paymentMethods.attach(paymentMethodId, {
  customer: customerId
});

// Set as default
await stripe.customers.update(customerId, {
  invoice_settings: {
    default_payment_method: paymentMethodId
  }
});
```

#### "Webhook signature verification failed"

**Cause**: Incorrect webhook secret

**Fix**:
1. Get correct secret from Stripe Dashboard
2. Update `.env`:
   ```bash
   STRIPE_WEBHOOK_SECRET=whsec_correct_secret_here
   ```
3. Restart:
   ```bash
   nself restart
   ```

#### "Rate limit exceeded"

**Cause**: Too many API requests

**Fix**:
```javascript
// Implement retry logic
import Stripe from 'stripe';
const stripe = new Stripe(process.env.STRIPE_API_KEY, {
  maxNetworkRetries: 2,
  timeout: 30000
});

// Use batch operations
// Instead of creating 100 customers one by one,
// create them in batches with delays
```

#### Payments stuck in "processing"

**Cause**: 3D Secure authentication pending

**Fix**: Implement 3DS2 handling:
```javascript
const { error, paymentIntent } = await stripe.confirmCardPayment(
  clientSecret,
  {
    payment_method: paymentMethodId,
    return_url: 'https://yoursite.com/payment/complete'  // Required for 3DS
  }
);

if (paymentIntent.status === 'requires_action') {
  // Handle 3DS authentication
  const { error: confirmError } = await stripe.handleCardAction(clientSecret);
}
```

### Debug Mode

Enable verbose logging:

```bash
# In .env
STRIPE_DEBUG=true
LOG_LEVEL=debug

nself restart
nself logs | grep stripe
```

---

## Best Practices

### Security

1. **Never expose secret keys**
   - Keep in `.env`, `.env.secrets`
   - Don't commit to git
   - Use environment variables

2. **Verify webhook signatures**
   ```javascript
   const event = stripe.webhooks.constructEvent(
     req.body,
     signature,
     webhookSecret
   );
   ```

3. **Use HTTPS only**
   - All Stripe communication must be over HTTPS
   - Configure SSL: `nself ssl enable`

### Performance

1. **Batch API calls**
   ```javascript
   // Bad: 100 individual calls
   for (const customer of customers) {
     await stripe.customers.create(customer);
   }

   // Good: Batch with delays
   const batches = chunk(customers, 10);
   for (const batch of batches) {
     await Promise.all(batch.map(c => stripe.customers.create(c)));
     await sleep(1000);
   }
   ```

2. **Cache product/price data**
   - Sync to database
   - Query database instead of Stripe API
   - Refresh periodically

3. **Use webhooks, not polling**
   - Don't poll for payment status
   - Listen to webhooks for updates

### User Experience

1. **Handle all payment states**
   - `requires_payment_method` - Payment failed, retry
   - `requires_action` - 3DS authentication needed
   - `processing` - Show pending state
   - `succeeded` - Payment complete

2. **Show clear error messages**
   ```javascript
   const errorMessages = {
     'card_declined': 'Your card was declined. Please try another card.',
     'insufficient_funds': 'Insufficient funds. Please try another payment method.',
     'expired_card': 'Your card has expired. Please try another card.'
   };
   ```

3. **Send receipts**
   - Stripe can email receipts automatically
   - Or send custom receipts via your email system

---

## Resources

- **[Stripe API Documentation](https://stripe.com/docs/api)** - Complete API reference
- **[Stripe Testing](https://stripe.com/docs/testing)** - Test card numbers
- **[Webhooks Guide](https://stripe.com/docs/webhooks)** - Webhook implementation
- **[Stripe Connect](https://stripe.com/docs/connect)** - Marketplace payments
- **[nself Plugin Docs](../plugins/stripe.md)** - nself Stripe plugin

---

## Support

- **Stripe Support**: support@stripe.com
- **nself Discord**: https://discord.gg/nself
- **GitHub Issues**: https://github.com/nself-org/cli/issues

---

**Your Stripe integration is complete! Start accepting payments.**
