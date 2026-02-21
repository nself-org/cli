# nself Plugins

**Version 0.9.9** | Extend nself with third-party integrations

---

## Overview

Plugins extend nself with third-party service integrations, providing database schemas, webhook handlers, CLI commands, and more. The plugin system enables seamless integration with payment processors, e-commerce platforms, DevOps tools, and other services.

---

## Available Plugins

| Plugin | Category | Description | Documentation |
|--------|----------|-------------|---------------|
| **Stripe** | Billing | Payment processing, subscriptions, invoices | [View Docs](stripe.md) |
| **GitHub** | DevOps | Repository sync, issues, PRs, Actions | [View Docs](github.md) |
| **Shopify** | E-Commerce | Products, orders, customers, inventory | [View Docs](shopify.md) |

---

## Quick Start

```bash
# List available plugins
nself plugin list

# Install a plugin
nself plugin install stripe

# Rebuild and restart
nself build && nself start

# Use plugin commands
nself plugin stripe sync
```

---

## Complete Documentation

For comprehensive plugin documentation, see:

- **[Complete Plugin Guide](index.md)** - Full plugin system documentation
- **[Plugin Development](development.md)** - Creating custom plugins
- **[Stripe Plugin](stripe.md)** - Payment processing integration
- **[GitHub Plugin](github.md)** - Repository and workflow sync
- **[Shopify Plugin](shopify.md)** - E-commerce integration

---

## Plugin System Features

- **Data Sync** - Automatic bidirectional sync with external services
- **Webhooks** - Real-time event processing
- **CLI Commands** - Plugin-specific commands
- **Database Integration** - Hasura GraphQL access to plugin data
- **Analytics Views** - Pre-built dashboard views
- **Update Management** - Version control and migration support

---

## Categories

| Category | Examples | Status |
|----------|----------|--------|
| **Billing** | Stripe, Paddle, Chargebee | Available |
| **E-Commerce** | Shopify, WooCommerce, BigCommerce | Available |
| **DevOps** | GitHub, GitLab, CircleCI | Available |
| **Marketing** | Mailchimp, SendGrid, Segment | Planned |
| **Analytics** | Mixpanel, Amplitude, PostHog | Planned |
| **Communication** | Slack, Discord, Twilio | Planned |

---

## Learn More

- **[Plugin Registry](https://plugins.nself.org)** - Browse all available plugins
- **[Plugin Development Guide](development.md)** - Create your own plugins
- **[GitHub Discussions](https://github.com/nself-org/cli/discussions)** - Request new plugins

---

**[← Back to Documentation](../README.md)** | **[View Full Plugin Guide →](index.md)**
