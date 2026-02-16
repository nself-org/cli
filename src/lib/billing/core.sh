#!/usr/bin/env bash

#
# nself billing/core.sh - Billing System Core Functions
# Part of nself v0.9.0 - Sprint 13: Billing Integration & Usage Tracking
#
# Core billing system initialization, configuration, and foundational functions.
#

# Prevent multiple sourcing
[[ -n "${NSELF_BILLING_CORE_LOADED:-}" ]] && return 0

set -euo pipefail

NSELF_BILLING_CORE_LOADED=1

# Source dependencies (namespaced to avoid clobbering caller's SCRIPT_DIR)
_BILLING_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "${_BILLING_DIR}/../.." && pwd)"

# Source utility functions (with fallback)
if [[ -f "${NSELF_ROOT}/lib/utils/output.sh" ]]; then
  source "${NSELF_ROOT}/lib/utils/output.sh"
elif [[ -f "${_BILLING_DIR}/../utils/output.sh" ]]; then
  source "${_BILLING_DIR}/../utils/output.sh"
else
  # Fallback output functions
  error() { printf "[ERROR] %s\n" "$*" >&2; }
  warn() { printf "[WARN] %s\n" "$*" >&2; }
  info() { printf "[INFO] %s\n" "$*"; }
  success() { printf "[SUCCESS] %s\n" "$*"; }
fi

if [[ -f "${NSELF_ROOT}/lib/utils/validation.sh" ]]; then
  source "${NSELF_ROOT}/lib/utils/validation.sh"
elif [[ -f "${_BILLING_DIR}/../utils/validation.sh" ]]; then
  source "${_BILLING_DIR}/../utils/validation.sh"
fi

# Billing configuration
BILLING_DB_HOST="${BILLING_DB_HOST:-localhost}"
BILLING_DB_PORT="${BILLING_DB_PORT:-5432}"
BILLING_DB_NAME="${BILLING_DB_NAME:-nself}"
BILLING_DB_USER="${BILLING_DB_USER:-postgres}"
BILLING_DB_PASSWORD="${BILLING_DB_PASSWORD:-}"

# Stripe configuration
STRIPE_SECRET_KEY="${STRIPE_SECRET_KEY:-}"
STRIPE_PUBLISHABLE_KEY="${STRIPE_PUBLISHABLE_KEY:-}"
STRIPE_WEBHOOK_SECRET="${STRIPE_WEBHOOK_SECRET:-}"
STRIPE_API_VERSION="${STRIPE_API_VERSION:-2023-10-16}"

# Billing paths
BILLING_DATA_DIR="${NSELF_ROOT}/.nself/billing"
BILLING_CACHE_DIR="${BILLING_DATA_DIR}/cache"
BILLING_EXPORT_DIR="${BILLING_DATA_DIR}/exports"
BILLING_LOG_FILE="${BILLING_DATA_DIR}/billing.log"

# Initialize billing system
billing_init() {
  local quiet="${1:-false}"

  # Create required directories
  mkdir -p "${BILLING_DATA_DIR}" "${BILLING_CACHE_DIR}" "${BILLING_EXPORT_DIR}"

  # Validate configuration
  if ! billing_validate_config; then
    if [[ "$quiet" != "true" ]]; then
      error "Billing configuration validation failed"
    fi
    return 1
  fi

  # Test database connection
  if ! billing_test_db_connection; then
    if [[ "$quiet" != "true" ]]; then
      error "Database connection failed"
    fi
    return 1
  fi

  # Test Stripe API if configured
  if [[ -n "$STRIPE_SECRET_KEY" ]]; then
    if ! billing_test_stripe_connection; then
      if [[ "$quiet" != "true" ]]; then
        warn "Stripe API connection failed (continuing with limited functionality)"
      fi
    fi
  fi

  if [[ "$quiet" != "true" ]]; then
    success "Billing system initialized"
  fi

  return 0
}

# Validate billing configuration
billing_validate_config() {
  local errors=0

  # Check required database configuration
  if [[ -z "$BILLING_DB_HOST" ]]; then
    error "BILLING_DB_HOST not set"
    ((errors++))
  fi

  if [[ -z "$BILLING_DB_NAME" ]]; then
    error "BILLING_DB_NAME not set"
    ((errors++))
  fi

  # Check Stripe configuration (optional but recommended)
  if [[ -z "$STRIPE_SECRET_KEY" ]]; then
    warn "STRIPE_SECRET_KEY not set - Stripe features disabled"
  fi

  if [[ -z "$STRIPE_PUBLISHABLE_KEY" ]]; then
    warn "STRIPE_PUBLISHABLE_KEY not set - Stripe features disabled"
  fi

  # Check directories are writable
  if [[ ! -w "$BILLING_DATA_DIR" ]]; then
    error "Billing data directory not writable: ${BILLING_DATA_DIR}"
    ((errors++))
  fi

  return $errors
}

# Helper function to create .pgpass file for secure credential storage
_billing_create_pgpass() {
  local pgpass_file="$1"

  # Set restrictive permissions BEFORE writing credentials
  if [[ -f "$pgpass_file" ]]; then
    rm -f "$pgpass_file"
  fi
  touch "$pgpass_file"
  chmod 600 "$pgpass_file"

  # Write credentials in .pgpass format: hostname:port:database:username:password
  printf "%s:%d:%s:%s:%s\n" \
    "$BILLING_DB_HOST" \
    "$BILLING_DB_PORT" \
    "$BILLING_DB_NAME" \
    "$BILLING_DB_USER" \
    "$BILLING_DB_PASSWORD" >"$pgpass_file"

  # Verify permissions are still restrictive after write
  chmod 600 "$pgpass_file"

  printf "%s" "$pgpass_file"
}

# Test database connection
billing_test_db_connection() {
  local result
  local pgpass_file

  # Use .pgpass file for secure password storage instead of environment variable
  pgpass_file=$(mktemp) || {
    # Fallback to environment variable if mktemp fails
    result=$(PGPASSWORD="$BILLING_DB_PASSWORD" psql -h "$BILLING_DB_HOST" \
      -p "$BILLING_DB_PORT" -U "$BILLING_DB_USER" -d "$BILLING_DB_NAME" \
      -t -c "SELECT 1;" 2>/dev/null || echo "")
  }

  if [[ -n "$pgpass_file" ]] && [[ -f "$pgpass_file" ]]; then
    trap "rm -f '$pgpass_file'; unset PGPASSFILE" RETURN

    # Create pgpass file with secure credentials
    _billing_create_pgpass "$pgpass_file" >/dev/null

    # Use .pgpass file via PGPASSFILE environment variable
    export PGPASSFILE="$pgpass_file"
    result=$(psql -h "$BILLING_DB_HOST" \
      -p "$BILLING_DB_PORT" -U "$BILLING_DB_USER" -d "$BILLING_DB_NAME" \
      -t -c "SELECT 1;" 2>/dev/null || echo "")
    unset PGPASSFILE
  fi

  if [[ "$result" =~ 1 ]]; then
    return 0
  else
    return 1
  fi
}

# Test Stripe API connection
billing_test_stripe_connection() {
  if [[ -z "$STRIPE_SECRET_KEY" ]]; then
    return 1
  fi

  local curl_config
  local response

  # Use curl config file for secure credential handling
  curl_config=$(mktemp) || return 1
  trap "rm -f '$curl_config'" RETURN

  chmod 600 "$curl_config"
  cat >"$curl_config" <<EOF
user = ":${STRIPE_SECRET_KEY}"
EOF
  chmod 600 "$curl_config"

  response=$(curl -s --config "$curl_config" \
    "https://api.stripe.com/v1/balance" 2>/dev/null || echo "")

  if [[ -n "$response" ]] && [[ ! "$response" =~ "error" ]]; then
    return 0
  else
    return 1
  fi
}

# Execute database query with parameterized query support
billing_db_query() {
  local query="$1"
  local format="${2:-tuples}"
  shift 2

  # Build psql command as array to prevent command injection
  local psql_opts=(-h "${BILLING_DB_HOST}" -p "${BILLING_DB_PORT}" -U "${BILLING_DB_USER}" -d "${BILLING_DB_NAME}")
  local pgpass_file

  # Create .pgpass file for secure credential handling
  pgpass_file=$(mktemp) || {
    # Fallback without .pgpass if mktemp fails (use array here too)
    local fallback_cmd=(psql -h "${BILLING_DB_HOST}" -p "${BILLING_DB_PORT}" -U "${BILLING_DB_USER}" -d "${BILLING_DB_NAME}" -t -c "$query")
    PGPASSWORD="$BILLING_DB_PASSWORD" "${fallback_cmd[@]}" 2>/dev/null
    return $?
  }

  trap "rm -f '$pgpass_file'; unset PGPASSFILE" RETURN
  _billing_create_pgpass "$pgpass_file" >/dev/null

  # Build variable bindings from remaining arguments (key-value pairs)
  while (($# >= 2)); do
    local var_name="$1"
    local var_value="$2"
    shift 2
    psql_opts+=(-v "${var_name}=${var_value}")
  done

  case "$format" in
    csv)
      psql_opts+=(--csv)
      ;;
    json)
      query="SELECT row_to_json(t) FROM (${query}) t;"
      psql_opts+=(-t)
      ;;
    *)
      psql_opts+=(-t)
      ;;
  esac

  export PGPASSFILE="$pgpass_file"
  psql "${psql_opts[@]}" -c "$query" 2>/dev/null
  unset PGPASSFILE
}

# Get current customer ID from environment or config
billing_get_customer_id() {
  if [[ -n "${NSELF_CUSTOMER_ID:-}" ]]; then
    printf "%s" "$NSELF_CUSTOMER_ID"
    return 0
  fi

  local project_config="${NSELF_ROOT}/.env"
  if [[ -f "$project_config" ]]; then
    local customer_id
    customer_id=$(grep "^NSELF_CUSTOMER_ID=" "$project_config" 2>/dev/null | cut -d= -f2-)
    if [[ -n "$customer_id" ]]; then
      printf "%s" "$customer_id"
      return 0
    fi
  fi

  local db_customer_id
  db_customer_id=$(billing_db_query "SELECT customer_id FROM billing_customers WHERE project_name=:'project_name' LIMIT 1;" "tuples" "project_name" "${PROJECT_NAME:-default}" 2>/dev/null | tr -d ' ')

  if [[ -n "$db_customer_id" ]]; then
    printf "%s" "$db_customer_id"
    return 0
  fi

  return 1
}

# Get current subscription
billing_get_subscription() {
  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  billing_db_query "
        SELECT
            subscription_id,
            plan_name,
            status,
            current_period_start,
            current_period_end,
            cancel_at_period_end
        FROM billing_subscriptions
        WHERE customer_id = :'customer_id'
        AND status IN ('active', 'trialing')
        ORDER BY created_at DESC
        LIMIT 1;
    " "tuples" "customer_id" "$customer_id"
}

# Record usage event
billing_record_usage() {
  local service="$1"
  local quantity="$2"
  local metadata="${3:-{}}"

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    warn "No customer ID - usage not recorded"
    return 1
  }

  local timestamp
  timestamp=$(date -u +"%Y-%m-%d %H:%M:%S")

  billing_db_query "
        INSERT INTO billing_usage_records
            (customer_id, service_name, quantity, metadata, recorded_at)
        VALUES
            (:'customer_id', :'service_name', :'quantity', :'metadata', :'recorded_at');
    " "tuples" "customer_id" "$customer_id" "service_name" "$service" "quantity" "$quantity" "metadata" "$metadata" "recorded_at" "$timestamp" >/dev/null

  billing_log "USAGE" "$service" "$quantity" "$metadata"
}

# Check quota for service
billing_check_quota() {
  local service="$1"
  local requested="${2:-1}"

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    warn "No customer ID - quota check skipped"
    return 0
  }

  local quota
  quota=$(billing_db_query "
        SELECT q.limit_value
        FROM billing_quotas q
        JOIN billing_subscriptions s ON s.plan_name = q.plan_name
        WHERE s.customer_id = :'customer_id'
        AND s.status = 'active'
        AND q.service_name = :'service_name'
        LIMIT 1;
    " "tuples" "customer_id" "$customer_id" "service_name" "$service" | tr -d ' ')

  if [[ -z "$quota" ]] || [[ "$quota" == "-1" ]]; then
    return 0
  fi

  local usage
  usage=$(billing_db_query "
        SELECT COALESCE(SUM(quantity), 0)
        FROM billing_usage_records ur
        JOIN billing_subscriptions s ON s.customer_id = ur.customer_id
        WHERE ur.customer_id = :'customer_id'
        AND ur.service_name = :'service_name'
        AND ur.recorded_at >= s.current_period_start
        AND ur.recorded_at <= s.current_period_end;
    " "tuples" "customer_id" "$customer_id" "service_name" "$service" | tr -d ' ')

  local total=$((usage + requested))
  if [[ $total -gt $quota ]]; then
    return 1
  fi

  return 0
}

# Get quota status
billing_get_quota_status() {
  local service="$1"

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    printf "unknown"
    return 1
  }

  local quota usage

  quota=$(billing_db_query "
        SELECT q.limit_value
        FROM billing_quotas q
        JOIN billing_subscriptions s ON s.plan_name = q.plan_name
        WHERE s.customer_id = :'customer_id'
        AND s.status = 'active'
        AND q.service_name = :'service_name'
        LIMIT 1;
    " "tuples" "customer_id" "$customer_id" "service_name" "$service" | tr -d ' ')

  usage=$(billing_db_query "
        SELECT COALESCE(SUM(quantity), 0)
        FROM billing_usage_records ur
        JOIN billing_subscriptions s ON s.customer_id = ur.customer_id
        WHERE ur.customer_id = :'customer_id'
        AND ur.service_name = :'service_name'
        AND ur.recorded_at >= s.current_period_start
        AND ur.recorded_at <= s.current_period_end;
    " "tuples" "customer_id" "$customer_id" "service_name" "$service" | tr -d ' ')

  local percent=0
  if [[ -n "$quota" ]] && [[ "$quota" != "-1" ]] && [[ $quota -gt 0 ]]; then
    percent=$((usage * 100 / quota))
  fi

  printf '{"service":"%s","usage":%d,"quota":%s,"percent":%d}\n' \
    "$service" "${usage:-0}" "${quota:--1}" "$percent"
}

# Generate invoice
billing_generate_invoice() {
  local customer_id="$1"
  local period_start="$2"
  local period_end="$3"

  local invoice_id
  invoice_id="inv_$(date +%s)_$(openssl rand -hex 4)"

  local total_amount=0

  billing_db_query "
        INSERT INTO billing_invoices
            (invoice_id, customer_id, period_start, period_end, total_amount, status)
        VALUES
            (:'invoice_id', :'customer_id', :'period_start', :'period_end', :'total_amount', :'status');
    " "tuples" "invoice_id" "$invoice_id" "customer_id" "$customer_id" "period_start" "$period_start" "period_end" "$period_end" "total_amount" "$total_amount" "status" "draft" >/dev/null

  printf "%s" "$invoice_id"
}

# Export billing data
billing_export_all() {
  local format="$1"
  local output_file="$2"

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  case "$format" in
    json)
      billing_db_query "
                SELECT json_build_object(
                    'customer', (SELECT row_to_json(c) FROM billing_customers c WHERE c.customer_id = :'customer_id'),
                    'subscription', (SELECT row_to_json(s) FROM billing_subscriptions s WHERE s.customer_id = :'customer_id' AND s.status = 'active'),
                    'invoices', (SELECT json_agg(row_to_json(i)) FROM billing_invoices i WHERE i.customer_id = :'customer_id'),
                    'usage', (SELECT json_agg(row_to_json(u)) FROM billing_usage_records u WHERE u.customer_id = :'customer_id')
                );
            " "tuples" "customer_id" "$customer_id" >"$output_file"
      ;;
    csv)
      local base="${output_file%.csv}"
      billing_db_query "SELECT * FROM billing_customers WHERE customer_id = :'customer_id';" "csv" "customer_id" "$customer_id" >"${base}_customer.csv"
      billing_db_query "SELECT * FROM billing_subscriptions WHERE customer_id = :'customer_id';" "csv" "customer_id" "$customer_id" >"${base}_subscriptions.csv"
      billing_db_query "SELECT * FROM billing_invoices WHERE customer_id = :'customer_id';" "csv" "customer_id" "$customer_id" >"${base}_invoices.csv"
      billing_db_query "SELECT * FROM billing_usage_records WHERE customer_id = :'customer_id';" "csv" "customer_id" "$customer_id" >"${base}_usage.csv"
      ;;
    *)
      error "Unsupported format: $format"
      return 1
      ;;
  esac

  return 0
}

# Log billing event
billing_log() {
  local event_type="$1"
  local service="$2"
  local value="$3"
  local metadata="${4:-}"

  local timestamp
  timestamp=$(date -u +"%Y-%m-%d %H:%M:%S")

  printf "[%s] %s | %s | %s | %s\n" \
    "$timestamp" "$event_type" "$service" "$value" "$metadata" \
    >>"$BILLING_LOG_FILE"
}

# Get billing summary
billing_get_summary() {
  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  billing_db_query "
        SELECT
            s.plan_name,
            s.status,
            COUNT(DISTINCT i.invoice_id) as invoice_count,
            COALESCE(SUM(i.total_amount), 0) as total_billed,
            COUNT(DISTINCT ur.service_name) as services_used
        FROM billing_subscriptions s
        LEFT JOIN billing_invoices i ON i.customer_id = s.customer_id
        LEFT JOIN billing_usage_records ur ON ur.customer_id = s.customer_id
        WHERE s.customer_id = :'customer_id'
        AND s.status = 'active'
        GROUP BY s.plan_name, s.status;
    " "tuples" "customer_id" "$customer_id"
}

# ============================================================================
# Database Initialization Functions
# ============================================================================

# Initialize billing database schema
# This function is idempotent - safe to run multiple times
billing_init_db() {
  local migration_file="${NSELF_ROOT}/src/database/migrations/015_create_billing_system.sql"

  # Check if migration file exists
  if [[ ! -f "$migration_file" ]]; then
    error "Billing migration file not found: ${migration_file}"
    return 1
  fi

  # Run migration
  billing_log "INIT" "database" "0" "Running billing schema migration"

  local result
  result=$(PGPASSWORD="$BILLING_DB_PASSWORD" psql \
    -h "$BILLING_DB_HOST" \
    -p "$BILLING_DB_PORT" \
    -U "$BILLING_DB_USER" \
    -d "$BILLING_DB_NAME" \
    -f "$migration_file" 2>&1)

  local exit_code=$?

  if [[ $exit_code -eq 0 ]]; then
    success "Billing database schema initialized"
    billing_log "INIT" "database" "1" "Schema migration successful"
    return 0
  else
    error "Database migration failed: ${result}"
    billing_log "ERROR" "database" "$exit_code" "Migration failed: ${result}"
    return 1
  fi
}

# Check database health
billing_check_db_health() {
  local health_status="healthy"
  local issues=()

  # Test basic connection
  if ! billing_test_db_connection; then
    health_status="unhealthy"
    issues+=("Database connection failed")
    printf '{"status":"%s","issues":["%s"]}\n' "$health_status" "${issues[0]}"
    return 1
  fi

  # Check if tables exist
  local table_count
  table_count=$(billing_db_query "
        SELECT COUNT(*)
        FROM information_schema.tables
        WHERE table_schema = 'public'
        AND table_name LIKE 'billing_%';
    " "tuples" 2>/dev/null | tr -d ' ')

  if [[ -z "$table_count" ]] || [[ "$table_count" -lt 8 ]]; then
    health_status="degraded"
    issues+=("Missing billing tables (expected 8+, found ${table_count})")
  fi

  # Check for orphaned records
  local orphaned_usage
  orphaned_usage=$(billing_db_query "
        SELECT COUNT(*)
        FROM billing_usage_records ur
        LEFT JOIN billing_customers c ON c.customer_id = ur.customer_id
        WHERE c.customer_id IS NULL;
    " "tuples" 2>/dev/null | tr -d ' ')

  if [[ -n "$orphaned_usage" ]] && [[ "$orphaned_usage" -gt 0 ]]; then
    health_status="degraded"
    issues+=("Found ${orphaned_usage} orphaned usage records")
  fi

  # Output health status
  printf '{"status":"%s","table_count":%s,"orphaned_records":%s}\n' \
    "$health_status" \
    "${table_count:-0}" \
    "${orphaned_usage:-0}"

  [[ "$health_status" == "healthy" ]]
}

# ============================================================================
# Customer Management Functions
# ============================================================================

# Create new billing customer
# Args: customer_id, project_name, email, name, company
billing_create_customer() {
  local customer_id="$1"
  local project_name="$2"
  local email="${3:-}"
  local name="${4:-}"
  local company="${5:-}"

  # Validate required parameters
  if [[ -z "$customer_id" ]] || [[ -z "$project_name" ]]; then
    error "Customer ID and project name are required"
    return 1
  fi

  # Check if customer already exists
  local existing_customer
  existing_customer=$(billing_db_query "
        SELECT customer_id FROM billing_customers
        WHERE customer_id = :'customer_id';
    " "tuples" "customer_id" "$customer_id" 2>/dev/null | tr -d ' ')

  if [[ -n "$existing_customer" ]]; then
    warn "Customer already exists: ${customer_id}"
    return 0 # Idempotent - return success
  fi

  # Create customer record
  billing_db_query "
        INSERT INTO billing_customers (
            customer_id,
            project_name,
            email,
            name,
            company
        ) VALUES (
            :'customer_id',
            :'project_name',
            :'email',
            :'name',
            :'company'
        );
    " "tuples" \
    "customer_id" "$customer_id" \
    "project_name" "$project_name" \
    "email" "$email" \
    "name" "$name" \
    "company" "$company" >/dev/null

  local exit_code=$?

  if [[ $exit_code -eq 0 ]]; then
    success "Customer created: ${customer_id}"
    billing_log "CREATE" "customer" "$customer_id" "email=${email}"

    # Create default free subscription
    billing_create_default_subscription "$customer_id"

    return 0
  else
    error "Failed to create customer: ${customer_id}"
    billing_log "ERROR" "customer" "$exit_code" "Failed to create ${customer_id}"
    return 1
  fi
}

# Create default free subscription for new customer
billing_create_default_subscription() {
  local customer_id="$1"

  local subscription_id="sub_$(date +%s)_$(openssl rand -hex 4 2>/dev/null || printf "%08x" $RANDOM)"
  local current_date
  current_date=$(date -u +"%Y-%m-%d %H:%M:%S")

  # Calculate period end (1 month from now) - platform compatible
  local period_end
  if date -v+1m >/dev/null 2>&1; then
    # BSD date (macOS)
    period_end=$(date -u -v+1m +"%Y-%m-%d %H:%M:%S")
  else
    # GNU date (Linux)
    period_end=$(date -u -d "+1 month" +"%Y-%m-%d %H:%M:%S")
  fi

  billing_db_query "
        INSERT INTO billing_subscriptions (
            subscription_id,
            customer_id,
            plan_name,
            status,
            billing_cycle,
            current_period_start,
            current_period_end
        ) VALUES (
            :'subscription_id',
            :'customer_id',
            'free',
            'active',
            'monthly',
            :'current_period_start',
            :'current_period_end'
        );
    " "tuples" \
    "subscription_id" "$subscription_id" \
    "customer_id" "$customer_id" \
    "current_period_start" "$current_date" \
    "current_period_end" "$period_end" >/dev/null

  local exit_code=$?

  if [[ $exit_code -eq 0 ]]; then
    billing_log "CREATE" "subscription" "$subscription_id" "customer=${customer_id},plan=free"
    return 0
  else
    error "Failed to create default subscription for customer: ${customer_id}"
    return 1
  fi
}

# Get customer details
# Args: customer_id (optional - uses billing_get_customer_id if not provided)
billing_get_customer() {
  local customer_id="${1:-}"

  if [[ -z "$customer_id" ]]; then
    customer_id=$(billing_get_customer_id) || {
      error "No customer ID found"
      return 1
    }
  fi

  billing_db_query "
        SELECT
            customer_id,
            project_name,
            email,
            name,
            company,
            stripe_customer_id,
            created_at,
            updated_at
        FROM billing_customers
        WHERE customer_id = :'customer_id'
        AND deleted_at IS NULL;
    " "tuples" "customer_id" "$customer_id"
}

# Update customer information
# Args: customer_id, field_name, field_value
billing_update_customer() {
  local customer_id="$1"
  local field_name="$2"
  local field_value="$3"

  # Validate parameters
  if [[ -z "$customer_id" ]] || [[ -z "$field_name" ]] || [[ -z "$field_value" ]]; then
    error "Customer ID, field name, and field value are required"
    return 1
  fi

  # Whitelist allowed fields to prevent SQL injection
  case "$field_name" in
    email | name | company | stripe_customer_id)
      # Valid field
      ;;
    *)
      error "Invalid field name: ${field_name}"
      return 1
      ;;
  esac

  # Update customer record using dynamic field name (safe because of whitelist)
  billing_db_query "
        UPDATE billing_customers
        SET ${field_name} = :'field_value',
            updated_at = NOW()
        WHERE customer_id = :'customer_id';
    " "tuples" \
    "field_value" "$field_value" \
    "customer_id" "$customer_id" >/dev/null

  local exit_code=$?

  if [[ $exit_code -eq 0 ]]; then
    success "Customer updated: ${customer_id} - ${field_name}=${field_value}"
    billing_log "UPDATE" "customer" "$customer_id" "${field_name}=${field_value}"
    return 0
  else
    error "Failed to update customer: ${customer_id}"
    billing_log "ERROR" "customer" "$exit_code" "Failed to update ${customer_id}"
    return 1
  fi
}

# Delete customer (soft delete)
# Args: customer_id
billing_delete_customer() {
  local customer_id="$1"

  if [[ -z "$customer_id" ]]; then
    error "Customer ID is required"
    return 1
  fi

  # Soft delete by setting deleted_at timestamp
  billing_db_query "
        UPDATE billing_customers
        SET deleted_at = NOW(),
            updated_at = NOW()
        WHERE customer_id = :'customer_id';
    " "tuples" "customer_id" "$customer_id" >/dev/null

  local exit_code=$?

  if [[ $exit_code -eq 0 ]]; then
    success "Customer deleted: ${customer_id}"
    billing_log "DELETE" "customer" "$customer_id" "soft_delete"
    return 0
  else
    error "Failed to delete customer: ${customer_id}"
    billing_log "ERROR" "customer" "$exit_code" "Failed to delete ${customer_id}"
    return 1
  fi
}

# List all customers
# Args: limit (optional, default 100), offset (optional, default 0)
billing_list_customers() {
  local limit="${1:-100}"
  local offset="${2:-0}"

  billing_db_query "
        SELECT
            customer_id,
            project_name,
            email,
            name,
            company,
            created_at
        FROM billing_customers
        WHERE deleted_at IS NULL
        ORDER BY created_at DESC
        LIMIT :'limit' OFFSET :'offset';
    " "tuples" "limit" "$limit" "offset" "$offset"
}

# Get customer's active plan
# Args: customer_id (optional)
billing_get_customer_plan() {
  local customer_id="${1:-}"

  if [[ -z "$customer_id" ]]; then
    customer_id=$(billing_get_customer_id) || {
      error "No customer ID found"
      return 1
    }
  fi

  billing_db_query "
        SELECT
            s.plan_name,
            p.display_name,
            p.description,
            p.price_monthly,
            p.price_yearly,
            s.billing_cycle,
            s.status,
            s.current_period_start,
            s.current_period_end
        FROM billing_subscriptions s
        JOIN billing_plans p ON p.plan_name = s.plan_name
        WHERE s.customer_id = :'customer_id'
        AND s.status IN ('active', 'trialing')
        ORDER BY s.created_at DESC
        LIMIT 1;
    " "tuples" "customer_id" "$customer_id"
}

# Export individual functions
export -f billing_init
export -f billing_validate_config
export -f billing_test_db_connection
export -f billing_test_stripe_connection
export -f billing_db_query
export -f billing_get_customer_id
export -f billing_get_subscription
export -f billing_record_usage
export -f billing_check_quota
export -f billing_get_quota_status
export -f billing_generate_invoice
export -f billing_export_all
export -f billing_log
export -f billing_get_summary
export -f billing_init_db
export -f billing_check_db_health
export -f billing_create_customer
export -f billing_create_default_subscription
export -f billing_get_customer
export -f billing_update_customer
export -f billing_delete_customer
export -f billing_list_customers
export -f billing_get_customer_plan

# ============================================================================
# Load Additional Billing Modules
# ============================================================================

# Source invoices module (if exists)
if [[ -f "${_BILLING_DIR}/invoices.sh" ]]; then
  source "${_BILLING_DIR}/invoices.sh"
fi

# Source payments module (if exists)
if [[ -f "${_BILLING_DIR}/payments.sh" ]]; then
  source "${_BILLING_DIR}/payments.sh"
fi

# Source reports module (if exists)
if [[ -f "${_BILLING_DIR}/reports.sh" ]]; then
  source "${_BILLING_DIR}/reports.sh"
fi
