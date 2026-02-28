# Getting Started Documentation

Everything you need to get started with ɳSelf.

## Overview

This section provides introductory documentation for new users, including installation, quick starts, and frequently asked questions.

## Quick Start

- **[Quick Start Guide](../getting-started/Quick-Start.md)** - Get running in 5 minutes
- **[Installation Guide](../getting-started/Installation.md)** - Detailed installation instructions
- **[FAQ](../getting-started/FAQ.md)** - Frequently asked questions
- **[Getting Started Overview](README.md)** - Complete getting started guide

## Installation Methods

### macOS
```bash
# Homebrew
brew install nself-org/nself/nself

# Or curl install
curl -sSL https://install.nself.org | bash
```

### Linux
```bash
# Install script
curl -sSL https://install.nself.org | bash

# Or manual download
wget https://github.com/nself-org/cli/releases/latest/download/nself-linux-amd64
```

### Windows (WSL)
```bash
# In WSL terminal
curl -sSL https://install.nself.org | bash
```

See **[Installation Guide](../getting-started/Installation.md)** for complete details.

## Your First Project

### 3-Minute Setup

```bash
# 1. Create project
mkdir myapp && cd myapp
nself init

# 2. Build and start
nself build && nself start

# 3. Access your API
# Visit: api.local.nself.org
```

### With Database Schema

```bash
# 1. Initialize project
nself init

# 2. Create schema
nself db schema scaffold saas
# Edit schema.dbml at dbdiagram.io

# 3. Apply schema (imports, migrates, seeds)
nself db schema apply schema.dbml

# 4. Build and start
nself build && nself start
```

See **[Quick Start Guide](../getting-started/Quick-Start.md)** for complete walkthrough.

## Common Questions

### "What is ɳSelf?"
ɳSelf is a complete self-hosted Backend-as-a-Service platform that provides all the features of commercial services like Supabase, Nhost, or Firebase.

### "Do I need Docker?"
Yes, ɳSelf uses Docker Compose to run all services. Install Docker Desktop from docker.com.

### "What services does it include?"
- **Always Running (4)**: PostgreSQL, Hasura GraphQL, Auth, Nginx
- **Optional (7)**: Redis, MinIO, Search, Mail, Functions, MLflow, Admin UI
- **Monitoring (10)**: Complete observability stack
- **Custom**: Unlimited services from 40+ templates

### "How much does it cost?"
ɳSelf is free and open-source (MIT license). You only pay for infrastructure (server, cloud, etc.).

### "Is it production-ready?"
Yes! ɳSelf v0.9.6 is stable and production-ready with comprehensive security audits completed.

See **[FAQ](../getting-started/FAQ.md)** for more questions and answers.

## Next Steps

### After Installation

1. **Learn Core Concepts**
   - [Architecture Overview](../architecture/ARCHITECTURE.md)
   - [Services Overview](../services/SERVICES.md)
   - [Project Structure](../architecture/PROJECT_STRUCTURE.md)

2. **Database Workflow**
   - [Database Workflow Guide](../guides/DATABASE-WORKFLOW.md)
   - [Database Commands](../commands/DB.md)

3. **Configuration**
   - [Configuration Guide](../configuration/README.md)
   - [Environment Variables](../configuration/ENVIRONMENT-VARIABLES.md)
   - [Custom Services](../configuration/CUSTOM-SERVICES-ENV-VARS.md)

4. **Deployment**
   - [Deployment Guide](../deployment/README.md)
   - [Production Deployment](../deployment/PRODUCTION-DEPLOYMENT.md)
   - [Cloud Providers](../deployment/CLOUD-PROVIDERS.md)

## Tutorials

### Quick Start Tutorials
- [SaaS in 15 Minutes](../tutorials/QUICK-START-SAAS.md)
- [B2B Platform](../tutorials/QUICK-START-B2B.md)
- [Marketplace](../tutorials/QUICK-START-MARKETPLACE.md)
- [Agency Platform](../tutorials/QUICK-START-AGENCY.md)

### Integration Tutorials
- [Stripe Integration](../tutorials/STRIPE-INTEGRATION.md)
- [Custom Domains](../tutorials/CUSTOM-DOMAINS.md)
- [File Uploads](../tutorials/file-uploads-quickstart.md)

## Learning Path

### Beginner (Day 1)
1. Install ɳSelf
2. Run demo: `nself init --demo && nself build && nself start`
3. Explore services: `nself urls`
4. View Admin UI at `admin.local.nself.org`

### Intermediate (Week 1)
1. Design database schema with DBML
2. Apply schema: `nself db schema apply`
3. Generate TypeScript types: `nself db types`
4. Build first custom service from template

### Advanced (Month 1)
1. Set up multi-tenancy
2. Configure OAuth providers
3. Deploy to production
4. Set up monitoring and alerts
5. Implement custom plugins

## Support

### Documentation
- [Complete Documentation](../README.md)
- [Command Reference](../commands/COMMANDS.md)
- [Troubleshooting](../guides/TROUBLESHOOTING.md)

### Community
- [GitHub Discussions](https://github.com/nself-org/cli/discussions)
- [GitHub Issues](https://github.com/nself-org/cli/issues)
- [Examples Repository](https://github.com/nself-org/cli-examples)

### Help Commands
```bash
# General help
nself help

# Command-specific help
nself help db
nself help deploy

# Run diagnostics
nself doctor

# Check status
nself status
```

---

**[← Back to Documentation Home](../README.md)**
