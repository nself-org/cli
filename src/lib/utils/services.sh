#!/usr/bin/env bash
set -euo pipefail

# services.sh - Standardized service counting and categorization
# Part of nself v0.4.7 - Provides consistent service counts across all commands
# POSIX-compliant, no Bash 4+ features

# Get script directory for sourcing dependencies
SERVICES_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source dependencies if not already loaded
if [[ -z "${COLOR_GREEN:-}" ]]; then
  source "$SERVICES_LIB_DIR/display.sh" 2>/dev/null || true
fi

# ============================================================
# Service Category Definitions (from project documentation)
# ============================================================
#
# 1. Required Services (4): Always enabled
#    - PostgreSQL, Hasura, Auth, Nginx
#
# 2. Optional Services (7 types): Must set ENABLED=true
#    - nself-admin, MinIO, Redis, Functions, MLflow, Mail, Search
#
# 3. Monitoring Bundle (10): All-or-nothing via MONITORING_ENABLED
#    - Prometheus, Grafana, Loki, Promtail, Tempo, Alertmanager
#    - cAdvisor, Node Exporter, Postgres Exporter, Redis Exporter
#
# 4. Custom Services (CS_N): User-defined via templates
#
# 5. Frontend Apps (FRONTEND_APP_N): External, nginx routing only
# ============================================================

# Count services in each category
# Returns: TOTAL REQUIRED OPTIONAL MONITORING CUSTOM
count_services_by_category() {
  local required=4 # Always: postgres, hasura, auth, nginx
  local optional=0
  local monitoring=0
  local custom=0

  # Source environment if not loaded
  if [[ -z "${PROJECT_NAME:-}" ]]; then
    for env_file in .env .env.dev .env.local; do
      if [[ -f "$env_file" ]]; then
        set -a
        source "$env_file" 2>/dev/null || true
        set +a
        break
      fi
    done
  fi

  # Count optional services
  [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]] && optional=$((optional + 1))
  [[ "${MINIO_ENABLED:-false}" == "true" ]] && optional=$((optional + 1))
  [[ "${REDIS_ENABLED:-false}" == "true" ]] && optional=$((optional + 1))
  [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]] && optional=$((optional + 1))
  [[ "${MLFLOW_ENABLED:-false}" == "true" ]] && optional=$((optional + 1))
  [[ "${MAILPIT_ENABLED:-false}" == "true" ]] && optional=$((optional + 1))
  [[ "${MEILISEARCH_ENABLED:-false}" == "true" ]] && optional=$((optional + 1))

  # Count monitoring bundle (all 10 or nothing)
  if [[ "${MONITORING_ENABLED:-false}" == "true" ]]; then
    monitoring=10

    # Check for individual overrides
    [[ "${TEMPO_ENABLED:-true}" == "false" ]] && monitoring=$((monitoring - 1))
    [[ "${PROMTAIL_ENABLED:-true}" == "false" ]] && monitoring=$((monitoring - 1))
    [[ "${ALERTMANAGER_ENABLED:-true}" == "false" ]] && monitoring=$((monitoring - 1))
    [[ "${CADVISOR_ENABLED:-true}" == "false" ]] && monitoring=$((monitoring - 1))
    [[ "${NODE_EXPORTER_ENABLED:-true}" == "false" ]] && monitoring=$((monitoring - 1))
    [[ "${POSTGRES_EXPORTER_ENABLED:-true}" == "false" ]] && monitoring=$((monitoring - 1))
    [[ "${REDIS_EXPORTER_ENABLED:-true}" == "false" ]] && monitoring=$((monitoring - 1))
  fi

  # Count custom services (CS_1 through CS_10)
  local n=1
  while [[ $n -le 10 ]]; do
    local cs_var="CS_${n}"
    local cs_val="${!cs_var:-}"
    if [[ -n "$cs_val" ]]; then
      custom=$((custom + 1))
    fi
    n=$((n + 1))
  done

  # Calculate total
  local total=$((required + optional + monitoring + custom))

  # Return as space-separated string
  printf "%d %d %d %d %d" "$total" "$required" "$optional" "$monitoring" "$custom"
}

# Get detailed service count with labels
# Returns human-readable summary
get_service_summary() {
  local counts
  counts=$(count_services_by_category)

  local total required optional monitoring custom
  read -r total required optional monitoring custom <<<"$counts"

  printf "Total: %d services\n" "$total"
  printf "  Required:   %d (PostgreSQL, Hasura, Auth, Nginx)\n" "$required"
  printf "  Optional:   %d (enabled via *_ENABLED)\n" "$optional"
  printf "  Monitoring: %d (MONITORING_ENABLED bundle)\n" "$monitoring"
  printf "  Custom:     %d (CS_1 through CS_N)\n" "$custom"
}

# Count running containers
count_running_containers() {
  local project_name="${PROJECT_NAME:-nself}"

  docker ps --filter "name=${project_name}_" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' '
}

# Count defined services in docker-compose.yml
count_compose_services() {
  if [[ -f "docker-compose.yml" ]]; then
    grep "^  [a-z_-]*:" docker-compose.yml 2>/dev/null | wc -l | tr -d ' '
  else
    echo "0"
  fi
}

# Get unified service status line
# Format: "X/Y services running (A required, B optional, C monitoring, D custom)"
get_unified_status_line() {
  local running=$(count_running_containers)

  local counts
  counts=$(count_services_by_category)

  local total required optional monitoring custom
  read -r total required optional monitoring custom <<<"$counts"

  local breakdown=""
  [[ $required -gt 0 ]] && breakdown="${required} required"
  [[ $optional -gt 0 ]] && breakdown="${breakdown}${breakdown:+, }${optional} optional"
  [[ $monitoring -gt 0 ]] && breakdown="${breakdown}${breakdown:+, }${monitoring} monitoring"
  [[ $custom -gt 0 ]] && breakdown="${breakdown}${breakdown:+, }${custom} custom"

  printf "%d/%d services running (%s)" "$running" "$total" "$breakdown"
}

# Count frontend apps (external, not in Docker)
count_frontend_apps() {
  local count=0

  # Check FRONTEND_APP_N or APP_N variables
  local n=1
  while [[ $n -le 10 ]]; do
    local app_name_var="FRONTEND_APP_${n}_NAME"
    local app_name="${!app_name_var:-}"

    # Also check APP_N format
    if [[ -z "$app_name" ]]; then
      app_name_var="APP_${n}_NAME"
      app_name="${!app_name_var:-}"
    fi

    if [[ -n "$app_name" ]]; then
      count=$((count + 1))
    fi
    n=$((n + 1))
  done

  printf "%d" "$count"
}

# Count routes (services with public URLs)
count_routes() {
  local routes=0

  # Required routes: 2 (api, auth)
  routes=$((routes + 2))

  # Optional service routes
  [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]] && routes=$((routes + 1))
  [[ "${MINIO_ENABLED:-false}" == "true" ]] && routes=$((routes + 1))
  [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]] && routes=$((routes + 1))
  [[ "${MAILPIT_ENABLED:-false}" == "true" ]] && routes=$((routes + 1))
  [[ "${MEILISEARCH_ENABLED:-false}" == "true" ]] && routes=$((routes + 1))
  [[ "${MLFLOW_ENABLED:-false}" == "true" ]] && routes=$((routes + 1))

  # Monitoring routes: 3 (grafana, prometheus, alertmanager)
  if [[ "${MONITORING_ENABLED:-false}" == "true" ]]; then
    routes=$((routes + 3))
  fi

  # Custom service routes (some may not have public routes)
  local n=1
  while [[ $n -le 10 ]]; do
    local cs_var="CS_${n}"
    local cs_val="${!cs_var:-}"
    if [[ -n "$cs_val" ]]; then
      # Parse CS definition: name:template:port
      local port=$(echo "$cs_val" | cut -d':' -f3)
      if [[ -n "$port" ]]; then
        routes=$((routes + 1))
      fi
    fi
    n=$((n + 1))
  done

  # Frontend app routes
  local frontend_count=$(count_frontend_apps)
  routes=$((routes + frontend_count))

  # Application root: 1
  routes=$((routes + 1))

  printf "%d" "$routes"
}

# Print standardized service count table
# Use this in `nself status` and `nself urls` for consistency
print_service_count_table() {
  local counts
  counts=$(count_services_by_category)

  local total required optional monitoring custom
  read -r total required optional monitoring custom <<<"$counts"

  local frontend=$(count_frontend_apps)
  local routes=$(count_routes)
  local running=$(count_running_containers)

  printf "\n"
  printf "${COLOR_CYAN}Service Summary${COLOR_RESET}\n"
  printf "================\n"
  printf "%-18s %s\n" "Docker Containers:" "$running/$total running"
  printf "%-18s %s\n" "  Required:" "$required"
  printf "%-18s %s\n" "  Optional:" "$optional"
  printf "%-18s %s\n" "  Monitoring:" "$monitoring"
  printf "%-18s %s\n" "  Custom (CS_N):" "$custom"
  printf "%-18s %s\n" "Frontend Apps:" "$frontend (external)"
  printf "%-18s %s\n" "Total Routes:" "$routes"
}

# Export functions
export -f count_services_by_category
export -f get_service_summary
export -f count_running_containers
export -f count_compose_services
export -f get_unified_status_line
export -f count_frontend_apps
export -f count_routes
export -f print_service_count_table
