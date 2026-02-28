# Custom Services Production Deployment Guide

**Version**: 0.4.8+ | **Last Updated**: January 30, 2026

---

## Overview

This guide covers production deployment patterns for nself custom services (CS_N). Custom services are your own backend microservices that integrate with the nself infrastructure. Learn how to package, optimize, deploy, and monitor them in production.

---

## Table of Contents

1. [Understanding Custom Services](#understanding-custom-services)
2. [Production Preparation](#production-preparation)
3. [Container Optimization](#container-optimization)
4. [Deployment Methods](#deployment-methods)
5. [Environment-Specific Configuration](#environment-specific-configuration)
6. [Secrets Management](#secrets-management)
7. [CI/CD Pipelines](#cicd-pipelines)
8. [Health Checks & Monitoring](#health-checks--monitoring)
9. [Scaling & Performance](#scaling--performance)
10. [Security Best Practices](#security-best-practices)
11. [Rollback Procedures](#rollback-procedures)
12. [Troubleshooting](#troubleshooting)

---

## Understanding Custom Services

### What Are Custom Services?

Custom services (CS_N) are **independent backend applications** that run alongside nself's core services:

```bash
# In .env
CS_1=api:express-js:8001        # REST API service
CS_2=worker:bullmq-js:8002      # Background job worker
CS_3=grpc:grpc:50051            # gRPC service
CS_4=ml_api:fastapi:8004        # ML inference API
```

### Architecture

```
Production VPS
├── Core Services (Always Running)
│   ├── PostgreSQL      - Database
│   ├── Hasura          - GraphQL API
│   ├── Auth            - Authentication
│   └── Nginx           - Reverse Proxy
│
├── Optional Services (Based on *_ENABLED)
│   ├── Redis           - Cache
│   ├── MinIO           - Object Storage
│   └── ...
│
└── Custom Services (Your Applications)
    ├── CS_1: api       - Your REST API
    ├── CS_2: worker    - Background worker
    ├── CS_3: grpc      - gRPC service
    └── CS_4: ml_api    - ML inference
```

### Deployment Inclusion

Custom services are **ALWAYS deployed** when defined in your environment's `.env` file:

- ✅ Staging: All CS_N services deployed
- ✅ Production: All CS_N services deployed
- ✅ Docker Compose: Full deployment
- ✅ Kubernetes: Full deployment

**Key Point**: Unlike frontend apps (which are excluded from production by default), custom services are backend infrastructure and always deploy.

---

## Production Preparation

### 1. Service Definition

Define your services in `.environments/prod/.env`:

```bash
# Custom Services
CS_1=payment-api:express-ts:8001
CS_2=notification-worker:bullmq-js:8002
CS_3=analytics-api:fastapi:8003
CS_4=image-processor:python:8004
```

### 2. Build Custom Services

```bash
# Generate from templates
nself build

# This creates:
services/
├── payment_api/
│   ├── Dockerfile
│   ├── package.json
│   └── src/
├── notification_worker/
│   ├── Dockerfile
│   ├── package.json
│   └── worker.js
└── ...
```

### 3. Customize Implementation

Templates provide scaffolding. Add your business logic:

```javascript
// services/payment_api/src/index.ts
import express from 'express';
import { Pool } from 'pg';

const app = express();
const db = new Pool({
  host: process.env.POSTGRES_HOST,
  database: process.env.POSTGRES_DB,
  user: process.env.POSTGRES_USER,
  password: process.env.POSTGRES_PASSWORD
});

// Your payment processing logic
app.post('/process-payment', async (req, res) => {
  // Custom implementation
});

app.listen(process.env.SERVICE_PORT);
```

### 4. Local Testing

```bash
# Test locally
nself start

# Verify service is running
nself status

# Check logs
nself logs payment_api

# Test endpoints
curl http://localhost:8001/health
```

---

## Container Optimization

### Multi-Stage Docker Builds

Optimize for production with multi-stage builds:

#### Node.js/TypeScript Example

```dockerfile
# services/payment_api/Dockerfile

# Stage 1: Build
FROM node:18-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production
COPY . .
RUN npm run build

# Stage 2: Production
FROM node:18-alpine
WORKDIR /app

# Security: Run as non-root
RUN addgroup -g 1001 -S nodejs && \
    adduser -S nodejs -u 1001

# Copy only necessary files
COPY --from=builder --chown=nodejs:nodejs /app/dist ./dist
COPY --from=builder --chown=nodejs:nodejs /app/node_modules ./node_modules
COPY --from=builder --chown=nodejs:nodejs /app/package.json ./

USER nodejs
EXPOSE 8001

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=40s --retries=3 \
  CMD node -e "require('http').get('http://localhost:8001/health', (r) => {process.exit(r.statusCode === 200 ? 0 : 1)})"

CMD ["node", "dist/index.js"]
```

#### Python/FastAPI Example

```dockerfile
# services/ml_api/Dockerfile

# Stage 1: Build
FROM python:3.11-slim AS builder
WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir --user -r requirements.txt

# Stage 2: Production
FROM python:3.11-slim
WORKDIR /app

# Security: Run as non-root
RUN useradd -m -u 1001 appuser

# Copy dependencies from builder
COPY --from=builder --chown=appuser:appuser /root/.local /home/appuser/.local

# Copy application
COPY --chown=appuser:appuser . .

USER appuser
ENV PATH=/home/appuser/.local/bin:$PATH

EXPOSE 8004

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=40s --retries=3 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8004/health')"

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8004"]
```

#### Go Example

```dockerfile
# services/grpc_service/Dockerfile

# Stage 1: Build
FROM golang:1.21-alpine AS builder
WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Stage 2: Production
FROM alpine:latest
WORKDIR /root/

# Security
RUN addgroup -g 1001 -S appuser && \
    adduser -S appuser -u 1001 -G appuser

# Copy binary
COPY --from=builder --chown=appuser:appuser /app/main .

USER appuser
EXPOSE 50051

# Health check (gRPC health probe)
HEALTHCHECK --interval=30s --timeout=3s --start-period=40s --retries=3 \
  CMD ["./main", "-health-check"]

CMD ["./main"]
```

### Image Size Optimization

| Language | Base Image | Optimized Size |
|----------|------------|----------------|
| Node.js | `node:18-alpine` | 150-200 MB |
| Python | `python:3.11-slim` | 200-300 MB |
| Go | `alpine:latest` | 10-20 MB |
| Rust | `alpine:latest` | 5-15 MB |

### Build Best Practices

1. **Use .dockerignore**

```
# services/payment_api/.dockerignore
node_modules
npm-debug.log
.env
.env.local
.git
.gitignore
README.md
.vscode
.idea
coverage
test
*.test.js
*.spec.js
```

2. **Layer Caching**

```dockerfile
# Copy dependency files first (changes less frequently)
COPY package*.json ./
RUN npm ci --only=production

# Copy source code last (changes more frequently)
COPY . .
```

3. **Security Scanning**

```bash
# Scan for vulnerabilities
docker scan myapp_payment_api:latest

# Or use trivy
trivy image myapp_payment_api:latest
```

---

## Deployment Methods

### Method 1: Docker Compose (VPS)

**Best for**: Single-server deployments, staging environments

```bash
# Deploy to production VPS
nself deploy prod

# What happens:
# 1. Builds docker-compose.yml with all services
# 2. Syncs services/ directory to server
# 3. Runs docker compose up -d
# 4. Custom services start automatically
```

#### Docker Compose Output

Your custom services are added to `docker-compose.yml`:

```yaml
# Auto-generated by nself build
services:
  # Core services (postgres, hasura, auth, nginx)
  # ...

  # Custom Service 1: Payment API
  payment_api:
    build:
      context: ./services/payment_api
      dockerfile: Dockerfile
      args:
        NODE_ENV: production
    container_name: ${PROJECT_NAME}_payment_api
    restart: unless-stopped
    ports:
      - "8001:8001"
    environment:
      SERVICE_NAME: payment_api
      SERVICE_PORT: 8001
      NODE_ENV: production
      POSTGRES_HOST: postgres
      POSTGRES_PORT: 5432
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      REDIS_HOST: redis
      REDIS_PORT: 6379
      HASURA_GRAPHQL_ENDPOINT: http://hasura:8080/v1/graphql
      HASURA_ADMIN_SECRET: ${HASURA_GRAPHQL_ADMIN_SECRET}
    depends_on:
      - postgres
      - redis
    networks:
      - nself-network
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8001/health"]
      interval: 30s
      timeout: 3s
      retries: 3
      start_period: 40s
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 512M
        reservations:
          cpus: '0.5'
          memory: 256M

  # Custom Service 2: Notification Worker
  notification_worker:
    build:
      context: ./services/notification_worker
      dockerfile: Dockerfile
    container_name: ${PROJECT_NAME}_notification_worker
    restart: unless-stopped
    environment:
      SERVICE_NAME: notification_worker
      REDIS_HOST: redis
      REDIS_PORT: 6379
      REDIS_PASSWORD: ${REDIS_PASSWORD}
    depends_on:
      - redis
    networks:
      - nself-network
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 256M
```

### Method 2: Kubernetes

**Best for**: Multi-server, high availability, auto-scaling

#### Convert to Kubernetes

```bash
# Convert your deployment to K8s manifests
nself infra k8s convert

# This generates:
.nself/k8s/manifests/
├── 00-namespace.yaml
├── 10-payment-api-deployment.yaml
├── 10-payment-api-service.yaml
├── 10-payment-api-configmap.yaml
├── 11-notification-worker-deployment.yaml
├── 11-notification-worker-configmap.yaml
└── ...
```

#### Example K8s Deployment

```yaml
# .nself/k8s/manifests/10-payment-api-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payment-api
  namespace: myapp
  labels:
    app: payment-api
    component: custom-service
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  selector:
    matchLabels:
      app: payment-api
  template:
    metadata:
      labels:
        app: payment-api
    spec:
      containers:
      - name: payment-api
        image: myregistry/payment-api:v1.0.0
        ports:
        - containerPort: 8001
          name: http
        env:
        - name: SERVICE_NAME
          value: "payment_api"
        - name: SERVICE_PORT
          value: "8001"
        - name: POSTGRES_HOST
          value: "postgres"
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: password
        resources:
          requests:
            cpu: "250m"
            memory: "256Mi"
          limits:
            cpu: "1000m"
            memory: "512Mi"
        livenessProbe:
          httpGet:
            path: /health
            port: 8001
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 3
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health
            port: 8001
          initialDelaySeconds: 10
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
---
apiVersion: v1
kind: Service
metadata:
  name: payment-api
  namespace: myapp
spec:
  type: ClusterIP
  ports:
  - port: 8001
    targetPort: 8001
    protocol: TCP
    name: http
  selector:
    app: payment-api
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: payment-api-hpa
  namespace: myapp
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: payment-api
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

#### Deploy to Kubernetes

```bash
# Deploy to K8s cluster
nself infra k8s deploy --env production

# Verify deployment
nself infra k8s status

# Scale custom service
nself infra k8s scale payment-api 5

# View logs
nself infra k8s logs payment-api -f
```

### Method 3: Cloud-Specific Deployment

Custom services deploy to any nself-supported cloud provider:

| Provider | Service Type | Deployment |
|----------|--------------|------------|
| AWS | ECS/EKS | `nself provision aws && nself deploy prod` |
| Google Cloud | GKE | `nself provision gcp && nself deploy prod` |
| Azure | AKS | `nself provision azure && nself deploy prod` |
| DigitalOcean | Droplet/DOKS | `nself provision digitalocean && nself deploy prod` |
| Hetzner | Cloud/K3s | `nself provision hetzner && nself deploy prod` |

See [Cloud Provider Deployment Guide](CLOUD-PROVIDERS.md) for provider-specific details.

---

## Environment-Specific Configuration

### Environment Hierarchy

```
.environments/
├── staging/
│   ├── .env              # Staging configuration
│   ├── .env.secrets      # Staging secrets
│   └── server.json       # Staging server SSH
└── prod/
    ├── .env              # Production configuration
    ├── .env.secrets      # Production secrets (chmod 600)
    └── server.json       # Production server SSH
```

### Production .env Example

```bash
# .environments/prod/.env

# Environment
ENV=prod
PROJECT_NAME=myapp
BASE_DOMAIN=example.com

# Custom Services
CS_1=payment-api:express-ts:8001
CS_2=notification-worker:bullmq-js:8002
CS_3=analytics-api:fastapi:8003
CS_4=image-processor:python:8004

# Custom Service Configuration
PAYMENT_API_STRIPE_ENABLED=true
PAYMENT_API_PAYPAL_ENABLED=false
PAYMENT_API_RATE_LIMIT=100
NOTIFICATION_WORKER_CONCURRENCY=5
NOTIFICATION_WORKER_MAX_RETRIES=3
ANALYTICS_API_BATCH_SIZE=1000
IMAGE_PROCESSOR_MAX_SIZE_MB=10
```

### Production Secrets Example

```bash
# .environments/prod/.env.secrets (chmod 600)

# Database
POSTGRES_PASSWORD=<generated-44-chars>

# Hasura
HASURA_GRAPHQL_ADMIN_SECRET=<generated-64-chars>

# Custom Service Secrets
PAYMENT_API_STRIPE_SECRET_KEY=sk_live_...
PAYMENT_API_STRIPE_WEBHOOK_SECRET=whsec_...
NOTIFICATION_WORKER_SENDGRID_API_KEY=SG....
ANALYTICS_API_CLICKHOUSE_PASSWORD=<secure-password>
IMAGE_PROCESSOR_S3_SECRET_KEY=<aws-secret>
```

### Service-Specific Environment Variables

Custom services receive these automatically:

```bash
# Service Identity
SERVICE_NAME=payment_api
SERVICE_PORT=8001
PROJECT_NAME=myapp
BASE_DOMAIN=example.com
NODE_ENV=production

# Database Access
POSTGRES_HOST=postgres
POSTGRES_PORT=5432
POSTGRES_DB=myapp_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=<from-secrets>
DATABASE_URL=postgresql://postgres:<password>@postgres:5432/myapp_db

# Redis Access (if enabled)
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=<from-secrets>
REDIS_URL=redis://:<password>@redis:6379

# Hasura Integration
HASURA_GRAPHQL_ENDPOINT=http://hasura:8080/v1/graphql
HASURA_ADMIN_SECRET=<from-secrets>

# MinIO (if enabled)
MINIO_ENDPOINT=http://minio:9000
MINIO_ROOT_USER=<from-config>
MINIO_ROOT_PASSWORD=<from-secrets>

# Plus any custom variables you define
```

---

## Secrets Management

### Generate Production Secrets

```bash
# Generate all secrets for production
nself config secrets generate --env prod

# This creates .environments/prod/.env.secrets with:
# - POSTGRES_PASSWORD (44 chars)
# - HASURA_GRAPHQL_ADMIN_SECRET (64 chars)
# - AUTH_JWT_SECRET (64 chars)
# - REDIS_PASSWORD (44 chars)
# - All other service passwords
```

### Add Custom Service Secrets

```bash
# Manually add your API keys to .env.secrets
echo "PAYMENT_API_STRIPE_SECRET_KEY=sk_live_..." >> .environments/prod/.env.secrets

# Set proper permissions
chmod 600 .environments/prod/.env.secrets
```

### Secrets in Kubernetes

```bash
# Create K8s secret from .env.secrets
kubectl create secret generic custom-service-secrets \
  --from-env-file=.environments/prod/.env.secrets \
  --namespace=myapp

# Or use nself
nself infra k8s secrets sync --env prod
```

### Vault Integration (Advanced)

For enterprise deployments, integrate with HashiCorp Vault:

```bash
# Store secrets in Vault
vault kv put secret/myapp/payment-api \
  stripe_key=sk_live_... \
  stripe_webhook=whsec_...

# Reference in deployment
# services/payment_api/src/config.ts
import vault from 'node-vault';

const secrets = await vault.read('secret/myapp/payment-api');
const stripeKey = secrets.data.stripe_key;
```

---

## CI/CD Pipelines

### GitHub Actions

```yaml
# .github/workflows/deploy-custom-services.yml
name: Deploy Custom Services

on:
  push:
    branches: [main]
    paths:
      - 'services/**'
      - '.environments/prod/.env'

jobs:
  deploy:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v3

      - name: Install nself
        run: |
          curl -fsSL https://install.nself.org | bash
          echo "$HOME/.nself/bin" >> $GITHUB_PATH

      - name: Setup SSH Key
        run: |
          mkdir -p ~/.ssh
          echo "${{ secrets.DEPLOY_SSH_KEY }}" > ~/.ssh/deploy_key
          chmod 600 ~/.ssh/deploy_key

      - name: Setup Production Environment
        run: |
          mkdir -p .environments/prod
          echo "${{ secrets.PROD_ENV }}" > .environments/prod/.env
          echo "${{ secrets.PROD_SECRETS }}" > .environments/prod/.env.secrets
          chmod 600 .environments/prod/.env.secrets

      - name: Build Custom Services
        run: nself build

      - name: Run Tests
        run: |
          # Test each custom service
          cd services/payment_api && npm test
          cd ../notification_worker && npm test

      - name: Security Scan
        run: |
          docker scan myapp_payment_api:latest || true

      - name: Deploy to Production
        run: |
          nself deploy prod --backend-only

      - name: Health Checks
        run: |
          nself deploy health --env prod

      - name: Notify on Failure
        if: failure()
        run: |
          curl -X POST ${{ secrets.SLACK_WEBHOOK }} \
            -d '{"text":"Custom services deployment failed!"}'
```

### GitLab CI

```yaml
# .gitlab-ci.yml
stages:
  - build
  - test
  - deploy

variables:
  DOCKER_HOST: tcp://docker:2375
  DOCKER_DRIVER: overlay2

build:
  stage: build
  image: docker:latest
  services:
    - docker:dind
  script:
    - docker build -t $CI_REGISTRY_IMAGE/payment-api:$CI_COMMIT_SHORT_SHA services/payment_api
    - docker build -t $CI_REGISTRY_IMAGE/notification-worker:$CI_COMMIT_SHORT_SHA services/notification_worker
    - docker login -u $CI_REGISTRY_USER -p $CI_REGISTRY_PASSWORD $CI_REGISTRY
    - docker push $CI_REGISTRY_IMAGE/payment-api:$CI_COMMIT_SHORT_SHA
    - docker push $CI_REGISTRY_IMAGE/notification-worker:$CI_COMMIT_SHORT_SHA

test:
  stage: test
  image: node:18
  script:
    - cd services/payment_api && npm ci && npm test
    - cd ../notification_worker && npm ci && npm test

deploy:prod:
  stage: deploy
  image: alpine:latest
  only:
    - main
  before_script:
    - apk add --no-cache curl bash openssh-client rsync
    - curl -fsSL https://install.nself.org | bash
    - eval $(ssh-agent -s)
    - echo "$DEPLOY_SSH_KEY" | tr -d '\r' | ssh-add -
    - mkdir -p ~/.ssh && chmod 700 ~/.ssh
  script:
    - export PATH="$HOME/.nself/bin:$PATH"
    - nself deploy prod
  environment:
    name: production
    url: https://api.example.com
```

### CI/CD Best Practices

1. **Separate Staging/Production Pipelines**
   - Deploy to staging automatically on merge
   - Deploy to production manually or on tag

2. **Run Tests Before Deploy**
   - Unit tests
   - Integration tests
   - E2E tests for custom services

3. **Security Scanning**
   - Scan Docker images for vulnerabilities
   - Check for hardcoded secrets
   - Validate dependencies

4. **Rollback Strategy**
   - Keep previous Docker images
   - Automated rollback on health check failure
   - Manual rollback capability

5. **Monitoring Integration**
   - Post-deployment health checks
   - Automated alerts on failure
   - Performance monitoring

---

## Health Checks & Monitoring

### Implement Health Endpoints

All custom services must expose `/health`:

#### Node.js Example

```javascript
// services/payment_api/src/health.ts
import { Router } from 'express';
import { Pool } from 'pg';

const router = Router();
const db = new Pool({ /* config */ });

router.get('/health', async (req, res) => {
  const checks = {
    status: 'healthy',
    service: process.env.SERVICE_NAME,
    timestamp: new Date().toISOString(),
    uptime: process.uptime(),
    checks: {}
  };

  try {
    // Database check
    await db.query('SELECT 1');
    checks.checks.database = 'healthy';
  } catch (error) {
    checks.status = 'unhealthy';
    checks.checks.database = 'unhealthy';
  }

  // Redis check (if used)
  try {
    await redis.ping();
    checks.checks.redis = 'healthy';
  } catch (error) {
    checks.status = 'degraded';
    checks.checks.redis = 'unhealthy';
  }

  const statusCode = checks.status === 'healthy' ? 200 : 503;
  res.status(statusCode).json(checks);
});

export default router;
```

#### Python/FastAPI Example

```python
# services/ml_api/health.py
from fastapi import APIRouter, Response
from datetime import datetime
import psutil

router = APIRouter()

@router.get("/health")
async def health_check(response: Response):
    checks = {
        "status": "healthy",
        "service": os.getenv("SERVICE_NAME"),
        "timestamp": datetime.utcnow().isoformat(),
        "uptime": time.time() - start_time,
        "checks": {}
    }

    # Database check
    try:
        await db.execute("SELECT 1")
        checks["checks"]["database"] = "healthy"
    except Exception as e:
        checks["status"] = "unhealthy"
        checks["checks"]["database"] = "unhealthy"

    # System resources
    checks["checks"]["memory_percent"] = psutil.virtual_memory().percent
    checks["checks"]["cpu_percent"] = psutil.cpu_percent()

    if checks["status"] != "healthy":
        response.status_code = 503

    return checks
```

### Prometheus Metrics

Expose `/metrics` endpoint for Prometheus:

#### Node.js with prom-client

```javascript
// services/payment_api/src/metrics.ts
import { Router } from 'express';
import promClient from 'prom-client';

const router = Router();
const register = new promClient.Registry();

// Default metrics
promClient.collectDefaultMetrics({ register });

// Custom metrics
const httpRequestDuration = new promClient.Histogram({
  name: 'http_request_duration_seconds',
  help: 'Duration of HTTP requests in seconds',
  labelNames: ['method', 'route', 'status'],
  registers: [register]
});

const paymentCounter = new promClient.Counter({
  name: 'payments_processed_total',
  help: 'Total number of payments processed',
  labelNames: ['status', 'provider'],
  registers: [register]
});

router.get('/metrics', async (req, res) => {
  res.set('Content-Type', register.contentType);
  res.end(await register.metrics());
});

export { httpRequestDuration, paymentCounter };
export default router;
```

### Grafana Dashboards

nself's monitoring bundle includes Grafana. Create custom service dashboards:

```json
{
  "dashboard": {
    "title": "Custom Services Overview",
    "panels": [
      {
        "title": "Payment API Requests/sec",
        "targets": [
          {
            "expr": "rate(http_requests_total{service=\"payment_api\"}[5m])"
          }
        ]
      },
      {
        "title": "Worker Queue Length",
        "targets": [
          {
            "expr": "bullmq_queue_length{service=\"notification_worker\"}"
          }
        ]
      },
      {
        "title": "Service Response Time (p95)",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))"
          }
        ]
      }
    ]
  }
}
```

### Log Aggregation

Custom service logs are automatically collected by Promtail (if monitoring enabled):

```javascript
// Use structured logging
console.log(JSON.stringify({
  level: 'info',
  service: process.env.SERVICE_NAME,
  message: 'Payment processed',
  payment_id: paymentId,
  amount: amount,
  currency: currency,
  provider: 'stripe',
  duration_ms: duration,
  timestamp: new Date().toISOString()
}));
```

---

## Scaling & Performance

### Horizontal Scaling (Docker Compose)

```yaml
# docker-compose.yml (manual edit or custom config)
services:
  payment_api:
    # ... existing config
    deploy:
      mode: replicated
      replicas: 3
      resources:
        limits:
          cpus: '1.0'
          memory: 512M
        reservations:
          cpus: '0.5'
          memory: 256M
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 3
```

### Horizontal Scaling (Kubernetes)

```bash
# Scale manually
nself infra k8s scale payment-api 5

# Auto-scaling
nself infra k8s scale payment-api --auto \
  --min 3 --max 10 \
  --cpu 70 --memory 80
```

### Load Balancing

Nginx automatically load-balances requests to scaled services:

```nginx
# nginx/sites/payment-api.conf (auto-generated)
upstream payment_api_backend {
    least_conn;
    server payment_api_1:8001;
    server payment_api_2:8001;
    server payment_api_3:8001;
}

server {
    listen 443 ssl http2;
    server_name payment-api.example.com;

    location / {
        proxy_pass http://payment_api_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Performance Optimization

1. **Database Connection Pooling**

```javascript
// Use connection pool, not individual connections
const pool = new Pool({
  max: 20,
  min: 5,
  idleTimeoutMillis: 30000
});
```

2. **Caching**

```javascript
// Use Redis for caching
const cacheKey = `payment:${paymentId}`;
let payment = await redis.get(cacheKey);

if (!payment) {
  payment = await db.query('SELECT * FROM payments WHERE id = $1', [paymentId]);
  await redis.setex(cacheKey, 3600, JSON.stringify(payment));
}
```

3. **Async Operations**

```javascript
// Don't block on non-critical operations
await processPayment(data);
sendNotification(data).catch(err => logger.error(err)); // Fire and forget
```

4. **Rate Limiting**

```javascript
import rateLimit from 'express-rate-limit';

const limiter = rateLimit({
  windowMs: 15 * 60 * 1000, // 15 minutes
  max: 100 // limit each IP to 100 requests per windowMs
});

app.use('/api/', limiter);
```

---

## Security Best Practices

### 1. Run as Non-Root User

Already shown in Dockerfile examples above.

### 2. Environment Variable Validation

```javascript
// services/payment_api/src/config.ts
const requiredEnvVars = [
  'POSTGRES_PASSWORD',
  'HASURA_ADMIN_SECRET',
  'STRIPE_SECRET_KEY'
];

for (const envVar of requiredEnvVars) {
  if (!process.env[envVar]) {
    throw new Error(`Missing required environment variable: ${envVar}`);
  }
}
```

### 3. Input Validation

```javascript
import { body, validationResult } from 'express-validator';

app.post('/process-payment',
  body('amount').isNumeric().custom(val => val > 0),
  body('currency').isLength({ min: 3, max: 3 }),
  async (req, res) => {
    const errors = validationResult(req);
    if (!errors.isEmpty()) {
      return res.status(400).json({ errors: errors.array() });
    }
    // Process payment
  }
);
```

### 4. Secrets in Memory Only

```javascript
// Never log secrets
logger.info('Processing payment', {
  amount: payment.amount,
  // DON'T: stripe_key: process.env.STRIPE_SECRET_KEY
});

// Clear sensitive data after use
let apiKey = process.env.SOME_API_KEY;
// use apiKey
apiKey = null; // Clear from memory
```

### 5. Network Security

```yaml
# docker-compose.yml
services:
  payment_api:
    networks:
      - nself-network
    # Don't expose to host unless necessary
    # ports:
    #   - "8001:8001"  # Remove this for internal-only services
```

### 6. Dependency Scanning

```bash
# Node.js
npm audit fix

# Python
pip-audit

# Go
go list -json -m all | nancy sleuth
```

### 7. CORS Configuration

```javascript
import cors from 'cors';

const corsOptions = {
  origin: process.env.ALLOWED_ORIGINS?.split(',') || [],
  credentials: true,
  optionsSuccessStatus: 200
};

app.use(cors(corsOptions));
```

---

## Rollback Procedures

### Docker Compose Rollback

```bash
# nself keeps a backup of previous deployment
nself deploy rollback --env prod

# This restores previous docker-compose.yml and restarts services
```

### Kubernetes Rollback

```bash
# Automatic rollback on failure
nself infra k8s deploy --auto-rollback

# Manual rollback
nself infra k8s rollback payment-api

# Rollback to specific revision
nself infra k8s rollback payment-api --revision 3

# Check rollout history
nself infra k8s rollout history payment-api
```

### Manual Rollback

```bash
# SSH to server
ssh user@server.example.com

# Navigate to deployment directory
cd /opt/nself

# Check git history
git log --oneline

# Rollback to previous commit
git checkout <previous-commit>

# Rebuild and restart
docker compose down
docker compose up -d --build
```

### Blue-Green Deployment

For zero-downtime updates:

```bash
# Deploy to "green" environment
CS_1_VERSION=v2.0.0 nself deploy prod --target green

# Test green environment
curl https://green.example.com/health

# Switch traffic from blue to green
nself deploy switch --from blue --to green

# Rollback if issues
nself deploy switch --from green --to blue
```

---

## Troubleshooting

### Service Won't Start

```bash
# Check logs
nself logs payment_api

# Common issues:
# 1. Missing environment variable
# 2. Port conflict
# 3. Database connection failed
# 4. Dependency not ready

# Check service health
docker exec myapp_payment_api curl http://localhost:8001/health

# Check resource usage
docker stats myapp_payment_api
```

### Database Connection Issues

```bash
# Test database connectivity
docker exec myapp_payment_api nc -zv postgres 5432

# Check PostgreSQL logs
nself logs postgres

# Verify credentials
docker exec myapp_payment_api env | grep POSTGRES
```

### High Memory Usage

```bash
# Check memory usage
docker stats

# Increase memory limit in docker-compose.yml
deploy:
  resources:
    limits:
      memory: 1G  # Increase from 512M

# Rebuild and restart
nself deploy prod --force
```

### Slow Performance

```bash
# Check metrics in Grafana
open https://grafana.example.com

# Enable slow query logging
POSTGRES_LOG_MIN_DURATION=1000 nself deploy prod

# Profile application
# Node.js: use --inspect flag and Chrome DevTools
# Python: use cProfile
# Go: use pprof
```

### Service Crash Loop

```bash
# Check restart count
docker ps -a | grep payment_api

# View crash logs
docker logs myapp_payment_api --tail 100

# Common causes:
# 1. Uncaught exception
# 2. Out of memory
# 3. Health check failing
# 4. Port already in use

# Fix and redeploy
nself deploy prod
```

---

## Summary

Custom services in nself production deployments:

✅ **Always deployed** with core services
✅ **Fully integrated** with monitoring, logging, networking
✅ **Production-optimized** with multi-stage builds
✅ **Scalable** via Docker Compose or Kubernetes
✅ **Secure** with secrets management and non-root users
✅ **Observable** with health checks, metrics, logs
✅ **Resilient** with health checks and rollback procedures

For cloud-specific deployment patterns, see [Cloud Provider Deployment Guide](CLOUD-PROVIDERS.md).

---

## See Also

- [Custom Services Overview](../services/SERVICES_CUSTOM.md)
- [Service Templates](../services/SERVICE-TEMPLATES.md)
- [Production Deployment](PRODUCTION-DEPLOYMENT.md)
- [Cloud Providers](CLOUD-PROVIDERS.md)
- [Kubernetes Management](../commands/INFRA.md)
- [Monitoring Bundle](../services/MONITORING-BUNDLE.md)
