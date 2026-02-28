# nself exec

**Category**: Utilities

Execute commands inside running service containers.

## Overview

Run arbitrary commands within Docker containers for debugging, maintenance, and administration tasks.

**Features**:
- ✅ Interactive shell access
- ✅ One-off command execution
- ✅ User specification
- ✅ Working directory control
- ✅ Environment variable passing

## Usage

```bash
nself exec [OPTIONS] <SERVICE> [COMMAND...]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-i, --interactive` | Keep STDIN open (interactive mode) | false |
| `-t, --tty` | Allocate pseudo-TTY | false |
| `-u, --user USER` | Run as specific user | container default |
| `-w, --workdir DIR` | Working directory | container default |
| `-e, --env VAR=VALUE` | Set environment variable | - |
| `--privileged` | Give extended privileges | false |

## Arguments

| Argument | Description |
|----------|-------------|
| `SERVICE` | Required: Service name |
| `COMMAND` | Command to execute (defaults to /bin/sh) |

## Examples

### Interactive Shell

```bash
nself exec postgres
```

**Opens interactive shell** (defaults to /bin/sh or /bin/bash):
```
root@postgres:/# ps aux
USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
postgres     1  0.0  1.2 223640 24512 ?        Ss   14:30   0:02 postgres
postgres    23  0.0  0.2 223764  5012 ?        Ss   14:30   0:00 postgres: checkpointer
...

root@postgres:/# exit
```

### Execute Single Command

```bash
nself exec postgres psql -U postgres -c "SELECT version();"
```

**Output**:
```
                                version
────────────────────────────────────────────────────────────
 PostgreSQL 15.3 on x86_64-pc-linux-gnu, compiled by gcc
(1 row)
```

### Run as Specific User

```bash
nself exec --user postgres postgres psql -c "\conninfo"
```

**Output**:
```
You are connected to database "myapp_db" as user "postgres"
```

### Set Working Directory

```bash
nself exec -w /var/lib/postgresql/data postgres ls -lh
```

**Lists contents of PostgreSQL data directory.**

### Pass Environment Variables

```bash
nself exec -e DEBUG=true api npm run debug
```

### Multiple Commands

```bash
nself exec postgres bash -c "
  psql -U postgres -c 'SELECT COUNT(*) FROM users;'
  psql -U postgres -c 'SELECT COUNT(*) FROM orders;'
"
```

## Service-Specific Examples

### PostgreSQL

```bash
# Open psql directly
nself exec postgres psql -U postgres -d myapp_db

# Create database backup
nself exec postgres pg_dump -U postgres myapp_db > backup.sql

# Analyze table
nself exec postgres psql -U postgres -c "ANALYZE users;"

# View connections
nself exec postgres psql -U postgres -c "SELECT * FROM pg_stat_activity;"
```

### Hasura

```bash
# Check Hasura version
nself exec hasura hasura version

# Export metadata
nself exec -w /hasura-metadata hasura hasura metadata export

# Clear cache
nself exec hasura curl -X POST http://localhost:8080/v1/metadata \
  -H "X-Hasura-Admin-Secret: $HASURA_ADMIN_SECRET" \
  -d '{"type":"clear_metadata","args":{}}'
```

### Redis

```bash
# Redis CLI
nself exec redis redis-cli

# Get all keys
nself exec redis redis-cli KEYS "*"

# Get specific value
nself exec redis redis-cli GET user:123

# Monitor commands
nself exec redis redis-cli MONITOR

# Get stats
nself exec redis redis-cli INFO stats
```

### MinIO

```bash
# List buckets
nself exec minio mc ls local

# Create bucket
nself exec minio mc mb local/new-bucket

# Copy file
nself exec minio mc cp /tmp/file.txt local/bucket/

# Get bucket policy
nself exec minio mc policy get local/bucket
```

### Nginx

```bash
# Test configuration
nself exec nginx nginx -t

# Reload configuration
nself exec nginx nginx -s reload

# View access logs
nself exec nginx tail -f /var/log/nginx/access.log

# View error logs
nself exec nginx tail -f /var/log/nginx/error.log
```

### Custom Services

```bash
# Node.js REPL
nself exec api node

# Run migration script
nself exec api npm run migrate

# Check environment
nself exec api env | grep DATABASE

# Debug
nself exec api node --inspect-brk=0.0.0.0:9229 index.js
```

## Interactive vs Non-Interactive

### Interactive Mode (-it)

```bash
nself exec -it postgres bash
```

**Use when**:
- Opening a shell
- Running interactive tools (psql, redis-cli)
- Need to provide input

### Non-Interactive Mode

```bash
nself exec postgres psql -c "SELECT 1"
```

**Use when**:
- Running scripts
- Automation
- CI/CD pipelines
- One-off commands

## Common Tasks

### Database Administration

```bash
# Create database
nself exec postgres createdb -U postgres newdb

# Drop database
nself exec postgres dropdb -U postgres olddb

# Restore backup
cat backup.sql | nself exec -i postgres psql -U postgres myapp_db

# Vacuum database
nself exec postgres psql -U postgres -c "VACUUM FULL;"
```

### File Operations

```bash
# Copy file from host to container
docker cp file.txt $(docker-compose ps -q api):/app/

# Copy file from container to host
docker cp $(docker-compose ps -q api):/app/logs/app.log ./

# View file in container
nself exec api cat /app/config.json

# Edit file in container
nself exec -it api vi /app/config.json
```

### Process Management

```bash
# View processes
nself exec postgres ps aux

# Kill process
nself exec postgres kill -9 <PID>

# Check resource usage
nself exec postgres top

# View open files
nself exec postgres lsof
```

### Network Debugging

```bash
# Test connectivity
nself exec api ping -c 3 postgres

# Check DNS
nself exec api nslookup postgres

# View listening ports
nself exec api netstat -tuln

# Curl internal service
nself exec api curl http://hasura:8080/healthz
```

## Troubleshooting

### Command not found

**Error**:
```
exec: "bash": executable file not found
```

**Solution**:
```bash
# Try sh instead
nself exec postgres sh

# Or install bash
nself exec postgres apt-get update && apt-get install -y bash
```

### Permission denied

**Error**:
```
permission denied
```

**Solution**:
```bash
# Run as root
nself exec --user root postgres command

# Or check file permissions
nself exec postgres ls -la /path/to/file
```

### Container not running

**Error**:
```
Error: No such container
```

**Solution**:
```bash
# Check if service is running
nself status postgres

# Start service
nself start postgres
```

### TTY allocation error

**Error**:
```
the input device is not a TTY
```

**Solution**:
```bash
# Remove -t flag or use -it together
nself exec -i postgres command
```

## Best Practices

### 1. Use for Debugging Only

```bash
# Good: Temporary debugging
nself exec api node debug.js

# Avoid: Permanent changes
# Don't manually edit files in containers
# Use proper config management instead
```

### 2. Prefer Service-Specific Commands

```bash
# Good: Use dedicated command
nself db shell

# Avoid: Manual psql exec
nself exec postgres psql -U postgres
```

### 3. Be Careful with Destructive Operations

```bash
# Dangerous!
nself exec postgres psql -U postgres -c "DROP DATABASE myapp_db;"

# Safer: Backup first
nself db backup before-drop.sql
nself exec postgres psql -U postgres -c "DROP DATABASE olddb;"
```

### 4. Use Scripts for Complex Tasks

```bash
# Create script
cat > debug.sh << 'EOF'
#!/bin/bash
echo "Checking database..."
psql -U postgres -c "SELECT COUNT(*) FROM users;"
echo "Checking cache..."
redis-cli DBSIZE
EOF

# Execute script
chmod +x debug.sh
docker cp debug.sh $(docker-compose ps -q postgres):/tmp/
nself exec postgres /tmp/debug.sh
```

## Automation

### In Shell Scripts

```bash
#!/bin/bash
# Automated database check

# Get user count
USERS=$(nself exec postgres psql -U postgres -t -c "SELECT COUNT(*) FROM users;")
echo "Total users: $USERS"

# Get cache size
CACHE=$(nself exec redis redis-cli DBSIZE | tr -d '\r')
echo "Cache keys: $CACHE"
```

### In CI/CD

```yaml
# .github/workflows/test.yml
- name: Run database seeds
  run: nself exec postgres psql -U postgres -f /seeds/test-data.sql

- name: Verify data
  run: |
    COUNT=$(nself exec postgres psql -U postgres -t -c "SELECT COUNT(*) FROM users;")
    if [ "$COUNT" -lt 1 ]; then
      echo "Seed failed"
      exit 1
    fi
```

## Related Commands

- `nself db shell` - PostgreSQL shell (preferred over exec)
- `nself logs` - View container logs
- `nself status` - Check container status

## See Also

- [Docker exec Documentation](https://docs.docker.com/engine/reference/commandline/exec/)
- [Debugging Guide](../../guides/DEBUGGING.md)
- [nself db shell](../db/shell.md)
