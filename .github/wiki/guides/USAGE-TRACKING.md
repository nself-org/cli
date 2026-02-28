# Usage Tracking System

**Part of nself v0.9.0 - Sprint 13: Billing Integration & Usage Tracking**

Complete guide to usage metering, tracking, aggregation, and reporting across all billable services.

---

## Table of Contents

1. [Overview](#overview)
2. [Tracked Services](#tracked-services)
3. [High-Volume Write Optimization](#high-volume-write-optimization)
4. [Usage Recording](#usage-recording)
5. [Usage Aggregation](#usage-aggregation)
6. [Usage Reporting](#usage-reporting)
7. [Usage Alerts](#usage-alerts)
8. [Export Functionality](#export-functionality)
9. [Statistics & Analytics](#statistics--analytics)
10. [Cleanup & Maintenance](#cleanup--maintenance)
11. [API Reference](#api-reference)
12. [Examples](#examples)

---

## Overview

The usage tracking system provides:

- **High-volume write optimization** with batch processing
- **Six billable service types** (API, storage, bandwidth, compute, database, functions)
- **Real-time and aggregated** reporting (hourly, daily, monthly)
- **Multiple export formats** (CSV, JSON, XLSX)
- **Automated alerts** based on quota thresholds
- **Comprehensive analytics** with trends and peak detection

### Architecture

```
┌─────────────────┐
│  Service Calls  │  (API requests, storage, bandwidth, etc.)
└────────┬────────┘
         │
         ▼
┌─────────────────────────┐
│  usage_track_*()        │  Service-specific tracking functions
└────────┬────────────────┘
         │
         ▼
┌─────────────────────────┐
│  Batch Queue            │  Optional batch processing for high volume
│  (usage_batch_add)      │
└────────┬────────────────┘
         │
         ▼
┌─────────────────────────┐
│  billing_usage_records  │  PostgreSQL table
└────────┬────────────────┘
         │
         ▼
┌─────────────────────────┐
│  Aggregation & Reports  │  Hourly, daily, monthly summaries
└─────────────────────────┘
```

---

## Tracked Services

The system tracks usage for six distinct services:

### 1. API Requests
- **Unit**: Requests
- **Tracking**: Per endpoint, method, status code
- **Metadata**: `{"endpoint":"/users","method":"GET","status":200}`

### 2. Storage
- **Unit**: GB-hours (gigabyte-hours)
- **Tracking**: Bytes stored over time
- **Metadata**: `{"bytes":1073741824,"hours":24}`

### 3. Bandwidth
- **Unit**: GB (gigabytes)
- **Tracking**: Data transfer (egress/ingress)
- **Metadata**: `{"bytes":524288000,"direction":"egress"}`

### 4. Compute
- **Unit**: CPU-hours
- **Tracking**: Container/VM compute time
- **Metadata**: Custom (container ID, instance type, etc.)

### 5. Database
- **Unit**: Queries
- **Tracking**: Query count and duration
- **Metadata**: `{"type":"SELECT","duration_ms":45}`

### 6. Functions
- **Unit**: Invocations
- **Tracking**: Serverless function calls
- **Metadata**: `{"function":"sendEmail","duration_ms":120,"memory_mb":256}`

---

## High-Volume Write Optimization

For services generating thousands of events per second, the system provides batch processing.

### Batch Configuration

```bash
# Set batch size (default: 100 records)
export USAGE_BATCH_SIZE=500

# Set batch timeout (default: 5 seconds)
export USAGE_BATCH_TIMEOUT=10
```

### Batch Processing Functions

#### `usage_batch_add()`
Add a usage record to the batch queue (non-blocking).

```bash
usage_batch_add "$customer_id" "api" 1 '{"endpoint":"/users","method":"GET"}'
```

#### `usage_batch_flush()`
Manually flush the batch queue to database (uses PostgreSQL COPY for maximum performance).

```bash
usage_batch_flush
```

#### `usage_batch_insert()`
Insert multiple records in a single transaction.

```bash
usage_batch_insert \
    "api:1:{\"endpoint\":\"/users\"}" \
    "api:1:{\"endpoint\":\"/posts\"}" \
    "database:1:{\"type\":\"SELECT\"}"
```

### Performance

- **Individual INSERT**: ~1,000 records/second
- **Batch INSERT (100 records)**: ~10,000 records/second
- **PostgreSQL COPY**: ~50,000+ records/second

---

## Usage Recording

### Service-Specific Functions

Each service has a dedicated tracking function with proper unit conversion.

#### API Requests

```bash
usage_track_api_request "/api/users" "GET" 200
usage_track_api_request "/api/posts" "POST" 201
```

#### Storage

```bash
# Track 5GB stored for 24 hours
bytes=5368709120  # 5GB
usage_track_storage "$bytes" 24
```

#### Bandwidth

```bash
# Track 500MB egress
bytes=524288000
usage_track_bandwidth "$bytes" "egress"
```

#### Compute Time

```bash
# Track 3600 seconds (1 hour) of CPU time
cpu_seconds=3600
usage_track_compute "$cpu_seconds" '{"instance":"t3.medium"}'
```

#### Database Queries

```bash
usage_track_database_query "SELECT" 45  # 45ms duration
usage_track_database_query "INSERT" 120
```

#### Function Invocations

```bash
usage_track_function "sendEmail" 120 256  # 120ms, 256MB memory
usage_track_function "processImage" 3500 512
```

### Direct Recording

All service functions ultimately call `billing_record_usage()`:

```bash
billing_record_usage "service_name" quantity "metadata_json"
```

---

## Usage Aggregation

Aggregate usage data for faster reporting and analysis.

### Aggregation Periods

```bash
# Hourly aggregation
usage_aggregate hourly "$customer_id" "2026-01-01" "2026-01-31"

# Daily aggregation
usage_aggregate daily "$customer_id" "2026-01-01" "2026-01-31"

# Monthly aggregation
usage_aggregate monthly "$customer_id" "2026-01-01" "2026-12-31"
```

### Aggregation Output

```
service_name | date       | event_count | total_quantity | total_cost | avg_quantity
-------------+------------+-------------+----------------+------------+--------------
api          | 2026-01-30 | 15420       | 15420          | 1.542      | 1.0
storage      | 2026-01-30 | 24          | 120.5          | 0.1205     | 5.02
bandwidth    | 2026-01-30 | 8934        | 45.67          | 2.2835     | 0.0051
```

### Materialized View Refresh

The system uses a materialized view for even faster aggregations:

```bash
usage_refresh_summary
```

This refreshes the `billing_usage_daily_summary` materialized view.

---

## Usage Reporting

### Get All Usage

```bash
# Table format (default)
usage_get_all "2026-01-01" "2026-01-31"

# JSON format
usage_get_all "2026-01-01" "2026-01-31" "json"

# CSV format
usage_get_all "2026-01-01" "2026-01-31" "csv"

# Detailed breakdown
usage_get_all "2026-01-01" "2026-01-31" "table" "true"
```

### Get Service-Specific Usage

```bash
# API usage for January
usage_get_service "api" "2026-01-01" "2026-01-31"

# Storage usage with detailed timeline
usage_get_service "storage" "2026-01-01" "2026-01-31" "table" "true"

# Bandwidth usage as JSON
usage_get_service "bandwidth" "2026-01-01" "2026-01-31" "json"
```

### Example Output (Table Format)

```
╔════════════════════════════════════════════════════════════════╗
║                      USAGE SUMMARY                             ║
╠════════════════════════════════════════════════════════════════╣
║ Period: 2026-01-01 to 2026-01-31                              ║
╠════════════════════════════════════════════════════════════════╣
║ Service          │ Usage          │ Unit       │ Cost        ║
╠══════════════════╪════════════════╪════════════╪═════════════╣
║ api              │       1.25M    │ requests   │ $125.00     ║
║ storage          │         45.2   │ GB-hours   │ $4.52       ║
║ bandwidth        │        156.8   │ GB         │ $7.84       ║
║ compute          │         12.5   │ CPU-hours  │ $0.63       ║
║ database         │       45.6K    │ queries    │ $0.46       ║
║ functions        │        8.9K    │ invocations│ $1.78       ║
╚══════════════════╧════════════════╧════════════╧═════════════╝
```

---

## Usage Alerts

Automated monitoring of usage against quotas with three alert levels.

### Alert Thresholds

```bash
# Configure thresholds (percentage of quota)
export USAGE_ALERT_WARNING=75    # Default: 75%
export USAGE_ALERT_CRITICAL=90   # Default: 90%
export USAGE_ALERT_EXCEEDED=100  # Default: 100%
```

### Check Alerts

```bash
# Check all services
usage_check_alerts

# Check specific service
usage_check_service_alert "$customer_id" "api"
```

### Alert Output

```
WARNING: api usage at 78% - 78000/100000
CRITICAL: storage usage at 92% - 9.2GB/10GB
QUOTA EXCEEDED: bandwidth - 105GB/100GB (105%)
```

### View Alert History

```bash
# Last 7 days (default)
usage_get_alerts

# Last 30 days
usage_get_alerts 30
```

### Alert Log Format

```
[2026-01-30 14:23:45] warning ALERT for api: 78000/100000 (78%)
[2026-01-30 15:45:12] critical ALERT for storage: 9.2/10 (92%)
[2026-01-30 16:30:00] exceeded ALERT for bandwidth: 105/100 (105%)
```

---

## Export Functionality

Export usage data in multiple formats with automatic file naming.

### Export Functions

```bash
# Export as CSV (default)
usage_export csv

# Export as JSON
usage_export json

# Export to specific file
usage_export csv "/path/to/usage.csv"

# Export with date range
usage_export csv "" "2026-01-01" "2026-01-31"

# Export specific service
usage_export json "" "2026-01-01" "2026-01-31" "api"
```

### CSV Export

**Features:**
- Headers included
- Comma-delimited
- Metadata as JSON string
- Sorted by timestamp (newest first)

**Format:**
```csv
timestamp,service,quantity,unit_cost,total_cost,metadata
2026-01-30 16:45:23,api,1,0.0001,0.0001,"{""endpoint"":""/users"",""method"":""GET""}"
2026-01-30 16:45:22,bandwidth,0.000512,0.01,0.00000512,"{""bytes"":524288,""direction"":""egress""}"
```

### JSON Export

**Features:**
- Complete metadata preserved
- Summary statistics included
- Nested structure for clarity

**Format:**
```json
{
  "customer_id": "cus_abc123",
  "export_date": "2026-01-30T16:45:23Z",
  "period": {
    "start": "2026-01-01",
    "end": "2026-01-31"
  },
  "usage_records": [
    {
      "timestamp": "2026-01-30T16:45:23Z",
      "service": "api",
      "quantity": 1,
      "unit_cost": 0.0001,
      "total_cost": 0.0001,
      "metadata": {"endpoint": "/users", "method": "GET"}
    }
  ],
  "summary": {
    "total_events": 125000,
    "total_cost": 140.23,
    "services": ["api", "storage", "bandwidth", "compute", "database", "functions"]
  }
}
```

### XLSX Export

Requires `csvkit` or similar tools. Falls back to CSV if not available.

```bash
# Install csvkit (if needed)
pip install csvkit

# Export as XLSX
usage_export xlsx
```

---

## Statistics & Analytics

Advanced analytics functions for usage patterns and trends.

### Usage Statistics

Get comprehensive stats for a service:

```bash
# API statistics for last 30 days
usage_get_stats "api" 30

# Storage statistics for last 7 days
usage_get_stats "storage" 7
```

**Output:**
```
service_name | total_events | total_quantity | avg_quantity | p95_quantity | p99_quantity
-------------+--------------+----------------+--------------+--------------+--------------
api          | 125000       | 125000         | 1.0          | 1.0          | 1.0
```

### Usage Trends

Day-over-day comparison with percentage change:

```bash
# All services, last 7 days
usage_get_trends

# Specific service, last 30 days
usage_get_trends "api" 30
```

**Output:**
```
date       | service_name | daily_total | previous_day | percent_change
-----------+--------------+-------------+--------------+----------------
2026-01-30 | api          | 5420        | 5123         | 5.80
2026-01-29 | api          | 5123        | 5456         | -6.10
```

### Peak Usage Detection

Find highest usage periods:

```bash
# Top 10 hours for API
usage_get_peaks "api" "hourly" 10

# Top 5 days for storage
usage_get_peaks "storage" "daily" 5
```

**Output:**
```
period              | total_usage | event_count
--------------------+-------------+-------------
2026-01-30 14:00:00 | 1250        | 1250
2026-01-30 15:00:00 | 1180        | 1180
```

---

## Cleanup & Maintenance

### Archive Old Records

Archive and delete usage records older than N days:

```bash
# Archive records older than 90 days (default)
usage_archive

# Archive records older than 30 days
usage_archive 30

# Archive to specific file
usage_archive 90 "/path/to/archive.csv"
```

**Process:**
1. Export records older than cutoff to CSV
2. Delete archived records from database
3. Report number of archived records

### Cleanup Batch Files

Flush pending batches and clean temporary files:

```bash
usage_cleanup_batch
```

### Maintenance Schedule

**Recommended:**
- **Daily**: Refresh materialized view (`usage_refresh_summary`)
- **Weekly**: Check alerts (`usage_check_alerts`)
- **Monthly**: Archive old records (`usage_archive`)
- **Quarterly**: Export historical data for long-term storage

---

## API Reference

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `USAGE_BATCH_SIZE` | 100 | Records per batch flush |
| `USAGE_BATCH_TIMEOUT` | 5 | Seconds before auto-flush |
| `USAGE_ALERT_WARNING` | 75 | Warning threshold (%) |
| `USAGE_ALERT_CRITICAL` | 90 | Critical threshold (%) |
| `USAGE_ALERT_EXCEEDED` | 100 | Exceeded threshold (%) |

### Core Functions

| Function | Parameters | Description |
|----------|------------|-------------|
| `usage_get_all` | start, end, format, detailed | Get all usage for period |
| `usage_get_service` | service, start, end, format, detailed | Get service usage |
| `usage_export` | format, file, start, end, service | Export usage data |
| `usage_aggregate` | period, customer_id, start, end | Aggregate usage |
| `usage_check_alerts` | - | Check all quota alerts |
| `usage_get_stats` | service, days | Get usage statistics |
| `usage_get_trends` | service, days | Get usage trends |
| `usage_archive` | days_to_keep, archive_file | Archive old records |

### Tracking Functions

| Function | Parameters | Description |
|----------|------------|-------------|
| `usage_track_api_request` | endpoint, method, status | Track API request |
| `usage_track_storage` | bytes, hours | Track storage usage |
| `usage_track_bandwidth` | bytes, direction | Track bandwidth |
| `usage_track_compute` | cpu_seconds, metadata | Track compute time |
| `usage_track_database_query` | type, duration_ms | Track DB query |
| `usage_track_function` | name, duration_ms, memory_mb | Track function |

### Batch Functions

| Function | Parameters | Description |
|----------|------------|-------------|
| `usage_init_batch` | - | Initialize batch processing |
| `usage_batch_add` | customer_id, service, qty, metadata | Add to batch |
| `usage_batch_flush` | - | Flush batch to database |
| `usage_batch_insert` | records... | Bulk insert records |

---

## Examples

### Example 1: Track API Endpoint Usage

```bash
#!/usr/bin/env bash
source "src/lib/billing/usage.sh"

# Track various API endpoints
usage_track_api_request "/api/users" "GET" 200
usage_track_api_request "/api/users/123" "GET" 200
usage_track_api_request "/api/posts" "POST" 201
usage_track_api_request "/api/invalid" "GET" 404

# View usage
usage_get_service "api" "$(date -u +"%Y-%m-%d 00:00:00")" "$(date -u +"%Y-%m-%d 23:59:59")"
```

### Example 2: High-Volume Batch Processing

```bash
#!/usr/bin/env bash
source "src/lib/billing/usage.sh"

# Initialize batch processing
usage_init_batch

# Get customer ID
customer_id=$(billing_get_customer_id)

# Track 1000 API requests (batched)
for i in {1..1000}; do
    usage_batch_add "$customer_id" "api" 1 "{\"endpoint\":\"/test\",\"i\":$i}"
done

# Batch auto-flushes at 100 records, but flush any remaining
usage_batch_flush
```

### Example 3: Storage Usage Monitoring

```bash
#!/usr/bin/env bash
source "src/lib/billing/usage.sh"

# Calculate storage usage
total_bytes=$(du -sb /data | cut -f1)
hours_since_last=$(( ($(date +%s) - $(stat -f %m /data/.last_check)) / 3600 ))

# Track usage
usage_track_storage "$total_bytes" "$hours_since_last"

# Update last check
touch /data/.last_check

# Check if approaching quota
usage_check_service_alert "$(billing_get_customer_id)" "storage"
```

### Example 4: Daily Usage Report

```bash
#!/usr/bin/env bash
source "src/lib/billing/usage.sh"

# Get today's date range
today=$(date -u +"%Y-%m-%d")
start="${today} 00:00:00"
end="${today} 23:59:59"

# Generate report
printf "Daily Usage Report - %s\n\n" "$today"

usage_get_all "$start" "$end" "table" "true"

# Export to file
output_file="/var/reports/usage_${today}.csv"
usage_export csv "$output_file" "$start" "$end"

printf "\nExported to: %s\n" "$output_file"
```

### Example 5: Usage Trend Analysis

```bash
#!/usr/bin/env bash
source "src/lib/billing/usage.sh"

# Analyze API usage trends
printf "API Usage Trends (Last 7 Days)\n"
printf "================================\n\n"

usage_get_trends "api" 7

# Find peak usage hours
printf "\nTop 5 Peak Hours\n"
printf "================\n\n"

usage_get_peaks "api" "hourly" 5

# Get detailed statistics
printf "\nDetailed Statistics (Last 30 Days)\n"
printf "==================================\n\n"

usage_get_stats "api" 30
```

### Example 6: Automated Quota Alerts

```bash
#!/usr/bin/env bash
source "src/lib/billing/usage.sh"

# Check quotas for all services
printf "Checking usage quotas...\n\n"

usage_check_alerts 2>&1 | while IFS= read -r line; do
    if [[ "$line" =~ "EXCEEDED" ]]; then
        # Send urgent alert
        echo "$line" | mail -s "URGENT: Quota Exceeded" admin@example.com
    elif [[ "$line" =~ "CRITICAL" ]]; then
        # Send warning
        echo "$line" | mail -s "WARNING: Quota Critical" admin@example.com
    fi

    # Log all alerts
    printf "%s\n" "$line"
done

# Show recent alert history
printf "\nRecent Alerts (Last 7 Days)\n"
printf "===========================\n\n"
usage_get_alerts 7
```

### Example 7: Monthly Billing Export

```bash
#!/usr/bin/env bash
source "src/lib/billing/usage.sh"

# Get last month's date range
if date -v-1m >/dev/null 2>&1; then
    # macOS
    start=$(date -v-1m -v1d +"%Y-%m-01 00:00:00")
    end=$(date -v-1m -v+1m -v-1d +"%Y-%m-%d 23:59:59")
else
    # Linux
    start=$(date -d "last month" +"%Y-%m-01 00:00:00")
    end=$(date -d "$(date +%Y-%m-01) -1 day" +"%Y-%m-%d 23:59:59")
fi

month=$(date -d "$start" +"%Y-%m" 2>/dev/null || date -j -f "%Y-%m-%d" "$(echo $start | cut -d' ' -f1)" +"%Y-%m")

printf "Generating monthly usage report for %s...\n" "$month"

# Export in multiple formats
usage_export csv "/var/billing/usage_${month}.csv" "$start" "$end"
usage_export json "/var/billing/usage_${month}.json" "$start" "$end"

# Archive old records (keep last 90 days)
usage_archive 90 "/var/billing/archive/usage_${month}_archive.csv"

printf "\nMonthly export complete\n"
```

---

## Performance Considerations

### High-Volume Systems

For systems generating **>10,000 events/second**:

1. **Enable batch processing**:
   ```bash
   export USAGE_BATCH_SIZE=1000
   ```

2. **Use PostgreSQL COPY** via `usage_batch_flush()`

3. **Partition the table** by date:
   ```sql
   CREATE TABLE billing_usage_records_2026_01
   PARTITION OF billing_usage_records
   FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
   ```

4. **Use async workers** to process batches

### Database Optimization

1. **Indexes** (already created):
   - `(customer_id, service_name)`
   - `(customer_id, recorded_at)`
   - `(aggregated)` (partial index for unaggregated records)

2. **Materialized view** for daily summaries

3. **Regular VACUUM** and **ANALYZE**:
   ```sql
   VACUUM ANALYZE billing_usage_records;
   ```

### Storage Optimization

1. **Archive old records** monthly
2. **Compress archives** (gzip recommended)
3. **Use object storage** (S3, MinIO) for long-term archives

---

## Related Documentation

- [Billing & Usage](./BILLING-AND-USAGE.md)
- [Quota Management](./QUOTAS.md)
- [Stripe Integration](./STRIPE_IMPLEMENTATION.md)
- Database Schema

---

**Version**: 0.9.0
**Last Updated**: 2026-01-30
**Status**: Production Ready
