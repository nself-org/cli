# nself hasura

**Version**: v0.9.9  
**Status**: Active

## Overview

Hasura GraphQL management commands for metadata operations and console access. This command provides convenient wrappers around Hasura CLI operations with automatic configuration from your nself environment.

## Usage

```bash
nself hasura <subcommand> [options]
```

## Subcommands

| Subcommand | Description |
|------------|-------------|
| `metadata apply` | Apply metadata to Hasura |
| `metadata export` | Export metadata from Hasura |
| `metadata reload` | Reload metadata cache |
| `console` | Open Hasura Console |
| `help` | Show help information |

## Prerequisites

### Hasura CLI (Optional)

For `metadata apply`, `metadata export`, and `console` commands with CLI features:

```bash
# Install globally
npm install -g hasura-cli

# Verify installation
hasura version
```

**Note:** The `metadata reload` command uses curl and doesn't require Hasura CLI.

## Commands

### metadata apply

Apply local metadata to Hasura instance.

```bash
nself hasura metadata apply
```

**Requirements:**
- Hasura CLI installed
- `HASURA_GRAPHQL_ADMIN_SECRET` set in environment
- Metadata files in `hasura/metadata/` directory

**What it does:**
1. Reads `HASURA_GRAPHQL_ADMIN_SECRET` from environment
2. Determines Hasura endpoint from config
3. Applies metadata from local directory

**Example:**
```bash
nself hasura metadata apply
```

### metadata export

Export metadata from Hasura instance to local files.

```bash
nself hasura metadata export
```

**Requirements:**
- Hasura CLI installed
- `HASURA_GRAPHQL_ADMIN_SECRET` set in environment

**What it does:**
1. Connects to Hasura instance
2. Exports all metadata (tables, relationships, permissions, etc.)
3. Saves to `hasura/metadata/` directory

**Example:**
```bash
nself hasura metadata export
```

### metadata reload

Reload Hasura metadata cache without restarting.

```bash
nself hasura metadata reload
```

**Requirements:**
- `HASURA_GRAPHQL_ADMIN_SECRET` set in environment
- Hasura service running

**What it does:**
1. Sends API request to Hasura
2. Triggers metadata cache reload
3. No service restart required

**Example:**
```bash
nself hasura metadata reload
```

**Use cases:**
- After database schema changes
- After modifying tracked tables
- To refresh stale metadata cache

### console

Open Hasura Console in browser.

```bash
nself hasura console
```

**With Hasura CLI:**
- Opens Hasura CLI console (with migration tracking)
- Automatically authenticates with admin secret
- Changes are tracked in metadata files

**Without Hasura CLI:**
- Prints URL to web console
- Manual navigation required

**Examples:**
```bash
# Open console
nself hasura console

# Or access directly at
# http://localhost:8080/console
```

## Configuration

### Environment Variables

The command automatically reads:

| Variable | Description | Default |
|----------|-------------|---------|
| `HASURA_GRAPHQL_ENDPOINT` | Hasura API endpoint | `http://localhost:8080` |
| `HASURA_PORT` | Hasura port | `8080` |
| `HASURA_GRAPHQL_ADMIN_SECRET` | Admin secret for authentication | (required) |

### Example .env

```bash
HASURA_PORT=8080
HASURA_GRAPHQL_ADMIN_SECRET=your-secret-key-here
HASURA_GRAPHQL_ENDPOINT=http://localhost:8080
```

## Examples

### Complete Metadata Workflow

```bash
# 1. Make changes in Hasura Console
nself hasura console

# 2. Export metadata after changes
nself hasura metadata export

# 3. Commit metadata to git
git add hasura/metadata/
git commit -m "Add users table and permissions"

# 4. Apply to another environment
nself hasura metadata apply
```

### Reload After Schema Change

```bash
# After running database migration
nself db migrate

# Reload Hasura metadata
nself hasura metadata reload
```

### Troubleshoot Metadata Issues

```bash
# Export current state
nself hasura metadata export

# Clear and reapply
nself hasura metadata apply
```

## Metadata Directory Structure

```
hasura/
└── metadata/
    ├── actions.graphql
    ├── actions.yaml
    ├── allow_list.yaml
    ├── cron_triggers.yaml
    ├── databases/
    │   └── default/
    │       └── tables/
    │           ├── public_users.yaml
    │           └── public_posts.yaml
    ├── query_collections.yaml
    ├── remote_schemas.yaml
    └── version.yaml
```

## Notes

- Metadata commands read from cascading environment files
- Admin secret is required for all operations
- Console access requires Hasura service to be running
- Metadata files should be version controlled
- Always test metadata changes in staging first

## Troubleshooting

### Admin Secret Not Set

```bash
# Error: HASURA_GRAPHQL_ADMIN_SECRET not set

# Solution: Set in environment
echo "HASURA_GRAPHQL_ADMIN_SECRET=your-secret" >> .env
nself restart hasura
```

### Hasura CLI Not Found

```bash
# Error: Hasura CLI not found

# Solution: Install CLI
npm install -g hasura-cli

# Or access console directly
nself urls | grep hasura
# Open console URL in browser
```

### Connection Failed

```bash
# Check Hasura is running
nself status hasura

# Check endpoint
nself config env | grep HASURA

# Restart service
nself restart hasura
```

### Metadata Apply Fails

```bash
# Export current metadata first
nself hasura metadata export

# Check for conflicts
diff -r hasura/metadata/ /path/to/backup/

# Apply with verbose output
hasura metadata apply --endpoint http://localhost:8080 --admin-secret $HASURA_GRAPHQL_ADMIN_SECRET
```

## See Also

- [db](../commands/DB.md) - Database operations
- [status](../commands/STATUS.md) - Service status
- [logs](../commands/LOGS.md) - View logs
- [urls](../commands/URLS.md) - Service URLs

---

**Documentation**: https://hasura.io/docs/latest/graphql/core/migrations/index.html  
**Category**: Core Services
