# Database Seeding Guide

Complete guide to seeding data in nself projects with best practices and examples.

---

## Table of Contents

1. [What is Seeding?](#what-is-seeding)
2. [Seed Directory Structure](#seed-directory-structure)
3. [Creating Seed Files](#creating-seed-files)
4. [Applying Seeds](#applying-seeds)
5. [Seed Tracking](#seed-tracking)
6. [Environment-Specific Seeds](#environment-specific-seeds)
7. [nHost Auth Seeds](#nhost-auth-seeds)
8. [Best Practices](#best-practices)
9. [Examples](#examples)

---

## What is Seeding?

**Database seeding** is the process of populating your database with initial or test data.

**Use cases:**
- **Development:** Create test users, sample products, demo data
- **Staging:** Load realistic test data for QA
- **Production:** Create initial admin users, default settings, reference data

**Seeds vs Migrations:**
- **Migrations:** Change database **structure** (tables, columns, indexes)
- **Seeds:** Insert **data** into existing tables

---

## Seed Directory Structure

```
project/
└── nself/
    └── seeds/
        ├── common/          # Applied in ALL environments
        │   ├── 001_auth_users.sql
        │   ├── 002_roles.sql
        │   └── 003_settings.sql
        ├── local/           # Local development only
        │   ├── 001_test_data.sql
        │   └── 002_demo_products.sql
        ├── staging/         # Staging environment
        │   └── 001_staging_users.sql
        └── production/      # Production environment
            └── 001_admin_user.sql
```

**Execution order:**
1. All seeds in `common/` (alphabetically)
2. All seeds in `<environment>/` (alphabetically)

**Naming convention:** `###_description.sql`
- `###` = Order number (001, 002, 003, ...)
- `description` = What the seed does
- `.sql` = SQL file extension

---

## Creating Seed Files

### Quick Create

```bash
# Create seed for common environment
nself db seed create my_data common

# Create seed for specific environment
nself db seed create test_users local
```

**Output:**
```
✓ Created seed: nself/seeds/local/001_test_users.sql
```

### Manual Creation

Create file `nself/seeds/common/001_example.sql`:

```sql
-- Seed: Example Data
-- Environment: common
-- Created: 2026-02-11

-- Your SQL here
INSERT INTO categories (name, slug)
VALUES
  ('Electronics', 'electronics'),
  ('Books', 'books'),
  ('Clothing', 'clothing')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO products (name, category, price)
VALUES
  ('Laptop', 'electronics', 999.99),
  ('Novel', 'books', 14.99),
  ('T-Shirt', 'clothing', 19.99)
ON CONFLICT (name) DO NOTHING;
```

---

## Applying Seeds

### Apply All Seeds

```bash
nself db seed apply
```

**What happens:**
1. Creates `nself_seeds` tracking table (if doesn't exist)
2. Reads seeds from `common/` directory
3. Reads seeds from current environment directory
4. Checks which seeds are already applied
5. Applies only new seeds
6. Records applied seeds in tracking table

**Output:**
```
ℹ Applying seeds for environment: local
ℹ Applying seed: 001_auth_users.sql
✓   Applied successfully
ℹ Applying seed: 002_test_data.sql
✓   Applied successfully
ℹ Already applied: 003_settings.sql

✓ Applied 2 seed(s)
```

### Apply Specific Seed

```bash
# By filename
nself db seed apply nself/seeds/common/001_auth_users.sql

# By path
nself db seed apply ./my-custom-seed.sql
```

### Alias: seed run

For backward compatibility:

```bash
# Same as 'seed apply'
nself db seed run
```

---

## Seed Tracking

nself tracks which seeds have been applied to prevent duplicate execution.

### Tracking Table

Automatically created on first `seed apply`:

```sql
CREATE TABLE nself_seeds (
  filename VARCHAR(255) PRIMARY KEY,
  applied_at TIMESTAMPTZ DEFAULT NOW(),
  environment VARCHAR(50)
);
```

### Check Seed Status

```bash
# List all seeds with status
nself db seed list
```

**Output:**
```
ℹ Available Seeds (environment: local)

Seed File                                          Status
-------------------------------------------------- ----------
common/001_auth_users.sql                          ✓ Applied
common/002_roles.sql                               ✓ Applied
common/003_settings.sql                            ○ Pending
local/001_test_data.sql                            ✓ Applied
local/002_demo_products.sql                        ○ Pending
```

### View Tracking Table

```bash
nself db query "SELECT * FROM nself_seeds ORDER BY applied_at DESC"
```

---

## Rolling Back Seeds

```bash
nself db seed rollback
```

**What it does:**
1. Finds last applied seed
2. Removes from tracking table
3. **Does NOT automatically undo changes**
4. You must manually revert the data

**Output:**
```
⚠ This will NOT automatically undo changes - manual intervention required
ℹ Rolling back: 002_test_data.sql
✓ Seed tracking removed: 002_test_data.sql
ℹ Note: You must manually revert database changes
```

**To fully rollback:**
1. Run `nself db seed rollback`
2. Manually write DELETE/UPDATE statements to undo changes
3. Or restore from backup: `nself db backup restore`

---

## Environment-Specific Seeds

### Setting Environment

```bash
# Via environment variable
export ENV=staging
nself db seed apply

# Or in .env file
ENV=staging
```

**Environments:**
- `local` (default) - Development
- `staging` - Staging/QA
- `production` - Production

### Example: Different Users Per Environment

**common/001_base_roles.sql** (all environments):
```sql
INSERT INTO roles (name) VALUES ('owner'), ('admin'), ('user')
ON CONFLICT DO NOTHING;
```

**local/001_test_users.sql** (development only):
```sql
-- Create 100 test users with weak passwords
INSERT INTO auth.users (email, password_hash)
SELECT
  'user' || i || '@test.local',
  crypt('password123', gen_salt('bf', 10))
FROM generate_series(1, 100) AS i;
```

**production/001_admin.sql** (production only):
```sql
-- Create single admin with strong password
INSERT INTO auth.users (email, password_hash)
VALUES (
  'admin@company.com',
  crypt('{{ADMIN_PASSWORD}}', gen_salt('bf', 10))
);
```

---

## nHost Auth Seeds

### Quick Create Staff Users

```bash
# Use built-in auth setup
nself auth setup --default-users
```

Creates:
- `owner@nself.org` (role: owner)
- `admin@nself.org` (role: admin)
- `support@nself.org` (role: support)

All with password: `npass123`

### Custom Auth Seed File

Create `nself/seeds/common/001_auth_users.sql`:

```sql
-- Ensure provider exists
INSERT INTO auth.providers (id) VALUES ('email')
ON CONFLICT (id) DO NOTHING;

-- Create user in auth.users
INSERT INTO auth.users (
  id,
  display_name,
  password_hash,
  email_verified,
  locale,
  default_role,
  metadata,
  created_at,
  updated_at
) VALUES (
  '11111111-1111-1111-1111-111111111111',  -- Fixed UUID for idempotency
  'Platform Owner',
  crypt('your_password', gen_salt('bf', 10)),  -- bcrypt hash
  true,
  'en',
  'user',
  '{"role": "owner"}'::jsonb,  -- Custom role in metadata
  NOW(),
  NOW()
) ON CONFLICT (id) DO UPDATE SET
  password_hash = EXCLUDED.password_hash,
  metadata = EXCLUDED.metadata,
  updated_at = NOW();

-- Link user to email provider
INSERT INTO auth.user_providers (
  id,
  user_id,
  provider_id,
  provider_user_id,
  access_token,
  created_at,
  updated_at
) VALUES (
  gen_random_uuid(),
  '11111111-1111-1111-1111-111111111111',
  'email',
  'owner@company.com',  -- The actual email
  'seed_token_' || gen_random_uuid()::text,  -- Dummy token
  NOW(),
  NOW()
) ON CONFLICT (provider_id, provider_user_id) DO NOTHING;
```

Apply:
```bash
nself db seed apply
```

### Using Auth Seed Template

Copy from templates:
```bash
cp src/templates/seeds/001_auth_users.sql.template \
   nself/seeds/common/001_auth_users.sql

# Edit placeholders
sed -i 's/{{DEFAULT_PASSWORD}}/your_password/g' nself/seeds/common/001_auth_users.sql
sed -i 's/{{OWNER_EMAIL}}/owner@yourcompany.com/g' nself/seeds/common/001_auth_users.sql

# Apply
nself db seed apply
```

---

## Best Practices

### 1. Make Seeds Idempotent

**Use `ON CONFLICT` to prevent duplicate inserts:**

```sql
-- ❌ BAD: Will fail on second run
INSERT INTO categories (id, name) VALUES (1, 'Books');

-- ✅ GOOD: Safe to run multiple times
INSERT INTO categories (id, name) VALUES (1, 'Books')
ON CONFLICT (id) DO NOTHING;

-- ✅ BETTER: Update on conflict
INSERT INTO categories (id, name, slug) VALUES (1, 'Books', 'books')
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  slug = EXCLUDED.slug;
```

### 2. Use Fixed IDs for Reference Data

```sql
-- ✅ Use fixed UUIDs for users you'll reference
INSERT INTO auth.users (id, display_name, ...)
VALUES ('11111111-1111-1111-1111-111111111111', 'Admin', ...)
ON CONFLICT (id) DO NOTHING;

-- Now you can reference this ID in other seeds
INSERT INTO posts (author_id, title, content)
VALUES ('11111111-1111-1111-1111-111111111111', 'First Post', '...');
```

### 3. Order Matters

Number files by dependency order:

```
001_users.sql       # Create users first
002_categories.sql  # Then categories
003_products.sql    # Then products (which reference categories)
004_orders.sql      # Finally orders (which reference users and products)
```

### 4. Separate by Purpose

```
common/
  001_auth_schema.sql   # Auth setup
  002_roles.sql         # Roles and permissions
  003_settings.sql      # App settings

local/
  001_test_users.sql    # Test users
  002_demo_data.sql     # Demo products, posts, etc.
```

### 5. Document Your Seeds

```sql
-- Seed: Initial Auth Users
-- Purpose: Creates default staff users for access
-- Environment: common (all environments)
-- Created: 2026-02-11
-- Author: DevOps Team
--
-- This seed creates three staff users:
-- - owner@company.com (full access)
-- - admin@company.com (admin access)
-- - support@company.com (support access)
--
-- Default password: npass123 (CHANGE IN PRODUCTION!)

-- Your SQL here...
```

### 6. Never Commit Production Secrets

```bash
# ❌ BAD: Password in seed file
INSERT INTO users (email, password)
VALUES ('admin@company.com', 'SuperSecret123!');

# ✅ GOOD: Use environment variable or manual creation
# Production users should be created via:
nself auth create-user --email=admin@company.com
```

### 7. Test Seeds Thoroughly

```bash
# Test apply
nself db seed apply

# Verify data
nself db query "SELECT COUNT(*) FROM auth.users"

# Test idempotency (run again)
nself db seed apply  # Should show "Already applied"

# Test rollback
nself db seed rollback

# Test reapply
nself db seed apply
```

---

## Examples

### Example 1: Product Catalog

`nself/seeds/common/001_products.sql`:

```sql
-- Categories
INSERT INTO categories (id, name, slug, description)
VALUES
  (1, 'Electronics', 'electronics', 'Electronic devices'),
  (2, 'Books', 'books', 'Physical and digital books'),
  (3, 'Clothing', 'clothing', 'Apparel and accessories')
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  slug = EXCLUDED.slug,
  description = EXCLUDED.description;

-- Products
INSERT INTO products (name, category_id, price, stock, description)
VALUES
  ('Laptop Pro', 1, 1299.99, 50, '15-inch laptop'),
  ('Wireless Mouse', 1, 29.99, 200, 'Ergonomic mouse'),
  ('Programming Book', 2, 49.99, 100, 'Learn to code'),
  ('T-Shirt', 3, 19.99, 500, 'Cotton t-shirt')
ON CONFLICT (name) DO UPDATE SET
  price = EXCLUDED.price,
  stock = EXCLUDED.stock;
```

### Example 2: Blog Posts with Authors

`nself/seeds/local/001_blog_posts.sql`:

```sql
-- Ensure test author exists
INSERT INTO auth.users (id, display_name, password_hash, email_verified)
VALUES (
  '99999999-9999-9999-9999-999999999999',
  'Test Author',
  crypt('password', gen_salt('bf', 10)),
  true
) ON CONFLICT (id) DO NOTHING;

-- Link to email
INSERT INTO auth.user_providers (id, user_id, provider_id, provider_user_id, access_token)
VALUES (
  gen_random_uuid(),
  '99999999-9999-9999-9999-999999999999',
  'email',
  'author@test.local',
  'seed_token_test'
) ON CONFLICT (provider_id, provider_user_id) DO NOTHING;

-- Create blog posts
INSERT INTO posts (author_id, title, slug, content, published)
SELECT
  '99999999-9999-9999-9999-999999999999',
  'Post ' || i,
  'post-' || i,
  'Content for post ' || i,
  true
FROM generate_series(1, 50) AS i
ON CONFLICT (slug) DO NOTHING;
```

### Example 3: Settings and Configuration

`nself/seeds/common/001_app_settings.sql`:

```sql
CREATE TABLE IF NOT EXISTS app_settings (
  key VARCHAR(100) PRIMARY KEY,
  value TEXT,
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO app_settings (key, value) VALUES
  ('app_name', 'My Application'),
  ('support_email', 'support@company.com'),
  ('items_per_page', '25'),
  ('enable_registrations', 'true'),
  ('maintenance_mode', 'false')
ON CONFLICT (key) DO UPDATE SET
  value = EXCLUDED.value,
  updated_at = NOW();
```

### Example 4: Roles and Permissions

`nself/seeds/common/002_roles_permissions.sql`:

```sql
-- Create roles
INSERT INTO roles (name, description) VALUES
  ('owner', 'Full system access'),
  ('admin', 'Administrative access'),
  ('moderator', 'Content moderation'),
  ('user', 'Standard user')
ON CONFLICT (name) DO UPDATE SET
  description = EXCLUDED.description;

-- Create permissions
INSERT INTO permissions (name, resource, action) VALUES
  ('manage_users', 'users', 'manage'),
  ('view_users', 'users', 'view'),
  ('create_posts', 'posts', 'create'),
  ('edit_posts', 'posts', 'edit'),
  ('delete_posts', 'posts', 'delete')
ON CONFLICT (name) DO NOTHING;

-- Assign permissions to roles
INSERT INTO role_permissions (role_name, permission_name)
SELECT 'owner', name FROM permissions
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_name, permission_name)
VALUES
  ('admin', 'manage_users'),
  ('admin', 'view_users'),
  ('admin', 'edit_posts'),
  ('moderator', 'view_users'),
  ('moderator', 'edit_posts'),
  ('user', 'create_posts')
ON CONFLICT DO NOTHING;
```

### Example 5: Test Data Generation

`nself/seeds/local/002_generate_test_data.sql`:

```sql
-- Generate 1000 test users
INSERT INTO users (email, name, created_at)
SELECT
  'user' || i || '@test.local',
  'Test User ' || i,
  NOW() - (random() * interval '365 days')
FROM generate_series(1, 1000) AS i
ON CONFLICT (email) DO NOTHING;

-- Generate random orders
INSERT INTO orders (user_id, total, status, created_at)
SELECT
  (SELECT id FROM users ORDER BY random() LIMIT 1),
  (random() * 1000)::numeric(10,2),
  (ARRAY['pending', 'completed', 'shipped'])[floor(random() * 3 + 1)],
  NOW() - (random() * interval '90 days')
FROM generate_series(1, 5000)
ON CONFLICT DO NOTHING;
```

---

## Advanced Topics

### Using Transactions

```sql
BEGIN;

-- All or nothing
INSERT INTO categories (...) VALUES (...);
INSERT INTO products (...) VALUES (...);

COMMIT;
```

### Conditional Seeding

```sql
-- Only seed if empty
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM users) THEN
    INSERT INTO users (...) VALUES (...);
  END IF;
END $$;
```

### Seeding from CSV

```sql
-- Create temp table
CREATE TEMP TABLE temp_products (
  name TEXT,
  price NUMERIC,
  stock INTEGER
);

-- Load from CSV (if file is mounted in container)
COPY temp_products FROM '/path/to/products.csv' WITH CSV HEADER;

-- Insert into real table
INSERT INTO products (name, price, stock)
SELECT name, price, stock FROM temp_products
ON CONFLICT (name) DO UPDATE SET
  price = EXCLUDED.price,
  stock = EXCLUDED.stock;
```

---

## Troubleshooting

### Seed Not Applying

**Check seed file location:**
```bash
ls -la nself/seeds/common/
ls -la nself/seeds/local/
```

**Check seed tracking:**
```bash
nself db seed list
```

**Force reapply:**
```bash
nself exec postgres psql -U postgres -d your_db \
  -c "DELETE FROM nself_seeds WHERE filename = '001_my_seed.sql'"
nself db seed apply
```

### SQL Syntax Errors

**Test SQL directly:**
```bash
nself exec postgres psql -U postgres -d your_db < nself/seeds/common/001_test.sql
```

**Check PostgreSQL logs:**
```bash
nself logs postgres --tail 50
```

### Seeds Applied in Wrong Order

**Verify alphabetical ordering:**
```bash
ls -1 nself/seeds/common/ | sort
```

**Rename files:**
```bash
mv 5_users.sql 001_users.sql
mv 10_products.sql 002_products.sql
```

---

## Next Steps

- Read [AUTH_SETUP.md](./AUTH_SETUP.md) for authentication seeding
- Read [DEV_WORKFLOW.md](./DEV_WORKFLOW.md) for complete workflow
- Explore seed templates in `src/templates/seeds/`
- Learn about database migrations: `nself db migrate --help`

---

**Questions? Issues?**
- GitHub: https://github.com/nself-org/cli/issues
- Docs: https://docs.nself.org
