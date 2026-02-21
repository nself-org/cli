# Row-Level Security (RLS) in nself

## Overview

nself implements comprehensive Row-Level Security (RLS) to enforce multi-tenant isolation and secure access control at the database level. This document explains how RLS works, how to use it, and how to test it.

## What is Row-Level Security?

Row-Level Security (RLS) is a PostgreSQL feature that restricts which rows users can access in database tables. Instead of granting or revoking access to entire tables, RLS policies control access to individual rows based on user attributes.

### Benefits of RLS

1. **Multi-Tenant Isolation** - Each tenant can only access their own data
2. **Defense in Depth** - Security enforcement at the database level
3. **Simplified Application Logic** - No need for tenant filtering in every query
4. **Audit Compliance** - Clear security policies enforced uniformly
5. **Prevents Data Leaks** - Even if application logic fails, database protects data

## Architecture

### Session Variables

RLS policies in nself use PostgreSQL session variables to determine access:

| Variable | Type | Purpose | Example |
|----------|------|---------|---------|
| `app.current_customer_id` | VARCHAR | Billing customer identifier | `'cust_123'` |
| `app.current_tenant_id` | VARCHAR | Whitelabel tenant identifier | `'acme-corp'` |
| `app.current_brand_id` | UUID | Whitelabel brand UUID | `'uuid-here'` |
| `app.user_role` | VARCHAR | User role | `'admin'`, `'customer'`, `'tenant_admin'` |
| `app.is_admin` | BOOLEAN | Super admin flag | `true`, `false` |

### User Roles

#### Billing System Roles

| Role | Access Level | Can Do |
|------|--------------|--------|
| `admin` | Full access | Everything |
| `customer` | Own data only | View/update own records, manage payment methods |
| `system` | System operations | Insert usage records, process webhooks |
| `webhook` | Webhook processing | Insert billing events |
| `anonymous` | Public only | View active billing plans |

#### Whitelabel System Roles

| Role | Access Level | Can Do |
|------|--------------|--------|
| `admin` | Full access | Everything |
| `tenant_admin` | Tenant scope | Manage brand, domains, themes, assets |
| `tenant_user` | Read-only | View own tenant's active resources |
| `system` | System operations | Background operations |
| `email_service` | Email operations | Read templates, update sent counts |
| `cdn_service` | CDN delivery | Read active assets |
| `ssl_service` | SSL operations | Access certificates/keys |
| `public` | Public only | View public assets, primary brands |

## Setting Session Variables

### Application Layer (Node.js)

```javascript
// Set session variables before queries
await db.query(`
  SET LOCAL app.current_customer_id = $1;
  SET LOCAL app.user_role = $2;
`, [customerId, 'customer']);

// Now queries automatically respect RLS
const customers = await db.query('SELECT * FROM billing_customers');
// Returns only current customer's record
```

### Hasura GraphQL

Add session variables in Hasura configuration:

```yaml
# Hasura metadata
x-hasura-role: customer
x-hasura-customer-id: cust_123
```

Hasura JWT claims mapping:

```json
{
  "sub": "user-123",
  "https://hasura.io/jwt/claims": {
    "x-hasura-role": "customer",
    "x-hasura-customer-id": "cust_123",
    "x-hasura-tenant-id": "acme-corp"
  }
}
```

Configure Hasura to set session variables:

```sql
-- In Hasura connection settings or pre-query hook
SET LOCAL app.current_customer_id = current_setting('request.jwt.claims.x-hasura-customer-id', true);
SET LOCAL app.user_role = current_setting('request.jwt.claims.x-hasura-role', true);
```

### Direct PostgreSQL

```sql
-- Set for current transaction
SET LOCAL app.current_customer_id = 'cust_123';
SET LOCAL app.user_role = 'customer';

-- Query automatically filtered
SELECT * FROM billing_customers;
-- Only returns rows where customer_id = 'cust_123'
```

## RLS Policies by Table

### Billing Tables

#### billing_customers

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Admins | Full access to all customers |
| `customer_read_own` | SELECT | Customers | View own record only |
| `customer_update_own` | UPDATE | Customers | Update own record only |
| `customer_no_delete` | DELETE | All | Prevent deletion (use soft delete) |

#### billing_plans

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Admins | Full access |
| `public_read_active_plans` | SELECT | Everyone | View active plans |
| `no_public_write` | INSERT/UPDATE/DELETE | Non-admins | Block modifications |

#### billing_subscriptions

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Admins | Full access |
| `customer_read_own` | SELECT | Customers | View own subscriptions |
| `customer_update_cancel` | UPDATE | Customers | Update/cancel own subscription |
| `no_customer_insert` | INSERT | Non-admins | Block creation |

#### billing_quotas

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Admins | Full access |
| `customer_read_own_plan_quotas` | SELECT | Customers | View quotas for active plan |
| `no_customer_write` | INSERT/UPDATE/DELETE | Customers | Block modifications |

#### billing_usage_records

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Admins | Full access |
| `customer_read_own` | SELECT | Customers | View own usage |
| `system_insert` | INSERT | System/Admin | Insert usage records |
| `no_customer_update` | UPDATE/DELETE | Customers | Block modifications |

#### billing_invoices

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Admins | Full access |
| `customer_read_own` | SELECT | Customers | View own invoices |
| `no_customer_write` | INSERT/UPDATE/DELETE | Customers | Block modifications |

#### billing_payment_methods

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Admins | Full access |
| `customer_read_own` | SELECT | Customers | View own payment methods |
| `customer_insert_own` | INSERT | Customers | Add payment methods |
| `customer_update_own` | UPDATE | Customers | Update own methods |
| `customer_soft_delete_own` | DELETE | Customers | Soft delete own methods |

#### billing_events

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Admins | Full access |
| `customer_read_own` | SELECT | Customers | View own events |
| `webhook_system_insert` | INSERT | System/Webhook | Insert webhook events |
| `no_customer_update` | UPDATE/DELETE | Customers | Block modifications |

### Whitelabel Tables

#### whitelabel_brands

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Super Admin | Full access |
| `tenant_admin_full_access` | ALL | Tenant Admin | Manage own brand |
| `tenant_user_read_own` | SELECT | Tenant Users | View own brand |
| `public_read_primary` | SELECT | Public | View primary brands |

#### whitelabel_domains

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Super Admin | Full access |
| `tenant_admin_manage_own` | ALL | Tenant Admin | Manage own domains |
| `tenant_user_read_own` | SELECT | Tenant Users | View own domains |
| `public_read_active` | SELECT | Public | View active domains (DNS verification) |

#### whitelabel_themes

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Super Admin | Full access |
| `tenant_admin_manage_own` | ALL | Tenant Admin | Manage own themes |
| `tenant_user_read_own` | SELECT | Tenant Users | View own themes |
| `public_read_system_themes` | SELECT | Public | View built-in system themes |

#### whitelabel_email_templates

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Super Admin | Full access |
| `tenant_admin_manage_own` | ALL | Tenant Admin | Manage own templates |
| `tenant_user_read_own` | SELECT | Tenant Users | View own templates |
| `system_read_for_sending` | SELECT | Email Service | Read for sending emails |
| `system_update_stats` | UPDATE | Email Service | Update sent counts |

#### whitelabel_assets

| Policy | Operation | Who | What |
|--------|-----------|-----|------|
| `admin_all_access` | ALL | Super Admin | Full access |
| `tenant_admin_manage_own` | ALL | Tenant Admin | Manage own assets |
| `tenant_user_read_own` | SELECT | Tenant Users | View own assets |
| `public_read_public_assets` | SELECT | Public | View public assets (CDN) |
| `cdn_read_assets` | SELECT | CDN Service | Read for delivery |
| `admin_only_secrets` | SELECT | Admin/SSL Service | Access certificates/keys |

## Helper Functions

### get_current_customer_id()

Returns the current customer ID from session variable.

```sql
SELECT get_current_customer_id();
-- Returns: 'cust_123' or NULL
```

### is_current_user_admin()

Checks if current user has admin privileges.

```sql
SELECT is_current_user_admin();
-- Returns: true or false
```

### get_current_tenant_id()

Returns the current tenant ID from session variable.

```sql
SELECT get_current_tenant_id();
-- Returns: 'acme-corp' or NULL
```

### get_current_brand_id()

Returns the current brand UUID from session variable.

```sql
SELECT get_current_brand_id();
-- Returns: UUID or NULL
```

### is_current_user_tenant_admin()

Checks if current user is a tenant administrator.

```sql
SELECT is_current_user_tenant_admin();
-- Returns: true or false
```

### get_current_user_role()

Returns the current user's role.

```sql
SELECT get_current_user_role();
-- Returns: 'admin', 'customer', 'tenant_admin', etc.
```

## Business Logic Functions (RLS-Aware)

### get_quota_usage(customer_id, service_name)

Get quota usage for a customer/service with RLS enforcement.

```sql
-- Customer can only query their own usage
SET LOCAL app.current_customer_id = 'cust_123';
SET LOCAL app.user_role = 'customer';

SELECT * FROM get_quota_usage('cust_123', 'api');
-- Returns quota info

SELECT * FROM get_quota_usage('cust_999', 'api');
-- ERROR: Access denied to customer data
```

### is_quota_exceeded(customer_id, service_name, quantity)

Check if quota would be exceeded with RLS enforcement.

```sql
SET LOCAL app.current_customer_id = 'cust_123';
SELECT is_quota_exceeded('cust_123', 'api', 1000);
-- Returns: true or false
```

## Usage Examples

### Example 1: Customer Viewing Their Data

```sql
-- Application sets session variables
SET LOCAL app.current_customer_id = 'cust_123';
SET LOCAL app.user_role = 'customer';

-- Customer queries their data
SELECT * FROM billing_customers;
-- Returns: 1 row (their own)

SELECT * FROM billing_subscriptions;
-- Returns: only their subscriptions

SELECT * FROM billing_usage_records;
-- Returns: only their usage

-- Customer tries to access another customer
SELECT * FROM billing_customers WHERE customer_id = 'cust_999';
-- Returns: 0 rows (blocked by RLS)
```

### Example 2: Admin Viewing All Data

```sql
-- Application sets admin session
SET LOCAL app.user_role = 'admin';
SET LOCAL app.is_admin = true;

-- Admin sees everything
SELECT * FROM billing_customers;
-- Returns: ALL customers

SELECT * FROM billing_subscriptions;
-- Returns: ALL subscriptions
```

### Example 3: Tenant Admin Managing Brand

```sql
-- Application sets tenant context
SET LOCAL app.current_tenant_id = 'acme-corp';
SET LOCAL app.user_role = 'tenant_admin';

-- Tenant admin manages their brand
SELECT * FROM whitelabel_brands;
-- Returns: 1 row (their brand only)

UPDATE whitelabel_brands
SET brand_name = 'ACME Corporation'
WHERE tenant_id = 'acme-corp';
-- Success: can update own brand

-- Add a custom domain
INSERT INTO whitelabel_domains (brand_id, domain)
VALUES ((SELECT id FROM whitelabel_brands WHERE tenant_id = 'acme-corp'), 'app.acme.com');
-- Success: can manage own domains

-- Try to modify another tenant
UPDATE whitelabel_brands
SET brand_name = 'Hacked'
WHERE tenant_id = 'other-tenant';
-- Fails: 0 rows updated (blocked by RLS)
```

### Example 4: Public User (CDN Asset Access)

```sql
-- Public user accessing assets
SET LOCAL app.user_role = 'public';

-- Can view public assets
SELECT * FROM whitelabel_assets WHERE is_public = true;
-- Returns: all public assets

-- Cannot view private assets
SELECT * FROM whitelabel_assets WHERE is_public = false;
-- Returns: 0 rows (blocked by RLS)
```

### Example 5: System Service (Email Sending)

```sql
-- Email service sending emails
SET LOCAL app.user_role = 'email_service';
SET LOCAL app.current_brand_id = 'uuid-of-brand';

-- Read template for sending
SELECT * FROM whitelabel_email_templates
WHERE template_name = 'welcome' AND is_active = true;
-- Returns: active templates

-- Update sent count
UPDATE whitelabel_email_templates
SET sent_count = sent_count + 1,
    last_sent_at = NOW()
WHERE id = 'template-uuid';
-- Success: can update statistics
```

## Testing RLS Policies

### Running the Test Suite

```bash
# Run the comprehensive test suite
psql -U postgres -d nself_db -f src/database/migrations/tests/test_rls_policies.sql
```

### Manual Testing

```sql
-- Test 1: Verify RLS is enabled
SELECT
    schemaname,
    tablename,
    rowsecurity
FROM pg_tables
WHERE schemaname = 'public'
AND (tablename LIKE 'billing_%' OR tablename LIKE 'whitelabel_%');

-- Test 2: Count policies per table
SELECT
    tablename,
    COUNT(*) as policy_count
FROM pg_policies
WHERE schemaname = 'public'
GROUP BY tablename
ORDER BY tablename;

-- Test 3: Test customer isolation
SET LOCAL app.current_customer_id = 'cust_123';
SET LOCAL app.user_role = 'customer';
SELECT COUNT(*) FROM billing_customers;
-- Should return: 1

-- Test 4: Test tenant isolation
SET LOCAL app.current_tenant_id = 'my-tenant';
SET LOCAL app.user_role = 'tenant_admin';
SELECT COUNT(*) FROM whitelabel_brands;
-- Should return: 1
```

## Performance Considerations

### Indexes for RLS

RLS policies use the following indexes for optimal performance:

```sql
-- Billing tables
CREATE INDEX idx_billing_customers_customer_id ON billing_customers(customer_id);
CREATE INDEX idx_billing_subscriptions_customer_status ON billing_subscriptions(customer_id, status);
CREATE INDEX idx_billing_usage_records_customer_service ON billing_usage_records(customer_id, service_name);

-- Whitelabel tables
CREATE INDEX idx_whitelabel_brands_tenant_active ON whitelabel_brands(tenant_id, is_active);
CREATE INDEX idx_whitelabel_domains_brand_active ON whitelabel_domains(brand_id, is_active);
CREATE INDEX idx_whitelabel_assets_public ON whitelabel_assets(is_public) WHERE is_public = true;
```

### Query Planning

Check if RLS policies are being used efficiently:

```sql
EXPLAIN ANALYZE
SELECT * FROM billing_customers;
-- Look for Index Scan on idx_billing_customers_customer_id
```

## Security Best Practices

### 1. Always Set Session Variables

Never query tables without setting session variables first:

```javascript
// BAD: No session variables
const result = await db.query('SELECT * FROM billing_customers');

// GOOD: Set session variables first
await db.query('SET LOCAL app.current_customer_id = $1', [customerId]);
await db.query('SET LOCAL app.user_role = $2', ['customer']);
const result = await db.query('SELECT * FROM billing_customers');
```

### 2. Use Transactions

Set session variables within transactions to ensure they're scoped correctly:

```javascript
await db.transaction(async (trx) => {
  await trx.raw('SET LOCAL app.current_customer_id = ?', [customerId]);
  await trx.raw('SET LOCAL app.user_role = ?', ['customer']);
  const result = await trx('billing_customers').select('*');
  return result;
});
```

### 3. Validate Session Variables

Always validate session variables in your application:

```javascript
function setCustomerContext(db, customerId, role) {
  // Validate inputs
  if (!customerId || !role) {
    throw new Error('Customer ID and role required');
  }

  // Whitelist allowed roles
  const allowedRoles = ['admin', 'customer', 'tenant_admin', 'system'];
  if (!allowedRoles.includes(role)) {
    throw new Error('Invalid role');
  }

  // Set session variables
  return db.query(`
    SET LOCAL app.current_customer_id = $1;
    SET LOCAL app.user_role = $2;
  `, [customerId, role]);
}
```

### 4. Don't Bypass RLS in Application Logic

RLS is defense in depth. Don't add customer_id filters in your application queries:

```javascript
// BAD: Redundant filtering (and might conflict with RLS)
const customers = await db.query(
  'SELECT * FROM billing_customers WHERE customer_id = $1',
  [customerId]
);

// GOOD: Let RLS handle filtering
await db.query('SET LOCAL app.current_customer_id = $1', [customerId]);
const customers = await db.query('SELECT * FROM billing_customers');
```

### 5. Audit RLS Policies Regularly

Periodically verify RLS policies are working:

```sql
-- Run verification function
SELECT * FROM verify_tenant_isolation();

-- Check for tables without RLS
SELECT tablename
FROM pg_tables t
WHERE schemaname = 'public'
AND (tablename LIKE 'billing_%' OR tablename LIKE 'whitelabel_%')
AND NOT EXISTS (
  SELECT 1 FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'public'
  AND c.relname = t.tablename
  AND c.relrowsecurity = true
);
```

## Troubleshooting

### Issue: Queries Return Empty Results

**Cause**: Session variables not set correctly.

**Solution**:
```sql
-- Check current session variables
SELECT
  current_setting('app.current_customer_id', true) as customer_id,
  current_setting('app.user_role', true) as role;

-- Set variables and retry
SET LOCAL app.current_customer_id = 'cust_123';
SET LOCAL app.user_role = 'customer';
```

### Issue: Permission Denied Errors

**Cause**: RLS policy blocking access.

**Solution**: Verify user has correct role and access:
```sql
-- Check what role is set
SELECT get_current_user_role();

-- Check if admin
SELECT is_current_user_admin();

-- View applicable policies
SELECT * FROM pg_policies WHERE tablename = 'billing_customers';
```

### Issue: Slow Queries After Enabling RLS

**Cause**: Missing indexes for RLS policy checks.

**Solution**: Add indexes on columns used in RLS policies:
```sql
-- Check query plan
EXPLAIN ANALYZE SELECT * FROM billing_customers;

-- Add missing indexes
CREATE INDEX idx_name ON table_name(column_used_in_policy);
```

## Migration Guide

### Applying RLS Migrations

```bash
# Apply billing RLS
psql -U postgres -d nself_db -f src/database/migrations/019_add_billing_rls.sql

# Apply whitelabel RLS
psql -U postgres -d nself_db -f src/database/migrations/020_add_whitelabel_rls.sql

# Test policies
psql -U postgres -d nself_db -f src/database/migrations/tests/test_rls_policies.sql
```

### Verifying Migration Success

```sql
-- Check RLS is enabled
SELECT COUNT(*) as tables_with_rls
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public'
AND c.relrowsecurity = true
AND c.relname LIKE 'billing_%' OR c.relname LIKE 'whitelabel_%';

-- Should return: 13 (8 billing + 5 whitelabel)

-- Check policy count
SELECT COUNT(*) as total_policies
FROM pg_policies
WHERE schemaname = 'public'
AND (tablename LIKE 'billing_%' OR tablename LIKE 'whitelabel_%');

-- Should return: 60+ policies
```

## Further Reading

- [PostgreSQL Row Security Policies](https://www.postgresql.org/docs/current/ddl-rowsecurity.html)
- [nself Security Architecture](../security/SECURITY-SYSTEM.md)
- [Multi-Tenant Design Patterns](../architecture/MULTI-TENANCY.md)

## Support

For issues or questions about RLS:
- GitHub Issues: https://github.com/nself-org/cli/issues
- Documentation: https://docs.nself.org
- Community: https://discord.gg/nself
