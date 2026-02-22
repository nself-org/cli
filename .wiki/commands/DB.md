# nself db

Comprehensive database management for nself projects. All database operations in one clean interface with smart defaults.

## Usage

```bash
nself db <subcommand> [OPTIONS]
```

## Subcommands

| Command | Description |
|---------|-------------|
| `migrate` | Database migrations (up, down, create, status) |
| `seed` | Environment-aware data seeding |
| `mock` | Deterministic mock data generation |
| `backup` | Backup management and scheduling |
| `restore` | Restore from backups |
| `schema` | Schema tools (diff, diagram, indexes) |
| `types` | Generate TypeScript/Go/Python types from schema |
| `shell` | Interactive PostgreSQL shell |
| `query` | Execute SQL queries |
| `inspect` | Database inspection and analysis |
| `data` | Data export/import/anonymize |
| `optimize` | Database maintenance (vacuum, analyze) |
| `reset` | Reset database to clean state |
| `status` | Quick database status overview |
| `hasura` | Hasura console and metadata management |

---

## Migrations

Manage database schema changes with versioned migrations.

### Commands

```bash
# Show migration status
nself db migrate status

# Run all pending migrations
nself db migrate up

# Run specific number of migrations
nself db migrate up 3

# Rollback last migration
nself db migrate down

# Rollback specific number
nself db migrate down 2

# Create new migration
nself db migrate create add_user_preferences

# Fresh: Drop all and re-run (NON-PRODUCTION ONLY)
nself db migrate fresh

# Repair migration tracking table
nself db migrate repair
```

### Migration Files

Migrations are stored in `nself/migrations/`:

```
nself/migrations/
├── 001_create_users.sql
├── 002_add_preferences.sql
└── 003_create_orders.sql
```

### Migration File Format

```sql
-- Migration: 001_create_users
-- Created: 2026-01-22

-- UP
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT UNIQUE NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- DOWN
DROP TABLE IF EXISTS users;
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NSELF_MIGRATIONS_DIR` | `nself/migrations` | Migrations directory |

---

## Seeding

Environment-aware data seeding with special handling for user accounts.

### Commands

```bash
# Run all seeds for current environment
nself db seed

# Run common seeds only
nself db seed common

# Run environment-specific seeds
nself db seed env

# Seed users (environment-aware)
nself db seed users

# Create new seed file
nself db seed create products

# Show seed status
nself db seed status
```

### Seed Directory Structure

```
nself/seeds/
├── common/              # Always runs first
│   ├── 01_categories.sql
│   └── 02_settings.sql
├── local/               # Development only
│   ├── 01_test_data.sql
│   └── 02_mock_users.sql
├── staging/             # Staging only
│   └── 01_qa_data.sql
└── production/          # Production only
    └── 01_admin_users.sql
```

### User Seeding by Environment

**Local/Development:**
- Generates 20 mock users by default
- Simple passwords ("password123")
- Test accounts: user@test.local, admin@test.local

**Staging:**
- Generates 100 mock users for load testing
- Stronger test passwords ("TestUser123!")
- QA accounts for testing

**Production:**
- **NO mock users** - only explicit configuration
- Reads from `NSELF_PROD_USERS` or `nself/config/prod-users.json`
- Generates secure random passwords

### Production User Configuration

Environment variable:
```bash
NSELF_PROD_USERS='admin@company.com:Admin User:admin,support@company.com:Support Team:moderator'
```

Or config file (`nself/config/prod-users.json`):
```json
{
  "users": [
    {
      "email": "admin@company.com",
      "display_name": "Admin User",
      "role": "admin"
    },
    {
      "email": "support@company.com",
      "display_name": "Support Team",
      "role": "moderator"
    }
  ]
}
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NSELF_SEEDS_DIR` | `nself/seeds` | Seeds directory |
| `NSELF_MOCK_USER_COUNT` | `20` (local), `100` (staging) | Mock users to create |
| `NSELF_PROD_USERS` | - | Production users (email:name:role,...) |

---

## Mock Data

Generate deterministic, shareable mock data for development and testing.

### Commands

```bash
# Auto-generate mock data from schema (recommended)
nself db mock auto

# Generate mock data with default settings
nself db mock

# Generate with specific seed (reproducible)
nself db mock --seed 12345

# Generate with row count
nself db mock --count 1000

# Preview what would be generated
nself db mock preview

# Clear all mock data
nself db mock clear

# Show mock configuration
nself db mock config
```

### Auto-Generation (Schema-Aware)

The `mock auto` command analyzes your database schema and generates appropriate mock data:

```bash
nself db mock auto
```

It automatically:
- Detects column types (generates appropriate data)
- Handles email columns → fake emails
- Handles name columns → fake names
- Handles URL columns → fake URLs
- Handles timestamps → random dates
- Uses deterministic seed (reproducible across team)

### Features

- **Deterministic**: Same seed produces same data every time
- **Shareable**: Team members can reproduce exact data sets
- **Schema-aware**: Respects foreign keys and constraints
- **Configurable**: Control row counts per table

### Configuration File

Create `nself/mock/config.json`:

```json
{
  "seed": 12345,
  "tables": {
    "users": {
      "count": 100,
      "exclude_columns": ["password_hash"]
    },
    "orders": {
      "count": 500
    },
    "products": {
      "count": 50
    }
  },
  "exclude_tables": ["schema_migrations", "audit_logs"]
}
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NSELF_MOCK_SEED` | Random | Seed for deterministic generation |
| `NSELF_MOCK_COUNT` | `100` | Default row count per table |
| `NSELF_MOCK_DIR` | `nself/mock` | Mock configuration directory |

---

## Backup

Create and manage database backups with scheduling support.

### Commands

```bash
# Create backup
nself db backup

# Create backup with custom name
nself db backup --name pre-migration

# List all backups
nself db backup list

# Create data-only backup (no schema)
nself db backup --data-only

# Create schema-only backup
nself db backup --schema-only

# Compressed backup
nself db backup --compress

# Schedule automated backups
nself db backup schedule

# Prune old backups (keep last N)
nself db backup prune 10
```

### Backup Types

| Type | Contents | Use Case |
|------|----------|----------|
| `full` | Schema + data | Complete restoration |
| `data` | Data only | Preserve schema, restore data |
| `schema` | Schema only | Structure backup |

### Backup Location

Backups are stored in `_backups/`:

```
_backups/
├── nself_full_20260122_143000.sql
├── nself_full_20260122_143000.sql.gz
└── nself_data_20260121_120000.sql
```

### Scheduling

The schedule command creates a cron job for automated backups:

```bash
# Daily backups at 2 AM
nself db backup schedule --daily

# Weekly backups on Sunday
nself db backup schedule --weekly

# Custom schedule
nself db backup schedule --cron "0 2 * * *"
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NSELF_BACKUPS_DIR` | `_backups` | Backup storage directory |
| `NSELF_BACKUP_COMPRESS` | `true` | Compress backups by default |
| `NSELF_BACKUP_RETENTION` | `30` | Days to keep backups |

---

## Restore

Restore database from backups with safety guards.

### Commands

```bash
# Restore from latest backup
nself db restore

# Restore from specific backup
nself db restore nself_full_20260122_143000.sql

# List available backups
nself db restore --list

# Restore from URL
nself db restore https://backups.example.com/latest.sql.gz

# Restore to different database
nself db restore backup.sql --database test_db
```

### Safety Features

- **Production Protection**: Requires explicit confirmation
- **Staging Warning**: Prompts before restore
- **Local**: Restores without confirmation

### Cross-Environment Restore

```bash
# Restore production backup to staging (with anonymization)
ENV=staging nself db restore prod_backup.sql --anonymize

# Restore to local development
ENV=local nself db restore staging_backup.sql
```

---

## Schema Tools

Design, import, and manage database schemas with full DBML support.

### Quick Start (Recommended Workflow)

```bash
# 1. Create a starter schema from template
nself db schema scaffold basic    # Also: ecommerce, saas, blog

# 2. Edit schema.dbml (or use dbdiagram.io to design)
# 3. Apply everything in one command:
nself db schema apply schema.dbml   # Import → migrate → seed
```

### Commands

```bash
# Design & Import
nself db schema scaffold <template>  # Create starter schema (basic, ecommerce, saas, blog)
nself db schema import <file.dbml>   # Convert DBML to SQL migration
nself db schema apply <file.dbml>    # Full workflow: import → migrate → seed

# Inspect & Export
nself db schema                      # Show current schema
nself db schema show users           # Show specific table schema
nself db schema diff staging         # Compare schema between environments
nself db schema diagram              # Generate DBML from database (reverse engineer)
nself db schema export > schema.sql  # Export schema to SQL file

# Indexes
nself db schema indexes              # Analyze and suggest indexes
```

### Schema Diff

Compare your local schema with another environment:

```bash
nself db schema diff staging
```

Output:
```
Schema Differences (local vs staging)
────────────────────────────────────
+ Table: user_preferences (missing in staging)
~ Column: users.last_login (different type)
- Index: idx_orders_date (missing in local)
```

### DBML Import (Design → Database)

Design your schema visually at [dbdiagram.io](https://dbdiagram.io), then import:

```bash
# Import DBML and create SQL migration
nself db schema import schema.dbml
```

This creates:
- `nself/migrations/20260122_imported_schema.up.sql` - SQL to create tables
- `nself/migrations/20260122_imported_schema.down.sql` - SQL to rollback

### DBML Export (Database → Design)

Generate DBML from your existing database:

```bash
nself db schema diagram > schema.dbml
```

Open the generated file at [dbdiagram.io](https://dbdiagram.io) to visualize.

### Schema Scaffold (Templates)

Start with pre-built schema templates:

```bash
nself db schema scaffold basic      # Users, profiles, posts
nself db schema scaffold ecommerce  # Products, orders, cart
nself db schema scaffold saas       # Organizations, members, projects
nself db schema scaffold blog       # Posts, categories, comments
```

Each template creates a `schema.dbml` file you can customize before importing.

### Full Workflow (One Command)

Apply a complete workflow from DBML to working database:

```bash
nself db schema apply schema.dbml
```

This automatically:
1. Imports DBML → creates SQL migration
2. Runs migration
3. Generates mock data (local/staging only)
4. Seeds sample users

### Index Advisor

Analyze queries and suggest missing indexes:

```bash
nself db schema indexes
```

Output:
```
Index Recommendations
─────────────────────
[HIGH] orders.user_id - Frequently joined, no index
[MEDIUM] products.category_id - Used in WHERE clauses
[LOW] users.created_at - Occasional range queries
```

---

## Type Generation

Generate typed interfaces from your database schema.

### Commands

```bash
# Generate TypeScript types
nself db types typescript

# Generate Go structs
nself db types go

# Generate Python dataclasses
nself db types python

# Generate to specific directory
nself db types typescript --output src/types/

# Include comments
nself db types typescript --comments
```

### TypeScript Output

```typescript
// Generated by nself db types
// DO NOT EDIT - regenerate with: nself db types typescript

export interface User {
  id: string;
  email: string;
  display_name: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface Order {
  id: string;
  user_id: string;
  total: number;
  status: 'pending' | 'completed' | 'cancelled';
  created_at: Date;
}
```

### Go Output

```go
// Generated by nself db types
// DO NOT EDIT - regenerate with: nself db types go

package models

import "time"

type User struct {
    ID          string    `json:"id" db:"id"`
    Email       string    `json:"email" db:"email"`
    DisplayName *string   `json:"display_name" db:"display_name"`
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type Order struct {
    ID        string    `json:"id" db:"id"`
    UserID    string    `json:"user_id" db:"user_id"`
    Total     float64   `json:"total" db:"total"`
    Status    string    `json:"status" db:"status"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
}
```

### Python Output

```python
# Generated by nself db types
# DO NOT EDIT - regenerate with: nself db types python

from dataclasses import dataclass
from datetime import datetime
from typing import Optional
from decimal import Decimal

@dataclass
class User:
    id: str
    email: str
    display_name: Optional[str]
    created_at: datetime
    updated_at: datetime

@dataclass
class Order:
    id: str
    user_id: str
    total: Decimal
    status: str
    created_at: datetime
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NSELF_TYPES_DIR` | `types` | Output directory for generated types |

---

## Interactive Shell

Access PostgreSQL directly for debugging and queries.

### Commands

```bash
# Open interactive psql shell
nself db shell

# Open read-only shell (safe for production)
nself db shell --readonly

# Execute single query
nself db query "SELECT * FROM users LIMIT 10"

# Execute query from file
nself db query -f query.sql

# Execute and output as JSON
nself db query "SELECT * FROM users" --json
```

---

## Inspection

Database analysis and performance insights (like Supabase inspect db).

### Commands

```bash
# Overview of all tables
nself db inspect

# Table sizes
nself db inspect size

# Cache hit ratios
nself db inspect cache

# Index usage analysis
nself db inspect index

# Find unused indexes
nself db inspect unused-indexes

# Table bloat analysis
nself db inspect bloat

# Slow query analysis
nself db inspect slow

# Current locks
nself db inspect locks

# Connection stats
nself db inspect connections
```

### Example Output

```
Database Overview: nhost
─────────────────────────────────
Tables: 15
Total Size: 256 MB
Connections: 12/100

Top Tables by Size:
  orders          128 MB  (50%)
  products         64 MB  (25%)
  users            32 MB  (12%)

Cache Hit Ratio: 98.5% (healthy)
Index Usage: 94.2% (good)
```

---

## Data Operations

Export, import, and anonymize data.

### Commands

```bash
# Export table as CSV
nself db data export users --format csv

# Export as JSON
nself db data export users --format json

# Export with WHERE clause
nself db data export orders --where "created_at > '2026-01-01'"

# Import data
nself db data import users.csv

# Anonymize PII data
nself db data anonymize

# Sync data from another environment (with anonymization)
nself db data sync staging --anonymize
```

### Anonymization

Automatically detects and anonymizes PII:

```bash
nself db data anonymize
```

Anonymized fields:
- Email addresses → fake emails
- Names → fake names
- Phone numbers → fake numbers
- IP addresses → random IPs
- Passwords → hashed placeholder

### Export Formats

| Format | Extension | Use Case |
|--------|-----------|----------|
| `csv` | `.csv` | Spreadsheets, data analysis |
| `json` | `.json` | API mocking, JavaScript |
| `sql` | `.sql` | Database restoration |

---

## Maintenance

Database optimization and cleanup.

### Commands

```bash
# Analyze and vacuum all tables
nself db optimize

# Vacuum specific table
nself db optimize users

# Full vacuum (reclaim disk space)
nself db optimize --full

# Reset database (NON-PRODUCTION ONLY)
nself db reset

# Reset with confirmation skip
nself db reset --force
```

### Optimization Schedule

For production, consider scheduling:

```bash
# Add to crontab: daily at 3 AM
0 3 * * * cd /path/to/project && nself db optimize
```

---

## Environment Awareness

All commands respect the current environment:

| Environment | Behavior |
|-------------|----------|
| `local` | Full access, no confirmations |
| `staging` | Warning prompts for destructive ops |
| `production` | Blocked/confirmed destructive ops |

Set environment:
```bash
# Via variable
ENV=production nself db migrate up

# Via .env file
# ENV=staging in .env
```

### Blocked in Production

These operations are blocked in production:
- `nself db migrate fresh`
- `nself db mock`
- `nself db reset`

### Require Confirmation in Production

These require typing `yes-destroy-production`:
- `nself db restore`
- `nself db migrate down`

---

## Quick Reference

```bash
# Quick Start (Recommended)
nself db schema scaffold basic      # Create starter schema
nself db schema apply schema.dbml   # Import → migrate → seed (all in one!)

# Migrations
nself db migrate status        # Check migration status
nself db migrate up            # Run pending migrations
nself db migrate down          # Rollback last migration
nself db migrate create NAME   # Create new migration

# Schema Design
nself db schema scaffold saas  # Create from template
nself db schema import file.dbml  # DBML → SQL migration
nself db schema diagram        # Database → DBML
nself db schema diff staging   # Compare schemas

# Seeding
nself db seed                  # Run all seeds
nself db seed users            # Seed users (env-aware)

# Mock Data
nself db mock auto             # Auto-generate from schema
nself db mock --seed 123       # Reproducible data

# Backup/Restore
nself db backup                # Create backup
nself db restore               # Restore latest

# Types
nself db types typescript      # Generate TS types

# Inspection
nself db inspect               # Database overview
nself db shell                 # Interactive psql

# Data
nself db data export users     # Export table
nself db data anonymize        # Anonymize PII
```

---

## Hasura

Manage Hasura Console and metadata. Hasura tracks schema, permissions, relationships, and computed fields as metadata. Keeping these alongside your database commands is a natural fit.

### Hasura Commands

```bash
# Open Hasura Console (with migration tracking)
nself db hasura console

# Apply local metadata to Hasura
nself db hasura metadata apply

# Export Hasura metadata to local files
nself db hasura metadata export

# Reload Hasura metadata cache (no restart)
nself db hasura metadata reload
```

### Prerequisites

**Hasura CLI** (optional — required for `console` and `metadata` commands):

```bash
npm install -g hasura-cli
hasura version
```

The `metadata reload` command uses `curl` and works without the Hasura CLI.

### Common Workflows

```bash
# After database schema change — sync Hasura
nself db migrate up
nself db hasura metadata reload

# Work with Hasura Console (changes tracked in metadata files)
nself db hasura console

# Export and commit metadata after console changes
nself db hasura metadata export
git add hasura/metadata/
git commit -m "Update Hasura permissions"

# Deploy metadata changes to another environment
nself db hasura metadata apply
```

### Hasura Environment Variables

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `HASURA_GRAPHQL_ENDPOINT` | `http://localhost:8080` | Hasura API endpoint |
| `HASURA_PORT` | `8080` | Hasura port |
| `HASURA_GRAPHQL_ADMIN_SECRET` | (required) | Admin secret |

> **Note:** `nself hasura` is a deprecated compatibility alias. Use `nself db hasura` going forward.

---

## See Also

- [ENV.md](ENV.md) - Environment management
- [DEPLOY.md](DEPLOY.md) - Deployment commands
- [PROD.md](PROD.md) - Production configuration
