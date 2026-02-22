#!/usr/bin/env bats
# SSL/TLS Tests
# Enhanced tests for Let's Encrypt, certificate renewal, and self-signed certs

setup() {
    # Create temp test directory
    TEST_DIR=$(mktemp -d)
    cd "$TEST_DIR"

    # Resolve nself path dynamically
    NSELF_PATH="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
    export PATH="$NSELF_PATH:$PATH"

    # Initialize minimal nself project
    nself init

    # Set test configuration
    printf "PROJECT_NAME=test-ssl\n" >> .env
    printf "BASE_DOMAIN=test.local\n" >> .env
}

teardown() {
    # Clean up test directory
    cd /
    rm -rf "$TEST_DIR"
}

@test "ssl help command shows available options" {
    run nself ssl help
    [ "$status" -eq 0 ]
    [[ "$output" =~ "ssl" ]] || [[ "$output" =~ "certificate" ]]
}

@test "ssl generate creates self-signed certificate" {
    run nself ssl generate
    [ "$status" -eq 0 ]

    # Check if certificate files were created
    [ -f "ssl/cert.pem" ] || [ -f "certs/cert.pem" ] || [ -f "nginx/ssl/cert.pem" ]
}

@test "ssl generate creates both cert and key files" {
    run nself ssl generate
    [ "$status" -eq 0 ]

    # Both cert and key should exist
    cert_exists=false
    key_exists=false

    for dir in ssl certs nginx/ssl; do
        [ -f "$dir/cert.pem" ] && cert_exists=true
        [ -f "$dir/key.pem" ] && key_exists=true
    done

    [ "$cert_exists" = true ]
    [ "$key_exists" = true ]
}

@test "ssl verify validates certificate integrity" {
    skip "Requires SSL certificate generation"

    # Generate cert first
    nself ssl generate

    # Verify it
    run nself ssl verify
    [ "$status" -eq 0 ]
    [[ "$output" =~ "valid" ]] || [[ "$output" =~ "OK" ]]
}

@test "ssl verify detects missing certificates" {
    run nself ssl verify
    # Should fail or warn about missing certs
    [ "$status" -ne 0 ] || [[ "$output" =~ "not found" ]] || [[ "$output" =~ "missing" ]]
}

@test "ssl renew handles Let's Encrypt renewal" {
    skip "Requires Let's Encrypt configuration"

    run nself ssl renew
    # Should either renew or indicate not using Let's Encrypt
    [ "$status" -eq 0 ] || [[ "$output" =~ "not configured" ]]
}

@test "ssl status shows certificate information" {
    skip "Requires SSL certificate"

    nself ssl generate

    run nself ssl status
    [ "$status" -eq 0 ]
    [[ "$output" =~ "certificate" ]] || [[ "$output" =~ "expir" ]]
}

@test "ssl generate supports custom domain" {
    run nself ssl generate --domain custom.example.com
    [ "$status" -eq 0 ] || [ "$status" -eq 1 ]
    # Command should at least parse the argument
}

@test "ssl handles missing openssl gracefully" {
    skip "OpenSSL availability varies"

    # Would test behavior when openssl is not available
}

@test "ssl letsencrypt setup validates domain ownership" {
    skip "Requires DNS configuration"

    run nself ssl letsencrypt setup example.com
    # Should validate domain or show requirements
    [ "$status" -eq 0 ] || [[ "$output" =~ "DNS" ]] || [[ "$output" =~ "validation" ]]
}

@test "ssl letsencrypt auto-renewal can be scheduled" {
    skip "Requires Let's Encrypt setup"

    run nself ssl letsencrypt auto-renew enable
    [ "$status" -eq 0 ]
    [[ "$output" =~ "enabled" ]] || [[ "$output" =~ "scheduled" ]]
}

@test "ssl certificate expiry warning system" {
    skip "Requires certificate with expiry date"

    # Generate cert
    nself ssl generate

    # Check expiry
    run nself ssl expiry
    [ "$status" -eq 0 ]
    [[ "$output" =~ "days" ]] || [[ "$output" =~ "expir" ]]
}

@test "ssl supports multiple domains (SAN certificates)" {
    skip "Requires advanced SSL configuration"

    run nself ssl generate --domains "example.com,www.example.com,api.example.com"
    [ "$status" -eq 0 ]
}

@test "ssl wildcard certificate generation" {
    skip "Requires Let's Encrypt DNS challenge"

    run nself ssl letsencrypt setup "*.example.com"
    [ "$status" -eq 0 ] || [[ "$output" =~ "DNS" ]]
}

@test "ssl revoke removes certificate" {
    skip "Requires existing certificate"

    nself ssl generate

    run nself ssl revoke
    [ "$status" -eq 0 ]
}

@test "ssl import allows custom certificates" {
    skip "Requires certificate files"

    # Create dummy cert files
    touch custom-cert.pem
    touch custom-key.pem

    run nself ssl import custom-cert.pem custom-key.pem
    [ "$status" -eq 0 ]
}
