#!/usr/bin/env bash

#
# nself billing/stripe.sh - Stripe API Integration
# Part of nself v0.9.0 - Sprint 13: Billing Integration & Usage Tracking
#
# Stripe customer management, subscriptions, payments, and webhook handling.
#

# Prevent multiple sourcing
[[ -n "${NSELF_BILLING_STRIPE_LOADED:-}" ]] && return 0

set -euo pipefail

NSELF_BILLING_STRIPE_LOADED=1

# Source dependencies (namespaced to avoid clobbering caller's SCRIPT_DIR)
_BILLING_STRIPE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${_BILLING_STRIPE_DIR}/core.sh"

# Stripe API endpoint
STRIPE_API_BASE="${STRIPE_API_BASE:-https://api.stripe.com/v1}"

# Make Stripe API request using secure curl config file
stripe_api_request() {
  local method="$1"
  local endpoint="$2"
  shift 2
  local params=("$@")

  if [[ -z "$STRIPE_SECRET_KEY" ]]; then
    error "STRIPE_SECRET_KEY not configured"
    return 1
  fi

  local url="${STRIPE_API_BASE}${endpoint}"
  local curl_config
  local response

  # Use curl config file instead of command line for sensitive credentials
  curl_config=$(mktemp) || {
    error "Failed to create temporary curl config"
    return 1
  }
  trap "rm -f '$curl_config'" RETURN

  # Set restrictive permissions BEFORE writing credentials
  chmod 600 "$curl_config" 2>/dev/null || true

  # Write credentials to config file in curl format
  cat >"$curl_config" <<EOF
user = ":${STRIPE_SECRET_KEY}"
EOF
  chmod 600 "$curl_config"

  local curl_opts=(-s --config "$curl_config" -X "$method")

  # Add parameters
  if [[ ${#params[@]} -gt 0 ]]; then
    for param in "${params[@]}"; do
      curl_opts+=(-d "$param")
    done
  fi

  # Make request - credentials never appear in command line
  response=$(curl "${curl_opts[@]}" "$url" 2>/dev/null)

  # Check for errors
  if echo "$response" | grep -q '"error"'; then
    local error_msg
    error_msg=$(echo "$response" | grep -o '"message":"[^"]*"' | cut -d'"' -f4)
    error "Stripe API error: ${error_msg}"
    return 1
  fi

  printf "%s" "$response"
}

# Customer Management
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# Show customer information
stripe_customer_show() {
  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  info "Customer Information"
  printf "\n"

  local response
  response=$(stripe_api_request GET "/customers/${customer_id}")

  if [[ -z "$response" ]]; then
    error "Failed to retrieve customer information"
    return 1
  fi

  local email name created
  email=$(echo "$response" | grep -o '"email":"[^"]*"' | cut -d'"' -f4)
  name=$(echo "$response" | grep -o '"name":"[^"]*"' | cut -d'"' -f4)
  created=$(echo "$response" | grep -o '"created":[0-9]*' | cut -d':' -f2)

  printf "Customer ID:  %s\n" "$customer_id"
  printf "Name:         %s\n" "${name:-N/A}"
  printf "Email:        %s\n" "${email:-N/A}"
  printf "Created:      %s\n" "$(date -r "$created" 2>/dev/null || echo "$created")"
  printf "\n"
}

# Update customer information
stripe_customer_update() {
  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  local params=()

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --email=*)
        params+=("email=${1#*=}")
        shift
        ;;
      --name=*)
        params+=("name=${1#*=}")
        shift
        ;;
      --phone=*)
        params+=("phone=${1#*=}")
        shift
        ;;
      *)
        error "Unknown parameter: $1"
        return 1
        ;;
    esac
  done

  if [[ ${#params[@]} -eq 0 ]]; then
    error "No update parameters provided"
    return 1
  fi

  info "Updating customer information..."

  local response
  response=$(stripe_api_request POST "/customers/${customer_id}" "${params[@]}")

  if [[ -n "$response" ]]; then
    success "Customer information updated"
    return 0
  else
    error "Failed to update customer information"
    return 1
  fi
}

# Open customer portal
stripe_customer_portal() {
  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  info "Creating customer portal session..."

  local return_url="${NSELF_BASE_URL:-http://localhost:3000}/billing"
  local response
  response=$(stripe_api_request POST "/billing_portal/sessions" \
    "customer=${customer_id}" \
    "return_url=${return_url}")

  if [[ -n "$response" ]]; then
    local portal_url
    portal_url=$(echo "$response" | grep -o '"url":"[^"]*"' | cut -d'"' -f4)

    if [[ -n "$portal_url" ]]; then
      printf "\nCustomer Portal URL:\n%s\n\n" "$portal_url"
      success "Portal session created"
      return 0
    fi
  fi

  error "Failed to create portal session"
  return 1
}

# Subscription Management
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# Show current subscription
stripe_subscription_show() {
  local subscription_data
  subscription_data=$(billing_get_subscription)

  if [[ -z "$subscription_data" ]]; then
    warn "No active subscription found"
    return 0
  fi

  info "Current Subscription"
  printf "\n"

  IFS='|' read -r sub_id plan status start end cancel_at_end <<<"$subscription_data"

  printf "Subscription ID:  %s\n" "$(echo "$sub_id" | tr -d ' ')"
  printf "Plan:             %s\n" "$(echo "$plan" | tr -d ' ')"
  printf "Status:           %s\n" "$(echo "$status" | tr -d ' ')"
  printf "Current Period:   %s to %s\n" "$(echo "$start" | tr -d ' ')" "$(echo "$end" | tr -d ' ')"

  if [[ "$(echo "$cancel_at_end" | tr -d ' ')" == "t" ]]; then
    warn "Subscription will cancel at period end"
  fi

  printf "\n"
}

# List available plans
stripe_plans_list() {
  info "Available Plans"
  printf "\n"

  printf "╔════════════════════════════════════════════════════════════════╗\n"
  printf "║ Plan       │ Price/Month │ Features                          ║\n"
  printf "╠════════════╪═════════════╪═══════════════════════════════════╣\n"
  printf "║ Free       │ \$0          │ 10K API requests, 1GB storage    ║\n"
  printf "║ Starter    │ \$29         │ 100K API requests, 10GB storage  ║\n"
  printf "║ Pro        │ \$99         │ 1M API requests, 100GB storage   ║\n"
  printf "║ Enterprise │ Custom      │ Unlimited, dedicated support      ║\n"
  printf "╚════════════╧═════════════╧═══════════════════════════════════╝\n"
  printf "\n"

  printf "Use 'nself billing plan show <plan>' for detailed information\n"
  printf "Use 'nself billing subscription upgrade <plan>' to change plans\n"
}

# Show plan details
stripe_plan_show() {
  local plan_name="$1"

  if [[ -z "$plan_name" ]]; then
    error "Plan name required"
    return 1
  fi

  local plan_details
  plan_details=$(billing_db_query "
        SELECT
            plan_name,
            price_monthly,
            price_yearly,
            description
        FROM billing_plans
        WHERE plan_name = :'plan_name';
    " "tuples" "plan_name" "$plan_name")

  if [[ -z "$plan_details" ]]; then
    error "Plan not found: ${plan_name}"
    return 1
  fi

  info "Plan Details: ${plan_name}"
  printf "\n"

  billing_db_query "
        SELECT
            service_name,
            limit_value,
            limit_type
        FROM billing_quotas
        WHERE plan_name = :'plan_name'
        ORDER BY service_name;
    " "tuples" "plan_name" "$plan_name" | while IFS='|' read -r service limit type; do
    service=$(echo "$service" | tr -d ' ')
    limit=$(echo "$limit" | tr -d ' ')
    type=$(echo "$type" | tr -d ' ')

    if [[ "$limit" == "-1" ]]; then
      printf "  %-12s: Unlimited\n" "$service"
    else
      printf "  %-12s: %s %s\n" "$service" "$limit" "$type"
    fi
  done

  printf "\n"
}

# Compare plans
stripe_plans_compare() {
  info "Plan Comparison"
  printf "\n"

  billing_db_query "
        SELECT
            p.plan_name,
            p.price_monthly,
            STRING_AGG(
                q.service_name || ':' || q.limit_value,
                ','
                ORDER BY q.service_name
            ) as quotas
        FROM billing_plans p
        LEFT JOIN billing_quotas q ON q.plan_name = p.plan_name
        GROUP BY p.plan_name, p.price_monthly
        ORDER BY p.price_monthly;
    " "tuples"
}

# Show current plan
stripe_plan_current() {
  local subscription_data
  subscription_data=$(billing_get_subscription)

  if [[ -z "$subscription_data" ]]; then
    warn "No active subscription"
    return 0
  fi

  local plan
  plan=$(echo "$subscription_data" | cut -d'|' -f2 | tr -d ' ')

  stripe_plan_show "$plan"
}

# Upgrade subscription
stripe_subscription_upgrade() {
  local new_plan="$1"

  if [[ -z "$new_plan" ]]; then
    error "Plan name required"
    return 1
  fi

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  info "Upgrading to plan: ${new_plan}"

  local subscription_data
  subscription_data=$(billing_get_subscription)

  if [[ -z "$subscription_data" ]]; then
    error "No active subscription found"
    return 1
  fi

  local sub_id
  sub_id=$(echo "$subscription_data" | cut -d'|' -f1 | tr -d ' ')

  local price_id
  price_id=$(billing_db_query "
        SELECT stripe_price_id
        FROM billing_plans
        WHERE plan_name = :'plan_name';
    " "tuples" "plan_name" "$new_plan" | tr -d ' ')

  if [[ -z "$price_id" ]]; then
    error "Plan not found: ${new_plan}"
    return 1
  fi

  local response
  response=$(stripe_api_request POST "/subscriptions/${sub_id}" \
    "items[0][price]=${price_id}" \
    "proration_behavior=always_invoice")

  if [[ -n "$response" ]]; then
    billing_db_query "
            UPDATE billing_subscriptions
            SET plan_name = :'plan_name',
                updated_at = NOW()
            WHERE subscription_id = :'sub_id';
        " "tuples" "plan_name" "$new_plan" "sub_id" "$sub_id" >/dev/null

    success "Subscription upgraded to ${new_plan}"
    return 0
  else
    error "Failed to upgrade subscription"
    return 1
  fi
}

# Downgrade subscription
stripe_subscription_downgrade() {
  local new_plan="$1"

  if [[ -z "$new_plan" ]]; then
    error "Plan name required"
    return 1
  fi

  warn "Downgrading to plan: ${new_plan}"
  printf "Downgrade will take effect at the end of current billing period.\n\n"

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  local subscription_data
  subscription_data=$(billing_get_subscription)

  if [[ -z "$subscription_data" ]]; then
    error "No active subscription found"
    return 1
  fi

  local sub_id
  sub_id=$(echo "$subscription_data" | cut -d'|' -f1 | tr -d ' ')

  local price_id
  price_id=$(billing_db_query "
        SELECT stripe_price_id
        FROM billing_plans
        WHERE plan_name = :'plan_name';
    " "tuples" "plan_name" "$new_plan" | tr -d ' ')

  if [[ -z "$price_id" ]]; then
    error "Plan not found: ${new_plan}"
    return 1
  fi

  local response
  response=$(stripe_api_request POST "/subscriptions/${sub_id}" \
    "items[0][price]=${price_id}" \
    "proration_behavior=none")

  if [[ -n "$response" ]]; then
    success "Subscription will downgrade to ${new_plan} at period end"
    return 0
  else
    error "Failed to schedule downgrade"
    return 1
  fi
}

# Cancel subscription
stripe_subscription_cancel() {
  local immediate=false

  if [[ "$1" == "--immediate" ]]; then
    immediate=true
  fi

  local subscription_data
  subscription_data=$(billing_get_subscription)

  if [[ -z "$subscription_data" ]]; then
    warn "No active subscription to cancel"
    return 0
  fi

  local sub_id
  sub_id=$(echo "$subscription_data" | cut -d'|' -f1 | tr -d ' ')

  if [[ "$immediate" == "true" ]]; then
    warn "Canceling subscription immediately"

    local response
    response=$(stripe_api_request DELETE "/subscriptions/${sub_id}")

    if [[ -n "$response" ]]; then
      billing_db_query "
                UPDATE billing_subscriptions
                SET status = 'canceled',
                    updated_at = NOW()
                WHERE subscription_id = :'sub_id';
            " "tuples" "sub_id" "$sub_id" >/dev/null

      success "Subscription canceled"
      return 0
    fi
  else
    info "Scheduling cancellation at period end"

    local response
    response=$(stripe_api_request POST "/subscriptions/${sub_id}" \
      "cancel_at_period_end=true")

    if [[ -n "$response" ]]; then
      billing_db_query "
                UPDATE billing_subscriptions
                SET cancel_at_period_end = true,
                    updated_at = NOW()
                WHERE subscription_id = :'sub_id';
            " "tuples" "sub_id" "$sub_id" >/dev/null

      success "Subscription will cancel at period end"
      return 0
    fi
  fi

  error "Failed to cancel subscription"
  return 1
}

# Reactivate subscription
stripe_subscription_reactivate() {
  local subscription_data
  subscription_data=$(billing_get_subscription)

  if [[ -z "$subscription_data" ]]; then
    error "No subscription found"
    return 1
  fi

  local sub_id cancel_at_end
  IFS='|' read -r sub_id _ _ _ _ cancel_at_end <<<"$subscription_data"
  sub_id=$(echo "$sub_id" | tr -d ' ')
  cancel_at_end=$(echo "$cancel_at_end" | tr -d ' ')

  if [[ "$cancel_at_end" != "t" ]]; then
    info "Subscription is already active"
    return 0
  fi

  info "Reactivating subscription"

  local response
  response=$(stripe_api_request POST "/subscriptions/${sub_id}" \
    "cancel_at_period_end=false")

  if [[ -n "$response" ]]; then
    billing_db_query "
            UPDATE billing_subscriptions
            SET cancel_at_period_end = false,
                updated_at = NOW()
            WHERE subscription_id = :'sub_id';
        " "tuples" "sub_id" "$sub_id" >/dev/null

    success "Subscription reactivated"
    return 0
  else
    error "Failed to reactivate subscription"
    return 1
  fi
}

# Payment Methods
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# List payment methods
stripe_payment_list() {
  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  info "Payment Methods"
  printf "\n"

  local response
  response=$(stripe_api_request GET "/customers/${customer_id}/payment_methods" \
    "type=card")

  printf "Payment methods listed in Stripe dashboard\n"
  printf "Use customer portal for full management: nself billing customer portal\n\n"
}

# Add payment method
stripe_payment_add() {
  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  info "Add Payment Method"
  printf "\n"
  printf "Please use the customer portal to add payment methods securely:\n\n"
  printf "  nself billing customer portal\n\n"
}

# Remove payment method
stripe_payment_remove() {
  local payment_method_id="$1"

  if [[ -z "$payment_method_id" ]]; then
    error "Payment method ID required"
    return 1
  fi

  info "Removing payment method: ${payment_method_id}"

  local response
  response=$(stripe_api_request POST "/payment_methods/${payment_method_id}/detach")

  if [[ -n "$response" ]]; then
    success "Payment method removed"
    return 0
  else
    error "Failed to remove payment method"
    return 1
  fi
}

# Set default payment method
stripe_payment_set_default() {
  local payment_method_id="$1"

  if [[ -z "$payment_method_id" ]]; then
    error "Payment method ID required"
    return 1
  fi

  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  info "Setting default payment method"

  local response
  response=$(stripe_api_request POST "/customers/${customer_id}" \
    "invoice_settings[default_payment_method]=${payment_method_id}")

  if [[ -n "$response" ]]; then
    success "Default payment method updated"
    return 0
  else
    error "Failed to update default payment method"
    return 1
  fi
}

# Invoices
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# List invoices
stripe_invoice_list() {
  local customer_id
  customer_id=$(billing_get_customer_id) || {
    error "No customer ID found"
    return 1
  }

  info "Recent Invoices"
  printf "\n"

  billing_db_query "
        SELECT
            invoice_id,
            TO_CHAR(created_at, 'YYYY-MM-DD'),
            total_amount,
            status
        FROM billing_invoices
        WHERE customer_id = :'customer_id'
        ORDER BY created_at DESC
        LIMIT 10;
    " "tuples" "customer_id" "$customer_id" | while IFS='|' read -r id date amount status; do
    printf "%-20s  %s  \$%-8s  %s\n" \
      "$(echo "$id" | tr -d ' ')" \
      "$(echo "$date" | tr -d ' ')" \
      "$(echo "$amount" | tr -d ' ')" \
      "$(echo "$status" | tr -d ' ')"
  done

  printf "\n"
}

# Show invoice details
stripe_invoice_show() {
  local invoice_id="$1"

  if [[ -z "$invoice_id" ]]; then
    error "Invoice ID required"
    return 1
  fi

  info "Invoice: ${invoice_id}"
  printf "\n"

  billing_db_query "
        SELECT
            invoice_id,
            total_amount,
            status,
            period_start,
            period_end,
            created_at
        FROM billing_invoices
        WHERE invoice_id = :'invoice_id';
    " "tuples" "invoice_id" "$invoice_id"
}

# Download invoice PDF
stripe_invoice_download() {
  local invoice_id="$1"

  if [[ -z "$invoice_id" ]]; then
    error "Invoice ID required"
    return 1
  fi

  info "Downloading invoice: ${invoice_id}"

  local response
  response=$(stripe_api_request GET "/invoices/${invoice_id}")

  local pdf_url
  pdf_url=$(echo "$response" | grep -o '"invoice_pdf":"[^"]*"' | cut -d'"' -f4)

  if [[ -n "$pdf_url" ]]; then
    local output_file="${BILLING_EXPORT_DIR}/${invoice_id}.pdf"
    curl -s "$pdf_url" -o "$output_file"

    success "Invoice downloaded: ${output_file}"
    return 0
  else
    error "Failed to get invoice PDF URL"
    return 1
  fi
}

# Pay invoice
stripe_invoice_pay() {
  local invoice_id="$1"

  if [[ -z "$invoice_id" ]]; then
    error "Invoice ID required"
    return 1
  fi

  info "Paying invoice: ${invoice_id}"

  local response
  response=$(stripe_api_request POST "/invoices/${invoice_id}/pay")

  if [[ -n "$response" ]]; then
    billing_db_query "
            UPDATE billing_invoices
            SET status = 'paid',
                paid_at = NOW()
            WHERE invoice_id = :'invoice_id';
        " "tuples" "invoice_id" "$invoice_id" >/dev/null

    success "Invoice paid"
    return 0
  else
    error "Failed to pay invoice"
    return 1
  fi
}

# Webhooks
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# Test webhook
stripe_webhook_test() {
  info "Testing webhook endpoint"

  if [[ -z "$STRIPE_WEBHOOK_SECRET" ]]; then
    warn "STRIPE_WEBHOOK_SECRET not configured"
  fi

  local webhook_url="${NSELF_BASE_URL:-http://localhost:3000}/api/webhooks/stripe"

  printf "\nWebhook URL: %s\n" "$webhook_url"
  printf "Configure this URL in your Stripe Dashboard\n\n"

  return 0
}

# List webhooks
stripe_webhook_list() {
  info "Webhook Endpoints"
  printf "\n"

  stripe_api_request GET "/webhook_endpoints" |
    grep -o '"url":"[^"]*"' | cut -d'"' -f4 |
    while read -r url; do
      printf "  %s\n" "$url"
    done

  printf "\n"
}

# List webhook events
stripe_webhook_events() {
  local limit="${1:-10}"

  info "Recent Webhook Events"
  printf "\n"

  stripe_api_request GET "/events?limit=${limit}"
}

# Export functions
export -f stripe_customer_show
export -f stripe_customer_update
export -f stripe_customer_portal
export -f stripe_subscription_show
export -f stripe_plans_list
export -f stripe_plan_show
export -f stripe_plans_compare
export -f stripe_plan_current
export -f stripe_subscription_upgrade
export -f stripe_subscription_downgrade
export -f stripe_subscription_cancel
export -f stripe_subscription_reactivate
export -f stripe_payment_list
export -f stripe_payment_add
export -f stripe_payment_remove
export -f stripe_payment_set_default
export -f stripe_invoice_list
export -f stripe_invoice_show
export -f stripe_invoice_download
export -f stripe_invoice_pay
export -f stripe_webhook_test
export -f stripe_webhook_list
export -f stripe_webhook_events
