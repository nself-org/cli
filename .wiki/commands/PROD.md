# nself prod

> **⚠️ DEPRECATED**: `nself prod` is deprecated and will be removed in v1.0.0.
> Please use `nself deploy production` instead.
> Run `nself deploy --help` for full usage information.

Production environment management, security hardening, and monitoring.

## Synopsis

```bash
nself prod <subcommand> [options]
```

## Description

The `nself prod` command provides comprehensive production environment management including initialization, security auditing, secrets management, SSL certificate handling, and firewall configuration.

## Subcommands

### status

Show production environment status (default subcommand).

```bash
nself prod status
nself prod
```

Displays:
- Environment settings (ENV, domain, debug mode)
- Secrets file status
- SSL certificate status and expiry
- Docker Compose file status

### init

Initialize production configuration.

```bash
nself prod init <domain> [--email <email>]
```

**Arguments:**
- `domain` - Production domain (e.g., `example.com`)

**Options:**
- `--email` - Email for SSL certificates

**What it does:**
- Sets `ENV=production`
- Disables debug mode
- Enables SSL with Let's Encrypt
- Disables Hasura dev mode and console
- Creates `docker-compose.prod.yml`

**Example:**
```bash
nself prod init example.com --email admin@example.com
```

### check / audit

Run a comprehensive security audit.

```bash
nself prod check [--verbose]
nself prod audit [--verbose]
```

**Options:**
- `--verbose, -v` - Show detailed output

**Checks performed:**
- Environment settings (DEBUG, LOG_LEVEL, etc.)
- Secrets strength and configuration
- SSL certificate validity and expiry
- Docker security settings
- Network security (HSTS, XSS protection, etc.)
- File permissions

**Exit codes:**
- `0` - All checks passed
- `1` - Critical failures detected
- `2` - Warnings only (no critical failures)

### secrets

Manage production secrets.

```bash
nself prod secrets <action> [options]
```

**Actions:**

#### generate
Generate all production secrets.

```bash
nself prod secrets generate [--force]
```

Creates `.env.secrets` with secure random values for:
- `POSTGRES_PASSWORD`
- `HASURA_GRAPHQL_ADMIN_SECRET`
- `JWT_SECRET`
- `COOKIE_SECRET`
- `MINIO_ROOT_PASSWORD`
- `REDIS_PASSWORD`
- `GRAFANA_ADMIN_PASSWORD`

#### validate
Validate secrets file.

```bash
nself prod secrets validate
```

Checks:
- All required secrets are present
- Secrets meet minimum length requirements
- File has correct permissions (600)

#### rotate
Rotate a specific secret.

```bash
nself prod secrets rotate <SECRET_NAME>
```

Creates a backup before rotating.

#### show
Show secrets (masked by default).

```bash
nself prod secrets show [--unmask]
```

### ssl

Manage SSL/TLS certificates.

```bash
nself prod ssl <action> [options]
```

**Actions:**

#### status
Check current SSL certificate status.

```bash
nself prod ssl status
```

Shows subject, issuer, validity dates, and days until expiry.

#### request
Request a Let's Encrypt certificate.

```bash
nself prod ssl request <domain> [--email <email>] [--staging]
```

**Options:**
- `--email` - Email for Let's Encrypt registration
- `--staging` - Use Let's Encrypt staging (for testing)

#### renew
Renew SSL certificates.

```bash
nself prod ssl renew [--force]
```

#### self-signed
Generate a self-signed certificate (for development).

```bash
nself prod ssl self-signed [domain]
```

#### verify
Verify the certificate chain matches the private key.

```bash
nself prod ssl verify
```

### firewall

Configure and manage firewall rules.

```bash
nself prod firewall <action> [options]
```

**Actions:**

#### status
Check firewall status.

```bash
nself prod firewall status
```

Detects and shows status of UFW, firewalld, or iptables.

#### configure
Configure recommended firewall rules.

```bash
nself prod firewall configure [--dry-run]
```

**Options:**
- `--dry-run` - Show commands without executing

**Default rules:**
- Deny incoming traffic by default
- Allow outgoing traffic
- Allow SSH (port 22) with rate limiting
- Allow HTTP (port 80)
- Allow HTTPS (port 443)

#### allow
Allow a specific port.

```bash
nself prod firewall allow <port> [protocol]
```

**Example:**
```bash
nself prod firewall allow 8080
nself prod firewall allow 53 udp
```

#### block
Block a specific port.

```bash
nself prod firewall block <port>
```

#### recommendations
Show security recommendations.

```bash
nself prod firewall recommendations
```

### harden

Apply all security hardening measures.

```bash
nself prod harden [--dry-run] [--skip-firewall]
```

**Options:**
- `--dry-run` - Preview changes without applying
- `--skip-firewall` - Don't modify firewall settings

**Hardening steps:**
1. Generate secrets (if missing)
2. Apply production environment settings
3. Fix SSL key permissions
4. Check firewall status
5. Fix sensitive file permissions

## Production Checklist

Before deploying to production:

1. **Initialize production environment**
   ```bash
   nself prod init example.com --email admin@example.com
   ```

2. **Generate secrets**
   ```bash
   nself prod secrets generate
   ```

3. **Run security audit**
   ```bash
   nself prod check
   ```

4. **Configure SSL**
   ```bash
   nself prod ssl request example.com
   ```

5. **Configure firewall**
   ```bash
   nself prod firewall configure --dry-run
   nself prod firewall configure
   ```

6. **Apply hardening**
   ```bash
   nself prod harden
   ```

7. **Build and deploy**
   ```bash
   nself build
   nself deploy prod
   ```

## Security Best Practices

### Secrets
- Generate with `nself prod secrets generate`
- Never commit `.env.secrets` to git
- Rotate secrets periodically
- Use minimum 32 characters for passwords
- Use minimum 64 characters for JWT secrets

### SSL/TLS
- Use Let's Encrypt for production
- Monitor certificate expiry
- Set up automatic renewal
- Use HSTS headers

### Firewall
- Default deny incoming traffic
- Only expose necessary ports (22, 80, 443)
- Use rate limiting for SSH
- Consider fail2ban for brute-force protection

### Docker
- Don't run containers as root
- Use resource limits
- Avoid privileged containers
- Don't expose ports on 0.0.0.0

## Files Generated

| File | Description |
|------|-------------|
| `.env.secrets` | Production secrets (600 permissions) |
| `docker-compose.prod.yml` | Production compose override |
| `ssl/cert.pem` | SSL certificate |
| `ssl/key.pem` | SSL private key (600 permissions) |

## Related Commands

- [nself env](ENV.md) - Environment management
- [nself staging](STAGING.md) - Staging management
- [nself deploy](DEPLOY.md) - Deployment

## See Also

- [Security Guide](../guides/SECURITY.md)
- [Deployment Pipeline](../guides/DEPLOYMENT.md)
- [SSL Configuration](../configuration/SSL.md)
