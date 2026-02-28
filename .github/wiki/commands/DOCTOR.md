# nself doctor - System Diagnostics

**Version 0.9.9** | Run comprehensive system health checks

---

## Overview

The `nself doctor` command runs diagnostic checks on your system and nself installation. It verifies dependencies, checks port availability, validates configuration, and identifies potential issues before they cause problems.

---

## Basic Usage

```bash
# Run all diagnostics
nself doctor

# Quick check (essential only)
nself doctor --quick

# Verbose output with details
nself doctor --verbose

# Check specific category
nself doctor --check docker
nself doctor --check ports
nself doctor --check config
```

---

## Diagnostic Categories

### System Requirements

| Check | Description |
|-------|-------------|
| Docker | Docker daemon running and accessible |
| Docker Compose | Docker Compose v2+ installed |
| Bash | Bash version 3.2+ |
| curl | HTTP client for API checks |
| git | Version control |
| openssl | SSL certificate operations |

### Port Availability

Checks if required ports are available:

| Port | Service |
|------|---------|
| 80 | HTTP (Nginx) |
| 443 | HTTPS (Nginx) |
| 5432 | PostgreSQL |
| 8080 | Hasura |
| 4000 | Auth Service |

### Configuration Validation

- `.env` file exists and is readable
- Required environment variables set
- No conflicting port assignments
- Valid domain configuration

### Docker Health

- Docker daemon responsive
- Sufficient disk space
- Network connectivity
- Image pull capability

---

## Output Example

```
nself Doctor - System Diagnostics
═══════════════════════════════════════════════════════════════

System Requirements
─────────────────────────────────────────────────────────────────
  ✓ Docker is available: Docker version 24.0.7
  ✓ Docker Compose is available: Docker Compose version v2.23.0
  ✓ Bash is available: GNU bash, version 5.2.15
  ✓ curl is available: curl 8.4.0
  ✓ git is available: git version 2.42.0
  ✓ openssl is available: OpenSSL 3.1.4

Port Availability
─────────────────────────────────────────────────────────────────
  ✓ Port 80 is available
  ✓ Port 443 is available
  ✓ Port 5432 is available
  ✓ Port 8080 is available

Configuration
─────────────────────────────────────────────────────────────────
  ✓ .env file found
  ✓ PROJECT_NAME is set: myapp
  ✓ BASE_DOMAIN is set: localhost

Summary
─────────────────────────────────────────────────────────────────
  ✓ All checks passed! System is ready for nself.
```

---

## Options Reference

| Option | Description |
|--------|-------------|
| `--quick` | Run essential checks only |
| `--verbose` | Show detailed output |
| `--check <category>` | Check specific category |
| `--fix` | Attempt to fix issues automatically |
| `--json` | Output results as JSON |

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All checks passed |
| 1 | One or more checks failed |
| 2 | Critical issue found |

---

## Common Issues

### Docker Not Running

```
✗ Docker daemon is not running
```

**Fix:** Start Docker Desktop or the Docker daemon:
```bash
# macOS/Windows
open -a Docker

# Linux
sudo systemctl start docker
```

### Port Already in Use

```
⚠ Port 5432 is already in use
```

**Fix:** Stop the conflicting service or change the port in `.env`:
```bash
# Find what's using the port
lsof -i :5432

# Or change in .env
POSTGRES_PORT=5433
```

### Missing Environment File

```
✗ .env file not found
```

**Fix:** Run `nself init` to create the configuration:
```bash
nself init
```

---

## See Also

- [status](STATUS.md) - Check service status
- [health](HEALTH.md) - Health monitoring
- [config](CONFIG.md) - Configuration management
