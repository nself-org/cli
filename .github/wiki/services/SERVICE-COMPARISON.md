# Service Comparison and Decision Matrix

Practical guide to choosing which optional services to enable in your nself deployment. Required services (PostgreSQL, Hasura, Auth, Nginx) are always running -- this page focuses on the decisions you actually need to make.

---

## Decision Matrix

Use this table to determine which optional services your project needs.

| Service | What It Does | Enable When | Skip When |
|---------|-------------|-------------|-----------|
| **Redis** | In-memory cache, session store, pub/sub messaging | You need session management, caching, background jobs (BullMQ), or horizontal scaling | Single-instance app using only JWT auth with no caching layer |
| **MinIO** | S3-compatible object storage for files and media | You handle file uploads, user avatars, documents, or need artifact storage for MLflow | Text-only application or you use an external S3 provider (AWS, GCP, Backblaze) |
| **nself-admin** | Web UI for managing your entire nself deployment | You want a visual dashboard for service health, config management, and log viewing | You prefer CLI-only workflows or are running in headless CI/CD environments |
| **Functions** | Serverless JavaScript/TypeScript runtime | You need lightweight endpoints, webhooks, or event-driven logic without a full custom service | You already have custom services (CS_N) handling all your backend logic |
| **Mail** | Email sending and testing (MailPit in dev, SMTP in prod) | Your app sends transactional email: signups, password resets, notifications | No email functionality needed, or you handle email entirely in a custom service |
| **Search** | Full-text search with typo tolerance (MeiliSearch) | You need instant search, faceted filtering, or autocomplete across your data | Simple queries handled by PostgreSQL full-text search are sufficient |
| **MLflow** | ML experiment tracking, model registry, artifact storage | You train models, run experiments, or need a model registry | No machine learning workloads in your project |
| **Monitoring** | Full observability: metrics, logs, traces, alerts (10 services) | Production deployment, staging validation, or debugging performance issues | Local development only, or you use an external monitoring provider (Datadog, New Relic) |

---

## Resource Impact

Approximate resource consumption per service under typical workloads. Actual usage varies with traffic and data volume.

| Service | RAM (Idle) | RAM (Active) | CPU (Avg) | Disk | Has Public Route |
|---------|-----------|-------------|-----------|------|-----------------|
| **PostgreSQL** | 128 MB | 256-512 MB | 0.25 cores | 1-50 GB | No (internal) |
| **Hasura** | 128 MB | 256 MB | 0.25 cores | Minimal | Yes |
| **Auth** | 64 MB | 128 MB | 0.1 cores | Minimal | Yes |
| **Nginx** | 32 MB | 64 MB | 0.1 cores | Minimal | Yes (gateway) |
| **Redis** | 64 MB | 256 MB | 0.25 cores | 1 GB | No (internal) |
| **MinIO** | 256 MB | 512 MB | 0.5 cores | Varies | Yes |
| **nself-admin** | 128 MB | 256 MB | 0.25 cores | 100 MB | Yes |
| **Functions** | 128 MB | 256 MB | 0.25 cores | 500 MB | Yes |
| **Mail (MailPit)** | 64 MB | 128 MB | 0.1 cores | 100 MB | Yes |
| **Search (MeiliSearch)** | 256 MB | 512 MB | 0.5 cores | 1-5 GB | Yes |
| **MLflow** | 256 MB | 512 MB | 0.5 cores | 1-10 GB | Yes |
| **Monitoring Bundle** | 1.5 GB | 3.5 GB | 2.5 cores | 10-26 GB | Yes (3 routes) |

### Monitoring Bundle Breakdown

The monitoring bundle enables 10 services as a group. Individual resource usage:

| Monitoring Service | RAM | CPU | Disk | Purpose |
|-------------------|-----|-----|------|---------|
| Prometheus | 512 MB | 0.5 cores | 5-10 GB | Metrics collection and storage |
| Grafana | 256 MB | 0.25 cores | 500 MB | Visualization dashboards |
| Loki | 256 MB | 0.25 cores | 2-5 GB | Log aggregation |
| Promtail | 64 MB | 0.1 cores | Minimal | Log shipping to Loki |
| Tempo | 128 MB | 0.25 cores | 1-5 GB | Distributed tracing |
| Alertmanager | 64 MB | 0.1 cores | Minimal | Alert routing |
| cAdvisor | 128 MB | 0.5 cores | Minimal | Container metrics |
| Node Exporter | 32 MB | 0.1 cores | Minimal | System metrics |
| Postgres Exporter | 32 MB | 0.1 cores | Minimal | Database metrics |
| Redis Exporter | 16 MB | 0.05 cores | Minimal | Redis metrics |

---

## Recommended Configurations

### Simple Blog or Website

Required services only. PostgreSQL stores your content, Hasura provides the API, Auth handles logins, Nginx routes traffic.

- **Containers**: 4
- **RAM**: ~400 MB
- **Recommended minimum**: 1 GB RAM, 1 vCPU

```bash
# .env - No optional services needed
PROJECT_NAME=myblog
ENV=dev
BASE_DOMAIN=localhost
POSTGRES_PASSWORD=secure-password
HASURA_GRAPHQL_ADMIN_SECRET=admin-secret
```

### SaaS Application

Add Redis for session management and caching, MinIO for file storage, and Mail for transactional email.

- **Containers**: 7
- **RAM**: ~1.2 GB
- **Recommended minimum**: 2 GB RAM, 2 vCPU

```bash
# .env - SaaS essentials
PROJECT_NAME=myapp
ENV=dev
BASE_DOMAIN=localhost
POSTGRES_PASSWORD=secure-password
HASURA_GRAPHQL_ADMIN_SECRET=admin-secret

REDIS_ENABLED=true
MINIO_ENABLED=true
MAILPIT_ENABLED=true
```

### E-Commerce Platform

Everything in SaaS, plus Search for product catalog and browse/filter functionality.

- **Containers**: 8
- **RAM**: ~1.7 GB
- **Recommended minimum**: 4 GB RAM, 2 vCPU

```bash
# .env - E-commerce stack
PROJECT_NAME=store
ENV=dev
BASE_DOMAIN=localhost
POSTGRES_PASSWORD=secure-password
HASURA_GRAPHQL_ADMIN_SECRET=admin-secret

REDIS_ENABLED=true
MINIO_ENABLED=true
MAILPIT_ENABLED=true
MEILISEARCH_ENABLED=true
```

### Enterprise / Multi-Tenant

All 7 optional services enabled. Full platform with admin UI, storage, cache, search, email, functions, and ML tracking.

- **Containers**: 11
- **RAM**: ~2.5 GB
- **Recommended minimum**: 4 GB RAM, 4 vCPU

```bash
# .env - Enterprise configuration
PROJECT_NAME=platform
ENV=dev
BASE_DOMAIN=localhost
POSTGRES_PASSWORD=secure-password
HASURA_GRAPHQL_ADMIN_SECRET=admin-secret

REDIS_ENABLED=true
MINIO_ENABLED=true
NSELF_ADMIN_ENABLED=true
FUNCTIONS_ENABLED=true
MAILPIT_ENABLED=true
MEILISEARCH_ENABLED=true
MLFLOW_ENABLED=true
```

### Full Demo (Everything)

All optional services, full monitoring bundle, plus 4 custom services. This is what `nself init --demo` generates.

- **Containers**: 25
- **RAM**: ~6 GB
- **Recommended minimum**: 8 GB RAM, 4 vCPU

```bash
# .env - Full demo
PROJECT_NAME=demo-app
ENV=dev
BASE_DOMAIN=localhost
POSTGRES_PASSWORD=secure-password
HASURA_GRAPHQL_ADMIN_SECRET=admin-secret

# All optional services
REDIS_ENABLED=true
MINIO_ENABLED=true
NSELF_ADMIN_ENABLED=true
FUNCTIONS_ENABLED=true
MAILPIT_ENABLED=true
MEILISEARCH_ENABLED=true
MLFLOW_ENABLED=true

# Monitoring bundle (10 services)
MONITORING_ENABLED=true

# Custom services (4)
CS_1=express-api:express-js:8001
CS_2=bullmq-worker:bullmq-js:8002
CS_3=grpc-api:grpc:8003
CS_4=ml-api:fastapi:8004
```

---

## Port Reference

Default ports for all services. Internal ports are used inside the Docker network. External ports are what you access from the host.

### Required Services

| Service | Internal Port | External Route | Protocol |
|---------|--------------|----------------|----------|
| PostgreSQL | 5432 | None (use `POSTGRES_PORT` for host access) | TCP |
| Hasura | 8080 | `api.<domain>` | HTTPS |
| Auth | 4000 | `auth.<domain>` | HTTPS |
| Nginx | 80, 443 | `<domain>` (gateway) | HTTP/HTTPS |

### Optional Services

| Service | Internal Port | External Route | Env Variable |
|---------|--------------|----------------|-------------|
| Redis | 6379 | None (internal only) | `REDIS_ENABLED=true` |
| MinIO API | 9000 | `minio.<domain>` | `MINIO_ENABLED=true` |
| MinIO Console | 9001 | `minio.<domain>/console` | (same) |
| nself-admin | 3021 | `admin.<domain>` | `NSELF_ADMIN_ENABLED=true` |
| Functions | 3008 | `functions.<domain>` | `FUNCTIONS_ENABLED=true` |
| MailPit SMTP | 1025 | None (internal SMTP) | `MAILPIT_ENABLED=true` |
| MailPit UI | 8025 | `mail.<domain>` | (same) |
| MeiliSearch | 7700 | `search.<domain>` | `MEILISEARCH_ENABLED=true` |
| MLflow | 5005 | `mlflow.<domain>` | `MLFLOW_ENABLED=true` |

### Monitoring Bundle

| Service | Internal Port | External Route | Can Disable |
|---------|--------------|----------------|-------------|
| Prometheus | 9090 | `prometheus.<domain>` | Not recommended |
| Grafana | 3000 | `grafana.<domain>` | Not recommended |
| Loki | 3100 | None (internal) | Not recommended |
| Promtail | 9080 | None (internal) | Required for Loki |
| Tempo | 3200, 14268 | None (internal) | `TEMPO_ENABLED=false` |
| Alertmanager | 9093 | `alertmanager.<domain>` | `ALERTMANAGER_ENABLED=false` |
| cAdvisor | 8081 | None (internal) | `CADVISOR_ENABLED=false` |
| Node Exporter | 9100 | None (internal) | `NODE_EXPORTER_ENABLED=false` |
| Postgres Exporter | 9187 | None (internal) | `POSTGRES_EXPORTER_ENABLED=false` |
| Redis Exporter | 9121 | None (internal) | `REDIS_EXPORTER_ENABLED=false` |

---

## Configuration Summary by Container Count

| Profile | Required | Optional | Monitoring | Custom | Total | Min RAM |
|---------|----------|----------|------------|--------|-------|---------|
| Simple blog | 4 | 0 | 0 | 0 | **4** | 1 GB |
| SaaS app | 4 | 3 | 0 | 0 | **7** | 2 GB |
| E-commerce | 4 | 4 | 0 | 0 | **8** | 4 GB |
| Enterprise | 4 | 7 | 0 | 0 | **11** | 4 GB |
| Prod + monitoring | 4 | 3 | 10 | 0 | **17** | 4 GB |
| Full demo | 4 | 7 | 10 | 4 | **25** | 8 GB |

Start with the fewest services you need. You can enable additional services at any time by adding the corresponding `*_ENABLED=true` variable to your `.env` file and running `nself build && nself restart`.

---

## Related Documentation

- [Services Overview](SERVICES.md) -- All service categories
- [Optional Services](SERVICES_OPTIONAL.md) -- Detailed optional service configuration
- [Monitoring Bundle](MONITORING-BUNDLE.md) -- Full monitoring stack reference
- [Custom Services](SERVICES_CUSTOM.md) -- Building your own services with CS_N
- [Demo Setup](DEMO_SETUP.md) -- Full 25-container demo configuration
- [Start Command Options](../configuration/START-COMMAND-OPTIONS.md) -- Tuning startup behavior

---

**[Back to Services Index](INDEX.md)** | **[Back to Documentation Home](../README.md)**
