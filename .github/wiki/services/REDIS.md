# Redis - Cache and Sessions Service

## Overview

Redis is an in-memory data structure store used within nself as the primary caching layer, session backend, and message broker. When enabled, Redis integrates with the Auth service for session management, supports rate limiting for API endpoints, provides pub/sub messaging between services, and serves as the backbone for BullMQ job queues in custom services.

Redis is one of the 7 optional services in nself. It must be explicitly enabled with `REDIS_ENABLED=true` in your `.env` file. Once enabled, `nself build` generates the appropriate Docker Compose configuration and `nself start` brings it online alongside your other services.

## Features

### Current Capabilities

- **Session Storage** - Persistent session backend for the nself Auth service
- **Application Caching** - Key-value cache for custom services and Hasura query results
- **Rate Limiting** - Token bucket and sliding window rate limiting for API endpoints
- **Pub/Sub Messaging** - Real-time message passing between services
- **Job Queues** - BullMQ-compatible queue backend for background task processing
- **Data Structures** - Lists, sets, sorted sets, hashes, streams, and HyperLogLog
- **Persistence** - Optional RDB snapshots and AOF logging for data durability
- **Cluster-Ready** - Single-node by default, with support for Redis Sentinel in production

### Integration Points

Redis integrates with the following nself services:

| Service | Integration | Purpose |
|---------|------------|---------|
| Auth | Session store | JWT refresh tokens, session persistence |
| Hasura | Cache backend | Query result caching via custom actions |
| Custom Services (CS_N) | Direct connection | Application-level caching and messaging |
| BullMQ Workers | Job queue | Background job processing and scheduling |
| Monitoring | Redis Exporter | Metrics collection via Prometheus |
| Rate Limiting | Token storage | API rate limit counters and windows |

## Configuration

### Basic Setup

Enable Redis in your `.env` file:

```bash
# Redis Configuration
REDIS_ENABLED=true
```

That is all that is required. nself provides sensible defaults for all other settings.

### Complete Configuration Reference

```bash
# Required
REDIS_ENABLED=true

# Version (default: 7-alpine)
REDIS_VERSION=7-alpine

# Port Configuration
REDIS_PORT=6379                    # External host port (default: 6379)
REDIS_INTERNAL_PORT=6379           # Internal container port (do not change)

# Authentication
REDIS_PASSWORD=your-secure-password  # Auto-generated if not set

# Memory Management
REDIS_MAXMEMORY=256mb              # Maximum memory allocation (default: 256mb)
REDIS_MAXMEMORY_POLICY=allkeys-lru # Eviction policy (default: allkeys-lru)

# Persistence
REDIS_PERSISTENCE=true             # Enable data persistence (default: true)
REDIS_AOF_ENABLED=true             # Append-only file logging (default: true)
REDIS_RDB_ENABLED=true             # RDB snapshot saving (default: true)
REDIS_RDB_FREQUENCY=900            # RDB save interval in seconds (default: 900)

# Connection Limits
REDIS_MAX_CLIENTS=10000            # Maximum simultaneous connections (default: 10000)
REDIS_TIMEOUT=300                  # Client idle timeout in seconds (default: 300)
REDIS_TCP_KEEPALIVE=60             # TCP keepalive interval (default: 60)

# Logging
REDIS_LOG_LEVEL=notice             # Options: debug, verbose, notice, warning
```

### Environment-Specific Configurations

#### Development

```bash
REDIS_ENABLED=true
REDIS_MAXMEMORY=128mb
REDIS_PERSISTENCE=false
REDIS_LOG_LEVEL=verbose
```

#### Staging

```bash
REDIS_ENABLED=true
REDIS_MAXMEMORY=512mb
REDIS_PERSISTENCE=true
REDIS_PASSWORD=staging-redis-password
```

#### Production

```bash
REDIS_ENABLED=true
REDIS_MAXMEMORY=2gb
REDIS_PERSISTENCE=true
REDIS_AOF_ENABLED=true
REDIS_RDB_ENABLED=true
REDIS_PASSWORD=strong-production-password
REDIS_MAX_CLIENTS=50000
REDIS_LOG_LEVEL=warning
```

### Memory Eviction Policies

| Policy | Description | Recommended For |
|--------|-------------|-----------------|
| `allkeys-lru` | Evict least recently used keys (default) | General caching |
| `volatile-lru` | Evict LRU keys with TTL set | Mixed cache + persistent data |
| `allkeys-lfu` | Evict least frequently used keys | Stable access patterns |
| `volatile-ttl` | Evict keys with shortest TTL | Time-sensitive data |
| `noeviction` | Return errors when memory full | Critical data only |

## Usage

### CLI Commands

Redis is managed through the `nself service redis` command group:

```bash
# Check Redis status
nself service redis status

# View Redis connection info
nself service redis info

# Open Redis CLI inside the container
nself service redis cli

# Flush all cached data (development only)
nself service redis flush

# View Redis configuration
nself service redis config

# Monitor real-time Redis commands
nself service redis monitor

# Check memory usage
nself service redis memory

# View slow query log
nself service redis slowlog
```

### General Service Commands

```bash
# View Redis logs
nself logs redis

# Execute a command inside the Redis container
nself exec redis redis-cli PING

# Restart Redis
nself restart redis

# Check all service URLs (Redis has no web UI)
nself urls
```

### Connecting from Custom Services

Redis is accessible within the Docker network at `redis:6379`. Use the following connection patterns in your custom services:

#### Node.js (ioredis)

```javascript
import Redis from 'ioredis';

const redis = new Redis({
  host: process.env.REDIS_HOST || 'redis',
  port: parseInt(process.env.REDIS_PORT || '6379'),
  password: process.env.REDIS_PASSWORD,
  maxRetriesPerRequest: 3,
  retryStrategy(times) {
    return Math.min(times * 50, 2000);
  }
});

// Caching example
await redis.set('user:123', JSON.stringify(userData), 'EX', 3600);
const cached = await redis.get('user:123');

// Pub/Sub example
const subscriber = redis.duplicate();
subscriber.subscribe('events');
subscriber.on('message', (channel, message) => {
  console.log(`Received: ${message} on ${channel}`);
});
```

#### Python

```python
import redis
import os

r = redis.Redis(
    host=os.environ.get('REDIS_HOST', 'redis'),
    port=int(os.environ.get('REDIS_PORT', 6379)),
    password=os.environ.get('REDIS_PASSWORD'),
    decode_responses=True
)

# Caching example
r.setex('user:123', 3600, json.dumps(user_data))
cached = r.get('user:123')

# Pub/Sub example
pubsub = r.pubsub()
pubsub.subscribe('events')
for message in pubsub.listen():
    if message['type'] == 'message':
        print(f"Received: {message['data']}")
```

#### Go

```go
import "github.com/redis/go-redis/v9"

rdb := redis.NewClient(&redis.Options{
    Addr:     os.Getenv("REDIS_HOST") + ":" + os.Getenv("REDIS_PORT"),
    Password: os.Getenv("REDIS_PASSWORD"),
    DB:       0,
})

// Caching example
err := rdb.Set(ctx, "user:123", userData, time.Hour).Err()
val, err := rdb.Get(ctx, "user:123").Result()
```

### BullMQ Job Queue Integration

Redis serves as the backend for BullMQ job queues in custom services:

```javascript
import { Queue, Worker } from 'bullmq';

const connection = {
  host: process.env.REDIS_HOST || 'redis',
  port: parseInt(process.env.REDIS_PORT || '6379'),
  password: process.env.REDIS_PASSWORD,
};

// Producer
const emailQueue = new Queue('email', { connection });
await emailQueue.add('send-welcome', { userId: 123 });

// Worker
const worker = new Worker('email', async (job) => {
  await sendWelcomeEmail(job.data.userId);
}, { connection });
```

### Rate Limiting Integration

Redis supports the nself rate limiting system configured through the Auth service:

```bash
# Enable rate limiting in .env
AUTH_RATE_LIMIT_ENABLED=true
AUTH_RATE_LIMIT_WINDOW=60          # Window in seconds
AUTH_RATE_LIMIT_MAX_REQUESTS=100   # Max requests per window
AUTH_RATE_LIMIT_STORE=redis        # Use Redis as backend
```

## Network and Routing

Redis does not expose a public web interface. It is accessible only within the Docker network and optionally on the host machine:

| Access Point | Address | Purpose |
|-------------|---------|---------|
| Internal (Docker) | `redis:6379` | Service-to-service communication |
| External (Host) | `localhost:6379` | Local development access |

Redis is intentionally not routed through nginx. Direct TCP connections provide the lowest latency for cache operations.

## Resource Requirements

| Resource | Minimum | Recommended | Production |
|----------|---------|-------------|------------|
| CPU | 0.1 cores | 0.25 cores | 0.5-1 core |
| Memory | 50MB | 256MB | 1-4GB |
| Storage | 100MB | 500MB | 2-10GB |
| Network | Minimal | Low | Medium |

Memory usage scales directly with the amount of cached data. The `REDIS_MAXMEMORY` setting prevents Redis from consuming more than the allocated amount.

## Monitoring

When the monitoring bundle is enabled (`MONITORING_ENABLED=true`), the Redis Exporter automatically collects metrics from Redis and exposes them to Prometheus.

### Available Metrics

- `redis_connected_clients` - Number of active client connections
- `redis_used_memory_bytes` - Current memory consumption
- `redis_commands_processed_total` - Total commands executed
- `redis_keyspace_hits_total` - Cache hit count
- `redis_keyspace_misses_total` - Cache miss count
- `redis_expired_keys_total` - Keys removed by TTL expiration
- `redis_evicted_keys_total` - Keys removed by memory eviction
- `redis_connected_slaves` - Number of connected replicas

### Grafana Dashboard

A pre-configured Redis dashboard is available in Grafana when monitoring is enabled:

```bash
# Access Grafana
# URL: https://grafana.local.nself.org
# Navigate to: Dashboards > Redis Overview
```

### Health Checks

```bash
# Check Redis health
nself health redis

# Docker health check (built into compose config)
# Uses: redis-cli ping
# Interval: 10s
# Timeout: 5s
# Retries: 3
```

## Security

### Authentication

Redis requires password authentication when `REDIS_PASSWORD` is set. If no password is provided, nself auto-generates a secure password during `nself build` and stores it in the `.env` file.

### Network Isolation

- Redis is only accessible within the Docker network by default
- External access on the host port can be disabled by setting `REDIS_PORT=0`
- Never expose Redis directly to the public internet

### Best Practices

1. Always use a strong password in staging and production environments
2. Set `REDIS_MAXMEMORY` to prevent unbounded memory growth
3. Use TLS for Redis connections in production (configure via `REDIS_TLS_ENABLED=true`)
4. Disable persistence in development if data durability is not needed
5. Regularly monitor memory usage through the monitoring dashboard
6. Use key prefixes to namespace data from different services

## Troubleshooting

### Redis not starting

```bash
# Check Redis logs for errors
nself logs redis

# Verify Redis is enabled in .env
grep REDIS_ENABLED .env

# Check for port conflicts
lsof -i :6379

# Run diagnostics
nself doctor
```

### Connection refused from custom services

```bash
# Verify Redis is running
nself service redis status

# Test connectivity from inside the Docker network
nself exec redis redis-cli PING
# Expected output: PONG

# Check the service is on the correct Docker network
docker network inspect ${PROJECT_NAME}_default
```

### High memory usage

```bash
# Check current memory consumption
nself service redis memory

# View key count and database sizes
nself exec redis redis-cli INFO keyspace

# Identify large keys
nself exec redis redis-cli --bigkeys

# Flush non-critical data if needed (development only)
nself service redis flush
```

### Persistence errors

```bash
# Check disk space
df -h

# Verify Redis data volume exists
docker volume ls | grep redis

# Check RDB/AOF file status
nself exec redis redis-cli LASTSAVE
nself exec redis redis-cli INFO persistence
```

### Slow performance

```bash
# Check slow query log
nself service redis slowlog

# Monitor commands in real-time
nself service redis monitor

# Review client connections
nself exec redis redis-cli CLIENT LIST
```

## Data Persistence

Redis data is stored in a Docker volume named `${PROJECT_NAME}_redis_data`. This volume persists across container restarts and rebuilds.

### Backup

```bash
# Trigger an RDB snapshot
nself exec redis redis-cli BGSAVE

# Include Redis in full backup
nself backup create --include redis
```

### Restore

```bash
# Restore from a backup
nself backup restore --include redis --from backup-2026-01-15.tar.gz
```

## Related Documentation

- [Optional Services Overview](SERVICES_OPTIONAL.md) - All optional services
- [Services Overview](SERVICES.md) - Complete service listing
- [Monitoring Bundle](MONITORING-BUNDLE.md) - Redis Exporter and metrics
- [Environment Variables](../configuration/ENVIRONMENT-VARIABLES.md) - Full configuration reference
- [Auth Documentation](../commands/AUTH.md) - Session and rate limiting configuration
- [Custom Services](SERVICES_CUSTOM.md) - Using Redis in custom services
- [Troubleshooting](../troubleshooting/README.md) - Common issues and solutions
