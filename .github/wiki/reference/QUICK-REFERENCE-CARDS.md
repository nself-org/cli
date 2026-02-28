# nself Quick Reference Cards

Printable cheat sheets for common nself operations.

---

## Card 1: Essential Commands

```
┌────────────────────────────────────────────────────────────┐
│                  nself ESSENTIAL COMMANDS                  │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  INITIALIZATION                                            │
│  ──────────────                                            │
│  nself init                  # Initialize new project     │
│  nself init --demo           # Initialize with demo config│
│  nself build                 # Generate infrastructure    │
│  nself start                 # Start all services         │
│  nself stop                  # Stop all services          │
│  nself restart               # Restart all services       │
│                                                            │
│  STATUS & MONITORING                                       │
│  ────────────────                                          │
│  nself status                # Service health status      │
│  nself logs                  # View all logs              │
│  nself logs [service]        # View specific service logs │
│  nself urls                  # Show all service URLs      │
│  nself health                # Run health checks          │
│                                                            │
│  DATABASE                                                  │
│  ────────                                                  │
│  nself db query "SQL"        # Execute SQL query          │
│  nself db migrate apply      # Run migrations             │
│  nself db backup             # Create database backup     │
│  nself db restore [file]     # Restore from backup        │
│                                                            │
│  SERVICES                                                  │
│  ────────                                                  │
│  nself admin hasura          # Open Hasura Console        │
│  nself admin minio           # Open MinIO Console         │
│  nself monitor               # Open monitoring dashboards │
│                                                            │
│  DEPLOYMENT                                                │
│  ──────────                                                │
│  nself deploy push [env]     # Deploy to environment      │
│  nself deploy logs [env]     # View remote logs           │
│  nself deploy exec [env] "cmd"  # Execute remote command  │
│                                                            │
│  HELP                                                      │
│  ────                                                      │
│  nself help                  # Show help                  │
│  nself version               # Show version               │
│  nself doctor                # Run diagnostics            │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

---

## Card 2: Database Operations

```
┌────────────────────────────────────────────────────────────┐
│              nself DATABASE OPERATIONS                     │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  QUERIES                                                   │
│  ───────                                                   │
│  nself db query "SELECT * FROM users"                      │
│  nself db query "SELECT * FROM users" --output json        │
│  nself db execute --file schema.sql                        │
│  nself db psql                       # Interactive shell  │
│                                                            │
│  MIGRATIONS                                                │
│  ──────────                                                │
│  nself db migrate create [name]      # Create migration   │
│  nself db migrate apply              # Apply migrations   │
│  nself db migrate rollback           # Rollback last      │
│  nself db migrate status             # Migration status   │
│                                                            │
│  BACKUPS                                                   │
│  ───────                                                   │
│  nself backup create [name]          # Create backup      │
│  nself backup list                   # List backups       │
│  nself backup restore [name]         # Restore backup     │
│  nself backup clean --older-than 30d # Remove old backups │
│                                                            │
│  TENANTS                                                   │
│  ───────                                                   │
│  nself tenant create [name]          # Create tenant      │
│  nself tenant list                   # List all tenants   │
│  nself tenant switch [id]            # Switch context     │
│  nself tenant delete [id]            # Delete tenant      │
│                                                            │
│  MAINTENANCE                                               │
│  ───────────                                               │
│  nself db vacuum                     # Vacuum database    │
│  nself db analyze                    # Update statistics  │
│  nself db reindex                    # Rebuild indexes    │
│  nself db size                       # Show database size │
│                                                            │
│  UTILITIES                                                 │
│  ─────────                                                 │
│  nself db dump > backup.sql          # Export database    │
│  nself db import backup.sql          # Import database    │
│  nself db reset --confirm            # Reset database     │
│  nself db seed                       # Run seed data      │
│                                                            │
│  CONNECTION                                                │
│  ──────────                                                │
│  Connection String (internal):                            │
│  postgresql://postgres:password@postgres:5432/dbname      │
│                                                            │
│  Connection String (external):                            │
│  postgresql://postgres:password@localhost:5432/dbname     │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

---

## Card 3: Deployment Workflow

```
┌────────────────────────────────────────────────────────────┐
│               nself DEPLOYMENT WORKFLOW                    │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  INITIAL SETUP                                             │
│  ─────────────                                             │
│  1. Create server.json with connection info:              │
│     {                                                      │
│       "production": {                                      │
│         "host": "your-server.com",                         │
│         "user": "root",                                    │
│         "port": 22                                         │
│       }                                                    │
│     }                                                      │
│                                                            │
│  2. Test connection:                                       │
│     ssh root@your-server.com                               │
│                                                            │
│  FIRST DEPLOYMENT                                          │
│  ────────────────                                          │
│  nself deploy provision --server your-server.com           │
│    # Installs Docker, Docker Compose, sets up firewall    │
│                                                            │
│  nself deploy push production                              │
│    # Copies files, builds services, starts containers     │
│                                                            │
│  nself deploy exec production "nself db migrate apply"     │
│    # Run migrations on server                             │
│                                                            │
│  SSL SETUP                                                 │
│  ─────────                                                 │
│  nself deploy exec production \                            │
│    "nself auth ssl cert --domain yourdomain.com \          │
│     --email your@email.com"                                │
│                                                            │
│  SUBSEQUENT DEPLOYMENTS                                    │
│  ──────────────────────                                    │
│  # Make code changes locally                              │
│  git add .                                                 │
│  git commit -m "Feature: xyz"                              │
│                                                            │
│  # Test locally                                            │
│  nself build                                               │
│  nself start                                               │
│                                                            │
│  # Deploy to production                                    │
│  nself deploy push production                              │
│                                                            │
│  # Run migrations if needed                                │
│  nself deploy exec production "nself db migrate apply"     │
│                                                            │
│  # Verify deployment                                       │
│  nself deploy exec production "nself health"               │
│  nself deploy logs production --tail 100                   │
│                                                            │
│  ROLLBACK                                                  │
│  ────────                                                  │
│  nself backup restore [backup-name]                        │
│  nself deploy exec production "nself restart"              │
│                                                            │
│  MONITORING                                                │
│  ──────────                                                │
│  nself deploy exec production "nself status"               │
│  nself deploy exec production "nself logs --tail 1000"     │
│  https://grafana.yourdomain.com                            │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

---

## Card 4: Troubleshooting Guide

```
┌────────────────────────────────────────────────────────────┐
│             nself TROUBLESHOOTING GUIDE                    │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  SERVICES WON'T START                                      │
│  ────────────────────                                      │
│  1. Check service status:                                  │
│     nself status                                           │
│                                                            │
│  2. View detailed logs:                                    │
│     nself logs --verbose                                   │
│     nself logs [service-name]                              │
│                                                            │
│  3. Check for port conflicts:                              │
│     docker ps -a                                           │
│     lsof -i :[port]                                        │
│                                                            │
│  4. Restart services:                                      │
│     nself stop                                             │
│     nself start --fresh                                    │
│                                                            │
│  DATABASE CONNECTION ERRORS                                │
│  ──────────────────────────                                │
│  1. Verify database is running:                            │
│     nself status postgres                                  │
│                                                            │
│  2. Test connection:                                       │
│     nself db query "SELECT 1"                              │
│                                                            │
│  3. Check credentials in .env:                             │
│     POSTGRES_DB, POSTGRES_USER, POSTGRES_PASSWORD          │
│                                                            │
│  4. View database logs:                                    │
│     nself logs postgres                                    │
│                                                            │
│  HASURA ERRORS                                             │
│  ─────────────                                             │
│  1. Check Hasura logs:                                     │
│     nself logs hasura                                      │
│                                                            │
│  2. Verify admin secret:                                   │
│     HASURA_GRAPHQL_ADMIN_SECRET in .env                    │
│                                                            │
│  3. Reload metadata:                                       │
│     nself service hasura metadata reload                   │
│                                                            │
│  4. Check database connection:                             │
│     Visit Hasura Console → Data → View Database            │
│                                                            │
│  SSL/HTTPS ISSUES                                          │
│  ────────────────                                          │
│  1. Check certificate status:                              │
│     nself auth ssl status                                  │
│                                                            │
│  2. Verify nginx config:                                   │
│     nself config validate                                  │
│                                                            │
│  3. Check nginx logs:                                      │
│     nself logs nginx                                       │
│                                                            │
│  4. Renew certificate:                                     │
│     nself auth ssl renew                                   │
│                                                            │
│  PERFORMANCE ISSUES                                        │
│  ──────────────────                                        │
│  1. Run diagnostics:                                       │
│     nself doctor                                           │
│                                                            │
│  2. Check resource usage:                                  │
│     docker stats                                           │
│                                                            │
│  3. Analyze slow queries:                                  │
│     nself db slow-queries                                  │
│                                                            │
│  4. Review metrics:                                        │
│     Visit Grafana dashboard                                │
│                                                            │
│  OUT OF DISK SPACE                                         │
│  ─────────────────                                         │
│  1. Check disk usage:                                      │
│     df -h                                                  │
│     du -sh /var/lib/docker                                 │
│                                                            │
│  2. Clean Docker resources:                                │
│     docker system prune -a                                 │
│                                                            │
│  3. Remove old backups:                                    │
│     nself backup clean --older-than 30d                    │
│                                                            │
│  4. Clean logs:                                            │
│     nself logs --clean --older-than 7d                     │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

---

## Card 5: Security Checklist

```
┌────────────────────────────────────────────────────────────┐
│              nself SECURITY CHECKLIST                      │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  BEFORE PRODUCTION                                         │
│  ─────────────────                                         │
│  □ Changed all default passwords                          │
│  □ Generated strong secrets (32+ characters)              │
│  □ SSL/HTTPS enabled                                       │
│  □ Firewall configured (only ports 80, 443, 22)           │
│  □ Database not exposed to internet                       │
│  □ Admin endpoints secured                                │
│  □ Rate limiting enabled                                  │
│  □ CORS configured correctly                              │
│  □ Content Security Policy set                            │
│  □ Backups automated and tested                           │
│                                                            │
│  ENVIRONMENT VARIABLES                                     │
│  ─────────────────────                                     │
│  Critical secrets to change:                              │
│                                                            │
│  POSTGRES_PASSWORD                                         │
│    Generate: openssl rand -base64 32                       │
│                                                            │
│  HASURA_GRAPHQL_ADMIN_SECRET                               │
│    Generate: openssl rand -base64 32                       │
│                                                            │
│  AUTH_JWT_SECRET                                           │
│    Generate: openssl rand -base64 32                       │
│                                                            │
│  MINIO_ROOT_PASSWORD                                       │
│    Generate: openssl rand -base64 24                       │
│                                                            │
│  REDIS_PASSWORD                                            │
│    Generate: openssl rand -base64 24                       │
│                                                            │
│  FIREWALL RULES                                            │
│  ──────────────                                            │
│  nself auth firewall enable                                │
│  nself auth firewall allow 22/tcp    # SSH                 │
│  nself auth firewall allow 80/tcp    # HTTP                │
│  nself auth firewall allow 443/tcp   # HTTPS               │
│  nself auth firewall status                                │
│                                                            │
│  SSL/TLS                                                   │
│  ───────                                                   │
│  nself auth ssl cert \                                     │
│    --domain yourdomain.com \                               │
│    --email your@email.com                                  │
│                                                            │
│  nself auth ssl renew              # Renew certificate     │
│  nself auth ssl status             # Check status          │
│                                                            │
│  SECURITY AUDIT                                            │
│  ──────────────                                            │
│  nself auth security audit         # Run security scan    │
│  nself auth security report        # Generate report      │
│                                                            │
│  REGULAR MAINTENANCE                                       │
│  ───────────────────                                       │
│  Weekly:                                                   │
│  - Review access logs                                      │
│  - Check for failed login attempts                         │
│  - Verify backups are working                             │
│                                                            │
│  Monthly:                                                  │
│  - Update services (nself update)                          │
│  - Review user permissions                                │
│  - Rotate API keys                                         │
│  - Security audit                                          │
│                                                            │
│  Quarterly:                                                │
│  - Review and update firewall rules                        │
│  - Penetration testing                                     │
│  - Disaster recovery drill                                │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

---

## Card 6: Common Configuration Patterns

```
┌────────────────────────────────────────────────────────────┐
│         nself CONFIGURATION PATTERNS                       │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  MINIMAL BLOG                                              │
│  ────────────                                              │
│  PROJECT_NAME=blog                                         │
│  POSTGRES_DB=blog_db                                       │
│  HASURA_GRAPHQL_ADMIN_SECRET=secret                        │
│  AUTH_JWT_SECRET=jwt-secret-32-chars                       │
│  MAILPIT_ENABLED=true                                      │
│                                                            │
│  Services: postgres, hasura, auth, nginx, mailpit         │
│  Total Containers: 5                                       │
│                                                            │
│  SAAS APPLICATION                                          │
│  ────────────────                                          │
│  PROJECT_NAME=my-saas                                      │
│  POSTGRES_DB=saas_db                                       │
│  REDIS_ENABLED=true                                        │
│  MINIO_ENABLED=true                                        │
│  MEILISEARCH_ENABLED=true                                  │
│  MONITORING_ENABLED=true                                   │
│                                                            │
│  # Custom services                                         │
│  CS_1=api:nestjs-api:8001                                  │
│  CS_2=worker:bullmq-js:8002                                │
│                                                            │
│  Services: postgres, hasura, auth, redis, minio,           │
│           meilisearch, nginx, + monitoring (10),           │
│           + custom (2)                                     │
│  Total Containers: 19                                      │
│                                                            │
│  E-COMMERCE                                                │
│  ──────────                                                │
│  PROJECT_NAME=shop                                         │
│  REDIS_ENABLED=true         # Cart & sessions              │
│  MINIO_ENABLED=true         # Product images               │
│  MAILPIT_ENABLED=true       # Order emails                 │
│                                                            │
│  # Custom services                                         │
│  CS_1=checkout:nestjs-api:8001                             │
│  CS_2=inventory:python-api:8002                            │
│  CS_3=notifications:bullmq-js:8003                         │
│                                                            │
│  REALTIME CHAT                                             │
│  ─────────────                                             │
│  PROJECT_NAME=chat                                         │
│  REDIS_ENABLED=true         # Message queue                │
│  MINIO_ENABLED=true         # File uploads                 │
│  MEILISEARCH_ENABLED=true   # Message search               │
│                                                            │
│  # Custom services                                         │
│  CS_1=websocket:node-ws:8001                               │
│                                                            │
│  ML PLATFORM                                               │
│  ───────────                                               │
│  PROJECT_NAME=ml-platform                                  │
│  MINIO_ENABLED=true         # Dataset storage              │
│  MLFLOW_ENABLED=true        # Experiment tracking          │
│  REDIS_ENABLED=true         # Job queue                    │
│                                                            │
│  # Custom services                                         │
│  CS_1=training:python-api:8001                             │
│  CS_2=inference:python-api:8002                            │
│  CS_3=scheduler:bullmq-js:8003                             │
│                                                            │
│  MULTI-TENANT SAAS                                         │
│  ──────────────────                                        │
│  PROJECT_NAME=multi-tenant                                 │
│  REDIS_ENABLED=true         # Rate limiting                │
│  MINIO_ENABLED=true         # Per-tenant storage           │
│  MONITORING_ENABLED=true    # Per-tenant metrics           │
│                                                            │
│  # Custom services                                         │
│  CS_1=api:nestjs-api:8001                                  │
│  CS_2=billing:nestjs-api:8002                              │
│  CS_3=worker:bullmq-js:8003                                │
│                                                            │
│  # Tenant-specific configuration                           │
│  TENANT_ISOLATION=true                                     │
│  TENANT_DB_PREFIX=tenant_                                  │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

---

## Printing Instructions

### For Best Results

1. **Print in landscape mode**
2. **Use A4 or Letter paper**
3. **Print at 100% scale (no shrinking)**
4. **Use monospace font for code blocks**

### Laminate for Durability

Print on cardstock or laminate for a desk reference card that will last.

### Digital Version

Save as PDF for quick reference on mobile devices.

---

## Additional Resources

- **Full Documentation:** [docs.nself.org](https://github.com/nself-org/cli/wiki)
- **Command Reference:** [docs/commands/COMMAND-TREE-V1.md](../commands/COMMAND-TREE-V1.md)
- **Examples:** [Examples](../examples/README.md)
- **Tutorials:** [Tutorials](../tutorials/README.md)

---

**Version:** 0.9.8
**Last Updated:** January 2026
