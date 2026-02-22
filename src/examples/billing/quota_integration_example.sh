#!/usr/bin/env bash
#
# Billing Quotas Integration Examples
# Part of nself v0.9.0 - Sprint 13: Billing Integration & Usage Tracking
#
# This file demonstrates real-world integration patterns for the quota system.
#

set -euo pipefail

# Source the quota system
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../../src/lib/billing/quotas.sh"

# ============================================================================
# Example 1: API Middleware Integration
# ============================================================================

# HTTP request handler with quota checking
handle_api_request() {
    local method="$1"
    local endpoint="$2"
    local customer_id="$3"

    # Set customer context
    export NSELF_CUSTOMER_ID="$customer_id"

    # Fast quota check (uses cache)
    if ! quota_check_fast "api" 1 60; then
        # Quota exceeded
        local enforcement_mode
        enforcement_mode=$(billing_db_query "
            SELECT enforcement_mode FROM billing_quotas q
            JOIN billing_subscriptions s ON s.plan_name = q.plan_name
            WHERE s.customer_id = '$customer_id'
            AND s.status = 'active'
            AND q.service_name = 'api'
            LIMIT 1;
        " | tr -d ' ')

        if [[ "$enforcement_mode" == "hard" ]]; then
            # Hard limit - block request
            echo '{"error":"Quota exceeded","code":"QUOTA_EXCEEDED","status":429}'
            return 1  # Exit code (429 written to stdout as JSON)
        else
            # Soft limit - log warning but continue
            billing_log "QUOTA_WARNING" "api" "1" "{\"customer_id\":\"$customer_id\"}"
        fi
    fi

    # Rate limiting check (burst protection)
    if ! quota_check_rate_limited "api" 100; then
        echo '{"error":"Rate limited","code":"RATE_LIMITED","status":429}'
        return 1  # Exit code (429 written to stdout as JSON)
    fi

    # Process request
    echo '{"success":true,"data":"Request processed"}'
    return 0
}

# ============================================================================
# Example 2: Serverless Function Gating
# ============================================================================

# Execute serverless function with quota check
execute_function_with_quota() {
    local function_name="$1"
    local customer_id="$2"
    shift 2
    local args=("$@")

    export NSELF_CUSTOMER_ID="$customer_id"

    # Check function invocation quota
    if ! quota_check_fast "functions" 1 30; then
        echo "Error: Function invocation quota exceeded"
        echo "Current plan limits reached. Please upgrade."
        return 1
    fi

    # Execute function
    echo "Executing function: $function_name"
    # ... actual function execution here ...

    # Record usage (automatically tracked by billing system)
    billing_record_usage "functions" 1 "{\"function\":\"$function_name\"}"

    return 0
}

# ============================================================================
# Example 3: Storage Operations with Quota Check
# ============================================================================

# Upload file with storage quota check
upload_file_with_quota() {
    local file_path="$1"
    local customer_id="$2"

    export NSELF_CUSTOMER_ID="$customer_id"

    # Get file size
    local file_size
    file_size=$(stat -f%z "$file_path" 2>/dev/null || stat -c%s "$file_path" 2>/dev/null)

    # Convert to GB
    local gb_size
    gb_size=$(awk "BEGIN {printf \"%.6f\", $file_size / 1073741824}")

    echo "File size: ${gb_size}GB"

    # Check storage quota
    if ! quota_check_fast "storage" "$gb_size" 60; then
        echo "Error: Storage quota exceeded"
        echo "File size: ${gb_size}GB would exceed your quota"

        # Show current usage
        quota_get_service "storage" true "table"

        return 1
    fi

    # Upload file
    echo "Uploading file: $file_path"
    # ... actual upload logic here ...

    # Record usage
    billing_record_usage "storage" "$gb_size" "{\"file\":\"$(basename "$file_path")\",\"bytes\":$file_size}"

    echo "Upload successful"
    return 0
}

# ============================================================================
# Example 4: Bandwidth Monitoring
# ============================================================================

# Track bandwidth with quota check
track_bandwidth_usage() {
    local bytes_transferred="$1"
    local direction="${2:-egress}"  # egress or ingress
    local customer_id="$3"

    export NSELF_CUSTOMER_ID="$customer_id"

    # Convert to GB
    local gb_transferred
    gb_transferred=$(awk "BEGIN {printf \"%.6f\", $bytes_transferred / 1073741824}")

    # Check bandwidth quota
    if ! quota_check_fast "bandwidth" "$gb_transferred" 60; then
        echo "Warning: Bandwidth quota approaching limit"

        # Check if hard or soft limit
        local mode
        mode=$(billing_db_query "
            SELECT enforcement_mode FROM billing_quotas q
            JOIN billing_subscriptions s ON s.plan_name = q.plan_name
            WHERE s.customer_id = '$customer_id'
            AND s.status = 'active'
            AND q.service_name = 'bandwidth'
            LIMIT 1;
        " | tr -d ' ')

        if [[ "$mode" == "hard" ]]; then
            echo "Error: Bandwidth quota exceeded (hard limit)"
            return 1
        fi
    fi

    # Record bandwidth usage
    billing_record_usage "bandwidth" "$gb_transferred" "{\"direction\":\"$direction\",\"bytes\":$bytes_transferred}"

    return 0
}

# ============================================================================
# Example 5: Database Connection Pooling with Quota
# ============================================================================

# Check database connection quota
check_database_quota() {
    local customer_id="$1"
    local connections_requested="${2:-1}"

    export NSELF_CUSTOMER_ID="$customer_id"

    # Check database connection quota
    if ! quota_check_fast "database" "$connections_requested" 60; then
        echo "Error: Database connection quota exceeded"
        echo "Maximum connections reached for your plan"
        return 1
    fi

    echo "Database connections available"
    return 0
}

# ============================================================================
# Example 6: Proactive Quota Monitoring
# ============================================================================

# Monitor quota and alert when approaching limits
monitor_quota_proactively() {
    local customer_id="$1"

    export NSELF_CUSTOMER_ID="$customer_id"

    # Check for alerts
    local alerts
    alerts=$(quota_get_alerts "json")

    if [[ "$alerts" != "null" ]] && [[ -n "$alerts" ]]; then
        echo "⚠️ Quota Alerts Detected:"
        echo "$alerts" | jq -r '.[] | "  - \(.service): \(.percentage)% (\(.severity))"'

        # Send notifications
        quota_check_alerts "$customer_id" true

        return 1
    else
        echo "✓ All quotas within limits"
        return 0
    fi
}

# ============================================================================
# Example 7: Quota Reset at Billing Period End
# ============================================================================

# Automated quota reset (run via cron)
automated_quota_reset() {
    echo "Starting automated quota reset..."

    # Reset all expired quotas
    quota_reset_all_expired

    # Refresh materialized views
    billing_db_query "SELECT refresh_billing_usage_summary();" >/dev/null

    echo "Quota reset complete"
}

# ============================================================================
# Example 8: Display Customer Quota Dashboard
# ============================================================================

# Show comprehensive quota dashboard for customer
show_quota_dashboard() {
    local customer_id="$1"

    export NSELF_CUSTOMER_ID="$customer_id"

    echo "╔════════════════════════════════════════════════════════════════╗"
    echo "║                    QUOTA DASHBOARD                             ║"
    echo "╚════════════════════════════════════════════════════════════════╝"
    echo ""

    # Show all quotas with usage
    quota_get_all true "table"

    echo ""

    # Show alerts if any
    echo "Recent Alerts:"
    quota_get_alerts "table"

    echo ""

    # Show overage charges
    echo "Overage Charges:"
    quota_show_overage "$customer_id" "table"
}

# ============================================================================
# Example 9: Cache Warming for High-Traffic Customers
# ============================================================================

# Warm cache for top customers (run periodically)
warm_cache_for_top_customers() {
    echo "Warming quota cache for top customers..."

    # Get top 100 customers by usage
    local top_customers
    top_customers=$(billing_db_query "
        SELECT DISTINCT customer_id
        FROM billing_usage_records
        WHERE recorded_at >= NOW() - INTERVAL '7 days'
        GROUP BY customer_id
        ORDER BY SUM(quantity) DESC
        LIMIT 100;
    ")

    local count=0
    while IFS= read -r customer_id; do
        customer_id=$(echo "$customer_id" | tr -d ' ')
        [[ -z "$customer_id" ]] && continue

        quota_cache_warm "$customer_id"
        count=$((count + 1))
    done <<< "$top_customers"

    echo "Warmed cache for $count customers"
}

# ============================================================================
# Example 10: Quota-Based Feature Gating
# ============================================================================

# Check if feature is available based on quota
check_feature_availability() {
    local feature="$1"
    local customer_id="$2"

    export NSELF_CUSTOMER_ID="$customer_id"

    case "$feature" in
        "advanced_analytics")
            # Check if pro plan or higher
            local plan
            plan=$(billing_db_query "
                SELECT plan_name FROM billing_subscriptions
                WHERE customer_id = '$customer_id'
                AND status = 'active'
                LIMIT 1;
            " | tr -d ' ')

            if [[ "$plan" == "pro" ]] || [[ "$plan" == "enterprise" ]]; then
                echo "Feature available"
                return 0
            else
                echo "Feature requires Pro plan or higher"
                return 1
            fi
            ;;

        "unlimited_storage")
            # Check storage quota
            local storage_limit
            storage_limit=$(billing_db_query "
                SELECT limit_value FROM billing_quotas q
                JOIN billing_subscriptions s ON s.plan_name = q.plan_name
                WHERE s.customer_id = '$customer_id'
                AND s.status = 'active'
                AND q.service_name = 'storage'
                LIMIT 1;
            " | tr -d ' ')

            if [[ "$storage_limit" == "-1" ]]; then
                echo "Unlimited storage available"
                return 0
            else
                echo "Unlimited storage not available on current plan"
                return 1
            fi
            ;;

        *)
            echo "Unknown feature: $feature"
            return 1
            ;;
    esac
}

# ============================================================================
# Example Usage
# ============================================================================

main() {
    echo "Billing Quotas Integration Examples"
    echo "====================================="
    echo ""

    # Set example customer
    local customer_id="cust_example_123"

    echo "1. API Request with Quota Check"
    handle_api_request "GET" "/api/users" "$customer_id" || true
    echo ""

    echo "2. Check Feature Availability"
    check_feature_availability "advanced_analytics" "$customer_id" || true
    echo ""

    echo "3. Monitor Quota Proactively"
    monitor_quota_proactively "$customer_id" || true
    echo ""

    echo "4. Show Quota Dashboard"
    show_quota_dashboard "$customer_id" || true
    echo ""

    echo "All examples completed!"
}

# Run examples if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
