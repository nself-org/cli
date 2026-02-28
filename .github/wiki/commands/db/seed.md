# nself db seed

**Category**: Database Commands

Load seed data into the database with ordered, idempotent execution.

## Overview

Apply seed data to populate database tables for development, testing, or initial production data.

**Features**:
- ✅ Ordered execution (01_, 02_, 03_ prefixes)
- ✅ Idempotent with ON CONFLICT
- ✅ Hot loading (no restart needed)
- ✅ Selective execution
- ✅ Multiple seed directories supported

## Usage

```bash
nself db seed [options] [seed_name]
```

## Arguments

| Argument | Description |
|----------|-------------|
| `seed_name` | Optional: Specific seed to run |
| (none) | Run all seeds in order |

## Options

| Option | Description |
|--------|-------------|
| `--list` | List available seed files |
| `-v, --verbose` | Show detailed output |
| `--force` | Skip confirmation prompts |

## Seed Directories

### Hasura-Style
- Location: `hasura/seeds/default/`
- Format: `*.sql` files

### Custom
- Location: `db/seeds/`
- Format: `*.sql` or `0N_name.sql` for ordering

## Examples

### Run All Seeds

```bash
nself db seed
```

**Output**:
```
→ Loading seed data...
  ⠋ Running: 01_users.sql
  ✓ Inserted 5 rows

  ⠋ Running: 02_roles.sql
  ✓ Inserted 3 rows

✓ All seeds applied successfully
```

### Run Specific Seed

```bash
nself db seed users
# Or
nself db seed 01_users
```

### List Available Seeds

```bash
nself db seed --list
```

**Output**:
```
Available seed files:
  01_users.sql
  02_roles.sql
  03_demo_data.sql
```

## Seed File Format

### Structure

```sql
-- Seed: Demo Users
-- Description: Create demo accounts for development
-- Idempotent: Uses ON CONFLICT to allow re-running

INSERT INTO auth.users (id, email, display_name, role)
VALUES
  ('demo-user-1', 'demo@example.com', 'Demo User', 'user'),
  ('demo-user-2', 'admin@example.com', 'Admin User', 'admin'),
  ('demo-user-3', 'test@example.com', 'Test User', 'user')
ON CONFLICT (email) DO NOTHING;

INSERT INTO user_preferences (user_id, theme, notifications)
VALUES
  ('demo-user-1', 'dark', true),
  ('demo-user-2', 'light', false)
ON CONFLICT (user_id) DO UPDATE
  SET theme = EXCLUDED.theme,
      notifications = EXCLUDED.notifications;
```

### Making Seeds Idempotent

**Use ON CONFLICT**:
```sql
-- For unique constraints
INSERT INTO roles (id, name)
VALUES (1, 'admin'), (2, 'user')
ON CONFLICT (id) DO NOTHING;

-- Update if exists
INSERT INTO settings (key, value)
VALUES ('app_name', 'MyApp')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;
```

**Use IF NOT EXISTS**:
```sql
-- For schema objects
CREATE TYPE IF NOT EXISTS user_status AS ENUM ('active', 'inactive');
```

**Check before inserting**:
```sql
-- Delete and re-insert for exact state
DELETE FROM demo_data WHERE demo = true;
INSERT INTO demo_data (...) VALUES (...);
```

## Ordering Seeds

### Numeric Prefix

```
db/seeds/
├── 01_users.sql
├── 02_roles.sql
├── 03_user_roles.sql
└── 04_demo_data.sql
```

**Execution order**: 01 → 02 → 03 → 04

### Naming Convention

- `0N_` prefix for ordering
- Descriptive names
- `.sql` extension

**Examples**:
- `01_core_tables.sql`
- `02_reference_data.sql`
- `03_test_users.sql`
- `04_demo_content.sql`

## Use Cases

### Development Environment

```bash
# Reset database and seed
nself db reset
nself db migrate up
nself db seed
```

**Seeds**:
```
01_admin_user.sql       # Create admin account
02_demo_users.sql       # Test users
03_sample_content.sql   # Example data
```

### Testing Environment

```bash
# Load test fixtures
nself db seed test_fixtures
```

**Seed**:
```sql
-- test_fixtures.sql
INSERT INTO users (id, email, password_hash)
VALUES
  ('test-user-1', 'test1@example.com', '$2a$10$...'),
  ('test-user-2', 'test2@example.com', '$2a$10$...')
ON CONFLICT (email) DO NOTHING;
```

### Production Initial Data

```bash
# One-time production seed
nself db seed initial_config
```

**Seed**:
```sql
-- initial_config.sql
INSERT INTO app_settings (key, value)
VALUES
  ('maintenance_mode', 'false'),
  ('registration_enabled', 'true'),
  ('api_rate_limit', '1000')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;
```

## Best Practices

### 1. Always Use ON CONFLICT

```sql
-- Good
INSERT INTO roles (name) VALUES ('admin')
ON CONFLICT (name) DO NOTHING;

-- Bad (fails on re-run)
INSERT INTO roles (name) VALUES ('admin');
```

### 2. Separate Concerns

```bash
# Good - one concern per file
01_users.sql
02_roles.sql
03_permissions.sql

# Avoid - mixed concerns
01_everything.sql
```

### 3. Use Descriptive Names

```bash
# Good
01_create_admin_user.sql
02_default_organization.sql

# Avoid
seed1.sql
data.sql
```

### 4. Document Dependencies

```sql
-- Seed: User Roles
-- Depends on: 01_users.sql, 02_roles.sql
-- Creates join table entries for user-role relationships
```

### 5. Safe for Production

```sql
-- Only seed if table is empty
INSERT INTO config (key, value)
SELECT 'app_name', 'MyApp'
WHERE NOT EXISTS (SELECT 1 FROM config WHERE key = 'app_name');
```

## Troubleshooting

### Seed fails with constraint violation

**Error**:
```
✗ ERROR: duplicate key value violates unique constraint
```

**Solution**: Add `ON CONFLICT` clause:
```sql
INSERT INTO users (email, name) VALUES (...)
ON CONFLICT (email) DO NOTHING;
```

### Foreign key violation

**Error**:
```
✗ ERROR: insert or update violates foreign key constraint
```

**Solution**: Ensure seeds run in correct order (dependencies first).

### Seed not found

**Error**:
```
✗ Seed file not found: users
```

**Solution**:
```bash
# List available seeds
nself db seed --list

# Use correct name
nself db seed 01_users
```

## Advanced Usage

### Custom Seed Directory

```bash
SEEDS_DIR=custom/path nself db seed
```

### Seed from URL

```bash
# Download and apply
curl https://example.com/seeds/demo.sql | nself db shell
```

### Conditional Seeding

```sql
-- Only in development
DO $$
BEGIN
  IF current_database() NOT LIKE '%prod%' THEN
    INSERT INTO users (...) VALUES (...);
  END IF;
END $$;
```

## Related Commands

- `nself db migrate` - Apply schema changes before seeding
- `nself db shell` - Inspect seeded data
- `nself db reset` - Reset before re-seeding
- `nself db backup` - Backup after seeding

## See Also

- [Database Management](../db/README.md)
- [nself db migrate](migrate.md)
- [Seeding Guide](../../guides/SEEDING.md)
