# nself ssl - SSL Certificate Management

> **⚠️ DEPRECATED**: `nself ssl` is deprecated and will be removed in v1.0.0.
> Please use `nself auth ssl` instead.
> Run `nself auth ssl --help` for full usage information.

**Version 0.9.9** | Manage SSL/TLS certificates

---

## Overview

The `nself ssl` command manages SSL certificates for your nself services. It supports automatic generation of development certificates using mkcert, as well as production certificate management with Let's Encrypt.

---

## Basic Usage

```bash
# Generate development certificates
nself ssl generate

# Check certificate status
nself ssl check

# Renew certificates
nself ssl renew

# Bootstrap SSL setup
nself ssl bootstrap
```

---

## Development Certificates

For local development, nself uses [mkcert](https://github.com/FiloSottile/mkcert) to generate locally-trusted certificates.

### Generate Certificates

```bash
# Generate for configured domain
nself ssl generate

# Generate for specific domain
nself ssl generate --domain myapp.localhost
```

### Trust Certificates

```bash
# Install root CA to system trust store
nself trust
```

---

## Production Certificates

For production, nself supports Let's Encrypt via Certbot.

### Setup Let's Encrypt

```bash
# Configure Let's Encrypt
nself ssl setup --provider letsencrypt

# Generate production cert
nself ssl generate --production
```

### Auto-Renewal

Certificates are automatically renewed via cron job. Check status:

```bash
nself ssl check --renewal
```

---

## Options Reference

| Option | Description |
|--------|-------------|
| `generate` | Generate new certificates |
| `check` | Check certificate status |
| `renew` | Renew certificates |
| `bootstrap` | Initial SSL setup |
| `--domain` | Specify domain |
| `--production` | Use production (Let's Encrypt) |
| `--provider` | Certificate provider |

---

## Certificate Locations

| Environment | Location |
|-------------|----------|
| Development | `ssl/cert.pem`, `ssl/key.pem` |
| Production | `/etc/letsencrypt/live/domain/` |

---

## See Also

- [trust](TRUST.md) - Install root CA
- [deploy](DEPLOY.md) - Production deployment
- [build](BUILD.md) - Generate configuration
