# Database Workflow Guide

Complete guide to the nself database workflow: from blank folder to production.

## The Zero-Config Experience

nself aims for **3 commands to working local, 5 to production**:

```bash
# Local Development (3 commands)
mkdir myapp && cd myapp
nself init
nself build
nself start

# To Production (add 2 more)
nself env create prod production
nself deploy prod
```

---

## Schema-First Workflow (Recommended)

For most projects, start by designing your database schema:

### Step 1: Scaffold Your Schema

```bash
# Create a project
nself init
nself build

# Create a starter schema (choose a template)
nself db schema scaffold basic      # Users, profiles, posts
nself db schema scaffold ecommerce  # Products, orders, cart
nself db schema scaffold saas       # Organizations, members, projects
nself db schema scaffold blog       # Posts, categories, comments
```

This creates `schema.dbml` - a human-readable schema definition.

### Step 2: Customize Your Schema

Edit `schema.dbml` or use [dbdiagram.io](https://dbdiagram.io) to design visually:

```dbml
// schema.dbml
Table users {
  id serial [pk]
  email varchar(255) [not null, unique]
  display_name varchar(100)
  role varchar(20) [default: 'user']
  created_at timestamptz [default: `NOW()`]
}

Table posts {
  id serial [pk]
  user_id integer [not null]
  title varchar(255) [not null]
  content text
  published boolean [default: false]
  created_at timestamptz [default: `NOW()`]
}

Ref: posts.user_id > users.id
```

### Step 3: Apply Everything (One Command!)

```bash
nself db schema apply schema.dbml
```

This single command:
1. **Imports DBML** → Creates SQL migration files
2. **Runs migrations** → Creates tables in database
3. **Generates mock data** → Populates tables with test data
4. **Seeds users** → Creates sample accounts you can log in with

### Step 4: Start Building!

Your database is ready with:
- Schema from your DBML
- Mock data for testing
- Sample users to log in:
  - `admin@example.com` (admin role)
  - `user@example.com` (user role)
  - `demo@example.com` (viewer role)

---

## Step-by-Step Workflow (Manual Control)

If you need more control over each step:

### 1. Design Schema

```bash
# Option A: Use a template
nself db schema scaffold saas

# Option B: Design at dbdiagram.io, download DBML

# Option C: Create schema.dbml manually
```

### 2. Import DBML to SQL

```bash
nself db schema import schema.dbml
```

Creates migration files:
```
nself/migrations/
├── 20260122143000_imported_schema.up.sql
└── 20260122143000_imported_schema.down.sql
```

### 3. Review and Run Migrations

```bash
# Review the generated SQL
cat nself/migrations/*_imported_schema.up.sql

# Run migrations
nself db migrate up

# Check status
nself db migrate status
```

### 4. Generate Mock Data

```bash
# Auto-generate based on schema
nself db mock auto

# Or with specific seed (reproducible)
nself db mock auto --seed 12345
```

### 5. Seed Users

```bash
# Environment-aware user seeding
nself db seed users
```

---

## Environment-Aware Behavior

All database commands adapt to your environment:

### Local Development (`ENV=local`)

```bash
# Everything enabled
nself db mock auto          # ✅ Generates mock data
nself db seed users         # ✅ Creates test accounts
nself db migrate fresh      # ✅ Allowed
nself db reset              # ✅ Allowed
```

Sample users created:
- 20 mock users with simple passwords
- Test accounts: `admin@example.com`, `user@example.com`, `demo@example.com`

### Staging (`ENV=staging`)

```bash
# Most operations allowed with warnings
nself db mock auto          # ✅ Generates mock data
nself db seed users         # ✅ Creates QA accounts
nself db migrate fresh      # ⚠️ Requires confirmation
nself db reset              # ⚠️ Requires confirmation
```

Sample users created:
- 100 mock users for load testing
- QA accounts with stronger test passwords

### Production (`ENV=production`)

```bash
# Destructive operations blocked
nself db mock auto          # ❌ Blocked
nself db migrate fresh      # ❌ Blocked
nself db reset              # ❌ Blocked

# Safe operations allowed
nself db migrate up         # ✅ Allowed
nself db seed users         # ✅ Only creates explicit users
nself db backup             # ✅ Allowed
```

Production users come from explicit configuration:
```bash
# Environment variable
NSELF_PROD_USERS='admin@company.com:Admin:admin,support@company.com:Support:moderator'
```

---

## DBML Format Reference

DBML (Database Markup Language) is a readable format for database schemas:

### Basic Syntax

```dbml
Table table_name {
  column_name type [constraints]
}
```

### Column Types

| DBML Type | PostgreSQL |
|-----------|------------|
| `int`, `integer` | INTEGER |
| `bigint` | BIGINT |
| `serial` | SERIAL |
| `varchar(n)` | VARCHAR(n) |
| `text` | TEXT |
| `boolean` | BOOLEAN |
| `timestamp`, `timestamptz` | TIMESTAMPTZ |
| `date` | DATE |
| `uuid` | UUID |
| `json`, `jsonb` | JSONB |
| `decimal(p,s)` | DECIMAL(p,s) |

### Constraints

```dbml
Table users {
  id serial [pk]                          // Primary key
  email varchar(255) [not null, unique]   // Not null + unique
  role varchar(20) [default: 'user']      // Default value
  created_at timestamptz [default: `NOW()`]  // SQL default
}
```

### Relationships

```dbml
// One-to-many
Ref: posts.user_id > users.id

// Many-to-many (via junction table)
Ref: post_tags.post_id > posts.id
Ref: post_tags.tag_id > tags.id
```

---

## Schema Templates

### Basic Template

Users, profiles, and posts - good for social apps, blogs, forums.

```bash
nself db schema scaffold basic
```

Tables: `users`, `profiles`, `posts`

### E-commerce Template

Products, orders, and cart - good for online stores.

```bash
nself db schema scaffold ecommerce
```

Tables: `users`, `products`, `categories`, `orders`, `order_items`, `cart_items`

### SaaS Template

Organizations and teams - good for B2B applications.

```bash
nself db schema scaffold saas
```

Tables: `organizations`, `users`, `organization_members`, `invitations`, `projects`, `api_keys`

### Blog Template

Content management - good for blogs, CMS, documentation.

```bash
nself db schema scaffold blog
```

Tables: `users`, `posts`, `categories`, `tags`, `post_categories`, `post_tags`, `comments`, `media`

---

## Modifying Your Schema

### Adding a New Table

1. Edit `schema.dbml`:
```dbml
Table comments {
  id serial [pk]
  post_id integer [not null]
  content text [not null]
  created_at timestamptz [default: `NOW()`]
}

Ref: comments.post_id > posts.id
```

2. Create migration from changes:
```bash
nself db schema import schema.dbml --name add_comments
```

3. Run migration:
```bash
nself db migrate up
```

### Modifying an Existing Table

For modifications, create a manual migration:

```bash
nself db migrate create add_user_avatar
```

Edit the generated file:
```sql
-- UP
ALTER TABLE users ADD COLUMN avatar_url TEXT;

-- DOWN
ALTER TABLE users DROP COLUMN avatar_url;
```

Run it:
```bash
nself db migrate up
```

---

## Reverse Engineering (Database → DBML)

Export your existing database to DBML:

```bash
nself db schema diagram > current-schema.dbml
```

This is useful for:
- Documenting existing databases
- Visualizing structure at dbdiagram.io
- Migrating to DBML workflow

---

## Complete Example: New Project

```bash
# 1. Create and initialize
mkdir my-saas-app && cd my-saas-app
nself init
nself build

# 2. Start services
nself start

# 3. Create and apply schema
nself db schema scaffold saas
# Edit schema.dbml if needed
nself db schema apply schema.dbml

# 4. Verify
nself db inspect              # Database overview
nself urls                    # Get API URLs

# 5. Generate types for your frontend
nself db types typescript     # Creates types/db.ts
```

Your app now has:
- Running services (Hasura, Auth, etc.)
- Database with your schema
- Mock data for testing
- Sample users to authenticate with
- TypeScript types for your frontend

---

## Complete Example: To Production

```bash
# After local development is working...

# 1. Create production environment
nself env create prod production

# 2. Configure production users
export NSELF_PROD_USERS='admin@company.com:Admin:admin'

# 3. Configure server (edit .environments/prod/server.json)
# {
#   "host": "your-server.example.com",
#   "user": "root",
#   "key": "~/.ssh/id_ed25519"
# }

# 4. Deploy
nself deploy prod

# 5. Run migrations on production
ENV=production nself db migrate up

# 6. Seed production users only
ENV=production nself db seed users
```

---

## Troubleshooting

### DBML Import Errors

**Problem**: Import fails with parse error

**Solution**: Check DBML syntax:
- Column names must be valid identifiers
- Types must be supported
- Brackets must be balanced

```bash
# Validate at dbdiagram.io before importing
```

### Mock Data Not Generating

**Problem**: `nself db mock auto` creates no data

**Solution**: Ensure tables exist:
```bash
nself db migrate status  # Check migrations ran
nself db schema show     # Verify tables exist
```

### Migration Conflicts

**Problem**: Migration already applied error

**Solution**: Check status and repair if needed:
```bash
nself db migrate status
nself db migrate repair   # Fix tracking table
```

---

## Best Practices

1. **Design schema first** - Use dbdiagram.io to visualize before coding
2. **Use templates** - Start from a scaffold, customize from there
3. **Keep DBML in sync** - Re-export after manual migrations
4. **Use seeds for required data** - Reference data, admin users
5. **Use mock for test data** - Generated, reproducible, disposable
6. **Always test migrations** - Run locally before staging/production
7. **Backup before migrate** - `nself db backup && nself db migrate up`

---

## See Also

- [DB.md](../commands/DB.md) - Complete command reference
- [ENV.md](../commands/ENV.md) - Environment management
- [DEPLOY.md](../commands/DEPLOY.md) - Deployment guide
