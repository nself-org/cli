# nself admin-dev

Quick toggle for nself-admin local development mode.

## Description

The `admin-dev` command is a convenience wrapper for nself-admin contributors to switch between running the admin UI from Docker vs locally. This command automatically rebuilds nginx configuration and restarts services when toggling development mode.

**Use Case:** When developing the nself-admin UI locally, you need nginx to route traffic to your local dev server instead of the Docker container.

---

## Usage

```bash
nself admin-dev <action> [options]
```

---

## Actions

### `on` - Enable Development Mode

Enable local development mode with auto-rebuild.

```bash
nself admin-dev on [port] [path]
```

**Parameters:**
- `port` (optional) - Port number for local dev server (default: 3025)
- `path` (optional) - Path to nself-admin repository

**Examples:**
```bash
# Enable on default port 3025
nself admin-dev on

# Enable on custom port
nself admin-dev on 3000

# Enable with custom port and path
nself admin-dev on 3000 ~/Sites/nself-admin

# Enable with path only (auto-detects port from package.json)
nself admin-dev on ~/Sites/nself-admin
```

**What it does:**
1. Updates environment file with dev mode settings
2. Rebuilds nginx configuration
3. Restarts nginx container
4. Shows instructions for starting local dev server

### `off` - Disable Development Mode

Disable local development mode and return to Docker container.

```bash
nself admin-dev off
```

**Example:**
```bash
nself admin-dev off
```

**What it does:**
1. Removes dev mode settings from environment file
2. Rebuilds nginx configuration
3. Restarts nginx and nself-admin containers
4. Routes admin.* back to Docker container

### `status` - Show Current Status

Display current development mode status.

```bash
nself admin-dev status
```

**Example:**
```bash
nself admin-dev status
```

**Shows:**
- Current mode (ON or OFF)
- Port number (if enabled)
- Path (if configured)
- Admin UI URL
- Instructions for toggling

---

## Options

| Option | Description |
|--------|-------------|
| `-h, --help` | Show help message |

---

## Configuration

When enabled, the following environment variables are set:

```bash
NSELF_ADMIN_DEV=true
NSELF_ADMIN_DEV_PORT=3025
NSELF_ADMIN_DEV_PATH=/path/to/nself-admin  # optional
```

### Environment File Selection

The command automatically detects and modifies the correct environment file:

- **Local development:** `.env`
- **Staging:** `.env.staging` (if it exists)
- **Production:** `.env.prod` (if it exists)

---

## Examples

### Basic Workflow

```bash
# 1. Enable dev mode
nself admin-dev on

# 2. Start local dev server (in nself-admin repo)
cd ~/Sites/nself-admin
PORT=3025 pnpm dev

# 3. Access admin UI (routes to localhost:3025)
open https://admin.local.nself.org

# 4. When done, disable dev mode
nself admin-dev off
```

### Custom Port

```bash
# Enable on port 3000
nself admin-dev on 3000

# Start dev server on port 3000
cd ~/Sites/nself-admin
PORT=3000 pnpm dev
```

### With Custom Path

```bash
# Enable with path (auto-detects port from package.json)
nself admin-dev on ~/Sites/nself-admin

# Start dev server
cd ~/Sites/nself-admin
pnpm dev
```

### Check Status

```bash
nself admin-dev status
```

**Output:**
```
Admin Development Mode Status
==============================

Status: ON (local development)
Port:   3025
Path:   /Users/admin/Sites/nself-admin
URL:    https://admin.local.nself.org

Nginx routes admin.* to localhost:3025

To disable: nself admin-dev off
```

---

## How It Works

### Enable Flow

1. **Update Environment:** Adds `NSELF_ADMIN_DEV=true` to active env file
2. **Rebuild Nginx:** Runs `nself build` to regenerate nginx config
3. **Restart Services:** Restarts nginx container to apply changes
4. **Show Instructions:** Displays command to start local dev server

### Nginx Routing

When dev mode is **ON**:
```nginx
# admin.local.nself.org → localhost:3025
location / {
  proxy_pass http://host.docker.internal:3025;
}
```

When dev mode is **OFF**:
```nginx
# admin.local.nself.org → Docker container
location / {
  proxy_pass http://nself-admin:3025;
}
```

---

## Troubleshooting

### Port Already in Use

```bash
# Check what's using the port
lsof -i :3025

# Kill the process or use a different port
nself admin-dev on 3026
```

### Nginx Not Restarting

```bash
# Manually restart nginx
docker restart ${PROJECT_NAME}_nginx

# Or stop and start all services
nself restart
```

### Changes Not Reflecting

```bash
# Rebuild and restart
nself build
nself restart nginx
```

### Dev Mode Stuck

```bash
# Force disable
nself admin-dev off

# Or manually remove from .env
grep -v "NSELF_ADMIN_DEV" .env > .env.tmp
mv .env.tmp .env
nself build
nself restart
```

---

## Related Commands

- **[service](SERVICE.md)** - Manage individual services
- **[build](BUILD.md)** - Rebuild configuration
- **[restart](RESTART.md)** - Restart services
- **[admin](ADMIN.md)** - Admin UI operations

---

## Notes

- This command is for **nself-admin contributors** developing the admin UI
- Regular users don't need this command
- The admin UI must be running on the specified port for routing to work
- Auto-rebuild ensures nginx config is always in sync
- Dev mode persists across restarts until explicitly disabled

---

**Version:** v0.4.7+
**Category:** Development Tools
**Related Documentation:** [Admin UI Guide](../configuration/Admin-UI.md)
