# Production Deployment Guide

Complete guide for deploying nself to production environments with best practices for security, performance, and reliability.

## 📚 Table of Contents

- [Pre-Deployment Checklist](#pre-deployment-checklist)
- [Server Requirements](#server-requirements)
- [Deployment Methods](#deployment-methods)
- [Initial Setup](#initial-setup)
- [Production Configuration](#production-configuration)
- [SSL/TLS Setup](#ssltls-setup)
- [Database Migration](#database-migration)
- [Monitoring Setup](#monitoring-setup)
- [Backup Strategy](#backup-strategy)
- [Security Hardening](#security-hardening)
- [Performance Optimization](#performance-optimization)
- [High Availability](#high-availability)
- [Maintenance](#maintenance)
- [Troubleshooting](#troubleshooting)

---

## Pre-Deployment Checklist

### Essential Tasks

- [ ] **Server provisioned** with adequate resources
- [ ] **Domain configured** with DNS pointing to server
- [ ] **SSL certificates** ready or Let's Encrypt configured
- [ ] **Firewall rules** configured
- [ ] **Backup strategy** defined and tested
- [ ] **Monitoring** tools configured
- [ ] **Secrets management** system in place
- [ ] **Load testing** completed
- [ ] **Security audit** performed
- [ ] **Rollback plan** documented

### Documentation Ready

- [ ] Deployment runbook
- [ ] Incident response plan
- [ ] Recovery procedures
- [ ] Contact list
- [ ] Architecture diagram

## Server Requirements

### Minimum Production Specs

| Component | Small | Medium | Large |
|-----------|-------|--------|-------|
| **CPU** | 4 cores | 8 cores | 16+ cores |
| **RAM** | 8GB | 16GB | 32GB+ |
| **Storage** | 100GB SSD | 250GB SSD | 500GB+ SSD |
| **Network** | 100Mbps | 1Gbps | 10Gbps |
| **OS** | Ubuntu 22.04 LTS | Ubuntu 22.04 LTS | Ubuntu 22.04 LTS |

### Estimated Capacity

| Server Size | Concurrent Users | Requests/sec | Database Size |
|-------------|-----------------|--------------|---------------|
| **Small** | 100-500 | 1,000 | < 10GB |
| **Medium** | 500-2,000 | 5,000 | < 100GB |
| **Large** | 2,000-10,000 | 20,000 | < 1TB |

## Deployment Methods

### Method 1: Direct Server Deployment

Best for: Single server, simple setup

```bash
# On your server
ssh user@production-server.com

# Clone nself
git clone https://github.com/nself-org/cli.git
cd nself

# Create production directory
mkdir -p /opt/myapp
cd /opt/myapp

# Initialize production config
nself init
nself prod

# Edit production settings
nano .env.prod

# Deploy
ENV=prod nself build
ENV=prod nself start
```

### Method 2: Docker Swarm

Best for: Multi-node, high availability

```bash
# Initialize swarm
docker swarm init --advertise-addr <manager-ip>

# Deploy stack
docker stack deploy -c docker-compose.yml myapp

# Scale services
docker service scale myapp_api=3
```

### Method 3: Kubernetes

Best for: Enterprise, complex orchestration

```yaml
# k8s-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nself-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nself
  template:
    metadata:
      labels:
        app: nself
    spec:
      containers:
      - name: postgres
        image: postgres:16-alpine
        env:
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: nself-secrets
              key: postgres-password
```

### Method 4: Automated CI/CD

Best for: Continuous deployment

```yaml
# .github/workflows/deploy.yml
name: Deploy to Production
on:
  push:
    branches: [main]
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v2
    - name: Deploy to server
      run: |
        ssh ${{ secrets.SERVER }} "cd /opt/myapp && git pull && nself restart"
```

## Initial Setup

### 1. Server Preparation

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com | bash
sudo usermod -aG docker $USER

# Install Docker Compose
sudo apt install docker-compose-plugin -y

# Configure firewall
sudo ufw allow 22/tcp   # SSH
sudo ufw allow 80/tcp   # HTTP
sudo ufw allow 443/tcp  # HTTPS
sudo ufw enable
```

### 2. Create Deployment User

```bash
# Create dedicated user
sudo useradd -m -s /bin/bash nself
sudo usermod -aG docker nself

# Set up SSH key
sudo su - nself
ssh-keygen -t ed25519 -C "nself@production"
```

### 3. Directory Structure

```bash
/opt/myapp/
├── .env.prod          # Production config
├── .env.secrets       # Sensitive data
├── backups/           # Backup directory
├── logs/              # Application logs
├── ssl/               # SSL certificates
└── data/              # Persistent data
```

## Production Configuration

### Generate Production Config

```bash
# Generate production template
nself prod

# This creates:
# - .env.prod with production defaults
# - docker-compose.prod.yml
# - nginx/production.conf
```

### Essential Production Settings

```bash
# .env.prod
ENV=prod
PROJECT_NAME=myapp-prod
BASE_DOMAIN=api.myapp.com

# Security
SSL_ENABLED=true
SSL_PROVIDER=letsencrypt
LETSENCRYPT_EMAIL=admin@myapp.com
NGINX_FORCE_HTTPS=true

# Database
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}  # From .env.secrets
POSTGRES_MAX_CONNECTIONS=200
POSTGRES_SHARED_BUFFERS=1GB

# Services
HASURA_GRAPHQL_DEV_MODE=false
HASURA_GRAPHQL_ENABLE_CONSOLE=false
AUTH_SESSION_SECURE=true
AUTH_COOKIE_SECURE=true

# Performance
REDIS_ENABLED=true
NGINX_CACHE_ENABLED=true

# Monitoring
PROMETHEUS_ENABLED=true
GRAFANA_ENABLED=true
```

### Secrets Management

```bash
# .env.secrets (NEVER commit!)
POSTGRES_PASSWORD=$(openssl rand -base64 32)
HASURA_GRAPHQL_ADMIN_SECRET=$(openssl rand -base64 32)
AUTH_JWT_SECRET=$(openssl rand -base64 64)
MINIO_ROOT_PASSWORD=$(openssl rand -base64 32)
ADMIN_PASSWORD=$(openssl rand -base64 24)

# External services
SENDGRID_API_KEY=SG.xxx
STRIPE_SECRET_KEY=sk_live_xxx
AWS_SECRET_ACCESS_KEY=xxx
```

## SSL/TLS Setup

### Let's Encrypt (Recommended)

```bash
# Configure Let's Encrypt
SSL_ENABLED=true
SSL_PROVIDER=letsencrypt
LETSENCRYPT_EMAIL=admin@myapp.com
LETSENCRYPT_STAGING=false  # Use true for testing

# Generate certificates
nself ssl --production

# Auto-renewal (add to crontab)
0 0 * * * /opt/myapp/nself ssl --renew
```

### Custom Certificates

```bash
# Place certificates
cp /path/to/cert.pem /opt/myapp/ssl/
cp /path/to/key.pem /opt/myapp/ssl/
cp /path/to/ca.pem /opt/myapp/ssl/

# Configure
SSL_ENABLED=true
SSL_PROVIDER=custom
SSL_CERT_PATH=/ssl/cert.pem
SSL_KEY_PATH=/ssl/key.pem
SSL_CA_PATH=/ssl/ca.pem
```

### Nginx SSL Configuration

```nginx
server {
    listen 443 ssl http2;
    server_name api.myapp.com;
    
    ssl_certificate /ssl/cert.pem;
    ssl_certificate_key /ssl/key.pem;
    
    # Security headers
    add_header Strict-Transport-Security "max-age=31536000" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    
    # SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
}
```

## Database Migration

### Backup Existing Data

```bash
# Backup current database
pg_dump -h old-server -U postgres myapp > backup.sql

# Or use nself backup
nself backup create pre-migration
```

### Migrate Data

```bash
# Restore to new database
psql -h localhost -U postgres myapp < backup.sql

# Run migrations
nself db migrate

# Verify data
nself db console
```

### Connection String Update

```bash
# Update Hasura connection
HASURA_GRAPHQL_DATABASE_URL=postgres://user:pass@localhost:5432/myapp

# Update Auth service
AUTH_DATABASE_URL=postgres://user:pass@localhost:5432/myapp
```

## Monitoring Setup

### Prometheus & Grafana

```bash
# Enable monitoring
PROMETHEUS_ENABLED=true
GRAFANA_ENABLED=true
LOKI_ENABLED=true

# Access Grafana
# http://monitoring.myapp.com
# Default: admin/admin
```

### Health Checks

```bash
# Configure health endpoints
HEALTH_CHECK_ENABLED=true
HEALTH_CHECK_PATH=/health
HEALTH_CHECK_INTERVAL=30s
```

### Alerts Configuration

```yaml
# prometheus/alerts.yml
groups:
- name: nself
  rules:
  - alert: ServiceDown
    expr: up == 0
    for: 5m
    annotations:
      summary: "Service {{ $labels.job }} is down"
      
  - alert: HighMemoryUsage
    expr: memory_usage > 90
    for: 10m
    annotations:
      summary: "High memory usage on {{ $labels.instance }}"
```

### External Monitoring

```bash
# StatusPage.io
curl -X POST https://api.statuspage.io/v1/components \
  -H "Authorization: OAuth $TOKEN" \
  -d '{"component": {"name": "API"}}'

# UptimeRobot
curl -X POST https://api.uptimerobot.com/v2/newMonitor \
  -d "api_key=$KEY" \
  -d "friendly_name=nself API" \
  -d "url=https://api.myapp.com/health"
```

## Backup Strategy

### Automated Backups

```bash
# Schedule daily backups
nself backup schedule --daily --retain 30

# Configure S3 backup
BACKUP_PROVIDER=s3
BACKUP_S3_BUCKET=myapp-backups
BACKUP_S3_REGION=us-east-1
AWS_ACCESS_KEY_ID=xxx
AWS_SECRET_ACCESS_KEY=xxx

# Test backup
nself backup create test-backup
nself backup verify test-backup
```

### Backup Script

```bash
#!/bin/bash
# /opt/myapp/scripts/backup.sh

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/opt/myapp/backups"

# Database backup
pg_dump -h localhost -U postgres myapp | gzip > $BACKUP_DIR/db_$DATE.sql.gz

# Files backup
tar -czf $BACKUP_DIR/files_$DATE.tar.gz /opt/myapp/data

# Upload to S3
aws s3 cp $BACKUP_DIR/db_$DATE.sql.gz s3://myapp-backups/
aws s3 cp $BACKUP_DIR/files_$DATE.tar.gz s3://myapp-backups/

# Clean old backups
find $BACKUP_DIR -name "*.gz" -mtime +30 -delete
```

### Disaster Recovery

```bash
# Recovery procedure
1. Provision new server
2. Install nself
3. Restore configuration
   scp backup-server:/backups/.env.prod .
   
4. Restore database
   nself backup restore latest
   
5. Start services
   ENV=prod nself start
   
6. Verify
   nself status
   nself doctor
```

## Security Hardening

### Firewall Configuration

```bash
# UFW rules
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp    # SSH
sudo ufw allow 80/tcp    # HTTP
sudo ufw allow 443/tcp   # HTTPS
sudo ufw allow from 10.0.0.0/8 to any port 5432  # PostgreSQL (internal)
sudo ufw enable
```

### Fail2ban Setup

```bash
# Install fail2ban
sudo apt install fail2ban -y

# Configure for nself
cat > /etc/fail2ban/jail.local <<EOF
[nself-auth]
enabled = true
port = 4000
filter = nself-auth
logpath = /opt/myapp/logs/auth.log
maxretry = 5
bantime = 3600
EOF
```

### Security Headers

```nginx
# nginx/security.conf
add_header X-Frame-Options "SAMEORIGIN" always;
add_header X-Content-Type-Options "nosniff" always;
add_header X-XSS-Protection "1; mode=block" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
add_header Content-Security-Policy "default-src 'self' https:; script-src 'self' 'unsafe-inline' https:; style-src 'self' 'unsafe-inline' https:;" always;
```

### Database Security

```sql
-- Restrict connections
ALTER DATABASE myapp CONNECTION LIMIT 100;

-- Create read-only user
CREATE USER readonly WITH PASSWORD 'secure_password';
GRANT CONNECT ON DATABASE myapp TO readonly;
GRANT USAGE ON SCHEMA public TO readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO readonly;

-- Enable SSL
ALTER SYSTEM SET ssl = on;
```

## Performance Optimization

### System Tuning

```bash
# /etc/sysctl.conf
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 8192
net.ipv4.ip_local_port_range = 1024 65535
vm.swappiness = 10
```

### Docker Optimization

```json
// /etc/docker/daemon.json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  },
  "storage-driver": "overlay2",
  "storage-opts": [
    "overlay2.override_kernel_check=true"
  ]
}
```

### PostgreSQL Tuning

```bash
# postgresql.conf
shared_buffers = 2GB              # 25% of RAM
effective_cache_size = 6GB        # 75% of RAM
maintenance_work_mem = 512MB
checkpoint_completion_target = 0.9
wal_buffers = 16MB
default_statistics_target = 100
random_page_cost = 1.1
```

### CDN Configuration

```bash
# CloudFlare
CLOUDFLARE_ENABLED=true
CLOUDFLARE_ZONE_ID=xxx
CLOUDFLARE_API_TOKEN=xxx

# Custom CDN
CDN_ENABLED=true
CDN_URL=https://cdn.myapp.com
CDN_PULL_ZONE=myapp
```

## High Availability

### Load Balancing

```nginx
# nginx/upstream.conf
upstream backend {
    least_conn;
    server backend1.myapp.com:8080 weight=3;
    server backend2.myapp.com:8080 weight=2;
    server backend3.myapp.com:8080 backup;
}
```

### Database Replication

```bash
# Primary server
POSTGRES_REPLICATION_MODE=master
POSTGRES_REPLICATION_USER=replicator
POSTGRES_REPLICATION_PASSWORD=xxx

# Replica server
POSTGRES_REPLICATION_MODE=slave
POSTGRES_MASTER_HOST=primary.myapp.com
POSTGRES_MASTER_PORT=5432
```

### Auto-Scaling

```yaml
# docker-compose.yml
services:
  api:
    deploy:
      replicas: 3
      update_config:
        parallelism: 1
        delay: 10s
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 3
```

## Maintenance

### Zero-Downtime Updates

```bash
#!/bin/bash
# rolling-update.sh

# Pull latest changes
git pull

# Build new images
ENV=prod nself build

# Rolling restart
for service in api worker websocket; do
  docker-compose up -d --no-deps --build $service
  sleep 30
  # Health check
  curl -f http://localhost/health || exit 1
done
```

### Maintenance Mode

```nginx
# Enable maintenance mode
location / {
    if (-f /opt/myapp/maintenance.flag) {
        return 503;
    }
    proxy_pass http://backend;
}

error_page 503 @maintenance;
location @maintenance {
    root /opt/myapp/static;
    rewrite ^.*$ /maintenance.html break;
}
```

### Database Maintenance

```bash
# Weekly vacuum
0 2 * * 0 psql -U postgres -d myapp -c "VACUUM ANALYZE;"

# Monthly reindex
0 3 1 * * psql -U postgres -d myapp -c "REINDEX DATABASE myapp;"

# Backup before maintenance
nself backup create pre-maintenance
```

## Troubleshooting

### Common Production Issues

#### High Memory Usage
```bash
# Check memory
docker stats

# Limit container memory
POSTGRES_MEMORY_LIMIT=2G
HASURA_MEMORY_LIMIT=1G

# Clear caches
sync && echo 3 > /proc/sys/vm/drop_caches
```

#### Connection Issues
```bash
# Check connections
netstat -an | grep ESTABLISHED | wc -l

# Increase limits
POSTGRES_MAX_CONNECTIONS=500
HASURA_GRAPHQL_CONNECTION_POOL_SIZE=100
```

#### Slow Queries
```sql
-- Find slow queries
SELECT query, calls, mean_time
FROM pg_stat_statements
ORDER BY mean_time DESC
LIMIT 10;

-- Add indexes
CREATE INDEX CONCURRENTLY idx_users_email ON users(email);
```

### Emergency Procedures

```bash
# Emergency stop
docker-compose down

# Force restart
docker stop $(docker ps -q)
docker system prune -af
ENV=prod nself start

# Rollback
git checkout <last-known-good>
ENV=prod nself build
ENV=prod nself restart
```

### Debug Production

```bash
# Enable debug logging
LOG_LEVEL=debug
DEBUG=true

# Check logs
nself logs > debug.log
docker logs <container> --tail 1000

# System diagnostics
nself doctor
htop
iotop
netstat -tulpn
```

## Post-Deployment

### Verification Checklist

- [ ] All services running: `nself status`
- [ ] SSL working: `curl -I https://api.myapp.com`
- [ ] Database accessible: `nself db console`
- [ ] Backups working: `nself backup create test`
- [ ] Monitoring active: Check Grafana
- [ ] Logs flowing: `nself logs`
- [ ] Health checks passing: `curl https://api.myapp.com/health`

### Performance Testing

```bash
# Load testing with k6
k6 run --vus 100 --duration 30s loadtest.js

# Stress testing
ab -n 10000 -c 100 https://api.myapp.com/

# Database performance
pgbench -c 10 -j 2 -t 1000 myapp
```

---

**Next Steps:**
- [Monitoring Guide](MONITORING-COMPLETE.md) - Set up comprehensive monitoring
- [Security Guide](SECURITY.md) - Additional security measures
- [Backup Guide](BACKUP_GUIDE.md) - Detailed backup strategies
- [Scaling Guide](../deployment/PRODUCTION-DEPLOYMENT.md) - Scale your deployment