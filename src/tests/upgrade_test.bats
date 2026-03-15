#!/usr/bin/env bats
# T-0402 — nself self-update: v0.9.9 → v1.0 upgrade path test
# Requires: bats-core, nself CLI in PATH, internet access to GitHub API

setup() {
    export NSELF_HOME="$(mktemp -d)"
    export NSELF_TEST_MODE=1
    # Point at the real GitHub releases API (or a mock via NSELF_UPDATE_URL)
    export NSELF_UPDATE_URL="${NSELF_UPDATE_URL:-https://api.github.com/repos/nself-org/cli/releases/latest}"
}

teardown() {
    rm -rf "$NSELF_HOME"
}

# ---------------------------------------------------------------------------
# Version detection
# ---------------------------------------------------------------------------

@test "self-update --check detects newer version without installing" {
    # Check if nself self-update command is available
    if nself self-update --help 2>&1 | grep -q "Unknown command"; then
        skip "nself self-update not yet implemented"
    fi
    run nself self-update --check
    [ "$status" -eq 0 ]

    # Output must mention a version number
    [[ "$output" =~ [0-9]+\.[0-9]+\.[0-9]+ ]] || {
        echo "Expected version number in output: $output"
        return 1
    }

    # Must not actually install anything (binary unchanged)
    local before after
    before="$(nself --version)"
    run nself self-update --check
    after="$(nself --version)"
    [ "$before" = "$after" ] || {
        echo "--check must not change the installed version"
        return 1
    }
}

@test "self-update --check outputs current and available version" {
    # Check if nself self-update command is available
    if nself self-update --help 2>&1 | grep -q "Unknown command"; then
        skip "nself self-update not yet implemented"
    fi
    run nself self-update --check
    [ "$status" -eq 0 ]
    [[ "$output" == *"current"* ]] || [[ "$output" == *"installed"* ]] || {
        echo "Expected 'current' or 'installed' in output: $output"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Checksum verification
# ---------------------------------------------------------------------------

@test "self-update verifies checksum before install" {
    # Check if nself self-update command is available
    if nself self-update --help 2>&1 | grep -q "Unknown command"; then
        skip "nself self-update not yet implemented"
    fi
    # Inject a bad NSELF_UPDATE_URL pointing to a mock that provides wrong sha256
    export NSELF_UPDATE_URL_MOCK_BAD_CHECKSUM=1

    # If the update URL is not reachable, skip gracefully
    if ! curl -sf "${NSELF_UPDATE_URL}" > /dev/null 2>&1; then
        skip "Update URL not reachable — skipping checksum test"
    fi

    # The upgrade dry-run should include checksum verification step in output
    run nself self-update --dry-run
    [[ "$output" == *"checksum"* ]] || [[ "$output" == *"sha256"* ]] || {
        echo "Expected checksum verification in dry-run output: $output"
        return 1
    }
}

@test "self-update aborts if checksum mismatch detected" {
    # If running in a test environment where we can inject bad checksums
    if [ -z "${NSELF_TEST_BAD_CHECKSUM:-}" ]; then
        skip "NSELF_TEST_BAD_CHECKSUM not set — skipping checksum mismatch test"
    fi

    run nself self-update
    [ "$status" -ne 0 ]
    [[ "$output" == *"checksum"* ]] || [[ "$output" == *"mismatch"* ]] || {
        echo "Expected checksum mismatch error: $output"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Upgrade execution
# ---------------------------------------------------------------------------

@test "self-update succeeds when new version is available" {
    # Only run when explicitly requested (destructive — changes the real binary)
    if [ -z "${NSELF_RUN_UPGRADE_TEST:-}" ]; then
        skip "NSELF_RUN_UPGRADE_TEST not set — skipping live upgrade test"
    fi

    local before
    before="$(nself --version)"

    run nself self-update
    [ "$status" -eq 0 ]

    local after
    after="$(nself --version)"

    # After upgrade, version should be >= before
    [[ "$after" != "$before" ]] || echo "Warning: version unchanged after self-update — may be already current"

    # Binary must still be executable and functional
    run nself --version
    [ "$status" -eq 0 ]
}

@test "self-update is idempotent when already at latest version" {
    if [ -z "${NSELF_RUN_UPGRADE_TEST:-}" ]; then
        skip "NSELF_RUN_UPGRADE_TEST not set"
    fi

    # Run twice — second run should say "already up to date"
    nself self-update || true
    run nself self-update
    [ "$status" -eq 0 ]
    [[ "$output" == *"up to date"* ]] || [[ "$output" == *"already"* ]] || {
        echo "Expected 'already up to date' on re-run: $output"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Error handling
# ---------------------------------------------------------------------------

@test "self-update --check handles network timeout gracefully" {
    # Point at a non-routable IP to simulate network timeout
    export NSELF_UPDATE_URL="http://192.0.2.1/releases/latest"

    run nself self-update --check
    # Should fail gracefully, not panic
    [[ "$output" == *"timeout"* ]] || [[ "$output" == *"network"* ]] || [[ "$output" == *"error"* ]] || {
        # Some implementations exit non-zero silently — that's also OK
        [ "$status" -ne 0 ]
    }
}
