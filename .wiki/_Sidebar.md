# nself v0.9.9

### [Home](Home.md)
### [Documentation](README.md)
### [Commands](COMMANDS.md)
### [Root Structure Policy](ROOT-STRUCTURE-POLICY.md)

---

## Getting Started

- [Quick Start](getting-started/Quick-Start.md)
- [Installation](getting-started/Installation.md)
- [FAQ](getting-started/FAQ.md)
- [Database Workflow](guides/DATABASE-WORKFLOW.md)
- [Demo Setup](services/DEMO_SETUP.md)

---

## Commands (v1.0)

- [Commands Index](commands/INDEX.md)
- [**Command Tree**](commands/COMMAND-TREE-V1.md) ⭐ Authoritative Reference
- [Command by Use Case](commands/COMMAND-USE-CASES.md)
- [SPORT Command Matrix](commands/SPORT-COMMAND-MATRIX.md)
- [Quick Reference](reference/COMMAND-REFERENCE.md)

### Core (5)
- [init](commands/INIT.md) - Initialize
- [build](commands/BUILD.md) - Build configs
- [start](commands/START.md) - Start services
- [stop](commands/STOP.md) - Stop services
- [restart](commands/RESTART.md) - Restart

### Database (11 subcommands)
- [db](commands/DB.md) - Database management
  - migrate, seed, mock, backup, schema, types

### Multi-Tenant (50+ subcommands)
- [tenant](commands/TENANT.md) - Multi-tenancy
  - Includes: billing, org, branding, domains

### Deployment (23 subcommands)
- [deploy](commands/DEPLOY.md) - Deployments
  - Includes: staging, prod, upgrade, servers

### Infrastructure (38 subcommands)
- [infra](commands/INFRA.md) - Infrastructure
  - Includes: provider, k8s, helm

### Services (43 subcommands)
- [service](commands/SERVICE.md) - Services
  - Includes: storage, email, search, redis

### Auth & Security (38 subcommands)
- [auth](commands/AUTH.md) - Authentication
  - Includes: oauth, mfa, ssl, roles

### Configuration (20 subcommands)
- [config](commands/CONFIG.md) - Configuration
  - Includes: env, secrets, vault

### Utilities (15 commands)
- [status](commands/STATUS.md) - Status
- [logs](commands/LOGS.md) - Logs
- [urls](commands/URLS.md) - URLs
- [doctor](commands/DOCTOR.md) - Diagnostics
- [monitor](commands/MONITOR.md) - Monitoring

### Plugins (v0.4.8)
- [plugin](commands/PLUGIN.md) - Plugins

---

## Configuration

- [Configuration Guide](configuration/README.md)
- [Environment Variables](configuration/ENVIRONMENT-VARIABLES.md)
- [Custom Services (CS_N)](configuration/CUSTOM-SERVICES-ENV-VARS.md)
- [Secrets Management](configuration/SECRETS-MANAGEMENT.md)
- [SSL Configuration](configuration/SSL.md)
- [Start Options](configuration/START-COMMAND-OPTIONS.md)
- [Admin UI](configuration/Admin-UI.md)

---

## Services

- [Services Overview](services/SERVICES.md)
- [Service Comparison](services/SERVICE-COMPARISON.md)
- [Required (4)](services/SERVICES_REQUIRED.md)
- [Optional (7)](services/SERVICES_OPTIONAL.md)
  - [Redis](services/REDIS.md) - Cache & sessions
  - [MinIO](services/MINIO.md) - Object storage
  - [Search](services/SEARCH.md) - Full-text search
  - [Mail](services/MAIL.md) - Email service
  - [Functions](services/FUNCTIONS.md) - Serverless runtime
  - [MLflow](services/MLFLOW.md) - ML tracking
  - [Admin UI](services/NSELF_ADMIN.md) - Web management
- [Monitoring (10)](services/MONITORING-BUNDLE.md)
- [Custom Services](services/SERVICES_CUSTOM.md)
- [Templates](reference/SERVICE_TEMPLATES.md)

---

## Plugins (v0.4.8)

- [Plugin Overview](plugins/README.md)
- [Plugin Development](plugins/development.md)
- [Stripe](plugins/stripe.md) - Payments
- [GitHub](plugins/github.md) - DevOps
- [Shopify](plugins/shopify.md) - E-commerce

---

## Guides

- [Guides Overview](guides/README.md)
- [Database Workflow](guides/DATABASE-WORKFLOW.md)
- [Service Communication](guides/SERVICE-TO-SERVICE-COMMUNICATION.md)
- [Deployment](guides/Deployment.md)
- [Backup Guide](guides/BACKUP_GUIDE.md)
- [Multi-App Setup](guides/MULTI_APP_SETUP.md)
- [OAuth Setup](guides/OAUTH-SETUP.md)
- [Real-Time Features](guides/REALTIME-FEATURES.md)
- [Organization Management](guides/ORGANIZATION-MANAGEMENT.md)
- [Billing & Usage](guides/BILLING-AND-USAGE.md)
- [Security](guides/SECURITY.md)
- [Troubleshooting](guides/TROUBLESHOOTING.md)
- [**Deprecated Commands Migration**](guides/DEPRECATED-COMMANDS-MIGRATION.md) ⚠️

---

## Tutorials

- [Tutorials Index](tutorials/INDEX.md)
- [SaaS Quick Start](tutorials/QUICK-START-SAAS.md)
- [B2B Quick Start](tutorials/QUICK-START-B2B.md)
- [Marketplace](tutorials/QUICK-START-MARKETPLACE.md)
- [Agency Platform](tutorials/QUICK-START-AGENCY.md)
- [Build Multi-Tenant SaaS](tutorials/BUILD-MULTI-TENANT-SAAS.md)
- [Zero to Production](tutorials/ZERO-TO-PRODUCTION-15MIN.md)
- [Stripe Integration](tutorials/STRIPE-INTEGRATION.md)
- [Custom Domains](tutorials/CUSTOM-DOMAINS.md)
- [File Uploads](tutorials/file-uploads-quickstart.md)

---

## Examples

- [Examples Index](examples/INDEX.md)
- [Features Overview](examples/FEATURES-OVERVIEW.md)
- [Real-Time Chat](examples/REALTIME-CHAT-SERVICE.md)
- [Deployment Examples](deployment/examples/README.md)

---

## Architecture

- [Architecture Index](architecture/INDEX.md)
- [System Design](architecture/ARCHITECTURE.md)
- [Multi-Tenancy](architecture/MULTI-TENANCY.md)
- [Project Structure](architecture/PROJECT_STRUCTURE.md)
- [Build System](architecture/BUILD_ARCHITECTURE.md)
- [Command Consolidation](architecture/COMMAND-CONSOLIDATION-MAP.md)

---

## Reference

- [Reference Index](reference/INDEX.md)
- [Command Reference](reference/COMMAND-REFERENCE.md)
- [Quick Reference Cards](reference/QUICK-REFERENCE-CARDS.md)
- [Service Scaffolding](reference/SERVICE-SCAFFOLDING-CHEATSHEET.md)
- [Feature Comparison](reference/FEATURE-COMPARISON.md)

### API Reference
- [GraphQL API](architecture/API.md)
- [Billing API](reference/api/BILLING-API.md)
- [White-Label API](reference/api/WHITE-LABEL-API.md)

---

## Deployment

- [Deployment Guide](deployment/README.md)
- [Production](deployment/PRODUCTION-DEPLOYMENT.md)
- [Cloud Providers](deployment/CLOUD-PROVIDERS.md)
- [Server Management](deployment/SERVER-MANAGEMENT.md)

---

## Infrastructure

- [Infrastructure Docs](infrastructure/README.md)
- [Kubernetes Guide](infrastructure/K8S-IMPLEMENTATION-GUIDE.md)

---

## Security

- [Security Index](security/INDEX.md)
- [Security Audit](security/SECURITY-AUDIT.md)
- [Best Practices](security/SECURITY-BEST-PRACTICES.md)
- [SQL Safety](security/SQL-SAFETY.md)
- [Rate Limiting](security/RATE-LIMITING.md)
- [Compliance](security/COMPLIANCE-GUIDE.md)
  - [GDPR](security/GDPR-COMPLIANCE.md)
  - [HIPAA](security/HIPAA-COMPLIANCE.md)
  - [SOC2](security/SOC2-COMPLIANCE.md)

---

## Performance

- [Optimization Guide](performance/PERFORMANCE-OPTIMIZATION-V0.9.8.md)
- [Benchmarks](development/PERFORMANCE-BENCHMARKS.md)

---

## Troubleshooting

- [Troubleshooting Guide](troubleshooting/README.md)
- [Error Messages](troubleshooting/ERROR-MESSAGES.md)
- [Billing Issues](troubleshooting/BILLING-TROUBLESHOOTING.md)
- [White-Label Issues](troubleshooting/WHITE-LABEL-TROUBLESHOOTING.md)

---

## Features

- [Features Index](features/INDEX.md)
- [Real-Time](features/REALTIME.md)
- [White-Label](features/WHITELABEL-SYSTEM.md)
- [File Uploads](features/file-upload-pipeline.md)

---

## Migrations

- [Migration Guides](migrations/INDEX.md)
- [From Firebase](migrations/FROM-FIREBASE.md)
- [From Supabase](migrations/FROM-SUPABASE.md)
- [From Nhost](migrations/FROM-NHOST.md)

---

## Testing & QA

- [Testing Docs](testing/README.md)
- [QA Reports](qa/README.md)

---

## Releases

- [Release Index](releases/INDEX.md)
- [Roadmap](releases/ROADMAP.md)
- [v0.9.8](releases/v0.9.8.md) - Current
- [v0.9.0](releases/v0.9.0.md) - Multi-Tenant
- [v0.4.8](releases/v0.4.8.md) - Plugins
- [Changelog](releases/CHANGELOG.md)

---

## Contributing

- [Contributing Guide](contributing/CONTRIBUTING.md)
- [Development](contributing/DEVELOPMENT.md)
- [Cross-Platform](contributing/CROSS-PLATFORM-COMPATIBILITY.md)
