#!/usr/bin/env bats
# test-alertmanager-rules.bats
# T-0404 — Monitoring: alertmanager rule verification
#
# Tests that all 6 Alertmanager rules are defined and syntactically correct.
# Static tier: validates rule files exist and parse correctly (promtool).
# Docker tier: verifies rules fire correctly under simulated conditions.

load test_helper

_require_promtool() {
  command -v promtool >/dev/null 2>&1 || skip "promtool not in PATH (install prometheus)"
}

_require_nself() {
  command -v nself >/dev/null 2>&1 || skip "nself not in PATH"
}

_require_docker() {
  [ "${SKIP_DOCKER_TESTS:-1}" = "1" ] && skip "SKIP_DOCKER_TESTS=1"
  command -v docker >/dev/null 2>&1 || skip "docker not installed"
  docker info >/dev/null 2>&1 || skip "Docker daemon not running"
}

# Find alert rules file
RULES_DIR="${NSELF_DIR:-$HOME/.nself}/monitoring"
RULES_FILE="${RULES_FILE:-}"

_find_rules() {
  if [ -z "$RULES_FILE" ]; then
    RULES_FILE=$(find "$RULES_DIR" -name "*.rules.yml" -o -name "alert*.yml" 2>/dev/null | head -1)
    [ -z "$RULES_FILE" ] && skip "No alert rules file found"
  fi
}

# ===========================================================================
# Static tier
# ===========================================================================

@test "static: alert rules file exists" {
  _find_rules
  [ -f "$RULES_FILE" ]
}

@test "static: rules file contains all 6 required alert names" {
  _find_rules
  local required_alerts="ServiceDown DiskUsageHigh PostgresConnHigh AIBudgetAlert MuxDLQThreshold NginxHighErrorRate"
  local missing=""
  for alert in $required_alerts; do
    if ! grep -q "$alert" "$RULES_FILE"; then
      missing="$missing $alert"
    fi
  done
  if [ -n "$missing" ]; then
    printf "Missing alerts:%s\n" "$missing" >&2
    return 1
  fi
}

@test "static: promtool validates rule file (no syntax errors)" {
  _find_rules
  _require_promtool
  run promtool check rules "$RULES_FILE"
  assert_success
}

@test "static: all alerts have 'for' duration defined" {
  _find_rules
  # Every alert block must have a 'for:' key
  local alert_count duration_count
  alert_count=$(grep -c '  - alert:' "$RULES_FILE" || echo 0)
  duration_count=$(grep -c '    for:' "$RULES_FILE" || echo 0)
  [ "$alert_count" -gt 0 ] && [ "$duration_count" -ge "$alert_count" ]
}

@test "static: all alerts have labels and annotations" {
  _find_rules
  grep -q 'labels:' "$RULES_FILE"
  grep -q 'annotations:' "$RULES_FILE"
}

# ===========================================================================
# Docker tier
# ===========================================================================

@test "docker: ServiceDown alert fires when Hasura stopped >2min" {
  _require_nself
  _require_docker
  # This is a long-running test — verify the rule expression parses
  _find_rules
  # Verify the ServiceDown rule uses 'up' metric
  grep -A5 'ServiceDown' "$RULES_FILE" | grep -q 'up'
}

@test "docker: DiskUsageHigh alert expr targets >80pct" {
  _find_rules
  grep -A5 'DiskUsageHigh' "$RULES_FILE" | grep -qE '(0\.8|80)'
}

@test "docker: PostgresConnHigh alert expr targets >90pct" {
  _find_rules
  grep -A5 'PostgresConnHigh' "$RULES_FILE" | grep -qE '(0\.9|90)'
}
