# Billing System API Reference

Complete API reference for the nself billing system with Stripe integration, usage metering, and quota enforcement.

**Version:** 0.9.0
**Sprint:** 13 - Billing Integration & Usage Tracking

> **⚠️ v0.9.6+ Command Structure:** Billing commands have been consolidated under `nself tenant billing`. While the examples below show `nself billing`, the new syntax is `nself tenant billing`. Both syntaxes work (old commands show deprecation warnings). See [Command Consolidation Map](../../architecture/COMMAND-CONSOLIDATION-MAP.md) for full details.

---

## Table of Contents

1. [Customer Management](#1-customer-management)
2. [Subscription Management](#2-subscription-management)
3. [Usage Tracking](#3-usage-tracking)
4. [Invoice Management](#4-invoice-management)
5. [Payment Methods](#5-payment-methods)
6. [Quota Management](#6-quota-management)
7. [Plan Management](#7-plan-management)
8. [Webhooks](#8-webhooks)
9. [Data Export](#9-data-export)
10. [Programmatic API](#10-programmatic-api)
11. [Configuration](#11-configuration)
12. [Return Codes](#12-return-codes)
13. [Error Handling](#13-error-handling)

---

## 1. Customer Management

### `nself billing customer show`

Display current customer information from Stripe.

**Syntax:**
```bash
nself billing customer show
nself billing customer info  # Alias
```

**Output:**
```
Customer ID:  cus_XXXXXXXXXXXXXX
Name:         John Doe
Email:        john@example.com
Created:      2026-01-15
```

**Return Codes:**
- `0` - Success
- `1` - No customer ID found or API error

**Example:**
```bash
nself billing customer show
```

**Programmatic Usage:**
```bash
# Bash function
billing_init
stripe_customer_show

# Get customer ID only
customer_id=$(billing_get_customer_id)
echo "$customer_id"
```

**cURL Equivalent:**
```bash
curl -u "${STRIPE_SECRET_KEY}:" \
  https://api.stripe.com/v1/customers/${CUSTOMER_ID}
```

---

### `nself billing customer update`

Update customer information in Stripe.

**Syntax:**
```bash
nself billing customer update [options]
```

**Options:**
- `--email=<email>` - Update email address
- `--name=<name>` - Update customer name
- `--phone=<phone>` - Update phone number

**Example:**
```bash
# Update email
nself billing customer update --email="newemail@example.com"

# Update multiple fields
nself billing customer update \
  --name="Jane Smith" \
  --email="jane@example.com" \
  --phone="+1-555-0123"
```

**Return Codes:**
- `0` - Update successful
- `1` - No parameters provided or API error

**Programmatic Usage:**
```bash
stripe_customer_update --email="new@example.com"
```

---

### `nself billing customer portal`

Generate a Stripe customer portal session URL for self-service management.

**Syntax:**
```bash
nself billing customer portal
```

**Output:**
```
Customer Portal URL:
https://billing.stripe.com/session/XXXXXXXXXX

Portal session created
```

**Use Case:**
The customer portal allows users to:
- Update payment methods
- View billing history
- Download invoices
- Cancel subscriptions
- Update billing information

**Return Codes:**
- `0` - Portal session created
- `1` - Failed to create session

**Programmatic Usage:**
```bash
portal_url=$(stripe_customer_portal | grep -o 'https://[^[:space:]]*')
echo "Visit: $portal_url"
```

---

## 2. Subscription Management

### `nself billing subscription show`

Display current subscription details.

**Syntax:**
```bash
nself billing subscription show
nself billing subscription current  # Alias
```

**Output:**
```
Current Subscription

Subscription ID:  sub_XXXXXXXXXXXXXX
Plan:             pro
Status:           active
Current Period:   2026-01-01 to 2026-02-01
```

**Return Codes:**
- `0` - Success
- `1` - No active subscription or error

**Programmatic Usage:**
```bash
# Get subscription data
subscription_data=$(billing_get_subscription)

# Parse fields
IFS='|' read -r sub_id plan status start end cancel_at_end <<< "$subscription_data"
```

---

### `nself billing subscription plans`

List all available billing plans.

**Syntax:**
```bash
nself billing subscription plans
```

**Output:**
```
Available Plans

╔════════════════════════════════════════════════════════════════╗
║ Plan       │ Price/Month │ Features                          ║
╠════════════╪═════════════╪═══════════════════════════════════╣
║ Free       │ $0          │ 10K API requests, 1GB storage    ║
║ Starter    │ $29         │ 100K API requests, 10GB storage  ║
║ Pro        │ $99         │ 1M API requests, 100GB storage   ║
║ Enterprise │ Custom      │ Unlimited, dedicated support      ║
╚════════════╧═════════════╧═══════════════════════════════════╝
```

**Return Codes:**
- `0` - Success

**Programmatic Usage:**
```bash
stripe_plans_list
```

---

### `nself billing subscription upgrade <plan>`

Upgrade to a higher-tier plan with immediate effect.

**Syntax:**
```bash
nself billing subscription upgrade <plan_name>
```

**Arguments:**
- `<plan_name>` - Plan to upgrade to (starter, pro, enterprise)

**Example:**
```bash
# Upgrade to Pro plan
nself billing subscription upgrade pro
```

**Behavior:**
- Prorates charges immediately
- Creates invoice for prorated amount
- Takes effect immediately
- Updates quota limits instantly

**Return Codes:**
- `0` - Upgrade successful
- `1` - Plan not found, no active subscription, or API error

**Programmatic Usage:**
```bash
stripe_subscription_upgrade "pro"
```

---

### `nself billing subscription downgrade <plan>`

Downgrade to a lower-tier plan (takes effect at period end).

**Syntax:**
```bash
nself billing subscription downgrade <plan_name>
```

**Arguments:**
- `<plan_name>` - Plan to downgrade to (free, starter, pro)

**Example:**
```bash
# Downgrade to Starter plan
nself billing subscription downgrade starter
```

**Behavior:**
- Scheduled to take effect at end of current billing period
- No immediate charges
- Current plan features remain active until period end
- No proration applied

**Return Codes:**
- `0` - Downgrade scheduled
- `1` - Plan not found, no active subscription, or API error

**Programmatic Usage:**
```bash
stripe_subscription_downgrade "starter"
```

---

### `nself billing subscription cancel`

Cancel current subscription.

**Syntax:**
```bash
nself billing subscription cancel [--immediate]
```

**Options:**
- `--immediate` - Cancel immediately (default: cancel at period end)

**Example:**
```bash
# Cancel at end of billing period (default)
nself billing subscription cancel

# Cancel immediately
nself billing subscription cancel --immediate
```

**Behavior:**

**Default (period end):**
- Subscription remains active until period end
- No refunds issued
- Access continues until period end
- Can be reactivated before period end

**Immediate:**
- Subscription canceled immediately
- Access revoked immediately
- No refunds issued
- Cannot be reactivated

**Return Codes:**
- `0` - Cancellation successful
- `1` - No active subscription or API error

**Programmatic Usage:**
```bash
# Cancel at period end
stripe_subscription_cancel

# Cancel immediately
stripe_subscription_cancel --immediate
```

---

### `nself billing subscription reactivate`

Reactivate a subscription scheduled for cancellation.

**Syntax:**
```bash
nself billing subscription reactivate
```

**Example:**
```bash
nself billing subscription reactivate
```

**Behavior:**
- Only works for subscriptions with `cancel_at_period_end=true`
- Removes scheduled cancellation
- Subscription continues normally
- No charges applied

**Return Codes:**
- `0` - Reactivation successful
- `1` - No subscription found or not scheduled for cancellation

**Programmatic Usage:**
```bash
stripe_subscription_reactivate
```

---

## 3. Usage Tracking

### `nself billing usage`

Display usage statistics for the current billing period.

**Syntax:**
```bash
nself billing usage [options]
```

**Options:**
- `--service=<name>` - Filter by service (api, storage, bandwidth, compute, database, functions)
- `--period=<period>` - Time period: `current` (default), `last-month`, `custom`
- `--start=<date>` - Start date for custom period (YYYY-MM-DD)
- `--end=<date>` - End date for custom period (YYYY-MM-DD)
- `--detailed` - Show detailed daily breakdown
- `--format=<format>` - Output format: `table` (default), `json`, `csv`

**Examples:**

**Current period summary:**
```bash
nself billing usage
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║                      USAGE SUMMARY                             ║
╠════════════════════════════════════════════════════════════════╣
║ Period: 2026-01-01 to 2026-01-30                              ║
╠════════════════════════════════════════════════════════════════╣
║ Service          │ Usage          │ Unit       │ Cost        ║
╠══════════════════╪════════════════╪════════════╪═════════════╣
║ api              │         45.2K  │ requests   │ Included    ║
║ storage          │          2.5GB │ GB-hours   │ Included    ║
║ bandwidth        │        125.0GB │ GB         │ $1.25       ║
║ compute          │          5.2   │ CPU-hours  │ Included    ║
║ database         │        152.3K  │ queries    │ Included    ║
║ functions        │          1.8K  │ invocations│ Included    ║
╚══════════════════╧════════════════╧════════════╧═════════════╝
```

**Specific service:**
```bash
nself billing usage --service=api
```

**Last month:**
```bash
nself billing usage --period=last-month
```

**Custom date range:**
```bash
nself billing usage \
  --period=custom \
  --start=2025-12-01 \
  --end=2025-12-31
```

**Detailed breakdown:**
```bash
nself billing usage --detailed
```

**JSON output:**
```bash
nself billing usage --format=json
```

**Return Codes:**
- `0` - Success
- `1` - Invalid parameters or query error

**Programmatic Usage:**
```bash
# Get all usage
usage_get_all "2026-01-01" "2026-01-31" "json" "false"

# Get service usage
usage_get_service "api" "2026-01-01" "2026-01-31" "table" "true"
```

---

### Usage Tracking Functions

Internal functions for tracking resource usage in your application.

#### `usage_track_api_request`

Track an API request.

**Bash Function:**
```bash
usage_track_api_request <endpoint> [method] [status_code]
```

**Parameters:**
- `endpoint` - API endpoint path
- `method` - HTTP method (default: GET)
- `status_code` - Response status code (default: 200)

**Example:**
```bash
usage_track_api_request "/api/users" "POST" 201
```

**Metadata Stored:**
```json
{
  "endpoint": "/api/users",
  "method": "POST",
  "status": 201
}
```

---

#### `usage_track_storage`

Track storage usage.

**Bash Function:**
```bash
usage_track_storage <bytes> [duration_hours]
```

**Parameters:**
- `bytes` - Bytes stored
- `duration_hours` - Storage duration (default: 1)

**Example:**
```bash
# Track 100MB stored for 24 hours
usage_track_storage 104857600 24
```

**Unit:** GB-hours

---

#### `usage_track_bandwidth`

Track bandwidth usage.

**Bash Function:**
```bash
usage_track_bandwidth <bytes> [direction]
```

**Parameters:**
- `bytes` - Bytes transferred
- `direction` - `egress` (default) or `ingress`

**Example:**
```bash
# Track 50MB egress
usage_track_bandwidth 52428800 "egress"
```

**Unit:** GB

---

#### `usage_track_compute`

Track compute time.

**Bash Function:**
```bash
usage_track_compute <cpu_seconds> [metadata]
```

**Parameters:**
- `cpu_seconds` - CPU seconds consumed
- `metadata` - Optional JSON metadata

**Example:**
```bash
usage_track_compute 3600 '{"instance_type":"t3.medium"}'
```

**Unit:** CPU-hours

---

#### `usage_track_database_query`

Track a database query.

**Bash Function:**
```bash
usage_track_database_query [query_type] [duration_ms]
```

**Parameters:**
- `query_type` - Query type (default: SELECT)
- `duration_ms` - Query duration in milliseconds (default: 0)

**Example:**
```bash
usage_track_database_query "INSERT" 45
```

---

#### `usage_track_function`

Track a serverless function invocation.

**Bash Function:**
```bash
usage_track_function <function_name> [duration_ms] [memory_mb]
```

**Parameters:**
- `function_name` - Function identifier
- `duration_ms` - Execution duration (default: 0)
- `memory_mb` - Memory allocated (default: 128)

**Example:**
```bash
usage_track_function "process-image" 850 256
```

---

## 4. Invoice Management

### `nself billing invoice list`

List recent invoices.

**Syntax:**
```bash
nself billing invoice list
```

**Output:**
```
Recent Invoices

in_XXXXXXXXXXXXXXXX  2026-01-01  $99.00    paid
in_XXXXXXXXXXXXXXXX  2025-12-01  $99.00    paid
in_XXXXXXXXXXXXXXXX  2025-11-01  $99.00    paid
```

**Return Codes:**
- `0` - Success
- `1` - Query error

**Programmatic Usage:**
```bash
stripe_invoice_list
```

---

### `nself billing invoice show <id>`

Display detailed invoice information.

**Syntax:**
```bash
nself billing invoice show <invoice_id>
```

**Arguments:**
- `<invoice_id>` - Invoice ID (format: `in_XXXXXXXXXXXXXXXX`)

**Example:**
```bash
nself billing invoice show in_1234567890ABCDEF
```

**Output:**
```
Invoice: in_1234567890ABCDEF

invoice_id    | total_amount | status | period_start | period_end   | created_at
in_123456... |       99.00 | paid   | 2026-01-01   | 2026-02-01   | 2026-01-01
```

**Return Codes:**
- `0` - Success
- `1` - Invoice not found or error

**Programmatic Usage:**
```bash
stripe_invoice_show "in_1234567890ABCDEF"
```

---

### `nself billing invoice download <id>`

Download invoice as PDF.

**Syntax:**
```bash
nself billing invoice download <invoice_id>
```

**Arguments:**
- `<invoice_id>` - Invoice ID

**Example:**
```bash
nself billing invoice download in_1234567890ABCDEF
```

**Output:**
```
Downloading invoice: in_1234567890ABCDEF
Invoice downloaded: /path/to/.nself/billing/exports/in_1234567890ABCDEF.pdf
```

**Default Location:** `${NSELF_ROOT}/.nself/billing/exports/`

**Return Codes:**
- `0` - Download successful
- `1` - Invoice not found or download failed

**Programmatic Usage:**
```bash
stripe_invoice_download "in_1234567890ABCDEF"
```

---

### `nself billing invoice pay <id>`

Pay an unpaid invoice.

**Syntax:**
```bash
nself billing invoice pay <invoice_id>
```

**Arguments:**
- `<invoice_id>` - Invoice ID

**Example:**
```bash
nself billing invoice pay in_1234567890ABCDEF
```

**Behavior:**
- Uses default payment method on file
- Creates charge immediately
- Updates invoice status to `paid`
- Sends receipt email

**Return Codes:**
- `0` - Payment successful
- `1` - Payment failed or no payment method

**Programmatic Usage:**
```bash
stripe_invoice_pay "in_1234567890ABCDEF"
```

---

## 5. Payment Methods

### `nself billing payment list`

List saved payment methods.

**Syntax:**
```bash
nself billing payment list
```

**Output:**
```
Payment Methods

Payment methods listed in Stripe dashboard
Use customer portal for full management: nself billing customer portal
```

**Note:** For security, full payment method details are only shown in the Stripe Customer Portal.

**Return Codes:**
- `0` - Success
- `1` - Error retrieving payment methods

**Programmatic Usage:**
```bash
stripe_payment_list
```

---

### `nself billing payment add`

Add a new payment method.

**Syntax:**
```bash
nself billing payment add
```

**Output:**
```
Add Payment Method

Please use the customer portal to add payment methods securely:

  nself billing customer portal
```

**Note:** Adding payment methods requires PCI compliance. Use the Stripe Customer Portal for secure payment method management.

**Return Codes:**
- `0` - Instructions displayed

**Programmatic Usage:**
```bash
stripe_payment_add
```

---

### `nself billing payment remove <id>`

Remove a payment method.

**Syntax:**
```bash
nself billing payment remove <payment_method_id>
```

**Arguments:**
- `<payment_method_id>` - Payment method ID (format: `pm_XXXXXXXXXXXXXXXX`)

**Example:**
```bash
nself billing payment remove pm_1234567890ABCDEF
```

**Behavior:**
- Detaches payment method from customer
- Cannot remove default payment method if it's the only one
- Does not affect past charges

**Return Codes:**
- `0` - Removal successful
- `1` - Payment method not found or error

**Programmatic Usage:**
```bash
stripe_payment_remove "pm_1234567890ABCDEF"
```

---

### `nself billing payment default <id>`

Set default payment method.

**Syntax:**
```bash
nself billing payment default <payment_method_id>
```

**Arguments:**
- `<payment_method_id>` - Payment method ID

**Example:**
```bash
nself billing payment default pm_1234567890ABCDEF
```

**Behavior:**
- Sets as default for future invoices
- Used for automatic subscription renewals
- Does not affect pending invoices

**Return Codes:**
- `0` - Default set successfully
- `1` - Payment method not found or error

**Programmatic Usage:**
```bash
stripe_payment_set_default "pm_1234567890ABCDEF"
```

---

## 6. Quota Management

### `nself billing quota`

Display quota limits and usage for all services.

**Syntax:**
```bash
nself billing quota [options]
```

**Options:**
- `--service=<name>` - Show specific service only
- `--usage` - Include current usage statistics
- `--alerts` - Show only services with quota alerts
- `--format=<format>` - Output format: `table`, `json`, `csv`

**Examples:**

**All quotas (limits only):**
```bash
nself billing quota
```

**Output:**
```
╔════════════════════════════════════════════════════════════════════════╗
║                            QUOTA LIMITS                                ║
╠════════════════════════════════════════════════════════════════════════╣
║ Plan: pro                                                              ║
╠════════════════════════════════════════════════════════════════════════╣
║ Service      │ Limit        │ Type         │ Mode                 ║
╠══════════════╪══════════════╪══════════════╪══════════════════════╣
║ api          │        1.0M  │ requests     │ soft                 ║
║ storage      │      100.0GB │ GB-hours     │ soft                 ║
║ bandwidth    │      500.0GB │ GB           │ soft                 ║
║ compute      │       50.0   │ CPU-hours    │ soft                 ║
║ database     │        5.0M  │ queries      │ soft                 ║
║ functions    │      100.0K  │ invocations  │ soft                 ║
╚══════════════╧══════════════╧══════════════╧══════════════════════╝
```

**With usage:**
```bash
nself billing quota --usage
```

**Output:**
```
╔════════════════════════════════════════════════════════════════════════╗
║                            QUOTA LIMITS                                ║
╠════════════════════════════════════════════════════════════════════════╣
║ Plan: pro                                                              ║
╠════════════════════════════════════════════════════════════════════════╣
║ Service      │ Limit        │ Current Usage │ Available  │ Status   ║
╠══════════════╪══════════════╪═══════════════╪════════════╪══════════╣
║ api          │        1.0M  │        45.2K  │     954.8K │ ✓ 4%     ║
║ storage      │      100.0GB │         2.5GB │      97.5GB│ ✓ 2%     ║
║ bandwidth    │      500.0GB │       125.0GB │     375.0GB│ ✓ 25%    ║
║ compute      │       50.0   │          5.2  │       44.8 │ ✓ 10%    ║
║ database     │        5.0M  │       152.3K  │       4.8M │ ✓ 3%     ║
║ functions    │      100.0K  │         1.8K  │      98.2K │ ✓ 1%     ║
╚══════════════╧══════════════╧═══════════════╪════════════╧══════════╝

Status Legend:
  ✓  - Below 75% of quota
  ⚡ - 75-89% of quota (Warning)
  ⚠  - 90% or above (Critical)
  ⚠ OVER - Quota exceeded
```

**Specific service:**
```bash
nself billing quota --service=api --usage
```

**Quota alerts:**
```bash
nself billing quota --alerts
```

**Return Codes:**
- `0` - Success
- `1` - No active subscription or error

**Programmatic Usage:**
```bash
# Get all quotas
quota_get_all "true" "table"

# Get service quota
quota_get_service "api" "true" "json"

# Get alerts
quota_get_alerts "table"
```

---

### Quota Enforcement Functions

#### `quota_enforce`

Enforce quota limits before allowing an operation.

**Bash Function:**
```bash
quota_enforce <service> [requested_quantity]
```

**Parameters:**
- `service` - Service name
- `requested_quantity` - Units to consume (default: 1)

**Return Codes:**
- `0` - Quota available or soft limit (operation allowed)
- `1` - Hard limit exceeded (operation blocked)

**Example:**
```bash
if quota_enforce "api" 1; then
  # Process API request
  usage_track_api_request "/api/endpoint" "GET" 200
else
  # Return 429 Too Many Requests
  echo "Quota exceeded"
  exit 1
fi
```

**Enforcement Modes:**

**Soft Limit:**
- Logs warning when exceeded
- Allows operation to proceed
- Can trigger alerts
- May incur overage charges

**Hard Limit:**
- Blocks operation when quota reached
- Returns error to user
- No overage charges possible
- Requires plan upgrade to continue

---

#### `billing_check_quota`

Check if quota is available without enforcement action.

**Bash Function:**
```bash
billing_check_quota <service> [requested_quantity]
```

**Parameters:**
- `service` - Service name
- `requested_quantity` - Units to check (default: 1)

**Return Codes:**
- `0` - Quota available
- `1` - Quota would be exceeded

**Example:**
```bash
if billing_check_quota "storage" 1073741824; then
  # OK to store 1GB
  store_file "$file"
  usage_track_storage 1073741824 1
else
  echo "Insufficient storage quota"
fi
```

---

#### `billing_get_quota_status`

Get detailed quota status for a service.

**Bash Function:**
```bash
billing_get_quota_status <service>
```

**Parameters:**
- `service` - Service name

**Output:** JSON
```json
{
  "service": "api",
  "usage": 45200,
  "quota": 1000000,
  "percent": 4
}
```

**Example:**
```bash
status=$(billing_get_quota_status "api")
usage=$(echo "$status" | jq -r '.usage')
quota=$(echo "$status" | jq -r '.quota')
percent=$(echo "$status" | jq -r '.percent')

echo "API Usage: $usage / $quota ($percent%)"
```

---

## 7. Plan Management

### `nself billing plan list`

List all available billing plans.

**Syntax:**
```bash
nself billing plan list
```

**Output:**
```
Available Plans

╔════════════════════════════════════════════════════════════════╗
║ Plan       │ Price/Month │ Features                          ║
╠════════════╪═════════════╪═══════════════════════════════════╣
║ Free       │ $0          │ 10K API requests, 1GB storage    ║
║ Starter    │ $29         │ 100K API requests, 10GB storage  ║
║ Pro        │ $99         │ 1M API requests, 100GB storage   ║
║ Enterprise │ Custom      │ Unlimited, dedicated support      ║
╚════════════╧═════════════╧═══════════════════════════════════╝

Use 'nself billing plan show <plan>' for detailed information
Use 'nself billing subscription upgrade <plan>' to change plans
```

**Return Codes:**
- `0` - Success

**Programmatic Usage:**
```bash
stripe_plans_list
```

---

### `nself billing plan show <name>`

Display detailed plan information.

**Syntax:**
```bash
nself billing plan show <plan_name>
```

**Arguments:**
- `<plan_name>` - Plan name (free, starter, pro, enterprise)

**Example:**
```bash
nself billing plan show pro
```

**Output:**
```
Plan Details: pro

  api         : 1000000 requests
  storage     : 100 GB-hours
  bandwidth   : 500 GB
  compute     : 50 CPU-hours
  database    : 5000000 queries
  functions   : 100000 invocations
```

**Return Codes:**
- `0` - Success
- `1` - Plan not found

**Programmatic Usage:**
```bash
stripe_plan_show "pro"
```

---

### `nself billing plan compare`

Compare all available plans side-by-side.

**Syntax:**
```bash
nself billing plan compare
```

**Output:**
Comparison table showing quotas for all plans.

**Return Codes:**
- `0` - Success

**Programmatic Usage:**
```bash
stripe_plans_compare
```

---

### `nself billing plan current`

Show details of current plan.

**Syntax:**
```bash
nself billing plan current
```

**Output:**
Same as `nself billing plan show` for the active plan.

**Return Codes:**
- `0` - Success
- `1` - No active subscription

**Programmatic Usage:**
```bash
stripe_plan_current
```

---

## 8. Webhooks

### `nself billing webhook test`

Test webhook endpoint configuration.

**Syntax:**
```bash
nself billing webhook test [event_type]
```

**Example:**
```bash
nself billing webhook test
```

**Output:**
```
Testing webhook endpoint

Webhook URL: https://yourdomain.com/api/webhooks/stripe
Configure this URL in your Stripe Dashboard
```

**Return Codes:**
- `0` - Test completed

**Programmatic Usage:**
```bash
stripe_webhook_test
```

---

### `nself billing webhook list`

List configured webhook endpoints.

**Syntax:**
```bash
nself billing webhook list
```

**Output:**
```
Webhook Endpoints

  https://yourdomain.com/api/webhooks/stripe
  https://staging.yourdomain.com/api/webhooks/stripe
```

**Return Codes:**
- `0` - Success
- `1` - API error

**Programmatic Usage:**
```bash
stripe_webhook_list
```

---

### `nself billing webhook events`

List recent webhook events.

**Syntax:**
```bash
nself billing webhook events [limit]
```

**Arguments:**
- `[limit]` - Number of events to retrieve (default: 10)

**Example:**
```bash
nself billing webhook events 20
```

**Output:**
Raw JSON of recent webhook events from Stripe.

**Return Codes:**
- `0` - Success
- `1` - API error

**Programmatic Usage:**
```bash
stripe_webhook_events 50
```

---

## 9. Data Export

### `nself billing export`

Export billing data in various formats.

**Syntax:**
```bash
nself billing export [type] [options]
```

**Export Types:**
- `usage` - Usage records
- `invoices` - Invoice history
- `subscriptions` - Subscription data
- `payments` - Payment method info
- `all` - Complete billing export (default)

**Options:**
- `--format=<format>` - Export format: `json` (default), `csv`
- `--output=<file>` - Output filename (auto-generated if not specified)
- `--year=<year>` - Filter by year (for invoices and usage)

**Examples:**

**Export all data as JSON:**
```bash
nself billing export --all --format=json
```

**Export usage as CSV:**
```bash
nself billing export usage --format=csv
```

**Output:**
```
Exporting billing data to: nself_billing_usage_20260130_143052.csv
Export complete: nself_billing_usage_20260130_143052.csv
```

**Export invoices for 2025:**
```bash
nself billing export invoices --year=2025 --format=json
```

**Custom output file:**
```bash
nself billing export usage \
  --format=csv \
  --output=/path/to/billing-report.csv
```

**Return Codes:**
- `0` - Export successful
- `1` - Export failed or invalid parameters

**Programmatic Usage:**
```bash
# Export all data
billing_export_all "json" "/tmp/billing-export.json" "2026"

# Export usage only
billing_export_usage "csv" "/tmp/usage.csv" "2026"

# Export invoices
billing_export_invoices "json" "/tmp/invoices.json" "2025"
```

**Export Formats:**

**JSON (all):**
```json
{
  "customer": {
    "customer_id": "cus_XXXXXX",
    "email": "user@example.com",
    "name": "John Doe"
  },
  "subscription": {
    "subscription_id": "sub_XXXXXX",
    "plan_name": "pro",
    "status": "active"
  },
  "invoices": [...],
  "usage": [...]
}
```

**CSV (usage):**
```csv
date,service_name,usage,events
2026-01-30,api,1523,152
2026-01-30,storage,45.2,12
```

---

## 10. Programmatic API

### Initialization

All programmatic usage requires initialization:

```bash
#!/usr/bin/env bash

# Source required libraries
source "${NSELF_ROOT}/src/lib/billing/core.sh"
source "${NSELF_ROOT}/src/lib/billing/usage.sh"
source "${NSELF_ROOT}/src/lib/billing/stripe.sh"
source "${NSELF_ROOT}/src/lib/billing/quotas.sh"

# Initialize billing system
billing_init || {
  echo "Failed to initialize billing"
  exit 1
}
```

---

### Core Functions

#### `billing_init`

Initialize billing system and validate configuration.

**Signature:**
```bash
billing_init [quiet]
```

**Parameters:**
- `quiet` - Set to "true" to suppress output

**Return:** 0 on success, 1 on failure

---

#### `billing_get_customer_id`

Get current customer ID from environment or database.

**Signature:**
```bash
customer_id=$(billing_get_customer_id)
```

**Return:** Customer ID string or exits with code 1

---

#### `billing_get_subscription`

Get current subscription details.

**Signature:**
```bash
subscription_data=$(billing_get_subscription)
```

**Return:** Pipe-delimited subscription data:
```
sub_id|plan_name|status|period_start|period_end|cancel_at_end
```

---

#### `billing_record_usage`

Record a usage event.

**Signature:**
```bash
billing_record_usage <service> <quantity> [metadata]
```

**Parameters:**
- `service` - Service name
- `quantity` - Usage quantity
- `metadata` - Optional JSON metadata (default: {})

**Example:**
```bash
billing_record_usage "api" 1 '{"endpoint":"/users","method":"GET"}'
```

---

#### `billing_db_query`

Execute a database query.

**Signature:**
```bash
result=$(billing_db_query "SQL_QUERY" [format])
```

**Parameters:**
- `SQL_QUERY` - SQL query string
- `format` - Output format: `tuples` (default), `csv`, `json`

**Example:**
```bash
result=$(billing_db_query "SELECT COUNT(*) FROM billing_usage_records WHERE service_name='api';")
```

---

### Integration Example

Complete integration example for a custom API:

```bash
#!/usr/bin/env bash

# api-handler.sh - Example API with billing integration

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="${SCRIPT_DIR}/../.."

# Source billing modules
source "${NSELF_ROOT}/src/lib/billing/core.sh"
source "${NSELF_ROOT}/src/lib/billing/usage.sh"
source "${NSELF_ROOT}/src/lib/billing/quotas.sh"

# Initialize
billing_init "true" || exit 1

# Function: Handle API request with quota enforcement
handle_api_request() {
  local endpoint="$1"
  local method="$2"

  # Check quota before processing
  if ! quota_enforce "api" 1; then
    echo "HTTP/1.1 429 Too Many Requests"
    echo "Content-Type: application/json"
    echo ""
    echo '{"error":"API quota exceeded","code":"QUOTA_EXCEEDED"}'
    return 1
  fi

  # Process request
  local status_code
  process_request "$endpoint" "$method" && status_code=200 || status_code=500

  # Track usage
  usage_track_api_request "$endpoint" "$method" "$status_code"

  return 0
}

# Function: Store file with quota check
store_file() {
  local file_path="$1"
  local file_size
  file_size=$(stat -f%z "$file_path" 2>/dev/null || stat -c%s "$file_path")

  # Check storage quota (convert to GB-hours for 24h)
  local gb_hours
  gb_hours=$(awk "BEGIN {print ($file_size / 1073741824) * 24}")

  if ! billing_check_quota "storage" "$gb_hours"; then
    echo "Insufficient storage quota"
    return 1
  fi

  # Store file
  cp "$file_path" /storage/

  # Track usage
  usage_track_storage "$file_size" 24

  return 0
}

# Function: Execute function with tracking
execute_function() {
  local function_name="$1"
  local start_time
  start_time=$(date +%s%3N)

  # Check quota
  if ! quota_enforce "functions" 1; then
    echo "Function quota exceeded"
    return 1
  fi

  # Execute
  run_function "$function_name"
  local exit_code=$?

  # Calculate duration
  local end_time
  end_time=$(date +%s%3N)
  local duration=$((end_time - start_time))

  # Track usage
  usage_track_function "$function_name" "$duration" 256

  return $exit_code
}

# Main handler
main() {
  handle_api_request "/api/users" "GET"
}

main "$@"
```

---

## 11. Configuration

### Environment Variables

**Required:**
```bash
# Database Configuration
BILLING_DB_HOST=localhost
BILLING_DB_PORT=5432
BILLING_DB_NAME=nself
BILLING_DB_USER=postgres
BILLING_DB_PASSWORD=secure_password
```

**Stripe Configuration:**
```bash
# Stripe API Keys
STRIPE_SECRET_KEY=sk_test_PLACEHOLDER
STRIPE_PUBLISHABLE_KEY=pk_test_XXXXXXXXXXXXXXXXXXXX
STRIPE_WEBHOOK_SECRET=whsec_XXXXXXXXXXXXXXXXXXXX
STRIPE_API_VERSION=2023-10-16
```

**Optional:**
```bash
# Customer Identification
NSELF_CUSTOMER_ID=cus_XXXXXXXXXXXXXX
PROJECT_NAME=myproject

# Paths (auto-configured)
BILLING_DATA_DIR=${NSELF_ROOT}/.nself/billing
BILLING_CACHE_DIR=${NSELF_ROOT}/.nself/billing/cache
BILLING_EXPORT_DIR=${NSELF_ROOT}/.nself/billing/exports
BILLING_LOG_FILE=${NSELF_ROOT}/.nself/billing/billing.log

# API Configuration
STRIPE_API_BASE=https://api.stripe.com/v1
NSELF_BASE_URL=https://yourdomain.com
```

---

### Database Schema

The billing system requires these PostgreSQL tables:

**billing_customers:**
```sql
CREATE TABLE billing_customers (
  customer_id VARCHAR(255) PRIMARY KEY,
  project_name VARCHAR(255),
  email VARCHAR(255),
  name VARCHAR(255),
  created_at TIMESTAMP DEFAULT NOW()
);
```

**billing_subscriptions:**
```sql
CREATE TABLE billing_subscriptions (
  subscription_id VARCHAR(255) PRIMARY KEY,
  customer_id VARCHAR(255) REFERENCES billing_customers(customer_id),
  plan_name VARCHAR(50),
  status VARCHAR(50),
  current_period_start TIMESTAMP,
  current_period_end TIMESTAMP,
  cancel_at_period_end BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);
```

**billing_plans:**
```sql
CREATE TABLE billing_plans (
  plan_name VARCHAR(50) PRIMARY KEY,
  price_monthly DECIMAL(10,2),
  price_yearly DECIMAL(10,2),
  stripe_price_id VARCHAR(255),
  description TEXT,
  created_at TIMESTAMP DEFAULT NOW()
);
```

**billing_quotas:**
```sql
CREATE TABLE billing_quotas (
  id SERIAL PRIMARY KEY,
  plan_name VARCHAR(50) REFERENCES billing_plans(plan_name),
  service_name VARCHAR(50),
  limit_value BIGINT,
  limit_type VARCHAR(50),
  enforcement_mode VARCHAR(10),
  overage_price DECIMAL(10,6),
  UNIQUE(plan_name, service_name)
);
```

**billing_usage_records:**
```sql
CREATE TABLE billing_usage_records (
  id SERIAL PRIMARY KEY,
  customer_id VARCHAR(255) REFERENCES billing_customers(customer_id),
  service_name VARCHAR(50),
  quantity DECIMAL(20,6),
  metadata JSONB,
  recorded_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_usage_customer_service ON billing_usage_records(customer_id, service_name);
CREATE INDEX idx_usage_recorded_at ON billing_usage_records(recorded_at);
```

**billing_invoices:**
```sql
CREATE TABLE billing_invoices (
  invoice_id VARCHAR(255) PRIMARY KEY,
  customer_id VARCHAR(255) REFERENCES billing_customers(customer_id),
  total_amount DECIMAL(10,2),
  status VARCHAR(50),
  period_start TIMESTAMP,
  period_end TIMESTAMP,
  created_at TIMESTAMP DEFAULT NOW(),
  paid_at TIMESTAMP
);
```

---

## 12. Return Codes

All billing commands follow standard POSIX exit codes:

| Code | Meaning | Usage |
|------|---------|-------|
| `0` | Success | Command completed successfully |
| `1` | General error | Invalid parameters, API errors, not found |
| `2` | Misuse | Invalid command syntax |
| `127` | Command not found | Internal error (missing function) |

**Examples:**

```bash
# Check if command succeeded
if nself billing usage; then
  echo "Success"
else
  echo "Failed with code: $?"
fi

# Capture return code
nself billing subscription upgrade pro
exit_code=$?

if [ $exit_code -eq 0 ]; then
  echo "Upgrade successful"
elif [ $exit_code -eq 1 ]; then
  echo "Upgrade failed"
fi
```

---

## 13. Error Handling

### Common Errors

**No Customer ID:**
```
Error: No customer ID found
```

**Solution:**
- Set `NSELF_CUSTOMER_ID` environment variable
- Or add to `.env` file
- Or create customer record in database

**Stripe API Error:**
```
Error: Stripe API error: Invalid API Key
```

**Solution:**
- Verify `STRIPE_SECRET_KEY` is set correctly
- Check API key is for correct environment (test vs live)
- Ensure key has required permissions

**Database Connection Failed:**
```
Error: Database connection failed
```

**Solution:**
- Check PostgreSQL is running
- Verify database credentials
- Test connection: `psql -h $BILLING_DB_HOST -U $BILLING_DB_USER -d $BILLING_DB_NAME`

**Quota Exceeded:**
```
Warning: Quota exceeded for api (soft limit)
```

**Solution:**
- Upgrade to higher plan
- Wait for quota reset (next billing period)
- Contact support for enterprise limits

**Plan Not Found:**
```
Error: Plan not found: invalid-plan
```

**Solution:**
- List available plans: `nself billing plan list`
- Use exact plan name (case-sensitive)

---

### Debugging

**Enable verbose output:**
```bash
# Set log level
export NSELF_LOG_LEVEL=debug

# Run command
nself billing usage --detailed
```

**Check billing logs:**
```bash
tail -f ${NSELF_ROOT}/.nself/billing/billing.log
```

**Test database connection:**
```bash
# Source core module
source "${NSELF_ROOT}/src/lib/billing/core.sh"

# Test connection
if billing_test_db_connection; then
  echo "Database OK"
else
  echo "Database connection failed"
fi
```

**Test Stripe connection:**
```bash
source "${NSELF_ROOT}/src/lib/billing/core.sh"

if billing_test_stripe_connection; then
  echo "Stripe API OK"
else
  echo "Stripe API connection failed"
fi
```

**Validate configuration:**
```bash
source "${NSELF_ROOT}/src/lib/billing/core.sh"

billing_validate_config
echo "Validation exit code: $?"
```

---

## Additional Resources

- **Stripe API Documentation:** https://stripe.com/docs/api
- **nself Documentation:** https://docs.nself.org/billing
- **Database Schema:** `/docs/database/billing-schema.sql`
- **Integration Examples:** `/src/examples/billing/`

---

**Last Updated:** 2026-01-30
**Version:** 0.9.0
**Sprint:** 13 - Billing Integration & Usage Tracking
