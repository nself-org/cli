# nself db migrate

**Category**: Database Commands

Manage database migrations with version tracking and rollback support.

## Overview

Database migration management for schema changes without container restarts.

**Two Systems Available**:

### Hasura-Style (Default)
- Directory: `hasura/migrations/default/`
- Format: `123456_migration_name/up.sql` and `down.sql`
- Tracking: `schema_migrations` table

### Simple-Style (Alternative)
- Directory: `db/migrations/`
- Format: `20260213_120000_name.sql` with `-- UP:` and `-- DOWN:` sections
- Tracking: `_migrations` table

## Usage

```bash
nself db migrate <action> [options]
```

## Actions

### up
Apply pending migrations.

```bash
nself db migrate up
```

**Process**:
1. Creates `schema_migrations` table if needed
2. Finds pending migrations
3. Applies in timestamp order
4. Records in tracking table
5. Shows timing for each migration

**Example Output**:
```
⠋ Initializing migration tracking...
✓ Migration tracking ready

→ Checking for pending migrations...
  ⠋ Applying: 20260213_120000_add_user_roles (2s)
  ✓ Applied successfully

✓ 1 migration(s) applied
```

### down
Rollback last migration.

```bash
nself db migrate down
```

**Process**:
1. Finds last applied migration
2. Executes `down.sql` or `-- DOWN:` section
3. Removes from tracking table

**Safety**: Requires confirmation in production.

### status
Show migration status.

```bash
nself db migrate status
```

**Output**:
```
Migration Status:

Applied Migrations:
  ✓ 20260213_100000_create_users
  ✓ 20260213_120000_add_user_roles

Pending Migrations:
  ○ 20260213_140000_add_indexes

Total: 2 applied, 1 pending
```

### create
Create new migration file.

```bash
nself db migrate create <name>
```

**Example**:
```bash
nself db migrate create add_user_preferences
```

**Creates**:
- Hasura: `hasura/migrations/default/123456_add_user_preferences/up.sql` and `down.sql`
- Simple: `db/migrations/20260213_120000_add_user_preferences.sql`

## Migration File Formats

### Hasura-Style

**Directory structure**:
```
hasura/migrations/default/
└── 1644851234567_add_user_roles/
    ├── up.sql      # Apply migration
    └── down.sql    # Rollback migration
```

**up.sql**:
```sql
CREATE TABLE user_roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id),
  role VARCHAR(50) NOT NULL
);

CREATE INDEX idx_user_roles_user_id ON user_roles(user_id);
```

**down.sql**:
```sql
DROP TABLE user_roles;
```

### Simple-Style

**File**: `db/migrations/20260213_120000_add_user_roles.sql`

```sql
-- Migration: add_user_roles
-- Created: 2026-02-13

-- UP: Apply migration
CREATE TABLE user_roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id),
  role VARCHAR(50) NOT NULL
);

CREATE INDEX idx_user_roles_user_id ON user_roles(user_id);

-- DOWN: Rollback migration
DROP TABLE user_roles;
```

## Options

| Option | Description |
|--------|-------------|
| `-v, --verbose` | Show detailed output |
| `--dry-run` | Show what would be done |
| `--force` | Skip confirmation prompts |

## Best Practices

### Writing Migrations

1. **One change per migration**
   ```bash
   # Good
   nself db migrate create add_email_to_users
   nself db migrate create add_email_index

   # Avoid
   nself db migrate create add_email_and_index_and_constraints
   ```

2. **Always write DOWN migrations**
   ```sql
   -- UP
   ALTER TABLE users ADD COLUMN email VARCHAR(255);

   -- DOWN
   ALTER TABLE users DROP COLUMN email;
   ```

3. **Make migrations idempotent**
   ```sql
   -- UP
   CREATE TABLE IF NOT EXISTS user_settings (...);
   ALTER TABLE users ADD COLUMN IF NOT EXISTS email VARCHAR(255);

   -- DOWN
   DROP TABLE IF EXISTS user_settings;
   ALTER TABLE users DROP COLUMN IF EXISTS email;
   ```

4. **Use transactions**
   ```sql
   BEGIN;
   -- Your changes here
   COMMIT;
   ```

### Testing Migrations

```bash
# 1. Apply migration
nself db migrate up

# 2. Test your application

# 3. If issues, rollback
nself db migrate down

# 4. Fix migration, re-apply
nself db migrate up
```

### Production Workflow

```bash
# 1. Create migration in dev
nself db migrate create add_feature

# 2. Write migration SQL

# 3. Test locally
nself db migrate up

# 4. Commit to git

# 5. Deploy to staging
ssh staging "cd /app && nself db migrate up"

# 6. Verify on staging

# 7. Deploy to production
ssh production "cd /app && nself db migrate up"
```

## Troubleshooting

### Migration fails

**Error**:
```
✗ Migration failed: syntax error at or near "TABLE"
```

**Solution**:
1. Check SQL syntax
2. View detailed error: `nself db migrate up --verbose`
3. Fix SQL in migration file
4. Re-run: `nself db migrate up`

### Container not running

**Error**:
```
✗ Database container not running
```

**Solution**:
```bash
nself start
nself db migrate up
```

### Permission denied

**Error**:
```
✗ Permission denied for table schema_migrations
```

**Solution**: Check `POSTGRES_USER` has sufficient permissions.

## Advanced Usage

### Custom migration directory

```bash
MIGRATIONS_DIR=custom/path nself db migrate up
```

### Migrate specific version

```bash
# Migrate to specific version (requires custom script)
nself db migrate to 20260213_120000
```

## Related Commands

- `nself db seed` - Load seed data
- `nself db backup` - Backup before migrations
- `nself db shell` - Inspect database
- `nself db reset` - Reset database (dev only)

## See Also

- [Database Management](../db/README.md)
- [nself db seed](seed.md)
- [Migration Guide](../../guides/MIGRATIONS.md)
