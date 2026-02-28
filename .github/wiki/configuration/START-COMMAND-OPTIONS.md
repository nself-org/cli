# nself Start Command Configuration Options

This document describes the configuration options available for the `nself start` command, including environment variables and command-line flags that control startup behavior.

## Overview

The `nself start` command has been enhanced with smart defaults and configurable options to handle various deployment scenarios. All options are **optional** - the command works out of the box with sensible defaults.

## Environment Variables

These environment variables can be set in your `.env` file or exported in your shell to customize the start behavior:

### Core Start Options

#### `NSELF_START_MODE`
- **Default:** `smart`
- **Options:** `smart`, `fresh`, `force`
- **Description:** Controls how containers are started
  - `smart`: Detects existing containers and resumes or recreates as needed
  - `fresh`: Always recreates containers (docker-compose up --force-recreate)
  - `force`: Forces recreation and removes orphans

```bash
# Example in .env
NSELF_START_MODE=smart
```

#### `NSELF_HEALTH_CHECK_TIMEOUT`
- **Default:** `120` (seconds)
- **Range:** 30-600
- **Description:** Maximum time to wait for health checks to pass

```bash
# Example: Wait up to 3 minutes for services to become healthy
NSELF_HEALTH_CHECK_TIMEOUT=180
```

#### `NSELF_HEALTH_CHECK_INTERVAL`
- **Default:** `2` (seconds)
- **Range:** 1-10
- **Description:** How often to check service health status

```bash
# Example: Check every 5 seconds
NSELF_HEALTH_CHECK_INTERVAL=5
```

#### `NSELF_HEALTH_CHECK_REQUIRED`
- **Default:** `80` (percent)
- **Range:** 0-100
- **Description:** Minimum percentage of services that must be healthy to consider start successful. Setting to 0 disables health check requirements.

```bash
# Example: Require only 60% of services to be healthy
NSELF_HEALTH_CHECK_REQUIRED=60

# Example: Disable health check requirements (always succeed)
NSELF_HEALTH_CHECK_REQUIRED=0
```

### Advanced Options

#### `NSELF_SKIP_HEALTH_CHECKS`
- **Default:** `false`
- **Options:** `true`, `false`
- **Description:** Skip all health checks and return immediately after starting containers

```bash
# Example: Skip health checks entirely
NSELF_SKIP_HEALTH_CHECKS=true
```

#### `NSELF_DOCKER_BUILD_TIMEOUT`
- **Default:** `300` (seconds)
- **Range:** 60-1800
- **Description:** Maximum time to wait for Docker image builds

```bash
# Example: Allow up to 10 minutes for building images
NSELF_DOCKER_BUILD_TIMEOUT=600
```

#### `NSELF_CLEANUP_ON_START`
- **Default:** `auto`
- **Options:** `auto`, `always`, `never`
- **Description:** Controls container cleanup before starting
  - `auto`: Clean up only if containers are in error state
  - `always`: Always remove existing containers before starting
  - `never`: Never remove existing containers

```bash
# Example: Always start fresh
NSELF_CLEANUP_ON_START=always
```

#### `NSELF_PARALLEL_LIMIT`
- **Default:** `5`
- **Range:** 1-20
- **Description:** Maximum number of containers to start in parallel

```bash
# Example: Start up to 10 containers at once
NSELF_PARALLEL_LIMIT=10
```

#### `NSELF_LOG_LEVEL`
- **Default:** `info`
- **Options:** `debug`, `info`, `warn`, `error`
- **Description:** Controls verbosity of start command output

```bash
# Example: Show detailed debug information
NSELF_LOG_LEVEL=debug
```

## Command-Line Flags

These flags override environment variables when specified:

### Basic Flags

```bash
# Show help
nself start --help

# Verbose output (overrides NSELF_LOG_LEVEL)
nself start --verbose
nself start -v

# Debug mode (implies verbose)
nself start --debug
nself start -d
```

### Health Check Control

```bash
# Skip health checks
nself start --skip-health-checks

# Custom timeout (seconds)
nself start --timeout 180

# Set required healthy percentage
nself start --health-required 60
```

### Start Mode Control

```bash
# Force recreate all containers
nself start --force-recreate
nself start --fresh

# Clean start (remove everything first)
nself start --clean-start

# Quick start (minimal checks)
nself start --quick
```

## Start Modes Explained

### Smart Mode (Default)

The default `smart` mode intelligently handles different scenarios:

1. **No existing containers**: Creates and starts all services
2. **Stopped containers**: Resumes existing containers
3. **Running containers**: Verifies health and reports status
4. **Mixed state**: Restarts stopped containers, keeps healthy running ones

```bash
# Uses smart mode by default
nself start
```

### Fresh Mode

Forces recreation of all containers, useful when you've updated configurations:

```bash
# Via environment variable
NSELF_START_MODE=fresh nself start

# Via command flag
nself start --fresh
```

### Force Mode

Most aggressive mode - removes everything and starts from scratch:

```bash
# Via environment variable
NSELF_START_MODE=force nself start

# Via command flag
nself start --force-recreate
```

## Health Check Behavior

### Progressive Health Checking

The start command uses progressive health checking to provide better feedback:

1. **Initial delay**: Waits a few seconds for containers to initialize
2. **Progressive monitoring**: Shows which services become healthy over time
3. **Partial success**: Can succeed with partial health (default 80%)
4. **Timeout handling**: Doesn't fail if timeout reached but services are running

### Configuring Health Requirements

Different scenarios may require different health check strategies:

```bash
# Development: Be lenient with health checks
NSELF_HEALTH_CHECK_REQUIRED=60
NSELF_HEALTH_CHECK_TIMEOUT=60

# Production: Require all services healthy
NSELF_HEALTH_CHECK_REQUIRED=100
NSELF_HEALTH_CHECK_TIMEOUT=180

# Quick iteration: Skip health checks
NSELF_SKIP_HEALTH_CHECKS=true
```

## Common Use Cases

### Development Workflow

Quick iteration with minimal checks:

```bash
# In .env
NSELF_START_MODE=smart
NSELF_HEALTH_CHECK_REQUIRED=60
NSELF_HEALTH_CHECK_TIMEOUT=60
```

### CI/CD Pipeline

Ensure clean state and full health:

```bash
# In CI script
export NSELF_START_MODE=fresh
export NSELF_HEALTH_CHECK_REQUIRED=100
export NSELF_HEALTH_CHECK_TIMEOUT=300
nself start
```

### Debugging Issues

Maximum verbosity and no health requirements:

```bash
nself start --debug --skip-health-checks
```

### Production Deployment

Careful startup with full validation:

```bash
# In .env.production
NSELF_START_MODE=smart
NSELF_HEALTH_CHECK_REQUIRED=100
NSELF_HEALTH_CHECK_TIMEOUT=180
NSELF_CLEANUP_ON_START=auto
NSELF_LOG_LEVEL=info
```

## Troubleshooting

### Services timing out but actually running

Adjust the health check requirements:

```bash
# Lower the required percentage
NSELF_HEALTH_CHECK_REQUIRED=70

# Or increase timeout
NSELF_HEALTH_CHECK_TIMEOUT=180
```

### Port conflicts

Use cleanup options:

```bash
# Force cleanup
NSELF_CLEANUP_ON_START=always nself start

# Or use fresh mode
nself start --fresh
```

### Slow startup times

Optimize parallel limits and timeouts:

```bash
# Increase parallel starts
NSELF_PARALLEL_LIMIT=10

# Reduce health check interval
NSELF_HEALTH_CHECK_INTERVAL=1
```

## Default Values Summary

| Variable | Default | Description |
|----------|---------|-------------|
| `NSELF_START_MODE` | `smart` | Container start strategy |
| `NSELF_HEALTH_CHECK_TIMEOUT` | `120` | Seconds to wait for health |
| `NSELF_HEALTH_CHECK_INTERVAL` | `2` | Seconds between health checks |
| `NSELF_HEALTH_CHECK_REQUIRED` | `80` | Percent of services required healthy |
| `NSELF_SKIP_HEALTH_CHECKS` | `false` | Skip health validation |
| `NSELF_DOCKER_BUILD_TIMEOUT` | `300` | Seconds for Docker builds |
| `NSELF_CLEANUP_ON_START` | `auto` | Container cleanup strategy |
| `NSELF_PARALLEL_LIMIT` | `5` | Parallel container starts |
| `NSELF_LOG_LEVEL` | `info` | Output verbosity |

## Migration from Previous Version

The new start command is fully backward compatible. Existing workflows continue to work without any changes. The enhancements only activate when you explicitly set the environment variables or use the new command flags.

## See Also

- [Configuration Guide](README.md)
- [Environment Variables Reference](./ENVIRONMENT-VARIABLES.md)
- [Docker Compose Configuration](../architecture/BUILD_ARCHITECTURE.md)
- [Troubleshooting Guide](../guides/Troubleshooting.md)