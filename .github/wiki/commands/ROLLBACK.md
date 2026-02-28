# nself backup rollback - Deployment Rollback

> **DEPRECATED COMMAND NAME**: This command was formerly `nself rollback` in v0.x. It has been consolidated to `nself backup rollback` in v1.0. The old command name may still work as an alias. For deployment rollbacks, use `nself deploy rollback`.

**Version 0.9.9** | Roll back to previous deployment

---

## Overview

The `nself backup rollback` command reverts to a previous deployment state. It's useful for recovering from failed deployments or reverting problematic changes.

> **Note**: This is a legacy command. Use `nself deploy rollback` for new workflows.

---

## Basic Usage

```bash
# Rollback to previous deployment
nself backup rollback

# Rollback to specific version
nself backup rollback --version v1.2.3

# Rollback specific environment
nself backup rollback staging
```

---

## Rollback Types

### Quick Rollback

```bash
# Roll back one version
nself backup rollback
```

Reverts to the previous deployment using stored state.

### Version Rollback

```bash
# Roll back to specific tag
nself backup rollback --version v1.2.3
```

Deploys a specific tagged version.

### Database Rollback

```bash
# Roll back with database
nself backup rollback --include-db
```

Also restores the database from the matching backup.

---

## Deployment History

View available rollback points:

```bash
nself history
```

```
Deployment History
─────────────────────────────────────────────────────────────────
  #1  v1.2.4  2024-01-20 10:15  current
  #2  v1.2.3  2024-01-19 14:30  stable
  #3  v1.2.2  2024-01-18 09:00
```

---

## Options Reference

| Option | Description |
|--------|-------------|
| `--version` | Target version/tag |
| `--include-db` | Also rollback database |
| `--dry-run` | Preview changes |
| `--force` | Skip confirmations |

---

## See Also

- [deploy](DEPLOY.md) - Deployment
- [history](HISTORY.md) - Deployment history
