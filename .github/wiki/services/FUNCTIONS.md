# Functions - Serverless Functions Runtime

## Overview

The Functions service provides a serverless runtime within nself for executing event-driven code without managing dedicated application containers. Functions are lightweight handlers triggered by HTTP requests, database events, scheduled tasks, or file upload notifications. They integrate directly with Hasura event triggers, MinIO webhooks, and the nself Auth service.

Functions is one of the 7 optional services in nself. Enable it with `FUNCTIONS_ENABLED=true` in your `.env` file. The runtime supports JavaScript and TypeScript out of the box, with each function deployed as an individual endpoint under the `functions` subdomain.

## Features

### Current Capabilities

- **HTTP Endpoints** - Create REST API endpoints with standard request/response handling
- **Event Handlers** - Process Hasura event triggers and database change notifications
- **Scheduled Tasks** - Cron-based execution for periodic background work
- **Webhook Receivers** - Accept and process incoming webhooks from external services
- **TypeScript Support** - First-class TypeScript with automatic transpilation
- **Hot Reload** - File-watching and automatic reload during development
- **Environment Access** - Full access to all nself environment variables
- **Service Connectivity** - Direct network access to PostgreSQL, Redis, MinIO, and other services
- **Middleware Support** - Request validation, authentication checks, and logging
- **Multi-Function Deployment** - Multiple functions deployed from a single directory

### Integration Points

| Service | Integration | Purpose |
|---------|------------|---------|
| Hasura | Event triggers | React to database INSERT, UPDATE, DELETE events |
| Auth | JWT validation | Authenticate function requests via nself Auth |
| PostgreSQL | Direct connection | Query and mutate data from function handlers |
| Redis | Cache/Queue | Cache results, manage state, process queues |
| MinIO | File processing | Handle file upload events and generate thumbnails |
| Monitoring | Prometheus metrics | Function invocation counts, latency, and errors |

## Configuration

### Basic Setup

Enable Functions in your `.env` file:

```bash
# Functions Configuration
FUNCTIONS_ENABLED=true
```

All other settings have sensible defaults.

### Complete Configuration Reference

```bash
# Required
FUNCTIONS_ENABLED=true

# Version
FUNCTIONS_VERSION=latest             # Runtime version (default: latest)

# Port Configuration
FUNCTIONS_PORT=3400                  # HTTP port (default: 3400)

# Route Configuration
FUNCTIONS_ROUTE=functions            # Creates functions.yourdomain.com

# Runtime Settings
FUNCTIONS_NODE_ENV=development       # Node environment (default: matches ENV)
FUNCTIONS_TIMEOUT=30                 # Function execution timeout in seconds (default: 30)
FUNCTIONS_MAX_PAYLOAD_SIZE=5mb       # Maximum request body size (default: 5mb)
FUNCTIONS_CONCURRENCY=10             # Maximum concurrent executions (default: 10)

# Development
FUNCTIONS_HOT_RELOAD=true            # Enable hot reload (default: true in dev)
FUNCTIONS_DEBUG=false                # Enable debug logging (default: false)

# Source Directory
FUNCTIONS_DIR=functions              # Directory containing function files (default: functions)

# Authentication
FUNCTIONS_AUTH_ENABLED=false         # Require authentication by default (default: false)
FUNCTIONS_WEBHOOK_SECRET=            # Shared secret for webhook validation
```

### Environment-Specific Configurations

#### Development

```bash
FUNCTIONS_ENABLED=true
FUNCTIONS_HOT_RELOAD=true
FUNCTIONS_DEBUG=true
FUNCTIONS_TIMEOUT=60
FUNCTIONS_NODE_ENV=development
```

#### Staging

```bash
FUNCTIONS_ENABLED=true
FUNCTIONS_HOT_RELOAD=false
FUNCTIONS_TIMEOUT=30
FUNCTIONS_NODE_ENV=staging
FUNCTIONS_AUTH_ENABLED=true
```

#### Production

```bash
FUNCTIONS_ENABLED=true
FUNCTIONS_HOT_RELOAD=false
FUNCTIONS_TIMEOUT=15
FUNCTIONS_NODE_ENV=production
FUNCTIONS_AUTH_ENABLED=true
FUNCTIONS_CONCURRENCY=50
FUNCTIONS_MAX_PAYLOAD_SIZE=10mb
FUNCTIONS_WEBHOOK_SECRET=secure-webhook-secret
```

## Directory Structure

Functions are organized in the `functions/` directory at the root of your nself project:

```
project/
  functions/
    api/
      hello.js              # GET/POST /api/hello
      users.js              # GET/POST /api/users
      upload-handler.ts     # POST /api/upload-handler
    events/
      user-created.js       # Hasura event handler
      order-updated.js      # Hasura event handler
    scheduled/
      daily-cleanup.js      # Cron-scheduled function
      weekly-report.ts      # Cron-scheduled function
    webhooks/
      stripe.js             # Stripe webhook handler
      github.js             # GitHub webhook handler
    _middleware.js           # Global middleware (optional)
    _utils/
      db.js                 # Shared database utilities
      email.js              # Shared email utilities
```

Each file exports a default handler function. The file path determines the HTTP route:
- `functions/api/hello.js` serves at `https://functions.local.nself.org/api/hello`
- `functions/events/user-created.js` serves at `https://functions.local.nself.org/events/user-created`

## Access

### HTTP Endpoints

**Local Development:**
- Base URL: `https://functions.local.nself.org`
- Direct access: `http://localhost:3400`

**Production:**
- Base URL: `https://functions.<your-domain>`

### Within Docker Network

Other services reach Functions at `http://functions:3400`.

## Usage

### CLI Commands

Functions are managed through the `nself service functions` command group:

```bash
# Check functions service status
nself service functions status

# List all deployed functions
nself service functions list

# View function logs
nself service functions logs

# Invoke a function from the command line
nself service functions invoke api/hello --method GET

# Invoke with a JSON payload
nself service functions invoke api/users --method POST --data '{"name": "Alice"}'

# Create a new function from a template
nself service functions create api/my-endpoint

# Validate all function files
nself service functions validate

# View function metrics
nself service functions metrics
```

### General Service Commands

```bash
# View functions runtime logs
nself logs functions

# Restart the functions service
nself restart functions

# Execute a command inside the functions container
nself exec functions node -e "console.log('test')"

# Check all service URLs
nself urls
```

### Writing Functions

#### Basic HTTP Endpoint

```javascript
// functions/api/hello.js
export default function handler(req, res) {
  if (req.method === 'GET') {
    return res.json({ message: 'Hello from nself Functions' });
  }

  if (req.method === 'POST') {
    const { name } = req.body;
    return res.json({ message: `Hello, ${name}` });
  }

  return res.status(405).json({ error: 'Method not allowed' });
}
```

#### TypeScript Endpoint

```typescript
// functions/api/users.ts
import { Request, Response } from 'express';
import { Pool } from 'pg';

const pool = new Pool({
  connectionString: process.env.DATABASE_URL,
});

export default async function handler(req: Request, res: Response) {
  if (req.method === 'GET') {
    const result = await pool.query('SELECT id, name, email FROM users LIMIT 50');
    return res.json({ users: result.rows });
  }

  if (req.method === 'POST') {
    const { name, email } = req.body;
    const result = await pool.query(
      'INSERT INTO users (name, email) VALUES ($1, $2) RETURNING *',
      [name, email]
    );
    return res.status(201).json({ user: result.rows[0] });
  }

  return res.status(405).json({ error: 'Method not allowed' });
}
```

#### Hasura Event Handler

```javascript
// functions/events/user-created.js
import nodemailer from 'nodemailer';

const transporter = nodemailer.createTransport({
  host: process.env.SMTP_HOST || 'mailpit',
  port: parseInt(process.env.SMTP_PORT || '1025'),
});

export default async function handler(req, res) {
  // Hasura event trigger payload
  const { event } = req.body;
  const { new: newUser } = event.data;

  // Send welcome email
  await transporter.sendMail({
    from: process.env.SMTP_FROM || 'noreply@local.nself.org',
    to: newUser.email,
    subject: 'Welcome to the platform',
    html: `<h1>Welcome, ${newUser.name}</h1>`,
  });

  return res.json({ success: true });
}
```

#### Scheduled Function

```javascript
// functions/scheduled/daily-cleanup.js
import { Pool } from 'pg';

// Schedule: runs daily at 2:00 AM UTC
export const config = {
  schedule: '0 2 * * *',
};

const pool = new Pool({
  connectionString: process.env.DATABASE_URL,
});

export default async function handler(req, res) {
  // Clean up expired sessions
  const result = await pool.query(
    "DELETE FROM sessions WHERE expires_at < NOW() - INTERVAL '30 days'"
  );

  return res.json({
    success: true,
    deleted: result.rowCount,
  });
}
```

#### File Upload Webhook (MinIO Integration)

```javascript
// functions/webhooks/file-uploaded.js
import sharp from 'sharp';
import { S3Client, GetObjectCommand, PutObjectCommand } from '@aws-sdk/client-s3';

const s3 = new S3Client({
  endpoint: 'http://minio:9000',
  region: 'us-east-1',
  credentials: {
    accessKeyId: process.env.MINIO_ROOT_USER,
    secretAccessKey: process.env.MINIO_ROOT_PASSWORD,
  },
  forcePathStyle: true,
});

export default async function handler(req, res) {
  const { Key, Records } = req.body;
  const record = Records[0];
  const bucket = record.s3.bucket.name;
  const key = record.s3.object.key;

  // Only process image uploads
  if (!key.match(/\.(jpg|jpeg|png|webp)$/i)) {
    return res.json({ skipped: true });
  }

  // Download original
  const original = await s3.send(new GetObjectCommand({ Bucket: bucket, Key: key }));
  const buffer = Buffer.from(await original.Body.transformToByteArray());

  // Create thumbnail
  const thumbnail = await sharp(buffer).resize(200, 200).jpeg().toBuffer();

  // Upload thumbnail
  const thumbKey = key.replace(/(\.[^.]+)$/, '-thumb$1');
  await s3.send(new PutObjectCommand({
    Bucket: bucket,
    Key: thumbKey,
    Body: thumbnail,
    ContentType: 'image/jpeg',
  }));

  return res.json({ success: true, thumbnail: thumbKey });
}
```

### Hasura Event Trigger Setup

Configure Hasura to call your functions on database events:

1. Open the Hasura Console at `https://api.local.nself.org/console`
2. Navigate to Events > Create Trigger
3. Configure the trigger:

| Setting | Value |
|---------|-------|
| Trigger Name | `user_created` |
| Table | `users` |
| Operations | Insert |
| Webhook URL | `http://functions:3400/events/user-created` |

Or configure via Hasura metadata:

```yaml
# hasura/metadata/event_triggers.yaml
- table:
    schema: public
    name: users
  event_triggers:
    - name: user_created
      definition:
        enable_manual: true
        insert:
          columns: '*'
      webhook: http://functions:3400/events/user-created
      retry_conf:
        num_retries: 3
        interval_sec: 10
```

## Network and Routing

| Access Point | Address | Purpose |
|-------------|---------|---------|
| HTTP (Browser) | `https://functions.local.nself.org` | Public function endpoints |
| HTTP (Docker) | `http://functions:3400` | Service-to-service calls |
| HTTP (Host) | `http://localhost:3400` | Local development access |

## Resource Requirements

| Resource | Minimum | Recommended | Production |
|----------|---------|-------------|------------|
| CPU | 0.25 cores | 0.5 cores | 1-2 cores |
| Memory | 128MB | 512MB | 1-2GB |
| Storage | 100MB | 500MB | 1GB |
| Network | Low | Medium | Medium-High |

Memory usage scales with function complexity and concurrent executions. Functions with large dependencies (e.g., image processing libraries) require more memory.

## Monitoring

When the monitoring bundle is enabled, Functions exposes runtime metrics.

### Available Metrics

- `functions_invocations_total` - Total function invocations by endpoint
- `functions_duration_seconds` - Function execution duration histogram
- `functions_errors_total` - Error count by endpoint and error type
- `functions_active_executions` - Currently running function count
- `functions_cold_starts_total` - Cold start count

### Grafana Dashboard

```bash
# Access Grafana
# URL: https://grafana.local.nself.org
# Navigate to: Dashboards > Functions Overview
```

### Health Checks

```bash
# Check functions service health
nself health functions

# Docker health check (built into compose config)
# Uses: curl -f http://localhost:3400/healthz
# Interval: 15s
# Timeout: 10s
# Retries: 3
```

## Security

### Authentication

Functions can require authentication for all or individual endpoints:

```bash
# Require auth for all functions
FUNCTIONS_AUTH_ENABLED=true

# Or per-function in the handler:
```

```javascript
// functions/api/protected.js
export const config = {
  auth: true,  // Require valid JWT
};

export default async function handler(req, res) {
  // req.user is populated from the JWT
  const userId = req.user.sub;
  return res.json({ userId });
}
```

### Webhook Validation

Validate incoming webhooks using shared secrets:

```javascript
// functions/webhooks/stripe.js
import crypto from 'crypto';

export default async function handler(req, res) {
  const signature = req.headers['stripe-signature'];
  const payload = JSON.stringify(req.body);
  const expected = crypto
    .createHmac('sha256', process.env.STRIPE_WEBHOOK_SECRET)
    .update(payload)
    .digest('hex');

  if (signature !== expected) {
    return res.status(401).json({ error: 'Invalid signature' });
  }

  // Process the webhook
  const event = req.body;
  // ... handle event
  return res.json({ received: true });
}
```

### Best Practices

1. Always validate and sanitize input data in function handlers
2. Use parameterized queries for all database operations
3. Set appropriate timeouts to prevent runaway functions
4. Use `FUNCTIONS_WEBHOOK_SECRET` to validate webhook sources
5. Enable authentication for sensitive endpoints
6. Keep function dependencies minimal to reduce attack surface
7. Never log sensitive data (passwords, tokens, PII)
8. Use environment variables for all secrets and configuration

## Troubleshooting

### Functions service not starting

```bash
# Check function runtime logs
nself logs functions

# Verify functions are enabled
grep FUNCTIONS_ENABLED .env

# Check for port conflicts
lsof -i :3400

# Run diagnostics
nself doctor
```

### Function returning 404

```bash
# List deployed functions
nself service functions list

# Verify the file exists in the functions directory
ls -la functions/api/

# Check the route mapping
# File: functions/api/hello.js
# URL:  https://functions.local.nself.org/api/hello

# Restart to pick up new files
nself restart functions
```

### Function timing out

```bash
# Increase the timeout
FUNCTIONS_TIMEOUT=60

# Check for long-running database queries
nself service functions logs --follow

# Verify downstream service connectivity
nself exec functions curl -s http://redis:6379/
nself exec functions curl -s http://minio:9000/
```

### Hot reload not working

```bash
# Verify hot reload is enabled
grep FUNCTIONS_HOT_RELOAD .env

# Check file permissions in the functions directory
ls -la functions/

# Restart the service to force a reload
nself restart functions
```

### Cannot connect to other services from functions

```bash
# Verify services are on the same Docker network
nself status

# Test connectivity from the functions container
nself exec functions curl -s http://redis:6379/
nself exec functions curl -s http://minio:9000/minio/health/live

# Check environment variables are available
nself exec functions env | grep DATABASE_URL
```

## Data Persistence

Functions are stateless by design. Store persistent data in PostgreSQL, Redis, or MinIO. The functions directory is mounted as a volume, so source code changes persist across container restarts.

## Related Documentation

- [Optional Services Overview](SERVICES_OPTIONAL.md) - All optional services
- [Services Overview](SERVICES.md) - Complete service listing
- [MinIO Documentation](MINIO.md) - File storage integration
- [Redis Documentation](REDIS.md) - Caching and queues
- [Environment Variables](../configuration/ENVIRONMENT-VARIABLES.md) - Full configuration reference
- [Custom Services](SERVICES_CUSTOM.md) - When you need more than serverless functions
- [Troubleshooting](../troubleshooting/README.md) - Common issues and solutions
