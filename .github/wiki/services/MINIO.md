# MinIO - S3-Compatible Object Storage

## Overview

MinIO is a high-performance, S3-compatible object storage service included as an optional component in nself. It provides a fully self-hosted alternative to AWS S3, Google Cloud Storage, and Azure Blob Storage, allowing your application to handle file uploads, media storage, backups, and static asset delivery without relying on external cloud providers.

MinIO is one of the 7 optional services in nself. Enable it with `MINIO_ENABLED=true` in your `.env` file. Once enabled, nself generates the Docker Compose configuration, creates default buckets, and routes the MinIO Console through nginx for browser-based management.

## Features

### Current Capabilities

- **S3-Compatible API** - Full compatibility with the AWS S3 API and client libraries
- **Web Console** - Built-in browser-based management interface for buckets and objects
- **Bucket Management** - Create, configure, and manage storage buckets with policies
- **Access Policies** - Granular read/write/admin policies per bucket and user
- **Versioning** - Object versioning for audit trails and rollback
- **Lifecycle Rules** - Automatic object expiration and tiering
- **Event Notifications** - Webhook and queue-based notifications on object changes
- **Encryption** - Server-side encryption for data at rest
- **Multi-Part Uploads** - Efficient handling of large file uploads
- **Presigned URLs** - Temporary signed URLs for secure direct access

### Integration Points

MinIO integrates with the following nself services:

| Service | Integration | Purpose |
|---------|------------|---------|
| Auth | Presigned URL generation | Secure file access via authenticated endpoints |
| Hasura | Actions/Events | File metadata in GraphQL, event-driven processing |
| Custom Services (CS_N) | S3 SDK | Direct file operations from application code |
| MLflow | Artifact store | ML model and experiment artifact storage |
| Functions | Event triggers | Serverless file processing on upload events |
| Monitoring | Prometheus metrics | Storage usage and request metrics |

## Configuration

### Basic Setup

Enable MinIO in your `.env` file:

```bash
# MinIO Configuration
MINIO_ENABLED=true
```

nself provides sensible defaults for all other settings, including auto-generated credentials.

### Complete Configuration Reference

```bash
# Required
MINIO_ENABLED=true

# Version
MINIO_VERSION=latest                 # Docker image tag (default: latest)

# Port Configuration
MINIO_PORT=9000                      # S3 API port (default: 9000)
MINIO_CONSOLE_PORT=9001              # Web Console port (default: 9001)

# Authentication
MINIO_ROOT_USER=minioadmin           # Root username (auto-generated if not set)
MINIO_ROOT_PASSWORD=minioadmin       # Root password (auto-generated if not set)

# Bucket Configuration
MINIO_DEFAULT_BUCKETS=uploads,public,private  # Comma-separated list (default: uploads)
MINIO_REGION=us-east-1               # S3 region (default: us-east-1)

# Route Configuration
MINIO_ROUTE=minio                    # Creates minio.yourdomain.com
MINIO_API_ROUTE=storage              # Creates storage.yourdomain.com for API

# Storage
MINIO_DATA_DIR=/data                 # Internal data directory (do not change)
MINIO_STORAGE_LIMIT=0                # Storage quota in bytes (0 = unlimited)

# Performance
MINIO_BROWSER=on                     # Enable web console (default: on)
MINIO_COMPRESSION=true               # Enable transparent compression (default: true)

# Notifications
MINIO_NOTIFY_WEBHOOK_ENABLED=false   # Webhook notifications on events
MINIO_NOTIFY_WEBHOOK_ENDPOINT=       # Webhook URL for notifications
```

### Environment-Specific Configurations

#### Development

```bash
MINIO_ENABLED=true
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin
MINIO_DEFAULT_BUCKETS=uploads,avatars,documents
```

#### Staging

```bash
MINIO_ENABLED=true
MINIO_ROOT_USER=staging-minio-user
MINIO_ROOT_PASSWORD=staging-secure-password
MINIO_DEFAULT_BUCKETS=uploads,public,private,backups
MINIO_COMPRESSION=true
```

#### Production

```bash
MINIO_ENABLED=true
MINIO_ROOT_USER=prod-minio-user
MINIO_ROOT_PASSWORD=strong-production-password
MINIO_DEFAULT_BUCKETS=uploads,public,private,backups,exports
MINIO_COMPRESSION=true
MINIO_STORAGE_LIMIT=107374182400     # 100GB limit
```

### Bucket Policies

nself configures default bucket policies during build:

| Bucket | Default Policy | Description |
|--------|---------------|-------------|
| `uploads` | Private | Authenticated upload/download only |
| `public` | Read-only public | Publicly accessible files |
| `private` | Private | Internal service-to-service storage |

## Access

### Web Console

After enabling and starting MinIO:

**Local Development:**
- Console URL: `https://minio.local.nself.org`
- Default credentials: Value of `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD`

**Production:**
- Console URL: `https://minio.<your-domain>`
- Requires authentication with configured credentials

### S3 API Endpoint

**Local Development:**
- API URL: `https://storage.local.nself.org` or `http://localhost:9000`

**Within Docker Network:**
- API URL: `http://minio:9000`

## Usage

### CLI Commands

MinIO is managed through the `nself service storage` command group:

```bash
# Check MinIO status
nself service storage status

# List all buckets
nself service storage list

# Create a new bucket
nself service storage create-bucket my-bucket

# Upload a file
nself service storage upload ./local-file.jpg uploads/images/photo.jpg

# Download a file
nself service storage download uploads/images/photo.jpg ./downloaded.jpg

# Generate a presigned URL (valid for 1 hour)
nself service storage presign uploads/images/photo.jpg --expiry 3600

# View bucket policy
nself service storage policy uploads

# Set bucket policy to public-read
nself service storage policy uploads --set public-read

# View storage usage
nself service storage usage
```

### General Service Commands

```bash
# View MinIO logs
nself logs minio

# Restart MinIO
nself restart minio

# Execute command inside MinIO container
nself exec minio mc ls local/

# Check all service URLs
nself urls
```

### Connecting from Custom Services

MinIO is accessible within the Docker network at `minio:9000`. Use any S3-compatible SDK.

#### Node.js (AWS SDK v3)

```javascript
import { S3Client, PutObjectCommand, GetObjectCommand } from '@aws-sdk/client-s3';
import { getSignedUrl } from '@aws-sdk/s3-request-presigner';

const s3 = new S3Client({
  endpoint: process.env.MINIO_ENDPOINT || 'http://minio:9000',
  region: process.env.MINIO_REGION || 'us-east-1',
  credentials: {
    accessKeyId: process.env.MINIO_ROOT_USER,
    secretAccessKey: process.env.MINIO_ROOT_PASSWORD,
  },
  forcePathStyle: true,  // Required for MinIO
});

// Upload a file
await s3.send(new PutObjectCommand({
  Bucket: 'uploads',
  Key: 'images/photo.jpg',
  Body: fileBuffer,
  ContentType: 'image/jpeg',
}));

// Generate a presigned download URL
const url = await getSignedUrl(s3, new GetObjectCommand({
  Bucket: 'uploads',
  Key: 'images/photo.jpg',
}), { expiresIn: 3600 });
```

#### Python (boto3)

```python
import boto3
import os

s3 = boto3.client(
    's3',
    endpoint_url=os.environ.get('MINIO_ENDPOINT', 'http://minio:9000'),
    aws_access_key_id=os.environ.get('MINIO_ROOT_USER'),
    aws_secret_access_key=os.environ.get('MINIO_ROOT_PASSWORD'),
    region_name=os.environ.get('MINIO_REGION', 'us-east-1'),
)

# Upload a file
s3.upload_file('local-file.jpg', 'uploads', 'images/photo.jpg')

# Generate presigned URL
url = s3.generate_presigned_url(
    'get_object',
    Params={'Bucket': 'uploads', 'Key': 'images/photo.jpg'},
    ExpiresIn=3600
)
```

#### Go (MinIO SDK)

```go
import "github.com/minio/minio-go/v7"
import "github.com/minio/minio-go/v7/pkg/credentials"

client, err := minio.New(os.Getenv("MINIO_ENDPOINT"), &minio.Options{
    Creds:  credentials.NewStaticV4(os.Getenv("MINIO_ROOT_USER"), os.Getenv("MINIO_ROOT_PASSWORD"), ""),
    Secure: false,
})

// Upload a file
_, err = client.FPutObject(ctx, "uploads", "images/photo.jpg", "local-file.jpg", minio.PutObjectOptions{
    ContentType: "image/jpeg",
})

// Generate presigned URL
url, err := client.PresignedGetObject(ctx, "uploads", "images/photo.jpg", time.Hour, nil)
```

### File Upload Pipeline

A common pattern in nself is a file upload pipeline that combines MinIO with Hasura events:

1. Client requests a presigned upload URL from a custom service
2. Client uploads the file directly to MinIO using the presigned URL
3. MinIO sends a webhook notification to a Functions endpoint
4. The Functions endpoint processes the file (resize, scan, etc.)
5. File metadata is stored in PostgreSQL via Hasura

```bash
# Enable the required services
MINIO_ENABLED=true
FUNCTIONS_ENABLED=true
MINIO_NOTIFY_WEBHOOK_ENABLED=true
MINIO_NOTIFY_WEBHOOK_ENDPOINT=http://functions:3400/api/file-uploaded
```

## Network and Routing

| Access Point | Address | Purpose |
|-------------|---------|---------|
| Console (Browser) | `https://minio.local.nself.org` | Web management interface |
| S3 API (Docker) | `http://minio:9000` | Service-to-service file operations |
| S3 API (Host) | `http://localhost:9000` | Local development access |
| Console (Host) | `http://localhost:9001` | Direct console access |

## Resource Requirements

| Resource | Minimum | Recommended | Production |
|----------|---------|-------------|------------|
| CPU | 0.25 cores | 0.5 cores | 1-2 cores |
| Memory | 256MB | 512MB | 2-4GB |
| Storage | 1GB | 10GB | 100GB+ |
| Network | Low | Medium | High |

Memory usage increases with concurrent connections and large file operations. Storage scales with your data volume.

## Monitoring

When the monitoring bundle is enabled, MinIO exposes Prometheus-compatible metrics.

### Available Metrics

- `minio_s3_requests_total` - Total S3 API requests by method
- `minio_s3_traffic_received_bytes` - Bytes received
- `minio_s3_traffic_sent_bytes` - Bytes sent
- `minio_bucket_usage_total_bytes` - Storage usage per bucket
- `minio_bucket_objects_count` - Object count per bucket
- `minio_node_disk_used_bytes` - Disk utilization

### Grafana Dashboard

```bash
# Access Grafana
# URL: https://grafana.local.nself.org
# Navigate to: Dashboards > MinIO Overview
```

### Health Checks

```bash
# Check MinIO health
nself health minio

# Docker health check (built into compose config)
# Uses: curl -f http://localhost:9000/minio/health/live
# Interval: 15s
# Timeout: 10s
# Retries: 3
```

## Security

### Authentication

MinIO uses root credentials for administrative access and supports creating additional users with limited permissions through the web console or CLI.

### Bucket Policies

Control access at the bucket level:

- **Private** - Only authenticated users with credentials can access
- **Public-Read** - Anyone can download, only authenticated users can upload
- **Public-Read-Write** - Open access (use with caution)
- **Custom** - Fine-grained JSON policies matching AWS S3 policy format

### Best Practices

1. Change default root credentials in staging and production
2. Use presigned URLs for client-side uploads instead of exposing credentials
3. Set appropriate bucket policies (default to private)
4. Enable server-side encryption for sensitive data
5. Use lifecycle rules to automatically expire temporary files
6. Restrict console access in production via IP whitelisting
7. Regularly audit bucket policies and access patterns

## Troubleshooting

### MinIO not starting

```bash
# Check MinIO logs
nself logs minio

# Verify MinIO is enabled
grep MINIO_ENABLED .env

# Check for port conflicts
lsof -i :9000
lsof -i :9001

# Run diagnostics
nself doctor
```

### Cannot access web console

```bash
# Verify nginx routing
nself urls

# Check MinIO is running
nself status

# Test direct console access
curl -s http://localhost:9001/

# Rebuild nginx configuration
nself build --force && nself restart nginx
```

### Upload failures

```bash
# Check available disk space
df -h

# Verify bucket exists
nself exec minio mc ls local/

# Check bucket policy allows uploads
nself exec minio mc policy get local/uploads

# Test S3 API connectivity
nself exec minio mc admin info local
```

### Permission denied errors

```bash
# Verify credentials in .env
grep MINIO_ROOT .env

# Check bucket policy
nself service storage policy uploads

# Test with root credentials
nself exec minio mc ls local/uploads/
```

### Large file upload timeouts

```bash
# MinIO supports multi-part uploads for large files
# Ensure your client SDK is configured for multi-part:
# - AWS SDK: automatically uses multi-part for files > 5MB
# - MinIO SDK: configurable threshold

# Increase nginx timeout if needed (in .env)
NGINX_PROXY_READ_TIMEOUT=600
NGINX_CLIENT_MAX_BODY_SIZE=1g
```

## Data Persistence

MinIO data is stored in a Docker volume named `${PROJECT_NAME}_minio_data`. This volume persists across container restarts and rebuilds.

### Backup

```bash
# Include MinIO in full backup
nself backup create --include minio

# Mirror a bucket to a local directory
nself exec minio mc mirror local/uploads /tmp/uploads-backup
```

### Restore

```bash
# Restore from backup
nself backup restore --include minio --from backup-2026-01-15.tar.gz
```

### Migration to External S3

If you later need to migrate from self-hosted MinIO to AWS S3 or another provider, use the MinIO client mirror command:

```bash
# Configure remote S3 target
nself exec minio mc alias set aws https://s3.amazonaws.com ACCESS_KEY SECRET_KEY

# Mirror all data
nself exec minio mc mirror local/uploads aws/your-bucket/uploads
```

## Related Documentation

- [Optional Services Overview](SERVICES_OPTIONAL.md) - All optional services
- [Services Overview](SERVICES.md) - Complete service listing
- [Functions Documentation](FUNCTIONS.md) - Serverless file processing
- [Environment Variables](../configuration/ENVIRONMENT-VARIABLES.md) - Full configuration reference
- [Custom Services](SERVICES_CUSTOM.md) - Using MinIO in custom services
- [Troubleshooting](../troubleshooting/README.md) - Common issues and solutions
