# nself restore - Database Restore

**Version 0.9.9** | Restore database from backup

---

## Overview

The `nself restore` command restores a PostgreSQL database from a backup file.

> **Note**: This is a legacy alias. Use `nself db restore` for new workflows.

---

## Basic Usage

```bash
# Restore latest backup
nself restore

# Restore specific backup
nself restore backups/myapp_dev_20240120.sql

# Interactive selection
nself restore --select
```

---

## Options

| Option | Description |
|--------|-------------|
| `--select` | Interactive backup selection |
| `--decrypt` | Decrypt encrypted backup |
| `--force` | Skip confirmation |

---

## Safety

Restore operations are protected in production:

```
⚠ This will overwrite the production database!
Are you absolutely sure? [yes/NO]
```

---

## See Also

- [db command](DB.md) - Full database operations
- [backup command](BACKUP.md) - Create backups
