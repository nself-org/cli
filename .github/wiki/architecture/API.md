# nself API Documentation

**Version:** v0.9.6
**Status:** Production Ready
**Last Updated:** January 30, 2026

Complete API reference for nself services, including GraphQL, Authentication, Storage, Real-time, and Custom Service APIs.

---

## Table of Contents

1. [Overview](#overview)
2. [GraphQL API (Hasura)](#graphql-api-hasura)
3. [Authentication API](#authentication-api)
4. [Storage API (MinIO)](#storage-api-minio)
5. [Real-Time API (WebSocket)](#real-time-api-websocket)
6. [Functions API](#functions-api)
7. [Custom Service APIs](#custom-service-apis)
8. [Multi-Tenancy API](#multi-tenancy-api)
9. [Billing API](#billing-api)
10. [Security & Authentication](#security--authentication)
11. [Rate Limiting](#rate-limiting)
12. [Error Handling](#error-handling)
13. [API Versioning](#api-versioning)
14. [Testing & Debugging](#testing--debugging)

---

## Overview

### API Architecture

nself provides a comprehensive suite of APIs built on modern standards:

```
┌──────────────────────────────────────────────────────────┐
│                     Client Applications                   │
│            (Web, Mobile, Desktop, IoT)                    │
└────────────────────┬─────────────────────────────────────┘
                     │
                     │ HTTPS / WSS
                     ▼
┌──────────────────────────────────────────────────────────┐
│                  Nginx Reverse Proxy                      │
│              (SSL Termination, Routing)                   │
└─┬────────┬──────────┬─────────┬───────────┬─────────────┘
  │        │          │         │           │
  │        │          │         │           │
  ▼        ▼          ▼         ▼           ▼
┌────┐ ┌──────┐ ┌─────────┐ ┌────────┐ ┌────────────┐
│ GQL│ │ Auth │ │ Storage │ │Realtime│ │   Custom   │
│API │ │ API  │ │   API   │ │  API   │ │  Services  │
└─┬──┘ └──┬───┘ └────┬────┘ └───┬────┘ └──────┬─────┘
  │       │          │           │             │
  └───────┴──────────┴───────────┴─────────────┘
                     │
                     ▼
           ┌──────────────────┐
           │   PostgreSQL DB  │
           │   + Extensions   │
           └──────────────────┘
```

### Available APIs

| API | Endpoint | Protocol | Purpose |
|-----|----------|----------|---------|
| **GraphQL** | `https://api.{domain}/v1/graphql` | HTTPS, WSS | Database operations, real-time subscriptions |
| **Authentication** | `https://auth.{domain}` | HTTPS | User authentication, OAuth, JWT management |
| **Storage** | `https://storage.{domain}` | HTTPS | File upload, download, management |
| **Real-Time** | `wss://realtime.{domain}` | WebSocket | Live messaging, presence, broadcasts |
| **Functions** | `https://functions.{domain}` | HTTPS | Serverless function execution |
| **Custom Services** | `https://{service}.{domain}` | HTTPS/gRPC | User-defined APIs |

### Local Development URLs

```bash
# Default local development endpoints
https://api.local.nself.org           # GraphQL API
https://auth.local.nself.org          # Authentication
https://storage.local.nself.org       # Storage/MinIO
wss://realtime.local.nself.org        # WebSocket
https://functions.local.nself.org     # Functions

# View all service URLs
nself urls
```

---

## GraphQL API (Hasura)

### Overview

Hasura provides an instant, auto-generated GraphQL API over PostgreSQL with real-time subscriptions, role-based access control, and powerful query capabilities.

**Version:** v2.44.0
**Endpoint:** `https://api.{domain}/v1/graphql`
**Console:** `https://api.{domain}/console` (dev only)

### Key Features

- Auto-generated CRUD operations
- Real-time subscriptions via WebSocket
- Role-based permissions (RLS)
- Remote schemas integration
- Actions for custom business logic
- Event triggers for workflows
- RESTified endpoints

### Authentication

Include JWT token in all requests:

```javascript
const headers = {
  'Authorization': `Bearer ${jwt_token}`,
  'Content-Type': 'application/json'
}
```

### Basic Queries

#### Fetch Data

```graphql
query GetUsers {
  users {
    id
    email
    name
    created_at
  }
}
```

#### Fetch with Filters

```graphql
query GetActiveUsers {
  users(
    where: { is_active: { _eq: true } }
    order_by: { created_at: desc }
    limit: 10
  ) {
    id
    email
    name
  }
}
```

#### Fetch with Relationships

```graphql
query GetUsersWithPosts {
  users {
    id
    name
    posts {
      id
      title
      content
      created_at
    }
  }
}
```

#### Aggregations

```graphql
query GetUserStats {
  users_aggregate {
    aggregate {
      count
      avg {
        age
      }
    }
  }
}
```

### Mutations

#### Insert Single Record

```graphql
mutation CreateUser($email: String!, $name: String!) {
  insert_users_one(object: {
    email: $email
    name: $name
  }) {
    id
    email
    name
    created_at
  }
}
```

#### Insert Multiple Records

```graphql
mutation CreateMultipleUsers($users: [users_insert_input!]!) {
  insert_users(objects: $users) {
    returning {
      id
      email
      name
    }
  }
}
```

#### Update Record

```graphql
mutation UpdateUser($id: uuid!, $name: String!) {
  update_users_by_pk(
    pk_columns: { id: $id }
    _set: { name: $name }
  ) {
    id
    name
    updated_at
  }
}
```

#### Delete Record

```graphql
mutation DeleteUser($id: uuid!) {
  delete_users_by_pk(id: $id) {
    id
    email
  }
}
```

#### Upsert (Insert or Update)

```graphql
mutation UpsertUser($email: String!, $name: String!) {
  insert_users_one(
    object: { email: $email, name: $name }
    on_conflict: {
      constraint: users_email_key
      update_columns: [name]
    }
  ) {
    id
    email
    name
  }
}
```

### Subscriptions (Real-Time)

#### Subscribe to New Records

```graphql
subscription OnNewUsers {
  users(
    order_by: { created_at: desc }
    limit: 1
  ) {
    id
    email
    name
    created_at
  }
}
```

#### Subscribe with Filters

```graphql
subscription OnUserStatusChange($userId: uuid!) {
  users_by_pk(id: $userId) {
    id
    status
    last_seen
  }
}
```

#### Subscribe to Aggregations

```graphql
subscription OnUserCountChange {
  users_aggregate {
    aggregate {
      count
    }
  }
}
```

### Advanced Features

#### Custom Actions

Define custom business logic:

```graphql
mutation ProcessPayment($amount: Float!, $userId: uuid!) {
  processPayment(amount: $amount, userId: $userId) {
    success
    transactionId
    message
  }
}
```

#### Remote Schemas

Integrate external GraphQL APIs:

```graphql
query GetWeather($city: String!) {
  weather(city: $city) {
    temperature
    conditions
    forecast {
      day
      high
      low
    }
  }
}
```

### Client Libraries

#### JavaScript (Apollo Client)

```javascript
import { ApolloClient, InMemoryCache, gql } from '@apollo/client';

const client = new ApolloClient({
  uri: 'https://api.local.nself.org/v1/graphql',
  cache: new InMemoryCache(),
  headers: {
    'Authorization': `Bearer ${token}`
  }
});

// Query
const { data } = await client.query({
  query: gql`
    query GetUsers {
      users {
        id
        email
        name
      }
    }
  `
});

// Mutation
const { data } = await client.mutate({
  mutation: gql`
    mutation CreateUser($email: String!, $name: String!) {
      insert_users_one(object: { email: $email, name: $name }) {
        id
      }
    }
  `,
  variables: { email: 'user@example.com', name: 'John' }
});
```

#### JavaScript (GraphQL Request)

```javascript
import { request, gql } from 'graphql-request';

const endpoint = 'https://api.local.nself.org/v1/graphql';
const headers = {
  'Authorization': `Bearer ${token}`
};

const query = gql`
  query GetUsers {
    users {
      id
      email
      name
    }
  }
`;

const data = await request(endpoint, query, {}, headers);
```

#### Python (gql)

```python
from gql import gql, Client
from gql.transport.requests import RequestsHTTPTransport

transport = RequestsHTTPTransport(
    url='https://api.local.nself.org/v1/graphql',
    headers={'Authorization': f'Bearer {token}'}
)

client = Client(transport=transport, fetch_schema_from_transport=True)

query = gql("""
    query GetUsers {
        users {
            id
            email
            name
        }
    }
""")

result = client.execute(query)
```

### Permissions & RLS

Hasura uses row-level security for fine-grained access control:

```yaml
# Example permission for 'user' role
table: users
role: user
permission:
  filter:
    id: { _eq: X-Hasura-User-Id }
  columns:
    - id
    - email
    - name
  check:
    id: { _eq: X-Hasura-User-Id }
```

### Performance Optimization

#### Query Optimization

```graphql
# Use limit and offset for pagination
query GetUsersPaginated($limit: Int = 20, $offset: Int = 0) {
  users(limit: $limit, offset: $offset) {
    id
    name
  }
}

# Select only needed fields
query GetUserEmails {
  users {
    email  # Don't fetch unnecessary fields
  }
}

# Use indexes for filtering
query GetUsersByStatus($status: String!) {
  users(where: { status: { _eq: $status } }) {
    id
    name
  }
}
```

---

## Authentication API

### Overview

nHost Auth provides complete authentication and authorization with JWT tokens, OAuth, MFA, and user management.

**Version:** 0.36.0
**Endpoint:** `https://auth.{domain}`
**Database:** Uses `auth` schema in PostgreSQL

### Sign Up

#### Email/Password Registration

```bash
POST /signup/email-password
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "SecurePassword123!",
  "displayName": "John Doe",
  "metadata": {
    "role": "user"
  }
}

# Response
{
  "session": {
    "accessToken": "eyJhbGci...",
    "accessTokenExpiresIn": 900,
    "refreshToken": "v4.public...",
    "user": {
      "id": "123e4567-e89b-12d3-a456-426614174000",
      "email": "user@example.com",
      "displayName": "John Doe",
      "emailVerified": false,
      "defaultRole": "user",
      "roles": ["user"]
    }
  }
}
```

#### With Email Verification

```bash
POST /signup/email-password
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "SecurePassword123!",
  "options": {
    "allowedRoles": ["user"],
    "defaultRole": "user",
    "redirectTo": "https://app.example.com/verify"
  }
}

# User receives verification email
# Click link to verify: /verify?token=xyz
```

### Sign In

#### Email/Password Login

```bash
POST /signin/email-password
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "SecurePassword123!"
}

# Response
{
  "session": {
    "accessToken": "eyJhbGci...",
    "accessTokenExpiresIn": 900,
    "refreshToken": "v4.public...",
    "user": {
      "id": "123e4567-e89b-12d3-a456-426614174000",
      "email": "user@example.com",
      "displayName": "John Doe",
      "defaultRole": "user"
    }
  }
}
```

#### Magic Link (Passwordless)

```bash
POST /signin/passwordless/email
Content-Type: application/json

{
  "email": "user@example.com",
  "options": {
    "redirectTo": "https://app.example.com/auth/callback"
  }
}

# User receives email with magic link
# Click link to authenticate automatically
```

#### OAuth Providers

```bash
# Google
GET /signin/provider/google

# GitHub
GET /signin/provider/github

# Other providers (must be configured)
GET /signin/provider/{provider}

# Callback after OAuth
GET /signin/provider/{provider}/callback?code=xyz&state=abc
```

### Token Management

#### Refresh Access Token

```bash
POST /token
Content-Type: application/json

{
  "refreshToken": "v4.public.eyJ..."
}

# Response
{
  "session": {
    "accessToken": "eyJhbGci...",
    "accessTokenExpiresIn": 900
  }
}
```

#### Revoke Refresh Token

```bash
POST /token/revoke
Authorization: Bearer {accessToken}
Content-Type: application/json

{
  "refreshToken": "v4.public.eyJ..."
}
```

### User Management

#### Get User Profile

```bash
GET /user
Authorization: Bearer {accessToken}

# Response
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "email": "user@example.com",
  "displayName": "John Doe",
  "emailVerified": true,
  "defaultRole": "user",
  "roles": ["user"],
  "metadata": {},
  "createdAt": "2026-01-29T10:00:00Z"
}
```

#### Update User Profile

```bash
POST /user
Authorization: Bearer {accessToken}
Content-Type: application/json

{
  "displayName": "Jane Doe",
  "metadata": {
    "avatar": "https://..."
  }
}
```

#### Change Password

```bash
POST /user/password
Authorization: Bearer {accessToken}
Content-Type: application/json

{
  "newPassword": "NewSecurePassword456!"
}
```

#### Reset Password

```bash
# Request password reset
POST /user/password/reset
Content-Type: application/json

{
  "email": "user@example.com",
  "options": {
    "redirectTo": "https://app.example.com/reset-password"
  }
}

# User receives reset email with token
# Submit new password
POST /user/password/reset/confirm
Content-Type: application/json

{
  "ticket": "reset-token-from-email",
  "newPassword": "NewSecurePassword456!"
}
```

### Sign Out

```bash
POST /signout
Authorization: Bearer {accessToken}

# Invalidates current session
# Client should discard tokens
```

### Multi-Factor Authentication (MFA)

#### Generate TOTP Secret

```bash
POST /mfa/totp/generate
Authorization: Bearer {accessToken}

# Response
{
  "imageUrl": "data:image/png;base64,...",
  "totpSecret": "JBSWY3DPEHPK3PXP"
}
```

#### Activate MFA

```bash
POST /mfa/totp/activate
Authorization: Bearer {accessToken}
Content-Type: application/json

{
  "code": "123456"
}
```

#### Verify MFA Code

```bash
POST /signin/mfa/totp
Content-Type: application/json

{
  "ticket": "mfa-ticket-from-initial-login",
  "code": "123456"
}
```

### OAuth Configuration

Configure OAuth providers via environment variables:

```bash
# Google OAuth
AUTH_PROVIDER_GOOGLE_ENABLED=true
AUTH_PROVIDER_GOOGLE_CLIENT_ID=your-client-id
AUTH_PROVIDER_GOOGLE_CLIENT_SECRET=your-client-secret

# GitHub OAuth
AUTH_PROVIDER_GITHUB_ENABLED=true
AUTH_PROVIDER_GITHUB_CLIENT_ID=your-client-id
AUTH_PROVIDER_GITHUB_CLIENT_SECRET=your-client-secret

# Manage via CLI
nself auth oauth enable google
nself auth oauth config google
```

### JWT Structure

JWT tokens include custom Hasura claims:

```json
{
  "sub": "123e4567-e89b-12d3-a456-426614174000",
  "iat": 1706529600,
  "exp": 1706530500,
  "https://hasura.io/jwt/claims": {
    "x-hasura-allowed-roles": ["user", "admin"],
    "x-hasura-default-role": "user",
    "x-hasura-user-id": "123e4567-e89b-12d3-a456-426614174000",
    "x-hasura-tenant-id": "tenant-abc123"
  }
}
```

### Client Libraries

#### JavaScript

```javascript
import { NhostClient } from '@nhost/nhost-js';

const nhost = new NhostClient({
  subdomain: 'local.nself.org',
  region: ''
});

// Sign up
const { session, error } = await nhost.auth.signUp({
  email: 'user@example.com',
  password: 'SecurePassword123!'
});

// Sign in
const { session, error } = await nhost.auth.signIn({
  email: 'user@example.com',
  password: 'SecurePassword123!'
});

// Get access token
const token = nhost.auth.getAccessToken();

// Sign out
await nhost.auth.signOut();
```

---

## Storage API (MinIO)

### Overview

MinIO provides S3-compatible object storage for files, images, videos, and documents.

**Endpoint:** `https://storage.{domain}`
**Console:** `https://minio.{domain}` (admin UI)
**Protocol:** S3-compatible API

### Enable Storage

```bash
# Enable in .env
MINIO_ENABLED=true

# Initialize storage
nself service storage init
```

### Upload File

#### Direct Upload (S3 API)

```javascript
import AWS from 'aws-sdk';

const s3 = new AWS.S3({
  endpoint: 'https://storage.local.nself.org',
  accessKeyId: process.env.MINIO_ROOT_USER,
  secretAccessKey: process.env.MINIO_ROOT_PASSWORD,
  s3ForcePathStyle: true,
  signatureVersion: 'v4'
});

// Upload file
const result = await s3.upload({
  Bucket: 'uploads',
  Key: 'documents/file.pdf',
  Body: fileBuffer,
  ContentType: 'application/pdf'
}).promise();

console.log('File URL:', result.Location);
```

#### Upload via GraphQL (with Hasura integration)

```javascript
// Use presigned URL for secure uploads
const mutation = gql`
  mutation GetUploadUrl($fileName: String!, $contentType: String!) {
    getUploadUrl(fileName: $fileName, contentType: $contentType) {
      url
      fileId
    }
  }
`;

const { data } = await client.mutate({
  mutation,
  variables: {
    fileName: 'document.pdf',
    contentType: 'application/pdf'
  }
});

// Upload file to presigned URL
await fetch(data.getUploadUrl.url, {
  method: 'PUT',
  body: fileBuffer,
  headers: {
    'Content-Type': 'application/pdf'
  }
});
```

### Download File

```javascript
// Get file
const file = await s3.getObject({
  Bucket: 'uploads',
  Key: 'documents/file.pdf'
}).promise();

console.log('File content:', file.Body);
```

### List Files

```javascript
// List all files in bucket
const files = await s3.listObjectsV2({
  Bucket: 'uploads',
  Prefix: 'documents/'
}).promise();

files.Contents.forEach(file => {
  console.log('File:', file.Key, 'Size:', file.Size);
});
```

### Delete File

```javascript
// Delete file
await s3.deleteObject({
  Bucket: 'uploads',
  Key: 'documents/file.pdf'
}).promise();
```

### Presigned URLs

Generate temporary URLs for secure access:

```javascript
// Generate presigned URL (valid for 1 hour)
const url = s3.getSignedUrl('getObject', {
  Bucket: 'uploads',
  Key: 'documents/file.pdf',
  Expires: 3600
});

console.log('Download URL:', url);
```

### Storage Configuration

```bash
# .env
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin
MINIO_CONSOLE_PORT=9001
MINIO_BROWSER=on
MINIO_DEFAULT_BUCKETS=uploads,public,private
```

---

## Real-Time API (WebSocket)

### Overview

WebSocket server for real-time messaging, presence tracking, and live collaboration.

**Endpoint:** `wss://realtime.{domain}`
**Protocol:** Socket.IO over WebSocket
**Database:** Uses `realtime` schema in PostgreSQL

### Initialize Real-Time

```bash
# Initialize real-time system
nself service realtime init

# Start WebSocket server
nself service realtime start

# Check status
nself service realtime status
```

### Connect to WebSocket

#### JavaScript Client

```javascript
import { io } from 'socket.io-client';

const socket = io('wss://realtime.local.nself.org', {
  auth: {
    token: 'your-jwt-token'
  },
  transports: ['websocket', 'polling']
});

socket.on('connect', () => {
  console.log('Connected:', socket.id);
});

socket.on('disconnect', (reason) => {
  console.log('Disconnected:', reason);
});
```

### Channel Operations

#### Subscribe to Channel

```javascript
// Subscribe to public channel
socket.emit('subscribe', { channel: 'general' });

// Listen for subscription confirmation
socket.on('subscribed', (data) => {
  console.log('Subscribed to:', data.channel);
});
```

#### Send Message

```javascript
socket.emit('message:send', {
  channel: 'general',
  content: 'Hello, world!',
  messageType: 'text'
});

// Listen for message confirmation
socket.on('message:sent', (data) => {
  console.log('Message sent:', data.id);
});
```

#### Receive Messages

```javascript
socket.on('message:new', (data) => {
  console.log('New message:', data);
  // {
  //   id: '...',
  //   channelId: '...',
  //   userId: '...',
  //   content: 'Hello, world!',
  //   messageType: 'text',
  //   sentAt: '2026-01-30T10:00:00Z'
  // }
});
```

### Presence Tracking

#### Update Presence

```javascript
socket.emit('presence:update', {
  channel: 'general',
  status: 'online',
  metadata: {
    displayName: 'Alice',
    avatar: 'https://...'
  }
});
```

#### Get Online Users

```javascript
socket.emit('presence:get', { channel: 'general' });

socket.on('presence:list', (data) => {
  console.log('Online users:', data.users);
  // [
  //   { userId: '...', status: 'online', metadata: {...} },
  //   { userId: '...', status: 'away', metadata: {...} }
  // ]
});
```

### Broadcasting Events

Send ephemeral events (typing indicators, cursor movement):

```javascript
// Broadcast typing indicator
socket.emit('broadcast', {
  channel: 'general',
  eventType: 'typing_start',
  payload: { displayName: 'Alice' }
});

// Listen for broadcasts
socket.on('broadcast', (data) => {
  console.log('Broadcast:', data.eventType, data.payload);
});
```

### Database Change Streaming

Subscribe to real-time database notifications:

```javascript
// Subscribe to table notifications
socket.emit('subscribe', { channel: 'table_users' });

// Listen for database changes
socket.on('db:notification', (data) => {
  console.log('Operation:', data.operation); // INSERT, UPDATE, DELETE
  console.log('Record:', data.record);
});
```

---

## Functions API

### Overview

Serverless functions runtime for custom business logic.

**Endpoint:** `https://functions.{domain}`
**Runtime:** Node.js, Python, Go (configurable)

### Enable Functions

```bash
# Enable in .env
FUNCTIONS_ENABLED=true

# Deploy function
nself service functions deploy my-function
```

### Function Structure

```javascript
// functions/my-function/index.js
export default async function handler(req, res) {
  const { body, query, headers } = req;

  // Your business logic
  const result = await processData(body);

  res.status(200).json({ success: true, data: result });
}
```

### Invoke Function

```bash
# HTTP request
curl -X POST https://functions.local.nself.org/my-function \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"key": "value"}'
```

---

## Custom Service APIs

### Overview

Create custom REST, GraphQL, or gRPC APIs using service templates.

### Define Custom Service

```bash
# .env
CS_1=my-api:express-ts:8001:api
CS_2=graphql-api:graphql-yoga:8002:graphql
CS_3=grpc-api:grpc:8003
```

### Access Endpoints

```bash
# REST API
https://my-api.local.nself.org

# GraphQL API
https://graphql-api.local.nself.org/graphql

# gRPC (internal)
grpc-api:8003
```

### Example: Express REST API

```javascript
// services/my-api/src/index.ts
import express from 'express';
import { Pool } from 'pg';

const app = express();
const db = new Pool({
  host: 'postgres',
  port: 5432,
  database: process.env.POSTGRES_DB
});

app.get('/users', async (req, res) => {
  const result = await db.query('SELECT * FROM users');
  res.json(result.rows);
});

app.post('/users', async (req, res) => {
  const { email, name } = req.body;
  const result = await db.query(
    'INSERT INTO users (email, name) VALUES ($1, $2) RETURNING *',
    [email, name]
  );
  res.json(result.rows[0]);
});

app.listen(8001, () => {
  console.log('API running on port 8001');
});
```

---

## Multi-Tenancy API

### Overview

Built-in multi-tenancy support with tenant isolation, member management, and billing integration.

### CLI Commands

```bash
# Create tenant
nself tenant create myapp --plan pro

# List tenants
nself tenant list

# Add member
nself tenant member add tenant-123 user@example.com admin

# Billing
nself tenant billing subscribe tenant-123 pro
```

### GraphQL API

All tables support automatic tenant isolation via JWT claims:

```graphql
# Query tenant-specific data
query GetTenantUsers {
  users {
    id
    email
    name
  }
}
# Automatically filtered by x-hasura-tenant-id from JWT
```

---

## Billing API

### Overview

Stripe-integrated billing system for SaaS applications.

### Plans

```graphql
query GetPlans {
  billing_plans {
    id
    name
    price
    interval
    features
  }
}
```

### Subscriptions

```graphql
mutation SubscribeToPlan($tenantId: uuid!, $planId: uuid!) {
  insert_billing_subscriptions_one(object: {
    tenant_id: $tenantId
    plan_id: $planId
  }) {
    id
    status
  }
}
```

### Usage Tracking

```graphql
query GetUsage($tenantId: uuid!) {
  billing_usage(
    where: { tenant_id: { _eq: $tenantId } }
    order_by: { created_at: desc }
  ) {
    metric
    quantity
    created_at
  }
}
```

---

## Security & Authentication

### JWT Authentication

All APIs require valid JWT tokens in the Authorization header:

```bash
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

### Row-Level Security (RLS)

PostgreSQL RLS policies enforce data access control:

```sql
-- Users can only see their own data
CREATE POLICY users_own_data ON users
  FOR SELECT
  USING (id = current_user_id());
```

### API Keys

For server-to-server communication:

```bash
# Generate API key
nself auth api-key create --name "Integration Key"

# Use in requests
X-API-Key: nself_sk_live_abc123xyz
```

---

## Rate Limiting

### Configuration

```bash
# .env
RATE_LIMIT_ENABLED=true
RATE_LIMIT_MAX_REQUESTS=100
RATE_LIMIT_WINDOW_MS=60000
```

### Headers

Rate limit info returned in response headers:

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1706530500
```

### CLI Management

```bash
# Configure rate limits
nself auth rate-limit config --max 100 --window 60s

# Check status
nself auth rate-limit status
```

---

## Error Handling

### Standard Error Format

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Email is required",
    "details": {
      "field": "email",
      "constraint": "not_null"
    }
  }
}
```

### HTTP Status Codes

| Code | Meaning | Usage |
|------|---------|-------|
| 200 | OK | Successful request |
| 201 | Created | Resource created successfully |
| 400 | Bad Request | Invalid input |
| 401 | Unauthorized | Missing or invalid authentication |
| 403 | Forbidden | Insufficient permissions |
| 404 | Not Found | Resource does not exist |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | Server-side error |

### GraphQL Errors

```json
{
  "errors": [
    {
      "message": "Field 'name' is required",
      "extensions": {
        "path": "$.selectionSet.users.args.name",
        "code": "validation-failed"
      }
    }
  ]
}
```

---

## API Versioning

### GraphQL Versioning

```
/v1/graphql    # Current stable version
/v2/graphql    # Future version (when available)
```

### REST API Versioning

```
/v1/users      # Version 1
/v2/users      # Version 2
```

### Custom Service Versioning

Implement versioning in your custom services:

```javascript
app.use('/v1', v1Router);
app.use('/v2', v2Router);
```

---

## Testing & Debugging

### GraphQL Playground

Access Hasura Console for interactive testing:

```
https://api.local.nself.org/console
```

### API Testing Tools

```bash
# Using curl
curl -X POST https://api.local.nself.org/v1/graphql \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ users { id email } }"}'

# Using httpie
http POST https://api.local.nself.org/v1/graphql \
  Authorization:"Bearer ${TOKEN}" \
  query='{ users { id email } }'
```

### Performance Testing

```bash
# Benchmark API
nself perf bench api --endpoint /v1/graphql --duration 60s

# Load testing
nself perf bench --rps 100 --duration 5m
```

### View API Logs

```bash
# View Hasura logs
nself logs hasura -f

# View Auth logs
nself logs auth -f

# View all service logs
nself logs -f
```

---

## Related Documentation

- **[Command Reference](../commands/COMMAND-TREE-V1.md)** - Complete CLI command tree
- **[Database Workflow](../guides/DATABASE-WORKFLOW.md)** - Schema to API workflow
- **[Service Communication](../guides/SERVICE-TO-SERVICE-COMMUNICATION.md)** - Internal API patterns
- **[Real-Time Features](../guides/REALTIME-FEATURES.md)** - WebSocket API details
- **[Security Guide](../guides/SECURITY.md)** - API security best practices
- **[Multi-Tenancy](./MULTI-TENANCY.md)** - Tenant isolation architecture

---

**[Back to Documentation Home](../README.md)**
