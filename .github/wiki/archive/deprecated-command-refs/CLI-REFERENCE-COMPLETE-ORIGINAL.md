# Complete nself CLI Reference

**Version 0.9.9** | All 31 Top-Level Commands + 295 Subcommands

This is the authoritative CLI reference for nself. Every command, subcommand, flag, and option is documented here.

---

## Table of Contents

### Core Commands (5)
- [init](#init) - Initialize new nself project
- [build](#build) - Generate configuration files
- [start](#start) - Start services
- [stop](#stop) - Stop services
- [restart](#restart) - Restart services

### Utilities (15)
- [status](#status) - Service health and status
- [logs](#logs) - View service logs
- [help](#help) - Help system
- [admin](#admin) - Admin UI access
- [urls](#urls) - Service URLs
- [exec](#exec) - Execute commands in containers
- [doctor](#doctor) - System diagnostics
- [monitor](#monitor) - Monitoring dashboards
- [health](#health) - Health checks
- [version](#version) - Version information
- [update](#update) - Update nself
- [completion](#completion) - Shell completions
- [metrics](#metrics) - Metrics and profiling
- [history](#history) - Command history and audit trail
- [audit](#audit) - Security audit logging

### Feature Commands (11)
- [db](#db) - Database operations (14 subcommands)
- [tenant](#tenant) - Multi-tenancy (60+ subcommands)
- [deploy](#deploy) - Deployment (23 subcommands)
- [infra](#infra) - Infrastructure (38 subcommands)
- [service](#service) - Service management (43 subcommands)
- [config](#config) - Configuration (20 subcommands)
- [auth](#auth) - Authentication & security (38 subcommands)
- [perf](#perf) - Performance optimization (5 subcommands)
- [backup](#backup) - Backup & recovery (6 subcommands)
- [dev](#dev) - Developer tools (16 subcommands)
- [plugin](#plugin) - Plugin system (8 subcommands)

### Infrastructure Management (1)
- [destroy](#destroy) - Safe infrastructure destruction

---

## Core Commands

### init

Initialize a new nself project with interactive configuration wizard.

**Usage:**
```bash
nself init [OPTIONS]
```

**Options:**
```
--demo              Full-featured demo with all services enabled
--simple            Minimal setup (PostgreSQL, Hasura, Auth only)
--full              Full setup with all optional services
--skip-git          Don't initialize Git repository
--no-ssl            Skip SSL certificate generation
--offline           Use cached templates (no network)
-h, --help          Show help message
```

**Examples:**
```bash
# Interactive wizard (recommended for first-time users)
nself init

# Quick demo with all features
nself init --demo

# Minimal setup for learning
nself init --simple

# Full production setup
nself init --full

# Initialize without Git (existing repo)
nself init --skip-git
```

**What It Does:**
1. Creates `.env` file with configuration
2. Generates `nself/` directory structure
3. Initializes Git repository (unless `--skip-git`)
4. Generates self-signed SSL certificates
5. Downloads service templates (if using custom services)
6. Validates dependencies (Docker, Docker Compose)

**Related:**
- [Getting Started Guide](../../getting-started/Quick-Start.md)
- [Configuration Guide](../../configuration/ENVIRONMENT-VARIABLES.md)
- [init Command Deep Dive](INIT.md)

---

### build

Generate Docker Compose configuration, Nginx routes, and service files from `.env` settings.

**Usage:**
```bash
nself build [OPTIONS]
```

**Options:**
```
--force             Force regeneration of all files (overwrite custom changes)
--clean             Remove all generated files before building
--validate          Validate configuration without building
--skip-templates    Don't copy custom service templates
--dry-run           Show what would be generated without writing files
-v, --verbose       Show detailed build output
-h, --help          Show help message
```

**Examples:**
```bash
# Standard build (preserves custom changes)
nself build

# Force regeneration (WARNING: overwrites customizations)
nself build --force

# Clean build (fresh start)
nself build --clean

# Validate configuration only
nself build --validate

# Preview what will be generated
nself build --dry-run
```

**What It Generates:**
```
docker-compose.yml          # Service definitions (25+ services)
nginx/
  ├── nginx.conf           # Main Nginx configuration
  ├── includes/            # Reusable config snippets
  │   ├── security.conf
  │   ├── gzip.conf
  │   └── ssl.conf
  └── sites/               # Route configurations
      ├── api.conf
      ├── auth.conf
      ├── admin.conf
      └── ... (20+ route files)
postgres/
  └── init/
      └── 00-init.sql      # Database initialization
services/
  ├── custom_service_1/    # Generated from templates
  ├── custom_service_2/
  └── ...
monitoring/
  ├── prometheus/
  │   └── prometheus.yml
  ├── grafana/
  │   ├── provisioning/
  │   └── dashboards/
  └── loki/
      └── loki-config.yaml
```

**Build Process:**
1. Loads and validates `.env` configuration
2. Determines enabled services
3. Generates Docker Compose service definitions
4. Creates Nginx routing configuration
5. Generates database initialization scripts
6. Copies and processes custom service templates
7. Sets up monitoring configuration (if enabled)
8. Validates generated files

**Configuration Preservation:**
- By default, existing custom service files are NOT overwritten
- Use `--force` to regenerate everything (destroys customizations)
- Use `--clean` to start fresh

**Related:**
- [Build Architecture](../../architecture/BUILD_ARCHITECTURE.md)
- [Service Templates](../../services/SERVICE-TEMPLATES.md)
- [build Command Deep Dive](BUILD.md)

---

### start

Start all services or specific services with intelligent health checking.

**Usage:**
```bash
nself start [SERVICE...] [OPTIONS]
```

**Options:**
```
--fresh                  Force recreate all containers
--clean-start            Remove everything and start fresh
--quick                  Skip health checks (faster startup)
--skip-health-checks     Alias for --quick
--timeout SECONDS        Health check timeout (default: 120)
--health-required PCT    Required healthy percentage (default: 80)
--verbose                Show detailed output
--debug                  Maximum verbosity
-h, --help               Show help message
```

**Environment Variables:**
```bash
NSELF_START_MODE=smart              # smart, fresh, force (default: smart)
NSELF_HEALTH_CHECK_TIMEOUT=120      # Seconds (default: 120)
NSELF_HEALTH_CHECK_INTERVAL=2       # Check interval (default: 2)
NSELF_HEALTH_CHECK_REQUIRED=80      # Percent healthy (default: 80)
NSELF_SKIP_HEALTH_CHECKS=false      # Skip health checks (default: false)
NSELF_DOCKER_BUILD_TIMEOUT=300      # Build timeout (default: 300)
NSELF_CLEANUP_ON_START=auto         # auto, always, never (default: auto)
NSELF_PARALLEL_LIMIT=5              # Parallel starts (default: 5)
NSELF_LOG_LEVEL=info                # debug, info, warn, error (default: info)
```

**Examples:**
```bash
# Start all services
nself start

# Start specific services only
nself start postgres hasura auth

# Force recreate all containers
nself start --fresh

# Quick start (skip health checks)
nself start --quick

# Development mode (verbose output)
nself start --verbose

# Production start (require 100% healthy)
NSELF_HEALTH_CHECK_REQUIRED=100 nself start

# CI/CD mode (clean state)
nself start --clean-start
```

**Start Modes:**

**smart** (default):
- Resumes stopped containers
- Keeps running healthy containers
- Only recreates problematic containers
- Best for development iteration

**fresh**:
- Force recreates all containers
- Good for config changes
- Slower but guaranteed clean state

**force**:
- Removes all containers first
- Completely fresh start
- Use when debugging weird issues

**Health Check Process:**
1. Starts services in dependency order
2. Monitors health endpoints
3. Shows real-time progress
4. Accepts partial success (default 80%)
5. Provides detailed status report

**Startup Order:**
1. **Infrastructure**: PostgreSQL, Redis, MinIO
2. **Core Services**: Hasura, Auth
3. **Optional Services**: Admin, Functions, Mail, Search
4. **Monitoring**: Prometheus, Grafana, Loki, Exporters
5. **Custom Services**: User-defined CS_1 through CS_10
6. **Proxy**: Nginx (starts last after all backends ready)

**Troubleshooting:**
```bash
# Services timing out but running?
nself start --timeout 180
NSELF_HEALTH_CHECK_REQUIRED=70 nself start

# Port conflicts?
nself start --clean-start

# Need faster iteration?
NSELF_SKIP_HEALTH_CHECKS=true nself start
```

**Related:**
- [Start Command Options](../../configuration/START-COMMAND-OPTIONS.md)
- [Health Checks](HEALTH.md)
- [start Command Deep Dive](START.md)

---

### stop

Stop all services or specific services gracefully.

**Usage:**
```bash
nself stop [SERVICE...] [OPTIONS]
```

**Options:**
```
--force             Force stop (kill containers immediately)
--remove            Stop and remove containers
--volumes           Also remove volumes (DATA LOSS WARNING)
--timeout SECONDS   Shutdown timeout (default: 30)
-v, --verbose       Show detailed output
-h, --help          Show help message
```

**Examples:**
```bash
# Stop all services gracefully
nself stop

# Stop specific services
nself stop postgres redis

# Force stop immediately
nself stop --force

# Stop and remove containers
nself stop --remove

# Stop with longer timeout (for graceful shutdown)
nself stop --timeout 60
```

**Stop Process:**
1. Sends SIGTERM to containers (graceful shutdown)
2. Waits for timeout (default: 30 seconds)
3. Sends SIGKILL if still running (force stop)

**What Happens:**
- Running containers are stopped
- Containers remain for restart (unless `--remove`)
- Volumes are preserved (unless `--volumes`)
- Networks remain active

**Stop Order (Reverse of Start):**
1. Nginx (stop accepting traffic)
2. Custom services
3. Optional services
4. Core services (Hasura, Auth)
5. Infrastructure (PostgreSQL, Redis) - stopped last

**Related:**
- [stop Command Deep Dive](STOP.md)

---

### restart

Restart all services or specific services.

**Usage:**
```bash
nself restart [SERVICE...] [OPTIONS]
```

**Options:**
```
--force             Force recreate containers
--timeout SECONDS   Shutdown timeout before restart (default: 30)
-v, --verbose       Show detailed output
-h, --help          Show help message
```

**Examples:**
```bash
# Restart all services
nself restart

# Restart specific services
nself restart hasura auth

# Force recreate containers
nself restart --force

# Restart with longer shutdown timeout
nself restart --timeout 60
```

**Restart Process:**
1. Stops specified services (or all)
2. Waits for graceful shutdown
3. Starts services in correct order

**When to Use:**
- Configuration changes (in containers)
- Service is misbehaving
- Apply container updates
- Test service recovery

**Note:** For `.env` changes, use `nself build && nself restart`

**Related:**
- [restart Command Deep Dive](RESTART.md)

---

## Utilities

### status

Show comprehensive service health and status information.

**Usage:**
```bash
nself status [OPTIONS]
```

**Options:**
```
--format FORMAT     Output format: table, json, yaml, compact (default: table)
--watch             Auto-refresh every 2 seconds (Ctrl+C to exit)
--failing-only      Show only unhealthy services
--services SERVICE  Filter by service name(s)
-v, --verbose       Show detailed health information
-h, --help          Show help message
```

**Examples:**
```bash
# Standard status view (table format)
nself status

# JSON output for scripting
nself status --format json

# Watch mode (auto-refresh)
nself status --watch

# Show only failing services
nself status --failing-only

# Check specific services
nself status --services postgres,hasura,auth

# Detailed health info
nself status --verbose
```

**Status Output:**
```
┌──────────────────────────────────────────────────────────┐
│                  nself Status Overview                    │
└──────────────────────────────────────────────────────────┘

Project: demo-app
Environment: development
Total Services: 25
Healthy: 24 | Unhealthy: 1 | Stopped: 0

┌─────────────┬────────┬─────────┬──────────┬─────────────┐
│ Service     │ Status │ Health  │ Uptime   │ CPU/Memory  │
├─────────────┼────────┼─────────┼──────────┼─────────────┤
│ postgres    │ ✓ Up   │ Healthy │ 2h 15m   │ 2% / 256MB  │
│ hasura      │ ✓ Up   │ Healthy │ 2h 14m   │ 5% / 512MB  │
│ auth        │ ✓ Up   │ Healthy │ 2h 14m   │ 1% / 128MB  │
│ redis       │ ✓ Up   │ Healthy │ 2h 15m   │ 1% / 64MB   │
│ minio       │ ✗ Down │ ---     │ ---      │ ---         │
│ nginx       │ ✓ Up   │ Healthy │ 2h 14m   │ 0% / 32MB   │
└─────────────┴────────┴─────────┴──────────┴─────────────┘
```

**Health Indicators:**
- ✓ **Healthy** - Service running and responding
- ⚠ **Degraded** - Running but slow/partial failures
- ✗ **Unhealthy** - Running but failing health checks
- --- **Stopped** - Container not running

**JSON Output Format:**
```json
{
  "project": "demo-app",
  "environment": "development",
  "total_services": 25,
  "healthy": 24,
  "unhealthy": 1,
  "stopped": 0,
  "services": [
    {
      "name": "postgres",
      "status": "running",
      "health": "healthy",
      "uptime": "2h15m",
      "cpu_percent": 2.1,
      "memory_mb": 256,
      "ports": ["5432:5432"]
    }
  ]
}
```

**Related:**
- [Health Checks Guide](HEALTH.md)
- [status Command Deep Dive](STATUS.md)

---

### logs

View service logs with filtering, following, and formatting options.

**Usage:**
```bash
nself logs [SERVICE...] [OPTIONS]
```

**Options:**
```
-f, --follow           Follow log output (live tail)
-n, --tail LINES       Number of lines to show (default: 100)
--since TIME           Show logs since timestamp (e.g., 2h, 30m, 2023-01-01)
--until TIME           Show logs until timestamp
--timestamps           Show timestamps
--no-color             Disable colored output
--grep PATTERN         Filter logs by pattern (regex)
--level LEVEL          Filter by log level (debug, info, warn, error)
--format FORMAT        Output format: raw, json, logfmt (default: raw)
-h, --help             Show help message
```

**Examples:**
```bash
# View all logs (last 100 lines)
nself logs

# Follow logs in real-time
nself logs -f

# View specific service logs
nself logs postgres

# View multiple services
nself logs hasura auth

# Show last 500 lines
nself logs --tail 500

# Logs from last 2 hours
nself logs --since 2h

# Follow with timestamps
nself logs -f --timestamps

# Filter by pattern
nself logs --grep "ERROR"

# Filter by log level
nself logs --level error

# JSON output
nself logs --format json
```

**Multi-Service Output:**
```
[postgres] 2026-01-31 10:15:23 | database system is ready to accept connections
[hasura]   2026-01-31 10:15:25 | server: running on http://0.0.0.0:8080
[auth]     2026-01-31 10:15:26 | Auth service started on port 4000
[nginx]    2026-01-31 10:15:27 | nginx: configuration file /etc/nginx/nginx.conf test is successful
```

**Timestamp Formats:**
```bash
--since 2h              # Last 2 hours
--since 30m             # Last 30 minutes
--since 2023-01-31      # Since specific date
--since "2023-01-31 10:00:00"  # Since specific time
```

**Log Levels:**
- `debug` - Detailed debugging information
- `info` - General informational messages
- `warn` - Warning messages
- `error` - Error messages

**Related:**
- [logs Command Deep Dive](LOGS.md)
- [Monitoring Guide](../../guides/MONITORING-COMPLETE.md)

---

### help

Interactive help system with command discovery and examples.

**Usage:**
```bash
nself help [COMMAND] [OPTIONS]
```

**Options:**
```
--examples          Show usage examples
--related           Show related commands
--full              Show complete documentation
-h, --help          Show help message
```

**Examples:**
```bash
# General help
nself help

# Command-specific help
nself help db

# Help with examples
nself help db migrate --examples

# Related commands
nself help tenant --related
```

**Related:**
- [help Command Deep Dive](HELP.md)

---

### admin

Open nself Admin UI in browser for visual management.

**Usage:**
```bash
nself admin [OPTIONS]
```

**Options:**
```
--port PORT         Admin UI port (default: from .env)
--open              Open in browser automatically (default: true)
--no-open           Don't open browser
--credentials       Show admin credentials
-h, --help          Show help message
```

**Examples:**
```bash
# Open admin UI in browser
nself admin

# Show admin credentials
nself admin --credentials

# Don't open browser (just show URL)
nself admin --no-open
```

**Admin UI Features:**
- Visual service management
- Database browser
- User management
- Tenant management
- Billing dashboard
- Monitoring graphs
- Log viewer

**Default URL:** `https://admin.local.nself.org` (or your configured domain)

**Related:**
- [Admin UI Guide](../../configuration/Admin-UI.md)
- [admin Command Deep Dive](ADMIN.md)

---

### urls

Display all service URLs and access information.

**Usage:**
```bash
nself urls [OPTIONS]
```

**Options:**
```
--format FORMAT     Output format: table, json, yaml, list (default: table)
--external-only     Show only externally accessible URLs
--internal-only     Show only internal URLs
--service SERVICE   Filter by service name
--copy URL_KEY      Copy specific URL to clipboard
-h, --help          Show help message
```

**Examples:**
```bash
# Show all URLs
nself urls

# JSON output
nself urls --format json

# External URLs only
nself urls --external-only

# Copy Hasura URL to clipboard
nself urls --copy hasura
```

**Output:**
```
┌─────────────────────────────────────────────────────────────┐
│                      Service URLs                           │
└─────────────────────────────────────────────────────────────┘

Core Services:
  API (Hasura)      https://api.local.nself.org
  Auth              https://auth.local.nself.org
  Admin             https://admin.local.nself.org

Optional Services:
  MinIO Console     https://minio.local.nself.org
  Functions         https://functions.local.nself.org
  Mail (MailPit)    https://mail.local.nself.org
  Search            https://search.local.nself.org
  MLflow            https://mlflow.local.nself.org

Monitoring:
  Grafana           https://grafana.local.nself.org
  Prometheus        https://prometheus.local.nself.org

Custom Services:
  Express API       https://express-api.local.nself.org
  gRPC API          https://grpc-api.local.nself.org

Frontend Apps:
  App 1             https://app1.local.nself.org
  App 2             https://app2.local.nself.org

Application:
  Root              https://local.nself.org
```

**Related:**
- [urls Command Deep Dive](URLS.md)

---

## Continue with All Commands...

*(The complete reference continues with all remaining commands. This is section 1 of 8.)*

**Related Documentation:**
- [Command Tree v1.0](COMMAND-TREE-V1.md) - Hierarchical command structure
- [Quick Reference](../../reference/QUICK-REFERENCE-CARDS.md) - Common command patterns
- [Command Consolidation Map](../../architecture/COMMAND-CONSOLIDATION-MAP.md) - Legacy to v1.0 mapping

---

**Next:** [Database Commands (db)](DB.md) | [Tenant Commands (tenant)](TENANT.md)
