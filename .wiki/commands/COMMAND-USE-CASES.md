# nself Commands by Use Case

**Find the right command for what you want to do.**

This guide is organized by task, not by command name. If you know WHAT you want to accomplish but not WHICH command to use, start here.

---

## Getting Started

| I want to... | Command | Details |
|---|---|---|
| Create a new project | `nself init` | [init](INIT.md) |
| Create a demo project with all features | `nself init --demo` | [init](INIT.md) |
| Use the simple setup wizard | `nself init --simple` | [init](INIT.md) |
| Generate all configuration files | `nself build` | [build](BUILD.md) |
| Start all services | `nself start` | [start](START.md) |
| Stop all services | `nself stop` | [stop](STOP.md) |
| Restart everything | `nself restart` | [restart](RESTART.md) |
| See all service URLs | `nself urls` | [urls](URLS.md) |
| Check if everything is healthy | `nself status` | [status](STATUS.md) |
| Run diagnostics when something is wrong | `nself doctor` | [doctor](DOCTOR.md) |
| Auto-fix common issues | `nself doctor --fix` | [doctor](DOCTOR.md) |

---

## Database Operations

| I want to... | Command | Details |
|---|---|---|
| Open a PostgreSQL shell | `nself db shell` | [db](DB.md) |
| Run a SQL query directly | `nself db query "SELECT ..."` | [db](DB.md) |
| Scaffold a schema from a template | `nself db schema scaffold saas` | [db](DB.md) |
| Import a DBML schema file | `nself db schema import schema.dbml` | [db](DB.md) |
| Run pending migrations | `nself db migrate up` | [db](DB.md) |
| Roll back the last migration | `nself db migrate down` | [db](DB.md) |
| Create a new migration file | `nself db migrate create add_users` | [db](DB.md) |
| Check migration status | `nself db migrate status` | [db](DB.md) |
| Seed the database with sample data | `nself db seed` | [db](DB.md) |
| Generate realistic mock data | `nself db mock auto` | [db](DB.md) |
| Back up the database | `nself db backup` | [db](DB.md) |
| Restore from a backup | `nself db restore latest` | [db](DB.md) |
| Generate TypeScript types from schema | `nself db types typescript` | [db](DB.md) |
| Generate Go structs from schema | `nself db types go` | [db](DB.md) |
| Check table sizes and disk usage | `nself db inspect size` | [db](DB.md) |
| Find slow queries | `nself db inspect slow` | [db](DB.md) |
| Export table data to CSV | `nself db data export users` | [db](DB.md) |
| Anonymize PII in the database | `nself db data anonymize` | [db](DB.md) |

---

## Adding & Managing Services

| I want to... | Command | Details |
|---|---|---|
| See which services are running | `nself status` | [status](STATUS.md) |
| Enable Redis caching | Set `REDIS_ENABLED=true` in .env, then `nself build` | [service](SERVICE.md) |
| Enable file storage (MinIO) | Set `MINIO_ENABLED=true` in .env, then `nself build` | [service](SERVICE.md) |
| Enable search (MeiliSearch) | Set `MEILISEARCH_ENABLED=true` in .env, then `nself build` | [service](SERVICE.md) |
| Enable email testing (MailPit) | Set `MAILPIT_ENABLED=true` in .env, then `nself build` | [service](SERVICE.md) |
| Enable serverless functions | Set `FUNCTIONS_ENABLED=true` in .env, then `nself build` | [service](SERVICE.md) |
| Enable ML experiment tracking | Set `MLFLOW_ENABLED=true` in .env, then `nself build` | [service](SERVICE.md) |
| Enable the admin UI | Set `NSELF_ADMIN_ENABLED=true` in .env, then `nself build` | [admin](ADMIN.md) |
| Enable full monitoring stack | Set `MONITORING_ENABLED=true` in .env, then `nself build` | [../services/MONITORING-BUNDLE.md](../services/MONITORING-BUNDLE.md) |
| List available service templates | `nself service list` | [service](SERVICE.md) |
| Scaffold a new custom service | `nself service scaffold` | [service](SERVICE.md) |
| Use the service creation wizard | `nself service wizard` | [service](SERVICE.md) |
| Add a custom service via env | Add `CS_1=api:express-js:8001` to .env, then `nself build` | [../services/SERVICES_CUSTOM.md](../services/SERVICES_CUSTOM.md) |

---

## Custom Services (CS_N)

| I want to... | Command | Details |
|---|---|---|
| Add an Express.js API | `CS_1=api:express-js:8001` in .env | [../services/SERVICES_CUSTOM.md](../services/SERVICES_CUSTOM.md) |
| Add a background worker | `CS_2=worker:bullmq-js:8002` in .env | [../services/SERVICES_CUSTOM.md](../services/SERVICES_CUSTOM.md) |
| Add a gRPC service | `CS_3=grpc:grpc:8003` in .env | [../services/SERVICES_CUSTOM.md](../services/SERVICES_CUSTOM.md) |
| Add a Python API | `CS_4=ml-api:python-api:8004` in .env | [../services/SERVICES_CUSTOM.md](../services/SERVICES_CUSTOM.md) |
| View available templates | `nself service list` | [../reference/SERVICE_TEMPLATES.md](../reference/SERVICE_TEMPLATES.md) |
| Rebuild after adding a service | `nself build && nself restart` | [build](BUILD.md) |

---

## Deployment

| I want to... | Command | Details |
|---|---|---|
| Deploy to staging | `nself deploy staging` | [deploy](DEPLOY.md) |
| Deploy to production | `nself deploy production` | [deploy](DEPLOY.md) |
| Create a preview environment for a branch | `nself deploy preview feature-branch` | [deploy](DEPLOY.md) |
| Do a zero-downtime deployment | `nself deploy blue-green` | [deploy](DEPLOY.md) |
| Do a gradual canary rollout | `nself deploy canary --percentage 20` | [deploy](DEPLOY.md) |
| Roll back the last deployment | `nself deploy rollback` | [deploy](DEPLOY.md) |
| Roll back to a specific version | `nself backup rollback --version 3` | [backup](BACKUP.md) |
| Check deployment status | `nself deploy status` | [deploy](DEPLOY.md) |
| View deployment history | `nself deploy history` | [deploy](DEPLOY.md) |
| Promote staging to production | `nself deploy promote staging production` | [deploy](DEPLOY.md) |

---

## Server Management

| I want to... | Command | Details |
|---|---|---|
| Provision a new server | `nself deploy provision digitalocean` | [deploy](DEPLOY.md) |
| Initialize a VPS for nself | `nself deploy server init <host>` | [deploy](DEPLOY.md) |
| Check if a server is ready | `nself deploy server check <host>` | [deploy](DEPLOY.md) |
| List all configured servers | `nself deploy server list` | [deploy](DEPLOY.md) |
| SSH into a server | `nself deploy server ssh <name>` | [deploy](DEPLOY.md) |
| Run diagnostics on a server | `nself deploy server diagnose <env>` | [deploy](DEPLOY.md) |
| Sync config to a remote server | `nself deploy sync push staging` | [deploy](DEPLOY.md) |
| Pull config from a remote server | `nself deploy sync pull staging` | [deploy](DEPLOY.md) |

---

## Cloud Infrastructure

| I want to... | Command | Details |
|---|---|---|
| List supported cloud providers | `nself infra provider list` | [infra](INFRA.md) |
| Configure cloud credentials | `nself infra provider init aws` | [infra](INFRA.md) |
| Estimate hosting costs | `nself infra provider cost estimate digitalocean` | [infra](INFRA.md) |
| Compare provider pricing | `nself infra provider cost compare` | [infra](INFRA.md) |
| Quick-deploy to a cloud provider | `nself infra provider deploy quick digitalocean` | [infra](INFRA.md) |
| Create a managed K8s cluster | `nself infra provider k8s-create aws my-cluster us-east-1 3 medium` | [infra](INFRA.md) |
| Convert Compose to K8s manifests | `nself infra k8s convert` | [infra](INFRA.md) |
| Deploy to Kubernetes | `nself infra k8s deploy` | [infra](INFRA.md) |
| Create a Helm chart | `nself infra helm init` | [infra](INFRA.md) |
| Install a Helm release | `nself infra helm install` | [infra](INFRA.md) |

---

## Multi-Tenancy

| I want to... | Command | Details |
|---|---|---|
| Initialize multi-tenancy | `nself tenant init` | [tenant](TENANT.md) |
| Create a new tenant | `nself tenant create "Acme Corp" --plan pro` | [tenant](TENANT.md) |
| List all tenants | `nself tenant list` | [tenant](TENANT.md) |
| View tenant details | `nself tenant show <tenant-id>` | [tenant](TENANT.md) |
| Suspend a tenant | `nself tenant suspend <tenant-id>` | [tenant](TENANT.md) |
| Add a member to a tenant | `nself tenant member add <tenant-id> user@email.com admin` | [tenant](TENANT.md) |
| Invite a member | `nself tenant member invite <tenant-id> user@email.com` | [tenant](TENANT.md) |
| Set up billing plans | `nself tenant billing plans` | [tenant](TENANT.md) |
| View tenant usage | `nself tenant billing usage <tenant-id>` | [tenant](TENANT.md) |
| View invoices | `nself tenant billing invoice <tenant-id>` | [tenant](TENANT.md) |
| Create an organization | `nself tenant org create "My Org"` | [tenant](TENANT.md) |

---

## Branding & White-Label

| I want to... | Command | Details |
|---|---|---|
| Upload a tenant logo | `nself tenant branding logo <tenant-id> logo.png` | [tenant](TENANT.md) |
| Set brand colors | `nself tenant branding colors <tenant-id> --primary #0066cc` | [tenant](TENANT.md) |
| Preview branding changes | `nself tenant branding preview <tenant-id>` | [tenant](TENANT.md) |
| Add a custom domain | `nself tenant domains add <tenant-id> app.example.com` | [tenant](TENANT.md) |
| Verify domain DNS | `nself tenant domains verify <tenant-id> app.example.com` | [tenant](TENANT.md) |
| Provision SSL for a custom domain | `nself tenant domains ssl <tenant-id> app.example.com` | [tenant](TENANT.md) |
| Customize email templates | `nself tenant email edit <tenant-id> welcome` | [tenant](TENANT.md) |
| Apply a theme | `nself tenant themes apply <tenant-id> dark-mode` | [tenant](TENANT.md) |
| Configure white-label settings | `nself dev whitelabel config` | [dev](DEV.md) |

---

## Security & Authentication

| I want to... | Command | Details |
|---|---|---|
| Set up OAuth (Google, GitHub, etc.) | `nself auth oauth enable --providers google,github` | [auth](AUTH.md) |
| Configure OAuth credentials | `nself auth oauth config google --client-id=... --client-secret=...` | [auth](AUTH.md) |
| Test an OAuth provider | `nself auth oauth test google` | [auth](AUTH.md) |
| Enable multi-factor authentication | `nself auth mfa enable` | [auth](AUTH.md) |
| Generate MFA backup codes | `nself auth mfa backup-codes` | [auth](AUTH.md) |
| Create a new role | `nself auth roles create editor` | [auth](AUTH.md) |
| Assign a role to a user | `nself auth roles assign user@email.com editor` | [auth](AUTH.md) |
| Generate SSL certificates | `nself auth ssl generate` | [auth](AUTH.md) |
| Trust local SSL certs (no browser warnings) | `nself auth ssl trust` | [auth](AUTH.md) |
| Renew SSL certificates | `nself auth ssl renew` | [auth](AUTH.md) |
| Configure rate limiting | `nself auth rate-limit config` | [auth](AUTH.md) |
| Run a security scan | `nself auth security scan` | [auth](AUTH.md) |
| Generate a security audit report | `nself auth security report` | [auth](AUTH.md) |
| Create a webhook endpoint | `nself auth webhooks create <url> [events]` | [auth](AUTH.md) |
| Test a webhook | `nself auth webhooks test <id>` | [auth](AUTH.md) |

---

## Configuration & Environment

| I want to... | Command | Details |
|---|---|---|
| Show current configuration | `nself config show` | [config](CONFIG.md) |
| Edit configuration | `nself config edit` | [config](CONFIG.md) |
| Validate my configuration | `nself config validate` | [config](CONFIG.md) |
| Switch to a different environment | `nself config env switch staging` | [config](CONFIG.md) |
| List all environments | `nself config env list` | [config](CONFIG.md) |
| Compare two environments | `nself config env diff staging prod` | [config](CONFIG.md) |
| Check my access level | `nself config env access` | [config](CONFIG.md) |
| List all secrets | `nself config secrets list` | [config](CONFIG.md) |
| Set a secret | `nself config secrets set API_KEY value` | [config](CONFIG.md) |
| Rotate all secrets | `nself config secrets rotate` | [config](CONFIG.md) |
| Initialize Vault integration | `nself config vault init` | [config](CONFIG.md) |
| Export config to a file | `nself config export > config.json` | [config](CONFIG.md) |
| Import config from a file | `nself config import config.json` | [config](CONFIG.md) |

---

## Monitoring & Debugging

| I want to... | Command | Details |
|---|---|---|
| Check service health | `nself status` | [status](STATUS.md) |
| Watch service health in real time | `nself status --watch` | [status](STATUS.md) |
| View logs for a service | `nself logs postgres` | [logs](LOGS.md) |
| Follow logs in real time | `nself logs hasura -f` | [logs](LOGS.md) |
| View the last 100 log lines | `nself logs --tail 100` | [logs](LOGS.md) |
| Open Grafana dashboards | `nself monitor grafana` | [monitor](MONITOR.md) |
| Open Prometheus | `nself monitor prometheus` | [monitor](MONITOR.md) |
| Open Alertmanager | `nself monitor alertmanager` | [monitor](MONITOR.md) |
| Run a deep health check | `nself health --deep` | [health](HEALTH.md) |
| Set a monitoring profile | `nself metrics profile standard` | [metrics](METRICS.md) |
| Run system diagnostics | `nself doctor` | [doctor](DOCTOR.md) |
| Execute a command inside a container | `nself exec postgres psql -U postgres` | [exec](EXEC.md) |
| View command history / audit trail | `nself history` | [history](HISTORY.md) |
| View security audit logs | `nself audit` | [audit](AUDIT.md) |

---

## Performance & Scaling

| I want to... | Command | Details |
|---|---|---|
| Profile a service | `nself perf profile postgres` | [perf](PERF.md) |
| Run a benchmark / load test | `nself perf bench` | [perf](PERF.md) |
| Scale a service horizontally | `nself perf scale hasura 3` | [perf](PERF.md) |
| Get optimization suggestions | `nself perf optimize` | [perf](PERF.md) |
| Auto-apply optimizations | `nself perf optimize --auto-fix` | [perf](PERF.md) |

---

## Backup & Recovery

| I want to... | Command | Details |
|---|---|---|
| Create a full backup | `nself backup create --full` | [backup](BACKUP.md) |
| Create an incremental backup | `nself backup create --incremental` | [backup](BACKUP.md) |
| List available backups | `nself backup list` | [backup](BACKUP.md) |
| Restore from a backup | `nself backup restore <backup-id>` | [backup](BACKUP.md) |
| Roll back to a previous version | `nself backup rollback --version 3` | [backup](BACKUP.md) |
| Reset to a clean state | `nself backup reset --confirm` | [backup](BACKUP.md) |
| Clean up old backups | `nself backup clean --age 30` | [backup](BACKUP.md) |

---

## Developer Tools

| I want to... | Command | Details |
|---|---|---|
| Add a frontend app for routing | `nself dev frontend add myapp 3000` | [dev](DEV.md) |
| List frontend apps | `nself dev frontend list` | [dev](DEV.md) |
| Remove a frontend app | `nself dev frontend remove myapp` | [dev](DEV.md) |
| Generate CI/CD config for GitHub Actions | `nself dev ci generate --provider github` | [dev](DEV.md) |
| Generate CI/CD config for GitLab | `nself dev ci generate --provider gitlab` | [dev](DEV.md) |
| Generate project documentation | `nself dev docs generate` | [dev](DEV.md) |
| Serve documentation locally | `nself dev docs serve` | [dev](DEV.md) |
| Configure white-label branding | `nself dev whitelabel config` | [dev](DEV.md) |
| Toggle developer mode | `nself dev mode on` | [dev](DEV.md) |

---

## Plugins

| I want to... | Command | Details |
|---|---|---|
| See available plugins | `nself plugin list` | [plugin](PLUGIN.md) |
| Install the Stripe plugin | `nself plugin install stripe` | [../plugins/stripe](/plugins/stripe) |
| Install the GitHub plugin | `nself plugin install github` | [../plugins/github](/plugins/github) |
| Install the Shopify plugin | `nself plugin install shopify` | [../plugins/shopify](/plugins/shopify) |
| Update all plugins | `nself plugin update --all` | [plugin](PLUGIN.md) |
| Check for plugin updates | `nself plugin updates` | [plugin](PLUGIN.md) |
| Remove a plugin | `nself plugin remove stripe` | [plugin](PLUGIN.md) |
| Create a custom plugin | `nself plugin create my-plugin` | [../plugins/development](../plugins/development) |

---

## File Storage

| I want to... | Command | Details |
|---|---|---|
| Initialize the storage system | `nself service storage init` | [service](SERVICE.md) |
| Upload a file | `nself service storage upload photo.jpg` | [service](SERVICE.md) |
| Upload with thumbnail generation | `nself service storage upload avatar.png --thumbnails` | [service](SERVICE.md) |
| List stored files | `nself service storage list` | [service](SERVICE.md) |
| Delete a file | `nself service storage delete path/to/file.txt` | [service](SERVICE.md) |
| Check pipeline status | `nself service storage status` | [service](SERVICE.md) |
| Set up GraphQL integration | `nself service storage graphql-setup` | [service](SERVICE.md) |

---

## Email

| I want to... | Command | Details |
|---|---|---|
| Send a test email | `nself service email test` | [service](SERVICE.md) |
| Configure email provider | `nself service email config <provider>` | [service](SERVICE.md) |
| Manage email templates | `nself service email template list` | [service](SERVICE.md) |

---

## Search

| I want to... | Command | Details |
|---|---|---|
| Initialize a search provider | `nself service search init meilisearch` | [service](SERVICE.md) |
| Rebuild search indexes | `nself service search index` | [service](SERVICE.md) |
| Test a search query | `nself service search query "search term"` | [service](SERVICE.md) |
| Configure search settings | `nself service search config` | [service](SERVICE.md) |

---

## System Maintenance

| I want to... | Command | Details |
|---|---|---|
| Update nself to the latest version | `nself update` | [update](UPDATE.md) |
| Check for available updates | `nself version --check` | [version](VERSION.md) |
| Show current version | `nself version` | [version](VERSION.md) |
| Set up shell completions | `nself completion bash` | [completion](COMPLETION.md) |
| Open the admin UI | `nself admin` | [admin](ADMIN.md) |
| Tear down all infrastructure | `nself destroy` | [destroy](DESTROY.md) |
| Preview what destroy would do | `nself destroy --dry-run` | [destroy](DESTROY.md) |
| Destroy containers but keep data | `nself destroy --keep-volumes` | [destroy](DESTROY.md) |

---

## Migrating From Other Platforms

| I want to... | Command | Details |
|---|---|---|
| Migrate from Firebase | `nself perf migrate from firebase` | [../migrations/FROM-FIREBASE.md](../migrations/FROM-FIREBASE.md) |
| Migrate from Supabase | `nself perf migrate from supabase` | [../migrations/FROM-SUPABASE.md](../migrations/FROM-SUPABASE.md) |
| Migrate from Nhost | `nself perf migrate from nhost` | [../migrations/FROM-NHOST.md](../migrations/FROM-NHOST.md) |

---

## Common Workflows

### New Project (5 minutes)

```bash
nself init --demo           # Create project with all features
nself build                 # Generate configuration
nself start                 # Start 25 containers
nself urls                  # See all service URLs
nself status                # Verify everything is healthy
```

### Daily Development

```bash
nself start                 # Start services
nself db migrate up         # Run any new migrations
nself logs hasura -f        # Follow API logs while working
nself stop                  # Stop when done
```

### Database Schema Workflow

```bash
nself db schema scaffold saas       # Start with a template
# Edit the generated DBML file
nself db schema apply schema.dbml   # Import + migrate
nself db types typescript           # Generate TypeScript types
nself db seed                       # Add sample data
```

### Deploy to Production

```bash
nself config validate               # Validate configuration
nself deploy staging                # Deploy to staging first
# Verify staging works
nself deploy production             # Deploy to production
nself deploy status                 # Check deployment status
```

### Set Up Multi-Tenant SaaS

```bash
nself tenant init                                   # Initialize multi-tenancy
nself tenant create "Acme Corp" --plan pro          # Create first tenant
nself tenant billing plans                          # Configure billing plans
nself tenant domains add acme app.acme.com          # Add custom domain
nself tenant branding logo acme logo.png            # Upload branding
```

---

## Still Cannot Find What You Need?

- **Browse the full command tree:** [COMMAND-TREE-V1.md](COMMAND-TREE-V1.md)
- **Read the complete reference:** [COMMANDS.md](COMMANDS.md)
- **Use built-in help:** `nself help <command>`
- **Run diagnostics:** `nself doctor`
- **Check the FAQ:** [../getting-started/FAQ.md](../getting-started/FAQ.md)

---

**[Back to Commands Index](INDEX.md)** | **[Back to Documentation Home](../README.md)**
