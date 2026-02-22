#!/usr/bin/env bash
# dependencies.sh - System dependency management for plugins

# Detect package manager
detect_package_manager() {

set -euo pipefail

  if command -v apt-get >/dev/null 2>&1; then
    echo "apt"
  elif command -v brew >/dev/null 2>&1; then
    echo "brew"
  elif command -v yum >/dev/null 2>&1; then
    echo "yum"
  elif command -v dnf >/dev/null 2>&1; then
    echo "dnf"
  elif command -v pacman >/dev/null 2>&1; then
    echo "pacman"
  elif command -v apk >/dev/null 2>&1; then
    echo "apk"
  else
    echo "unknown"
  fi
}

# Check if a command exists
command_exists() {
  command -v "$1" >/dev/null 2>&1
}

# Verify a dependency is installed
verify_dependency() {
  local name="$1"
  local verify_cmd="$2"
  
  if [[ -z "$verify_cmd" ]]; then
    # Default: check if command exists
    command_exists "$name"
  else
    # Run custom verification command
    eval "$verify_cmd" >/dev/null 2>&1
  fi
}

# Get version from command
get_version() {
  local verify_cmd="$1"
  local output
  
  output=$(eval "$verify_cmd" 2>&1)
  
  # Try to extract version number (common patterns)
  if [[ "$output" =~ ([0-9]+\.[0-9]+(\.[0-9]+)?) ]]; then
    echo "${BASH_REMATCH[1]}"
  else
    echo "unknown"
  fi
}

# Compare versions (returns 0 if v1 >= v2)
version_compare() {
  local v1="$1"
  local v2="$2"
  
  [[ "$v1" == "unknown" ]] && return 1
  [[ "$v2" == "unknown" ]] && return 0
  
  # Remove 'v' prefix
  v1="${v1#v}"
  v2="${v2#v}"
  
  # Split and compare
  local v1_major v1_minor v1_patch
  local v2_major v2_minor v2_patch
  
  IFS='.' read -r v1_major v1_minor v1_patch <<<"$v1"
  IFS='.' read -r v2_major v2_minor v2_patch <<<"$v2"
  
  v1_major="${v1_major:-0}"
  v1_minor="${v1_minor:-0}"
  v1_patch="${v1_patch:-0}"
  v2_major="${v2_major:-0}"
  v2_minor="${v2_minor:-0}"
  v2_patch="${v2_patch:-0}"
  
  if ((v1_major > v2_major)); then
    return 0
  elif ((v1_major < v2_major)); then
    return 1
  fi
  
  if ((v1_minor > v2_minor)); then
    return 0
  elif ((v1_minor < v2_minor)); then
    return 1
  fi
  
  if ((v1_patch >= v2_patch)); then
    return 0
  else
    return 1
  fi
}

# Parse systemDependencies from plugin.json
parse_system_dependencies() {
  local manifest="$1"
  local dep_type="${2:-required}" # required or recommended
  
  [[ ! -f "$manifest" ]] && return 1
  
  # Extract dependencies section (crude but Bash 3.2 compatible)
  local in_section=false
  local in_type=false
  local brace_count=0
  local deps=""
  
  while IFS= read -r line; do
    # Detect systemDependencies section
    if [[ "$line" =~ \"systemDependencies\" ]]; then
      in_section=true
      continue
    fi
    
    if [[ "$in_section" == "true" ]]; then
      # Detect dependency type (required/recommended)
      if [[ "$line" =~ \"$dep_type\" ]]; then
        in_type=true
        continue
      fi
      
      if [[ "$in_type" == "true" ]]; then
        # Count braces to track nesting
        local open_braces=$(echo "$line" | tr -cd '[' | wc -c | tr -d ' ')
        local close_braces=$(echo "$line" | tr -cd ']' | wc -c | tr -d ' ')
        
        ((brace_count += open_braces - close_braces))
        
        deps+="$line"$'\n'
        
        # End of dependency type array
        if ((brace_count < 0)); then
          break
        fi
      fi
    fi
  done < "$manifest"
  
  echo "$deps"
}

# Extract dependency field value
get_dep_field() {
  local dep_json="$1"
  local field="$2"
  
  echo "$dep_json" | grep "\"$field\"" | head -1 | sed 's/.*"'"$field"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
}

# Check all system dependencies for a plugin
check_plugin_dependencies() {
  local plugin_name="$1"
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"
  
  [[ ! -f "$manifest" ]] && return 0
  
  printf "\n${CLI_BOLD}System Dependencies for ${plugin_name}:${CLI_RESET}\n\n"
  
  local missing_required=0
  local missing_recommended=0
  
  # Check required dependencies
  printf "${CLI_DIM}Required:${CLI_RESET}\n"
  local required_deps=$(parse_system_dependencies "$manifest" "required")
  
  if [[ -z "$required_deps" ]]; then
    printf "  ${CLI_DIM}(none)${CLI_RESET}\n"
  else
    # Parse each dependency (simple line-by-line)
    local current_dep=""
    while IFS= read -r line; do
      if [[ "$line" =~ \{$ ]]; then
        current_dep=""
      elif [[ "$line" =~ \"name\" ]]; then
        local name=$(echo "$line" | sed 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
        current_dep="$name"
      elif [[ "$line" =~ \"verify\" ]]; then
        local verify_cmd=$(echo "$line" | sed 's/.*"verify"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
        local min_version=$(echo "$required_deps" | grep -A10 "\"name\"[[:space:]]*:[[:space:]]*\"$current_dep\"" | grep '"minVersion"' | sed 's/.*"minVersion"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
        
        if verify_dependency "$current_dep" "$verify_cmd"; then
          local version=$(get_version "$verify_cmd")
          
          # Check minimum version if specified
          if [[ -n "$min_version" ]] && ! version_compare "$version" "$min_version"; then
            printf "  ${CLI_YELLOW}⚠${CLI_RESET} %-20s v%s ${CLI_YELLOW}(requires ≥%s)${CLI_RESET}\n" "$current_dep" "$version" "$min_version"
            missing_required=$((missing_required + 1))
          else
            printf "  ${CLI_GREEN}✓${CLI_RESET} %-20s v%s\n" "$current_dep" "$version"
          fi
        else
          printf "  ${CLI_RED}✗${CLI_RESET} %-20s ${CLI_RED}not installed${CLI_RESET}\n" "$current_dep"
          missing_required=$((missing_required + 1))
        fi
      fi
    done <<< "$required_deps"
  fi
  
  # Check recommended dependencies
  printf "\n${CLI_DIM}Recommended:${CLI_RESET}\n"
  local recommended_deps=$(parse_system_dependencies "$manifest" "recommended")
  
  if [[ -z "$recommended_deps" ]]; then
    printf "  ${CLI_DIM}(none)${CLI_RESET}\n"
  else
    # Similar parsing for recommended deps
    local current_dep=""
    while IFS= read -r line; do
      if [[ "$line" =~ \{$ ]]; then
        current_dep=""
      elif [[ "$line" =~ \"name\" ]]; then
        local name=$(echo "$line" | sed 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
        current_dep="$name"
      elif [[ "$line" =~ \"verify\" ]]; then
        local verify_cmd=$(echo "$line" | sed 's/.*"verify"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
        
        if verify_dependency "$current_dep" "$verify_cmd"; then
          local version=$(get_version "$verify_cmd")
          printf "  ${CLI_GREEN}✓${CLI_RESET} %-20s v%s\n" "$current_dep" "$version"
        else
          printf "  ${CLI_DIM}○${CLI_RESET} %-20s ${CLI_DIM}not installed${CLI_RESET}\n" "$current_dep"
          missing_recommended=$((missing_recommended + 1))
        fi
      fi
    done <<< "$recommended_deps"
  fi
  
  printf "\n"
  
  # Return status
  if [[ $missing_required -gt 0 ]]; then
    printf "${CLI_RED}✗${CLI_RESET} %d required dependencies missing\n" "$missing_required"
    printf "Install with: ${CLI_CYAN}nself plugin %s install-deps${CLI_RESET}\n\n" "$plugin_name"
    return 1
  elif [[ $missing_recommended -gt 0 ]]; then
    printf "${CLI_YELLOW}⚠${CLI_RESET} %d recommended dependencies missing (optional)\n\n" "$missing_recommended"
    return 0
  else
    printf "${CLI_GREEN}✓${CLI_RESET} All dependencies satisfied\n\n"
    return 0
  fi
}

# Install system dependencies for a plugin
install_plugin_dependencies() {
  local plugin_name="$1"
  local check_only="${2:-false}"
  local manifest="$PLUGIN_DIR/$plugin_name/plugin.json"
  
  [[ ! -f "$manifest" ]] && return 1
  
  # Detect package manager
  local pkg_mgr=$(detect_package_manager)
  
  if [[ "$pkg_mgr" == "unknown" ]]; then
    printf "${CLI_RED}Error:${CLI_RESET} Could not detect package manager\n"
    printf "Supported: apt, brew, yum, dnf, pacman, apk\n"
    return 1
  fi
  
  printf "\n${CLI_BOLD}Installing dependencies for ${plugin_name}${CLI_RESET}\n"
  printf "Package manager: ${CLI_CYAN}%s${CLI_RESET}\n\n" "$pkg_mgr"
  
  # Parse required dependencies
  local required_deps=$(parse_system_dependencies "$manifest" "required")
  
  if [[ -z "$required_deps" ]]; then
    printf "${CLI_GREEN}✓${CLI_RESET} No system dependencies required\n\n"
    return 0
  fi
  
  # Track what needs installing
  local to_install=()
  
  # Parse and check each dependency
  local current_dep=""
  local current_verify=""
  local current_pkg=""
  
  while IFS= read -r line; do
    if [[ "$line" =~ \"name\" ]]; then
      current_dep=$(echo "$line" | sed 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    elif [[ "$line" =~ \"verify\" ]]; then
      current_verify=$(echo "$line" | sed 's/.*"verify"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    elif [[ "$line" =~ \"$pkg_mgr\" ]]; then
      current_pkg=$(echo "$line" | sed 's/.*"'"$pkg_mgr"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

      # Check if already installed
      if ! verify_dependency "$current_dep" "$current_verify"; then
        # BUG FIX: Split space-delimited package strings into individual array elements
        IFS=' ' read -ra _pkgs <<< "$current_pkg"
        for _pkg in "${_pkgs[@]}"; do
          to_install+=("$_pkg")
        done
        printf "${CLI_BLUE}→${CLI_RESET} Will install: %s\n" "$current_pkg"
      else
        printf "${CLI_GREEN}✓${CLI_RESET} Already installed: %s\n" "$current_dep"
      fi
    elif [[ "$line" =~ \"custom_install\" ]]; then
      # BUG FIX: Handle custom_install commands
      local custom_cmd=$(echo "$line" | sed 's/.*"custom_install"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

      # Check if already installed
      if ! verify_dependency "$current_dep" "$current_verify"; then
        printf "${CLI_BLUE}→${CLI_RESET} Will run custom install for: %s\n" "$current_dep"

        if [[ "$check_only" != "true" ]]; then
          printf "  Command: %s\n" "$custom_cmd"
          printf "  Run custom install? [Y/n]: "
          read -r custom_response
          custom_response=$(echo "$custom_response" | tr '[:upper:]' '[:lower:]')

          if [[ "$custom_response" != "n" && "$custom_response" != "no" ]]; then
            eval "$custom_cmd"
            if [[ $? -eq 0 ]]; then
              printf "${CLI_GREEN}✓${CLI_RESET} Custom install succeeded: %s\n" "$current_dep"
            else
              printf "${CLI_RED}✗${CLI_RESET} Custom install failed: %s\n" "$current_dep"
            fi
          else
            printf "${CLI_YELLOW}⚠${CLI_RESET} Skipped: %s\n" "$current_dep"
          fi
        fi
      else
        printf "${CLI_GREEN}✓${CLI_RESET} Already installed: %s\n" "$current_dep"
      fi
    fi
  done <<< "$required_deps"

  # Also process recommended dependencies (optional, with default=no)
  local recommended_deps=$(parse_system_dependencies "$manifest" "recommended")

  if [[ -n "$recommended_deps" ]]; then
    printf "\n${CLI_DIM}Recommended dependencies:${CLI_RESET}\n"

    local current_dep=""
    local current_verify=""

    while IFS= read -r line; do
      if [[ "$line" =~ \"name\" ]]; then
        current_dep=$(echo "$line" | sed 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
      elif [[ "$line" =~ \"verify\" ]]; then
        current_verify=$(echo "$line" | sed 's/.*"verify"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
      elif [[ "$line" =~ \"$pkg_mgr\" ]]; then
        local current_pkg=$(echo "$line" | sed 's/.*"'"$pkg_mgr"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

        if ! verify_dependency "$current_dep" "$current_verify"; then
          # Split space-delimited packages
          IFS=' ' read -ra _pkgs <<< "$current_pkg"
          for _pkg in "${_pkgs[@]}"; do
            to_install+=("$_pkg")
          done
          printf "${CLI_BLUE}→${CLI_RESET} Available (recommended): %s\n" "$current_pkg"
        else
          printf "${CLI_GREEN}✓${CLI_RESET} Already installed: %s\n" "$current_dep"
        fi
      elif [[ "$line" =~ \"custom_install\" ]]; then
        local custom_cmd=$(echo "$line" | sed 's/.*"custom_install"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

        if ! verify_dependency "$current_dep" "$current_verify"; then
          printf "${CLI_BLUE}→${CLI_RESET} Recommended custom install: %s\n" "$current_dep"

          if [[ "$check_only" != "true" ]]; then
            printf "  Command: %s\n" "$custom_cmd"
            printf "  Install recommended dependency? [y/N]: "
            read -r custom_response
            custom_response=$(echo "$custom_response" | tr '[:upper:]' '[:lower:]')

            if [[ "$custom_response" == "y" || "$custom_response" == "yes" ]]; then
              eval "$custom_cmd"
              if [[ $? -eq 0 ]]; then
                printf "${CLI_GREEN}✓${CLI_RESET} Custom install succeeded: %s\n" "$current_dep"
              else
                printf "${CLI_RED}✗${CLI_RESET} Custom install failed: %s\n" "$current_dep"
              fi
            else
              printf "${CLI_DIM}○${CLI_RESET} Skipped: %s\n" "$current_dep"
            fi
          fi
        else
          printf "${CLI_GREEN}✓${CLI_RESET} Already installed: %s\n" "$current_dep"
        fi
      fi
    done <<< "$recommended_deps"
  fi

  if [[ ${#to_install[@]} -eq 0 ]]; then
    printf "\n${CLI_GREEN}✓${CLI_RESET} All required dependencies already installed\n\n"
    return 0
  fi
  
  # Check-only mode
  if [[ "$check_only" == "true" ]]; then
    printf "\n${CLI_YELLOW}⚠${CLI_RESET} Dry run - would install:\n"
    for pkg in "${to_install[@]}"; do
      printf "  - %s\n" "$pkg"
    done
    printf "\n"
    return 0
  fi
  
  # Confirm installation
  printf "\nInstall %d package(s)? [Y/n]: " "${#to_install[@]}"
  read -r response
  response=$(echo "$response" | tr '[:upper:]' '[:lower:]')
  
  if [[ "$response" == "n" || "$response" == "no" ]]; then
    printf "${CLI_YELLOW}⚠${CLI_RESET} Installation cancelled\n\n"
    return 1
  fi
  
  # Install packages
  printf "\n"
  case "$pkg_mgr" in
    apt)
      sudo apt-get update
      sudo apt-get install -y "${to_install[@]}"
      ;;
    brew)
      brew install "${to_install[@]}"
      ;;
    yum|dnf)
      sudo $pkg_mgr install -y "${to_install[@]}"
      ;;
    pacman)
      sudo pacman -S --noconfirm "${to_install[@]}"
      ;;
    apk)
      sudo apk add "${to_install[@]}"
      ;;
  esac
  
  if [[ $? -eq 0 ]]; then
    printf "\n${CLI_GREEN}✓${CLI_RESET} Dependencies installed successfully\n\n"
    return 0
  else
    printf "\n${CLI_RED}✗${CLI_RESET} Installation failed\n\n"
    return 1
  fi
}

# Export functions
export -f detect_package_manager
export -f command_exists
export -f verify_dependency
export -f get_version
export -f version_compare
export -f parse_system_dependencies
export -f get_dep_field
export -f check_plugin_dependencies
export -f install_plugin_dependencies
