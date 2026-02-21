# nself update - Update nself

**Version 0.9.9** | Update nself CLI to latest version

---

## Overview

The `nself update` command updates the nself CLI and optionally the nself-admin dashboard to the latest version.

---

## Basic Usage

```bash
# Update nself CLI
nself update

# Update with admin dashboard
nself update --admin

# Check for updates only
nself update --check
```

---

## Update Methods

### Homebrew (Recommended)

```bash
brew update && brew upgrade nself
```

### curl Installer

```bash
curl -sSL https://install.nself.org | bash
```

### npm

```bash
npm update -g @nself-org/cli
```

### Manual

```bash
# Download latest
git clone https://github.com/nself-org/cli.git
cd nself
./install.sh
```

---

## Options Reference

| Option | Description |
|--------|-------------|
| `--check` | Check for updates only |
| `--admin` | Also update nself-admin |
| `--force` | Force update even if current |
| `--channel` | Update channel (stable/beta) |

---

## Version Channels

| Channel | Description |
|---------|-------------|
| `stable` | Production-ready releases |
| `beta` | Preview features |
| `dev` | Development builds |

```bash
# Switch to beta channel
nself update --channel beta
```

---

## Rollback

If an update causes issues:

```bash
# Homebrew
brew install nself-org/nself/nself@0.4.7

# Manual
git checkout v0.4.7
./install.sh
```

---

## See Also

- [version](VERSION.md) - Check current version
- [doctor](DOCTOR.md) - System diagnostics
