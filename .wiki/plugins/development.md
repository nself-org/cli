# Plugin Development Guide

**Version 0.9.9** | Create custom plugins for nself

---

## Overview

This guide covers everything you need to know to create custom plugins for nself. Plugins extend nself with third-party integrations, providing database schemas, webhook handlers, CLI commands, and more.

---

## Table of Contents

- [Quick Start](#quick-start)
- [Plugin Structure](#plugin-structure)
- [Plugin Manifest](#plugin-manifest)
- [Database Schema](#database-schema)
- [CLI Actions](#cli-actions)
- [Webhook Handlers](#webhook-handlers)
- [Service Containers](#service-containers)
- [Nginx Routes](#nginx-routes)
- [Lifecycle Hooks](#lifecycle-hooks)
- [Testing](#testing)
- [Publishing](#publishing)
- [Best Practices](#best-practices)

---

## Quick Start

### Create Plugin Scaffold

```bash
# Generate plugin structure
nself plugin create my-integration

# Creates:
~/.nself/plugins/my-integration/
├── plugin.json           # Plugin manifest
├── install.sh            # Installation script
├── uninstall.sh          # Removal script
├── schema/
│   └── tables.sql        # Database schema
├── actions/
│   └── sync.sh           # Example action
├── webhooks/
│   └── handler.sh        # Webhook handler
└── README.md             # Documentation
```

### Minimal Plugin

The simplest possible plugin:

**plugin.json**
```json
{
  "name": "my-integration",
  "version": "1.0.0",
  "description": "My custom integration",
  "author": "Your Name",
  "category": "custom",
  "minNselfVersion": "0.4.5"
}
```

**install.sh**
```bash
#!/usr/bin/env bash
echo "Installing my-integration plugin..."
# Plugin installation logic
```

**uninstall.sh**
```bash
#!/usr/bin/env bash
echo "Uninstalling my-integration plugin..."
# Plugin cleanup logic
```

---

## Plugin Structure

### Directory Layout

```
plugins/my-plugin/
├── plugin.json               # Required: Plugin manifest
├── install.sh               # Required: Installation script
├── uninstall.sh             # Required: Removal script
├── README.md                # Recommended: Documentation
│
├── schema/                  # Database schema
│   ├── tables.sql           # Initial table definitions
│   └── migrations/          # Schema migrations
│       ├── 001_initial.sql
│       ├── 002_add_column.sql
│       └── ...
│
├── actions/                 # CLI commands
│   ├── sync.sh              # Sync command
│   ├── list.sh              # List command
│   ├── show.sh              # Show command
│   └── ...
│
├── webhooks/                # Webhook handling
│   ├── handler.sh           # Main webhook handler
│   ├── verify.sh            # Signature verification
│   └── events/              # Event-specific handlers
│       ├── created.sh
│       ├── updated.sh
│       └── deleted.sh
│
├── services/                # Docker services
│   └── webhook-handler/
│       ├── Dockerfile
│       ├── package.json
│       └── src/
│
├── nginx/                   # Nginx configuration
│   └── routes.conf          # Route definitions
│
├── hooks/                   # Lifecycle hooks
│   ├── pre-install.sh
│   ├── post-install.sh
│   ├── pre-uninstall.sh
│   └── post-uninstall.sh
│
└── lib/                     # Shared libraries
    ├── api.sh               # API client functions
    └── utils.sh             # Utility functions
```

### File Permissions

Ensure scripts are executable:

```bash
chmod +x install.sh uninstall.sh
chmod +x actions/*.sh
chmod +x webhooks/*.sh
chmod +x hooks/*.sh
```

---

## Plugin Manifest

### Complete Manifest Example

**plugin.json**
```json
{
  "name": "my-integration",
  "version": "1.0.0",
  "description": "Integration with My Service",
  "longDescription": "Full integration with My Service including data sync, webhooks, and analytics.",
  "author": "Your Name",
  "license": "MIT",
  "category": "custom",
  "tags": ["api", "sync", "webhooks"],

  "minNselfVersion": "0.4.5",
  "maxNselfVersion": "1.0.0",

  "repository": "https://github.com/yourname/nself-my-integration",
  "homepage": "https://github.com/yourname/nself-my-integration#readme",
  "bugs": "https://github.com/yourname/nself-my-integration/issues",
  "documentation": "https://github.com/yourname/nself-my-integration/wiki",

  "requires": {
    "env": [
      "MY_SERVICE_API_KEY"
    ],
    "optionalEnv": [
      "MY_SERVICE_WEBHOOK_SECRET",
      "MY_SERVICE_SYNC_INTERVAL"
    ],
    "services": ["postgres", "hasura"],
    "plugins": []
  },

  "provides": {
    "services": ["my-integration-webhook"],
    "routes": [
      {
        "subdomain": "my-webhooks",
        "target": "my-integration-webhook",
        "port": 8001,
        "description": "My Service Webhook Endpoint"
      }
    ],
    "tables": [
      "my_integration_items",
      "my_integration_events",
      "my_integration_webhook_log"
    ],
    "views": [
      "my_integration_stats"
    ]
  },

  "actions": {
    "sync": {
      "script": "actions/sync.sh",
      "description": "Sync data from My Service",
      "usage": "nself plugin my-integration sync [--full]"
    },
    "list": {
      "script": "actions/list.sh",
      "description": "List synced items",
      "usage": "nself plugin my-integration list [--limit N]"
    },
    "show": {
      "script": "actions/show.sh",
      "description": "Show item details",
      "usage": "nself plugin my-integration show <id>"
    },
    "webhook": {
      "script": "actions/webhook.sh",
      "description": "Manage webhooks",
      "usage": "nself plugin my-integration webhook <list|retry|stats>"
    }
  },

  "hooks": {
    "pre-install": "hooks/pre-install.sh",
    "post-install": "hooks/post-install.sh",
    "pre-uninstall": "hooks/pre-uninstall.sh",
    "post-uninstall": "hooks/post-uninstall.sh",
    "pre-update": "hooks/pre-update.sh",
    "post-update": "hooks/post-update.sh"
  },

  "config": {
    "syncInterval": 3600,
    "batchSize": 100,
    "retryAttempts": 3
  }
}
```

### Manifest Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique plugin identifier (lowercase, hyphens) |
| `version` | Yes | Semantic version (x.y.z) |
| `description` | Yes | Short description (< 100 chars) |
| `author` | Yes | Plugin author name |
| `category` | Yes | Plugin category |
| `minNselfVersion` | Yes | Minimum compatible nself version |
| `requires` | No | Dependencies (env vars, services) |
| `provides` | No | What plugin provides |
| `actions` | No | CLI commands |
| `hooks` | No | Lifecycle hooks |

### Categories

Available plugin categories:

- `billing` - Payment and subscription services
- `ecommerce` - E-commerce platforms
- `devops` - Development and CI/CD tools
- `productivity` - Workspace and productivity tools
- `communication` - Messaging and email services
- `finance` - Banking and accounting
- `analytics` - Analytics and tracking
- `custom` - General purpose

---

## Database Schema

### Initial Schema

**schema/tables.sql**
```sql
-- My Integration Tables
-- Plugin: my-integration
-- Version: 1.0.0

-- Main items table
CREATE TABLE IF NOT EXISTS my_integration_items (
    id TEXT PRIMARY KEY,
    external_id TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    data JSONB DEFAULT '{}',
    synced_at TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_my_integration_items_external_id
    ON my_integration_items(external_id);
CREATE INDEX IF NOT EXISTS idx_my_integration_items_synced_at
    ON my_integration_items(synced_at);

-- Webhook event log
CREATE TABLE IF NOT EXISTS my_integration_webhook_log (
    id SERIAL PRIMARY KEY,
    event_id TEXT UNIQUE NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT DEFAULT 'pending',
    processed_at TIMESTAMPTZ,
    error TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_my_integration_webhook_log_status
    ON my_integration_webhook_log(status);
CREATE INDEX IF NOT EXISTS idx_my_integration_webhook_log_event_type
    ON my_integration_webhook_log(event_type);

-- Analytics view
CREATE OR REPLACE VIEW my_integration_stats AS
SELECT
    COUNT(*) AS total_items,
    COUNT(*) FILTER (WHERE synced_at > NOW() - INTERVAL '24 hours') AS synced_today,
    MAX(synced_at) AS last_sync
FROM my_integration_items;

-- Set up Hasura tracking (handled by nself)
COMMENT ON TABLE my_integration_items IS 'hasura:track';
COMMENT ON TABLE my_integration_webhook_log IS 'hasura:track';
```

### Migrations

**schema/migrations/001_initial.sql**
```sql
-- Migration: 001_initial
-- Description: Initial schema
-- Date: 2026-01-24

-- Up migration
CREATE TABLE IF NOT EXISTS my_integration_items (
    id TEXT PRIMARY KEY,
    external_id TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Down migration (in separate file or marked section)
-- DROP TABLE IF EXISTS my_integration_items;
```

**schema/migrations/002_add_data_column.sql**
```sql
-- Migration: 002_add_data_column
-- Description: Add data JSONB column
-- Date: 2026-01-25

ALTER TABLE my_integration_items
ADD COLUMN IF NOT EXISTS data JSONB DEFAULT '{}';
```

### Running Migrations

```bash
# Apply pending migrations
nself plugin my-integration migrate

# Check migration status
nself plugin my-integration migrate --status

# Rollback last migration
nself plugin my-integration migrate --rollback
```

---

## CLI Actions

### Action Script Template

**actions/sync.sh**
```bash
#!/usr/bin/env bash
# Action: sync
# Description: Sync data from My Service

set -euo pipefail

# Load plugin utilities
PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$PLUGIN_DIR/lib/utils.sh"
source "$PLUGIN_DIR/lib/api.sh"

# Parse arguments
FULL_SYNC=false
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --full)
            FULL_SYNC=true
            shift
            ;;
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --help|-h)
            echo "Usage: nself plugin my-integration sync [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --full     Perform full sync (not incremental)"
            echo "  --verbose  Show detailed output"
            echo "  --help     Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Check required environment
require_env "MY_SERVICE_API_KEY"

# Perform sync
log_info "Starting sync..."

if [[ "$FULL_SYNC" == "true" ]]; then
    log_info "Performing full sync"
    sync_all_items
else
    log_info "Performing incremental sync"
    sync_recent_items
fi

log_success "Sync completed"
```

### Utility Functions

**lib/utils.sh**
```bash
#!/usr/bin/env bash
# Shared utility functions

# Logging functions
log_info() {
    printf "\033[34m[INFO]\033[0m %s\n" "$1"
}

log_success() {
    printf "\033[32m[SUCCESS]\033[0m %s\n" "$1"
}

log_error() {
    printf "\033[31m[ERROR]\033[0m %s\n" "$1" >&2
}

log_warning() {
    printf "\033[33m[WARNING]\033[0m %s\n" "$1"
}

# Environment checking
require_env() {
    local var_name="$1"
    if [[ -z "${!var_name:-}" ]]; then
        log_error "Required environment variable $var_name is not set"
        exit 1
    fi
}

# Database helpers
db_query() {
    local query="$1"
    nself db query "$query"
}

db_insert() {
    local table="$1"
    local columns="$2"
    local values="$3"
    db_query "INSERT INTO $table ($columns) VALUES ($values)"
}
```

### API Client

**lib/api.sh**
```bash
#!/usr/bin/env bash
# API client functions

API_BASE_URL="https://api.myservice.com/v1"

# Make authenticated API request
api_request() {
    local method="$1"
    local endpoint="$2"
    local data="${3:-}"

    local url="${API_BASE_URL}${endpoint}"
    local auth_header="Authorization: Bearer ${MY_SERVICE_API_KEY}"

    if [[ -n "$data" ]]; then
        curl -s -X "$method" "$url" \
            -H "$auth_header" \
            -H "Content-Type: application/json" \
            -d "$data"
    else
        curl -s -X "$method" "$url" \
            -H "$auth_header"
    fi
}

# API helper functions
api_get() {
    api_request "GET" "$1"
}

api_post() {
    api_request "POST" "$1" "$2"
}

# Sync functions
sync_all_items() {
    local items
    items=$(api_get "/items")

    echo "$items" | jq -c '.data[]' | while read -r item; do
        process_item "$item"
    done
}

sync_recent_items() {
    local since
    since=$(db_query "SELECT MAX(synced_at) FROM my_integration_items" | tail -1)

    local items
    items=$(api_get "/items?since=${since}")

    echo "$items" | jq -c '.data[]' | while read -r item; do
        process_item "$item"
    done
}

process_item() {
    local item="$1"
    local id external_id name

    id=$(echo "$item" | jq -r '.id')
    external_id=$(echo "$item" | jq -r '.external_id')
    name=$(echo "$item" | jq -r '.name')

    db_query "INSERT INTO my_integration_items (id, external_id, name, data, synced_at)
              VALUES ('$id', '$external_id', '$name', '$item', NOW())
              ON CONFLICT (id) DO UPDATE SET
                  name = EXCLUDED.name,
                  data = EXCLUDED.data,
                  synced_at = NOW()"
}
```

---

## Webhook Handlers

### Main Handler

**webhooks/handler.sh**
```bash
#!/usr/bin/env bash
# Main webhook handler

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$PLUGIN_DIR/lib/utils.sh"
source "$PLUGIN_DIR/webhooks/verify.sh"

# Read webhook payload
PAYLOAD=$(cat)
SIGNATURE="${HTTP_X_WEBHOOK_SIGNATURE:-}"

# Verify signature
if ! verify_signature "$PAYLOAD" "$SIGNATURE"; then
    log_error "Invalid webhook signature"
    echo '{"error": "Invalid signature"}'
    exit 1
fi

# Parse event
EVENT_TYPE=$(echo "$PAYLOAD" | jq -r '.type')
EVENT_ID=$(echo "$PAYLOAD" | jq -r '.id')

log_info "Processing webhook: $EVENT_TYPE ($EVENT_ID)"

# Log event
db_query "INSERT INTO my_integration_webhook_log (event_id, event_type, payload)
          VALUES ('$EVENT_ID', '$EVENT_TYPE', '$PAYLOAD')"

# Route to event handler
case "$EVENT_TYPE" in
    item.created)
        source "$PLUGIN_DIR/webhooks/events/created.sh"
        handle_item_created "$PAYLOAD"
        ;;
    item.updated)
        source "$PLUGIN_DIR/webhooks/events/updated.sh"
        handle_item_updated "$PAYLOAD"
        ;;
    item.deleted)
        source "$PLUGIN_DIR/webhooks/events/deleted.sh"
        handle_item_deleted "$PAYLOAD"
        ;;
    *)
        log_warning "Unknown event type: $EVENT_TYPE"
        ;;
esac

# Mark as processed
db_query "UPDATE my_integration_webhook_log
          SET status = 'processed', processed_at = NOW()
          WHERE event_id = '$EVENT_ID'"

echo '{"success": true}'
```

### Signature Verification

**webhooks/verify.sh**
```bash
#!/usr/bin/env bash
# Webhook signature verification

verify_signature() {
    local payload="$1"
    local signature="$2"
    local secret="${MY_SERVICE_WEBHOOK_SECRET:-}"

    if [[ -z "$secret" ]]; then
        log_warning "No webhook secret configured, skipping verification"
        return 0
    fi

    local expected
    expected=$(echo -n "$payload" | openssl dgst -sha256 -hmac "$secret" | cut -d' ' -f2)

    if [[ "$signature" == "$expected" ]]; then
        return 0
    else
        return 1
    fi
}
```

### Event Handlers

**webhooks/events/created.sh**
```bash
#!/usr/bin/env bash
# Handle item.created events

handle_item_created() {
    local payload="$1"
    local item

    item=$(echo "$payload" | jq -c '.data')

    local id external_id name
    id=$(echo "$item" | jq -r '.id')
    external_id=$(echo "$item" | jq -r '.external_id')
    name=$(echo "$item" | jq -r '.name')

    db_query "INSERT INTO my_integration_items (id, external_id, name, data, synced_at)
              VALUES ('$id', '$external_id', '$name', '$item', NOW())
              ON CONFLICT (id) DO NOTHING"

    log_info "Created item: $id"
}
```

---

## Service Containers

### Webhook Service

For plugins that need a dedicated service container:

**services/webhook-handler/Dockerfile**
```dockerfile
FROM node:20-alpine

WORKDIR /app

COPY package*.json ./
RUN npm ci --production

COPY . .

EXPOSE 8001

CMD ["node", "src/server.js"]
```

**services/webhook-handler/src/server.js**
```javascript
const express = require('express');
const crypto = require('crypto');

const app = express();
app.use(express.json());

const PORT = process.env.PORT || 8001;
const WEBHOOK_SECRET = process.env.MY_SERVICE_WEBHOOK_SECRET;

// Verify webhook signature
function verifySignature(payload, signature) {
  if (!WEBHOOK_SECRET) return true;

  const expected = crypto
    .createHmac('sha256', WEBHOOK_SECRET)
    .update(JSON.stringify(payload))
    .digest('hex');

  return signature === expected;
}

// Webhook endpoint
app.post('/webhook', (req, res) => {
  const signature = req.headers['x-webhook-signature'];

  if (!verifySignature(req.body, signature)) {
    return res.status(401).json({ error: 'Invalid signature' });
  }

  const { type, id, data } = req.body;

  console.log(`Processing ${type} event: ${id}`);

  // Process event asynchronously
  processEvent(type, id, data)
    .then(() => console.log(`Processed ${id}`))
    .catch(err => console.error(`Error processing ${id}:`, err));

  res.json({ success: true });
});

// Health check
app.get('/health', (req, res) => {
  res.json({ status: 'healthy' });
});

app.listen(PORT, () => {
  console.log(`Webhook handler listening on port ${PORT}`);
});
```

---

## Nginx Routes

### Route Configuration

**nginx/routes.conf**
```nginx
# My Integration Routes
# Plugin: my-integration

# Webhook endpoint
location /webhooks/my-integration {
    proxy_pass http://my-integration-webhook:8001/webhook;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    # Pass webhook headers
    proxy_set_header X-Webhook-Signature $http_x_webhook_signature;

    # Timeouts
    proxy_connect_timeout 30s;
    proxy_read_timeout 60s;
}
```

---

## Lifecycle Hooks

### Installation Hooks

**hooks/pre-install.sh**
```bash
#!/usr/bin/env bash
# Pre-installation hook

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$PLUGIN_DIR/lib/utils.sh"

log_info "Running pre-install checks..."

# Check required environment variables
if [[ -z "${MY_SERVICE_API_KEY:-}" ]]; then
    log_error "MY_SERVICE_API_KEY is required"
    log_info "Please set MY_SERVICE_API_KEY in your .env file"
    exit 1
fi

# Verify API access
if ! api_get "/health" > /dev/null 2>&1; then
    log_error "Cannot connect to My Service API"
    log_info "Please verify your API key"
    exit 1
fi

log_success "Pre-install checks passed"
```

**hooks/post-install.sh**
```bash
#!/usr/bin/env bash
# Post-installation hook

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$PLUGIN_DIR/lib/utils.sh"

log_info "Running post-install setup..."

# Run initial sync
log_info "Performing initial data sync..."
"$PLUGIN_DIR/actions/sync.sh" --full

# Show setup instructions
log_success "Plugin installed successfully!"
echo ""
echo "Next steps:"
echo "1. Configure webhooks in My Service dashboard"
echo "   URL: https://$(nself urls --plain | head -1)/webhooks/my-integration"
echo "2. Set MY_SERVICE_WEBHOOK_SECRET in your .env"
echo "3. Run 'nself build && nself start' to apply changes"
```

### Uninstallation Hooks

**hooks/pre-uninstall.sh**
```bash
#!/usr/bin/env bash
# Pre-uninstallation hook

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$PLUGIN_DIR/lib/utils.sh"

log_info "Preparing for uninstallation..."

# Stop any running sync processes
pkill -f "my-integration sync" || true

log_success "Ready for uninstallation"
```

---

## Testing

### Local Testing

```bash
# Install plugin locally (development mode)
nself plugin install --local ./my-plugin

# Test individual actions
nself plugin my-plugin sync --verbose

# Test webhook handler
curl -X POST http://localhost/webhooks/my-plugin \
  -H "Content-Type: application/json" \
  -d '{"type": "item.created", "id": "test-123", "data": {"name": "Test"}}'

# View database changes
nself db query "SELECT * FROM my_integration_items LIMIT 10"

# Check logs
docker logs myapp_my-integration-webhook
```

### Validation

```bash
# Validate plugin manifest
nself plugin validate ./my-plugin

# Check for common issues
nself plugin check ./my-plugin

# Lint shell scripts
shellcheck actions/*.sh webhooks/*.sh
```

### Test Script

**test/test.sh**
```bash
#!/usr/bin/env bash
# Plugin test suite

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "Running plugin tests..."

# Test 1: Manifest validation
echo "Test 1: Manifest validation"
if jq empty "$PLUGIN_DIR/plugin.json" 2>/dev/null; then
    echo "  PASS: Valid JSON"
else
    echo "  FAIL: Invalid JSON"
    exit 1
fi

# Test 2: Required files exist
echo "Test 2: Required files"
for file in install.sh uninstall.sh plugin.json; do
    if [[ -f "$PLUGIN_DIR/$file" ]]; then
        echo "  PASS: $file exists"
    else
        echo "  FAIL: $file missing"
        exit 1
    fi
done

# Test 3: Scripts are executable
echo "Test 3: Script permissions"
for script in install.sh uninstall.sh actions/*.sh; do
    if [[ -x "$PLUGIN_DIR/$script" ]]; then
        echo "  PASS: $script is executable"
    else
        echo "  FAIL: $script is not executable"
        exit 1
    fi
done

echo ""
echo "All tests passed!"
```

---

## Publishing

### Prepare for Publication

1. **Version your plugin**
   ```bash
   # Update version in plugin.json
   jq '.version = "1.0.0"' plugin.json > tmp.json && mv tmp.json plugin.json
   ```

2. **Update documentation**
   - Complete README.md
   - Add usage examples
   - Document all environment variables

3. **Create release**
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

### Submit to Registry

Submit a pull request to the [nself-plugins repository](https://github.com/acamarata/nself-plugins):

1. Fork the repository
2. Add your plugin to `plugins/` directory
3. Update `registry.json` with your plugin metadata
4. Submit pull request

### Registry Entry Format

```json
{
  "name": "my-integration",
  "version": "1.0.0",
  "description": "Integration with My Service",
  "author": "Your Name",
  "category": "custom",
  "repository": "https://github.com/yourname/nself-my-integration",
  "downloadUrl": "https://github.com/yourname/nself-my-integration/releases/download/v1.0.0/my-integration-1.0.0.tar.gz",
  "checksum": "sha256:abc123...",
  "minNselfVersion": "0.4.5"
}
```

---

## Best Practices

### Code Quality

1. **Use strict mode**
   ```bash
   set -euo pipefail
   ```

2. **Handle errors gracefully**
   ```bash
   if ! api_get "/items"; then
       log_error "Failed to fetch items"
       exit 1
   fi
   ```

3. **Validate inputs**
   ```bash
   if [[ -z "${1:-}" ]]; then
       log_error "Item ID required"
       exit 1
   fi
   ```

### Security

1. **Never log secrets**
   ```bash
   # Wrong
   log_info "Using API key: $API_KEY"

   # Right
   log_info "API key configured: ${API_KEY:0:4}****"
   ```

2. **Verify webhook signatures**
   Always verify webhook signatures when a secret is configured.

3. **Use parameterized queries**
   ```bash
   # Escape user input for SQL
   escaped_value=$(printf '%s' "$value" | sed "s/'/''/g")
   ```

### Performance

1. **Batch operations**
   ```bash
   # Process items in batches
   BATCH_SIZE=100
   ```

2. **Use incremental sync**
   Only sync changed items when possible.

3. **Handle rate limits**
   ```bash
   # Respect rate limits
   sleep 0.5  # 2 requests per second max
   ```

### Documentation

1. **Document all commands**
   Include `--help` output for every action.

2. **Provide examples**
   Show real-world usage examples.

3. **Document environment variables**
   List all required and optional variables.

---

## Related Documentation

- [Plugin Command](../commands/PLUGIN.md) - Plugin management commands
- [Plugin Overview](index.md) - Plugin system introduction
- [Database Command](../commands/DB.md) - Database operations

---

*Last Updated: January 2026 | Version 0.9.9*
