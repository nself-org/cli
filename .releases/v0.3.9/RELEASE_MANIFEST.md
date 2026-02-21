# nself v0.3.9 Release Manifest

## Release Information
- **Version**: 0.3.9
- **Release Date**: September 2, 2025
- **Git Tag**: v0.3.9
- **Commit**: 42355cf

## Package Files

### Source Archives
- `nself-v0.3.9.tar.gz` - Full source archive
  - SHA256: `9c20d0613c6dbc08a54a252ca4a92148135099b91409734a59414dd9725d2222`
  - Size: ~5MB

### Installation Scripts
- `install.sh` - Universal installer for Linux/macOS
- `install-debian.sh` - Debian/Ubuntu specific installer
- `install-rhel.sh` - RHEL/CentOS/Fedora specific installer

### Package Managers
- `nself.rb` - Homebrew formula for macOS
- `Dockerfile` - Docker image definition
- `docker-compose.release.yml` - Docker Compose deployment

## Installation Methods

### 1. Quick Install (Linux/macOS)
```bash
curl -sSL https://install.nself.org | bash
# or
wget -qO- https://install.nself.org | bash
```

### 2. Homebrew (macOS)
```bash
brew tap nself-org/nself
brew install nself
```

### 3. Docker
```bash
docker pull ghcr.io/nself-org/cli:0.3.9
docker run --rm -it ghcr.io/nself-org/cli:0.3.9 help
```

### 4. Manual Installation
```bash
wget https://github.com/nself-org/cli/archive/v0.3.9.tar.gz
tar -xzf v0.3.9.tar.gz
cd nself-0.3.9
./install.sh
```

### 5. Debian/Ubuntu
```bash
wget https://raw.githubusercontent.com/nself-org/cli/v0.3.9/releases/v0.3.9/install-debian.sh
chmod +x install-debian.sh
./install-debian.sh
```

### 6. RHEL/CentOS/Fedora
```bash
wget https://raw.githubusercontent.com/nself-org/cli/v0.3.9/releases/v0.3.9/install-rhel.sh
chmod +x install-rhel.sh
./install-rhel.sh
```

## Verification

### Verify Installation
```bash
nself version
# Should output: nself 0.3.9
```

### Verify SHA256
```bash
shasum -a 256 nself-v0.3.9.tar.gz
# Should match: 9c20d0613c6dbc08a54a252ca4a92148135099b91409734a59414dd9725d2222
```

## System Requirements

### Minimum Requirements
- OS: Linux (Ubuntu 20.04+, CentOS 7+) or macOS 11+
- Docker: 20.10+
- Docker Compose: 2.0+ or Docker Compose Plugin
- RAM: 4GB minimum, 8GB recommended
- Disk: 10GB free space

### Recommended Setup
- OS: Ubuntu 22.04 LTS or macOS 13+
- Docker Desktop or Docker Engine latest
- RAM: 16GB
- Disk: 50GB free space
- CPU: 4+ cores

## What's Included

### Core Components
- PostgreSQL 16 Alpine
- Hasura GraphQL Engine v2.44.0
- Nhost Auth v0.36.0
- Hasura Storage v0.6.1
- MinIO (S3-compatible storage)
- Nginx Alpine
- MailPit (development email)
- Redis 7 Alpine (optional)
- Admin UI (nself-admin v0.0.3)

### CLI Commands (35+)
- Core: init, build, start, stop, restart, status, logs
- Management: doctor, db, admin, email, validate
- Development: reset, diff, exec
- Production: prod, ssl, trust
- Utilities: version, help, update

## Support

### Documentation
- GitHub: https://github.com/nself-org/cli/docs/
- Release Notes: https://github.com/nself-org/cli/releases/tag/v0.3.9

### Issues
- GitHub Issues: https://github.com/nself-org/cli/issues

### Community
- Discord: Coming soon
- Twitter: @nself_org

## License
MIT License - See LICENSE file for details