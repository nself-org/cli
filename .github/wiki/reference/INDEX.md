# Reference Documentation

Quick reference guides and cheat sheets for ɳSelf.

## Overview

This section contains quick reference materials, cheat sheets, and comprehensive API documentation.

## Quick References

- **[Command Reference](COMMAND-REFERENCE.md)** - Complete command cheat sheet
- **[Quick Navigation](QUICK-NAVIGATION.md)** - Fast navigation guide
- **[Service Templates](SERVICE_TEMPLATES.md)** - All available service templates
- **[Service Scaffolding Cheatsheet](SERVICE-SCAFFOLDING-CHEATSHEET.md)** - Service creation guide

## API Reference

- **[API Documentation](api/README.md)** - Complete API reference
- **[Billing API](api/BILLING-API.md)** - Billing and subscription endpoints
- **[White-Label API](api/WHITE-LABEL-API.md)** - Branding and customization endpoints

## Command Quick Reference

### Core Commands
```bash
nself init          # Initialize project
nself build         # Generate configs
nself start         # Start services
nself stop          # Stop services
nself restart       # Restart services
nself status        # Check status
```

### Database Commands
```bash
nself db migrate up             # Run migrations
nself db seed                   # Seed database
nself db mock                   # Generate mock data
nself db backup                 # Backup database
nself db schema scaffold saas   # Create schema template
nself db types                  # Generate TypeScript types
```

### Multi-Tenant Commands
```bash
nself tenant create "Acme"      # Create tenant
nself tenant billing usage      # View usage
nself tenant org list           # List organizations
nself tenant domains add        # Add custom domain
nself tenant branding set       # Set branding
```

### Deployment Commands
```bash
nself env create prod           # Create environment
nself deploy prod               # Deploy to production
nself deploy sync pull          # Sync configurations
```

See **[Command Reference](COMMAND-REFERENCE.md)** for complete list.

## Service Templates Quick Reference

### JavaScript/TypeScript

| Template | Description | Port |
|----------|-------------|------|
| `express-js` | Express.js API | 8001 |
| `fastify-js` | Fastify API | 8002 |
| `nestjs-ts` | NestJS framework | 8003 |
| `bullmq-js` | Queue worker | N/A |
| `hono-js` | Hono edge runtime | 8004 |

### Python

| Template | Description | Port |
|----------|-------------|------|
| `fastapi-py` | FastAPI framework | 8001 |
| `flask-py` | Flask framework | 8002 |
| `django-py` | Django framework | 8003 |
| `celery-py` | Task queue | N/A |

### Go

| Template | Description | Port |
|----------|-------------|------|
| `gin-go` | Gin framework | 8001 |
| `fiber-go` | Fiber framework | 8002 |
| `echo-go` | Echo framework | 8003 |
| `grpc-go` | gRPC service | 50051 |

See **[Service Templates](SERVICE_TEMPLATES.md)** for complete list.

## Configuration Quick Reference

### Environment Variables

**Required Services:**
```bash
POSTGRES_DB=myapp
POSTGRES_USER=postgres
POSTGRES_PASSWORD=secure
HASURA_GRAPHQL_ADMIN_SECRET=secret
```

**Optional Services:**
```bash
REDIS_ENABLED=true
MINIO_ENABLED=true
NSELF_ADMIN_ENABLED=true
MONITORING_ENABLED=true
```

**Custom Services:**
```bash
CS_1=api:express-js:8001
CS_2=worker:bullmq-js:8002
CS_3=ml-api:fastapi-py:8003
```

## GraphQL Quick Reference

### Queries
```graphql
# Fetch users
query {
  users { id email }
}

# With filters
query {
  users(where: {email: {_like: "%@example.com"}}) {
    id
    email
  }
}

# With pagination
query {
  users(limit: 10, offset: 0) {
    id
    email
  }
}
```

### Mutations
```graphql
# Insert
mutation {
  insert_users_one(object: {email: "user@example.com"}) {
    id
  }
}

# Update
mutation {
  update_users_by_pk(
    pk_columns: {id: "uuid"}
    _set: {email: "new@example.com"}
  ) {
    id
  }
}

# Delete
mutation {
  delete_users_by_pk(id: "uuid") {
    id
  }
}
```

### Subscriptions
```graphql
# Real-time updates
subscription {
  users {
    id
    email
  }
}
```

## Database Schema Quick Reference

### Common Patterns

**Timestamps:**
```sql
created_at TIMESTAMP DEFAULT NOW()
updated_at TIMESTAMP DEFAULT NOW()
```

**Soft Deletes:**
```sql
deleted_at TIMESTAMP
is_deleted BOOLEAN DEFAULT FALSE
```

**UUID Primary Keys:**
```sql
id UUID PRIMARY KEY DEFAULT gen_random_uuid()
```

**Foreign Keys:**
```sql
user_id UUID REFERENCES users(id) ON DELETE CASCADE
```

**Indexes:**
```sql
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_created_at ON users(created_at DESC);
```

## Deployment Quick Reference

### Production Checklist

```bash
# 1. Environment setup
nself env create prod production

# 2. Configure server
# Edit .environments/prod/server.json

# 3. Security
- [ ] Strong passwords
- [ ] SSL enabled
- [ ] Secrets configured
- [ ] Firewall rules

# 4. Deploy
nself deploy prod

# 5. Verify
nself status --env prod
nself health --env prod
```

## Monitoring Quick Reference

### Accessing Dashboards

```bash
# View all URLs
nself urls

# Grafana: http://grafana.local.nself.org
# Prometheus: http://prometheus.local.nself.org
# Alertmanager: http://alertmanager.local.nself.org
```

### Common Metrics

**System:**
- CPU usage
- Memory usage
- Disk I/O
- Network traffic

**Database:**
- Active connections
- Query performance
- Table sizes
- Index usage

**Application:**
- Request rate
- Error rate
- Response time
- Queue depth

## Troubleshooting Quick Reference

### Common Issues

**Services won't start:**
```bash
nself doctor
nself logs [service]
docker ps -a
```

**Port conflicts:**
```bash
nself stop
lsof -i :PORT
nself start
```

**Database issues:**
```bash
nself db reset
nself db migrate up
nself db seed
```

**Permission errors:**
```bash
chmod 600 .env
chmod 755 services/
```

See **[Troubleshooting Guide](../guides/TROUBLESHOOTING.md)** for complete list.

## Related Documentation

### Full Documentation
- [Complete Documentation](../README.md)
- [Getting Started](../getting-started/README.md)
- [Command Reference](../commands/COMMANDS.md)

### Detailed Guides
- [Database Workflow](../guides/DATABASE-WORKFLOW.md)
- [Deployment Guide](../deployment/README.md)
- [Configuration Guide](../configuration/README.md)

### Examples
- [Examples Index](../examples/README.md)
- [Tutorials](../tutorials/README.md)

---

**[← Back to Documentation Home](../README.md)**
