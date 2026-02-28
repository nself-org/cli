# nself dev

**Category**: Development Commands

Developer tools and utilities for nself development and testing.

## Overview

All development operations use `nself dev <subcommand>` for developer-specific tools, testing workflows, and frontend integration.

**Features**:
- ✅ Development mode management
- ✅ Frontend framework detection
- ✅ CI/CD integration tools
- ✅ Documentation generation
- ✅ Testing utilities
- ✅ Platform detection

## Subcommands

| Subcommand | Description | Use Case |
|------------|-------------|----------|
| [mode](#nself-dev-mode) | Switch development modes | Hot reload vs production mode |
| [frontend](#nself-dev-frontend) | Frontend tools | Framework detection and setup |
| [platforms](#nself-dev-platforms) | Platform detection | Multi-platform support |
| [build](#nself-dev-build) | Multi-platform builds | Cross-platform compatibility |
| [test](#nself-dev-test) | Workflow testing | Test deployment workflows |
| [ci](#nself-dev-ci) | CI/CD integration | GitHub Actions, GitLab CI |
| [docs](#nself-dev-docs) | Documentation generation | Auto-generate docs |
| [whitelabel](#nself-dev-whitelabel) | White-label customization | Branding |
| [scaffold](#nself-dev-scaffold) | Code scaffolding | Generate boilerplate |
| [migrate-data](#nself-dev-migrate-data) | Data migration tools | Transform data |
| [seed-generate](#nself-dev-seed-generate) | Generate seed data | Create test data |
| [mock](#nself-dev-mock) | Mock services | Testing without dependencies |
| [tunnel](#nself-dev-tunnel) | Local tunneling | Expose local to internet |
| [hot-reload](#nself-dev-hot-reload) | Hot reload services | Development speed |
| [debug](#nself-dev-debug) | Debugging tools | Troubleshoot issues |
| [profile](#nself-dev-profile) | Performance profiling | Optimize performance |

## nself dev mode

Switch between development modes.

**Usage**:
```bash
nself dev mode <mode>
```

**Modes**:
- `development` - Full development mode (hot reload, verbose logging)
- `production` - Production-like mode (optimized, minimal logging)
- `test` - Test mode (mock services, test data)
- `debug` - Debug mode (extra logging, debugging tools)

**Examples**:
```bash
# Development mode
nself dev mode development

# Production mode
nself dev mode production

# Debug mode
nself dev mode debug
```

**Effects**:
- Changes `ENV` variable
- Adjusts logging levels
- Enables/disables hot reload
- Configures service behaviors

## nself dev frontend

Frontend framework detection and configuration.

**Usage**:
```bash
nself dev frontend <action> [OPTIONS]
```

**Actions**:
- `detect` - Auto-detect frontend framework
- `configure` - Configure frontend app
- `add` - Add frontend application
- `remove` - Remove frontend application
- `list` - List configured frontends

**Supported Frameworks**:
- React (Next.js, Create React App, Vite)
- Vue (Nuxt, Vue CLI, Vite)
- Angular
- Svelte (SvelteKit, Vite)
- Solid.js
- Qwik

**Package Managers**:
- pnpm (preferred)
- npm
- yarn
- bun

**Examples**:
```bash
# Detect framework
nself dev frontend detect

# Add frontend app
nself dev frontend add --name admin --port 3000 --framework nextjs

# List frontends
nself dev frontend list
```

**Output**:
```
Frontend Applications
──────────────────────────────────────────
admin-app    Next.js 14    pnpm    3000    ✓ Running
user-app     Vite + React  npm     3001    ✓ Running
```

## nself dev platforms

Detect and manage multi-platform support.

**Usage**:
```bash
nself dev platforms <action>
```

**Actions**:
- `detect` - Detect current platform
- `list` - List supported platforms
- `test` - Test platform compatibility
- `switch` - Switch platform mode

**Platforms**:
- macOS (darwin-arm64, darwin-x64)
- Linux (linux-x64, linux-arm64)
- Windows (WSL)

**Examples**:
```bash
# Detect platform
nself dev platforms detect

# List supported platforms
nself dev platforms list

# Test compatibility
nself dev platforms test
```

## nself dev build

Multi-platform build tools.

**Usage**:
```bash
nself dev build <target> [OPTIONS]
```

**Targets**:
- `docker` - Build Docker images
- `binary` - Build standalone binary
- `package` - Create distribution package

**Options**:
- `--platform PLATFORM` - Target platform
- `--arch ARCH` - Target architecture
- `--output DIR` - Output directory

**Examples**:
```bash
# Build for current platform
nself dev build binary

# Build for specific platform
nself dev build binary --platform linux --arch arm64

# Build Docker image
nself dev build docker --tag myapp:latest
```

## nself dev test

Test deployment workflows.

**Usage**:
```bash
nself dev test <workflow> [OPTIONS]
```

**Workflows**:
- `deploy` - Test deployment process
- `rollback` - Test rollback process
- `migration` - Test migrations
- `backup` - Test backup/restore
- `integration` - Integration tests

**Examples**:
```bash
# Test deployment workflow
nself dev test deploy --env staging

# Test migrations
nself dev test migration

# Run integration tests
nself dev test integration
```

## nself dev ci

CI/CD integration tools.

**Usage**:
```bash
nself dev ci <action> [PROVIDER]
```

**Actions**:
- `init` - Initialize CI configuration
- `generate` - Generate CI workflow
- `validate` - Validate CI config
- `test` - Test CI locally

**Providers**:
- GitHub Actions
- GitLab CI
- CircleCI
- Jenkins
- Travis CI

**Examples**:
```bash
# Generate GitHub Actions workflow
nself dev ci generate github-actions

# Validate workflow
nself dev ci validate

# Test locally
nself dev ci test
```

**Generated Workflow**:
```yaml
# .github/workflows/nself-deploy.yml
name: Deploy nself

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Install nself
        run: curl -sSL https://install.nself.org | bash
      - name: Deploy to staging
        run: nself deploy staging
```

## nself dev docs

Documentation generation.

**Usage**:
```bash
nself dev docs <action> [OPTIONS]
```

**Actions**:
- `generate` - Generate documentation
- `serve` - Serve docs locally
- `build` - Build static docs
- `deploy` - Deploy docs

**Generates**:
- API documentation (GraphQL schema)
- Database schema docs
- Configuration reference
- Command reference

**Examples**:
```bash
# Generate docs
nself dev docs generate

# Serve locally
nself dev docs serve

# Build for deployment
nself dev docs build
```

## nself dev scaffold

Code scaffolding and generators.

**Usage**:
```bash
nself dev scaffold <type> <name> [OPTIONS]
```

**Types**:
- `migration` - Database migration
- `seed` - Seed data file
- `function` - Serverless function
- `api` - API endpoint
- `model` - Data model
- `component` - Frontend component

**Examples**:
```bash
# Scaffold migration
nself dev scaffold migration add_user_profiles

# Scaffold function
nself dev scaffold function send_email --runtime nodejs20

# Scaffold API endpoint
nself dev scaffold api users --crud
```

## nself dev seed-generate

Generate realistic seed data.

**Usage**:
```bash
nself dev seed-generate <table> [OPTIONS]
```

**Options**:
- `--count N` - Number of records
- `--locale LOCALE` - Data locale (en_US, es_ES, etc.)
- `--template TEMPLATE` - Use template

**Examples**:
```bash
# Generate 100 users
nself dev seed-generate users --count 100

# Generate with Spanish locale
nself dev seed-generate users --count 50 --locale es_ES
```

## nself dev mock

Mock external services for testing.

**Usage**:
```bash
nself dev mock <service> [OPTIONS]
```

**Services**:
- `api` - Mock API endpoints
- `email` - Mock email service
- `storage` - Mock S3 storage
- `payment` - Mock payment gateway

**Examples**:
```bash
# Mock email service
nself dev mock email --port 1025

# Mock payment gateway
nself dev mock payment stripe
```

## nself dev tunnel

Create local tunnel to internet.

**Usage**:
```bash
nself dev tunnel [OPTIONS]
```

**Options**:
- `--port PORT` - Local port to expose
- `--subdomain NAME` - Custom subdomain
- `--provider PROVIDER` - Tunnel provider (ngrok, cloudflared)

**Examples**:
```bash
# Expose local port 8080
nself dev tunnel --port 8080

# With custom subdomain
nself dev tunnel --port 8080 --subdomain myapp
```

**Output**:
```
Tunnel created successfully!

Local:  http://localhost:8080
Public: https://myapp.ngrok.io

Press Ctrl+C to close tunnel
```

## nself dev hot-reload

Configure hot reload for services.

**Usage**:
```bash
nself dev hot-reload <service> [OPTIONS]
```

**Options**:
- `--enable` - Enable hot reload
- `--disable` - Disable hot reload
- `--watch PATH` - Watch specific path

**Examples**:
```bash
# Enable hot reload for custom service
nself dev hot-reload api --enable

# Watch specific directory
nself dev hot-reload api --watch ./src
```

## nself dev debug

Debugging tools and utilities.

**Usage**:
```bash
nself dev debug <action> [OPTIONS]
```

**Actions**:
- `attach` - Attach debugger to service
- `logs` - Enhanced debug logging
- `trace` - Distributed tracing
- `breakpoint` - Set breakpoints

**Examples**:
```bash
# Attach debugger to Node.js service
nself dev debug attach api --port 9229

# Enable debug logs
nself dev debug logs api --level debug
```

## nself dev profile

Performance profiling tools.

**Usage**:
```bash
nself dev profile <service> [OPTIONS]
```

**Options**:
- `--duration N` - Profile duration (seconds)
- `--output FILE` - Save profile to file
- `--format FORMAT` - Output format (json, flamegraph)

**Examples**:
```bash
# Profile API service
nself dev profile api --duration 60

# Generate flamegraph
nself dev profile api --format flamegraph --output profile.svg
```

## Development Workflows

### Full-Stack Development

```bash
# 1. Start in development mode
nself dev mode development

# 2. Enable hot reload
nself dev hot-reload api --enable

# 3. Start services
nself start

# 4. Develop with auto-reload
# Services automatically reload on code changes
```

### Testing Workflow

```bash
# 1. Switch to test mode
nself dev mode test

# 2. Generate test data
nself dev seed-generate users --count 100

# 3. Run tests
nself dev test integration

# 4. Clean up
nself db reset
```

### Documentation Workflow

```bash
# 1. Generate documentation
nself dev docs generate

# 2. Preview locally
nself dev docs serve

# 3. Build for production
nself dev docs build

# 4. Deploy
nself dev docs deploy
```

## Best Practices

### 1. Use Development Mode Locally

```bash
# Always use development mode for local work
nself dev mode development
```

### 2. Test with Production Mode

```bash
# Test in production mode before deploying
nself dev mode production
nself start
# Run tests
```

### 3. Generate Realistic Seed Data

```bash
# Use seed generator for test data
nself dev seed-generate users --count 1000
nself dev seed-generate orders --count 5000
```

### 4. Profile Before Optimizing

```bash
# Profile to find bottlenecks
nself dev profile api --duration 60
# Optimize based on results
```

## Related Commands

- `nself start` - Start development environment
- `nself logs` - View development logs
- `nself db seed` - Load seed data
- `nself test` - Run tests

## See Also

- [Development Guide](../../guides/DEVELOPMENT.md)
- [Testing Guide](../../guides/TESTING.md)
- [CI/CD Integration](../../guides/CI-CD.md)
- [Frontend Integration](../../guides/FRONTEND.md)
