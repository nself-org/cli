#!/usr/bin/env bash

# hosts.sh - Manage /etc/hosts entries for local development

# Check if a hosts entry exists
hosts_entry_exists() {

set -euo pipefail

  local domain="$1"
  grep -q "127\.0\.0\.1.*[[:space:]]${domain}[[:space:]]*" /etc/hosts 2>/dev/null ||
    grep -q "127\.0\.0\.1.*[[:space:]]${domain}$" /etc/hosts 2>/dev/null
}

# Add entry to /etc/hosts (requires sudo)
add_hosts_entry() {
  local domain="$1"

  if hosts_entry_exists "$domain"; then
    return 0
  fi

  echo "127.0.0.1 $domain" | sudo tee -a /etc/hosts >/dev/null
  return $?
}

# Check and add all required hosts entries
ensure_hosts_entries() {
  local base_domain="${1:-localhost}"
  local project_name="${2:-nself}"
  local needs_update=false
  local missing_domains=()

  # Domains we need based on the base domain - initialize as array
  local required_domains
  required_domains=()

  if [[ "$base_domain" == "localhost" ]]; then
    # Core subdomains
    required_domains=(
      "api.localhost"
      "auth.localhost"
    )

    # Optional service domains if enabled
    [[ "${STORAGE_ENABLED:-false}" == "true" ]] && required_domains+=("storage.localhost")
    [[ "${HASURA_ENABLED:-true}" == "true" ]] && required_domains+=("console.localhost")
    [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]] && required_domains+=("functions.localhost")
    [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]] && required_domains+=("admin.localhost")
    [[ "${MINIO_ENABLED:-false}" == "true" ]] && required_domains+=("minio.localhost")
    [[ "${MAILPIT_ENABLED:-false}" == "true" ]] && required_domains+=("mail.localhost")
    [[ "${MEILISEARCH_ENABLED:-false}" == "true" ]] && required_domains+=("search.localhost")
    [[ "${MLFLOW_ENABLED:-false}" == "true" ]] && required_domains+=("mlflow.localhost")

    # Monitoring domains
    [[ "${GRAFANA_ENABLED:-false}" == "true" ]] && required_domains+=("grafana.localhost")
    [[ "${PROMETHEUS_ENABLED:-false}" == "true" ]] && required_domains+=("prometheus.localhost")
    [[ "${ALERTMANAGER_ENABLED:-false}" == "true" ]] && required_domains+=("alertmanager.localhost")

    # Custom service domains
    for i in {1..10}; do
      local cs_var="CS_${i}"
      local cs_route_var="CS_${i}_ROUTE"
      local cs_public_var="CS_${i}_PUBLIC"

      if [[ -n "${!cs_var:-}" ]] && [[ "${!cs_public_var:-true}" == "true" ]] && [[ -n "${!cs_route_var:-}" ]]; then
        required_domains+=("${!cs_route_var}.localhost")
      fi
    done

    # Frontend app domains
    for i in {1..10}; do
      local app_route_var="FRONTEND_APP_${i}_ROUTE"
      local app_name_var="FRONTEND_APP_${i}_NAME"

      if [[ -n "${!app_name_var:-}" ]] && [[ -n "${!app_route_var:-}" ]]; then
        required_domains+=("${!app_route_var}.localhost")
        # Also add API and auth subdomains for frontend apps if needed
        required_domains+=("api.${!app_route_var}.localhost")
        required_domains+=("auth.${!app_route_var}.localhost")
      fi
    done
  elif [[ "$base_domain" == "local.nself.org" ]]; then
    # local.nself.org uses wildcard DNS, no hosts entries needed
    return 0
  else
    # For custom domains, check if they resolve
    if ! host "$base_domain" >/dev/null 2>&1; then
      required_domains+=("$base_domain" "api.$base_domain" "auth.$base_domain" "storage.$base_domain")
    fi
  fi

  # Check which domains are missing
  if [[ ${#required_domains[@]} -gt 0 ]]; then
    for domain in "${required_domains[@]}"; do
      if ! hosts_entry_exists "$domain"; then
        missing_domains+=("$domain")
        needs_update=true
      fi
    done
  fi

  if [[ "$needs_update" == "false" ]]; then
    return 0
  fi

  # Inform user about missing entries more concisely
  printf "\n✓ %d entries need to be added to /etc/hosts for routing (requires sudo)\n" "${#missing_domains[@]}"

  # Check if we can use sudo without password
  local can_sudo_nopass=false
  if sudo -n true 2>/dev/null; then
    can_sudo_nopass=true
  fi

  # Check if we're in an interactive terminal
  local is_interactive=false
  if [[ -t 0 ]]; then
    is_interactive=true
  fi

  # Determine the best approach
  if [[ "$can_sudo_nopass" == "true" ]]; then
    # Can sudo without password, just do it
    echo "Adding entries to /etc/hosts..."

    # Build the entries string
    local entries=""
    if [[ ${#missing_domains[@]} -gt 0 ]]; then
      for domain in "${missing_domains[@]}"; do
        entries+="127.0.0.1 $domain\n"
      done
    fi

    # Add all entries at once
    if printf "%s" "$entries" | sudo tee -a /etc/hosts >/dev/null; then
      echo "✅ Successfully added ${#missing_domains[@]} entries to /etc/hosts"
      return 0
    else
      echo "❌ Failed to update /etc/hosts"
    fi
  elif [[ "$is_interactive" == "true" ]]; then
    # Build the entries string
    local entries=""
    if [[ ${#missing_domains[@]} -gt 0 ]]; then
      for domain in "${missing_domains[@]}"; do
        entries+="127.0.0.1 $domain\n"
      done
    fi

    # Try to add entries with sudo
    if printf "%s" "$entries" | sudo tee -a /etc/hosts >/dev/null 2>&1; then
      printf "\n✓ Successfully added %d entries to /etc/hosts\n" "${#missing_domains[@]}"
      return 0
    else
      echo "✗ Failed to update /etc/hosts"
    fi
  else
    # Non-interactive or can't sudo
    echo "⚠️  Cannot automatically update /etc/hosts (non-interactive terminal)"
  fi

  # Show manual instructions if we couldn't update
  if [[ "$can_sudo_nopass" != "true" ]] || [[ "$is_interactive" != "true" ]]; then
    echo ""
    echo "You can manually add these lines to /etc/hosts:"
    if [[ ${#missing_domains[@]} -gt 0 ]]; then
      for domain in "${missing_domains[@]}"; do
        echo "127.0.0.1 $domain"
      done
    fi
    echo ""
    echo "Or run with sudo: sudo nself start"
    echo "Or use BASE_DOMAIN=local.nself.org which doesn't require /etc/hosts changes."
  fi

  return 0
}

# Remove nself entries from /etc/hosts (cleanup)
remove_hosts_entries() {
  local base_domain="${1:-localhost}"

  if [[ "$base_domain" == "local.nself.org" ]]; then
    return 0 # No entries to remove for wildcard domain
  fi

  echo "Removing nself entries from /etc/hosts (requires sudo)..."

  # Create a pattern to match our entries
  local pattern="127\.0\.0\.1.*\.\(localhost\|${base_domain}\)"

  # Remove matching lines
  sudo sed -i.bak "/$pattern/d" /etc/hosts

  echo "✅ Cleaned up /etc/hosts entries"
}

# Check if we can resolve a domain
can_resolve_domain() {
  local domain="$1"

  # First check if it's in /etc/hosts
  if hosts_entry_exists "$domain"; then
    return 0
  fi

  # Then check DNS resolution
  if host "$domain" >/dev/null 2>&1; then
    return 0
  fi

  # Check if it resolves to localhost via ping
  if ping -c 1 -W 1 "$domain" 2>/dev/null | grep -q "127.0.0.1"; then
    return 0
  fi

  return 1
}

# Export functions
export -f hosts_entry_exists
export -f add_hosts_entry
export -f ensure_hosts_entries
export -f remove_hosts_entries
export -f can_resolve_domain
