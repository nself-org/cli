# nself dev

Developer experience tools for SDK generation, documentation, and testing.

## Description

The `dev` command provides a comprehensive suite of developer experience tools including:
- SDK generation from GraphQL schema (TypeScript, Python)
- API documentation generation
- Test environment setup and management
- Mock data generation
- Database snapshot management

**Use Cases:**
- Generate type-safe SDKs for frontend/mobile apps
- Create API documentation for external developers
- Set up test fixtures and factories
- Generate mock data for development
- Snapshot testing workflows

---

## Usage

```bash
nself dev <command> [options]
```

---

## Commands

### SDK Generation

#### `sdk generate` - Generate SDK from GraphQL Schema

Generate a type-safe SDK for your GraphQL API.

```bash
nself dev sdk generate <language> [output]
```

**Parameters:**
- `language` - SDK language (`typescript` or `python`)
- `output` (optional) - Output directory (default: `./sdk/<language>`)

**Examples:**
```bash
# Generate TypeScript SDK
nself dev sdk generate typescript

# Generate to custom location
nself dev sdk generate typescript ./frontend/sdk

# Generate Python SDK
nself dev sdk generate python

# Generate Python SDK to custom location
nself dev sdk generate python ./backend/sdk
```

**What it generates:**
- Type definitions from GraphQL schema
- Query/mutation functions
- GraphQL client configuration
- Authentication helpers
- Error handling utilities

---

### Documentation Generation

#### `docs generate` - Generate API Documentation

Generate comprehensive API documentation from your GraphQL schema.

```bash
nself dev docs generate [output]
```

**Parameters:**
- `output` (optional) - Output directory (default: `./docs/api`)

**Example:**
```bash
# Generate to default location
nself dev docs generate

# Generate to custom location
nself dev docs generate ./documentation/api
```

**What it generates:**
- GraphQL schema documentation
- Query/mutation reference
- Type documentation
- Example queries
- Authentication guide

#### `docs openapi` - Generate OpenAPI Specification

Generate OpenAPI/Swagger specification for REST endpoints.

```bash
nself dev docs openapi [output]
```

**Parameters:**
- `output` (optional) - Output file path (default: `./docs/api/openapi.yaml`)

**Example:**
```bash
# Generate to default location
nself dev docs openapi

# Generate to custom location
nself dev docs openapi ./api-spec.yaml
```

---

### Testing Tools

#### `test init` - Initialize Test Environment

Set up test environment with fixtures, factories, and snapshots.

```bash
nself dev test init [dir]
```

**Parameters:**
- `dir` (optional) - Test directory (default: `.nself/test`)

**Example:**
```bash
nself dev test init
```

**Creates:**
```
.nself/test/
├── fixtures/       # Test fixture data
├── factories/      # Mock data factories
├── snapshots/      # Database snapshots
└── integration/    # Integration tests
```

#### `test fixtures` - Generate Test Fixtures

Generate test fixture data for entities.

```bash
nself dev test fixtures <entity> [count] [output]
```

**Parameters:**
- `entity` - Entity name (e.g., `users`, `posts`)
- `count` (optional) - Number of fixtures (default: 10)
- `output` (optional) - Output file (default: `.nself/test/fixtures/<entity>.json`)

**Examples:**
```bash
# Generate 10 user fixtures
nself dev test fixtures users

# Generate 50 user fixtures
nself dev test fixtures users 50

# Generate to custom file
nself dev test fixtures users 100 ./test/data/users.json
```

**Output format:**
```json
[
  {
    "id": "uuid-here",
    "email": "user1@example.com",
    "displayName": "User 1",
    "createdAt": "2026-01-30T12:00:00Z"
  }
]
```

#### `test factory` - Generate Mock Data Factory

Generate a mock data factory for an entity.

```bash
nself dev test factory <entity> [output]
```

**Parameters:**
- `entity` - Entity name
- `output` (optional) - Output directory (default: `.nself/test/factories`)

**Examples:**
```bash
# Generate user factory
nself dev test factory users

# Generate to custom location
nself dev test factory users ./test/factories
```

**Generated factory:**
```typescript
// Example TypeScript factory
export const UserFactory = {
  build(overrides = {}) {
    return {
      id: uuid(),
      email: faker.internet.email(),
      displayName: faker.person.fullName(),
      createdAt: new Date(),
      ...overrides
    };
  },

  buildList(count, overrides = {}) {
    return Array.from({ length: count }, () => this.build(overrides));
  }
};
```

#### `test snapshot create` - Create Database Snapshot

Create a database snapshot for testing.

```bash
nself dev test snapshot create <name>
```

**Parameters:**
- `name` - Snapshot name

**Example:**
```bash
# Create baseline snapshot
nself dev test snapshot create baseline

# Create snapshot for specific test
nself dev test snapshot create user-test-state
```

#### `test snapshot restore` - Restore Database Snapshot

Restore database from a snapshot.

```bash
nself dev test snapshot restore <name>
```

**Parameters:**
- `name` - Snapshot name

**Example:**
```bash
# Restore baseline snapshot
nself dev test snapshot restore baseline

# Restore specific test state
nself dev test snapshot restore user-test-state
```

#### `test run` - Run Integration Tests

Run integration tests.

```bash
nself dev test run [dir]
```

**Parameters:**
- `dir` (optional) - Test directory (default: `.nself/test/integration`)

**Example:**
```bash
nself dev test run
```

---

### Mock Data Generation

#### `mock` - Generate Mock Data

Generate mock data and output as JSON.

```bash
nself dev mock <entity> <count>
```

**Parameters:**
- `entity` - Entity type (`users`, `posts`, etc.)
- `count` - Number of records

**Examples:**
```bash
# Generate 100 mock users
nself dev mock users 100

# Generate 50 mock posts
nself dev mock posts 50

# Save to file
nself dev mock users 1000 > users.json
```

**Supported Entities:**
- `users` - User accounts
- `posts` - Blog posts/content
- More coming soon

---

## Options

| Option | Description |
|--------|-------------|
| `-h, --help` | Show help message |
| `-v, --verbose` | Verbose output |
| `--debug` | Debug mode |

---

## Environment Variables

```bash
# Required for SDK generation
HASURA_GRAPHQL_ADMIN_SECRET=your-admin-secret

# Optional
PROJECT_NAME=myapp
BASE_DOMAIN=localhost
```

---

## Complete Examples

### TypeScript SDK Generation Workflow

```bash
# 1. Generate TypeScript SDK
nself dev sdk generate typescript ./frontend/sdk

# 2. Generate API documentation
nself dev docs generate ./frontend/docs

# 3. Install SDK in frontend
cd frontend
npm install ../sdk

# 4. Use in your app
import { createClient } from './sdk';
const client = createClient({ endpoint: 'https://api.yourdomain.com' });
```

### Python SDK Generation Workflow

```bash
# 1. Generate Python SDK
nself dev sdk generate python ./backend/sdk

# 2. Install SDK in backend
cd backend
pip install -e ../sdk

# 3. Use in your app
from sdk import Client
client = Client(endpoint='https://api.yourdomain.com')
```

### Test Environment Setup

```bash
# 1. Initialize test environment
nself dev test init

# 2. Generate test fixtures
nself dev test fixtures users 100
nself dev test fixtures posts 50

# 3. Create mock data factories
nself dev test factory users
nself dev test factory posts

# 4. Create baseline snapshot
nself dev test snapshot create baseline

# 5. Run tests
nself dev test run
```

### Snapshot Testing Workflow

```bash
# 1. Create initial state
nself db schema apply schema.dbml
nself db seed

# 2. Create snapshot
nself dev test snapshot create clean-state

# 3. Run tests (modifies database)
npm test

# 4. Restore to clean state
nself dev test snapshot restore clean-state

# 5. Run next test suite
npm run test:integration
```

---

## SDK Features

### TypeScript SDK

Generated TypeScript SDK includes:
- Full type safety with GraphQL schema types
- Autocomplete support in IDEs
- Query/mutation functions
- Subscription support
- Authentication helpers
- Error handling
- Retry logic
- Cache management

**Example usage:**
```typescript
import { createClient } from './sdk';

const client = createClient({
  endpoint: 'https://api.yourdomain.com',
  headers: {
    'Authorization': 'Bearer token'
  }
});

// Type-safe queries
const users = await client.users.list({ limit: 10 });

// Type-safe mutations
const newUser = await client.users.create({
  email: 'user@example.com',
  displayName: 'John Doe'
});

// Subscriptions
client.users.subscribe(
  { where: { status: 'active' } },
  (data) => console.log(data)
);
```

### Python SDK

Generated Python SDK includes:
- Type hints throughout
- Async/await support
- Query/mutation functions
- Authentication helpers
- Error handling
- Retry logic

**Example usage:**
```python
from sdk import Client

client = Client(
    endpoint='https://api.yourdomain.com',
    auth_token='Bearer token'
)

# Type-hinted queries
users = await client.users.list(limit=10)

# Type-hinted mutations
new_user = await client.users.create(
    email='user@example.com',
    display_name='John Doe'
)
```

---

## Documentation Features

### Generated Documentation Includes

- **Schema Overview:** All types, queries, mutations
- **Type Reference:** Detailed type documentation
- **Query Examples:** Example queries with variables
- **Mutation Examples:** Example mutations with variables
- **Authentication Guide:** How to authenticate requests
- **Error Reference:** Possible errors and handling
- **Pagination Guide:** How to paginate results
- **Filtering Guide:** Available filters and operators

---

## Testing Best Practices

### Snapshot Testing

```bash
# Before each test suite
nself dev test snapshot restore baseline

# Run tests
npm test

# After tests (cleanup)
nself dev test snapshot restore baseline
```

### Fixture Management

```bash
# Generate minimal fixtures for fast tests
nself dev test fixtures users 10

# Generate large dataset for performance tests
nself dev test fixtures users 10000
```

### Factory Patterns

```typescript
// Use factories for flexible test data
const user = UserFactory.build({ email: 'specific@test.com' });
const users = UserFactory.buildList(5, { role: 'admin' });
```

---

## Troubleshooting

### SDK Generation Fails

```bash
# Check Hasura is running
nself status

# Verify admin secret
echo $HASURA_GRAPHQL_ADMIN_SECRET

# Test GraphQL endpoint
curl http://localhost:8080/v1/graphql -H "x-hasura-admin-secret: $HASURA_GRAPHQL_ADMIN_SECRET"
```

### Documentation Not Generated

```bash
# Ensure output directory exists
mkdir -p ./docs/api

# Run with verbose flag
nself dev docs generate --verbose
```

### Snapshot Restore Fails

```bash
# List available snapshots
ls .nself/test/snapshots/

# Verify database is running
nself status

# Try manual restore
nself db restore .nself/test/snapshots/baseline.sql
```

---

## Related Commands

- **[db](DB.md)** - Database management
- **[testing](../testing/README.md)** - Testing utilities
- **[build](BUILD.md)** - Build configuration

---

**Version:** v0.7.0+
**Category:** Developer Tools
**Related Documentation:**
- [Developer Tools Guide](../contributing/DEVELOPMENT.md)
- [Testing Guide](../testing/README.md)
