# Frequently Asked Questions (FAQ)

**Version 0.9.9** | Comprehensive answers to common questions about ɳSelf

---

## Table of Contents

- [General Questions](#general-questions) - What is nself, how it compares, system requirements
- [Getting Started](#getting-started) - Installation, init, build, start, configuration
- [Multi-Tenancy](#multi-tenancy) - Tenant management, isolation, domains, branding
- [Billing & Usage](#billing--usage) - Stripe integration, subscriptions, invoicing

**See also:** [Troubleshooting Guide](../guides/TROUBLESHOOTING.md) | [Security Guide](../guides/SECURITY.md) | [Performance Guide](../performance/PERFORMANCE-OPTIMIZATION-V0.9.8.md)

---

## General Questions

### What is ɳSelf?

ɳSelf is a complete self-hosted Backend-as-a-Service (BaaS) platform that provides all the features of commercial services like Supabase, Nhost, or Firebase, but runs entirely on your own infrastructure using Docker Compose. It includes PostgreSQL, GraphQL API (Hasura), authentication, storage, functions, monitoring, and more - all configured and managed through a simple CLI.

You get a production-ready backend stack in minutes without vendor lock-in, subscription fees, or data ownership concerns. Everything runs on your servers, whether that's your laptop for development or cloud infrastructure for production.

**[View Architecture Documentation](../architecture/ARCHITECTURE.md)** | **[Quick Start Guide](Quick-Start.md)**

---

### How is it different from Supabase/Nhost/Firebase?

The key difference is **complete ownership and control**:

| Feature | ɳSelf | Supabase | Nhost | Firebase |
|---------|-------|----------|-------|----------|
| Self-hosted | Full control, any infrastructure | Limited (complex setup) | Limited (complex setup) | Cloud only |
| Vendor lock-in | None - standard tools | Partial | Partial | High |
| Data ownership | 100% yours | Shared infrastructure | Shared infrastructure | Google-owned |
| Cost model | Infrastructure only | Subscription tiers | Subscription tiers | Usage-based pricing |
| Customization | Unlimited (40+ service templates) | Limited | Limited | Very limited |
| Multi-tenancy | Built-in support | DIY | DIY | DIY |
| White-label | Full branding control | Limited | Limited | None |

ɳSelf uses industry-standard open-source tools (PostgreSQL, Hasura, MinIO, etc.) orchestrated through Docker Compose. This means you can migrate away easily, customize anything, and scale on your terms.

**[View Services Overview](../services/SERVICES.md)** | **[Compare Commercial BaaS](Quick-Start.md#what-you-get)**

---

### Is it production ready?

Yes. ɳSelf has been production-ready since v0.4.0 and includes:

- **Comprehensive security audit** with no critical vulnerabilities found
- **Cross-platform compatibility** tested on macOS (Bash 3.2+), Linux (all major distros), and WSL
- **12 automated CI/CD tests** including portability, unit tests, and integration tests
- **Zero-downtime deployments** with built-in health checks and rollback
- **Complete monitoring stack** (Prometheus, Grafana, Loki, Tempo, Alertmanager)
- **Production deployment tools** including SSL/TLS, backups, and environment management

Current version 0.9.8 has been thoroughly tested and is being used in production environments. The platform is stable and receives regular security updates.

**[View Security Audit](../security/SECURITY-AUDIT.md)** | **[Deployment Guide](../guides/Deployment.md)**

---

### What are the system requirements?

**Minimum (basic functionality):**
- 2 CPU cores
- 4 GB RAM
- 10 GB storage
- Docker Desktop 4.0+ (includes Docker Compose v2)
- macOS 10.15+, Linux (any recent distro), or Windows with WSL2

**Recommended (development with optional services):**
- 4 CPU cores
- 8 GB RAM
- 20 GB storage
- Fast SSD for database performance

**Production (full stack with monitoring):**
- 8+ CPU cores
- 16+ GB RAM
- 50+ GB storage (depending on data volume)
- Cloud instance (AWS, GCP, Azure, DigitalOcean, etc.)

The actual requirements depend on which services you enable. The required 4 services (PostgreSQL, Hasura, Auth, Nginx) run comfortably on the minimum specs.

**[View Demo Setup](../services/DEMO_SETUP.md)** | **[Resource Requirements](../services/SERVICES.md#resource-requirements)**

---

### Can I use it for commercial projects?

Yes. nself is released under the MIT License, which permits commercial use. You can:

- Build and sell SaaS applications
- Deploy for client projects
- Use in enterprise environments
- White-label and rebrand
- Modify and extend as needed

There are no usage limits, user caps, or subscription fees. You only pay for your infrastructure costs (server hosting, domain, etc.).

The only restriction is that you cannot sell nself itself as a competing platform product. You can absolutely use it to power your commercial applications.

**[View LICENSE](../LICENSE)** | **[White-Label Guide](#white-label)**

---

### How much does it cost?

nself itself is **free and open-source**. Your only costs are infrastructure:

**Local Development:** $0 (runs on your laptop)

**Basic Production (small SaaS):**
- $5-20/month - DigitalOcean/Linode droplet
- $10-15/month - Domain name
- $0 - Let's Encrypt SSL certificates
- **Total: ~$15-35/month**

**Medium Production (growing app):**
- $50-100/month - Larger cloud instance
- $20/month - Managed PostgreSQL backup
- $10/month - Domain + CDN
- **Total: ~$80-130/month**

**Enterprise Production:**
- $200-500/month - High-availability setup
- $50/month - Managed services
- Custom setup costs

Compare to Supabase Pro ($25/month per project) or Firebase ($25-100/month), but with nself you own everything and can run unlimited projects on the same infrastructure.

**[View Deployment Guide](../guides/Deployment.md)** | **[Cloud Provider Comparison](../guides/PROVIDERS-COMPLETE.md)**

---

### What services are included?

**Required Services (4):** Always running
- **PostgreSQL 16** - Primary database with 60+ extensions
- **Hasura GraphQL Engine** - Automatic GraphQL/REST API generation
- **Auth Service** - JWT-based authentication with OAuth support
- **Nginx** - Reverse proxy, SSL termination, routing

**Optional Services (7):** Enable as needed
- **nself Admin** - Web management dashboard
- **Redis** - Caching and session storage
- **MinIO** - S3-compatible object storage
- **Functions** - Serverless functions runtime
- **Mail** - Email service (MailPit for dev, SMTP for prod)
- **Search** - Full-text search (MeiliSearch, Typesense, Sonic)
- **MLflow** - ML experiment tracking and model registry

**Monitoring Bundle (10):** Full observability stack
- Prometheus, Grafana, Loki, Promtail, Tempo, Alertmanager
- cAdvisor, Node Exporter, Postgres Exporter, Redis Exporter

**Custom Services:** Unlimited
- Build from 40+ templates (Express, FastAPI, Django, Go, Rust, etc.)
- Or use any Docker image

**Plugins (3+):** Third-party integrations
- Stripe (billing), GitHub (DevOps), Shopify (e-commerce)

**[View Services Documentation](../services/SERVICES.md)** | **[Plugin System](../plugins/index.md)**

---

### What databases are supported?

nself uses **PostgreSQL 16** as the primary database with:

- **60+ PostgreSQL extensions** pre-installed (PostGIS, pgvector, pg_cron, etc.)
- **Row Level Security (RLS)** for multi-tenant data isolation
- **Full-text search** with tsvector and trigram indexes
- **JSONB support** for semi-structured data
- **Automatic GraphQL API** via Hasura

You can also:
- **Use external PostgreSQL** by setting `POSTGRES_EXTERNAL=true`
- **Connect to other databases** via Hasura remote schemas or custom services
- **Add Redis** for caching and sessions
- **Use MinIO** for file storage (S3-compatible)

PostgreSQL is the only relational database directly supported. For other databases (MySQL, MongoDB, etc.), you can create custom services that connect to them and expose APIs.

**[View Database Workflow](../guides/DATABASE-WORKFLOW.md)** | **[Database Commands](../commands/DB.md)**

---

### Does it support GraphQL and REST APIs?

Yes, both:

**GraphQL (via Hasura):**
- Automatic API generation from your database schema
- Queries, mutations, subscriptions (real-time)
- Relationships and nested queries
- Role-based access control (RBAC)
- Field-level permissions
- Custom business logic via Actions and Remote Schemas

**REST:**
- Hasura provides REST endpoints from GraphQL
- Custom REST APIs via service templates (Express, FastAPI, etc.)
- Standard HTTP methods (GET, POST, PUT, DELETE)
- OpenAPI/Swagger documentation available

**WebSocket/Real-time:**
- GraphQL subscriptions for real-time data
- WebSocket services from templates
- Server-Sent Events (SSE) support

**gRPC:**
- gRPC service templates available
- Can be exposed via Nginx with grpc_pass

**[View API Reference](../architecture/API.md)** | **[Custom Services](../services/SERVICES_CUSTOM.md)**

---

### Can I migrate from Supabase/Firebase?

Yes. Migration paths exist for both:

**From Supabase:**
1. Export PostgreSQL database (`pg_dump`)
2. Import to nself PostgreSQL
3. Migrate auth users (Supabase and nself both use standard JWT)
4. Replace Supabase client with Hasura client or direct GraphQL
5. Move storage files to MinIO

Supabase uses PostgreSQL and PostgREST, making data migration straightforward. Auth migration requires mapping Supabase's auth schema to nself's format.

**From Firebase:**
1. Export Firestore data to JSON
2. Transform to relational schema (use `nself db schema scaffold` as template)
3. Import to PostgreSQL via custom migration
4. Replace Firebase Auth with nself Auth
5. Migrate Cloud Storage to MinIO

Firebase migration is more complex due to NoSQL → SQL transformation, but nself's database-first workflow helps structure the new schema.

**Migration tools coming in v0.5.0** will automate much of this process.

**[View Database Tools](../commands/DB.md)** | **[Contact for Migration Support](https://github.com/acamarata/nself/discussions)**

---

### What programming languages are supported?

nself is language-agnostic. The platform provides GraphQL/REST APIs that work with any language. For custom services, 40+ templates are available:

**JavaScript/TypeScript:**
- Express.js, Fastify, NestJS, Hono
- Next.js, Nuxt.js, SvelteKit
- BullMQ workers, Socket.io

**Python:**
- FastAPI, Flask, Django, Starlette
- Celery workers, RQ workers
- Jupyter notebooks (via MLflow)

**Go:**
- Gin, Fiber, Echo, Chi
- gRPC services
- Goroutine workers

**Others:**
- Rust (Actix, Rocket)
- Java (Spring Boot, Quarkus)
- PHP (Laravel, Symfony)
- Ruby (Rails, Sinatra)
- C# (.NET, ASP.NET Core)
- Elixir (Phoenix)

Can't find your stack? Templates are just Dockerfiles with placeholders. Create your own or use any Docker image directly.

**[View Service Templates](../services/SERVICE-TEMPLATES.md)** | **[Custom Services Guide](../services/SERVICES_CUSTOM.md)**

---

### How do I update nself to the latest version?

**Method 1: Homebrew (macOS/Linux)**
```bash
brew update
brew upgrade nself
```

**Method 2: Manual Update**
```bash
cd ~/.nself
git pull origin main
```

**Method 3: Re-install**
```bash
curl -sSL https://install.nself.org | bash
```

**After updating:**
```bash
nself version  # Verify new version
cd ~/your-project
nself build    # Regenerate configs with new features
nself start    # Restart with new version
```

Your project configuration (`.env` files) and data are never modified during updates. Only the nself CLI and templates are updated.

**Check for updates:**
```bash
nself version --check
```

**[View Changelog](../releases/CHANGELOG.md)** | **[Release Notes](../releases/INDEX.md)**

---

### Where is data stored?

All data is stored in Docker volumes on your local machine or server:

**Database:**
- Volume: `{project}_postgres_data`
- Location: Docker's volume directory (usually `/var/lib/docker/volumes`)
- Contains: PostgreSQL data files

**Storage (MinIO):**
- Volume: `{project}_minio_data`
- Contains: Uploaded files (S3-compatible)

**Configuration:**
- Location: Your project directory
- Files: `.env`, `.env.local`, `docker-compose.yml`, `nginx/`, etc.

**Backups:**
- Location: `nself/backups/` in your project directory
- Format: SQL dumps (gzipped)

To backup everything:
```bash
# Database
nself db backup

# Copy entire project
cp -r ~/myproject ~/myproject-backup

# Or use deployment sync
nself deploy backup
```

Data never leaves your infrastructure unless you explicitly export or deploy it.

**[View Backup Guide](../guides/BACKUP_GUIDE.md)** | **[Architecture](../architecture/ARCHITECTURE.md)**

---

### Can I run multiple projects?

Yes. Each nself project is completely isolated:

```bash
# Project 1
mkdir ~/app1 && cd ~/app1
nself init
# Uses ports 80, 443, 5432, etc.

# Project 2 (different ports)
mkdir ~/app2 && cd ~/app2
nself init
# Auto-assigns ports 8080, 8443, 5433, etc.
```

**Port Auto-Assignment:**
nself detects port conflicts and automatically assigns alternative ports (8080, 8081, etc.). You can also manually configure ports in `.env`.

**Resource Sharing:**
Projects share Docker images (efficient) but have separate:
- Docker networks
- Volumes (databases are isolated)
- Configuration files
- Domains/subdomains

**Running simultaneously:**
Both projects can run at the same time if they use different ports. Use `docker ps` to see all running containers.

**[View Multi-App Setup](../guides/MULTI_APP_SETUP.md)** | **[Port Configuration](../guides/TROUBLESHOOTING.md#port-conflict-resolution)**

---

### What cloud providers are supported?

nself runs on **any infrastructure with Docker**, including:

**Major Cloud Providers (26 supported):**
- AWS (EC2, ECS, Fargate, Lightsail)
- Google Cloud Platform (Compute Engine, Cloud Run, GKE)
- Microsoft Azure (VMs, Container Instances, AKS)
- DigitalOcean (Droplets, App Platform, Kubernetes)
- Linode, Vultr, Hetzner, OVH, Scaleway

**Specialized Platforms:**
- Fly.io, Railway, Render
- Platform.sh, Clever Cloud
- Self-hosted (bare metal, home servers)

**Kubernetes:**
- Full Kubernetes support via `nself k8s` command
- Helm charts available
- Works on any K8s cluster (EKS, GKE, AKS, self-hosted)

**Deployment is simple:**
```bash
nself env create prod production
# Edit .environments/prod/server.json with your server
nself deploy prod
```

nself handles SSH, Docker setup, zero-downtime deployment, and health checks automatically.

**[View Deployment Guide](../guides/Deployment.md)** | **[Cloud Provider Comparison](../guides/PROVIDERS-COMPLETE.md)** | **[Kubernetes Guide](../commands/K8S.md)**

---

### How do I get help or report bugs?

**Documentation:**
- Browse comprehensive docs at [docs](../Home.md)
- Quick Start: [Quick Start Guide](Quick-Start.md)
- Troubleshooting: [../guides/TROUBLESHOOTING.md](../guides/TROUBLESHOOTING.md)

**Community Support:**
- GitHub Discussions: [Ask questions](https://github.com/acamarata/nself/discussions)
- GitHub Issues: [Report bugs](https://github.com/acamarata/nself/issues)

**Before Reporting:**
```bash
# Run diagnostics
nself doctor > diagnostics.txt

# Include in your issue:
# - nself version (nself version)
# - OS and Docker version
# - Error messages and logs
# - Contents of diagnostics.txt
```

**Commercial Support:**
Available for enterprise customers. Contact for SLAs, priority support, and custom features.

**Contributing:**
Pull requests are welcome! See [CONTRIBUTING.md](../contributing/CONTRIBUTING.md) for guidelines.

---

## Getting Started

### How do I install nself?

**Quick Install (recommended):**
```bash
curl -sSL https://install.nself.org | bash
```

This installs to `~/.nself/` and adds to your PATH automatically.

**Homebrew (macOS/Linux):**
```bash
brew tap acamarata/nself
brew install nself
```

**Manual Installation:**
```bash
git clone https://github.com/acamarata/nself.git ~/.nself
echo 'export PATH="$PATH:$HOME/.nself/bin"' >> ~/.bashrc
source ~/.bashrc
```

**Verify installation:**
```bash
nself version
```

**First project:**
```bash
mkdir myapp && cd myapp
nself init
nself build
nself start
```

Installation takes under 1 minute. First startup takes 2-5 minutes to download Docker images.

**[View Installation Guide](Installation.md)** | **[Quick Start](Quick-Start.md)**

---

### How long does setup take?

**Initial Installation:** < 1 minute
- Download and install CLI
- Add to PATH

**First Project Setup:** 5-10 minutes total
```bash
nself init      # 30 seconds (wizard)
nself build     # 1-2 minutes (generate configs)
nself start     # 2-5 minutes (download images)
```

**Subsequent Projects:** 2-3 minutes
- Docker images are cached
- Only configuration generation needed

**With Database Schema:** Add 2-3 minutes
```bash
nself db schema scaffold basic  # 10 seconds
nself db schema apply           # 1-2 minutes (migrate + mock data)
```

**Full Production Setup:** 20-30 minutes
- Includes SSL setup, environment configuration, deployment

Most time is spent downloading Docker images on first run. After that, starting a new project takes minutes.

**[View Quick Start](Quick-Start.md)** | **[Demo Setup](../services/DEMO_SETUP.md)**

---

### Do I need Docker experience?

No. nself abstracts away Docker complexity:

**What you don't need to know:**
- Writing Dockerfiles
- Docker Compose syntax
- Container networking
- Volume management
- Image building

**What nself handles automatically:**
- Image pulling and building
- Network creation
- Volume management
- Service orchestration
- Health checks
- Log aggregation

**Simple commands:**
```bash
nself start   # Instead of: docker compose up -d
nself stop    # Instead of: docker compose down
nself logs    # Instead of: docker compose logs -f
nself status  # Instead of: docker ps + health checks
```

You only need Docker Desktop installed. nself does the rest.

**Helpful to know (optional):**
- `docker ps` - See running containers
- `docker logs <container>` - View specific logs
- `docker system prune` - Clean up disk space

**[View Quick Start](Quick-Start.md)** | **[Troubleshooting](../guides/TROUBLESHOOTING.md)**

---

### Can I run it on Windows?

Yes, via **WSL 2 (Windows Subsystem for Linux)**:

**Setup:**
1. Install WSL 2:
   ```powershell
   wsl --install
   ```
2. Install Docker Desktop for Windows (with WSL 2 backend)
3. Open WSL terminal (Ubuntu)
4. Install nself:
   ```bash
   curl -sSL https://install.nself.org | bash
   ```

**All nself commands work identically in WSL:**
```bash
cd /mnt/c/Users/YourName/projects
mkdir myapp && cd myapp
nself init
nself build
nself start
```

**Benefits of WSL:**
- Full Linux environment on Windows
- Native Docker performance
- Bash 4+ (better than macOS)
- Compatible with all nself features

**Native Windows (without WSL):** Not currently supported. nself requires Bash and Docker with Linux containers.

**[View WSL Setup Guide](https://docs.microsoft.com/en-us/windows/wsl/install)** | **[Docker Desktop for Windows](https://docs.docker.com/desktop/windows/install/)**

---

### What cloud providers are supported?

nself runs on **any cloud provider with Linux VMs and Docker**. Full support for 26 providers via `nself provider` command:

**Tier 1 (Fully Tested):**
- AWS (EC2, Lightsail)
- Google Cloud Platform (Compute Engine)
- Microsoft Azure (VMs)
- DigitalOcean (Droplets)
- Linode/Akamai
- Hetzner Cloud
- Vultr
- OVH

**Tier 2 (Community Supported):**
- Fly.io, Railway, Render
- Scaleway, UpCloud
- IBM Cloud, Oracle Cloud
- Alibaba Cloud, Tencent Cloud

**Kubernetes:**
- Any managed Kubernetes (EKS, GKE, AKS, DOKS)
- Self-hosted Kubernetes
- K3s, MicroK8s

**Deployment is identical across providers:**
```bash
nself env create prod production
# Edit server.json with your server IP/hostname
nself deploy prod
```

nself handles platform-specific details automatically. You can even deploy to multiple providers for redundancy.

**[View Provider Comparison](../guides/PROVIDERS-COMPLETE.md)** | **[Deployment Guide](../guides/Deployment.md)** | **[Kubernetes Guide](../commands/K8S.md)**

---

### How do I configure custom domains?

**Local Development:**
Uses `local.nself.org` by default (wildcard DNS to 127.0.0.1).

**Custom Domain Setup:**

1. **Configure in .env:**
   ```bash
   BASE_DOMAIN=api.myapp.com
   ```

2. **Add DNS records:**
   ```
   api.myapp.com         → Your server IP
   *.api.myapp.com       → Your server IP (for subdomains)

   # Or individual subdomains:
   hasura.myapp.com      → Your server IP
   auth.myapp.com        → Your server IP
   storage.myapp.com     → Your server IP
   ```

3. **Enable SSL:**
   ```bash
   nself ssl --production
   # Uses Let's Encrypt automatically
   ```

4. **Rebuild and deploy:**
   ```bash
   nself build
   nself deploy prod
   ```

**Subdomain Pattern:**
- `api.myapp.com` - Hasura GraphQL
- `auth.myapp.com` - Authentication
- `storage.myapp.com` - File storage
- `admin.myapp.com` - Admin dashboard

You can customize subdomain prefixes in `.env`:
```bash
HASURA_SUBDOMAIN=graphql  # graphql.myapp.com
AUTH_SUBDOMAIN=accounts   # accounts.myapp.com
```

**[View Domain Selection Guide](../guides/domain-selection-guide.md)** | **[SSL Setup](../guides/Deployment.md)**

---

### Can I try a demo before committing?

Yes! Run the full demo with all services enabled:

```bash
mkdir nself-demo && cd nself-demo
nself init --demo
nself build
nself start
```

**Demo includes:**
- All 4 required services
- All 7 optional services enabled
- Full monitoring stack (10 services)
- 4 custom services from templates
- Sample database schema with mock data
- **Total: 25 services**

**Access the demo:**
```bash
nself urls  # Shows all available URLs
```

**Demo data includes:**
- Sample users (admin, user, demo roles)
- Mock data for testing
- Pre-configured dashboards
- Example API queries

**When done:**
```bash
nself stop
cd ..
rm -rf nself-demo  # Clean up
```

No commitment required. Demo runs entirely on your machine.

**[View Demo Setup Guide](../services/DEMO_SETUP.md)** | **[Quick Start](Quick-Start.md)**

---

### How do I enable/disable services?

Services are controlled via environment variables in `.env`:

**Enable a service:**
```bash
# Add to .env
REDIS_ENABLED=true
MINIO_ENABLED=true
MEILISEARCH_ENABLED=true
```

**Enable monitoring bundle (10 services):**
```bash
MONITORING_ENABLED=true
```

**After changing:**
```bash
nself build   # Regenerate docker-compose.yml
nself start   # Start new services
```

**Disable a service:**
```bash
# Remove from .env or set to false
REDIS_ENABLED=false
```

Then:
```bash
nself build
nself restart
```

**View enabled services:**
```bash
nself status          # Shows running services
nself urls            # Shows accessible URLs
grep _ENABLED .env    # Shows configuration
```

**Required services** (PostgreSQL, Hasura, Auth, Nginx) cannot be disabled.

**[View Services Documentation](../services/SERVICES.md)** | **[Environment Variables](../configuration/ENVIRONMENT-VARIABLES.md)**

---

### What happens on first run?

**Step-by-step breakdown:**

1. **nself init (30 seconds)**
   - Wizard asks configuration questions
   - Generates `.env` file
   - Creates project structure

2. **nself build (1-2 minutes)**
   - Generates `docker-compose.yml`
   - Creates Nginx configurations
   - Sets up SSL certificates (self-signed for local)
   - Generates database init scripts
   - Creates custom service scaffolds

3. **nself start (2-5 minutes first time)**
   - Pulls Docker images (~500MB-2GB depending on services)
   - Creates Docker network
   - Creates Docker volumes
   - Starts services in dependency order
   - Runs health checks
   - Initializes database
   - Applies initial migrations

**What gets created:**
```
myapp/
├── .env                  # Your configuration
├── .env.secrets          # Auto-generated passwords
├── docker-compose.yml    # Generated orchestration
├── nginx/                # Reverse proxy configs
│   ├── nginx.conf
│   └── sites/
├── postgres/
│   └── init/
│       └── 00-init.sql
├── ssl/
│   ├── cert.pem
│   └── key.pem
└── nself/
    ├── migrations/
    ├── seeds/
    └── backups/
```

**After first run:**
Subsequent starts take 10-30 seconds since images are cached.

**[View Quick Start](Quick-Start.md)** | **[Architecture](../architecture/ARCHITECTURE.md)**

---

### How do I add custom environment variables?

**Method 1: Add to .env**
```bash
# Edit .env
MY_API_KEY=abc123
MY_SECRET=xyz789
CUSTOM_SETTING=value
```

**Method 2: Environment-specific files**
```bash
# .env.local (local development only)
DEBUG=true
LOG_LEVEL=debug

# .env.prod (production only)
DEBUG=false
LOG_LEVEL=error
```

**Access in custom services:**

Docker Compose automatically passes all variables:
```javascript
// Node.js service
const apiKey = process.env.MY_API_KEY;
```

```python
# Python service
import os
api_key = os.getenv('MY_API_KEY')
```

**Secure secrets:**

For sensitive values, use `.env.secrets` (gitignored):
```bash
echo "STRIPE_SECRET_KEY=sk_live_xxx" >> .env.secrets
```

**Rebuild after changes:**
```bash
nself build   # Picks up new variables
nself restart # Applies to running containers
```

**[View Environment Variables Guide](../configuration/ENVIRONMENT-VARIABLES.md)** | **[Custom Services](../services/SERVICES_CUSTOM.md)**

---

### Can I import an existing database?

Yes, multiple methods:

**Method 1: SQL Import**
```bash
# Export from existing database
pg_dump -h old-host -U user database > export.sql

# Import to nself
nself db restore export.sql
```

**Method 2: Direct Connection**
```bash
# Get nself database connection string
nself db connection-string

# Use with psql, pg_restore, or your tool
pg_restore -h localhost -p 5432 -U postgres -d myapp backup.dump
```

**Method 3: External Database**

Use your existing database instead of the built-in one:
```bash
# In .env
POSTGRES_EXTERNAL=true
POSTGRES_HOST=your-db-host.com
POSTGRES_PORT=5432
POSTGRES_DB=your-database
POSTGRES_USER=your-user
POSTGRES_PASSWORD=your-password
```

**After import:**
```bash
# Generate GraphQL API
nself build

# Track tables in Hasura
# Visit https://api.local.nself.org
# Go to Data tab → Track all tables
```

**Migration from Supabase/Nhost:**
These platforms use PostgreSQL, so dumps are directly compatible.

**[View Database Workflow](../guides/DATABASE-WORKFLOW.md)** | **[Database Commands](../commands/DB.md)**

---

### How do I back up my data?

**Automatic Backups:**
```bash
# Create backup
nself db backup

# Creates: nself/backups/myapp_local_20260130_143022.sql.gz
```

**Schedule Regular Backups:**
```bash
# Add to crontab
crontab -e

# Daily at 2 AM
0 2 * * * cd ~/myapp && nself db backup

# Keep last 7 days
0 3 * * * find ~/myapp/nself/backups -name "*.sql.gz" -mtime +7 -delete
```

**Backup Everything:**
```bash
# Database + configuration + volumes
tar -czf myapp-full-backup.tar.gz \
  ~/myapp \
  --exclude="node_modules" \
  --exclude=".git"

# Docker volumes (requires root)
sudo tar -czf volumes-backup.tar.gz \
  /var/lib/docker/volumes/myapp_*
```

**Restore Backup:**
```bash
# From SQL file
nself db restore nself/backups/myapp_prod_20260130.sql.gz

# Or use deployment sync
nself deploy restore prod
```

**Backup to Cloud Storage:**
```bash
# AWS S3
aws s3 cp nself/backups/ s3://my-backups/ --recursive

# Automated with deployment
nself deploy backup prod --s3 s3://my-backups/
```

**[View Backup Guide](../guides/BACKUP_GUIDE.md)** | **[Deployment](../guides/Deployment.md)**

---

### What's the learning curve?

**Minimal - designed for rapid onboarding:**

**Day 1 (1-2 hours):**
- Install nself
- Create first project
- Understand basic commands (init, build, start, stop)
- Explore admin dashboard
- Make first GraphQL query

**Week 1:**
- Design database schema with DBML
- Configure authentication
- Add custom services
- Deploy to staging
- Understand environment management

**Week 2:**
- Production deployment
- SSL/TLS setup
- Monitoring and alerts
- Backup strategies
- Performance optimization

**Month 1:**
- Multi-tenant architecture
- Plugin integration (Stripe, etc.)
- Advanced Hasura features
- Kubernetes deployment (optional)
- Custom service development

**Learning resources:**
- Comprehensive documentation
- Working examples in demo setup
- Command help: `nself <command> --help`
- Active community discussions

Most developers are productive within hours, not days.

**[View Quick Start](Quick-Start.md)** | **[Database Workflow](../guides/DATABASE-WORKFLOW.md)** | **[Deployment Guide](../guides/Deployment.md)**

---

### How do I migrate from another platform?

**From Supabase:**

1. **Export database:**
   ```bash
   # From Supabase dashboard or CLI
   pg_dump postgresql://postgres:[password]@db.[project].supabase.co:5432/postgres > supabase-export.sql
   ```

2. **Import to nself:**
   ```bash
   nself db restore supabase-export.sql
   ```

3. **Auth migration:**
   - Export users from Supabase Auth
   - nself uses compatible JWT format
   - Run migration script (available in v0.5.0)

4. **Storage migration:**
   - Download files from Supabase Storage
   - Upload to MinIO via S3 API

5. **Update application code:**
   ```javascript
   // Before (Supabase)
   import { createClient } from '@supabase/supabase-js'
   const supabase = createClient(url, key)

   // After (nself)
   import { GraphQLClient } from 'graphql-request'
   const client = new GraphQLClient('https://api.myapp.com/v1/graphql')
   ```

**From Firebase:**

More complex due to NoSQL → SQL transformation:

1. **Export Firestore data:**
   ```bash
   gcloud firestore export gs://my-bucket/export
   ```

2. **Design PostgreSQL schema:**
   ```bash
   nself db schema scaffold basic  # Start with template
   # Modify schema.dbml to match your data structure
   ```

3. **Transform and import:**
   - Write custom script to transform JSON → SQL
   - Import using `nself db restore`

4. **Migrate Auth and Storage** (similar to Supabase)

**Migration assistance:**
Automated migration tools coming in v0.5.0. Contact via GitHub Discussions for migration support.

**[View Database Tools](../commands/DB.md)** | **[Community Support](https://github.com/acamarata/nself/discussions)**

---

### Can I use it with existing projects?

Yes! nself can be added to existing projects:

**Method 1: Backend Replacement**

Replace your existing backend with nself:
```bash
cd existing-project
mkdir backend && cd backend
nself init
nself build
nself start
```

Update your frontend to point to nself APIs:
```javascript
// Before
const API_URL = 'https://old-api.com'

// After
const API_URL = 'https://api.local.nself.org'
```

**Method 2: Alongside Existing Backend**

Run nself on different ports:
```bash
# .env
NGINX_HTTP_PORT=8080
NGINX_HTTPS_PORT=8443
HASURA_PORT=8081
```

Gradually migrate functionality to nself.

**Method 3: Microservices Addition**

Use nself for specific features:
- Add real-time subscriptions
- Offload authentication
- Add file storage
- Implement new features

Your existing backend can call nself APIs or vice versa.

**Database Integration:**

Connect to existing database:
```bash
POSTGRES_EXTERNAL=true
POSTGRES_HOST=existing-db.com
```

**[View Multi-App Setup](../guides/MULTI_APP_SETUP.md)** | **[Architecture](../architecture/ARCHITECTURE.md)**

---

### How do I add team members?

**Local Development:**

1. **Share repository:**
   ```bash
   git add .env.dev .gitignore
   git commit -m "Add nself configuration"
   git push
   ```

2. **Team member setup:**
   ```bash
   git clone repo
   cd project
   nself build
   nself start
   ```

3. **Personal overrides:**
   ```bash
   # Each dev creates .env.local (gitignored)
   echo "POSTGRES_PORT=5433" > .env.local
   ```

**Production Access:**

Managed via environment files and SSH keys:

```bash
# Developer tiers
# Dev: .env.dev + .env.local only
# Sr Dev: + .env.staging access
# Lead Dev: + .env.prod + .secrets access
```

**SSH Key Management:**
```bash
# Add team member SSH key to server
ssh-copy-id -i teammate-key.pub user@server

# Grant environment access
nself env grant staging teammate@company.com
```

**Database Access:**

Create role-specific PostgreSQL users:
```sql
-- Read-only for devs
CREATE USER dev_readonly PASSWORD 'xxx';
GRANT SELECT ON ALL TABLES IN SCHEMA public TO dev_readonly;

-- Read-write for seniors
CREATE USER dev_readwrite PASSWORD 'xxx';
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES TO dev_readwrite;
```

**[View Environment Management](../commands/ENV.md)** | **[Multi-App Setup](../guides/MULTI_APP_SETUP.md)**

---

### What if I need help with setup?

**Self-Service Resources:**

1. **Documentation** (you're reading it!)
   - [Quick Start Guide](Quick-Start.md)
   - [Troubleshooting](../guides/TROUBLESHOOTING.md)
   - [Video tutorials](https://github.com/acamarata/nself#videos) (coming soon)

2. **Automated Diagnostics:**
   ```bash
   nself doctor        # Check for issues
   nself doctor --fix  # Auto-fix common problems
   ```

3. **Built-in Help:**
   ```bash
   nself --help
   nself <command> --help
   ```

**Community Support:**

1. **GitHub Discussions:**
   - Ask questions
   - Share solutions
   - Request features
   - [Join discussions](https://github.com/acamarata/nself/discussions)

2. **GitHub Issues:**
   - Report bugs
   - Include `nself doctor` output
   - [Submit issues](https://github.com/acamarata/nself/issues)

**Commercial Support:**

For enterprises needing:
- Dedicated support engineer
- SLA guarantees
- Custom feature development
- Migration assistance
- Training workshops

Contact via GitHub Discussions or email (coming soon).

**[View Troubleshooting Guide](../guides/TROUBLESHOOTING.md)** | **[Community](https://github.com/acamarata/nself/discussions)**

---

## Multi-Tenancy

### How does multi-tenancy work?

nself supports multi-tenancy through **Row Level Security (RLS)** in PostgreSQL:

**Architecture:**
- All tenant data in same database
- Tenant ID in every table
- RLS policies enforce isolation
- No data leakage between tenants

**Implementation:**

1. **Add tenant_id to tables:**
   ```sql
   CREATE TABLE products (
     id UUID PRIMARY KEY,
     tenant_id UUID NOT NULL,
     name TEXT NOT NULL,
     CONSTRAINT fk_tenant FOREIGN KEY (tenant_id)
       REFERENCES tenants(id)
   );
   ```

2. **Enable RLS:**
   ```sql
   ALTER TABLE products ENABLE ROW LEVEL SECURITY;
   ```

3. **Create policies:**
   ```sql
   CREATE POLICY tenant_isolation ON products
     USING (tenant_id = current_setting('app.tenant_id')::UUID);
   ```

4. **Set tenant context:**
   ```javascript
   // In your application
   await client.query(
     `SET app.tenant_id = '${tenantId}'`
   );
   ```

Hasura GraphQL automatically respects RLS policies. Each API request only sees data for that tenant.

**[View Multi-Tenant Guide](#how-is-tenant-data-isolated)** | **[Database Security](../guides/DATABASE-WORKFLOW.md)**

---

### How is tenant data isolated?

**Multiple layers of isolation:**

**1. Database Level (RLS):**
```sql
-- Every query automatically filtered
CREATE POLICY tenant_isolation ON products
  USING (tenant_id = current_setting('app.tenant_id')::UUID);

-- Even admin can't bypass without explicitly disabling
```

**2. API Level (Hasura Permissions):**
```json
{
  "role": "user",
  "table": "products",
  "permission": {
    "filter": {
      "tenant_id": {"_eq": "X-Hasura-Tenant-Id"}
    }
  }
}
```

**3. Application Level:**
```javascript
// Middleware sets tenant context
app.use((req, res, next) => {
  const tenantId = req.user.tenantId;
  req.db.query(`SET app.tenant_id = '${tenantId}'`);
  next();
});
```

**4. Schema Level:**
```sql
-- Foreign key constraints prevent orphaned data
CONSTRAINT fk_tenant FOREIGN KEY (tenant_id)
  REFERENCES tenants(id) ON DELETE CASCADE;
```

**Verification:**

Test isolation in development:
```sql
-- User A context
SET app.tenant_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa';
SELECT COUNT(*) FROM products; -- Returns only Tenant A data

-- User B context
SET app.tenant_id = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb';
SELECT COUNT(*) FROM products; -- Returns only Tenant B data
```

**Performance:**
RLS policies use indexes on `tenant_id`, so queries remain fast even with millions of rows.

**[View Database Workflow](../guides/DATABASE-WORKFLOW.md)** | **[Security Best Practices](#security)**

---

### Can tenants have custom domains?

Yes! Full white-label domain support:

**Architecture:**

```bash
# Tenant A
https://app.companyA.com → Your nself backend

# Tenant B
https://portal.companyB.com → Same nself backend

# Tenant C
https://dashboard.companyC.com → Same nself backend
```

**Setup:**

1. **Configure tenant domains in database:**
   ```sql
   CREATE TABLE tenants (
     id UUID PRIMARY KEY,
     name TEXT NOT NULL,
     custom_domain TEXT UNIQUE,
     created_at TIMESTAMPTZ DEFAULT NOW()
   );

   INSERT INTO tenants (id, name, custom_domain) VALUES
     ('aaa...', 'Company A', 'app.companyA.com'),
     ('bbb...', 'Company B', 'portal.companyB.com');
   ```

2. **Add wildcard Nginx config:**
   ```nginx
   # nginx/sites/tenant-wildcard.conf
   server {
     server_name ~^(?<tenant>.+)$;

     location / {
       # Extract tenant from domain
       proxy_set_header X-Tenant-Domain $tenant;
       proxy_pass http://hasura:8080;
     }
   }
   ```

3. **DNS Configuration:**
   ```
   # Each tenant adds CNAME:
   app.companyA.com      CNAME   your-app.com
   portal.companyB.com   CNAME   your-app.com
   ```

4. **SSL Certificates:**
   ```bash
   # Automated with Let's Encrypt wildcard
   nself ssl --wildcard --production

   # Or per-domain
   certbot certonly -d app.companyA.com
   certbot certonly -d portal.companyB.com
   ```

5. **Tenant Resolution:**
   ```javascript
   // Middleware identifies tenant by domain
   const domain = req.hostname; // "app.companyA.com"
   const tenant = await getTenantByDomain(domain);
   req.tenantId = tenant.id;
   ```

**Subdomain Pattern (Alternative):**
```
companyA.your-app.com
companyB.your-app.com
```
Simpler DNS (one wildcard record), but less white-label.

**[View White-Label Guide](#white-label)** | **[Domain Selection](../guides/domain-selection-guide.md)**

---

### What's the tenant limit?

**Technical Limits:**

There is no hard-coded tenant limit. Constraints are based on infrastructure:

**Database:**
- **Rows:** PostgreSQL can handle billions of rows
- **Connections:** Default 100 connections (increase with `POSTGRES_MAX_CONNECTIONS`)
- **Schema:** Shared schema across all tenants (efficient)

**Practical Limits by Infrastructure:**

| Setup | Tenants | Notes |
|-------|---------|-------|
| Small (4GB RAM) | 100-500 | Shared hosting, few users per tenant |
| Medium (8GB RAM) | 500-2,000 | Typical SaaS, moderate activity |
| Large (16GB RAM) | 2,000-10,000 | High activity, needs optimization |
| Enterprise | 10,000+ | Horizontal scaling, read replicas |

**Scaling Strategies:**

1. **Vertical Scaling:**
   ```bash
   # Increase instance size
   # 16GB → 32GB → 64GB
   ```

2. **Connection Pooling:**
   ```bash
   # PgBouncer for connection management
   HASURA_POOL_SIZE=50
   POSTGRES_MAX_CONNECTIONS=200
   ```

3. **Read Replicas:**
   ```bash
   # Offload read queries
   POSTGRES_READ_REPLICA=replica.db.com
   ```

4. **Caching:**
   ```bash
   # Redis for frequently accessed data
   REDIS_ENABLED=true
   ```

5. **Database Sharding (Advanced):**
   - Split tenants across multiple databases
   - Route by tenant_id hash
   - Requires custom application logic

**Monitoring:**
```bash
nself perf              # Track performance
nself db query "SELECT COUNT(DISTINCT tenant_id) FROM tenants"
```

**[View Performance & Scaling](#performance--scaling)** | **[Monitoring](../services/MONITORING-BUNDLE.md)**

---

### How do I migrate to multi-tenant?

**Migration from single-tenant to multi-tenant:**

**Step 1: Add tenants table**
```sql
CREATE TABLE tenants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  slug TEXT UNIQUE NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create initial tenant for existing data
INSERT INTO tenants (id, name, slug)
VALUES ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 'Default Tenant', 'default');
```

**Step 2: Add tenant_id to existing tables**
```sql
-- Add column
ALTER TABLE users ADD COLUMN tenant_id UUID;
ALTER TABLE products ADD COLUMN tenant_id UUID;
ALTER TABLE orders ADD COLUMN tenant_id UUID;

-- Set existing data to default tenant
UPDATE users SET tenant_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa';
UPDATE products SET tenant_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa';
UPDATE orders SET tenant_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa';

-- Make NOT NULL after backfill
ALTER TABLE users ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE products ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE orders ALTER COLUMN tenant_id SET NOT NULL;

-- Add foreign keys
ALTER TABLE users ADD CONSTRAINT fk_tenant
  FOREIGN KEY (tenant_id) REFERENCES tenants(id);
```

**Step 3: Create indexes**
```sql
-- Critical for query performance
CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_products_tenant ON products(tenant_id);
CREATE INDEX idx_orders_tenant ON orders(tenant_id);
```

**Step 4: Enable RLS**
```sql
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE products ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders ENABLE ROW LEVEL SECURITY;

-- Create policies
CREATE POLICY tenant_isolation_users ON users
  USING (tenant_id = current_setting('app.tenant_id')::UUID);

CREATE POLICY tenant_isolation_products ON products
  USING (tenant_id = current_setting('app.tenant_id')::UUID);

CREATE POLICY tenant_isolation_orders ON orders
  USING (tenant_id = current_setting('app.tenant_id')::UUID);
```

**Step 5: Update application code**
```javascript
// Add tenant context to all requests
app.use(async (req, res, next) => {
  if (req.user) {
    const tenantId = req.user.tenantId;
    await req.db.query(`SET app.tenant_id = '${tenantId}'`);
  }
  next();
});
```

**Step 6: Update Hasura permissions**
```json
{
  "role": "user",
  "permission": {
    "filter": {
      "tenant_id": {"_eq": "X-Hasura-Tenant-Id"}
    }
  }
}
```

**Step 7: Test isolation**
```bash
# Create test tenants
# Verify queries only return tenant-specific data
# Test cross-tenant access is blocked
```

**Automated Migration (v0.5.0):**
```bash
nself db migrate-to-multitenancy
```

**[View Database Workflow](../guides/DATABASE-WORKFLOW.md)** | **[Migration Commands](../commands/MIGRATE.md)**

---

### Can I have tenant-specific databases?

Yes, but it's more complex and less common:

**Approach 1: Database per Tenant (Full Isolation)**

```bash
# Create separate database for each tenant
CREATE DATABASE tenant_companya;
CREATE DATABASE tenant_companyb;
```

**Pros:**
- Maximum isolation (regulatory compliance)
- Independent backups
- Can scale tenants independently

**Cons:**
- More complex management
- Higher resource usage
- Schema changes must be applied to all databases
- No cross-tenant queries/analytics

**Implementation:**
```javascript
// Dynamic connection based on tenant
const dbName = `tenant_${tenant.slug}`;
const client = new Client({
  database: dbName,
  ...config
});
```

**Approach 2: Schema per Tenant (PostgreSQL Schemas)**

```sql
-- Each tenant gets a schema
CREATE SCHEMA tenant_companya;
CREATE SCHEMA tenant_companyb;

-- Tables in each schema
CREATE TABLE tenant_companya.users (...);
CREATE TABLE tenant_companyb.users (...);
```

**Pros:**
- Better than separate databases (shared extensions, connections)
- Still good isolation
- Easier management than separate databases

**Cons:**
- Still complex to manage
- Schema changes apply to all
- Higher maintenance than RLS approach

**Recommendation:**

For most use cases, **use RLS (single database, tenant_id column)** because:
- Simpler to manage
- Better performance (one connection pool)
- Easier schema migrations
- Cross-tenant analytics possible
- Works great for 99% of SaaS apps

Only use separate databases if you have:
- Regulatory requirements (HIPAA, financial)
- Very large tenants needing dedicated resources
- Requirement for independent backups

**[View Multi-Tenancy Best Practices](#how-does-multi-tenancy-work)** | **[Database Architecture](../architecture/ARCHITECTURE.md)**

---

### How do I handle tenant-specific settings?

**Method 1: Tenant Settings Table**

```sql
CREATE TABLE tenant_settings (
  tenant_id UUID PRIMARY KEY REFERENCES tenants(id),
  branding JSONB,
  features JSONB,
  limits JSONB,
  metadata JSONB,
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Example data
INSERT INTO tenant_settings (tenant_id, branding, features, limits) VALUES (
  'aaaa-...',
  '{"logo": "https://...", "primaryColor": "#FF5733"}',
  '{"analytics": true, "aiAssistant": false}',
  '{"maxUsers": 100, "storage": 10737418240}'
);
```

**Method 2: Feature Flags**

```sql
CREATE TABLE tenant_features (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  feature_key TEXT NOT NULL,
  enabled BOOLEAN DEFAULT false,
  config JSONB,
  UNIQUE(tenant_id, feature_key)
);

-- Enable feature for tenant
INSERT INTO tenant_features (tenant_id, feature_key, enabled, config) VALUES
  ('aaaa-...', 'advanced_analytics', true, '{"retention": 365}');
```

**Method 3: Plan-Based Settings**

```sql
CREATE TABLE subscription_plans (
  id UUID PRIMARY KEY,
  name TEXT NOT NULL,
  max_users INTEGER,
  storage_gb INTEGER,
  features TEXT[]
);

ALTER TABLE tenants ADD COLUMN plan_id UUID REFERENCES subscription_plans(id);

-- Apply plan limits
SELECT
  t.*,
  p.max_users,
  p.storage_gb,
  p.features
FROM tenants t
JOIN subscription_plans p ON t.plan_id = p.id;
```

**Access in Application:**

```javascript
// Fetch tenant settings
const settings = await getTenantSettings(tenantId);

// Check feature access
if (settings.features.analytics) {
  // Show analytics dashboard
}

// Enforce limits
if (currentUserCount >= settings.limits.maxUsers) {
  throw new Error('User limit reached');
}
```

**Access in GraphQL:**

```graphql
query GetTenantSettings {
  tenant_settings_by_pk(tenant_id: "...") {
    branding
    features
    limits
  }
}
```

**Hot-Reload Settings:**

Cache settings but watch for updates:
```javascript
// Redis cache with TTL
const settings = await redis.get(`tenant:${tenantId}:settings`);
if (!settings) {
  const fresh = await db.getTenantSettings(tenantId);
  await redis.setex(`tenant:${tenantId}:settings`, 300, JSON.stringify(fresh));
  return fresh;
}
```

**[View Plugin System](../plugins/index.md)** | **[Configuration](../configuration/ENVIRONMENT-VARIABLES.md)**

---

### What about tenant-specific authentication?

**Tenant-Aware Auth System:**

nself supports tenant isolation in authentication:

**1. User-Tenant Relationship:**

```sql
CREATE TABLE user_tenants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  role TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(user_id, tenant_id)
);

-- Users can belong to multiple tenants
INSERT INTO user_tenants (user_id, tenant_id, role) VALUES
  ('user-1', 'tenant-a', 'admin'),
  ('user-1', 'tenant-b', 'member');
```

**2. Login Flow:**

```javascript
// Step 1: Authenticate user
const user = await authenticateUser(email, password);

// Step 2: Get user's tenants
const tenants = await getUserTenants(user.id);

// Step 3: User selects tenant (or use subdomain)
const selectedTenant = tenants[0];

// Step 4: Generate JWT with tenant claim
const token = jwt.sign({
  userId: user.id,
  tenantId: selectedTenant.tenant_id,
  role: selectedTenant.role
}, secret);
```

**3. Hasura JWT Claims:**

```json
{
  "sub": "user-uuid",
  "https://hasura.io/jwt/claims": {
    "x-hasura-default-role": "user",
    "x-hasura-allowed-roles": ["user", "admin"],
    "x-hasura-user-id": "user-uuid",
    "x-hasura-tenant-id": "tenant-uuid"
  }
}
```

**4. Automatic Tenant Detection:**

```javascript
// From subdomain
const subdomain = req.hostname.split('.')[0]; // "companya"
const tenant = await getTenantBySlug(subdomain);

// From custom domain
const domain = req.hostname; // "app.companya.com"
const tenant = await getTenantByDomain(domain);

// From path
const tenantSlug = req.params.tenant; // "/companya/dashboard"
const tenant = await getTenantBySlug(tenantSlug);
```

**5. Tenant Switching:**

```javascript
// User switches between their tenants
app.post('/api/switch-tenant', async (req, res) => {
  const { tenantId } = req.body;

  // Verify user has access
  const hasAccess = await userHasTenantAccess(req.user.id, tenantId);
  if (!hasAccess) throw new Error('Access denied');

  // Issue new token
  const newToken = jwt.sign({
    userId: req.user.id,
    tenantId: tenantId,
    role: getUserRoleInTenant(req.user.id, tenantId)
  }, secret);

  res.json({ token: newToken });
});
```

**6. SSO per Tenant (Advanced):**

```sql
CREATE TABLE tenant_sso_config (
  tenant_id UUID PRIMARY KEY REFERENCES tenants(id),
  provider TEXT NOT NULL, -- "okta", "auth0", "google"
  client_id TEXT NOT NULL,
  client_secret TEXT NOT NULL,
  config JSONB
);

-- Tenant A uses Okta
-- Tenant B uses Google Workspace
-- Tenant C uses username/password
```

**[View Authentication Guide](../architecture/API.md)** | **[Multi-Tenancy](#multi-tenancy)**

---

### Can tenants invite their own users?

Yes! Implement tenant-scoped user management:

**1. Invitations Table:**

```sql
CREATE TABLE tenant_invitations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  email TEXT NOT NULL,
  role TEXT NOT NULL,
  invited_by UUID NOT NULL REFERENCES users(id),
  token TEXT UNIQUE NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_invitations_token ON tenant_invitations(token);
CREATE INDEX idx_invitations_tenant ON tenant_invitations(tenant_id);
```

**2. Invitation Flow:**

```javascript
// Tenant admin invites user
async function inviteUser(tenantId, email, role, invitedBy) {
  // Check inviter has permission
  const hasPermission = await checkPermission(invitedBy, tenantId, 'invite_users');
  if (!hasPermission) throw new Error('Permission denied');

  // Generate secure token
  const token = generateSecureToken();
  const expiresAt = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000); // 7 days

  // Create invitation
  await db.query(`
    INSERT INTO tenant_invitations
      (tenant_id, email, role, invited_by, token, expires_at)
    VALUES ($1, $2, $3, $4, $5, $6)
  `, [tenantId, email, role, invitedBy, token, expiresAt]);

  // Send email
  await sendInvitationEmail(email, token, tenantId);
}
```

**3. Invitation Email:**

```html
<p>You've been invited to join Company A on MyApp!</p>
<p>Role: Team Member</p>
<a href="https://app.myapp.com/accept-invitation?token=xxx">
  Accept Invitation
</a>
<p>This invitation expires in 7 days.</p>
```

**4. Acceptance Flow:**

```javascript
// User accepts invitation
async function acceptInvitation(token, password) {
  // Verify token
  const invitation = await db.query(`
    SELECT * FROM tenant_invitations
    WHERE token = $1 AND expires_at > NOW() AND accepted_at IS NULL
  `, [token]);

  if (!invitation) throw new Error('Invalid or expired invitation');

  // Create user account
  const user = await createUser({
    email: invitation.email,
    password: password
  });

  // Add to tenant
  await db.query(`
    INSERT INTO user_tenants (user_id, tenant_id, role)
    VALUES ($1, $2, $3)
  `, [user.id, invitation.tenant_id, invitation.role]);

  // Mark invitation as accepted
  await db.query(`
    UPDATE tenant_invitations
    SET accepted_at = NOW()
    WHERE id = $1
  `, [invitation.id]);

  return user;
}
```

**5. GraphQL API:**

```graphql
mutation InviteUser($input: InviteUserInput!) {
  invite_user(input: $input) {
    success
    invitation_id
  }
}

mutation AcceptInvitation($token: String!, $password: String!) {
  accept_invitation(token: $token, password: $password) {
    user_id
    tenant_id
    token
  }
}

query PendingInvitations($tenantId: uuid!) {
  tenant_invitations(
    where: {
      tenant_id: {_eq: $tenantId}
      accepted_at: {_is_null: true}
      expires_at: {_gt: "now()"}
    }
  ) {
    id
    email
    role
    created_at
    expires_at
  }
}
```

**6. Permissions & Limits:**

```javascript
// Check if tenant can invite more users
const planLimit = tenant.plan.max_users;
const currentUsers = await getUserCount(tenantId);

if (currentUsers >= planLimit) {
  throw new Error('User limit reached. Upgrade plan to invite more users.');
}
```

**[View Authentication](../architecture/API.md)** | **[Billing & Usage](#billing--usage)**

---

### How do I manage tenant billing separately?

**Stripe Plugin Integration:**

nself includes a Stripe plugin for per-tenant billing:

**1. Install Plugin:**

```bash
nself plugin install stripe
nself build
nself start
```

**2. Link Tenants to Stripe:**

```sql
-- Extend tenants table
ALTER TABLE tenants ADD COLUMN stripe_customer_id TEXT UNIQUE;

-- When creating tenant, create Stripe customer
INSERT INTO tenants (id, name, stripe_customer_id) VALUES (
  gen_random_uuid(),
  'Company A',
  'cus_xxxxxxxxxxxxx'
);
```

**3. Subscription Management:**

```javascript
// Create subscription for tenant
async function createTenantSubscription(tenantId, planId) {
  const tenant = await getTenant(tenantId);

  // Create or get Stripe customer
  let customerId = tenant.stripe_customer_id;
  if (!customerId) {
    const customer = await stripe.customers.create({
      email: tenant.billing_email,
      name: tenant.name,
      metadata: { tenant_id: tenantId }
    });
    customerId = customer.id;
    await updateTenant(tenantId, { stripe_customer_id: customerId });
  }

  // Create subscription
  const subscription = await stripe.subscriptions.create({
    customer: customerId,
    items: [{ price: planId }],
    metadata: { tenant_id: tenantId }
  });

  return subscription;
}
```

**4. Usage-Based Billing:**

```javascript
// Track metered usage
async function reportUsage(tenantId, metric, quantity) {
  const tenant = await getTenant(tenantId);

  await stripe.subscriptionItems.createUsageRecord(
    tenant.subscription_item_id,
    {
      quantity: quantity,
      timestamp: Math.floor(Date.now() / 1000),
      action: 'increment'
    }
  );

  // Also store locally for analytics
  await db.query(`
    INSERT INTO usage_logs (tenant_id, metric, quantity, logged_at)
    VALUES ($1, $2, $3, NOW())
  `, [tenantId, metric, quantity]);
}

// Example: Track API calls
app.use(async (req, res, next) => {
  await reportUsage(req.tenantId, 'api_calls', 1);
  next();
});
```

**5. Webhook Handling:**

```javascript
// Stripe webhook for subscription events
app.post('/webhooks/stripe', async (req, res) => {
  const event = req.body;

  switch (event.type) {
    case 'invoice.payment_succeeded':
      await handlePaymentSuccess(event.data.object);
      break;

    case 'invoice.payment_failed':
      await handlePaymentFailure(event.data.object);
      break;

    case 'customer.subscription.deleted':
      await handleSubscriptionCanceled(event.data.object);
      break;
  }

  res.json({ received: true });
});

async function handlePaymentFailure(invoice) {
  const tenantId = invoice.metadata.tenant_id;

  // Suspend tenant access
  await db.query(`
    UPDATE tenants
    SET status = 'suspended', suspended_at = NOW()
    WHERE id = $1
  `, [tenantId]);

  // Notify tenant admin
  await sendPaymentFailureEmail(tenantId);
}
```

**6. Billing Dashboard:**

```graphql
query TenantBilling($tenantId: uuid!) {
  tenants_by_pk(id: $tenantId) {
    name
    stripe_customer_id
    plan {
      name
      price
    }
    stripe_subscriptions {
      status
      current_period_end
      items {
        price {
          unit_amount
          recurring {
            interval
          }
        }
      }
    }
    stripe_invoices(limit: 10, order_by: {created: desc}) {
      id
      amount_due
      status
      invoice_pdf
    }
  }

  usage_logs(
    where: {tenant_id: {_eq: $tenantId}}
    order_by: {logged_at: desc}
  ) {
    metric
    quantity
    logged_at
  }
}
```

**[View Stripe Plugin](../plugins/stripe.md)** | **[Billing & Usage](#billing--usage)**

---

### What's the performance impact of multi-tenancy?

**Negligible to minimal** when implemented correctly:

**RLS Performance:**

PostgreSQL Row Level Security is highly optimized:

```sql
-- Without RLS (single tenant)
SELECT * FROM products WHERE category = 'electronics';
-- Index scan on category

-- With RLS (multi-tenant)
SELECT * FROM products WHERE category = 'electronics';
-- Index scan on (tenant_id, category)
-- RLS policy adds: AND tenant_id = 'xxx'
```

**Indexing Strategy:**

```sql
-- Composite indexes for multi-tenant queries
CREATE INDEX idx_products_tenant_category
  ON products(tenant_id, category);

CREATE INDEX idx_orders_tenant_date
  ON orders(tenant_id, created_at DESC);

CREATE INDEX idx_users_tenant_email
  ON users(tenant_id, email);
```

**Query Performance:**

| Query Type | Single-Tenant | Multi-Tenant (RLS) | Impact |
|------------|---------------|-------------------|--------|
| Simple SELECT | 1ms | 1.1ms | +10% |
| Filtered queries | 5ms | 5.2ms | +4% |
| JOIN queries | 10ms | 10.5ms | +5% |
| Aggregations | 20ms | 21ms | +5% |

**Optimization Techniques:**

1. **Proper Indexing:**
   ```sql
   -- Always index tenant_id
   CREATE INDEX idx_table_tenant ON table_name(tenant_id);

   -- Composite indexes for common queries
   CREATE INDEX idx_table_tenant_status
     ON table_name(tenant_id, status);
   ```

2. **Connection Pooling:**
   ```bash
   # Hasura config
   HASURA_POOL_SIZE=50

   # Or use PgBouncer
   PGBOUNCER_ENABLED=true
   ```

3. **Caching:**
   ```bash
   # Redis for frequently accessed tenant data
   REDIS_ENABLED=true
   ```

4. **Partition Tables (Large Scale):**
   ```sql
   -- For 10,000+ tenants
   CREATE TABLE products_2024 PARTITION OF products
     FOR VALUES FROM ('2024-01-01') TO ('2025-01-01');
   ```

**Monitoring:**

```bash
# Check query performance
nself perf profile

# Analyze slow queries
nself db query "
  SELECT query, mean_exec_time, calls
  FROM pg_stat_statements
  ORDER BY mean_exec_time DESC
  LIMIT 20;
"
```

**Benchmark Results:**

10,000 tenants, 1M rows per table:
- Query latency: < 10ms (p95)
- Throughput: 10,000+ req/sec
- Resource usage: Same as single-tenant

**[View Performance Guide](#performance--scaling)** | **[Monitoring](../services/MONITORING-BUNDLE.md)**

---

## Billing & Usage

### How does Stripe integration work?

nself includes a **Stripe plugin** for complete payment integration:

**Installation:**

```bash
# Install plugin
nself plugin install stripe

# Configure API key
echo "STRIPE_API_KEY=sk_test_PLACEHOLDER" >> .env
echo "STRIPE_WEBHOOK_SECRET=whsec_xxxxx" >> .env

# Rebuild
nself build && nself start
```

**What Gets Installed:**

1. **Database Schema:**
   - `stripe_customers` - Customer records
   - `stripe_subscriptions` - Subscription data
   - `stripe_invoices` - Invoice history
   - `stripe_payment_methods` - Saved cards/methods
   - `stripe_products` - Product catalog
   - `stripe_prices` - Pricing plans

2. **Webhook Handler:**
   - Endpoint: `https://your-domain.com/webhooks/stripe`
   - Processes: payments, subscriptions, customers
   - Signature verification included

3. **CLI Commands:**
   ```bash
   nself plugin stripe sync              # Sync all data
   nself plugin stripe customers list    # View customers
   nself plugin stripe subscriptions     # View subscriptions
   ```

4. **GraphQL API:**
   ```graphql
   query GetCustomerSubscriptions($email: String!) {
     stripe_customers(where: {email: {_eq: $email}}) {
       id
       email
       stripe_subscriptions {
         status
         current_period_end
         items {
           price {
             unit_amount
             recurring {
               interval
             }
           }
         }
       }
     }
   }
   ```

**Usage Example:**

```javascript
// Create customer and subscription
const customer = await stripe.customers.create({
  email: user.email,
  metadata: { user_id: user.id }
});

const subscription = await stripe.subscriptions.create({
  customer: customer.id,
  items: [{ price: 'price_xxxxx' }],
  metadata: { tenant_id: tenant.id }
});

// Data automatically synced to PostgreSQL via webhooks
```

**[View Stripe Plugin Documentation](../plugins/stripe.md)** | **[Plugin System](../plugins/index.md)**

---

### What metrics are tracked?

**Built-in Metrics (Monitoring Bundle):**

Enable comprehensive monitoring:
```bash
MONITORING_ENABLED=true
nself build && nself start
```

**System Metrics:**
- CPU usage per container
- Memory usage and limits
- Disk I/O and space
- Network traffic

**Database Metrics:**
- Query performance (pg_stat_statements)
- Connection count
- Cache hit ratio
- Transaction rate
- Table sizes and bloat

**Application Metrics:**
- HTTP request rate
- Response times (p50, p95, p99)
- Error rates (4xx, 5xx)
- GraphQL query performance

**Business Metrics (Custom):**

Track via custom services or database:

```sql
CREATE TABLE usage_metrics (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  metric_name TEXT NOT NULL,
  value NUMERIC NOT NULL,
  dimensions JSONB,
  recorded_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_metrics_tenant_name_time
  ON usage_metrics(tenant_id, metric_name, recorded_at DESC);

-- Track API calls
INSERT INTO usage_metrics (tenant_id, metric_name, value)
VALUES ('xxx', 'api_calls', 1);

-- Track storage usage
INSERT INTO usage_metrics (tenant_id, metric_name, value, dimensions)
VALUES ('xxx', 'storage_bytes', 1048576, '{"file_type": "image"}');

-- Track feature usage
INSERT INTO usage_metrics (tenant_id, metric_name, value, dimensions)
VALUES ('xxx', 'feature_use', 1, '{"feature": "ai_assistant"}');
```

**Query Metrics:**

```sql
-- API calls per tenant (last 24h)
SELECT
  tenant_id,
  COUNT(*) as api_calls,
  SUM(CASE WHEN status >= 500 THEN 1 ELSE 0 END) as errors
FROM api_logs
WHERE recorded_at > NOW() - INTERVAL '24 hours'
GROUP BY tenant_id;

-- Storage usage
SELECT
  tenant_id,
  SUM(value) / 1024 / 1024 / 1024 as storage_gb
FROM usage_metrics
WHERE metric_name = 'storage_bytes'
GROUP BY tenant_id;
```

**Grafana Dashboards:**

Pre-built dashboards included:
- System Overview
- Database Performance
- API Metrics
- Tenant Usage
- Cost Attribution

Access at `https://grafana.local.nself.org`

**[View Monitoring Bundle](../services/MONITORING-BUNDLE.md)** | **[Metrics Command](../commands/METRICS.md)**

---

### How are quotas enforced?

**Multi-Level Quota Enforcement:**

**1. Database Level:**

```sql
CREATE TABLE tenant_quotas (
  tenant_id UUID PRIMARY KEY REFERENCES tenants(id),
  max_users INTEGER DEFAULT 10,
  max_storage_bytes BIGINT DEFAULT 10737418240, -- 10GB
  max_api_calls_per_day INTEGER DEFAULT 10000,
  features TEXT[] DEFAULT ARRAY['basic'],
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Check function
CREATE OR REPLACE FUNCTION check_quota(
  p_tenant_id UUID,
  p_quota_type TEXT,
  p_requested INTEGER DEFAULT 1
) RETURNS BOOLEAN AS $$
DECLARE
  current_usage INTEGER;
  quota_limit INTEGER;
BEGIN
  CASE p_quota_type
    WHEN 'users' THEN
      SELECT COUNT(*), q.max_users INTO current_usage, quota_limit
      FROM user_tenants ut
      JOIN tenant_quotas q ON q.tenant_id = p_tenant_id
      WHERE ut.tenant_id = p_tenant_id
      GROUP BY q.max_users;

    WHEN 'storage' THEN
      SELECT SUM(file_size), q.max_storage_bytes INTO current_usage, quota_limit
      FROM files f
      JOIN tenant_quotas q ON q.tenant_id = p_tenant_id
      WHERE f.tenant_id = p_tenant_id
      GROUP BY q.max_storage_bytes;

    WHEN 'api_calls' THEN
      SELECT COUNT(*), q.max_api_calls_per_day INTO current_usage, quota_limit
      FROM api_logs a
      JOIN tenant_quotas q ON q.tenant_id = p_tenant_id
      WHERE a.tenant_id = p_tenant_id
        AND a.created_at > NOW() - INTERVAL '24 hours'
      GROUP BY q.max_api_calls_per_day;
  END CASE;

  RETURN (current_usage + p_requested) <= quota_limit;
END;
$$ LANGUAGE plpgsql;
```

**2. Application Level:**

```javascript
// Middleware for API rate limiting
app.use(async (req, res, next) => {
  const tenantId = req.user.tenantId;

  // Check quota
  const withinQuota = await db.query(`
    SELECT check_quota($1, 'api_calls', 1) as allowed
  `, [tenantId]);

  if (!withinQuota.rows[0].allowed) {
    return res.status(429).json({
      error: 'API quota exceeded',
      message: 'Your plan allows 10,000 calls per day. Upgrade to increase limit.',
      upgrade_url: '/billing/upgrade'
    });
  }

  // Log API call
  await logApiCall(tenantId, req);

  next();
});

// Check before creating user
async function createTenantUser(tenantId, userData) {
  const canAdd = await checkQuota(tenantId, 'users');

  if (!canAdd) {
    throw new QuotaExceededError({
      quota: 'max_users',
      current: await getUserCount(tenantId),
      limit: await getQuotaLimit(tenantId, 'max_users'),
      upgradeUrl: '/billing/upgrade'
    });
  }

  return await createUser(userData);
}
```

**3. Nginx Level (Rate Limiting):**

```nginx
# nginx/sites/api.conf
limit_req_zone $tenant_id zone=tenant_api:10m rate=100r/s;

server {
  location /v1/graphql {
    limit_req zone=tenant_api burst=20 nodelay;

    # Return 429 if exceeded
    limit_req_status 429;

    proxy_pass http://hasura:8080;
  }
}
```

**4. Redis-Based Rate Limiting:**

```javascript
const redis = require('redis').createClient();

async function checkRateLimit(tenantId, limit, window) {
  const key = `ratelimit:${tenantId}:${Math.floor(Date.now() / window)}`;
  const current = await redis.incr(key);

  if (current === 1) {
    await redis.expire(key, window / 1000);
  }

  if (current > limit) {
    throw new RateLimitError({
      limit,
      window,
      retryAfter: await redis.ttl(key)
    });
  }

  return {
    allowed: true,
    remaining: limit - current,
    resetAt: await redis.ttl(key)
  };
}

// Usage
app.use(async (req, res, next) => {
  try {
    const status = await checkRateLimit(
      req.tenantId,
      10000,  // 10k requests
      86400000 // per 24 hours
    );

    res.setHeader('X-RateLimit-Remaining', status.remaining);
    res.setHeader('X-RateLimit-Reset', status.resetAt);

    next();
  } catch (err) {
    res.status(429).json({
      error: 'Rate limit exceeded',
      retryAfter: err.retryAfter
    });
  }
});
```

**5. Storage Quotas:**

```javascript
// Before file upload
async function checkStorageQuota(tenantId, fileSize) {
  const usage = await db.query(`
    SELECT
      COALESCE(SUM(file_size), 0) as current_bytes,
      q.max_storage_bytes
    FROM files f
    RIGHT JOIN tenant_quotas q ON q.tenant_id = $1
    WHERE f.tenant_id = $1 OR f.tenant_id IS NULL
    GROUP BY q.max_storage_bytes
  `, [tenantId]);

  const { current_bytes, max_storage_bytes } = usage.rows[0];

  if (current_bytes + fileSize > max_storage_bytes) {
    throw new QuotaExceededError({
      quota: 'storage',
      current: formatBytes(current_bytes),
      limit: formatBytes(max_storage_bytes),
      requested: formatBytes(fileSize)
    });
  }
}
```

**6. Feature Flags:**

```javascript
// Check feature access
async function hasFeatureAccess(tenantId, feature) {
  const result = await db.query(`
    SELECT $2 = ANY(features) as has_access
    FROM tenant_quotas
    WHERE tenant_id = $1
  `, [tenantId, feature]);

  return result.rows[0].has_access;
}

// Usage
if (!await hasFeatureAccess(tenantId, 'ai_assistant')) {
  throw new FeatureNotAvailableError({
    feature: 'ai_assistant',
    availableIn: ['professional', 'enterprise'],
    upgradeUrl: '/billing/upgrade'
  });
}
```

**[View Monitoring](../services/MONITORING-BUNDLE.md)** | **[Multi-Tenancy](#multi-tenancy)**

---

### Can I customize pricing plans?

Yes, completely customizable:

**1. Define Plans in Database:**

```sql
CREATE TABLE subscription_plans (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  slug TEXT UNIQUE NOT NULL,
  description TEXT,
  price_monthly NUMERIC NOT NULL,
  price_yearly NUMERIC,
  features JSONB NOT NULL,
  quotas JSONB NOT NULL,
  stripe_price_id TEXT,
  active BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO subscription_plans (name, slug, price_monthly, price_yearly, features, quotas, stripe_price_id) VALUES
('Free', 'free', 0, 0,
 '["basic_analytics", "email_support"]',
 '{"max_users": 3, "max_storage_gb": 1, "max_api_calls_daily": 1000}',
 NULL),

('Professional', 'professional', 29.99, 299.99,
 '["advanced_analytics", "ai_assistant", "priority_support", "custom_branding"]',
 '{"max_users": 25, "max_storage_gb": 100, "max_api_calls_daily": 100000}',
 'price_xxxxx'),

('Enterprise', 'enterprise', 199.99, 1999.99,
 '["everything", "sso", "sla", "dedicated_support", "custom_integrations"]',
 '{"max_users": null, "max_storage_gb": 1000, "max_api_calls_daily": null}',
 'price_yyyyy');
```

**2. Usage-Based Pricing:**

```sql
CREATE TABLE usage_pricing (
  id UUID PRIMARY KEY,
  plan_id UUID REFERENCES subscription_plans(id),
  metric_name TEXT NOT NULL, -- "api_calls", "storage_gb", "ai_tokens"
  included_quantity INTEGER, -- Free tier
  price_per_unit NUMERIC NOT NULL,
  unit_name TEXT NOT NULL, -- "per 1,000 calls", "per GB", etc.
  stripe_price_id TEXT
);

INSERT INTO usage_pricing (plan_id, metric_name, included_quantity, price_per_unit, unit_name) VALUES
(pro_plan_id, 'api_calls', 100000, 0.01, 'per 1,000 calls'),
(pro_plan_id, 'storage_gb', 100, 0.10, 'per GB per month'),
(pro_plan_id, 'ai_tokens', 1000000, 0.002, 'per 1,000 tokens');
```

**3. Create Stripe Products:**

```javascript
// Sync plans to Stripe
async function syncPlansToStripe() {
  const plans = await getPlans();

  for (const plan of plans) {
    // Create product
    const product = await stripe.products.create({
      name: plan.name,
      description: plan.description,
      metadata: { plan_id: plan.id }
    });

    // Create price
    const price = await stripe.prices.create({
      product: product.id,
      unit_amount: Math.round(plan.price_monthly * 100),
      currency: 'usd',
      recurring: { interval: 'month' }
    });

    // Update plan with Stripe ID
    await db.query(`
      UPDATE subscription_plans
      SET stripe_price_id = $1
      WHERE id = $2
    `, [price.id, plan.id]);
  }
}
```

**4. Plan Comparison UI:**

```graphql
query GetPlans {
  subscription_plans(where: {active: {_eq: true}}) {
    id
    name
    slug
    description
    price_monthly
    price_yearly
    features
    quotas
  }
}
```

```javascript
// Display pricing table
const plans = await getPlans();

plans.forEach(plan => {
  console.log(`
    ${plan.name}: $${plan.price_monthly}/month
    Features: ${plan.features.join(', ')}
    Limits: ${plan.quotas.max_users} users, ${plan.quotas.max_storage_gb}GB storage
  `);
});
```

**5. Custom Pricing:**

```sql
-- Enterprise custom pricing
CREATE TABLE custom_pricing (
  id UUID PRIMARY KEY,
  tenant_id UUID UNIQUE REFERENCES tenants(id),
  base_price NUMERIC NOT NULL,
  custom_quotas JSONB,
  custom_features JSONB,
  contract_start DATE,
  contract_end DATE,
  notes TEXT
);

-- Override plan quotas for specific tenant
INSERT INTO custom_pricing (tenant_id, base_price, custom_quotas) VALUES
('big-customer-id', 499.99, '{
  "max_users": 500,
  "max_storage_gb": 5000,
  "max_api_calls_daily": null,
  "dedicated_instance": true
}');
```

**6. Tiered Pricing:**

```sql
CREATE TABLE pricing_tiers (
  id UUID PRIMARY KEY,
  metric_name TEXT NOT NULL,
  min_quantity INTEGER NOT NULL,
  max_quantity INTEGER,
  price_per_unit NUMERIC NOT NULL
);

-- Volume discounts
INSERT INTO pricing_tiers (metric_name, min_quantity, max_quantity, price_per_unit) VALUES
('api_calls', 0, 100000, 0.02),           -- $0.02 per 1k for first 100k
('api_calls', 100001, 1000000, 0.015),    -- $0.015 per 1k for 100k-1M
('api_calls', 1000001, null, 0.01);       -- $0.01 per 1k for 1M+
```

**[View Stripe Plugin](../plugins/stripe.md)** | **[Multi-Tenancy](#multi-tenancy)**

---

### How do invoices work?

**Automated Invoicing via Stripe Plugin:**

**1. Stripe Auto-Generates Invoices:**

```javascript
// Stripe creates invoices automatically for subscriptions
// Invoices are synced to your database via webhooks

// View in database:
SELECT * FROM stripe_invoices WHERE customer_id = 'cus_xxx';

// Structure:
{
  id: 'in_xxxxx',
  customer_id: 'cus_xxxxx',
  subscription_id: 'sub_xxxxx',
  amount_due: 2999, // cents
  amount_paid: 2999,
  status: 'paid',
  invoice_pdf: 'https://pay.stripe.com/invoice/xxx/pdf',
  period_start: '2026-01-01',
  period_end: '2026-01-31',
  created: '2026-01-01'
}
```

**2. Access Invoices via API:**

```graphql
query GetInvoices($customerId: String!) {
  stripe_invoices(
    where: {customer_id: {_eq: $customerId}}
    order_by: {created: desc}
  ) {
    id
    amount_due
    amount_paid
    status
    invoice_pdf
    period_start
    period_end
    lines {
      description
      amount
      quantity
    }
  }
}
```

**3. Usage-Based Invoicing:**

```javascript
// Report usage throughout the month
await stripe.subscriptionItems.createUsageRecord(
  subscription_item_id,
  {
    quantity: api_calls_count,
    timestamp: Math.floor(Date.now() / 1000)
  }
);

// Stripe automatically:
// 1. Aggregates usage at end of billing period
// 2. Calculates charges based on pricing
// 3. Generates invoice
// 4. Charges customer
// 5. Sends invoice email
```

**4. Custom Invoice Generation:**

For non-Stripe invoicing:

```sql
CREATE TABLE invoices (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  invoice_number TEXT UNIQUE NOT NULL,
  amount NUMERIC NOT NULL,
  currency TEXT DEFAULT 'USD',
  status TEXT NOT NULL, -- draft, sent, paid, overdue, void
  due_date DATE NOT NULL,
  paid_at TIMESTAMPTZ,
  line_items JSONB NOT NULL,
  pdf_url TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Generate invoice
INSERT INTO invoices (
  tenant_id,
  invoice_number,
  amount,
  due_date,
  line_items
) VALUES (
  'tenant-id',
  'INV-2026-001',
  299.99,
  '2026-02-01',
  '[
    {"description": "Professional Plan - January 2026", "amount": 29.99},
    {"description": "Additional Storage (50GB)", "amount": 5.00},
    {"description": "API Overages (50k calls)", "amount": 0.50}
  ]'
);
```

**5. PDF Generation:**

```javascript
const PDFDocument = require('pdfkit');

async function generateInvoicePDF(invoiceId) {
  const invoice = await getInvoice(invoiceId);
  const tenant = await getTenant(invoice.tenant_id);

  const doc = new PDFDocument();
  const stream = fs.createWriteStream(`invoices/${invoice.invoice_number}.pdf`);

  doc.pipe(stream);

  // Header
  doc.fontSize(20).text('INVOICE', { align: 'right' });
  doc.fontSize(10).text(`Invoice #: ${invoice.invoice_number}`);
  doc.text(`Date: ${invoice.created_at.toLocaleDateString()}`);
  doc.text(`Due: ${invoice.due_date.toLocaleDateString()}`);

  // Bill to
  doc.moveDown().fontSize(12).text('Bill To:');
  doc.fontSize(10).text(tenant.name);
  doc.text(tenant.billing_address);

  // Line items
  doc.moveDown().fontSize(12).text('Description', 50, 300);
  doc.text('Amount', 400, 300, { align: 'right' });

  let y = 320;
  invoice.line_items.forEach(item => {
    doc.fontSize(10).text(item.description, 50, y);
    doc.text(`$${item.amount}`, 400, y, { align: 'right' });
    y += 20;
  });

  // Total
  doc.moveDown().fontSize(14).text(`Total: $${invoice.amount}`, { align: 'right' });

  doc.end();

  return `invoices/${invoice.invoice_number}.pdf`;
}
```

**6. Email Invoices:**

```javascript
// Send invoice email
async function sendInvoiceEmail(invoiceId) {
  const invoice = await getInvoice(invoiceId);
  const tenant = await getTenant(invoice.tenant_id);

  await sendEmail({
    to: tenant.billing_email,
    subject: `Invoice ${invoice.invoice_number} from YourApp`,
    html: `
      <h2>Invoice ${invoice.invoice_number}</h2>
      <p>Amount Due: $${invoice.amount}</p>
      <p>Due Date: ${invoice.due_date}</p>
      <p><a href="${invoice.pdf_url}">Download PDF</a></p>
      <p><a href="${invoice.payment_url}">Pay Now</a></p>
    `
  });
}
```

**[View Stripe Plugin](../plugins/stripe.md)** | **[Email Service](../commands/EMAIL.md)**

---

---

For additional topics, see the dedicated guides:

- **[Troubleshooting](../guides/TROUBLESHOOTING.md)** - Common issues and solutions
- **[Security Guide](../guides/SECURITY.md)** - Security configuration and best practices
- **[Performance Optimization](../performance/PERFORMANCE-OPTIMIZATION-V0.9.8.md)** - Scaling and performance tuning
- **[White-Label System](../features/WHITELABEL-SYSTEM.md)** - Custom branding and theming
