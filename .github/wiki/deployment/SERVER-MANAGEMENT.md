# Deploy Server Management - Complete Guide

**Command:** `nself deploy server` and `nself deploy sync`

Comprehensive server management, initialization, diagnostics, and environment synchronization for nself deployments.

---

## Quick Navigation

- [Overview](#overview)
- [Server Management Commands](#server-management-commands)
- [Sync Commands](#sync-operations)
- [Common Workflows](#common-workflows)
- [Security](#security-considerations)
- [Troubleshooting](#troubleshooting)

---

Complete documentation for the 10 newly implemented deploy server management features.

## Overview

The `nself deploy server` and `nself deploy sync` commands provide comprehensive server management, initialization, diagnostics, and environment synchronization.

## Implemented Features

### 1. Server Initialization (`deploy server init`)

**Purpose**: Initialize a fresh VPS server for nself deployment with complete automation.

**Usage**:
```bash
nself deploy server init <host> [options]
nself deploy server init root@server.example.com --domain example.com
```

**Options**:
- `--host, -h` - Server hostname or IP
- `--user, -u` - SSH user (default: root)
- `--port, -p` - SSH port (default: 22)
- `--key, -k` - SSH private key file
- `--domain, -d` - Domain for SSL setup
- `--env, -e` - Environment name (default: prod)
- `--skip-ssl` - Skip SSL certificate setup
- `--skip-dns` - Skip DNS fallback configuration
- `--yes, -y` - Skip confirmation prompts

**What It Does**:

**Phase 1: System Setup**
- Updates system packages (apt-get or yum)
- Installs Docker and Docker Compose from official repositories
- Enables and starts Docker service
- Verifies installation

**Phase 2: Security Configuration**
- Installs and configures UFW firewall
  - Denies all incoming by default
  - Allows SSH, HTTP (80), HTTPS (443)
- Installs and configures fail2ban
  - Protects against SSH brute force
  - 5 retries, 1-hour ban, 10-minute window
- Hardens SSH configuration
  - Disables password authentication
  - Enables public key authentication only
  - Disables X11 forwarding

**Phase 3: nself Environment**
- Creates `/var/www/nself` directory structure
- Creates backup and log directories
- Configures DNS fallback (Cloudflare, Google DNS)
- Sets up SSL if domain provided and resolves
- Creates environment marker file

**Example**:
```bash
# Basic initialization
nself deploy server init root@192.168.1.100

# With domain for SSL
nself deploy server init root@server.example.com --domain example.com

# Non-interactive for automation
nself deploy server init root@server.example.com --domain example.com --yes

# Custom SSH port and key
nself deploy server init root@server.example.com --port 2222 --key ~/.ssh/deploy_key
```

**Output**:
```
╔════════════════════════════════════════════════════════════════╗
║  nself deploy server init                                      ║
║  Initialize VPS for nself deployment                           ║
╚════════════════════════════════════════════════════════════════╝

Server Configuration
  Host:     server.example.com
  User:     root
  Port:     22
  Domain:   example.com
  Env:      prod

This will:
  1. Update system packages
  2. Install Docker and Docker Compose
  3. Configure firewall (UFW)
  4. Setup fail2ban for SSH protection
  5. Configure DNS fallback (optional)
  6. Setup Let's Encrypt SSL (optional)

Continue? [y/N]:
```

---

### 2. Server Readiness Check (`deploy server check`)

**Purpose**: Verify a server is ready for nself deployment with 8 comprehensive checks.

**Usage**:
```bash
nself deploy server check <host>
nself deploy server check user@host:port
```

**Checks Performed**:
1. **SSH Connectivity** - Can connect to server
2. **Docker Installation** - Docker is installed and version
3. **Docker Service** - Docker daemon is running
4. **Docker Compose** - Compose plugin is available
5. **Disk Space** - Available disk space and usage percentage
6. **Memory** - Total and available RAM
7. **Firewall** - UFW status (active/inactive)
8. **Required Ports** - Ports 80 and 443 availability

**Example**:
```bash
nself deploy server check root@server.example.com
```

**Output**:
```
╔════════════════════════════════════════════════════════════════╗
║  nself deploy server check                                     ║
║  Verify server readiness for deployment                        ║
╚════════════════════════════════════════════════════════════════╝

  [1/8] SSH Connectivity... PASS
  [2/8] Docker Installation... PASS (v24.0.7)
  [3/8] Docker Service... PASS
  [4/8] Docker Compose... PASS (v2.23.0)
  [5/8] Disk Space... PASS (45G available, 15% used)
  [6/8] Memory... PASS (4.0G total, 3.2G available)
  [7/8] Firewall... PASS (active)
  [8/8] Required Ports (80, 443)... PASS (available)

Check Summary
  Passed: 8/8

✓ Server is ready for deployment
```

**Status Codes**:
- **PASS** (green) - Check passed
- **WARN** (yellow) - Warning, not critical
- **FAIL** (red) - Check failed

**Recommendations**:
- 8/8 passed: Ready for deployment
- 6-7 passed: Mostly ready (warnings)
- <6 passed: Not ready, run `server init`

---

### 3. Server Status (`deploy server status`)

**Purpose**: Quick status check of all configured remote servers.

**Usage**:
```bash
nself deploy server status
```

**What It Shows**:
- All environments with server configurations
- Connection status (online/offline)
- Server uptime if online
- Summary statistics

**Example**:
```bash
nself deploy server status
```

**Output**:
```
╔════════════════════════════════════════════════════════════════╗
║  nself deploy server status                                    ║
║  Check server connectivity                                     ║
╚════════════════════════════════════════════════════════════════╝

  staging         ● ONLINE   up 2 days, 5 hours
  production      ● ONLINE   up 15 days, 3 hours
  backup          ● OFFLINE  root@backup.example.com:22

  Total: 3 server(s), 2 online, 1 offline
```

**Indicators**:
- `●` (green) - Server is online and reachable
- `●` (red) - Server is offline or unreachable

---

### 4. Server Diagnostics (`deploy server diagnose`)

**Purpose**: Comprehensive server diagnostics with detailed network and system information.

**Usage**:
```bash
nself deploy server diagnose <environment>
```

**Diagnostics Performed**:

**Network Diagnostics**:
1. DNS Resolution - Resolves hostname to IP
2. ICMP Ping - Network reachability and latency
3. Port 22 (SSH) - SSH port accessibility
4. Port 80 (HTTP) - HTTP port availability
5. Port 443 (HTTPS) - HTTPS port availability

**SSH Connection Test**:
- Attempts SSH connection
- Retrieves server information if successful

**Server Information** (if connected):
- Hostname
- Operating system
- Kernel version
- Uptime and load average
- Memory capacity
- Docker version

**Example**:
```bash
nself deploy server diagnose prod
```

**Output**:
```
╔════════════════════════════════════════════════════════════════╗
║  nself deploy server diagnose                                  ║
║  Full server diagnostics                                       ║
╚════════════════════════════════════════════════════════════════╝

Environment: prod
  Host: server.example.com
  User: root
  Port: 22

Network Diagnostics
  [1/5] DNS Resolution... OK → 192.168.1.100
  [2/5] ICMP Ping... OK (15.3 ms)
  [3/5] Port 22 (SSH)... OPEN
  [4/5] Port 80 (HTTP)... OPEN
  [5/5] Port 443 (HTTPS)... OPEN

SSH Connection Test
  Attempting SSH connection... SUCCESS

Server Information
  hostname:    prod-server-01
  os:          ubuntu
  kernel:      5.15.0-91-generic
  uptime:      up 2 days, 5 hours
  load:        0.15, 0.20, 0.18
  memory:      4.0G
  docker:      24.0.7

✓ Diagnostics complete
```

**Recommendations** (if connection fails):
- Verify SSH key authorization
- Check SSH is running on correct port
- Ensure firewall allows SSH
- Try manual connection command

---

### 5. Server List (`deploy server list`)

**Purpose**: List all configured servers across environments.

**Usage**:
```bash
nself deploy server list
```

**Information Shown**:
- Environment name
- Server hostname
- SSH user
- SSH port
- Connection status (online/offline)

**Example**:
```bash
nself deploy server list
```

**Output**:
```
╔════════════════════════════════════════════════════════════════╗
║  nself deploy server                                           ║
║  Server List                                                   ║
╚════════════════════════════════════════════════════════════════╝

NAME            HOST                      USER       PORT       STATUS
--------------- ------------------------- ---------- ---------- ----------
staging         staging.example.com       deploy     22         online
production      prod.example.com          root       22         online
backup          backup.example.com        root       2222       offline

Total: 3 server(s)
```

**Status Indicator**:
- Quick connectivity check (2-second timeout)
- Shows online/offline status
- Color-coded: green (online), red (offline)

---

### 6. Server Add (`deploy server add`)

**Purpose**: Add or update server configuration for an environment.

**Usage**:
```bash
nself deploy server add <name> --host <host> [options]
```

**Options**:
- `--host, -h` - Server hostname or IP (required)
- `--user, -u` - SSH user (default: root)
- `--port, -p` - SSH port (default: 22)
- `--key, -k` - SSH private key file path
- `--path` - Deployment path (default: /var/www/nself)

**What It Does**:
- Creates environment directory if it doesn't exist
- Creates or updates `server.json` configuration
- Stores SSH connection details
- Records creation timestamp

**Example**:
```bash
# Basic server addition
nself deploy server add staging --host staging.example.com

# With custom user and port
nself deploy server add prod --host prod.example.com --user deploy --port 2222

# With SSH key
nself deploy server add prod --host prod.example.com --key ~/.ssh/production_key

# With custom deploy path
nself deploy server add staging --host staging.example.com --path /opt/myapp
```

**Output**:
```
✓ Server added: staging

Server details:
  Host:        staging.example.com
  User:        root
  Port:        22
  Deploy path: /var/www/nself

ℹ Test connection with: nself deploy server check staging
```

**server.json Format**:
```json
{
  "name": "staging",
  "type": "remote",
  "host": "staging.example.com",
  "port": 22,
  "user": "root",
  "key": "",
  "deploy_path": "/var/www/nself",
  "description": "Remote server configuration",
  "created_at": "2026-01-30T12:34:56Z"
}
```

---

### 7. Server Remove (`deploy server remove`)

**Purpose**: Remove server configuration from an environment.

**Usage**:
```bash
nself deploy server remove <name> [--force]
```

**Options**:
- `--force, -f` - Skip confirmation prompt

**What It Does**:
- Removes `server.json` configuration
- Preserves environment directory and .env files
- Requires confirmation unless `--force` used

**Safety**:
- Does NOT delete remote server data
- Does NOT delete local environment directory
- Only removes server configuration

**Example**:
```bash
# Remove with confirmation
nself deploy server remove old-server

# Remove without confirmation
nself deploy server remove old-server --force
```

**Output**:
```
This will remove server configuration:
  Name: old-server
  Host: old.example.com

WARNING: This will NOT delete the environment or remote data
         Only the server.json configuration will be removed

Are you sure? [y/N]: y

✓ Server configuration removed: old-server

ℹ The environment directory still exists at: .environments/old-server
ℹ To completely remove the environment, use: nself env delete old-server
```

---

### 8. Server SSH (`deploy server ssh`)

**Purpose**: Quick SSH connection to a configured server or execute remote commands.

**Usage**:
```bash
# Interactive SSH session
nself deploy server ssh <name>

# Execute remote command
nself deploy server ssh <name> <command>
```

**Features**:
- Uses stored SSH configuration (host, user, port, key)
- Supports interactive sessions
- Supports command execution
- Automatically applies correct SSH options

**Example**:
```bash
# Connect interactively
nself deploy server ssh staging

# Execute remote command
nself deploy server ssh staging "docker ps"

# Check disk space
nself deploy server ssh prod "df -h"

# View logs
nself deploy server ssh staging "tail -f /var/log/nginx/access.log"
```

**Output (interactive)**:
```
ℹ Connecting to staging (root@staging.example.com:22)...

root@staging:~#
```

**Output (command execution)**:
```
ℹ Executing on staging: docker ps

CONTAINER ID   IMAGE              STATUS         PORTS
a1b2c3d4e5f6   nginx:latest       Up 2 days      80/tcp, 443/tcp
```

---

### 9. Server Info (`deploy server info`)

**Purpose**: Display comprehensive information about a configured server.

**Usage**:
```bash
nself deploy server info <name>
```

**Information Shown**:
- **Connection Details**: Host, user, port, SSH key, deploy path
- **Connectivity Test**: Real-time SSH connection test
- **Remote System Info**: Hostname, OS, kernel, CPU, memory, disk, uptime
- **Deployment Status**: Whether nself is deployed, container counts
- **Quick Actions**: Common commands for this server

**Example**:
```bash
nself deploy server info prod
```

**Output**:
```
╔════════════════════════════════════════════════════════════════╗
║  nself deploy server info                                      ║
║  Server Details: prod                                          ║
╚════════════════════════════════════════════════════════════════╝

Connection Details
  Name:        prod
  Host:        prod.example.com
  User:        root
  Port:        22
  Type:        remote
  SSH Key:     ~/.ssh/production_key
  Deploy Path: /var/www/nself
  Description: Production server

Connectivity
  Testing SSH connection... CONNECTED

Remote System Information
  hostname:      prod-server-01
  os:            Ubuntu 22.04.3 LTS
  kernel:        5.15.0-91-generic
  arch:          x86_64
  cpu_cores:     4
  memory:        8.0G
  disk_root:     50G
  disk_avail:    38G
  uptime:        up 15 days, 3 hours
  docker:        24.0.7
  compose:       2.23.0

Deployment Status
  Status:      Deployed
  Containers:  24/24 running

Quick Actions
  Connect:     nself deploy server ssh prod
  Diagnose:    nself deploy server diagnose prod
  Deploy:      nself deploy prod
```

---

### 10. Sync Operations (`deploy sync`)

Comprehensive environment synchronization between local and remote servers.

#### 10a. Sync Pull (`deploy sync pull`)

**Purpose**: Pull configuration files from remote environment to local.

**Usage**:
```bash
nself deploy sync pull <environment> [options]
```

**Options**:
- `--dry-run` - Preview without making changes
- `--force, -f` - Skip confirmation prompt

**What It Pulls**:
- `.env` - Environment configuration
- `.env.secrets` - Secret credentials
- `docker-compose.yml` - Docker configuration

**Example**:
```bash
# Pull from staging
nself deploy sync pull staging

# Dry run first
nself deploy sync pull staging --dry-run

# Force without confirmation
nself deploy sync pull prod --force
```

**Output**:
```
╔════════════════════════════════════════════════════════════════╗
║  nself deploy sync pull                                        ║
║  Pull configuration from staging                               ║
╚════════════════════════════════════════════════════════════════╝

Sync Configuration
  Source:      root@staging.example.com:/var/www/nself
  Destination: .environments/staging

✓ Connected

Files to Pull
  - .env
  - .env.secrets
  - docker-compose.yml

This will overwrite local files. Continue? [y/N]: y

ℹ Pulling files...
  Pulling .env... OK
  Pulling .env.secrets... OK
  Pulling docker-compose.yml... OK

✓ Sync complete: staging → local

ℹ Files synced to: .environments/staging
```

#### 10b. Sync Push (`deploy sync push`)

**Purpose**: Push configuration files from local to remote environment.

**Usage**:
```bash
nself deploy sync push <environment> [options]
```

**Options**:
- `--dry-run` - Preview without making changes
- `--force, -f` - Skip confirmation prompt

**What It Pushes**:
- `.env` - Environment configuration
- `.env.secrets` - Secret credentials (with chmod 600)

**Safety**:
- Shows warning for production environments
- Requires confirmation unless `--force`
- Sets proper permissions on secrets

**Example**:
```bash
# Push to staging
nself deploy sync push staging

# Dry run first
nself deploy sync push staging --dry-run

# Force without confirmation
nself deploy sync push staging --force
```

**Output**:
```
╔════════════════════════════════════════════════════════════════╗
║  nself deploy sync push                                        ║
║  Push configuration to staging                                 ║
╚════════════════════════════════════════════════════════════════╝

Sync Configuration
  Source:      .environments/staging
  Destination: root@staging.example.com:/var/www/nself

✓ Connected

Files to Push
  - .env
  - .env.secrets

This will overwrite remote files. Continue? [y/N]: y

ℹ Pushing files...
  Pushing .env... OK
  Pushing .env.secrets... OK

✓ Sync complete: local → staging

ℹ Files synced to: root@staging.example.com:/var/www/nself
```

#### 10c. Sync Status (`deploy sync status`)

**Purpose**: Show synchronization status for all environments.

**Usage**:
```bash
nself deploy sync status
```

**Information Shown**:
- Environment name
- Sync status (synced/not synced)
- Last sync timestamp
- Files status (complete/partial/missing)

**Example**:
```bash
nself deploy sync status
```

**Output**:
```
╔════════════════════════════════════════════════════════════════╗
║  nself deploy sync                                             ║
║  Synchronization Status                                        ║
╚════════════════════════════════════════════════════════════════╝

ENVIRONMENT     STATUS     LAST SYNC                 FILES
--------------- ---------- ------------------------- ----------
staging         synced     2026-01-30T14:23:15Z     complete
production      synced     2026-01-28T09:15:43Z     complete
backup          not synced never                     partial

Legend:
  complete - .env and .env.secrets present
  partial  - only .env present
  missing  - configuration files missing

ℹ Sync files between environments:
  Pull: nself deploy sync pull <environment>
  Push: nself deploy sync push <environment>
```

#### 10d. Sync Full (`deploy sync full`)

**Purpose**: Perform complete synchronization including configs, services, and restart.

**Usage**:
```bash
nself deploy sync full <environment> [options]
```

**Options**:
- `--dry-run` - Preview without making changes
- `--force, -f` - Skip confirmation prompt
- `--no-rebuild` - Skip service restart

**What It Syncs**:
1. Environment files (.env, .env.secrets)
2. Docker Compose configuration
3. Nginx configuration directory
4. Custom services directory
5. Restarts services on remote (optional)

**Example**:
```bash
# Full sync to staging
nself deploy sync full staging

# Dry run first
nself deploy sync full staging --dry-run

# Sync without restart
nself deploy sync full staging --no-rebuild

# Force sync
nself deploy sync full prod --force
```

**Output**:
```
╔════════════════════════════════════════════════════════════════╗
║  nself deploy sync full                                        ║
║  Full synchronization to staging                               ║
╚════════════════════════════════════════════════════════════════╝

Full Sync Plan
  1. Sync environment files (.env, .env.secrets)
  2. Sync docker-compose.yml and configs
  3. Sync nginx configuration
  4. Sync custom services
  5. Restart services on remote

This will perform a full sync to staging. Continue? [y/N]: y

✓ Connected

Step 1: Environment Files
  Syncing .env... OK
  Syncing .env.secrets... OK

Step 2: Docker Configuration
  Syncing docker-compose.yml... OK

Step 3: Nginx Configuration
  Syncing nginx directory... OK

Step 4: Custom Services
  Syncing services directory... OK

Step 5: Restart Services
  Restarting services on remote... OK

✓ Full sync complete: 5 file(s) synced

ℹ Next: nself deploy staging
```

---

## Common Workflows

### Initialize a New Production Server

```bash
# 1. Initialize the server
nself deploy server init root@prod.example.com --domain example.com --yes

# 2. Create production environment locally
nself env create prod

# 3. Add server configuration
nself deploy server add prod --host prod.example.com

# 4. Generate production secrets
nself config secrets generate --env prod

# 5. Build for production
nself build --env prod

# 6. Push configuration to server
nself deploy sync push prod

# 7. Deploy to production
nself deploy prod
```

### Check Server Health

```bash
# Quick status of all servers
nself deploy server status

# Detailed check of specific server
nself deploy server check root@server.example.com

# Full diagnostics for environment
nself deploy server diagnose prod

# Get comprehensive server info
nself deploy server info prod
```

### Sync Configurations

```bash
# Check sync status
nself deploy sync status

# Pull staging config to local
nself deploy sync pull staging

# Edit local files
vim .environments/staging/.env

# Push changes back to staging
nself deploy sync push staging

# Full sync including services
nself deploy sync full staging
```

### SSH Quick Access

```bash
# Interactive SSH
nself deploy server ssh prod

# Execute remote command
nself deploy server ssh prod "docker ps"

# View logs
nself deploy server ssh staging "tail -f /var/log/nginx/error.log"

# Check disk space
nself deploy server ssh prod "df -h"
```

---

## Security Considerations

### Server Initialization
- Disables password authentication (key-only)
- Enables UFW firewall with strict rules
- Configures fail2ban for SSH protection
- Hardens SSH configuration

### Secrets Management
- `.env.secrets` automatically set to chmod 600
- Never synced to version control
- Encrypted in transit via SSH
- Production warnings on push operations

### SSH Key Management
- Keys stored in server.json configuration
- Keys are gitignored by default
- Use separate keys per environment
- Store keys securely (encrypted volume)

---

## Troubleshooting

### Connection Issues

**Problem**: "Cannot connect to server"
```bash
# Check DNS resolution
host server.example.com

# Check network connectivity
ping server.example.com

# Check port accessibility
nc -zv server.example.com 22

# Full diagnostics
nself deploy server diagnose prod
```

**Problem**: "Permission denied (publickey)"
```bash
# Verify SSH key is specified
nself deploy server info prod

# Test SSH manually
ssh -i ~/.ssh/key root@server.example.com

# Add key to server
ssh-copy-id -i ~/.ssh/key.pub root@server.example.com
```

### Sync Issues

**Problem**: "Files not found on remote"
```bash
# Check if deployed
nself deploy server ssh prod "ls -la /var/www/nself"

# Create directory structure
nself deploy server init root@server.example.com

# Try full sync
nself deploy sync full prod
```

**Problem**: "Permission denied during sync"
```bash
# Check SSH key permissions
chmod 600 ~/.ssh/deploy_key

# Check remote directory permissions
nself deploy server ssh prod "ls -ld /var/www/nself"

# Fix remote permissions
nself deploy server ssh prod "chown -R root:root /var/www/nself"
```

---

## Configuration Reference

### server.json Structure
```json
{
  "name": "production",
  "type": "remote",
  "host": "prod.example.com",
  "port": 22,
  "user": "root",
  "key": "~/.ssh/production_key",
  "deploy_path": "/var/www/nself",
  "description": "Production server",
  "created_at": "2026-01-30T12:34:56Z"
}
```

### Environment Directory Structure
```
.environments/
├── prod/
│   ├── .env              # Environment configuration
│   ├── .env.secrets      # Sensitive credentials (chmod 600)
│   ├── server.json       # SSH connection details
│   └── .sync-history     # Sync history log (auto-generated)
└── staging/
    ├── .env
    ├── .env.secrets
    ├── server.json
    └── .sync-history
```

---

## Best Practices

1. **Always use `server check` before deployment**
   ```bash
   nself deploy server check prod
   ```

2. **Use dry-run for sync operations first**
   ```bash
   nself deploy sync push prod --dry-run
   ```

3. **Verify with `server info` before making changes**
   ```bash
   nself deploy server info prod
   ```

4. **Monitor sync status regularly**
   ```bash
   nself deploy sync status
   ```

5. **Use separate SSH keys per environment**
   - Development: `~/.ssh/dev_key`
   - Staging: `~/.ssh/staging_key`
   - Production: `~/.ssh/production_key`

6. **Always backup before full sync**
   ```bash
   nself backup create
   nself deploy sync full prod
   ```

7. **Use `--force` with caution**
   - Never use `--force` for production without review
   - Always verify with dry-run first

8. **Regular server diagnostics**
   ```bash
   # Weekly health check
   nself deploy server status
   nself deploy server diagnose prod
   ```

---

## Integration with Other Commands

### With `nself env`
```bash
# Create environment first
nself env create prod

# Then add server configuration
nself deploy server add prod --host prod.example.com
```

### With `nself config`
```bash
# Generate secrets
nself config secrets generate --env prod

# Validate before sync
nself config validate prod

# Then sync to remote
nself deploy sync push prod
```

### With `nself backup`
```bash
# Backup before sync
nself backup create

# Perform sync
nself deploy sync full prod

# Rollback if needed
nself backup rollback
```

---

---

## Server Management Commands

The `nself deploy server` command provides comprehensive server management with 10 core subcommands:

| Subcommand | Purpose | Safety Level |
|------------|---------|--------------|
| `init` | Initialize VPS for nself deployment | Destructive (SSH hardening) |
| `check` | Verify server readiness | Read-only |
| `status` | Quick status of all servers | Read-only |
| `diagnose` | Comprehensive server diagnostics | Read-only |
| `list` | List all configured servers | Read-only |
| `add` | Add server configuration | Modifies config |
| `remove` | Remove server configuration | Modifies config |
| `ssh` | Quick SSH connection or command | Interactive |
| `info` | Display comprehensive server info | Read-only |
| `sync` | Synchronize files (use `nself deploy sync`) | Modifies files |

---

## Command Examples by Use Case

### Initial Server Setup

```bash
# Complete server initialization workflow
# 1. Initialize the VPS
nself deploy server init root@prod.example.com --domain example.com --yes

# 2. Verify server is ready
nself deploy server check root@prod.example.com

# 3. Add to nself configuration
nself deploy server add prod --host prod.example.com

# 4. Test SSH connection
nself deploy server ssh prod "uptime"

# 5. View complete info
nself deploy server info prod
```

### Daily Operations

```bash
# Quick health check
nself deploy server status

# Detailed diagnostics for specific server
nself deploy server diagnose prod

# Execute remote commands
nself deploy server ssh staging "docker ps"
nself deploy server ssh prod "df -h"

# Interactive SSH session
nself deploy server ssh prod
```

### Configuration Management

```bash
# List all servers
nself deploy server list

# Add new server
nself deploy server add staging \
  --host staging.example.com \
  --user deploy \
  --port 2222 \
  --key ~/.ssh/staging_key

# Remove old server
nself deploy server remove old-prod --force

# Update existing (re-add)
nself deploy server add prod --host new-prod.example.com
```

### Health Monitoring

```bash
# Check all servers
nself deploy server status

# Deep dive on specific server
nself deploy server check prod
nself deploy server diagnose prod
nself deploy server info prod

# Monitor in script
#!/bin/bash
if nself deploy server check prod > /dev/null 2>&1; then
  echo "Server healthy"
else
  echo "Server issues detected"
  nself deploy server diagnose prod
fi
```

---

## Related Documentation

- [Production Deployment Guide](./PRODUCTION-DEPLOYMENT.md)
- [Environment Management](../configuration/ENVIRONMENT-VARIABLES.md)
- [Security Best Practices](../security/SECURITY-BEST-PRACTICES.md)
- [Destroy Command](../commands/DESTROY.md) - Safe infrastructure teardown
- [Troubleshooting Guide](../troubleshooting/README.md)
