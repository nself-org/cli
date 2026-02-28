# nself start

**Category**: Core Commands

Start all project services with automatic monorepo detection.

## Overview

The `start` command intelligently starts your nself backend services and optionally frontend applications if running in a monorepo structure.

**Features**:
- ✅ Automatic monorepo detection
- ✅ Parallel backend + frontend startup  
- ✅ Smart health checking
- ✅ Graceful cleanup on Ctrl+C
- ✅ Configurable behavior via environment variables
- ✅ Backward compatible (single backend projects work unchanged)

## Usage

```bash
nself start [OPTIONS]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-v, --verbose` | Show detailed Docker output | false |
| `-d, --debug` | Show debug information | false |
| `-h, --help` | Show help message | - |
| `--skip-health-checks` | Skip health validation | false |
| `--timeout N` | Health check timeout (seconds) | 120 |
| `--fresh` | Force recreate all containers | false |
| `--clean-start` | Remove containers before starting | false |
| `--quick` | Quick start with relaxed checks | false |

## Environment Variables

Configure startup behavior (all optional):

```bash
# Start mode
NSELF_START_MODE=smart|fresh|force  # Default: smart

# Health checking
NSELF_HEALTH_CHECK_TIMEOUT=120  # Seconds to wait
NSELF_HEALTH_CHECK_INTERVAL=2   # Check interval
NSELF_HEALTH_CHECK_REQUIRED=80  # Percent healthy required
NSELF_SKIP_HEALTH_CHECKS=false  # Skip validation

# Performance
NSELF_PARALLEL_LIMIT=5  # Parallel container starts
NSELF_LOG_LEVEL=info    # Verbosity level
```

## Monorepo Support

### Detection

`nself start` automatically detects monorepo structure:

**Criteria**:
1. Current directory has `/backend` subdirectory
2. `/backend/docker-compose.yml` exists

If detected → Starts backend + frontend apps in parallel  
If not → Starts backend only (original behavior)

### Monorepo Structure

```
my-project/
├── backend/               # nself backend
│   ├── .env
│   └── docker-compose.yml
├── admin-app/             # Frontend app 1
│   ├── package.json
│   └── pnpm-lock.yaml
├── user-app/              # Frontend app 2
│   ├── package.json
│   └── package-lock.json
└── .nself-monorepo.conf   # Optional config
```

### Frontend Detection

Auto-detects frontend apps by:
1. Scanning directories with `package.json` (excluding backend/, node_modules/, .git/, etc.)
2. Checking for `dev` script in package.json
3. Detecting package manager from lock files (pnpm > yarn > bun > npm)

### Package Manager Detection

Priority order:
- `pnpm-lock.yaml` → uses `pnpm dev`
- `yarn.lock` → uses `yarn run dev`
- `bun.lockb` → uses `bun dev`
- `package-lock.json` → uses `npm run dev`

### Configuration

Optional `.nself-monorepo.conf` in project root:

```bash
# Backend directory (default: backend)
BACKEND_DIR=backend

# Auto-detect frontends (default: true)
AUTO_DETECT_FRONTENDS=true

# Excluded directories
EXCLUDE_DIRS=node_modules,.git,.ai,docs,scripts

# Wait for backend (default: true)
WAIT_FOR_BACKEND=true
BACKEND_TIMEOUT=30
```

## Examples

### Basic Usage

```bash
# Standard start
nself start

# Verbose output
nself start --verbose

# Quick start for development
nself start --quick
```

### Monorepo Usage

```bash
# From monorepo root (auto-detects backend + frontends)
cd ~/my-project
nself start

# Output:
# 🚀 Monorepo Mode Detected
#
# ⠋ Starting nself backend...
# ✓ Backend services starting
#
# Auto-detecting frontend applications...
#
#   ⠋ Starting admin-app (pnpm)... ✓
#   ⠋ Starting user-app (npm)... ✓
#
# ╔═══════════════════════════════════════════════════╗
# ║             MONOREPO STATUS                       ║
# ╚═══════════════════════════════════════════════════╝
#
# Backend (nself services):
#   ✓ Running (PID: 12345)
#   → GraphQL: https://api.localhost
#   → Auth:    https://auth.localhost
#
# Frontend Applications:
#   ✓ App 1 running (PID: 12346)
#   ✓ App 2 running (PID: 12347)
#
# Press Ctrl+C to stop all services
```

### Advanced Usage

```bash
# Force recreate all containers
nself start --fresh

# Clean start (remove everything first)
nself start --clean-start

# Custom health check timeout
nself start --timeout 180

# Skip health checks (faster iteration)
NSELF_SKIP_HEALTH_CHECKS=true nself start
```

## Startup Modes

### smart (default)
- Resumes stopped containers
- Keeps running healthy containers
- Only recreates problematic containers

### fresh
- Force recreates all containers
- Uses `docker-compose up --force-recreate`
- Good for configuration changes

### force
- Most aggressive cleanup
- Removes all containers first
- Starts completely fresh

## Health Checking

Progressive health check process:
1. Waits for containers to start
2. Monitors health check status
3. Accepts partial success (default 80%)
4. Doesn't fail on timeout if services running
5. Can be skipped with `--skip-health-checks`

## Troubleshooting

### Services timing out but running

```bash
# Lower health requirement
NSELF_HEALTH_CHECK_REQUIRED=70 nself start

# Increase timeout
NSELF_HEALTH_CHECK_TIMEOUT=180 nself start
```

### Port conflicts

```bash
# Force cleanup before start
NSELF_CLEANUP_ON_START=always nself start

# Or use fresh mode
nself start --fresh
```

### Frontend not starting

Check requirements:
1. `package.json` exists in app directory
2. `dev` script exists in package.json
3. Package manager is installed (pnpm/npm/yarn/bun)

View logs:
```bash
cat .nself/frontend-logs/<app-name>.log
```

### Need faster startup

```bash
# Skip health checks
NSELF_SKIP_HEALTH_CHECKS=true nself start

# Increase parallel limit
NSELF_PARALLEL_LIMIT=10 nself start
```

## Related Commands

- `nself build` - Generate configuration before starting
- `nself stop` - Stop all services
- `nself status` - Check service health
- `nself logs` - View service logs
- `nself urls` - Show service URLs

## See Also

- [nself build](build.md)
- [nself stop](stop.md)
- [nself status](../utilities/status.md)
- [Monorepo Guide](../../guides/MONOREPO-SETUP.md)
- [Start Command Options](../../configuration/START-COMMAND-OPTIONS.md)
