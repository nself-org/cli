#!/usr/bin/env bats
# Multi-Tenancy Tests
# Tests for tenant isolation, provisioning, and billing

setup() {
    # Create temp test directory
    TEST_DIR=$(mktemp -d)
    cd "$TEST_DIR"

    # Resolve nself path dynamically
    NSELF_PATH="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
    export PATH="$NSELF_PATH:$PATH"

    # Initialize minimal nself project
    nself init 2>/dev/null || true

    # Enable multi-tenancy
    printf "PROJECT_NAME=test-tenancy\n" >> .env
    printf "MULTI_TENANT=true\n" >> .env
    printf "TENANT_ISOLATION=strict\n" >> .env
}

teardown() {
    # Stop any running containers
    docker compose down 2>/dev/null || true

    # Clean up test directory
    cd /
    rm -rf "$TEST_DIR"
}

@test "tenant help command shows available options" {
    run nself tenant help
    [ "$status" -eq 0 ]
    [[ "$output" =~ "tenant" ]] || [[ "$output" =~ "organization" ]]
}

@test "tenant create requires name" {
    run nself tenant create
    # Should fail or show usage without tenant name
    [ "$status" -ne 0 ] || [[ "$output" =~ "usage" ]] || [[ "$output" =~ "required" ]]
}

@test "tenant create provisions new tenant" {
    skip "Requires running services with multi-tenancy"

    nself build
    nself start

    run nself tenant create "Test Tenant"
    [ "$status" -eq 0 ]
    [[ "$output" =~ "created" ]] || [[ "$output" =~ "success" ]]
}

@test "tenant list shows all tenants" {
    skip "Requires running services"

    nself build
    nself start

    run nself tenant list
    [ "$status" -eq 0 ]
}

@test "tenant isolation prevents cross-tenant data access" {
    skip "Requires running services with multiple tenants"

    # This would test RLS (Row Level Security) policies
    # by attempting to access data from another tenant
}

@test "tenant create generates unique subdomain" {
    skip "Requires DNS/routing configuration"

    nself build
    nself start

    run nself tenant create "acme-corp"
    [ "$status" -eq 0 ]
    [[ "$output" =~ "acme-corp" ]] || [[ "$output" =~ "subdomain" ]]
}

@test "tenant delete removes tenant data" {
    skip "Requires running services"

    nself build
    nself start

    # Create tenant
    nself tenant create "test-delete"

    # Delete tenant
    run nself tenant delete "test-delete"
    [ "$status" -eq 0 ]
}

@test "tenant delete requires confirmation for safety" {
    skip "Requires running services"

    nself build
    nself start

    nself tenant create "test-confirm"

    # Should require confirmation
    run nself tenant delete "test-confirm"
    # Either status ne 0 or output asks for confirmation
    [ "$status" -ne 0 ] || [[ "$output" =~ "confirm" ]] || [[ "$output" =~ "sure" ]]
}

@test "tenant suspend disables tenant access" {
    skip "Requires running services"

    nself build
    nself start

    nself tenant create "test-suspend"

    run nself tenant suspend "test-suspend"
    [ "$status" -eq 0 ]
    [[ "$output" =~ "suspend" ]]
}

@test "tenant resume reactivates suspended tenant" {
    skip "Requires running services"

    nself build
    nself start

    nself tenant create "test-resume"
    nself tenant suspend "test-resume"

    run nself tenant resume "test-resume"
    [ "$status" -eq 0 ]
    [[ "$output" =~ "resume" ]] || [[ "$output" =~ "activ" ]]
}

@test "tenant quota shows resource limits" {
    skip "Requires running services"

    nself build
    nself start

    run nself tenant quota "test-tenant"
    [ "$status" -eq 0 ]
    [[ "$output" =~ "quota" ]] || [[ "$output" =~ "limit" ]]
}

@test "tenant quota set updates resource limits" {
    skip "Requires running services"

    nself build
    nself start

    nself tenant create "test-quota"

    run nself tenant quota set "test-quota" --storage 10GB
    [ "$status" -eq 0 ]
}

@test "tenant billing shows usage and costs" {
    skip "Requires running services with billing"

    nself build
    nself start

    run nself tenant billing "test-tenant"
    [ "$status" -eq 0 ]
    [[ "$output" =~ "billing" ]] || [[ "$output" =~ "usage" ]]
}

@test "tenant member add adds user to tenant" {
    skip "Requires running services"

    nself build
    nself start

    nself tenant create "test-member"

    run nself tenant member add "test-member" "user@example.com"
    [ "$status" -eq 0 ]
}

@test "tenant member remove removes user from tenant" {
    skip "Requires running services"

    nself build
    nself start

    nself tenant create "test-member-remove"
    nself tenant member add "test-member-remove" "user@example.com"

    run nself tenant member remove "test-member-remove" "user@example.com"
    [ "$status" -eq 0 ]
}

@test "tenant member list shows all members" {
    skip "Requires running services"

    nself build
    nself start

    nself tenant create "test-members"
    nself tenant member add "test-members" "user1@example.com"
    nself tenant member add "test-members" "user2@example.com"

    run nself tenant member list "test-members"
    [ "$status" -eq 0 ]
    [[ "$output" =~ "user1@example.com" ]]
    [[ "$output" =~ "user2@example.com" ]]
}

@test "tenant backup creates tenant-specific backup" {
    skip "Requires running services"

    nself build
    nself start

    nself tenant create "test-backup"

    run nself tenant backup "test-backup"
    [ "$status" -eq 0 ]
    [[ "$output" =~ "backup" ]]
}

@test "tenant restore restores tenant from backup" {
    skip "Requires running services and backup"

    nself build
    nself start

    run nself tenant restore "test-tenant" backup.tar.gz
    [ "$status" -eq 0 ]
}

@test "tenant stats shows usage statistics" {
    skip "Requires running services"

    nself build
    nself start

    run nself tenant stats "test-tenant"
    [ "$status" -eq 0 ]
    [[ "$output" =~ "user" ]] || [[ "$output" =~ "request" ]] || [[ "$output" =~ "storage" ]]
}

@test "tenant handles invalid tenant names" {
    # Test name validation
    run nself tenant create "Invalid Tenant Name!"
    [ "$status" -ne 0 ] || [[ "$output" =~ "invalid" ]] || [[ "$output" =~ "name" ]]
}
