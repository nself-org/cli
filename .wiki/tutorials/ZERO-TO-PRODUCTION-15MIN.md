# Zero to Production in 15 Minutes

Deploy a complete full-stack application from zero to production in just 15 minutes using nself.

**Time Required:** 15 minutes
**Difficulty:** Beginner
**Cost:** ~$5/month (DigitalOcean droplet)

---

## What You'll Deploy

By the end of this tutorial, you'll have:

- ✅ PostgreSQL database
- ✅ Hasura GraphQL API
- ✅ Authentication service (JWT-based)
- ✅ SSL certificate (Let's Encrypt)
- ✅ Nginx reverse proxy
- ✅ Automated backups
- ✅ Monitoring dashboards
- ✅ Production-ready infrastructure

All running on a single VPS with professional-grade configuration.

---

## Prerequisites

- A domain name (or use a free subdomain from afraid.org)
- A server (DigitalOcean, Linode, Vultr, or any VPS)
- 5 minutes to wait for DNS propagation
- Basic command-line knowledge

---

## Timeline

| Min | Task |
|-----|------|
| 0-2 | Install nself and initialize project |
| 2-4 | Configure environment |
| 4-6 | Provision server |
| 6-10 | Deploy to server |
| 10-12 | Configure SSL |
| 12-14 | Setup monitoring |
| 14-15 | Verify everything works |

Let's go!

---

## Step 1: Install nself (2 minutes)

### On Your Local Machine

**macOS:**
```bash
brew install acamarata/nself/nself
```

**Linux:**
```bash
curl -sSL https://install.nself.org | bash
```

**Windows (WSL):**
```bash
# In WSL terminal
curl -sSL https://install.nself.org | bash
```

**Verify installation:**
```bash
nself version
# Should output: nself v0.9.9
```

---

## Step 2: Initialize Project (2 minutes)

```bash
# Create project directory
mkdir my-production-app
cd my-production-app

# Initialize with demo configuration
nself init --demo

# This creates:
# - .env file with sensible defaults
# - Project structure
# - Sample configuration
```

**What just happened?**
- Created `.env` with 25 services configured
- All optional services enabled (Redis, MinIO, monitoring, etc.)
- Development settings applied

---

## Step 3: Configure for Production (2 minutes)

Edit `.env` file:

```bash
nano .env
```

**Change these critical values:**

```bash
# Project Info
PROJECT_NAME=my-app
ENV=production
BASE_DOMAIN=yourdomain.com  # ← Your domain!

# Security (GENERATE NEW VALUES!)
POSTGRES_PASSWORD=$(openssl rand -base64 32)
HASURA_GRAPHQL_ADMIN_SECRET=$(openssl rand -base64 32)
AUTH_JWT_SECRET=$(openssl rand -base64 32)

# SSL
SSL_ENABLED=true
SSL_EMAIL=your-email@example.com
SSL_DOMAIN=yourdomain.com

# Backups
BACKUP_ENABLED=true
BACKUP_SCHEDULE="0 2 * * *"  # Daily at 2 AM
BACKUP_RETENTION_DAYS=30

# Monitoring
MONITORING_ENABLED=true
```

**Quick secret generation:**
```bash
# Generate all secrets at once
cat << 'EOF' >> .env

# Production Secrets (Generated)
POSTGRES_PASSWORD=$(openssl rand -base64 32)
HASURA_GRAPHQL_ADMIN_SECRET=$(openssl rand -base64 32)
AUTH_JWT_SECRET=$(openssl rand -base64 32)
MINIO_ROOT_PASSWORD=$(openssl rand -base64 32)
REDIS_PASSWORD=$(openssl rand -base64 32)
EOF
```

---

## Step 4: Provision Server (2 minutes)

### Option A: Create Server Manually

1. **DigitalOcean:**
   - Go to https://digitalocean.com
   - Create Droplet
   - Ubuntu 22.04 LTS
   - Basic Plan: $6/month
   - Add your SSH key

2. **Point DNS:**
   - Add A record: `@` → your server IP
   - Add A record: `*.yourdomain.com` → your server IP
   - Wait 1-5 minutes for propagation

### Option B: Automated Provisioning

```bash
# Using nself deploy (supports DigitalOcean, Vultr, Linode)
nself infra provider create \
  --provider digitalocean \
  --token YOUR_DO_TOKEN \
  --region nyc3 \
  --size s-1vcpu-2gb \
  --domain yourdomain.com

# This creates the server AND configures DNS automatically
```

**Verify DNS:**
```bash
nslookup yourdomain.com
# Should return your server IP
```

---

## Step 5: Deploy to Server (4 minutes)

### Configure Server Connection

Create `server.json`:

```json
{
  "production": {
    "host": "yourdomain.com",
    "user": "root",
    "port": 22,
    "privateKey": "~/.ssh/id_rsa"
  }
}
```

### Deploy

```bash
# Build infrastructure locally first (verify it works)
nself build
nself start
nself status

# All good? Deploy to production!
nself deploy push production

# This will:
# 1. SSH to server
# 2. Install Docker
# 3. Copy project files
# 4. Build services
# 5. Start everything
```

**Watch deployment progress:**
```bash
# In another terminal
nself deploy logs production --follow
```

**Deployment takes ~3-4 minutes on first run.**

---

## Step 6: Configure SSL (2 minutes)

```bash
# Automatic SSL with Let's Encrypt
nself deploy exec production "nself auth ssl cert --domain yourdomain.com --email your-email@example.com"

# This will:
# 1. Stop nginx
# 2. Request certificate from Let's Encrypt
# 3. Install certificate
# 4. Configure auto-renewal
# 5. Restart nginx with HTTPS
```

**Verify SSL:**
```bash
curl -I https://yourdomain.com
# Should return: HTTP/2 200

# Check certificate
nself deploy exec production "nself auth ssl status"
```

---

## Step 7: Setup Monitoring (2 minutes)

Monitoring was already enabled in `.env` (MONITORING_ENABLED=true).

**Access dashboards:**

```bash
# Get all service URLs
nself deploy exec production "nself urls"

# Output includes:
# ✓ Grafana:       https://grafana.yourdomain.com
# ✓ Prometheus:    https://prometheus.yourdomain.com
# ✓ Hasura:        https://api.yourdomain.com
```

**Login to Grafana:**
1. Open `https://grafana.yourdomain.com`
2. Username: `admin`
3. Password: (from `.env` GRAFANA_ADMIN_PASSWORD, or default: `admin`)
4. View pre-configured dashboards:
   - System Overview
   - PostgreSQL Metrics
   - Container Metrics
   - Application Logs

---

## Step 8: Verify Everything (1 minute)

### Health Checks

```bash
# Check all services
nself deploy exec production "nself health"

# Expected output:
# ✓ postgres       (healthy)
# ✓ hasura         (healthy)
# ✓ auth           (healthy)
# ✓ nginx          (healthy)
# ✓ redis          (healthy)
# ✓ minio          (healthy)
# ✓ prometheus     (healthy)
# ✓ grafana        (healthy)
# ... (all 25 services)
```

### Test API

```bash
# Test GraphQL endpoint
curl https://api.yourdomain.com/healthz
# Should return: {"status":"ok"}

# Test Auth endpoint
curl https://auth.yourdomain.com/healthz
# Should return: {"status":"ok"}
```

### Test Frontend

```bash
# Visit your domain
curl -I https://yourdomain.com
# Should return: HTTP/2 200
```

---

## Step 9: Production Checklist

Before going live, verify:

### Security

- [ ] Changed all default passwords
- [ ] SSL certificate installed and working
- [ ] Firewall configured (only ports 80, 443, 22 open)
- [ ] Database not exposed to internet
- [ ] Admin secrets are strong (32+ characters)

```bash
# Check security
nself deploy exec production "nself auth security audit"
```

### Backups

- [ ] Automated backups enabled
- [ ] Backup schedule configured
- [ ] Test restore process

```bash
# Verify backups
nself deploy exec production "nself backup list"

# Test backup
nself deploy exec production "nself backup create test-backup"

# Test restore
nself deploy exec production "nself backup restore test-backup --dry-run"
```

### Monitoring

- [ ] Grafana dashboards accessible
- [ ] Alerts configured
- [ ] Log retention set
- [ ] Metrics being collected

```bash
# Verify monitoring
nself deploy exec production "nself monitor status"
```

### Performance

- [ ] Database indexed
- [ ] Caching enabled (Redis)
- [ ] CDN configured (optional)
- [ ] Connection pooling enabled

```bash
# Check performance
nself deploy exec production "nself perf bench"
```

---

## You're Live!

Congratulations! You just deployed a production-ready full-stack application in 15 minutes.

### What You Have Now

- **Database:** PostgreSQL with automated backups
- **API:** Hasura GraphQL with JWT authentication
- **Auth:** nHost authentication service
- **SSL:** Let's Encrypt certificates with auto-renewal
- **Monitoring:** Grafana + Prometheus + Loki
- **Storage:** MinIO S3-compatible object storage
- **Cache:** Redis for sessions and caching
- **Email:** MailPit (replace with real SMTP for production emails)

### Access Your Services

All services are available at subdomains:

| Service | URL | Purpose |
|---------|-----|---------|
| **App** | https://yourdomain.com | Your application |
| **API** | https://api.yourdomain.com | GraphQL API |
| **Auth** | https://auth.yourdomain.com | Authentication |
| **Admin** | https://admin.yourdomain.com | Management UI |
| **Grafana** | https://grafana.yourdomain.com | Monitoring |
| **MinIO** | https://minio.yourdomain.com | Storage console |

---

## Next Steps

### 1. Build Your Application

```bash
# Add database tables
nself deploy exec production "nself db migrate create init"

# Configure Hasura metadata
nself deploy exec production "nself admin hasura"
```

### 2. Setup Continuous Deployment

```bash
# Configure GitHub Actions
nself dev ci init --provider github

# Now git push automatically deploys to production!
```

### 3. Scale Your Infrastructure

```bash
# Add more servers
nself deploy provision --server worker1.yourdomain.com

# Setup load balancing
nself infra lb add --servers "app1.yourdomain.com,app2.yourdomain.com"
```

### 4. Add Custom Services

```bash
# Add a Node.js API
echo "CS_1=api:express-js:8001" >> .env

# Add a Python worker
echo "CS_2=worker:python-api:8002" >> .env

# Rebuild and deploy
nself build
nself deploy push production
```

---

## Maintenance

### Daily Tasks

```bash
# Check health
nself deploy exec production "nself health"

# View logs
nself deploy exec production "nself logs --tail 100"
```

### Weekly Tasks

```bash
# Check backups
nself deploy exec production "nself backup list"

# Review metrics
# Visit: https://grafana.yourdomain.com

# Update services
nself deploy exec production "nself update"
```

### Monthly Tasks

```bash
# Review security audit
nself deploy exec production "nself auth security audit"

# Check disk usage
nself deploy exec production "df -h"

# Review logs for errors
nself deploy exec production "nself logs --grep ERROR --since 30d"
```

---

## Troubleshooting

### Services Won't Start

```bash
# Check logs
nself deploy exec production "nself logs --verbose"

# Restart services
nself deploy exec production "nself restart"

# Check disk space
nself deploy exec production "df -h"
```

### SSL Issues

```bash
# Verify certificate
nself deploy exec production "nself auth ssl status"

# Renew manually
nself deploy exec production "nself auth ssl renew"

# Check nginx config
nself deploy exec production "nself config validate"
```

### Database Issues

```bash
# Check database status
nself deploy exec production "nself db status"

# View database logs
nself deploy exec production "nself logs postgres"

# Test connection
nself deploy exec production "nself db query 'SELECT 1;'"
```

### Performance Issues

```bash
# Run diagnostics
nself deploy exec production "nself doctor"

# Check metrics
# Visit: https://grafana.yourdomain.com

# Analyze slow queries
nself deploy exec production "nself db slow-queries"
```

---

## Costs

### Minimal Setup (~$10/month)

- DigitalOcean Droplet (2GB RAM): $12/month
- Domain name: ~$12/year (~$1/month)
- SSL: Free (Let's Encrypt)
- **Total: ~$13/month**

### Recommended Setup (~$25/month)

- DigitalOcean Droplet (4GB RAM): $24/month
- Domain name: ~$12/year (~$1/month)
- Backups (DigitalOcean): $4.80/month (20% of droplet cost)
- **Total: ~$30/month**

### Production Setup (~$100/month)

- DigitalOcean Droplets (2x 8GB RAM): $96/month
- Load Balancer: $12/month
- Domain name: ~$1/month
- Backups: $19.20/month
- CDN (optional): ~$5-20/month
- **Total: ~$130/month**

---

## Resources

- **Full Documentation:** [docs.nself.org](https://github.com/acamarata/nself/wiki)
- **Example Projects:** [examples/](../examples/README.md)
- **Video Tutorials:** [YouTube Channel](https://youtube.com/@nself)
- **Community:** [GitHub Discussions](https://github.com/acamarata/nself/discussions)

---

## Get Help

- **Issues:** [GitHub Issues](https://github.com/acamarata/nself/issues)
- **Discord:** [Join Community](https://discord.gg/nself)
- **Email:** support@nself.org

---

**Congratulations!** You're now running a production application with professional infrastructure.

Welcome to nself. Let's build something amazing.

---

**Version:** 0.9.8
**Last Updated:** January 2026
**Estimated Reading Time:** 10 minutes
**Actual Deployment Time:** 15 minutes
