# nself dev frontend - Frontend Application Management

> **⚠️ DEPRECATED in v0.9.6**: This command has been consolidated.
> Please use `nself dev frontend` instead.
> See [Command Consolidation Map](../architecture/COMMAND-CONSOLIDATION-MAP.md) and [v0.9.6 Release Notes](../releases/v0.9.6.md) for details.

**Version 0.4.6** | Frontend deployment and tracking

---

## Overview

The `nself dev frontend` command manages frontend applications integrated with your nself backend. Track deployments, manage environment variables, and integrate with deployment providers like Vercel and Netlify.

---

## Usage

```bash
nself dev frontend <subcommand> [options]
```

---

## Subcommands

### `status` (default)

Show frontend deployment status.

```bash
nself dev frontend                  # Show all frontends
nself dev frontend status           # Same as above
```

### `list`

List configured frontend apps.

```bash
nself dev frontend list             # List names
nself dev frontend list --json      # JSON output
```

### `add <name>`

Add a new frontend application.

```bash
nself dev frontend add webapp --port 3000
nself dev frontend add admin --port 3001 --route admin-ui
```

### `remove <name>`

Remove a frontend application.

```bash
nself dev frontend remove webapp
```

### `deploy <name>`

Deploy frontend (Vercel/Netlify integration).

```bash
nself dev frontend deploy webapp              # Deploy to staging
nself dev frontend deploy webapp --env prod   # Deploy to production
```

### `logs <name>`

View frontend build/deploy logs.

```bash
nself dev frontend logs webapp           # Recent logs
nself dev frontend logs webapp --limit 50  # Last 50 entries
```

### `env <name>`

Show environment variables for frontend.

```bash
nself dev frontend env webapp           # Show env vars
nself dev frontend env webapp --json    # JSON output
```

---

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `--port N` | Frontend port | 3000 |
| `--route PATH` | Route prefix | app name |
| `--provider NAME` | Deployment provider | auto-detect |
| `--env NAME` | Target environment | production |
| `--limit N` | Log entries to show | 20 |
| `--json` | Output in JSON format | false |
| `-h, --help` | Show help message | - |

---

## Configuration

Frontends are configured in `.env`:

```bash
# Frontend Apps
FRONTEND_APP_1_NAME=webapp
FRONTEND_APP_1_PORT=3000
FRONTEND_APP_1_ROUTE=app

FRONTEND_APP_2_NAME=admin
FRONTEND_APP_2_PORT=3001
FRONTEND_APP_2_ROUTE=admin
```

---

## Examples

```bash
# Add a new frontend
nself dev frontend add myapp --port 3000

# Check status
nself dev frontend status

# Deploy to production
nself dev frontend deploy myapp --env prod

# Get environment variables for frontend
nself dev frontend env myapp

# View deployment logs
nself dev frontend logs myapp
```

---

## Environment Variables

The `env` subcommand generates framework-specific variables:

### Next.js

```bash
NEXT_PUBLIC_GRAPHQL_URL=https://api.local.nself.org/v1/graphql
NEXT_PUBLIC_AUTH_URL=https://auth.local.nself.org
NEXT_PUBLIC_STORAGE_URL=https://minio.local.nself.org
```

### Vite

```bash
VITE_GRAPHQL_URL=https://api.local.nself.org/v1/graphql
VITE_AUTH_URL=https://auth.local.nself.org
```

### Create React App

```bash
REACT_APP_GRAPHQL_URL=https://api.local.nself.org/v1/graphql
REACT_APP_AUTH_URL=https://auth.local.nself.org
```

---

## Deployment Providers

### Vercel

If `vercel.json` or `.vercel/` directory exists:

```bash
nself dev frontend deploy webapp           # Deploy preview
nself dev frontend deploy webapp --env prod  # Deploy production
```

Requires: `vercel` CLI (`npm i -g vercel`)

### Netlify

If `netlify.toml` or `.netlify/` directory exists:

```bash
nself dev frontend deploy webapp           # Deploy preview
nself dev frontend deploy webapp --env prod  # Deploy production
```

Requires: `netlify` CLI (`npm i -g netlify-cli`)

---

## Output Example

```
  ➞ Frontend Applications

  Name                 Port     Route           Status       URL
  ----                 ----     -----           ------       ---
  webapp               3000     app             running      https://app.local.nself.org
  admin                3001     admin           stopped      https://admin.local.nself.org

  ➞ Deployment Integrations

  ✓ Vercel configured
```

---

## Related Commands

- [urls](URLS.md) - Service URLs
- [deploy](DEPLOY.md) - Backend deployment
- [status](STATUS.md) - Service status

---

*Last Updated: January 24, 2026 | Version: 0.4.8*
