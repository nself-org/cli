# Multi-Tenant Billing Commands

> **⚠️ DEPRECATED**: `nself billing` is deprecated and will be removed in v1.0.0.
> Please use `nself tenant billing` instead.
> Run `nself tenant billing --help` for full usage information.
>
> **Note:** These commands are part of the multi-tenant features. For single-tenant deployments, most billing features are optional.

Comprehensive billing and usage management for nself multi-tenant deployments. Integrated Stripe payment processing, real-time usage tracking, quota enforcement, and detailed billing analytics.

## Usage

```bash
nself tenant billing <subcommand> [OPTIONS]
```

## Subcommands

| Command | Description |
|---------|-------------|
| `usage` | Show current usage statistics and metrics |
| `invoice` | Manage invoices (list, show, download, pay) |
| `subscription` | Manage subscriptions (show, upgrade, downgrade, cancel) |
| `payment` | Manage payment methods |
| `quota` | Check quota limits and usage |
| `plan` | Manage billing plans |
| `export` | Export billing data to CSV/JSON |
| `customer` | Manage customer information |
| `webhook` | Test and manage webhook endpoints |

---

## Quick Start

### Enable Billing

Add to `.env`:
```bash
BILLING_ENABLED=true
BILLING_STRIPE_ENABLED=true
STRIPE_PUBLISHABLE_KEY=pk_test_...
STRIPE_SECRET_KEY=sk_test_PLACEHOLDER...
STRIPE_WEBHOOK_SECRET=whsec_test_...
```

### Check Your Usage

```bash
# Current period usage
nself tenant billing usage

# Last month usage
nself tenant billing usage --period=last-month

# Specific service
nself tenant billing usage --service=api

# Detailed breakdown
nself tenant billing usage --detailed
```

### View Your Plan and Quota

```bash
# Show current plan
nself tenant billing plan current

# Check quota limits
nself tenant billing quota

# Quota with current usage
nself tenant billing quota --usage
```

### Manage Subscription

```bash
# View current subscription
nself tenant billing subscription show

# View available plans
nself tenant billing subscription plans

# Upgrade to Pro plan
nself tenant billing subscription upgrade pro

# Downgrade to Starter
nself tenant billing subscription downgrade starter
```

---

## Usage Tracking

Monitor API calls, storage, bandwidth, and other metrics.

### Commands

```bash
# Show current billing period usage
nself tenant billing usage

# Show usage for specific service
nself tenant billing usage --service=api

# Show usage for last month
nself tenant billing usage --period=last-month

# Custom date range
nself tenant billing usage --period=custom --start=2026-01-01 --end=2026-01-31

# Detailed breakdown
nself tenant billing usage --detailed

# JSON format
nself tenant billing usage --format=json

# CSV format
nself tenant billing usage --format=csv
```

### Supported Services

| Service | Unit | Description |
|---------|------|-------------|
| `api` | requests | API calls/requests |
| `storage` | GB-hours | Storage consumption |
| `bandwidth` | GB | Data transferred |
| `compute` | CPU-hours | Function execution time |
| `database` | connections | Active DB connections |
| `functions` | invocations | Serverless function calls |

### Period Options

| Period | Description |
|--------|-------------|
| `current` | Current billing month |
| `last-month` | Previous calendar month |
| `custom` | Specify --start and --end dates |

### Output Formats

| Format | Use Case |
|--------|----------|
| `table` | Default, human-readable display |
| `json` | Machine-readable, API integration |
| `csv` | Spreadsheet, data analysis |

### Usage Examples

```bash
# Quick overview
nself tenant billing usage

# API calls this month
nself tenant billing usage --service=api

# Storage usage details
nself tenant billing usage --service=storage --detailed

# Compare months
nself tenant billing usage --period=current
nself tenant billing usage --period=last-month

# Export for analysis
nself tenant billing usage --format=csv --period=last-month > usage_last_month.csv

# Function invocations
nself tenant billing usage --service=functions --detailed

# Bandwidth usage
nself tenant billing usage --service=bandwidth --format=json
```

### Usage Display Example

```
Usage Report: 2026-01-01 to 2026-01-30
══════════════════════════════════════════

SERVICE        USAGE           QUOTA        PERCENTAGE
────────────────────────────────────────────────────
api            45,230 req      1,000,000    4.5%
storage        512 GB-hrs      Unlimited    -
bandwidth      2.5 GB          50 GB        5.0%
compute        12 CPU-hrs      100 CPU-hrs  12%
functions      892 invoc       Unlimited    -
database       8 conn          20 conn      40%

Total Cost: $12.50
Included in plan: $10.00
Overage charges: $2.50
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_ENABLED` | `false` | Enable billing system |
| `BILLING_USAGE_TRACKING_ENABLED` | `true` | Track detailed usage |
| `BILLING_USAGE_AGGREGATION` | `hourly` | Aggregation interval (realtime, hourly, daily, monthly) |
| `BILLING_USAGE_RETENTION_DAYS` | `90` | Keep usage records for X days |
| `BILLING_USAGE_ANOMALY_DETECTION` | `true` | Alert on unusual usage patterns |

---

## Invoices

Create, view, pay, and download invoices.

### Commands

```bash
# List all invoices
nself tenant billing invoice list

# Show invoice details
nself tenant billing invoice show <invoice-id>

# Download invoice as PDF
nself tenant billing invoice download <invoice-id>

# Pay unpaid invoice
nself tenant billing invoice pay <invoice-id>
```

### Invoice Statuses

| Status | Description |
|--------|-------------|
| `draft` | Not yet finalized |
| `open` | Awaiting payment |
| `paid` | Payment received |
| `uncollectible` | Cannot collect (write off) |
| `void` | Canceled invoice |

### List Invoices

```bash
# Show all invoices
nself tenant billing invoice list

# JSON output
nself tenant billing invoice list --format=json

# Filter by status
nself tenant billing invoice list --status=open

# Filter by date
nself tenant billing invoice list --start=2026-01-01 --end=2026-01-31

# Limit results
nself tenant billing invoice list --limit=20
```

### Show Invoice Details

```bash
# Display full invoice details
nself tenant billing invoice show in_1ABC123

# Output as JSON
nself tenant billing invoice show in_1ABC123 --format=json

# Show payment details
nself tenant billing invoice show in_1ABC123 --include=payments
```

Invoice Details Include:
- Invoice number and date
- Amount due and paid
- Services and line items
- Payment method
- Customer information
- Tax information

### Download Invoice

```bash
# Download as PDF
nself tenant billing invoice download in_1ABC123

# Save with custom filename
nself tenant billing invoice download in_1ABC123 --output=invoice_jan_2026.pdf

# Download latest invoice
nself tenant billing invoice download --latest
```

### Pay Invoice

```bash
# Pay with default payment method
nself tenant billing invoice pay in_1ABC123

# Pay with specific payment method
nself tenant billing invoice pay in_1ABC123 --payment-method=pm_1XYZ

# Confirm payment
nself tenant billing invoice pay in_1ABC123 --force
```

### Invoice Examples

```bash
# View unpaid invoices
nself tenant billing invoice list --status=open

# Download all invoices from 2025
for invoice in $(nself tenant billing invoice list --start=2025-01-01 --end=2025-12-31 --format=json | jq -r '.invoices[].id'); do
  nself tenant billing invoice download "$invoice"
done

# Check invoice for specific project
nself tenant billing invoice show in_1ABC123 --include=metadata

# Pay invoice and confirm
nself tenant billing invoice pay in_1ABC123 && echo "Payment successful"
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_AUTO_GENERATE_INVOICES` | `true` | Auto-create invoices |
| `BILLING_INVOICE_PREFIX` | `INV` | Invoice number prefix |
| `BILLING_CURRENCY` | `usd` | Invoice currency |
| `BILLING_INVOICE_DUE_DAYS` | `7` | Days to pay invoice |
| `BILLING_SEND_INVOICE_EMAILS` | `true` | Email invoices to customers |
| `BILLING_COMPANY_NAME` | - | Your company name |
| `BILLING_COMPANY_TAX_ID` | - | Tax/VAT ID |

---

## Subscriptions

Manage billing plans, upgrades, downgrades, and cancellations.

### Commands

```bash
# Show current subscription
nself tenant billing subscription show

# List available plans
nself tenant billing subscription plans

# Upgrade subscription
nself tenant billing subscription upgrade <plan-name>

# Downgrade subscription
nself tenant billing subscription downgrade <plan-name>

# Cancel subscription
nself tenant billing subscription cancel

# Reactivate canceled subscription
nself tenant billing subscription reactivate
```

### Available Plans

| Plan | Price | Features |
|------|-------|----------|
| `free` | $0 | Development, limited quotas |
| `starter` | $19/mo | Small projects, higher quotas |
| `pro` | $79/mo | Production, unlimited quotas |
| `enterprise` | Custom | Large-scale deployments |

### Show Current Subscription

```bash
# Display current plan and status
nself tenant billing subscription show

# JSON format
nself tenant billing subscription show --format=json

# Include billing history
nself tenant billing subscription show --include=history

# Show renewal date
nself tenant billing subscription show --renewal
```

Output Includes:
- Current plan name
- Monthly cost
- Billing cycle dates
- Status (active, canceled, expired)
- Features and quotas
- Renewal/expiration date

### List Available Plans

```bash
# Show all available plans
nself tenant billing subscription plans

# Compare plans side-by-side
nself tenant billing subscription plans --compare

# Show annual pricing
nself tenant billing subscription plans --billing=annual

# Show features by plan
nself tenant billing subscription plans --features
```

Plan Comparison Shows:
- Plan name and price
- API request quota
- Storage quota
- Bandwidth quota
- Team members
- Support level
- Advanced features

### Upgrade Subscription

```bash
# Upgrade to specific plan
nself tenant billing subscription upgrade pro

# View pricing before upgrade
nself tenant billing subscription upgrade pro --preview

# Upgrade with annual billing
nself tenant billing subscription upgrade pro --billing=annual

# Confirm upgrade
nself tenant billing subscription upgrade pro --force
```

Upgrade Notes:
- Prorated charges applied immediately
- Access to new features instant
- Trial period (if applicable) resets
- Old plan credit applied

### Downgrade Subscription

```bash
# Downgrade to lower plan
nself tenant billing subscription downgrade starter

# Preview downgrade impact
nself tenant billing subscription downgrade starter --preview

# Keep current features until renewal
nself tenant billing subscription downgrade starter --at-end-of-period

# Confirm downgrade
nself tenant billing subscription downgrade starter --force
```

Downgrade Notes:
- Effective at end of billing period (default)
- Or immediate with credit applied
- Can cancel anytime
- Features reduced at renewal

### Cancel Subscription

```bash
# Initiate cancellation
nself tenant billing subscription cancel

# Cancel at end of billing period (default)
nself tenant billing subscription cancel --at-period-end

# Cancel immediately
nself tenant billing subscription cancel --immediately

# Provide cancellation reason
nself tenant billing subscription cancel --reason="switching-providers"

# Confirm cancellation
nself tenant billing subscription cancel --force
```

Cancellation Reasons:
- `low-quality`
- `too-expensive`
- `switching-providers`
- `no-longer-needed`
- `other`

### Reactivate Subscription

```bash
# Reactivate canceled subscription
nself tenant billing subscription reactivate

# Reactivate with different plan
nself tenant billing subscription reactivate --plan=pro

# Effective immediately
nself tenant billing subscription reactivate --immediately
```

Reactivation Notes:
- Must be within 30 days of cancellation
- Pro-rated charges for remaining period
- Features restored instantly

### Subscription Workflow Examples

```bash
# View current plan and consider upgrading
nself tenant billing subscription show
nself tenant billing subscription plans --compare
nself tenant billing subscription upgrade pro --preview
nself tenant billing subscription upgrade pro

# Downgrade to save costs
nself tenant billing subscription downgrade starter --preview
nself tenant billing subscription downgrade starter --at-period-end

# Team plan management
nself tenant billing subscription show --include=members
nself tenant billing subscription upgrade pro  # More team members

# Subscription lifecycle
nself tenant billing subscription cancel --at-period-end
nself tenant billing subscription show  # Check status
nself tenant billing subscription reactivate  # Change mind
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_DEFAULT_PLAN` | `free` | Starting plan for new users |
| `BILLING_ALLOW_SELF_SERVICE_UPGRADE` | `true` | Users can upgrade |
| `BILLING_ALLOW_SELF_SERVICE_DOWNGRADE` | `true` | Users can downgrade |
| `BILLING_TRIAL_PERIOD_DAYS` | `14` | Free trial duration |
| `BILLING_FREQUENCY` | `both` | Monthly, annual, or both |
| `BILLING_ANNUAL_DISCOUNT` | `20` | Annual discount percentage |
| `BILLING_CANCELLATION_MODE` | `at_period_end` | Immediate or at period end |

---

## Quota Management

View and enforce usage limits.

### Commands

```bash
# Show all quota limits
nself tenant billing quota

# Show quota for specific service
nself tenant billing quota --service=api

# Show quota with current usage
nself tenant billing quota --usage

# Show quota alerts
nself tenant billing quota --alerts

# JSON format
nself tenant billing quota --format=json
```

### Show Quota Limits

```bash
# Display all quotas
nself tenant billing quota

# API quota details
nself tenant billing quota --service=api

# Storage quota details
nself tenant billing quota --service=storage

# Quota with remaining capacity
nself tenant billing quota --usage
```

Quota Display:
- Limit value
- Current usage
- Percentage used
- Remaining capacity
- Grace period (if exceeded)
- Alert threshold

### View Usage Within Quotas

```bash
# All quotas with usage
nself tenant billing quota --usage

# Service-specific with usage
nself tenant billing quota --service=api --usage

# Human-readable format
nself tenant billing quota --usage --format=table

# JSON export
nself tenant billing quota --usage --format=json
```

### Quota Alerts

```bash
# Show active alerts
nself tenant billing quota --alerts

# Show all alerts (including dismissed)
nself tenant billing quota --alerts --all

# Alerts for specific service
nself tenant billing quota --alerts --service=storage

# Dismiss alert
nself tenant billing quota --dismiss <alert-id>
```

Alert Types:
- `warning` - Usage at 80% of quota
- `critical` - Usage at 95% of quota
- `exceeded` - Over quota limit

### Quota Limits by Plan

| Service | Free | Starter | Pro | Enterprise |
|---------|------|---------|-----|------------|
| API | 1K/mo | 50K/mo | 1M/mo | Unlimited |
| Storage | 1 GB | 50 GB | 500 GB | Custom |
| Bandwidth | 5 GB | 50 GB | 500 GB | Custom |
| Compute | 100 hrs | 1K hrs | Unlimited | Unlimited |
| Team Members | 1 | 5 | Unlimited | Unlimited |
| Projects | 1 | 5 | Unlimited | Unlimited |

### Quota Configuration Examples

```bash
# Check if you're approaching limits
nself tenant billing quota --usage

# If API quota warning
nself tenant billing subscription upgrade pro  # Get higher quota

# Monitor specific service
while true; do
  nself tenant billing quota --service=storage --usage
  sleep 3600  # Check hourly
done

# Export quota report
nself tenant billing quota --format=csv > quota_report.csv

# Get alerts only
nself tenant billing quota --alerts
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_QUOTA_ENFORCEMENT` | `true` | Enforce limits |
| `BILLING_QUOTA_CHECK_MODE` | `realtime` | Realtime or batched |
| `BILLING_QUOTA_GRACE_PERIOD` | `30` | Grace period (minutes) |
| `BILLING_QUOTA_WARNING_THRESHOLD` | `80` | Warning at X% |
| `BILLING_DEFAULT_API_CALLS_QUOTA` | `1000` | API quota (free tier) |
| `BILLING_DEFAULT_STORAGE_QUOTA` | `1073741824` | Storage (1GB, bytes) |
| `BILLING_DEFAULT_BANDWIDTH_QUOTA` | `5368709120` | Bandwidth (5GB, bytes) |
| `BILLING_DEFAULT_COMPUTE_MINUTES_QUOTA` | `100` | Compute quota (minutes) |

---

## Billing Plans

View and compare subscription plans.

### Commands

```bash
# List all plans
nself tenant billing plan list

# Show plan details
nself tenant billing plan show <plan-name>

# Compare plans
nself tenant billing plan compare

# Show current plan
nself tenant billing plan current
```

### List Plans

```bash
# Show all available plans
nself tenant billing plan list

# Monthly pricing
nself tenant billing plan list --billing=monthly

# Annual pricing
nself tenant billing plan list --billing=annual

# Show features
nself tenant billing plan list --features

# JSON format
nself tenant billing plan list --format=json
```

Plan List Display:
- Plan name and tier
- Monthly and annual pricing
- Key features
- Quota limits
- Support level

### Show Plan Details

```bash
# Detailed information for plan
nself tenant billing plan show pro

# Include pricing history
nself tenant billing plan show pro --history

# Compare to current plan
nself tenant billing plan show pro --compare

# JSON output
nself tenant billing plan show pro --format=json
```

Plan Details Include:
- Plan description
- Price (monthly and annual)
- All features and quotas
- Included services
- Support tier
- Proration information

### Compare Plans

```bash
# Side-by-side comparison
nself tenant billing plan compare

# Compare specific plans
nself tenant billing plan compare --plans=free,starter,pro

# Show cost difference
nself tenant billing plan compare --cost

# Show feature differences
nself tenant billing plan compare --features
```

Comparison Shows:
- All features across plans
- Quota differences
- Price differences
- Feature set progression

### Show Current Plan

```bash
# Display your current plan
nself tenant billing plan current

# With features
nself tenant billing plan current --features

# With quotas
nself tenant billing plan current --quotas

# Cost breakdown
nself tenant billing plan current --cost
```

### Plan Features Reference

#### Free Plan
- Perfect for development
- 1K API calls/month
- 1 GB storage
- 5 GB bandwidth
- Community support
- Single team member

#### Starter Plan ($19/month)
- Small projects
- 50K API calls/month
- 50 GB storage
- 50 GB bandwidth
- Email support
- 5 team members

#### Pro Plan ($79/month)
- Production applications
- 1M API calls/month
- 500 GB storage
- 500 GB bandwidth
- Priority support
- Unlimited team members
- Custom integrations

#### Enterprise Plan (Custom)
- Large-scale deployments
- Unlimited everything
- Dedicated support
- SLA guarantee
- Custom features
- On-premise option

### Plan Workflow Examples

```bash
# Check plan pricing
nself tenant billing plan list

# View Pro plan details
nself tenant billing plan show pro

# Compare all plans
nself tenant billing plan compare

# Understand current plan
nself tenant billing plan current --features

# Check plan before upgrade
nself tenant billing plan show pro --compare
nself tenant billing subscription upgrade pro
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_FREQUENCY` | `both` | Monthly, annual, or both |
| `BILLING_ANNUAL_DISCOUNT` | `20` | Annual savings % |

---

## Payment Methods

Manage credit cards and other payment methods.

### Commands

```bash
# List payment methods
nself tenant billing payment list

# Add new payment method
nself tenant billing payment add

# Remove payment method
nself tenant billing payment remove <payment-id>

# Set as default
nself tenant billing payment default <payment-id>
```

### List Payment Methods

```bash
# Show all saved payment methods
nself tenant billing payment list

# Include details
nself tenant billing payment list --detailed

# JSON format
nself tenant billing payment list --format=json

# Show only active methods
nself tenant billing payment list --active
```

Payment Method Display:
- Card/account type
- Last 4 digits (card) or account number
- Expiration date (cards)
- Default status
- Added date

### Add Payment Method

```bash
# Interactive prompt for adding card
nself tenant billing payment add

# Add via Stripe link
nself tenant billing payment add --method=card

# Bank account payment
nself tenant billing payment add --method=us_bank_account

# SEPA debit (Europe)
nself tenant billing payment add --method=sepa_debit

# iDEAL (Netherlands)
nself tenant billing payment add --method=ideal

# ACH/bank transfer
nself tenant billing payment add --method=ach
```

Supported Payment Methods:
- **card** - Credit/debit cards (most common)
- **us_bank_account** - US bank account
- **sepa_debit** - European bank account
- **ideal** - Dutch iDEAL
- **giropay** - German Giropay
- **ach** - US ACH transfer

### Remove Payment Method

```bash
# Remove payment method
nself tenant billing payment remove pm_1XYZ123

# Confirm removal
nself tenant billing payment remove pm_1XYZ123 --force

# Update default if removing
nself tenant billing payment remove pm_1XYZ123 --new-default=pm_2ABC456
```

Removal Notes:
- Cannot remove if only method
- Cannot remove if default with active subscription
- Requires explicit confirmation in production

### Set Default Payment Method

```bash
# Set as default
nself tenant billing payment default pm_1XYZ123

# Confirm change
nself tenant billing payment default pm_1XYZ123 --force

# Update for specific subscription
nself tenant billing payment default pm_1XYZ123 --subscription=sub_123ABC
```

Default Method:
- Used for all future charges
- Cannot remove while default
- Can have multiple saved, one default
- Applies to all subscriptions

### Payment Method Examples

```bash
# Add credit card
nself tenant billing payment add

# View all payment methods
nself tenant billing payment list --detailed

# Make Amex card default
nself tenant billing payment default pm_1AmexCard

# Remove old card
nself tenant billing payment remove pm_1OldCard

# Update payment for renewal
nself tenant billing payment add  # Add new card
nself tenant billing payment default pm_newcard  # Set as default
nself tenant billing payment remove pm_oldcard  # Remove old card

# List with JSON for automation
nself tenant billing payment list --format=json
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_PAYMENT_METHODS` | `card,us_bank_account` | Accepted methods |
| `BILLING_REQUIRE_PAYMENT_METHOD` | `false` | Require payment method |
| `BILLING_ALLOW_MULTIPLE_PAYMENT_METHODS` | `true` | Multiple methods allowed |
| `BILLING_PAYMENT_RETRY_ATTEMPTS` | `3` | Retry failed payments |
| `BILLING_PAYMENT_RETRY_DELAY` | `3` | Days between retries |

---

## Export Billing Data

Export usage, invoices, and other billing data.

### Commands

```bash
# Export all billing data
nself tenant billing export --all

# Export usage data
nself tenant billing export usage

# Export invoices
nself tenant billing export invoices

# Export subscriptions
nself tenant billing export subscriptions

# Export payments
nself tenant billing export payments
```

### Export Options

```bash
# Specify format
nself tenant billing export usage --format=csv
nself tenant billing export usage --format=json

# Custom output file
nself tenant billing export usage --output=my_usage.csv

# Filter by date
nself tenant billing export invoices --start=2026-01-01 --end=2026-01-31

# Filter by year
nself tenant billing export invoices --year=2025

# Include metadata
nself tenant billing export --all --include=metadata
```

### Export Usage Data

```bash
# CSV format for spreadsheets
nself tenant billing export usage --format=csv

# Current month
nself tenant billing export usage --format=csv

# Specific month
nself tenant billing export usage --format=csv --start=2026-01-01 --end=2026-01-31

# All history
nself tenant billing export usage --format=csv --all-time

# By service
nself tenant billing export usage --format=csv --service=api
```

CSV Columns:
- Date
- Service
- Usage Quantity
- Unit
- Cost
- Plan Quota
- Over-quota

### Export Invoices

```bash
# All invoices
nself tenant billing export invoices --format=json

# Invoices from 2025
nself tenant billing export invoices --year=2025 --format=csv

# Paid invoices
nself tenant billing export invoices --status=paid --format=csv

# Date range
nself tenant billing export invoices --start=2026-01-01 --end=2026-12-31
```

CSV Columns:
- Invoice Number
- Invoice Date
- Amount
- Status
- Customer
- Description

### Export All Data

```bash
# Complete export
nself tenant billing export --all --format=json --output=complete_export.json

# All data in separate files
nself tenant billing export --all --format=csv

# Creates:
# - nself_billing_usage_[timestamp].csv
# - nself_billing_invoices_[timestamp].csv
# - nself_billing_subscriptions_[timestamp].csv
# - nself_billing_payments_[timestamp].csv
```

### Export Examples

```bash
# Export for tax/accounting
nself tenant billing export invoices --year=2025 --format=csv --output=2025_invoices.csv

# Usage analysis
nself tenant billing export usage --format=json --output=usage_data.json

# Import to accounting software
nself tenant billing export invoices --all --format=csv > invoices_import.csv

# Archive all billing data
mkdir billing_backup_2025
nself tenant billing export --all --format=json --output=billing_backup_2025/complete.json

# Export specific service usage
nself tenant billing export usage --service=api --format=csv > api_usage.csv
nself tenant billing export usage --service=storage --format=csv > storage_usage.csv
```

### Output Formats

| Format | Best For | Notes |
|--------|----------|-------|
| `csv` | Excel, Google Sheets | Easy import to spreadsheets |
| `json` | Programming, APIs | Machine readable, complete |

---

## Customer Information

Manage customer account details and billing portal.

### Commands

```bash
# Show customer info
nself tenant billing customer show

# Update customer details
nself tenant billing customer update

# Access billing portal
nself tenant billing customer portal
```

### Show Customer Info

```bash
# Display account information
nself tenant billing customer show

# Include billing history
nself tenant billing customer show --include=history

# Show all subscriptions
nself tenant billing customer show --subscriptions

# JSON output
nself tenant billing customer show --format=json
```

Customer Info Includes:
- Email address
- Name and company
- Billing address
- Tax ID
- Phone number
- Creation date
- Account status

### Update Customer Details

```bash
# Interactive update
nself tenant billing customer update

# Update name
nself tenant billing customer update --name="New Name"

# Update email
nself tenant billing customer update --email="new@example.com"

# Update address
nself tenant billing customer update \
  --address="123 Main St" \
  --city="San Francisco" \
  --state="CA" \
  --zip="94105"

# Update tax ID
nself tenant billing customer update --tax-id="12-3456789"

# Update company
nself tenant billing customer update --company="New Company Inc"
```

### Access Billing Portal

```bash
# Open self-service portal
nself tenant billing customer portal

# Print portal URL
nself tenant billing customer portal --url

# Show portal features
nself tenant billing customer portal --features
```

Billing Portal Features:
- View and download invoices
- Manage subscriptions
- Update payment methods
- View usage
- Update billing address
- Cancel subscription
- Access billing history

### Customer Portal Configuration

The customer portal allows self-service:
- Subscription management
- Invoice downloads
- Payment method updates
- Billing address changes
- Account information

```bash
# Access features
nself tenant billing customer portal  # Opens web link
```

### Customer Management Examples

```bash
# View current customer info
nself tenant billing customer show

# Update billing address
nself tenant billing customer update --address="456 Oak Ave" --city="NYC" --state="NY"

# Add tax ID
nself tenant billing customer update --tax-id="98-7654321"

# Portal for subscription changes
nself tenant billing customer portal
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_PORTAL_ENABLED` | `true` | Enable portal |
| `BILLING_PORTAL_PATH` | `/billing` | Portal URL path |
| `BILLING_PORTAL_ALLOW_CANCELLATION` | `true` | Allow cancel in portal |
| `BILLING_PORTAL_ALLOW_PAYMENT_UPDATE` | `true` | Update payment method |
| `BILLING_PORTAL_ALLOW_INVOICE_HISTORY` | `true` | View invoices |

---

## Stripe Integration

Test and configure Stripe webhooks and integrations.

### Commands

```bash
# Test webhook endpoint
nself tenant billing webhook test

# List webhooks
nself tenant billing webhook list

# View webhook events
nself tenant billing webhook events
```

### Test Webhook

```bash
# Test webhook endpoint
nself tenant billing webhook test

# Test specific event
nself tenant billing webhook test --event=customer.subscription.created

# Test with custom payload
nself tenant billing webhook test --event=invoice.payment_succeeded --payload=custom.json

# Verbose output
nself tenant billing webhook test --verbose

# Show expected vs actual
nself tenant billing webhook test --compare
```

Webhook Test Events:
- `customer.created`
- `customer.subscription.created`
- `customer.subscription.updated`
- `customer.subscription.deleted`
- `invoice.created`
- `invoice.finalized`
- `invoice.payment_succeeded`
- `invoice.payment_failed`
- `payment_method.attached`
- `charge.succeeded`
- `charge.failed`

### List Webhooks

```bash
# Show all registered webhooks
nself tenant billing webhook list

# Include endpoint URLs
nself tenant billing webhook list --urls

# Show enabled/disabled status
nself tenant billing webhook list --status

# JSON format
nself tenant billing webhook list --format=json
```

### View Webhook Events

```bash
# Recent webhook events
nself tenant billing webhook events

# Last 50 events
nself tenant billing webhook events --limit=50

# Filter by type
nself tenant billing webhook events --type=invoice.payment_succeeded

# Show failures
nself tenant billing webhook events --status=failed

# Time range
nself tenant billing webhook events --start=2026-01-01 --end=2026-01-31

# Retry failed events
nself tenant billing webhook events --retry --failed
```

### Stripe Configuration

Set up Stripe integration in your `.env`:

```bash
# Keys from https://dashboard.stripe.com/apikeys
STRIPE_PUBLISHABLE_KEY=pk_test_51ABC...
STRIPE_SECRET_KEY=sk_test_PLACEHOLDER...

# Webhook secret from https://dashboard.stripe.com/webhooks
STRIPE_WEBHOOK_SECRET=whsec_test_...

# Webhook URL (must be publicly accessible HTTPS)
BILLING_WEBHOOK_URL=https://api.yourdomain.com/webhooks/stripe

# Optional: specify Stripe API version
STRIPE_API_VERSION=2024-11-20
```

### Stripe API Reference

For detailed Stripe documentation:
- **Dashboard**: https://dashboard.stripe.com
- **API Docs**: https://stripe.com/docs/api
- **Libraries**: https://stripe.com/docs/libraries
- **Webhooks**: https://stripe.com/docs/webhooks

### Setup Stripe Webhook (Manual)

1. Log in to [Stripe Dashboard](https://dashboard.stripe.com)
2. Go to **Developers** → **Webhooks**
3. Click **Add Endpoint**
4. Enter endpoint URL: `https://yourdomain.com/webhooks/stripe`
5. Select events (or select all)
6. Copy webhook secret from **Signing secret**
7. Add to `.env`: `STRIPE_WEBHOOK_SECRET=whsec_...`

### Webhook Events Diagram

```
Stripe Account Event
        ↓
Stripe Webhook Service
        ↓
HTTPS POST to your endpoint
        ↓
Webhook signature verification
        ↓
Process event and update database
        ↓
Return 200 OK
```

### Webhook Examples

```bash
# Test webhook setup
nself tenant billing webhook test

# View recent events
nself tenant billing webhook events

# Check for failures
nself tenant billing webhook events --status=failed

# Retry failed webhook
nself tenant billing webhook events --retry --id=evt_123ABC

# Monitor webhook health
while true; do
  nself tenant billing webhook events --limit=10
  sleep 300  # Check every 5 minutes
done
```

---

## Environment Variables Reference

### Feature Enablement

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_ENABLED` | `false` | Enable billing system |
| `BILLING_STRIPE_ENABLED` | `false` | Enable Stripe payments |

### Stripe Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `STRIPE_PUBLISHABLE_KEY` | - | Stripe public key |
| `STRIPE_SECRET_KEY` | - | Stripe secret key |
| `STRIPE_WEBHOOK_SECRET` | - | Webhook signing secret |
| `STRIPE_API_VERSION` | - | API version (YYYY-MM-DD) |

### Usage Tracking

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_USAGE_TRACKING_ENABLED` | `true` | Track usage |
| `BILLING_USAGE_AGGREGATION` | `hourly` | Aggregation interval |
| `BILLING_USAGE_RETENTION_DAYS` | `90` | Retention period |
| `BILLING_USAGE_ANOMALY_DETECTION` | `true` | Detect anomalies |
| `BILLING_USAGE_ANOMALY_THRESHOLD` | `200` | Anomaly threshold % |

### Quota Enforcement

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_QUOTA_ENFORCEMENT` | `true` | Enforce limits |
| `BILLING_QUOTA_CHECK_MODE` | `realtime` | Check frequency |
| `BILLING_QUOTA_GRACE_PERIOD` | `30` | Grace period (min) |
| `BILLING_QUOTA_WARNING_THRESHOLD` | `80` | Warning threshold % |

### Default Quotas (Free Tier)

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_DEFAULT_API_CALLS_QUOTA` | `1000` | API calls/month |
| `BILLING_DEFAULT_STORAGE_QUOTA` | `1073741824` | Storage (bytes) |
| `BILLING_DEFAULT_BANDWIDTH_QUOTA` | `5368709120` | Bandwidth (bytes) |
| `BILLING_DEFAULT_COMPUTE_MINUTES_QUOTA` | `100` | Compute minutes |
| `BILLING_DEFAULT_USERS_QUOTA` | `1` | Team members |
| `BILLING_DEFAULT_PROJECTS_QUOTA` | `1` | Projects |

### Subscriptions & Plans

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_DEFAULT_PLAN` | `free` | Default plan |
| `BILLING_ALLOW_SELF_SERVICE_UPGRADE` | `true` | Allow upgrades |
| `BILLING_ALLOW_SELF_SERVICE_DOWNGRADE` | `true` | Allow downgrades |
| `BILLING_TRIAL_PERIOD_DAYS` | `14` | Trial duration |
| `BILLING_FREQUENCY` | `both` | Billing frequency |
| `BILLING_ANNUAL_DISCOUNT` | `20` | Annual discount % |

### Invoicing

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_AUTO_GENERATE_INVOICES` | `true` | Auto-generate |
| `BILLING_INVOICE_PREFIX` | `INV` | Invoice prefix |
| `BILLING_CURRENCY` | `usd` | Currency code |
| `BILLING_INVOICE_DUE_DAYS` | `7` | Days to pay |
| `BILLING_SEND_INVOICE_EMAILS` | `true` | Email invoices |

### Payment Methods

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_PAYMENT_METHODS` | `card,us_bank_account` | Accepted methods |
| `BILLING_REQUIRE_PAYMENT_METHOD` | `false` | Require method |
| `BILLING_ALLOW_MULTIPLE_PAYMENT_METHODS` | `true` | Multiple methods |
| `BILLING_PAYMENT_RETRY_ATTEMPTS` | `3` | Retry attempts |
| `BILLING_PAYMENT_RETRY_DELAY` | `3` | Retry delay (days) |

### Portal & Customer

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_PORTAL_ENABLED` | `true` | Enable portal |
| `BILLING_PORTAL_PATH` | `/billing` | Portal URL |
| `BILLING_PORTAL_ALLOW_CANCELLATION` | `true` | Allow cancellation |
| `BILLING_PORTAL_ALLOW_PAYMENT_UPDATE` | `true` | Update payment |
| `BILLING_PORTAL_ALLOW_INVOICE_HISTORY` | `true` | View invoices |

### Company Information

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_COMPANY_NAME` | - | Company name |
| `BILLING_COMPANY_ADDRESS` | - | Street address |
| `BILLING_COMPANY_CITY` | - | City |
| `BILLING_COMPANY_STATE` | - | State/Province |
| `BILLING_COMPANY_ZIP` | - | Postal code |
| `BILLING_COMPANY_COUNTRY` | - | Country |
| `BILLING_COMPANY_TAX_ID` | - | Tax/VAT ID |

### Webhooks

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_WEBHOOK_URL` | - | Webhook endpoint |
| `BILLING_WEBHOOK_EVENTS` | - | Event types |
| `BILLING_WEBHOOK_RETRY_POLICY` | `auto` | Retry policy |
| `BILLING_WEBHOOK_SIGNATURE_TOLERANCE` | `300` | Tolerance (sec) |

### Development & Testing

| Variable | Default | Description |
|----------|---------|-------------|
| `BILLING_TEST_MODE` | `true` | Test mode |
| `BILLING_DEV_BYPASS` | `false` | Dev bypass |
| `BILLING_SEED_TEST_DATA` | `false` | Seed test data |
| `BILLING_MOCK_STRIPE` | `false` | Mock Stripe |

---

## Common Workflows

### Setup Billing for New Project

```bash
# 1. Get Stripe API keys from https://dashboard.stripe.com/apikeys
# 2. Add to .env
BILLING_ENABLED=true
BILLING_STRIPE_ENABLED=true
STRIPE_PUBLISHABLE_KEY=pk_test_...
STRIPE_SECRET_KEY=sk_test_PLACEHOLDER...

# 3. Get webhook secret from https://dashboard.stripe.com/webhooks
STRIPE_WEBHOOK_SECRET=whsec_test_...

# 4. Test setup
nself tenant billing usage
nself tenant billing quota
nself tenant billing plan list
```

### Monitoring Usage

```bash
# Daily usage check
nself tenant billing usage --service=api

# Check quota status
nself tenant billing quota --usage

# Get alerts
nself tenant billing quota --alerts

# Export for analysis
nself tenant billing export usage --format=csv
```

### User Upgrades

```bash
# User checks their plan
nself tenant billing subscription show

# See available plans
nself tenant billing subscription plans --compare

# Upgrade to Pro
nself tenant billing subscription upgrade pro

# Confirm upgrade
nself tenant billing invoice list --limit=1
```

### Failed Payments

```bash
# Check unpaid invoices
nself tenant billing invoice list --status=open

# View invoice details
nself tenant billing invoice show in_123ABC

# Add/update payment method
nself tenant billing payment add

# Pay invoice
nself tenant billing invoice pay in_123ABC
```

### Compliance & Audit

```bash
# Export all 2025 data
nself tenant billing export invoices --year=2025 --format=csv

# Customer information
nself tenant billing customer show

# Payment history
nself tenant billing payment list --detailed

# Subscription audit
nself tenant billing subscription show --include=history
```

---

## Troubleshooting

### Stripe API Key Invalid

**Error**: `Stripe API key invalid`

**Solutions**:
- Verify keys from https://dashboard.stripe.com/apikeys
- Ensure no extra spaces or special characters
- Check if using test vs live keys correctly
- Test keys start with `pk_test_` and `sk_test_PLACEHOLDER`
- Live keys start with `pk_live_` and `sk_live_`

```bash
# Check current keys
echo "Publishable: $STRIPE_PUBLISHABLE_KEY"
echo "Secret: ${STRIPE_SECRET_KEY:0:10}***"
```

### Webhook Signature Verification Failed

**Error**: `Webhook signature verification failed`

**Solutions**:
- Verify `STRIPE_WEBHOOK_SECRET` matches dashboard
- Check webhook URL is publicly accessible HTTPS
- Ensure server time is synchronized (NTP)
- Check webhook signing secret not confused with API key

```bash
# Test webhook
nself tenant billing webhook test

# Check endpoint
echo $BILLING_WEBHOOK_URL
```

### Usage Not Tracking

**Error**: Usage shows zero or doesn't update

**Solutions**:
- Verify `BILLING_ENABLED=true`
- Check `BILLING_USAGE_TRACKING_ENABLED=true`
- Verify services are configured
- Check aggregation interval setting
- Review logs for tracking errors

```bash
# Check billing status
nself tenant billing usage

# Check quota
nself tenant billing quota

# Check logs
nself logs --service=billing
```

### Quota Not Enforcing

**Error**: Quota exceeded but requests still allowed

**Solutions**:
- Verify `BILLING_QUOTA_ENFORCEMENT=true`
- Check `BILLING_QUOTA_CHECK_MODE=realtime`
- Ensure grace period not active
- Check subscription plan quotas set

```bash
# Check quota settings
nself tenant billing quota --usage

# View alerts
nself tenant billing quota --alerts
```

### Invoices Not Generating

**Error**: No invoices created for payments

**Solutions**:
- Verify `BILLING_AUTO_GENERATE_INVOICES=true`
- Check Stripe subscription settings
- Ensure webhook events received
- Review invoice creation logs

```bash
# List invoices
nself tenant billing invoice list

# Check webhooks
nself tenant billing webhook events --type=invoice
```

### Proration Incorrect

**Error**: Upgrade/downgrade charges incorrect

**Solutions**:
- Verify `BILLING_PRORATION_ENABLED=true`
- Check `BILLING_PRORATION_BEHAVIOR` setting
- Verify subscription cycle dates in Stripe
- Review prorated charge in Stripe dashboard

```bash
# Check current subscription
nself tenant billing subscription show

# Compare plan costs
nself tenant billing plan compare
```

---

## API Integration

### GraphQL API

If `BILLING_GRAPHQL_ENABLED=true`, query billing data via GraphQL:

```graphql
query {
  billing {
    currentUsage {
      service
      usage
      quota
      percentage
    }
    invoices(limit: 10) {
      id
      amount
      status
      createdAt
    }
    subscription {
      plan
      status
      renewalDate
    }
  }
}
```

### REST API

Billing operations via REST endpoints:

```bash
# Get current usage
curl https://api.yourdomain.com/billing/usage

# Get invoices
curl https://api.yourdomain.com/billing/invoices

# Get subscription
curl https://api.yourdomain.com/billing/subscription
```

---

## Database Schema

Billing data stored in PostgreSQL (when synced):

```sql
-- Tables created in BILLING_DB_SCHEMA (default: billing)
billing.usage_events       -- Raw usage events
billing.usage_summary      -- Aggregated usage
billing.invoices           -- Invoice records
billing.subscriptions      -- Subscription data
billing.payment_methods    -- Saved payment methods
billing.customers          -- Customer information
billing.webhook_events     -- Webhook logs
billing.quotas             -- Quota configuration
```

### Query Examples

```sql
-- Current month usage by service
SELECT service, SUM(quantity) as total
FROM billing.usage_summary
WHERE date_part('month', created_at) = date_part('month', NOW())
GROUP BY service;

-- Customer invoices
SELECT id, amount, status, created_at
FROM billing.invoices
WHERE customer_id = 'cus_123ABC'
ORDER BY created_at DESC;

-- Usage trends
SELECT DATE(created_at) as date, service, SUM(quantity) as usage
FROM billing.usage_events
WHERE created_at >= NOW() - INTERVAL '30 days'
GROUP BY DATE(created_at), service
ORDER BY date, service;
```

---

## Best Practices

### Usage Monitoring
- Check quota alerts weekly: `nself tenant billing quota --alerts`
- Review monthly usage: `nself tenant billing usage --period=current`
- Set up cost monitoring for production

### Plan Management
- Review available plans quarterly: `nself tenant billing plan list`
- Check utilization vs plan: `nself tenant billing quota --usage`
- Upgrade before hitting soft limits

### Invoice Management
- Download invoices monthly for records
- Pay invoices on time to maintain service
- Keep payment method updated
- Export invoices annually for tax purposes

### Payment Security
- Never commit API keys to git
- Use `.secrets` file for production keys
- Rotate keys regularly
- Monitor API key usage
- Use webhook secret for signature verification

### Compliance
- Keep audit trail of all billing changes
- Export data for tax/accounting yearly
- Maintain customer information accuracy
- Document custom billing logic

---

## See Also

- [ENV.md](ENV.md) - Environment configuration
- [CONFIG.md](CONFIG.md) - General configuration
- [DEPLOY.md](DEPLOY.md) - Deployment and production
- [PROD.md](PROD.md) - Production best practices
- [Stripe API Docs](https://stripe.com/docs/api)
- [Stripe Dashboard](https://dashboard.stripe.com)
