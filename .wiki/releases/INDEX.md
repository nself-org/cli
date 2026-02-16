# nself Releases & Changelog

Complete release history and roadmap for nself - Self-Hosted Infrastructure Manager.

**Current Stable Version:** v0.9.9

---

## Latest Release

### [v0.9.9](v0.9.9.md) - Current Stable

**Released:** February 2026

**Status:** QA & Stabilization ✅

**Highlights:**
- **80% Test Coverage**: 700+ tests across unit, integration, and E2E
- **100% Documentation**: Every command and feature fully documented
- **UX Polish**: Standardized errors, progress indicators, input validation
- **Performance Optimized**: Build caching, batched operations, parallel execution
- **Production Features**: Health endpoints, graceful shutdown, automated backups
- **Quality Metrics**: Benchmarks, security audits, cross-platform verification

**[View Release Notes →](v0.9.9.md)**

---

## Recent Releases

### [v0.9.7](v0.9.7.md) - Security & CI/CD Complete

**Released:** January 31, 2026

**Highlights:**
- All CI/CD Passing: 7/7 GitHub Actions workflows green
- Tenant Isolation Complete: 100% test coverage with RLS policies
- Enterprise Security: Comprehensive scanning, secrets management, rate limiting
- Compliance Ready: GDPR (85%), HIPAA (75%), SOC 2 (70%)

**[View Release Notes →](v0.9.7.md)**

### [v0.9.6](v0.9.6.md) - Command Consolidation

**Released:** January 30, 2026

**Highlights:**
- Command Consolidation: 79 → 31 top-level commands
- 285+ Subcommands organized by domain
- 100% Backward Compatible with migration warnings

**[View Release Notes →](v0.9.6.md)**

### [v0.9.5](v0.9.5.md) - Feature Parity & Security

**Status:** Production Ready

**Highlights:**
- **Real-Time Communication**: Complete WebSocket system with channels, presence, and database subscriptions
- **Security Hardening**: SQL injection fixes, CSP framework, comprehensive security audit
- **Enhanced OAuth**: PKCE support, state validation, improved token management
- **Feature Parity**: Matches Supabase/Nhost real-time capabilities
- **Migration Tools**: One-command migration from Supabase and Nhost
- **Complete Documentation**: 20+ new guides and comprehensive CLI reference

**[View Release Notes →](v0.9.5.md)**

### [v0.9.0](v0.9.0.md) - January 30, 2026

**Status:** Production Ready

**Highlights:**
- **Multi-Tenant Platform**: Complete tenant management with isolation
- **Billing Integration**: Stripe-based usage tracking, invoicing, and subscriptions
- **White-Labeling**: Custom domains, branding, email templates, and themes
- **OAuth Management**: Google, GitHub, Microsoft, Slack provider integration
- **File Storage**: Advanced upload pipeline with thumbnails, virus scanning, and compression
- **Member Management**: Role-based access control per tenant
- **150+ Commands**: Comprehensive CLI for all platform features

**[View Release Notes →](v0.9.0.md)**

---

## Recent Releases

### [v0.4.8](v0.4.8.md) - January 24, 2026

**Status:** Stable

**Highlights:**
- Plugin System with extensible architecture
- Stripe, GitHub, and Shopify integrations
- Plugin Registry with distributed architecture
- Database schemas and webhook handlers
- CLI actions for plugin management

**[View Release Notes →](v0.4.8.md)**

### [v0.4.7](v0.4.7.md) - January 23, 2026

**Status:** Stable

**Highlights:**
- 26 Cloud Providers with massively expanded infrastructure support
- Kubernetes Support with full K8s deployment and management
- Helm Charts generation, deployment, and management
- Enhanced Deployment with preview, canary, and blue-green strategies

**[View Release Notes →](v0.4.7.md)**

### [v0.4.6](v0.4.6.md) - January 22, 2026

**Status:** Stable

**Highlights:**
- Performance profiling and analysis (`nself perf`)
- Benchmarking and load testing (`nself bench`)
- Health check management, operation audit trail
- Configuration management, server infrastructure
- Frontend application management

**[View Release Notes →](v0.4.6.md)**

### [v0.4.5](v0.4.5.md) - January 21, 2026

**Status:** Stable

**Highlights:**
- 10 cloud provider support with normalized sizing
- One-command provisioning to any provider
- Environment synchronization (db, files, config)
- CI/CD integration (GitHub Actions, GitLab CI)
- Shell completions (bash, zsh, fish)

**[View Release Notes →](v0.4.5.md)**

### [v0.4.4](v0.4.4.md) - January 20, 2026

**Status:** Stable

**Highlights:**
- DBML Schema Workflow (design → import → migrate → seed)
- Schema templates: basic, ecommerce, saas, blog
- Type generation (TypeScript, Go, Python)
- Database inspection and mock data generation

**[View Release Notes →](v0.4.4.md)**

### [v0.4.3](v0.4.3.md) - January 19, 2026

**Status:** Stable

**Highlights:**
- Comprehensive deployment pipeline (local → staging → production)
- Environment management commands (env, deploy, prod, staging)
- SSH deployment with zero-downtime support
- 16 Dockerfile templates for service generation

**[View Release Notes →](v0.4.3.md)**

### [v0.4.2](v0.4.2.md) - January 18, 2026

**Status:** Stable

**Highlights:**
- 6 new service management commands (email, search, functions, mlflow, metrics, monitor)
- 16+ email provider support with SMTP pre-flight checks
- 6 search engines supported (PostgreSQL, MeiliSearch, Typesense, Elasticsearch, OpenSearch, Sonic)
- 92 unit tests, complete documentation

**[View Release Notes →](v0.4.2.md)**

### [v0.4.1](v0.4.1.md) - January 17, 2026

**Status:** Stable

**Highlights:**
- Fixed Bash 3.2 compatibility for macOS
- Fixed cross-platform sed, stat, and timeout commands
- Fixed portable output formatting (POSIX compliance)

**[View Release Notes →](v0.4.1.md)**

### [v0.4.0](v0.4.0.md) - October 2025

**Status:** Stable

**Highlights:**
- Production-ready release
- All core features complete and tested
- Enhanced cross-platform compatibility (Bash 3.2+)

**[View Release Notes →](v0.4.0.md)**

### [v0.3.9](v0.3.9.md) - September 2025

**Status:** Stable

**Highlights:**
- Admin UI with comprehensive management features
- 25 services in demo configuration

**[View Release Notes →](v0.3.9.md)**

---

## Roadmap

See our [Roadmap](ROADMAP.md) for planned features and improvements.

### Upcoming Releases

| Version | Target | Focus |
|---------|--------|-------|
| **v0.9.9** | February 2026 | QA & Final Testing |
| **v1.0.0 LTS** | Q1 2026 | **Production Ready LTS Release** |
| **v1.1.0** | Q2 2026 | Plugin Marketplace & Additional Features |

---

## Complete Changelog

See [CHANGELOG.md](CHANGELOG.md) for complete version history with all changes.

### All Releases

| Version | Date | Status | Highlights |
|---------|------|--------|------------|
| [v0.9.8](v0.9.8.md) | Feb 2026 | **Current** | Production Ready (80% coverage, 100% docs) |
| [v0.9.7](v0.9.7.md) | Jan 31, 2026 | Stable | Security & CI/CD Complete |
| [v0.9.6](v0.9.6.md) | Jan 30, 2026 | Stable | Command Consolidation (79→31 commands) |
| [v0.9.5](v0.9.5.md) | Jan 30, 2026 | Stable | Real-Time, Security Hardening, Enhanced OAuth |
| [v0.9.0](v0.9.0.md) | Jan 30, 2026 | Stable | Multi-Tenant Platform, OAuth, Storage |
| [v0.4.8](v0.4.8.md) | Jan 24, 2026 | Stable | Plugin System (Stripe, GitHub, Shopify) |
| [v0.4.7](v0.4.7.md) | Jan 23, 2026 | Stable | Infrastructure Everywhere, K8s, Helm |
| [v0.4.6](v0.4.6.md) | Jan 22, 2026 | Stable | Scaling & Performance |
| [v0.4.5](v0.4.5.md) | Jan 21, 2026 | Stable | Provider Support, 10 cloud providers |
| [v0.4.4](v0.4.4.md) | Jan 20, 2026 | Stable | Database Tools, DBML workflow |
| [v0.4.3](v0.4.3.md) | Jan 19, 2026 | Stable | Deployment Pipeline |
| [v0.4.2](v0.4.2.md) | Jan 18, 2026 | Stable | Service & monitoring commands |
| [v0.4.1](v0.4.1.md) | Jan 17, 2026 | Stable | Platform compatibility fixes |
| [v0.4.0](v0.4.0.md) | Oct 2025 | Stable | Production-ready release |
| [v0.3.10](v0.3.10.md) | Sep 2025 | Stable | Critical bug fixes, WSL support |
| [v0.3.9](v0.3.9.md) | Sep 2025 | Stable | Admin UI, enhanced init |
| [v0.3.8](v0.3.8.md) | Aug 2024 | Stable | Admin UI, search, VPS deployment |
| [v0.3.7](v0.3.7.md) | Aug 2025 | Stable | CI/CD, backup system |
| [v0.3.6](v0.3.6.md) | Aug 2025 | Stable | Major refactoring |
| [v0.3.5](v0.3.5.md) | Aug 2025 | Stable | Complete SSL/HTTPS support |
| [v0.3.4](v0.3.4.md) | Aug 2025 | Stable | Command headers, service detection |
| [v0.3.3](v0.3.3.md) | Aug 2025 | Stable | Auto-fix default, Docker cleanup |
| [v0.3.2](v0.3.2.md) | Aug 2025 | Stable | Command resolution fix |
| [v0.3.1](v0.3.1.md) | Jan 2025 | Stable | Config validation, auto-fix |
| [v0.3.0](v0.3.0.md) | Jan 2025 | Stable | **Breaking:** Architecture refactor |
| [v0.2.4](v0.2.4.md) | Jan 2025 | Legacy | Email provider support |
| [v0.2.3](v0.2.3.md) | Jan 2025 | Legacy | SSL trust, installation fixes |
| [v0.2.2](v0.2.2.md) | Jan 2025 | Legacy | UI improvements |
| [v0.2.1](v0.2.1.md) | Jan 2025 | Legacy | Database tools |
| [v0.2.0](v0.2.0.md) | Jan 2025 | Legacy | Modular architecture |
| [v0.1.0](v0.1.0.md) | Jan 2025 | Legacy | Initial release |

---

## Version Guidelines

### Semantic Versioning

nself follows [Semantic Versioning](https://semver.org/):

**Format:** `MAJOR.MINOR.PATCH`

- **MAJOR**: Breaking changes, major feature additions
- **MINOR**: New features, backwards-compatible
- **PATCH**: Bug fixes, minor improvements

### Release Channels

- **Stable** (recommended): Production-ready releases (e.g., v0.4.1)
- **Beta**: Feature-complete pre-releases (e.g., v0.5.0-beta.1)
- **RC**: Release candidates (e.g., v0.5.0-rc.1)

### Updating nself

```bash
# Check current version
nself version

# Update to latest stable
nself update

# Update to specific version
NSELF_VERSION=v0.4.1 bash <(curl -sSL https://install.nself.org)
```

---

## Installation Methods

| Method | Command | Platforms |
|--------|---------|-----------|
| **curl (Primary)** | `curl -sSL https://install.nself.org \| bash` | macOS, Linux, WSL |
| **Homebrew** | `brew install acamarata/nself/nself` | macOS, Linux |
| **npm** | `npm install -g nself-cli` | All |
| **apt-get** | See Debian package | Ubuntu, Debian |
| **dnf/yum** | See RPM package | Fedora, RHEL |
| **AUR** | `yay -S nself` | Arch Linux |
| **Docker** | `docker pull acamarata/nself:latest` | All |

---

## Platform Support

| Platform | Status | Notes |
|----------|--------|-------|
| macOS (Bash 3.2) | ✅ Full | Default shell supported |
| Ubuntu/Debian | ✅ Full | All versions |
| Fedora/RHEL | ✅ Full | All versions |
| Arch Linux | ✅ Full | AUR package |
| Alpine Linux | ✅ Full | Docker-based |
| WSL/WSL2 | ✅ Full | Windows integration |

---

## Stay Updated

### Release Notifications

- **Watch Repository**: Get notified of new releases on GitHub
- **Release Notes**: Subscribe to [Discussions](https://github.com/acamarata/nself/discussions)
- **Changelog**: Check [CHANGELOG.md](../CHANGELOG.md) for detailed changes

---

## Documentation

- **[Home](../Home.md)** - Documentation homepage
- **[Commands Reference](../commands/COMMANDS.md)** - Complete CLI reference
- **[Contributing](../contributing/CONTRIBUTING.md)** - Contribution guidelines
- **[Architecture](../architecture/ARCHITECTURE.md)** - System design

---

## Support

- **Issues**: [Report bugs](https://github.com/acamarata/nself/issues)
- **Discussions**: [Ask questions](https://github.com/acamarata/nself/discussions)

---

**Last Updated:** February 16, 2026 | **Current Version:** v0.9.9
