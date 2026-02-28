# nself Command Reference

Complete documentation for all nself CLI commands.

## Quick Navigation

- [Core Commands](#core-commands) - Essential commands for project lifecycle
- [Database Commands](#database-commands) - Database management and migrations  
- [Configuration Commands](#configuration-commands) - Environment and secrets management
- [Service Commands](#service-commands) - Optional service management
- [Deployment Commands](#deployment-commands) - Production deployment
- [Development Commands](#development-commands) - Developer tools
- [Utilities](#utilities) - Helper commands

## Core Commands

| Command | Description |
|---------|-------------|
| [init](core/init.md) | Initialize a new nself project |
| [build](core/build.md) | Generate all configuration files |
| [start](core/start.md) | Start all services (supports monorepo) |
| [stop](core/stop.md) | Stop all services |
| [restart](core/restart.md) | Restart all services |

## Database Commands

All database operations use `nself db <subcommand>`.

| Subcommand | Description |
|------------|-------------|
| [migrate](db/migrate.md) | Manage database migrations |
| [seed](db/seed.md) | Load seed data |
| [shell](db/shell.md) | Open PostgreSQL shell |
| [backup](db/backup.md) | Create database backup |
| [restore](db/restore.md) | Restore from backup |
| [reset](db/reset.md) | Reset database (dev only) |

## Configuration Commands

All configuration operations use `nself config <subcommand>`.

| Subcommand | Description |
|------------|-------------|
| [show](config/show.md) | Show current configuration |
| [get](config/get.md) | Get configuration value |
| [set](config/set.md) | Set configuration value |
| [list](config/list.md) | List all configuration |
| [env](config/env.md) | Environment management |
| [secrets](config/secrets.md) | Secrets management |
| [vault](config/vault.md) | Vault operations |

## Service Commands

All service operations use `nself service <type> <action>`.

| Service Type | Description |
|--------------|-------------|
| [admin](service/admin.md) | nself Admin UI |
| [storage](service/storage.md) | MinIO S3 storage |
| [email](service/email.md) | Email service |
| [search](service/search.md) | Search service (MeiliSearch) |
| [redis](service/redis.md) | Redis cache |
| [functions](service/functions.md) | Serverless functions |
| [mlflow](service/mlflow.md) | ML experiment tracking |
| [realtime](service/realtime.md) | Realtime subscriptions |

## Deployment Commands  

All deployment operations use `nself deploy <subcommand>`.

| Subcommand | Description |
|------------|-------------|
| [staging](deploy/staging.md) | Deploy to staging |
| [production](deploy/production.md) | Deploy to production |
| [upgrade](deploy/upgrade.md) | Upgrade deployment |
| [server](deploy/server.md) | Server management |
| [provision](deploy/provision.md) | Provision infrastructure |
| [sync](deploy/sync.md) | Sync deployment |
| [release](deploy/release.md) | Create release package |
| [protect](deploy/protect.md) | Environment protection |

## Development Commands

All development operations use `nself dev <subcommand>`.

| Subcommand | Description |
|------------|-------------|
| [mode](dev/mode.md) | Switch development mode |
| [frontend](dev/frontend.md) | Frontend tools |
| [platforms](dev/platforms.md) | Platform detection |
| [build](dev/build.md) | Multi-platform builds |
| [test](dev/test.md) | Workflow testing |
| [ci](dev/ci.md) | CI/CD integration |
| [docs](dev/docs.md) | Documentation generation |

## Utilities

| Command | Description |
|---------|-------------|
| [status](utilities/status.md) | Service status |
| [logs](utilities/logs.md) | View service logs |
| [urls](utilities/urls.md) | Show service URLs |
| [exec](utilities/exec.md) | Execute in container |
| [doctor](utilities/doctor.md) | Diagnostics |
| [monitor](utilities/monitor.md) | Monitoring dashboards |
| [health](utilities/health.md) | Health checks |
| [version](utilities/version.md) | Show version |
| [help](utilities/help.md) | Help system |

## Command Hierarchy

```
nself
├── Core (5)
│   ├── init
│   ├── build  
│   ├── start
│   ├── stop
│   └── restart
├── db (6 subcommands)
│   ├── migrate
│   ├── seed
│   ├── shell
│   ├── backup
│   ├── restore
│   └── reset
├── config (11 subcommands)
├── auth (13+ subcommands)
├── service (43+ subcommands across 8 types)
├── tenant (50+ subcommands)
├── deploy (23+ subcommands)
├── infra (38 subcommands)
├── perf (5 subcommands)
├── dev (16+ subcommands)
├── backup (8 subcommands)
└── Utilities (15 commands)
```

## Getting Help

- `nself help` - General help
- `nself <command> --help` - Command-specific help
- `nself <command> <subcommand> --help` - Subcommand-specific help

## See Also

- [Installation Guide](../installation/README.md)
- [Quick Start](../quick-start/README.md)
- [Configuration Reference](../configuration/README.md)
