# Complete Troubleshooting Guide

**Version 0.9.9** | Symptom → Diagnosis → Solution

---

## Table of Contents

1. [Quick Diagnostic Commands](#quick-diagnostic-commands)
2. [Services Won't Start](#services-wont-start)
3. [Database Issues](#database-issues)
4. [Authentication Problems](#authentication-problems)
5. [API/GraphQL Errors](#apigraphql-errors)
6. [Performance Issues](#performance-issues)
7. [Storage Problems](#storage-problems)
8. [Network/Connectivity Issues](#networkconnectivity-issues)
9. [SSL/Certificate Errors](#sslcertificate-errors)
10. [Build/Configuration Errors](#buildconfiguration-errors)
11. [Monitoring Not Working](#monitoring-not-working)
12. [Multi-Tenancy Issues](#multi-tenancy-issues)

---

## Quick Diagnostic Commands

**Run these first to gather information:**

```bash
# 1. Overall system status
nself status

# 2. Check for errors in logs
nself logs --level error --since 1h

# 3. Run diagnostics
nself doctor

# 4. Check resource usage
nself metrics

# 5. Verify configuration
nself config validate

# 6. Check service health
nself health
```

---

## Services Won't Start

### Symptom: `nself start` fails immediately

**Diagnostic:**
```bash
# Check what failed
nself status

# View detailed logs
nself logs

# Check Docker daemon
docker info

# Check for port conflicts
sudo lsof -i -P -n | grep LISTEN
```

**Common Causes & Solutions:**

#### 1. Port Already in Use

**Error:**
```
Error starting userland proxy: listen tcp4 0.0.0.0:5432: bind: address already in use
```

**Solution:**
```bash
# Find process using port
sudo lsof -i :5432

# Kill the process
sudo kill -9 PID

# Or change port in .env
POSTGRES_PORT=5433

# Rebuild and restart
nself build && nself start
```

---

#### 2. Docker Not Running

**Error:**
```
Cannot connect to the Docker daemon. Is the docker daemon running?
```

**Solution:**
```bash
# macOS
open -a Docker

# Linux
sudo systemctl start docker
sudo systemctl enable docker

# Verify
docker info
```

---

#### 3. Insufficient Memory

**Error:**
```
Container exited with code 137 (OOM Killed)
```

**Solution:**
```bash
# Check Docker memory limits
docker info | grep Memory

# Increase Docker memory (Docker Desktop)
# Settings → Resources → Memory → Increase to 8GB+

# Reduce service memory limits
# Edit docker-compose.yml:
services:
  postgres:
    mem_limit: 2G  # Reduce if needed

# Restart
nself restart
```

---

#### 4. Missing Environment Variables

**Error:**
```
Error: POSTGRES_PASSWORD is required
```

**Solution:**
```bash
# Check .env file exists
ls -la .env

# Validate configuration
nself config validate

# Generate missing variables
nself init --reconfigure

# Or manually add to .env:
POSTGRES_PASSWORD=$(openssl rand -base64 32)
```

---

#### 5. Corrupted Docker State

**Error:**
```
Error response from daemon: network ... not found
```

**Solution:**
```bash
# Clean up Docker state
nself stop
docker system prune -af --volumes

# Rebuild
nself build
nself start
```

---

### Symptom: Service starts then immediately stops

**Diagnostic:**
```bash
# Check recent container logs
nself logs service-name --tail 100

# Check exit code
docker ps -a | grep service-name
```

**Common Causes & Solutions:**

#### 1. Configuration Error

**Check logs for:**
```
Error parsing config file
Invalid configuration
```

**Solution:**
```bash
# Validate service config
nself config validate

# Check service-specific config
cat docker-compose.yml | grep service-name -A 20

# Rebuild configuration
nself build --force
nself restart service-name
```

---

#### 2. Dependency Not Ready

**Symptom:** Hasura starts before PostgreSQL is ready

**Solution:**
```bash
# Increase health check timeout
# Edit docker-compose.yml:
services:
  hasura:
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 5s
      retries: 10  # Increase this

# Restart
nself restart
```

---

## Database Issues

### Symptom: Cannot connect to database

**Error:**
```
could not connect to server: Connection refused
```

**Diagnostic:**
```bash
# Check if PostgreSQL is running
nself status --services postgres

# Try manual connection
docker exec -it $(docker ps -qf "name=postgres") psql -U postgres

# Check logs
nself logs postgres --tail 50
```

**Solutions:**

#### 1. PostgreSQL Not Started

```bash
# Start PostgreSQL
nself start postgres

# Wait for ready
nself health postgres --wait 60
```

---

#### 2. Wrong Credentials

```bash
# Check .env for correct credentials
grep POSTGRES .env

# Reset password
POSTGRES_PASSWORD=new-password

# Rebuild
nself build && nself restart postgres
```

---

#### 3. Connection Pool Exhausted

**Error:**
```
FATAL: sorry, too many clients already
```

**Solution:**
```bash
# Check active connections
nself db query "SELECT count(*) FROM pg_stat_activity;"

# Increase connection limit
# .env:
POSTGRES_MAX_CONNECTIONS=200

# Rebuild
nself build && nself restart postgres

# Or use connection pooler (PgBouncer)
nself service enable pgbouncer
```

---

### Symptom: Slow database queries

**Diagnostic:**
```bash
# Find slow queries
nself db query "
  SELECT query, calls, mean_exec_time, max_exec_time
  FROM pg_stat_statements
  ORDER BY mean_exec_time DESC
  LIMIT 10;
"

# Check for missing indexes
nself db inspect indexes --missing

# Check table sizes
nself db inspect tables --size
```

**Solutions:**

#### 1. Missing Indexes

```bash
# Identify table scans
nself db query "
  SELECT schemaname, tablename, seq_scan, seq_tup_read
  FROM pg_stat_user_tables
  WHERE seq_scan > 0
  ORDER BY seq_tup_read DESC
  LIMIT 10;
"

# Create index
nself db query "CREATE INDEX CONCURRENTLY idx_users_email ON users(email);"

# Analyze table
nself db query "ANALYZE users;"
```

---

#### 2. Database Bloat

```bash
# Check for bloat
nself db inspect bloat

# Vacuum database
nself db optimize vacuum

# For severe bloat
nself db optimize vacuum-full  # WARNING: Locks tables
```

---

#### 3. Insufficient Resources

```bash
# Check PostgreSQL memory
nself status postgres --verbose

# Increase shared_buffers
# .env:
POSTGRES_SHARED_BUFFERS=4GB

# Rebuild
nself build && nself restart postgres
```

---

## Authentication Problems

### Symptom: Cannot log in

**Error:**
```
Invalid credentials
```

**Diagnostic:**
```bash
# Check Auth service
nself status --services auth

# Check Auth logs
nself logs auth --tail 50

# Verify user exists
nself db query "SELECT email, email_verified FROM auth.users WHERE email = 'user@example.com';"
```

**Solutions:**

#### 1. Email Not Verified

```bash
# Manually verify email
nself auth verify-email user@example.com

# Or via database
nself db query "UPDATE auth.users SET email_verified = true WHERE email = 'user@example.com';"
```

---

#### 2. Account Locked

```bash
# Check lock status
nself db query "SELECT email, locked_until FROM auth.users WHERE email = 'user@example.com';"

# Unlock account
nself auth unlock user@example.com
```

---

#### 3. JWT Secret Mismatch

**Error:**
```
Invalid JWT signature
```

**Solution:**
```bash
# Check JWT secret consistency
grep JWT .env

# Must match between Auth and Hasura
HASURA_GRAPHQL_JWT_SECRET='{"type":"HS256","key":"same-key-here"}'
AUTH_JWT_SECRET=same-key-here

# Rebuild
nself build && nself restart
```

---

### Symptom: Sessions expire immediately

**Diagnostic:**
```bash
# Check session duration settings
grep SESSION .env

# Check Redis (if session store)
docker exec -it $(docker ps -qf "name=redis") redis-cli KEYS "session:*"
```

**Solution:**
```bash
# Increase session duration
# .env:
AUTH_ACCESS_TOKEN_EXPIRES_IN=900  # 15 minutes
AUTH_REFRESH_TOKEN_EXPIRES_IN=2592000  # 30 days

# Rebuild
nself build && nself restart auth
```

---

## API/GraphQL Errors

### Symptom: GraphQL queries fail with permission error

**Error:**
```
{
  "errors": [
    {
      "message": "field \"users\" not found in type: 'query_root'",
      "extensions": {
        "code": "validation-failed"
      }
    }
  ]
}
```

**Diagnostic:**
```bash
# Check Hasura permissions
# Open Hasura Console
nself admin hasura

# Check table permissions
nself db query "SELECT * FROM information_schema.table_privileges WHERE grantee = 'your_role';"
```

**Solutions:**

#### 1. Missing Permissions

```bash
# Grant select permission in Hasura Console
# Or via metadata:
nself hasura metadata apply -f hasura/metadata/tables.yaml
```

---

#### 2. RLS Blocking Access

```bash
# Check RLS policies
nself db query "SELECT * FROM pg_policies WHERE tablename = 'users';"

# Temporarily disable RLS (development only!)
nself db query "ALTER TABLE users DISABLE ROW LEVEL SECURITY;"

# Or fix policy
nself db query "
  CREATE POLICY user_select_own ON users
    FOR SELECT
    USING (id = current_setting('hasura.user.id')::uuid);
"
```

---

### Symptom: Slow GraphQL responses

**Diagnostic:**
```bash
# Check Hasura logs for slow queries
nself logs hasura | grep "duration"

# Enable query logging
# .env:
HASURA_GRAPHQL_ENABLE_CONSOLE=true
HASURA_GRAPHQL_DEV_MODE=true
HASURA_GRAPHQL_ENABLE_TELEMETRY=false
HASURA_GRAPHQL_LOG_LEVEL=warn

# View in Hasura Console "Analyze" tab
```

**Solutions:**

#### 1. N+1 Query Problem

**Before (slow):**
```graphql
{
  users {
    id
    posts {  # Separate query per user
      title
    }
  }
}
```

**After (fast - use relationship):**
```graphql
# Define relationship in Hasura first
# Then query efficiently
{
  users {
    id
    posts_aggregate {  # Single query
      aggregate {
        count
      }
    }
  }
}
```

---

#### 2. Missing Database Index

```bash
# Hasura shows which fields are queried most
# Create indexes for them
nself db query "CREATE INDEX CONCURRENTLY idx_posts_user_id ON posts(user_id);"
```

---

## Performance Issues

### Symptom: High CPU usage

**Diagnostic:**
```bash
# Check which service
nself status --verbose

# View container stats
docker stats

# Check process inside container
nself exec postgres top
```

**Solutions:**

#### 1. Database CPU Spike

```bash
# Find expensive queries
nself db query "
  SELECT pid, usename, query, state
  FROM pg_stat_activity
  WHERE state = 'active' AND query NOT LIKE '%pg_stat_activity%'
  ORDER BY query_start;
"

# Kill runaway query
nself db query "SELECT pg_terminate_backend(12345);"

# Add query timeout
# .env:
POSTGRES_STATEMENT_TIMEOUT=30000  # 30 seconds
```

---

#### 2. Hasura CPU High

```bash
# Check active subscriptions
nself logs hasura | grep "subscription"

# Limit subscriptions per connection
# .env:
HASURA_GRAPHQL_LIVE_QUERIES_MULTIPLEXED_REFETCH_INTERVAL=1000
HASURA_GRAPHQL_LIVE_QUERIES_MULTIPLEXED_BATCH_SIZE=100
```

---

### Symptom: High memory usage

**Diagnostic:**
```bash
# View memory by container
docker stats --no-stream

# Check for memory leaks
nself logs --grep "memory" --since 24h
```

**Solutions:**

#### 1. PostgreSQL Memory High

```bash
# Reduce shared_buffers
# .env:
POSTGRES_SHARED_BUFFERS=2GB  # Down from 8GB

# Reduce work_mem
POSTGRES_WORK_MEM=32MB  # Down from 64MB

# Restart
nself restart postgres
```

---

#### 2. Node.js Service Memory Leak

```bash
# Check custom service memory
docker stats custom_service

# Restart service (temporary fix)
nself restart custom_service

# Find leak (add to service):
node --max-old-space-size=2048 --expose-gc app.js

# Use heap snapshots
npm install -g node-heapdump
# Take snapshot
kill -USR2 <pid>
```

---

## Storage Problems

### Symptom: File uploads fail

**Error:**
```
Failed to upload file: Permission denied
```

**Diagnostic:**
```bash
# Check MinIO status
nself status --services minio

# Check MinIO logs
nself logs minio

# Test connectivity
curl https://minio.local.nself.org/health
```

**Solutions:**

#### 1. MinIO Not Started

```bash
# Start MinIO
nself start minio

# Verify
nself urls | grep minio
```

---

#### 2. Bucket Doesn't Exist

```bash
# List buckets
nself exec minio mc ls local

# Create bucket
nself exec minio mc mb local/my-bucket

# Set public policy (if needed)
nself exec minio mc anonymous set download local/my-bucket
```

---

#### 3. CORS Issues

```bash
# Configure CORS
nself exec minio mc anonymous set-json /tmp/cors.json local/my-bucket

# cors.json:
cat > /tmp/cors.json <<EOF
{
  "CORSRules": [
    {
      "AllowedOrigins": ["*"],
      "AllowedMethods": ["GET", "PUT", "POST", "DELETE"],
      "AllowedHeaders": ["*"]
    }
  ]
}
EOF
```

---

## Network/Connectivity Issues

### Symptom: Cannot access services from browser

**Error:**
```
This site can't be reached
```

**Diagnostic:**
```bash
# Check Nginx status
nself status --services nginx

# Check Nginx logs
nself logs nginx

# Test internally
nself exec nginx curl http://hasura:8080/healthz

# Check DNS resolution
nslookup api.local.nself.org
```

**Solutions:**

#### 1. Nginx Not Running

```bash
# Start Nginx
nself start nginx

# Check for config errors
nself exec nginx nginx -t
```

---

#### 2. DNS Not Configured

```bash
# Check /etc/hosts
grep nself /etc/hosts

# Should have:
127.0.0.1 local.nself.org
127.0.0.1 api.local.nself.org
127.0.0.1 auth.local.nself.org

# If missing, rebuild
nself build
```

---

#### 3. Firewall Blocking

```bash
# Check firewall status
sudo ufw status

# Allow ports
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Reload
sudo ufw reload
```

---

## SSL/Certificate Errors

### Symptom: SSL certificate warning in browser

**Error:**
```
Your connection is not private
NET::ERR_CERT_AUTHORITY_INVALID
```

**Solutions:**

#### 1. Self-Signed Certificate (Development)

```bash
# Trust self-signed cert (macOS)
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ssl/cert.pem

# Chrome: Type "thisisunsafe" on warning page
```

---

#### 2. Certificate Expired

```bash
# Check expiry
openssl x509 -in ssl/cert.pem -noout -dates

# Regenerate (development)
nself config ssl generate

# Production: Renew Let's Encrypt
sudo certbot renew
nself restart nginx
```

---

#### 3. Certificate Mismatch

**Error:**
```
ERR_CERT_COMMON_NAME_INVALID
```

**Solution:**
```bash
# Check certificate CN
openssl x509 -in ssl/cert.pem -noout -subject

# Should match domain in browser
# If not, regenerate for correct domain
openssl req -new -x509 -days 365 -nodes \
  -out ssl/cert.pem \
  -keyout ssl/key.pem \
  -subj "/CN=yourdomain.com"
```

---

## Build/Configuration Errors

### Symptom: `nself build` fails

**Diagnostic:**
```bash
# Run build with verbose output
nself build --verbose

# Validate configuration
nself config validate

# Check for syntax errors in .env
cat .env | grep -v "^#" | grep -v "^$"
```

**Solutions:**

#### 1. Invalid Environment Variable

**Error:**
```
Error: Invalid value for POSTGRES_PORT
```

**Solution:**
```bash
# Check value type
grep POSTGRES_PORT .env

# Must be number
POSTGRES_PORT=5432  # Not "5432" (no quotes)

# Rebuild
nself build
```

---

#### 2. Missing Required Variable

**Error:**
```
Error: PROJECT_NAME is required
```

**Solution:**
```bash
# Add to .env
PROJECT_NAME=my-app

# Or re-run init
nself init --reconfigure
```

---

#### 3. Template Parsing Error

**Error:**
```
Error parsing template: unexpected EOF
```

**Solution:**
```bash
# Clean generated files
rm -rf docker-compose.yml nginx/ services/

# Rebuild from scratch
nself build --clean
```

---

## Monitoring Not Working

### Symptom: Grafana shows "No Data"

**Diagnostic:**
```bash
# Check monitoring services
nself status --services prometheus,grafana,loki

# Check Prometheus targets
curl https://prometheus.local.nself.org/api/v1/targets

# Check Grafana datasources
nself exec grafana grafana-cli admin reset-admin-password admin
```

**Solutions:**

#### 1. Prometheus Not Scraping

```bash
# Check Prometheus config
nself exec prometheus cat /etc/prometheus/prometheus.yml

# Verify targets are up
# Open: https://prometheus.local.nself.org/targets

# Restart Prometheus
nself restart prometheus
```

---

#### 2. Grafana Datasource Not Configured

```bash
# Open Grafana
nself monitor

# Go to Configuration → Data Sources
# Add Prometheus:
# URL: http://prometheus:9090
# Access: Server

# Test and Save
```

---

#### 3. Logs Not Appearing (Loki)

```bash
# Check Promtail is running
nself status --services promtail

# Promtail must be running for logs to reach Loki!
# If not enabled:
# .env:
MONITORING_ENABLED=true

# Rebuild
nself build && nself restart
```

---

## Multi-Tenancy Issues

### Symptom: Tenant data leaking between tenants

**CRITICAL SECURITY ISSUE**

**Diagnostic:**
```bash
# Check RLS is enabled
nself db query "
  SELECT schemaname, tablename, rowsecurity
  FROM pg_tables
  WHERE schemaname = 'public';
"

# Check current tenant context
nself db query "SELECT current_setting('app.tenant_id', true);"
```

**Solution:**
```bash
# Enable RLS on all tables
nself db query "ALTER TABLE users ENABLE ROW LEVEL SECURITY;"

# Create tenant isolation policy
nself db query "
  CREATE POLICY tenant_isolation ON users
    USING (tenant_id = current_setting('app.tenant_id')::uuid);
"

# Verify
nself tenant verify-isolation
```

---

## Debug Mode

**Enable maximum verbosity:**

```bash
# .env:
ENV=development
LOG_LEVEL=debug
HASURA_GRAPHQL_DEV_MODE=true
HASURA_GRAPHQL_ENABLE_CONSOLE=true

# Rebuild
nself build && nself restart

# View all logs
nself logs -f --timestamps
```

---

## Getting Help

### Before Filing an Issue

1. **Run diagnostics:**
   ```bash
   nself doctor > diagnostics.txt
   ```

2. **Collect logs:**
   ```bash
   nself logs --since 1h > logs.txt
   ```

3. **System info:**
   ```bash
   nself version
   docker version
   uname -a
   ```

4. **Search existing issues:**
   - GitHub: https://github.com/acamarata/nself/issues
   - Discord: https://discord.gg/nself

### Filing a Bug Report

**Include:**
- nself version
- Operating system
- Docker version
- Full error message
- Steps to reproduce
- Relevant logs
- Configuration (with secrets redacted)

**Template:**
```markdown
**nself version:** 0.9.8
**OS:** Ubuntu 22.04
**Docker:** 24.0.5

**Issue:** Services won't start after upgrading

**Steps to reproduce:**
1. Ran `nself update`
2. Ran `nself restart`
3. PostgreSQL fails to start

**Error:**
```
ERROR: could not access file "pg_hba.conf": Permission denied
```

**Logs:**
<attached logs.txt>

**Config:**
<attached diagnostics.txt>
```

---

## Related Documentation

- [Error Messages Reference](ERROR-MESSAGES.md)
- [Performance Tuning](../development/PERFORMANCE-BENCHMARKS.md)
- [Security Guide](../security/README.md)
- [Monitoring Guide](../guides/MONITORING-COMPLETE.md)

---

**Emergency Support:**
- Critical Production Issues: support@nself.org
- Security Vulnerabilities: security@nself.org
- Community Discord: https://discord.gg/nself
