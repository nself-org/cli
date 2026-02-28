# nself plugin

**Category**: Plugin System Commands

Manage nself plugins and extensions.

## Overview

All plugin operations use `nself plugin <subcommand>` for installing, managing, and developing nself plugins.

**Features**:
- ✅ Plugin marketplace
- ✅ Custom plugin development
- ✅ Plugin dependency management
- ✅ Plugin versioning
- ✅ Plugin sandboxing

## Subcommands

| Subcommand | Description | Use Case |
|------------|-------------|----------|
| [install](#nself-plugin-install) | Install plugin | Add functionality |
| [uninstall](#nself-plugin-uninstall) | Uninstall plugin | Remove plugin |
| [list](#nself-plugin-list) | List plugins | View installed |
| [search](#nself-plugin-search) | Search marketplace | Find plugins |
| [update](#nself-plugin-update) | Update plugin | Get latest version |
| [create](#nself-plugin-create) | Create plugin | Plugin development |
| [publish](#nself-plugin-publish) | Publish plugin | Share plugin |
| [configure](#nself-plugin-configure) | Configure plugin | Plugin settings |

## Official Plugins

### Available Plugins (53+)

#### Analytics & Monitoring
- `analytics` - Google Analytics integration
- `mixpanel` - Mixpanel analytics
- `segment` - Segment.io integration
- `sentry` - Error tracking
- `datadog` - DataDog monitoring
- `newrelic` - New Relic APM

#### Authentication & Security
- `auth0` - Auth0 integration
- `okta` - Okta SSO
- `saml` - SAML authentication
- `ldap` - LDAP/Active Directory
- `2fa` - Two-factor authentication
- `captcha` - reCAPTCHA integration

#### Communication
- `sendgrid` - SendGrid email
- `twilio` - SMS/voice via Twilio
- `slack` - Slack notifications
- `discord` - Discord webhooks
- `telegram` - Telegram bot
- `whatsapp` - WhatsApp Business API

#### Payment & Billing
- `stripe` - Stripe payments
- `paypal` - PayPal integration
- `paddle` - Paddle billing
- `chargebee` - Chargebee subscriptions
- `braintree` - Braintree payments

#### Storage & CDN
- `aws-s3` - AWS S3 storage
- `cloudflare-r2` - Cloudflare R2
- `backblaze-b2` - Backblaze B2
- `cloudinary` - Image optimization
- `imgix` - Image CDN

#### Search & Indexing
- `algolia` - Algolia search
- `elasticsearch` - Elasticsearch
- `typesense` - Typesense search
- `meilisearch` - MeiliSearch (built-in)

#### CMS & Content
- `wordpress` - WordPress integration
- `contentful` - Contentful CMS
- `strapi` - Strapi CMS
- `sanity` - Sanity.io

#### Development Tools
- `graphql-codegen` - GraphQL code generation
- `prisma` - Prisma ORM
- `kysely` - Kysely query builder
- `drizzle` - Drizzle ORM

#### Social Media
- `facebook` - Facebook integration
- `twitter` - Twitter/X API
- `linkedin` - LinkedIn integration
- `instagram` - Instagram API

#### AI & ML
- `openai` - OpenAI API
- `huggingface` - Hugging Face models
- `tensorflow` - TensorFlow serving
- `langchain` - LangChain integration
- `cohere` - Cohere AI models

#### E-commerce
- `shopify` - Shopify integration
- `woocommerce` - WooCommerce
- `magento` - Magento
- `bigcommerce` - BigCommerce

## nself plugin install

Install plugin from marketplace or file.

**Usage**:
```bash
nself plugin install <plugin_name> [OPTIONS]
```

**Options**:
- `--version VERSION` - Install specific version
- `--from-file FILE` - Install from local file
- `--from-url URL` - Install from URL
- `--no-deps` - Skip dependencies

**Examples**:
```bash
# Install from marketplace
nself plugin install stripe

# Specific version
nself plugin install stripe --version 2.1.0

# From local file
nself plugin install --from-file ./custom-plugin.tar.gz

# From URL
nself plugin install --from-url https://plugins.example.com/custom-plugin.tar.gz
```

**Output**:
```
Installing plugin: stripe

→ Downloading stripe v2.1.0...
  ✓ Downloaded (2.3 MB)

→ Verifying signature...
  ✓ Signature valid

→ Installing dependencies...
  ✓ stripe-node v12.8.0

→ Configuring plugin...
  ✓ Configuration template created: .env.stripe

→ Running post-install script...
  ✓ Database migrations applied
  ✓ Webhooks configured

Plugin installed successfully!

Next steps:
  1. Configure: nself plugin configure stripe
  2. Set environment variables in .env.stripe
  3. Restart services: nself restart

Documentation: https://plugins.nself.org/stripe
```

## nself plugin list

List installed plugins.

**Usage**:
```bash
nself plugin list [OPTIONS]
```

**Options**:
- `--format FORMAT` - Output format (table/json)
- `--enabled-only` - Show only enabled plugins
- `--updates` - Check for updates

**Examples**:
```bash
# List all plugins
nself plugin list

# Check for updates
nself plugin list --updates

# JSON output
nself plugin list --format json
```

**Output**:
```
Installed Plugins

Name              Version    Status     Update Available
──────────────────────────────────────────────────────────
stripe            2.1.0      enabled    2.2.0
sendgrid          1.5.3      enabled    -
sentry            3.2.1      enabled    3.3.0
analytics         1.0.0      disabled   -

4 plugins installed (3 enabled, 1 disabled)
2 updates available
```

## nself plugin search

Search plugin marketplace.

**Usage**:
```bash
nself plugin search <query> [OPTIONS]
```

**Options**:
- `--category CATEGORY` - Filter by category
- `--verified` - Verified plugins only
- `--limit N` - Limit results

**Examples**:
```bash
# Search for payment plugins
nself plugin search payment

# Search in category
nself plugin search --category analytics

# Verified only
nself plugin search stripe --verified
```

**Output**:
```
Search Results: "payment"

Name          Category    Downloads    Rating    Verified
────────────────────────────────────────────────────────────
stripe        Payment     45.2K        ⭐⭐⭐⭐⭐      ✓
paypal        Payment     23.1K        ⭐⭐⭐⭐       ✓
paddle        Payment     12.5K        ⭐⭐⭐⭐⭐      ✓
braintree     Payment     8.9K         ⭐⭐⭐⭐       ✓

4 results found

Install with: nself plugin install <name>
```

## nself plugin configure

Configure plugin settings.

**Usage**:
```bash
nself plugin configure <plugin_name> [OPTIONS]
```

**Options**:
- `--interactive` - Interactive configuration
- `--set KEY=VALUE` - Set specific config value
- `--show` - Show current configuration

**Examples**:
```bash
# Interactive configuration
nself plugin configure stripe --interactive

# Set specific value
nself plugin configure stripe --set STRIPE_SECRET_KEY=sk_test_...

# Show configuration
nself plugin configure stripe --show
```

**Interactive Configuration**:
```
Configuring plugin: stripe

Required Settings
──────────────────────────────────────────────────────────
Stripe Secret Key: [hidden input] ✓
Stripe Publishable Key: pk_test_... ✓

Optional Settings
──────────────────────────────────────────────────────────
Enable webhooks? [Y/n]: y
Webhook URL: https://api.example.com/webhooks/stripe ✓
Webhook secret: [hidden input] ✓

Currency [USD]: USD ✓

Configuration saved!

Test your configuration:
  nself plugin test stripe
```

## nself plugin update

Update plugin to latest version.

**Usage**:
```bash
nself plugin update <plugin_name|--all> [OPTIONS]
```

**Options**:
- `--all` - Update all plugins
- `--check` - Check for updates only
- `--version VERSION` - Update to specific version

**Examples**:
```bash
# Update specific plugin
nself plugin update stripe

# Update all plugins
nself plugin update --all

# Check for updates
nself plugin update --check
```

## nself plugin create

Create new plugin from template.

**Usage**:
```bash
nself plugin create <plugin_name> [OPTIONS]
```

**Options**:
- `--template TEMPLATE` - Use template (basic/advanced)
- `--language LANG` - Plugin language (typescript/javascript/python)

**Examples**:
```bash
# Create basic plugin
nself plugin create my-plugin

# Advanced TypeScript plugin
nself plugin create my-plugin --template advanced --language typescript
```

**Generated Structure**:
```
my-plugin/
├── package.json
├── plugin.json           # Plugin manifest
├── README.md
├── src/
│   ├── index.ts
│   ├── config.ts
│   ├── hooks/
│   │   ├── pre-init.ts
│   │   ├── post-init.ts
│   │   └── pre-start.ts
│   └── api/
│       └── routes.ts
├── migrations/
│   └── 001_initial.sql
└── tests/
    └── index.test.ts
```

**plugin.json**:
```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "My custom nself plugin",
  "author": "Your Name",
  "license": "MIT",
  "nself_version": ">=0.9.0",
  "dependencies": {},
  "hooks": {
    "pre-init": "dist/hooks/pre-init.js",
    "post-init": "dist/hooks/post-init.js"
  },
  "config": {
    "API_KEY": {
      "type": "string",
      "required": true,
      "description": "API key for service"
    }
  }
}
```

## nself plugin publish

Publish plugin to marketplace.

**Usage**:
```bash
nself plugin publish [OPTIONS]
```

**Options**:
- `--dry-run` - Validate without publishing
- `--access PUBLIC|PRIVATE` - Plugin access level

**Requirements**:
- Valid plugin.json
- README.md
- Tests passing
- Code signed

**Examples**:
```bash
# Publish plugin
nself plugin publish

# Dry run first
nself plugin publish --dry-run

# Private plugin
nself plugin publish --access private
```

## Plugin Development

### Plugin Lifecycle Hooks

```typescript
// src/hooks/pre-init.ts
export default async (context: PluginContext) => {
  console.log('Running before nself init')
  // Setup plugin requirements
}

// src/hooks/post-init.ts
export default async (context: PluginContext) => {
  console.log('Running after nself init')
  // Configure plugin
}

// src/hooks/pre-start.ts
export default async (context: PluginContext) => {
  console.log('Running before nself start')
  // Prepare plugin services
}
```

### Plugin API

```typescript
import { Plugin } from '@nself/plugin-sdk'

export default class MyPlugin extends Plugin {
  async onInit() {
    // Plugin initialization
  }

  async onStart() {
    // When services start
  }

  async onStop() {
    // When services stop
  }

  async getRoutes() {
    return [
      {
        path: '/api/my-plugin',
        method: 'GET',
        handler: this.handleRequest
      }
    ]
  }

  async handleRequest(req, res) {
    res.json({ message: 'Hello from plugin!' })
  }
}
```

## Best Practices

### 1. Use Official Plugins

```bash
# Official plugins are verified and maintained
nself plugin install stripe
nself plugin install sendgrid
```

### 2. Keep Plugins Updated

```bash
# Weekly update check
nself plugin update --check

# Update all
nself plugin update --all
```

### 3. Test Plugins Before Production

```bash
# Test in development
ENV=dev nself plugin install new-plugin

# Test thoroughly
nself dev test integration

# Then install in production
ENV=prod nself plugin install new-plugin
```

### 4. Document Plugin Configuration

```bash
# Save plugin config
nself plugin configure stripe --show > docs/stripe-config.md
```

## Related Commands

- `nself config` - Plugin configuration
- `nself service` - Plugin services
- `nself dev` - Plugin development

## See Also

- [Plugin Development Guide](../../guides/PLUGIN-DEVELOPMENT.md)
- [Plugin Marketplace](https://plugins.nself.org)
- [Plugin SDK Documentation](../../sdk/PLUGIN-SDK.md)
- [Official Plugins](../../plugins/OFFICIAL.md)
