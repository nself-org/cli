# nself v0.3.9 - Production Ready Release

## 🎉 Major Release

We're excited to announce nself v0.3.9, a major production-ready release with comprehensive bug fixes, improved stability, and streamlined installation.

## ✨ Key Highlights

- **35+ CLI Commands** - Complete command suite for managing your self-hosted backend
- **Admin UI** - Web-based administration interface at localhost:3100
- **Smart Defaults** - Simplified initialization with intelligent configuration
- **Production Ready** - Comprehensive testing, bug fixes, and stability improvements
- **Multi-Platform** - Support for Linux, macOS, Docker, and Kubernetes (coming soon)

## 🚀 What's New

### Simplified Installation
- Streamlined `nself init` with minimal defaults
- Smart configuration detection and auto-fix
- Improved environment file handling

### Bug Fixes & Stability
- Fixed environment loading priority issues
- Resolved command execution hanging problems
- Improved error handling and recovery
- Fixed Docker Compose integration issues
- Resolved SSL certificate generation problems

### Enhanced Reset Command
- Organized timestamp-based backups
- Clean state restoration
- Proper backup folder structure (_backup/YYYYMMDD_HHMMSS/)

### Developer Experience
- Better error messages and debugging
- Improved command documentation
- Consistent command patterns
- Enhanced validation and diagnostics

## 📦 Installation

### Quick Install
```bash
curl -sSL https://install.nself.org | bash
```

### Homebrew (macOS)
```bash
brew tap nself-org/nself
brew install nself
```

### Docker
```bash
docker pull ghcr.io/nself-org/cli:0.3.9
```

## 🛠️ Components

- PostgreSQL 16 Alpine with 60+ extensions
- Hasura GraphQL Engine v2.44.0
- Nhost Auth v0.36.0
- Hasura Storage v0.6.1
- MinIO S3-compatible storage
- Nginx reverse proxy
- Redis 7 Alpine (optional)
- Admin UI (nself-admin v0.0.3)

## 📋 Requirements

- Docker 20.10+
- Docker Compose 2.0+
- 4GB RAM minimum (8GB recommended)
- 10GB disk space

## 🔧 Getting Started

```bash
# Create new project
mkdir my-backend && cd my-backend

# Initialize
nself init

# Build and start
nself build
nself start

# Enable admin UI
nself admin enable

# Check status
nself status
```

## 📚 Documentation

- [Commands Reference](https://github.com/nself-org/cli/blob/main/docs/COMMANDS.md)
- [Installation Guide](https://github.com/nself-org/cli/blob/main/docs/INSTALLATION.md)
- [Configuration](https://github.com/nself-org/cli/blob/main/docs/CONFIGURATION.md)

## 🐛 Bug Fixes

- Fixed environment loading causing hangs in status/build commands
- Resolved stop command compose wrapper issues
- Fixed exec command container detection
- Improved validation timeout handling
- Fixed SSL certificate generation for nginx
- Resolved admin UI integration issues
- Fixed reset command backup organization
- Removed legacy config-server code

## 💬 Community

- GitHub Issues: [Report bugs or request features](https://github.com/nself-org/cli/issues)
- Telegram: [@nself_updates](https://t.me/nself_updates)
- Discord: Coming soon

## 🙏 Acknowledgments

Thank you to all contributors and early adopters who helped test and improve nself!

## 📄 License

MIT License - Free for personal use

---

**Full Changelog**: https://github.com/nself-org/cli/compare/v0.3.8...v0.3.9