#!/usr/bin/env bash


# dockerfile-generator.sh - Auto-generate missing Dockerfiles for any service

# Source utilities - don't override parent SCRIPT_DIR
DOCKERFILE_GEN_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

source "$DOCKERFILE_GEN_SCRIPT_DIR/../utils/display.sh" 2>/dev/null || true

# Generate appropriate Dockerfile based on service name and context
generate_dockerfile_for_service() {
  local service_name="$1"
  local service_path="${2:-./$service_name}"

  # Silent generation

  # Create directory if it doesn't exist
  mkdir -p "$service_path"

  # Determine service type based on name and generate appropriate files
  case "$service_name" in
    functions)
      generate_functions_service "$service_path"
      ;;
    auth)
      generate_auth_service "$service_path"
      ;;
    storage)
      generate_storage_service "$service_path"
      ;;
    hasura)
      generate_hasura_service "$service_path"
      ;;
    *)
      # Default to a basic Node.js service
      generate_generic_node_service "$service_name" "$service_path"
      ;;
  esac

  return 0
}

# Generate functions service
generate_functions_service() {
  local path="$1"

  cat >"$path/Dockerfile" <<'EOF'
FROM node:18-alpine
# Install health check tools
RUN apk add --no-cache curl wget
WORKDIR /app
COPY package*.json ./
RUN npm install --production
COPY . .
# Use FUNCTIONS_PORT from environment
ARG FUNCTIONS_PORT=4300
ENV FUNCTIONS_PORT=${FUNCTIONS_PORT}
EXPOSE ${FUNCTIONS_PORT}
CMD ["node", "index.js"]
EOF

  cat >"$path/package.json" <<'EOF'
{
  "name": "functions",
  "version": "1.0.0",
  "description": "Serverless functions",
  "scripts": {
    "start": "node index.js",
    "dev": "nodemon index.js"
  },
  "dependencies": {
    "express": "^4.18.0",
    "cors": "^2.8.5"
  },
  "devDependencies": {
    "nodemon": "^3.0.0"
  }
}
EOF

  cat >"$path/index.js" <<'EOF'
const express = require('express');
const cors = require('cors');
const app = express();
const port = process.env.FUNCTIONS_PORT || 3000;

app.use(cors());
app.use(express.json());

app.get('/', (req, res) => {
  res.json({ message: 'Functions service ready', version: '1.0.0' });
});

app.get('/health', (req, res) => {
  res.json({ status: 'ok', service: 'functions' });
});

// Add your serverless functions here
app.post('/functions/:name', async (req, res) => {
  const { name } = req.params;
  res.json({ 
    function: name,
    result: 'Function executed successfully',
    timestamp: new Date()
  });
});

app.listen(port, () => {
  console.log(`Functions service listening on port ${port}`);
});
EOF

  # Successfully generated
}

# Generate auth service placeholder
generate_auth_service() {
  local path="$1"

  # Auth is usually hasura-auth, just create a marker
  cat >"$path/Dockerfile" <<'EOF'
# Auth service is provided by hasura-auth image
# This is a placeholder for docker-compose compatibility
FROM busybox:latest
CMD ["echo", "Auth service uses hasura-auth image"]
EOF

  # Successfully generated
}

# Generate storage service placeholder
generate_storage_service() {
  local path="$1"

  # Storage is usually hasura-storage, just create a marker
  cat >"$path/Dockerfile" <<'EOF'
# Storage service is provided by hasura-storage image
# This is a placeholder for docker-compose compatibility
FROM busybox:latest
CMD ["echo", "Storage service uses hasura-storage image"]
EOF

  # Successfully generated
}

# Generate hasura service placeholder
generate_hasura_service() {
  local path="$1"

  # Hasura uses official image, just create a marker
  cat >"$path/Dockerfile" <<'EOF'
# Hasura service is provided by hasura/graphql-engine image
# This is a placeholder for docker-compose compatibility
FROM busybox:latest
CMD ["echo", "Hasura service uses official hasura image"]
EOF

  # Successfully generated
}

# Generate generic Node.js service
generate_generic_node_service() {
  local service_name="$1"
  local path="$2"

  cat >"$path/Dockerfile" <<'EOF'
FROM node:18-alpine
# Install health check tools
RUN apk add --no-cache curl wget
WORKDIR /app
COPY package*.json ./
RUN npm install --production
COPY . .
EXPOSE 3000
CMD ["node", "index.js"]
EOF

  cat >"$path/package.json" <<EOF
{
  "name": "$service_name",
  "version": "1.0.0",
  "description": "$service_name service",
  "scripts": {
    "start": "node index.js"
  },
  "dependencies": {
    "express": "^4.18.0"
  }
}
EOF

  cat >"$path/index.js" <<EOF
const express = require('express');
const app = express();
const port = process.env.PORT || 3000;

app.get('/', (req, res) => {
  res.json({ message: '${service_name} service ready' });
});

app.get('/health', (req, res) => {
  res.json({ status: 'ok', service: '${service_name}' });
});

app.listen(port, () => {
  console.log('${service_name} service listening on port ' + port);
});
EOF

  # Successfully generated
}

# Export functions
export -f generate_dockerfile_for_service
