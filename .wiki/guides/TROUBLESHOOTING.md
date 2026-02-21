# nself v0.3.9 Troubleshooting Guide

Comprehensive guide to diagnosing and fixing common issues with nself v0.3.9.

## Table of Contents

- [Quick Diagnostics](#quick-diagnostics)
- [Installation Issues](#installation-issues)
- [Docker Issues](#docker-issues)
- [Service Issues](#service-issues)
- [Network Issues](#network-issues)
- [Database Issues](#database-issues)
- [SSL/Certificate Issues](#sslcertificate-issues)
- [Performance Issues](#performance-issues)
- [Configuration Issues](#configuration-issues)
- [Command-Specific Issues](#command-specific-issues)
- [macOS Compatibility](#macos-compatibility)
- [Go Module Issues](#go-module-issues)
- [Port Conflict Resolution](#port-conflict-resolution)

## Quick Diagnostics

**Always start with these commands:**

```bash
# Run comprehensive diagnostics
nself doctor

# Check service status
nself status

# View recent logs
nself logs --tail 50

# Validate configuration
nself config validate
```

## Installation Issues

### Issue: "command not found: nself"

**Symptoms:**
```bash
$ nself version
bash: nself: command not found
```

**Solutions:**

1. **Check if nself is installed:**
   ```bash
   ls -la ~/.nself/bin/nself.sh
   ```

2. **Reinstall nself:**
   ```bash
   curl -sSL https://raw.githubusercontent.com/nself-org/cli/main/install.sh | bash
   ```

3. **Fix PATH manually:**
   ```bash
   echo 'export PATH="$HOME/.nself/bin:$PATH"' >> ~/.bashrc
   source ~/.bashrc
   ```

4. **For zsh users:**
   ```bash
   echo 'export PATH="$HOME/.nself/bin:$PATH"' >> ~/.zshrc
   source ~/.zshrc
   ```

---

### Issue: Installation fails with permission errors

**Symptoms:**
```
[ERROR] Cannot write to /usr/local/bin
Permission denied
```

**Solutions:**

1. **Install to user directory (recommended):**
   ```bash
   curl -sSL https://raw.githubusercontent.com/nself-org/cli/main/install.sh | bash
   ```

2. **System-wide installation (not recommended):**
   ```bash
   sudo curl -sSL https://raw.githubusercontent.com/nself-org/cli/main/install.sh | bash
   ```

---

## Docker Issues

### Issue: Docker daemon not running

**Symptoms:**
```
[ERROR] Docker daemon is not running
Cannot connect to Docker daemon at unix:///var/run/docker.sock
```

**Solutions:**

1. **macOS/Windows - Start Docker Desktop:**
   - Open Docker Desktop application
   - Wait for "Docker Desktop is running" status

2. **Linux - Start Docker service:**
   ```bash
   sudo systemctl start docker
   sudo systemctl enable docker  # Auto-start on boot
   ```

3. **Check Docker status:**
   ```bash
   docker version
   docker ps
   ```

---

### Issue: Permission denied on Docker socket

**Symptoms:**
```
Got permission denied while trying to connect to Docker daemon socket
```

**Solutions:**

1. **Add user to docker group (Linux):**
   ```bash
   sudo usermod -aG docker $USER
   newgrp docker  # Or logout and login
   ```

2. **Fix socket permissions:**
   ```bash
   sudo chmod 666 /var/run/docker.sock
   ```

3. **Run with sudo (temporary):**
   ```bash
   sudo nself up
   ```

---

### Issue: Docker Compose not found

**Symptoms:**
```
[ERROR] Docker Compose v2 is not available
docker: 'compose' is not a docker command
```

**Solutions:**

1. **Install Docker Compose v2:**
   ```bash
   # macOS/Windows: Update Docker Desktop
   
   # Linux:
   sudo apt update
   sudo apt install docker-compose-plugin
   ```

2. **Verify installation:**
   ```bash
   docker compose version
   ```

---

## Service Issues

### Issue: Services won't start

**Symptoms:**
```
[ERROR] Failed to start services
Container nself_postgres_1 exited with code 1
```

**Diagnostic Steps:**

1. **Check logs:**
   ```bash
   nself logs postgres --tail 50
   ```

2. **Check port conflicts:**
   ```bash
   nself doctor  # Will check all ports
   
   # Manual check:
   lsof -i :5432  # PostgreSQL
   lsof -i :8080  # Hasura
   lsof -i :80    # Nginx
   ```

3. **Reset and retry:**
   ```bash
   nself down
   nself up
   ```

4. **Hard reset if needed:**
   ```bash
   nself reset --hard  # WARNING: Deletes data
   nself init
   nself build
   nself up
   ```

---

### Issue: Service keeps restarting

**Symptoms:**
```
Container nself_hasura_1 is restarting, wait until...
```

**Solutions:**

1. **Check service logs:**
   ```bash
   nself logs hasura --tail 100
   ```

2. **Common causes:**
   - Database not ready
   - Invalid configuration
   - Missing environment variables
   - Resource limits

3. **Fix configuration:**
   ```bash
   nself config validate
   # Fix any reported issues
   nself down
   nself up
   ```

---

### Issue: Port already in use

**Symptoms:**
```
[ERROR] Port 5432 is already in use
bind: address already in use
```

**Solutions:**

1. **Let auto-fix handle it:**
   ```bash
   nself up  # Auto-fix will attempt to resolve
   ```

2. **Find and stop conflicting process:**
   ```bash
   # Find process
   lsof -i :5432
   
   # Stop if it's our container
   docker stop <container_name>
   
   # Stop system PostgreSQL
   sudo systemctl stop postgresql
   ```

3. **Change port in configuration:**
   ```bash
   # Edit .env.local
   POSTGRES_PORT=5433
   
   # Rebuild and restart
   nself build
   nself up
   ```

---

## Network Issues

### Issue: Cannot access services

**Symptoms:**
- Browser shows "This site can't be reached"
- `curl https://api.localhost` fails

**Solutions:**

1. **Check if services are running:**
   ```bash
   nself status
   ```

2. **Check nginx is running:**
   ```bash
   docker ps | grep nginx
   nself logs nginx --tail 20
   ```

3. **Test direct access:**
   ```bash
   # Bypass nginx
   curl http://localhost:8080/healthz  # Hasura
   curl http://localhost:4000/healthz  # Auth
   ```

4. **Check hosts file:**
   ```bash
   # Should have these entries:
   cat /etc/hosts | grep localhost
   # 127.0.0.1 localhost
   # 127.0.0.1 api.localhost auth.localhost storage.localhost
   ```

5. **Flush DNS cache:**
   ```bash
   # macOS
   sudo dscacheutil -flushcache
   
   # Linux
   sudo systemctl restart systemd-resolved
   
   # Windows
   ipconfig /flushdns
   ```

---

### Issue: SSL certificate errors

**Symptoms:**
- Browser shows "Your connection is not private"
- NET::ERR_CERT_AUTHORITY_INVALID

**Solutions:**

1. **Trust certificates:**
   ```bash
   nself auth ssl trust
   ```

2. **Regenerate certificates:**
   ```bash
   rm -rf nginx/ssl/*
   nself build
   nself auth ssl trust
   ```

3. **For Chrome - type this in the error page:**
   ```
   thisisunsafe
   ```

4. **Add certificate exception:**
   - Click "Advanced" in browser
   - Click "Proceed to site (unsafe)"

---

## Database Issues

### Issue: Cannot connect to database

**Symptoms:**
```
[ERROR] Connection to database failed
FATAL: password authentication failed for user "postgres"
```

**Solutions:**

1. **Check database is running:**
   ```bash
   nself status
   docker ps | grep postgres
   ```

2. **Verify credentials:**
   ```bash
   # Check .env.local
   grep POSTGRES .env.local
   ```

3. **Test connection:**
   ```bash
   # Using psql
   psql postgresql://postgres:yourpassword@localhost:5432/postgres
   
   # Using docker
   docker exec -it nself_postgres_1 psql -U postgres
   ```

4. **Reset database:**
   ```bash
   nself db reset
   ```

---

### Issue: Migrations fail

**Symptoms:**
```
[ERROR] Migration failed
relation "users" already exists
```

**Solutions:**

1. **Check migration status:**
   ```bash
   nself db console
   SELECT * FROM schema_migrations;
   ```

2. **Reset migrations:**
   ```bash
   # Remove migration tracking
   nself db console
   DROP TABLE IF EXISTS schema_migrations;
   
   # Re-run migrations
   nself db migrate
   ```

3. **Start fresh:**
   ```bash
   nself db reset --force
   nself db migrate
   nself db seed
   ```

---

## SSL/Certificate Issues

### Issue: Certificate expired

**Symptoms:**
```
[WARNING] Certificate expires in -5 days
SSL certificate has expired
```

**Solutions:**

1. **Regenerate certificates:**
   ```bash
   rm -rf nginx/ssl/*
   nself build
   nself auth ssl trust
   nself restart nginx
   ```

2. **For production - use Let's Encrypt:**
   ```bash
   # Install certbot
   sudo apt install certbot
   
   # Generate certificates
   sudo certbot certonly --standalone -d api.yourdomain.com
   ```

---

## Performance Issues

### Issue: Services running slowly

**Symptoms:**
- Slow response times
- High CPU/memory usage
- Timeouts

**Solutions:**

1. **Check resource usage:**
   ```bash
   nself status  # Shows CPU/memory per service
   docker stats  # Real-time stats
   ```

2. **Increase resource limits:**
   ```bash
   # Edit docker-compose.yml
   services:
     hasura:
       mem_limit: 4g  # Increase from 2g
       cpus: '2.0'    # Increase from 1.0
   ```

3. **Check for memory leaks:**
   ```bash
   # Monitor over time
   watch -n 5 'docker stats --no-stream'
   ```

4. **Optimize database:**
   ```bash
   nself db console
   VACUUM ANALYZE;  # PostgreSQL optimization
   ```

---

### Issue: Disk space issues

**Symptoms:**
```
[ERROR] No space left on device
```

**Solutions:**

1. **Check disk usage:**
   ```bash
   df -h
   du -sh *
   ```

2. **Clean Docker resources:**
   ```bash
   # Remove unused images
   docker image prune -a
   
   # Remove unused volumes
   docker volume prune
   
   # Complete cleanup
   docker system prune -a --volumes
   ```

3. **Remove old backups:**
   ```bash
   ls -lah backups/
   rm backups/postgres_*.sql.gz  # Keep recent ones
   ```

---

## Configuration Issues

### Issue: Invalid environment variables

**Symptoms:**
```
[ERROR] Invalid configuration in .env.local
HASURA_GRAPHQL_JWT_SECRET: invalid JSON
```

**Solutions:**

1. **Validate configuration:**
   ```bash
   nself config validate
   ```

2. **Common fixes:**
   ```bash
   # Generate secure passwords
   nself config secrets generate
   
   # Fix JWT secret format
   HASURA_GRAPHQL_JWT_SECRET='{"type":"HS256","key":"32-character-secret-key-here!!!"}'
   ```

3. **Compare with example:**
   ```bash
   diff .env.local .env.example
   ```

---

### Issue: Services not enabled

**Symptoms:**
- Optional services not starting
- Missing from docker-compose.yml

**Solutions:**

1. **Enable in .env.local:**
   ```bash
   REDIS_ENABLED=true
   FUNCTIONS_ENABLED=true
   DASHBOARD_ENABLED=true
   ```

2. **Rebuild configuration:**
   ```bash
   nself build
   nself up
   ```

---

## Command-Specific Issues

### Issue: nself init fails

**Symptoms:**
```
[ERROR] Cannot initialize in the nself repository!
```

**Solution:**
```bash
# Create a project directory
mkdir ~/myproject
cd ~/myproject
nself init
```

---

### Issue: nself build fails

**Symptoms:**
```
[ERROR] Failed to build Docker images
```

**Solutions:**

1. **Clear cache and rebuild:**
   ```bash
   nself build --no-cache
   ```

2. **Check Docker disk space:**
   ```bash
   docker system df
   docker system prune -a
   ```

3. **Pull images manually:**
   ```bash
   docker pull postgres:15-alpine
   docker pull hasura/graphql-engine:v2.35.0
   ```

---

### Issue: nself update fails

**Symptoms:**
```
[ERROR] Failed to download update
```

**Solutions:**

1. **Manual update:**
   ```bash
   cd ~/.nself
   git pull origin main
   ```

2. **Reinstall:**
   ```bash
   rm -rf ~/.nself
   curl -sSL https://raw.githubusercontent.com/nself-org/cli/main/install.sh | bash
   ```

---

## Getting Help

If these solutions don't resolve your issue:

1. **Run diagnostics:**
   ```bash
   nself doctor > diagnostics.txt
   nself status >> diagnostics.txt
   nself logs --tail 100 >> diagnostics.txt
   ```

2. **Check GitHub Issues:**
   https://github.com/nself-org/cli/issues

3. **Create a new issue with:**
   - nself version
   - Operating system
   - Docker version
   - Error messages
   - Diagnostics output

4. **Community Support:**
   - GitHub Discussions
   - Include output from `nself doctor`

---

## Prevention Tips

1. **Regular maintenance:**
   ```bash
   # Weekly
   docker system prune
   nself doctor
   
   # Monthly
   nself update --check
   ```

2. **Before production:**
   ```bash
   nself config validate
   nself doctor
   ```

3. **Monitor resources:**
   ```bash
   # Add to crontab
   0 * * * * docker stats --no-stream >> /var/log/nself-stats.log
   ```

4. **Backup regularly:**
   ```bash
   # Daily backup
   nself db backup
   ```

---

## macOS Compatibility

### Bash 3.2 Issues

macOS ships with bash 3.2, which lacks some modern features. nself v0.3.0+ includes full compatibility.

**Symptom**: "declare: -A: invalid option" errors  
**Solution**: Already fixed in v0.3.0 with automatic fallbacks

**Symptom**: Script errors with associative arrays  
**Solution**: Update to v0.3.0 or later
```bash
nself update
```

### Docker Desktop

**Symptom**: Docker not starting automatically  
**Solution**: nself now offers to start Docker Desktop
```bash
nself up
# When prompted, choose 'Y' to start Docker Desktop
```

---

## Go Module Issues

### Missing go.sum Entry

**Symptom**: Build fails with "missing go.sum entry for module"  
**Solution**: nself v0.3.0+ handles this automatically

**Auto-Fix Options**:
1. Run `go mod tidy` automatically (recommended)
2. Manual fix instructions provided
3. Disable Go services temporarily
4. Rebuild without cache

**Manual Fix**:
```bash
cd services/go/your-service
go mod init your-service  # If no go.mod exists
go mod tidy
go mod download
nself build
```

### Build Cache Issues

**Symptom**: Go build errors persist after fixes  
**Solution**: Rebuild without cache
```bash
nself up
# When prompted after Go errors, choose 'Y' for no-cache rebuild
```

---

## Port Conflict Resolution

### Automatic Port Configuration

nself v0.3.0+ includes intelligent port conflict handling.

**When conflicts detected**, you'll see:
```
Port conflict options:
  1) Stop conflicting services
  2) Use alternative ports (auto-configure)
  3) Continue anyway (may fail)
  4) Cancel
```

### Option 2: Alternative Ports

Automatically finds free ports and updates `.env.local`:
- Port 80 → 8080, 8081, etc.
- Port 443 → 8443, 8444, etc.
- Port 5432 → 5433, 5434, etc.

**Changes are**:
- Saved to `.env.local`
- Applied immediately via auto-rebuild
- Persistent across restarts

### Manual Port Configuration

Edit `.env.local`:
```bash
NGINX_HTTP_PORT=8080
NGINX_HTTPS_PORT=8443
POSTGRES_PORT=5433
HASURA_PORT=8081
```

Then rebuild:
```bash
nself build
nself up
```

### Check Current Ports

```bash
# View configured ports
grep -E "_PORT=" .env.local

# Check what's using a port
lsof -i :443  # macOS/Linux
netstat -an | grep :443  # Windows
```

---

## Emergency Recovery

If everything is broken:

```bash
# Complete reset
cd ~
rm -rf myproject
docker stop $(docker ps -aq)
docker system prune -a --volumes
mkdir myproject
cd myproject
nself init
nself build
nself up
```

This will give you a fresh start with no data.