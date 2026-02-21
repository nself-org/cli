# nself version

**Category**: Utilities

Display version information for nself and all service components.

## Overview

Shows version numbers for the nself CLI, Docker images, and all running services.

**Features**:
- ✅ nself CLI version
- ✅ Service container versions
- ✅ Dependency versions
- ✅ Update availability check
- ✅ JSON output support

## Usage

```bash
nself version [OPTIONS]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-s, --short` | Show only nself CLI version | false |
| `--services` | Show service versions | false |
| `--check-updates` | Check for available updates | false |
| `--format FORMAT` | Output format (table/json) | table |

## Examples

### Basic Version

```bash
nself version
```

**Output**:
```
nself CLI v0.9.9

Installation:
  Path: /usr/local/bin/nself
  Method: Homebrew
  Platform: darwin-arm64
```

### With Service Versions

```bash
nself version --services
```

**Output**:
```
nself CLI v0.9.9

Core Services
──────────────────────────────────────────
PostgreSQL      15.3
Hasura          v2.35.0
nHost Auth      v0.22.0
Nginx           1.25.2

Optional Services
──────────────────────────────────────────
Redis           7.2.1
MinIO           RELEASE.2024-01-05T22-17-24Z
nself Admin     v0.5.2

Monitoring Services
──────────────────────────────────────────
Prometheus      v2.47.0
Grafana         10.1.5
Loki            2.9.2
```

### Check for Updates

```bash
nself version --check-updates
```

**Output**:
```
nself CLI v0.9.9

✓ You are running the latest version

Latest release: v0.9.9
Release date: 2026-02-10
Release notes: https://github.com/nself-org/cli/releases/tag/v0.9.9
```

Or if update available:
```
nself CLI v0.9.8

⚠ Update available: v0.9.9

Current: v0.9.8
Latest: v0.9.9
Released: 2 days ago

Changelog:
- Feature: Monorepo support for nself start
- Fix: Database migration idempotency
- Security: Updated SSL certificate generation

Update with:
  brew upgrade nself        # macOS/Linux (Homebrew)
  npm update -g @nself/cli  # npm
  curl -sSL https://install.nself.org | bash  # Install script
```

### Short Version

```bash
nself version --short
```

**Output**:
```
v0.9.9
```

**Use in scripts**:
```bash
VERSION=$(nself version --short)
echo "Running nself $VERSION"
```

### JSON Output

```bash
nself version --format json
```

**Output**:
```json
{
  "cli": {
    "version": "0.9.9",
    "commit": "abc1234",
    "built": "2026-02-10T12:00:00Z",
    "platform": "darwin-arm64",
    "install_method": "homebrew"
  },
  "services": {
    "postgres": "15.3",
    "hasura": "v2.35.0",
    "auth": "v0.22.0",
    "nginx": "1.25.2",
    "redis": "7.2.1",
    "minio": "RELEASE.2024-01-05T22-17-24Z"
  },
  "dependencies": {
    "docker": "24.0.5",
    "docker_compose": "v2.20.2",
    "node": "v20.10.0"
  }
}
```

## Version Information

### nself CLI

- **Version number**: Semantic versioning (MAJOR.MINOR.PATCH)
- **Build commit**: Git commit hash
- **Build date**: When CLI was compiled
- **Platform**: OS and architecture

### Service Versions

Versions pulled from Docker container labels and runtime info.

**PostgreSQL**:
```bash
nself exec postgres psql -V
# PostgreSQL 15.3 (Debian 15.3-1.pgdg120+1)
```

**Hasura**:
```bash
nself exec hasura hasura version
# Hasura GraphQL Engine: v2.35.0
```

**Redis**:
```bash
nself exec redis redis-server --version
# Redis server v=7.2.1
```

## Update Checking

### Update Sources

1. **GitHub Releases** (primary)
   - https://github.com/nself-org/cli/releases

2. **Homebrew** (macOS/Linux)
   - brew info nself

3. **npm Registry** (Node.js)
   - npm view @nself/cli version

### Update Frequency

```bash
# In .env
VERSION_CHECK_INTERVAL=weekly  # daily, weekly, monthly, never
```

### Disable Update Checks

```bash
# In .env
DISABLE_UPDATE_CHECK=true

# Or environment variable
export NSELF_DISABLE_UPDATE_CHECK=true
```

## Version Compatibility

### Service Compatibility Matrix

| nself | PostgreSQL | Hasura | Auth |
|-------|-----------|--------|------|
| 0.9.x | 15.x | 2.35+ | 0.22+ |
| 0.8.x | 14.x-15.x | 2.30+ | 0.20+ |
| 0.7.x | 14.x | 2.25+ | 0.18+ |

### Breaking Changes

Check changelog for breaking changes:
```bash
nself version --check-updates
# Click release notes link for details
```

## Installation Methods

### Homebrew (macOS/Linux)

```bash
# Install
brew install nself

# Update
brew upgrade nself

# Check version
nself version
```

### npm (Cross-platform)

```bash
# Install
npm install -g @nself/cli

# Update
npm update -g @nself/cli

# Check version
nself version
```

### Install Script (Linux/macOS)

```bash
# Install/Update
curl -sSL https://install.nself.org | bash

# Check version
nself version
```

### From Source

```bash
# Clone
git clone https://github.com/nself-org/cli
cd nself

# Build
make build

# Install
sudo make install

# Check version
nself version
```

## Version in Scripts

### Require Minimum Version

```bash
#!/bin/bash
REQUIRED="0.9.0"
CURRENT=$(nself version --short | tr -d 'v')

if [ "$(printf '%s\n' "$REQUIRED" "$CURRENT" | sort -V | head -n1)" != "$REQUIRED" ]; then
  echo "Error: nself >= $REQUIRED required (have $CURRENT)"
  exit 1
fi
```

### Version-Specific Features

```bash
#!/bin/bash
VERSION=$(nself version --short | tr -d 'v')

if [[ "$VERSION" > "0.9.0" ]]; then
  # Use monorepo feature (>= 0.9.0)
  nself start
else
  # Fallback for older versions
  cd backend && nself start
fi
```

## Related Commands

- `nself update` - Update nself CLI
- `nself help` - Show help information
- `nself doctor` - Check installation health

## See Also

- [Installation Guide](../../installation/README.md)
- [Upgrade Guide](../../guides/UPGRADE.md)
- [Changelog](../../releases/CHANGELOG.md)
- [GitHub Releases](https://github.com/nself-org/cli/releases)
