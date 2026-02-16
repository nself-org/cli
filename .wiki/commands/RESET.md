# nself reset - Reset Project

**Version 0.9.9** | Reset project to initial state

---

## Overview

The `nself reset` command resets your nself project to a clean state. It removes generated files, containers, and optionally configuration, allowing you to start fresh.

---

## Basic Usage

```bash
# Reset generated files only
nself reset

# Reset including containers
nself reset --containers

# Complete reset (everything)
nself reset --all

# Dry run
nself reset --dry-run
```

---

## Reset Levels

### Level 1: Generated Files (Default)

```bash
nself reset
```

Removes:
- `docker-compose.yml`
- `nginx/` directory
- `ssl/` directory
- `services/` (generated from templates)

Keeps:
- `.env` file
- Docker containers/volumes
- Custom service code (if modified)

### Level 2: With Containers

```bash
nself reset --containers
```

Also removes:
- All project containers
- Project networks

### Level 3: Complete Reset

```bash
nself reset --all
```

Also removes:
- `.env` file
- All volumes (database data)
- All images

---

## Options Reference

| Option | Description |
|--------|-------------|
| `--containers` | Also remove containers |
| `--volumes` | Also remove volumes |
| `--all` | Remove everything |
| `--keep-env` | Keep .env file |
| `--dry-run` | Show what would be removed |
| `--force` | Skip confirmation |

---

## Common Use Cases

### Rebuild Configuration

```bash
# Reset and rebuild
nself reset
nself build
nself start
```

### Fresh Start

```bash
# Complete reset
nself reset --all
nself init
```

### Fix Corrupted State

```bash
nself reset --containers
nself build
nself start
```

---

## See Also

- [clean](CLEAN.md) - Clean Docker resources
- [init](INIT.md) - Initialize project
- [build](BUILD.md) - Build configuration
