# nself Environments Guide

**Version 0.9.9** | Managing development, staging, and production environments

---

## Overview

nself supports multiple environments with cascading configuration. This guide covers setting up and managing environments for your project.

---

## Environment Types

| Environment | Purpose | Configuration |
|-------------|---------|---------------|
| `local` | Developer machine | `.env.dev` + `.env.local` |
| `staging` | Testing/QA | `.env.dev` + `.env.staging` |
| `production` | Live users | `.env.dev` + `.env.prod` + `.secrets` |

---

## Configuration Hierarchy

Files load in order, with later files overriding earlier:

```
.env.dev          # Base config (committed)
    ↓
.env.local        # Local overrides (gitignored)
    ↓
.env.staging      # Staging overrides
    ↓
.env.prod         # Production overrides
    ↓
.secrets          # Sensitive credentials
```

---

## Setting Up Environments

### Local Development

```bash
# Create project
nself init

# Base configuration
cp .env.example .env.dev

# Local overrides (gitignored)
echo "DEBUG=true" > .env.local
```

### Staging

```bash
# Create staging environment
nself env create staging

# Configure staging
vi .environments/staging/.env.staging
vi .environments/staging/server.json
```

### Production

```bash
# Create production environment
nself env create prod

# Configure production (on server)
vi .env.prod

# Generate secrets (on server)
nself init --secure
```

---

## Switching Environments

```bash
# Switch to staging
nself env switch staging

# Switch to production
nself env switch prod

# Switch back to local
nself env switch local
```

---

## Environment Variables

### Required Variables

| Variable | Description |
|----------|-------------|
| `PROJECT_NAME` | Project identifier |
| `ENV` | Environment (dev/staging/prod) |
| `BASE_DOMAIN` | Domain name |

### Environment-Specific

| Variable | Dev | Staging | Prod |
|----------|-----|---------|------|
| `DEBUG` | true | true | false |
| `LOG_LEVEL` | debug | info | warn |
| `SSL_MODE` | mkcert | letsencrypt | letsencrypt |

---

## Syncing Environments

```bash
# Pull staging config
nself sync config staging

# Pull production data (anonymized)
nself sync db prod --anonymize
```

---

## Access Control

| Role | Local | Staging | Prod |
|------|-------|---------|------|
| Dev | ✅ | ❌ | ❌ |
| Sr Dev | ✅ | ✅ | ❌ |
| Lead | ✅ | ✅ | ✅ |

---

## Best Practices

1. **Never commit secrets** - Use `.secrets` file on server
2. **Use cascading config** - Base in `.env.dev`, overrides per environment
3. **Anonymize production data** - When syncing to local/staging
4. **Separate credentials** - Different passwords per environment
5. **Test in staging first** - Before deploying to production

---

## See Also

- [env command](../commands/ENV.md)
- [deploy command](../commands/DEPLOY.md)
- [sync command](../commands/SYNC.md)
