#!/usr/bin/env bash
# topological_sort.sh — Topological sort for plugin dependency ordering (T-0344).
#
# Usage:
#   source topological_sort.sh
#   # Define edges: plugin_add_dep <plugin> <dependency>
#   plugin_add_dep claw ai
#   plugin_add_dep claw mux
#   plugin_add_dep claw notify
#   plugin_add_dep mux redis
#   # Sort
#   result=$(plugin_topo_sort "claw")
#   # result: "notify ai mux claw" (dependencies first)
#
# Bash 3.2 compatible — no declare -A, no echo -e.
#
# Implementation: Kahn's algorithm using parallel arrays for adjacency.
# Each plugin's dependencies are stored as space-separated strings.

# ---------------------------------------------------------------------------
# Built-in dependency graph (from plugin manifests).
# Mirrors what the CLI reads from plugin.json `dependencies` fields.
# ---------------------------------------------------------------------------

# Format: PLUGIN_DEPS_<name>=<space-separated deps>
# These are set in the manifest but hardcoded here for Bash 3.2 compat.
PLUGIN_DEPS_claw="ai mux notify"
PLUGIN_DEPS_mux=""
PLUGIN_DEPS_ai=""
PLUGIN_DEPS_notify=""
PLUGIN_DEPS_voice="notify"
PLUGIN_DEPS_browser=""
PLUGIN_DEPS_cron=""
PLUGIN_DEPS_google=""

# ---------------------------------------------------------------------------
# plugin_get_deps <plugin>
#   Outputs the direct dependencies of <plugin>.
# ---------------------------------------------------------------------------
plugin_get_deps() {
  local plugin="$1"
  local var_name="PLUGIN_DEPS_${plugin}"
  # Eval is safe here — var_name is controlled, never user input
  local deps
  eval "deps=\"\${${var_name}:-}\""
  printf "%s\n" "$deps"
}

# ---------------------------------------------------------------------------
# plugin_topo_sort <plugin1> [plugin2 ...]
#   Outputs a newline-separated ordered list of plugins to install,
#   dependencies before the plugins that depend on them.
#   Handles multiple requested plugins simultaneously.
# ---------------------------------------------------------------------------
plugin_topo_sort() {
  # Collect all unique plugins needed (requested + transitive deps)
  local requested="$*"
  local all_plugins=""
  local queue="$requested"

  # Expand transitive dependencies
  local visited=""
  while [ -n "$queue" ]; do
    local item
    item=$(printf "%s" "$queue" | awk '{print $1}')
    queue=$(printf "%s" "$queue" | cut -d' ' -f2-)
    # Trim leading/trailing spaces
    queue="${queue# }"
    queue="${queue% }"

    # Skip if already visited
    if printf "%s" "$visited" | grep -qw "$item"; then
      continue
    fi
    visited="$visited $item"
    all_plugins="$all_plugins $item"

    # Add deps to queue
    local deps
    deps=$(plugin_get_deps "$item")
    for dep in $deps; do
      if ! printf "%s" "$visited" | grep -qw "$dep"; then
        queue="$queue $dep"
      fi
    done
  done

  # Trim
  all_plugins="${all_plugins# }"

  # Kahn's algorithm — compute in-degrees and sort
  # In-degree: number of unresolved dependencies
  local sorted=""
  local remaining="$all_plugins"

  # Max iterations = number of nodes
  local max_iter
  max_iter=$(printf "%s\n" $all_plugins | wc -l | tr -d ' ')
  local iter=0

  while [ -n "$remaining" ]; do
    iter=$((iter + 1))
    if [ $iter -gt $((max_iter + 5)) ]; then
      printf "[topo_sort] ERROR: Circular dependency detected in: %s\n" "$remaining" >&2
      return 1
    fi

    local ready=""
    local still_pending=""

    for plugin in $remaining; do
      local deps
      deps=$(plugin_get_deps "$plugin")
      local all_deps_met=true

      for dep in $deps; do
        # dep is met if it's in the sorted list OR not in remaining at all
        if ! printf "%s\n" $remaining | grep -qx "$dep"; then
          : # dep already sorted or not in the install set
        else
          # dep is still pending
          all_deps_met=false
          break
        fi
      done

      if $all_deps_met; then
        ready="$ready $plugin"
      else
        still_pending="$still_pending $plugin"
      fi
    done

    # Trim
    ready="${ready# }"
    still_pending="${still_pending# }"

    if [ -z "$ready" ] && [ -n "$still_pending" ]; then
      printf "[topo_sort] ERROR: No progress — circular dependency in: %s\n" "$still_pending" >&2
      return 1
    fi

    sorted="$sorted $ready"
    remaining="$still_pending"
  done

  # Output one plugin per line, trimmed
  for plugin in $sorted; do
    printf "%s\n" "$plugin"
  done
}

# ---------------------------------------------------------------------------
# plugin_install_ordered <plugin1> [plugin2 ...]
#   Resolves dependencies and prints the install order with progress messages.
# ---------------------------------------------------------------------------
plugin_install_ordered() {
  local requested="$*"
  local ordered

  ordered=$(plugin_topo_sort $requested) || {
    printf "[plugin] ERROR: Failed to resolve dependency order\n" >&2
    return 1
  }

  local total
  total=$(printf "%s\n" "$ordered" | wc -l | tr -d ' ')

  printf "Installing dependencies (%s):\n" "$total"

  local i=1
  for plugin in $ordered; do
    printf "  [%d/%d] nself-%s\n" "$i" "$total" "$plugin"
    i=$((i + 1))
  done
}

export -f plugin_get_deps
export -f plugin_topo_sort
export -f plugin_install_ordered
