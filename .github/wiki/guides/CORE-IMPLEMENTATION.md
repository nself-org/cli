# Billing Core Library - Implementation Summary

## Overview

Full implementation of `/Users/admin/Sites/nself/src/lib/billing/core.sh` with production-ready code for nself's billing system.

**File Statistics:**
- Total Lines: 899
- Exported Functions: 23
- Version: 0.9.0 (Sprint 13: Billing Integration & Usage Tracking)

## Implementation Details

### 1. Database Connection Functions

#### `billing_test_db_connection()`
- Tests PostgreSQL connection using psql
- Returns 0 on success, 1 on failure
- Uses environment variables with defaults
- Silent operation (errors suppressed)

#### `billing_db_query(query, format, ...params)`
- **Parameterized queries** using psql's `-v` flag
- Prevents SQL injection by using `:variable_name` placeholders
- Supports 3 output formats: tuples, csv, json
- Variable binding from key-value pairs
- Error handling with proper exit codes

**Example Usage:**
```bash
billing_db_query "SELECT * FROM billing_customers WHERE customer_id = :'id'" \
    "tuples" "id" "cust_123"
```

### 2. Database Initialization Functions

#### `billing_init_db()`
- **Idempotent** - safe to run multiple times
- Executes migration file: `015_create_billing_system.sql`
- Creates 8 billing tables:
  1. billing_customers
  2. billing_plans
  3. billing_subscriptions
  4. billing_quotas
  5. billing_usage_records
  6. billing_invoices
  7. billing_payment_methods
  8. billing_events
- Comprehensive error logging
- Returns 0 on success, 1 on failure

#### `billing_check_db_health()`
- Tests database connection
- Verifies table count (expects 8+ billing tables)
- Detects orphaned usage records
- Returns JSON health status:
  ```json
  {
    "status": "healthy|degraded|unhealthy",
    "table_count": 8,
    "orphaned_records": 0
  }
  ```

### 3. Customer Management Functions

#### `billing_create_customer(customer_id, project_name, email, name, company)`
- **Idempotent** - checks for existing customer first
- Creates customer record in `billing_customers` table
- Automatically creates default free subscription
- Parameterized query for SQL injection prevention
- Comprehensive logging of all operations
- Returns 0 on success (including if customer exists)

#### `billing_create_default_subscription(customer_id)`
- Creates free plan subscription for new customers
- Generates unique subscription ID: `sub_<timestamp>_<random>`
- Sets billing period to 1 month from creation
- **Cross-platform date handling** (BSD/GNU compatible)
- Automatic logging

#### `billing_get_customer(customer_id?)`
- Retrieves customer details
- Optional customer_id (uses `billing_get_customer_id()` if not provided)
- Excludes soft-deleted customers (deleted_at IS NULL)
- Returns all customer fields including Stripe ID

#### `billing_update_customer(customer_id, field_name, field_value)`
- Updates single customer field
- **Whitelisted fields** to prevent SQL injection:
  - email
  - name
  - company
  - stripe_customer_id
- Auto-updates `updated_at` timestamp
- Comprehensive logging

#### `billing_delete_customer(customer_id)`
- **Soft delete** - sets `deleted_at` timestamp
- Preserves historical data
- Allows for customer recovery
- Updates `updated_at` timestamp

#### `billing_list_customers(limit?, offset?)`
- Lists all active customers (not deleted)
- Pagination support (default: limit 100, offset 0)
- Ordered by creation date (newest first)
- Parameterized query for safe limit/offset

#### `billing_get_customer_plan(customer_id?)`
- Retrieves customer's active subscription plan
- Joins with `billing_plans` for full details
- Returns plan info, pricing, and billing cycle
- Only returns active or trialing subscriptions

### 4. Existing Enhanced Functions

#### `billing_get_customer_id()`
- Multi-source customer ID resolution:
  1. Environment variable: `NSELF_CUSTOMER_ID`
  2. Project config: `.env` file
  3. Database lookup by project name
- Returns first found customer ID

#### `billing_get_subscription(customer_id?)`
- Gets active subscription details
- Uses parameterized query
- Returns subscription period and cancellation status

#### `billing_record_usage(service, quantity, metadata?)`
- Records usage event in `billing_usage_records`
- Auto-generates timestamp (UTC)
- Parameterized insert query
- Comprehensive logging
- Metadata support (JSON)

#### `billing_check_quota(service, requested?)`
- Checks if quota allows requested usage
- Handles unlimited quotas (-1)
- Calculates current period usage
- Returns 0 if quota available, 1 if exceeded

#### `billing_get_quota_status(service)`
- Returns detailed quota status as JSON
- Calculates percentage used
- Handles unlimited quotas
- Output format:
  ```json
  {
    "service": "api",
    "usage": 5000,
    "quota": 10000,
    "percent": 50
  }
  ```

#### `billing_generate_invoice(customer_id, period_start, period_end)`
- Generates invoice for billing period
- Calculates usage-based charges
- Unique invoice ID: `inv_<timestamp>_<random>`
- Parameterized queries throughout
- Returns invoice_id

#### `billing_export_all(format, output_file, year?)`
- Exports billing data in JSON or CSV format
- JSON: Single file with all data
- CSV: Multiple files (_customer, _subscriptions, _invoices, _usage)
- Parameterized queries for security

#### `billing_get_summary(customer_id?)`
- Aggregate billing statistics
- Invoice count and total billed
- Services used count
- Joins subscriptions, invoices, and usage tables

### 5. Configuration Functions

#### `billing_init(quiet?)`
- Creates required directories
- Validates configuration
- Tests database connection
- Tests Stripe API (if configured)
- Returns 0 on success

#### `billing_validate_config()`
- Validates required database settings
- Checks directory permissions
- Warns about missing Stripe config
- Returns error count

#### `billing_test_stripe_connection()`
- Tests Stripe API connectivity
- Uses balance endpoint
- Returns 0 on success

#### `billing_log(event_type, service, value, metadata?)`
- Logs all billing events to file
- Format: `[timestamp] TYPE | service | value | metadata`
- Appends to: `$BILLING_LOG_FILE`

## Security Features

### 1. SQL Injection Prevention
- **All queries use parameterized variables** (`:variable_name`)
- Field name whitelisting in update functions
- No string concatenation in SQL queries
- psql's `-v` flag for safe variable binding

### 2. Input Validation
- Required parameter checks
- Type validation where applicable
- Whitelist validation for field names

### 3. Error Handling
- Comprehensive error checking on all database operations
- Proper exit codes (0 = success, 1+ = error)
- Error logging for audit trail
- Graceful degradation (quota checks allow if no billing setup)

## Cross-Platform Compatibility

### Date Handling
```bash
# BSD date (macOS)
if date -v+1m >/dev/null 2>&1; then
    period_end=$(date -u -v+1m +"%Y-%m-%d %H:%M:%S")
# GNU date (Linux)
else
    period_end=$(date -u -d "+1 month" +"%Y-%m-%d %H:%M:%S")
fi
```

### Random Generation
```bash
# Fallback if openssl unavailable
subscription_id="sub_$(date +%s)_$(openssl rand -hex 4 2>/dev/null || printf "%08x" $RANDOM)"
```

## Idempotency

Functions designed to be **idempotent** (safe to run multiple times):
- `billing_init_db()` - Migration tables use `IF NOT EXISTS`
- `billing_create_customer()` - Checks for existing customer first
- `billing_create_default_subscription()` - Only called if customer is new

## Environment Variables

All variables use defaults via `${VAR:-default}` pattern:

### Database Configuration
- `BILLING_DB_HOST` (default: localhost)
- `BILLING_DB_PORT` (default: 5432)
- `BILLING_DB_NAME` (default: nself)
- `BILLING_DB_USER` (default: postgres)
- `BILLING_DB_PASSWORD` (default: empty)

### Stripe Configuration (Optional)
- `STRIPE_SECRET_KEY`
- `STRIPE_PUBLISHABLE_KEY`
- `STRIPE_WEBHOOK_SECRET`
- `STRIPE_API_VERSION` (default: 2023-10-16)

### File Paths
- `BILLING_DATA_DIR` - `$NSELF_ROOT/.nself/billing`
- `BILLING_CACHE_DIR` - `$BILLING_DATA_DIR/cache`
- `BILLING_EXPORT_DIR` - `$BILLING_DATA_DIR/exports`
- `BILLING_LOG_FILE` - `$BILLING_DATA_DIR/billing.log`

## Logging

All operations are logged with:
- Timestamp (UTC)
- Event type (CREATE, UPDATE, DELETE, USAGE, ERROR, INIT)
- Service/resource name
- Value/ID
- Optional metadata

**Log Location:** `$NSELF_ROOT/.nself/billing/billing.log`

## Complete Function List (23 Functions)

### Core Functions
1. `billing_init(quiet?)`
2. `billing_validate_config()`
3. `billing_log(event_type, service, value, metadata?)`

### Database Functions
4. `billing_test_db_connection()`
5. `billing_db_query(query, format, ...params)`
6. `billing_init_db()`
7. `billing_check_db_health()`

### Customer Management
8. `billing_create_customer(customer_id, project_name, email, name, company)`
9. `billing_create_default_subscription(customer_id)`
10. `billing_get_customer(customer_id?)`
11. `billing_update_customer(customer_id, field_name, field_value)`
12. `billing_delete_customer(customer_id)`
13. `billing_list_customers(limit?, offset?)`
14. `billing_get_customer_id()`
15. `billing_get_customer_plan(customer_id?)`

### Subscription & Usage
16. `billing_get_subscription(customer_id?)`
17. `billing_record_usage(service, quantity, metadata?)`
18. `billing_check_quota(service, requested?)`
19. `billing_get_quota_status(service)`

### Invoicing & Reporting
20. `billing_generate_invoice(customer_id, period_start, period_end)`
21. `billing_get_summary(customer_id?)`
22. `billing_export_all(format, output_file, year?)`

### Stripe Integration
23. `billing_test_stripe_connection()`

## Usage Examples

### Initialize Billing System
```bash
# Initialize with schema migration
billing_init_db

# Initialize runtime
billing_init
```

### Create Customer
```bash
billing_create_customer \
    "cust_abc123" \
    "my-project" \
    "user@example.com" \
    "John Doe" \
    "Acme Corp"
```

### Record Usage
```bash
billing_record_usage "api" 100 '{"endpoint":"/users","method":"GET"}'
```

### Check Quota
```bash
if billing_check_quota "api" 1000; then
    echo "Quota available"
else
    echo "Quota exceeded"
fi
```

### Get Customer Plan
```bash
billing_get_customer_plan "cust_abc123"
```

### Export Data
```bash
billing_export_all "json" "/tmp/billing_export.json"
billing_export_all "csv" "/tmp/billing_export.csv"
```

### Check Health
```bash
billing_check_db_health
# Output: {"status":"healthy","table_count":8,"orphaned_records":0}
```

## Dependencies

- **Required:**
  - PostgreSQL client (psql)
  - bash 3.2+
  - Standard Unix utilities (date, grep, tr)

- **Optional:**
  - openssl (for random generation, falls back to $RANDOM)
  - curl (for Stripe API testing)

- **Internal:**
  - `src/lib/utils/output.sh` - success(), error(), warn()
  - `src/lib/utils/validation.sh` - validation utilities

## Testing

Run tests with:
```bash
bash /Users/admin/Sites/nself/src/tests/integration/test-billing.sh
```

## Related Files

- Schema: `/Users/admin/Sites/nself/src/database/migrations/015_create_billing_system.sql`
- CLI: `/Users/admin/Sites/nself/src/cli/billing.sh`
- Tests: `/Users/admin/Sites/nself/src/tests/integration/test-billing.sh`
- Related Libraries:
  - `/Users/admin/Sites/nself/src/lib/billing/usage.sh`
  - `/Users/admin/Sites/nself/src/lib/billing/quotas.sh`
  - `/Users/admin/Sites/nself/src/lib/billing/stripe.sh`

## Implementation Status

✅ **COMPLETE** - All core billing functionality implemented and production-ready.

### What Was Added
- Database initialization (`billing_init_db`)
- Database health checking (`billing_check_db_health`)
- Customer creation (`billing_create_customer`)
- Default subscription creation (`billing_create_default_subscription`)
- Customer retrieval (`billing_get_customer`)
- Customer updates (`billing_update_customer`)
- Customer deletion (`billing_delete_customer`)
- Customer listing (`billing_list_customers`)
- Customer plan retrieval (`billing_get_customer_plan`)

### What Was Enhanced
- **All database queries** now use parameterized queries
- Cross-platform date handling (BSD/GNU compatible)
- Improved error handling and logging
- Input validation and whitelisting
- Comprehensive documentation

---

**Generated:** 2026-01-30
**Author:** nself Development Team
**Version:** 0.9.0
