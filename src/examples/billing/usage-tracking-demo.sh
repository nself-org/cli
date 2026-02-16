#!/usr/bin/env bash
set -euo pipefail
#
# nself Usage Tracking - Comprehensive Demo
# Demonstrates all major usage tracking features
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Source billing modules
source "${NSELF_ROOT}/src/lib/billing/usage.sh"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

demo_section() {
    printf "\n"
    printf "${BLUE}════════════════════════════════════════════════════════════════${NC}\n"
    printf "${BLUE}%s${NC}\n" "$1"
    printf "${BLUE}════════════════════════════════════════════════════════════════${NC}\n"
    printf "\n"
}

demo_info() {
    printf "${YELLOW}ℹ${NC} %s\n" "$1"
}

demo_success() {
    printf "${GREEN}✓${NC} %s\n" "$1"
}

# ============================================================================
# Demo 1: Basic Usage Tracking
# ============================================================================

demo_basic_tracking() {
    demo_section "Demo 1: Basic Usage Tracking"

    demo_info "Tracking various service usage..."

    # Track API requests
    printf "\nTracking API requests:\n"
    usage_track_api_request "/api/users" "GET" 200
    demo_success "GET /api/users - 200"

    usage_track_api_request "/api/posts" "POST" 201
    demo_success "POST /api/posts - 201"

    usage_track_api_request "/api/comments" "GET" 200
    demo_success "GET /api/comments - 200"

    # Track storage
    printf "\nTracking storage usage:\n"
    local storage_bytes=5368709120  # 5GB
    usage_track_storage "$storage_bytes" 24
    demo_success "5GB stored for 24 hours"

    # Track bandwidth
    printf "\nTracking bandwidth:\n"
    local bandwidth_bytes=524288000  # 500MB
    usage_track_bandwidth "$bandwidth_bytes" "egress"
    demo_success "500MB egress"

    # Track compute
    printf "\nTracking compute time:\n"
    local cpu_seconds=3600  # 1 hour
    usage_track_compute "$cpu_seconds" '{"instance":"t3.medium"}'
    demo_success "1 CPU-hour on t3.medium"

    # Track database queries
    printf "\nTracking database queries:\n"
    usage_track_database_query "SELECT" 45
    demo_success "SELECT query (45ms)"

    usage_track_database_query "INSERT" 120
    demo_success "INSERT query (120ms)"

    # Track function invocations
    printf "\nTracking serverless functions:\n"
    usage_track_function "sendEmail" 120 256
    demo_success "sendEmail function (120ms, 256MB)"

    usage_track_function "processImage" 3500 512
    demo_success "processImage function (3500ms, 512MB)"

    printf "\n${GREEN}✓ Basic tracking complete${NC}\n"
}

# ============================================================================
# Demo 2: Batch Processing (High-Volume)
# ============================================================================

demo_batch_processing() {
    demo_section "Demo 2: High-Volume Batch Processing"

    demo_info "Simulating high-volume API traffic (1000 requests)..."

    # Initialize batch
    usage_init_batch

    local customer_id
    customer_id=$(billing_get_customer_id 2>/dev/null) || customer_id="demo_customer"

    # Track 1000 API requests using batch processing
    local start_time
    start_time=$(date +%s)

    for i in {1..1000}; do
        usage_batch_add "$customer_id" "api" 1 "{\"endpoint\":\"/test\",\"batch\":$i}"

        # Show progress every 100 records
        if [[ $((i % 100)) -eq 0 ]]; then
            printf "."
        fi
    done

    printf "\n"

    # Flush any remaining
    usage_batch_flush

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    demo_success "Processed 1000 records in ${duration} seconds"
    demo_success "Throughput: ~$((1000 / (duration + 1))) records/second"

    printf "\n${GREEN}✓ Batch processing complete${NC}\n"
}

# ============================================================================
# Demo 3: Usage Aggregation
# ============================================================================

demo_aggregation() {
    demo_section "Demo 3: Usage Aggregation"

    demo_info "Aggregating usage data..."

    local customer_id
    customer_id=$(billing_get_customer_id 2>/dev/null) || {
        demo_info "Skipping aggregation (no customer ID configured)"
        return 0
    }

    # Get date range (last 7 days)
    local end_date
    end_date=$(date -u +"%Y-%m-%d %H:%M:%S")

    local start_date
    if date -v-7d >/dev/null 2>&1; then
        # macOS
        start_date=$(date -v-7d -u +"%Y-%m-%d %H:%M:%S")
    else
        # Linux
        start_date=$(date -d "7 days ago" -u +"%Y-%m-%d %H:%M:%S")
    fi

    printf "\nDaily aggregation:\n"
    usage_aggregate daily "$customer_id" "$start_date" "$end_date" 2>/dev/null || \
        demo_info "Daily aggregation requires database connection"

    printf "\nHourly aggregation:\n"
    usage_aggregate hourly "$customer_id" "$start_date" "$end_date" 2>/dev/null || \
        demo_info "Hourly aggregation requires database connection"

    printf "\n${GREEN}✓ Aggregation demo complete${NC}\n"
}

# ============================================================================
# Demo 4: Usage Reporting
# ============================================================================

demo_reporting() {
    demo_section "Demo 4: Usage Reporting"

    demo_info "Generating usage reports..."

    local today
    today=$(date -u +"%Y-%m-%d")
    local start="${today} 00:00:00"
    local end="${today} 23:59:59"

    printf "\nGenerating table report:\n"
    usage_get_all "$start" "$end" "table" 2>/dev/null || \
        demo_info "Table report requires database connection and customer setup"

    printf "\nGenerating JSON report:\n"
    usage_get_all "$start" "$end" "json" 2>/dev/null | head -20 || \
        demo_info "JSON report requires database connection and customer setup"

    printf "\nService-specific report (API):\n"
    usage_get_service "api" "$start" "$end" "table" 2>/dev/null || \
        demo_info "Service report requires database connection and customer setup"

    printf "\n${GREEN}✓ Reporting demo complete${NC}\n"
}

# ============================================================================
# Demo 5: Usage Alerts
# ============================================================================

demo_alerts() {
    demo_section "Demo 5: Usage Alerts"

    demo_info "Checking quota alerts..."

    printf "\nCurrent alert thresholds:\n"
    printf "  Warning:  ${USAGE_ALERT_WARNING}%%\n"
    printf "  Critical: ${USAGE_ALERT_CRITICAL}%%\n"
    printf "  Exceeded: ${USAGE_ALERT_EXCEEDED}%%\n"

    printf "\nChecking all service quotas:\n"
    usage_check_alerts 2>/dev/null || \
        demo_info "Alert checking requires database connection and customer setup"

    printf "\nRecent alerts (last 7 days):\n"
    usage_get_alerts 7 2>/dev/null || \
        demo_info "No alerts found"

    printf "\n${GREEN}✓ Alert demo complete${NC}\n"
}

# ============================================================================
# Demo 6: Export Functionality
# ============================================================================

demo_export() {
    demo_section "Demo 6: Export Functionality"

    demo_info "Exporting usage data to various formats..."

    local export_dir="/tmp/nself-usage-demo"
    mkdir -p "$export_dir"

    local today
    today=$(date +"%Y%m%d")

    printf "\nExporting as CSV:\n"
    local csv_file="${export_dir}/usage_${today}.csv"
    usage_export csv "$csv_file" "" "" "api" 2>/dev/null && \
        demo_success "CSV export: $csv_file" || \
        demo_info "CSV export requires database connection"

    printf "\nExporting as JSON:\n"
    local json_file="${export_dir}/usage_${today}.json"
    usage_export json "$json_file" "" "" "api" 2>/dev/null && \
        demo_success "JSON export: $json_file" || \
        demo_info "JSON export requires database connection"

    printf "\nExported files:\n"
    ls -lh "$export_dir" 2>/dev/null || printf "  (no files exported)\n"

    printf "\n${GREEN}✓ Export demo complete${NC}\n"
}

# ============================================================================
# Demo 7: Statistics & Analytics
# ============================================================================

demo_statistics() {
    demo_section "Demo 7: Statistics & Analytics"

    demo_info "Generating usage statistics..."

    printf "\nAPI usage statistics (last 30 days):\n"
    usage_get_stats "api" 30 2>/dev/null || \
        demo_info "Statistics require database connection and historical data"

    printf "\nUsage trends (last 7 days):\n"
    usage_get_trends "api" 7 2>/dev/null || \
        demo_info "Trends require database connection and historical data"

    printf "\nPeak usage periods (top 5):\n"
    usage_get_peaks "api" "hourly" 5 2>/dev/null || \
        demo_info "Peak detection requires database connection and historical data"

    printf "\n${GREEN}✓ Statistics demo complete${NC}\n"
}

# ============================================================================
# Demo 8: Cleanup & Maintenance
# ============================================================================

demo_cleanup() {
    demo_section "Demo 8: Cleanup & Maintenance"

    demo_info "Demonstrating cleanup operations..."

    printf "\nRefreshing materialized view:\n"
    usage_refresh_summary 2>/dev/null && \
        demo_success "Materialized view refreshed" || \
        demo_info "Refresh requires database connection"

    printf "\nArchiving old records (demo - would archive records >90 days):\n"
    demo_info "Command: usage_archive 90"
    demo_info "This would export old records to CSV and delete them from database"

    printf "\nCleaning up batch files:\n"
    usage_cleanup_batch
    demo_success "Batch files cleaned"

    printf "\n${GREEN}✓ Cleanup demo complete${NC}\n"
}

# ============================================================================
# Demo 9: Real-World Scenario - API Monitoring
# ============================================================================

demo_realworld_api_monitoring() {
    demo_section "Demo 9: Real-World Scenario - API Monitoring"

    demo_info "Simulating real API traffic monitoring..."

    local endpoints=(
        "/api/users:GET:200"
        "/api/users/:id:GET:200"
        "/api/posts:GET:200"
        "/api/posts:POST:201"
        "/api/comments:GET:200"
        "/api/auth/login:POST:200"
        "/api/auth/logout:POST:200"
        "/api/profile:GET:200"
        "/api/settings:PUT:200"
        "/api/invalid:GET:404"
    )

    printf "\nSimulating 50 API requests across different endpoints:\n"

    for i in {1..50}; do
        # Pick random endpoint
        local idx=$((RANDOM % ${#endpoints[@]}))
        local endpoint_data="${endpoints[$idx]}"

        IFS=':' read -r endpoint method status <<< "$endpoint_data"

        usage_track_api_request "$endpoint" "$method" "$status"

        printf "."

        # Small delay to simulate real traffic
        sleep 0.01
    done

    printf "\n"
    demo_success "Tracked 50 API requests"

    printf "\nEndpoint distribution:\n"
    for endpoint_data in "${endpoints[@]}"; do
        IFS=':' read -r endpoint method status <<< "$endpoint_data"
        printf "  ${method} ${endpoint} - ${status}\n"
    done

    printf "\n${GREEN}✓ API monitoring demo complete${NC}\n"
}

# ============================================================================
# Demo 10: Real-World Scenario - Storage Billing
# ============================================================================

demo_realworld_storage_billing() {
    demo_section "Demo 10: Real-World Scenario - Storage Billing"

    demo_info "Simulating storage usage calculation for billing..."

    # Simulate different storage tiers
    local small_files=1073741824      # 1GB
    local medium_files=10737418240    # 10GB
    local large_files=107374182400    # 100GB

    printf "\nTracking tiered storage usage:\n"

    printf "  Tier 1 (Hot): "
    usage_track_storage "$small_files" 720  # 30 days
    demo_success "1GB for 720 hours"

    printf "  Tier 2 (Warm): "
    usage_track_storage "$medium_files" 720
    demo_success "10GB for 720 hours"

    printf "  Tier 3 (Cold): "
    usage_track_storage "$large_files" 720
    demo_success "100GB for 720 hours"

    printf "\nTotal storage tracked: 111GB over 30 days\n"
    printf "Billing unit: GB-hours\n"

    local total_gb_hours
    total_gb_hours=$(awk "BEGIN {printf \"%.2f\", (1 + 10 + 100) * 720}")
    printf "Total GB-hours: %s\n" "$total_gb_hours"

    printf "\n${GREEN}✓ Storage billing demo complete${NC}\n"
}

# ============================================================================
# Main Demo Runner
# ============================================================================

main() {
    printf "\n"
    printf "╔════════════════════════════════════════════════════════════════╗\n"
    printf "║        nself Usage Tracking - Comprehensive Demo              ║\n"
    printf "║                                                                ║\n"
    printf "║  This demo showcases all major features of the usage          ║\n"
    printf "║  tracking system including tracking, aggregation, reporting,  ║\n"
    printf "║  alerts, exports, and real-world scenarios.                   ║\n"
    printf "╚════════════════════════════════════════════════════════════════╝\n"

    # Run demos
    demo_basic_tracking
    demo_batch_processing
    demo_aggregation
    demo_reporting
    demo_alerts
    demo_export
    demo_statistics
    demo_cleanup
    demo_realworld_api_monitoring
    demo_realworld_storage_billing

    # Summary
    printf "\n"
    printf "╔════════════════════════════════════════════════════════════════╗\n"
    printf "║                       DEMO COMPLETE                            ║\n"
    printf "╠════════════════════════════════════════════════════════════════╣\n"
    printf "║                                                                ║\n"
    printf "║  ${GREEN}✓${NC} All 10 demos executed successfully                        ║\n"
    printf "║                                                                ║\n"
    printf "║  For full documentation, see:                                 ║\n"
    printf "║  .wiki/guides/USAGE-TRACKING.md                                ║\n"
    printf "║                                                                ║\n"
    printf "╚════════════════════════════════════════════════════════════════╝\n"
    printf "\n"
}

# Run main demo
main
