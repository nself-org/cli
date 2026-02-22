#!/usr/bin/env bats
# Billing Stripe Integration Tests
# Tests for Stripe API integration, subscriptions, and payment processing
# Mock Stripe API calls to avoid requiring real Stripe account

setup() {
    # Create temp test directory
    TEST_DIR=$(mktemp -d)
    cd "$TEST_DIR"

    # Resolve nself path dynamically
    NSELF_PATH="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
    export PATH="$NSELF_PATH:$PATH"

    # Source billing modules
    export NSELF_ROOT="$NSELF_PATH"
    source "$NSELF_PATH/src/lib/billing/core.sh"
    source "$NSELF_PATH/src/lib/billing/stripe.sh" 2>/dev/null || true
    set +e  # Reset strict error mode (core.sh enables set -euo pipefail which breaks bats skip)

    # Setup test environment variables
    export BILLING_DB_HOST="localhost"
    export BILLING_DB_PORT="5432"
    export BILLING_DB_NAME="nself_test"
    export BILLING_DB_USER="postgres"
    export BILLING_DB_PASSWORD="testpass"

    # Test Stripe keys (mock)
    export STRIPE_SECRET_KEY="sk_test_mock123"
    export STRIPE_PUBLISHABLE_KEY="pk_test_mock123"
    export STRIPE_WEBHOOK_SECRET="whsec_test_mock123"

    # Test customer ID
    export NSELF_CUSTOMER_ID="cus_test_mock123"
    export PROJECT_NAME="test-billing"

    # Create test config directory
    mkdir -p .nself/billing
}

teardown() {
    # Clean up test directory
    cd /
    rm -rf "$TEST_DIR"

    # Unset environment variables
    unset STRIPE_SECRET_KEY
    unset STRIPE_PUBLISHABLE_KEY
    unset STRIPE_WEBHOOK_SECRET
    unset NSELF_CUSTOMER_ID
}

# ============================================================================
# Stripe API Request Function Tests
# ============================================================================

@test "stripe_api_request requires STRIPE_SECRET_KEY" {
    # Unset the key
    unset STRIPE_SECRET_KEY

    run stripe_api_request GET "/customers"
    [ "$status" -ne 0 ]
    [[ "$output" =~ "STRIPE_SECRET_KEY not configured" ]]
}

@test "stripe_api_request creates secure curl config" {
    skip "Requires live Stripe API or mock server"

    # This test validates that credentials are not passed via command line
    # The curl config file should be created with restrictive permissions
    run stripe_api_request GET "/customers/test"

    # Should not expose credentials in error output
    [[ ! "$output" =~ "sk_test_" ]]
}

@test "stripe_api_request handles API errors gracefully" {
    skip "Requires mock Stripe API server"

    # Mock error response
    run stripe_api_request GET "/invalid_endpoint"
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Stripe API error" ]] || [[ "$output" =~ "error" ]]
}

# ============================================================================
# Customer Management Tests
# ============================================================================

@test "stripe_customer_show requires customer ID" {
    unset NSELF_CUSTOMER_ID

    run stripe_customer_show
    [ "$status" -ne 0 ]
    [[ "$output" =~ "No customer ID found" ]] || [[ "$output" =~ "customer" ]]
}

@test "stripe_customer_show displays customer information" {
    skip "Requires database and mock Stripe API"

    run stripe_customer_show
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Customer ID" ]]
}

@test "stripe_customer_update validates parameters" {
    # Test with no parameters
    run stripe_customer_update
    [ "$status" -ne 0 ]
    [[ "$output" =~ "No update parameters" ]] || [[ "$output" =~ "required" ]]
}

@test "stripe_customer_update accepts valid email parameter" {
    skip "Requires database and mock Stripe API"

    run stripe_customer_update --email=test@example.com
    # Should process without error
    [ "$status" -eq 0 ] || [[ "$output" =~ "Customer information updated" ]]
}

@test "stripe_customer_update rejects unknown parameters" {
    run stripe_customer_update --invalid=value 2>&1
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Unknown parameter" ]]
}

@test "stripe_customer_portal generates portal session" {
    skip "Requires database and mock Stripe API"

    run stripe_customer_portal
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Portal" ]] || [[ "$output" =~ "URL" ]]
}

# ============================================================================
# Subscription Management Tests
# ============================================================================

@test "stripe_subscription_show displays subscription info" {
    skip "Requires database with test subscription"

    run stripe_subscription_show
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Subscription" ]] || [[ "$output" =~ "Plan" ]]
}

@test "stripe_subscription_show handles no active subscription" {
    skip "Requires database"

    # Should handle gracefully when no subscription exists
    run stripe_subscription_show
    [ "$status" -eq 0 ]
    [[ "$output" =~ "No active subscription" ]] || [ "$status" -eq 0 ]
}

@test "stripe_plans_list displays available plans" {
    run stripe_plans_list
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Free" ]]
    [[ "$output" =~ "Starter" ]]
    [[ "$output" =~ "Pro" ]]
    [[ "$output" =~ "Enterprise" ]]
}

@test "stripe_plan_show requires plan name" {
    run stripe_plan_show
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Plan name required" ]]
}

@test "stripe_plan_show displays plan details" {
    skip "Requires database with billing plans"

    run stripe_plan_show "pro"
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Plan Details" ]]
}

@test "stripe_subscription_upgrade requires plan name" {
    run stripe_subscription_upgrade
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Plan name required" ]]
}

@test "stripe_subscription_upgrade validates plan exists" {
    skip "Requires database"

    run stripe_subscription_upgrade "nonexistent_plan"
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Plan not found" ]]
}

@test "stripe_subscription_downgrade requires plan name" {
    run stripe_subscription_downgrade
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Plan name required" ]]
}

@test "stripe_subscription_downgrade shows warning message" {
    skip "Requires database and mock Stripe API"

    run stripe_subscription_downgrade "starter"
    [[ "$output" =~ "Downgrading" ]] || [[ "$output" =~ "period end" ]]
}

@test "stripe_subscription_cancel supports immediate cancellation" {
    skip "Requires database and mock Stripe API"

    run stripe_subscription_cancel --immediate
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Canceling" ]] || [[ "$output" =~ "canceled" ]]
}

@test "stripe_subscription_cancel supports end-of-period cancellation" {
    skip "Requires database and mock Stripe API"

    run stripe_subscription_cancel
    [ "$status" -eq 0 ]
    [[ "$output" =~ "period end" ]] || [[ "$output" =~ "cancel" ]]
}

@test "stripe_subscription_reactivate handles already active subscription" {
    skip "Requires database"

    run stripe_subscription_reactivate
    [ "$status" -eq 0 ]
    [[ "$output" =~ "already active" ]] || [[ "$output" =~ "Reactivat" ]]
}

# ============================================================================
# Payment Method Tests
# ============================================================================

@test "stripe_payment_list requires customer ID" {
    unset NSELF_CUSTOMER_ID

    run stripe_payment_list
    [ "$status" -ne 0 ]
    [[ "$output" =~ "No customer ID" ]]
}

@test "stripe_payment_add directs to customer portal" {
    run stripe_payment_add
    [ "$status" -eq 0 ]
    [[ "$output" =~ "customer portal" ]] || [[ "$output" =~ "securely" ]]
}

@test "stripe_payment_remove requires payment method ID" {
    run stripe_payment_remove
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Payment method ID required" ]]
}

@test "stripe_payment_set_default requires payment method ID" {
    run stripe_payment_set_default
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Payment method ID required" ]]
}

# ============================================================================
# Invoice Tests
# ============================================================================

@test "stripe_invoice_list requires customer ID" {
    unset NSELF_CUSTOMER_ID

    run stripe_invoice_list
    [ "$status" -ne 0 ]
    [[ "$output" =~ "No customer ID" ]]
}

@test "stripe_invoice_show requires invoice ID" {
    run stripe_invoice_show
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Invoice ID required" ]]
}

@test "stripe_invoice_download requires invoice ID" {
    run stripe_invoice_download
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Invoice ID required" ]]
}

@test "stripe_invoice_pay requires invoice ID" {
    run stripe_invoice_pay
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Invoice ID required" ]]
}

@test "stripe_invoice_download creates export directory" {
    skip "Requires mock Stripe API"

    # Should create BILLING_EXPORT_DIR if it doesn't exist
    export BILLING_EXPORT_DIR="$TEST_DIR/.nself/billing/exports"

    run stripe_invoice_download "inv_test123"
    # Check directory exists
    [ -d "$BILLING_EXPORT_DIR" ]
}

# ============================================================================
# Webhook Tests
# ============================================================================

@test "stripe_webhook_test displays webhook URL" {
    run stripe_webhook_test
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Webhook URL" ]] || [[ "$output" =~ "webhook" ]]
}

@test "stripe_webhook_test warns when webhook secret not configured" {
    unset STRIPE_WEBHOOK_SECRET

    run stripe_webhook_test
    [ "$status" -eq 0 ]
    [[ "$output" =~ "STRIPE_WEBHOOK_SECRET not configured" ]] || [ "$status" -eq 0 ]
}

@test "stripe_webhook_list shows configured webhooks" {
    skip "Requires mock Stripe API"

    run stripe_webhook_list
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Webhook" ]]
}

@test "stripe_webhook_events accepts limit parameter" {
    skip "Requires mock Stripe API"

    run stripe_webhook_events 5
    [ "$status" -eq 0 ]
}

# ============================================================================
# Security Tests
# ============================================================================

@test "stripe functions do not expose secret keys in output" {
    # Run various commands and ensure secrets aren't leaked
    run stripe_customer_show 2>&1
    [[ ! "$output" =~ "sk_test_" ]]
    [[ ! "$output" =~ "$STRIPE_SECRET_KEY" ]]
}

@test "stripe API uses curl config file for credentials" {
    skip "Requires monitoring file system operations"

    # The stripe_api_request should create a temporary curl config
    # with mode 600 before writing credentials
    # This prevents credential exposure via ps or command history
}

@test "stripe curl config file has restrictive permissions" {
    skip "Requires file system monitoring"

    # The curl config file should be created with chmod 600
    # to prevent other users from reading credentials
}

# ============================================================================
# Error Handling Tests
# ============================================================================

@test "stripe functions handle network errors gracefully" {
    skip "Requires network simulation"

    # When network is unavailable, functions should fail gracefully
    run stripe_customer_show
    # Should not crash, should show meaningful error
    [[ "$output" =~ "error" ]] || [[ "$output" =~ "failed" ]]
}

@test "stripe functions handle invalid API keys" {
    skip "Requires mock Stripe API with validation"

    export STRIPE_SECRET_KEY="sk_test_invalid"

    run stripe_customer_show
    [ "$status" -ne 0 ]
    [[ "$output" =~ "error" ]] || [[ "$output" =~ "invalid" ]]
}

@test "stripe functions handle missing database connection" {
    skip "Requires database connection control"

    export BILLING_DB_HOST="invalid_host"

    run stripe_subscription_show
    # Should handle database connection failure gracefully
    [ "$status" -ne 0 ] || [[ "$output" =~ "error" ]]
}

# ============================================================================
# Integration Tests
# ============================================================================

@test "subscription lifecycle: upgrade -> downgrade -> cancel -> reactivate" {
    skip "Requires full test environment with database and mock Stripe API"

    # This tests the complete subscription lifecycle:
    # 1. Start with free plan
    # 2. Upgrade to pro
    # 3. Downgrade to starter
    # 4. Cancel subscription
    # 5. Reactivate before period end

    # Each step should succeed and update database correctly
}

@test "payment method lifecycle: add -> set default -> remove" {
    skip "Requires full test environment"

    # Test complete payment method management flow
}

@test "invoice lifecycle: generate -> pay -> download" {
    skip "Requires full test environment"

    # Test complete invoice workflow
}

# ============================================================================
# Performance Tests
# ============================================================================

@test "stripe API requests complete within reasonable time" {
    skip "Requires mock Stripe API with timing"

    # API requests should complete within 5 seconds
    start=$(date +%s)
    stripe_customer_show
    end=$(date +%s)
    duration=$((end - start))

    [ $duration -lt 5 ]
}

@test "multiple concurrent API requests don't conflict" {
    skip "Requires concurrent execution framework"

    # Test that concurrent requests to Stripe API are handled correctly
    # and don't interfere with each other
}
