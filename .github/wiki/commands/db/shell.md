# nself db shell

**Category**: Database Commands

Open an interactive PostgreSQL shell (psql) connected to your nself database.

## Overview

Provides direct SQL access to your PostgreSQL database for querying, administration, and debugging.

**Features**:
- ✅ Automatic connection to project database
- ✅ psql with full features (history, tab completion)
- ✅ Hot execution (no restart needed)
- ✅ Environment-aware (connects to correct database per ENV)
- ✅ Run SQL files directly

## Usage

```bash
nself db shell [OPTIONS] [SQL_FILE]
```

## Options

| Option | Description |
|--------|-------------|
| `-c, --command SQL` | Execute single SQL command |
| `-f, --file FILE` | Execute SQL from file |
| `--readonly` | Connect as read-only user |
| `-v, --verbose` | Show detailed output |

## Arguments

| Argument | Description |
|----------|-------------|
| `SQL_FILE` | Optional: SQL file to execute |

## Examples

### Interactive Shell

```bash
nself db shell
```

**Output**:
```
psql (15.3)
Type "help" for help.

myapp_db=# SELECT * FROM users LIMIT 5;
 id |    email           | created_at
----+--------------------+------------
  1 | user1@example.com  | 2026-02-01
  2 | user2@example.com  | 2026-02-02
(2 rows)

myapp_db=# \dt
         List of relations
 Schema |     Name     | Type  |  Owner
--------+--------------+-------+---------
 public | users        | table | postgres
 public | roles        | table | postgres
 public | permissions  | table | postgres
(3 rows)

myapp_db=# \q
```

### Execute Single Command

```bash
nself db shell -c "SELECT COUNT(*) FROM users;"
```

**Output**:
```
 count
-------
   142
(1 row)
```

### Execute SQL File

```bash
nself db shell -f scripts/analytics.sql
```

**Output**:
```
→ Executing: scripts/analytics.sql
 user_count | order_count | revenue
------------+-------------+---------
        142 |        1523 | 45328.50
(1 row)

✓ Script executed successfully
```

### Read-Only Access

```bash
nself db shell --readonly
```

**Use when**:
- Querying production data
- Preventing accidental modifications
- Running analytics queries

## Common psql Commands

### Meta Commands

```sql
-- List all tables
\dt

-- Describe table structure
\d users

-- List all schemas
\dn

-- List all databases
\l

-- List all roles
\du

-- Show current connection info
\conninfo

-- List all indexes
\di

-- Execute SQL from file
\i path/to/file.sql

-- Toggle timing
\timing

-- Quit
\q
```

### Useful Queries

```sql
-- Table sizes
SELECT
  schemaname,
  tablename,
  pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename))
FROM pg_tables
WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;

-- Active connections
SELECT
  pid,
  usename,
  application_name,
  client_addr,
  state,
  query
FROM pg_stat_activity
WHERE datname = current_database();

-- Database size
SELECT pg_size_pretty(pg_database_size(current_database()));

-- Recent queries
SELECT
  query,
  calls,
  total_time,
  mean_time
FROM pg_stat_statements
ORDER BY total_time DESC
LIMIT 10;
```

## Running Complex Queries

### Multi-line Queries

```sql
myapp_db=# SELECT
myapp_db-#   users.email,
myapp_db-#   COUNT(orders.id) as order_count
myapp_db-# FROM users
myapp_db-# LEFT JOIN orders ON users.id = orders.user_id
myapp_db-# GROUP BY users.email
myapp_db-# HAVING COUNT(orders.id) > 10;
```

### Transactions

```sql
BEGIN;

UPDATE users SET status = 'active' WHERE id = 123;
UPDATE user_stats SET last_login = NOW() WHERE user_id = 123;

COMMIT;
-- Or ROLLBACK; to undo
```

### Import/Export

```sql
-- Export to CSV
\copy (SELECT * FROM users) TO '/tmp/users.csv' CSV HEADER;

-- Import from CSV
\copy users FROM '/tmp/users.csv' CSV HEADER;

-- Export with query
\copy (SELECT * FROM users WHERE created_at > '2026-01-01') TO '/tmp/recent_users.csv' CSV HEADER;
```

## Database Introspection

### Table Information

```bash
nself db shell -c "\d+ users"
```

**Shows**:
- Column names and types
- Constraints (PRIMARY KEY, FOREIGN KEY, UNIQUE)
- Indexes
- Triggers
- Table size

### Constraint Information

```bash
nself db shell -c "SELECT conname, contype FROM pg_constraint WHERE conrelid = 'users'::regclass;"
```

### Index Usage Statistics

```bash
nself db shell -c "SELECT schemaname, tablename, indexname, idx_scan FROM pg_stat_user_indexes ORDER BY idx_scan DESC;"
```

## Troubleshooting

### Connection refused

**Error**:
```
psql: error: connection to server failed: Connection refused
```

**Solutions**:
```bash
# Check if PostgreSQL is running
nself status postgres

# If not running
nself start postgres

# Check port
grep POSTGRES_PORT .env
```

### Permission denied

**Error**:
```
ERROR: permission denied for table users
```

**Solutions**:
```bash
# Check user permissions
nself db shell -c "\du"

# Grant permissions (as admin)
nself db shell -c "GRANT ALL PRIVILEGES ON TABLE users TO myuser;"
```

### Database does not exist

**Error**:
```
FATAL: database "myapp_db" does not exist
```

**Solutions**:
```bash
# Check database name in .env
grep POSTGRES_DB .env

# Create database if missing
nself db shell -c "CREATE DATABASE myapp_db;"

# Or reset and reinitialize
nself db reset
nself db migrate up
```

### Query timeout

**Error**:
```
ERROR: canceling statement due to statement timeout
```

**Solutions**:
```sql
-- Increase timeout for session
SET statement_timeout = '60s';

-- Or globally (requires restart)
ALTER DATABASE myapp_db SET statement_timeout = '60s';
```

## Environment-Specific Shell

### Development

```bash
nself db shell
# Connects to dev database (from .env.dev or .env.local)
```

### Staging

```bash
nself env switch staging
nself db shell
# Connects to staging database
```

### Production

```bash
nself env switch prod
nself db shell --readonly
# Read-only recommended for production
```

## Automation and Scripts

### Automated Reporting

```bash
# Daily user report
nself db shell -f reports/daily-users.sql > /tmp/daily-report.txt

# Email report
nself db shell -f reports/weekly-stats.sql | mail -s "Weekly Stats" admin@example.com
```

### Health Checks

```bash
# Check if database is accessible
if nself db shell -c "SELECT 1" > /dev/null 2>&1; then
  echo "Database OK"
else
  echo "Database ERROR"
  exit 1
fi
```

### Data Validation

```bash
# Verify migrations
nself db shell -c "SELECT version FROM schema_migrations ORDER BY version;"

# Check for data integrity issues
nself db shell -f scripts/integrity-check.sql
```

## Safety Tips

### 1. Always Use Transactions for Updates

```sql
BEGIN;
-- Your UPDATE/DELETE statements
-- Verify changes with SELECT
COMMIT;  -- Or ROLLBACK if wrong
```

### 2. Test Queries with LIMIT First

```sql
-- Test with small dataset first
SELECT * FROM users WHERE status = 'inactive' LIMIT 5;

-- Then run full query
DELETE FROM users WHERE status = 'inactive';
```

### 3. Backup Before Bulk Changes

```bash
# Backup before major changes
nself db backup before-bulk-update.sql

# Make changes
nself db shell -f bulk-update.sql

# If problems, restore
nself db restore backups/before-bulk-update.sql
```

### 4. Use EXPLAIN for Performance

```sql
EXPLAIN ANALYZE
SELECT * FROM users
WHERE email LIKE '%@example.com'
ORDER BY created_at DESC
LIMIT 100;
```

## Related Commands

- `nself db migrate` - Apply schema migrations
- `nself db seed` - Load seed data
- `nself db backup` - Backup database before queries
- `nself db restore` - Restore if something goes wrong

## See Also

- [Database Management](README.md)
- [Migration Guide](../../guides/MIGRATIONS.md)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [psql Guide](https://www.postgresql.org/docs/current/app-psql.html)
