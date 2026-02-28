# Billing Quotas System

**Part of nself v0.9.0 - Sprint 13: Billing Integration & Usage Tracking**

## Overview

The nself billing quotas system provides production-ready quota management with real-time enforcement, caching, and automated monitoring. It supports both soft limits (warnings) and hard limits (blocks), with overage calculation and rate limiting integration.

## Features

- **Plan-based quotas**: Define limits per service and plan
- **Fast quota checking**: Redis-cached lookups (< 10ms typical)
- **Soft vs hard limits**: Warning vs blocking enforcement
- **Overage billing**: Calculate and charge for usage beyond quotas
- **Rate limiting**: Protect against burst traffic
- **Automated monitoring**: Alert generation and notifications
- **Billing period resets**: Automatic quota resets at period end
- **Cache optimization**: Configurable TTL and warming

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Request Handler                          │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
                ┌───────────────────────┐
                │  quota_check_fast()   │
                └───────────┬───────────┘
                            │
                ┌───────────▼────────────┐
                │   Redis Cache          │
                │   (60s TTL)            │
                └───────────┬────────────┘
                            │
                   Cache Hit? │ No
                            │
                            ▼
                ┌───────────────────────┐
                │   Database Query      │
                │   (with indexes)      │
                └───────────┬───────────┘
                            │
                            ▼
                ┌───────────────────────┐
                │   Enforce Quota       │
                │   (soft/hard mode)    │
                └───────────────────────┘
```

## Database Schema

### billing_quotas

```sql
CREATE TABLE billing_quotas (
    quota_id SERIAL PRIMARY KEY,
    plan_name VARCHAR(100) NOT NULL,
    service_name VARCHAR(100) NOT NULL,
    limit_value BIGINT DEFAULT -1,          -- -1 = unlimited
    limit_type VARCHAR(50) DEFAULT 'requests',
    enforcement_mode VARCHAR(20) DEFAULT 'soft',
    overage_price DECIMAL(10, 6) DEFAULT 0.00,
    reset_period VARCHAR(20) DEFAULT 'monthly',
    UNIQUE(plan_name, service_name)
);
```

## Configuration

### Environment Variables

```bash
# Quota cache TTL (seconds)
QUOTA_CACHE_TTL=60

# Enable/disable caching
QUOTA_CACHE_ENABLED=true

# Customer ID
NSELF_CUSTOMER_ID=cust_abc123
```

### Alert Thresholds

```bash
QUOTA_ALERT_WARNING=75      # 75% of quota
QUOTA_ALERT_CRITICAL=90     # 90% of quota
QUOTA_ALERT_EXCEEDED=100    # 100% (over quota)
```

## Usage

### Basic Quota Checking

#### Fast Check (Cached)

```bash
# Check if API quota allows 1 request
if quota_check_fast "api" 1; then
    echo "Request allowed"
else
    echo "Quota exceeded"
    exit 1
fi

# Check with custom cache TTL
quota_check_fast "api" 1 120  # 120 second cache
```

#### With Rate Limiting

```bash
# Check quota with burst protection
if quota_check_rate_limited "api" 100; then
    echo "Request allowed"
else
    echo "Rate limited"
    exit 1
fi
```

### Display Quotas

#### All Services

```bash
# Show all quotas (table format)
quota_get_all false "table"

# Show with current usage
quota_get_all true "table"

# Export as JSON
quota_get_all true "json"

# Export as CSV
quota_get_all true "csv"
```

#### Specific Service

```bash
# Show API quota details
quota_get_service "api" true "table"

# Get as JSON
quota_get_service "api" true "json"
```

### Quota Alerts

#### Check for Alerts

```bash
# Check alerts without notifications
quota_get_alerts "table"

# Check and send notifications
quota_check_alerts "" true
```

#### Monitor All Customers

```bash
# Run quota monitoring (use in cron)
quota_monitor_all
```

### Overage Calculation

#### Calculate Overages

```bash
# Calculate for all services
quota_calculate_overage

# Calculate for specific service
quota_calculate_overage "" "api"
```

#### Display Overage Report

```bash
# Show as table
quota_show_overage "" "table"

# Export as JSON
quota_show_overage "" "json"
```

### Quota Reset

#### Reset for Customer

```bash
# Reset all services
quota_reset "cust_abc123"

# Reset specific service
quota_reset "cust_abc123" "api"
```

#### Reset All Expired

```bash
# Reset all customers with expired billing periods
quota_reset_all_expired
```

### Cache Management

#### Invalidate Cache

```bash
# Invalidate all quota caches
quota_cache_invalidate_all

# Warm cache for customer
quota_cache_warm "cust_abc123"
```

## API Reference

### Core Functions

#### `quota_check_fast(service, requested, cache_ttl)`

Fast quota check with Redis caching.

**Parameters:**
- `service`: Service name (api, storage, bandwidth, etc.)
- `requested`: Quantity requested (default: 1)
- `cache_ttl`: Cache TTL in seconds (default: 60)

**Returns:**
- `0`: Quota available
- `1`: Quota exceeded (if hard limit)
- `0`: Quota exceeded but allowed (if soft limit)

**Example:**
```bash
if quota_check_fast "api" 1 300; then
    # Process request
fi
```

#### `quota_check_rate_limited(service, max_requests_per_sec)`

Check quota with rate limiting protection.

**Parameters:**
- `service`: Service name
- `max_requests_per_sec`: Maximum requests per second (default: 10)

**Returns:**
- `0`: Request allowed
- `1`: Rate limited

**Example:**
```bash
if quota_check_rate_limited "api" 100; then
    # High-traffic service
fi
```

#### `quota_enforce(service, requested)`

Enforce quota check with mode-based handling.

**Parameters:**
- `service`: Service name
- `requested`: Quantity requested (default: 1)

**Returns:**
- `0`: Request allowed
- `1`: Request blocked (hard limit exceeded)

**Example:**
```bash
quota_enforce "api" 1 || { echo "Access denied"; exit 1; }
```

### Display Functions

#### `quota_get_all(show_usage, format)`

Get all quotas for current customer.

**Parameters:**
- `show_usage`: Show current usage (true/false)
- `format`: Output format (table/json/csv)

**Example:**
```bash
quota_get_all true "table"
```

#### `quota_get_service(service, show_usage, format)`

Get quota for specific service.

**Parameters:**
- `service`: Service name
- `show_usage`: Show current usage (true/false)
- `format`: Output format (table/json/csv)

**Example:**
```bash
quota_get_service "api" true "json"
```

#### `quota_get_alerts(format)`

Get quota alerts (services approaching limits).

**Parameters:**
- `format`: Output format (table/json/csv)

**Example:**
```bash
quota_get_alerts "table"
```

### Maintenance Functions

#### `quota_reset(customer_id, service)`

Reset quota for billing period.

**Parameters:**
- `customer_id`: Customer ID (optional, uses current)
- `service`: Service name (optional, resets all)

**Example:**
```bash
quota_reset "cust_abc123" "api"
```

#### `quota_reset_all_expired()`

Reset quotas for all customers with expired billing periods.

**Example:**
```bash
# Run via cron at midnight
0 0 * * * /usr/local/bin/nself billing quota reset-all-expired
```

#### `quota_calculate_overage(customer_id, service)`

Calculate overage charges for billing period.

**Parameters:**
- `customer_id`: Customer ID (optional)
- `service`: Service name (optional)

**Example:**
```bash
quota_calculate_overage "cust_abc123"
```

#### `quota_show_overage(customer_id, format)`

Display overage report.

**Parameters:**
- `customer_id`: Customer ID (optional)
- `format`: Output format (table/json/csv)

**Example:**
```bash
quota_show_overage "" "table"
```

### Monitoring Functions

#### `quota_check_alerts(customer_id, send_notifications)`

Check for quota alerts and optionally send notifications.

**Parameters:**
- `customer_id`: Customer ID (optional)
- `send_notifications`: Send notifications (true/false)

**Example:**
```bash
quota_check_alerts "" true
```

#### `quota_monitor_all()`

Monitor all customers and generate alerts.

**Example:**
```bash
# Run via cron every 15 minutes
*/15 * * * * /usr/local/bin/nself billing quota monitor
```

### Cache Functions

#### `quota_cache_invalidate_all()`

Invalidate all quota caches.

**Example:**
```bash
quota_cache_invalidate_all
```

#### `quota_cache_warm(customer_id)`

Warm quota cache for customer.

**Parameters:**
- `customer_id`: Customer ID (optional)

**Example:**
```bash
quota_cache_warm "cust_abc123"
```

## Performance

### Benchmarks

| Operation | Without Cache | With Cache | Improvement |
|-----------|--------------|------------|-------------|
| quota_check_fast | ~50ms | ~5ms | 10x faster |
| quota_get_all | ~200ms | ~50ms | 4x faster |
| quota_get_service | ~30ms | ~8ms | 3.75x faster |

### Optimization Tips

1. **Enable Redis caching**: Dramatically improves performance
2. **Increase cache TTL**: For read-heavy workloads (up to 300s)
3. **Use quota_check_fast**: In hot paths (API handlers)
4. **Warm cache on startup**: Pre-populate for active customers
5. **Use indexes**: Database indexes on customer_id, service_name, recorded_at

## Integration Examples

### API Middleware

```bash
# Express.js middleware example
check_quota_middleware() {
    local service="api"
    local customer_id="$1"

    # Fast check
    if ! quota_check_fast "$service" 1; then
        echo '{"error":"Quota exceeded"}'
        return 429  # Too Many Requests
    fi

    # Rate limit check
    if ! quota_check_rate_limited "$service" 100; then
        echo '{"error":"Rate limited"}'
        return 429
    fi

    return 0
}
```

### Serverless Functions

```bash
# Check quota before function execution
check_function_quota() {
    local function_name="$1"

    if ! quota_check_fast "functions" 1 30; then
        echo "Function invocation blocked - quota exceeded"
        exit 1
    fi

    # Execute function
    run_function "$function_name"
}
```

### Storage Operations

```bash
# Check storage quota before upload
check_storage_quota() {
    local file_size="$1"  # in bytes
    local gb_size
    gb_size=$(awk "BEGIN {print $file_size / 1073741824}")

    if ! quota_check_fast "storage" "$gb_size" 60; then
        echo "Upload blocked - storage quota exceeded"
        return 1
    fi

    return 0
}
```

## Cron Jobs

### Recommended Schedule

```cron
# Reset expired quotas (daily at midnight)
0 0 * * * /usr/local/bin/nself billing quota reset-all-expired

# Monitor all customers (every 15 minutes)
*/15 * * * * /usr/local/bin/nself billing quota monitor

# Warm cache for top customers (every hour)
0 * * * * /usr/local/bin/nself billing quota warm-cache

# Generate overage reports (end of month)
0 0 1 * * /usr/local/bin/nself billing quota overage-report
```

## Troubleshooting

### Quota Check Always Fails

**Problem**: `quota_check_fast` always returns 1

**Solutions:**
1. Check customer ID is set: `echo $NSELF_CUSTOMER_ID`
2. Verify subscription is active: `nself billing subscription status`
3. Check quota configuration: `quota_get_all true "table"`

### Cache Not Working

**Problem**: Quota checks are slow

**Solutions:**
1. Verify Redis is running: `docker ps | grep redis`
2. Check Redis connection: `redis-cli ping`
3. Enable cache: `export QUOTA_CACHE_ENABLED=true`

### Alerts Not Generated

**Problem**: No alerts when quota exceeded

**Solutions:**
1. Run manual check: `quota_check_alerts "" true`
2. Verify alert thresholds: `echo $QUOTA_ALERT_WARNING`
3. Check subscription status: `nself billing subscription status`

### Overage Calculation Wrong

**Problem**: Overage charges incorrect

**Solutions:**
1. Verify usage records: `nself billing usage get-all --detailed`
2. Check quota limits: `quota_get_all false "table"`
3. Refresh materialized view: `nself billing maintenance refresh-views`

## Best Practices

1. **Always use quota_check_fast**: In hot paths for best performance
2. **Cache appropriately**: Balance freshness vs performance (30-300s)
3. **Monitor regularly**: Run `quota_monitor_all` every 15 minutes
4. **Reset at period end**: Use cron for automated resets
5. **Warm cache proactively**: For high-traffic customers
6. **Use soft limits**: For better customer experience
7. **Alert early**: At 75% threshold, not just at 100%
8. **Log all quota events**: For auditing and debugging

## Security Considerations

1. **Validate customer ID**: Always verify before quota checks
2. **Rate limit checks**: Prevent quota check abuse
3. **Secure cache keys**: Use customer ID in cache keys
4. **Audit logs**: Log all quota enforcement decisions
5. **Encrypt sensitive data**: Customer info in cache

## Related Documentation

- [Billing & Usage](./BILLING-AND-USAGE.md)
- [Usage Tracking](./USAGE-TRACKING.md)
- [Plans Management](./BILLING-AND-USAGE.md)
- [Redis](../services/REDIS.md)
- [Rate Limiting](../security/RATE-LIMITING.md)

## Support

For issues or questions:
- GitHub: https://github.com/nself-org/cli/issues
- Documentation: https://docs.nself.org/billing/quotas
- Community: https://discord.gg/nself
