# Billing & Usage Tracking Guide

**nself v0.9.0** | Complete guide to billing, usage metering, and subscription management

---

## Table of Contents

1. [Overview](#overview)
2. [Core Concepts](#core-concepts)
3. [Getting Started](#getting-started)
4. [Usage Tracking](#usage-tracking)
5. [Stripe Integration](#stripe-integration)
6. [Quota System](#quota-system)
7. [Pricing Plans](#pricing-plans)
8. [Invoice Management](#invoice-management)
9. [Reporting & Analytics](#reporting--analytics)
10. [Best Practices](#best-practices)
11. [Advanced Topics](#advanced-topics)

---

## Overview

### What is the nself Billing System?

The nself billing system provides comprehensive usage-based billing and subscription management infrastructure. It's a complete solution for SaaS pricing, metering, and revenue operations that integrates seamlessly with Stripe.

### Architecture

The billing system consists of four main components:

1. **Usage Metering** - Automatic tracking of resource consumption
2. **Quota Management** - Plan-based limits and enforcement
3. **Stripe Integration** - Payment processing and subscription management
4. **Analytics & Reporting** - Revenue insights and usage trends

```
┌─────────────────┐      Usage Events      ┌──────────────────┐
│  Application    │ ──────────────────────► │  Usage Meter     │
│  Services       │                         │  (PostgreSQL)    │
└─────────────────┘                         └────────┬─────────┘
                                                     │
                                                     │ Aggregation
                                                     ▼
┌─────────────────┐      Webhooks          ┌──────────────────┐
│  Stripe API     │ ◄─────────────────────►│  nself Billing   │
│  (Payments)     │      Sync              │  Engine          │
└─────────────────┘                         └──────────────────┘
```

### Use Cases

- **SaaS Pricing** - Tiered subscriptions with usage-based billing
- **API Monetization** - Per-request pricing with quotas
- **Multi-Tenant Billing** - Isolated billing per tenant or organization
- **Metered Services** - Storage, bandwidth, compute usage tracking
- **Enterprise Licensing** - Custom quotas and pricing
- **Freemium Models** - Free tier with paid upgrade paths

### Comparison to Other Solutions

| Feature | nself Billing | Stripe Only | Chargebee | Lago |
|---------|--------------|-------------|-----------|------|
| **Usage Metering** | ✅ Built-in | ❌ Manual | ✅ Built-in | ✅ Built-in |
| **Quota Enforcement** | ✅ Real-time | ❌ No | ✅ Limited | ✅ Yes |
| **Stripe Integration** | ✅ Native | ✅ Native | ✅ Yes | ✅ Yes |
| **Multi-Tenancy** | ✅ Built-in | ❌ Manual | ✅ Yes | ❌ Limited |
| **Self-Hosted** | ✅ Yes | ❌ No | ❌ No | ✅ Yes |
| **Open Source** | ✅ Yes | ❌ No | ❌ No | ✅ Yes |
| **Cost** | Free (self-host) | Stripe fees only | $249+/mo | Free (self-host) |

---

## Core Concepts

### 1. Customers

A **customer** represents a billing entity - typically an organization or individual user.

**Customer Properties:**
- Stripe customer ID (synced from Stripe)
- Organization ID (maps to nself organizations)
- Billing email and contact info
- Payment methods
- Billing address
- Tax information

**Customer Lifecycle:**
1. User/organization signs up
2. Customer record created in nself and Stripe
3. Payment method added
4. Subscription created
5. Usage tracked and billed
6. Invoices generated and paid

### 2. Subscriptions

**Subscriptions** define the pricing plan and billing cycle for a customer.

**Subscription Properties:**
- Plan name (free, starter, pro, enterprise)
- Billing interval (monthly, yearly)
- Current status (active, trialing, past_due, canceled)
- Current period start/end dates
- Stripe subscription ID
- Auto-renewal settings

**Subscription States:**
- `trialing` - Free trial period (no charges)
- `active` - Subscription is paid and active
- `past_due` - Payment failed, grace period
- `canceled` - Subscription terminated
- `paused` - Temporarily suspended

### 3. Usage Metering

**Usage metering** tracks resource consumption automatically across different services.

**Tracked Services:**
- **API Requests** - Per-endpoint request counting
- **Storage** - GB-hours of storage used (MinIO, PostgreSQL)
- **Bandwidth** - GB of data transferred in/out
- **Compute** - CPU-hours from Functions or custom services
- **Database** - Query count, connection hours
- **Functions** - Invocation count and execution duration

**Usage Events:**
Each usage event contains:
- Timestamp (when the usage occurred)
- Service type (api, storage, bandwidth, etc.)
- Quantity (how much was used)
- Customer/organization ID
- Metadata (additional context)

### 4. Quotas and Limits

**Quotas** define usage limits per pricing plan.

**Quota Types:**
- **Soft Limits** - Warning triggered, usage continues
- **Hard Limits** - Usage blocked when exceeded
- **Overage Billing** - Additional charges beyond quota

**Quota Dimensions:**
- Per-service quotas (API quota separate from storage quota)
- Time-based quotas (monthly, daily, hourly)
- Per-organization or per-tenant
- Global vs. resource-specific

### 5. Billing Cycles

**Billing cycles** determine when usage is measured and invoiced.

**Cycle Types:**
- **Calendar Month** - 1st to last day of month
- **Subscription Anniversary** - Based on start date
- **Custom Period** - Defined start/end dates

**Billing Process:**
1. Cycle begins (start date)
2. Usage tracked throughout cycle
3. Cycle ends (end date)
4. Usage aggregated and calculated
5. Invoice generated with line items
6. Payment attempted via Stripe
7. Next cycle begins

### 6. Invoices

**Invoices** itemize charges for a billing period.

**Invoice Components:**
- Subscription base price
- Usage-based charges (metered services)
- One-time charges
- Discounts and credits
- Taxes
- Total amount due

**Invoice States:**
- `draft` - Being prepared, not finalized
- `open` - Finalized, awaiting payment
- `paid` - Payment successful
- `void` - Canceled, not collectible
- `uncollectible` - Payment failed permanently

---

## Getting Started

### Prerequisites

Before using the billing system, ensure you have:

1. **nself installed** - Version 0.9.0 or later
2. **PostgreSQL running** - Billing tables will be created
3. **Stripe account** - Sign up at [stripe.com](https://stripe.com)
4. **Stripe API keys** - Get from Stripe Dashboard

### Initialize Billing System

Create the billing database schema and configuration:

```bash
# Initialize billing tables and functions
nself billing init

# This creates:
# - billing.customers table
# - billing.subscriptions table
# - billing.usage_events table
# - billing.quotas table
# - billing.invoices table (synced from Stripe)
# - billing.plans table
# - Metering functions
# - Quota enforcement triggers
```

### Configure Stripe

Add your Stripe API credentials to `.env`:

```bash
# Stripe Configuration
STRIPE_API_KEY=sk_test_PLACEHOLDER           # Secret key from Stripe
STRIPE_WEBHOOK_SECRET=whsec_xxxxxxxxxxxx      # Webhook signing secret
STRIPE_PUBLISHABLE_KEY=pk_test_xxxxxxxxxxxx   # Public key for client-side

# Billing Settings
BILLING_ENABLED=true
BILLING_CURRENCY=usd                          # Currency code
BILLING_DEFAULT_PLAN=free                     # Default plan for new users
BILLING_TRIAL_DAYS=14                         # Free trial length
```

**Getting Stripe Keys:**

1. Go to [Stripe Dashboard](https://dashboard.stripe.com)
2. Navigate to **Developers → API Keys**
3. Copy **Secret key** and **Publishable key**
4. For webhooks, go to **Developers → Webhooks** (see [Stripe Integration](#stripe-integration))

### Test Mode vs. Production

Stripe provides separate keys for testing and production:

**Test Mode (Development)**:
```bash
ENV=dev
STRIPE_API_KEY=sk_test_PLACEHOLDER
```
- Uses test cards (4242 4242 4242 4242)
- No real charges
- Safe for development

**Production (Live)**:
```bash
ENV=prod
STRIPE_API_KEY=sk_live_xxxxxxxxxxxx
```
- Real charges to real cards
- Requires PCI compliance
- Use with caution

### Create Your First Plan

Define pricing plans with quotas:

```bash
# Create free plan
nself billing plan create free \
  --price 0 \
  --interval month \
  --quota-api 10000 \
  --quota-storage 1 \
  --quota-bandwidth 5

# Output:
# ✓ Plan created: free
#   Price: $0/month
#   Quotas:
#     - API Requests: 10,000/month
#     - Storage: 1 GB
#     - Bandwidth: 5 GB/month

# Create pro plan with usage-based pricing
nself billing plan create pro \
  --price 49 \
  --interval month \
  --quota-api 100000 \
  --quota-storage 50 \
  --quota-bandwidth 100 \
  --overage-api 0.001 \
  --overage-storage 0.10 \
  --overage-bandwidth 0.05

# Output:
# ✓ Plan created: pro
#   Base Price: $49/month
#   Quotas:
#     - API Requests: 100,000/month (overage: $0.001/req)
#     - Storage: 50 GB (overage: $0.10/GB)
#     - Bandwidth: 100 GB/month (overage: $0.05/GB)
```

### Quick Start Example

Complete setup from scratch:

```bash
# 1. Initialize billing
nself billing init

# 2. Configure Stripe in .env
echo "STRIPE_API_KEY=sk_test_PLACEHOLDER_key_here" >> .env
echo "BILLING_ENABLED=true" >> .env

# 3. Restart services to load new config
nself restart

# 4. Create pricing plans
nself billing plan create free --price 0 --quota-api 10000
nself billing plan create starter --price 19 --quota-api 50000
nself billing plan create pro --price 49 --quota-api 200000

# 5. Check current usage (starts at 0)
nself billing usage

# 6. View available plans
nself billing plan list

# Output:
#  name     | price  | interval | api_quota | storage_quota | status
# ----------|--------|----------|-----------|---------------|--------
#  free     | $0     | month    | 10,000    | 1 GB          | active
#  starter  | $19    | month    | 50,000    | 10 GB         | active
#  pro      | $49    | month    | 200,000   | 50 GB         | active
```

---

## Usage Tracking

### Automatic Metering

nself automatically tracks usage across all enabled services. No manual instrumentation required.

**How It Works:**

1. **Request Middleware** - Every API request is counted
2. **Storage Monitors** - MinIO and PostgreSQL usage polled hourly
3. **Bandwidth Tracking** - Nginx logs parsed for data transfer
4. **Function Execution** - Runtime duration and invocations tracked
5. **Database Activity** - Query count and connection time measured

### Viewing Current Usage

```bash
# Show current billing period usage
nself billing usage

# Output:
# Usage Report: 2026-01-01 to 2026-01-29
#
#  Service        | Usage          | Quota         | % Used | Status
# ----------------|----------------|---------------|--------|--------
#  API Requests   | 45,231         | 100,000       | 45%    | OK
#  Storage        | 12.3 GB        | 50 GB         | 25%    | OK
#  Bandwidth      | 23.5 GB        | 100 GB        | 24%    | OK
#  Compute        | 145 CPU-hours  | Unlimited     | -      | OK
#  Database       | 1.2M queries   | Unlimited     | -      | OK
#  Functions      | 8,234 calls    | 100,000       | 8%     | OK
#
# Total Estimated Cost: $49.00 (base) + $0.00 (overage) = $49.00
```

### Service-Specific Usage

```bash
# Check API usage only
nself billing usage --service=api

# Output:
# API Usage Report: 2026-01-01 to 2026-01-29
#
#  Endpoint                        | Requests | Avg Response | Error Rate
# ---------------------------------|----------|--------------|------------
#  POST /graphql                   | 28,442   | 125ms        | 0.2%
#  GET /api/users                  | 8,321    | 45ms         | 0.0%
#  POST /api/auth/login            | 4,521    | 89ms         | 1.1%
#  GET /api/products               | 3,947    | 67ms         | 0.1%
#
# Total: 45,231 requests
# Quota: 100,000 requests/month
# Remaining: 54,769 (55%)

# Check storage usage with breakdown
nself billing usage --service=storage --detailed

# Output:
# Storage Usage Report: 2026-01-01 to 2026-01-29
#
#  Resource Type      | Usage      | % of Total
# --------------------|------------|------------
#  PostgreSQL         | 5.2 GB     | 42%
#  MinIO (S3)         | 6.8 GB     | 55%
#  Redis              | 0.3 GB     | 3%
#
# Total: 12.3 GB
# Quota: 50 GB
# Remaining: 37.7 GB (75%)
```

### Historical Usage

```bash
# Last month's usage
nself billing usage --period=last-month

# Output:
# Usage Report: 2025-12-01 to 2025-12-31
#
#  Service        | Usage          | Quota         | % Used | Status
# ----------------|----------------|---------------|--------|--------
#  API Requests   | 89,234         | 100,000       | 89%    | OK
#  Storage        | 10.5 GB        | 50 GB         | 21%    | OK
#  Bandwidth      | 45.2 GB        | 100 GB        | 45%    | OK

# Custom date range
nself billing usage \
  --period=custom \
  --start=2025-12-01 \
  --end=2025-12-15

# Output shows usage for specified dates
```

### Manual Usage Recording

For custom services or external integrations:

```bash
# Record API usage
nself billing usage record api 1000

# Record storage usage (in GB)
nself billing usage record storage 5.5

# Record with metadata
nself billing usage record api 500 \
  --metadata '{"endpoint": "/api/custom", "user_id": "123"}'

# Batch import from CSV
nself billing usage import usage_data.csv

# CSV format:
# timestamp,service,quantity,metadata
# 2026-01-29T10:00:00Z,api,1000,{"endpoint":"/api/test"}
```

### Real-Time Usage Dashboards

View usage in real-time via Hasura GraphQL:

```graphql
# Get current usage for organization
query GetOrganizationUsage($orgId: uuid!) {
  billing_usage_current(
    where: { org_id: { _eq: $orgId } }
  ) {
    service
    current_usage
    quota
    percentage_used
    status
    updated_at
  }
}
```

**Response:**
```json
{
  "billing_usage_current": [
    {
      "service": "api",
      "current_usage": 45231,
      "quota": 100000,
      "percentage_used": 45.23,
      "status": "ok",
      "updated_at": "2026-01-29T10:30:00Z"
    },
    {
      "service": "storage",
      "current_usage": 12.3,
      "quota": 50,
      "percentage_used": 24.6,
      "status": "ok",
      "updated_at": "2026-01-29T10:30:00Z"
    }
  ]
}
```

### Usage Alerts

Configure alerts when approaching quotas:

```bash
# Set alert at 80% of quota
nself billing quota alert api --threshold 80

# Set multiple thresholds
nself billing quota alert storage --threshold 75 --threshold 90 --threshold 95

# Configure alert destinations
nself billing quota alert config \
  --email admin@example.com \
  --webhook https://example.com/alerts \
  --slack https://hooks.slack.com/services/xxx
```

**Alert Email Example:**
```
Subject: [nself] API Quota Alert - 85% Used

Your organization "Acme Corp" has used 85% of your monthly API quota:

Current Usage: 85,000 requests
Quota: 100,000 requests
Remaining: 15,000 requests (15%)

You're on the Pro plan ($49/month).

Actions:
- Upgrade to Enterprise for unlimited API requests
- Review API usage patterns to optimize
- Contact support for custom quota increases

View usage: https://admin.nself.org/billing/usage
```

### Usage Export

Export usage data for external analysis:

```bash
# Export as CSV
nself billing export usage --format=csv --output=usage_2026_01.csv

# Export as JSON
nself billing export usage --format=json --output=usage_2026_01.json

# Export specific service
nself billing export usage --service=api --format=csv

# Export with date range
nself billing export usage \
  --start=2026-01-01 \
  --end=2026-01-31 \
  --format=csv
```

**CSV Output:**
```csv
timestamp,service,quantity,org_id,metadata
2026-01-29T10:00:00Z,api,125,123e4567-...,"{""endpoint"":""/graphql""}"
2026-01-29T10:01:00Z,storage,12.3,123e4567-...,"{""type"":""postgres""}"
2026-01-29T10:02:00Z,bandwidth,0.5,123e4567-...,"{""direction"":""egress""}"
```

---

## Stripe Integration

### Setup Stripe Account

1. **Sign up** at [stripe.com](https://stripe.com)
2. **Activate account** (provide business info)
3. **Get API keys** from Dashboard → Developers → API Keys
4. **Enable test mode** for development

### Configure Webhooks

Stripe sends webhook events for subscription and payment updates. Configure webhooks to keep nself in sync:

**1. Create Webhook Endpoint:**

Go to Stripe Dashboard → Developers → Webhooks → Add endpoint

**Endpoint URL:**
```
https://your-domain.com/webhooks/stripe
```

**2. Select Events to Listen:**
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
✓ payment_intent.succeeded
✓ payment_intent.payment_failed
✓ payment_method.attached
✓ payment_method.detached
```

**3. Copy Webhook Signing Secret:**

After creating the webhook, copy the signing secret (starts with `whsec_`).

Add to `.env`:
```bash
STRIPE_WEBHOOK_SECRET=whsec_xxxxxxxxxxxx
```

**4. Test Webhooks (Local Development):**

Use Stripe CLI to forward webhooks to localhost:

```bash
# Install Stripe CLI
brew install stripe/stripe-cli/stripe

# Login
stripe login

# Forward webhooks to local server
stripe listen --forward-to http://localhost/webhooks/stripe

# Output:
# > Ready! Your webhook signing secret is whsec_xxxxx (^C to quit)
# Use this secret in your .env file
```

### Customer Management

Create and manage Stripe customers from nself:

```bash
# Create customer (automatic with first subscription)
nself billing customer show

# Output:
# Customer: cus_NxxxxxxxxxxxxxxA
# Organization: Acme Corp
# Email: billing@acmecorp.com
# Created: 2026-01-15
# Payment Methods: 1 card on file
# Subscriptions: 1 active (Pro Plan)
# Lifetime Value: $147 (3 months)

# Update customer info
nself billing customer update \
  --email new-billing@acmecorp.com \
  --name "Acme Corporation LLC" \
  --address "123 Main St" \
  --city "San Francisco" \
  --state "CA" \
  --postal_code "94102" \
  --country "US"

# Open Stripe Customer Portal (for self-service)
nself billing customer portal

# Output:
# Customer Portal URL (expires in 1 hour):
# https://billing.stripe.com/p/session/xxxxx
```

### Subscription Lifecycle

```bash
# View current subscription
nself billing subscription show

# Output:
# Subscription: sub_1234567890abcdef
# Plan: Pro ($49/month)
# Status: active
# Current Period: 2026-01-15 to 2026-02-15
# Next Invoice: 2026-02-15 ($49.00)
# Cancel at Period End: No

# List available plans
nself billing subscription plans

# Output:
#  name        | price   | interval | features
# -------------|---------|----------|------------------------------------------
#  free        | $0      | month    | 10K API, 1GB storage, Community support
#  starter     | $19     | month    | 50K API, 10GB storage, Email support
#  pro         | $49     | month    | 200K API, 50GB storage, Priority support
#  enterprise  | Custom  | -        | Unlimited, Custom SLA, Dedicated support

# Upgrade subscription
nself billing subscription upgrade enterprise

# Output:
# ✓ Subscription upgraded to Enterprise
#   Proration credit: -$15.67
#   New monthly charge: $299.00
#   Effective immediately

# Downgrade subscription (at period end)
nself billing subscription downgrade starter

# Output:
# ✓ Subscription will downgrade to Starter on 2026-02-15
#   You will continue to have Pro access until then
#   New monthly charge (starting 2026-02-15): $19.00

# Cancel subscription
nself billing subscription cancel

# Interactive prompt:
# Are you sure you want to cancel your Pro subscription? (y/N): y
# Cancel immediately or at period end? (immediate/end) [end]: end
#
# ✓ Subscription will be canceled on 2026-02-15
#   You will continue to have access until then
#   No refunds for partial months

# Reactivate canceled subscription
nself billing subscription reactivate

# Output:
# ✓ Subscription reactivated
#   You will be billed $49.00 on 2026-02-15
#   Cancellation removed
```

### Payment Methods

```bash
# List payment methods
nself billing payment list

# Output:
#  id           | type | brand | last4 | exp_date | default
# --------------|------|-------|-------|----------|--------
#  pm_1abc...   | card | Visa  | 4242  | 12/2028  | Yes
#  pm_2def...   | card | MC    | 5555  | 06/2027  | No

# Add payment method (opens Stripe Checkout)
nself billing payment add

# Output:
# Opening Stripe Checkout...
# Complete payment method setup in your browser
# Checkout URL: https://checkout.stripe.com/c/pay/xxxxx

# Set default payment method
nself billing payment default pm_1abc...

# Output:
# ✓ Default payment method updated to Visa ending in 4242

# Remove payment method
nself billing payment remove pm_2def...

# Output:
# ✓ Payment method removed (MC ending in 5555)
```

### Handling Webhooks

Webhook events are automatically processed by nself. Monitor webhook activity:

```bash
# List recent webhook events
nself billing webhook list

# Output:
#  event_id      | type                           | status  | received_at
# ---------------|--------------------------------|---------|--------------------
#  evt_1abc...   | customer.subscription.updated  | success | 2026-01-29 10:15:00
#  evt_2def...   | invoice.paid                   | success | 2026-01-15 08:30:00
#  evt_3ghi...   | payment_intent.succeeded       | success | 2026-01-15 08:30:00

# View webhook details
nself billing webhook show evt_1abc...

# Output:
# Event: evt_1abc...
# Type: customer.subscription.updated
# Status: success
# Received: 2026-01-29 10:15:00
# Processed: 2026-01-29 10:15:01
# Payload:
# {
#   "subscription": {
#     "id": "sub_123",
#     "status": "active",
#     "current_period_end": 1739491200
#   }
# }

# Test webhook endpoint
nself billing webhook test

# Output:
# Testing webhook endpoint...
# ✓ Webhook signature verified
# ✓ Event processed successfully
# Test event: customer.subscription.updated (test mode)

# Retry failed webhook
nself billing webhook retry evt_4jkl...

# Output:
# Retrying webhook evt_4jkl...
# ✓ Webhook processed successfully
```

---

## Quota System

### Understanding Quotas

Quotas limit usage to prevent abuse and manage costs. Each pricing plan has different quota levels.

**Quota Enforcement Modes:**

1. **Soft Limit (Warning)**
   - Usage continues after threshold
   - Warning notifications sent
   - Useful for approaching limits

2. **Hard Limit (Blocking)**
   - Usage blocked when exceeded
   - 429 Too Many Requests error
   - Requires upgrade to continue

3. **Overage Billing (Metered)**
   - Usage continues after quota
   - Additional charges applied
   - Billed on next invoice

### Viewing Quotas

```bash
# Show all quotas for current plan
nself billing quota

# Output:
# Quota Report - Pro Plan
#
#  Service        | Quota           | Used         | Remaining  | Status
# ----------------|-----------------|--------------|------------|--------
#  API Requests   | 200,000/month   | 45,231       | 154,769    | OK
#  Storage        | 50 GB           | 12.3 GB      | 37.7 GB    | OK
#  Bandwidth      | 100 GB/month    | 23.5 GB      | 76.5 GB    | OK
#  Compute        | Unlimited       | 145 hrs      | -          | OK
#  Database       | Unlimited       | 1.2M queries | -          | OK
#  Functions      | 100,000/month   | 8,234        | 91,766     | OK
#
# Overage Pricing (if quota exceeded):
#   - API: $0.001 per request
#   - Storage: $0.10 per GB
#   - Bandwidth: $0.05 per GB

# Show quota with current usage percentages
nself billing quota --usage

# Output includes progress bars:
# API Requests:  [===========>................] 45% (45,231 / 100,000)
# Storage:       [=====>.......................]  25% (12.3 GB / 50 GB)
# Bandwidth:     [=====>.......................]  24% (23.5 GB / 100 GB)

# Check specific service quota
nself billing quota --service=api

# Output:
# API Request Quota - Pro Plan
#
# Total Quota: 200,000 requests/month
# Used: 45,231 requests (23%)
# Remaining: 154,769 requests (77%)
# Resets: 2026-02-01 (3 days)
#
# Top Endpoints:
#   1. POST /graphql       - 28,442 requests (63%)
#   2. GET /api/users      - 8,321 requests (18%)
#   3. POST /api/auth      - 4,521 requests (10%)
#
# Status: OK
# Overage: $0.001 per additional request

# View quota alerts
nself billing quota --alerts

# Output:
# Active Quota Alerts:
#
#  Service    | Threshold | Status           | Alert On
# ------------|-----------|------------------|----------
#  API        | 80%       | OK (45% used)    | 2026-02-07
#  API        | 90%       | OK (45% used)    | 2026-02-17
#  Storage    | 75%       | OK (25% used)    | Never
#  Bandwidth  | 80%       | OK (24% used)    | Never
```

### Configuring Quotas

```bash
# Set custom quota for organization (admin only)
nself billing quota set \
  --org=acme-corp \
  --service=api \
  --limit=500000 \
  --mode=hard

# Output:
# ✓ Custom quota set for Acme Corp
#   Service: API Requests
#   Limit: 500,000/month
#   Mode: Hard limit (blocks at quota)
#   Plan: Enterprise (custom)

# Set soft limit with overage billing
nself billing quota set \
  --org=acme-corp \
  --service=storage \
  --limit=100 \
  --mode=overage \
  --overage-rate=0.08

# Output:
# ✓ Quota updated
#   Service: Storage
#   Limit: 100 GB
#   Mode: Overage billing
#   Rate: $0.08/GB over quota

# Reset quota to plan defaults
nself billing quota reset --org=acme-corp

# Output:
# ✓ Quotas reset to Pro plan defaults
#   All custom overrides removed
```

### Quota Alerts

Configure when and how to be notified:

```bash
# Add alert threshold
nself billing quota alert api --threshold 80 --email admin@example.com

# Output:
# ✓ Alert configured
#   Service: API Requests
#   Threshold: 80% (160,000 requests)
#   Notification: Email to admin@example.com

# Add multiple thresholds
nself billing quota alert storage \
  --threshold 75 --email team@example.com \
  --threshold 90 --email admin@example.com \
  --threshold 95 --webhook https://example.com/alerts

# Configure Slack alerts
nself billing quota alert api \
  --threshold 80 \
  --slack https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXX

# Disable alerts for a service
nself billing quota alert api --disable

# List all alerts
nself billing quota alert list

# Output:
#  service    | threshold | channels                    | status
# ------------|-----------|-----------------------------|---------
#  api        | 80%       | email, slack                | active
#  api        | 90%       | email                       | active
#  storage    | 75%       | email                       | active
#  storage    | 90%       | email, webhook              | active
#  bandwidth  | 80%       | email                       | active
```

### Handling Quota Exceeded

When a hard limit quota is reached:

**API Request Example:**
```bash
curl https://api.yourdomain.com/graphql
```

**Response (429 Too Many Requests):**
```json
{
  "error": {
    "code": "quota_exceeded",
    "message": "API request quota exceeded",
    "details": {
      "service": "api",
      "quota": 200000,
      "used": 200000,
      "reset_at": "2026-02-01T00:00:00Z",
      "upgrade_url": "https://yourdomain.com/billing/upgrade"
    }
  }
}
```

**Programmatic Handling:**
```javascript
// Check quota before making requests
const checkQuota = async () => {
  const response = await fetch('/api/quota/api');
  const quota = await response.json();

  if (quota.percentage_used >= 95) {
    showWarning('Approaching API quota limit');
  }

  if (quota.remaining === 0) {
    throw new Error('API quota exceeded. Please upgrade your plan.');
  }
};

// Handle quota exceeded error
try {
  await makeApiRequest();
} catch (error) {
  if (error.code === 'quota_exceeded') {
    redirectToUpgrade();
  }
}
```

---

## Pricing Plans

### Built-in Plans

nself comes with four standard pricing tiers:

**Free Plan ($0/month)**
```yaml
price: $0
interval: month
quotas:
  api_requests: 10,000/month
  storage: 1 GB
  bandwidth: 5 GB/month
  compute: 10 CPU-hours/month
  database: 100,000 queries/month
  functions: 10,000 invocations/month
features:
  - Community support
  - Basic monitoring
  - 1 organization
  - 5 team members
overage: Not available (hard limits)
```

**Starter Plan ($19/month)**
```yaml
price: $19
interval: month
quotas:
  api_requests: 50,000/month
  storage: 10 GB
  bandwidth: 25 GB/month
  compute: 50 CPU-hours/month
  database: 500,000 queries/month
  functions: 50,000 invocations/month
features:
  - Email support (24h response)
  - Advanced monitoring
  - 3 organizations
  - 15 team members
  - Custom domains
overage:
  api: $0.0005/request
  storage: $0.12/GB
  bandwidth: $0.08/GB
```

**Pro Plan ($49/month)**
```yaml
price: $49
interval: month
quotas:
  api_requests: 200,000/month
  storage: 50 GB
  bandwidth: 100 GB/month
  compute: 200 CPU-hours/month
  database: 2,000,000 queries/month
  functions: 200,000 invocations/month
features:
  - Priority support (4h response)
  - Full monitoring & alerts
  - 10 organizations
  - 50 team members
  - Custom domains
  - SSO (SAML)
  - SLA: 99.9% uptime
overage:
  api: $0.0003/request
  storage: $0.10/GB
  bandwidth: $0.05/GB
```

**Enterprise Plan (Custom)**
```yaml
price: Contact sales
interval: annual (with custom terms)
quotas:
  api_requests: Unlimited (or custom)
  storage: Unlimited (or custom)
  bandwidth: Unlimited (or custom)
  compute: Unlimited (or custom)
  database: Unlimited (or custom)
  functions: Unlimited (or custom)
features:
  - Dedicated support (1h response)
  - Custom monitoring & dashboards
  - Unlimited organizations
  - Unlimited team members
  - Custom domains
  - SSO (SAML, OIDC)
  - SLA: 99.99% uptime
  - Custom integrations
  - On-premise deployment
  - Dedicated infrastructure
overage: Negotiated
```

### Creating Custom Plans

```bash
# Create custom plan
nself billing plan create premium \
  --price 99 \
  --interval month \
  --quota-api 500000 \
  --quota-storage 100 \
  --quota-bandwidth 250 \
  --overage-api 0.0002 \
  --overage-storage 0.08 \
  --overage-bandwidth 0.04 \
  --features "Priority Support,Advanced Monitoring,SSO"

# Output:
# ✓ Plan created: premium
#   Price: $99/month
#   Quotas:
#     - API: 500,000/month ($0.0002/req overage)
#     - Storage: 100 GB ($0.08/GB overage)
#     - Bandwidth: 250 GB/month ($0.04/GB overage)
#   Features:
#     - Priority Support
#     - Advanced Monitoring
#     - SSO

# Create annual plan with discount
nself billing plan create pro-annual \
  --price 490 \
  --interval year \
  --copy-from pro \
  --discount 17

# Output:
# ✓ Plan created: pro-annual
#   Price: $490/year (17% discount vs. $588 monthly)
#   Quotas: Copied from Pro plan
#   Billing: Annual (save $98/year)

# Create usage-only plan (no base fee)
nself billing plan create pay-as-you-go \
  --price 0 \
  --interval month \
  --quota-api 0 \
  --overage-api 0.001 \
  --quota-storage 0 \
  --overage-storage 0.15 \
  --mode metered

# Output:
# ✓ Plan created: pay-as-you-go
#   Base Price: $0/month
#   Quotas: None (usage-only)
#   Pricing:
#     - API: $0.001 per request
#     - Storage: $0.15 per GB
#   Mode: Metered (charged for all usage)
```

### Plan Comparison

```bash
# Compare all plans
nself billing plan compare

# Output:
# Plan Comparison
#
#                  | Free     | Starter   | Pro       | Enterprise
# -----------------|----------|-----------|-----------|-------------
# Price            | $0       | $19       | $49       | Custom
# API Requests     | 10K      | 50K       | 200K      | Unlimited
# Storage          | 1 GB     | 10 GB     | 50 GB     | Unlimited
# Bandwidth        | 5 GB     | 25 GB     | 100 GB    | Unlimited
# Compute          | 10 hrs   | 50 hrs    | 200 hrs   | Unlimited
# Organizations    | 1        | 3         | 10        | Unlimited
# Team Members     | 5        | 15        | 50        | Unlimited
# Support          | Community| Email 24h | Priority 4h| Dedicated 1h
# SLA              | -        | -         | 99.9%     | 99.99%
# SSO              | -        | -         | SAML      | SAML + OIDC
# Custom Domains   | -        | ✓         | ✓         | ✓
# Overage Billing  | -        | ✓         | ✓         | Negotiated
#
# Annual billing available for Starter+ (save 15%)

# Compare specific plans
nself billing plan compare starter pro

# Output shows detailed side-by-side comparison
```

### Feature Gating

Control feature access based on plan:

**In Code (Backend):**
```javascript
// Check if feature is available for user's plan
const checkFeature = async (userId, feature) => {
  const user = await getUser(userId);
  const plan = await getPlan(user.org_id);

  return plan.features.includes(feature);
};

// Gate SSO feature
if (await checkFeature(userId, 'sso')) {
  // Allow SSO login
} else {
  throw new Error('SSO is only available on Pro and Enterprise plans');
}

// Gate custom domain feature
const addCustomDomain = async (orgId, domain) => {
  const plan = await getPlan(orgId);

  if (!plan.features.includes('custom_domains')) {
    throw new Error('Custom domains require Starter plan or higher');
  }

  // Proceed with adding domain
};
```

**In Database (RLS Policies):**
```sql
-- Only Pro+ can create more than 3 organizations
CREATE POLICY org_count_limit ON organizations
  FOR INSERT
  WITH CHECK (
    (
      SELECT COUNT(*)
      FROM organizations
      WHERE owner_user_id = auth.uid()
    ) < (
      SELECT
        CASE
          WHEN billing_plan = 'free' THEN 1
          WHEN billing_plan = 'starter' THEN 3
          WHEN billing_plan = 'pro' THEN 10
          ELSE 999999  -- Enterprise
        END
      FROM billing_customers
      WHERE user_id = auth.uid()
    )
  );
```

**In Frontend (React):**
```javascript
import { usePlan } from './hooks/usePlan';

const FeatureGate = ({ feature, children, fallback }) => {
  const { plan, hasFeature } = usePlan();

  if (hasFeature(feature)) {
    return children;
  }

  return fallback || (
    <UpgradePrompt
      message={`${feature} is available on ${plan.requiredPlan}+ plans`}
    />
  );
};

// Usage
<FeatureGate feature="sso">
  <SSOLoginButton />
</FeatureGate>

<FeatureGate feature="custom_domains">
  <CustomDomainSettings />
</FeatureGate>
```

---

## Invoice Management

### Viewing Invoices

```bash
# List all invoices
nself billing invoice list

# Output:
#  id          | date       | amount  | status | due_date   | paid_date
# -------------|------------|---------|--------|------------|------------
#  in_1abc...  | 2026-01-15 | $49.00  | paid   | 2026-01-15 | 2026-01-15
#  in_2def...  | 2025-12-15 | $49.00  | paid   | 2025-12-15 | 2025-12-15
#  in_3ghi...  | 2025-11-15 | $49.00  | paid   | 2025-11-15 | 2025-11-16

# Filter by status
nself billing invoice list --status=unpaid

# Filter by date range
nself billing invoice list --year=2025

# Show invoice details
nself billing invoice show in_1abc...

# Output:
# Invoice: in_1abc...
# Customer: Acme Corp (cus_Nxxxxxxx)
# Date: 2026-01-15
# Due Date: 2026-01-15
# Status: paid
# Amount: $49.00
#
# Line Items:
#  description              | quantity | unit_price | amount
# --------------------------|----------|------------|--------
#  Pro Plan (Monthly)       | 1        | $49.00     | $49.00
#  API Overage (5,000 req)  | 5,000    | $0.0003    | $1.50
#  Storage Overage (2.5 GB) | 2.5      | $0.10      | $0.25
#                                      Subtotal:    | $50.75
#                                      Tax (8.5%):  | $4.31
#                                      Total:       | $55.06
#
# Payment Method: Visa ending in 4242
# Paid: 2026-01-15 08:30:05 UTC
```

### Downloading Invoices

```bash
# Download invoice PDF
nself billing invoice download in_1abc...

# Output:
# Downloading invoice in_1abc...
# ✓ Invoice downloaded: invoice_in_1abc_2026-01-15.pdf

# Download multiple invoices
nself billing invoice download --year=2025 --output=invoices_2025/

# Output:
# Downloading 12 invoices...
# ✓ Downloaded to: invoices_2025/
#   - invoice_in_1abc_2026-01-15.pdf
#   - invoice_in_2def_2025-12-15.pdf
#   ...
```

### Paying Unpaid Invoices

```bash
# Pay a specific invoice
nself billing invoice pay in_4jkl...

# Output:
# Processing payment for invoice in_4jkl...
# Amount: $49.00
# Payment method: Visa ending in 4242
#
# Processing...
# ✓ Payment successful
#   Transaction ID: ch_1abc...
#   Paid on: 2026-01-29 10:45:00 UTC

# Pay all unpaid invoices
nself billing invoice pay --all

# Interactive confirmation:
# You have 2 unpaid invoices totaling $98.00:
#   - in_4jkl... ($49.00, due 2026-01-29)
#   - in_5mno... ($49.00, due 2026-01-22)
#
# Proceed with payment? (y/N): y
#
# Processing payments...
# ✓ in_4jkl... paid ($49.00)
# ✓ in_5mno... paid ($49.00)
# Total paid: $98.00
```

### Failed Payments

When a payment fails:

```bash
# View failed invoices
nself billing invoice list --status=past_due

# Output:
#  id          | date       | amount  | status    | attempts | next_retry
# -------------|------------|---------|-----------|----------|-------------
#  in_6pqr...  | 2026-01-15 | $49.00  | past_due  | 3        | 2026-01-20

# Retry failed payment
nself billing invoice pay in_6pqr...

# If card declined, update payment method first:
nself billing payment add
# Then retry:
nself billing invoice pay in_6pqr...
```

**Email Notification (Payment Failed):**
```
Subject: [nself] Payment Failed - Action Required

Your payment of $49.00 for the Pro plan could not be processed.

Invoice: in_6pqr...
Amount: $49.00
Attempted: 2026-01-15 08:30:00 UTC
Reason: Card declined (insufficient funds)

Action Required:
1. Update your payment method or add a new card
2. We will automatically retry in 3 days

If payment is not received by 2026-01-29, your account will be:
- Downgraded to Free plan
- API access rate-limited
- Services may be restricted

Update payment method:
https://yourdomain.com/billing/payment

Questions? Contact support: support@yourdomain.com
```

### Invoice Customization

Customize invoice appearance and details:

```bash
# Set company information
nself billing invoice config \
  --company-name "Acme Corporation" \
  --company-address "123 Main St, San Francisco, CA 94102" \
  --company-email "billing@acmecorp.com" \
  --company-tax-id "12-3456789"

# Set invoice footer
nself billing invoice config \
  --footer "Thank you for your business! Questions? support@acmecorp.com"

# Set invoice logo
nself billing invoice config \
  --logo https://acmecorp.com/logo.png

# Preview invoice template
nself billing invoice preview

# Generates a sample invoice PDF for review
```

---

## Reporting & Analytics

### Revenue Reports

```bash
# Monthly revenue report
nself billing report revenue --period=month

# Output:
# Revenue Report: January 2026
#
# Total Revenue: $14,523
# New MRR: $847
# Churned MRR: -$196
# Net New MRR: $651
#
# Breakdown by Plan:
#  plan        | customers | revenue  | % of total
# -------------|-----------|----------|------------
#  Free        | 1,247     | $0       | 0%
#  Starter     | 183       | $3,477   | 24%
#  Pro         | 145       | $7,105   | 49%
#  Enterprise  | 12        | $3,941   | 27%
#
# Revenue by Type:
#  type                | amount   | % of total
# ---------------------|----------|------------
#  Subscriptions       | $12,340  | 85%
#  Usage Overage       | $1,823   | 13%
#  One-time Charges    | $360     | 2%

# Annual revenue trends
nself billing report revenue --period=year --format=chart

# Output: ASCII chart showing monthly revenue

# Export revenue data
nself billing report revenue --period=year --format=csv --output=revenue_2026.csv
```

### Usage Trends

```bash
# Usage trends over time
nself billing report usage --period=quarter

# Output:
# Usage Trends: Q1 2026 (Jan - Mar)
#
# API Requests:
#  month | total      | avg/day | growth
# -------|------------|---------|--------
#  Jan   | 2,456,234  | 79,233  | +15%
#  Feb   | 2,834,122  | 101,218 | +15%
#  Mar   | 3,245,891  | 104,706 | +15%
#
# Storage (GB):
#  month | avg     | peak  | growth
# -------|---------|-------|--------
#  Jan   | 234.5   | 267.3 | +8%
#  Feb   | 253.2   | 289.1 | +8%
#  Mar   | 273.4   | 312.5 | +8%
#
# Bandwidth (GB):
#  month | total   | avg/day | growth
# -------|---------|---------|--------
#  Jan   | 1,234   | 39.8    | +12%
#  Feb   | 1,382   | 49.4    | +12%
#  Mar   | 1,548   | 49.9    | +12%

# Usage by customer segment
nself billing report usage --segment=plan

# Output:
# Usage by Plan: January 2026
#
#  plan       | customers | api_requests | storage | bandwidth
# ------------|-----------|--------------|---------|------------
#  Free       | 1,247     | 8,234,523    | 1.2 TB  | 45.3 TB
#  Starter    | 183       | 4,523,122    | 1.8 TB  | 23.5 TB
#  Pro        | 145       | 12,456,789   | 7.2 TB  | 89.4 TB
#  Enterprise | 12        | 45,234,891   | 34.5 TB | 234.8 TB
```

### Customer Lifetime Value (LTV)

```bash
# LTV by plan
nself billing report ltv

# Output:
# Customer Lifetime Value Report
#
#  plan       | avg_ltv | avg_months | churn_rate | new_this_month | total
# ------------|---------|------------|------------|----------------|-------
#  Starter    | $285    | 15 months  | 5.2%       | 23             | 183
#  Pro        | $1,470  | 30 months  | 2.8%       | 12             | 145
#  Enterprise | $14,340 | 48 months  | 1.2%       | 1              | 12
#
# LTV/CAC Ratio: 3.4:1 (healthy)
# Payback Period: 4.2 months

# Customer cohort analysis
nself billing report cohorts --months=12

# Output:
# Cohort Retention Analysis (12 months)
#
#  cohort  | m0   | m1   | m2   | m3   | m6   | m12
# ---------|------|------|------|------|------|------
#  Jan 25  | 100% | 92%  | 87%  | 83%  | 75%  | 68%
#  Feb 25  | 100% | 94%  | 89%  | 85%  | 78%  | 71%
#  Mar 25  | 100% | 93%  | 88%  | 84%  | 76%  | -
#  ...
#
# Average 12-month retention: 69%
```

### Churn Analysis

```bash
# Churn report
nself billing report churn --period=quarter

# Output:
# Churn Analysis: Q1 2026
#
# Customer Churn:
#  month | churned | churn_rate | MRR_churned
# -------|---------|------------|-------------
#  Jan   | 8       | 4.2%       | $196
#  Feb   | 6       | 3.1%       | $147
#  Mar   | 7       | 3.5%       | $183
#
# Churn by Plan:
#  plan       | churned | churn_rate
# ------------|---------|------------
#  Free       | 0       | 0% (n/a)
#  Starter    | 15      | 8.2%
#  Pro        | 5       | 3.4%
#  Enterprise | 1       | 8.3%
#
# Churn Reasons (from cancellation surveys):
#  1. Too expensive (38%)
#  2. Not using anymore (24%)
#  3. Missing features (18%)
#  4. Switching to competitor (12%)
#  5. Other (8%)
#
# Win-back Campaigns: 23% success rate (5 of 21)

# Identify at-risk customers
nself billing report at-risk

# Output:
# At-Risk Customers (High Churn Probability)
#
#  customer        | plan    | risk_score | reasons
# -----------------|---------|------------|----------------------------------
#  Acme Corp       | Pro     | 78%        | Usage down 60%, Payment failed
#  Widget Inc      | Starter | 65%        | No activity last 14 days
#  Gadget Co       | Pro     | 52%        | Opened cancellation page 3x
#
# Recommended Actions:
# - Send engagement email
# - Offer discount or upgrade incentive
# - Schedule check-in call
```

### Dashboard Queries

Create custom dashboards with GraphQL:

```graphql
# Real-time revenue dashboard
query RevenueMetrics($orgId: uuid!) {
  # Current MRR
  mrr: billing_subscriptions_aggregate(
    where: {
      org_id: { _eq: $orgId }
      status: { _eq: "active" }
    }
  ) {
    aggregate {
      sum {
        amount
      }
    }
  }

  # New customers this month
  new_customers: billing_customers_aggregate(
    where: {
      org_id: { _eq: $orgId }
      created_at: { _gte: "2026-01-01" }
    }
  ) {
    aggregate {
      count
    }
  }

  # Churn rate
  churned: billing_subscriptions_aggregate(
    where: {
      org_id: { _eq: $orgId }
      canceled_at: {
        _gte: "2026-01-01"
        _lte: "2026-01-31"
      }
    }
  ) {
    aggregate {
      count
    }
  }

  # Usage trends
  usage_trend: billing_usage_events(
    where: { org_id: { _eq: $orgId } }
    order_by: { timestamp: asc }
    limit: 30
  ) {
    date: timestamp
    service
    quantity
  }
}
```

**Grafana Dashboard:**

If monitoring is enabled, create Grafana dashboards:

```bash
# Import billing dashboard
nself monitoring dashboard import billing

# Output:
# ✓ Dashboard imported: nself Billing
#   URL: https://grafana.yourdomain.com/d/billing
#
# Panels:
# - Revenue trends (MRR, ARR)
# - Customer growth
# - Churn rate
# - Usage by service
# - Top customers by revenue
# - Plan distribution
```

---

## Best Practices

### 1. Pricing Strategy

**Start with Simple Tiers:**
- 3-4 plans maximum (Free, Starter, Pro, Enterprise)
- Clear value differentiation between tiers
- Anchor pricing (make middle tier most attractive)

**Usage-Based vs. Subscription:**
```
Subscription (Predictable):
✓ Predictable revenue
✓ Simpler billing
✗ Less fair for low-usage customers

Usage-Based (Fair):
✓ Fair pay-per-use model
✓ Grows with customer
✗ Unpredictable revenue
✗ Complex billing

Hybrid (Best of Both):
✓ Base subscription + usage overage
✓ Predictable baseline revenue
✓ Fair for variable usage
```

**Price Anchoring:**
```
❌ Bad:
Starter $19  |  Pro $49  |  Enterprise $299
(Linear, no anchor)

✓ Good:
Starter $19  |  Pro $49  |  Enterprise Contact Sales
(Pro looks reasonable, Enterprise drives leads)

✓ Best:
Free $0  |  Starter $19  |  Pro $49  |  Enterprise Custom
(Free for acquisition, middle tier anchored, clear upgrade path)
```

### 2. Quota Design

**Set Generous Free Tier Limits:**
- Allow meaningful usage without payment
- Low enough to encourage upgrades
- High enough to show value

**Example:**
```
❌ Too Restrictive:
Free: 100 API requests/month
(Can't even evaluate the product)

✓ Good:
Free: 10,000 API requests/month
(Can build and test, but need to upgrade for production)

✓ Best with Soft Limits:
Free: 10,000 API requests/month, then throttled to 100/day
(Can continue using, but severely limited - strong upgrade incentive)
```

**Hard Limits vs. Soft Limits:**
```
Hard Limit (Strict):
- Blocks usage at quota
- Use for: Preventing abuse, clear tier boundaries
- Risk: Customer frustration, lost revenue

Soft Limit (Flexible):
- Allows overage with charges
- Use for: Revenue optimization, customer flexibility
- Risk: Bill shock, unexpected charges

Recommendation:
- Free plan: Hard limits
- Paid plans: Soft limits with overage billing
- Enterprise: Custom/unlimited
```

### 3. Usage Optimization

**Track Everything:**
```bash
# Log usage events with context
await trackUsage({
  service: 'api',
  quantity: 1,
  endpoint: '/graphql',
  user_id: userId,
  org_id: orgId,
  metadata: {
    operation: 'GetUser',
    response_time: 125,
    status_code: 200
  }
});
```

**Batch Usage Events:**
```javascript
// ❌ Bad: One database write per request
onApiRequest(async (req) => {
  await db.insert('usage_events', { service: 'api', quantity: 1 });
});

// ✓ Good: Batch writes every minute
let usageBuffer = [];
onApiRequest((req) => {
  usageBuffer.push({ service: 'api', quantity: 1, timestamp: new Date() });
});

setInterval(async () => {
  if (usageBuffer.length > 0) {
    await db.insert('usage_events', usageBuffer);
    usageBuffer = [];
  }
}, 60000); // Every minute
```

**Aggregate Smart:**
```sql
-- Pre-aggregate daily usage (cron job)
INSERT INTO billing.usage_daily
SELECT
  DATE(timestamp) AS date,
  org_id,
  service,
  SUM(quantity) AS total_quantity,
  COUNT(*) AS event_count
FROM billing.usage_events
WHERE DATE(timestamp) = CURRENT_DATE - INTERVAL '1 day'
GROUP BY DATE(timestamp), org_id, service;

-- Delete raw events older than 30 days
DELETE FROM billing.usage_events
WHERE timestamp < CURRENT_DATE - INTERVAL '30 days';
```

### 4. Customer Communication

**Proactive Notifications:**

**Approaching Quota (80%):**
```
Subject: [nself] You're using 80% of your API quota

Hi Alice,

Your organization "Acme Corp" has used 80% of this month's API quota:

Used: 80,000 / 100,000 requests
Remaining: 20,000 (20%)
Resets: February 1, 2026

What happens next:
- At 100%: Requests will be rate-limited (Pro plan)
- Overage charges: $0.0003 per additional request

Actions you can take:
1. Upgrade to Enterprise for unlimited API requests
2. Optimize API usage (see tips: link)
3. Do nothing - overage charges apply automatically

View usage: https://yourdomain.com/billing/usage
Upgrade now: https://yourdomain.com/billing/plans

Questions? Reply to this email.

Best,
The nself Team
```

**Failed Payment (Day 1):**
```
Subject: [nself] Payment Failed - Please Update

Hi Alice,

We couldn't process your payment for the Pro plan ($49.00).

Invoice: in_6pqr...
Amount: $49.00
Reason: Card declined

Next steps:
1. Update your payment method
2. We'll automatically retry in 3 days
3. No service interruption yet

Update payment: https://yourdomain.com/billing/payment

This is attempt 1 of 3. Need help? Contact support.
```

**Failed Payment (Day 7 - Final Warning):**
```
Subject: [nself] URGENT: Payment Required - Service Interruption

Hi Alice,

Your payment of $49.00 is now 7 days overdue.

Action required immediately:
Your account will be downgraded to Free plan in 48 hours if payment is not received.

This means:
- API quota reduced from 200K to 10K/month
- Storage limit reduced from 50GB to 1GB
- Priority support removed
- Advanced features disabled

Avoid interruption:
Update payment method: https://yourdomain.com/billing/payment

This is your final reminder. Contact support if you need help.
```

### 5. Compliance & Security

**PCI-DSS Compliance:**
- Never store credit card numbers
- Use Stripe.js for client-side tokenization
- All payment data stays with Stripe
- Only store Stripe customer/payment method IDs

**Tax Handling:**
```bash
# Enable Stripe Tax for automatic tax calculation
STRIPE_TAX_ENABLED=true
STRIPE_TAX_INCLUSIVE=false  # Show tax separately on invoices

# Manual tax configuration
BILLING_TAX_RATE=0.085  # 8.5%
BILLING_TAX_REGIONS=US-CA,US-NY,US-TX  # Nexus states
```

**Data Privacy:**
```javascript
// Anonymize usage data for analytics
const anonymizeUsage = (usage) => {
  return {
    ...usage,
    user_id: hashUserId(usage.user_id),  // One-way hash
    org_id: hashOrgId(usage.org_id),
    ip_address: null,  // Remove PII
    metadata: sanitizeMetadata(usage.metadata)
  };
};

// GDPR: Right to erasure
const deleteCustomerData = async (customerId) => {
  await stripe.customers.del(customerId);  // Delete from Stripe
  await db.delete('billing_customers', { stripe_id: customerId });
  await db.delete('usage_events', { customer_id: customerId });
  // Anonymize instead of delete for analytics
  await db.update('invoices',
    { customer_id: 'deleted-user' },
    { customer_id: customerId }
  );
};
```

### 6. Trial Best Practices

**Free Trial Strategy:**
```bash
# 14-day trial for Pro plan
BILLING_TRIAL_DAYS=14
BILLING_TRIAL_PLANS=pro,enterprise

# Trial behavior
BILLING_TRIAL_REQUIRES_PAYMENT=true  # Require card upfront
BILLING_TRIAL_NOTIFY_DAYS_BEFORE=3   # Notify 3 days before trial ends
```

**Trial Conversion Tactics:**
```
Day 1: Welcome email + onboarding guide
Day 3: Feature highlight #1 (most valuable feature)
Day 7: Feature highlight #2 + success stories
Day 11: Trial ending soon (3 days left)
Day 13: Final reminder (1 day left)
Day 14: Trial ended - convert or downgrade
Day 15: Win-back offer (if didn't convert)
```

**Reverse Trial (Freemium to Paid):**
```bash
# Allow free users to try Pro features for 7 days
nself billing trial start --user=user@example.com --plan=pro --days=7

# Automatically downgrade after trial
nself billing trial auto-downgrade --enable
```

---

## Advanced Topics

### Multi-Currency Support

Support multiple currencies for global customers:

```bash
# Enable multi-currency
BILLING_MULTI_CURRENCY=true
BILLING_DEFAULT_CURRENCY=usd
BILLING_SUPPORTED_CURRENCIES=usd,eur,gbp,cad,aud

# Create plan in multiple currencies
nself billing plan create pro \
  --price-usd 49 \
  --price-eur 45 \
  --price-gbp 39 \
  --price-cad 65 \
  --price-aud 70

# Customer currency detection
# 1. From IP geolocation
# 2. From user preferences
# 3. From organization settings
# 4. Fallback to default
```

### Marketplace / Reseller Billing

Support marketplace integrations (AWS Marketplace, Azure Marketplace):

```bash
# Configure marketplace integration
BILLING_MARKETPLACE_ENABLED=true
BILLING_MARKETPLACE_PROVIDER=aws  # or azure, gcp

# Marketplace customers are billed through marketplace
# Usage is still tracked in nself
# Revenue is reported via marketplace APIs

# Create marketplace customer
nself billing customer create \
  --email customer@example.com \
  --marketplace aws \
  --marketplace-customer-id cus_xxxxx
```

### Usage-Based Pricing Models

**1. Tiered Pricing (Volume Discounts):**
```yaml
# API pricing tiers
api_pricing:
  tier_1:
    range: 0 - 100,000
    price_per_request: $0.001
  tier_2:
    range: 100,001 - 500,000
    price_per_request: $0.0008
  tier_3:
    range: 500,001+
    price_per_request: $0.0005

# Example: 550,000 requests
# First 100,000: 100,000 × $0.001 = $100
# Next 400,000:   400,000 × $0.0008 = $320
# Next 50,000:    50,000 × $0.0005 = $25
# Total: $445
```

**2. Graduated Pricing:**
```yaml
# Storage pricing (like AWS S3)
storage_pricing:
  first_50gb: $0.10/GB
  next_450gb: $0.08/GB
  next_500gb: $0.06/GB
  over_1tb: $0.04/GB

# Example: 600 GB
# First 50 GB:  50 × $0.10 = $5.00
# Next 450 GB:  450 × $0.08 = $36.00
# Next 100 GB:  100 × $0.06 = $6.00
# Total: $47.00
```

**3. Pay-Per-Use (No Base Fee):**
```yaml
# Pure usage pricing
compute_pricing:
  cpu_hour: $0.05
  memory_gb_hour: $0.01
  gpu_hour: $2.50

# Example: 100 CPU-hours, 500 GB-hours memory
# CPU: 100 × $0.05 = $5.00
# Memory: 500 × $0.01 = $5.00
# Total: $10.00
```

**4. Hybrid (Base + Usage):**
```yaml
# Base subscription + usage overages
pro_plan:
  base_price: $49/month
  included:
    api_requests: 200,000
    storage: 50 GB
    bandwidth: 100 GB
  overage:
    api_requests: $0.0003/request
    storage: $0.10/GB
    bandwidth: $0.05/GB

# Example: 250,000 API, 60 GB storage, 120 GB bandwidth
# Base: $49.00
# API overage: 50,000 × $0.0003 = $15.00
# Storage overage: 10 × $0.10 = $1.00
# Bandwidth overage: 20 × $0.05 = $1.00
# Total: $66.00
```

### Discounts and Promotions

```bash
# Create promotional coupon
nself billing coupon create LAUNCH2026 \
  --type percent_off \
  --amount 50 \
  --duration repeating \
  --duration_in_months 3 \
  --max_redemptions 100

# Output:
# ✓ Coupon created: LAUNCH2026
#   Discount: 50% off
#   Duration: 3 months
#   Max uses: 100
#   Expires: 2026-03-31

# Apply coupon to subscription
nself billing subscription apply-coupon LAUNCH2026

# Volume discount for annual billing
nself billing plan create pro-annual \
  --price 490 \
  --interval year \
  --discount_percentage 17  # Save $98/year

# Referral credits
nself billing credit add \
  --customer cus_xxxxx \
  --amount 25 \
  --reason "Referral bonus"
```

### Custom Billing Cycles

```bash
# Set custom billing cycle (not calendar month)
nself billing subscription update \
  --billing-cycle-anchor 15  # Bill on 15th of each month

# Align billing cycles (useful for enterprise)
nself billing subscription align \
  --date 2026-02-01  # Move next billing to specific date

# Prorated charges
# Automatically calculated on:
# - Mid-cycle upgrades
# - Mid-cycle downgrades
# - Subscription changes
```

### Webhook Automation

Automate actions based on billing events:

```javascript
// Webhook handler
app.post('/webhooks/stripe', async (req, res) => {
  const sig = req.headers['stripe-signature'];
  let event;

  try {
    event = stripe.webhooks.constructEvent(req.body, sig, WEBHOOK_SECRET);
  } catch (err) {
    return res.status(400).send(`Webhook Error: ${err.message}`);
  }

  // Handle events
  switch (event.type) {
    case 'customer.subscription.updated':
      await handleSubscriptionUpdate(event.data.object);
      break;

    case 'invoice.payment_failed':
      await handlePaymentFailed(event.data.object);
      break;

    case 'customer.subscription.deleted':
      await handleSubscriptionCanceled(event.data.object);
      break;

    case 'invoice.paid':
      await handleInvoicePaid(event.data.object);
      break;

    default:
      console.log(`Unhandled event type: ${event.type}`);
  }

  res.json({ received: true });
});

// Handlers
const handlePaymentFailed = async (invoice) => {
  const customer = await getCustomer(invoice.customer);

  // Send notification
  await sendEmail(customer.email, 'payment_failed', {
    amount: invoice.amount_due,
    invoice_id: invoice.id,
    attempt: invoice.attempt_count
  });

  // After 3 failed attempts, downgrade
  if (invoice.attempt_count >= 3) {
    await downgradeSubscription(invoice.subscription, 'free');
    await sendEmail(customer.email, 'account_downgraded');
  }
};

const handleSubscriptionCanceled = async (subscription) => {
  const customer = await getCustomer(subscription.customer);

  // Remove features
  await revokeFeatures(customer.org_id, subscription.plan.id);

  // Send win-back email
  setTimeout(async () => {
    await sendEmail(customer.email, 'win_back_offer', {
      discount_code: 'COMEBACK50'
    });
  }, 7 * 24 * 60 * 60 * 1000); // 7 days later
};
```

### Billing API (Programmatic Access)

Expose billing functionality via API:

```javascript
// REST API endpoints
app.get('/api/v1/billing/usage', authenticateUser, async (req, res) => {
  const { org_id } = req.user;
  const usage = await getUsageForOrg(org_id);
  res.json(usage);
});

app.get('/api/v1/billing/quota', authenticateUser, async (req, res) => {
  const { org_id } = req.user;
  const quotas = await getQuotasForOrg(org_id);
  res.json(quotas);
});

app.post('/api/v1/billing/subscription/upgrade', authenticateUser, async (req, res) => {
  const { org_id } = req.user;
  const { plan } = req.body;

  const subscription = await upgradeSubscription(org_id, plan);
  res.json(subscription);
});

// GraphQL API
const typeDefs = gql`
  type Usage {
    service: String!
    current: Float!
    quota: Float!
    percentage: Float!
    status: UsageStatus!
  }

  enum UsageStatus {
    OK
    WARNING
    EXCEEDED
  }

  type Query {
    usage(service: String): [Usage!]!
    quota(service: String): [Quota!]!
    subscription: Subscription
    invoices(limit: Int, status: InvoiceStatus): [Invoice!]!
  }

  type Mutation {
    upgradeSubscription(plan: String!): Subscription!
    cancelSubscription: Subscription!
    addPaymentMethod(token: String!): PaymentMethod!
  }
`;
```

---

## Next Steps

- **[Organization Management](./ORGANIZATION-MANAGEMENT.md)** - Connect billing to organizations
- **[Real-Time Features](./REALTIME-FEATURES.md)** - Usage tracking for WebSocket connections
- **[Security Guide](./SECURITY.md)** - Secure billing data and PCI compliance
- **[Database Workflow](./DATABASE-WORKFLOW.md)** - Manage billing schema migrations
- **[Examples](./EXAMPLES.md)** - Complete billing integration examples

---

## Additional Resources

- **Stripe Documentation**: [stripe.com/docs](https://stripe.com/docs)
- **Stripe Billing**: [stripe.com/docs/billing](https://stripe.com/docs/billing)
- **Stripe Webhooks**: [stripe.com/docs/webhooks](https://stripe.com/docs/webhooks)
- **PCI Compliance**: [stripe.com/docs/security](https://stripe.com/docs/security)
- **SaaS Metrics**: [stripe.com/atlas/guides/business-of-saas](https://stripe.com/atlas/guides/business-of-saas)

---

**Last Updated**: January 29, 2026
**nself Version**: v0.9.0 (Billing & Usage Tracking)
**Status**: Production Ready
