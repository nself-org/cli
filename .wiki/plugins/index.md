# nself Plugins

**Version 0.9.9** | Extend nself with third-party integrations

---

## Overview

Plugins extend nself with third-party service integrations, providing database schemas, webhook handlers, CLI commands, and more. The plugin system enables seamless integration with payment processors, e-commerce platforms, DevOps tools, and other services.

---

## Table of Contents

- [Available Plugins](#available-plugins)
- [Quick Start](#quick-start)
- [Plugin Architecture](#plugin-architecture)
- [Installation](#installation)
- [Environment Variables](#environment-variables)
- [Webhook Configuration](#webhook-configuration)
- [Database Integration](#database-integration)
- [Plugin Registry](#plugin-registry)
- [Update Management](#update-management)
- [Plugin Categories](#plugin-categories)
- [Planned Plugins](#planned-plugins)
- [Creating Custom Plugins](#creating-custom-plugins)

---

## Available Plugins

| Plugin | Category | Description | Version | Status |
|--------|----------|-------------|---------|--------|
| [Stripe](stripe.md) | Billing | Payment processing, subscriptions, invoices | 1.2.0 | Available |
| [GitHub](github.md) | DevOps | Repository sync, issues, PRs, Actions | 1.3.0 | Available |
| [Shopify](shopify.md) | E-Commerce | Products, orders, customers, inventory | 1.1.0 | Available |

### Plugin Features Matrix

| Plugin | Data Sync | Webhooks | CLI Actions | Analytics Views |
|--------|-----------|----------|-------------|-----------------|
| Stripe | Full | Yes | 5 commands | 2 views |
| GitHub | Full | Yes | 6 commands | 3 views |
| Shopify | Full | Yes | 5 commands | 4 views |

---

## Quick Start

### List Available Plugins

```bash
# Show all available plugins
nself plugin list

# Filter by category
nself plugin list --category billing

# Show installed plugins
nself plugin list --installed
```

### Install a Plugin

```bash
# Install plugin
nself plugin install stripe

# Rebuild configuration
nself build

# Restart services
nself start
```

### Use Plugin Commands

```bash
# Sync data
nself plugin stripe sync

# List customers
nself plugin stripe customers list

# View plugin status
nself plugin stripe status
```

---

## Plugin Architecture

Each plugin provides:

### 1. Database Schema

Tables for storing synced data:
- Prefixed tables (e.g., `stripe_customers`, `github_repos`)
- Migration support for schema updates
- Automatic Hasura tracking

### 2. Webhook Handlers

Real-time event processing:
- Signature verification
- Event routing
- Error handling and retry
- Event logging

### 3. CLI Actions

Commands for data management:
- Sync commands
- List and search
- Show details
- Webhook management

### 4. Analytics Views

Pre-built SQL views for insights:
- Revenue reports
- Activity summaries
- Performance metrics

### 5. Service Containers

Docker services for plugins:
- Webhook handlers
- Background workers
- API proxies

### 6. Nginx Routes

URL routing configuration:
- Webhook endpoints
- API proxies
- Custom routes

---

## Installation

### Before Installing

1. **Set required environment variables**

   ```bash
   # Add to .env
   echo "STRIPE_API_KEY=sk_test_PLACEHOLDER" >> .env
   ```

2. **Verify API access**

   ```bash
   # Check plugin requirements
   nself plugin install stripe --check-env
   ```

### Installation Process

```bash
# Install plugin
nself plugin install stripe
```

The install process:
1. Downloads plugin from registry (plugins.nself.org)
2. Validates compatibility with nself version
3. Verifies plugin checksum
4. Checks required environment variables
5. Creates database tables
6. Configures Hasura tracking
7. Sets up webhook handlers
8. Configures nginx routes
9. Runs initial data sync
10. Regenerates nself configuration

### Post-Installation

```bash
# Rebuild to include plugin services
nself build

# Start services including plugin
nself start

# Verify plugin status
nself plugin status stripe
```

---

## Environment Variables

Each plugin requires specific environment variables. These should be added to your `.env` file.

### Required vs Optional

| Plugin | Required | Optional |
|--------|----------|----------|
| Stripe | `STRIPE_API_KEY` | `STRIPE_WEBHOOK_SECRET`, `STRIPE_SYNC_INTERVAL` |
| GitHub | `GITHUB_TOKEN` | `GITHUB_ORG`, `GITHUB_WEBHOOK_SECRET`, `GITHUB_REPOS` |
| Shopify | `SHOPIFY_STORE`, `SHOPIFY_ACCESS_TOKEN` | `SHOPIFY_WEBHOOK_SECRET`, `SHOPIFY_API_VERSION` |

### Stripe Configuration

```bash
# Required
STRIPE_API_KEY=sk_test_PLACEHOLDER

# Optional
STRIPE_WEBHOOK_SECRET=whsec_xxxxx
STRIPE_SYNC_INTERVAL=3600
STRIPE_TEST_MODE=true
```

### GitHub Configuration

```bash
# Required
GITHUB_TOKEN=ghp_xxxxx

# Optional
GITHUB_WEBHOOK_SECRET=xxx
GITHUB_ORG=myorganization
GITHUB_REPOS=owner/repo1,owner/repo2
```

### Shopify Configuration

```bash
# Required
SHOPIFY_STORE=your-store
SHOPIFY_ACCESS_TOKEN=shpat_xxxxx

# Optional
SHOPIFY_API_VERSION=2024-01
SHOPIFY_WEBHOOK_SECRET=xxx
```

### Checking Configuration

```bash
# See what's missing
nself plugin status stripe

# Check all plugins
nself plugin status
```

---

## Webhook Configuration

### Endpoint Pattern

Plugins use standardized webhook endpoints:

```
https://your-domain.com/webhooks/<plugin-name>
```

Examples:
- Stripe: `https://example.com/webhooks/stripe`
- GitHub: `https://example.com/webhooks/github`
- Shopify: `https://example.com/webhooks/shopify`

### Setting Up Webhooks

#### Stripe

1. Go to [Stripe Dashboard > Webhooks](https://dashboard.stripe.com/webhooks)
2. Add endpoint: `https://your-domain.com/webhooks/stripe`
3. Select events:
   - `customer.created`, `customer.updated`
   - `subscription.created`, `subscription.updated`
   - `invoice.paid`, `invoice.payment_failed`
4. Copy signing secret to `STRIPE_WEBHOOK_SECRET`

#### GitHub

1. Go to Repository/Org > Settings > Webhooks
2. Add webhook:
   - URL: `https://your-domain.com/webhooks/github`
   - Content type: `application/json`
   - Secret: Your `GITHUB_WEBHOOK_SECRET`
3. Select events:
   - Push, Pull requests, Issues
   - Workflow runs, Releases

#### Shopify

1. Go to Settings > Notifications > Webhooks
2. Create webhooks for topics:
   - `orders/create`, `orders/updated`
   - `products/create`, `products/update`
   - `customers/create`, `customers/update`
3. URL: `https://your-domain.com/webhooks/shopify`

### Webhook Security

Always configure webhook secrets for signature verification:

```bash
STRIPE_WEBHOOK_SECRET=whsec_xxxxx
GITHUB_WEBHOOK_SECRET=xxx
SHOPIFY_WEBHOOK_SECRET=xxx
```

### Local Development

For local development, use tunneling:

```bash
# Stripe CLI
stripe listen --forward-to localhost/webhooks/stripe

# ngrok
ngrok http 443
```

---

## Database Integration

### Table Prefixes

Each plugin creates prefixed tables:

| Plugin | Prefix | Example Tables |
|--------|--------|----------------|
| Stripe | `stripe_` | `stripe_customers`, `stripe_subscriptions` |
| GitHub | `github_` | `github_repositories`, `github_issues` |
| Shopify | `shopify_` | `shopify_products`, `shopify_orders` |

### Viewing Tables

```bash
# List plugin tables
nself db tables | grep stripe_
nself db tables | grep github_
nself db tables | grep shopify_

# Query plugin data
nself db query "SELECT COUNT(*) FROM stripe_customers"
```

### Hasura Integration

Plugin tables are automatically:
- Tracked in Hasura
- Configured with relationships
- Available via GraphQL

```graphql
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

### Analytics Views

Plugins provide pre-built views:

```bash
# Stripe views
stripe_revenue_by_month
stripe_subscription_mrr

# GitHub views
github_open_items
github_recent_activity
github_workflow_stats

# Shopify views
shopify_sales_overview
shopify_top_products
shopify_low_inventory
shopify_customer_value
```

---

## Plugin Registry

### Architecture

nself uses a two-tier registry system:

**Primary: plugins.nself.org**
- Cloudflare Worker-based API
- KV storage for caching
- Sub-second response times
- Global edge distribution

**Fallback: GitHub**
- Raw file access from nself-plugins repo
- Used when primary is unavailable
- Stale cache as last resort

### Registry Endpoints

```
https://plugins.nself.org/registry.json     # Full registry
https://plugins.nself.org/plugins/:name     # Plugin info
https://plugins.nself.org/plugins/:name/:v  # Specific version
https://plugins.nself.org/health            # Health check
https://plugins.nself.org/categories        # Category list
```

### Configuration

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

```bash
# Clear cache and refresh
nself plugin refresh

# View cache status
ls -la ~/.nself/cache/plugins/

# Force download without cache
nself plugin install stripe --no-cache
```

---

## Update Management

### Checking for Updates

```bash
# Check for plugin updates
nself plugin updates

# JSON output
nself plugin updates --json
```

### Updating Plugins

```bash
# Update specific plugin
nself plugin update stripe

# Update all plugins
nself plugin update --all

# Dry run (see what would update)
nself plugin update --dry-run
```

### After Updating

```bash
# Rebuild configuration
nself build

# Restart services
nself start
```

---

## Plugin Categories

| Category | Description | Available Plugins |
|----------|-------------|-------------------|
| billing | Payment processing and subscriptions | Stripe |
| ecommerce | Online stores and inventory | Shopify |
| devops | Development tools and CI/CD | GitHub |
| productivity | Workspace and collaboration tools | (Coming soon) |
| communication | Messaging and notifications | (Coming soon) |
| finance | Banking and accounting | (Coming soon) |

### Filtering by Category

```bash
nself plugin list --category billing
nself plugin list --category devops
```

---

## Planned Plugins

### High Priority

| Plugin | Category | Description | ETA |
|--------|----------|-------------|-----|
| PayPal | Billing | Alternative payment processing | Q1 2026 |
| Linear | DevOps | Issue tracking integration | Q1 2026 |
| Plaid | Finance | Banking data aggregation | Q2 2026 |
| Notion | Productivity | Workspace database sync | Q2 2026 |

### Medium Priority

| Plugin | Category | Description | ETA |
|--------|----------|-------------|-----|
| Intercom | Communication | Customer messaging | Q2 2026 |
| SendGrid | Communication | Email delivery and webhooks | Q2 2026 |
| Square | Billing | POS and payments | Q3 2026 |
| Airtable | Productivity | Spreadsheet/database sync | Q3 2026 |

See the [plugin roadmap](https://github.com/acamarata/nself-plugins/blob/main/docs/PLANNED.md) for the full list.

---

## Creating Custom Plugins

### Quick Start

```bash
# Create plugin scaffold
nself plugin create my-plugin

# Creates:
~/.nself/plugins/my-plugin/
├── plugin.json           # Plugin manifest
├── install.sh            # Installation script
├── uninstall.sh          # Removal script
├── schema/
│   └── tables.sql        # Database schema
├── actions/
│   └── sync.sh           # Example action
└── README.md             # Documentation
```

### Plugin Structure

```
plugins/my-plugin/
├── plugin.json           # Required: Plugin manifest
├── install.sh           # Required: Installation script
├── uninstall.sh         # Required: Removal script
├── README.md            # Recommended: Documentation
├── schema/
│   ├── tables.sql       # Initial schema
│   └── migrations/      # Schema migrations
├── actions/
│   ├── sync.sh          # Sync command
│   └── list.sh          # List command
├── webhooks/
│   ├── handler.sh       # Main webhook handler
│   └── events/          # Event handlers
├── services/
│   └── webhook-handler/ # Docker service
├── nginx/
│   └── routes.conf      # Nginx configuration
└── hooks/
    ├── pre-install.sh
    └── post-install.sh
```

### Development Guide

For complete documentation on creating plugins, see the [Plugin Development Guide](development.md).

Topics covered:
- Plugin manifest format
- Database schema design
- CLI action development
- Webhook handler implementation
- Service container setup
- Testing and validation
- Publishing to the registry

---

## Related Documentation

- [Plugin Command](../commands/PLUGIN.md) - Complete plugin command reference
- [Plugin Development](development.md) - Creating custom plugins
- [Stripe Plugin](stripe.md) - Stripe integration details
- [GitHub Plugin](github.md) - GitHub integration details
- [Shopify Plugin](shopify.md) - Shopify integration details
- [Database Command](../commands/DB.md) - Database operations

---

## External Resources

- [Plugin Registry](https://github.com/acamarata/nself-plugins) - Official plugin repository
- [Plugin API](https://plugins.nself.org) - Registry API endpoint
- [Plugin Roadmap](https://github.com/acamarata/nself-plugins/blob/main/docs/PLANNED.md) - Upcoming plugins

---

*Last Updated: January 2026 | Version 0.9.9*
