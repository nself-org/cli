# nself Services Documentation

nself provides a comprehensive Backend-as-a-Service platform with three categories of services: Required, Optional, and Custom. This modular architecture allows you to build exactly what you need.

## Service Categories

### [Required Services](SERVICES_REQUIRED.md) (4 services)
Core infrastructure that every nself project needs:
- **PostgreSQL** - Primary database
- **Hasura** - GraphQL API engine
- **Auth** - Authentication service
- **Nginx** - Reverse proxy and routing

### [Optional Services](SERVICES_OPTIONAL.md) (Variable)
Additional services you can enable based on your needs:
- **[Monitoring Bundle](MONITORING-BUNDLE.md)** - Complete observability stack (10 services total)
  - Prometheus, Grafana, Loki, Promtail, Tempo, Alertmanager
  - cAdvisor, Node Exporter, Postgres Exporter, Redis Exporter
- **[nself Admin](NSELF_ADMIN.md)** - Web-based management interface for your entire deployment
- **Mail Services** - Email sending and testing (MailPit for dev, SMTP for production)
- **Search Services** - Full-text search engines (MeiliSearch, Typesense, Sonic)
- **Storage Services** - Object storage and file management (MinIO, nHost Storage)
- **ML Services** - Machine learning infrastructure (MLflow, Label Studio, BentoML)
- **Cache Services** - Redis and caching layers

### [Custom Services](SERVICES_CUSTOM.md) (Up to 10)
Your own microservices built from templates or custom code:
- Backend APIs (REST, GraphQL, gRPC)
- Background workers and job processors
- WebSocket servers
- ML models and data pipelines

## Service Counts

### Minimal Setup (4 services)
Just the required services for basic functionality.

### Typical Development (15-20 services)
Required + Redis + Mail + Search + a few custom services.

### [Full Demo Setup](DEMO_SETUP.md) (24 services)
Our comprehensive demo configuration includes:
- **Required Services (4)**: PostgreSQL, Hasura, Auth, Nginx
- **Optional Services (16)**:
  - Monitoring Bundle (10): Full observability stack
  - Individual Services (6): Redis, MinIO, Storage API, MailPit, MeiliSearch, nself Admin
- **Custom Services (4)**:
  - `express_api` - Express.js REST API
  - `bullmq_worker` - Background job processor
  - `go_grpc` - Go gRPC service
  - `python_api` - Python FastAPI for ML

[→ See Complete Demo Documentation](DEMO_SETUP.md)

### Maximum Configuration (40+ services)
All available services enabled with maximum custom services.

## Service Management

### Enabling Services
Services are controlled through environment variables in your `.env` file:

```bash
# Required services (always enabled)
POSTGRES_ENABLED=true
HASURA_ENABLED=true
AUTH_ENABLED=true
NGINX_ENABLED=true

# Optional services
REDIS_ENABLED=true
MONITORING_ENABLED=true  # Enables full monitoring bundle
MEILISEARCH_ENABLED=true

# Custom services
CS_1=my_api:express-js:8001
CS_2=worker:bullmq-js:8002
```

### Service Dependencies
Some services depend on others:
- Auth requires PostgreSQL
- Hasura requires PostgreSQL
- Monitoring exporters require their respective services
- Custom services typically need PostgreSQL and/or Redis

### Resource Requirements

| Service Type | CPU | Memory | Storage |
|-------------|-----|---------|---------|
| Required (4) | 2 cores | 2GB | 1GB |
| + Monitoring (10) | 3 cores | 4GB | 5GB |
| + Custom (4) | 4 cores | 6GB | 10GB |
| Full Stack (26) | 6 cores | 8GB | 20GB |

## Quick Reference

### List All Services
```bash
nself services list         # Show all available services
nself services enabled      # Show enabled services
nself services status       # Show running services
```

### Service URLs
After starting, services are available at:
- GraphQL API: `https://api.<domain>`
- Auth: `https://auth.<domain>`
- Storage: `https://storage.<domain>`
- Monitoring: `https://grafana.<domain>`
- Custom: `https://<service>.<domain>`

## Next Steps

1. Review [Required Services](SERVICES_REQUIRED.md) to understand core components
2. Explore [Optional Services](SERVICES_OPTIONAL.md) for additional capabilities
3. Learn about [Custom Services](SERVICES_CUSTOM.md) to build your own backends
4. Check [Monitoring Bundle](MONITORING-BUNDLE.md) for observability setup

## Related Documentation

- [Environment Configuration](../configuration/ENVIRONMENT-VARIABLES.md)
- [Docker Compose Structure](../architecture/ARCHITECTURE.md)
- [Service Templates](SERVICE-TEMPLATES.md)
- [Networking & Routing](../architecture/ARCHITECTURE.md)
