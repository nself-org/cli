# nself destroy - Safe Infrastructure Destruction

> **Deprecated:** `nself destroy` is a compatibility wrapper. Use `nself infra destroy` instead.
> This stub will be removed in v1.0.0.

**Command:** `nself infra destroy` (was: `nself destroy`)

Safe destruction of project infrastructure with configurable scope and comprehensive safety features.

---

## Overview

The `destroy` command provides a controlled way to tear down nself infrastructure with multiple safety mechanisms, selective destruction options, and intelligent cleanup.

## Features

- **Multi-Level Safety**: Double confirmation for data destruction
- **Selective Destruction**: Target specific resources (containers, volumes, networks, files)
- **Dry Run Mode**: Preview destruction without executing
- **Force Mode**: Non-interactive destruction for automation
- **Intelligent Cleanup**: Removes Docker resources and generated files
- **Data Preservation**: Option to keep volumes while removing other resources
- **Detailed Reporting**: Shows exactly what will be destroyed

---

## Quick Reference

```bash
# Interactive destruction (safest)
nself destroy

# Preview what will be destroyed
nself destroy --dry-run

# Destroy but preserve data volumes
nself destroy --keep-volumes

# Only remove containers
nself destroy --containers-only

# Only remove generated files
nself destroy --generated-only

# Non-interactive (for scripts)
nself destroy --force
```

---

## Usage

```bash
nself destroy [OPTIONS]
```

### Options

| Option | Description |
|--------|-------------|
| `-f, --force` | Skip all confirmation prompts (DANGEROUS) |
| `-y, --yes` | Auto-confirm (same as --force) |
| `--dry-run` | Show what would be destroyed without executing |
| `--keep-volumes` | Preserve data volumes (keep databases, files, etc.) |
| `--containers-only` | Only remove Docker containers |
| `--volumes-only` | Only remove Docker volumes (requires confirmation) |
| `--networks-only` | Only remove Docker networks |
| `--generated-only` | Only remove generated files (docker-compose.yml, etc.) |
| `--verbose` | Show detailed output |
| `-h, --help` | Show help message |

---

## Destruction Scope

### What Gets Destroyed (Default Full Destruction)

**Docker Resources:**
- ✗ All Docker containers (running and stopped)
- ✗ All Docker volumes (databases, files, cache) - **DATA LOSS!**
- ✗ All Docker networks

**Generated Files:**
- ✗ `docker-compose.yml`
- ✗ `nginx/` directory
- ✗ `services/` directory (custom services)
- ✗ `monitoring/` directory (if exists)
- ✗ `ssl/` directory (if exists)
- ✗ `postgres/` directory (init scripts)
- ✗ `.env.runtime`

### What Gets Preserved (Always)

**Configuration:**
- ✓ `.env` files (all variants)
- ✓ `.env.dev`, `.env.staging`, `.env.prod`
- ✓ `.env.secrets`

**Code:**
- ✓ Source code and custom files
- ✓ Version control (`.git/`)
- ✓ Node modules and dependencies

---

## Examples

### Interactive Destruction (Recommended)

```bash
nself destroy
```

**Output:**
```
╔════════════════════════════════════════════════════════════════╗
║  nself destroy                                                 ║
║  Safe destruction of project infrastructure                   ║
╚════════════════════════════════════════════════════════════════╝

Destruction Plan:

  ✗ 25 containers (24 running, 1 stopped)
  ✗ 8 volumes (ALL DATA WILL BE LOST)
  ✗ 2 networks
  ✗ Generated files:
      - docker-compose.yml
      - nginx/ directory
      - services/ directory (custom services)
      - monitoring/ directory (if exists)
      - ssl/ directory (if exists)

Preserved:
  ✓ .env files (configuration)
  ✓ Source code and custom files

WARNING: This will permanently delete all data!
This includes:
  - PostgreSQL databases
  - Redis cache data
  - MinIO stored files
  - All other persistent data

Are you sure you want to destroy project 'myapp'? (yes/no): yes

FINAL WARNING: Type the project name 'myapp' to confirm: myapp

→ Beginning destruction sequence...

✓ Removed 25 containers
✓ Removed 8 volumes
✓ Removed 2 networks
✓ Removed 7 generated files/directories

✓ Destruction complete!
✓ Project 'myapp' has been destroyed

Next steps:

  nself init   - Initialize a new project
  nself build  - Rebuild from existing .env
```

### Dry Run (Preview Only)

```bash
nself destroy --dry-run
```

Shows exactly what would be destroyed without making any changes. Perfect for:
- Understanding impact before execution
- Verifying selective destruction options
- Automation testing

### Preserve Data Volumes

```bash
nself destroy --keep-volumes
```

Removes containers, networks, and generated files but preserves all data volumes:
- PostgreSQL databases remain intact
- Redis data preserved
- MinIO files preserved
- Can rebuild and restart without data loss

**Use case:** Clean rebuild while preserving data

### Selective Destruction

#### Containers Only
```bash
nself destroy --containers-only
```

Only stops and removes Docker containers. Leaves volumes, networks, and files intact.

**Use case:** Force restart all containers

#### Volumes Only
```bash
nself destroy --volumes-only
```

Only removes Docker volumes. Requires confirmation due to data loss.

**Use case:** Clean data while keeping configuration

#### Networks Only
```bash
nself destroy --networks-only
```

Only removes Docker networks. Safe operation with no data loss.

**Use case:** Fix network conflicts

#### Generated Files Only
```bash
nself destroy --generated-only
```

Only removes generated files (docker-compose.yml, nginx/, services/, etc.). No Docker operations.

**Use case:** Force regeneration of configs via `nself build`

### Non-Interactive (Automation)

```bash
nself destroy --force
```

**⚠️ DANGEROUS:** Skips all confirmations. Use only in automation with proper safeguards.

**Production Safety:** Never use `--force` for production environments without manual review.

---

## Safety Features

### 1. Interactive Confirmation

By default, requires explicit confirmation:
```
Are you sure you want to destroy project 'myapp'? (yes/no):
```

### 2. Double Confirmation for Data Loss

When destroying volumes, requires typing the project name:
```
FINAL WARNING: Type the project name 'myapp' to confirm:
```

### 3. Dry Run Mode

Preview destruction without making changes:
```bash
nself destroy --dry-run
```

### 4. Color-Coded Warnings

- **Red (✗)**: Items to be destroyed
- **Yellow (⚠)**: Data loss warnings
- **Green (✓)**: Preserved items

### 5. Selective Destruction

Use `--*-only` flags to limit scope:
- Reduces blast radius
- Allows targeted cleanup
- Safer for production

---

## Common Workflows

### Clean Rebuild (Preserve Data)

```bash
# 1. Destroy infrastructure but keep data
nself destroy --keep-volumes

# 2. Rebuild configuration
nself build

# 3. Start services (data intact)
nself start
```

### Force Container Restart

```bash
# 1. Only remove containers
nself destroy --containers-only

# 2. Restart services
nself start
```

### Complete Fresh Start

```bash
# 1. Full destruction (interactive)
nself destroy

# 2. Re-initialize
nself init

# 3. Build and start
nself build && nself start
```

### Automation Script

```bash
#!/bin/bash
# Automated environment teardown (with safety backup)

# 1. Backup first (safety)
nself backup create

# 2. Destroy (non-interactive)
nself destroy --force

# 3. Clean Docker system (optional)
docker system prune -af --volumes
```

---

## Troubleshooting

### Resources Not Fully Removed

**Symptom:** Some containers/volumes remain after destruction

**Solutions:**
```bash
# 1. Force removal manually
docker ps -a | grep myapp | awk '{print $1}' | xargs docker rm -f
docker volume ls | grep myapp | awk '{print $2}' | xargs docker volume rm

# 2. Use Docker system prune
docker system prune -af --volumes

# 3. Check with verbose mode
nself destroy --verbose
```

### Permission Denied

**Symptom:** Cannot remove certain files or volumes

**Solutions:**
```bash
# 1. Check file ownership
ls -la

# 2. Fix permissions
sudo chown -R $USER:$USER .

# 3. Use sudo for Docker operations (if needed)
sudo nself destroy
```

### Volumes In Use

**Symptom:** "volume is in use" errors

**Solutions:**
```bash
# 1. Stop all containers first
nself stop

# 2. Remove containers before volumes
nself destroy --containers-only
nself destroy --volumes-only

# 3. Force Docker cleanup
docker container prune -f
docker volume prune -f
```

---

## Best Practices

### 1. Always Backup First

```bash
# Create backup before destruction
nself backup create
nself destroy
```

### 2. Use Dry Run

```bash
# Preview before executing
nself destroy --dry-run

# If satisfied, execute
nself destroy
```

### 3. Preserve Data in Development

```bash
# Keep databases during rebuilds
nself destroy --keep-volumes
```

### 4. Never Force in Production

```bash
# ❌ WRONG (production)
nself destroy --force

# ✓ RIGHT (production)
nself destroy  # Interactive with confirmations
```

### 5. Clean Rebuild Process

```bash
# Step-by-step safe rebuild
nself backup create          # Safety backup
nself destroy --keep-volumes # Remove infra, keep data
nself build                  # Regenerate configs
nself start                  # Restart with data intact
```

---

## Integration with Other Commands

### With Backup

```bash
# Backup before destruction
nself backup create
nself destroy

# If needed, rollback
nself backup restore <backup-id>
```

### With Build

```bash
# Force config regeneration
nself destroy --generated-only
nself build
```

### With Stop

```bash
# Graceful vs destructive
nself stop               # Stops services (safe)
nself destroy            # Removes everything (destructive)
nself destroy --keep-volumes  # Middle ground
```

### With Reset

```bash
# Different reset strategies
nself backup reset       # Database only
nself destroy --volumes-only  # All volumes
nself destroy            # Complete teardown
```

---

## Comparison with Other Commands

| Command | Containers | Volumes | Networks | Files | Data Loss |
|---------|-----------|---------|----------|-------|-----------|
| `nself stop` | Stops | Keeps | Keeps | Keeps | None |
| `nself stop --volumes` | Stops | Removes | Keeps | Keeps | Yes |
| `nself destroy --keep-volumes` | Removes | Keeps | Removes | Removes | None |
| `nself destroy` | Removes | Removes | Removes | Removes | Yes |

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (all resources destroyed) |
| 1 | Partial failure (some resources remain) |
| 2 | User cancelled operation |
| 3 | docker-compose.yml not found |

---

## Related Commands

- `nself stop` - Stop services without removal
- `nself backup create` - Create backup before destruction
- `nself backup restore` - Restore from backup after destruction
- `nself build` - Rebuild configuration
- `nself init` - Initialize new project

---

## Security Considerations

### Data Loss Prevention

1. **Always backup production data:**
   ```bash
   nself backup create --env prod
   ```

2. **Use --keep-volumes for development:**
   ```bash
   nself destroy --keep-volumes
   ```

3. **Never use --force without review:**
   ```bash
   # Review first
   nself destroy --dry-run
   # Then execute
   nself destroy
   ```

### Production Checklist

Before running `destroy` in production:

- [ ] Backup created and verified
- [ ] Team notified of downtime
- [ ] Maintenance mode enabled
- [ ] Confirmed correct environment (not production by mistake)
- [ ] Reviewed dry-run output
- [ ] Rollback plan prepared

---

## Advanced Usage

### Scripting with Error Handling

```bash
#!/bin/bash
set -e

# Backup with error handling
if ! nself backup create; then
  echo "Backup failed - aborting destruction"
  exit 1
fi

# Destroy with confirmation
if nself destroy --dry-run; then
  read -p "Proceed with destruction? (yes/no): " confirm
  if [[ "$confirm" == "yes" ]]; then
    nself destroy --force
  fi
fi
```

### CI/CD Integration

```yaml
# .github/workflows/teardown.yml
name: Teardown Environment

on:
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environment to destroy'
        required: true
        type: choice
        options:
          - staging
          - dev

jobs:
  teardown:
    runs-on: ubuntu-latest
    steps:
      - name: Backup
        run: nself backup create --env ${{ inputs.environment }}

      - name: Destroy
        run: nself destroy --force --keep-volumes
```

---

## FAQ

**Q: Can I undo a destroy operation?**
A: Only if you created a backup first. Use `nself backup restore <backup-id>` to recover.

**Q: What's the difference between stop and destroy?**
A: `stop` pauses services (reversible), `destroy` removes them (requires rebuild).

**Q: Is it safe to use destroy in development?**
A: Yes, with `--keep-volumes` flag to preserve databases.

**Q: Can I destroy only one service?**
A: No, destroy operates on the entire project. Use `docker rm <container>` for single services.

**Q: What happens to .env files?**
A: They are always preserved (never deleted by destroy).

**Q: Can I recover after accidental destruction?**
A: Only data in Docker volumes is lost. Rebuild with `nself build && nself start`. If you had backups, use `nself backup restore`.

---

**Version:** nself v1.0.0+
**Command Type:** Destructive
**Safety Level:** Interactive (with --force bypass)
**Related Docs:** [Backup Guide](../guides/BACKUP-RECOVERY.md), [Stop Command](STOP.md)
