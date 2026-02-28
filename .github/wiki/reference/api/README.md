# nself API Reference

Complete API documentation for nself services.

---

## Overview

nself provides multiple API interfaces:

1. **GraphQL API** - Hasura auto-generated API from your database
2. **Authentication API** - User authentication and management
3. **Custom Service APIs** - Your own REST/GraphQL/gRPC APIs

---

## GraphQL API (Hasura)

### Endpoint

```
https://api.{BASE_DOMAIN}/v1/graphql
```

**Local development:**
```
https://api.local.nself.org/v1/graphql
```

### Features

- **Auto-generated from database schema** - No code generation needed
- **Real-time subscriptions** - Live data updates via WebSockets
- **Role-based access control** - Per-table, per-column permissions
- **Custom business logic** - Actions and Event Triggers

### Quick Start

```graphql
# Query
query GetUsers {
  users {
    id
    email
    name
    created_at
  }
}

# Mutation
mutation CreateUser($email: String!, $name: String!) {
  insert_users_one(object: {email: $email, name: $name}) {
    id
    email
    name
  }
}

# Subscription
subscription OnUserCreated {
  users(order_by: {created_at: desc}, limit: 1) {
    id
    email
    name
    created_at
  }
}
```

### Authentication

Include JWT token in requests:

```javascript
const headers = {
  'Authorization': `Bearer ${jwt_token}`,
  'Content-Type': 'application/json'
}
```

**[Full GraphQL API Documentation](../../architecture/API.md)**

---

## Authentication API

### Endpoint

```
https://auth.{BASE_DOMAIN}
```

**Local development:**
```
https://auth.local.nself.org
```

### Available Endpoints

#### Sign Up
```bash
POST /signup/email-password
{
  "email": "user@example.com",
  "password": "secure-password",
  "displayName": "John Doe"
}
```

#### Sign In
```bash
POST /signin/email-password
{
  "email": "user@example.com",
  "password": "secure-password"
}

# Returns
{
  "session": {
    "accessToken": "eyJhbGci...",
    "refreshToken": "v4.public...",
    "user": {
      "id": "uuid",
      "email": "user@example.com",
      "displayName": "John Doe"
    }
  }
}
```

#### Refresh Token
```bash
POST /token
{
  "refreshToken": "v4.public..."
}
```

#### Sign Out
```bash
POST /signout
Authorization: Bearer {accessToken}
```

#### OAuth Providers
```bash
# Google
GET /signin/provider/google

# GitHub
GET /signin/provider/github

# Other providers configured via environment
```

**[Full Authentication API Documentation](../../commands/AUTH.md)**

---

## Custom Service APIs

### REST APIs

Define custom REST APIs using templates:

```bash
# .env
CS_1=my-api:express-ts:8001:api
```

**Endpoint:**
```
https://my-api.{BASE_DOMAIN}
```

**Example:**
```javascript
// Express.js example
app.get('/users', async (req, res) => {
  const users = await db.query('SELECT * FROM users');
  res.json(users);
});

app.post('/users', async (req, res) => {
  const { email, name } = req.body;
  const user = await db.query(
    'INSERT INTO users (email, name) VALUES ($1, $2) RETURNING *',
    [email, name]
  );
  res.json(user);
});
```

### GraphQL APIs

Create custom GraphQL servers:

```bash
# .env
CS_2=graphql-api:graphql-yoga:8002:graphql
```

**Example:**
```javascript
// GraphQL Yoga example
const typeDefs = `
  type Query {
    users: [User!]!
    user(id: ID!): User
  }

  type User {
    id: ID!
    email: String!
    name: String!
  }
`;

const resolvers = {
  Query: {
    users: () => db.query('SELECT * FROM users'),
    user: (_, { id }) => db.query('SELECT * FROM users WHERE id = $1', [id])
  }
};
```

### gRPC APIs

Build gRPC services:

```bash
# .env
CS_3=grpc-api:grpc:8003
```

**Example:**
```protobuf
// user.proto
syntax = "proto3";

service UserService {
  rpc GetUser (GetUserRequest) returns (User);
  rpc ListUsers (ListUsersRequest) returns (UserList);
}

message User {
  string id = 1;
  string email = 2;
  string name = 3;
}
```

---

## Database Connection

### From Frontend

Use Hasura GraphQL API:

```javascript
// Apollo Client
import { ApolloClient, InMemoryCache } from '@apollo/client';

const client = new ApolloClient({
  uri: 'https://api.local.nself.org/v1/graphql',
  cache: new InMemoryCache(),
  headers: {
    'Authorization': `Bearer ${token}`
  }
});
```

### From Custom Services

Direct PostgreSQL connection:

```javascript
// Node.js with pg
const { Pool } = require('pg');

const pool = new Pool({
  host: process.env.POSTGRES_HOST || 'postgres',
  port: process.env.POSTGRES_PORT || 5432,
  database: process.env.POSTGRES_DB,
  user: process.env.POSTGRES_USER,
  password: process.env.POSTGRES_PASSWORD
});

// Query
const result = await pool.query('SELECT * FROM users');
```

**Important:** Use service names (e.g., `postgres`, `redis`) not `localhost` for Docker networking.

**[Service Communication Guide](../../guides/SERVICE-TO-SERVICE-COMMUNICATION.md)**

---

## Real-Time Features

### GraphQL Subscriptions

```graphql
subscription OnNewUser {
  users(order_by: {created_at: desc}, limit: 1) {
    id
    email
    name
    created_at
  }
}
```

### WebSocket Connection

```javascript
// Apollo Client with subscriptions
import { WebSocketLink } from '@apollo/client/link/ws';

const wsLink = new WebSocketLink({
  uri: 'wss://api.local.nself.org/v1/graphql',
  options: {
    reconnect: true,
    connectionParams: {
      headers: {
        Authorization: `Bearer ${token}`
      }
    }
  }
});
```

**[Real-Time Features Guide](../../guides/REALTIME-FEATURES.md)**

---

## Permissions & Security

### Row-Level Security (RLS)

Hasura permissions system:

```yaml
# User can only see their own data
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

### JWT Claims

JWT tokens include custom claims:

```json
{
  "sub": "user-uuid",
  "iat": 1234567890,
  "exp": 1234567890,
  "https://hasura.io/jwt/claims": {
    "x-hasura-allowed-roles": ["user", "admin"],
    "x-hasura-default-role": "user",
    "x-hasura-user-id": "user-uuid"
  }
}
```

### API Rate Limiting

Configure rate limits per service:

```bash
# .env
CS_1_RATE_LIMIT=100  # 100 requests per minute
```

---

## API Versioning

### Hasura API Versions

Hasura supports multiple API versions:

```
/v1/graphql    # Current stable
/v2/graphql    # Future version
```

### Custom API Versioning

Version your custom APIs:

```javascript
// Express.js
app.use('/v1', v1Router);
app.use('/v2', v2Router);

// Access at
// https://my-api.local.nself.org/v1/users
// https://my-api.local.nself.org/v2/users
```

---

## Error Handling

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

### Authentication Errors

```json
{
  "status": 401,
  "error": "invalid_token",
  "message": "JWT token is invalid or expired"
}
```

### Custom API Errors

```javascript
// Standard error format
{
  "error": {
    "code": "USER_NOT_FOUND",
    "message": "User with ID 123 not found",
    "details": {}
  }
}
```

---

## API Testing

### GraphQL Playground

Access Hasura Console:

```
https://api.local.nself.org/console
```

**Note:** Only enabled in development (`HASURA_GRAPHQL_ENABLE_CONSOLE=true`)

### REST API Testing

```bash
# Using curl
curl -X POST https://api.local.nself.org/v1/graphql \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d '{"query": "{ users { id email } }"}'

# Using httpie
http POST https://api.local.nself.org/v1/graphql \
  Authorization:"Bearer ${TOKEN}" \
  query='{ users { id email } }'
```

### Automated Testing

```bash
# Performance testing
nself bench api --endpoint /v1/graphql --duration 60s

# Load testing
nself bench load --rps 100 --duration 5m
```

**[Benchmarking Guide](../../commands/BENCH.md)**

---

## API Documentation Tools

### Hasura Console

Built-in API explorer:
- GraphQL schema browser
- Query builder
- Real-time subscriptions testing

### Swagger/OpenAPI (Custom APIs)

Generate OpenAPI specs for custom APIs:

```javascript
// Express with swagger-jsdoc
const swaggerJsdoc = require('swagger-jsdoc');

const options = {
  definition: {
    openapi: '3.0.0',
    info: {
      title: 'My API',
      version: '1.0.0',
    },
  },
  apis: ['./routes/*.js'],
};

const swaggerSpec = swaggerJsdoc(options);
```

---

## Related Documentation

- **[Architecture Overview](../../architecture/ARCHITECTURE.md)** - System design
- **[Database Workflow](../../guides/DATABASE-WORKFLOW.md)** - Schema to API
- **[Service Communication](../../guides/SERVICE-TO-SERVICE-COMMUNICATION.md)** - Internal APIs
- **[Real-Time Features](../../guides/REALTIME-FEATURES.md)** - Subscriptions
- **[Custom Services](../../services/SERVICES_CUSTOM.md)** - Building APIs

---

**[Back to Documentation Home](../../README.md)**
