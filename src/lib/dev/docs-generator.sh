#!/usr/bin/env bash
# docs-generator.sh - Generate API documentation
# Part of nself v0.7.0 - Sprint 19: Developer Experience Tools


# Get Hasura endpoint
get_hasura_endpoint() {

set -euo pipefail

  local project_name="${PROJECT_NAME:-nself}"
  local base_domain="${BASE_DOMAIN:-localhost}"

  if [[ "$base_domain" == "localhost" ]]; then
    printf "http://localhost:8080"
  else
    printf "https://api.%s" "$base_domain"
  fi
}

# Generate GraphQL documentation
generate_graphql_docs() {
  local schema_file="${1:-schema.graphql}"
  local output_dir="${2:-./.wiki/api}"

  printf "Generating GraphQL documentation...\n"

  mkdir -p "$output_dir"

  # Generate overview documentation
  cat >"$output_dir/README.md" <<'EOF'
# API Documentation

## GraphQL API

### Endpoint

```
https://api.yourdomain.com/v1/graphql
```

### Authentication

All requests require authentication via JWT token in the Authorization header:

```
Authorization: Bearer <access_token>
```

For admin operations, use the admin secret:

```
x-hasura-admin-secret: <admin_secret>
```

## Quick Start

### Queries

Query data from your database:

```graphql
query GetUsers {
  users {
    id
    email
    displayName
    createdAt
  }
}
```

### Mutations

Insert, update, or delete data:

```graphql
mutation CreateUser($email: String!, $displayName: String!) {
  insert_users_one(object: {
    email: $email
    displayName: $displayName
  }) {
    id
    email
  }
}
```

### Subscriptions

Subscribe to real-time updates:

```graphql
subscription OnUserCreated {
  users(order_by: {createdAt: desc}, limit: 1) {
    id
    email
    displayName
  }
}
```

## Common Patterns

### Pagination

```graphql
query GetUsersPaginated($limit: Int!, $offset: Int!) {
  users(limit: $limit, offset: $offset, order_by: {createdAt: desc}) {
    id
    email
  }
  users_aggregate {
    aggregate {
      count
    }
  }
}
```

### Filtering

```graphql
query SearchUsers($search: String!) {
  users(where: {
    _or: [
      {email: {_ilike: $search}},
      {displayName: {_ilike: $search}}
    ]
  }) {
    id
    email
    displayName
  }
}
```

### Relationships

```graphql
query GetUserWithPosts {
  users {
    id
    email
    posts {
      id
      title
      content
    }
  }
}
```

## Error Handling

GraphQL errors are returned in the following format:

```json
{
  "errors": [
    {
      "message": "Error message",
      "extensions": {
        "code": "ERROR_CODE",
        "path": "$.field"
      }
    }
  ]
}
```

Common error codes:

- `validation-failed` - Invalid input
- `constraint-violation` - Database constraint violated
- `permission-denied` - Insufficient permissions
- `invalid-jwt` - Authentication failed

## Rate Limiting

API requests are rate limited to:
- 1000 requests per hour for authenticated users
- 100 requests per hour for unauthenticated users

Rate limit headers:
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1234567890
```

## Best Practices

1. **Use fragments** for reusable field selections
2. **Request only needed fields** to optimize performance
3. **Use variables** instead of string interpolation
4. **Handle errors gracefully** with proper error handling
5. **Implement pagination** for large datasets
6. **Use subscriptions sparingly** to avoid overload

## SDKs

Official SDKs are available for:

- [TypeScript/JavaScript](./sdks/typescript.md)
- [Python](./sdks/python.md)

## Support

For issues or questions:
- GitHub: https://github.com/acamarata/nself
- Documentation: https://docs.nself.org
EOF

  # Generate Postman collection template
  cat >"$output_dir/postman-collection.json" <<'EOF'
{
  "info": {
    "name": "nself API",
    "description": "API collection for nself backend",
    "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
  },
  "auth": {
    "type": "bearer",
    "bearer": [
      {
        "key": "token",
        "value": "{{access_token}}",
        "type": "string"
      }
    ]
  },
  "variable": [
    {
      "key": "base_url",
      "value": "http://localhost:8080",
      "type": "string"
    },
    {
      "key": "access_token",
      "value": "",
      "type": "string"
    },
    {
      "key": "admin_secret",
      "value": "",
      "type": "string"
    }
  ],
  "item": [
    {
      "name": "Authentication",
      "item": [
        {
          "name": "Sign Up",
          "request": {
            "method": "POST",
            "header": [
              {
                "key": "Content-Type",
                "value": "application/json"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "{\n  \"query\": \"mutation SignUp($email: String!, $password: String!, $displayName: String) { signUp(email: $email, password: $password, displayName: $displayName) { accessToken user { id email } } }\",\n  \"variables\": {\n    \"email\": \"user@example.com\",\n    \"password\": \"secure-password\",\n    \"displayName\": \"John Doe\"\n  }\n}"
            },
            "url": {
              "raw": "{{base_url}}/v1/graphql",
              "host": ["{{base_url}}"],
              "path": ["v1", "graphql"]
            }
          }
        },
        {
          "name": "Sign In",
          "request": {
            "method": "POST",
            "header": [
              {
                "key": "Content-Type",
                "value": "application/json"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "{\n  \"query\": \"mutation SignIn($email: String!, $password: String!) { signIn(email: $email, password: $password) { accessToken user { id email } } }\",\n  \"variables\": {\n    \"email\": \"user@example.com\",\n    \"password\": \"secure-password\"\n  }\n}"
            },
            "url": {
              "raw": "{{base_url}}/v1/graphql",
              "host": ["{{base_url}}"],
              "path": ["v1", "graphql"]
            }
          }
        }
      ]
    },
    {
      "name": "Users",
      "item": [
        {
          "name": "Get Users",
          "request": {
            "method": "POST",
            "header": [
              {
                "key": "Content-Type",
                "value": "application/json"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "{\n  \"query\": \"query GetUsers { users { id email displayName createdAt } }\"\n}"
            },
            "url": {
              "raw": "{{base_url}}/v1/graphql",
              "host": ["{{base_url}}"],
              "path": ["v1", "graphql"]
            }
          }
        },
        {
          "name": "Get User by ID",
          "request": {
            "method": "POST",
            "header": [
              {
                "key": "Content-Type",
                "value": "application/json"
              }
            ],
            "body": {
              "mode": "raw",
              "raw": "{\n  \"query\": \"query GetUser($id: uuid!) { users_by_pk(id: $id) { id email displayName createdAt } }\",\n  \"variables\": {\n    \"id\": \"00000000-0000-0000-0000-000000000000\"\n  }\n}"
            },
            "url": {
              "raw": "{{base_url}}/v1/graphql",
              "host": ["{{base_url}}"],
              "path": ["v1", "graphql"]
            }
          }
        }
      ]
    }
  ]
}
EOF

  printf "Documentation generated at: %s\n" "$output_dir"
}

# Generate OpenAPI specification from GraphQL
generate_openapi_spec() {
  local output_file="${1:-./.wiki/api/openapi.yaml}"

  printf "Generating OpenAPI specification...\n"

  mkdir -p "$(dirname "$output_file")"

  cat >"$output_file" <<'EOF'
openapi: 3.0.3
info:
  title: nself API
  description: REST API for nself backend
  version: 1.0.0
  contact:
    name: nself Support
    url: https://docs.nself.org
    email: support@nself.org
  license:
    name: MIT
    url: https://opensource.org/licenses/MIT

servers:
  - url: http://localhost:8080
    description: Local development
  - url: https://api.{domain}
    description: Production
    variables:
      domain:
        default: example.com
        description: Your domain

tags:
  - name: Authentication
    description: User authentication operations
  - name: Users
    description: User management
  - name: Storage
    description: File storage operations

paths:
  /auth/signup:
    post:
      tags:
        - Authentication
      summary: Sign up new user
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - email
                - password
              properties:
                email:
                  type: string
                  format: email
                password:
                  type: string
                  format: password
                  minLength: 8
                displayName:
                  type: string
                metadata:
                  type: object
      responses:
        '200':
          description: User created successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AuthResponse'
        '400':
          description: Invalid input
        '409':
          description: User already exists

  /auth/signin:
    post:
      tags:
        - Authentication
      summary: Sign in existing user
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - email
                - password
              properties:
                email:
                  type: string
                  format: email
                password:
                  type: string
                  format: password
      responses:
        '200':
          description: Sign in successful
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AuthResponse'
        '401':
          description: Invalid credentials

  /auth/signout:
    post:
      tags:
        - Authentication
      summary: Sign out current user
      security:
        - BearerAuth: []
      responses:
        '200':
          description: Sign out successful
        '401':
          description: Unauthorized

  /auth/refresh:
    post:
      tags:
        - Authentication
      summary: Refresh access token
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - refreshToken
              properties:
                refreshToken:
                  type: string
      responses:
        '200':
          description: Token refreshed
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AuthResponse'
        '401':
          description: Invalid refresh token

  /v1/graphql:
    post:
      tags:
        - GraphQL
      summary: Execute GraphQL query or mutation
      security:
        - BearerAuth: []
        - AdminSecret: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - query
              properties:
                query:
                  type: string
                  description: GraphQL query or mutation
                variables:
                  type: object
                  description: Query variables
                operationName:
                  type: string
                  description: Operation name (for multi-operation documents)
      responses:
        '200':
          description: Query executed successfully
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: object
                  errors:
                    type: array
                    items:
                      type: object

  /storage/upload:
    post:
      tags:
        - Storage
      summary: Upload file
      security:
        - BearerAuth: []
      requestBody:
        required: true
        content:
          multipart/form-data:
            schema:
              type: object
              required:
                - file
              properties:
                file:
                  type: string
                  format: binary
                bucketId:
                  type: string
      responses:
        '200':
          description: File uploaded successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/FileMetadata'

  /storage/files/{fileId}:
    get:
      tags:
        - Storage
      summary: Get file
      parameters:
        - name: fileId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        '200':
          description: File retrieved
          content:
            application/octet-stream:
              schema:
                type: string
                format: binary

    delete:
      tags:
        - Storage
      summary: Delete file
      security:
        - BearerAuth: []
      parameters:
        - name: fileId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        '200':
          description: File deleted
        '404':
          description: File not found

components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT

    AdminSecret:
      type: apiKey
      in: header
      name: x-hasura-admin-secret

  schemas:
    User:
      type: object
      properties:
        id:
          type: string
          format: uuid
        email:
          type: string
          format: email
        displayName:
          type: string
        createdAt:
          type: string
          format: date-time

    AuthResponse:
      type: object
      properties:
        accessToken:
          type: string
        refreshToken:
          type: string
        user:
          $ref: '#/components/schemas/User'

    FileMetadata:
      type: object
      properties:
        id:
          type: string
          format: uuid
        name:
          type: string
        mimeType:
          type: string
        size:
          type: integer
        bucketId:
          type: string
        url:
          type: string
          format: uri
        createdAt:
          type: string
          format: date-time

    Error:
      type: object
      properties:
        message:
          type: string
        code:
          type: string
        details:
          type: object
EOF

  printf "OpenAPI specification generated at: %s\n" "$output_file"
}

# Generate API documentation
generate_docs() {
  local output_dir="${1:-./.wiki/api}"

  printf "Generating API documentation...\n"

  # Generate GraphQL documentation
  generate_graphql_docs "schema.graphql" "$output_dir"

  # Generate OpenAPI spec
  generate_openapi_spec "$output_dir/openapi.yaml"

  # Generate examples directory
  mkdir -p "$output_dir/examples"

  # Create example queries
  cat >"$output_dir/examples/queries.graphql" <<'EOF'
# Get all users
query GetUsers {
  users {
    id
    email
    displayName
    createdAt
  }
}

# Get user by ID
query GetUser($id: uuid!) {
  users_by_pk(id: $id) {
    id
    email
    displayName
    createdAt
  }
}

# Search users
query SearchUsers($search: String!) {
  users(where: {
    _or: [
      {email: {_ilike: $search}},
      {displayName: {_ilike: $search}}
    ]
  }) {
    id
    email
    displayName
  }
}

# Get users with pagination
query GetUsersPaginated($limit: Int!, $offset: Int!) {
  users(limit: $limit, offset: $offset, order_by: {createdAt: desc}) {
    id
    email
    displayName
  }
  users_aggregate {
    aggregate {
      count
    }
  }
}
EOF

  cat >"$output_dir/examples/mutations.graphql" <<'EOF'
# Sign up new user
mutation SignUp($email: String!, $password: String!, $displayName: String) {
  signUp(email: $email, password: $password, displayName: $displayName) {
    accessToken
    refreshToken
    user {
      id
      email
      displayName
    }
  }
}

# Sign in user
mutation SignIn($email: String!, $password: String!) {
  signIn(email: $email, password: $password) {
    accessToken
    refreshToken
    user {
      id
      email
    }
  }
}

# Update user
mutation UpdateUser($id: uuid!, $displayName: String!) {
  update_users_by_pk(pk_columns: {id: $id}, _set: {displayName: $displayName}) {
    id
    displayName
  }
}

# Delete user
mutation DeleteUser($id: uuid!) {
  delete_users_by_pk(id: $id) {
    id
  }
}
EOF

  cat >"$output_dir/examples/subscriptions.graphql" <<'EOF'
# Subscribe to new users
subscription OnUserCreated {
  users(order_by: {createdAt: desc}, limit: 1) {
    id
    email
    displayName
    createdAt
  }
}

# Subscribe to user updates
subscription OnUserUpdated($userId: uuid!) {
  users_by_pk(id: $userId) {
    id
    email
    displayName
    updatedAt
  }
}
EOF

  printf "\nDocumentation generated successfully!\n"
  printf "Location: %s\n" "$output_dir"
  printf "\nGenerated files:\n"
  printf "  - README.md - API overview\n"
  printf "  - openapi.yaml - OpenAPI specification\n"
  printf "  - postman-collection.json - Postman collection\n"
  printf "  - examples/ - GraphQL query examples\n"
}

export -f get_hasura_endpoint generate_graphql_docs
export -f generate_openapi_spec generate_docs
