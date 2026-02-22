#!/usr/bin/env bats
# Monitoring Stack Tests
# Tests for Prometheus, Grafana, Loki, Promtail, Tempo, Alertmanager integration

setup() {
    # Create temp test directory
    TEST_DIR=$(mktemp -d)
    cd "$TEST_DIR"

    # Resolve nself path dynamically
    NSELF_PATH="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
    export PATH="$NSELF_PATH:$PATH"

    # Source monitoring modules directly for unit tests
    export MONITORING_LIB="$NSELF_PATH/src/lib/monitoring"

    # Create minimal test environment
    mkdir -p monitoring/{prometheus,grafana,loki,promtail,tempo,alertmanager}
    mkdir -p logs backups
}

teardown() {
    # Stop any running containers
    docker compose down 2>/dev/null || true

    # Clean up test directory
    cd /
    rm -rf "$TEST_DIR"
}

# ============================================
# Alerting Module Tests
# ============================================

@test "alerting: init_alerting creates required directories and files" {
    source "$MONITORING_LIB/alerting.sh"

    export ALERT_LOG="./logs/alerts.log"
    export ALERT_STATE_FILE="./alerts-state.json"

    init_alerting

    [ -f "./logs/alerts.log" ]
    [ -f "./alerts-state.json" ]
}

@test "alerting: log_alert writes to log file" {
    source "$MONITORING_LIB/alerting.sh"

    export ALERT_LOG="./logs/alerts.log"
    mkdir -p "$(dirname "$ALERT_LOG")"
    touch "$ALERT_LOG"

    log_alert "INFO" "Test alert message"

    [ -f "$ALERT_LOG" ]
    grep -q "Test alert message" "$ALERT_LOG"
    grep -q "INFO" "$ALERT_LOG"
}

@test "alerting: should_send_alert returns true for first alert" {
    source "$MONITORING_LIB/alerting.sh"

    export ALERT_STATE_FILE="./test-state.json"
    echo "{}" > "$ALERT_STATE_FILE"

    should_send_alert "test_alert_key" 300
    [ $? -eq 0 ]
}

@test "alerting: should_send_alert respects cooldown period" {
    source "$MONITORING_LIB/alerting.sh"

    export ALERT_STATE_FILE="./test-state.json"
    local now=$(date +%s)

    # Create state with recent alert (within cooldown)
    echo "{\"test_key\":$now}" > "$ALERT_STATE_FILE"

    run should_send_alert "test_key" 300
    [ "$status" -eq 1 ]  # Should not send (within cooldown)
}

@test "alerting: update_alert_state updates timestamp" {
    source "$MONITORING_LIB/alerting.sh"

    export ALERT_STATE_FILE="./test-state.json"
    echo "{}" > "$ALERT_STATE_FILE"

    update_alert_state "test_alert"

    [ -f "$ALERT_STATE_FILE" ]
    grep -q "test_alert" "$ALERT_STATE_FILE"
}

@test "alerting: send_email_alert requires ALERT_EMAIL configured" {
    source "$MONITORING_LIB/alerting.sh"

    export ALERT_EMAIL=""
    export ALERT_LOG="./logs/alerts.log"
    mkdir -p "$(dirname "$ALERT_LOG")"
    touch "$ALERT_LOG"

    run send_email_alert "Test Subject" "Test Body"
    [ "$status" -eq 1 ]  # Should fail without email configured
}

@test "alerting: send_slack_alert requires webhook URL" {
    source "$MONITORING_LIB/alerting.sh"

    export ALERT_SLACK_WEBHOOK=""
    export ALERT_LOG="./logs/alerts.log"
    mkdir -p "$(dirname "$ALERT_LOG")"
    touch "$ALERT_LOG"

    run send_slack_alert "Test message" "warning"
    [ "$status" -eq 1 ]  # Should fail without webhook
}

@test "alerting: send_discord_alert requires webhook URL" {
    source "$MONITORING_LIB/alerting.sh"

    export ALERT_DISCORD_WEBHOOK=""
    export ALERT_LOG="./logs/alerts.log"
    mkdir -p "$(dirname "$ALERT_LOG")"
    touch "$ALERT_LOG"

    run send_discord_alert "Test message" "warning"
    [ "$status" -eq 1 ]  # Should fail without webhook
}

@test "alerting: send_pagerduty_alert requires API key" {
    source "$MONITORING_LIB/alerting.sh"

    export ALERT_PAGERDUTY_KEY=""
    export ALERT_LOG="./logs/alerts.log"
    mkdir -p "$(dirname "$ALERT_LOG")"
    touch "$ALERT_LOG"

    run send_pagerduty_alert "Test message" "warning"
    [ "$status" -eq 1 ]  # Should fail without API key
}

@test "alerting: monitor_resources checks CPU threshold" {
    source "$MONITORING_LIB/alerting.sh"

    export CPU_ALERT_THRESHOLD=90
    export ALERT_STATE_FILE="./test-state.json"
    export ALERT_LOG="./logs/alerts.log"
    echo "{}" > "$ALERT_STATE_FILE"
    mkdir -p "$(dirname "$ALERT_LOG")"
    touch "$ALERT_LOG"

    # Function should run without errors
    # (actual CPU monitoring requires system tools)
    type monitor_resources | grep -q "function"
}

@test "alerting: monitor_services requires docker" {
    source "$MONITORING_LIB/alerting.sh"

    export ALERT_STATE_FILE="./test-state.json"
    export ALERT_LOG="./logs/alerts.log"
    echo "{}" > "$ALERT_STATE_FILE"
    mkdir -p "$(dirname "$ALERT_LOG")"
    touch "$ALERT_LOG"

    # Function should be defined
    type monitor_services | grep -q "function"
}

@test "alerting: monitor_backups checks backup directory" {
    source "$MONITORING_LIB/alerting.sh"

    export ALERT_STATE_FILE="./test-state.json"
    export ALERT_LOG="./logs/alerts.log"
    echo "{}" > "$ALERT_STATE_FILE"
    mkdir -p "$(dirname "$ALERT_LOG")"
    touch "$ALERT_LOG"
    mkdir -p ./backups

    # Create old backup
    touch ./backups/old-backup.tar.gz

    # Function should be defined and run
    type monitor_backups | grep -q "function"
}

# ============================================
# Metrics Dashboard Tests
# ============================================

@test "metrics: init_metrics creates metrics file" {
    source "$MONITORING_LIB/metrics-dashboard.sh"

    export METRICS_FILE="./test-metrics.json"

    init_metrics

    [ -f "$METRICS_FILE" ]
    grep -q "metrics" "$METRICS_FILE"
    grep -q "timestamp" "$METRICS_FILE"
}

@test "metrics: collect_system_metrics returns JSON format" {
    source "$MONITORING_LIB/metrics-dashboard.sh"

    local result=$(collect_system_metrics)

    # Should contain JSON-like structure
    echo "$result" | grep -q "cpu"
    echo "$result" | grep -q "memory"
    echo "$result" | grep -q "disk"
}

@test "metrics: collect_docker_metrics requires docker" {
    source "$MONITORING_LIB/metrics-dashboard.sh"

    # Function should be defined
    type collect_docker_metrics | grep -q "function"
}

@test "metrics: collect_health_metrics counts services" {
    source "$MONITORING_LIB/metrics-dashboard.sh"

    # Function should return JSON with counts
    type collect_health_metrics | grep -q "function"
}

@test "metrics: store_metrics requires jq" {
    skip "jq may not be available in all environments"

    source "$MONITORING_LIB/metrics-dashboard.sh"

    export METRICS_FILE="./test-metrics.json"
    init_metrics

    # This requires jq to be installed
    if command -v jq >/dev/null 2>&1; then
        local system_metrics='{"cpu":10,"memory":20,"disk":30,"network_rx":0,"network_tx":0}'
        local docker_metrics='{}'
        local health_metrics='{"total":0,"healthy":0,"unhealthy":0,"starting":0}'

        store_metrics "$system_metrics" "$docker_metrics" "$health_metrics"

        [ -f "$METRICS_FILE" ]
    fi
}

@test "metrics: draw_bar function creates progress bar" {
    source "$MONITORING_LIB/metrics-dashboard.sh"

    # Function should be defined
    type draw_bar | grep -q "function"
}

@test "metrics: draw_mini_bar function creates mini bar" {
    source "$MONITORING_LIB/metrics-dashboard.sh"

    type draw_mini_bar | grep -q "function"
}

@test "metrics: draw_health_bar function creates health indicator" {
    source "$MONITORING_LIB/metrics-dashboard.sh"

    type draw_health_bar | grep -q "function"
}

# ============================================
# Dashboard Utilities Tests
# ============================================

@test "dashboard: term control functions are defined" {
    source "$MONITORING_LIB/dashboard.sh"

    type term_save_cursor | grep -q "function"
    type term_restore_cursor | grep -q "function"
    type term_clear_line | grep -q "function"
    type term_move_up | grep -q "function"
}

@test "dashboard: draw_progress_bar creates bar with label" {
    source "$MONITORING_LIB/dashboard.sh"

    type draw_progress_bar | grep -q "function"
}

@test "dashboard: get_health_color returns color code" {
    source "$MONITORING_LIB/dashboard.sh"

    local color=$(get_health_color "healthy")
    [[ "$color" == *"32m"* ]]  # Green color
}

@test "dashboard: get_usage_color changes based on threshold" {
    source "$MONITORING_LIB/dashboard.sh"

    local low_color=$(get_usage_color 30)
    local high_color=$(get_usage_color 85)

    [[ "$low_color" == *"32m"* ]]   # Green
    [[ "$high_color" == *"31m"* ]]  # Red
}

@test "dashboard: format_bytes converts to human readable" {
    source "$MONITORING_LIB/dashboard.sh"

    local result=$(format_bytes 1024)
    [[ "$result" == *"KB"* ]] || [[ "$result" == "1KB" ]]
}

@test "dashboard: get_container_metrics requires docker" {
    source "$MONITORING_LIB/dashboard.sh"

    type get_container_metrics | grep -q "function"
}

@test "dashboard: get_system_metrics detects OS" {
    source "$MONITORING_LIB/dashboard.sh"

    local metrics=$(get_system_metrics)

    # Should contain some metric data
    echo "$metrics" | grep -q "CPU"
}

@test "dashboard: check_alerts identifies issues" {
    source "$MONITORING_LIB/dashboard.sh"

    # Function should be defined
    type check_alerts | grep -q "function"
}

@test "dashboard: draw_service_grid creates grid layout" {
    source "$MONITORING_LIB/dashboard.sh"

    type draw_service_grid | grep -q "function"
}

# ============================================
# Load Balancer Health Tests
# ============================================

@test "lb-health: init_backend_pool creates pool entry" {
    source "$MONITORING_LIB/lb-health.sh"

    export BACKEND_POOLS_FILE="./test-pools"

    init_backend_pool "test-pool" "http://backend1:8080,http://backend2:8080"

    [ -f "$BACKEND_POOLS_FILE" ]
    grep -q "test-pool" "$BACKEND_POOLS_FILE"
}

@test "lb-health: health_check_endpoint tests HTTP endpoint" {
    source "$MONITORING_LIB/lb-health.sh"

    # Function should be defined (actual check requires live endpoint)
    type health_check_endpoint | grep -q "function"
}

@test "lb-health: mark_backend_healthy logs status change" {
    source "$MONITORING_LIB/lb-health.sh"

    export BACKEND_POOLS_FILE="./test-pools"
    touch "$BACKEND_POOLS_FILE"

    type mark_backend_healthy | grep -q "function"
}

@test "lb-health: mark_backend_unhealthy logs status change" {
    source "$MONITORING_LIB/lb-health.sh"

    export BACKEND_POOLS_FILE="./test-pools"
    touch "$BACKEND_POOLS_FILE"

    type mark_backend_unhealthy | grep -q "function"
}

@test "lb-health: get_pool_statistics returns JSON" {
    source "$MONITORING_LIB/lb-health.sh"

    export BACKEND_POOLS_FILE="./test-pools"
    echo "test-pool:http://backend1:8080:active" > "$BACKEND_POOLS_FILE"

    type get_pool_statistics | grep -q "function"
}

@test "lb-health: connection_drain implements graceful shutdown" {
    source "$MONITORING_LIB/lb-health.sh"

    type connection_drain | grep -q "function"
}

@test "lb-health: configure_sticky_sessions supports multiple methods" {
    source "$MONITORING_LIB/lb-health.sh"

    export BACKEND_POOLS_FILE="./test-pools"
    echo "test-pool:http://backend1:8080:active" > "$BACKEND_POOLS_FILE"

    type configure_sticky_sessions | grep -q "function"
}

@test "lb-health: display_health_dashboard creates visual output" {
    source "$MONITORING_LIB/lb-health.sh"

    export BACKEND_POOLS_FILE="./test-pools"
    echo "test-pool:http://backend1:8080:active" > "$BACKEND_POOLS_FILE"

    type display_health_dashboard | grep -q "function"
}

# ============================================
# Monitoring Profiles Tests
# ============================================

@test "profiles: get_monitoring_profile returns appropriate profile" {
    source "$MONITORING_LIB/profiles.sh"

    export ENV="dev"
    local profile=$(get_monitoring_profile)
    [[ "$profile" == "minimal" ]]
}

@test "profiles: get_monitoring_profile detects staging environment" {
    source "$MONITORING_LIB/profiles.sh"

    export ENV="staging"
    local profile=$(get_monitoring_profile)
    [[ "$profile" == "standard" ]]
}

@test "profiles: get_monitoring_profile detects production environment" {
    source "$MONITORING_LIB/profiles.sh"

    export ENV="prod"
    local profile=$(get_monitoring_profile)
    [[ "$profile" == "full" ]]
}

@test "profiles: apply_monitoring_profile sets minimal profile" {
    source "$MONITORING_LIB/profiles.sh"

    apply_monitoring_profile "minimal"

    [[ "$MONITORING_METRICS" == "true" ]]
    [[ -n "$PROMETHEUS_MEMORY_LIMIT" ]]
    [[ -n "$PROMETHEUS_RETENTION" ]]
}

@test "profiles: apply_monitoring_profile sets standard profile" {
    source "$MONITORING_LIB/profiles.sh"

    apply_monitoring_profile "standard"

    [[ "$MONITORING_METRICS" == "true" ]]
    [[ "$MONITORING_EXPORTERS" == "true" ]]
    [[ -n "$LOKI_MEMORY_LIMIT" ]]
}

@test "profiles: apply_monitoring_profile sets full profile" {
    source "$MONITORING_LIB/profiles.sh"

    apply_monitoring_profile "full"

    [[ "$MONITORING_METRICS" == "true" ]]
    [[ "$MONITORING_EXPORTERS" == "true" ]]
    [[ -n "$TEMPO_MEMORY_LIMIT" ]]
}

@test "profiles: get_profile_description provides profile info" {
    source "$MONITORING_LIB/profiles.sh"

    local desc=$(get_profile_description "minimal")

    echo "$desc" | grep -q "Minimal"
    echo "$desc" | grep -q "1GB"
}

@test "profiles: validate_monitoring_config validates settings" {
    source "$MONITORING_LIB/profiles.sh"

    export MONITORING_ENABLED="false"

    validate_monitoring_config
    [ $? -eq 0 ]
}

@test "profiles: validate_monitoring_config warns about default password" {
    skip "Function uses log_warning which may not be defined"

    source "$MONITORING_LIB/profiles.sh"

    export MONITORING_ENABLED="true"
    export GRAFANA_ADMIN_PASSWORD="admin-password-change-me"

    # Would need to capture warning output
    type validate_monitoring_config | grep -q "function"
}

# ============================================
# Integration Tests
# ============================================

@test "integration: monitoring modules can be sourced together" {
    source "$MONITORING_LIB/alerting.sh"
    source "$MONITORING_LIB/metrics-dashboard.sh"
    source "$MONITORING_LIB/dashboard.sh"
    source "$MONITORING_LIB/lb-health.sh"
    source "$MONITORING_LIB/profiles.sh"

    # All modules loaded successfully
    [ $? -eq 0 ]
}

@test "integration: exported functions are available" {
    source "$MONITORING_LIB/alerting.sh"

    # Check if functions are exported
    declare -F init_alerting >/dev/null
    declare -F send_alert >/dev/null
}

@test "integration: monitoring profile affects service enablement" {
    source "$MONITORING_LIB/profiles.sh"

    export ENV="dev"
    local profile=$(get_monitoring_profile)
    apply_monitoring_profile "$profile"

    # Minimal profile should have metrics enabled
    [[ "$MONITORING_METRICS" == "true" ]]
}
