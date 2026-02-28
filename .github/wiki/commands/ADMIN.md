# nself admin - Admin Dashboard

**Version 0.9.9** | Open the nself admin dashboard

---

## Overview

The `nself admin` command opens the nself-admin web dashboard, providing a graphical interface for managing services, viewing logs, monitoring metrics, and performing database operations.

---

## Basic Usage

```bash
# Open admin dashboard in browser
nself admin

# Show admin URL only
nself admin --url

# Start admin if not running
nself admin --start
```

---

## Dashboard Features

### Service Management
- View all service status
- Start/stop/restart services
- View service configurations

### Log Viewer
- Real-time log streaming
- Multi-service log aggregation
- Search and filter logs

### Database Browser
- Query editor with syntax highlighting
- Table browser
- Data export

### Metrics Dashboard
- CPU/Memory usage
- Request rates
- Error rates

---

## Requirements

The admin dashboard requires `NSELF_ADMIN_ENABLED=true` in your `.env` file.

```bash
# Enable admin dashboard
echo "NSELF_ADMIN_ENABLED=true" >> .env
nself build
nself start
```

---

## Access URL

Default: `https://admin.local.nself.org`

Or with custom domain: `https://admin.{BASE_DOMAIN}`

---

## Authentication

Default credentials (development):
- **Username**: admin
- **Password**: (from `NSELF_ADMIN_PASSWORD` in .env)

---

## Options Reference

| Option | Description |
|--------|-------------|
| `--url` | Show URL only, don't open browser |
| `--start` | Start admin if not running |
| `--port` | Custom port |

---

## See Also

- [status](STATUS.md) - CLI service status
- [logs](LOGS.md) - CLI log viewer
- [urls](URLS.md) - Show all URLs
