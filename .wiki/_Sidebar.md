# nself v0.9.9

### [Home](Home)
### [Documentation](README)
### [Commands](COMMANDS)
### [Root Structure Policy](ROOT-STRUCTURE-POLICY)

---

## Getting Started

- [Quick Start](getting-started/Quick-Start)
- [Installation](getting-started/Installation)
- [FAQ](getting-started/FAQ)
- [Database Workflow](guides/DATABASE-WORKFLOW)
- [Demo Setup](services/DEMO_SETUP)

---

## Commands (v1.0)

- [Commands Index](commands/INDEX)
- [Command by Use Case](commands/COMMAND-USE-CASES)
- [Commands Overview](commands/README)
- [Command Tree](commands/COMMAND-TREE-V1)
- [SPORT Command Matrix](commands/SPORT-COMMAND-MATRIX)
- [Quick Reference](reference/COMMAND-REFERENCE)

### Core (5)
- [init](commands/INIT) - Initialize
- [build](commands/BUILD) - Build configs
- [start](commands/START) - Start services
- [stop](commands/STOP) - Stop services
- [restart](commands/RESTART) - Restart

### Database (11 subcommands)
- [db](commands/DB) - Database management
  - migrate, seed, mock, backup, schema, types

### Multi-Tenant (50+ subcommands)
- [tenant](commands/TENANT) - Multi-tenancy
  - Includes: billing, org, branding, domains

### Deployment (23 subcommands)
- [deploy](commands/DEPLOY) - Deployments
  - Includes: staging, prod, upgrade, servers

### Infrastructure (38 subcommands)
- [infra](commands/INFRA) - Infrastructure
  - Includes: provider, k8s, helm

### Services (43 subcommands)
- [service](commands/SERVICE) - Services
  - Includes: storage, email, search, redis

### Auth & Security (38 subcommands)
- [auth](commands/AUTH) - Authentication
  - Includes: oauth, mfa, ssl, roles

### Configuration (20 subcommands)
- [config](commands/CONFIG) - Configuration
  - Includes: env, secrets, vault

### Utilities (15 commands)
- [status](commands/STATUS) - Status
- [logs](commands/LOGS) - Logs
- [urls](commands/URLS) - URLs
- [doctor](commands/DOCTOR) - Diagnostics
- [monitor](commands/MONITOR) - Monitoring

### Plugins (v0.4.8)
- [plugin](commands/PLUGIN) - Plugins

---

## Configuration

- [Configuration Guide](configuration/README)
- [Environment Variables](configuration/ENVIRONMENT-VARIABLES)
- [Custom Services (CS_N)](configuration/CUSTOM-SERVICES-ENV-VARS)
- [Secrets Management](configuration/SECRETS-MANAGEMENT)
- [SSL Configuration](configuration/SSL)
- [Start Options](configuration/START-COMMAND-OPTIONS)
- [Admin UI](configuration/Admin-UI)

---

## Services

- [Services Overview](services/SERVICES)
- [Service Comparison](services/SERVICE-COMPARISON)
- [Required (4)](services/SERVICES_REQUIRED)
- [Optional (7)](services/SERVICES_OPTIONAL)
  - [Redis](services/REDIS) - Cache & sessions
  - [MinIO](services/MINIO) - Object storage
  - [Search](services/SEARCH) - Full-text search
  - [Mail](services/MAIL) - Email service
  - [Functions](services/FUNCTIONS) - Serverless runtime
  - [MLflow](services/MLFLOW) - ML tracking
  - [Admin UI](services/NSELF_ADMIN) - Web management
- [Monitoring (10)](services/MONITORING-BUNDLE)
- [Custom Services](services/SERVICES_CUSTOM)
- [Templates](reference/SERVICE_TEMPLATES)

---

## Plugins (v0.4.8)

- [Plugin Overview](plugins/README)
- [Plugin Development](plugins/development)
- [Stripe](plugins/stripe) - Payments
- [GitHub](plugins/github) - DevOps
- [Shopify](plugins/shopify) - E-commerce

---

## Guides

- [Guides Overview](guides/README)
- [Database Workflow](guides/DATABASE-WORKFLOW)
- [Service Communication](guides/SERVICE-TO-SERVICE-COMMUNICATION)
- [Deployment](guides/Deployment)
- [Backup Guide](guides/BACKUP_GUIDE)
- [Multi-App Setup](guides/MULTI_APP_SETUP)
- [OAuth Setup](guides/OAUTH-SETUP)
- [Real-Time Features](guides/REALTIME-FEATURES)
- [Organization Management](guides/ORGANIZATION-MANAGEMENT)
- [Billing & Usage](guides/BILLING-AND-USAGE)
- [Security](guides/SECURITY)
- [Troubleshooting](guides/TROUBLESHOOTING)

---

## Tutorials

- [Tutorials Index](tutorials/INDEX)
- [SaaS Quick Start](tutorials/QUICK-START-SAAS)
- [B2B Quick Start](tutorials/QUICK-START-B2B)
- [Marketplace](tutorials/QUICK-START-MARKETPLACE)
- [Agency Platform](tutorials/QUICK-START-AGENCY)
- [Build Multi-Tenant SaaS](tutorials/BUILD-MULTI-TENANT-SAAS)
- [Zero to Production](tutorials/ZERO-TO-PRODUCTION-15MIN)
- [Stripe Integration](tutorials/STRIPE-INTEGRATION)
- [Custom Domains](tutorials/CUSTOM-DOMAINS)
- [File Uploads](tutorials/file-uploads-quickstart)

---

## Examples

- [Examples Index](examples/INDEX)
- [Features Overview](examples/FEATURES-OVERVIEW)
- [Real-Time Chat](examples/REALTIME-CHAT-SERVICE)
- [Deployment Examples](deployment/examples/README)

---

## Architecture

- [Architecture Index](architecture/INDEX)
- [System Design](architecture/ARCHITECTURE)
- [Multi-Tenancy](architecture/MULTI-TENANCY)
- [Project Structure](architecture/PROJECT_STRUCTURE)
- [Build System](architecture/BUILD_ARCHITECTURE)
- [Command Consolidation](architecture/COMMAND-CONSOLIDATION-MAP)

---

## Reference

- [Reference Index](reference/INDEX)
- [Command Reference](reference/COMMAND-REFERENCE)
- [Quick Reference Cards](reference/QUICK-REFERENCE-CARDS)
- [Service Scaffolding](reference/SERVICE-SCAFFOLDING-CHEATSHEET)
- [Feature Comparison](reference/FEATURE-COMPARISON)

### API Reference
- [GraphQL API](architecture/API)
- [Billing API](reference/api/BILLING-API)
- [OAuth API](reference/api/OAUTH-API)
- [White-Label API](reference/api/WHITE-LABEL-API)

---

## Deployment

- [Deployment Guide](deployment/README)
- [Production](deployment/PRODUCTION-DEPLOYMENT)
- [Cloud Providers](deployment/CLOUD-PROVIDERS)
- [Server Management](deployment/SERVER-MANAGEMENT)

---

## Infrastructure

- [Infrastructure Docs](infrastructure/README)
- [Kubernetes Guide](infrastructure/K8S-IMPLEMENTATION-GUIDE)

---

## Security

- [Security Index](security/INDEX)
- [Security Audit](security/SECURITY-AUDIT)
- [Best Practices](security/SECURITY-BEST-PRACTICES)
- [SQL Safety](security/SQL-SAFETY)
- [Rate Limiting](security/RATE-LIMITING)
- [Compliance](security/COMPLIANCE-GUIDE)
  - [GDPR](security/GDPR-COMPLIANCE)
  - [HIPAA](security/HIPAA-COMPLIANCE)
  - [SOC2](security/SOC2-COMPLIANCE)

---

## Performance

- [Optimization Guide](performance/PERFORMANCE-OPTIMIZATION-V0.9.8.md)
- [Benchmarks](development/PERFORMANCE-BENCHMARKS)

---

## Troubleshooting

- [Troubleshooting Guide](troubleshooting/README)
- [Error Messages](troubleshooting/ERROR-MESSAGES)
- [Billing Issues](troubleshooting/BILLING-TROUBLESHOOTING)
- [White-Label Issues](troubleshooting/WHITE-LABEL-TROUBLESHOOTING)

---

## Features

- [Features Index](features/INDEX)
- [Real-Time](features/REALTIME)
- [White-Label](features/WHITELABEL-SYSTEM)
- [File Uploads](features/file-upload-pipeline)

---

## Migrations

- [Migration Guides](migrations/INDEX)
- [From Firebase](migrations/FROM-FIREBASE)
- [From Supabase](migrations/FROM-SUPABASE)
- [From Nhost](migrations/FROM-NHOST)

---

## Testing & QA

- [Testing Docs](testing/README)
- [QA Reports](qa/README)

---

## Releases

- [Release Index](releases/INDEX)
- [Roadmap](releases/ROADMAP)
- [v0.9.8](releases/v0.9.8.md) - Current
- [v0.9.0](releases/v0.9.0.md) - Multi-Tenant
- [v0.4.8](releases/v0.4.8.md) - Plugins
- [Changelog](releases/CHANGELOG)

---

## Contributing

- [Contributing Guide](contributing/CONTRIBUTING)
- [Development](contributing/DEVELOPMENT)
- [Cross-Platform](contributing/CROSS-PLATFORM-COMPATIBILITY)
