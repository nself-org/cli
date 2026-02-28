# nself service functions

> **DEPRECATED COMMAND NAME**: This command was formerly `nself functions` in v0.x. It has been consolidated to `nself service functions` in v1.0. The old command name may still work as an alias.

Create and manage serverless functions for your nself project.

## Quick Start

```bash
# Enable functions service
nself service functions enable

# Create a function
nself service functions create hello basic

# Rebuild and start
nself build && nself start

# Test your function
nself service functions test hello
```

## Commands

| Command | Description |
|---------|-------------|
| `nself service functions` | Show status |
| `nself service functions enable` | Enable functions service |
| `nself service functions disable` | Disable functions service |
| `nself service functions list` | List all functions |
| `nself service functions create <name> [template]` | Create a new function |
| `nself service functions delete <name>` | Delete a function |
| `nself service functions test <name> [data]` | Test a function |
| `nself service functions logs [-f]` | View function logs |
| `nself service functions deploy [target]` | Deploy functions |

## Creating Functions

### JavaScript Functions

```bash
# Create with a template
nself service functions create myfunction basic
nself service functions create webhook webhook
nself service functions create api api
nself service functions create cleanup scheduled
```

### TypeScript Functions

Add the `--ts` flag for TypeScript:

```bash
nself service functions create myfunction basic --ts
nself service functions create api api --ts
```

## Templates

### Basic Template

Simple request-response function:

```javascript
async function handler(event, context) {
  return {
    statusCode: 200,
    headers: { 'Content-Type': 'application/json' },
    body: {
      message: 'Function executed successfully',
      timestamp: new Date().toISOString()
    }
  };
}
```

### Webhook Template

Handle incoming webhooks:

```javascript
async function handler(event, context) {
  const { action, data } = event.body || {};

  switch (action) {
    case 'create':
      // Handle create
      break;
    case 'update':
      // Handle update
      break;
    case 'delete':
      // Handle delete
      break;
  }

  return {
    statusCode: 200,
    body: { received: true, action }
  };
}
```

### API Template

Full CRUD endpoint:

```javascript
async function handler(event, context) {
  const { method, query, body } = event;

  switch (method) {
    case 'GET':
      return handleGet(query);
    case 'POST':
      return handlePost(body);
    case 'PUT':
      return handlePut(body);
    case 'DELETE':
      return handleDelete(query);
    default:
      return { statusCode: 405, body: { error: 'Method not allowed' } };
  }
}
```

### Scheduled Template

Background/cron tasks:

```javascript
async function handler(event, context) {
  try {
    await performScheduledTask();
    return {
      statusCode: 200,
      body: { success: true, executedAt: new Date().toISOString() }
    };
  } catch (error) {
    return { statusCode: 500, body: { error: error.message } };
  }
}
```

## Testing Functions

```bash
# Test with default empty body
nself service functions test myfunction

# Test with JSON data
nself service functions test myfunction '{"action":"test","data":{"id":123}}'

# Test via HTTP
curl http://localhost:4300/function/myfunction
curl -X POST http://localhost:4300/function/myfunction \
  -H "Content-Type: application/json" \
  -d '{"action":"test"}'
```

## Deployment

### Local Deployment

Restarts the functions service to pick up changes:

```bash
nself service functions deploy local
```

### Production Deployment

Deploy to a remote server via SSH:

```bash
# Configure in .env
DEPLOY_HOST=user@production-server.com
DEPLOY_PATH=/opt/myapp
DEPLOY_KEY=~/.ssh/deploy_key

# Deploy
nself service functions deploy production
```

### Validate Only

Check functions for errors without deploying:

```bash
nself service functions deploy validate
```

## Directory Structure

```
./functions/
├── hello.js           # Basic function
├── api.js            # API endpoint
├── webhook.ts        # TypeScript webhook
└── scheduled.js      # Scheduled task
```

## Accessing Functions

Functions are accessible at:
- **URL**: `http://localhost:4300/function/{name}`
- **Route**: `https://functions.<your-domain>/function/{name}`

## Event Object

```javascript
{
  method: 'POST',           // HTTP method
  path: '/function/name',   // Request path
  query: URLSearchParams,   // Query parameters
  headers: {},              // Request headers
  body: {}                  // Parsed JSON body
}
```

## Context Object

```javascript
{
  functionName: 'myfunction',
  requestId: 'unique-id',
  // Additional runtime context
}
```

## Response Format

```javascript
{
  statusCode: 200,          // HTTP status code
  headers: {                // Optional headers
    'Content-Type': 'application/json'
  },
  body: {}                  // Response body (auto-serialized)
}
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `FUNCTIONS_ENABLED` | Enable functions service | `false` |
| `FUNCTIONS_PORT` | Functions service port | `4300` |
| `FUNCTIONS_TIMEOUT` | Execution timeout (ms) | `30000` |

## Viewing Logs

```bash
# View recent logs
nself service functions logs

# Follow logs in real-time
nself service functions logs -f
```

## Best Practices

1. **Keep functions small** - Single responsibility
2. **Handle errors gracefully** - Return proper status codes
3. **Validate input** - Check event.body before using
4. **Use TypeScript** - Better type safety and IDE support
5. **Test locally first** - Use `nself service functions test` before deploying
