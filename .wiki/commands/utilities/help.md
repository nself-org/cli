# nself help

**Category**: Utilities

Display help information for nself commands.

## Overview

Shows comprehensive help documentation, command usage, and examples directly in the terminal.

**Features**:
- ✅ Command-specific help
- ✅ Option descriptions
- ✅ Usage examples
- ✅ Related commands
- ✅ Quick search

## Usage

```bash
nself help [COMMAND] [SUBCOMMAND]
```

## Examples

### General Help

```bash
nself help
```

**Output**:
```
nself v0.9.9 - Backend-as-a-Service Made Simple

Usage:
  nself <command> [options]

Core Commands:
  init        Initialize new project
  build       Generate configuration
  start       Start all services
  stop        Stop all services
  restart     Restart services

Database:
  db migrate  Manage migrations
  db seed     Load seed data
  db shell    Open PostgreSQL shell
  db backup   Create database backup
  db restore  Restore from backup
  db reset    Reset database (dev only)

Configuration:
  config show      Show configuration
  config get       Get config value
  config set       Set config value
  config env       Environment management
  config secrets   Secrets management

Additional Commands:
  auth, service, tenant, deploy, infra,
  perf, backup, dev, plugin

Utilities:
  status, logs, urls, health, doctor, exec,
  version, monitor, admin, help

Run 'nself help <command>' for more information
Run 'nself <command> --help' for detailed options
```

### Command-Specific Help

```bash
nself help start
```

**Output**:
```
nself start - Start all services

Usage:
  nself start [OPTIONS]

Description:
  Intelligently starts nself backend services and optionally
  frontend applications if running in a monorepo structure.

Options:
  -v, --verbose         Show detailed output
  -d, --debug          Show debug information
  -h, --help           Show this help
  --skip-health-checks Skip health validation
  --timeout N          Health check timeout (default: 120s)
  --fresh              Force recreate containers
  --clean-start        Remove containers before starting
  --quick              Quick start with relaxed checks

Examples:
  nself start
  nself start --verbose
  nself start --fresh

Monorepo Support:
  When /backend directory detected, starts backend
  services and auto-detected frontend applications.

Related Commands:
  nself stop       Stop services
  nself restart    Restart services
  nself status     Check service status

More info: https://docs.nself.org/commands/start
```

### Subcommand Help

```bash
nself help db migrate
```

**Output**:
```
nself db migrate - Manage database migrations

Usage:
  nself db migrate <action> [options]

Actions:
  up        Apply pending migrations
  down      Rollback last migration
  status    Show migration status
  create    Create new migration

Options:
  -v, --verbose    Show detailed output
  --dry-run        Show what would be done
  --force          Skip confirmations

Examples:
  nself db migrate up
  nself db migrate down
  nself db migrate status
  nself db migrate create add_users_table

Migration Formats:
  - Hasura-style: hasura/migrations/default/
  - Simple-style: db/migrations/

Related Commands:
  nself db seed      Load seed data
  nself db backup    Backup before migration
  nself db shell     Inspect database

More info: https://docs.nself.org/commands/db/migrate
```

### Flag Help

```bash
nself start --help
```

**Same as**: `nself help start`

## Help Search

### Search Commands

```bash
nself help | grep database
```

**Output**:
```
  db migrate  Manage migrations
  db seed     Load seed data
  db shell    Open PostgreSQL shell
  db backup   Create database backup
  db restore  Restore from backup
  db reset    Reset database (dev only)
```

### Find Command

```bash
nself help | grep -i backup
```

**Output**:
```
  db backup      Create database backup
  backup create  Create full backup
  backup list    List available backups
  backup restore Restore from backup
```

## Quick Reference

### Common Commands

```bash
# Get started
nself help init
nself help build
nself help start

# Database operations
nself help db
nself help db migrate
nself help db seed

# Troubleshooting
nself help status
nself help logs
nself help doctor

# Deployment
nself help deploy
nself help deploy staging
```

### Command Categories

```bash
# Core workflow
nself help init build start stop restart

# Database management
nself help db migrate seed shell backup restore

# Configuration
nself help config env secrets vault

# Monitoring
nself help status logs health monitor
```

## Help Formats

### Plain Text (Default)

Standard terminal output with formatting.

### Markdown

```bash
HELP_FORMAT=markdown nself help start > start-help.md
```

### JSON

```bash
HELP_FORMAT=json nself help start
```

**Output**:
```json
{
  "command": "start",
  "usage": "nself start [OPTIONS]",
  "description": "Start all services",
  "options": [
    {
      "flag": "-v, --verbose",
      "description": "Show detailed output",
      "default": false
    }
  ],
  "examples": [
    "nself start",
    "nself start --verbose"
  ],
  "related": ["stop", "restart", "status"]
}
```

## Online Documentation

### Open Command Docs

```bash
nself help --web start
```

**Opens in browser**: https://docs.nself.org/commands/start

### Wiki Documentation

Full documentation available at:
- **GitHub Wiki**: https://github.com/nself-org/cli/wiki
- **Website**: https://nself.org/docs

## Help Topics

### List All Topics

```bash
nself help topics
```

**Output**:
```
Available Help Topics:

Getting Started:
  installation   - Installing nself
  quickstart     - Quick start guide
  concepts       - Core concepts

Commands:
  core          - Core commands
  database      - Database management
  configuration - Configuration
  services      - Service management

Guides:
  deployment    - Deploying to production
  development   - Development workflows
  security      - Security best practices
  troubleshooting - Common issues
```

### View Topic

```bash
nself help topics quickstart
```

## Command Aliases

### Show Aliases

```bash
nself help aliases
```

**Output**:
```
Command Aliases:

ps        → status
up        → start
down      → stop
ls        → urls
v         → version

Example:
  nself ps     (same as: nself status)
  nself up     (same as: nself start)
```

## Interactive Help

### Help Browser

```bash
nself help --interactive
```

**Features**:
- Arrow key navigation
- Search commands
- Category browsing
- Command examples

## Contextual Help

### Environment-Specific Help

```bash
# Development
ENV=dev nself help deploy
# Shows dev-specific deployment info

# Production
ENV=prod nself help deploy
# Shows production warnings and requirements
```

## Man Pages (Unix)

### Install Man Pages

```bash
nself install-manpages
```

### View Man Page

```bash
man nself
man nself-start
man nself-db-migrate
```

## Help Configuration

### Custom Help Format

```bash
# In .env or ~/.nself/config
HELP_FORMAT=compact  # compact, detailed, minimal
HELP_COLOR=auto      # auto, always, never
HELP_PAGER=less      # less, more, cat
```

### Disable Color

```bash
NO_COLOR=1 nself help start
# Or
nself help start --no-color
```

## Related Commands

- `nself version` - Show version information
- `nself doctor` - Diagnose issues
- `nself help topics` - Browse help topics

## See Also

- [Online Documentation](https://docs.nself.org)
- [GitHub Wiki](https://github.com/nself-org/cli/wiki)
- [Quick Start Guide](../../quick-start/README.md)
