# Multi-Cloud Deployment Guide

**Version**: 0.4.8+ | **Last Updated**: January 30, 2026

---

## Overview

nself supports deployment to 26+ cloud providers with custom services fully integrated. This guide covers provider-specific configurations, optimization strategies, and best practices for deploying custom services (CS_N) to production cloud infrastructure.

---

## Table of Contents

1. [Provider Categories](#provider-categories)
2. [Major Cloud Providers](#major-cloud-providers)
3. [Developer Cloud Providers](#developer-cloud-providers)
4. [Budget EU Providers](#budget-eu-providers)
5. [Multi-Cloud Strategies](#multi-cloud-strategies)
6. [Cost Optimization](#cost-optimization)
7. [Auto-Scaling Configuration](#auto-scaling-configuration)
8. [Load Balancer Setup](#load-balancer-setup)
9. [Provider-Specific Examples](#provider-specific-examples)

---

## Provider Categories

| Category | Providers | Best For |
|----------|-----------|----------|
| **Major Cloud** | AWS, GCP, Azure, Oracle, IBM | Enterprise, compliance, global scale |
| **Developer Cloud** | DigitalOcean, Linode, Vultr, Scaleway, UpCloud | Simplicity, predictable pricing |
| **Budget EU** | Hetzner, Contabo, OVH, Netcup, IONOS | Best value, GDPR compliance |
| **Budget Global** | BuyVM, RackNerd, Hosthatch, Dediserve | Ultra-low cost |
| **Regional** | Tencent, Alibaba, Yandex, Selectel | China, Russia, specific regions |

---

## Major Cloud Providers

### Amazon Web Services (AWS)

#### Overview

| Attribute | Value |
|-----------|-------|
| **Best For** | Enterprise, complex architectures, massive scale |
| **Compute Options** | EC2, ECS, EKS, Fargate |
| **Managed K8s** | EKS (Elastic Kubernetes Service) |
| **Starting Price** | ~$8/mo (t3.micro) |
| **Free Tier** | 750 hours/mo EC2 for 12 months |

#### Provision Server

```bash
# Initialize AWS provider
nself infra provider init aws

# Provision EC2 instance for Docker Compose
nself infra provider server create aws --size medium --region us-east-1

# This creates:
# - t3.medium (2 vCPU, 4GB RAM)
# - 50GB SSD storage
# - Security groups configured
# - SSH key pair generated
```

#### Deploy Custom Services

```bash
# Deploy to AWS EC2
nself deploy production

# Custom services deploy via Docker Compose
# All CS_N services included automatically
```

#### Deploy to EKS (Kubernetes)

```bash
# Create EKS cluster
aws eks create-cluster \
  --name myapp-prod \
  --role-arn arn:aws:iam::123456789012:role/eks-service-role \
  --resources-vpc-config subnetIds=subnet-xxx,subnet-yyy

# Configure kubectl
aws eks update-kubeconfig --name myapp-prod --region us-east-1

# Convert and deploy
nself infra k8s convert
nself infra k8s deploy --env production

# Custom services deploy as K8s Deployments with:
# - Auto-scaling (HPA)
# - Load balancing (ALB)
# - Health checks
# - Rolling updates
```

#### Cost Optimization

```bash
# Use Spot Instances for workers
nself infra provider server create aws --spot --size large

# Use ARM instances (Graviton2) - 20% cheaper
nself infra provider server create aws --architecture arm64 --size medium

# Reserved Instances for production
# Save 30-40% with 1-year commitment
```

#### Auto-Scaling

```bash
# EKS with Cluster Autoscaler
nself infra k8s scale payment-api --auto \
  --min 3 --max 20 \
  --cpu 70

# EC2 Auto Scaling Group
aws autoscaling create-auto-scaling-group \
  --auto-scaling-group-name myapp-asg \
  --min-size 2 --max-size 10 \
  --desired-capacity 3
```

#### Load Balancer

```bash
# Application Load Balancer
aws elbv2 create-load-balancer \
  --name myapp-alb \
  --subnets subnet-xxx subnet-yyy \
  --security-groups sg-xxx

# Configure target groups for custom services
aws elbv2 create-target-group \
  --name payment-api-tg \
  --protocol HTTP \
  --port 8001 \
  --vpc-id vpc-xxx \
  --health-check-path /health
```

#### Monitoring

- CloudWatch metrics (automatic)
- nself monitoring bundle (Prometheus/Grafana)
- X-Ray for distributed tracing

#### Example Production Setup

```bash
# Complete AWS production deployment

# 1. Provision infrastructure
nself infra provider server create aws \
  --size large \
  --region us-east-1 \
  --high-availability \
  --backup-enabled

# 2. Configure environment
nself config env create prod prod

# 3. Generate secrets
nself config secrets generate --env prod

# 4. Deploy
nself deploy production

# Services deployed:
# - Core: PostgreSQL, Hasura, Auth, Nginx
# - Custom: payment-api, notification-worker, analytics-api
# - Monitoring: Prometheus, Grafana, Loki
```

---

### Google Cloud Platform (GCP)

#### Overview

| Attribute | Value |
|-----------|-------|
| **Best For** | ML/AI workloads, data analytics, BigQuery |
| **Compute Options** | Compute Engine, GKE, Cloud Run |
| **Managed K8s** | GKE (Google Kubernetes Engine) |
| **Starting Price** | ~$6/mo (e2-micro) |
| **Free Tier** | e2-micro always free |

#### Provision Server

```bash
# Initialize GCP
nself infra provider init gcp

# Provision VM instance
nself infra provider server create gcp --size medium --region us-central1

# Creates:
# - e2-medium (2 vCPU, 4GB)
# - 50GB SSD boot disk
# - Firewall rules configured
```

#### Deploy to GKE

```bash
# Create GKE cluster
gcloud container clusters create myapp-prod \
  --num-nodes=3 \
  --machine-type=e2-standard-2 \
  --region=us-central1

# Get credentials
gcloud container clusters get-credentials myapp-prod --region us-central1

# Deploy custom services
nself infra k8s convert
nself infra k8s deploy --env production
```

#### Cost Optimization

```bash
# Use preemptible VMs (80% cheaper)
nself infra provider server create gcp --preemptible --size medium

# Use committed use discounts
# Save 37% with 1-year commitment

# Use Google's free tier
nself infra provider server create gcp --size micro  # Always free
```

#### Auto-Scaling

```bash
# GKE Autopilot mode (fully managed)
gcloud container clusters create myapp-auto \
  --enable-autoscaling \
  --min-nodes=2 --max-nodes=10

# Custom service auto-scaling
nself infra k8s scale payment-api --auto \
  --min 2 --max 15 \
  --cpu 70 --memory 80
```

#### Load Balancer

```bash
# Cloud Load Balancing (automatic with GKE)
# Configured via Ingress resource

# nself generates Ingress automatically
# Access at: payment-api.example.com
```

#### Monitoring

- Cloud Monitoring (Stackdriver)
- nself monitoring bundle
- Cloud Trace for distributed tracing

---

### Microsoft Azure

#### Overview

| Attribute | Value |
|-----------|-------|
| **Best For** | Microsoft stack, enterprise, hybrid cloud |
| **Compute Options** | Virtual Machines, AKS, Container Instances |
| **Managed K8s** | AKS (Azure Kubernetes Service) |
| **Starting Price** | ~$7/mo (B1s) |
| **Free Tier** | 750 hours B1s for 12 months |

#### Provision Server

```bash
# Initialize Azure
nself infra provider init azure

# Provision VM
nself infra provider server create azure --size medium --region eastus

# Creates:
# - Standard_B2s (2 vCPU, 4GB)
# - 50GB SSD
# - Network security group
```

#### Deploy to AKS

```bash
# Create AKS cluster
az aks create \
  --resource-group myapp-rg \
  --name myapp-aks \
  --node-count 3 \
  --node-vm-size Standard_D2s_v3

# Get credentials
az aks get-credentials --resource-group myapp-rg --name myapp-aks

# Deploy
nself infra k8s deploy --env production
```

#### Cost Optimization

```bash
# Use Spot VMs
nself infra provider server create azure --spot --size large

# Use Azure reservations
# Save 40% with 1-year commitment
```

#### Auto-Scaling

```bash
# AKS cluster autoscaler
az aks update \
  --resource-group myapp-rg \
  --name myapp-aks \
  --enable-cluster-autoscaler \
  --min-count 2 --max-count 10
```

---

### Oracle Cloud Infrastructure (OCI)

#### Overview

| Attribute | Value |
|-----------|-------|
| **Best For** | Budget workloads, ARM development, always free tier |
| **Free Tier** | **2 AMD VMs + 4 ARM cores FOREVER** |
| **Compute Options** | Compute VMs, OKE (Kubernetes), Container Instances |
| **Starting Price** | **FREE** |

#### Provision Server (Free Tier)

```bash
# Initialize Oracle Cloud
nself infra provider init oracle

# Provision FREE VM (amazing value)
nself infra provider server create oracle --size free

# This gives you:
# - 2 AMD VMs (1GB each) OR 1 ARM VM (4 cores, 24GB)
# - 200GB storage
# - 10TB bandwidth/month
# - FOREVER FREE
```

#### Deploy Custom Services

```bash
# Use ARM VM for best free tier value
nself infra provider server create oracle --size free --arm

# Deploy normally
nself deploy production

# You get full nself stack + custom services for FREE:
# - PostgreSQL, Hasura, Auth, Nginx
# - payment-api, notification-worker
# - Redis, monitoring (optional)
```

#### ARM Optimization

Oracle's ARM instances are powerful and free:

```dockerfile
# Optimize Dockerfile for ARM
FROM --platform=linux/arm64 node:18-alpine

# Build for ARM
docker buildx build --platform linux/arm64 -t payment-api .
```

#### Cost (After Free Tier)

```bash
# Paid instances
nself infra provider server create oracle --size small  # ~$10/mo
nself infra provider server create oracle --size medium # ~$30/mo
```

---

## Developer Cloud Providers

### DigitalOcean

#### Overview

| Attribute | Value |
|-----------|-------|
| **Best For** | Developer experience, simple pricing, great docs |
| **Compute Options** | Droplets, DOKS (Kubernetes), App Platform |
| **Starting Price** | $6/mo |
| **Free Credit** | $200 for 60 days |

#### Provision Droplet

```bash
# Initialize DigitalOcean
nself infra provider init digitalocean

# Provision droplet
nself infra provider server create digitalocean --size medium --region nyc1

# Creates:
# - 4GB RAM, 2 vCPU
# - 80GB SSD
# - 4TB bandwidth
# - Backups enabled (optional)
```

#### Deploy Custom Services

```bash
# Deploy via Docker Compose
nself deploy production

# All custom services included:
# - CS_1: payment-api
# - CS_2: notification-worker
# - CS_3: analytics-api
```

#### Deploy to DOKS (Kubernetes)

```bash
# Create Kubernetes cluster
doctl kubernetes cluster create myapp-prod \
  --count 3 \
  --size s-2vcpu-4gb \
  --region nyc1

# Deploy custom services
nself infra k8s convert
nself infra k8s deploy --env production

# Custom services get:
# - Load balancing (via LoadBalancer service)
# - Auto-scaling (HPA)
# - Persistent volumes
```

#### Cost Optimization

```bash
# Right-size your droplet
# Start small, scale up as needed
nself infra provider server create digitalocean --size small  # $6/mo

# Use snapshots for backup (cheaper than backups)
doctl compute snapshot create --droplet-id <id>

# Reserved instances available (no commitment period)
```

#### Monitoring

- DigitalOcean monitoring (free, basic)
- nself monitoring bundle (advanced)

#### Production Example

```bash
# Recommended production setup

# 1. Provision droplet
nself infra provider server create digitalocean \
  --size dedicated-cpu-2 \
  --region nyc1 \
  --vpc myapp-vpc \
  --tag production

# 2. Setup
nself config env create prod prod
nself config secrets generate --env prod

# 3. Deploy
nself deploy production

# Cost: ~$48/mo for 2 dedicated vCPU, 4GB RAM
```

---

### Linode (Akamai)

#### Overview

| Attribute | Value |
|-----------|-------|
| **Best For** | Reliable VPS, good support, Akamai CDN |
| **Compute Options** | Linodes, LKE (Kubernetes) |
| **Starting Price** | $5/mo (Nanode) |
| **Free Credit** | $100 for 60 days |

#### Provision Linode

```bash
# Initialize Linode
nself infra provider init linode

# Provision server
nself infra provider server create linode --size medium --region us-east

# Creates:
# - 4GB RAM, 2 vCPU
# - 80GB SSD
# - 4TB bandwidth
```

#### Deploy Custom Services

```bash
nself deploy production
# Custom services deploy normally via Docker Compose
```

#### Deploy to LKE

```bash
# Create Kubernetes cluster
linode-cli lke cluster-create \
  --label myapp-prod \
  --region us-east \
  --k8s_version 1.28

# Deploy
nself infra k8s deploy --env production
```

---

### Vultr

#### Overview

| Attribute | Value |
|-----------|-------|
| **Best For** | Global coverage (32 locations), bare metal options |
| **Compute Options** | Cloud Compute, VKE (Kubernetes), Bare Metal |
| **Starting Price** | $5/mo ($2.50 for IPv6-only) |
| **Free Credit** | $250 for 30 days |

#### Provision Server

```bash
# Initialize Vultr
nself infra provider init vultr

# Provision instance
nself infra provider server create vultr --size medium --region ewr

# High-frequency compute (better CPU)
nself infra provider server create vultr --size high-frequency --region ewr

# Bare metal (dedicated hardware)
nself infra provider server create vultr --bare-metal --size bm-small
```

#### Global Deployment

Vultr has 32+ locations worldwide:

```bash
# Deploy to multiple regions
nself infra provider server create vultr --size medium --region ewr  # Newark
nself infra provider server create vultr --size medium --region lax  # Los Angeles
nself infra provider server create vultr --size medium --region fra  # Frankfurt
nself infra provider server create vultr --size medium --region sgp  # Singapore

# Setup geo-routing with CloudFlare or Route53
```

---

### Hetzner Cloud

#### Overview

| Attribute | Value |
|-----------|-------|
| **Best For** | **BEST price/performance ratio in industry** |
| **Compute Options** | Cloud Servers, Dedicated Servers |
| **Starting Price** | €3.29/mo (~$3.50) for 2GB RAM |
| **Bandwidth** | 20TB included (most generous) |

#### Provision Server

```bash
# Initialize Hetzner
nself infra provider init hetzner

# Provision cloud server
nself infra provider server create hetzner --size medium --region fsn1

# Creates CX21:
# - 2 vCPU
# - 4GB RAM
# - 40GB SSD
# - 20TB bandwidth
# - Cost: €5.83/mo (~$6.20)
```

#### Deploy Custom Services

```bash
nself deploy production

# Hetzner's value makes it perfect for:
# - Multiple custom services
# - Heavy bandwidth usage
# - Cost-conscious production
```

#### Production Recommendation

```bash
# Best value production setup
nself infra provider server create hetzner --size large --region fsn1

# CX31:
# - 2 vCPU
# - 8GB RAM
# - 80GB SSD
# - 20TB bandwidth
# - €11.90/mo (~$12.70)

# Can easily run:
# - Core services
# - 4-6 custom services
# - Full monitoring stack
# - All for ~$13/mo
```

#### Dedicated Servers

```bash
# Even better value for high-traffic apps
nself infra provider server create hetzner --dedicated --size ax41-nvme

# AX41-NVMe:
# - AMD Ryzen 5 3600 (6 cores, 12 threads)
# - 64GB RAM
# - 2x 512GB NVMe SSD
# - 20TB bandwidth
# - €49/mo (~$52)
```

---

## Multi-Cloud Strategies

### Active-Active Multi-Cloud

Deploy to multiple providers for redundancy:

```bash
# Region 1: AWS US-East (primary)
nself infra provider server create aws --region us-east-1
nself config env create prod-aws prod
nself deploy prod-aws

# Region 2: GCP US-Central (secondary)
nself infra provider server create gcp --region us-central1
nself config env create prod-gcp prod
nself deploy prod-gcp

# Region 3: Hetzner EU (tertiary)
nself infra provider server create hetzner --region fsn1
nself config env create prod-eu prod
nself deploy prod-eu
```

Setup geo-routing with CloudFlare:

```javascript
// CloudFlare Workers for geo-routing
export default {
  async fetch(request) {
    const country = request.cf.country;

    // Route by geography
    if (country === 'US') {
      return fetch('https://api-aws.example.com' + request.url.pathname);
    } else if (country in ['DE', 'FR', 'GB', 'IT', 'ES']) {
      return fetch('https://api-hetzner.example.com' + request.url.pathname);
    } else {
      return fetch('https://api-gcp.example.com' + request.url.pathname);
    }
  }
}
```

### Active-Passive (DR)

```bash
# Primary: DigitalOcean
nself deploy production

# Disaster Recovery: Vultr (standby)
nself config env create prod-dr prod
nself deploy prod-dr

# Automated failover with health checks
```

### Hybrid Cloud

```bash
# On-premises: Core database
# Cloud: Custom services + APIs

# Deploy custom services to cloud
nself infra provider server create aws --size large
CS_1=payment-api:express-ts:8001 nself deploy production

# Connect to on-prem database
POSTGRES_HOST=on-prem.example.com
POSTGRES_PORT=5432
```

---

## Cost Optimization

### Provider Price Comparison

Monthly cost for running nself + 4 custom services (4GB RAM, 2 vCPU):

| Provider | Instance | RAM | vCPU | Price/mo | Bandwidth |
|----------|----------|-----|------|----------|-----------|
| **Hetzner** | CX21 | 4GB | 2 | $6.20 | 20TB ⭐ |
| **Vultr** | Regular | 4GB | 2 | $12 | 3TB |
| **Linode** | 4GB | 4GB | 2 | $24 | 4TB |
| **DigitalOcean** | Basic | 4GB | 2 | $24 | 4TB |
| **Oracle** | Free Tier | 24GB | 4 | **FREE** | 10TB ⭐⭐ |
| **AWS** | t3.medium | 4GB | 2 | $30 | 100GB + $0.09/GB |
| **GCP** | e2-medium | 4GB | 2 | $32 | 100GB + $0.12/GB |
| **Azure** | B2s | 4GB | 2 | $34 | 100GB + $0.087/GB |

**Winner for value**:
1. Oracle (FREE tier - unbeatable)
2. Hetzner (best price/performance)
3. Vultr (good global coverage + value)

### Cost Optimization Strategies

#### 1. Right-Sizing

Start small, scale up as needed:

```bash
# Start with small instance
nself infra provider server create hetzner --size small  # 2GB RAM, $3.50/mo

# Monitor resource usage
nself metrics

# Scale up when needed
nself infra provider server create hetzner --size medium  # 4GB RAM, $6.20/mo
```

#### 2. Spot/Preemptible Instances

Save 60-80% for fault-tolerant workloads:

```bash
# AWS Spot
nself infra provider server create aws --spot --size large

# GCP Preemptible
nself infra provider server create gcp --preemptible --size large

# Good for:
# - Background workers
# - Batch processing
# - Development/staging
```

#### 3. Reserved/Committed Instances

Save 30-40% with commitment:

```bash
# AWS Reserved Instances (1-3 years)
# GCP Committed Use Discounts
# Azure Reserved VM Instances

# Best for: Production workloads with predictable usage
```

#### 4. ARM Architecture

Save 20% with ARM instances:

```bash
# AWS Graviton2
nself infra provider server create aws --architecture arm64

# Oracle Always Free ARM (4 cores, 24GB!)
nself infra provider server create oracle --size free --arm

# Build multi-arch images
docker buildx build --platform linux/amd64,linux/arm64 -t payment-api .
```

#### 5. Bandwidth Optimization

- Choose providers with generous bandwidth (Hetzner: 20TB, Oracle: 10TB free)
- Use CDN for static assets (CloudFlare, Fastly)
- Compress responses (gzip)
- Cache aggressively

#### 6. Storage Optimization

```bash
# Use object storage for files (cheaper than block storage)
MINIO_ENABLED=true nself deploy production

# Or use provider object storage
# - AWS S3
# - GCP Cloud Storage
# - DigitalOcean Spaces
```

---

## Auto-Scaling Configuration

### Docker Compose (Single Server)

Limited auto-scaling, but can use resource limits:

```yaml
# docker-compose.yml
services:
  payment_api:
    deploy:
      replicas: 3
      resources:
        limits:
          cpus: '1.0'
          memory: 512M
        reservations:
          cpus: '0.5'
          memory: 256M
```

### Kubernetes (All Providers)

Full horizontal pod autoscaling:

```bash
# Scale based on CPU
nself infra k8s scale payment-api --auto \
  --min 2 --max 10 \
  --cpu 70

# Scale based on memory
nself infra k8s scale payment-api --auto \
  --min 2 --max 10 \
  --memory 80

# Scale based on custom metrics
nself infra k8s scale payment-api --auto \
  --min 2 --max 10 \
  --custom 'http_requests_per_second>100'
```

### Provider-Specific Auto-Scaling

#### AWS

```bash
# ECS Service Auto Scaling
aws application-autoscaling register-scalable-target \
  --service-namespace ecs \
  --scalable-dimension ecs:service:DesiredCount \
  --resource-id service/myapp/payment-api \
  --min-capacity 2 \
  --max-capacity 10

# Scaling policy
aws application-autoscaling put-scaling-policy \
  --policy-name payment-api-cpu-scaling \
  --service-namespace ecs \
  --scalable-dimension ecs:service:DesiredCount \
  --resource-id service/myapp/payment-api \
  --policy-type TargetTrackingScaling \
  --target-tracking-scaling-policy-configuration \
    "PredefinedMetricSpecification={PredefinedMetricType=ECSServiceAverageCPUUtilization},TargetValue=70.0"
```

#### GCP

```bash
# Managed Instance Group Auto Scaling
gcloud compute instance-groups managed set-autoscaling myapp-mig \
  --max-num-replicas 10 \
  --min-num-replicas 2 \
  --target-cpu-utilization 0.70 \
  --cool-down-period 60
```

#### DigitalOcean

```bash
# DOKS HPA
kubectl autoscale deployment payment-api \
  --cpu-percent=70 \
  --min=2 \
  --max=10
```

---

## Load Balancer Setup

### Nginx (Built-in)

nself includes Nginx for load balancing:

```nginx
# nginx/sites/payment-api.conf (auto-generated)
upstream payment_api_backend {
    least_conn;
    server payment_api_1:8001 max_fails=3 fail_timeout=30s;
    server payment_api_2:8001 max_fails=3 fail_timeout=30s;
    server payment_api_3:8001 max_fails=3 fail_timeout=30s;
}

server {
    listen 443 ssl http2;
    server_name payment-api.example.com;

    ssl_certificate /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;

    location / {
        proxy_pass http://payment_api_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Health checks
        proxy_next_upstream error timeout http_502 http_503 http_504;
    }
}
```

### Provider Load Balancers

#### AWS Application Load Balancer

```bash
# Create ALB
aws elbv2 create-load-balancer \
  --name myapp-alb \
  --subnets subnet-xxx subnet-yyy \
  --security-groups sg-xxx

# Create target group for each custom service
aws elbv2 create-target-group \
  --name payment-api-tg \
  --protocol HTTP \
  --port 8001 \
  --vpc-id vpc-xxx \
  --health-check-enabled \
  --health-check-path /health \
  --health-check-interval-seconds 30 \
  --health-check-timeout-seconds 5 \
  --healthy-threshold-count 2 \
  --unhealthy-threshold-count 3
```

#### GCP Cloud Load Balancing

```bash
# Create backend service
gcloud compute backend-services create payment-api-backend \
  --protocol HTTP \
  --health-checks payment-api-health \
  --global

# Add instance group
gcloud compute backend-services add-backend payment-api-backend \
  --instance-group myapp-ig \
  --instance-group-zone us-central1-a \
  --global
```

#### DigitalOcean Load Balancer

```bash
# Create load balancer
doctl compute load-balancer create \
  --name myapp-lb \
  --region nyc1 \
  --forwarding-rules entry_protocol:https,entry_port:443,target_protocol:http,target_port:8001 \
  --health-check protocol:http,port:8001,path:/health \
  --tag-name payment-api
```

---

## Provider-Specific Examples

### Complete AWS Production Setup

```bash
#!/bin/bash
# Complete AWS production deployment with custom services

# 1. Initialize AWS
nself infra provider init aws
export AWS_REGION=us-east-1

# 2. Provision infrastructure
nself infra provider server create aws \
  --size large \
  --region $AWS_REGION \
  --high-availability \
  --multi-az

# 3. Setup environment
nself config env create prod prod

# 4. Configure custom services
cat > .environments/prod/.env <<EOF
ENV=prod
PROJECT_NAME=myapp
BASE_DOMAIN=example.com

# Custom Services
CS_1=payment-api:express-ts:8001
CS_2=notification-worker:bullmq-js:8002
CS_3=analytics-api:fastapi:8003
CS_4=ml-inference:fastapi:8004

# Enable monitoring
MONITORING_ENABLED=true
REDIS_ENABLED=true
MINIO_ENABLED=true
EOF

# 5. Generate secrets
nself config secrets generate --env prod

# 6. Add custom secrets
cat >> .environments/prod/.env.secrets <<EOF
PAYMENT_API_STRIPE_SECRET_KEY=sk_live_...
NOTIFICATION_WORKER_SENDGRID_API_KEY=SG....
ANALYTICS_API_CLICKHOUSE_PASSWORD=...
ML_INFERENCE_S3_SECRET_KEY=...
EOF

chmod 600 .environments/prod/.env.secrets

# 7. Deploy
nself deploy production

# 8. Configure DNS
# Point *.example.com to server IP

# 9. Setup SSL
nself auth ssl bootstrap --env prod

# 10. Verify
nself deploy health --env prod
curl https://payment-api.example.com/health
curl https://analytics-api.example.com/health

# 11. Setup monitoring alerts
# Access Grafana at https://grafana.example.com

echo "✓ AWS production deployment complete"
echo "  - Core services: Running"
echo "  - Custom services: 4 deployed"
echo "  - Monitoring: Enabled"
echo "  - SSL: Let's Encrypt configured"
```

### Complete Hetzner Budget Setup

```bash
#!/bin/bash
# Ultra-budget production setup on Hetzner
# Total cost: ~$13/mo for full stack + custom services

# 1. Initialize Hetzner
nself infra provider init hetzner

# 2. Provision server (best value)
nself infra provider server create hetzner \
  --size large \
  --region fsn1 \
  --backups

# CX31: 8GB RAM, 2 vCPU, 80GB SSD, 20TB bandwidth
# Cost: €11.90/mo (~$12.70)

# 3. Setup
nself config env create prod prod

# 4. Configure (enable everything, we have resources)
cat > .environments/prod/.env <<EOF
ENV=prod
PROJECT_NAME=myapp
BASE_DOMAIN=example.com

# Custom Services (can run 4-6 easily)
CS_1=api:express-ts:8001
CS_2=worker:bullmq-js:8002
CS_3=analytics:fastapi:8003
CS_4=webhooks:express-ts:8004

# Enable optional services
MONITORING_ENABLED=true
REDIS_ENABLED=true
MINIO_ENABLED=true
MEILISEARCH_ENABLED=true
EOF

# 5. Deploy
nself config secrets generate --env prod
nself deploy production

# 6. SSL
nself auth ssl bootstrap --env prod

echo "✓ Hetzner budget setup complete"
echo "  - Total cost: ~$13/mo"
echo "  - Services: 4 core + 7 optional + 10 monitoring + 4 custom"
echo "  - Bandwidth: 20TB included"
```

### Oracle Free Tier Setup

```bash
#!/bin/bash
# COMPLETELY FREE production setup on Oracle Cloud
# Cost: $0/mo forever

# 1. Initialize Oracle Cloud
nself infra provider init oracle

# 2. Provision FREE ARM instance
nself infra provider server create oracle --size free --arm

# This gives you:
# - 4 ARM cores (Ampere Altra)
# - 24GB RAM
# - 200GB storage
# - 10TB bandwidth
# - FOREVER FREE

# 3. Setup
nself config env create prod prod

# 4. Configure (be generous, it's free)
cat > .environments/prod/.env <<EOF
ENV=prod
PROJECT_NAME=myapp
BASE_DOMAIN=example.com

# Custom Services (ARM optimized)
CS_1=api:express-ts:8001
CS_2=worker:bullmq-js:8002
CS_3=grpc:grpc:50051

# Enable services
MONITORING_ENABLED=true
REDIS_ENABLED=true
EOF

# 5. Build for ARM
docker buildx create --use
docker buildx build --platform linux/arm64 -t myapp_api services/api

# 6. Deploy
nself config secrets generate --env prod
nself deploy production

echo "✓ Oracle free tier setup complete"
echo "  - Cost: $0/mo FOREVER"
echo "  - ARM cores: 4"
echo "  - RAM: 24GB"
echo "  - Perfect for side projects, MVPs, learning"
```

---

## Summary

### Provider Recommendations by Use Case

| Use Case | Recommended Provider | Why |
|----------|---------------------|-----|
| **Side Project / MVP** | Oracle (free tier) | Free forever, generous resources |
| **Budget Production** | Hetzner | Best value, 20TB bandwidth |
| **Developer Experience** | DigitalOcean | Simple, great docs |
| **Global Scale** | AWS / GCP | Most regions, enterprise features |
| **ML/AI Workloads** | GCP | Best ML tools |
| **European GDPR** | Hetzner / Scaleway | EU data centers |
| **High Availability** | Multi-cloud | Redundancy across providers |

### Custom Services Support

✅ All 26+ providers support custom services (CS_N)
✅ Deploy via Docker Compose (single server) or Kubernetes (multi-server)
✅ Full monitoring, logging, health checks included
✅ Auto-scaling available on K8s-enabled providers
✅ Provider-agnostic deployment (same commands everywhere)

---

## See Also

- [Custom Services Production Deployment](CUSTOM-SERVICES-PRODUCTION.md)
- [Complete Provider List](../guides/PROVIDERS-COMPLETE.md)
- [Kubernetes Management](../commands/K8S.md)
- [Production Deployment](PRODUCTION-DEPLOYMENT.md)
- [Provider Command Reference](../commands/PROVIDER.md)
