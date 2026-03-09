#!/usr/bin/env bats
# T-0386 — Plugin install order: topological sort + dependency validation
# Requires: bats-core, nself CLI available in PATH

setup() {
    # Use a temporary nself environment so tests don't affect real installations
    export NSELF_HOME="$(mktemp -d)"
    export NSELF_TEST_MODE=1
}

teardown() {
    rm -rf "$NSELF_HOME"
}

# ---------------------------------------------------------------------------
# Dependency ordering
# ---------------------------------------------------------------------------

@test "install claw suite installs ai and mux first" {
    # nself-claw depends on: nself-ai, nself-mux
    # nself-mux depends on: nself-ai
    # Expected install order: ai -> mux -> claw

    run nself plugin install --dry-run --order-only claw
    [ "$status" -eq 0 ]

    # Extract install order from output
    local first_idx second_idx third_idx
    first_idx=$(echo "$output" | grep -n "nself-ai" | head -1 | cut -d: -f1)
    second_idx=$(echo "$output" | grep -n "nself-mux" | head -1 | cut -d: -f1)
    third_idx=$(echo "$output" | grep -n "nself-claw" | head -1 | cut -d: -f1)

    [ -n "$first_idx" ]  || { echo "nself-ai not found in output"; return 1; }
    [ -n "$second_idx" ] || { echo "nself-mux not found in output"; return 1; }
    [ -n "$third_idx" ]  || { echo "nself-claw not found in output"; return 1; }

    [ "$first_idx" -lt "$second_idx" ]  || { echo "ai must come before mux"; return 1; }
    [ "$second_idx" -lt "$third_idx" ] || { echo "mux must come before claw"; return 1; }
}

@test "install google plugin before google-dependent plugins" {
    # Any plugin that depends on nself-google must install after it
    run nself plugin install --dry-run --order-only claw
    [ "$status" -eq 0 ]

    # If claw transitively depends on google, google must appear before claw
    if echo "$output" | grep -q "nself-google"; then
        local google_idx claw_idx
        google_idx=$(echo "$output" | grep -n "nself-google" | head -1 | cut -d: -f1)
        claw_idx=$(echo "$output" | grep -n "nself-claw" | head -1 | cut -d: -f1)
        [ "$google_idx" -lt "$claw_idx" ] || { echo "google must come before claw"; return 1; }
    fi
}

@test "install ai mux claw in one command resolves full order" {
    run nself plugin install --dry-run --order-only ai mux claw
    [ "$status" -eq 0 ]

    local ai_idx mux_idx claw_idx
    ai_idx=$(echo "$output" | grep -n "nself-ai" | head -1 | cut -d: -f1)
    mux_idx=$(echo "$output" | grep -n "nself-mux" | head -1 | cut -d: -f1)
    claw_idx=$(echo "$output" | grep -n "nself-claw" | head -1 | cut -d: -f1)

    [ "$ai_idx" -lt "$mux_idx" ]   || { echo "ai must be before mux"; return 1; }
    [ "$ai_idx" -lt "$claw_idx" ]  || { echo "ai must be before claw"; return 1; }
    [ "$mux_idx" -lt "$claw_idx" ] || { echo "mux must be before claw"; return 1; }
}

# ---------------------------------------------------------------------------
# Circular dependency detection
# ---------------------------------------------------------------------------

@test "circular dependency detected and rejected" {
    # Inject a test plugin that has a circular dependency (plugin_a → plugin_b → plugin_a)
    mkdir -p "$NSELF_HOME/plugin-registry"
    cat > "$NSELF_HOME/plugin-registry/circular_a.json" <<EOF
{
  "name": "circular_a",
  "version": "0.1.0",
  "dependencies": ["circular_b"]
}
EOF
    cat > "$NSELF_HOME/plugin-registry/circular_b.json" <<EOF
{
  "name": "circular_b",
  "version": "0.1.0",
  "dependencies": ["circular_a"]
}
EOF

    run nself plugin install --dry-run circular_a
    [ "$status" -ne 0 ]
    [[ "$output" == *"circular"* ]] || [[ "$output" == *"cycle"* ]] || \
        { echo "expected circular dependency error in: $output"; return 1; }
}

@test "self-referential dependency rejected" {
    mkdir -p "$NSELF_HOME/plugin-registry"
    cat > "$NSELF_HOME/plugin-registry/self_dep.json" <<EOF
{
  "name": "self_dep",
  "version": "0.1.0",
  "dependencies": ["self_dep"]
}
EOF

    run nself plugin install --dry-run self_dep
    [ "$status" -ne 0 ]
}

# ---------------------------------------------------------------------------
# Independent plugins
# ---------------------------------------------------------------------------

@test "independent plugins can install in any order" {
    # notify, cron, google have no interdependencies — install order is flexible
    run nself plugin install --dry-run --order-only notify cron google
    [ "$status" -eq 0 ]

    # All three must appear in the output
    echo "$output" | grep -q "nself-notify" || { echo "notify missing"; return 1; }
    echo "$output" | grep -q "nself-cron"   || { echo "cron missing"; return 1; }
    echo "$output" | grep -q "nself-google" || { echo "google missing"; return 1; }
}

@test "already installed plugin skipped in order output" {
    # Simulate ai already installed
    mkdir -p "$NSELF_HOME/plugins/installed"
    touch "$NSELF_HOME/plugins/installed/nself-ai"

    run nself plugin install --dry-run --order-only claw
    [ "$status" -eq 0 ]

    # nself-ai should be marked as already-installed, not re-queued
    [[ "$output" == *"already installed"* ]] || [[ "$output" == *"skip"* ]] || \
        { echo "expected ai to be skipped; output: $output"; return 1; }
}
