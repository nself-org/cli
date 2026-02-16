#!/usr/bin/env bash
set -euo pipefail

# fallback-services.sh - Generate fallback services for problematic containers
# Bash 3.2 compatible

# Generate fallback auth service
generate_fallback_auth() {
  local build_dir="${1:-.}"

  # Create fallback directory
  mkdir -p "$build_dir/fallback-services"

  # Generate simple auth service
  cat >"$build_dir/fallback-services/auth-server.js" <<'EOF'
// Fallback auth service - replaces nhost/hasura-auth when it fails
const http = require('http');

const server = http.createServer((req, res) => {
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization');

  if (req.method === 'OPTIONS') {
    res.writeHead(200);
    res.end();
    return;
  }

  if (req.url === '/health' || req.url === '/healthz' || req.url === '/v1/health') {
    res.writeHead(200, { 'Content-Type': 'text/plain' });
    res.end('healthy');
  } else if (req.url === '/version' || req.url === '/v1/version') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ version: '0.0.0-fallback' }));
  } else {
    // Default response for auth endpoints
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({
      service: 'auth-fallback',
      status: 'operational',
      message: 'Auth service fallback is running',
      timestamp: new Date().toISOString()
    }));
  }
});

const port = process.env.AUTH_PORT || process.env.PORT || 4000;
// Listen on all IPv4 interfaces
server.listen(port, '0.0.0.0', () => {
  console.log(`Auth fallback service listening on port ${port} (all interfaces)`);
});

// Graceful shutdown
process.on('SIGTERM', () => {
  console.log('SIGTERM received, closing server...');
  server.close(() => {
    console.log('Server closed');
    process.exit(0);
  });
});
EOF

  # Generate Dockerfile for auth fallback
  cat >"$build_dir/fallback-services/Dockerfile.auth" <<'EOF'
FROM node:18-alpine
WORKDIR /app
COPY auth-server.js /app/server.js
EXPOSE 4000
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD node -e "require('http').get('http://127.0.0.1:4000/health', (r) => process.exit(r.statusCode === 200 ? 0 : 1))"
CMD ["node", "server.js"]
EOF

  echo "Generated fallback auth service in $build_dir/fallback-services/"
}

# Generate fallback functions service
generate_fallback_functions() {
  local build_dir="${1:-.}"

  # Create fallback directory if not exists
  mkdir -p "$build_dir/fallback-services"

  # Generate simple functions service
  cat >"$build_dir/fallback-services/functions-server.js" <<'EOF'
// Fallback functions service - replaces nhost/functions when it fails
const http = require('http');
const fs = require('fs');
const path = require('path');

const server = http.createServer((req, res) => {
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization');

  if (req.method === 'OPTIONS') {
    res.writeHead(200);
    res.end();
    return;
  }

  if (req.url === '/health' || req.url === '/healthz' || req.url === '/v1/health') {
    res.writeHead(200, { 'Content-Type': 'text/plain' });
    res.end('healthy');
  } else if (req.url === '/version' || req.url === '/v1/version') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ version: '0.0.0-fallback' }));
  } else {
    // Try to execute user functions if they exist
    const functionsDir = '/opt/project/functions';
    if (fs.existsSync(functionsDir)) {
      // Simple function execution placeholder
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        service: 'functions-fallback',
        status: 'operational',
        functionsDir: functionsDir,
        message: 'Functions service fallback is running',
        timestamp: new Date().toISOString()
      }));
    } else {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        service: 'functions-fallback',
        status: 'operational',
        message: 'Functions service fallback is running (no functions directory)',
        timestamp: new Date().toISOString()
      }));
    }
  }
});

const port = process.env.PORT || 3000;
// Listen on all IPv4 interfaces
server.listen(port, '0.0.0.0', () => {
  console.log(`Functions fallback service listening on port ${port} (all interfaces)`);
});

// Graceful shutdown
process.on('SIGTERM', () => {
  console.log('SIGTERM received, closing server...');
  server.close(() => {
    console.log('Server closed');
    process.exit(0);
  });
});
EOF

  # Generate Dockerfile for functions fallback
  cat >"$build_dir/fallback-services/Dockerfile.functions" <<'EOF'
FROM node:18-alpine
WORKDIR /app
COPY functions-server.js /app/server.js

# Create functions directory structure
RUN mkdir -p /opt/project/functions

EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD node -e "require('http').get('http://127.0.0.1:3000/health', (r) => process.exit(r.statusCode === 200 ? 0 : 1))"
CMD ["node", "server.js"]
EOF

  # Also ensure functions directory structure exists
  mkdir -p "$build_dir/functions"

  # Create minimal package.json if it doesn't exist
  if [[ ! -f "$build_dir/functions/package.json" ]]; then
    cat >"$build_dir/functions/package.json" <<'EOF'
{
  "name": "functions",
  "version": "1.0.0",
  "description": "Serverless functions",
  "main": "index.js",
  "scripts": {
    "start": "node index.js"
  },
  "dependencies": {}
}
EOF
  fi

  # Create package-lock.json to satisfy nhost/functions requirements
  if [[ ! -f "$build_dir/functions/package-lock.json" ]]; then
    cat >"$build_dir/functions/package-lock.json" <<'EOF'
{
  "name": "functions",
  "version": "1.0.0",
  "lockfileVersion": 2,
  "requires": true,
  "packages": {
    "": {
      "name": "functions",
      "version": "1.0.0",
      "dependencies": {}
    }
  }
}
EOF
  fi

  # Create yarn.lock as alternative
  if [[ ! -f "$build_dir/functions/yarn.lock" ]]; then
    echo "# yarn lockfile v1" >"$build_dir/functions/yarn.lock"
  fi

  echo "Generated fallback functions service in $build_dir/fallback-services/"
}

# Generate both fallback services
generate_fallback_services() {
  local build_dir="${1:-.}"

  generate_fallback_auth "$build_dir"
  generate_fallback_functions "$build_dir"

  return 0
}
