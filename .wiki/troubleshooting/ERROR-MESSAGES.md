# nself Error Messages Guide

This guide explains common error messages you might encounter and how to resolve them.

## Quick Reference

| Error Type | Command to Fix |
|------------|----------------|
| Port conflicts | `nself doctor --fix` |
| Container failures | `nself logs <service>` then `nself restart <service>` |
| Configuration issues | `nself config validate --fix` |
| Build problems | `docker builder prune -f` then `nself build` |
| Health check | `nself doctor` |

---

## Common Error Scenarios

### 1. Port Conflict Errors

**Error Message:**
```
✗ Container 'postgres' failed to start

  Reason: Port 5432 is already in use
```

**Solutions:**

1. **Find and stop the conflicting process:**
   ```bash
   # macOS/Linux
   lsof -ti:5432 | xargs kill -9

   # Or for specific services
   brew services stop postgresql  # macOS
   sudo systemctl stop postgresql # Linux
   ```

2. **Change the port in `.env.local`:**
   ```bash
   # Add to .env.local
   POSTGRES_PORT=5433

   # Then rebuild and restart
   nself build && nself start
   ```

3. **Use automatic fix:**
   ```bash
   nself doctor --fix
   ```

**Prevention:**
- Use `.env.local` to override ports that conflict with your system
- Run `nself doctor` before starting services

---

### 2. Container Startup Failures

**Error Message:**
```
✗ Container 'hasura' failed to start

  Reason: Connection to database failed
```

**Solutions:**

1. **Check logs for detailed error:**
   ```bash
   nself logs hasura
   nself logs hasura --tail 100
   ```

2. **Verify dependencies are running:**
   ```bash
   nself status
   docker ps
   ```

3. **Restart in dependency order:**
   ```bash
   nself restart postgres
   sleep 5
   nself restart hasura
   ```

4. **Full rebuild:**
   ```bash
   nself stop
   nself build
   nself start
   ```

**Common Causes:**
- Database not ready when service starts
- Network configuration issues
- Missing environment variables
- Resource constraints (memory/CPU)

---

### 3. Missing Configuration

**Error Message:**
```
✗ Required configuration missing

  File: .env not found or incomplete

  Missing variables:
  • PROJECT_NAME
  • POSTGRES_PASSWORD
  • HASURA_GRAPHQL_ADMIN_SECRET
```

**Solutions:**

1. **Initialize configuration:**
   ```bash
   nself init
   ```

2. **Copy from example:**
   ```bash
   cp .env.example .env
   # Edit .env with your values
   ```

3. **Validate and auto-fix:**
   ```bash
   nself config validate --fix
   ```

**Required Variables:**
- `PROJECT_NAME` - Your project identifier
- `POSTGRES_PASSWORD` - Database password
- `HASURA_GRAPHQL_ADMIN_SECRET` - Hasura admin secret
- `BASE_DOMAIN` - Your domain (default: local.nself.org)

---

### 4. Permission Denied

**Error Message:**
```
✗ Permission denied

  Cannot access: /var/run/docker.sock
```

**Solutions:**

1. **Fix file ownership:**
   ```bash
   sudo chown -R $(whoami) ./
   ```

2. **Add user to docker group (Linux):**
   ```bash
   sudo usermod -aG docker $USER
   # Log out and back in
   ```

3. **Check Docker is running:**
   ```bash
   docker info
   ```

4. **macOS - Restart Docker Desktop:**
   - Docker Desktop handles permissions automatically
   - If issues persist, restart Docker Desktop

**Common Causes:**
- Docker daemon not running
- User not in docker group (Linux)
- File ownership issues
- Insufficient sudo privileges

---

### 5. Network Connection Failures

**Error Message:**
```
✗ Network connection failed

  Service: hasura
  URL: http://localhost:8080
  Error: connection refused
```

**Solutions:**

1. **Check service is running:**
   ```bash
   docker ps | grep hasura
   ```

2. **Check Docker networking:**
   ```bash
   docker network ls
   docker network inspect nself_default
   ```

3. **Restart Docker networking:**
   ```bash
   nself stop
   docker network prune -f
   nself start
   ```

4. **Test connectivity:**
   ```bash
   curl -v http://localhost:8080
   ```

**Common Causes:**
- Service not started
- Port mapping incorrect
- Firewall blocking connections
- VPN interfering with Docker networking

---

### 6. Docker Not Running

**Error Message:**
```
✗ Docker is not running
```

**Solutions:**

**macOS:**
```bash
# Start Docker Desktop
open -a Docker

# Wait 10-15 seconds for initialization

# Verify
docker info
```

**Linux:**
```bash
# Start Docker service
sudo systemctl start docker

# Enable on boot
sudo systemctl enable docker

# Verify
docker info
```

**Windows:**
- Start Docker Desktop from Start Menu
- Or run: `"C:\Program Files\Docker\Docker\Docker Desktop.exe"`

---

### 7. Insufficient Resources

**Error Message:**
```
✗ Insufficient memory

  Available: 2GB
  Required: 4GB
```

**Solutions:**

1. **Close unnecessary applications**

2. **Increase Docker memory (macOS):**
   - Docker Desktop → Settings → Resources → Memory
   - Recommended: 4GB minimum, 8GB optimal

3. **Disable optional services:**
   ```bash
   # In .env.local
   MONITORING_ENABLED=false
   MLFLOW_ENABLED=false
   REDIS_ENABLED=false
   ```

4. **Check disk space:**
   ```bash
   df -h .
   docker system df
   ```

5. **Clean Docker resources:**
   ```bash
   docker system prune -a --volumes
   nself clean
   ```

---

### 8. Database Connection Errors

**Error Message:**
```
✗ Database connection failed

  Database: PostgreSQL
  Error: connection refused
```

**Solutions:**

1. **Check if database is running:**
   ```bash
   docker ps | grep postgres
   nself logs postgres
   ```

2. **Verify connection settings in `.env`:**
   ```bash
   POSTGRES_PORT=5432
   POSTGRES_USER=postgres
   POSTGRES_PASSWORD=<your-password>
   POSTGRES_DB=nhost
   ```

3. **Check for port conflicts:**
   ```bash
   lsof -i :5432
   ```

4. **Restart database:**
   ```bash
   nself restart postgres
   sleep 5
   nself status
   ```

5. **Test connection:**
   ```bash
   docker exec -it ${PROJECT_NAME}_postgres psql -U postgres
   ```

**Connection URL Format:**
```
postgresql://user:password@localhost:5432/database
```

---

### 9. Build Failures

**Error Message:**
```
✗ Build failed for 'custom-service'

  Stage: RUN npm install
  Error: ENOENT: no such file or directory
```

**Solutions:**

1. **Check Dockerfile syntax:**
   ```bash
   cat services/custom-service/Dockerfile
   ```

2. **Clean build cache:**
   ```bash
   docker builder prune -f
   nself build --no-cache
   ```

3. **Check disk space:**
   ```bash
   df -h
   docker system df
   ```

4. **Rebuild specific service:**
   ```bash
   docker-compose build custom-service
   ```

5. **Check Docker daemon logs:**
   - **macOS:** Docker Desktop → Troubleshoot → View logs
   - **Linux:** `journalctl -u docker.service`

**Common Causes:**
- Syntax errors in Dockerfile
- Missing files in build context
- Network issues downloading dependencies
- Insufficient disk space
- Build cache corruption

---

### 10. Health Check Failures

**Error Message:**
```
✗ Service 'hasura' is unhealthy

  Possible solutions:
  1. Check service logs for errors
  2. Verify dependencies are running
  3. Restart the service
```

**Solutions:**

1. **Check detailed logs:**
   ```bash
   nself logs hasura --tail 50
   ```

2. **Inspect container:**
   ```bash
   docker inspect ${PROJECT_NAME}_hasura
   ```

3. **Check dependencies:**
   ```bash
   nself status
   ```

4. **Restart service:**
   ```bash
   nself restart hasura
   ```

5. **Monitor resources:**
   ```bash
   docker stats ${PROJECT_NAME}_hasura
   ```

**Note:** Some services take 30-60 seconds to become healthy after starting.

---

## Diagnostic Commands

### Quick Health Check
```bash
nself doctor
```

### Comprehensive Diagnostics
```bash
nself doctor --verbose
```

### Auto-Fix Common Issues
```bash
nself doctor --fix
```

### View Service Logs
```bash
nself logs <service>
nself logs <service> --tail 100
nself logs <service> --follow
```

### Service Status
```bash
nself status
nself status --detailed
```

### Container Information
```bash
docker ps -a
docker stats
docker inspect ${PROJECT_NAME}_<service>
```

---

## Prevention Best Practices

1. **Always run `nself doctor` before starting**
   - Checks system requirements
   - Identifies potential issues
   - Suggests fixes

2. **Use `.env.local` for customizations**
   - Override ports that conflict
   - Set machine-specific values
   - Keep `.env` clean

3. **Monitor resource usage**
   ```bash
   docker stats
   df -h
   free -h  # Linux
   ```

4. **Keep Docker updated**
   ```bash
   docker --version
   docker compose version
   ```

5. **Regular cleanup**
   ```bash
   nself clean
   docker system prune -a
   ```

---

## Getting Help

### Documentation
- **Main docs:** `docs/README.md`
- **Configuration:** `docs/configuration/`
- **Troubleshooting:** `docs/troubleshooting/`

### Commands
```bash
nself help
nself <command> --help
```

### Community
- GitHub Issues: [github.com/nself-org/cli/issues](https://github.com/nself-org/cli/issues)
- Discussions: [github.com/nself-org/cli/discussions](https://github.com/nself-org/cli/discussions)

---

## Error Message Format

All nself error messages follow this format for consistency:

```
✗ [Problem Statement]

  Reason: [Specific cause]

  Possible solutions:
  1. [Primary solution with command]

  2. [Alternative solution]

  3. [Automatic fix if available]

  Run 'nself doctor' for more diagnostics
```

This format ensures:
- **Clear problem statement** - What went wrong
- **Specific reason** - Why it failed
- **Actionable solutions** - How to fix it (numbered)
- **Command examples** - Copy-paste ready
- **Additional help** - Where to get more info

---

## Contributing

Found an unclear error message? Please:
1. Open an issue with the error message
2. Describe what was confusing
3. Suggest improvements

Error messages are continually improved based on user feedback.
