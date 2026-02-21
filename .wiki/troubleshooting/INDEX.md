# Troubleshooting Documentation

Solutions for common issues and problems.

## Overview

This section provides troubleshooting guides for specific features and common issues.

## Feature-Specific Troubleshooting

- **[Billing Troubleshooting](BILLING-TROUBLESHOOTING.md)** - Billing and subscription issues
- **[White-Label Troubleshooting](WHITE-LABEL-TROUBLESHOOTING.md)** - Branding and customization issues

## General Troubleshooting

See **[Main Troubleshooting Guide](../guides/TROUBLESHOOTING.md)** for comprehensive troubleshooting.

## Common Issues

### Services Won't Start

**Problem:** Services fail to start or immediately exit

**Solutions:**
```bash
# Check service status
nself status

# View logs
nself logs [service-name]

# Run diagnostics
nself doctor

# Check for port conflicts
lsof -i :5432  # PostgreSQL
lsof -i :8080  # Hasura
lsof -i :4000  # Auth

# Restart services
nself stop && nself start
```

### Database Connection Issues

**Problem:** Cannot connect to database

**Solutions:**
```bash
# Check PostgreSQL is running
nself status postgres

# View PostgreSQL logs
nself logs postgres

# Verify connection details
cat .env | grep POSTGRES

# Test connection
nself exec postgres psql -U postgres -d myapp
```

### Build Failures

**Problem:** `nself build` fails

**Solutions:**
```bash
# Check for syntax errors in .env
nself config validate

# Remove generated files and rebuild
rm -rf docker-compose.yml nginx/
nself build

# Check permissions
ls -la services/

# View build errors
nself build --verbose
```

### Permission Errors

**Problem:** Permission denied errors

**Solutions:**
```bash
# Fix .env permissions
chmod 600 .env

# Fix service directories
chmod 755 services/
chmod 644 services/**/Dockerfile

# Fix SSL certificates
chmod 600 ssl/*.pem

# Check Docker permissions
sudo usermod -aG docker $USER
```

### Port Conflicts

**Problem:** Address already in use

**Solutions:**
```bash
# Find what's using the port
lsof -i :PORT
netstat -an | grep PORT

# Kill the process
kill -9 PID

# Or change the port in .env
POSTGRES_PORT=5433
HASURA_PORT=8081
```

### Memory Issues

**Problem:** Out of memory errors

**Solutions:**
```bash
# Check Docker memory allocation
docker stats

# Increase Docker memory (Docker Desktop)
# Preferences → Resources → Memory → 8GB+

# Disable unused services
MONITORING_ENABLED=false
REDIS_ENABLED=false

# Restart Docker
```

### SSL Certificate Issues

**Problem:** SSL/HTTPS not working

**Solutions:**
```bash
# Regenerate self-signed certificates
nself auth ssl setup

# Trust certificate
nself auth ssl trust add local.nself.org

# Check certificate
openssl x509 -in ssl/cert.pem -text -noout

# Force HTTPS
SSL_ENABLED=true
```

## Billing Troubleshooting

See **[Billing Troubleshooting](BILLING-TROUBLESHOOTING.md)** for:
- Subscription issues
- Usage tracking problems
- Invoice generation errors
- Payment webhook failures

## White-Label Troubleshooting

See **[White-Label Troubleshooting](WHITE-LABEL-TROUBLESHOOTING.md)** for:
- Branding not applying
- Theme issues
- Custom domain problems
- Email template errors

## Platform-Specific Issues

### macOS

**Docker Desktop Issues:**
```bash
# Reset Docker
# Docker Desktop → Troubleshoot → Reset to factory defaults

# Update Docker Desktop
# Download latest from docker.com

# Check resource allocation
# Preferences → Resources → increase CPU/Memory
```

**Homebrew Issues:**
```bash
# Update Homebrew
brew update

# Reinstall nself
brew uninstall nself
brew install nself-org/nself/nself

# Check version
nself version
```

### Linux

**Docker Installation:**
```bash
# Install Docker
curl -fsSL https://get.docker.com | sh

# Add user to docker group
sudo usermod -aG docker $USER

# Enable Docker service
sudo systemctl enable docker
sudo systemctl start docker
```

**Permission Issues:**
```bash
# Fix Docker socket permissions
sudo chmod 666 /var/run/docker.sock

# Or add user to docker group (logout required)
sudo usermod -aG docker $USER
```

### WSL (Windows)

**Docker Integration:**
```bash
# Enable WSL integration
# Docker Desktop → Settings → Resources → WSL Integration

# Install in WSL
curl -sSL https://install.nself.org | bash

# Check Docker
docker ps
```

**Performance:**
```bash
# Move project to WSL filesystem (faster)
# Not: /mnt/c/...
# Use: ~/projects/...
```

## Diagnostic Commands

### System Diagnostics

```bash
# Comprehensive diagnostics
nself doctor

# Specific checks
nself doctor --check docker
nself doctor --check permissions
nself doctor --check ports
nself doctor --check environment
```

### Health Checks

```bash
# All services
nself health

# Specific service
nself health postgres
nself health hasura

# With details
nself health --verbose
```

### Log Viewing

```bash
# All logs
nself logs

# Specific service
nself logs postgres
nself logs hasura
nself logs auth

# Follow logs
nself logs --follow

# Last N lines
nself logs --tail 100
```

### Status Checks

```bash
# Service status
nself status

# Detailed status
nself status --verbose

# JSON output
nself status --json
```

## Getting Help

### Documentation

- [Main Troubleshooting Guide](../guides/TROUBLESHOOTING.md)
- [FAQ](../getting-started/FAQ.md)
- [Command Reference](../commands/COMMANDS.md)

### Community Support

- [GitHub Discussions](https://github.com/nself-org/cli/discussions)
- [GitHub Issues](https://github.com/nself-org/cli/issues)
- [Discord Community](https://discord.gg/nself)

### Reporting Bugs

When reporting issues, include:

```bash
# System information
nself version
docker --version
docker-compose --version
uname -a

# Configuration
cat .env (remove secrets!)

# Logs
nself logs > logs.txt

# Status
nself status --verbose > status.txt
```

Create issue at: [github.com/nself-org/cli/issues](https://github.com/nself-org/cli/issues)

---

**[← Back to Documentation Home](../README.md)**
