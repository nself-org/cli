# nself plugin - Plugin Management

**Version 0.9.9** | Extend nself with third-party integrations

---

## Overview

The `nself plugin` command manages plugins that extend nself with integrations for payment processors, e-commerce platforms, DevOps tools, and more.

Plugins provide:
- Database schemas for synced data
- Webhook handlers for real-time updates
- CLI commands for data management
- Pre-built analytics views
- Nginx route configurations
- Service container definitions

---

## Table of Contents

- [Usage](#usage)
- [Commands](#commands)
- [Plugin Registry](#plugin-registry)
- [Update Management](#update-management)
- [Plugin Actions](#plugin-actions)
- [Environment Variables](#environment-variables)
- [Webhook Setup](#webhook-setup)
- [Database Tables](#database-tables)
- [Plugin Architecture](#plugin-architecture)
- [nself Integration](#nself-integration)
- [Examples](#examples)
- [Plugin Categories](#plugin-categories)
- [Creating Plugins](#creating-plugins)
- [Troubleshooting](#troubleshooting)

---

## Usage

```bash
nself plugin <command> [options]
nself plugin <plugin-name> <action> [args]
```

---

## Commands

### `list`

List available and installed plugins.

```bash
# Show all available plugins
nself plugin list

# Show only installed plugins
nself plugin list --installed

# Filter by category
nself plugin list --category billing

# Show detailed information
nself plugin list --verbose

# JSON output for scripting
nself plugin list --json
```

**Output Example:**

```
Available Plugins
═══════════════════════════════════════════════════════════════

Billing
  ✓ stripe      1.2.0 (installed)   Payment processing & subscriptions
  ○ paypal      1.0.0               Alternative payments

E-Commerce
  ○ shopify     1.1.0               Store, products, orders sync
  ○ square      0.9.0               POS and payments

DevOps
  ✓ github      1.3.0 (installed)   Repository, issues, CI integration
  ○ linear      1.0.0               Issue tracking

Installed: 2 | Available: 6
```

### `install <name>`

Install a plugin from the registry.

```bash
nself plugin install stripe
nself plugin install github
nself plugin install shopify

# Install specific version
nself plugin install stripe@1.2.0

# Skip initial sync
nself plugin install stripe --skip-sync

# Force reinstall
nself plugin install stripe --force
```

The install process:
1. Downloads plugin from registry (plugins.nself.org)
2. Validates compatibility with nself version
3. Verifies plugin checksum
4. Checks required environment variables
5. Creates database tables (runs migrations)
6. Sets up webhook handlers
7. Configures nginx routes
8. Runs initial data sync (unless --skip-sync)
9. Regenerates nself configuration

### `remove <name>`

Remove an installed plugin.

```bash
# Remove plugin and all data
nself plugin remove stripe

# Remove plugin but keep database tables
nself plugin remove stripe --keep-data

# Force remove without confirmation
nself plugin remove stripe --force
```

### `update [name]`

Update plugins to latest version.

```bash
# Update specific plugin
nself plugin update stripe

# Update all plugins
nself plugin update --all

# Check what would be updated (dry run)
nself plugin update --dry-run
```

### `updates`

Check for available plugin updates.

```bash
# Check for updates
nself plugin updates

# JSON output
nself plugin updates --json
```

**Output Example:**

```
Plugin Updates Available
═══════════════════════════════════════════════════════════════

  stripe      1.1.0 → 1.2.0     New features: subscription pausing
  github      1.2.0 → 1.3.0     Bug fixes, performance improvements

Run 'nself plugin update --all' to update all plugins
Run 'nself plugin update <name>' to update a specific plugin
```

### `refresh`

Refresh the plugin registry cache.

```bash
# Force refresh from registry
nself plugin refresh

# Show registry metadata
nself plugin refresh --info
```

### `status [name]`

Show plugin status and health.

```bash
# Status of all plugins
nself plugin status

# Status of specific plugin
nself plugin status stripe

# Detailed status with all info
nself plugin status stripe --verbose
```

Output includes:
- Installation status and version
- Last sync time
- Webhook endpoint status
- Environment variable status
- Database table counts
- Service container status

**Output Example:**

```
Plugin: stripe
═══════════════════════════════════════════════════════════════

Status:       Installed
Version:      1.2.0
Installed:    2026-01-20 14:30:00
Last Sync:    2026-01-24 10:15:00

Environment Variables
  ✓ STRIPE_API_KEY          Set
  ✓ STRIPE_WEBHOOK_SECRET   Set
  ○ STRIPE_SYNC_INTERVAL    Not set (default: 3600)

Webhook
  Endpoint:   https://api.local.nself.org/webhooks/stripe
  Status:     Active
  Events:     1,234 processed, 3 failed

Database
  Tables:     8
  Records:    customers: 456, subscriptions: 123, invoices: 789

Service
  Container:  myapp_stripe_webhook
  Status:     Running (healthy)
```

---

## Plugin Registry

### Registry Architecture

nself uses a two-tier registry system for maximum reliability:

**Primary Registry (plugins.nself.org)**
- Cloudflare Worker-based API
- KV storage for caching
- Sub-second response times
- Global edge distribution

**Fallback (GitHub)**
- Raw GitHub file access
- Used when primary is unavailable
- Stale cache as last resort

### Registry Endpoints

```
https://plugins.nself.org/registry.json     Full registry
https://plugins.nself.org/plugins/:name     Plugin info
https://plugins.nself.org/plugins/:name/:v  Specific version
https://plugins.nself.org/health            Health check
https://plugins.nself.org/categories        Category list
```

### Registry Configuration

Configure registry settings in environment:

```bash
# Custom registry URL (for private registries)
NSELF_PLUGIN_REGISTRY=https://plugins.internal.company.com

# Registry cache TTL (default: 300 seconds)
NSELF_REGISTRY_CACHE_TTL=600

# Plugin cache directory
NSELF_PLUGIN_CACHE=$HOME/.nself/cache/plugins

# Plugin installation directory
NSELF_PLUGIN_DIR=$HOME/.nself/plugins
```

### Cache Management

The registry client caches data locally:

```bash
# View cache status
ls -la ~/.nself/cache/plugins/

# Clear cache and refresh
nself plugin refresh

# Force download without cache
nself plugin install stripe --no-cache
```

---

## Update Management

### Automatic Update Checking

Check for updates to installed plugins:

```bash
# Check for available updates
nself plugin updates
```

### Update Process

When updating plugins:

1. Downloads new plugin version
2. Verifies checksum
3. Backs up current installation
4. Runs database migrations (if any)
5. Updates plugin files
6. Regenerates configuration
7. Restarts affected services

### Version Pinning

Pin plugins to specific versions:

```bash
# Install specific version
nself plugin install stripe@1.2.0

# View available versions
nself plugin versions stripe
```

### Update Notifications

nself can notify about available updates:

```bash
# Enable update notifications
NSELF_PLUGIN_UPDATE_CHECK=true

# Check interval (daily by default)
NSELF_PLUGIN_UPDATE_INTERVAL=86400
```

---

## Plugin Actions

Each plugin provides its own set of actions. Use `nself plugin <name> --help` to see available actions.

### Built-in Actions (All Plugins)

Every installed plugin has these built-in actions:

```bash
nself plugin <name> init              # Initialize database schema
nself plugin <name> integrate         # Show CS_N service configuration for .env
```

**`init`** - Applies database schema SQL files from the plugin's `schema/` directory. If no SQL files exist, displays the expected database tables and guides you to run the plugin as a service.

**`integrate`** - Generates the configuration needed to run the plugin as a custom service (CS_N). Outputs the environment variables to add to your `.env` file, including the CS_N definition, required vars, and optional vars.

### Running Plugins as Services

Plugins that provide API servers (TypeScript, Python, Go, etc.) run as Docker containers via the CS_N custom service system. The workflow is:

```bash
# 1. Install the plugin
nself plugin install devices

# 2. See what configuration is needed
nself plugin devices integrate

# 3. Add the output to your .env file (example output):
#    CS_7=devices:express-js:3603
#    DEVICES_PLUGIN_ENABLED=true
#    DEVICES_PLUGIN_PORT=3603
#    DATABASE_URL=...

# 4. Build and start
nself build && nself restart

# 5. Initialize database schema (if needed)
nself plugin devices init
```

### Action Types

Plugins have three types of actions:

1. **Built-in actions** (`init`, `integrate`) - Always available
2. **Script actions** - Shell scripts in the plugin's `actions/` directory
3. **Service actions** - Defined in `plugin.json`, require the plugin service to be running

When you run a service action and the plugin is not yet running, the CLI will guide you through the setup process.

### Stripe Plugin

```bash
nself plugin stripe sync              # Sync customer/subscription data
nself plugin stripe customers         # List and manage customers
nself plugin stripe subscriptions     # View subscriptions
nself plugin stripe invoices          # View invoices
nself plugin stripe products          # View products and prices
nself plugin stripe webhook           # Manage webhook events
nself plugin stripe stats             # Revenue and subscription stats
```

### GitHub Plugin

```bash
nself plugin github sync              # Sync repository data
nself plugin github repos             # List repositories
nself plugin github issues            # View issues
nself plugin github prs               # View pull requests
nself plugin github actions           # View workflow runs
nself plugin github releases          # View releases
nself plugin github webhook           # Manage webhook events
nself plugin github stats             # Repository statistics
```

### Shopify Plugin

```bash
nself plugin shopify sync             # Sync store data
nself plugin shopify products         # List products
nself plugin shopify orders           # View orders
nself plugin shopify customers        # View customers
nself plugin shopify inventory        # Check inventory levels
nself plugin shopify webhook          # Manage webhook events
nself plugin shopify stats            # Sales statistics
```

---

## Environment Variables

Plugins require environment variables for API access. Add to your `.env`:

### Stripe

```bash
# Required
STRIPE_API_KEY=sk_test_PLACEHOLDER

# Optional
STRIPE_WEBHOOK_SECRET=whsec_xxxxx
STRIPE_SYNC_INTERVAL=3600
STRIPE_TEST_MODE=true
```

### GitHub

```bash
# Required
GITHUB_TOKEN=ghp_xxxxx

# Optional
GITHUB_WEBHOOK_SECRET=xxx
GITHUB_ORG=myorganization
GITHUB_REPOS=owner/repo1,owner/repo2
GITHUB_SYNC_INTERVAL=3600
```

### Shopify

```bash
# Required
SHOPIFY_STORE=your-store
SHOPIFY_ACCESS_TOKEN=shpat_xxxxx

# Optional
SHOPIFY_API_VERSION=2024-01
SHOPIFY_WEBHOOK_SECRET=xxx
SHOPIFY_SYNC_INTERVAL=3600
```

### Checking Required Variables

```bash
# See what's missing
nself plugin status stripe

# Or check during install
nself plugin install stripe --check-env
```

---

## Webhook Setup

Plugins that support webhooks need endpoint configuration in the external service.

### Webhook URL Pattern

```
https://your-domain.com/webhooks/<plugin-name>
```

Examples:
- Stripe: `https://your-domain.com/webhooks/stripe`
- GitHub: `https://your-domain.com/webhooks/github`
- Shopify: `https://your-domain.com/webhooks/shopify`

### Webhook Architecture

```
External Service
      │
      ▼
   Nginx (/webhooks/stripe)
      │
      ▼
Webhook Handler Container
      │
      ├─► Signature Verification
      ├─► Event Parsing
      ├─► Database Update
      └─► Custom Actions
```

### Signature Verification

Always set the webhook secret in your `.env` for security:

```bash
STRIPE_WEBHOOK_SECRET=whsec_xxxxx
GITHUB_WEBHOOK_SECRET=xxx
SHOPIFY_WEBHOOK_SECRET=xxx
```

### Webhook Event Log

All webhook events are logged:

```bash
# View recent events
nself plugin stripe webhook list --limit 20

# View failed events
nself plugin stripe webhook list --status failed

# Retry failed event
nself plugin stripe webhook retry evt_xxxxx

# Event statistics
nself plugin stripe webhook stats
```

### Local Development

For local development, use tunneling:

```bash
# Stripe CLI
stripe listen --forward-to localhost/webhooks/stripe

# ngrok
ngrok http 443
# Then configure webhook URL in service dashboard
```

---

## Database Tables

Each plugin creates prefixed tables in your database:

| Plugin | Table Prefix | Tables |
|--------|--------------|--------|
| Stripe | `stripe_` | 8 tables |
| GitHub | `github_` | 8 tables |
| Shopify | `shopify_` | 9 tables |

### View Tables

```bash
# List plugin tables
nself db tables | grep stripe_
nself db tables | grep github_
nself db tables | grep shopify_

# Count records
nself db query "SELECT COUNT(*) FROM stripe_customers"
```

### Database Migrations

Plugins use migrations for schema changes:

```bash
# Run pending migrations
nself plugin stripe migrate

# View migration status
nself plugin stripe migrate --status

# Rollback last migration
nself plugin stripe migrate --rollback
```

### Hasura Integration

Plugin tables are automatically tracked in Hasura:

- Tables registered with Hasura
- Relationships configured
- Permissions applied based on plugin settings
- GraphQL queries available immediately

```graphql
# Query plugin data via GraphQL
query {
  stripe_customers(limit: 10) {
    id
    email
    stripe_subscriptions {
      status
      current_period_end
    }
  }
}
```

---

## Plugin Architecture

### Directory Structure

```
~/.nself/plugins/
├── stripe/
│   ├── plugin.json           # Plugin manifest
│   ├── install.sh            # Installation script
│   ├── uninstall.sh          # Removal script
│   ├── schema/
│   │   ├── tables.sql        # Initial schema
│   │   └── migrations/       # Schema migrations
│   ├── actions/
│   │   ├── sync.sh           # Sync command
│   │   ├── customers.sh      # Customer commands
│   │   └── webhooks.sh       # Webhook commands
│   ├── webhooks/
│   │   ├── handler.sh        # Main webhook handler
│   │   └── events/           # Event-specific handlers
│   ├── services/
│   │   └── webhook-handler/  # Docker service
│   ├── nginx/
│   │   └── routes.conf       # Nginx route config
│   └── README.md             # Documentation
└── github/
    └── ...
```

### Plugin Manifest (plugin.json)

```json
{
  "name": "stripe",
  "version": "1.2.0",
  "description": "Stripe payment processing integration",
  "author": "nself Team",
  "category": "billing",
  "minNselfVersion": "0.4.5",
  "repository": "https://github.com/acamarata/nself-plugins",
  "homepage": "https://github.com/acamarata/nself-plugins/tree/main/plugins/stripe",

  "requires": {
    "env": ["STRIPE_API_KEY"],
    "services": ["postgres", "hasura"]
  },

  "provides": {
    "services": ["stripe-webhook-handler"],
    "routes": [
      {
        "subdomain": "stripe-webhooks",
        "target": "stripe-webhook-handler",
        "port": 8001
      }
    ],
    "tables": [
      "stripe_customers",
      "stripe_subscriptions",
      "stripe_invoices"
    ]
  },

  "actions": {
    "sync": "actions/sync.sh",
    "customers": "actions/customers.sh",
    "subscriptions": "actions/subscriptions.sh",
    "webhook": "actions/webhooks.sh"
  },

  "hooks": {
    "pre-install": "hooks/pre-install.sh",
    "post-install": "hooks/post-install.sh",
    "pre-uninstall": "hooks/pre-uninstall.sh",
    "post-uninstall": "hooks/post-uninstall.sh"
  }
}
```

### Plugin Lifecycle

**Installation:**
```
1. Download from registry
2. Verify checksum
3. Extract to plugin directory
4. Run pre-install hook
5. Apply database schema
6. Configure Hasura tracking
7. Generate nginx routes
8. Deploy service container
9. Run post-install hook
10. Initial data sync
```

**Uninstallation:**
```
1. Run pre-uninstall hook
2. Stop service container
3. Remove nginx routes
4. Untrack from Hasura
5. Drop database tables (unless --keep-data)
6. Remove plugin files
7. Run post-uninstall hook
8. Regenerate configuration
```

---

## nself Integration

### Build Integration

Plugins integrate with `nself build`:

```bash
nself build
# Includes:
# - Plugin nginx routes
# - Plugin service containers
# - Plugin environment setup
```

### Status Integration

Plugin status shown in `nself status`:

```bash
nself status
# Shows:
# Plugin Services
#   ✓ stripe-webhook    running   healthy
#   ✓ github-webhook    running   healthy
```

### URLs Integration

Plugin URLs shown in `nself urls`:

```bash
nself urls
# Shows:
# Plugin Services
#   https://stripe-webhooks.local.nself.org   Stripe Webhooks
#   https://github-webhooks.local.nself.org   GitHub Webhooks
```

### Doctor Integration

Plugin health checked in `nself doctor`:

```bash
nself doctor
# Checks:
# ✓ Plugin: stripe - healthy
# ✓ Plugin: github - healthy
# ✗ Plugin: shopify - missing SHOPIFY_WEBHOOK_SECRET
```

---

## Examples

### Install and Configure Stripe

```bash
# Set environment variables
echo "STRIPE_API_KEY=sk_test_PLACEHOLDER" >> .env
echo "STRIPE_WEBHOOK_SECRET=whsec_xxx" >> .env

# Install plugin
nself plugin install stripe

# Rebuild configuration
nself build

# Restart services
nself start

# Initial sync
nself plugin stripe sync

# View customers
nself plugin stripe customers list
```

### Check Plugin Health

```bash
# Check all plugins
nself plugin status

# Detailed status
nself plugin status stripe

# View recent webhook events
nself plugin stripe webhook list --limit 10
```

### Retry Failed Webhook

```bash
# List failed events
nself plugin stripe webhook list --status failed

# Retry specific event
nself plugin stripe webhook retry evt_123456

# Retry all failed events
nself plugin stripe webhook retry --all-failed
```

### Update All Plugins

```bash
# Check what needs updating
nself plugin updates

# Update all
nself plugin update --all

# Rebuild and restart
nself build && nself start
```

### Export Plugin Data

```bash
# Export to JSON
nself plugin stripe customers list --json > customers.json

# Export to CSV
nself plugin stripe invoices list --csv > invoices.csv

# SQL query
nself db query "SELECT * FROM stripe_customers" --csv > customers.csv
```

---

## Plugin Categories

| Category | Description | Plugins |
|----------|-------------|---------|
| billing | Payment & subscriptions | Stripe, PayPal |
| ecommerce | Online stores | Shopify, Square |
| devops | Dev tools & CI | GitHub, Linear |
| productivity | Workspace tools | Notion, Airtable |
| communication | Messaging | Intercom, SendGrid |
| finance | Banking & accounting | Plaid |

### Category Commands

```bash
# List plugins by category
nself plugin list --category billing

# See available categories
nself plugin categories
```

---

## Creating Plugins

For creating custom plugins, see the [Plugin Development Guide](../plugins/development.md).

### Quick Start

```bash
# Create plugin scaffold
nself plugin create my-plugin

# Creates:
# ~/.nself/plugins/my-plugin/
# ├── plugin.json
# ├── install.sh
# ├── uninstall.sh
# ├── schema/tables.sql
# ├── actions/sync.sh
# └── README.md
```

### Plugin Development Workflow

1. Create plugin structure
2. Define database schema
3. Implement CLI actions
4. Create webhook handlers (if needed)
5. Configure nginx routes (if needed)
6. Write documentation
7. Test locally
8. Submit to registry

### Testing Plugins

```bash
# Test install (local plugin)
nself plugin install --local ./my-plugin

# Test actions
nself plugin my-plugin sync --verbose

# Validate plugin manifest
nself plugin validate ./my-plugin
```

---

## Troubleshooting

### Plugin Won't Install

```bash
# Check nself version
nself --version

# Check required env vars
nself plugin status <name>

# Check registry connectivity
curl https://plugins.nself.org/health

# Verbose install
nself plugin install stripe --verbose
```

### Webhooks Not Received

1. Verify webhook URL is accessible externally
2. Check webhook secret matches in both places
3. Review nginx logs: `docker logs <project>_nginx`
4. Check webhook handler logs: `docker logs <project>_stripe_webhook`

```bash
# Test webhook endpoint
curl -X POST https://your-domain.com/webhooks/stripe \
  -H "Content-Type: application/json" \
  -d '{"type": "test"}'

# Check nginx routing
nself urls | grep webhook
```

### Sync Failing

```bash
# Check API credentials
nself plugin <name> status

# View sync logs
nself plugin <name> sync --verbose

# Check rate limits
nself plugin <name> rate-limit
```

### Database Errors

```bash
# Check table exists
nself db tables | grep stripe_

# View migration status
nself plugin stripe migrate --status

# Re-run migrations
nself plugin stripe migrate

# Check Hasura tracking
nself db hasura tables
```

### Service Not Starting

```bash
# Check container status
docker ps | grep stripe

# View container logs
docker logs myapp_stripe_webhook

# Check environment
docker exec myapp_stripe_webhook env | grep STRIPE
```

### Registry Unavailable

```bash
# Clear cache and retry
rm -rf ~/.nself/cache/plugins/*
nself plugin refresh

# Check connectivity
curl https://plugins.nself.org/health

# Use fallback
curl https://raw.githubusercontent.com/acamarata/nself-plugins/main/registry.json
```

---

## Related Commands

- [db](DB.md) - Database management
- [build](BUILD.md) - Generate configuration
- [start](START.md) - Start services
- [status](STATUS.md) - Check service status
- [urls](URLS.md) - Show service URLs
- [doctor](DOCTOR.md) - System health check

---

## Related Documentation

- [Plugin Overview](../plugins/index.md) - Plugin system introduction
- [Stripe Plugin](../plugins/stripe.md) - Stripe integration details
- [GitHub Plugin](../plugins/github.md) - GitHub integration details
- [Shopify Plugin](../plugins/shopify.md) - Shopify integration details
- [Plugin Development](../plugins/development.md) - Creating custom plugins

---

*Last Updated: February 2026 | Version 0.9.9*
