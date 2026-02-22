#!/usr/bin/env bash
# nself Branding System
# Manages white-label branding including logos, colors, fonts, and custom CSS
# Part of Sprint 14: White-Label & Customization (60pts) for v0.9.0


# Get script directory and source dependencies (namespaced to avoid clobbering caller globals)
BRANDING_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

_BRANDING_LIB_ROOT="$(dirname "$BRANDING_LIB_DIR")"

# Source dependencies
source "$_BRANDING_LIB_ROOT/utils/platform-compat.sh" 2>/dev/null || true
source "$_BRANDING_LIB_ROOT/utils/validation.sh" 2>/dev/null || true

# Color definitions for output (guard against double-declaration when sourced together)
[[ -z "${RED:-}" ]] && readonly RED='\033[0;31m'
[[ -z "${GREEN:-}" ]] && readonly GREEN='\033[0;32m'
[[ -z "${YELLOW:-}" ]] && readonly YELLOW='\033[1;33m'
[[ -z "${BLUE:-}" ]] && readonly BLUE='\033[0;34m'
[[ -z "${CYAN:-}" ]] && readonly CYAN='\033[0;36m'
[[ -z "${NC:-}" ]] && readonly NC='\033[0m'

# Branding configuration paths
readonly BRANDING_DIR="${PROJECT_ROOT:-$PWD}/branding"
readonly LOGOS_DIR="${BRANDING_DIR}/logos"
readonly CSS_DIR="${BRANDING_DIR}/css"
readonly FONTS_DIR="${BRANDING_DIR}/fonts"
readonly ASSETS_DIR="${BRANDING_DIR}/assets"
readonly VERSIONS_DIR="${BRANDING_DIR}/versions"

# Default brand configuration
readonly DEFAULT_PRIMARY_COLOR="#0066cc"
readonly DEFAULT_SECONDARY_COLOR="#ff6600"
readonly DEFAULT_PRIMARY_FONT="Inter, system-ui, sans-serif"
readonly DEFAULT_SECONDARY_FONT="Georgia, serif"

# File validation constraints
readonly MAX_LOGO_SIZE_MB=5
readonly MAX_CSS_SIZE_MB=2
readonly MAX_FONT_SIZE_MB=1
readonly SUPPORTED_LOGO_FORMATS="png jpg jpeg svg webp"
readonly SUPPORTED_FONT_FORMATS="woff woff2 ttf otf"

# Security settings
readonly SECURE_FILE_PERMS="0644"
readonly SECURE_DIR_PERMS="0755"

# ============================================================================
# INPUT VALIDATION - Injection Prevention
# ============================================================================

# Validate brand name (prevent injection attacks)
validate_brand_name() {
  local brand_name="$1"

  if [[ -z "$brand_name" ]]; then
    printf "${RED}Error: Brand name cannot be empty${NC}\n" >&2
    return 1
  fi

  # Max length 255 characters
  if [[ ${#brand_name} -gt 255 ]]; then
    printf "${RED}Error: Brand name too long (${#brand_name}). Maximum 255 characters${NC}\n" >&2
    return 1
  fi

  # Allow alphanumeric, space, hyphen, underscore
  if ! [[ "$brand_name" =~ ^[a-zA-Z0-9[:space:]_-]+$ ]]; then
    printf "${RED}Error: Brand name contains invalid characters. Only alphanumeric, space, hyphen, and underscore allowed${NC}\n" >&2
    return 1
  fi

  return 0
}

# Validate tenant ID format
validate_tenant_id() {
  local tenant_id="$1"

  if [[ -z "$tenant_id" ]]; then
    printf "${RED}Error: Tenant ID cannot be empty${NC}\n" >&2
    return 1
  fi

  # Max length 64 characters
  if [[ ${#tenant_id} -gt 64 ]]; then
    printf "${RED}Error: Tenant ID too long. Maximum 64 characters${NC}\n" >&2
    return 1
  fi

  # Only alphanumeric, hyphen, underscore
  if ! [[ "$tenant_id" =~ ^[a-zA-Z0-9_-]+$ ]]; then
    printf "${RED}Error: Tenant ID contains invalid characters. Only alphanumeric, hyphen, and underscore allowed${NC}\n" >&2
    return 1
  fi

  return 0
}

# Validate logo type
validate_logo_type() {
  local logo_type="$1"

  case "$logo_type" in
    main | icon | email | favicon)
      return 0
      ;;
    *)
      printf "${RED}Error: Invalid logo type: $logo_type. Must be main, icon, email, or favicon${NC}\n" >&2
      return 1
      ;;
  esac
}

# Validate file size (returns 1 if exceeds max)
validate_file_size() {
  local file_path="$1"
  local max_size_mb="$2"

  if [[ ! -f "$file_path" ]]; then
    printf "${RED}Error: File does not exist: $file_path${NC}\n" >&2
    return 1
  fi

  local file_size_mb
  file_size_mb=$(branding::get_file_size_mb "$file_path")

  if (($(printf "%.0f" "$file_size_mb") > max_size_mb)); then
    printf "${RED}Error: File too large (%.2f MB). Maximum: %d MB${NC}\n" "$file_size_mb" "$max_size_mb" >&2
    return 1
  fi

  return 0
}

# Validate file extension against whitelist
validate_file_extension() {
  local file_path="$1"
  local allowed_extensions="$2"

  local extension="${file_path##*.}"
  extension=$(printf "%s" "$extension" | tr '[:upper:]' '[:lower:]')

  for allowed in $allowed_extensions; do
    if [[ "$extension" == "$allowed" ]]; then
      return 0
    fi
  done

  printf "${RED}Error: Unsupported file extension '.%s'. Allowed: %s${NC}\n" "$extension" "$allowed_extensions" >&2
  return 1
}

# Validate file by checking magic bytes (binary file type checking)
validate_file_magic_bytes() {
  local file_path="$1"
  local expected_type="$2"

  if ! command -v file >/dev/null 2>&1; then
    return 0 # Skip validation if 'file' command not available
  fi

  local mime_type
  mime_type=$(file -b --mime-type "$file_path")

  case "$expected_type" in
    image/png)
      [[ "$mime_type" == "image/png" ]] && return 0
      ;;
    image/jpeg)
      [[ "$mime_type" == "image/jpeg" ]] && return 0
      ;;
    image/svg+xml | text/xml | text/plain)
      [[ "$mime_type" =~ (image/svg\+xml|text/xml|text/plain) ]] && return 0
      ;;
    image/webp)
      [[ "$mime_type" == "image/webp" ]] && return 0
      ;;
    *)
      return 0 # Unknown type, skip validation
      ;;
  esac

  printf "${RED}Error: File does not match expected type. Expected: %s, Got: %s${NC}\n" "$expected_type" "$mime_type" >&2
  return 1
}

# Validate string length constraints
validate_string_length() {
  local value="$1"
  local min_length="${2:-0}"
  local max_length="${3:-1000}"
  local field_name="${4:-Value}"

  local actual_length=${#value}

  if [[ $actual_length -lt $min_length ]]; then
    printf "${RED}Error: %s too short (%d chars). Minimum: %d${NC}\n" "$field_name" "$actual_length" "$min_length" >&2
    return 1
  fi

  if [[ $actual_length -gt $max_length ]]; then
    printf "${RED}Error: %s too long (%d chars). Maximum: %d${NC}\n" "$field_name" "$actual_length" "$max_length" >&2
    return 1
  fi

  return 0
}

# Validate CSS file for security issues before upload
validate_css_security() {
  local css_path="$1"

  if [[ ! -f "$css_path" ]]; then
    printf "${RED}Error: CSS file not found: $css_path${NC}\n" >&2
    return 1
  fi

  local issues=0

  # Check for JavaScript in CSS (XSS vector)
  if grep -qi 'javascript:' "$css_path" 2>/dev/null; then
    printf "${RED}Error: CSS contains JavaScript references (XSS vector)${NC}\n" >&2
    issues=$((issues + 1))
  fi

  # Check for expression() (IE XSS vector)
  if grep -qi 'expression(' "$css_path" 2>/dev/null; then
    printf "${RED}Error: CSS contains expression() - potential XSS vulnerability${NC}\n" >&2
    issues=$((issues + 1))
  fi

  # Check for behavior: (IE XSS vector)
  if grep -qi 'behavior:' "$css_path" 2>/dev/null; then
    printf "${RED}Error: CSS contains behavior: - potential XSS vulnerability${NC}\n" >&2
    issues=$((issues + 1))
  fi

  # Warn about external URLs (potential data exfiltration)
  if grep -q 'url([^)]*https*://' "$css_path" 2>/dev/null; then
    printf "${YELLOW}Warning: CSS contains external URLs - verify they are trusted${NC}\n"
  fi

  if [[ $issues -gt 0 ]]; then
    return 1
  fi

  return 0
}

# Escape HTML special characters in template variables
escape_html() {
  local text="$1"

  # Replace HTML special characters
  text="${text//&/&amp;}"
  text="${text//</&lt;}"
  text="${text//>/&gt;}"
  text="${text//\"/&quot;}"
  text="${text//\'/&#39;}"

  printf "%s" "$text"
}

# Escape JSON string values
escape_json_string() {
  local text="$1"

  # Escape backslashes first
  text="${text//\\/\\\\}"
  # Escape quotes
  text="${text//\"/\\\"}"
  # Escape newlines
  text="${text//$'\n'/\\n}"
  # Escape carriage returns
  text="${text//$'\r'/\\r}"
  # Escape tabs
  text="${text//$'\t'/\\t}"

  printf "%s" "$text"
}

# ============================================================================
# Branding Initialization
# ============================================================================

initialize_branding_system() {
  local tenant_id="${1:-default}"

  printf "${CYAN}Initializing branding system...${NC}\n"

  # Create branding directories with secure permissions
  branding::create_secure_directory "$BRANDING_DIR"
  branding::create_secure_directory "$LOGOS_DIR"
  branding::create_secure_directory "$CSS_DIR"
  branding::create_secure_directory "$FONTS_DIR"
  branding::create_secure_directory "$ASSETS_DIR"
  branding::create_secure_directory "$VERSIONS_DIR"

  # Create tenant-specific directory if needed
  if [[ "$tenant_id" != "default" ]]; then
    branding::create_secure_directory "${BRANDING_DIR}/${tenant_id}"
  fi

  # Create default branding config with versioning
  local config_file="${BRANDING_DIR}/config.json"
  if [[ ! -f "$config_file" ]]; then
    branding::create_default_config "$config_file" "$tenant_id"
    printf "${GREEN}✓${NC} Created default branding configuration\n"
  fi

  # Create .gitignore for sensitive assets
  cat >"${BRANDING_DIR}/.gitignore" <<'EOF'
# Uploaded logos and assets
logos/*
assets/*
fonts/*

# Keep config but not versions
versions/*

# Allow example files
!logos/.gitkeep
!assets/.gitkeep
!fonts/.gitkeep
EOF

  # Create .gitkeep files
  touch "${LOGOS_DIR}/.gitkeep"
  touch "${ASSETS_DIR}/.gitkeep"
  touch "${FONTS_DIR}/.gitkeep"

  printf "${GREEN}✓${NC} Branding system initialized\n"
  return 0
}

# Create secure directory with proper permissions
branding::create_secure_directory() {
  local dir="$1"

  if [[ ! -d "$dir" ]]; then
    mkdir -p "$dir"
    chmod "$SECURE_DIR_PERMS" "$dir"
  fi
}

# Create default configuration file
branding::create_default_config() {
  local config_file="$1"
  local tenant_id="${2:-default}"
  local timestamp
  timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

  cat >"$config_file" <<EOF
{
  "version": "1.0.0",
  "tenantId": "${tenant_id}",
  "brand": {
    "name": "nself",
    "tagline": "Powerful Backend for Modern Applications",
    "description": "Open-source backend infrastructure platform"
  },
  "colors": {
    "primary": "${DEFAULT_PRIMARY_COLOR}",
    "secondary": "${DEFAULT_SECONDARY_COLOR}",
    "accent": "#00cc66",
    "background": "#ffffff",
    "text": "#333333",
    "textLight": "#666666",
    "border": "#e0e0e0",
    "success": "#00cc66",
    "warning": "#ff9900",
    "error": "#cc0000",
    "info": "#0066cc"
  },
  "typography": {
    "fonts": {
      "primary": "${DEFAULT_PRIMARY_FONT}",
      "secondary": "${DEFAULT_SECONDARY_FONT}",
      "code": "Fira Code, Consolas, monospace"
    },
    "sizes": {
      "base": "16px",
      "small": "14px",
      "large": "18px",
      "h1": "32px",
      "h2": "24px",
      "h3": "20px"
    },
    "weights": {
      "normal": "400",
      "medium": "500",
      "semibold": "600",
      "bold": "700"
    },
    "lineHeights": {
      "tight": "1.25",
      "normal": "1.5",
      "relaxed": "1.75"
    }
  },
  "logos": {
    "main": null,
    "icon": null,
    "email": null,
    "favicon": null
  },
  "customCSS": null,
  "customFonts": [],
  "theme": "light",
  "createdAt": "${timestamp}",
  "updatedAt": "${timestamp}"
}
EOF

  chmod "$SECURE_FILE_PERMS" "$config_file"
}

# ============================================================================
# Brand Management
# ============================================================================

create_brand() {
  local brand_name="$1"
  local tenant_id="${2:-default}"
  local tagline="${3:-}"
  local description="${4:-}"

  # Validate all inputs
  validate_brand_name "$brand_name" || return 1
  validate_tenant_id "$tenant_id" || return 1

  if [[ -n "$tagline" ]]; then
    validate_string_length "$tagline" 0 500 "Tagline" || return 1
  fi

  if [[ -n "$description" ]]; then
    validate_string_length "$description" 0 2000 "Description" || return 1
  fi

  printf "${CYAN}Creating brand: %s${NC}\n" "$brand_name"

  # Initialize branding system if not already done
  if [[ ! -d "$BRANDING_DIR" ]]; then
    initialize_branding_system "$tenant_id"
  fi

  # Create version backup before modification
  branding::create_version_backup

  # Update brand name in config
  local config_file="${BRANDING_DIR}/config.json"
  if command -v jq >/dev/null 2>&1; then
    local temp_file
    temp_file=$(mktemp)
    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    # Build jq filter dynamically
    local jq_filter=".brand.name = \$name | .tenantId = \$tenant | .updatedAt = \$timestamp"
    [[ -n "$tagline" ]] && jq_filter="$jq_filter | .brand.tagline = \$tagline"
    [[ -n "$description" ]] && jq_filter="$jq_filter | .brand.description = \$description"

    if [[ -n "$tagline" ]] && [[ -n "$description" ]]; then
      jq --arg name "$brand_name" --arg tenant "$tenant_id" \
        --arg tagline "$tagline" --arg description "$description" \
        --arg timestamp "$timestamp" \
        "$jq_filter" "$config_file" >"$temp_file" && mv "$temp_file" "$config_file"
    elif [[ -n "$tagline" ]]; then
      jq --arg name "$brand_name" --arg tenant "$tenant_id" \
        --arg tagline "$tagline" --arg timestamp "$timestamp" \
        "$jq_filter" "$config_file" >"$temp_file" && mv "$temp_file" "$config_file"
    else
      jq --arg name "$brand_name" --arg tenant "$tenant_id" \
        --arg timestamp "$timestamp" \
        "$jq_filter" "$config_file" >"$temp_file" && mv "$temp_file" "$config_file"
    fi

    chmod "$SECURE_FILE_PERMS" "$config_file"
  else
    printf "${YELLOW}Warning: jq not installed. Cannot update configuration.${NC}\n"
    return 1
  fi

  printf "${GREEN}✓${NC} Brand '%s' created successfully\n" "$brand_name"
  printf "\n${BLUE}Next steps:${NC}\n"
  printf "  1. Set brand colors: ${CYAN}nself whitelabel branding set-colors --primary #hexcode${NC}\n"
  printf "  2. Upload logo: ${CYAN}nself whitelabel logo upload <path> --type main${NC}\n"
  printf "  3. Customize fonts: ${CYAN}nself whitelabel branding set-fonts --primary \"Font Name\"${NC}\n"
  printf "  4. Add custom CSS: ${CYAN}nself whitelabel branding set-css <path>${NC}\n"

  return 0
}

# Update existing brand
update_brand() {
  local config_file="${BRANDING_DIR}/config.json"

  if [[ ! -f "$config_file" ]]; then
    printf "${RED}Error: No brand configuration found. Create a brand first.${NC}\n" >&2
    return 1
  fi

  # Parse arguments
  local name=""
  local tagline=""
  local description=""

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --name)
        name="$2"
        shift 2
        ;;
      --tagline)
        tagline="$2"
        shift 2
        ;;
      --description)
        description="$2"
        shift 2
        ;;
      *)
        printf "${RED}Error: Unknown option '%s'${NC}\n" "$1" >&2
        return 1
        ;;
    esac
  done

  if [[ -z "$name" ]] && [[ -z "$tagline" ]] && [[ -z "$description" ]]; then
    printf "${RED}Error: At least one field must be specified${NC}\n" >&2
    return 1
  fi

  printf "${CYAN}Updating brand configuration...${NC}\n"

  # Create version backup
  branding::create_version_backup

  # Update config
  if command -v jq >/dev/null 2>&1; then
    local temp_file
    temp_file=$(mktemp)
    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    local jq_filter=".updatedAt = \$timestamp"

    [[ -n "$name" ]] && jq_filter="$jq_filter | .brand.name = \$name"
    [[ -n "$tagline" ]] && jq_filter="$jq_filter | .brand.tagline = \$tagline"
    [[ -n "$description" ]] && jq_filter="$jq_filter | .brand.description = \$description"

    jq --arg name "$name" --arg tagline "$tagline" \
      --arg description "$description" --arg timestamp "$timestamp" \
      "$jq_filter" "$config_file" >"$temp_file" && mv "$temp_file" "$config_file"

    chmod "$SECURE_FILE_PERMS" "$config_file"
  fi

  printf "${GREEN}✓${NC} Brand updated successfully\n"
  return 0
}

# Delete brand (with confirmation)
delete_brand() {
  local tenant_id="${1:-default}"

  printf "${YELLOW}Warning: This will delete all branding configuration and assets.${NC}\n"
  printf "Are you sure? (yes/no): "
  read -r confirmation

  confirmation=$(printf "%s" "$confirmation" | tr '[:upper:]' '[:lower:]')

  if [[ "$confirmation" != "yes" ]]; then
    printf "${CYAN}Deletion cancelled.${NC}\n"
    return 0
  fi

  printf "${CYAN}Deleting brand...${NC}\n"

  # Create final backup
  branding::create_version_backup

  # Remove branding directory
  if [[ -d "$BRANDING_DIR" ]]; then
    rm -rf "$BRANDING_DIR"
    printf "${GREEN}✓${NC} Brand deleted successfully\n"
  else
    printf "${YELLOW}No branding found to delete${NC}\n"
  fi

  return 0
}

# ============================================================================
# Color Management
# ============================================================================

set_brand_colors() {
  local primary=""
  local secondary=""
  local accent=""
  local background=""
  local text=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --primary)
        primary="$2"
        shift 2
        ;;
      --secondary)
        secondary="$2"
        shift 2
        ;;
      --accent)
        accent="$2"
        shift 2
        ;;
      --background)
        background="$2"
        shift 2
        ;;
      --text)
        text="$2"
        shift 2
        ;;
      *)
        printf "${RED}Error: Unknown option '%s'${NC}\n" "$1" >&2
        return 1
        ;;
    esac
  done

  printf "${CYAN}Updating brand colors...${NC}\n"

  local config_file="${BRANDING_DIR}/config.json"
  if [[ ! -f "$config_file" ]]; then
    printf "${RED}Error: Branding not initialized. Run 'nself whitelabel init' first.${NC}\n" >&2
    return 1
  fi

  # Validate hex colors
  if [[ -n "$primary" ]] && ! validate_hex_color "$primary"; then
    printf "${RED}Error: Invalid primary color format. Use #RRGGBB${NC}\n" >&2
    return 1
  fi

  if [[ -n "$secondary" ]] && ! validate_hex_color "$secondary"; then
    printf "${RED}Error: Invalid secondary color format. Use #RRGGBB${NC}\n" >&2
    return 1
  fi

  # Update colors in config
  if command -v jq >/dev/null 2>&1; then
    local temp_file
    temp_file=$(mktemp)
    local jq_filter='.updatedAt = now | todate'

    [[ -n "$primary" ]] && jq_filter="$jq_filter | .colors.primary = \"$primary\""
    [[ -n "$secondary" ]] && jq_filter="$jq_filter | .colors.secondary = \"$secondary\""
    [[ -n "$accent" ]] && jq_filter="$jq_filter | .colors.accent = \"$accent\""
    [[ -n "$background" ]] && jq_filter="$jq_filter | .colors.background = \"$background\""
    [[ -n "$text" ]] && jq_filter="$jq_filter | .colors.text = \"$text\""

    jq "$jq_filter" "$config_file" >"$temp_file" && mv "$temp_file" "$config_file"
  fi

  # Generate CSS variables file
  generate_css_variables

  printf "${GREEN}✓${NC} Brand colors updated successfully\n"
  [[ -n "$primary" ]] && printf "  Primary: %s\n" "$primary"
  [[ -n "$secondary" ]] && printf "  Secondary: %s\n" "$secondary"
  [[ -n "$accent" ]] && printf "  Accent: %s\n" "$accent"

  return 0
}

validate_hex_color() {
  local color="$1"
  # Match #RGB or #RRGGBB format
  if [[ "$color" =~ ^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$ ]]; then
    return 0
  fi
  return 1
}

# Validate color palette for accessibility
validate_color_palette() {
  local primary="$1"
  local secondary="$2"
  local background="${3:-#ffffff}"

  # Basic validation - all colors must be valid hex
  if ! validate_hex_color "$primary"; then
    printf "${RED}Error: Invalid primary color${NC}\n" >&2
    return 1
  fi

  if ! validate_hex_color "$secondary"; then
    printf "${RED}Error: Invalid secondary color${NC}\n" >&2
    return 1
  fi

  if ! validate_hex_color "$background"; then
    printf "${RED}Error: Invalid background color${NC}\n" >&2
    return 1
  fi

  # Warning if primary and secondary are too similar
  # (Simple check - compare first 2 chars of hex)
  local p_start="${primary:1:2}"
  local s_start="${secondary:1:2}"

  if [[ "$p_start" == "$s_start" ]]; then
    printf "${YELLOW}Warning: Primary and secondary colors may be too similar${NC}\n"
  fi

  return 0
}

# ============================================================================
# Font Management
# ============================================================================

set_brand_fonts() {
  local primary=""
  local secondary=""
  local code=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --primary)
        primary="$2"
        shift 2
        ;;
      --secondary)
        secondary="$2"
        shift 2
        ;;
      --code)
        code="$2"
        shift 2
        ;;
      *)
        printf "${RED}Error: Unknown option '%s'${NC}\n" "$1" >&2
        return 1
        ;;
    esac
  done

  if [[ -z "$primary" ]] && [[ -z "$secondary" ]] && [[ -z "$code" ]]; then
    printf "${RED}Error: At least one font must be specified${NC}\n" >&2
    return 1
  fi

  printf "${CYAN}Updating brand fonts...${NC}\n"

  local config_file="${BRANDING_DIR}/config.json"
  if [[ ! -f "$config_file" ]]; then
    printf "${RED}Error: Branding not initialized. Run 'nself whitelabel init' first.${NC}\n" >&2
    return 1
  fi

  # Create version backup
  branding::create_version_backup

  # Update fonts in config
  if command -v jq >/dev/null 2>&1; then
    local temp_file
    temp_file=$(mktemp)
    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    local jq_filter=".updatedAt = \$timestamp"

    [[ -n "$primary" ]] && jq_filter="$jq_filter | .typography.fonts.primary = \$primary"
    [[ -n "$secondary" ]] && jq_filter="$jq_filter | .typography.fonts.secondary = \$secondary"
    [[ -n "$code" ]] && jq_filter="$jq_filter | .typography.fonts.code = \$code"

    jq --arg primary "$primary" --arg secondary "$secondary" \
      --arg code "$code" --arg timestamp "$timestamp" \
      "$jq_filter" "$config_file" >"$temp_file" && mv "$temp_file" "$config_file"

    chmod "$SECURE_FILE_PERMS" "$config_file"
  fi

  # Generate CSS variables file
  generate_css_variables

  printf "${GREEN}✓${NC} Brand fonts updated successfully\n"
  [[ -n "$primary" ]] && printf "  Primary: %s\n" "$primary"
  [[ -n "$secondary" ]] && printf "  Secondary: %s\n" "$secondary"
  [[ -n "$code" ]] && printf "  Code: %s\n" "$code"

  return 0
}

# Set typography settings (sizes, weights, line heights)
set_typography() {
  local config_file="${BRANDING_DIR}/config.json"

  if [[ ! -f "$config_file" ]]; then
    printf "${RED}Error: Branding not initialized${NC}\n" >&2
    return 1
  fi

  # Parse arguments
  local base_size=""
  local h1_size=""
  local h2_size=""
  local normal_weight=""
  local bold_weight=""
  local line_height=""

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --base-size)
        base_size="$2"
        shift 2
        ;;
      --h1-size)
        h1_size="$2"
        shift 2
        ;;
      --h2-size)
        h2_size="$2"
        shift 2
        ;;
      --normal-weight)
        normal_weight="$2"
        shift 2
        ;;
      --bold-weight)
        bold_weight="$2"
        shift 2
        ;;
      --line-height)
        line_height="$2"
        shift 2
        ;;
      *)
        printf "${RED}Error: Unknown option '%s'${NC}\n" "$1" >&2
        return 1
        ;;
    esac
  done

  printf "${CYAN}Updating typography settings...${NC}\n"

  # Create version backup
  branding::create_version_backup

  # Update typography in config
  if command -v jq >/dev/null 2>&1; then
    local temp_file
    temp_file=$(mktemp)
    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    local jq_filter=".updatedAt = \$timestamp"

    [[ -n "$base_size" ]] && jq_filter="$jq_filter | .typography.sizes.base = \$baseSize"
    [[ -n "$h1_size" ]] && jq_filter="$jq_filter | .typography.sizes.h1 = \$h1Size"
    [[ -n "$h2_size" ]] && jq_filter="$jq_filter | .typography.sizes.h2 = \$h2Size"
    [[ -n "$normal_weight" ]] && jq_filter="$jq_filter | .typography.weights.normal = \$normalWeight"
    [[ -n "$bold_weight" ]] && jq_filter="$jq_filter | .typography.weights.bold = \$boldWeight"
    [[ -n "$line_height" ]] && jq_filter="$jq_filter | .typography.lineHeights.normal = \$lineHeight"

    jq --arg baseSize "$base_size" --arg h1Size "$h1_size" --arg h2Size "$h2_size" \
      --arg normalWeight "$normal_weight" --arg boldWeight "$bold_weight" \
      --arg lineHeight "$line_height" --arg timestamp "$timestamp" \
      "$jq_filter" "$config_file" >"$temp_file" && mv "$temp_file" "$config_file"

    chmod "$SECURE_FILE_PERMS" "$config_file"
  fi

  # Regenerate CSS variables
  generate_css_variables

  printf "${GREEN}✓${NC} Typography settings updated\n"
  return 0
}

# Upload custom font file
upload_font() {
  local font_path="$1"
  local font_name="${2:-custom}"

  if [[ ! -f "$font_path" ]]; then
    printf "${RED}Error: Font file not found: %s${NC}\n" "$font_path" >&2
    return 1
  fi

  printf "${CYAN}Uploading custom font...${NC}\n"

  # Validate font name
  validate_string_length "$font_name" 1 64 "Font name" || return 1

  # Validate font file extension
  validate_file_extension "$font_path" "$SUPPORTED_FONT_FORMATS" || return 1

  # Validate font file size
  validate_file_size "$font_path" "$MAX_FONT_SIZE_MB" || return 1

  # Validate font file
  if ! branding::validate_font_file "$font_path"; then
    return 1
  fi

  # Get file extension
  local extension="${font_path##*.}"
  extension=$(printf "%s" "$extension" | tr '[:upper:]' '[:lower:]')

  # Generate versioned filename
  local timestamp
  timestamp=$(date +"%Y%m%d_%H%M%S")
  local font_filename="${font_name}-${timestamp}.${extension}"
  local dest_path="${FONTS_DIR}/${font_filename}"

  # Create version backup
  branding::create_version_backup

  # Copy font with secure permissions
  cp "$font_path" "$dest_path"
  chmod "$SECURE_FILE_PERMS" "$dest_path"

  # Update config to track custom font
  local config_file="${BRANDING_DIR}/config.json"
  if command -v jq >/dev/null 2>&1; then
    local temp_file
    temp_file=$(mktemp)
    local update_timestamp
    update_timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    jq --arg name "$font_name" --arg path "$font_filename" --arg timestamp "$update_timestamp" \
      '.customFonts += [{"name": $name, "path": $path}] | .updatedAt = $timestamp' \
      "$config_file" >"$temp_file" && mv "$temp_file" "$config_file"

    chmod "$SECURE_FILE_PERMS" "$config_file"
  fi

  local file_size
  file_size=$(branding::get_file_size_mb "$dest_path")

  printf "${GREEN}✓${NC} Font uploaded successfully\n"
  printf "  Name: %s\n" "$font_name"
  printf "  Path: %s\n" "$dest_path"
  printf "  Size: %.2f MB\n" "$file_size"

  return 0
}

# Validate font file
branding::validate_font_file() {
  local file_path="$1"

  # Check file exists
  if [[ ! -f "$file_path" ]]; then
    printf "${RED}Error: File does not exist${NC}\n" >&2
    return 1
  fi

  # Validate file extension
  local extension="${file_path##*.}"
  extension=$(printf "%s" "$extension" | tr '[:upper:]' '[:lower:]')

  local is_valid=0
  for fmt in $SUPPORTED_FONT_FORMATS; do
    if [[ "$extension" == "$fmt" ]]; then
      is_valid=1
      break
    fi
  done

  if [[ $is_valid -eq 0 ]]; then
    printf "${RED}Error: Unsupported font format '${extension}'. Supported: %s${NC}\n" "$SUPPORTED_FONT_FORMATS" >&2
    return 1
  fi

  # Check file size
  local file_size_mb
  file_size_mb=$(branding::get_file_size_mb "$file_path")

  if (($(printf "%.0f" "$file_size_mb") > MAX_FONT_SIZE_MB)); then
    printf "${RED}Error: Font file too large (%.2f MB). Maximum: %d MB${NC}\n" "$file_size_mb" "$MAX_FONT_SIZE_MB" >&2
    return 1
  fi

  return 0
}

# ============================================================================
# Logo Management
# ============================================================================

upload_brand_logo() {
  local logo_path="$1"
  local logo_type="${2:-main}"

  # Validate logo path
  if [[ ! -f "$logo_path" ]]; then
    printf "${RED}Error: Logo file not found: %s${NC}\n" "$logo_path" >&2
    return 1
  fi

  # Get real path
  logo_path=$(branding::get_absolute_path "$logo_path")

  # Validate logo type
  validate_logo_type "$logo_type" || return 1

  printf "${CYAN}Uploading %s logo...${NC}\n" "$logo_type"

  # Validate file extension (whitelist check)
  validate_file_extension "$logo_path" "$SUPPORTED_LOGO_FORMATS" || return 1

  # Validate file size
  validate_file_size "$logo_path" "$MAX_LOGO_SIZE_MB" || return 1

  # Validate file by magic bytes
  local extension="${logo_path##*.}"
  extension=$(printf "%s" "$extension" | tr '[:upper:]' '[:lower:]')
  case "$extension" in
    png)
      validate_file_magic_bytes "$logo_path" "image/png" || return 1
      ;;
    jpg | jpeg)
      validate_file_magic_bytes "$logo_path" "image/jpeg" || return 1
      ;;
    svg)
      validate_file_magic_bytes "$logo_path" "image/svg+xml" || return 1
      ;;
    webp)
      validate_file_magic_bytes "$logo_path" "image/webp" || return 1
      ;;
  esac

  # Validate file
  if ! branding::validate_logo_file "$logo_path"; then
    return 1
  fi

  # Extract file extension
  local extension="${logo_path##*.}"
  extension=$(printf "%s" "$extension" | tr '[:upper:]' '[:lower:]')

  # Generate unique filename with timestamp for versioning
  local timestamp
  timestamp=$(date +"%Y%m%d_%H%M%S")
  local logo_filename="logo-${logo_type}-${timestamp}.${extension}"
  local dest_path="${LOGOS_DIR}/${logo_filename}"

  # Create version backup
  branding::create_version_backup

  # Copy logo with secure permissions
  cp "$logo_path" "$dest_path"
  chmod "$SECURE_FILE_PERMS" "$dest_path"

  # Create symlink for easy access (current version)
  local symlink_path="${LOGOS_DIR}/logo-${logo_type}.${extension}"
  ln -sf "$logo_filename" "$symlink_path" 2>/dev/null || true

  # Update config
  local config_file="${BRANDING_DIR}/config.json"
  if command -v jq >/dev/null 2>&1; then
    local temp_file
    temp_file=$(mktemp)
    local update_timestamp
    update_timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    jq --arg type "$logo_type" --arg path "$logo_filename" --arg timestamp "$update_timestamp" \
      '.logos[$type] = $path | .updatedAt = $timestamp' \
      "$config_file" >"$temp_file" && mv "$temp_file" "$config_file"

    chmod "$SECURE_FILE_PERMS" "$config_file"
  fi

  # Get file size for feedback
  local file_size
  file_size=$(branding::get_file_size_mb "$dest_path")

  printf "${GREEN}✓${NC} Logo uploaded successfully\n"
  printf "  Type: %s\n" "$logo_type"
  printf "  Path: %s\n" "$dest_path"
  printf "  Size: %.2f MB\n" "$file_size"
  printf "  Format: %s\n" "$extension"

  return 0
}

# Validate logo file
branding::validate_logo_file() {
  local file_path="$1"

  # Check file exists
  if [[ ! -f "$file_path" ]]; then
    printf "${RED}Error: File does not exist${NC}\n" >&2
    return 1
  fi

  # Validate file extension
  local extension="${file_path##*.}"
  extension=$(printf "%s" "$extension" | tr '[:upper:]' '[:lower:]')

  local is_valid=0
  for fmt in $SUPPORTED_LOGO_FORMATS; do
    if [[ "$extension" == "$fmt" ]]; then
      is_valid=1
      break
    fi
  done

  if [[ $is_valid -eq 0 ]]; then
    printf "${RED}Error: Unsupported logo format '${extension}'. Supported: %s${NC}\n" "$SUPPORTED_LOGO_FORMATS" >&2
    return 1
  fi

  # Check file size
  local file_size_mb
  file_size_mb=$(branding::get_file_size_mb "$file_path")

  if (($(printf "%.0f" "$file_size_mb") > MAX_LOGO_SIZE_MB)); then
    printf "${RED}Error: Logo file too large (%.2f MB). Maximum: %d MB${NC}\n" "$file_size_mb" "$MAX_LOGO_SIZE_MB" >&2
    return 1
  fi

  # Basic file type validation using 'file' command if available
  if command -v file >/dev/null 2>&1; then
    local file_type
    file_type=$(file -b --mime-type "$file_path")

    case "$extension" in
      png)
        if [[ "$file_type" != "image/png" ]]; then
          printf "${RED}Error: File is not a valid PNG image${NC}\n" >&2
          return 1
        fi
        ;;
      jpg | jpeg)
        if [[ "$file_type" != "image/jpeg" ]]; then
          printf "${RED}Error: File is not a valid JPEG image${NC}\n" >&2
          return 1
        fi
        ;;
      svg)
        if [[ "$file_type" != "image/svg+xml" ]] && [[ "$file_type" != "text/xml" ]] && [[ "$file_type" != "text/plain" ]]; then
          printf "${RED}Error: File is not a valid SVG image${NC}\n" >&2
          return 1
        fi
        ;;
      webp)
        if [[ "$file_type" != "image/webp" ]]; then
          printf "${RED}Error: File is not a valid WebP image${NC}\n" >&2
          return 1
        fi
        ;;
    esac
  fi

  return 0
}

# Get file size in MB
branding::get_file_size_mb() {
  local file_path="$1"

  if [[ ! -f "$file_path" ]]; then
    echo "0"
    return 1
  fi

  # Get file size in bytes
  # Get file size using platform-safe wrapper
  local size_bytes
  size_bytes=$(safe_stat_size "$file_path")

  # Convert to MB
  local size_mb
  size_mb=$(awk "BEGIN {printf \"%.2f\", $size_bytes/1024/1024}")
  echo "$size_mb"
}

# Get absolute path (cross-platform)
branding::get_absolute_path() {
  local path="$1"

  if [[ "$path" = /* ]]; then
    echo "$path"
  else
    echo "$(pwd)/${path#./}"
  fi
}

upload_logo() {
  local logo_path="$1"
  local logo_type="main"

  shift
  # Parse additional options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --type)
        logo_type="$2"
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done

  upload_brand_logo "$logo_path" "$logo_type"
}

list_logos() {
  printf "${CYAN}Configured Logos:${NC}\n\n"

  local config_file="${BRANDING_DIR}/config.json"
  if [[ ! -f "$config_file" ]]; then
    printf "${YELLOW}No branding configured${NC}\n"
    return 0
  fi

  if command -v jq >/dev/null 2>&1; then
    local logos
    logos=$(jq -r '.logos | to_entries[] | "\(.key): \(.value // "not set")"' "$config_file")
    printf "%s\n" "$logos"
  else
    printf "${YELLOW}jq not available - cannot display logos${NC}\n"
  fi

  return 0
}

remove_logo() {
  local logo_type="$1"

  printf "${CYAN}Removing %s logo...${NC}\n" "$logo_type"

  local config_file="${BRANDING_DIR}/config.json"
  if command -v jq >/dev/null 2>&1; then
    local logo_filename
    logo_filename=$(jq -r --arg type "$logo_type" '.logos[$type]' "$config_file")

    if [[ "$logo_filename" != "null" ]] && [[ -n "$logo_filename" ]]; then
      local logo_path="${LOGOS_DIR}/${logo_filename}"
      [[ -f "$logo_path" ]] && rm -f "$logo_path"
    fi

    local temp_file
    temp_file=$(mktemp)
    jq --arg type "$logo_type" '.logos[$type] = null | .updatedAt = now | todate' \
      "$config_file" >"$temp_file" && mv "$temp_file" "$config_file"
  fi

  printf "${GREEN}✓${NC} Logo removed successfully\n"
  return 0
}

# ============================================================================
# Custom CSS Management
# ============================================================================

set_custom_css() {
  local css_path="$1"

  if [[ ! -f "$css_path" ]]; then
    printf "${RED}Error: CSS file not found: %s${NC}\n" "$css_path" >&2
    return 1
  fi

  printf "${CYAN}Setting custom CSS...${NC}\n"

  # Validate file extension
  validate_file_extension "$css_path" "css" || return 1

  # Validate file size
  validate_file_size "$css_path" "$MAX_CSS_SIZE_MB" || return 1

  # Validate CSS file structure
  if ! branding::validate_css_file "$css_path"; then
    return 1
  fi

  # Validate CSS for security issues (XSS prevention)
  if ! validate_css_security "$css_path"; then
    printf "${YELLOW}CSS security validation failed. Proceeding with caution.${NC}\n"
    # Don't return 1 - allow user to proceed if they accept warnings
  fi

  # Create version backup
  branding::create_version_backup

  # Generate versioned filename
  local timestamp
  timestamp=$(date +"%Y%m%d_%H%M%S")
  local css_filename="custom-${timestamp}.css"
  local dest_path="${CSS_DIR}/${css_filename}"

  # Copy CSS with secure permissions
  cp "$css_path" "$dest_path"
  chmod "$SECURE_FILE_PERMS" "$dest_path"

  # Create symlink for current version
  local symlink_path="${CSS_DIR}/custom.css"
  ln -sf "$css_filename" "$symlink_path" 2>/dev/null || true

  # Update config
  local config_file="${BRANDING_DIR}/config.json"
  if command -v jq >/dev/null 2>&1; then
    local temp_file
    temp_file=$(mktemp)
    local update_timestamp
    update_timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    jq --arg path "$css_filename" --arg timestamp "$update_timestamp" \
      '.customCSS = $path | .updatedAt = $timestamp' \
      "$config_file" >"$temp_file" && mv "$temp_file" "$config_file"

    chmod "$SECURE_FILE_PERMS" "$config_file"
  fi

  # Get file size
  local file_size
  file_size=$(branding::get_file_size_mb "$dest_path")

  printf "${GREEN}✓${NC} Custom CSS set successfully\n"
  printf "  Path: %s\n" "$dest_path"
  printf "  Size: %.2f MB\n" "$file_size"

  # Scan for potential security issues
  branding::scan_css_security "$dest_path"

  return 0
}

# Validate CSS file
branding::validate_css_file() {
  local file_path="$1"

  # Check file exists
  if [[ ! -f "$file_path" ]]; then
    printf "${RED}Error: File does not exist${NC}\n" >&2
    return 1
  fi

  # Check file extension
  if [[ "${file_path##*.}" != "css" ]]; then
    printf "${RED}Error: File must have .css extension${NC}\n" >&2
    return 1
  fi

  # Check file size
  local file_size_mb
  file_size_mb=$(branding::get_file_size_mb "$file_path")

  if (($(printf "%.0f" "$file_size_mb") > MAX_CSS_SIZE_MB)); then
    printf "${RED}Error: CSS file too large (%.2f MB). Maximum: %d MB${NC}\n" "$file_size_mb" "$MAX_CSS_SIZE_MB" >&2
    return 1
  fi

  # Basic syntax validation (check for balanced braces)
  local open_braces
  local close_braces
  open_braces=$(grep -o '{' "$file_path" | wc -l | tr -d ' ')
  close_braces=$(grep -o '}' "$file_path" | wc -l | tr -d ' ')

  if [[ "$open_braces" -ne "$close_braces" ]]; then
    printf "${RED}Error: CSS syntax error - mismatched braces${NC}\n" >&2
    return 1
  fi

  return 0
}

# Scan CSS for potential security issues
branding::scan_css_security() {
  local css_path="$1"
  local issues=0

  # Check for external URLs (potential data exfiltration)
  if grep -q 'url([^)]*//' "$css_path" 2>/dev/null; then
    printf "${YELLOW}Warning: CSS contains external URLs${NC}\n"
    issues=$((issues + 1))
  fi

  # Check for @import (can load external stylesheets)
  if grep -q '@import' "$css_path" 2>/dev/null; then
    printf "${YELLOW}Warning: CSS contains @import statements${NC}\n"
    issues=$((issues + 1))
  fi

  # Check for expression() (IE-specific XSS vector)
  if grep -qi 'expression(' "$css_path" 2>/dev/null; then
    printf "${RED}Error: CSS contains expression() - potential XSS vulnerability${NC}\n"
    issues=$((issues + 1))
  fi

  if [[ $issues -eq 0 ]]; then
    printf "${GREEN}✓${NC} CSS security scan passed\n"
  else
    printf "${YELLOW}⚠${NC}  CSS security scan found %d potential issue(s)\n" "$issues"
  fi

  return 0
}

# Remove custom CSS
remove_custom_css() {
  printf "${CYAN}Removing custom CSS...${NC}\n"

  local config_file="${BRANDING_DIR}/config.json"

  if [[ ! -f "$config_file" ]]; then
    printf "${RED}Error: No branding configuration found${NC}\n" >&2
    return 1
  fi

  # Create version backup
  branding::create_version_backup

  # Get current CSS file
  local css_filename
  if command -v jq >/dev/null 2>&1; then
    css_filename=$(jq -r '.customCSS // ""' "$config_file")

    if [[ -n "$css_filename" ]] && [[ "$css_filename" != "null" ]]; then
      local css_path="${CSS_DIR}/${css_filename}"
      [[ -f "$css_path" ]] && rm -f "$css_path"

      # Remove symlink
      [[ -L "${CSS_DIR}/custom.css" ]] && rm -f "${CSS_DIR}/custom.css"
    fi

    # Update config
    local temp_file
    temp_file=$(mktemp)
    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    jq --arg timestamp "$timestamp" \
      '.customCSS = null | .updatedAt = $timestamp' \
      "$config_file" >"$temp_file" && mv "$temp_file" "$config_file"

    chmod "$SECURE_FILE_PERMS" "$config_file"
  fi

  printf "${GREEN}✓${NC} Custom CSS removed\n"
  return 0
}

# ============================================================================
# Version Management & Backup
# ============================================================================

branding::create_version_backup() {
  local config_file="${BRANDING_DIR}/config.json"

  if [[ ! -f "$config_file" ]]; then
    return 0
  fi

  # Create versions directory if needed
  if [[ ! -d "$VERSIONS_DIR" ]]; then
    mkdir -p "$VERSIONS_DIR"
    chmod "$SECURE_DIR_PERMS" "$VERSIONS_DIR"
  fi

  # Generate backup filename with timestamp
  local timestamp
  timestamp=$(date +"%Y%m%d_%H%M%S")
  local backup_file="${VERSIONS_DIR}/config-${timestamp}.json"

  # Copy config to versions
  cp "$config_file" "$backup_file"
  chmod "$SECURE_FILE_PERMS" "$backup_file"

  # Keep only last 10 versions (cleanup old backups)
  branding::cleanup_old_versions
}

branding::cleanup_old_versions() {
  local version_count
  version_count=$(find "$VERSIONS_DIR" -name "config-*.json" 2>/dev/null | wc -l | tr -d ' ')

  if [[ $version_count -gt 10 ]]; then
    # Remove oldest versions, keep 10
    find "$VERSIONS_DIR" -name "config-*.json" -type f 2>/dev/null |
      sort | head -n -10 | xargs rm -f 2>/dev/null || true
  fi
}

branding::list_versions() {
  printf "${CYAN}Available Branding Versions${NC}\n"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  if [[ ! -d "$VERSIONS_DIR" ]]; then
    printf "${YELLOW}No versions found${NC}\n"
    return 0
  fi

  local versions
  versions=$(find "$VERSIONS_DIR" -name "config-*.json" -type f 2>/dev/null | sort -r)

  if [[ -z "$versions" ]]; then
    printf "${YELLOW}No versions found${NC}\n"
    return 0
  fi

  local count=1
  while IFS= read -r version_file; do
    local filename
    filename=$(basename "$version_file")
    local timestamp
    timestamp=$(echo "$filename" | sed 's/config-\(.*\)\.json/\1/')

    # Format timestamp for display
    local year="${timestamp:0:4}"
    local month="${timestamp:4:2}"
    local day="${timestamp:6:2}"
    local hour="${timestamp:9:2}"
    local minute="${timestamp:11:2}"
    local second="${timestamp:13:2}"

    printf "${GREEN}%2d.${NC} %s-%s-%s %s:%s:%s\n" \
      "$count" "$year" "$month" "$day" "$hour" "$minute" "$second"

    count=$((count + 1))
  done <<<"$versions"
}

branding::restore_version() {
  local version_timestamp="$1"

  if [[ -z "$version_timestamp" ]]; then
    printf "${RED}Error: Version timestamp required${NC}\n" >&2
    printf "Use: nself whitelabel branding list-versions\n"
    return 1
  fi

  local version_file="${VERSIONS_DIR}/config-${version_timestamp}.json"

  if [[ ! -f "$version_file" ]]; then
    printf "${RED}Error: Version not found: %s${NC}\n" "$version_timestamp" >&2
    return 1
  fi

  printf "${CYAN}Restoring version: %s${NC}\n" "$version_timestamp"

  # Create backup of current before restore
  branding::create_version_backup

  # Restore the version
  local config_file="${BRANDING_DIR}/config.json"
  cp "$version_file" "$config_file"
  chmod "$SECURE_FILE_PERMS" "$config_file"

  # Regenerate CSS
  generate_css_variables

  printf "${GREEN}✓${NC} Version restored successfully\n"
  return 0
}

# ============================================================================
# CSS Variables Generation
# ============================================================================

generate_css_variables() {
  local config_file="${BRANDING_DIR}/config.json"
  local output_file="${CSS_DIR}/variables.css"

  printf "${CYAN}Generating CSS variables...${NC}\n"

  if [[ ! -f "$config_file" ]]; then
    printf "${RED}Error: Branding config not found${NC}\n" >&2
    return 1
  fi

  # Start CSS file with header
  cat >"$output_file" <<'EOF'
/**
 * nself White-Label CSS Variables
 * Auto-generated from branding configuration
 * Do not edit manually - changes will be overwritten
 */

:root {
  /* ========================================
   * Colors
   * ======================================== */
EOF

  # Add colors if jq available
  if command -v jq >/dev/null 2>&1; then
    local colors
    colors=$(jq -r '.colors | to_entries[] | "  --color-\(.key): \(.value);"' "$config_file")
    printf "%s\n" "$colors" >>"$output_file"

    # Add fonts section
    cat >>"$output_file" <<'EOF'

  /* ========================================
   * Typography - Fonts
   * ======================================== */
EOF

    local fonts
    fonts=$(jq -r '.typography.fonts | to_entries[] | "  --font-\(.key): \(.value);"' "$config_file")
    printf "%s\n" "$fonts" >>"$output_file"

    # Add font sizes section
    cat >>"$output_file" <<'EOF'

  /* ========================================
   * Typography - Sizes
   * ======================================== */
EOF

    local sizes
    sizes=$(jq -r '.typography.sizes | to_entries[] | "  --font-size-\(.key): \(.value);"' "$config_file")
    printf "%s\n" "$sizes" >>"$output_file"

    # Add font weights section
    cat >>"$output_file" <<'EOF'

  /* ========================================
   * Typography - Weights
   * ======================================== */
EOF

    local weights
    weights=$(jq -r '.typography.weights | to_entries[] | "  --font-weight-\(.key): \(.value);"' "$config_file")
    printf "%s\n" "$weights" >>"$output_file"

    # Add line heights section
    cat >>"$output_file" <<'EOF'

  /* ========================================
   * Typography - Line Heights
   * ======================================== */
EOF

    local line_heights
    line_heights=$(jq -r '.typography.lineHeights | to_entries[] | "  --line-height-\(.key): \(.value);"' "$config_file")
    printf "%s\n" "$line_heights" >>"$output_file"
  fi

  # Close CSS file
  printf "}\n" >>"$output_file"

  # Set secure permissions
  chmod "$SECURE_FILE_PERMS" "$output_file"

  printf "${GREEN}✓${NC} CSS variables generated: %s\n" "$output_file"

  return 0
}

# ============================================================================
# Branding Preview
# ============================================================================

preview_branding() {
  printf "${CYAN}Branding Preview${NC}\n"
  printf "%s\n" "$(printf '%.s=' {1..60})"

  local config_file="${BRANDING_DIR}/config.json"
  if [[ ! -f "$config_file" ]]; then
    printf "${RED}No branding configured${NC}\n"
    return 0
  fi

  if command -v jq >/dev/null 2>&1; then
    local brand_name
    brand_name=$(jq -r '.brand.name // "Not set"' "$config_file")
    printf "\n${BLUE}Brand:${NC} %s\n" "$brand_name"

    local primary
    primary=$(jq -r '.colors.primary // "Not set"' "$config_file")
    printf "${BLUE}Primary Color:${NC} %s\n" "$primary"

    local secondary
    secondary=$(jq -r '.colors.secondary // "Not set"' "$config_file")
    printf "${BLUE}Secondary Color:${NC} %s\n" "$secondary"

    local font_primary
    font_primary=$(jq -r '.fonts.primary // "Not set"' "$config_file")
    printf "${BLUE}Primary Font:${NC} %s\n" "$font_primary"

    printf "\n${BLUE}Logos:${NC}\n"
    jq -r '.logos | to_entries[] | "  \(.key): \(.value // "not set")"' "$config_file"

    printf "\n${BLUE}Updated:${NC} "
    jq -r '.updatedAt // "Unknown"' "$config_file"
    printf "\n"
  else
    printf "${YELLOW}jq not available - cannot display preview${NC}\n"
  fi

  return 0
}

# ============================================================================
# Multi-Tenant Support
# ============================================================================

branding::get_tenant_config_path() {
  local tenant_id="${1:-default}"

  if [[ "$tenant_id" == "default" ]]; then
    echo "${BRANDING_DIR}/config.json"
  else
    echo "${BRANDING_DIR}/${tenant_id}/config.json"
  fi
}

branding::ensure_tenant_isolation() {
  local tenant_id="${1:-default}"

  if [[ "$tenant_id" == "default" ]]; then
    return 0
  fi

  # Create tenant-specific directories
  local tenant_dir="${BRANDING_DIR}/${tenant_id}"
  branding::create_secure_directory "$tenant_dir"
  branding::create_secure_directory "${tenant_dir}/logos"
  branding::create_secure_directory "${tenant_dir}/css"
  branding::create_secure_directory "${tenant_dir}/fonts"
  branding::create_secure_directory "${tenant_dir}/assets"

  return 0
}

list_tenants() {
  printf "${CYAN}Configured Tenants${NC}\n"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  # Default tenant
  printf "${GREEN}1.${NC} default (main)\n"

  # Find tenant directories
  if [[ -d "$BRANDING_DIR" ]]; then
    local count=2
    local tenant_dirs
    tenant_dirs=$(find "$BRANDING_DIR" -mindepth 1 -maxdepth 1 -type d -not -name "logos" -not -name "css" -not -name "fonts" -not -name "assets" -not -name "versions" 2>/dev/null)

    if [[ -n "$tenant_dirs" ]]; then
      while IFS= read -r tenant_dir; do
        local tenant_name
        tenant_name=$(basename "$tenant_dir")
        printf "${GREEN}%d.${NC} %s\n" "$count" "$tenant_name"
        count=$((count + 1))
      done <<<"$tenant_dirs"
    fi
  fi

  return 0
}

# ============================================================================
# Asset Management
# ============================================================================

branding::get_asset_info() {
  local asset_path="$1"

  if [[ ! -f "$asset_path" ]]; then
    printf "${RED}Asset not found${NC}\n"
    return 1
  fi

  printf "${CYAN}Asset Information${NC}\n"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  printf "Path: %s\n" "$asset_path"

  # File size
  local size_mb
  size_mb=$(branding::get_file_size_mb "$asset_path")
  printf "Size: %.2f MB\n" "$size_mb"

  # File type
  if command -v file >/dev/null 2>&1; then
    local file_type
    file_type=$(file -b --mime-type "$asset_path")
    printf "MIME Type: %s\n" "$file_type"
  fi

  # Permissions
  local perms
  perms=$(safe_stat_perms "$asset_path")
  printf "Permissions: %s\n" "$perms"

  # Last modified
  if command -v stat >/dev/null 2>&1; then
    local mtime
    if stat --version 2>/dev/null | grep -q GNU; then
      mtime=$(stat -c "%y" "$asset_path" | cut -d'.' -f1)
    else
      mtime=$(stat -f "%Sm" -t "%Y-%m-%d %H:%M:%S" "$asset_path")
    fi
    printf "Modified: %s\n" "$mtime"
  fi

  return 0
}

clean_unused_assets() {
  printf "${CYAN}Cleaning unused assets...${NC}\n"

  local config_file="${BRANDING_DIR}/config.json"
  if [[ ! -f "$config_file" ]]; then
    printf "${YELLOW}No configuration found${NC}\n"
    return 0
  fi

  local cleaned=0

  # Get list of referenced assets from config
  if command -v jq >/dev/null 2>&1; then
    # Check logos directory
    if [[ -d "$LOGOS_DIR" ]]; then
      local referenced_logos
      referenced_logos=$(jq -r '.logos | .[] | select(. != null)' "$config_file" 2>/dev/null)

      for logo_file in "$LOGOS_DIR"/*; do
        [[ -f "$logo_file" ]] || continue

        local basename_file
        basename_file=$(basename "$logo_file")

        # Skip .gitkeep
        [[ "$basename_file" == ".gitkeep" ]] && continue

        # Check if referenced
        local is_referenced=0
        while IFS= read -r ref; do
          if [[ "$basename_file" == "$ref" ]]; then
            is_referenced=1
            break
          fi
        done <<<"$referenced_logos"

        # Remove if not referenced and not a symlink
        if [[ $is_referenced -eq 0 ]] && [[ ! -L "$logo_file" ]]; then
          rm -f "$logo_file"
          printf "  Removed: %s\n" "$basename_file"
          cleaned=$((cleaned + 1))
        fi
      done
    fi
  fi

  if [[ $cleaned -eq 0 ]]; then
    printf "${GREEN}✓${NC} No unused assets found\n"
  else
    printf "${GREEN}✓${NC} Cleaned %d unused asset(s)\n" "$cleaned"
  fi

  return 0
}

# ============================================================================
# Helper Functions
# ============================================================================

view_whitelabel_settings() {
  preview_branding "$@"
}

initialize_whitelabel_system() {
  local tenant_id="${1:-default}"
  initialize_branding_system "$tenant_id"
  printf "${GREEN}✓${NC} White-label system initialized\n"
  return 0
}

list_whitelabel_resources() {
  printf "${CYAN}White-Label Resources${NC}\n"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  printf "${BLUE}Brands:${NC}\n"
  preview_branding
  printf "\n"

  printf "${BLUE}Logos:${NC}\n"
  list_logos
  printf "\n"

  printf "${BLUE}Tenants:${NC}\n"
  list_tenants

  return 0
}

export_whitelabel_config() {
  local format="${1:-json}"
  local tenant_id="${2:-default}"
  local config_file
  config_file=$(branding::get_tenant_config_path "$tenant_id")

  if [[ ! -f "$config_file" ]]; then
    printf "${RED}Error: No branding configuration found for tenant: %s${NC}\n" "$tenant_id" >&2
    return 1
  fi

  case "$format" in
    json)
      cat "$config_file"
      ;;
    yaml | yml)
      if command -v yq >/dev/null 2>&1; then
        yq eval -P "$config_file"
      elif command -v python3 >/dev/null 2>&1; then
        python3 -c "import json, yaml, sys; print(yaml.dump(json.load(open('$config_file'))))"
      else
        printf "${RED}Error: yq or python3 required for YAML export${NC}\n" >&2
        return 1
      fi
      ;;
    *)
      printf "${RED}Error: Unsupported format '%s'. Supported: json, yaml${NC}\n" "$format" >&2
      return 1
      ;;
  esac

  return 0
}

import_whitelabel_config() {
  local config_path="$1"
  local tenant_id="${2:-default}"

  if [[ ! -f "$config_path" ]]; then
    printf "${RED}Error: Config file not found: %s${NC}\n" "$config_path" >&2
    return 1
  fi

  printf "${CYAN}Importing white-label configuration...${NC}\n"

  # Validate JSON
  if command -v jq >/dev/null 2>&1; then
    if ! jq empty "$config_path" 2>/dev/null; then
      printf "${RED}Error: Invalid JSON in config file${NC}\n" >&2
      return 1
    fi

    # Validate required fields
    local has_brand
    has_brand=$(jq 'has("brand")' "$config_path")
    if [[ "$has_brand" != "true" ]]; then
      printf "${RED}Error: Config missing required 'brand' section${NC}\n" >&2
      return 1
    fi
  fi

  # Ensure tenant isolation
  branding::ensure_tenant_isolation "$tenant_id"

  # Get config path
  local config_file
  config_file=$(branding::get_tenant_config_path "$tenant_id")

  # Backup existing config
  if [[ -f "$config_file" ]]; then
    branding::create_version_backup
    printf "${YELLOW}Created backup of existing configuration${NC}\n"
  fi

  # Import new config
  cp "$config_path" "$config_file"
  chmod "$SECURE_FILE_PERMS" "$config_file"

  # Regenerate CSS variables
  generate_css_variables

  printf "${GREEN}✓${NC} Configuration imported successfully\n"

  return 0
}

# Validate entire branding configuration
validate_branding_config() {
  local tenant_id="${1:-default}"
  local config_file
  config_file=$(branding::get_tenant_config_path "$tenant_id")

  printf "${CYAN}Validating branding configuration...${NC}\n"

  if [[ ! -f "$config_file" ]]; then
    printf "${RED}Error: Configuration not found${NC}\n" >&2
    return 1
  fi

  local errors=0

  # Validate JSON structure
  if command -v jq >/dev/null 2>&1; then
    if ! jq empty "$config_file" 2>/dev/null; then
      printf "${RED}✗${NC} Invalid JSON\n"
      errors=$((errors + 1))
    else
      printf "${GREEN}✓${NC} Valid JSON\n"
    fi

    # Check required fields
    local required_fields="version brand colors typography logos"
    for field in $required_fields; do
      if jq -e ".$field" "$config_file" >/dev/null 2>&1; then
        printf "${GREEN}✓${NC} Field '%s' present\n" "$field"
      else
        printf "${RED}✗${NC} Field '%s' missing\n" "$field"
        errors=$((errors + 1))
      fi
    done

    # Validate colors are hex codes
    local colors
    colors=$(jq -r '.colors | to_entries[] | "\(.key)=\(.value)"' "$config_file")
    while IFS= read -r color_line; do
      local color_name="${color_line%%=*}"
      local color_value="${color_line#*=}"

      if validate_hex_color "$color_value"; then
        printf "${GREEN}✓${NC} Color '%s': %s\n" "$color_name" "$color_value"
      else
        printf "${RED}✗${NC} Invalid color '%s': %s\n" "$color_name" "$color_value"
        errors=$((errors + 1))
      fi
    done <<<"$colors"
  fi

  # Check file references exist
  if command -v jq >/dev/null 2>&1; then
    local logo_files
    logo_files=$(jq -r '.logos | .[] | select(. != null)' "$config_file" 2>/dev/null)

    while IFS= read -r logo_file; do
      [[ -z "$logo_file" ]] && continue

      local logo_path="${LOGOS_DIR}/${logo_file}"
      if [[ -f "$logo_path" ]]; then
        printf "${GREEN}✓${NC} Logo exists: %s\n" "$logo_file"
      else
        printf "${YELLOW}⚠${NC}  Logo missing: %s\n" "$logo_file"
      fi
    done <<<"$logo_files"
  fi

  printf "\n"
  if [[ $errors -eq 0 ]]; then
    printf "${GREEN}✓${NC} Configuration is valid\n"
    return 0
  else
    printf "${RED}✗${NC} Configuration has %d error(s)\n" "$errors"
    return 1
  fi
}
