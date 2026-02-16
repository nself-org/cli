#!/usr/bin/env bash
# routes-display.sh - Display available service routes to users


# Get the directory where this script is located
ROUTES_DISPLAY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

# Go up 3 levels: services -> lib -> src -> nself root (only if not already set)
NSELF_ROOT="${NSELF_ROOT:-$(cd "$ROUTES_DISPLAY_DIR/../../.." && pwd)}"

# Source platform compatibility utilities
source "$ROUTES_DISPLAY_DIR/../utils/platform-compat.sh" 2>/dev/null || true

# Collect all configured routes
routes::collect_all() {
  local base_domain="${BASE_DOMAIN:-localhost}"
  local project_name="${PROJECT_NAME:-app}"

  # Always include base routes
  echo "api.${base_domain}"
  echo "${base_domain}"

  # Add mail route if enabled
  if [[ "${MAILPIT_ENABLED:-true}" == "true" ]]; then
    echo "mail.${base_domain}"
  fi

  # Add project route
  echo "${project_name}.${base_domain}"

  # Add other service routes
  [[ "${HASURA_ENABLED:-false}" == "true" ]] && echo "${HASURA_ROUTE:-api.${base_domain}}"
  [[ "${AUTH_ENABLED:-false}" == "true" ]] && echo "${AUTH_ROUTE:-auth.${base_domain}}"
  [[ "${STORAGE_ENABLED:-false}" == "true" ]] && echo "${STORAGE_ROUTE:-storage.${base_domain}}"

  # Add frontend app routes
  local app_count="${FRONTEND_APP_COUNT:-0}"
  if [[ "$app_count" -gt 0 ]]; then
    for ((i = 1; i <= app_count; i++)); do
      local subdomain_var="FRONTEND_APP_${i}_SUBDOMAIN"
      local subdomain="${!subdomain_var:-}"
      [[ -n "$subdomain" ]] && echo "${subdomain}.${base_domain}"
    done
  fi
}

# Source dependencies
source "$ROUTES_DISPLAY_DIR/../utils/display.sh" 2>/dev/null || true
source "$ROUTES_DISPLAY_DIR/service-routes.sh" 2>/dev/null || true

# Display all available routes
routes::display_all() {
  local base_domain="${BASE_DOMAIN:-localhost}"
  local project_name="${PROJECT_NAME:-app}"

  echo
  echo "Available Routes (all SSL-secured):"
  echo "=================================="

  # Core services
  if [[ "${HASURA_ENABLED:-false}" == "true" ]]; then
    local hasura_route="${HASURA_ROUTE:-api.${base_domain}}"
    echo "  🔧 GraphQL API:    https://$hasura_route/console"
  fi

  if [[ "${AUTH_ENABLED:-false}" == "true" ]]; then
    local auth_route="${AUTH_ROUTE:-auth.${base_domain}}"
    echo "  🔐 Authentication: https://$auth_route"
  fi

  if [[ "${STORAGE_ENABLED:-false}" == "true" ]]; then
    local storage_route="${STORAGE_ROUTE:-storage.${base_domain}}"
    echo "  📁 Storage:        https://$storage_route"

    local storage_console_route="${STORAGE_CONSOLE_ROUTE:-storage-console.${base_domain}}"
    echo "  📊 Storage Console: https://$storage_console_route"
  fi

  # Additional services
  if [[ "${MAILPIT_ENABLED:-true}" == "true" ]]; then
    local mailpit_route="${MAILPIT_ROUTE:-mail.${base_domain}}"
    echo "  📧 Mail UI:        https://$mailpit_route"
  fi

  if [[ "${MEILISEARCH_ENABLED:-false}" == "true" ]]; then
    local meilisearch_route="${MEILISEARCH_ROUTE:-search.${base_domain}}"
    echo "  🔍 Search Engine:  https://$meilisearch_route"
  fi

  if [[ "${ADMINER_ENABLED:-false}" == "true" ]]; then
    local adminer_route="${ADMINER_ROUTE:-db.${base_domain}}"
    echo "  🗄️  Database UI:    https://$adminer_route"
  fi

  if [[ "${BULLMQ_DASHBOARD_ENABLED:-false}" == "true" ]]; then
    local bullmq_route="${BULLMQ_DASHBOARD_ROUTE:-queues.${base_domain}}"
    echo "  📊 Queue Dashboard: https://$bullmq_route"
  fi

  if [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]]; then
    local functions_route="${FUNCTIONS_ROUTE:-functions.${base_domain}}"
    echo "  ⚡ Functions:       https://$functions_route"
  fi

  if [[ "${DASHBOARD_ENABLED:-false}" == "true" ]]; then
    local dashboard_route="${DASHBOARD_ROUTE:-dashboard.${base_domain}}"
    echo "  📈 Dashboard:       https://$dashboard_route"
  fi

  if [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]]; then
    local admin_route="${NSELF_ADMIN_ROUTE:-admin.${base_domain}}"
    echo "  🛠️  Admin UI:        https://$admin_route"
  fi

  if [[ "${MLFLOW_ENABLED:-false}" == "true" ]]; then
    local mlflow_route="${MLFLOW_ROUTE:-mlflow.${base_domain}}"
    echo "  🤖 MLFlow:         https://$mlflow_route"
  fi

  # Frontend applications
  echo
  echo "Frontend Applications:"
  echo "===================="

  local app_count="${FRONTEND_APP_COUNT:-0}"
  local has_apps=false

  # Check FRONTEND_APP_N variables
  if [[ "$app_count" -gt 0 ]]; then
    for ((i = 1; i <= app_count; i++)); do
      local name_var="FRONTEND_APP_${i}_SYSTEM_NAME"
      local display_var="FRONTEND_APP_${i}_DISPLAY_NAME"
      local route_var="FRONTEND_APP_${i}_ROUTE"

      # Use eval for Bash 3.2 compatibility
      eval "local app_name=\${$name_var:-\${$display_var:-}}"
      eval "local route=\${$route_var:-}"

      if [[ -n "$app_name" ]]; then
        if [[ -z "$route" ]]; then
          route="$app_name"
        fi

        if [[ "$route" != *".${base_domain}" ]]; then
          route="${route}.${base_domain}"
        fi

        # Use sed for Bash 3.2 compatible capitalization
        local app_name_cap=$(echo "$app_name" | sed 's/^./\u&/')
        echo "  📱 ${app_name_cap}:           https://$route"
        has_apps=true
      fi
    done
  fi

  # Check direct FRONTEND_APPS variable
  if [[ "$app_count" -eq 0 && -n "${FRONTEND_APPS:-}" ]]; then
    IFS=',' read -ra APPS <<<"$FRONTEND_APPS"
    for app_config in "${APPS[@]}"; do
      IFS=':' read -r app_name app_short app_prefix app_port <<<"$app_config"

      if [[ -n "$app_name" ]]; then
        local route="${app_short:-$app_name}.${base_domain}"
        # Use sed for Bash 3.2 compatible capitalization
        local app_name_cap=$(echo "$app_name" | sed 's/^./\u&/')
        echo "  📱 ${app_name_cap}:           https://$route"
        has_apps=true
      fi
    done
  fi

  [[ "$has_apps" == "false" ]] && echo "  (No frontend applications configured)"

  # Custom services
  echo
  echo "Custom Services:"
  echo "==============="

  local has_custom=false
  for i in {1..20}; do
    local cs_var="CS_${i}"
    # Use eval for Bash 3.2 compatibility
    eval "local cs_value=\${$cs_var:-}"
    if [[ -n "$cs_value" ]]; then
      IFS=':' read -r cs_type cs_name cs_port cs_route cs_internal <<<"$cs_value"

      if [[ -n "$cs_route" && "$cs_internal" != "true" ]]; then
        local full_route="${cs_route}.${base_domain}"
        # Use sed for Bash 3.2 compatible capitalization
        local cs_name_cap=$(echo "$cs_name" | sed 's/^./\u&/')
        echo "  🔧 ${cs_name_cap} (${cs_type}): https://$full_route"
        has_custom=true
      fi
    fi
  done

  [[ "$has_custom" == "false" ]] && echo "  (No custom services configured)"

  echo
  echo "🎉 All routes are accessible without certificate warnings!"

  # Show SSL certificate info
  if [[ "$base_domain" == "localhost" ]]; then
    echo "🔒 Using mkcert-signed certificates for localhost development"
  else
    echo "🔒 Using SSL certificates for $base_domain"
  fi

  # Show next steps
  echo
  echo "Next Steps:"
  echo "==========="
  echo "1. Run 'nself start' to launch all services"
  echo "2. Run 'nself status' to check service health"
  if [[ "$base_domain" == "localhost" ]]; then
    echo "3. Run 'nself trust' if you see certificate warnings"
  fi
  echo
}

# Display routes in compact format
routes::display_compact() {
  local base_domain="${BASE_DOMAIN:-localhost}"
  local routes=()

  # Collect all routes
  while IFS= read -r route; do
    [[ -n "$route" && "$route" != "127.0.0.1" && "$route" != "::1" ]] && routes+=("https://$route")
  done < <(routes::collect_all)

  if [[ ${#routes[@]} -gt 0 ]]; then
    echo
    echo "Available Routes:"
    printf '  %s\n' "${routes[@]}" | head -10
    [[ ${#routes[@]} -gt 10 ]] && echo "  ... and $((${#routes[@]} - 10)) more"
  fi
}

# Check route accessibility
routes::health_check() {
  local routes=()
  local accessible=0
  local total=0

  # Collect all routes (excluding IPs)
  while IFS= read -r route; do
    [[ -n "$route" && "$route" != "127.0.0.1" && "$route" != "::1" && "$route" != *"*"* ]] && routes+=("$route")
  done < <(routes::collect_all)

  echo "Checking route accessibility..."
  echo "=============================="

  for route in "${routes[@]}"; do
    ((total++))
    local url="https://$route/health"

    if safe_timeout 5 curl -k -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null | grep -q "200\|404"; then
      echo "  ✅ $route"
      ((accessible++))
    else
      echo "  ❌ $route (not responding)"
    fi
  done

  echo
  echo "Summary: $accessible/$total routes accessible"

  if [[ $accessible -lt $total ]]; then
    echo "💡 Some routes may not be accessible until services are started ('nself start')"
  fi
}

# Export functions
export -f routes::display_all
export -f routes::display_compact
export -f routes::health_check
