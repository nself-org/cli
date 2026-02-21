# Migrating from Nhost to nself

**Last Updated**: January 30, 2026
**Migration Difficulty**: Medium
**Estimated Time**: 4-8 hours
**Compatibility**: 95% (same core stack)

---

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Feature Comparison](#feature-comparison)
4. [Step-by-Step Migration](#step-by-step-migration)
5. [Data Migration](#data-migration)
6. [Authentication Migration](#authentication-migration)
7. [Storage Migration](#storage-migration)
8. [Functions Migration](#functions-migration)
9. [GraphQL Schema Migration](#graphql-schema-migration)
10. [Frontend Code Changes](#frontend-code-changes)
11. [Common Pitfalls](#common-pitfalls)
12. [Rollback Procedure](#rollback-procedure)
13. [Automated Migration Tools](#automated-migration-tools)

---

## Overview

Nhost and nself share the same core technology stack (PostgreSQL, Hasura, nHost Auth), making migration straightforward. The primary difference is deployment model: Nhost is cloud-first with a management dashboard, while nself is CLI-first and self-hosted.

### Why Migrate to nself?

- **Full infrastructure control** - Own your data and deployment
- **No vendor lock-in** - Open-source stack
- **Cost optimization** - Predictable self-hosting costs vs. usage-based pricing
- **Advanced features** - Multi-tenancy, billing integration, plugin system (v0.8.0+)
- **More database tools** - DBML support, type generation, profiling
- **Extensive CLI** - 56 commands for every operation

### Key Differences

| Aspect | Nhost | nself |
|--------|-------|-------|
| **Interface** | Web dashboard + CLI | CLI + optional web admin |
| **Hosting** | Managed cloud or self-hosted | Self-hosted only |
| **Configuration** | Web UI, config files | `.env` file + CLI commands |
| **Deployment** | `nhost deploy` | `nself build && nself start` |
| **User Management** | Dashboard UI | CLI (`nself auth user create`) |
| **Database Backups** | Automatic (cloud) | Manual + Scheduled (`nself backup create`) |

---

## Prerequisites

### Before You Start

- [ ] Access to Nhost project (admin credentials)
- [ ] PostgreSQL dump capability (or Nhost CLI access)
- [ ] Hasura metadata export capability
- [ ] List of all environment variables used
- [ ] Storage bucket inventory
- [ ] nself installed on target server
- [ ] Backup of current Nhost project

### Required Tools

```bash
# Install nself
brew install nself
# OR
curl -sSL https://install.nself.org | bash

# Install Nhost CLI (for data export)
npm install -g nhost

# Install PostgreSQL client tools
# macOS:
brew install postgresql
# Ubuntu/Debian:
sudo apt-get install postgresql-client
```

### Migration Checklist

```bash
# 1. Document current setup
nhost config show > nhost-config.txt
nhost env list > nhost-env.txt

# 2. Create full backup
nhost db dump > nhost-backup.sql
nhost metadata export > nhost-metadata.json

# 3. Export storage inventory
# (via Nhost Dashboard or API)

# 4. List all services in use
# - Auth providers enabled
# - Functions deployed
# - Custom domains
# - Environment variables
```

---

## Feature Comparison

### Authentication Features

| Feature | Nhost | nself | Notes |
|---------|-------|-------|-------|
| Email/Password | ✅ | ✅ | Direct compatibility |
| Magic Links | ✅ | ✅ | Same implementation |
| OAuth Providers | ✅ 10+ | ✅ 13+ | nself has 3 additional (GitLab, Bitbucket, Slack) |
| MFA (TOTP) | ✅ | ✅ | Same implementation |
| WebAuthn | ✅ | ✅ | Same implementation |
| Custom Claims | ✅ | ✅ | Same JWT approach |
| SMS Auth | ⚠️ Via Twilio | ⚠️ Planned | Use Twilio directly in both |

**Migration Impact**: Minimal - Same nHost Auth service

### Database Features

| Feature | Nhost | nself | Notes |
|---------|-------|-------|-------|
| PostgreSQL 14+ | ✅ | ✅ | Same version |
| Hasura GraphQL | ✅ | ✅ | Same engine |
| Extensions | Opt-in | ✅ 60+ | More readily available in nself |
| Migrations | ✅ Hasura CLI | ✅ `nself db migrate` | Same underlying system |
| DBML Support | ❌ | ✅ | nself advantage |
| Type Generation | ✅ GraphQL Codegen | ✅ TypeScript, Go, Python | nself has more languages |

**Migration Impact**: Low - Same database, enhanced tooling in nself

### Storage Features

| Feature | Nhost | nself | Notes |
|---------|-------|-------|-------|
| S3-compatible | ✅ | ✅ MinIO | Both S3-compatible |
| File Permissions | ✅ | ✅ | Hasura-integrated |
| Image Transformations | ✅ | ⚠️ Planned | **Nhost advantage** - will need workaround |
| CDN | ✅ 80+ locations | ❌ | Use Cloudflare in front of nself |

**Migration Impact**: Medium - Image transformations require alternative solution

---

## Step-by-Step Migration

### Phase 1: Setup nself Project (30 minutes)

```bash
# 1. Initialize nself project
mkdir my-project && cd my-project
nself init --wizard

# Answer wizard questions:
# - Project name: [your-project]
# - Environment: dev
# - Domain: localhost (for testing)

# 2. Configure services to match Nhost
# Edit .env file
nano .env
```

**`.env` Configuration (match Nhost setup):**

```bash
# Basic
PROJECT_NAME=my-nhost-migration
ENV=dev
BASE_DOMAIN=localhost

# Database (use same credentials pattern)
POSTGRES_DB=myapp_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=your-secure-password
POSTGRES_PORT=5432

# Hasura
HASURA_GRAPHQL_ADMIN_SECRET=your-admin-secret
HASURA_GRAPHQL_JWT_SECRET={"type":"HS256","key":"your-jwt-secret-key-min-32-chars"}

# Auth (nHost Auth - same service Nhost uses)
AUTH_SERVER_URL=http://auth.localhost
AUTH_CLIENT_URL=http://localhost:3000

# Storage (enable MinIO)
MINIO_ENABLED=true
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin

# Optional: Enable services you need
REDIS_ENABLED=true
FUNCTIONS_ENABLED=true
MAILPIT_ENABLED=true  # For dev email testing
```

```bash
# 3. Build configuration
nself build

# 4. Start services
nself start

# 5. Verify all services are running
nself doctor
nself urls
```

**Expected Output:**

```
✓ PostgreSQL (postgres) - healthy
✓ Hasura (hasura) - healthy
✓ Auth (auth) - healthy
✓ MinIO (minio) - healthy
✓ Nginx (nginx) - healthy

Available URLs:
- GraphQL API: http://api.localhost
- Auth API: http://auth.localhost
- MinIO Console: http://minio.localhost
- Admin: http://admin.localhost
```

---

### Phase 2: Database Migration (1-2 hours)

#### Step 1: Export from Nhost

```bash
# Option A: Using Nhost CLI
nhost db dump > nhost-database.sql

# Option B: Using pg_dump directly (if you have connection details)
pg_dump -h db.nhost.run -U nhost -d your-nhost-db > nhost-database.sql

# Option C: From Nhost Dashboard
# Go to Database → Backups → Download Latest Backup
```

#### Step 2: Prepare Database Dump

```bash
# Clean the dump file (remove Nhost-specific settings)
cat nhost-database.sql | \
  grep -v "nhost_" | \
  grep -v "pg_cron" | \  # If not using pg_cron
  sed 's/nhost/postgres/g' > cleaned-database.sql
```

#### Step 3: Import to nself

```bash
# Method 1: Using nself CLI (recommended)
nself db import cleaned-database.sql

# Method 2: Using psql directly
docker exec -i $(docker ps -qf "name=postgres") \
  psql -U postgres -d myapp_db < cleaned-database.sql

# Verify import
nself db shell
# In psql:
\dt  # List tables
\du  # List users
SELECT count(*) FROM auth.users;  # Verify data
\q
```

#### Step 4: Verify Schema

```bash
# Generate schema diagram to compare with Nhost
nself db schema:diagram

# Compare tables
nself db inspect --tables

# Check for missing extensions
nself db extensions:list
```

---

### Phase 3: Authentication Migration (30 minutes)

#### Users Data

Authentication users are stored in the same `auth.users` table, so if you imported the database dump, users are already migrated.

**Verify:**

```bash
# Connect to database
nself db shell

# Check users migrated
SELECT id, email, created_at FROM auth.users LIMIT 10;

# Check roles
SELECT * FROM auth.roles;

# Check user roles
SELECT * FROM auth.user_roles;
```

#### OAuth Providers

**Nhost Configuration** → **nself Configuration**

```bash
# In .env, configure OAuth providers you use:

# GitHub
AUTH_PROVIDER_GITHUB_ENABLED=true
AUTH_PROVIDER_GITHUB_CLIENT_ID=your-github-client-id
AUTH_PROVIDER_GITHUB_CLIENT_SECRET=your-github-secret

# Google
AUTH_PROVIDER_GOOGLE_ENABLED=true
AUTH_PROVIDER_GOOGLE_CLIENT_ID=your-google-client-id
AUTH_PROVIDER_GOOGLE_CLIENT_SECRET=your-google-secret

# Facebook
AUTH_PROVIDER_FACEBOOK_ENABLED=true
AUTH_PROVIDER_FACEBOOK_CLIENT_ID=your-facebook-app-id
AUTH_PROVIDER_FACEBOOK_CLIENT_SECRET=your-facebook-secret

# Twitter
AUTH_PROVIDER_TWITTER_ENABLED=true
AUTH_PROVIDER_TWITTER_CONSUMER_KEY=your-twitter-key
AUTH_PROVIDER_TWITTER_CONSUMER_SECRET=your-twitter-secret

# Apple
AUTH_PROVIDER_APPLE_ENABLED=true
AUTH_PROVIDER_APPLE_CLIENT_ID=your-apple-service-id
AUTH_PROVIDER_APPLE_TEAM_ID=your-apple-team-id
AUTH_PROVIDER_APPLE_KEY_ID=your-apple-key-id
AUTH_PROVIDER_APPLE_PRIVATE_KEY=your-apple-private-key
```

**Restart auth service:**

```bash
nself restart --service=auth
```

#### JWT Secrets

**CRITICAL**: Your JWT secret must match Nhost's secret if you want existing tokens to work.

```bash
# Find Nhost JWT secret (from Nhost Dashboard → Settings → Environment Variables)
# HASURA_GRAPHQL_JWT_SECRET

# Copy EXACT value to nself .env
HASURA_GRAPHQL_JWT_SECRET='{"type":"HS256","key":"your-exact-nhost-jwt-secret"}'

# Rebuild and restart
nself build
nself restart
```

**Test Authentication:**

```bash
# Test login
curl -X POST http://auth.localhost/v1/signin/email-password \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "password"
  }'

# Should return access_token and refresh_token
```

---

### Phase 4: Storage Migration (1-2 hours)

#### Step 1: Inventory Nhost Storage

```bash
# Using Nhost Dashboard:
# 1. Go to Storage
# 2. List all buckets
# 3. Note bucket names, policies, file counts

# Using Nhost API (if available):
curl https://api.nhost.io/v1/storage/buckets \
  -H "Authorization: Bearer YOUR_NHOST_ADMIN_TOKEN"
```

#### Step 2: Download Files from Nhost

**Script to download all files:**

```bash
#!/bin/bash
# download-nhost-storage.sh

NHOST_PROJECT_URL="https://yourproject.nhost.run"
NHOST_BUCKET="default"
OUTPUT_DIR="./nhost-storage-backup"

mkdir -p $OUTPUT_DIR/$NHOST_BUCKET

# Get file list
curl "$NHOST_PROJECT_URL/v1/storage/files?bucketId=$NHOST_BUCKET" | \
  jq -r '.files[].id' | \
  while read FILE_ID; do
    echo "Downloading $FILE_ID..."
    curl "$NHOST_PROJECT_URL/v1/storage/files/$FILE_ID" \
      -o "$OUTPUT_DIR/$NHOST_BUCKET/$FILE_ID"
  done
```

```bash
chmod +x download-nhost-storage.sh
./download-nhost-storage.sh
```

#### Step 3: Create Buckets in nself (MinIO)

```bash
# Access MinIO Console
# http://minio.localhost (or http://localhost:9001)
# Login: minioadmin / minioadmin (or your configured credentials)

# OR use MinIO CLI (mc)
docker exec -it $(docker ps -qf "name=minio") mc alias set local http://localhost:9000 minioadmin minioadmin

# Create buckets to match Nhost
docker exec -it $(docker ps -qf "name=minio") mc mb local/default
docker exec -it $(docker ps -qf "name=minio") mc mb local/avatars
docker exec -it $(docker ps -qf "name=minio") mc mb local/uploads

# Set public policy if needed (match Nhost bucket policies)
docker exec -it $(docker ps -qf "name=minio") mc policy set download local/default
```

#### Step 4: Upload Files to MinIO

```bash
#!/bin/bash
# upload-to-minio.sh

MINIO_HOST="localhost:9000"
MINIO_ACCESS_KEY="minioadmin"
MINIO_SECRET_KEY="minioadmin"
BUCKET="default"
SOURCE_DIR="./nhost-storage-backup/default"

# Install MinIO client if not installed
# brew install minio/stable/mc (macOS)
# OR
# wget https://dl.min.io/client/mc/release/linux-amd64/mc
# chmod +x mc

# Configure MinIO client
mc alias set nself http://$MINIO_HOST $MINIO_ACCESS_KEY $MINIO_SECRET_KEY

# Upload all files
mc mirror $SOURCE_DIR nself/$BUCKET
```

```bash
chmod +x upload-to-minio.sh
./upload-to-minio.sh
```

#### Step 5: Verify Storage Migration

```bash
# List files in MinIO
docker exec -it $(docker ps -qf "name=minio") mc ls nself/default

# Check file count matches
docker exec -it $(docker ps -qf "name=minio") mc du nself/default

# Test file access
curl http://minio.localhost/default/test-file.jpg
```

---

### Phase 5: Hasura Metadata Migration (30 minutes)

#### Step 1: Export Hasura Metadata from Nhost

```bash
# Using Hasura Console (Nhost Hasura instance)
# 1. Open Hasura Console: https://yourproject.nhost.run/console
# 2. Go to Settings → Metadata Actions → Export Metadata

# OR using Hasura CLI
cd /tmp/nhost-metadata
hasura metadata export --endpoint https://yourproject.nhost.run/v1/graphql --admin-secret YOUR_ADMIN_SECRET

# This creates metadata/ directory with YAML files
```

#### Step 2: Review and Clean Metadata

```bash
cd /tmp/nhost-metadata/metadata

# Check for Nhost-specific configurations
grep -r "nhost" .
grep -r "cloud" .

# Remove cloud-specific remote schemas if any
# Edit metadata/remote_schemas.yaml if needed
```

#### Step 3: Import Metadata to nself

```bash
# Option 1: Using Hasura CLI
cd /tmp/nhost-metadata
hasura metadata apply --endpoint http://api.localhost/v1/graphql --admin-secret your-admin-secret

# Option 2: Via Hasura Console
# 1. Open http://api.localhost
# 2. Login with admin secret
# 3. Go to Settings → Metadata Actions → Import Metadata
# 4. Upload metadata files
```

#### Step 4: Verify Metadata

```bash
# Check tables tracked
curl http://api.localhost/v1/graphql \
  -H "x-hasura-admin-secret: your-admin-secret" \
  -d '{"query": "{ __schema { types { name } } }"}'

# Check permissions
curl http://api.localhost/v1/metadata \
  -H "x-hasura-admin-secret: your-admin-secret" \
  -d '{"type": "export_metadata", "args": {}}'
```

---

### Phase 6: Functions Migration (1 hour)

Nhost and nself both support serverless functions, but the deployment process differs.

#### Nhost Functions → nself Functions

**Nhost Structure:**
```
nhost/
  functions/
    hello.ts
    users/
      create.ts
      update.ts
```

**nself Structure:**
```
functions/
  src/
    hello.ts
    users/
      create.ts
      update.ts
```

#### Migration Steps

```bash
# 1. Enable functions in nself
# In .env:
FUNCTIONS_ENABLED=true

# 2. Rebuild to create functions service
nself build
nself restart

# 3. Copy Nhost functions to nself
cp -r /path/to/nhost/functions/* ./functions/src/

# 4. Update package.json if needed
cd functions
npm install  # Install dependencies

# 5. Test functions locally
nself logs --service=functions

# 6. Test function endpoint
curl http://functions.localhost/hello
```

#### Code Changes Required

**Nhost Function:**
```typescript
// nhost/functions/hello.ts
import { Request, Response } from 'express'

export default (req: Request, res: Response) => {
  res.json({ message: 'Hello from Nhost!' })
}
```

**nself Function (same, but verify endpoint):**
```typescript
// functions/src/hello.ts
import { Request, Response } from 'express'

export default (req: Request, res: Response) => {
  res.json({ message: 'Hello from nself!' })
}
```

**No changes needed** - Same Express.js handler signature.

---

### Phase 7: Environment Variables (15 minutes)

Map Nhost environment variables to nself equivalents.

**Nhost → nself Mapping:**

| Nhost Variable | nself Variable | Notes |
|----------------|----------------|-------|
| `NHOST_BACKEND_URL` | `API_URL` or `http://api.${BASE_DOMAIN}` | GraphQL endpoint |
| `NHOST_SUBDOMAIN` | `PROJECT_NAME` | Project identifier |
| `NHOST_REGION` | 🔵 N/A | Not applicable (self-hosted) |
| `NEXT_PUBLIC_NHOST_SUBDOMAIN` | `NEXT_PUBLIC_API_URL` | Frontend config |
| `HASURA_GRAPHQL_ADMIN_SECRET` | `HASURA_GRAPHQL_ADMIN_SECRET` | Same |
| `HASURA_GRAPHQL_JWT_SECRET` | `HASURA_GRAPHQL_JWT_SECRET` | Same |
| `POSTGRES_PASSWORD` | `POSTGRES_PASSWORD` | Same |

**Frontend `.env` (Next.js example):**

```bash
# Nhost
NEXT_PUBLIC_NHOST_SUBDOMAIN=myproject
NEXT_PUBLIC_NHOST_REGION=us-east-1

# nself
NEXT_PUBLIC_API_URL=http://api.localhost
NEXT_PUBLIC_AUTH_URL=http://auth.localhost
NEXT_PUBLIC_STORAGE_URL=http://minio.localhost
```

---

## Frontend Code Changes

### Option 1: Minimal Changes (Use Nhost SDK with nself backend)

The Nhost JavaScript SDK can work with nself by configuring custom URLs.

**Before (Nhost Cloud):**
```typescript
import { NhostClient } from '@nhost/nhost-js'

const nhost = new NhostClient({
  subdomain: 'myproject',
  region: 'us-east-1'
})
```

**After (nself):**
```typescript
import { NhostClient } from '@nhost/nhost-js'

const nhost = new NhostClient({
  backendUrl: 'http://localhost:1337', // nself auth service
  graphqlUrl: 'http://api.localhost/v1/graphql',
  storageUrl: 'http://minio.localhost',
  authUrl: 'http://auth.localhost/v1'
})
```

**Configuration in production:**
```typescript
const nhost = new NhostClient({
  backendUrl: process.env.NEXT_PUBLIC_AUTH_URL,
  graphqlUrl: process.env.NEXT_PUBLIC_API_URL + '/v1/graphql',
  storageUrl: process.env.NEXT_PUBLIC_STORAGE_URL,
  authUrl: process.env.NEXT_PUBLIC_AUTH_URL + '/v1'
})
```

### Option 2: Use Standard GraphQL Client

Switch to Apollo Client, URQL, or graphql-request for more control.

**Apollo Client Setup:**

```typescript
// lib/apollo-client.ts
import { ApolloClient, InMemoryCache, createHttpLink } from '@apollo/client'
import { setContext } from '@apollo/client/link/context'

const httpLink = createHttpLink({
  uri: process.env.NEXT_PUBLIC_API_URL + '/v1/graphql'
})

const authLink = setContext((_, { headers }) => {
  const token = localStorage.getItem('accessToken')
  return {
    headers: {
      ...headers,
      authorization: token ? `Bearer ${token}` : "",
    }
  }
})

export const apolloClient = new ApolloClient({
  link: authLink.concat(httpLink),
  cache: new InMemoryCache()
})
```

**Usage:**
```typescript
import { useQuery, gql } from '@apollo/client'

const GET_USERS = gql`
  query GetUsers {
    users {
      id
      email
    }
  }
`

function UsersList() {
  const { loading, error, data } = useQuery(GET_USERS)
  // ...
}
```

---

## Common Pitfalls

### Pitfall 1: JWT Secret Mismatch

**Symptom**: Existing tokens don't work after migration

**Cause**: Different JWT secret between Nhost and nself

**Solution**:
```bash
# Use EXACT same JWT secret from Nhost
# Find in Nhost Dashboard → Settings → Env Vars
# Copy to nself .env
HASURA_GRAPHQL_JWT_SECRET='{"type":"HS256","key":"exact-nhost-secret"}'

# Restart
nself restart
```

### Pitfall 2: Database Connection Refused

**Symptom**: `connection refused` when importing database

**Cause**: PostgreSQL not fully started

**Solution**:
```bash
# Wait for PostgreSQL to be ready
nself doctor

# Or check manually
docker exec $(docker ps -qf "name=postgres") pg_isready
```

### Pitfall 3: Storage URLs Don't Work

**Symptom**: 404 errors when accessing files

**Cause**: MinIO bucket policies not configured

**Solution**:
```bash
# Set bucket to public (if needed)
docker exec -it $(docker ps -qf "name=minio") mc policy set download local/default

# Or configure specific bucket policy
docker exec -it $(docker ps -qf "name=minio") mc policy set-json local/default policy.json
```

### Pitfall 4: Image Transformations Missing

**Symptom**: Image resize/crop URLs don't work

**Cause**: nself doesn't have built-in image transformations yet (Nhost does)

**Workarounds**:
1. Use Cloudinary or imgix (external service)
2. Add image transformation proxy (CS_N custom service)
3. Pre-generate image sizes during upload (Functions)

**Example (Cloudinary):**
```typescript
// Upload to MinIO, then transform via Cloudinary
const imageUrl = `https://res.cloudinary.com/your-cloud/image/upload/w_300,h_300,c_fill/minio/${filename}`
```

### Pitfall 5: Functions Environment Variables

**Symptom**: Functions can't access database or APIs

**Cause**: Environment variables not passed to functions runtime

**Solution**:
```bash
# Add to .env
FUNCTIONS_DATABASE_URL=postgresql://postgres:password@postgres:5432/myapp_db
FUNCTIONS_GRAPHQL_URL=http://hasura:8080/v1/graphql
FUNCTIONS_GRAPHQL_ADMIN_SECRET=your-admin-secret

# Restart functions
nself restart --service=functions
```

---

## Rollback Procedure

If migration fails and you need to rollback to Nhost:

### Step 1: Keep Nhost Running

**DON'T** delete your Nhost project until migration is verified in production.

### Step 2: Switch DNS Back

```bash
# If you updated DNS to point to nself:
# 1. Change DNS A record back to Nhost IP
# 2. Wait for DNS propagation (5-60 minutes)
# 3. Verify site is back on Nhost
```

### Step 3: Restore Data (if needed)

```bash
# If you made database changes in nself that need to sync back:
# 1. Export from nself
nself db export > nself-changes.sql

# 2. Import to Nhost
nhost db restore nself-changes.sql
```

### Step 4: Frontend Rollback

```bash
# Revert environment variables to Nhost URLs
NEXT_PUBLIC_NHOST_SUBDOMAIN=myproject
NEXT_PUBLIC_NHOST_REGION=us-east-1

# Rebuild and deploy
npm run build
vercel deploy
```

---

## Automated Migration Tools

### Migration Helper Script

```bash
#!/bin/bash
# migrate-from-nhost.sh

set -e

echo "🚀 nself Migration Helper - Nhost to nself"
echo "=========================================="

# Configuration
NHOST_PROJECT=$1
NSELF_PROJECT=$2

if [ -z "$NHOST_PROJECT" ] || [ -z "$NSELF_PROJECT" ]; then
  echo "Usage: ./migrate-from-nhost.sh <nhost-project-id> <nself-project-name>"
  exit 1
fi

echo "📥 Step 1: Exporting from Nhost..."
mkdir -p migration-backup
nhost db dump --project $NHOST_PROJECT > migration-backup/database.sql
nhost metadata export --project $NHOST_PROJECT --output migration-backup/metadata

echo "🔧 Step 2: Initializing nself project..."
mkdir -p $NSELF_PROJECT
cd $NSELF_PROJECT
nself init --name $NSELF_PROJECT --env dev

echo "📦 Step 3: Building nself stack..."
nself build
nself start

echo "⏳ Waiting for services to be ready..."
sleep 30

echo "📥 Step 4: Importing database..."
nself db import ../migration-backup/database.sql

echo "📥 Step 5: Importing Hasura metadata..."
cd ../migration-backup/metadata
hasura metadata apply --endpoint http://api.localhost/v1/graphql --admin-secret $HASURA_GRAPHQL_ADMIN_SECRET

echo "✅ Migration complete!"
echo ""
echo "Next steps:"
echo "1. Verify data: nself db shell"
echo "2. Test authentication: curl http://auth.localhost/healthz"
echo "3. Test GraphQL: curl http://api.localhost/v1/graphql"
echo "4. Migrate storage files manually"
echo "5. Update frontend environment variables"
echo ""
echo "🎉 Your nself instance is ready at: http://api.localhost"
```

**Usage:**
```bash
chmod +x migrate-from-nhost.sh
./migrate-from-nhost.sh nhost-project-id my-nself-project
```

---

## Verification Checklist

After migration, verify everything works:

### Database
- [ ] All tables present (`nself db shell → \dt`)
- [ ] Row counts match Nhost
- [ ] Foreign keys intact
- [ ] Triggers functioning
- [ ] Extensions loaded

### Authentication
- [ ] Users can login with email/password
- [ ] OAuth providers work
- [ ] JWT tokens valid
- [ ] User roles correct
- [ ] Refresh tokens work

### Storage
- [ ] All files accessible
- [ ] File counts match
- [ ] Public files accessible without auth
- [ ] Private files require auth
- [ ] Upload works

### GraphQL API
- [ ] All queries work
- [ ] Mutations work
- [ ] Subscriptions work (real-time)
- [ ] Permissions enforced
- [ ] Aggregations work

### Functions
- [ ] All functions deployed
- [ ] Functions accessible via HTTP
- [ ] Environment variables available
- [ ] Database connections work

### Frontend
- [ ] Authentication flow works
- [ ] Data fetching works
- [ ] File uploads work
- [ ] Real-time updates work
- [ ] No console errors

---

## Performance Optimization

After migration, optimize nself for production:

### Database Optimization

```bash
# Analyze database for query optimization
nself db analyze

# Create recommended indexes
nself db schema:indexes

# Enable connection pooling
# In .env:
PGBOUNCER_ENABLED=true
PGBOUNCER_POOL_MODE=transaction
PGBOUNCER_MAX_CLIENT_CONN=100
```

### Caching

```bash
# Enable Redis for session caching
REDIS_ENABLED=true
AUTH_REDIS_ENABLED=true
```

### Monitoring

```bash
# Enable monitoring bundle
MONITORING_ENABLED=true

# Rebuild and restart
nself build
nself restart

# Access Grafana
# http://grafana.localhost
```

---

## Cost Comparison

### Nhost Pricing (2026)
- **Free**: Limited resources
- **Pro ($25/mo)**: 8GB DB, 100GB storage
- **Team ($599/mo)**: 100GB DB, 1TB storage
- **Enterprise**: Custom pricing

### nself Costs (Self-Hosted)
- **nself**: Free (open-source)
- **Infrastructure**:
  - Small VPS: $5-10/mo (DigitalOcean, Hetzner)
  - Medium VPS: $20-40/mo
  - Large: $80-160/mo

**Break-even**: Around $25-50/mo in infrastructure costs

**Estimated Savings**: 40-70% at scale

---

## Support & Resources

### nself Community
- **GitHub**: https://github.com/nself-org/cli
- **Discord**: https://discord.gg/nself
- **Docs**: https://nself.org/docs

### Migration Support
- **GitHub Issues**: Tag with `migration` and `nhost`
- **Discord #migration channel**
- **Email**: support@nself.org

### Professional Services
For assistance with migration:
- Migration consulting
- Custom automation scripts
- Post-migration optimization
- Training sessions

Contact: migrations@nself.org

---

## Conclusion

Migrating from Nhost to nself is straightforward due to shared technology (PostgreSQL, Hasura, nHost Auth). The main effort is in data export/import and frontend URL configuration changes.

**Timeline Summary**:
- ✅ **Setup nself**: 30 minutes
- ✅ **Database migration**: 1-2 hours
- ✅ **Authentication setup**: 30 minutes
- ✅ **Storage migration**: 1-2 hours
- ✅ **Metadata import**: 30 minutes
- ✅ **Functions migration**: 1 hour
- ✅ **Frontend changes**: 1-2 hours
- ✅ **Testing & verification**: 1-2 hours

**Total**: 6-10 hours for complete migration

**Recommended Approach**:
1. Migrate to staging environment first
2. Test thoroughly for 1-2 weeks
3. Migrate production during low-traffic period
4. Keep Nhost running for 1 week as fallback

Good luck with your migration! 🚀
