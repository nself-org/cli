# Migrating from Supabase to nself

**Last Updated**: January 30, 2026
**Migration Difficulty**: Medium-High
**Estimated Time**: 8-16 hours
**Compatibility**: 85% (different API layers, same PostgreSQL core)

---

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Feature Comparison](#feature-comparison)
4. [Architecture Differences](#architecture-differences)
5. [Step-by-Step Migration](#step-by-step-migration)
6. [Database Migration](#database-migration)
7. [Authentication Migration](#authentication-migration)
8. [Row Level Security (RLS) Migration](#row-level-security-rls-migration)
9. [Storage Migration](#storage-migration)
10. [Edge Functions to Functions Migration](#edge-functions-to-functions-migration)
11. [Realtime Subscriptions Migration](#realtime-subscriptions-migration)
12. [Frontend Code Changes](#frontend-code-changes)
13. [Common Pitfalls](#common-pitfalls)
14. [Rollback Procedure](#rollback-procedure)
15. [Automated Migration Tools](#automated-migration-tools)

---

## Overview

Supabase and nself both use PostgreSQL as their database foundation but differ significantly in their API layer and approach:

- **Supabase**: Uses PostgREST (REST API) + pg_graphql extension (GraphQL) + custom Realtime server
- **nself**: Uses Hasura GraphQL Engine (GraphQL-first) + WebSocket subscriptions

### Why Migrate to nself?

- **Full infrastructure control** - Self-hosted, no vendor lock-in
- **More powerful GraphQL** - Hasura vs. pg_graphql extension (schema stitching, remote schemas, actions)
- **Advanced features** - Multi-tenancy, billing integration, plugin system
- **Better real-time** - Full GraphQL subscriptions vs. Supabase Realtime channels
- **Comprehensive CLI** - 31 top-level commands with 285+ subcommands for every operation
- **Cost control** - Predictable self-hosting costs

### Key Differences

| Aspect | Supabase | nself |
|--------|----------|-------|
| **API Layer** | PostgREST (REST-first) + pg_graphql | Hasura GraphQL Engine (GraphQL-first) |
| **Authentication** | GoTrue (Supabase Auth) | nHost Auth |
| **Storage** | Supabase Storage | MinIO (S3-compatible) |
| **Functions** | Deno Edge Functions | Node.js/Deno Functions |
| **Realtime** | Custom Realtime server | GraphQL subscriptions (Hasura) |
| **Dashboard** | Supabase Studio | Hasura Console + nself-admin |

---

## Prerequisites

### Before You Start

- [ ] Access to Supabase project (owner/admin)
- [ ] PostgreSQL dump capability (Supabase Dashboard or CLI)
- [ ] List of all RLS policies
- [ ] Storage bucket inventory and policies
- [ ] Edge Functions inventory
- [ ] List of Auth providers configured
- [ ] nself installed on target server
- [ ] Full backup of Supabase project

### Required Tools

```bash
# Install nself
brew install nself
# OR
curl -sSL https://install.nself.org | bash

# Install Supabase CLI
npm install -g supabase

# Install PostgreSQL client tools
# macOS:
brew install postgresql
# Ubuntu/Debian:
sudo apt-get install postgresql-client

# Install jq for JSON processing
brew install jq  # macOS
sudo apt-get install jq  # Ubuntu
```

### Migration Checklist

```bash
# 1. Export Supabase configuration
supabase projects list
supabase db dump --project-id your-project-id > supabase-dump.sql

# 2. Export Auth users (if using Supabase Auth API)
# Use Supabase Dashboard → Authentication → Export Users (CSV)

# 3. List storage buckets and policies
supabase storage ls
# Document bucket policies from Dashboard

# 4. Export Edge Functions
# Copy from supabase/functions/ directory

# 5. Document RLS policies
# From Supabase Dashboard → Database → Policies
```

---

## Feature Comparison

### API Differences

| Feature | Supabase | nself | Migration Effort |
|---------|----------|-------|------------------|
| **REST API** | ✅ PostgREST | ✅ Hasura REST endpoints | Medium - Different syntax |
| **GraphQL API** | ⚠️ pg_graphql (limited) | ✅ Full Hasura GraphQL | Low - Upgrade |
| **Subscriptions** | ⚠️ Realtime (custom) | ✅ GraphQL WebSocket | Medium - Different protocol |
| **Filters** | PostgREST syntax | GraphQL where clauses | High - Rewrite queries |
| **Pagination** | Range headers | GraphQL limit/offset | Medium - Different approach |

### Authentication Differences

| Feature | Supabase | nself | Migration Effort |
|---------|----------|-------|------------------|
| **Auth Service** | GoTrue | nHost Auth | Medium - Different API |
| **User Table** | `auth.users` | `auth.users` | Low - Same schema |
| **OAuth** | 25+ providers | 13+ providers | Low - Reconfigure |
| **Magic Links** | ✅ | ✅ | Low - Same concept |
| **Phone Auth** | ✅ | ⚠️ Planned | High - Workaround needed |

### Storage Differences

| Feature | Supabase | nself | Migration Effort |
|---------|----------|-------|------------------|
| **Backend** | Supabase Storage | MinIO (S3) | Medium - Different API |
| **Bucket Policies** | Declarative SQL | MinIO policies + RLS | High - Rewrite policies |
| **RLS Integration** | Built-in | Via Hasura + MinIO | Medium - Configure integration |
| **CDN** | Global CDN | Self-hosted (add Cloudflare) | Medium - External CDN |

---

## Architecture Differences

### REST vs. GraphQL

**Supabase (PostgREST):**
```javascript
// Supabase REST API
const { data, error } = await supabase
  .from('posts')
  .select('id, title, author(name)')
  .eq('published', true)
  .order('created_at', { ascending: false })
  .range(0, 9)
```

**nself (Hasura GraphQL):**
```graphql
# Hasura GraphQL
query GetPosts {
  posts(
    where: { published: { _eq: true } }
    order_by: { created_at: desc }
    limit: 10
  ) {
    id
    title
    author {
      name
    }
  }
}
```

### Realtime Differences

**Supabase Realtime:**
```javascript
// Supabase Realtime (PostgreSQL CDC + WebSocket)
const channel = supabase
  .channel('posts-channel')
  .on('postgres_changes',
    { event: 'INSERT', schema: 'public', table: 'posts' },
    (payload) => console.log(payload)
  )
  .subscribe()
```

**nself (GraphQL Subscriptions):**
```graphql
# Hasura GraphQL Subscription
subscription OnNewPost {
  posts(order_by: { created_at: desc }, limit: 1) {
    id
    title
    content
    created_at
  }
}
```

---

## Step-by-Step Migration

### Phase 1: Setup nself Project (30 minutes)

```bash
# 1. Initialize nself project
mkdir my-supabase-migration && cd my-supabase-migration
nself init --wizard

# Answer wizard:
# - Project name: [your-project]
# - Environment: dev
# - Domain: localhost

# 2. Configure .env to match Supabase features
nano .env
```

**`.env` Configuration:**

```bash
# Basic
PROJECT_NAME=my-supabase-migration
ENV=dev
BASE_DOMAIN=localhost

# Database
POSTGRES_DB=myapp_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=your-secure-password
POSTGRES_PORT=5432

# Hasura GraphQL
HASURA_GRAPHQL_ADMIN_SECRET=your-admin-secret
HASURA_GRAPHQL_ENABLE_CONSOLE=true
HASURA_GRAPHQL_DEV_MODE=true

# JWT Configuration (important for auth)
HASURA_GRAPHQL_JWT_SECRET={"type":"HS256","key":"your-jwt-secret-min-32-chars-long"}

# Auth (nHost Auth)
AUTH_SERVER_URL=http://auth.localhost
AUTH_CLIENT_URL=http://localhost:3000
AUTH_JWT_EXPIRES_IN=900
AUTH_REFRESH_TOKEN_EXPIRES_IN=2592000

# Storage (MinIO - S3 compatible)
MINIO_ENABLED=true
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin
MINIO_BROWSER=on

# Optional services
REDIS_ENABLED=true
FUNCTIONS_ENABLED=true
MAILPIT_ENABLED=true

# Monitoring (recommended)
MONITORING_ENABLED=true
```

```bash
# 3. Build and start
nself build
nself start

# 4. Verify
nself doctor
nself urls
```

---

### Phase 2: Database Schema Migration (2-3 hours)

#### Step 1: Export from Supabase

```bash
# Method 1: Supabase CLI (recommended)
supabase db dump --project-id your-project-id > supabase-dump.sql

# Method 2: pg_dump directly (if you have connection string)
pg_dump "postgresql://postgres:[password]@db.[project-ref].supabase.co:5432/postgres" > supabase-dump.sql

# Method 3: Supabase Dashboard
# Go to Database → Backups → Download
```

#### Step 2: Clean Supabase-Specific Objects

```bash
# Remove Supabase-specific schemas and objects
cat supabase-dump.sql | \
  grep -v "supabase_functions" | \
  grep -v "supabase_migrations" | \
  grep -v "pg_graphql" | \
  grep -v "pg_stat_statements" | \
  grep -v "pgsodium" | \
  sed 's/supabase_admin/postgres/g' > cleaned-dump.sql
```

**Important schemas to preserve:**
- ✅ `public` - Your application tables
- ✅ `auth` - User authentication tables (compatible with nHost Auth)
- ✅ `storage` - Storage metadata (will need conversion)
- ❌ `supabase_functions` - Not needed (different functions system)
- ❌ `realtime` - Not needed (different realtime system)

#### Step 3: Import to nself

```bash
# Import database
nself db import cleaned-dump.sql

# Verify tables
nself db shell
```

```sql
-- In psql shell
\dt public.*  -- List public tables
\dt auth.*    -- List auth tables
\dt storage.* -- List storage tables

-- Check row counts
SELECT schemaname, tablename, n_tup_ins
FROM pg_stat_user_tables
WHERE schemaname = 'public';

\q
```

#### Step 4: Recreate Foreign Keys and Triggers

Some foreign keys might not import correctly. Verify and recreate:

```bash
# Check for broken foreign keys
nself db shell
```

```sql
-- List all foreign keys
SELECT
    tc.table_name,
    kcu.column_name,
    ccu.table_name AS foreign_table_name,
    ccu.column_name AS foreign_column_name
FROM information_schema.table_constraints AS tc
JOIN information_schema.key_column_usage AS kcu
  ON tc.constraint_name = kcu.constraint_name
JOIN information_schema.constraint_column_usage AS ccu
  ON ccu.constraint_name = tc.constraint_name
WHERE tc.constraint_type = 'FOREIGN KEY';
```

---

### Phase 3: Authentication Migration (1-2 hours)

Supabase uses GoTrue (auth.users) and nself uses nHost Auth (also auth.users). The schema is similar but APIs differ.

#### Step 1: Migrate User Data

**Good news**: If you imported the database dump, `auth.users` table is already migrated.

```bash
nself db shell
```

```sql
-- Verify users imported
SELECT id, email, created_at, last_sign_in_at FROM auth.users LIMIT 10;

-- Check user count
SELECT COUNT(*) FROM auth.users;
```

#### Step 2: Recreate User Passwords

**CRITICAL**: Password hashes may not be compatible between GoTrue and nHost Auth.

**Option A: Force password reset** (recommended for security)

```bash
# Create a script to send password reset emails to all users
cat > reset-all-passwords.sh << 'EOF'
#!/bin/bash
# This requires access to auth API

AUTH_URL="http://auth.localhost/v1"

# Get all user emails
EMAILS=$(nself db shell -c "SELECT email FROM auth.users;" | tail -n +3 | head -n -2)

for EMAIL in $EMAILS; do
  echo "Sending reset email to: $EMAIL"
  curl -X POST "$AUTH_URL/user/password-reset" \
    -H "Content-Type: application/json" \
    -d "{\"email\": \"$EMAIL\"}"
done
EOF

chmod +x reset-all-passwords.sh
```

**Option B: Migrate password hashes** (if GoTrue and nHost Auth use same algorithm)

```sql
-- Check password hash format in both systems
SELECT id, email, encrypted_password FROM auth.users LIMIT 1;

-- If compatible, hashes are already migrated
-- If not compatible, users must reset passwords
```

#### Step 3: Configure OAuth Providers

Map Supabase OAuth config to nself:

```bash
# In .env, add OAuth providers:

# GitHub
AUTH_PROVIDER_GITHUB_ENABLED=true
AUTH_PROVIDER_GITHUB_CLIENT_ID=your-github-client-id
AUTH_PROVIDER_GITHUB_CLIENT_SECRET=your-github-secret
AUTH_PROVIDER_GITHUB_REDIRECT_URI=http://auth.localhost/v1/auth/callback/github

# Google
AUTH_PROVIDER_GOOGLE_ENABLED=true
AUTH_PROVIDER_GOOGLE_CLIENT_ID=your-google-client-id
AUTH_PROVIDER_GOOGLE_CLIENT_SECRET=your-google-secret

# Add others as needed (Facebook, Apple, etc.)
```

**Update OAuth redirect URIs** in provider dashboards:
- **Old**: `https://[project-ref].supabase.co/auth/v1/callback`
- **New**: `http://auth.localhost/v1/auth/callback` (dev) or `https://auth.yourdomain.com/v1/auth/callback` (prod)

```bash
# Restart auth service
nself restart --service=auth
```

#### Step 4: Test Authentication

```bash
# Test email/password signup
curl -X POST http://auth.localhost/v1/signup/email-password \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "SecurePass123!"
  }'

# Test email/password login
curl -X POST http://auth.localhost/v1/signin/email-password \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "SecurePass123!"
  }'

# Should return access_token and refresh_token
```

---

### Phase 4: Row Level Security (RLS) Migration (2-4 hours)

Supabase and nself both use PostgreSQL RLS, BUT:
- **Supabase**: Primarily uses RLS policies (SQL-based)
- **nself**: Uses Hasura permissions (GraphQL) + optional PostgreSQL RLS

#### Step 1: Document Existing RLS Policies

From Supabase Dashboard → Database → Policies, document all RLS policies:

```sql
-- Example Supabase RLS policy
CREATE POLICY "Users can view their own posts"
  ON posts
  FOR SELECT
  USING (auth.uid() = user_id);

CREATE POLICY "Users can update their own posts"
  ON posts
  FOR UPDATE
  USING (auth.uid() = user_id);
```

#### Step 2: Choose Migration Strategy

**Option A: Convert to Hasura Permissions** (recommended)

Use Hasura Console to define permissions instead of SQL policies.

**Access Hasura Console:**
```bash
# Open http://api.localhost
# Login with admin secret
# Go to Data → [table] → Permissions
```

**Example: Convert RLS to Hasura:**

**Supabase RLS:**
```sql
CREATE POLICY "Users can view their own posts"
  ON posts FOR SELECT
  USING (auth.uid() = user_id);
```

**Hasura Permission:**
```yaml
# In Hasura Console
table: posts
role: user
permissions:
  select:
    filter:
      user_id: { _eq: X-Hasura-User-Id }
    columns: [id, title, content, user_id, created_at]
```

**Advantages**:
- Faster (no RLS overhead)
- Easier to manage (GUI + GraphQL)
- Better for complex permissions

**Option B: Keep PostgreSQL RLS** (if you prefer SQL)

RLS policies can coexist with Hasura permissions.

```sql
-- Enable RLS on table
ALTER TABLE posts ENABLE ROW LEVEL SECURITY;

-- Recreate policies (modify auth.uid() to work with nHost Auth)
CREATE POLICY "Users can view their own posts"
  ON posts
  FOR SELECT
  USING (current_setting('hasura.user.id')::uuid = user_id);
```

**Note**: `auth.uid()` (Supabase) → `current_setting('hasura.user.id')::uuid` (nself/Hasura)

#### Step 3: Create Hasura Permissions

For each table with RLS in Supabase, create Hasura permissions:

**Example: Posts table**

```yaml
# Hasura Console → Data → posts → Permissions

# User role (authenticated users)
role: user
permissions:
  insert:
    check:
      user_id: { _eq: X-Hasura-User-Id }
    columns: [title, content]

  select:
    filter:
      _or:
        - published: { _eq: true }
        - user_id: { _eq: X-Hasura-User-Id }
    columns: [id, title, content, user_id, published, created_at]

  update:
    filter:
      user_id: { _eq: X-Hasura-User-Id }
    check:
      user_id: { _eq: X-Hasura-User-Id }
    columns: [title, content, published]

  delete:
    filter:
      user_id: { _eq: X-Hasura-User-Id }

# Anonymous role (unauthenticated users)
role: anonymous
permissions:
  select:
    filter:
      published: { _eq: true }
    columns: [id, title, content, created_at]
```

#### Step 4: Test Permissions

```bash
# Test as authenticated user
curl -X POST http://api.localhost/v1/graphql \
  -H "Content-Type: application/json" \
  -H "x-hasura-role: user" \
  -H "x-hasura-user-id: user-uuid-here" \
  -d '{
    "query": "query { posts { id title } }"
  }'

# Test as anonymous
curl -X POST http://api.localhost/v1/graphql \
  -H "Content-Type: application/json" \
  -H "x-hasura-role: anonymous" \
  -d '{
    "query": "query { posts(where: {published: {_eq: true}}) { id title } }"
  }'
```

---

### Phase 5: Storage Migration (2-3 hours)

Supabase Storage → MinIO (S3-compatible)

#### Step 1: Export Storage Inventory

```bash
# List all buckets in Supabase
supabase storage ls

# For each bucket, list files
supabase storage ls bucket-name

# Export to JSON for reference
supabase storage ls bucket-name --json > bucket-inventory.json
```

#### Step 2: Download Files

**Download script:**

```bash
#!/bin/bash
# download-supabase-storage.sh

SUPABASE_URL="https://[project-ref].supabase.co"
SUPABASE_KEY="your-anon-key"
BUCKET="default"
OUTPUT_DIR="./storage-backup/$BUCKET"

mkdir -p $OUTPUT_DIR

# Get file list
curl "$SUPABASE_URL/storage/v1/object/list/$BUCKET" \
  -H "apikey: $SUPABASE_KEY" \
  -H "Authorization: Bearer $SUPABASE_KEY" | \
  jq -r '.[].name' | \
  while read FILE; do
    echo "Downloading $FILE..."
    curl "$SUPABASE_URL/storage/v1/object/public/$BUCKET/$FILE" \
      -o "$OUTPUT_DIR/$FILE"
  done
```

```bash
chmod +x download-supabase-storage.sh
./download-supabase-storage.sh
```

#### Step 3: Create MinIO Buckets

```bash
# Access MinIO Console
# http://minio.localhost (or http://localhost:9001)

# OR use mc CLI
docker exec -it $(docker ps -qf "name=minio") mc alias set local http://localhost:9000 minioadmin minioadmin

# Create buckets
docker exec -it $(docker ps -qf "name=minio") mc mb local/default
docker exec -it $(docker ps -qf "name=minio") mc mb local/avatars
docker exec -it $(docker ps -qf "name=minio") mc mb local/uploads

# Set public policy (if bucket was public in Supabase)
docker exec -it $(docker ps -qf "name=minio") mc policy set download local/default
```

#### Step 4: Upload Files to MinIO

```bash
#!/bin/bash
# upload-to-minio.sh

BUCKET="default"
SOURCE_DIR="./storage-backup/$BUCKET"

# Upload all files
docker exec -i $(docker ps -qf "name=minio") mc mirror $SOURCE_DIR local/$BUCKET
```

#### Step 5: Migrate Storage Policies

Supabase storage policies → Hasura actions + MinIO policies

**Supabase Policy Example:**
```sql
-- Supabase storage policy (SQL)
CREATE POLICY "Users can upload to their folder"
  ON storage.objects
  FOR INSERT
  WITH CHECK (
    bucket_id = 'avatars' AND
    auth.uid()::text = (storage.foldername(name))[1]
  );
```

**nself Equivalent:**

**Option A: Hasura Action (recommended)**

Create a Hasura action that validates upload permissions before generating presigned URL.

**Option B: MinIO Bucket Policy**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {"AWS": ["*"]},
      "Action": ["s3:GetObject"],
      "Resource": ["arn:aws:s3:::avatars/*"]
    }
  ]
}
```

---

### Phase 6: Edge Functions Migration (1-2 hours)

Supabase Edge Functions (Deno) → nself Functions (Node.js or Deno)

#### Step 1: Copy Functions

```bash
# Supabase structure:
# supabase/functions/
#   hello/index.ts
#   user-create/index.ts

# nself structure:
# functions/src/
#   hello.ts
#   user-create.ts

# Copy files
cp -r supabase/functions/* functions/src/
```

#### Step 2: Update Function Code

**Supabase Edge Function:**
```typescript
// supabase/functions/hello/index.ts
import { serve } from "https://deno.land/std@0.168.0/http/server.ts"

serve(async (req) => {
  const { name } = await req.json()

  return new Response(
    JSON.stringify({ message: `Hello ${name}!` }),
    { headers: { "Content-Type": "application/json" } }
  )
})
```

**nself Function (Node.js):**
```typescript
// functions/src/hello.ts
import { Request, Response } from 'express'

export default async (req: Request, res: Response) => {
  const { name } = req.body

  res.json({ message: `Hello ${name}!` })
}
```

**Or nself Function (Deno - if EDGE_RUNTIME=deno):**
```typescript
// functions/src/hello.ts
export default async (req: Request): Promise<Response> => {
  const { name } = await req.json()

  return new Response(
    JSON.stringify({ message: `Hello ${name}!` }),
    { headers: { "Content-Type": "application/json" } }
  )
}
```

#### Step 3: Update Dependencies

**Supabase (Deno):**
```typescript
import { createClient } from 'https://esm.sh/@supabase/supabase-js@2'
```

**nself (Node.js):**
```typescript
import { GraphQLClient } from 'graphql-request'

const client = new GraphQLClient(process.env.GRAPHQL_URL!, {
  headers: {
    'x-hasura-admin-secret': process.env.HASURA_ADMIN_SECRET!
  }
})
```

#### Step 4: Deploy Functions

```bash
# Install dependencies
cd functions
npm install

# Restart functions service
nself restart --service=functions

# Test function
curl http://functions.localhost/hello \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{"name": "World"}'
```

---

### Phase 7: Realtime Subscriptions Migration (1-2 hours)

Supabase Realtime → Hasura GraphQL Subscriptions

#### Supabase Realtime Code

```javascript
// Supabase Realtime
import { createClient } from '@supabase/supabase-js'

const supabase = createClient(URL, KEY)

const channel = supabase
  .channel('posts-channel')
  .on('postgres_changes',
    { event: 'INSERT', schema: 'public', table: 'posts' },
    (payload) => {
      console.log('New post:', payload.new)
    }
  )
  .on('postgres_changes',
    { event: 'UPDATE', schema: 'public', table: 'posts' },
    (payload) => {
      console.log('Updated post:', payload.new)
    }
  )
  .subscribe()
```

#### nself GraphQL Subscription

**Using Apollo Client:**

```typescript
import { ApolloClient, InMemoryCache, split, HttpLink } from '@apollo/client'
import { GraphQLWsLink } from '@apollo/client/link/subscriptions'
import { createClient } from 'graphql-ws'
import { getMainDefinition } from '@apollo/client/utilities'

// HTTP Link for queries and mutations
const httpLink = new HttpLink({
  uri: 'http://api.localhost/v1/graphql'
})

// WebSocket Link for subscriptions
const wsLink = new GraphQLWsLink(createClient({
  url: 'ws://api.localhost/v1/graphql',
  connectionParams: {
    headers: {
      Authorization: `Bearer ${accessToken}`
    }
  }
}))

// Split based on operation type
const splitLink = split(
  ({ query }) => {
    const definition = getMainDefinition(query)
    return (
      definition.kind === 'OperationDefinition' &&
      definition.operation === 'subscription'
    )
  },
  wsLink,
  httpLink
)

const client = new ApolloClient({
  link: splitLink,
  cache: new InMemoryCache()
})

// Subscribe to new posts
const POSTS_SUBSCRIPTION = gql`
  subscription OnNewPost {
    posts(order_by: { created_at: desc }, limit: 1) {
      id
      title
      content
      created_at
    }
  }
`

const { data } = useSubscription(POSTS_SUBSCRIPTION)
```

---

## Frontend Code Changes

### REST API → GraphQL Migration

**Before (Supabase - REST API):**

```typescript
import { createClient } from '@supabase/supabase-js'

const supabase = createClient(
  process.env.NEXT_PUBLIC_SUPABASE_URL!,
  process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY!
)

// Fetch posts
const { data: posts, error } = await supabase
  .from('posts')
  .select('id, title, author(name)')
  .eq('published', true)
  .order('created_at', { ascending: false })

// Create post
const { data, error } = await supabase
  .from('posts')
  .insert({ title: 'New Post', content: 'Content' })

// Update post
const { data, error } = await supabase
  .from('posts')
  .update({ title: 'Updated' })
  .eq('id', postId)
```

**After (nself - GraphQL):**

```typescript
import { GraphQLClient, gql } from 'graphql-request'

const client = new GraphQLClient(
  process.env.NEXT_PUBLIC_API_URL!,
  {
    headers: {
      Authorization: `Bearer ${accessToken}`
    }
  }
)

// Fetch posts
const GET_POSTS = gql`
  query GetPosts {
    posts(
      where: { published: { _eq: true } }
      order_by: { created_at: desc }
    ) {
      id
      title
      author {
        name
      }
    }
  }
`

const { posts } = await client.request(GET_POSTS)

// Create post
const CREATE_POST = gql`
  mutation CreatePost($title: String!, $content: String!) {
    insert_posts_one(object: { title: $title, content: $content }) {
      id
      title
    }
  }
`

await client.request(CREATE_POST, { title: 'New Post', content: 'Content' })

// Update post
const UPDATE_POST = gql`
  mutation UpdatePost($id: uuid!, $title: String!) {
    update_posts_by_pk(pk_columns: { id: $id }, _set: { title: $title }) {
      id
      title
    }
  }
`

await client.request(UPDATE_POST, { id: postId, title: 'Updated' })
```

### Storage API Changes

**Before (Supabase Storage):**

```typescript
// Upload file
const { data, error } = await supabase.storage
  .from('avatars')
  .upload('user-avatar.png', file)

// Get public URL
const { data } = supabase.storage
  .from('avatars')
  .getPublicUrl('user-avatar.png')

// Download file
const { data, error } = await supabase.storage
  .from('avatars')
  .download('user-avatar.png')
```

**After (nself - MinIO S3):**

```typescript
import { S3Client, PutObjectCommand, GetObjectCommand } from '@aws-sdk/client-s3'
import { getSignedUrl } from '@aws-sdk/s3-request-presigner'

const s3Client = new S3Client({
  region: 'us-east-1',
  endpoint: process.env.NEXT_PUBLIC_STORAGE_URL,
  credentials: {
    accessKeyId: 'minioadmin',
    secretAccessKey: 'minioadmin'
  },
  forcePathStyle: true
})

// Upload file
const uploadCommand = new PutObjectCommand({
  Bucket: 'avatars',
  Key: 'user-avatar.png',
  Body: file
})
await s3Client.send(uploadCommand)

// Get public URL
const url = `${process.env.NEXT_PUBLIC_STORAGE_URL}/avatars/user-avatar.png`

// Get presigned URL (for private files)
const command = new GetObjectCommand({
  Bucket: 'avatars',
  Key: 'user-avatar.png'
})
const presignedUrl = await getSignedUrl(s3Client, command, { expiresIn: 3600 })
```

---

## Common Pitfalls

### Pitfall 1: RLS auth.uid() Not Working

**Symptom**: RLS policies fail with "column does not exist: auth.uid()"

**Cause**: Supabase's `auth.uid()` function doesn't exist in nself

**Solution**:
```sql
-- Replace auth.uid() with Hasura session variable
-- OLD (Supabase):
USING (auth.uid() = user_id)

-- NEW (nself):
USING (current_setting('hasura.user.id')::uuid = user_id)
```

### Pitfall 2: PostgREST Query Syntax

**Symptom**: GraphQL queries don't work like PostgREST

**Cause**: Different query syntax

**Solution**: Rewrite queries in GraphQL syntax (see Frontend Code Changes above)

### Pitfall 3: Storage URLs Different

**Symptom**: Image URLs return 404

**Cause**: Different storage URL structure

**Solution**:
```typescript
// OLD (Supabase):
// https://[project-ref].supabase.co/storage/v1/object/public/avatars/user.png

// NEW (nself):
// http://minio.localhost/avatars/user.png

// Update all storage URLs in database
UPDATE posts
SET image_url = REPLACE(image_url,
  'https://[project-ref].supabase.co/storage/v1/object/public/',
  'http://minio.localhost/'
);
```

### Pitfall 4: Edge Functions Environment Variables

**Symptom**: Functions can't access Supabase client

**Cause**: Different client library needed

**Solution**:
```typescript
// Replace Supabase client with GraphQL client
import { GraphQLClient } from 'graphql-request'

const client = new GraphQLClient(process.env.GRAPHQL_URL!, {
  headers: {
    'x-hasura-admin-secret': process.env.HASURA_ADMIN_SECRET!
  }
})
```

---

## Rollback Procedure

If migration fails:

### Step 1: Keep Supabase Active

Don't delete Supabase project until fully tested.

### Step 2: DNS Rollback

```bash
# Change DNS back to Supabase
# A record: [your-domain] → Supabase IP

# Wait for propagation (5-60 minutes)
```

### Step 3: Frontend Rollback

```bash
# Revert environment variables
NEXT_PUBLIC_SUPABASE_URL=https://[project-ref].supabase.co
NEXT_PUBLIC_SUPABASE_ANON_KEY=your-anon-key

# Rebuild and deploy
npm run build
vercel deploy
```

---

## Automated Migration Tools

### Migration Helper Script

```bash
#!/bin/bash
# migrate-from-supabase.sh

set -e

echo "🚀 nself Migration Helper - Supabase to nself"
echo "=============================================="

PROJECT_ID=$1
NSELF_PROJECT=$2

if [ -z "$PROJECT_ID" ] || [ -z "$NSELF_PROJECT" ]; then
  echo "Usage: ./migrate-from-supabase.sh <supabase-project-id> <nself-project-name>"
  exit 1
fi

echo "📥 Step 1: Exporting from Supabase..."
mkdir -p migration-backup
supabase db dump --project-id $PROJECT_ID > migration-backup/database.sql

echo "🔧 Step 2: Initializing nself project..."
mkdir -p $NSELF_PROJECT
cd $NSELF_PROJECT
nself init --name $NSELF_PROJECT --env dev

echo "📦 Step 3: Building nself stack..."
nself build
nself start

echo "⏳ Waiting for services..."
sleep 30

echo "📥 Step 4: Importing database..."
nself db import ../migration-backup/database.sql

echo "✅ Migration complete!"
echo ""
echo "⚠️  IMPORTANT: Manual steps required:"
echo "1. Configure OAuth providers in .env"
echo "2. Convert RLS policies to Hasura permissions"
echo "3. Migrate storage files (see docs)"
echo "4. Update frontend code (REST → GraphQL)"
echo "5. Test authentication and permissions"
```

---

## Performance Tuning

After migration, optimize for production:

### Database Optimization

```bash
# Analyze and vacuum
nself db analyze

# Create recommended indexes
nself db schema:indexes

# Enable connection pooling
PGBOUNCER_ENABLED=true
```

### Caching

```bash
# Enable Redis caching
REDIS_ENABLED=true
AUTH_REDIS_ENABLED=true
```

---

## Cost Comparison

### Supabase Pricing (2026)
- **Free**: 500MB DB, 1GB storage
- **Pro ($25/mo)**: 8GB DB, 100GB storage
- **Team ($599/mo)**: 100GB DB, 1TB storage
- **Enterprise**: Custom

### nself (Self-Hosted)
- **nself**: Free (open-source)
- **Infrastructure**: $5-160/mo (VPS)
- **Break-even**: ~$25-50/mo

**Estimated Savings**: 40-70% at scale

---

## Conclusion

Migrating from Supabase to nself requires:
- Database export/import (straightforward)
- RLS → Hasura permissions conversion (medium effort)
- REST → GraphQL code changes (high effort)
- Storage migration (medium effort)
- Functions rewrite (medium effort)

**Total Time**: 8-16 hours

**Recommended Approach**:
1. Migrate to staging first
2. Test thoroughly (2-4 weeks)
3. Migrate production during low-traffic period
4. Keep Supabase running for 2 weeks as fallback

Good luck! 🚀
