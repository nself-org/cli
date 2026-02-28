# SSL Configuration

**Version 0.9.9** | SSL/TLS certificate configuration

---

## Overview

nself provides automatic SSL certificate management for both development and production environments.

---

## Development (mkcert)

For local development, nself uses [mkcert](https://github.com/FiloSottile/mkcert) to generate locally-trusted certificates.

### Setup

```bash
# Generate certificates
nself ssl generate

# Trust the root CA
nself trust
```

### Certificate Location

```
ssl/
├── cert.pem      # Certificate
├── key.pem       # Private key
└── ca.pem        # Root CA (optional)
```

### Supported Domains

Development certificates cover:
- `*.localhost`
- `*.local.nself.org`
- `*.{PROJECT_NAME}.localhost`

---

## Production (Let's Encrypt)

For production, use Let's Encrypt for free, auto-renewing certificates.

### Setup

```bash
# Configure Let's Encrypt
nself ssl setup --provider letsencrypt --email admin@example.com

# Generate certificate
nself ssl generate --production
```

### Auto-Renewal

Certificates are automatically renewed via cron:

```bash
# Check renewal status
nself ssl check --renewal

# Manual renewal
nself ssl renew
```

---

## Nginx Configuration

Generated SSL configuration:

```nginx
ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
ssl_prefer_server_ciphers on;
ssl_session_timeout 1d;
ssl_session_cache shared:SSL:10m;
ssl_session_tickets off;

# HSTS
add_header Strict-Transport-Security "max-age=63072000" always;
```

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SSL_PROVIDER` | Certificate provider | `mkcert` |
| `SSL_EMAIL` | Let's Encrypt email | - |
| `SSL_DOMAIN` | Primary domain | `BASE_DOMAIN` |
| `FORCE_HTTPS` | Redirect HTTP to HTTPS | `true` |

---

## Custom Certificates

To use your own certificates:

```bash
# Copy certificates
cp /path/to/cert.pem ssl/cert.pem
cp /path/to/key.pem ssl/key.pem

# Rebuild nginx config
nself build
```

---

## Troubleshooting

### Browser Shows Warning

```bash
# Re-trust the CA
nself trust

# Restart browser
```

### Let's Encrypt Rate Limits

```bash
# Use staging environment for testing
nself ssl generate --staging
```

### Certificate Expired

```bash
# Force renewal
nself ssl renew --force
```

---

## See Also

- [ssl command](../commands/SSL.md)
- [trust command](../commands/TRUST.md)
- [Security Guide](../guides/SECURITY.md)
