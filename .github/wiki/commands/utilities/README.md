# nself Utilities

Helper commands for managing, monitoring, and troubleshooting your nself project.

## Overview

Utility commands provide essential tools for day-to-day operations, debugging, and system administration.

## Quick Navigation

| Command | Description |
|---------|-------------|
| [status](status.md) | Check service status and health |
| [logs](logs.md) | View and tail service logs |
| [urls](urls.md) | Display all service URLs |
| [health](health.md) | Run comprehensive health checks |
| [doctor](doctor.md) | Diagnose and fix common issues |
| [exec](exec.md) | Execute commands in containers |
| [version](version.md) | Show version information |
| [help](help.md) | Display help documentation |
| [monitor](monitor.md) | Real-time monitoring dashboard |
| [admin](#nself-admin) | Open admin web UI |
| [update](#nself-update) | Update nself CLI |
| [completion](#nself-completion) | Shell completions |
| [metrics](#nself-metrics) | Performance metrics |
| [history](#nself-history) | Command history and audit trail |
| [audit](#nself-audit) | Security audit logs |

## Common Workflows

### Daily Operations

```bash
# Check if services are running
nself status

# View recent logs
nself logs --tail 100

# Access service URLs
nself urls

# Open admin dashboard
nself admin
```

### Troubleshooting

```bash
# Run diagnostics
nself doctor

# Check health
nself health --verbose

# Monitor in real-time
nself monitor

# View specific service logs
nself logs postgres --follow
```

### Debugging

```bash
# Execute command in container
nself exec postgres psql -U postgres

# View detailed health checks
nself health --verbose postgres

# Export diagnostics
nself doctor --export diagnostics.txt
```

### Monitoring

```bash
# Real-time dashboard
nself monitor

# Service status
nself status --resources

# Live logs
nself logs -f --all

# Health checks
nself health --check-updates
```

## Utilities by Category

### Status & Monitoring

- **status** - Quick service status overview
- **health** - Detailed health diagnostics
- **monitor** - Live monitoring dashboard
- **metrics** - Performance metrics and analytics

### Logs & Debugging

- **logs** - View and stream logs
- **exec** - Execute commands in containers
- **doctor** - Automated diagnostics and fixes

### Information

- **urls** - Service endpoints and access info
- **version** - Version information
- **help** - Command documentation

### Administration

- **admin** - Web-based admin UI
- **update** - Update nself CLI
- **completion** - Shell auto-completion
- **history** - Command history
- **audit** - Security audit logs

## Usage Patterns

### Quick Health Check

```bash
# Fast overview
nself status --quiet && echo "All services healthy"

# Detailed check
nself health

# Full diagnostics
nself doctor
```

### Log Analysis

```bash
# Recent errors
nself logs --level error --since 1h

# Specific service
nself logs postgres --tail 50

# Multiple services
nself logs postgres hasura auth
```

### Performance Monitoring

```bash
# Resource usage
nself status --resources

# Live monitoring
nself monitor --view metrics

# Export metrics
nself metrics --export metrics.json
```

## Command Aliases

Many utilities have short aliases for convenience:

```bash
nself ps       # → nself status
nself up       # → nself start
nself down     # → nself stop
nself ls       # → nself urls
nself v        # → nself version
```

## Output Formats

Most utilities support multiple output formats:

```bash
# Human-readable (default)
nself status

# JSON (for scripting)
nself status --format json

# YAML
nself status --format yaml

# Quiet (exit codes only)
nself status --quiet
```

## Exit Codes

Utilities use standard exit codes:

| Code | Meaning |
|------|---------|
| 0 | Success / Healthy |
| 1 | Warning / Partial failure |
| 2 | Error / Unhealthy |
| 3 | Critical / Service down |

**Use in scripts**:
```bash
if nself status --quiet; then
  echo "Services healthy"
else
  echo "Services need attention"
  nself doctor
fi
```

## Integration

### Shell Scripts

```bash
#!/bin/bash
# Check services before deployment

if ! nself health --quick; then
  echo "Services unhealthy, aborting"
  exit 1
fi

# Deploy
git pull
nself build
nself restart
```

### CI/CD

```yaml
# .github/workflows/deploy.yml
- name: Health check
  run: nself health

- name: Check logs
  if: failure()
  run: nself logs --tail 100
```

### Monitoring

```bash
# Continuous monitoring
while true; do
  nself status --format json >> status-log.jsonl
  sleep 60
done
```

## See Also

- [Core Commands](../core/README.md)
- [Database Commands](../db/README.md)
- [Service Management](../../guides/SERVICE-MANAGEMENT.md)
- [Troubleshooting Guide](../../guides/TROUBLESHOOTING.md)
