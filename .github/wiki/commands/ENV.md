# nself config env

> **DEPRECATED COMMAND NAME**: This command was formerly `nself env` in v0.x. It has been consolidated to `nself config env` in v1.0. The old command name may still work as an alias.

Environment management command for creating, switching, and managing multiple deployment environments.

## Synopsis

```bash
nself config env <subcommand> [options]
```

## Description

The `nself config env` command provides comprehensive environment management capabilities. It allows you to create separate configurations for local development, staging, and production environments, with support for environment-specific variables, secrets, and server configurations.

## Subcommands

### create

Create a new environment configuration.

```bash
nself config env create <name> [template] [--force]
```

**Arguments:**
- `name` - Environment name (will be sanitized to lowercase alphanumeric with hyphens)
- `template` - Base template: `local`, `staging`, or `prod` (default: `local`)

**Options:**
- `--force, -f` - Overwrite existing environment

**Examples:**
```bash
nself config env create dev                    # Create local dev environment
nself config env create staging staging        # Create staging environment
nself config env create production prod        # Create production environment
nself config env create qa staging             # Create QA from staging template
```

### list

List all available environments.

```bash
nself config env list
```

Shows all environments with the current active environment marked with `*`.

### switch

Switch to a different environment.

```bash
nself config env switch <name> [--quiet]
```

**Arguments:**
- `name` - Environment to switch to

**What happens:**
1. Current `.env` files are backed up to `.env-backups/`
2. Environment-specific config is merged and applied to `.env.local`
3. Current environment marker is updated in `.current-env`

**Examples:**
```bash
nself config env switch staging
nself config env switch production
```

### status

Show current environment status.

```bash
nself config env status
```

Displays:
- Current active environment
- Domain configuration
- Enabled services
- Server connection status (for remote environments)

### info

Show detailed information about an environment.

```bash
nself config env info [name]
```

**Arguments:**
- `name` - Environment name (default: current environment)

### diff

Compare two environments.

```bash
nself config env diff <env1> <env2> [--values]
```

**Arguments:**
- `env1` - First environment to compare
- `env2` - Second environment to compare

**Options:**
- `--values` - Show actual values (secrets are masked)

**Examples:**
```bash
nself config env diff staging production
nself config env diff dev staging --values
```

### validate

Validate an environment's configuration.

```bash
nself config env validate [name]
```

Checks:
- Required environment variables are present
- Server configuration is valid
- Secrets file exists and has correct permissions
- Referenced files exist

### delete

Delete an environment configuration.

```bash
nself config env delete <name> [--force]
```

**Note:** Cannot delete the currently active environment.

### export

Export an environment as a tarball.

```bash
nself config env export <name> [--output <file>]
```

**Options:**
- `--output, -o` - Output filename (default: `<name>-env.tar.gz`)

### import

Import an environment from a tarball.

```bash
nself config env import <file> [--name <name>]
```

**Options:**
- `--name` - Override the environment name

## Environment Structure

Each environment is stored in `.environments/<name>/` with the following structure:

```
.environments/
└── staging/
    ├── .env              # Environment configuration
    ├── .env.secrets      # Secrets (600 permissions, git-ignored)
    └── server.json       # Remote server configuration
```

### .env File

Contains non-sensitive environment configuration:

```bash
ENV=staging
DEBUG=false
BASE_DOMAIN=staging.example.com
SSL_ENABLED=true
POSTGRES_DB=myproject_staging
```

### .env.secrets File

Contains sensitive secrets (automatically set to 600 permissions):

```bash
POSTGRES_PASSWORD=<secret>
HASURA_GRAPHQL_ADMIN_SECRET=<secret>
JWT_SECRET=<secret>
```

### server.json File

Contains remote server connection details:

```json
{
  "name": "staging",
  "type": "staging",
  "host": "staging.example.com",
  "port": 22,
  "user": "deploy",
  "key": "~/.ssh/staging_key",
  "deploy_path": "/opt/nself"
}
```

## Environment Cascade

When an environment is active, configuration is loaded in this order (later values override earlier):

1. `.env.dev` (base development config)
2. `.environments/<name>/.env` (environment-specific config)
3. `.env.local` (generated merged config)
4. `.env.secrets` (secrets)

## Best Practices

1. **Never commit secrets** - Add `.environments/*/.env.secrets` to `.gitignore`
2. **Use templates** - Start from standard templates for consistency
3. **Validate before deploy** - Run `nself env validate` before deploying
4. **Back up secrets** - Store production secrets securely outside git

## Related Commands

- [nself deploy](DEPLOY.md) - Deploy to environments
- [nself prod](PROD.md) - Production environment management
- [nself staging](STAGING.md) - Staging environment management

## See Also

- [Environment Configuration Guide](../guides/ENVIRONMENTS.md)
- [Deployment Pipeline](../guides/DEPLOYMENT.md)
