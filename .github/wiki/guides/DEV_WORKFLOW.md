# Developer Workflow Guide

Complete step-by-step guide from `nself init` to a fully working application with authentication.

**Goal:** Get from zero to working auth in **under 5 minutes**.

---

## Quick Start (5 Minutes)

```bash
# 1. Initialize project (30 seconds)
nself init --demo
cd demo-app

# 2. Build configuration (30 seconds)
nself build

# 3. Start all services (2 minutes)
nself start

# 4. Setup authentication (1 minute)
nself auth setup --default-users

# 5. Verify it works (30 seconds)
curl -k -X POST https://auth.local.nself.org/signin/email-password \
  -H "Content-Type: application/json" \
  -d '{"email":"owner@nself.org","password":"npass123"}'

# ✅ You should get an access_token back!
```

That's it! You now have:
- ✅ PostgreSQL database with auth schema
- ✅ Hasura GraphQL API tracking auth tables
- ✅ nHost authentication service
- ✅ 3 staff users ready to use
- ✅ Working login endpoint

---

## Detailed Workflow

### Step 1: Project Initialization

```bash
# Create a new project
nself init

# Follow the interactive prompts:
# - Project name: my-app
# - Base domain: local.my-app.com
# - Enable demo mode: Yes
```

**What happens:**
- Creates `.env` file with your configuration
- Sets up project structure directories
- Configures all services (PostgreSQL, Hasura, Auth, Nginx, etc.)

**Verify:**
```bash
ls -la
# You should see: .env, docker-compose.yml (placeholder), etc.
```

---

### Step 2: Build Configuration

```bash
nself build
```

**What happens:**
- Generates `docker-compose.yml` with all services
- Creates Nginx reverse proxy configuration
- Generates SSL certificates
- Sets up database initialization scripts
- Creates service-specific configurations

**Verify:**
```bash
ls -la
# You should see: docker-compose.yml, nginx/, ssl/, postgres/
docker compose config --services
# Should list all services
```

---

### Step 3: Start Services

```bash
nself start
```

**What happens:**
- Starts all Docker containers
- Waits for health checks
- Shows service status
- Reports any issues

**Expected output:**
```
✓ Starting services...
✓ PostgreSQL      (healthy)
✓ Hasura          (healthy)
✓ Auth            (healthy)
✓ Nginx           (healthy)

✓ 20/20 services running
✓ Health checks: 17/19 passing

🎉 All services started successfully!
```

**Verify:**
```bash
nself status
# All services should show as running

nself urls
# Should list all service URLs
```

---

### Step 4: Setup Authentication

This is the **critical step** that the old workflow was missing!

```bash
nself auth setup --default-users
```

**What happens:**
1. Checks PostgreSQL is running
2. Applies Hasura metadata (tracks auth.users, auth.user_providers, etc.)
3. Creates 3 staff users:
   - `owner@nself.org` (role: owner)
   - `admin@nself.org` (role: admin)
   - `support@nself.org` (role: support)
4. All with password: `npass123` (development only!)
5. Verifies auth service can query users

**Expected output:**
```
ℹ Auth Setup Wizard

ℹ Checking Hasura metadata...
✓ Hasura metadata configured
ℹ Creating default users...
✓ User created: owner@nself.org
ℹ User ID: 11111111-1111-1111-1111-111111111111
✓ User created: admin@nself.org
ℹ User ID: 22222222-2222-2222-2222-222222222222
✓ User created: support@nself.org
ℹ User ID: 33333333-3333-3333-3333-333333333333
✓ Created 3 default users (password: npass123)
ℹ Verifying auth service...
✓ Auth service configured correctly!

✓ Auth setup complete!

ℹ Next steps:
  - Test login: curl -k https://auth.local.nself.org/signin/email-password
  - Create more users: nself auth create-user user@example.com
  - List users: nself auth list-users
```

**Manual alternative:**
```bash
# Interactive mode (prompts for confirmation)
nself auth setup
```

---

### Step 5: Verify Authentication Works

```bash
# Test login via API
curl -k -X POST https://auth.local.nself.org/signin/email-password \
  -H "Content-Type: application/json" \
  -d '{
    "email": "owner@nself.org",
    "password": "npass123"
  }'
```

**Expected response:**
```json
{
  "session": {
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "...",
    "expires_in": 900,
    "user": {
      "id": "11111111-1111-1111-1111-111111111111",
      "email": "owner@nself.org",
      "displayName": "Platform Owner",
      "metadata": {
        "role": "owner"
      }
    }
  }
}
```

**Test GraphQL access:**
```bash
# Query users via Hasura
curl -X POST http://localhost:8080/v1/graphql \
  -H "x-hasura-admin-secret: $HASURA_GRAPHQL_ADMIN_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"query":"{ users { id display_name metadata } }"}'
```

**Expected response:**
```json
{
  "data": {
    "users": [
      {
        "id": "11111111-1111-1111-1111-111111111111",
        "display_name": "Platform Owner",
        "metadata": {"role": "owner"}
      },
      {
        "id": "22222222-2222-2222-2222-222222222222",
        "display_name": "Administrator",
        "metadata": {"role": "admin"}
      },
      {
        "id": "33333333-3333-3333-3333-333333333333",
        "display_name": "Support Staff",
        "metadata": {"role": "support"}
      }
    ]
  }
}
```

---

## Additional Workflows

### Creating More Users

```bash
# Interactive mode
nself auth create-user

# Non-interactive with flags
nself auth create-user \
  --email=newuser@example.com \
  --password=SecurePass123! \
  --role=user \
  --name="New User"
```

### Listing All Users

```bash
nself auth list-users
```

Output:
```
 id                                   | email              | display_name      | role    | email_verified | disabled | created_at
--------------------------------------+--------------------+-------------------+---------+----------------+----------+----------------------------
 11111111-1111-1111-1111-111111111111 | owner@nself.org    | Platform Owner    | owner   | t              | f        | 2026-02-11 10:00:00.000000
 22222222-2222-2222-2222-222222222222 | admin@nself.org    | Administrator     | admin   | t              | f        | 2026-02-11 10:00:01.000000
 33333333-3333-3333-3333-333333333333 | support@nself.org  | Support Staff     | support | t              | f        | 2026-02-11 10:00:02.000000
```

### Applying Custom Seeds

```bash
# Create a seed file
nself db seed create my_data local

# Edit the file: nself/seeds/local/001_my_data.sql
# Add your INSERT statements

# Apply all seeds
nself db seed apply

# List seed status
nself db seed list
```

### Viewing Logs

```bash
# View logs for specific service
nself logs auth

# Follow logs in real-time
nself logs auth --follow

# View logs for all services
nself logs --all
```

### Database Operations

```bash
# Interactive PostgreSQL shell
nself exec postgres psql -U postgres -d myapp_db

# Run SQL query
nself db query "SELECT * FROM auth.users"

# Database backup
nself db backup

# Database migration
nself db migrate up
```

### Hasura Management

```bash
# Track auth tables (done automatically by auth setup)
nself hasura track schema auth

# Export current metadata
nself hasura metadata export

# Reload metadata
nself hasura metadata reload

# Open Hasura console
nself hasura console
```

---

## Development Cycle

### Making Changes

```bash
# 1. Make changes to code, configuration, etc.
vim .env

# 2. Rebuild if configuration changed
nself build

# 3. Restart affected services
nself restart

# Or restart specific service
nself restart postgres
```

### Testing Changes

```bash
# Check service health
nself status

# View logs for debugging
nself logs <service>

# Test endpoints
nself urls
```

### Resetting Environment

```bash
# Stop services
nself stop

# Complete reset (removes containers, volumes, networks)
nself destroy

# Start fresh
nself build && nself start
```

---

## Connecting Frontend Applications

### Option 1: External Frontend (Next.js, React, etc.)

```bash
# Add to .env
FRONTEND_APP_1_NAME=webapp
FRONTEND_APP_1_PORT=3000
FRONTEND_APP_1_ROUTE=app

# Rebuild nginx config
nself build

# Restart nginx
nself restart nginx

# Frontend will be available at: https://app.local.nself.org
```

### Option 2: Frontend as Custom Service

```bash
# Add to .env
CS_1=frontend:next-js:3000

# Rebuild
nself build && nself start
```

### Using Auth in Frontend

```javascript
// Example: Login from frontend
const response = await fetch('https://auth.local.nself.org/signin/email-password', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({
    email: 'owner@nself.org',
    password: 'npass123'
  })
});

const {session} = await response.json();
const accessToken = session.access_token;

// Use token for GraphQL requests
const data = await fetch('https://api.local.nself.org/v1/graphql', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${accessToken}`
  },
  body: JSON.stringify({
    query: '{ users { id display_name } }'
  })
});
```

---

## Troubleshooting Common Issues

### Auth Service Returns 500 Error

**Error:**
```json
{
  "status": 500,
  "message": "field 'users' not found in type: 'query_root'"
}
```

**Solution:**
```bash
# Hasura hasn't tracked auth tables
nself auth setup
# or manually:
nself hasura track schema auth
nself hasura metadata reload
```

### Services Not Starting

```bash
# Check Docker status
docker ps

# Check for port conflicts
lsof -i :80
lsof -i :443

# View detailed logs
nself logs --all

# Try fresh start
nself stop && nself start
```

### Can't Connect to Services

```bash
# Verify services are running
nself status

# Check URLs
nself urls

# Test local DNS
ping api.local.nself.org

# On macOS, might need to add to /etc/hosts:
# 127.0.0.1 api.local.nself.org auth.local.nself.org
```

### Seed Files Not Applying

```bash
# Check seed file location
ls -la nself/seeds/common/
ls -la nself/seeds/local/

# Check seed tracking
nself db seed list

# Force reapply (remove tracking first)
nself exec postgres psql -U postgres -d myapp_db -c "DELETE FROM nself_seeds WHERE filename = '001_auth_users.sql'"
nself db seed apply
```

### Database Connection Issues

```bash
# Check PostgreSQL running
nself status | grep postgres

# Check database exists
nself exec postgres psql -U postgres -l

# Recreate database
nself db reset  # ⚠️ Destructive!
```

---

## Production Deployment

### Before Going to Production

1. **Change all default passwords:**
```bash
# Create production users with strong passwords
nself auth create-user \
  --email=admin@yourdomain.com \
  --password=$(openssl rand -base64 24) \
  --role=owner
```

2. **Update environment variables:**
```bash
# Edit .env for production
ENV=production
BASE_DOMAIN=yourdomain.com
AUTH_DEFAULT_PASSWORD=  # Remove default!
```

3. **Use real SSL certificates:**
```bash
# Replace self-signed certs
nself auth ssl install /path/to/production-cert.pem
```

4. **Enable monitoring:**
```bash
# Add to .env
MONITORING_ENABLED=true
nself build && nself start
```

5. **Set up backups:**
```bash
# Configure automated backups
nself db backup --schedule daily
```

---

## Best Practices

### Security
- ✅ Always change default passwords
- ✅ Use environment-specific `.env` files
- ✅ Never commit `.env` to git
- ✅ Use strong passwords in production
- ✅ Enable rate limiting
- ✅ Use real SSL certificates

### Development
- ✅ Use `nself db seed apply` for test data
- ✅ Version control your seed files
- ✅ Use migrations for schema changes
- ✅ Test with realistic data volumes
- ✅ Monitor logs during development

### Deployment
- ✅ Test in staging first
- ✅ Use environment-aware seeding
- ✅ Enable monitoring and alerting
- ✅ Configure automated backups
- ✅ Document custom configurations

---

## Next Steps

- Read [AUTH_SETUP.md](./AUTH_SETUP.md) for detailed auth information
- Read [SEEDING.md](./SEEDING.md) for advanced seeding patterns
- Explore [Command Reference](../commands/) for all available commands
- Check [Troubleshooting Guide](../troubleshooting/) for common issues

---

**Questions? Issues?**
- GitHub Issues: https://github.com/nself-org/cli/issues
- Documentation: https://docs.nself.org
- Discord: [Coming soon]
