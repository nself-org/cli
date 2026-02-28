# Blue-Green Deployment Pattern for nself Custom Services

Zero-downtime deployment strategy using blue-green deployments.

---

## Overview

Blue-green deployment maintains two identical production environments:

- **Blue**: Currently serving production traffic
- **Green**: New version being deployed

After the green environment is verified, traffic switches from blue to green. If issues arise, instantly rollback to blue.

---

## Architecture

```
                    ┌─────────────────┐
                    │  Load Balancer  │
                    │   (Nginx/ALB)   │
                    └────────┬────────┘
                             │
                    ┌────────┴────────┐
                    │                 │
            ┌───────▼──────┐  ┌──────▼───────┐
            │     BLUE     │  │    GREEN     │
            │  (Active)    │  │  (Standby)   │
            │              │  │              │
            │ payment-api  │  │ payment-api  │
            │ analytics    │  │ analytics    │
            │ worker       │  │ worker       │
            └──────────────┘  └──────────────┘
                    │                 │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │   PostgreSQL    │
                    │   (Shared DB)   │
                    └─────────────────┘
```

---

## Implementation Methods

### Method 1: Docker Compose (Single Server)

#### Setup

```bash
# Project structure
.
├── docker-compose.blue.yml
├── docker-compose.green.yml
├── nginx/
│   ├── nginx.conf
│   └── sites/
│       ├── blue-upstream.conf
│       └── green-upstream.conf
└── services/
    ├── payment-api/
    ├── notification-worker/
    └── analytics-api/
```

#### Blue Environment (docker-compose.blue.yml)

```yaml
version: '3.8'

services:
  # Custom Service: Payment API (Blue)
  payment-api-blue:
    build:
      context: ./services/payment-api
      dockerfile: Dockerfile
    container_name: myapp_payment_api_blue
    restart: unless-stopped
    ports:
      - "8001:8001"
    environment:
      SERVICE_NAME: payment_api
      SERVICE_PORT: 8001
      ENVIRONMENT: blue
      POSTGRES_HOST: postgres
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      REDIS_HOST: redis
    networks:
      - nself-network
    labels:
      - "deployment=blue"
      - "service=payment-api"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8001/health"]
      interval: 10s
      timeout: 3s
      retries: 3

  # Notification Worker (Blue)
  notification-worker-blue:
    build:
      context: ./services/notification-worker
      dockerfile: Dockerfile
    container_name: myapp_notification_worker_blue
    restart: unless-stopped
    environment:
      SERVICE_NAME: notification_worker
      ENVIRONMENT: blue
      REDIS_HOST: redis
      REDIS_PASSWORD: ${REDIS_PASSWORD}
    networks:
      - nself-network
    labels:
      - "deployment=blue"
      - "service=notification-worker"

  # Analytics API (Blue)
  analytics-api-blue:
    build:
      context: ./services/analytics-api
      dockerfile: Dockerfile
    container_name: myapp_analytics_api_blue
    restart: unless-stopped
    ports:
      - "8003:8003"
    environment:
      SERVICE_NAME: analytics_api
      SERVICE_PORT: 8003
      ENVIRONMENT: blue
      POSTGRES_HOST: postgres
    networks:
      - nself-network
    labels:
      - "deployment=blue"
      - "service=analytics-api"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8003/health"]
      interval: 10s
      timeout: 3s
      retries: 3

networks:
  nself-network:
    external: true
```

#### Green Environment (docker-compose.green.yml)

```yaml
version: '3.8'

services:
  # Custom Service: Payment API (Green)
  payment-api-green:
    build:
      context: ./services/payment-api
      dockerfile: Dockerfile
    container_name: myapp_payment_api_green
    restart: unless-stopped
    ports:
      - "8101:8001"  # Different external port
    environment:
      SERVICE_NAME: payment_api
      SERVICE_PORT: 8001
      ENVIRONMENT: green
      POSTGRES_HOST: postgres
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      REDIS_HOST: redis
    networks:
      - nself-network
    labels:
      - "deployment=green"
      - "service=payment-api"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8001/health"]
      interval: 10s
      timeout: 3s
      retries: 3

  # Notification Worker (Green)
  notification-worker-green:
    build:
      context: ./services/notification-worker
      dockerfile: Dockerfile
    container_name: myapp_notification_worker_green
    restart: unless-stopped
    environment:
      SERVICE_NAME: notification_worker
      ENVIRONMENT: green
      REDIS_HOST: redis
      REDIS_PASSWORD: ${REDIS_PASSWORD}
    networks:
      - nself-network
    labels:
      - "deployment=green"
      - "service=notification-worker"

  # Analytics API (Green)
  analytics-api-green:
    build:
      context: ./services/analytics-api
      dockerfile: Dockerfile
    container_name: myapp_analytics_api_green
    restart: unless-stopped
    ports:
      - "8103:8003"  # Different external port
    environment:
      SERVICE_NAME: analytics_api
      SERVICE_PORT: 8003
      ENVIRONMENT: green
      POSTGRES_HOST: postgres
    networks:
      - nself-network
    labels:
      - "deployment=green"
      - "service=analytics-api"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8003/health"]
      interval: 10s
      timeout: 3s
      retries: 3

networks:
  nself-network:
    external: true
```

#### Nginx Configuration

```nginx
# nginx/sites/payment-api.conf

# Blue upstream
upstream payment_api_blue {
    server payment-api-blue:8001 max_fails=3 fail_timeout=30s;
}

# Green upstream
upstream payment_api_green {
    server payment-api-green:8001 max_fails=3 fail_timeout=30s;
}

# Active backend (symlink to blue or green)
upstream payment_api_active {
    server payment-api-blue:8001 max_fails=3 fail_timeout=30s;
}

server {
    listen 443 ssl http2;
    server_name payment-api.example.com;

    ssl_certificate /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;

    location / {
        proxy_pass http://payment_api_active;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_next_upstream error timeout http_502 http_503 http_504;
    }

    # Blue test endpoint (for verification)
    location /blue {
        proxy_pass http://payment_api_blue;
        internal;
    }

    # Green test endpoint (for verification)
    location /green {
        proxy_pass http://payment_api_green;
        internal;
    }
}
```

#### Deployment Script

```bash
#!/bin/bash
# deploy-blue-green.sh - Blue-green deployment automation

set -e

ENVIRONMENT=${1:-green}
CURRENT_ENV=$(cat .current_deployment 2>/dev/null || echo "blue")
NEW_ENV="green"

if [ "$CURRENT_ENV" = "green" ]; then
    NEW_ENV="blue"
fi

echo "Current deployment: $CURRENT_ENV"
echo "Deploying to: $NEW_ENV"

# Step 1: Build new environment
echo "Building $NEW_ENV environment..."
docker compose -f docker-compose.${NEW_ENV}.yml build --no-cache

# Step 2: Start new environment
echo "Starting $NEW_ENV environment..."
docker compose -f docker-compose.${NEW_ENV}.yml up -d

# Step 3: Wait for services to be ready
echo "Waiting for services to be healthy..."
sleep 30

# Step 4: Health checks
echo "Running health checks on $NEW_ENV..."
HEALTH_CHECKS=(
    "http://localhost:8101/health"  # payment-api-green
    "http://localhost:8103/health"  # analytics-api-green
)

for url in "${HEALTH_CHECKS[@]}"; do
    for i in {1..30}; do
        if curl -f -s "$url" > /dev/null; then
            echo "✓ $url healthy"
            break
        fi
        if [ $i -eq 30 ]; then
            echo "✗ $url failed health check"
            echo "Deployment aborted"
            docker compose -f docker-compose.${NEW_ENV}.yml down
            exit 1
        fi
        echo "Waiting for $url... ($i/30)"
        sleep 2
    done
done

# Step 5: Run smoke tests
echo "Running smoke tests on $NEW_ENV..."
./src/scripts/smoke-tests.sh "$NEW_ENV" || {
    echo "Smoke tests failed. Rolling back."
    docker compose -f docker-compose.${NEW_ENV}.yml down
    exit 1
}

# Step 6: Switch traffic
echo "Switching traffic to $NEW_ENV..."
cat > nginx/sites/active-deployment.conf <<EOF
# Active deployment: $NEW_ENV
upstream payment_api_active {
    server payment-api-${NEW_ENV}:8001;
}

upstream analytics_api_active {
    server analytics-api-${NEW_ENV}:8003;
}
EOF

# Reload nginx
docker exec myapp_nginx nginx -s reload

echo "Traffic switched to $NEW_ENV"

# Step 7: Monitor new environment
echo "Monitoring $NEW_ENV for 60 seconds..."
sleep 60

# Step 8: Verify no errors
ERROR_COUNT=$(docker logs myapp_payment_api_${NEW_ENV} --since 60s 2>&1 | grep -i error | wc -l || true)
if [ "$ERROR_COUNT" -gt 10 ]; then
    echo "⚠ High error count detected: $ERROR_COUNT"
    echo "Consider rolling back with: ./deploy-blue-green.sh rollback"
    exit 1
fi

# Step 9: Stop old environment
echo "Stopping old $CURRENT_ENV environment..."
docker compose -f docker-compose.${CURRENT_ENV}.yml down

# Step 10: Update current deployment
echo "$NEW_ENV" > .current_deployment

echo "✓ Deployment complete!"
echo "  Active: $NEW_ENV"
echo "  Previous: $CURRENT_ENV (stopped)"
```

#### Rollback Script

```bash
#!/bin/bash
# rollback-blue-green.sh - Instant rollback

set -e

CURRENT_ENV=$(cat .current_deployment)
PREVIOUS_ENV="blue"

if [ "$CURRENT_ENV" = "blue" ]; then
    PREVIOUS_ENV="green"
fi

echo "Rolling back from $CURRENT_ENV to $PREVIOUS_ENV"

# Start previous environment (if not running)
docker compose -f docker-compose.${PREVIOUS_ENV}.yml up -d

# Wait for services
echo "Waiting for services..."
sleep 20

# Switch traffic
cat > nginx/sites/active-deployment.conf <<EOF
# Active deployment: $PREVIOUS_ENV (rollback)
upstream payment_api_active {
    server payment-api-${PREVIOUS_ENV}:8001;
}

upstream analytics_api_active {
    server analytics-api-${PREVIOUS_ENV}:8003;
}
EOF

docker exec myapp_nginx nginx -s reload

echo "$PREVIOUS_ENV" > .current_deployment
echo "✓ Rollback complete to $PREVIOUS_ENV"
```

---

### Method 2: Kubernetes

#### Blue Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payment-api-blue
  namespace: myapp
  labels:
    app: payment-api
    version: blue
spec:
  replicas: 3
  selector:
    matchLabels:
      app: payment-api
      version: blue
  template:
    metadata:
      labels:
        app: payment-api
        version: blue
    spec:
      containers:
      - name: payment-api
        image: myregistry/payment-api:v1.0.0
        ports:
        - containerPort: 8001
        # ... rest of config
```

#### Green Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payment-api-green
  namespace: myapp
  labels:
    app: payment-api
    version: green
spec:
  replicas: 3
  selector:
    matchLabels:
      app: payment-api
      version: green
  template:
    metadata:
      labels:
        app: payment-api
        version: green
    spec:
      containers:
      - name: payment-api
        image: myregistry/payment-api:v1.1.0  # New version
        ports:
        - containerPort: 8001
        # ... rest of config
```

#### Service (Traffic Selector)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: payment-api
  namespace: myapp
spec:
  selector:
    app: payment-api
    version: blue  # Change to "green" to switch traffic
  ports:
  - port: 8001
    targetPort: 8001
```

#### Switch Traffic Script

```bash
#!/bin/bash
# switch-k8s.sh - Switch traffic between blue/green in Kubernetes

CURRENT=$(kubectl get service payment-api -n myapp -o jsonpath='{.spec.selector.version}')
NEW="green"

if [ "$CURRENT" = "green" ]; then
    NEW="blue"
fi

echo "Switching from $CURRENT to $NEW"

# Update service selector
kubectl patch service payment-api -n myapp -p "{\"spec\":{\"selector\":{\"version\":\"$NEW\"}}}"

echo "✓ Traffic switched to $NEW"

# Verify
sleep 10
kubectl get endpoints payment-api -n myapp
```

---

## Complete Deployment Workflow

```bash
# 1. Deploy to green environment
./deploy-blue-green.sh green

# What it does:
# - Builds new Docker images
# - Starts green containers
# - Waits for health checks (30s)
# - Runs smoke tests
# - Switches nginx to green
# - Monitors for 60s
# - Stops blue containers

# 2. If issues detected, rollback instantly
./rollback-blue-green.sh

# Rollback is instant because:
# - Blue environment still exists
# - Just switch nginx upstream
# - No rebuild needed
```

---

## Database Migrations

Blue-green deployments require careful handling of database changes:

### Safe Migration Pattern

```bash
#!/bin/bash
# migrate-blue-green.sh

# Phase 1: Backward-compatible migration (before deployment)
echo "Running backward-compatible migrations..."
docker exec myapp_postgres psql -U postgres -d myapp_db <<EOF
-- Add new column (nullable)
ALTER TABLE payments ADD COLUMN new_field VARCHAR(255);

-- Add index in background
CREATE INDEX CONCURRENTLY idx_payments_new ON payments(new_field);
EOF

# Phase 2: Deploy green environment
./deploy-blue-green.sh green

# Green is live, blue still works with nullable column

# Phase 3: After verification, make column required (optional)
docker exec myapp_postgres psql -U postgres -d myapp_db <<EOF
-- Only after green is stable
ALTER TABLE payments ALTER COLUMN new_field SET NOT NULL;
EOF

# Phase 4: Remove blue environment
docker compose -f docker-compose.blue.yml down
```

### Breaking Changes (Requires Downtime)

```bash
# For breaking changes, use maintenance mode:

# 1. Enable maintenance mode
cat > nginx/sites/maintenance.conf <<EOF
server {
    listen 443 ssl http2;
    server_name payment-api.example.com;
    return 503 "Maintenance in progress";
}
EOF

docker exec myapp_nginx nginx -s reload

# 2. Run breaking migration
docker exec myapp_postgres psql -U postgres -d myapp_db < breaking-migration.sql

# 3. Deploy new version
./deploy-blue-green.sh green

# 4. Disable maintenance mode
# (switching to green automatically restores service)
```

---

## Monitoring During Deployment

```bash
# Monitor key metrics during blue-green switch
watch -n 1 '
echo "=== Container Status ==="
docker ps --filter "label=service=payment-api" --format "table {{.Names}}\t{{.Status}}"

echo ""
echo "=== Health Checks ==="
curl -s http://localhost:8001/health | jq .status
curl -s http://localhost:8101/health | jq .status

echo ""
echo "=== Active Nginx Upstream ==="
docker exec myapp_nginx cat /etc/nginx/sites/active-deployment.conf | grep "server payment"

echo ""
echo "=== Error Rates ==="
docker logs myapp_payment_api_blue --since 60s 2>&1 | grep -c ERROR || echo 0
docker logs myapp_payment_api_green --since 60s 2>&1 | grep -c ERROR || echo 0
'
```

---

## Benefits of Blue-Green Deployment

✅ **Zero-downtime updates** - Traffic switches instantly
✅ **Instant rollback** - Switch back to previous version in seconds
✅ **Safe testing** - Verify new version before routing traffic
✅ **Database safety** - Test migrations before full deployment
✅ **Gradual rollout** - Can route percentage of traffic to each environment

---

## Limitations

⚠️ **Resource usage** - Requires 2x infrastructure during deployment
⚠️ **Database complexity** - Migrations must be backward-compatible
⚠️ **Session handling** - Need sticky sessions or stateless design
⚠️ **Cost** - Higher cloud costs during deployment window

---

## Advanced: Canary Deployment

Gradually shift traffic from blue to green:

```nginx
# nginx/sites/payment-api.conf (canary)

upstream payment_api_backend {
    # 90% traffic to blue
    server payment-api-blue:8001 weight=90;

    # 10% traffic to green (canary)
    server payment-api-green:8001 weight=10;
}
```

Gradually increase green weight:
- 10% → monitor for issues
- 25% → continue monitoring
- 50% → verify metrics
- 100% → full rollout

---

## See Also

- [Custom Services Production Deployment](../CUSTOM-SERVICES-PRODUCTION.md)
- [Cloud Providers Deployment](../CLOUD-PROVIDERS.md)
- [Kubernetes Manifests Example](kubernetes-manifests.yml)
