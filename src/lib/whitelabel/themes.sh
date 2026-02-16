#!/usr/bin/env bash

# nself Theme System - FULL IMPLEMENTATION
# Manages theme creation, CSS variables, dark/light mode, and theme preview
# Part of Sprint 14: White-Label & Customization (60pts) for v0.9.0
#
# Features:
# - JSONB configuration storage in PostgreSQL
# - CSS variable generation from theme config
# - Theme inheritance and preview
# - Validation of theme configurations
# - Multi-tenant theme isolation
# - Built-in themes: light, dark, high-contrast


# Color definitions for output (guard against double-declaration when sourced together)
[[ -z "${RED:-}" ]] && readonly RED='\033[0;31m'

set -euo pipefail

[[ -z "${GREEN:-}" ]] && readonly GREEN='\033[0;32m'
[[ -z "${YELLOW:-}" ]] && readonly YELLOW='\033[1;33m'
[[ -z "${BLUE:-}" ]] && readonly BLUE='\033[0;34m'
[[ -z "${CYAN:-}" ]] && readonly CYAN='\033[0;36m'
[[ -z "${MAGENTA:-}" ]] && readonly MAGENTA='\033[0;35m'
[[ -z "${NC:-}" ]] && readonly NC='\033[0m'
[[ -z "${BOLD:-}" ]] && readonly BOLD='\033[1m'

# Theme configuration
readonly THEMES_DIR="${PROJECT_ROOT:-$(pwd)}/branding/themes"
readonly ACTIVE_THEME_FILE="${THEMES_DIR}/.active"

# Default theme templates
readonly DEFAULT_THEMES="light dark high-contrast"

# Database configuration
: "${POSTGRES_DB:=nself_db}"
: "${POSTGRES_USER:=postgres}"
: "${POSTGRES_PASSWORD:=postgres}"
: "${POSTGRES_HOST:=localhost}"
: "${POSTGRES_PORT:=5432}"
: "${PROJECT_NAME:=nself}"

# ============================================================================
# Database Helper Functions
# ============================================================================

# Execute SQL query
exec_sql() {
  local query="$1"
  local output_format="${2:-tuples_only}"

  local docker_cmd="docker exec -i ${PROJECT_NAME}_postgres"

  case "$output_format" in
    tuples_only)
      $docker_cmd psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -A -c "$query" 2>/dev/null || echo ""
      ;;
    json)
      $docker_cmd psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -A -c "$query" 2>/dev/null || echo "{}"
      ;;
    table)
      $docker_cmd psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$query" 2>/dev/null || echo ""
      ;;
    *)
      $docker_cmd psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$query" 2>/dev/null || echo ""
      ;;
  esac
}

# Execute SQL file
exec_sql_file() {
  local file_path="$1"

  if [[ ! -f "$file_path" ]]; then
    printf "${RED}Error: SQL file not found: %s${NC}\n" "$file_path" >&2
    return 1
  fi

  docker exec -i "${PROJECT_NAME}_postgres" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" <"$file_path" 2>/dev/null
}

# Check if database is available
check_database_connection() {
  if ! docker ps | grep -q "${PROJECT_NAME}_postgres"; then
    printf "${RED}Error: PostgreSQL container not running${NC}\n" >&2
    printf "${YELLOW}Run 'nself start' first${NC}\n" >&2
    return 1
  fi

  local result
  result=$(exec_sql "SELECT 1;" "tuples_only" 2>/dev/null || echo "")

  if [[ "$result" != "1" ]]; then
    printf "${RED}Error: Cannot connect to database${NC}\n" >&2
    return 1
  fi

  return 0
}

# Get default brand ID
get_default_brand_id() {
  local brand_id
  brand_id=$(exec_sql "SELECT id FROM whitelabel_brands WHERE tenant_id = 'default' LIMIT 1;" "tuples_only")

  if [[ -z "$brand_id" ]]; then
    # Create default brand if doesn't exist
    brand_id=$(exec_sql "INSERT INTO whitelabel_brands (tenant_id, brand_name, is_primary, is_active) VALUES ('default', 'nself', true, true) RETURNING id;" "tuples_only")
  fi

  echo "$brand_id"
}

# Get brand ID by tenant
get_brand_id_by_tenant() {
  local tenant_id="${1:-default}"
  local brand_id

  brand_id=$(exec_sql "SELECT id FROM whitelabel_brands WHERE tenant_id = '$tenant_id' LIMIT 1;" "tuples_only")

  if [[ -z "$brand_id" ]]; then
    printf "${RED}Error: Brand not found for tenant: %s${NC}\n" "$tenant_id" >&2
    return 1
  fi

  echo "$brand_id"
}

# ============================================================================
# Theme System Initialization
# ============================================================================

initialize_themes_system() {
  printf "${CYAN}Initializing theme system...${NC}\n"

  # Check database connection
  if ! check_database_connection; then
    return 1
  fi

  # Create themes directory for local cache
  mkdir -p "$THEMES_DIR"

  # Check if themes table exists
  local table_exists
  table_exists=$(exec_sql "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'whitelabel_themes');" "tuples_only")

  if [[ "$table_exists" != "t" ]]; then
    printf "${YELLOW}Warning: whitelabel_themes table not found. Run migrations first.${NC}\n"
    printf "${YELLOW}Run: nself migrate run 016_create_whitelabel_system${NC}\n"
    return 1
  fi

  # Get default brand
  local brand_id
  brand_id=$(get_default_brand_id)

  if [[ -z "$brand_id" ]]; then
    printf "${RED}Error: Could not create or find default brand${NC}\n" >&2
    return 1
  fi

  # Create default themes in database if they don't exist
  for theme in $DEFAULT_THEMES; do
    create_default_theme "$theme" "$brand_id"
  done

  # Set light theme as active if no active theme
  local active_theme
  active_theme=$(exec_sql "SELECT theme_name FROM whitelabel_themes WHERE brand_id = '$brand_id' AND is_active = true LIMIT 1;" "tuples_only")

  if [[ -z "$active_theme" ]]; then
    exec_sql "UPDATE whitelabel_themes SET is_active = true WHERE brand_id = '$brand_id' AND theme_name = 'light';"
    printf "light" >"$ACTIVE_THEME_FILE"
  else
    printf "%s" "$active_theme" >"$ACTIVE_THEME_FILE"
  fi

  printf "${GREEN}✓${NC} Theme system initialized\n"
  printf "${BLUE}Active theme:${NC} %s\n" "$(cat "$ACTIVE_THEME_FILE")"

  return 0
}

# ============================================================================
# Default Theme Creation
# ============================================================================

create_default_theme() {
  local theme_name="$1"
  local brand_id="$2"

  # Check if theme already exists in database
  local exists
  exists=$(exec_sql "SELECT EXISTS (SELECT 1 FROM whitelabel_themes WHERE brand_id = '$brand_id' AND theme_name = '$theme_name');" "tuples_only")

  if [[ "$exists" == "t" ]]; then
    return 0
  fi

  printf "${CYAN}Creating default theme: %s${NC}\n" "$theme_name"

  case "$theme_name" in
    light)
      create_light_theme_db "$brand_id"
      ;;
    dark)
      create_dark_theme_db "$brand_id"
      ;;
    high-contrast)
      create_high_contrast_theme_db "$brand_id"
      ;;
    *)
      create_custom_theme_db "$theme_name" "$brand_id"
      ;;
  esac

  # Also create local file cache
  local theme_dir="${THEMES_DIR}/${theme_name}"
  mkdir -p "$theme_dir"

  # Export from DB to local file
  export_theme_to_file "$theme_name" "$brand_id" "${theme_dir}/theme.json"

  # Generate CSS
  if [[ -f "${theme_dir}/theme.json" ]]; then
    generate_theme_css "${theme_dir}/theme.json" "${theme_dir}/theme.css"
  fi

  return 0
}

create_light_theme_db() {
  local brand_id="$1"

  local colors_json
  colors_json=$(
    cat <<'EOF'
{
  "primary": "#0066cc",
  "primaryHover": "#0052a3",
  "secondary": "#6c757d",
  "accent": "#00cc66",
  "background": "#ffffff",
  "backgroundAlt": "#f8f9fa",
  "surface": "#ffffff",
  "surfaceAlt": "#f1f3f5",
  "text": "#212529",
  "textSecondary": "#6c757d",
  "textMuted": "#adb5bd",
  "border": "#dee2e6",
  "borderLight": "#e9ecef",
  "success": "#28a745",
  "warning": "#ffc107",
  "error": "#dc3545",
  "info": "#17a2b8"
}
EOF
  )

  local typography_json
  typography_json=$(
    cat <<'EOF'
{
  "fontFamily": "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
  "fontFamilyMono": "'Fira Code', 'Courier New', monospace",
  "fontSize": "16px",
  "fontWeight": "400",
  "lineHeight": "1.5"
}
EOF
  )

  local spacing_json
  spacing_json=$(
    cat <<'EOF'
{
  "xs": "4px",
  "sm": "8px",
  "md": "16px",
  "lg": "24px",
  "xl": "32px",
  "xxl": "48px"
}
EOF
  )

  local borders_json
  borders_json=$(
    cat <<'EOF'
{
  "radius": "4px",
  "radiusLg": "8px",
  "width": "1px"
}
EOF
  )

  local shadows_json
  shadows_json=$(
    cat <<'EOF'
{
  "sm": "0 1px 3px rgba(0,0,0,0.12)",
  "md": "0 4px 6px rgba(0,0,0,0.1)",
  "lg": "0 10px 20px rgba(0,0,0,0.15)"
}
EOF
  )

  # Escape single quotes for SQL
  colors_json=$(echo "$colors_json" | sed "s/'/''/g")
  typography_json=$(echo "$typography_json" | sed "s/'/''/g")
  spacing_json=$(echo "$spacing_json" | sed "s/'/''/g")
  borders_json=$(echo "$borders_json" | sed "s/'/''/g")
  shadows_json=$(echo "$shadows_json" | sed "s/'/''/g")

  exec_sql "
    INSERT INTO whitelabel_themes (
      brand_id, theme_name, display_name, description, version, author,
      mode, colors, typography, spacing, borders, shadows,
      is_default, is_system, is_active
    ) VALUES (
      '$brand_id',
      'light',
      'Light Theme',
      'Clean and bright light theme',
      '1.0.0',
      'nself',
      'light',
      '$colors_json'::jsonb,
      '$typography_json'::jsonb,
      '$spacing_json'::jsonb,
      '$borders_json'::jsonb,
      '$shadows_json'::jsonb,
      true,
      true,
      true
    )
    ON CONFLICT (brand_id, theme_name) DO NOTHING;
  "
}

create_dark_theme_db() {
  local brand_id="$1"

  local colors_json
  colors_json=$(
    cat <<'EOF'
{
  "primary": "#4a9eff",
  "primaryHover": "#6bb0ff",
  "secondary": "#8b949e",
  "accent": "#3fb950",
  "background": "#0d1117",
  "backgroundAlt": "#161b22",
  "surface": "#161b22",
  "surfaceAlt": "#21262d",
  "text": "#c9d1d9",
  "textSecondary": "#8b949e",
  "textMuted": "#6e7681",
  "border": "#30363d",
  "borderLight": "#21262d",
  "success": "#3fb950",
  "warning": "#d29922",
  "error": "#f85149",
  "info": "#58a6ff"
}
EOF
  )

  local typography_json='{"fontFamily": "-apple-system, BlinkMacSystemFont, '\''Segoe UI'\'', Roboto, sans-serif", "fontFamilyMono": "'\''Fira Code'\'', '\''Courier New'\'', monospace", "fontSize": "16px", "fontWeight": "400", "lineHeight": "1.5"}'
  local spacing_json='{"xs": "4px", "sm": "8px", "md": "16px", "lg": "24px", "xl": "32px", "xxl": "48px"}'
  local borders_json='{"radius": "4px", "radiusLg": "8px", "width": "1px"}'
  local shadows_json='{"sm": "0 0 0 1px rgba(255,255,255,0.05)", "md": "0 0 0 1px rgba(255,255,255,0.1)", "lg": "0 0 0 1px rgba(255,255,255,0.15)"}'

  colors_json=$(echo "$colors_json" | sed "s/'/''/g")

  exec_sql "
    INSERT INTO whitelabel_themes (
      brand_id, theme_name, display_name, description, version, author,
      mode, colors, typography, spacing, borders, shadows,
      is_default, is_system, is_active
    ) VALUES (
      '$brand_id',
      'dark',
      'Dark Theme',
      'Easy on the eyes dark theme',
      '1.0.0',
      'nself',
      'dark',
      '$colors_json'::jsonb,
      '$typography_json'::jsonb,
      '$spacing_json'::jsonb,
      '$borders_json'::jsonb,
      '$shadows_json'::jsonb,
      false,
      true,
      false
    )
    ON CONFLICT (brand_id, theme_name) DO NOTHING;
  "
}

create_high_contrast_theme_db() {
  local brand_id="$1"

  local colors_json
  colors_json=$(
    cat <<'EOF'
{
  "primary": "#ffff00",
  "primaryHover": "#ffff66",
  "secondary": "#ffffff",
  "accent": "#00ff00",
  "background": "#000000",
  "backgroundAlt": "#1a1a1a",
  "surface": "#000000",
  "surfaceAlt": "#1a1a1a",
  "text": "#ffffff",
  "textSecondary": "#ffffff",
  "textMuted": "#cccccc",
  "border": "#ffffff",
  "borderLight": "#666666",
  "success": "#00ff00",
  "warning": "#ffff00",
  "error": "#ff0000",
  "info": "#00ffff"
}
EOF
  )

  local typography_json='{"fontFamily": "Arial, sans-serif", "fontFamilyMono": "'\''Courier New'\'', monospace", "fontSize": "18px", "fontWeight": "600", "lineHeight": "1.6"}'
  local spacing_json='{"xs": "6px", "sm": "12px", "md": "20px", "lg": "28px", "xl": "36px", "xxl": "52px"}'
  local borders_json='{"radius": "0px", "radiusLg": "0px", "width": "2px"}'
  local shadows_json='{"sm": "0 0 0 2px #ffffff", "md": "0 0 0 3px #ffffff", "lg": "0 0 0 4px #ffffff"}'

  colors_json=$(echo "$colors_json" | sed "s/'/''/g")

  exec_sql "
    INSERT INTO whitelabel_themes (
      brand_id, theme_name, display_name, description, version, author,
      mode, colors, typography, spacing, borders, shadows,
      is_default, is_system, is_active
    ) VALUES (
      '$brand_id',
      'high-contrast',
      'High Contrast',
      'Maximum contrast for accessibility',
      '1.0.0',
      'nself',
      'dark',
      '$colors_json'::jsonb,
      '$typography_json'::jsonb,
      '$spacing_json'::jsonb,
      '$borders_json'::jsonb,
      '$shadows_json'::jsonb,
      false,
      true,
      false
    )
    ON CONFLICT (brand_id, theme_name) DO NOTHING;
  "
}

# ============================================================================
# CSS Generation
# ============================================================================

generate_theme_css() {
  local config_file="$1"
  local css_file="$2"

  if [[ ! -f "$config_file" ]]; then
    printf "${RED}Error: Theme config not found: %s${NC}\n" "$config_file" >&2
    return 1
  fi

  # Start CSS file
  cat >"$css_file" <<'EOF'
/**
 * nself Theme CSS
 * Auto-generated from theme configuration
 */

:root {
EOF

  # Generate CSS variables from JSON config
  if command -v jq >/dev/null 2>&1; then
    # Colors
    jq -r '.variables.colors // .colors | to_entries[] | "  --color-\(.key): \(.value);"' "$config_file" >>"$css_file" 2>/dev/null || true
    printf "\n" >>"$css_file"

    # Typography
    jq -r '.variables.typography // .typography | to_entries[] | "  --typography-\(.key): \(.value);"' "$config_file" >>"$css_file" 2>/dev/null || true
    printf "\n" >>"$css_file"

    # Spacing
    jq -r '.variables.spacing // .spacing | to_entries[] | "  --spacing-\(.key): \(.value);"' "$config_file" >>"$css_file" 2>/dev/null || true
    printf "\n" >>"$css_file"

    # Borders
    jq -r '.variables.borders // .borders | to_entries[] | "  --border-\(.key): \(.value);"' "$config_file" >>"$css_file" 2>/dev/null || true
    printf "\n" >>"$css_file"

    # Shadows
    jq -r '.variables.shadows // .shadows | to_entries[] | "  --shadow-\(.key): \(.value);"' "$config_file" >>"$css_file" 2>/dev/null || true
    printf "\n" >>"$css_file"
  fi

  # Close CSS
  cat >>"$css_file" <<'EOF'
}

/* Base styles */
body {
  font-family: var(--typography-fontFamily);
  font-size: var(--typography-fontSize);
  font-weight: var(--typography-fontWeight);
  line-height: var(--typography-lineHeight);
  color: var(--color-text);
  background-color: var(--color-background);
}

/* Utility classes */
.bg-primary { background-color: var(--color-primary); }
.bg-surface { background-color: var(--color-surface); }
.text-primary { color: var(--color-primary); }
.text-secondary { color: var(--color-textSecondary); }
.border { border: var(--border-width) solid var(--color-border); }
.rounded { border-radius: var(--border-radius); }
.shadow-sm { box-shadow: var(--shadow-sm); }
.shadow-md { box-shadow: var(--shadow-md); }
.shadow-lg { box-shadow: var(--shadow-lg); }

/* Spacing utilities */
.p-xs { padding: var(--spacing-xs); }
.p-sm { padding: var(--spacing-sm); }
.p-md { padding: var(--spacing-md); }
.p-lg { padding: var(--spacing-lg); }
.p-xl { padding: var(--spacing-xl); }
.m-xs { margin: var(--spacing-xs); }
.m-sm { margin: var(--spacing-sm); }
.m-md { margin: var(--spacing-md); }
.m-lg { margin: var(--spacing-lg); }
.m-xl { margin: var(--spacing-xl); }
EOF

  printf "${GREEN}✓${NC} CSS generated: %s\n" "$css_file"

  return 0
}

# ============================================================================
# Theme Management
# ============================================================================

create_theme() {
  local theme_name="$1"
  local tenant_id="${2:-default}"

  # Validate theme name
  if [[ ! "$theme_name" =~ ^[a-z0-9-]+$ ]]; then
    printf "${RED}Error: Invalid theme name. Use lowercase letters, numbers, and hyphens only.${NC}\n" >&2
    return 1
  fi

  # Check database connection
  if ! check_database_connection; then
    return 1
  fi

  # Get brand ID
  local brand_id
  brand_id=$(get_brand_id_by_tenant "$tenant_id")

  if [[ -z "$brand_id" ]]; then
    return 1
  fi

  # Check if theme already exists
  local exists
  exists=$(exec_sql "SELECT EXISTS (SELECT 1 FROM whitelabel_themes WHERE brand_id = '$brand_id' AND theme_name = '$theme_name');" "tuples_only")

  if [[ "$exists" == "t" ]]; then
    printf "${YELLOW}Warning: Theme already exists: %s${NC}\n" "$theme_name"
    return 0
  fi

  printf "${CYAN}Creating theme: %s${NC}\n" "$theme_name"

  # Create theme in database
  create_custom_theme_db "$theme_name" "$brand_id"

  # Create local directory
  local theme_dir="${THEMES_DIR}/${theme_name}"
  mkdir -p "$theme_dir"

  # Export to file
  export_theme_to_file "$theme_name" "$brand_id" "${theme_dir}/theme.json"

  # Generate CSS
  if [[ -f "${theme_dir}/theme.json" ]]; then
    generate_theme_css "${theme_dir}/theme.json" "${theme_dir}/theme.css"
  fi

  printf "${GREEN}✓${NC} Theme created: %s\n" "$theme_name"
  printf "\nNext steps:\n"
  printf "  1. Edit theme: nself whitelabel theme edit %s\n" "$theme_name"
  printf "  2. Preview theme: nself whitelabel theme preview %s\n" "$theme_name"
  printf "  3. Activate theme: nself whitelabel theme activate %s\n" "$theme_name"

  return 0
}

create_custom_theme_db() {
  local theme_name="$1"
  local brand_id="$2"

  # Use light theme as template
  local colors_json='{"primary": "#0066cc", "primaryHover": "#0052a3", "secondary": "#6c757d", "accent": "#00cc66", "background": "#ffffff", "backgroundAlt": "#f8f9fa", "surface": "#ffffff", "surfaceAlt": "#f1f3f5", "text": "#212529", "textSecondary": "#6c757d", "textMuted": "#adb5bd", "border": "#dee2e6", "borderLight": "#e9ecef", "success": "#28a745", "warning": "#ffc107", "error": "#dc3545", "info": "#17a2b8"}'
  local typography_json='{"fontFamily": "-apple-system, sans-serif", "fontFamilyMono": "'\''Courier New'\'', monospace", "fontSize": "16px", "fontWeight": "400", "lineHeight": "1.5"}'
  local spacing_json='{"xs": "4px", "sm": "8px", "md": "16px", "lg": "24px", "xl": "32px", "xxl": "48px"}'
  local borders_json='{"radius": "4px", "radiusLg": "8px", "width": "1px"}'
  local shadows_json='{"sm": "0 1px 3px rgba(0,0,0,0.12)", "md": "0 4px 6px rgba(0,0,0,0.1)", "lg": "0 10px 20px rgba(0,0,0,0.15)"}'

  exec_sql "
    INSERT INTO whitelabel_themes (
      brand_id, theme_name, display_name, description, version, author,
      mode, colors, typography, spacing, borders, shadows,
      is_default, is_system, is_active
    ) VALUES (
      '$brand_id',
      '$theme_name',
      '$theme_name',
      'Custom theme',
      '1.0.0',
      'Custom',
      'light',
      '$colors_json'::jsonb,
      '$typography_json'::jsonb,
      '$spacing_json'::jsonb,
      '$borders_json'::jsonb,
      '$shadows_json'::jsonb,
      false,
      false,
      false
    );
  "
}

edit_theme() {
  local theme_name="$1"
  local tenant_id="${2:-default}"

  # Check database connection
  if ! check_database_connection; then
    return 1
  fi

  # Get brand ID
  local brand_id
  brand_id=$(get_brand_id_by_tenant "$tenant_id")

  # Export theme to temp file
  local temp_file
  temp_file=$(mktemp)

  export_theme_to_file "$theme_name" "$brand_id" "$temp_file"

  if [[ ! -f "$temp_file" ]]; then
    printf "${RED}Error: Theme not found: %s${NC}\n" "$theme_name" >&2
    return 1
  fi

  # Open in default editor
  local editor="${EDITOR:-vi}"
  "$editor" "$temp_file"

  # Validate JSON
  if command -v jq >/dev/null 2>&1; then
    if ! jq empty "$temp_file" 2>/dev/null; then
      printf "${RED}Error: Invalid JSON in edited theme${NC}\n" >&2
      rm -f "$temp_file"
      return 1
    fi
  fi

  # Import back to database
  import_theme_from_file "$temp_file" "$brand_id"

  # Update local cache
  local theme_dir="${THEMES_DIR}/${theme_name}"
  mkdir -p "$theme_dir"
  cp "$temp_file" "${theme_dir}/theme.json"

  # Regenerate CSS
  generate_theme_css "${theme_dir}/theme.json" "${theme_dir}/theme.css"

  # Update compiled CSS in database
  if [[ -f "${theme_dir}/theme.css" ]]; then
    local css_content
    css_content=$(cat "${theme_dir}/theme.css" | sed "s/'/''/g")
    exec_sql "UPDATE whitelabel_themes SET compiled_css = '$css_content' WHERE brand_id = '$brand_id' AND theme_name = '$theme_name';"
  fi

  rm -f "$temp_file"

  printf "${GREEN}✓${NC} Theme updated: %s\n" "$theme_name"

  return 0
}

activate_theme() {
  local theme_name="$1"
  local tenant_id="${2:-default}"

  # Check database connection
  if ! check_database_connection; then
    return 1
  fi

  # Get brand ID
  local brand_id
  brand_id=$(get_brand_id_by_tenant "$tenant_id")

  # Check if theme exists
  local exists
  exists=$(exec_sql "SELECT EXISTS (SELECT 1 FROM whitelabel_themes WHERE brand_id = '$brand_id' AND theme_name = '$theme_name');" "tuples_only")

  if [[ "$exists" != "t" ]]; then
    printf "${RED}Error: Theme not found: %s${NC}\n" "$theme_name" >&2
    return 1
  fi

  printf "${CYAN}Activating theme: %s${NC}\n" "$theme_name"

  # Deactivate all themes for this brand
  exec_sql "UPDATE whitelabel_themes SET is_active = false WHERE brand_id = '$brand_id';"

  # Activate the selected theme
  exec_sql "UPDATE whitelabel_themes SET is_active = true WHERE brand_id = '$brand_id' AND theme_name = '$theme_name';"

  # Update brand's active theme
  local theme_id
  theme_id=$(exec_sql "SELECT id FROM whitelabel_themes WHERE brand_id = '$brand_id' AND theme_name = '$theme_name';" "tuples_only")
  exec_sql "UPDATE whitelabel_brands SET active_theme_id = '$theme_id' WHERE id = '$brand_id';"

  # Update local cache
  printf "%s" "$theme_name" >"$ACTIVE_THEME_FILE"

  printf "${GREEN}✓${NC} Theme activated: %s\n" "$theme_name"

  return 0
}

preview_theme() {
  local theme_name="$1"
  local tenant_id="${2:-default}"

  # Check database connection
  if ! check_database_connection; then
    return 1
  fi

  # Get brand ID
  local brand_id
  brand_id=$(get_brand_id_by_tenant "$tenant_id")

  # Query theme from database
  local theme_data
  theme_data=$(exec_sql "SELECT display_name, description, mode, is_active FROM whitelabel_themes WHERE brand_id = '$brand_id' AND theme_name = '$theme_name';" "tuples_only")

  if [[ -z "$theme_data" ]]; then
    printf "${RED}Error: Theme not found: %s${NC}\n" "$theme_name" >&2
    return 1
  fi

  printf "${CYAN}${BOLD}Theme Preview: %s${NC}\n" "$theme_name"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  # Parse theme data
  IFS='|' read -r display_name description mode is_active <<<"$theme_data"

  printf "${BLUE}Name:${NC} %s\n" "$display_name"
  printf "${BLUE}Description:${NC} %s\n" "$description"
  printf "${BLUE}Mode:${NC} %s\n" "$mode"
  printf "${BLUE}Status:${NC} %s\n\n" "$([ "$is_active" = "t" ] && echo "${GREEN}Active${NC}" || echo "Inactive")"

  # Get colors
  printf "${BLUE}${BOLD}Colors:${NC}\n"
  local colors
  colors=$(exec_sql "SELECT jsonb_pretty(colors) FROM whitelabel_themes WHERE brand_id = '$brand_id' AND theme_name = '$theme_name';" "tuples_only")
  if [[ -n "$colors" ]]; then
    echo "$colors" | sed 's/^/  /'
  fi

  printf "\n${BLUE}${BOLD}Typography:${NC}\n"
  local typography
  typography=$(exec_sql "SELECT jsonb_pretty(typography) FROM whitelabel_themes WHERE brand_id = '$brand_id' AND theme_name = '$theme_name';" "tuples_only")
  if [[ -n "$typography" ]]; then
    echo "$typography" | sed 's/^/  /'
  fi

  printf "\n${BLUE}${BOLD}Spacing:${NC}\n"
  local spacing
  spacing=$(exec_sql "SELECT jsonb_pretty(spacing) FROM whitelabel_themes WHERE brand_id = '$brand_id' AND theme_name = '$theme_name';" "tuples_only")
  if [[ -n "$spacing" ]]; then
    echo "$spacing" | sed 's/^/  /'
  fi

  return 0
}

export_theme() {
  local theme_name="$1"
  local output_file="${2:-}"
  local tenant_id="${3:-default}"

  # Check database connection
  if ! check_database_connection; then
    return 1
  fi

  # Get brand ID
  local brand_id
  brand_id=$(get_brand_id_by_tenant "$tenant_id")

  if [[ -n "$output_file" ]]; then
    export_theme_to_file "$theme_name" "$brand_id" "$output_file"
    printf "${GREEN}✓${NC} Theme exported to: %s\n" "$output_file"
  else
    # Output to stdout
    export_theme_to_file "$theme_name" "$brand_id" "/dev/stdout"
  fi

  return 0
}

export_theme_to_file() {
  local theme_name="$1"
  local brand_id="$2"
  local output_file="$3"

  # Build JSON from database
  local theme_json
  theme_json=$(exec_sql "
    SELECT json_build_object(
      'name', theme_name,
      'displayName', display_name,
      'description', description,
      'version', version,
      'author', author,
      'mode', mode,
      'variables', json_build_object(
        'colors', colors,
        'typography', typography,
        'spacing', spacing,
        'borders', borders,
        'shadows', shadows
      ),
      'createdAt', created_at,
      'updatedAt', updated_at
    )::text
    FROM whitelabel_themes
    WHERE brand_id = '$brand_id' AND theme_name = '$theme_name';
  " "tuples_only")

  if [[ -z "$theme_json" ]]; then
    printf "${RED}Error: Theme not found: %s${NC}\n" "$theme_name" >&2
    return 1
  fi

  # Pretty print if jq available
  if command -v jq >/dev/null 2>&1; then
    echo "$theme_json" | jq '.' >"$output_file"
  else
    echo "$theme_json" >"$output_file"
  fi

  return 0
}

import_theme() {
  local theme_file="$1"
  local tenant_id="${2:-default}"

  if [[ ! -f "$theme_file" ]]; then
    printf "${RED}Error: Theme file not found: %s${NC}\n" "$theme_file" >&2
    return 1
  fi

  # Validate JSON
  if command -v jq >/dev/null 2>&1; then
    if ! jq empty "$theme_file" 2>/dev/null; then
      printf "${RED}Error: Invalid JSON in theme file${NC}\n" >&2
      return 1
    fi
  fi

  # Check database connection
  if ! check_database_connection; then
    return 1
  fi

  # Get brand ID
  local brand_id
  brand_id=$(get_brand_id_by_tenant "$tenant_id")

  # Import theme
  import_theme_from_file "$theme_file" "$brand_id"

  return 0
}

import_theme_from_file() {
  local theme_file="$1"
  local brand_id="$2"

  if ! command -v jq >/dev/null 2>&1; then
    printf "${RED}Error: jq required for theme import${NC}\n" >&2
    return 1
  fi

  local theme_name
  theme_name=$(jq -r '.name' "$theme_file")

  if [[ -z "$theme_name" ]] || [[ "$theme_name" == "null" ]]; then
    printf "${RED}Error: Theme name not found in file${NC}\n" >&2
    return 1
  fi

  printf "${CYAN}Importing theme: %s${NC}\n" "$theme_name"

  # Extract fields
  local display_name
  display_name=$(jq -r '.displayName // .name' "$theme_file")
  local description
  description=$(jq -r '.description // "Imported theme"' "$theme_file")
  local version
  version=$(jq -r '.version // "1.0.0"' "$theme_file")
  local author
  author=$(jq -r '.author // "Unknown"' "$theme_file")
  local mode
  mode=$(jq -r '.mode // "light"' "$theme_file")

  # Extract JSON fields
  local colors
  colors=$(jq -c '.variables.colors // .colors' "$theme_file" | sed "s/'/''/g")
  local typography
  typography=$(jq -c '.variables.typography // .typography' "$theme_file" | sed "s/'/''/g")
  local spacing
  spacing=$(jq -c '.variables.spacing // .spacing' "$theme_file" | sed "s/'/''/g")
  local borders
  borders=$(jq -c '.variables.borders // .borders' "$theme_file" | sed "s/'/''/g")
  local shadows
  shadows=$(jq -c '.variables.shadows // .shadows' "$theme_file" | sed "s/'/''/g")

  # Insert or update theme
  exec_sql "
    INSERT INTO whitelabel_themes (
      brand_id, theme_name, display_name, description, version, author,
      mode, colors, typography, spacing, borders, shadows,
      is_default, is_system, is_active
    ) VALUES (
      '$brand_id',
      '$theme_name',
      '$display_name',
      '$description',
      '$version',
      '$author',
      '$mode',
      '$colors'::jsonb,
      '$typography'::jsonb,
      '$spacing'::jsonb,
      '$borders'::jsonb,
      '$shadows'::jsonb,
      false,
      false,
      false
    )
    ON CONFLICT (brand_id, theme_name) DO UPDATE SET
      display_name = EXCLUDED.display_name,
      description = EXCLUDED.description,
      version = EXCLUDED.version,
      author = EXCLUDED.author,
      mode = EXCLUDED.mode,
      colors = EXCLUDED.colors,
      typography = EXCLUDED.typography,
      spacing = EXCLUDED.spacing,
      borders = EXCLUDED.borders,
      shadows = EXCLUDED.shadows,
      updated_at = NOW();
  "

  # Create local directory and cache
  local theme_dir="${THEMES_DIR}/${theme_name}"
  mkdir -p "$theme_dir"

  # Copy theme config
  cp "$theme_file" "${theme_dir}/theme.json"

  # Generate CSS
  generate_theme_css "${theme_dir}/theme.json" "${theme_dir}/theme.css"

  # Update compiled CSS in database
  if [[ -f "${theme_dir}/theme.css" ]]; then
    local css_content
    css_content=$(cat "${theme_dir}/theme.css" | sed "s/'/''/g")
    exec_sql "UPDATE whitelabel_themes SET compiled_css = '$css_content' WHERE brand_id = '$brand_id' AND theme_name = '$theme_name';"
  fi

  printf "${GREEN}✓${NC} Theme imported: %s\n" "$theme_name"

  return 0
}

list_themes() {
  local tenant_id="${1:-default}"

  # Check database connection
  if ! check_database_connection; then
    return 1
  fi

  # Get brand ID
  local brand_id
  brand_id=$(get_brand_id_by_tenant "$tenant_id")

  printf "${CYAN}${BOLD}Available Themes${NC}\n"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  # Query themes from database
  local themes
  themes=$(exec_sql "
    SELECT
      theme_name,
      display_name,
      mode,
      is_active,
      is_system,
      version
    FROM whitelabel_themes
    WHERE brand_id = '$brand_id'
    ORDER BY is_system DESC, is_active DESC, theme_name ASC;
  " "table")

  if [[ -z "$themes" ]]; then
    printf "${YELLOW}No themes found${NC}\n"
    return 0
  fi

  printf "%s\n" "$themes"

  return 0
}

get_active_theme() {
  local tenant_id="${1:-default}"

  # Check database connection
  if ! check_database_connection; then
    # Fallback to local file
    if [[ -f "$ACTIVE_THEME_FILE" ]]; then
      cat "$ACTIVE_THEME_FILE"
    else
      printf "light"
    fi
    return 0
  fi

  # Get brand ID
  local brand_id
  brand_id=$(get_brand_id_by_tenant "$tenant_id" 2>/dev/null)

  if [[ -z "$brand_id" ]]; then
    printf "light"
    return 0
  fi

  # Query active theme
  local active_theme
  active_theme=$(exec_sql "SELECT theme_name FROM whitelabel_themes WHERE brand_id = '$brand_id' AND is_active = true LIMIT 1;" "tuples_only")

  if [[ -z "$active_theme" ]]; then
    printf "light"
  else
    printf "%s" "$active_theme"
  fi

  return 0
}

delete_theme() {
  local theme_name="$1"
  local tenant_id="${2:-default}"

  # Check database connection
  if ! check_database_connection; then
    return 1
  fi

  # Get brand ID
  local brand_id
  brand_id=$(get_brand_id_by_tenant "$tenant_id")

  # Check if theme is system theme
  local is_system
  is_system=$(exec_sql "SELECT is_system FROM whitelabel_themes WHERE brand_id = '$brand_id' AND theme_name = '$theme_name';" "tuples_only")

  if [[ "$is_system" == "t" ]]; then
    printf "${RED}Error: Cannot delete system theme: %s${NC}\n" "$theme_name" >&2
    return 1
  fi

  # Check if theme is active
  local is_active
  is_active=$(exec_sql "SELECT is_active FROM whitelabel_themes WHERE brand_id = '$brand_id' AND theme_name = '$theme_name';" "tuples_only")

  if [[ "$is_active" == "t" ]]; then
    printf "${RED}Error: Cannot delete active theme. Activate another theme first.${NC}\n" >&2
    return 1
  fi

  printf "${CYAN}Deleting theme: %s${NC}\n" "$theme_name"

  # Delete from database
  exec_sql "DELETE FROM whitelabel_themes WHERE brand_id = '$brand_id' AND theme_name = '$theme_name';"

  # Remove local cache
  local theme_dir="${THEMES_DIR}/${theme_name}"
  if [[ -d "$theme_dir" ]]; then
    rm -rf "$theme_dir"
  fi

  printf "${GREEN}✓${NC} Theme deleted: %s\n" "$theme_name"

  return 0
}

# ============================================================================
# Theme Validation
# ============================================================================

validate_theme_config() {
  local theme_file="$1"

  if [[ ! -f "$theme_file" ]]; then
    printf "${RED}Error: Theme file not found${NC}\n" >&2
    return 1
  fi

  if ! command -v jq >/dev/null 2>&1; then
    printf "${YELLOW}Warning: jq not available. Cannot validate theme.${NC}\n" >&2
    return 0
  fi

  printf "${CYAN}Validating theme configuration...${NC}\n"

  # Validate JSON syntax
  if ! jq empty "$theme_file" 2>/dev/null; then
    printf "${RED}✗ Invalid JSON syntax${NC}\n" >&2
    return 1
  fi
  printf "${GREEN}✓${NC} Valid JSON syntax\n"

  # Check required fields
  local required_fields="name displayName mode"
  for field in $required_fields; do
    if ! jq -e ".$field" "$theme_file" >/dev/null 2>&1; then
      printf "${RED}✗ Missing required field: %s${NC}\n" "$field" >&2
      return 1
    fi
  done
  printf "${GREEN}✓${NC} All required fields present\n"

  # Validate mode
  local mode
  mode=$(jq -r '.mode' "$theme_file")
  if [[ "$mode" != "light" ]] && [[ "$mode" != "dark" ]] && [[ "$mode" != "auto" ]]; then
    printf "${RED}✗ Invalid mode: %s (must be light, dark, or auto)${NC}\n" "$mode" >&2
    return 1
  fi
  printf "${GREEN}✓${NC} Valid mode: %s\n" "$mode"

  # Check for color variables
  if ! jq -e '.variables.colors // .colors' "$theme_file" >/dev/null 2>&1; then
    printf "${YELLOW}⚠ Warning: No color variables defined${NC}\n"
  else
    printf "${GREEN}✓${NC} Color variables defined\n"
  fi

  printf "${GREEN}✓${NC} Theme configuration valid\n"

  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f initialize_themes_system
export -f create_theme
export -f edit_theme
export -f activate_theme
export -f preview_theme
export -f export_theme
export -f import_theme
export -f list_themes
export -f get_active_theme
export -f delete_theme
export -f validate_theme_config
export -f generate_theme_css
export -f check_database_connection
