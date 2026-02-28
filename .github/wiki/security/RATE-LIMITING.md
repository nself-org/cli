# Rate Limiting & DDoS Protection

Comprehensive guide to rate limiting, DDoS protection, and abuse prevention in nself.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Rate Limit Zones](#rate-limit-zones)
- [DDoS Protection Layers](#ddos-protection-layers)
- [CLI Commands](#cli-commands)
- [Monitoring & Alerts](#monitoring--alerts)
- [Whitelist & Blacklist](#whitelist--blacklist)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Overview

nself provides comprehensive rate limiting and DDoS protection out of the box using nginx's powerful rate limiting capabilities combined with PostgreSQL-based tracking and monitoring.

### Features

- **Multi-zone rate limiting** - Different limits for different endpoint types
- **Token bucket algorithm** - Smooth rate limiting with burst handling
- **Connection limits** - Prevent connection exhaustion
- **Request size limits** - Protect against large payload attacks
- **Timeout protection** - Defend against slowloris and slow POST attacks
- **IP whitelist/blacklist** - Bypass or block specific IPs
- **Real-time monitoring** - Track violations and identify attackers
- **Automatic alerting** - Detect suspicious patterns
- **Grafana dashboards** - Visualize rate limit metrics

## Quick Start

### 1. Initialize Rate Limiting

```bash
# Initialize rate limiting database tables
nself auth rate-limit init
```

### 2. Check Current Configuration

```bash
# View current rate limits
nself auth rate-limit list

# Check rate limiting status
nself auth rate-limit status
```

### 3. Customize Rate Limits

```bash
# Set GraphQL API limit to 200 requests per minute
nself auth rate-limit set graphql_api 200r/m

# Set auth endpoint limit to 5 requests per minute (stricter)
nself auth rate-limit set auth 5r/m

# Apply changes
nself build
nself restart nginx
```

### 4. Monitor Violations

```bash
# View recent violations
nself auth rate-limit violations

# Check for alerts
nself auth rate-limit alerts

# Analyze nginx logs
nself auth rate-limit analyze
```

## Configuration

### Environment Variables

Configure rate limits in your `.env` file:

```bash
# Rate Limit Configuration
RATE_LIMIT_GENERAL_RATE=10r/s          # General API endpoints
RATE_LIMIT_GRAPHQL_RATE=100r/m         # GraphQL API (Hasura)
RATE_LIMIT_AUTH_RATE=10r/m             # Authentication endpoints
RATE_LIMIT_UPLOAD_RATE=5r/m            # File uploads
RATE_LIMIT_STATIC_RATE=1000r/m         # Static assets (CSS/JS/images)
RATE_LIMIT_WEBHOOK_RATE=30r/m          # Webhooks
RATE_LIMIT_FUNCTIONS_RATE=50r/m        # Serverless functions
RATE_LIMIT_USER_RATE=1000r/m           # Per-user (authenticated requests)

# Connection Limits
CLIENT_MAX_BODY_SIZE=10M               # Max request body size
CLIENT_HEADER_BUFFER_SIZE=1k           # Header buffer size
LARGE_CLIENT_HEADER_BUFFERS=8k         # Large header buffer
CLIENT_BODY_BUFFER_SIZE=16k            # Body buffer size

# Timeout Protection
CLIENT_BODY_TIMEOUT=12s                # Body read timeout
CLIENT_HEADER_TIMEOUT=12s              # Header read timeout
KEEPALIVE_TIMEOUT=15s                  # Keep-alive timeout
SEND_TIMEOUT=10s                       # Send timeout
```

### Rate Format

- `r/s` - Requests per second (e.g., `10r/s` = 10 requests per second)
- `r/m` - Requests per minute (e.g., `100r/m` = 100 requests per minute)

## Rate Limit Zones

nself implements 8 different rate limiting zones:

### 1. General API Zone

**Default**: 10 requests/second
**Purpose**: Default rate limit for all API endpoints
**Variable**: `RATE_LIMIT_GENERAL_RATE`

```bash
nself auth rate-limit set general 20r/s
```

### 2. GraphQL API Zone

**Default**: 100 requests/minute
**Purpose**: Hasura GraphQL endpoint
**Variable**: `RATE_LIMIT_GRAPHQL_RATE`
**Burst**: 20 requests

Applied to: `api.${BASE_DOMAIN}`

```bash
nself auth rate-limit set graphql_api 200r/m
```

### 3. Authentication Zone

**Default**: 10 requests/minute
**Purpose**: Login, signup, password reset (prevents brute force)
**Variable**: `RATE_LIMIT_AUTH_RATE`
**Burst**: 5 requests

Applied to: `auth.${BASE_DOMAIN}`

```bash
nself auth rate-limit set auth 5r/m
```

### 4. Upload Zone

**Default**: 5 requests/minute
**Purpose**: File upload endpoints (prevents storage abuse)
**Variable**: `RATE_LIMIT_UPLOAD_RATE`
**Burst**: 2 requests

Applied to: MinIO S3 API (`storage.${BASE_DOMAIN}`)

```bash
nself auth rate-limit set uploads 10r/m
```

### 5. Static Assets Zone

**Default**: 1000 requests/minute
**Purpose**: CSS, JavaScript, images (high throughput)
**Variable**: `RATE_LIMIT_STATIC_RATE`
**Burst**: 50 requests

Applied to: Static file routes

```bash
nself auth rate-limit set static 2000r/m
```

### 6. Webhook Zone

**Default**: 30 requests/minute
**Purpose**: Incoming webhooks
**Variable**: `RATE_LIMIT_WEBHOOK_RATE`
**Burst**: 10 requests

Applied to: `webhooks.${BASE_DOMAIN}`

```bash
nself auth rate-limit set webhooks 50r/m
```

### 7. Functions Zone

**Default**: 50 requests/minute
**Purpose**: Serverless function invocations
**Variable**: `RATE_LIMIT_FUNCTIONS_RATE`
**Burst**: 15 requests

Applied to: `functions.${BASE_DOMAIN}`

```bash
nself auth rate-limit set functions 100r/m
```

### 8. User API Zone

**Default**: 1000 requests/minute
**Purpose**: Per-user rate limiting (requires authentication)
**Variable**: `RATE_LIMIT_USER_RATE`

Based on `Authorization` header instead of IP address.

```bash
nself auth rate-limit set user_api 2000r/m
```

## DDoS Protection Layers

nself implements a multi-layer DDoS protection strategy:

### Layer 1: Connection Limiting

Prevents connection exhaustion attacks by limiting concurrent connections.

**Configuration**:
- `limit_conn_per_ip`: Max concurrent connections per IP (default: 10)
- `limit_conn_server`: Max total server connections

**Applied to**:
- GraphQL API: 10 connections per IP
- Auth endpoints: 5 connections per IP
- Uploads: 5 connections per IP
- Webhooks: 10 connections per IP
- Functions: 10 connections per IP

### Layer 2: Request Size Limits

Protects against memory exhaustion from large payloads.

**Configuration**:
```bash
CLIENT_MAX_BODY_SIZE=10M               # Default body size
CLIENT_HEADER_BUFFER_SIZE=1k           # Header buffer
LARGE_CLIENT_HEADER_BUFFERS=8k         # Large headers
CLIENT_BODY_BUFFER_SIZE=16k            # Body buffer
```

**Per-endpoint overrides**:
- Storage/uploads: 1000M (1GB)
- Webhooks: 10M
- General API: 10M (default)

### Layer 3: Timeout Protection

Defends against slowloris and slow POST attacks.

**Configuration**:
```bash
CLIENT_BODY_TIMEOUT=12s                # Slow POST protection
CLIENT_HEADER_TIMEOUT=12s              # Slowloris protection
KEEPALIVE_TIMEOUT=15s                  # Connection timeout
SEND_TIMEOUT=10s                       # Response timeout
```

### Layer 4: Rate Limiting (Token Bucket)

Smooths request rates and prevents burst attacks.

- Each zone has independent token bucket
- Burst handling allows temporary spikes
- Refill rate maintains sustainable throughput

## CLI Commands

### View Configuration

```bash
# List all rate limit zones and their settings
nself auth rate-limit list

# Check rate limiting status
nself auth rate-limit status
```

### Modify Rate Limits

```bash
# Set rate limit for a zone
nself auth rate-limit set <zone> <rate>

# Examples
nself auth rate-limit set graphql_api 200r/m
nself auth rate-limit set auth 5r/m
nself auth rate-limit set uploads 10r/m

# Apply changes
nself build && nself restart nginx
```

### Reset Rate Limits

```bash
# Reset rate limits for a specific IP
nself auth rate-limit reset 192.168.1.100
```

### Monitoring

```bash
# View violations in last 24 hours
nself auth rate-limit violations

# View violations in last 6 hours
nself auth rate-limit violations 6

# Check for suspicious patterns
nself auth rate-limit alerts

# Analyze nginx error logs
nself auth rate-limit analyze
```

### Whitelist Management

```bash
# Add IP to whitelist (bypass rate limits)
nself auth rate-limit whitelist add 10.0.0.1 "Internal server"

# List whitelisted IPs
nself auth rate-limit whitelist list

# Remove from whitelist
nself auth rate-limit whitelist remove 10.0.0.1

# Apply changes
nself build && nself restart nginx
```

### Blacklist Management

```bash
# Block IP permanently
nself auth rate-limit block add 1.2.3.4 "Brute force attack"

# Block IP for 1 hour (3600 seconds)
nself auth rate-limit block add 1.2.3.4 "Suspicious activity" 3600

# Remove block
nself auth rate-limit block remove 1.2.3.4

# Apply changes
nself build && nself restart nginx
```

## Monitoring & Alerts

### Violation Tracking

All rate limit decisions are logged to PostgreSQL:

```sql
-- View recent violations
SELECT * FROM rate_limit.log
WHERE allowed = false
ORDER BY requested_at DESC
LIMIT 100;

-- Top violating IPs
SELECT
  SPLIT_PART(key, ':', 2) as ip,
  COUNT(*) as violations
FROM rate_limit.log
WHERE allowed = false
  AND requested_at >= NOW() - INTERVAL '1 hour'
GROUP BY ip
ORDER BY violations DESC;
```

### Automatic Alerts

The system checks for:

1. **High violators**: IPs with >100 violations/hour
2. **Spike detection**: 5x increase in violations
3. **Potential DDoS**: >10,000 requests in 5 minutes

```bash
# Check for alerts
nself auth rate-limit alerts
```

### Grafana Dashboard

If monitoring is enabled (`MONITORING_ENABLED=true`), rate limiting metrics are available in Grafana:

- Request rate over time
- Violation rate trend
- Top violating IPs
- Active rate limit buckets

Access: `https://grafana.${BASE_DOMAIN}`

### Prometheus Metrics

Metrics exported for Prometheus:

- `nself_rate_limit_requests_total` - Total tracked requests
- `nself_rate_limit_violations_total` - Total violations
- `nself_rate_limit_violation_rate` - Violation rate (0-1)
- `nself_rate_limit_active_buckets` - Number of active buckets

## Whitelist & Blacklist

### IP Whitelist

Trusted IPs bypass all rate limiting.

**Use cases**:
- Internal servers
- Monitoring services
- Trusted partners
- CI/CD systems

**Storage**: `rate_limit.whitelist` table in PostgreSQL

```bash
# Add internal server
nself auth rate-limit whitelist add 10.0.0.1 "Internal API server"

# Add monitoring
nself auth rate-limit whitelist add 203.0.113.50 "UptimeRobot"
```

### IP Blacklist

Blocked IPs are denied all access.

**Use cases**:
- Known attackers
- Abusive bots
- Brute force attempts
- DDoS sources

**Storage**: `rate_limit.blacklist` table in PostgreSQL

**Features**:
- Temporary or permanent blocks
- Auto-expiry support
- Reason tracking

```bash
# Permanent block
nself auth rate-limit block add 198.51.100.10 "DDoS attack"

# Temporary block (1 hour)
nself auth rate-limit block add 198.51.100.20 "Brute force" 3600
```

## Best Practices

### Development

```bash
# Lenient limits for local development
RATE_LIMIT_GENERAL_RATE=100r/s
RATE_LIMIT_GRAPHQL_RATE=1000r/m
RATE_LIMIT_AUTH_RATE=50r/m
```

### Staging

```bash
# Production-like limits for realistic testing
RATE_LIMIT_GENERAL_RATE=20r/s
RATE_LIMIT_GRAPHQL_RATE=200r/m
RATE_LIMIT_AUTH_RATE=15r/m
```

### Production

```bash
# Strict limits for security
RATE_LIMIT_GENERAL_RATE=10r/s
RATE_LIMIT_GRAPHQL_RATE=100r/m
RATE_LIMIT_AUTH_RATE=10r/m       # Prevent brute force
RATE_LIMIT_UPLOAD_RATE=5r/m      # Prevent storage abuse
```

### Security Recommendations

1. **Enable rate limiting in production**
   ```bash
   # Verify in .env.prod
   nself doctor
   ```

2. **Monitor violations regularly**
   ```bash
   # Daily check
   nself auth rate-limit violations 24
   ```

3. **Set up alerts**
   ```bash
   # Add to cron (every hour)
   0 * * * * nself auth rate-limit alerts
   ```

4. **Whitelist known good IPs**
   ```bash
   # Internal infrastructure
   nself auth rate-limit whitelist add 10.0.0.0/8 "Internal network"
   ```

5. **Block repeat offenders**
   ```bash
   # Check top violators
   nself auth rate-limit violations

   # Block if needed
   nself auth rate-limit block add <ip> "Repeated violations"
   ```

6. **Customize per environment**
   ```bash
   # .env.dev - lenient
   # .env.staging - realistic
   # .env.prod - strict
   ```

### Performance Tips

1. **Burst handling**: Allow temporary spikes while preventing sustained abuse
2. **Token buckets**: More efficient than fixed windows
3. **Database cleanup**: Regularly clean old logs
   ```sql
   DELETE FROM rate_limit.log
   WHERE requested_at < NOW() - INTERVAL '7 days';
   ```

## Troubleshooting

### "Rate limiting not configured" Warning

```bash
# Generate configuration
nself build

# Restart nginx
nself restart nginx
```

### High Violation Count

```bash
# Identify top violators
nself auth rate-limit violations

# Check if legitimate traffic
# If attack, block the IP
nself auth rate-limit block add <ip> "Abuse"

# If legitimate, increase limits or whitelist
nself auth rate-limit set <zone> <higher-rate>
# OR
nself auth rate-limit whitelist add <ip> "Legitimate high traffic"
```

### Legitimate Users Being Rate Limited

```bash
# Option 1: Increase zone limit
nself auth rate-limit set graphql_api 500r/m

# Option 2: Whitelist the IP
nself auth rate-limit whitelist add <ip> "High-volume user"

# Option 3: Reset their limits
nself auth rate-limit reset <ip>
```

### Changes Not Applied

```bash
# Rebuild and restart
nself build
nself restart nginx

# Verify
nself auth rate-limit status
```

### Database Errors

```bash
# Initialize/reinitialize rate limiting
nself auth rate-limit init

# Check PostgreSQL
nself status postgres
```

### Nginx Errors in Logs

```bash
# View nginx errors
nself logs nginx

# Check rate limit config syntax
docker exec nself_nginx nginx -t
```

## Advanced Configuration

### Custom Rate Limit Rules

Edit `.env` to add custom zones:

```bash
# Custom zone for admin endpoints
RATE_LIMIT_ADMIN_RATE=5r/m
```

Then modify `nginx/includes/rate-limits.conf` (after build) to use it.

### Geographic Blocking

Requires `ngx_http_geoip2_module` (not included by default).

### User-Based Rate Limiting

Already implemented via `user_api` zone. Uses `Authorization` header instead of IP.

## Related Documentation

- [Security Best Practices](SECURITY-BEST-PRACTICES.md)
- [Production Checklist](../deployment/PRODUCTION-DEPLOYMENT.md)
- [Nginx Configuration](../infrastructure/README.md)
- [Monitoring & Observability](../services/MONITORING-BUNDLE.md)

## Support

For issues or questions:
- GitHub Issues: https://github.com/nself-org/cli/issues
- Documentation: https://docs.nself.org
- Security: security@nself.org
