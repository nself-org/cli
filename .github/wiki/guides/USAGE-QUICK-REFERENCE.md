# Usage Tracking - Quick Reference Guide

**Version**: 0.9.0 | **Quick Start** for nself billing usage tracking

---

## 🚀 Quick Start

### Basic Setup

```bash
# Source the usage module
source "src/lib/billing/usage.sh"

# Track your first API request
usage_track_api_request "/api/users" "GET" 200
```

---

## 📊 Common Commands

### Track Usage

```bash
# API requests
usage_track_api_request "/api/users" "GET" 200
usage_track_api_request "/api/posts" "POST" 201

# Storage (5GB for 24 hours)
usage_track_storage 5368709120 24

# Bandwidth (500MB egress)
usage_track_bandwidth 524288000 "egress"

# Compute time (1 hour)
usage_track_compute 3600 '{"instance":"t3.medium"}'

# Database queries
usage_track_database_query "SELECT" 45

# Function invocations
usage_track_function "sendEmail" 120 256
```

### View Usage

```bash
# Today's usage
today=$(date -u +"%Y-%m-%d")
usage_get_all "${today} 00:00:00" "${today} 23:59:59"

# This month
usage_get_all "2026-01-01" "2026-01-31"

# Specific service
usage_get_service "api" "2026-01-01" "2026-01-31"

# Detailed breakdown
usage_get_all "2026-01-01" "2026-01-31" "table" "true"
```

### Export Data

```bash
# Export as CSV
usage_export csv

# Export as JSON
usage_export json

# Export with custom filename
usage_export csv "/path/to/usage.csv" "2026-01-01" "2026-01-31"

# Export specific service
usage_export json "" "2026-01-01" "2026-01-31" "api"
```

### Check Alerts

```bash
# Check all quotas
usage_check_alerts

# View recent alerts
usage_get_alerts 7  # Last 7 days
```

### Analytics

```bash
# Statistics
usage_get_stats "api" 30  # Last 30 days

# Trends
usage_get_trends "api" 7  # Last 7 days

# Peak periods
usage_get_peaks "api" "hourly" 10  # Top 10 hours
```

---

## 🔥 High-Volume Scenarios

### Batch Processing (>1000 events/sec)

```bash
# Initialize
usage_init_batch

customer_id=$(billing_get_customer_id)

# Track many events
for i in {1..10000}; do
    usage_batch_add "$customer_id" "api" 1 "{\"batch\":$i}"
done

# Flush
usage_batch_flush
```

### Bulk Insert

```bash
usage_batch_insert \
    "api:1:{\"endpoint\":\"/users\"}" \
    "api:1:{\"endpoint\":\"/posts\"}" \
    "database:1:{\"type\":\"SELECT\"}"
```

---

## 📈 Aggregation

```bash
customer_id=$(billing_get_customer_id)

# Hourly
usage_aggregate hourly "$customer_id" "2026-01-01" "2026-01-31"

# Daily
usage_aggregate daily "$customer_id" "2026-01-01" "2026-01-31"

# Monthly
usage_aggregate monthly "$customer_id" "2026-01-01" "2026-12-31"

# Refresh materialized view
usage_refresh_summary
```

---

## 🧹 Maintenance

```bash
# Archive old records (>90 days)
usage_archive 90

# Archive to specific file
usage_archive 90 "/backup/usage_archive.csv"

# Clean batch files
usage_cleanup_batch
```

---

## ⚙️ Configuration

### Environment Variables

```bash
# Batch processing
export USAGE_BATCH_SIZE=500
export USAGE_BATCH_TIMEOUT=10

# Alert thresholds (percentage)
export USAGE_ALERT_WARNING=75
export USAGE_ALERT_CRITICAL=90
export USAGE_ALERT_EXCEEDED=100
```

---

## 🎯 Real-World Examples

### Daily Report Script

```bash
#!/usr/bin/env bash
source "src/lib/billing/usage.sh"

today=$(date -u +"%Y-%m-%d")
start="${today} 00:00:00"
end="${today} 23:59:59"

# Generate report
usage_get_all "$start" "$end" "table" "true"

# Export
output="/var/reports/usage_${today}.csv"
usage_export csv "$output" "$start" "$end"

echo "Report saved to: $output"
```

### Quota Monitoring

```bash
#!/usr/bin/env bash
source "src/lib/billing/usage.sh"

# Check quotas
usage_check_alerts 2>&1 | while read -r line; do
    if [[ "$line" =~ "EXCEEDED" ]]; then
        # Send urgent notification
        echo "$line" | mail -s "URGENT: Quota Exceeded" admin@example.com
    elif [[ "$line" =~ "CRITICAL" ]]; then
        # Send warning
        echo "$line" | mail -s "WARNING: Critical Usage" admin@example.com
    fi
done
```

### API Endpoint Wrapper

```bash
api_call() {
    local endpoint="$1"
    local method="$2"

    # Make actual API call
    local status
    status=$(curl -s -o /dev/null -w "%{http_code}" -X "$method" "https://api.example.com$endpoint")

    # Track usage
    usage_track_api_request "$endpoint" "$method" "$status"

    return 0
}

# Use it
api_call "/users" "GET"
api_call "/posts" "POST"
```

### Storage Monitoring

```bash
#!/usr/bin/env bash
source "src/lib/billing/usage.sh"

# Calculate storage usage
total_bytes=$(du -sb /data | cut -f1)

# Get hours since last check
last_check="/data/.usage_check"
if [[ -f "$last_check" ]]; then
    last_time=$(stat -f %m "$last_check" 2>/dev/null || stat -c %Y "$last_check")
    now=$(date +%s)
    hours=$(( (now - last_time) / 3600 ))
else
    hours=1
fi

# Track usage
usage_track_storage "$total_bytes" "$hours"

# Update timestamp
touch "$last_check"

# Check quota
usage_check_service_alert "$(billing_get_customer_id)" "storage"
```

---

## 📋 Cron Jobs

### Daily Tasks

```cron
# Daily report at 1 AM
0 1 * * * /usr/local/bin/nself usage report daily

# Refresh materialized view
0 2 * * * /usr/local/bin/nself usage refresh-summary
```

### Weekly Tasks

```cron
# Weekly quota check on Monday 9 AM
0 9 * * 1 /usr/local/bin/nself usage check-alerts
```

### Monthly Tasks

```cron
# Archive old records on 1st of month
0 3 1 * * /usr/local/bin/nself usage archive 90

# Monthly export
0 4 1 * * /usr/local/bin/nself usage export json /var/billing/monthly/
```

---

## 🔍 Troubleshooting

### Issue: Batch not flushing

```bash
# Manually flush
usage_batch_flush

# Check batch file
cat "${BILLING_CACHE_DIR}/usage_batch.tmp"

# Clean and restart
usage_cleanup_batch
usage_init_batch
```

### Issue: Database connection failed

```bash
# Test connection
billing_test_db_connection

# Check environment
echo $BILLING_DB_HOST
echo $BILLING_DB_NAME
echo $BILLING_DB_USER
```

### Issue: No customer ID found

```bash
# Check customer ID
billing_get_customer_id

# Set manually
export NSELF_CUSTOMER_ID="cus_abc123"

# Or in .env
echo "NSELF_CUSTOMER_ID=cus_abc123" >> .env
```

### Issue: Export fails

```bash
# Check export directory
ls -la "${BILLING_EXPORT_DIR}"

# Create if missing
mkdir -p "${BILLING_EXPORT_DIR}"

# Check permissions
chmod 755 "${BILLING_EXPORT_DIR}"
```

---

## 📚 Related Commands

```bash
# View billing summary
billing_get_summary

# Check quotas
quota_get_all true

# View subscription
billing_get_subscription

# Generate invoice
billing_generate_invoice "$customer_id" "2026-01-01" "2026-01-31"
```

---

## 🔗 Links

- **Full Documentation**: [USAGE-TRACKING.md](./USAGE-TRACKING.md)
- **Implementation Summary**: [USAGE-IMPLEMENTATION-SUMMARY.md](./USAGE-IMPLEMENTATION-SUMMARY.md)
- **Database Schema**: `database/migrations/015_create_billing_system.sql`
- **Test Suite**: [../../src/tests/unit/test-billing-usage.sh](../../src/tests/unit/test-billing-usage.sh)
- **Demo Script**: [../../src/examples/billing/usage-tracking-demo.sh](../../src/examples/billing/usage-tracking-demo.sh)

---

## 💡 Tips

1. **Use batch processing** for high-volume scenarios (>100 events/sec)
2. **Refresh materialized view** daily for faster aggregations
3. **Archive old records** monthly to keep database lean
4. **Set up alerts** to prevent quota overages
5. **Export regularly** for compliance and auditing
6. **Monitor trends** to predict future usage
7. **Use specific service exports** for detailed analysis

---

**Quick Reference** | Version 0.9.0 | Last Updated: 2026-01-30
