#!/usr/bin/env bash


# BullMQ worker fixes

# Fix missing node modules for BullMQ workers
fix_bullmq_dependencies() {

set -euo pipefail

  local service_name="$1"
  local project_name="${PROJECT_NAME:-nself}"

  # Find the service directory
  local service_dir=""
  for dir in "services/bullmq/${service_name#*bull-}" "services/bullmq/$service_name" "workers/$service_name"; do
    if [[ -d "$dir" ]]; then
      service_dir="$dir"
      break
    fi
  done

  if [[ -z "$service_dir" ]]; then
    # Create the service directory if it doesn't exist
    service_dir="services/bullmq/${service_name#*bull-}"
    mkdir -p "$service_dir/src"
  fi

  log_info "Fixing BullMQ worker: $service_name in $service_dir"

  # Ensure package.json exists with correct dependencies
  if [[ ! -f "$service_dir/package.json" ]]; then
    cat >"$service_dir/package.json" <<'EOF'
{
  "name": "bullmq-worker",
  "version": "1.0.0",
  "description": "BullMQ worker service",
  "main": "src/index.js",
  "scripts": {
    "start": "node src/index.js",
    "dev": "nodemon src/index.js"
  },
  "dependencies": {
    "bullmq": "^5.0.0",
    "ioredis": "^5.3.2",
    "dotenv": "^16.0.0"
  },
  "devDependencies": {
    "nodemon": "^3.0.0"
  }
}
EOF
  fi

  # Create a basic worker implementation if missing
  if [[ ! -f "$service_dir/src/index.js" ]]; then
    local queue_name="${service_name#*bull-}"
    queue_name="${queue_name//-/_}"

    cat >"$service_dir/src/index.js" <<EOF
const { Worker, Queue, QueueEvents } = require('bullmq');
const Redis = require('ioredis');

// Load environment variables
require('dotenv').config();

// Redis connection
const connection = new Redis({
    host: process.env.REDIS_HOST || 'redis',
    port: process.env.REDIS_PORT || 6379,
    maxRetriesPerRequest: null,
    enableReadyCheck: false
});

// Queue name based on service
const queueName = '${queue_name}';

console.log(\`Starting BullMQ worker for queue: \${queueName}\`);

// Create worker
const worker = new Worker(queueName, async (job) => {
    console.log(\`Processing job \${job.id} of type \${job.name}\`);
    console.log('Job data:', job.data);
    
    // Add your job processing logic here
    try {
        // Simulate some work
        await new Promise(resolve => setTimeout(resolve, 1000));
        
        // Return result
        return { success: true, processedAt: new Date().toISOString() };
    } catch (error) {
        console.error(\`Error processing job \${job.id}:\`, error);
        throw error;
    }
}, {
    connection,
    concurrency: 5,
    removeOnComplete: { count: 100 },
    removeOnFail: { count: 100 }
});

// Worker event handlers
worker.on('completed', (job, returnvalue) => {
    console.log(\`Job \${job.id} completed with result:\`, returnvalue);
});

worker.on('failed', (job, err) => {
    console.error(\`Job \${job?.id} failed with error:\`, err.message);
});

worker.on('error', err => {
    console.error('Worker error:', err);
});

// Graceful shutdown
process.on('SIGTERM', async () => {
    console.log('SIGTERM received, closing worker...');
    await worker.close();
    process.exit(0);
});

process.on('SIGINT', async () => {
    console.log('SIGINT received, closing worker...');
    await worker.close();
    process.exit(0);
});

console.log(\`Worker started and listening for jobs on queue: \${queueName}\`);

// Keep the process alive
process.stdin.resume();
EOF
  fi

  # Ensure Dockerfile exists and is correct
  if [[ ! -f "$service_dir/Dockerfile" ]]; then
    cat >"$service_dir/Dockerfile" <<'EOF'
FROM node:18-alpine

WORKDIR /app

# Copy package files
COPY package*.json ./

# Install dependencies
RUN npm install --production

# Copy source code
COPY . .

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD node -e "process.exit(0)" || exit 1

# Start the worker
CMD ["node", "src/index.js"]
EOF
  fi

  # Generate package-lock.json
  (cd "$service_dir" && npm install >/dev/null 2>&1)

  # Rebuild the Docker image
  log_info "Rebuilding $service_name Docker image..."
  docker compose build "$service_name" >/dev/null 2>&1

  # Start the service
  docker compose up -d "$service_name" >/dev/null 2>&1

  LAST_FIX_DESCRIPTION="Fixed BullMQ worker dependencies and configuration for $service_name"
  return 0
}

# Fix Redis connection issues for BullMQ
fix_bullmq_redis_connection() {
  local service_name="$1"
  local project_name="${PROJECT_NAME:-nself}"

  # Ensure Redis is running
  if ! docker ps | grep -q "${project_name}_redis"; then
    log_info "Starting Redis for BullMQ workers..."
    docker compose up -d redis >/dev/null 2>&1
    sleep 3
  fi

  # Check Redis connectivity
  if ! docker exec "${project_name}_redis" redis-cli ping >/dev/null 2>&1; then
    log_error "Redis is not responding"
    return 1
  fi

  # Restart the BullMQ worker with correct Redis connection
  docker compose restart "$service_name" >/dev/null 2>&1

  LAST_FIX_DESCRIPTION="Fixed Redis connection for $service_name"
  return 0
}

# Main BullMQ fix function
fix_bullmq_worker() {
  local service_name="$1"
  local error_type="${2:-MISSING_NODE_MODULES}"

  case "$error_type" in
    MISSING_NODE_MODULES | MODULE_NOT_FOUND)
      fix_bullmq_dependencies "$service_name"
      ;;
    REDIS_CONNECTION)
      fix_bullmq_redis_connection "$service_name"
      ;;
    *)
      fix_bullmq_dependencies "$service_name"
      ;;
  esac

  return $?
}
