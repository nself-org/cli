# Your First nself Project

Welcome to nself! This guide walks you through creating your first project from start to finish.

## Prerequisites

- nself CLI installed ([Installation Guide](../getting-started/Installation.md))
- Docker and Docker Compose
- Basic command line knowledge
- 5-10 minutes

## Step 1: Initialize Your Project

Create a new directory and initialize nself:

```bash
mkdir my-first-app
cd my-first-app
nself init
```

This launches the interactive wizard that will ask you:

- **Project Name** - The name of your application (e.g., "My Blog")
- **Environment** - Usually `dev` for local development
- **Base Domain** - Usually `localhost` for development
- **Services** - Which optional services you want (Redis, MinIO, etc.)

### Example Initialization

```bash
$ nself init
Welcome to nself!

Project name: my-blog
Environment: dev
Base domain: localhost
Enable Redis? (y/n): y
Enable MinIO? (y/n): n
Enable monitoring? (y/n): n
```

## Step 2: Build Your Configuration

After initialization, build the Docker Compose configuration:

```bash
nself build
```

This generates:
- `docker-compose.yml` - All your services
- `.env` - Configuration file
- `nginx/` - Web server configuration
- `services/` - Custom service templates (if any)

## Step 3: Start Your Services

Launch all services:

```bash
nself start
```

You'll see output like:

```
Starting nself services...
Creating network nself_network
Creating postgresql container... ✓
Creating hasura container... ✓
Creating auth container... ✓
Creating nginx container... ✓
Creating redis container... ✓

All services started successfully!
```

## Step 4: Access Your Application

Once services are running, check what's available:

```bash
nself urls
```

This shows all your service endpoints:

- **GraphQL API**: `http://localhost:8080/graphql`
- **Authentication**: `http://localhost:8080/auth`
- **Admin Panel**: `http://localhost:8080/admin`
- **Redis** (if enabled): `localhost:6379`

### Default Credentials

- **Hasura Admin Secret**: Check `.env` for `HASURA_GRAPHQL_ADMIN_SECRET`
- **Auth Admin**: Default setup in auth service
- **Database**: User `postgres`, password in `.env`

## Step 5: Explore the Services

### Open Hasura Console

```bash
nself admin
```

This opens the Hasura console where you can:
- Create database tables
- Set up GraphQL permissions
- Write migrations
- Test queries

### View Logs

```bash
nself logs
```

Or follow logs from a specific service:

```bash
nself logs postgres -f     # Follow PostgreSQL logs
nself logs hasura -f       # Follow Hasura logs
```

## Common Next Steps

### 1. Create Your First Table

In the Hasura console:

```sql
CREATE TABLE posts (
  id SERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  content TEXT,
  created_at TIMESTAMP DEFAULT NOW()
);
```

### 2. Add GraphQL Permissions

Set up row-level security for your users:

```
posts table:
- admins can: SELECT, INSERT, UPDATE, DELETE
- users can: SELECT their own posts
- public: SELECT only published posts
```

### 3. Build a Custom Service

Add your API in `.env`:

```bash
CS_1=my-api:express-js:8001
```

Then rebuild:

```bash
nself build
nself restart my-api
```

Edit your service code in `services/my-api/`.

### 4. Enable Monitoring

See how your application performs:

```bash
# Enable monitoring in .env
MONITORING_ENABLED=true

# Rebuild and restart
nself build
nself restart

# View Grafana dashboards
nself monitor
```

## Troubleshooting

### Port Already in Use

If port 8080 is already in use:

```bash
# Change in .env
NGINX_PORT=8081

# Rebuild
nself build
nself restart
```

### Database Connection Issues

```bash
# Check database health
nself health

# View database logs
nself logs postgres

# Test database connection
nself db console
```

### Services Won't Start

```bash
# Try fresh start
nself start --fresh

# Or clean everything and restart
nself backup create backup-before-fresh
nself start --clean-start
```

## Next: Building Real Applications

Now that you have nself running, check out:

- [Core Concepts](../getting-started/CONCEPTS.md) - Understand multi-tenancy, RLS, etc.
- [Authentication Guide](../guides/AUTHENTICATION.md) - Set up users and permissions
- [Database Guide](../guides/DATABASE-WORKFLOW.md) - Advanced database features
- [Deployment Guide](../guides/DEPLOYMENT-ARCHITECTURE.md) - Deploy to production

## Getting Help

- **Documentation**: `nself help`
- **Command Help**: `nself help <command>`
- **Health Check**: `nself doctor`
- **Community**: [GitHub Issues](https://github.com/nself-org/cli/issues)

---

**Congratulations!** You've successfully created your first nself project. Start building!
