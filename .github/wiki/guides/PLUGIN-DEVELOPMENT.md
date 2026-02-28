# nself Plugin Development Guide

**Version**: nself v0.8.0
**Last Updated**: January 29, 2026

Complete guide to creating plugins for nself's extensible plugin architecture.

---

## Table of Contents

1. [Overview](#1-overview)
2. [Plugin Types](#2-plugin-types)
3. [Plugin Structure](#3-plugin-structure)
4. [Plugin Manifest (plugin.json)](#4-plugin-manifest-pluginjson)
5. [Creating Your First Plugin](#5-creating-your-first-plugin)
6. [Plugin CLI Commands](#6-plugin-cli-commands)
7. [Database Schema](#7-database-schema)
8. [Data Sync Plugins](#8-data-sync-plugins)
9. [Webhook Handlers](#9-webhook-handlers)
10. [CLI Extension](#10-cli-extension)
11. [Configuration Management](#11-configuration-management)
12. [Testing Plugins](#12-testing-plugins)
13. [Publishing Plugins](#13-publishing-plugins)
14. [Official Plugins](#14-official-plugins)
15. [Plugin Security](#15-plugin-security)
16. [Best Practices](#16-best-practices)

---

## 1. Overview

### What are nself plugins?

**nself plugins** are self-contained extensions that sync external service data into your PostgreSQL database and keep it synchronized in real-time through webhooks.

Unlike Custom Services (CS_N) which are independent backend applications, plugins provide:

- **Schema Sync**: Mirror external service data structures in PostgreSQL
- **Webhook Handling**: Automatic real-time updates from external services
- **Data Validation**: Sanity checks to verify DB matches external service
- **Historical Backfill**: Download historical data on first setup
- **CLI Commands**: Plugin-specific management commands

### Plugin Architecture and Lifecycle

```
┌─────────────────────────────────────────────────────────┐
│                    Plugin Lifecycle                      │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  1. Install      → nself plugin install <name>           │
│                                                           │
│  2. Configure    → nself <name> init                     │
│                  → Edit .env with API keys               │
│                                                           │
│  3. Schema       → nself <name> schema apply             │
│                  → Creates DB tables                     │
│                                                           │
│  4. Sync         → nself <name> sync                     │
│                  → Full initial data sync                │
│                                                           │
│  5. Webhooks     → nself <name> webhook register         │
│                  → Real-time updates                     │
│                                                           │
│  6. Maintain     → nself <name> check                    │
│                  → Verify data integrity                 │
│                                                           │
└─────────────────────────────────────────────────────────┘
```

### Official vs Community Plugins

| Type | Source | Support | Quality |
|------|--------|---------|---------|
| **Official** | `nself-org/plugins` | Full support | Production-tested |
| **Community** | Third-party repos | Community | Varies |
| **Local** | Your filesystem | Self-maintained | Custom |

### Plugin Registry System

The plugin registry is a centralized catalog hosted at:
- **Primary**: `https://plugins.nself.org` (Cloudflare Worker)
- **Fallback**: `https://raw.githubusercontent.com/nself-org/plugins/main/registry.json`

Registry provides:
- Plugin discovery and search
- Version management
- Dependency tracking
- Checksum verification
- Update notifications

---

## 2. Plugin Types

### Data Sync Plugins

**Purpose**: Sync external services to PostgreSQL

**Examples**:
- `nself-stripe` - Stripe billing and payments
- `nself-shopify` - E-commerce store data
- `nself-github` - Repository and issue data

**Capabilities**:
- Database schema creation
- Full data synchronization
- Incremental updates
- Webhook event handling
- Sanity checking

### Service Plugins

**Purpose**: Add new services to nself stack

**Examples**:
- `nself-rabbitmq` - Message queue service
- `nself-elasticsearch` - Full-text search
- `nself-vault` - Secrets management

**Capabilities**:
- Docker Compose service definitions
- Nginx routing configuration
- Service lifecycle management
- Health checking

### CLI Extension Plugins

**Purpose**: Add new commands to nself CLI

**Examples**:
- `nself-deploy-tools` - Advanced deployment utilities
- `nself-analytics` - Usage analytics and reporting
- `nself-testing` - E2E testing framework

**Capabilities**:
- Custom command implementation
- Argument parsing
- Output formatting
- Integration with core commands

### Webhook Plugins

**Purpose**: Handle incoming webhooks from external services

**Examples**:
- `nself-github-hooks` - GitHub webhook processor
- `nself-slack-events` - Slack event subscriptions
- `nself-twilio-webhooks` - Twilio SMS/voice events

**Capabilities**:
- Webhook endpoint registration
- Signature verification
- Event routing
- Database persistence

### Integration Plugins

**Purpose**: Connect to third-party APIs

**Examples**:
- `nself-sendgrid` - Email delivery tracking
- `nself-twilio` - SMS and voice call logs
- `nself-aws-services` - AWS service integration

**Capabilities**:
- API client implementation
- Authentication management
- Rate limiting
- Error handling

---

## 3. Plugin Structure

### Directory Layout

```
my-plugin/
├── plugin.json                 # Plugin manifest (REQUIRED)
├── README.md                   # Documentation (REQUIRED)
├── LICENSE                     # License file (REQUIRED)
│
├── schema/
│   ├── schema.sql              # Database schema
│   ├── migrations/             # Schema migrations
│   │   ├── 001_initial.sql
│   │   └── 002_add_indexes.sql
│   └── seeds/                  # Seed data
│       └── initial_data.sql
│
├── src/
│   ├── sync.ts                 # Data sync logic
│   ├── webhooks.ts             # Webhook handlers
│   ├── client.ts               # API client
│   ├── types.ts                # TypeScript types
│   └── utils.ts                # Utility functions
│
├── commands/
│   ├── sync.sh                 # Sync command
│   ├── check.sh                # Sanity check command
│   ├── webhook.sh              # Webhook management
│   └── status.sh               # Status reporting
│
├── config/
│   ├── default.json            # Default configuration
│   └── schema.json             # Configuration schema
│
├── docker/
│   ├── Dockerfile              # Optional service container
│   └── docker-compose.yml      # Service definition
│
├── tests/
│   ├── unit/
│   │   ├── sync.test.ts
│   │   └── webhooks.test.ts
│   ├── integration/
│   │   └── e2e.test.ts
│   └── fixtures/
│       └── sample_data.json
│
└── scripts/
    ├── install.sh              # Post-install script
    ├── uninstall.sh            # Pre-uninstall script
    └── migrate.sh              # Migration runner
```

### File Descriptions

| File/Directory | Purpose | Required |
|----------------|---------|----------|
| **plugin.json** | Plugin manifest with metadata | ✅ Yes |
| **README.md** | Usage documentation | ✅ Yes |
| **LICENSE** | License information | ✅ Yes |
| **schema/** | Database schema definitions | For DB plugins |
| **src/** | Core plugin logic | For sync plugins |
| **commands/** | CLI command implementations | For CLI plugins |
| **config/** | Configuration files | Optional |
| **docker/** | Container definitions | For service plugins |
| **tests/** | Test suite | Recommended |
| **scripts/** | Install/setup scripts | Optional |

---

## 4. Plugin Manifest (plugin.json)

### Complete Specification

```json
{
  "name": "nself-stripe",
  "version": "1.0.0",
  "description": "Stripe billing and payments sync to PostgreSQL",
  "author": "Your Name <email@example.com>",
  "license": "MIT",
  "homepage": "https://github.com/yourusername/nself-stripe",
  "repository": {
    "type": "git",
    "url": "https://github.com/yourusername/nself-stripe"
  },
  "bugs": "https://github.com/yourusername/nself-stripe/issues",

  "keywords": [
    "stripe",
    "billing",
    "payments",
    "subscriptions"
  ],

  "category": "billing",

  "dependencies": {
    "nself": ">=0.4.8",
    "postgres": ">=14.0",
    "node": ">=18.0"
  },

  "postgresExtensions": [
    "uuid-ossp",
    "pgcrypto"
  ],

  "capabilities": {
    "database": true,
    "webhooks": true,
    "cli": true,
    "service": false
  },

  "configuration": {
    "required": [
      "STRIPE_API_KEY",
      "STRIPE_WEBHOOK_SECRET"
    ],
    "optional": [
      "STRIPE_API_VERSION",
      "STRIPE_SYNC_INTERVAL",
      "STRIPE_WEBHOOK_PATH"
    ],
    "schema": {
      "STRIPE_API_KEY": {
        "type": "string",
        "description": "Stripe secret API key (sk_live_...)",
        "secret": true,
        "validation": "^sk_(test|live)_[a-zA-Z0-9]{24,}$"
      },
      "STRIPE_WEBHOOK_SECRET": {
        "type": "string",
        "description": "Stripe webhook signing secret",
        "secret": true,
        "validation": "^whsec_[a-zA-Z0-9]{32,}$"
      },
      "STRIPE_API_VERSION": {
        "type": "string",
        "description": "Stripe API version",
        "default": "2024-01-01"
      },
      "STRIPE_SYNC_INTERVAL": {
        "type": "integer",
        "description": "Sync interval in minutes",
        "default": 60,
        "min": 15,
        "max": 1440
      },
      "STRIPE_WEBHOOK_PATH": {
        "type": "string",
        "description": "Webhook endpoint path",
        "default": "/webhooks/stripe"
      }
    }
  },

  "permissions": {
    "database": {
      "read": ["public.*"],
      "write": ["stripe_*"],
      "create": ["stripe_*"],
      "drop": ["stripe_*"]
    },
    "network": {
      "outbound": ["api.stripe.com"]
    },
    "filesystem": {
      "read": [".env"],
      "write": ["logs/stripe-*.log"]
    }
  },

  "webhooks": {
    "path": "/webhooks/stripe",
    "events": [
      "customer.*",
      "subscription.*",
      "invoice.*",
      "payment_intent.*",
      "charge.*",
      "refund.*"
    ]
  },

  "database": {
    "schema": "stripe",
    "tables": [
      "stripe_customers",
      "stripe_subscriptions",
      "stripe_invoices",
      "stripe_payment_intents",
      "stripe_charges",
      "stripe_refunds",
      "stripe_events"
    ]
  },

  "commands": {
    "sync": {
      "description": "Sync data from Stripe to PostgreSQL",
      "usage": "nself stripe sync [--since <date>] [--resource <type>]"
    },
    "check": {
      "description": "Verify database matches Stripe",
      "usage": "nself stripe check [--fix]"
    },
    "webhook": {
      "description": "Manage webhook endpoints",
      "usage": "nself stripe webhook <register|test|status|logs>"
    },
    "backfill": {
      "description": "Download historical data",
      "usage": "nself stripe backfill [--from <date>]"
    },
    "status": {
      "description": "Show sync status",
      "usage": "nself stripe status [<resource>]"
    }
  },

  "install": {
    "script": "scripts/install.sh",
    "postInstall": "npm install"
  },

  "uninstall": {
    "script": "scripts/uninstall.sh",
    "keepData": false
  }
}
```

### Minimal Example

For a simple plugin, the minimum required fields:

```json
{
  "name": "nself-myservice",
  "version": "1.0.0",
  "description": "My service integration",
  "author": "Your Name",
  "license": "MIT",

  "dependencies": {
    "nself": ">=0.4.8"
  },

  "capabilities": {
    "database": true,
    "webhooks": false,
    "cli": true,
    "service": false
  },

  "configuration": {
    "required": ["MYSERVICE_API_KEY"]
  }
}
```

---

## 5. Creating Your First Plugin

### Step-by-Step Tutorial

Let's create a simple plugin that syncs data from a fictional "TaskTracker" API.

#### Step 1: Initialize Plugin Scaffold

```bash
# Create plugin directory
mkdir nself-tasktracker
cd nself-tasktracker

# Initialize plugin structure
nself plugin init nself-tasktracker
```

This generates:
```
nself-tasktracker/
├── plugin.json
├── README.md
├── LICENSE
├── schema/
│   └── schema.sql
├── src/
│   └── sync.ts
├── commands/
│   └── sync.sh
└── tests/
    └── test.sh
```

#### Step 2: Define Plugin Manifest

Edit `plugin.json`:

```json
{
  "name": "nself-tasktracker",
  "version": "0.1.0",
  "description": "TaskTracker task sync to PostgreSQL",
  "author": "Your Name <you@example.com>",
  "license": "MIT",

  "dependencies": {
    "nself": ">=0.4.8",
    "postgres": ">=14.0",
    "node": ">=18.0"
  },

  "capabilities": {
    "database": true,
    "webhooks": true,
    "cli": true,
    "service": false
  },

  "configuration": {
    "required": [
      "TASKTRACKER_API_KEY",
      "TASKTRACKER_WORKSPACE_ID"
    ],
    "optional": [
      "TASKTRACKER_SYNC_INTERVAL"
    ],
    "schema": {
      "TASKTRACKER_API_KEY": {
        "type": "string",
        "description": "TaskTracker API key",
        "secret": true
      },
      "TASKTRACKER_WORKSPACE_ID": {
        "type": "string",
        "description": "TaskTracker workspace ID"
      },
      "TASKTRACKER_SYNC_INTERVAL": {
        "type": "integer",
        "description": "Sync interval in minutes",
        "default": 60
      }
    }
  },

  "database": {
    "schema": "tasktracker",
    "tables": [
      "tasktracker_tasks",
      "tasktracker_projects",
      "tasktracker_users"
    ]
  },

  "commands": {
    "sync": {
      "description": "Sync tasks from TaskTracker",
      "usage": "nself tasktracker sync"
    },
    "status": {
      "description": "Show sync status",
      "usage": "nself tasktracker status"
    }
  }
}
```

#### Step 3: Create Database Schema

Edit `schema/schema.sql`:

```sql
-- Create schema
CREATE SCHEMA IF NOT EXISTS tasktracker;

-- Tasks table
CREATE TABLE IF NOT EXISTS tasktracker.tasks (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL CHECK (status IN ('todo', 'in_progress', 'done', 'archived')),
    priority TEXT CHECK (priority IN ('low', 'medium', 'high')),
    project_id TEXT,
    assignee_id TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    due_date TIMESTAMPTZ,

    -- Metadata
    synced_at TIMESTAMPTZ DEFAULT NOW(),
    sync_version INTEGER DEFAULT 1
);

-- Projects table
CREATE TABLE IF NOT EXISTS tasktracker.projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    synced_at TIMESTAMPTZ DEFAULT NOW(),
    sync_version INTEGER DEFAULT 1
);

-- Users table
CREATE TABLE IF NOT EXISTS tasktracker.users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    avatar_url TEXT,
    role TEXT,
    created_at TIMESTAMPTZ NOT NULL,

    synced_at TIMESTAMPTZ DEFAULT NOW(),
    sync_version INTEGER DEFAULT 1
);

-- Foreign keys
ALTER TABLE tasktracker.tasks
    ADD CONSTRAINT fk_project
    FOREIGN KEY (project_id)
    REFERENCES tasktracker.projects(id)
    ON DELETE SET NULL;

ALTER TABLE tasktracker.tasks
    ADD CONSTRAINT fk_assignee
    FOREIGN KEY (assignee_id)
    REFERENCES tasktracker.users(id)
    ON DELETE SET NULL;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasktracker.tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasktracker.tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasktracker.tasks(assignee_id);
CREATE INDEX IF NOT EXISTS idx_tasks_updated ON tasktracker.tasks(updated_at DESC);

-- Sync metadata table
CREATE TABLE IF NOT EXISTS tasktracker.sync_state (
    resource TEXT PRIMARY KEY,
    last_sync_at TIMESTAMPTZ,
    last_sync_cursor TEXT,
    records_synced INTEGER DEFAULT 0,
    sync_errors INTEGER DEFAULT 0,
    last_error TEXT,
    status TEXT DEFAULT 'idle' CHECK (status IN ('idle', 'syncing', 'error'))
);

-- Insert initial sync state
INSERT INTO tasktracker.sync_state (resource) VALUES
    ('tasks'),
    ('projects'),
    ('users')
ON CONFLICT (resource) DO NOTHING;
```

#### Step 4: Implement Sync Logic

Create `src/sync.ts`:

```typescript
import axios from 'axios';
import { Pool } from 'pg';

interface TaskTrackerConfig {
    apiKey: string;
    workspaceId: string;
    apiUrl: string;
}

interface Task {
    id: string;
    title: string;
    description?: string;
    status: string;
    priority?: string;
    project_id?: string;
    assignee_id?: string;
    created_at: string;
    updated_at: string;
    completed_at?: string;
    due_date?: string;
}

class TaskTrackerSync {
    private config: TaskTrackerConfig;
    private db: Pool;

    constructor(config: TaskTrackerConfig, db: Pool) {
        this.config = config;
        this.db = db;
    }

    async syncTasks(since?: Date): Promise<number> {
        console.log('Fetching tasks from TaskTracker...');

        // Update sync state
        await this.db.query(
            `UPDATE tasktracker.sync_state
             SET status = 'syncing', last_error = NULL
             WHERE resource = 'tasks'`
        );

        try {
            // Fetch tasks from API
            const params: any = {
                workspace_id: this.config.workspaceId
            };

            if (since) {
                params.updated_since = since.toISOString();
            }

            const response = await axios.get(
                `${this.config.apiUrl}/tasks`,
                {
                    headers: {
                        'Authorization': `Bearer ${this.config.apiKey}`,
                        'Accept': 'application/json'
                    },
                    params
                }
            );

            const tasks: Task[] = response.data.tasks;

            console.log(`Fetched ${tasks.length} tasks`);

            // Upsert tasks into database
            let synced = 0;
            for (const task of tasks) {
                await this.db.query(`
                    INSERT INTO tasktracker.tasks (
                        id, title, description, status, priority,
                        project_id, assignee_id, created_at, updated_at,
                        completed_at, due_date, synced_at, sync_version
                    ) VALUES (
                        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), 1
                    )
                    ON CONFLICT (id) DO UPDATE SET
                        title = EXCLUDED.title,
                        description = EXCLUDED.description,
                        status = EXCLUDED.status,
                        priority = EXCLUDED.priority,
                        project_id = EXCLUDED.project_id,
                        assignee_id = EXCLUDED.assignee_id,
                        updated_at = EXCLUDED.updated_at,
                        completed_at = EXCLUDED.completed_at,
                        due_date = EXCLUDED.due_date,
                        synced_at = NOW(),
                        sync_version = tasktracker.tasks.sync_version + 1
                `, [
                    task.id,
                    task.title,
                    task.description,
                    task.status,
                    task.priority,
                    task.project_id,
                    task.assignee_id,
                    task.created_at,
                    task.updated_at,
                    task.completed_at,
                    task.due_date
                ]);

                synced++;
            }

            // Update sync state
            await this.db.query(`
                UPDATE tasktracker.sync_state
                SET status = 'idle',
                    last_sync_at = NOW(),
                    records_synced = records_synced + $1
                WHERE resource = 'tasks'
            `, [synced]);

            console.log(`Synced ${synced} tasks successfully`);

            return synced;

        } catch (error: any) {
            // Update error state
            await this.db.query(`
                UPDATE tasktracker.sync_state
                SET status = 'error',
                    sync_errors = sync_errors + 1,
                    last_error = $1
                WHERE resource = 'tasks'
            `, [error.message]);

            throw error;
        }
    }

    async syncProjects(): Promise<number> {
        // Similar implementation for projects
        return 0;
    }

    async syncUsers(): Promise<number> {
        // Similar implementation for users
        return 0;
    }

    async syncAll(): Promise<void> {
        await this.syncUsers();
        await this.syncProjects();
        await this.syncTasks();
    }
}

export default TaskTrackerSync;

// CLI entry point
if (require.main === module) {
    const config: TaskTrackerConfig = {
        apiKey: process.env.TASKTRACKER_API_KEY || '',
        workspaceId: process.env.TASKTRACKER_WORKSPACE_ID || '',
        apiUrl: process.env.TASKTRACKER_API_URL || 'https://api.tasktracker.com/v1'
    };

    const db = new Pool({
        host: process.env.POSTGRES_HOST || 'localhost',
        port: parseInt(process.env.POSTGRES_PORT || '5432'),
        database: process.env.POSTGRES_DB || 'nself',
        user: process.env.POSTGRES_USER || 'postgres',
        password: process.env.POSTGRES_PASSWORD
    });

    const sync = new TaskTrackerSync(config, db);

    sync.syncAll()
        .then(() => {
            console.log('Sync completed successfully');
            process.exit(0);
        })
        .catch((error) => {
            console.error('Sync failed:', error.message);
            process.exit(1);
        })
        .finally(() => {
            db.end();
        });
}
```

#### Step 5: Add CLI Commands

Create `commands/sync.sh`:

```bash
#!/usr/bin/env bash
# sync.sh - Sync command for TaskTracker plugin

set -euo pipefail

PLUGIN_DIR="${PLUGIN_DIR:-$HOME/.nself/plugins/nself-tasktracker}"

# Load environment
if [[ -f ".env" ]]; then
    set -a
    source ".env"
    set +a
fi

# Check required variables
if [[ -z "${TASKTRACKER_API_KEY:-}" ]]; then
    printf "\033[31mError: TASKTRACKER_API_KEY not set\033[0m\n" >&2
    printf "Set in .env: TASKTRACKER_API_KEY=your-api-key\n"
    exit 1
fi

if [[ -z "${TASKTRACKER_WORKSPACE_ID:-}" ]]; then
    printf "\033[31mError: TASKTRACKER_WORKSPACE_ID not set\033[0m\n" >&2
    printf "Set in .env: TASKTRACKER_WORKSPACE_ID=your-workspace-id\n"
    exit 1
fi

# Parse arguments
SINCE=""
RESOURCE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --since)
            SINCE="$2"
            shift 2
            ;;
        --resource)
            RESOURCE="$2"
            shift 2
            ;;
        *)
            shift
            ;;
    esac
done

# Run sync
printf "Starting TaskTracker sync...\n"

cd "$PLUGIN_DIR"

if [[ -n "$SINCE" ]]; then
    export SYNC_SINCE="$SINCE"
fi

if [[ -n "$RESOURCE" ]]; then
    export SYNC_RESOURCE="$RESOURCE"
fi

# Run TypeScript sync
if command -v node >/dev/null 2>&1; then
    npx ts-node src/sync.ts
else
    printf "\033[31mError: Node.js not found\033[0m\n" >&2
    exit 1
fi
```

Create `commands/status.sh`:

```bash
#!/usr/bin/env bash
# status.sh - Status command for TaskTracker plugin

set -euo pipefail

# Database connection
DB_CONTAINER="${PROJECT_NAME:-nself}_postgres"

if ! docker ps --format "{{.Names}}" 2>/dev/null | grep -q "^${DB_CONTAINER}$"; then
    printf "\033[31mError: PostgreSQL container not running\033[0m\n" >&2
    exit 1
fi

# Query sync state
docker exec "$DB_CONTAINER" psql \
    -U "${POSTGRES_USER:-postgres}" \
    -d "${POSTGRES_DB:-nself}" \
    -c "SELECT
            resource,
            last_sync_at,
            records_synced,
            sync_errors,
            status,
            last_error
        FROM tasktracker.sync_state
        ORDER BY resource;" \
    2>/dev/null

printf "\n"
printf "Run 'nself tasktracker sync' to sync data\n"
```

#### Step 6: Write Tests

Create `tests/sync.test.ts`:

```typescript
import { describe, it, expect, beforeAll, afterAll } from '@jest/globals';
import { Pool } from 'pg';
import TaskTrackerSync from '../src/sync';

describe('TaskTracker Sync', () => {
    let db: Pool;
    let sync: TaskTrackerSync;

    beforeAll(async () => {
        db = new Pool({
            host: 'localhost',
            port: 5432,
            database: 'test_nself',
            user: 'postgres',
            password: 'test'
        });

        // Apply schema
        const schema = require('fs').readFileSync('schema/schema.sql', 'utf8');
        await db.query(schema);

        sync = new TaskTrackerSync({
            apiKey: 'test_key',
            workspaceId: 'test_workspace',
            apiUrl: 'http://localhost:3000'
        }, db);
    });

    afterAll(async () => {
        await db.end();
    });

    it('should sync tasks', async () => {
        const count = await sync.syncTasks();
        expect(count).toBeGreaterThanOrEqual(0);
    });

    it('should handle sync errors', async () => {
        // Test error handling
    });
});
```

#### Step 7: Publish Plugin

Once tested, publish to the registry:

```bash
# Validate plugin structure
nself plugin validate

# Test locally first
nself plugin install .

# Publish to registry (requires registry account)
nself plugin publish
```

---

## 6. Plugin CLI Commands

### User Commands

Users interact with plugins through the `nself plugin` command:

```bash
# Discovery
nself plugin list                    # List all available plugins
nself plugin list --installed        # Show only installed plugins
nself plugin list --category billing # Filter by category
nself plugin search stripe           # Search for plugins
nself plugin info nself-stripe       # Show plugin details

# Installation
nself plugin install nself-stripe    # Install from registry
nself plugin install nself-stripe@1.2.0  # Specific version
nself plugin install ./my-plugin     # Install local plugin
nself plugin uninstall nself-stripe  # Remove plugin

# Management
nself plugin status                  # Show all plugin status
nself plugin status nself-stripe     # Specific plugin status
nself plugin update                  # Update all plugins
nself plugin update nself-stripe     # Update specific plugin

# Configuration
nself plugin config nself-stripe     # Configure plugin
nself plugin refresh                 # Refresh registry cache
```

### Plugin-Specific Commands

Once installed, plugins add their own commands:

```bash
# Stripe plugin commands
nself stripe sync                    # Full sync
nself stripe sync --since 2024-01-01 # Incremental
nself stripe check                   # Sanity check
nself stripe check --fix             # Auto-fix discrepancies
nself stripe webhook register        # Register webhook
nself stripe webhook test            # Test webhook
nself stripe status                  # Show status

# TaskTracker plugin commands
nself tasktracker sync               # Sync tasks
nself tasktracker status             # Show status
```

### Implementing Plugin Commands

Commands are Bash scripts in the `commands/` directory:

```bash
commands/
├── sync.sh      # nself <plugin> sync
├── check.sh     # nself <plugin> check
├── webhook.sh   # nself <plugin> webhook
└── status.sh    # nself <plugin> status
```

Each command receives arguments and environment:

```bash
#!/usr/bin/env bash
# commands/mycommand.sh

# Available environment variables:
# - PLUGIN_DIR: Plugin installation directory
# - NSELF_PROJECT_DIR: Current project directory
# - All .env variables

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --flag)
            FLAG="true"
            shift
            ;;
        *)
            ARGUMENT="$1"
            shift
            ;;
    esac
done

# Implement command logic
printf "Running command with flag: %s\n" "$FLAG"
```

---

## 7. Database Schema

### Schema Design Principles

1. **Use a dedicated schema** (not `public`)
2. **Prefix all tables** with plugin name
3. **Include sync metadata** (synced_at, sync_version)
4. **Add appropriate indexes**
5. **Use foreign keys** for relationships
6. **Add check constraints** for validation

### Example Schema

```sql
-- Create dedicated schema
CREATE SCHEMA IF NOT EXISTS stripe;

-- Main data table
CREATE TABLE IF NOT EXISTS stripe.customers (
    -- Primary data (from Stripe)
    id TEXT PRIMARY KEY,
    email TEXT,
    name TEXT,
    description TEXT,
    phone TEXT,
    address JSONB,
    metadata JSONB,
    created TIMESTAMPTZ NOT NULL,

    -- Subscription info
    default_source TEXT,
    currency TEXT,
    balance INTEGER,
    delinquent BOOLEAN DEFAULT false,

    -- Sync metadata
    synced_at TIMESTAMPTZ DEFAULT NOW(),
    sync_version INTEGER DEFAULT 1,
    is_deleted BOOLEAN DEFAULT false
);

-- Indexes
CREATE INDEX idx_customers_email ON stripe.customers(email);
CREATE INDEX idx_customers_created ON stripe.customers(created DESC);
CREATE INDEX idx_customers_synced ON stripe.customers(synced_at DESC);

-- Sync state tracking
CREATE TABLE IF NOT EXISTS stripe.sync_state (
    resource TEXT PRIMARY KEY,
    last_sync_at TIMESTAMPTZ,
    last_sync_cursor TEXT,
    records_synced INTEGER DEFAULT 0,
    sync_errors INTEGER DEFAULT 0,
    last_error TEXT,
    status TEXT DEFAULT 'idle' CHECK (status IN ('idle', 'syncing', 'error'))
);
```

### Migrations

Create migrations in `schema/migrations/`:

```sql
-- schema/migrations/001_initial.sql
CREATE SCHEMA IF NOT EXISTS stripe;
CREATE TABLE stripe.customers (...);

-- schema/migrations/002_add_indexes.sql
CREATE INDEX idx_customers_email ON stripe.customers(email);

-- schema/migrations/003_add_subscriptions.sql
CREATE TABLE stripe.subscriptions (...);
```

Migration runner in `scripts/migrate.sh`:

```bash
#!/usr/bin/env bash
# Run all migrations

for migration in schema/migrations/*.sql; do
    printf "Running %s...\n" "$(basename "$migration")"
    psql "$DATABASE_URL" -f "$migration"
done
```

### Namespacing

Use plugin-specific schema to avoid conflicts:

```sql
-- ✅ GOOD: Dedicated schema
CREATE SCHEMA stripe;
CREATE TABLE stripe.customers (...);

-- ❌ BAD: Using public schema
CREATE TABLE stripe_customers (...);
```

### Relationships

Define relationships with foreign keys:

```sql
-- Parent table
CREATE TABLE stripe.customers (
    id TEXT PRIMARY KEY,
    ...
);

-- Child table with FK
CREATE TABLE stripe.subscriptions (
    id TEXT PRIMARY KEY,
    customer_id TEXT NOT NULL,
    ...

    CONSTRAINT fk_customer
        FOREIGN KEY (customer_id)
        REFERENCES stripe.customers(id)
        ON DELETE CASCADE
);
```

### Versioning

Track schema versions in sync metadata:

```sql
-- Schema version table
CREATE TABLE stripe.schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMPTZ DEFAULT NOW(),
    migration_file TEXT NOT NULL
);

-- Record migrations
INSERT INTO stripe.schema_version (version, migration_file)
VALUES (1, '001_initial.sql');
```

---

## 8. Data Sync Plugins

### API Client Implementation

Create a robust API client with proper error handling:

```typescript
import axios, { AxiosInstance, AxiosRequestConfig } from 'axios';

interface ClientConfig {
    apiKey: string;
    apiUrl: string;
    timeout?: number;
    retries?: number;
}

class APIClient {
    private client: AxiosInstance;
    private config: ClientConfig;

    constructor(config: ClientConfig) {
        this.config = {
            timeout: 30000,
            retries: 3,
            ...config
        };

        this.client = axios.create({
            baseURL: config.apiUrl,
            timeout: this.config.timeout,
            headers: {
                'Authorization': `Bearer ${config.apiKey}`,
                'Accept': 'application/json',
                'User-Agent': 'nself-plugin/1.0'
            }
        });

        // Add retry interceptor
        this.client.interceptors.response.use(
            response => response,
            async error => {
                const config = error.config;

                if (!config || !config.retries) {
                    config.retries = 0;
                }

                if (config.retries < this.config.retries!) {
                    config.retries++;

                    // Exponential backoff
                    const delay = Math.pow(2, config.retries) * 1000;
                    await new Promise(resolve => setTimeout(resolve, delay));

                    return this.client(config);
                }

                return Promise.reject(error);
            }
        );
    }

    async get<T>(path: string, params?: any): Promise<T> {
        const response = await this.client.get(path, { params });
        return response.data;
    }

    async post<T>(path: string, data?: any): Promise<T> {
        const response = await this.client.post(path, data);
        return response.data;
    }

    async paginate<T>(
        path: string,
        params?: any,
        pageKey: string = 'page'
    ): Promise<T[]> {
        const results: T[] = [];
        let page = 1;
        let hasMore = true;

        while (hasMore) {
            const response: any = await this.get(path, {
                ...params,
                [pageKey]: page
            });

            results.push(...response.data);
            hasMore = response.has_more || false;
            page++;
        }

        return results;
    }
}

export default APIClient;
```

### Incremental Sync vs Full Sync

```typescript
class DataSync {
    async fullSync(): Promise<void> {
        console.log('Performing full sync...');

        // Clear existing data (optional)
        await this.db.query('TRUNCATE stripe.customers CASCADE');

        // Fetch all data
        const customers = await this.client.paginate('/customers');

        // Insert all records
        for (const customer of customers) {
            await this.insertCustomer(customer);
        }
    }

    async incrementalSync(since: Date): Promise<void> {
        console.log(`Performing incremental sync since ${since}...`);

        // Fetch only updated records
        const customers = await this.client.get('/customers', {
            updated_since: since.toISOString()
        });

        // Upsert records
        for (const customer of customers.data) {
            await this.upsertCustomer(customer);
        }
    }

    async sync(): Promise<void> {
        // Get last sync time
        const result = await this.db.query(
            'SELECT last_sync_at FROM stripe.sync_state WHERE resource = $1',
            ['customers']
        );

        const lastSync = result.rows[0]?.last_sync_at;

        if (!lastSync) {
            // First sync - do full
            await this.fullSync();
        } else {
            // Subsequent syncs - incremental
            await this.incrementalSync(new Date(lastSync));
        }

        // Update sync time
        await this.db.query(
            'UPDATE stripe.sync_state SET last_sync_at = NOW() WHERE resource = $1',
            ['customers']
        );
    }
}
```

### Handling Rate Limits

```typescript
class RateLimiter {
    private requests: number = 0;
    private resetTime: number = Date.now();
    private limit: number;
    private window: number;

    constructor(limit: number = 100, window: number = 60000) {
        this.limit = limit;
        this.window = window;
    }

    async checkLimit(): Promise<void> {
        const now = Date.now();

        // Reset counter if window expired
        if (now - this.resetTime > this.window) {
            this.requests = 0;
            this.resetTime = now;
        }

        // Wait if limit reached
        if (this.requests >= this.limit) {
            const waitTime = this.window - (now - this.resetTime);
            console.log(`Rate limit reached. Waiting ${waitTime}ms...`);
            await new Promise(resolve => setTimeout(resolve, waitTime));

            this.requests = 0;
            this.resetTime = Date.now();
        }

        this.requests++;
    }

    async execute<T>(fn: () => Promise<T>): Promise<T> {
        await this.checkLimit();
        return fn();
    }
}

// Usage
const limiter = new RateLimiter(100, 60000); // 100 req/min

for (const item of items) {
    await limiter.execute(async () => {
        return this.client.get(`/items/${item.id}`);
    });
}
```

### Error Recovery and Retries

```typescript
class SyncManager {
    async syncWithRecovery(): Promise<void> {
        // Save checkpoint before sync
        const checkpoint = await this.createCheckpoint();

        try {
            await this.sync();

            // Mark checkpoint as successful
            await this.markCheckpointSuccess(checkpoint);

        } catch (error) {
            console.error('Sync failed:', error);

            // Save error details
            await this.db.query(
                'UPDATE stripe.sync_state SET last_error = $1, sync_errors = sync_errors + 1',
                [error.message]
            );

            // Rollback to checkpoint
            await this.rollbackToCheckpoint(checkpoint);

            throw error;
        }
    }

    async createCheckpoint(): Promise<string> {
        const checkpointId = `checkpoint_${Date.now()}`;

        // Save current state
        await this.db.query(
            'INSERT INTO stripe.checkpoints (id, created_at, state) VALUES ($1, NOW(), $2)',
            [checkpointId, JSON.stringify(await this.getState())]
        );

        return checkpointId;
    }

    async rollbackToCheckpoint(checkpointId: string): Promise<void> {
        // Restore state from checkpoint
        const result = await this.db.query(
            'SELECT state FROM stripe.checkpoints WHERE id = $1',
            [checkpointId]
        );

        if (result.rows.length > 0) {
            const state = JSON.parse(result.rows[0].state);
            await this.restoreState(state);
        }
    }
}
```

### Sync Scheduling

Add cron-style scheduling for automatic syncs:

```typescript
import cron from 'node-cron';

class SyncScheduler {
    private tasks: Map<string, cron.ScheduledTask> = new Map();

    scheduleSync(resource: string, schedule: string): void {
        // Cancel existing task
        this.cancelSync(resource);

        // Schedule new task
        const task = cron.schedule(schedule, async () => {
            console.log(`Running scheduled sync for ${resource}...`);

            try {
                await this.sync(resource);
            } catch (error) {
                console.error(`Scheduled sync failed for ${resource}:`, error);
            }
        });

        this.tasks.set(resource, task);
        console.log(`Scheduled ${resource} sync: ${schedule}`);
    }

    cancelSync(resource: string): void {
        const task = this.tasks.get(resource);
        if (task) {
            task.stop();
            this.tasks.delete(resource);
        }
    }

    cancelAll(): void {
        for (const [resource, task] of this.tasks) {
            task.stop();
        }
        this.tasks.clear();
    }
}

// Usage
const scheduler = new SyncScheduler();

// Sync every hour
scheduler.scheduleSync('customers', '0 * * * *');

// Sync every 15 minutes
scheduler.scheduleSync('subscriptions', '*/15 * * * *');

// Sync daily at midnight
scheduler.scheduleSync('invoices', '0 0 * * *');
```

### Conflict Resolution

Handle conflicts when local data differs from remote:

```typescript
enum ConflictResolution {
    REMOTE_WINS = 'remote_wins',
    LOCAL_WINS = 'local_wins',
    NEWEST_WINS = 'newest_wins',
    MANUAL = 'manual'
}

class ConflictResolver {
    async resolveConflict(
        local: any,
        remote: any,
        strategy: ConflictResolution
    ): Promise<any> {
        switch (strategy) {
            case ConflictResolution.REMOTE_WINS:
                return remote;

            case ConflictResolution.LOCAL_WINS:
                return local;

            case ConflictResolution.NEWEST_WINS:
                const localTime = new Date(local.updated_at).getTime();
                const remoteTime = new Date(remote.updated_at).getTime();
                return remoteTime > localTime ? remote : local;

            case ConflictResolution.MANUAL:
                // Log conflict for manual resolution
                await this.logConflict(local, remote);
                return null;

            default:
                return remote;
        }
    }

    async logConflict(local: any, remote: any): Promise<void> {
        await this.db.query(`
            INSERT INTO stripe.conflicts (
                resource_type,
                resource_id,
                local_data,
                remote_data,
                created_at
            ) VALUES ($1, $2, $3, $4, NOW())
        `, [
            'customer',
            local.id,
            JSON.stringify(local),
            JSON.stringify(remote)
        ]);
    }
}
```

---

## 9. Webhook Handlers

### Registering Webhook Endpoints

```typescript
class WebhookManager {
    async registerWebhook(): Promise<void> {
        const webhookUrl = this.getWebhookUrl();

        console.log(`Registering webhook at ${webhookUrl}...`);

        try {
            const response = await this.client.post('/webhooks', {
                url: webhookUrl,
                enabled_events: [
                    'customer.created',
                    'customer.updated',
                    'customer.deleted',
                    'subscription.*',
                    'invoice.*'
                ],
                description: 'nself-stripe webhook'
            });

            const webhookSecret = response.secret;

            console.log('Webhook registered successfully');
            console.log(`Webhook ID: ${response.id}`);
            console.log(`Add to .env: STRIPE_WEBHOOK_SECRET=${webhookSecret}`);

        } catch (error) {
            console.error('Failed to register webhook:', error);
            throw error;
        }
    }

    getWebhookUrl(): string {
        const domain = process.env.BASE_DOMAIN || 'localhost';
        const path = process.env.STRIPE_WEBHOOK_PATH || '/webhooks/stripe';

        return `https://api.${domain}${path}`;
    }
}
```

### Signature Verification

```typescript
import crypto from 'crypto';

class WebhookVerifier {
    verifySignature(
        payload: string,
        signature: string,
        secret: string
    ): boolean {
        // Extract timestamp and signatures
        const parts = signature.split(',');
        const timestamp = parts.find(p => p.startsWith('t='))?.slice(2);
        const sigs = parts.filter(p => p.startsWith('v1='));

        if (!timestamp || sigs.length === 0) {
            return false;
        }

        // Compute expected signature
        const signedPayload = `${timestamp}.${payload}`;
        const expected = crypto
            .createHmac('sha256', secret)
            .update(signedPayload, 'utf8')
            .digest('hex');

        // Check if any signature matches
        return sigs.some(sig => {
            const actual = sig.slice(3);
            return crypto.timingSafeEqual(
                Buffer.from(expected),
                Buffer.from(actual)
            );
        });
    }

    checkTimestamp(signature: string, tolerance: number = 300): boolean {
        const parts = signature.split(',');
        const timestamp = parts.find(p => p.startsWith('t='))?.slice(2);

        if (!timestamp) {
            return false;
        }

        const now = Math.floor(Date.now() / 1000);
        const diff = now - parseInt(timestamp);

        return diff <= tolerance;
    }
}
```

### Event Processing

```typescript
interface WebhookEvent {
    id: string;
    type: string;
    data: {
        object: any;
        previous_attributes?: any;
    };
    created: number;
}

class WebhookProcessor {
    async processEvent(event: WebhookEvent): Promise<void> {
        console.log(`Processing event: ${event.type} (${event.id})`);

        // Log event for audit trail
        await this.logEvent(event);

        // Route to handler
        switch (event.type) {
            case 'customer.created':
                await this.handleCustomerCreated(event);
                break;

            case 'customer.updated':
                await this.handleCustomerUpdated(event);
                break;

            case 'customer.deleted':
                await this.handleCustomerDeleted(event);
                break;

            case 'subscription.created':
            case 'subscription.updated':
                await this.handleSubscriptionUpdated(event);
                break;

            default:
                console.log(`Unhandled event type: ${event.type}`);
        }
    }

    async handleCustomerCreated(event: WebhookEvent): Promise<void> {
        const customer = event.data.object;

        await this.db.query(`
            INSERT INTO stripe.customers (
                id, email, name, description, created, synced_at
            ) VALUES ($1, $2, $3, $4, to_timestamp($5), NOW())
            ON CONFLICT (id) DO NOTHING
        `, [
            customer.id,
            customer.email,
            customer.name,
            customer.description,
            customer.created
        ]);

        console.log(`Customer created: ${customer.id}`);
    }

    async handleCustomerUpdated(event: WebhookEvent): Promise<void> {
        const customer = event.data.object;

        await this.db.query(`
            UPDATE stripe.customers
            SET email = $2,
                name = $3,
                description = $4,
                synced_at = NOW(),
                sync_version = sync_version + 1
            WHERE id = $1
        `, [
            customer.id,
            customer.email,
            customer.name,
            customer.description
        ]);

        console.log(`Customer updated: ${customer.id}`);
    }

    async handleCustomerDeleted(event: WebhookEvent): Promise<void> {
        const customer = event.data.object;

        await this.db.query(`
            UPDATE stripe.customers
            SET is_deleted = true,
                synced_at = NOW()
            WHERE id = $1
        `, [customer.id]);

        console.log(`Customer deleted: ${customer.id}`);
    }

    async logEvent(event: WebhookEvent): Promise<void> {
        await this.db.query(`
            INSERT INTO stripe.events (
                id, type, data, created_at, processed_at
            ) VALUES ($1, $2, $3, to_timestamp($4), NOW())
            ON CONFLICT (id) DO NOTHING
        `, [
            event.id,
            event.type,
            JSON.stringify(event.data),
            event.created
        ]);
    }
}
```

### Idempotency

Ensure webhooks are processed exactly once:

```typescript
class IdempotencyManager {
    async isProcessed(eventId: string): Promise<boolean> {
        const result = await this.db.query(
            'SELECT id FROM stripe.events WHERE id = $1',
            [eventId]
        );

        return result.rows.length > 0;
    }

    async processOnce(
        eventId: string,
        handler: () => Promise<void>
    ): Promise<void> {
        // Check if already processed
        if (await this.isProcessed(eventId)) {
            console.log(`Event ${eventId} already processed, skipping`);
            return;
        }

        // Process with transaction
        const client = await this.db.connect();

        try {
            await client.query('BEGIN');

            // Execute handler
            await handler();

            // Mark as processed
            await client.query(
                'INSERT INTO stripe.events (id, processed_at) VALUES ($1, NOW()) ON CONFLICT DO NOTHING',
                [eventId]
            );

            await client.query('COMMIT');

        } catch (error) {
            await client.query('ROLLBACK');
            throw error;

        } finally {
            client.release();
        }
    }
}
```

### Error Handling

```typescript
class WebhookErrorHandler {
    async handleError(
        event: WebhookEvent,
        error: Error
    ): Promise<void> {
        console.error(`Error processing event ${event.id}:`, error);

        // Log error
        await this.db.query(`
            INSERT INTO stripe.webhook_errors (
                event_id,
                event_type,
                error_message,
                error_stack,
                created_at,
                retry_count
            ) VALUES ($1, $2, $3, $4, NOW(), 0)
            ON CONFLICT (event_id) DO UPDATE SET
                retry_count = stripe.webhook_errors.retry_count + 1,
                last_retry_at = NOW()
        `, [
            event.id,
            event.type,
            error.message,
            error.stack
        ]);

        // Queue for retry if transient error
        if (this.isTransientError(error)) {
            await this.queueRetry(event);
        }
    }

    isTransientError(error: Error): boolean {
        // Network errors, timeouts, rate limits
        return error.message.includes('ECONNREFUSED') ||
               error.message.includes('timeout') ||
               error.message.includes('rate limit');
    }

    async queueRetry(event: WebhookEvent): Promise<void> {
        await this.db.query(`
            INSERT INTO stripe.webhook_retry_queue (
                event_id,
                event_data,
                scheduled_at
            ) VALUES ($1, $2, NOW() + INTERVAL '5 minutes')
        `, [
            event.id,
            JSON.stringify(event)
        ]);
    }
}
```

---

## 10. CLI Extension

### Adding Custom Commands

Commands are shell scripts that follow nself conventions:

```bash
#!/usr/bin/env bash
# commands/mycommand.sh - Custom command implementation

set -euo pipefail

# Available environment variables:
# - PLUGIN_DIR: Plugin installation directory
# - NSELF_PROJECT_DIR: Current project directory
# - All variables from .env

# Source display utilities if available
if [[ -f "$PLUGIN_DIR/../_shared/display.sh" ]]; then
    source "$PLUGIN_DIR/../_shared/display.sh"
else
    # Fallback functions
    log_success() { printf "\033[32m✓ %s\033[0m\n" "$1"; }
    log_error() { printf "\033[31m✗ %s\033[0m\n" "$1" >&2; }
    log_info() { printf "\033[34mℹ %s\033[0m\n" "$1"; }
    log_warning() { printf "\033[33m⚠ %s\033[0m\n" "$1"; }
fi

# Command implementation
main() {
    log_info "Running custom command..."

    # Command logic here

    log_success "Command completed"
}

main "$@"
```

### Argument Parsing

```bash
#!/usr/bin/env bash
# Robust argument parsing

# Default values
OPTION=""
FLAG=false
POSITIONAL_ARGS=()

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        -o|--option)
            OPTION="$2"
            shift 2
            ;;
        -f|--flag)
            FLAG=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        -*)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
        *)
            POSITIONAL_ARGS+=("$1")
            shift
            ;;
    esac
done

# Restore positional parameters
set -- "${POSITIONAL_ARGS[@]}"

# Validate required arguments
if [[ -z "$OPTION" ]]; then
    log_error "Option is required"
    show_help
    exit 1
fi
```

### Output Formatting

```bash
# Colored output
log_success() {
    printf "\033[32m✓ %s\033[0m\n" "$1"
}

log_error() {
    printf "\033[31m✗ %s\033[0m\n" "$1" >&2
}

log_info() {
    printf "\033[34mℹ %s\033[0m\n" "$1"
}

log_warning() {
    printf "\033[33m⚠ %s\033[0m\n" "$1"
}

# Table output
print_table() {
    local headers=("$@")

    # Header
    printf "%-15s %-20s %-10s\n" "${headers[@]}"
    printf "%-15s %-20s %-10s\n" "---------------" "--------------------" "----------"

    # Rows (read from stdin)
    while IFS='|' read -r col1 col2 col3; do
        printf "%-15s %-20s %-10s\n" "$col1" "$col2" "$col3"
    done
}

# Usage
printf "value1|value2|value3\n" | print_table "Column 1" "Column 2" "Column 3"
```

### Error Handling

```bash
#!/usr/bin/env bash
set -euo pipefail  # Exit on error, undefined var, pipe failure

# Trap errors
trap 'error_handler $? $LINENO' ERR

error_handler() {
    local exit_code=$1
    local line_num=$2

    log_error "Command failed at line $line_num with exit code $exit_code"

    # Cleanup on error
    cleanup

    exit "$exit_code"
}

cleanup() {
    # Cleanup logic
    log_info "Cleaning up..."
}

# Validate prerequisites
check_prerequisites() {
    local missing=0

    if ! command -v docker >/dev/null 2>&1; then
        log_error "Docker is required but not installed"
        ((missing++))
    fi

    if [[ -z "${REQUIRED_VAR:-}" ]]; then
        log_error "REQUIRED_VAR is not set"
        ((missing++))
    fi

    if [[ $missing -gt 0 ]]; then
        log_error "Missing $missing prerequisites"
        return 1
    fi

    return 0
}

# Main execution
main() {
    check_prerequisites || exit 1

    # Command logic

    log_success "Done"
}

main "$@"
```

### Help Text

```bash
show_help() {
    cat << EOF
Usage: nself myplugin mycommand [options] [arguments]

Description:
  Detailed description of what this command does.

Options:
  -o, --option VALUE    Option description
  -f, --flag            Flag description
  -h, --help            Show this help message

Arguments:
  ARG1                  First argument description
  ARG2                  Second argument (optional)

Examples:
  nself myplugin mycommand --option value arg1
  nself myplugin mycommand --flag arg1 arg2

Environment Variables:
  PLUGIN_VAR1           Description of variable
  PLUGIN_VAR2           Another variable

For more information: https://docs.nself.org/plugins/myplugin
EOF
}
```

---

## 11. Configuration Management

### Plugin Settings

Configuration is managed through environment variables in `.env`:

```bash
# .env
# TaskTracker Plugin Configuration
TASKTRACKER_API_KEY=your-api-key-here
TASKTRACKER_WORKSPACE_ID=workspace-123
TASKTRACKER_API_URL=https://api.tasktracker.com/v1
TASKTRACKER_SYNC_INTERVAL=60
TASKTRACKER_WEBHOOK_PATH=/webhooks/tasktracker
```

### Configuration Schema

Define schema in `plugin.json`:

```json
{
  "configuration": {
    "required": ["TASKTRACKER_API_KEY"],
    "optional": ["TASKTRACKER_SYNC_INTERVAL"],
    "schema": {
      "TASKTRACKER_API_KEY": {
        "type": "string",
        "description": "API key for TaskTracker",
        "secret": true,
        "validation": "^tt_[a-zA-Z0-9]{32}$"
      },
      "TASKTRACKER_WORKSPACE_ID": {
        "type": "string",
        "description": "Workspace ID",
        "validation": "^[a-z0-9-]+$"
      },
      "TASKTRACKER_SYNC_INTERVAL": {
        "type": "integer",
        "description": "Sync interval in minutes",
        "default": 60,
        "min": 15,
        "max": 1440
      }
    }
  }
}
```

### Secrets Management

Keep secrets secure:

1. **Never commit secrets** to git
2. **Use .env.secrets** for sensitive data
3. **Validate secrets** format
4. **Encrypt secrets** at rest if needed

```bash
# Check if secrets are set
check_secrets() {
    local missing=0

    if [[ -z "${API_KEY:-}" ]]; then
        log_error "API_KEY not set in .env"
        printf "  Add: API_KEY=your-key-here\n"
        ((missing++))
    fi

    if [[ -z "${WEBHOOK_SECRET:-}" ]]; then
        log_error "WEBHOOK_SECRET not set in .env"
        printf "  Add: WEBHOOK_SECRET=your-secret-here\n"
        ((missing++))
    fi

    if [[ $missing -gt 0 ]]; then
        log_error "Missing $missing required secrets"
        return 1
    fi

    return 0
}
```

### Environment-Specific Configuration

Support multiple environments:

```bash
# .env.dev
TASKTRACKER_API_URL=https://sandbox.tasktracker.com/v1
TASKTRACKER_SYNC_INTERVAL=15

# .env.prod
TASKTRACKER_API_URL=https://api.tasktracker.com/v1
TASKTRACKER_SYNC_INTERVAL=60
```

Load based on `ENV` variable:

```bash
# Load environment-specific config
load_env() {
    local env="${ENV:-dev}"
    local env_file=".env.${env}"

    if [[ -f ".env" ]]; then
        set -a
        source ".env"
        set +a
    fi

    if [[ -f "$env_file" ]]; then
        set -a
        source "$env_file"
        set +a
        log_info "Loaded $env_file"
    fi
}
```

### Configuration Validation

```bash
validate_config() {
    local errors=0

    # Check required variables
    for var in "${REQUIRED_VARS[@]}"; do
        if [[ -z "${!var:-}" ]]; then
            log_error "Required variable not set: $var"
            ((errors++))
        fi
    done

    # Validate formats
    if [[ -n "${API_KEY:-}" ]]; then
        if [[ ! "$API_KEY" =~ ^tt_[a-zA-Z0-9]{32}$ ]]; then
            log_error "Invalid API_KEY format"
            ((errors++))
        fi
    fi

    # Validate ranges
    if [[ -n "${SYNC_INTERVAL:-}" ]]; then
        if (( SYNC_INTERVAL < 15 || SYNC_INTERVAL > 1440 )); then
            log_error "SYNC_INTERVAL must be between 15 and 1440"
            ((errors++))
        fi
    fi

    if [[ $errors -gt 0 ]]; then
        log_error "Configuration validation failed with $errors errors"
        return 1
    fi

    log_success "Configuration valid"
    return 0
}
```

---

## 12. Testing Plugins

### Unit Tests

Test individual functions and modules:

```typescript
// tests/unit/sync.test.ts
import { describe, it, expect, jest, beforeEach } from '@jest/globals';
import TaskTrackerSync from '../../src/sync';
import APIClient from '../../src/client';

// Mock dependencies
jest.mock('../../src/client');

describe('TaskTrackerSync', () => {
    let sync: TaskTrackerSync;
    let mockClient: jest.Mocked<APIClient>;
    let mockDb: any;

    beforeEach(() => {
        mockClient = new APIClient({
            apiKey: 'test',
            apiUrl: 'http://test'
        }) as jest.Mocked<APIClient>;

        mockDb = {
            query: jest.fn()
        };

        sync = new TaskTrackerSync(
            { apiKey: 'test', workspaceId: 'test', apiUrl: 'http://test' },
            mockDb
        );
    });

    describe('syncTasks', () => {
        it('should fetch and sync tasks', async () => {
            // Mock API response
            mockClient.get = jest.fn().mockResolvedValue({
                tasks: [
                    {
                        id: 'task_1',
                        title: 'Test Task',
                        status: 'todo',
                        created_at: '2024-01-01T00:00:00Z',
                        updated_at: '2024-01-01T00:00:00Z'
                    }
                ]
            });

            mockDb.query.mockResolvedValue({ rows: [] });

            const count = await sync.syncTasks();

            expect(count).toBe(1);
            expect(mockDb.query).toHaveBeenCalled();
        });

        it('should handle API errors', async () => {
            mockClient.get = jest.fn().mockRejectedValue(
                new Error('API error')
            );

            await expect(sync.syncTasks()).rejects.toThrow('API error');
        });

        it('should handle incremental sync', async () => {
            const since = new Date('2024-01-01');

            mockClient.get = jest.fn().mockResolvedValue({
                tasks: []
            });

            await sync.syncTasks(since);

            expect(mockClient.get).toHaveBeenCalledWith(
                expect.anything(),
                expect.objectContaining({
                    updated_since: since.toISOString()
                })
            );
        });
    });
});
```

### Integration Tests

Test end-to-end plugin functionality:

```typescript
// tests/integration/e2e.test.ts
import { describe, it, expect, beforeAll, afterAll } from '@jest/globals';
import { Pool } from 'pg';
import { exec } from 'child_process';
import { promisify } from 'util';

const execAsync = promisify(exec);

describe('TaskTracker Plugin E2E', () => {
    let db: Pool;

    beforeAll(async () => {
        // Setup test database
        db = new Pool({
            host: 'localhost',
            port: 5432,
            database: 'test_nself',
            user: 'postgres',
            password: 'test'
        });

        // Apply schema
        await execAsync('psql test_nself < schema/schema.sql');
    });

    afterAll(async () => {
        // Cleanup
        await db.end();
    });

    it('should install plugin', async () => {
        const { stdout } = await execAsync('nself plugin install .');
        expect(stdout).toContain('installed successfully');
    });

    it('should apply schema', async () => {
        await execAsync('nself tasktracker schema apply');

        const result = await db.query(`
            SELECT table_name
            FROM information_schema.tables
            WHERE table_schema = 'tasktracker'
        `);

        expect(result.rows.length).toBeGreaterThan(0);
    });

    it('should sync data', async () => {
        const { stdout } = await execAsync('nself tasktracker sync');
        expect(stdout).toContain('Synced');

        const result = await db.query('SELECT COUNT(*) FROM tasktracker.tasks');
        expect(parseInt(result.rows[0].count)).toBeGreaterThan(0);
    });

    it('should check data integrity', async () => {
        const { stdout } = await execAsync('nself tasktracker check');
        expect(stdout).toContain('✓');
    });
});
```

### Mock External APIs

Use nock or similar to mock API responses:

```typescript
import nock from 'nock';

describe('API Client', () => {
    beforeEach(() => {
        nock.cleanAll();
    });

    it('should fetch tasks from API', async () => {
        // Mock API response
        nock('https://api.tasktracker.com')
            .get('/v1/tasks')
            .query({ workspace_id: 'test' })
            .reply(200, {
                tasks: [
                    { id: 'task_1', title: 'Test Task' }
                ]
            });

        const client = new APIClient({
            apiKey: 'test',
            apiUrl: 'https://api.tasktracker.com/v1'
        });

        const response = await client.get('/tasks', {
            workspace_id: 'test'
        });

        expect(response.tasks).toHaveLength(1);
    });

    it('should handle rate limiting', async () => {
        // Mock rate limit error
        nock('https://api.tasktracker.com')
            .get('/v1/tasks')
            .reply(429, { error: 'Rate limit exceeded' });

        // Then success
        nock('https://api.tasktracker.com')
            .get('/v1/tasks')
            .reply(200, { tasks: [] });

        const client = new APIClient({
            apiKey: 'test',
            apiUrl: 'https://api.tasktracker.com/v1'
        });

        const response = await client.get('/tasks');

        expect(response.tasks).toBeDefined();
    });
});
```

### CI/CD for Plugins

`.github/workflows/test.yml`:

```yaml
name: Plugin Tests

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest

    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: test
          POSTGRES_DB: test_nself
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432

    steps:
      - uses: actions/checkout@v3

      - name: Setup Node.js
        uses: actions/setup-node@v3
        with:
          node-version: '18'
          cache: 'npm'

      - name: Install dependencies
        run: npm ci

      - name: Run linter
        run: npm run lint

      - name: Run unit tests
        run: npm run test:unit

      - name: Run integration tests
        run: npm run test:integration
        env:
          DATABASE_URL: postgresql://postgres:test@localhost:5432/test_nself

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage/lcov.info
```

---

## 13. Publishing Plugins

### Versioning Strategy

Follow Semantic Versioning (SemVer):

- **Major** (1.0.0): Breaking changes
- **Minor** (0.1.0): New features, backward compatible
- **Patch** (0.0.1): Bug fixes

```bash
# Update version in plugin.json
jq '.version = "1.2.3"' plugin.json > tmp.json && mv tmp.json plugin.json

# Create git tag
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

### Documentation Requirements

Required documentation:

1. **README.md**
   - Installation instructions
   - Configuration guide
   - Usage examples
   - Troubleshooting

2. **CHANGELOG.md**
   - Version history
   - Changes in each version

3. **LICENSE**
   - License text

4. **API Documentation**
   - If plugin provides API

Example README.md structure:

```markdown
# nself-tasktracker

TaskTracker integration for nself - sync tasks to PostgreSQL.

## Installation

\`\`\`bash
nself plugin install nself-tasktracker
\`\`\`

## Configuration

Add to your `.env`:

\`\`\`bash
TASKTRACKER_API_KEY=your-api-key
TASKTRACKER_WORKSPACE_ID=workspace-123
\`\`\`

## Usage

### Sync tasks

\`\`\`bash
nself tasktracker sync
\`\`\`

### Check sync status

\`\`\`bash
nself tasktracker status
\`\`\`

## Database Schema

Creates the following tables:
- `tasktracker.tasks`
- `tasktracker.projects`
- `tasktracker.users`

## Troubleshooting

### Error: API key invalid

Ensure your API key is correct and has necessary permissions.

## License

MIT
```

### Backward Compatibility

Maintain backward compatibility:

- Don't remove configuration options (deprecate instead)
- Don't change database schema destructively
- Provide migration path for breaking changes

```typescript
// Example: Deprecate old config, support new
const apiKey = process.env.TASKTRACKER_API_KEY ||
               process.env.TASKTRACKER_TOKEN; // Old name

if (process.env.TASKTRACKER_TOKEN) {
    console.warn('TASKTRACKER_TOKEN is deprecated, use TASKTRACKER_API_KEY');
}
```

### Registry Submission

Submit to the nself plugin registry:

```bash
# 1. Validate plugin
nself plugin validate

# 2. Test installation locally
nself plugin install .
nself tasktracker status

# 3. Create GitHub release
git tag v1.0.0
git push origin v1.0.0

# 4. Submit to registry (requires account)
nself plugin publish --registry https://plugins.nself.org

# Or manually submit PR to registry repo
# https://github.com/nself-org/plugins
```

Registry entry format:

```json
{
  "name": "nself-tasktracker",
  "version": "1.0.0",
  "description": "TaskTracker task sync to PostgreSQL",
  "author": "Your Name",
  "category": "productivity",
  "repository": "https://github.com/yourusername/nself-tasktracker",
  "downloadUrl": "https://github.com/yourusername/nself-tasktracker/archive/v1.0.0.tar.gz",
  "checksum": "sha256:abc123...",
  "verified": false
}
```

---

## 14. Official Plugins

### nself-stripe

**Status**: Planned (first official plugin)

Syncs Stripe billing and payment data to PostgreSQL.

#### Features

- **11 Synced Resources**: customers, subscriptions, invoices, payment intents, charges, refunds, disputes, payouts, products, prices, events
- **Real-Time Webhooks**: Automatic updates via Stripe webhooks
- **Sanity Checks**: Verify DB matches Stripe
- **Historical Backfill**: Download years of data
- **Audit Log**: All Stripe events stored
- **Multi-Account**: Support multiple Stripe accounts
- **Test Mode**: Separate sync for test vs live

#### Installation

```bash
nself plugin install nself-stripe
```

#### Configuration

```bash
# .env
STRIPE_API_KEY=sk_live_...
STRIPE_WEBHOOK_SECRET=whsec_...
STRIPE_API_VERSION=2024-01-01
STRIPE_SYNC_INTERVAL=60
```

#### Usage

```bash
# Setup
nself stripe init
nself stripe schema apply

# Sync
nself stripe sync
nself stripe backfill --from 2020-01-01

# Webhooks
nself stripe webhook register
nself stripe webhook status

# Validation
nself stripe check
nself stripe check --fix

# Status
nself stripe status
```

#### Database Schema

```
stripe/
├── customers
├── subscriptions
├── invoices
├── payment_intents
├── charges
├── refunds
├── disputes
├── payouts
├── products
├── prices
└── events
```

### Future Official Plugins

These are potential future official plugins (not currently planned):

#### nself-shopify

E-commerce store sync:
- Products, variants, collections
- Orders, customers
- Inventory tracking
- Fulfillments

#### nself-github

Repository and development tracking:
- Repositories, branches, commits
- Issues, pull requests, discussions
- Actions runs, deployments
- Team and permissions

---

## 15. Plugin Security

### Permission Model

Plugins declare required permissions in `plugin.json`:

```json
{
  "permissions": {
    "database": {
      "read": ["public.*"],
      "write": ["plugin_*"],
      "create": ["plugin_*"],
      "drop": ["plugin_*"]
    },
    "network": {
      "outbound": ["api.example.com", "*.amazonaws.com"]
    },
    "filesystem": {
      "read": [".env", "config/"],
      "write": ["logs/", "cache/"]
    },
    "docker": {
      "execute": false,
      "volumes": []
    }
  }
}
```

### Sandbox Execution

Plugins run in isolated environments:

- **Database**: Limited to plugin schema
- **Network**: Only allowed domains
- **Filesystem**: Restricted paths
- **Process**: No system access

### Data Access Restrictions

```typescript
// Example: Enforce database restrictions
class SecureDBClient {
    private allowedSchemas: string[];

    constructor(allowedSchemas: string[]) {
        this.allowedSchemas = allowedSchemas;
    }

    async query(sql: string): Promise<any> {
        // Parse and validate query
        if (!this.isQueryAllowed(sql)) {
            throw new Error('Query accesses restricted schema');
        }

        return this.db.query(sql);
    }

    isQueryAllowed(sql: string): boolean {
        // Check if query only accesses allowed schemas
        const schemas = this.extractSchemas(sql);

        return schemas.every(schema =>
            this.allowedSchemas.includes(schema)
        );
    }
}
```

### Security Review Process

Official plugins undergo security review:

1. **Code Review**: Manual inspection
2. **Static Analysis**: Automated scanning
3. **Dependency Check**: Verify dependencies
4. **Permission Audit**: Review requested permissions
5. **Test Execution**: Run in sandbox
6. **Documentation Review**: Clear security implications

Community plugins are marked "unverified" until reviewed.

---

## 16. Best Practices

### Plugin Naming Conventions

- **Format**: `nself-<service>` (lowercase, hyphenated)
- **Examples**: `nself-stripe`, `nself-tasktracker`
- **Avoid**: `nself_stripe`, `NselfStripe`, `stripe-nself`

### Code Organization

```
src/
├── index.ts           # Main entry point
├── client.ts          # API client
├── sync.ts            # Sync logic
├── webhooks.ts        # Webhook handlers
├── types.ts           # Type definitions
└── utils/
    ├── validation.ts
    ├── formatting.ts
    └── errors.ts
```

### Performance Considerations

1. **Batch Operations**: Insert multiple records at once
```typescript
// ❌ Bad: Individual inserts
for (const item of items) {
    await db.query('INSERT INTO ...', [item]);
}

// ✅ Good: Batch insert
const values = items.map(item => [item.id, item.name]);
await db.query('INSERT INTO ... VALUES ?', [values]);
```

2. **Connection Pooling**: Reuse database connections
```typescript
// ✅ Good: Use connection pool
const pool = new Pool({ max: 10 });
```

3. **Rate Limiting**: Respect API limits
```typescript
const limiter = new RateLimiter(100, 60000); // 100/min
```

4. **Pagination**: Handle large datasets
```typescript
for await (const page of paginate('/items')) {
    await processBatch(page);
}
```

5. **Indexes**: Add indexes for frequent queries
```sql
CREATE INDEX idx_customers_email ON stripe.customers(email);
CREATE INDEX idx_tasks_status ON tasktracker.tasks(status);
```

### Error Handling Best Practices

```typescript
class PluginError extends Error {
    constructor(
        message: string,
        public code: string,
        public details?: any
    ) {
        super(message);
        this.name = 'PluginError';
    }
}

// Usage
throw new PluginError(
    'Failed to sync tasks',
    'SYNC_FAILED',
    { resource: 'tasks', count: 0 }
);
```

### Logging Best Practices

```typescript
import winston from 'winston';

const logger = winston.createLogger({
    level: process.env.LOG_LEVEL || 'info',
    format: winston.format.combine(
        winston.format.timestamp(),
        winston.format.json()
    ),
    transports: [
        new winston.transports.File({
            filename: 'logs/plugin.log'
        }),
        new winston.transports.Console({
            format: winston.format.simple()
        })
    ]
});

// Usage
logger.info('Sync started', { resource: 'tasks' });
logger.error('Sync failed', { error: error.message });
```

### Configuration Best Practices

1. **Provide defaults** for optional settings
2. **Validate** configuration on startup
3. **Document** all options
4. **Support** environment-specific configs

### Testing Best Practices

1. **Unit tests**: > 80% coverage
2. **Integration tests**: Critical paths
3. **Mock external APIs**: Don't hit real services
4. **Test error cases**: Not just happy path
5. **CI/CD**: Automated testing

### Documentation Best Practices

1. **README**: Clear, comprehensive
2. **Examples**: Real-world usage
3. **Troubleshooting**: Common issues
4. **API docs**: If applicable
5. **Changelog**: Version history

---

## Summary

This guide covered everything needed to create nself plugins:

1. **Plugin Architecture**: Understanding the lifecycle and structure
2. **Plugin Types**: Data sync, service, CLI, webhook, integration
3. **Implementation**: Database schema, sync logic, webhooks, CLI
4. **Testing**: Unit, integration, CI/CD
5. **Publishing**: Versioning, documentation, registry submission
6. **Security**: Permissions, sandboxing, reviews
7. **Best Practices**: Naming, performance, errors, logging

### Next Steps

1. Review the [Official Plugin Example](https://github.com/nself-org/plugins/tree/main/plugins/nself-stripe)
2. Use `nself plugin init` to scaffold a new plugin
3. Test locally with `nself plugin install .`
4. Publish to the registry when ready

### Resources

- **Plugin Registry**: https://plugins.nself.org
- **Official Plugins**: https://github.com/nself-org/plugins
- **Documentation**: https://docs.nself.org/plugins
- **Discord**: https://discord.gg/nself

---

**Happy Plugin Development!**
