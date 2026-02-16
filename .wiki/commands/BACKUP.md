# nself backup - Database Backup

**Version 0.9.9** | Create database backups

---

## Overview

The `nself backup` command creates PostgreSQL database backups. For full database management, see [db command](DB.md).

> **Note**: This is a legacy alias. Use `nself db backup` for new workflows.

---

## Basic Usage

```bash
# Create backup
nself backup

# Backup with custom name
nself backup --name mybackup

# Encrypted backup
nself backup --encrypt
```

---

## Backup Location

Backups are stored in `backups/`:

```
backups/
├── myapp_dev_20240120_101500.sql
├── myapp_dev_20240119_091000.sql
└── myapp_dev_20240118_080000.sql
```

---

## Options

| Option | Description |
|--------|-------------|
| `--name` | Custom backup name |
| `--encrypt` | Encrypt backup |
| `--compress` | Compress with gzip |
| `--output` | Output directory |

---

## See Also

- [db command](DB.md) - Full database operations
- [restore command](RESTORE.md) - Restore backups
