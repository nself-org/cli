# nself db

Database management and utilities.

## Overview

All database operations use `nself db <subcommand>` for:
- Schema migrations
- Seed data
- Backups and restores
- Database shell access
- Development utilities

## Subcommands

| Command | Description | Use Case |
|---------|-------------|----------|
| [migrate](migrate.md) | Manage schema migrations | Schema changes, version control |
| [seed](seed.md) | Load seed data | Test data, initial data |
| [shell](shell.md) | PostgreSQL shell | Direct database access |
| [backup](backup.md) | Create backup | Before major changes |
| [restore](restore.md) | Restore from backup | Rollback, copy data |
| [reset](reset.md) | Reset database | Fresh start (dev only) |

## Quick Start

### First Time Setup

```bash
# 1. Start services
nself start

# 2. Apply migrations
nself db migrate up

# 3. Load seed data
nself db seed
```

### Daily Development

```bash
# Create new migration
nself db migrate create add_feature

# Edit migration file
# db/migrations/20260213_120000_add_feature.sql

# Apply migration
nself db migrate up

# Add test data
nself db seed
```

### Before Major Changes

```bash
# Backup current state
nself db backup before_refactor.sql

# Make changes
nself db migrate up

# If issues, restore
nself db restore backups/before_refactor.sql
```

## Common Workflows

### New Feature Development

```bash
# 1. Create migration for schema
nself db migrate create add_comments

# 2. Write migration SQL
# Add tables, columns, indexes

# 3. Apply migration
nself db migrate up

# 4. Create seed for test data
echo "INSERT INTO comments ..." > db/seeds/05_comments.sql

# 5. Load seed data
nself db seed comments

# 6. Develop feature

# 7. If schema needs changes
nself db migrate down  # Rollback
# Edit migration
nself db migrate up    # Re-apply
```

### Database Reset

```bash
# Fresh start (dev only)
nself db reset
nself db migrate up
nself db seed
```

### Production Deployment

```bash
# 1. Backup production
ssh prod "cd /app && nself db backup pre_deploy_$(date +%Y%m%d).sql"

# 2. Apply migrations
ssh prod "cd /app && nself db migrate up"

# 3. Verify
ssh prod "cd /app && nself db migrate status"
```

## Environment-Specific Behavior

### Development
- `nself db reset` allowed
- Seeds include test data
- Verbose output

### Staging
- `nself db reset` disabled
- Seeds include realistic data
- Requires confirmation

### Production
- `nself db reset` disabled
- Seeds only for initial config
- Extra confirmation required
- Automatic backups

## Configuration

### Environment Variables

```bash
# Database connection
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=myapp
POSTGRES_USER=postgres
POSTGRES_PASSWORD=secret

# Migration settings
MIGRATIONS_DIR=db/migrations
MIGRATIONS_TABLE=schema_migrations

# Seed settings
SEEDS_DIR=db/seeds
```

### Container Name

```bash
# Auto-detected from PROJECT_NAME
PROJECT_NAME=myapp  # Container: myapp_postgres

# Or use COMPOSE_PROJECT_NAME
COMPOSE_PROJECT_NAME=myapp  # Container: myapp_postgres
```

## Directory Structure

```
myproject/
├── hasura/
│   ├── migrations/
│   │   └── default/
│   │       ├── 1644851234567_create_users/
│   │       │   ├── up.sql
│   │       │   └── down.sql
│   │       └── 1644852000000_add_roles/
│   │           ├── up.sql
│   │           └── down.sql
│   └── seeds/
│       └── default/
│           ├── 01_users.sql
│           └── 02_roles.sql
├── db/
│   ├── migrations/
│   │   ├── 20260213_120000_create_users.sql
│   │   └── 20260213_140000_add_roles.sql
│   └── seeds/
│       ├── 01_users.sql
│       └── 02_roles.sql
└── backups/
    ├── backup_20260213_100000.sql
    └── backup_20260213_140000.sql
```

## Troubleshooting

### Connection Issues

```bash
# Check container is running
docker ps | grep postgres

# If not running
nself start

# Check connectivity
nself db shell
```

### Migration Issues

```bash
# Check status
nself db migrate status

# View detailed error
nself db migrate up --verbose

# Rollback if needed
nself db migrate down
```

### Permission Issues

```bash
# Check user permissions
nself db shell
\du  # List roles

# Grant permissions
GRANT ALL PRIVILEGES ON DATABASE mydb TO myuser;
```

## See Also

- [Migration Guide](../../guides/MIGRATIONS.md)
- [Seeding Guide](../../guides/SEEDING.md)
- [Database Configuration](../../configuration/DATABASE.md)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
