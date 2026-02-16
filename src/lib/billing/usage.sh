#!/usr/bin/env bash

#
# nself billing/usage.sh - Usage Metering and Tracking
# Part of nself v0.9.0 - Sprint 13: Billing Integration & Usage Tracking
#
# Track and report usage across all billable services: API requests, storage,
# bandwidth, compute time, database operations, and serverless functions.
#

# Prevent multiple sourcing
[[ -n "${NSELF_BILLING_USAGE_LOADED:-}" ]] && return 0

set -euo pipefail

NSELF_BILLING_USAGE_LOADED=1

# Source dependencies (namespaced to avoid clobbering caller's SCRIPT_DIR)
_BILLING_USAGE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${_BILLING_USAGE_DIR}/core.sh"

# Usage service definitions
declare -a USAGE_SERVICES=(
  "api"
  "storage"
  "bandwidth"
  "compute"
  "database"
  "functions"
)

# ============================================================================
# INPUT VALIDATION - Injection Prevention
# ============================================================================

# Validate service name against whitelist
validate_service_name() {
  local service="$1"

  # Check against known services
  for valid_svc in "${USAGE_SERVICES[@]}"; do
    if [[ "$service" == "$valid_svc" ]]; then
      return 0
    fi
  done

  error "Invalid service name: $service. Must be one of: ${USAGE_SERVICES[*]}"
  return 1
}

# Validate quantity is numeric and non-negative
validate_quantity() {
  local quantity="$1"

  # Must be numeric (integer or float)
  if ! [[ "$quantity" =~ ^[0-9]+(\.[0-9]+)?$ ]]; then
    error "Invalid quantity: $quantity. Must be a positive number"
    return 1
  fi

  # Must not be negative
  if (($(awk "BEGIN {print ($quantity < 0)}"))); then
    error "Invalid quantity: $quantity. Cannot be negative"
    return 1
  fi

  return 0
}

# Validate customer ID format (alphanumeric, hyphen, underscore only)
validate_customer_id() {
  local customer_id="$1"

  if [[ -z "$customer_id" ]]; then
    error "Customer ID cannot be empty"
    return 1
  fi

  if ! [[ "$customer_id" =~ ^[a-zA-Z0-9_-]+$ ]]; then
    error "Invalid customer ID format: $customer_id. Only alphanumeric, hyphen, and underscore allowed"
    return 1
  fi

  # Max length 255 characters
  if [[ ${#customer_id} -gt 255 ]]; then
    error "Customer ID too long: ${#customer_id} characters. Maximum 255"
    return 1
  fi

  return 0
}

# Validate date format (YYYY-MM-DD HH:MM:SS)
validate_date_format() {
  local date_str="$1"

  if ! [[ "$date_str" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}[[:space:]][0-9]{2}:[0-9]{2}:[0-9]{2}$ ]]; then
    error "Invalid date format: $date_str. Expected: YYYY-MM-DD HH:MM:SS"
    return 1
  fi

  return 0
}

# Validate period parameter
validate_period() {
  local period="$1"

  case "$period" in
    hourly | daily | monthly)
      return 0
      ;;
    *)
      error "Invalid period: $period. Must be hourly, daily, or monthly"
      return 1
      ;;
  esac
}

# Validate format parameter
validate_format() {
  local format="$1"

  case "$format" in
    json | csv | table | xlsx)
      return 0
      ;;
    *)
      error "Invalid format: $format. Must be json, csv, table, or xlsx"
      return 1
      ;;
  esac
}

# Sanitize metadata JSON for safe SQL injection
sanitize_metadata_json() {
  local metadata="$1"

  # Escape single quotes for SQL
  printf "%s" "$metadata" | sed "s/'/''/g"
}

# Validate metadata is valid JSON
validate_metadata() {
  local metadata="$1"

  if [[ -z "$metadata" ]]; then
    return 0 # Optional field
  fi

  # Try to parse with jq if available
  if command -v jq >/dev/null 2>&1; then
    if ! echo "$metadata" | jq empty 2>/dev/null; then
      error "Invalid metadata JSON: $metadata"
      return 1
    fi
  else
    # Basic JSON structure check
    if ! [[ "$metadata" =~ ^\{.*\}$ ]]; then
      error "Invalid metadata JSON: $metadata"
      return 1
    fi
  fi

  return 0
}

# Validate alert threshold value
validate_alert_threshold() {
  local threshold="$1"

  if ! [[ "$threshold" =~ ^[0-9]+$ ]]; then
    error "Invalid alert threshold: $threshold. Must be a whole number"
    return 1
  fi

  if [[ $threshold -lt 0 ]] || [[ $threshold -gt 100 ]]; then
    error "Invalid alert threshold: $threshold. Must be between 0 and 100"
    return 1
  fi

  return 0
}

# Validate limit parameter (for pagination)
validate_limit() {
  local limit="$1"
  local max_limit="${2:-1000}"

  if ! [[ "$limit" =~ ^[0-9]+$ ]]; then
    error "Invalid limit: $limit. Must be a positive integer"
    return 1
  fi

  if [[ $limit -lt 1 ]] || [[ $limit -gt $max_limit ]]; then
    error "Invalid limit: $limit. Must be between 1 and $max_limit"
    return 1
  fi

  return 0
}

# Batch processing configuration
USAGE_BATCH_SIZE="${USAGE_BATCH_SIZE:-100}"
USAGE_BATCH_FILE="${BILLING_CACHE_DIR}/usage_batch.tmp"
USAGE_BATCH_TIMEOUT="${USAGE_BATCH_TIMEOUT:-5}" # seconds

# Alert thresholds (percentage of quota)
USAGE_ALERT_WARNING="${USAGE_ALERT_WARNING:-75}"
USAGE_ALERT_CRITICAL="${USAGE_ALERT_CRITICAL:-90}"
USAGE_ALERT_EXCEEDED="${USAGE_ALERT_EXCEEDED:-100}"

# Initialize batch file
usage_init_batch() {
  mkdir -p "${BILLING_CACHE_DIR}"
  if [[ ! -f "$USAGE_BATCH_FILE" ]]; then
    printf "" >"$USAGE_BATCH_FILE"
  fi
}

# Get all usage for period
usage_get_all() {
  local start_date="$1"
  local end_date="$2"
  local format="${3:-table}"
  local detailed="${4:-false}"

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  case "$format" in
    json)
      usage_get_all_json "$customer_id" "$start_date" "$end_date" "$detailed"
      ;;
    csv)
      usage_get_all_csv "$customer_id" "$start_date" "$end_date" "$detailed"
      ;;
    table)
      usage_get_all_table "$customer_id" "$start_date" "$end_date" "$detailed"
      ;;
    *)
      error "Unsupported format: $format"
      return 1
      ;;
  esac
}

# Get usage for specific service
usage_get_service() {
  local service="$1"
  local start_date="$2"
  local end_date="$3"
  local format="${4:-table}"
  local detailed="${5:-false}"

  # Validate inputs
  validate_service_name "$service" || return 1
  validate_format "$format" || return 1

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  validate_customer_id "$customer_id" || return 1

  case "$format" in
    json)
      usage_get_service_json "$customer_id" "$service" "$start_date" "$end_date" "$detailed"
      ;;
    csv)
      usage_get_service_csv "$customer_id" "$service" "$start_date" "$end_date" "$detailed"
      ;;
    table)
      usage_get_service_table "$customer_id" "$service" "$start_date" "$end_date" "$detailed"
      ;;
    *)
      error "Unsupported format: $format"
      return 1
      ;;
  esac
}

# Display usage as table
usage_get_all_table() {
  local customer_id="$1"
  local start_date="$2"
  local end_date="$3"
  local detailed="$4"

  printf "╔════════════════════════════════════════════════════════════════╗\n"
  printf "║                      USAGE SUMMARY                             ║\n"
  printf "╠════════════════════════════════════════════════════════════════╣\n"
  printf "║ Period: %-54s ║\n" "${start_date} to ${end_date}"
  printf "╠════════════════════════════════════════════════════════════════╣\n"
  printf "║ Service          │ Usage          │ Unit       │ Cost        ║\n"
  printf "╠══════════════════╪════════════════╪════════════╪═════════════╣\n"

  local service
  for service in "${USAGE_SERVICES[@]}"; do
    local usage cost unit

    # Get usage data
    usage=$(billing_db_query "
            SELECT COALESCE(SUM(quantity), 0)
            FROM billing_usage_records
            WHERE customer_id = '${customer_id}'
            AND service_name = '${service}'
            AND recorded_at >= '${start_date}'
            AND recorded_at <= '${end_date}';
        " | tr -d ' ')

    # Get unit and cost from plan
    read -r unit cost <<<"$(usage_get_service_pricing "$service")"

    # Format numbers
    local usage_fmt
    usage_fmt=$(usage_format_number "$usage" "$service")

    local cost_fmt
    if [[ -n "$cost" ]] && [[ "$cost" != "0" ]]; then
      cost_fmt="\$$(usage_calculate_cost "$usage" "$cost")"
    else
      cost_fmt="Included"
    fi

    printf "║ %-16s │ %14s │ %-10s │ %11s ║\n" \
      "$service" "$usage_fmt" "$unit" "$cost_fmt"
  done

  printf "╚══════════════════╧════════════════╧════════════╧═════════════╝\n"

  if [[ "$detailed" == "true" ]]; then
    printf "\n"
    usage_show_detailed_breakdown "$customer_id" "$start_date" "$end_date"
  fi
}

# Display service usage as table
usage_get_service_table() {
  local customer_id="$1"
  local service="$2"
  local start_date="$3"
  local end_date="$4"
  local detailed="$5"

  # Get total usage
  local total_usage
  total_usage=$(billing_db_query "
        SELECT COALESCE(SUM(quantity), 0)
        FROM billing_usage_records
        WHERE customer_id = '${customer_id}'
        AND service_name = '${service}'
        AND recorded_at >= '${start_date}'
        AND recorded_at <= '${end_date}';
    " | tr -d ' ')

  # Get unit and pricing
  local unit cost
  read -r unit cost <<<"$(usage_get_service_pricing "$service")"

  printf "╔════════════════════════════════════════════════════════════════╗\n"
  printf "║ Service: %-54s ║\n" "$service"
  printf "╠════════════════════════════════════════════════════════════════╣\n"
  printf "║ Period: %-55s ║\n" "${start_date} to ${end_date}"
  printf "╠════════════════════════════════════════════════════════════════╣\n"
  printf "║                                                                ║\n"
  printf "║ Total Usage:  %48s ║\n" "$(usage_format_number "$total_usage" "$service") $unit"
  printf "║ Unit Cost:    %48s ║\n" "\$${cost} per ${unit}"
  printf "║ Total Cost:   %48s ║\n" "\$$(usage_calculate_cost "$total_usage" "$cost")"
  printf "║                                                                ║\n"
  printf "╚════════════════════════════════════════════════════════════════╝\n"

  if [[ "$detailed" == "true" ]]; then
    printf "\n"
    usage_show_service_timeline "$customer_id" "$service" "$start_date" "$end_date"
  fi
}

# Export usage as JSON
usage_get_all_json() {
  local customer_id="$1"
  local start_date="$2"
  local end_date="$3"
  local detailed="$4"

  if [[ "$detailed" == "true" ]]; then
    billing_db_query "
            SELECT json_build_object(
                'period', json_build_object(
                    'start', '${start_date}',
                    'end', '${end_date}'
                ),
                'services', (
                    SELECT json_agg(
                        json_build_object(
                            'service', service_name,
                            'usage', total_usage,
                            'unit', unit,
                            'cost', cost,
                            'daily_breakdown', daily_data
                        )
                    )
                    FROM (
                        SELECT
                            service_name,
                            SUM(quantity) as total_usage,
                            'requests' as unit,
                            0 as cost,
                            json_agg(json_build_object(
                                'date', DATE(recorded_at),
                                'usage', quantity
                            )) as daily_data
                        FROM billing_usage_records
                        WHERE customer_id = '${customer_id}'
                        AND recorded_at >= '${start_date}'
                        AND recorded_at <= '${end_date}'
                        GROUP BY service_name, DATE(recorded_at)
                    ) s
                    GROUP BY service_name
                )
            );
        "
  else
    billing_db_query "
            SELECT json_agg(
                json_build_object(
                    'service', service_name,
                    'usage', total_usage
                )
            )
            FROM (
                SELECT
                    service_name,
                    SUM(quantity) as total_usage
                FROM billing_usage_records
                WHERE customer_id = '${customer_id}'
                AND recorded_at >= '${start_date}'
                AND recorded_at <= '${end_date}'
                GROUP BY service_name
            ) s;
        "
  fi
}

# Export usage as CSV
usage_get_all_csv() {
  local customer_id="$1"
  local start_date="$2"
  local end_date="$3"
  local detailed="$4"

  if [[ "$detailed" == "true" ]]; then
    billing_db_query "
            SELECT
                DATE(recorded_at) as date,
                service_name,
                SUM(quantity) as usage,
                COUNT(*) as events
            FROM billing_usage_records
            WHERE customer_id = '${customer_id}'
            AND recorded_at >= '${start_date}'
            AND recorded_at <= '${end_date}'
            GROUP BY DATE(recorded_at), service_name
            ORDER BY date DESC, service_name;
        " csv
  else
    billing_db_query "
            SELECT
                service_name,
                SUM(quantity) as total_usage,
                COUNT(*) as total_events
            FROM billing_usage_records
            WHERE customer_id = '${customer_id}'
            AND recorded_at >= '${start_date}'
            AND recorded_at <= '${end_date}'
            GROUP BY service_name
            ORDER BY service_name;
        " csv
  fi
}

# Export service usage as JSON
usage_get_service_json() {
  local customer_id="$1"
  local service="$2"
  local start_date="$3"
  local end_date="$4"
  local detailed="$5"

  billing_db_query "
        SELECT json_build_object(
            'service', '${service}',
            'period', json_build_object(
                'start', '${start_date}',
                'end', '${end_date}'
            ),
            'total_usage', COALESCE(SUM(quantity), 0),
            'total_events', COUNT(*),
            'daily_breakdown', json_agg(
                json_build_object(
                    'date', DATE(recorded_at),
                    'usage', quantity,
                    'metadata', metadata
                )
            )
        )
        FROM billing_usage_records
        WHERE customer_id = '${customer_id}'
        AND service_name = '${service}'
        AND recorded_at >= '${start_date}'
        AND recorded_at <= '${end_date}';
    "
}

# Export service usage as CSV
usage_get_service_csv() {
  local customer_id="$1"
  local service="$2"
  local start_date="$3"
  local end_date="$4"
  local detailed="$5"

  billing_db_query "
        SELECT
            recorded_at,
            quantity,
            metadata
        FROM billing_usage_records
        WHERE customer_id = '${customer_id}'
        AND service_name = '${service}'
        AND recorded_at >= '${start_date}'
        AND recorded_at <= '${end_date}'
        ORDER BY recorded_at DESC;
    " csv
}

# Show detailed usage breakdown
usage_show_detailed_breakdown() {
  local customer_id="$1"
  local start_date="$2"
  local end_date="$3"

  printf "Detailed Daily Breakdown:\n"
  printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"

  billing_db_query "
        SELECT
            TO_CHAR(DATE(recorded_at), 'YYYY-MM-DD') as date,
            service_name,
            SUM(quantity) as daily_usage
        FROM billing_usage_records
        WHERE customer_id = '${customer_id}'
        AND recorded_at >= '${start_date}'
        AND recorded_at <= '${end_date}'
        GROUP BY DATE(recorded_at), service_name
        ORDER BY DATE(recorded_at) DESC, service_name;
    " | while IFS='|' read -r date service usage; do
    # Trim whitespace
    date=$(printf "%s" "$date" | tr -d ' ')
    service=$(printf "%s" "$service" | tr -d ' ')
    usage=$(printf "%s" "$usage" | tr -d ' ')

    printf "  %s  │  %-12s  │  %s\n" "$date" "$service" "$usage"
  done

  printf "\n"
}

# Show service timeline
usage_show_service_timeline() {
  local customer_id="$1"
  local service="$2"
  local start_date="$3"
  local end_date="$4"

  printf "Daily Timeline:\n"
  printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n"

  billing_db_query "
        SELECT
            TO_CHAR(DATE(recorded_at), 'YYYY-MM-DD') as date,
            SUM(quantity) as daily_usage,
            COUNT(*) as event_count
        FROM billing_usage_records
        WHERE customer_id = '${customer_id}'
        AND service_name = '${service}'
        AND recorded_at >= '${start_date}'
        AND recorded_at <= '${end_date}'
        GROUP BY DATE(recorded_at)
        ORDER BY DATE(recorded_at) DESC;
    " | while IFS='|' read -r date usage events; do
    # Trim whitespace
    date=$(printf "%s" "$date" | tr -d ' ')
    usage=$(printf "%s" "$usage" | tr -d ' ')
    events=$(printf "%s" "$events" | tr -d ' ')

    # Create simple bar chart
    local bar
    bar=$(usage_create_bar "$usage" 50)

    printf "  %s  │  %10s  │  %s events  %s\n" \
      "$date" "$usage" "$events" "$bar"
  done

  printf "\n"
}

# Get service pricing info
usage_get_service_pricing() {
  local service="$1"

  # Default pricing (can be overridden by plan)
  case "$service" in
    api)
      printf "requests 0.0001"
      ;;
    storage)
      printf "GB-hours 0.001"
      ;;
    bandwidth)
      printf "GB 0.01"
      ;;
    compute)
      printf "CPU-hours 0.05"
      ;;
    database)
      printf "queries 0.00001"
      ;;
    functions)
      printf "invocations 0.0002"
      ;;
    *)
      printf "units 0"
      ;;
  esac
}

# Format usage number based on service type
usage_format_number() {
  local number="$1"
  local service="$2"

  if [[ $number -gt 1000000 ]]; then
    printf "%.2fM" "$(awk "BEGIN {print $number/1000000}")"
  elif [[ $number -gt 1000 ]]; then
    printf "%.2fK" "$(awk "BEGIN {print $number/1000}")"
  else
    printf "%s" "$number"
  fi
}

# Calculate cost
usage_calculate_cost() {
  local usage="$1"
  local unit_cost="$2"

  awk "BEGIN {printf \"%.2f\", $usage * $unit_cost}"
}

# Create simple bar chart
usage_create_bar() {
  local value="$1"
  local max_width="${2:-50}"

  # Find max value for scaling (simple approach)
  local bar_length
  bar_length=$((value > max_width ? max_width : value))

  printf "["
  local i
  for ((i = 0; i < bar_length; i++)); do
    printf "▓"
  done
  for ((i = bar_length; i < max_width; i++)); do
    printf "░"
  done
  printf "]"
}

# ============================================================================
# BATCH PROCESSING - High-Volume Write Optimization
# ============================================================================

# Add usage record to batch queue
usage_batch_add() {
  local customer_id="$1"
  local service="$2"
  local quantity="$3"
  local metadata="${4:-{}}"
  local timestamp="${5:-$(date -u +"%Y-%m-%d %H:%M:%S")}"

  # Validate all inputs before processing
  validate_customer_id "$customer_id" || return 1
  validate_service_name "$service" || return 1
  validate_quantity "$quantity" || return 1
  validate_metadata "$metadata" || return 1

  if [[ -n "$timestamp" ]]; then
    validate_date_format "$timestamp" || return 1
  fi

  usage_init_batch

  # Escape single quotes in metadata for SQL (prevent SQL injection)
  local safe_metadata
  safe_metadata=$(sanitize_metadata_json "$metadata")

  # Append to batch file (CSV format for fast COPY)
  printf "%s,%s,%s,%s,'%s','%s'\n" \
    "$customer_id" "$service" "$quantity" "0.00" "$safe_metadata" "$timestamp" \
    >>"$USAGE_BATCH_FILE"

  # Check if batch size reached
  local batch_count
  batch_count=$(wc -l <"$USAGE_BATCH_FILE" 2>/dev/null || echo 0)

  if [[ $batch_count -ge $USAGE_BATCH_SIZE ]]; then
    usage_batch_flush
  fi
}

# Flush batch to database using COPY for maximum performance
usage_batch_flush() {
  if [[ ! -f "$USAGE_BATCH_FILE" ]] || [[ ! -s "$USAGE_BATCH_FILE" ]]; then
    return 0 # Nothing to flush
  fi

  # Use PostgreSQL COPY for bulk insert (much faster than individual INSERTs)
  PGPASSWORD="$BILLING_DB_PASSWORD" psql \
    -h "$BILLING_DB_HOST" \
    -p "$BILLING_DB_PORT" \
    -U "$BILLING_DB_USER" \
    -d "$BILLING_DB_NAME" \
    -c "COPY billing_usage_records (customer_id, service_name, quantity, unit_cost, metadata, recorded_at) FROM STDIN WITH (FORMAT CSV, DELIMITER ',');" \
    <"$USAGE_BATCH_FILE" 2>/dev/null

  # Clear batch file
  printf "" >"$USAGE_BATCH_FILE"

  return 0
}

# Batch insert multiple usage records at once
usage_batch_insert() {
  local -a records=("$@")

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    warn "No customer ID - usage not recorded"
    return 1
  }

  usage_init_batch

  # Build VALUES clause for batch insert
  local values_clause=""
  local timestamp
  timestamp=$(date -u +"%Y-%m-%d %H:%M:%S")

  local record
  for record in "${records[@]}"; do
    # Record format: "service:quantity:metadata"
    IFS=':' read -r service quantity metadata <<<"$record"

    # Escape single quotes
    local safe_metadata
    safe_metadata=$(printf "%s" "${metadata:-{}}" | sed "s/'/''/g")

    if [[ -n "$values_clause" ]]; then
      values_clause+=","
    fi

    values_clause+="('${customer_id}','${service}',${quantity},0.00,'${safe_metadata}','${timestamp}')"
  done

  if [[ -z "$values_clause" ]]; then
    return 0
  fi

  # Single INSERT with multiple rows
  billing_db_query "
        INSERT INTO billing_usage_records
            (customer_id, service_name, quantity, unit_cost, metadata, recorded_at)
        VALUES ${values_clause};
    " >/dev/null

  return 0
}

# ============================================================================
# USAGE AGGREGATION - Hourly, Daily, Monthly
# ============================================================================

# Aggregate usage for a specific period
usage_aggregate() {
  local period="${1:-daily}" # hourly, daily, monthly
  local customer_id="$2"
  local start_date="$3"
  local end_date="$4"

  # Validate period parameter
  validate_period "$period" || return 1

  if [[ -z "$customer_id" ]]; then
    customer_id=$(billing_get_customer_id) || {
      error "No customer ID found"
      return 1
    }
  fi

  # Validate customer ID
  validate_customer_id "$customer_id" || return 1

  case "$period" in
    hourly)
      usage_aggregate_hourly "$customer_id" "$start_date" "$end_date"
      ;;
    daily)
      usage_aggregate_daily "$customer_id" "$start_date" "$end_date"
      ;;
    monthly)
      usage_aggregate_monthly "$customer_id" "$start_date" "$end_date"
      ;;
    *)
      error "Invalid period: $period. Use hourly, daily, or monthly"
      return 1
      ;;
  esac
}

# Aggregate hourly usage
usage_aggregate_hourly() {
  local customer_id="$1"
  local start_date="$2"
  local end_date="$3"

  local where_clause="customer_id = '${customer_id}'"
  if [[ -n "$start_date" ]]; then
    where_clause+=" AND recorded_at >= '${start_date}'"
  fi
  if [[ -n "$end_date" ]]; then
    where_clause+=" AND recorded_at <= '${end_date}'"
  fi

  billing_db_query "
        SELECT
            service_name,
            DATE_TRUNC('hour', recorded_at) as hour,
            COUNT(*) as event_count,
            SUM(quantity) as total_quantity,
            SUM(quantity * unit_cost) as total_cost,
            AVG(quantity) as avg_quantity,
            MIN(quantity) as min_quantity,
            MAX(quantity) as max_quantity
        FROM billing_usage_records
        WHERE ${where_clause}
        GROUP BY service_name, DATE_TRUNC('hour', recorded_at)
        ORDER BY hour DESC, service_name;
    "
}

# Aggregate daily usage
usage_aggregate_daily() {
  local customer_id="$1"
  local start_date="$2"
  local end_date="$3"

  local where_clause="customer_id = '${customer_id}'"
  if [[ -n "$start_date" ]]; then
    where_clause+=" AND recorded_at >= '${start_date}'"
  fi
  if [[ -n "$end_date" ]]; then
    where_clause+=" AND recorded_at <= '${end_date}'"
  fi

  billing_db_query "
        SELECT
            service_name,
            DATE(recorded_at) as date,
            COUNT(*) as event_count,
            SUM(quantity) as total_quantity,
            SUM(quantity * unit_cost) as total_cost,
            AVG(quantity) as avg_quantity,
            MIN(quantity) as min_quantity,
            MAX(quantity) as max_quantity
        FROM billing_usage_records
        WHERE ${where_clause}
        GROUP BY service_name, DATE(recorded_at)
        ORDER BY date DESC, service_name;
    "
}

# Aggregate monthly usage
usage_aggregate_monthly() {
  local customer_id="$1"
  local start_date="$2"
  local end_date="$3"

  local where_clause="customer_id = '${customer_id}'"
  if [[ -n "$start_date" ]]; then
    where_clause+=" AND recorded_at >= '${start_date}'"
  fi
  if [[ -n "$end_date" ]]; then
    where_clause+=" AND recorded_at <= '${end_date}'"
  fi

  billing_db_query "
        SELECT
            service_name,
            TO_CHAR(recorded_at, 'YYYY-MM') as month,
            COUNT(*) as event_count,
            SUM(quantity) as total_quantity,
            SUM(quantity * unit_cost) as total_cost,
            AVG(quantity) as avg_quantity,
            MIN(quantity) as min_quantity,
            MAX(quantity) as max_quantity
        FROM billing_usage_records
        WHERE ${where_clause}
        GROUP BY service_name, TO_CHAR(recorded_at, 'YYYY-MM')
        ORDER BY month DESC, service_name;
    "
}

# Refresh materialized view for faster aggregations
usage_refresh_summary() {
  billing_db_query "SELECT refresh_billing_usage_summary();" >/dev/null
  success "Usage summary refreshed"
}

# ============================================================================
# USAGE ALERTS - Threshold Monitoring
# ============================================================================

# Check usage against quotas and trigger alerts
usage_check_alerts() {
  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  local service
  for service in "${USAGE_SERVICES[@]}"; do
    usage_check_service_alert "$customer_id" "$service"
  done
}

# Check alert status for specific service
usage_check_service_alert() {
  local customer_id="$1"
  local service="$2"

  # Get quota and usage
  local quota_data
  quota_data=$(billing_db_query "
        SELECT
            q.limit_value,
            q.enforcement_mode,
            COALESCE(SUM(ur.quantity), 0) as usage
        FROM billing_quotas q
        JOIN billing_subscriptions s ON s.plan_name = q.plan_name
        LEFT JOIN billing_usage_records ur ON
            ur.customer_id = s.customer_id
            AND ur.service_name = q.service_name
            AND ur.recorded_at >= s.current_period_start
            AND ur.recorded_at <= s.current_period_end
        WHERE s.customer_id = '${customer_id}'
        AND s.status = 'active'
        AND q.service_name = '${service}'
        GROUP BY q.limit_value, q.enforcement_mode
        LIMIT 1;
    ")

  if [[ -z "$quota_data" ]]; then
    return 0 # No quota configured
  fi

  local limit mode usage
  IFS='|' read -r limit mode usage <<<"$quota_data"

  # Trim whitespace
  limit=$(printf "%s" "$limit" | tr -d ' ')
  mode=$(printf "%s" "$mode" | tr -d ' ')
  usage=$(printf "%s" "$usage" | tr -d ' ')

  # Skip unlimited quotas
  if [[ "$limit" == "-1" ]]; then
    return 0
  fi

  # Calculate percentage
  local percent=0
  if [[ $limit -gt 0 ]]; then
    percent=$(awk "BEGIN {printf \"%.0f\", ($usage * 100 / $limit)}")
  fi

  # Trigger alerts based on thresholds
  if [[ $percent -ge $USAGE_ALERT_EXCEEDED ]]; then
    usage_trigger_alert "$customer_id" "$service" "exceeded" "$usage" "$limit" "$percent"
  elif [[ $percent -ge $USAGE_ALERT_CRITICAL ]]; then
    usage_trigger_alert "$customer_id" "$service" "critical" "$usage" "$limit" "$percent"
  elif [[ $percent -ge $USAGE_ALERT_WARNING ]]; then
    usage_trigger_alert "$customer_id" "$service" "warning" "$usage" "$limit" "$percent"
  fi
}

# Trigger usage alert
usage_trigger_alert() {
  local customer_id="$1"
  local service="$2"
  local level="$3" # warning, critical, exceeded
  local usage="$4"
  local limit="$5"
  local percent="$6"

  local timestamp
  timestamp=$(date -u +"%Y-%m-%d %H:%M:%S")

  # Log alert
  billing_log "ALERT" "$service" "${level}:${percent}%" \
    "usage=${usage},limit=${limit}"

  # Store alert in database (optional - could add billing_alerts table)
  # For now, just log to file and output
  local alert_file="${BILLING_DATA_DIR}/alerts.log"
  printf "[%s] %s ALERT for %s: %s/%s (%s%%)\n" \
    "$timestamp" "$level" "$service" "$usage" "$limit" "$percent" \
    >>"$alert_file"

  # Output to stderr for real-time monitoring
  case "$level" in
    exceeded)
      error "QUOTA EXCEEDED: $service - ${usage}/${limit} (${percent}%)"
      ;;
    critical)
      warn "CRITICAL: $service usage at ${percent}% - ${usage}/${limit}"
      ;;
    warning)
      warn "WARNING: $service usage at ${percent}% - ${usage}/${limit}"
      ;;
  esac
}

# Get alert history
usage_get_alerts() {
  local days="${1:-7}"
  local alert_file="${BILLING_DATA_DIR}/alerts.log"

  if [[ ! -f "$alert_file" ]]; then
    printf "No alerts found\n"
    return 0
  fi

  # Get alerts from last N days
  local cutoff_date
  if date -v-${days}d >/dev/null 2>&1; then
    # macOS
    cutoff_date=$(date -v-${days}d +"%Y-%m-%d")
  else
    # Linux
    cutoff_date=$(date -d "${days} days ago" +"%Y-%m-%d")
  fi

  grep -E "^\[${cutoff_date}" "$alert_file" 2>/dev/null || true
}

# ============================================================================
# SERVICE-SPECIFIC TRACKING FUNCTIONS
# ============================================================================

# Track API request
usage_track_api_request() {
  local endpoint="$1"
  local method="${2:-GET}"
  local status_code="${3:-200}"

  local metadata
  metadata=$(printf '{"endpoint":"%s","method":"%s","status":%d}' \
    "$endpoint" "$method" "$status_code")

  billing_record_usage "api" 1 "$metadata"
}

# Track storage usage
usage_track_storage() {
  local bytes="$1"
  local duration_hours="${2:-1}"

  # Convert to GB-hours
  local gb_hours
  gb_hours=$(awk "BEGIN {printf \"%.6f\", ($bytes / 1073741824) * $duration_hours}")

  billing_record_usage "storage" "$gb_hours" "{\"bytes\":$bytes,\"hours\":$duration_hours}"
}

# Track bandwidth
usage_track_bandwidth() {
  local bytes="$1"
  local direction="${2:-egress}" # egress or ingress

  # Convert to GB
  local gb
  gb=$(awk "BEGIN {printf \"%.6f\", $bytes / 1073741824}")

  billing_record_usage "bandwidth" "$gb" "{\"bytes\":$bytes,\"direction\":\"$direction\"}"
}

# Track compute time
usage_track_compute() {
  local cpu_seconds="$1"
  local metadata="${2:-{}}"

  # Convert to CPU-hours
  local cpu_hours
  cpu_hours=$(awk "BEGIN {printf \"%.6f\", $cpu_seconds / 3600}")

  billing_record_usage "compute" "$cpu_hours" "$metadata"
}

# Track database query
usage_track_database_query() {
  local query_type="${1:-SELECT}"
  local duration_ms="${2:-0}"

  local metadata
  metadata=$(printf '{"type":"%s","duration_ms":%d}' "$query_type" "$duration_ms")

  billing_record_usage "database" 1 "$metadata"
}

# Track function invocation
usage_track_function() {
  local function_name="$1"
  local duration_ms="${2:-0}"
  local memory_mb="${3:-128}"

  local metadata
  metadata=$(printf '{"function":"%s","duration_ms":%d,"memory_mb":%d}' \
    "$function_name" "$duration_ms" "$memory_mb")

  billing_record_usage "functions" 1 "$metadata"
}

# ============================================================================
# ENHANCED EXPORT FUNCTIONALITY
# ============================================================================

# Export usage to file with proper formatting
usage_export() {
  local format="${1:-csv}"
  local output_file="${2}"
  local start_date="${3}"
  local end_date="${4}"
  local service="${5:-all}"

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  # Default output file
  if [[ -z "$output_file" ]]; then
    local timestamp
    timestamp=$(date +"%Y%m%d_%H%M%S")
    output_file="${BILLING_EXPORT_DIR}/usage_${timestamp}.${format}"
  fi

  # Ensure export directory exists
  mkdir -p "$(dirname "$output_file")"

  case "$format" in
    csv)
      usage_export_csv "$customer_id" "$output_file" "$start_date" "$end_date" "$service"
      ;;
    json)
      usage_export_json "$customer_id" "$output_file" "$start_date" "$end_date" "$service"
      ;;
    xlsx)
      usage_export_xlsx "$customer_id" "$output_file" "$start_date" "$end_date" "$service"
      ;;
    *)
      error "Unsupported format: $format. Use csv, json, or xlsx"
      return 1
      ;;
  esac

  success "Usage exported to: $output_file"
  printf "%s" "$output_file"
}

# Export to CSV with headers
usage_export_csv() {
  local customer_id="$1"
  local output_file="$2"
  local start_date="$3"
  local end_date="$4"
  local service="$5"

  local where_clause="customer_id = '${customer_id}'"
  if [[ -n "$start_date" ]]; then
    where_clause+=" AND recorded_at >= '${start_date}'"
  fi
  if [[ -n "$end_date" ]]; then
    where_clause+=" AND recorded_at <= '${end_date}'"
  fi
  if [[ "$service" != "all" ]] && [[ -n "$service" ]]; then
    where_clause+=" AND service_name = '${service}'"
  fi

  # Write with headers
  {
    printf "timestamp,service,quantity,unit_cost,total_cost,metadata\n"
    billing_db_query "
            SELECT
                recorded_at,
                service_name,
                quantity,
                unit_cost,
                (quantity * unit_cost) as total_cost,
                metadata::text
            FROM billing_usage_records
            WHERE ${where_clause}
            ORDER BY recorded_at DESC;
        " csv
  } >"$output_file"
}

# Export to JSON
usage_export_json() {
  local customer_id="$1"
  local output_file="$2"
  local start_date="$3"
  local end_date="$4"
  local service="$5"

  local where_clause="customer_id = '${customer_id}'"
  if [[ -n "$start_date" ]]; then
    where_clause+=" AND recorded_at >= '${start_date}'"
  fi
  if [[ -n "$end_date" ]]; then
    where_clause+=" AND recorded_at <= '${end_date}'"
  fi
  if [[ "$service" != "all" ]] && [[ -n "$service" ]]; then
    where_clause+=" AND service_name = '${service}'"
  fi

  billing_db_query "
        SELECT json_build_object(
            'customer_id', '${customer_id}',
            'export_date', NOW(),
            'period', json_build_object(
                'start', '${start_date}',
                'end', '${end_date}'
            ),
            'usage_records', (
                SELECT json_agg(
                    json_build_object(
                        'timestamp', recorded_at,
                        'service', service_name,
                        'quantity', quantity,
                        'unit_cost', unit_cost,
                        'total_cost', quantity * unit_cost,
                        'metadata', metadata
                    )
                )
                FROM billing_usage_records
                WHERE ${where_clause}
                ORDER BY recorded_at DESC
            ),
            'summary', (
                SELECT json_build_object(
                    'total_events', COUNT(*),
                    'total_cost', SUM(quantity * unit_cost),
                    'services', json_agg(DISTINCT service_name)
                )
                FROM billing_usage_records
                WHERE ${where_clause}
            )
        );
    " >"$output_file"
}

# Export to XLSX (requires csvkit or similar)
usage_export_xlsx() {
  local customer_id="$1"
  local output_file="$2"
  local start_date="$3"
  local end_date="$4"
  local service="$5"

  # First export as CSV
  local temp_csv="${output_file%.xlsx}.tmp.csv"
  usage_export_csv "$customer_id" "$temp_csv" "$start_date" "$end_date" "$service"

  # Convert to XLSX if csvkit is available
  if command -v csvformat >/dev/null 2>&1; then
    # Use csvkit to convert
    csvformat "$temp_csv" >"$output_file"
    rm -f "$temp_csv"
  elif command -v xlsx2csv >/dev/null 2>&1; then
    # Alternative: use xlsx2csv in reverse
    warn "XLSX export requires csvkit. Falling back to CSV."
    mv "$temp_csv" "${output_file%.xlsx}.csv"
  else
    warn "XLSX export not available. Saved as CSV instead."
    mv "$temp_csv" "${output_file%.xlsx}.csv"
  fi
}

# ============================================================================
# USAGE STATISTICS AND ANALYTICS
# ============================================================================

# Get usage statistics for a service
usage_get_stats() {
  local service="$1"
  local period="${2:-30}" # days

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  # Calculate date range
  local start_date
  if date -v-${period}d >/dev/null 2>&1; then
    # macOS
    start_date=$(date -v-${period}d -u +"%Y-%m-%d %H:%M:%S")
  else
    # Linux
    start_date=$(date -d "${period} days ago" -u +"%Y-%m-%d %H:%M:%S")
  fi

  billing_db_query "
        SELECT
            service_name,
            COUNT(*) as total_events,
            SUM(quantity) as total_quantity,
            AVG(quantity) as avg_quantity,
            MIN(quantity) as min_quantity,
            MAX(quantity) as max_quantity,
            PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY quantity) as median_quantity,
            PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY quantity) as p95_quantity,
            PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY quantity) as p99_quantity
        FROM billing_usage_records
        WHERE customer_id = '${customer_id}'
        AND service_name = '${service}'
        AND recorded_at >= '${start_date}'
        GROUP BY service_name;
    "
}

# Get usage trends (day-over-day comparison)
usage_get_trends() {
  local service="${1:-all}"
  local days="${2:-7}"

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  local service_filter=""
  if [[ "$service" != "all" ]]; then
    service_filter="AND service_name = '${service}'"
  fi

  billing_db_query "
        WITH daily_usage AS (
            SELECT
                DATE(recorded_at) as date,
                service_name,
                SUM(quantity) as daily_total,
                LAG(SUM(quantity)) OVER (
                    PARTITION BY service_name
                    ORDER BY DATE(recorded_at)
                ) as previous_day
            FROM billing_usage_records
            WHERE customer_id = '${customer_id}'
            ${service_filter}
            AND recorded_at >= NOW() - INTERVAL '${days} days'
            GROUP BY DATE(recorded_at), service_name
        )
        SELECT
            date,
            service_name,
            daily_total,
            previous_day,
            CASE
                WHEN previous_day > 0 THEN
                    ROUND(((daily_total - previous_day) / previous_day * 100)::numeric, 2)
                ELSE 0
            END as percent_change
        FROM daily_usage
        ORDER BY date DESC, service_name;
    "
}

# Get top usage periods
usage_get_peaks() {
  local service="$1"
  local period="${2:-hourly}" # hourly or daily
  local limit="${3:-10}"

  # Validate all inputs
  validate_service_name "$service" || return 1
  validate_period "$period" || return 1
  validate_limit "$limit" 10000 || return 1

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  validate_customer_id "$customer_id" || return 1

  local trunc_period
  case "$period" in
    hourly) trunc_period="hour" ;;
    daily) trunc_period="day" ;;
    *)
      error "Invalid period: $period"
      return 1
      ;;
  esac

  billing_db_query "
        SELECT
            DATE_TRUNC('${trunc_period}', recorded_at) as period,
            SUM(quantity) as total_usage,
            COUNT(*) as event_count
        FROM billing_usage_records
        WHERE customer_id = '${customer_id}'
        AND service_name = '${service}'
        GROUP BY DATE_TRUNC('${trunc_period}', recorded_at)
        ORDER BY total_usage DESC
        LIMIT ${limit};
    "
}

# ============================================================================
# CLEANUP AND MAINTENANCE
# ============================================================================

# Archive old usage records
usage_archive() {
  local days_to_keep="${1:-90}"
  local archive_file="${2}"

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  # Calculate cutoff date
  local cutoff_date
  if date -v-${days_to_keep}d >/dev/null 2>&1; then
    # macOS
    cutoff_date=$(date -v-${days_to_keep}d -u +"%Y-%m-%d")
  else
    # Linux
    cutoff_date=$(date -d "${days_to_keep} days ago" -u +"%Y-%m-%d")
  fi

  # Default archive file
  if [[ -z "$archive_file" ]]; then
    local timestamp
    timestamp=$(date +"%Y%m%d")
    archive_file="${BILLING_DATA_DIR}/archive/usage_${timestamp}.csv"
  fi

  mkdir -p "$(dirname "$archive_file")"

  # Export old records to archive
  billing_db_query "
        SELECT
            recorded_at,
            service_name,
            quantity,
            unit_cost,
            metadata
        FROM billing_usage_records
        WHERE customer_id = '${customer_id}'
        AND recorded_at < '${cutoff_date}'
        ORDER BY recorded_at;
    " csv >"$archive_file"

  local archived_count
  archived_count=$(wc -l <"$archive_file" 2>/dev/null || echo 0)
  archived_count=$((archived_count - 1)) # Subtract header

  if [[ $archived_count -gt 0 ]]; then
    # Delete archived records
    billing_db_query "
            DELETE FROM billing_usage_records
            WHERE customer_id = '${customer_id}'
            AND recorded_at < '${cutoff_date}';
        " >/dev/null

    success "Archived $archived_count records to: $archive_file"
  else
    printf "No records to archive\n"
    rm -f "$archive_file"
  fi
}

# Cleanup batch files
usage_cleanup_batch() {
  usage_batch_flush # Flush any pending
  if [[ -f "$USAGE_BATCH_FILE" ]]; then
    rm -f "$USAGE_BATCH_FILE"
  fi
}

# Export functions
export -f usage_init_batch
export -f usage_batch_add
export -f usage_batch_flush
export -f usage_batch_insert
export -f usage_aggregate
export -f usage_aggregate_hourly
export -f usage_aggregate_daily
export -f usage_aggregate_monthly
export -f usage_refresh_summary
export -f usage_check_alerts
export -f usage_check_service_alert
export -f usage_trigger_alert
export -f usage_get_alerts
export -f usage_get_all
export -f usage_get_service
export -f usage_export
export -f usage_export_csv
export -f usage_export_json
export -f usage_export_xlsx
export -f usage_get_stats
export -f usage_get_trends
export -f usage_get_peaks
export -f usage_archive
export -f usage_cleanup_batch
export -f usage_track_api_request
export -f usage_track_storage
export -f usage_track_bandwidth
export -f usage_track_compute
export -f usage_track_database_query
export -f usage_track_function
