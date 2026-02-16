# nself version - Version Information

**Version 0.9.9** | Display version information

---

## Overview

The `nself version` command displays version information for nself and related components.

---

## Basic Usage

```bash
# Show nself version
nself version

# Short version only
nself version --short
nself -v

# Verbose with all components
nself version --verbose
```

---

## Output Example

```
nself v0.4.8

Components:
  Docker:         24.0.7
  Docker Compose: v2.23.0
  Bash:           5.2.15
  mkcert:         v1.4.4

Installation:
  Path:    /usr/local/bin/nself
  Source:  Homebrew

Latest: v0.4.8 (up to date)
```

---

## Options Reference

| Option | Short | Description |
|--------|-------|-------------|
| `--short` | `-s` | Version number only |
| `--verbose` | `-v` | Include all components |
| `--check` | | Check for updates |
| `--json` | | JSON output |

---

## Check for Updates

```bash
# Check if update available
nself version --check

# Update if available
nself update
```

---

## See Also

- [update](UPDATE.md) - Update nself
- [doctor](DOCTOR.md) - System diagnostics
