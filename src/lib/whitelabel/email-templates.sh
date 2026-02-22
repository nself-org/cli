#!/usr/bin/env bash
# nself Email Templates System
# Manages custom email templates with variable injection and multi-language support
# Part of Sprint 14: White-Label & Customization (60pts) for v0.9.0


# Color definitions for output (guard against double-declaration when sourced together)
[[ -z "${RED:-}" ]] && readonly RED='\033[0;31m'

set -euo pipefail

[[ -z "${GREEN:-}" ]] && readonly GREEN='\033[0;32m'
[[ -z "${YELLOW:-}" ]] && readonly YELLOW='\033[1;33m'
[[ -z "${BLUE:-}" ]] && readonly BLUE='\033[0;34m'
[[ -z "${CYAN:-}" ]] && readonly CYAN='\033[0;36m'
[[ -z "${NC:-}" ]] && readonly NC='\033[0m'

# Email template configuration
readonly TEMPLATES_DIR="${PROJECT_ROOT}/branding/email-templates"
readonly TEMPLATES_LANG_DIR="${TEMPLATES_DIR}/languages"
readonly TEMPLATES_PREVIEW_DIR="${TEMPLATES_DIR}/previews"

# Default language
readonly DEFAULT_LANGUAGE="en"

# Template types
readonly TEMPLATE_TYPES="welcome password-reset verify-email invite password-change account-update notification alert"

# ============================================================================
# INPUT VALIDATION - Injection Prevention
# ============================================================================

# Validate template type
validate_template_type() {
  local template_type="$1"

  for valid_type in $TEMPLATE_TYPES; do
    if [[ "$template_type" == "$valid_type" ]]; then
      return 0
    fi
  done

  printf "${RED}Error: Invalid template type: $template_type${NC}\n" >&2
  printf "Valid types: %s\n" "$TEMPLATE_TYPES" >&2
  return 1
}

# Validate language code (ISO 639-1 format: en, fr, es, etc.)
validate_language_code() {
  local language="$1"

  if [[ -z "$language" ]]; then
    printf "${RED}Error: Language code cannot be empty${NC}\n" >&2
    return 1
  fi

  # Must be 2-5 lowercase letters (en, en-US, zh-CN, etc.)
  if ! [[ "$language" =~ ^[a-z]{2}(-[a-zA-Z]{2,4})?$ ]]; then
    printf "${RED}Error: Invalid language code: $language. Use ISO 639-1 format (e.g., en, fr, es)${NC}\n" >&2
    return 1
  fi

  return 0
}

# Validate template variable name (only uppercase, digits, underscore)
validate_variable_name() {
  local var_name="$1"

  if [[ -z "$var_name" ]]; then
    printf "${RED}Error: Variable name cannot be empty${NC}\n" >&2
    return 1
  fi

  if ! [[ "$var_name" =~ ^[A-Z][A-Z0-9_]*$ ]]; then
    printf "${RED}Error: Invalid variable name: $var_name. Must be uppercase alphanumeric and underscore${NC}\n" >&2
    return 1
  fi

  # Max length 64 characters
  if [[ ${#var_name} -gt 64 ]]; then
    printf "${RED}Error: Variable name too long. Maximum 64 characters${NC}\n" >&2
    return 1
  fi

  return 0
}

# Validate template variable value for HTML injection
validate_variable_value() {
  local var_value="$1"
  local field_name="${2:-Value}"

  if [[ -z "$var_value" ]]; then
    return 0 # Empty values are allowed
  fi

  # Max length 10000 characters to prevent abuse
  if [[ ${#var_value} -gt 10000 ]]; then
    printf "${RED}Error: %s too long. Maximum 10000 characters${NC}\n" "$field_name" >&2
    return 1
  fi

  # Check for potentially dangerous patterns (no code injection)
  if [[ "$var_value" =~ \$\( ]] || [[ "$var_value" =~ \` ]] || [[ "$var_value" =~ eval ]] || [[ "$var_value" =~ exec ]]; then
    printf "${RED}Error: %s contains potentially dangerous code${NC}\n" "$field_name" >&2
    return 1
  fi

  return 0
}

# Validate email template subject line
validate_email_subject() {
  local subject="$1"

  if [[ -z "$subject" ]]; then
    printf "${RED}Error: Email subject cannot be empty${NC}\n" >&2
    return 1
  fi

  if [[ ${#subject} -gt 255 ]]; then
    printf "${RED}Error: Email subject too long. Maximum 255 characters${NC}\n" >&2
    return 1
  fi

  # Warn about common spam trigger patterns
  if [[ "$subject" =~ ^\[\[|\{\{|\$\( ]]; then
    printf "${YELLOW}Warning: Subject line starts with template-like syntax${NC}\n"
  fi

  return 0
}

# Validate all variables in a template have values
validate_template_variables() {
  local template_content="$1"
  shift
  local -a provided_vars=("$@")

  # Extract all {{VAR_NAME}} patterns from template
  local template_vars
  template_vars=$(echo "$template_content" | grep -oE '\{\{[A-Z][A-Z0-9_]*\}\}' | sort -u)

  if [[ -z "$template_vars" ]]; then
    return 0 # No variables to validate
  fi

  local missing_vars=0

  while IFS= read -r var_pattern; do
    # Remove {{ and }}
    local var_name="${var_pattern:2:-2}"

    # Check if variable is provided
    local found=0
    for var in "${provided_vars[@]}"; do
      if [[ "$var" == "$var_name="* ]]; then
        found=1
        break
      fi
    done

    if [[ $found -eq 0 ]]; then
      # Check if it's a default variable
      case "$var_name" in
        CURRENT_YEAR | BRAND_NAME | APP_URL | LOGO_URL | COMPANY_ADDRESS | SUPPORT_EMAIL | SUPPORT_URL) ;;
        *)
          printf "${YELLOW}Warning: Template variable not provided: %s${NC}\n" "$var_name" >&2
          missing_vars=$((missing_vars + 1))
          ;;
      esac
    fi
  done <<<"$template_vars"

  return 0 # Don't fail, just warn
}

# Escape HTML special characters in email variables
escape_html_for_email() {
  local text="$1"

  # Replace HTML special characters
  text="${text//&/&amp;}"
  text="${text//</&lt;}"
  text="${text//>/&gt;}"
  text="${text//\"/&quot;}"
  text="${text//\'/&#39;}"

  printf "%s" "$text"
}

# Sanitize URL to prevent javascript: and data: protocols
sanitize_url() {
  local url="$1"

  # Check for dangerous protocols
  if [[ "$url" =~ ^(javascript:|data:|vbscript:) ]]; then
    printf "${RED}Error: URL contains dangerous protocol: $url${NC}\n" >&2
    return 1
  fi

  # URL should start with http://, https://, or /
  if ! [[ "$url" =~ ^(https?://|/) ]]; then
    printf "${RED}Error: Invalid URL format: $url. Must start with http://, https://, or /{{NC}\n" >&2
    return 1
  fi

  printf "%s" "$url"
  return 0
}

# ============================================================================
# Email Template System Initialization
# ============================================================================

initialize_email_templates() {
  printf "${CYAN}Initializing email templates system...${NC}\n"

  # Create template directories
  mkdir -p "$TEMPLATES_DIR"
  mkdir -p "$TEMPLATES_LANG_DIR"
  mkdir -p "$TEMPLATES_PREVIEW_DIR"

  # Create default language directory
  mkdir -p "${TEMPLATES_LANG_DIR}/${DEFAULT_LANGUAGE}"

  # Create default templates for each type
  for template_type in $TEMPLATE_TYPES; do
    create_default_template "$template_type" "$DEFAULT_LANGUAGE"
  done

  # Create template variables reference
  create_template_variables_reference

  printf "${GREEN}✓${NC} Email templates initialized\n"
  return 0
}

# ============================================================================
# Default Template Creation
# ============================================================================

create_default_template() {
  local template_type="$1"
  local language="$2"

  local template_dir="${TEMPLATES_LANG_DIR}/${language}"
  mkdir -p "$template_dir"

  local html_file="${template_dir}/${template_type}.html"
  local txt_file="${template_dir}/${template_type}.txt"
  local meta_file="${template_dir}/${template_type}.json"

  # Skip if already exists
  [[ -f "$html_file" ]] && return 0

  case "$template_type" in
    welcome)
      create_welcome_template "$html_file" "$txt_file" "$meta_file"
      ;;
    password-reset)
      create_password_reset_template "$html_file" "$txt_file" "$meta_file"
      ;;
    verify-email)
      create_verify_email_template "$html_file" "$txt_file" "$meta_file"
      ;;
    invite)
      create_invite_template "$html_file" "$txt_file" "$meta_file"
      ;;
    password-change)
      create_password_change_template "$html_file" "$txt_file" "$meta_file"
      ;;
    account-update)
      create_account_update_template "$html_file" "$txt_file" "$meta_file"
      ;;
    notification)
      create_notification_template "$html_file" "$txt_file" "$meta_file"
      ;;
    alert)
      create_alert_template "$html_file" "$txt_file" "$meta_file"
      ;;
  esac

  return 0
}

create_welcome_template() {
  local html_file="$1"
  local txt_file="$2"
  local meta_file="$3"

  # HTML version
  cat >"$html_file" <<'EOF'
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Welcome to {{BRAND_NAME}}</title>
  <style>
    body {
      font-family: var(--font-primary, Arial, sans-serif);
      line-height: 1.6;
      color: var(--color-text, #333);
      max-width: 600px;
      margin: 0 auto;
      padding: 20px;
    }
    .header {
      text-align: center;
      padding: 20px 0;
      border-bottom: 2px solid var(--color-primary, #0066cc);
    }
    .logo {
      max-width: 200px;
    }
    .content {
      padding: 30px 0;
    }
    .button {
      display: inline-block;
      padding: 12px 30px;
      background-color: var(--color-primary, #0066cc);
      color: #ffffff;
      text-decoration: none;
      border-radius: 5px;
      margin: 20px 0;
    }
    .footer {
      text-align: center;
      padding: 20px 0;
      border-top: 1px solid #e0e0e0;
      color: var(--color-textLight, #666);
      font-size: 14px;
    }
  </style>
</head>
<body>
  <div class="header">
    <img src="{{LOGO_URL}}" alt="{{BRAND_NAME}}" class="logo">
  </div>

  <div class="content">
    <h1>Welcome to {{BRAND_NAME}}!</h1>

    <p>Hi {{USER_NAME}},</p>

    <p>Welcome aboard! We're excited to have you as part of our community.</p>

    <p>Your account has been successfully created. Here's what you can do next:</p>

    <ul>
      <li>Complete your profile</li>
      <li>Explore our features</li>
      <li>Connect with other users</li>
    </ul>

    <p style="text-align: center;">
      <a href="{{APP_URL}}" class="button">Get Started</a>
    </p>

    <p>If you have any questions, feel free to reach out to our support team.</p>

    <p>Best regards,<br>
    The {{BRAND_NAME}} Team</p>
  </div>

  <div class="footer">
    <p>&copy; {{CURRENT_YEAR}} {{BRAND_NAME}}. All rights reserved.</p>
    <p>{{COMPANY_ADDRESS}}</p>
  </div>
</body>
</html>
EOF

  # Plain text version
  cat >"$txt_file" <<'EOF'
Welcome to {{BRAND_NAME}}!

Hi {{USER_NAME}},

Welcome aboard! We're excited to have you as part of our community.

Your account has been successfully created. Here's what you can do next:

- Complete your profile
- Explore our features
- Connect with other users

Get started: {{APP_URL}}

If you have any questions, feel free to reach out to our support team.

Best regards,
The {{BRAND_NAME}} Team

---
© {{CURRENT_YEAR}} {{BRAND_NAME}}. All rights reserved.
{{COMPANY_ADDRESS}}
EOF

  # Metadata
  cat >"$meta_file" <<'EOF'
{
  "name": "welcome",
  "subject": "Welcome to {{BRAND_NAME}}!",
  "description": "Welcome email sent to new users upon registration",
  "variables": [
    "BRAND_NAME",
    "USER_NAME",
    "LOGO_URL",
    "APP_URL",
    "CURRENT_YEAR",
    "COMPANY_ADDRESS"
  ],
  "category": "authentication"
}
EOF
}

create_password_reset_template() {
  local html_file="$1"
  local txt_file="$2"
  local meta_file="$3"

  cat >"$html_file" <<'EOF'
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>Password Reset Request</title>
  <style>
    body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
    .header { text-align: center; padding: 20px 0; }
    .content { padding: 20px 0; }
    .button { display: inline-block; padding: 12px 30px; background-color: #0066cc; color: #fff; text-decoration: none; border-radius: 5px; }
    .warning { background-color: #fff3cd; border: 1px solid #ffc107; padding: 15px; border-radius: 5px; margin: 20px 0; }
    .footer { text-align: center; padding: 20px 0; color: #666; font-size: 14px; }
  </style>
</head>
<body>
  <div class="header">
    <h1>Password Reset Request</h1>
  </div>

  <div class="content">
    <p>Hi {{USER_NAME}},</p>

    <p>We received a request to reset your password for your {{BRAND_NAME}} account.</p>

    <p style="text-align: center;">
      <a href="{{RESET_URL}}" class="button">Reset Password</a>
    </p>

    <div class="warning">
      <strong>Security Notice:</strong> This link will expire in {{EXPIRY_TIME}}. If you didn't request this password reset, please ignore this email or contact support if you have concerns.
    </div>

    <p>Alternatively, you can copy and paste this link into your browser:</p>
    <p style="word-break: break-all; color: #0066cc;">{{RESET_URL}}</p>
  </div>

  <div class="footer">
    <p>&copy; {{CURRENT_YEAR}} {{BRAND_NAME}}</p>
  </div>
</body>
</html>
EOF

  cat >"$txt_file" <<'EOF'
Password Reset Request

Hi {{USER_NAME}},

We received a request to reset your password for your {{BRAND_NAME}} account.

Reset your password: {{RESET_URL}}

SECURITY NOTICE: This link will expire in {{EXPIRY_TIME}}. If you didn't request this password reset, please ignore this email or contact support.

---
© {{CURRENT_YEAR}} {{BRAND_NAME}}
EOF

  cat >"$meta_file" <<'EOF'
{
  "name": "password-reset",
  "subject": "Reset Your Password",
  "description": "Password reset email with secure reset link",
  "variables": ["BRAND_NAME", "USER_NAME", "RESET_URL", "EXPIRY_TIME", "CURRENT_YEAR"],
  "category": "security"
}
EOF
}

create_verify_email_template() {
  local html_file="$1"
  local txt_file="$2"
  local meta_file="$3"

  cat >"$html_file" <<'EOF'
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>Verify Your Email Address</title>
  <style>
    body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
    .button { display: inline-block; padding: 12px 30px; background-color: #00cc66; color: #fff; text-decoration: none; border-radius: 5px; }
  </style>
</head>
<body>
  <h1>Verify Your Email Address</h1>
  <p>Hi {{USER_NAME}},</p>
  <p>Please verify your email address by clicking the button below:</p>
  <p style="text-align: center;">
    <a href="{{VERIFY_URL}}" class="button">Verify Email</a>
  </p>
  <p>Or copy this link: {{VERIFY_URL}}</p>
  <p>This link expires in {{EXPIRY_TIME}}.</p>
</body>
</html>
EOF

  cat >"$txt_file" <<'EOF'
Verify Your Email Address

Hi {{USER_NAME}},

Please verify your email address by visiting: {{VERIFY_URL}}

This link expires in {{EXPIRY_TIME}}.
EOF

  cat >"$meta_file" <<'EOF'
{
  "name": "verify-email",
  "subject": "Verify Your Email Address",
  "description": "Email verification with confirmation link",
  "variables": ["USER_NAME", "VERIFY_URL", "EXPIRY_TIME"],
  "category": "authentication"
}
EOF
}

create_invite_template() {
  local html_file="$1"
  local txt_file="$2"
  local meta_file="$3"

  cat >"$html_file" <<'EOF'
<!DOCTYPE html>
<html>
<body>
  <h1>You're Invited!</h1>
  <p>Hi {{RECIPIENT_NAME}},</p>
  <p>{{SENDER_NAME}} has invited you to join {{BRAND_NAME}}.</p>
  <p><a href="{{INVITE_URL}}">Accept Invitation</a></p>
</body>
</html>
EOF

  cat >"$txt_file" <<'EOF'
You're Invited!

Hi {{RECIPIENT_NAME}},

{{SENDER_NAME}} has invited you to join {{BRAND_NAME}}.

Accept invitation: {{INVITE_URL}}
EOF

  cat >"$meta_file" <<'EOF'
{
  "name": "invite",
  "subject": "You're invited to {{BRAND_NAME}}",
  "description": "User invitation email",
  "variables": ["RECIPIENT_NAME", "SENDER_NAME", "BRAND_NAME", "INVITE_URL"],
  "category": "social"
}
EOF
}

create_password_change_template() {
  local html_file="$1"
  local txt_file="$2"
  local meta_file="$3"

  cat >"$html_file" <<'EOF'
<!DOCTYPE html>
<html>
<body>
  <h1>Password Changed</h1>
  <p>Hi {{USER_NAME}},</p>
  <p>Your password was successfully changed on {{CHANGE_DATE}}.</p>
  <p>If you didn't make this change, contact support immediately.</p>
</body>
</html>
EOF

  cat >"$txt_file" <<'EOF'
Password Changed

Hi {{USER_NAME}},

Your password was successfully changed on {{CHANGE_DATE}}.

If you didn't make this change, contact support immediately.
EOF

  cat >"$meta_file" <<'EOF'
{
  "name": "password-change",
  "subject": "Your password has been changed",
  "description": "Password change confirmation",
  "variables": ["USER_NAME", "CHANGE_DATE"],
  "category": "security"
}
EOF
}

create_account_update_template() {
  local html_file="$1"
  local txt_file="$2"
  local meta_file="$3"

  cat >"$html_file" <<'EOF'
<!DOCTYPE html>
<html>
<body>
  <h1>Account Updated</h1>
  <p>Hi {{USER_NAME}},</p>
  <p>Your account information was updated: {{UPDATE_DESCRIPTION}}</p>
</body>
</html>
EOF

  cat >"$txt_file" <<'EOF'
Account Updated

Hi {{USER_NAME}},

Your account information was updated: {{UPDATE_DESCRIPTION}}
EOF

  cat >"$meta_file" <<'EOF'
{
  "name": "account-update",
  "subject": "Account Update Notification",
  "description": "General account update notification",
  "variables": ["USER_NAME", "UPDATE_DESCRIPTION"],
  "category": "account"
}
EOF
}

create_notification_template() {
  local html_file="$1"
  local txt_file="$2"
  local meta_file="$3"

  cat >"$html_file" <<'EOF'
<!DOCTYPE html>
<html>
<body>
  <h1>{{NOTIFICATION_TITLE}}</h1>
  <p>{{NOTIFICATION_MESSAGE}}</p>
  <p><a href="{{ACTION_URL}}">{{ACTION_TEXT}}</a></p>
</body>
</html>
EOF

  cat >"$txt_file" <<'EOF'
{{NOTIFICATION_TITLE}}

{{NOTIFICATION_MESSAGE}}

{{ACTION_TEXT}}: {{ACTION_URL}}
EOF

  cat >"$meta_file" <<'EOF'
{
  "name": "notification",
  "subject": "{{NOTIFICATION_TITLE}}",
  "description": "Generic notification template",
  "variables": ["NOTIFICATION_TITLE", "NOTIFICATION_MESSAGE", "ACTION_URL", "ACTION_TEXT"],
  "category": "notifications"
}
EOF
}

create_alert_template() {
  local html_file="$1"
  local txt_file="$2"
  local meta_file="$3"

  cat >"$html_file" <<'EOF'
<!DOCTYPE html>
<html>
<body style="background-color: #fff3cd;">
  <h1 style="color: #856404;">⚠️ {{ALERT_TITLE}}</h1>
  <p>{{ALERT_MESSAGE}}</p>
  <p><strong>Action Required:</strong> {{ACTION_REQUIRED}}</p>
</body>
</html>
EOF

  cat >"$txt_file" <<'EOF'
⚠️ {{ALERT_TITLE}}

{{ALERT_MESSAGE}}

Action Required: {{ACTION_REQUIRED}}
EOF

  cat >"$meta_file" <<'EOF'
{
  "name": "alert",
  "subject": "⚠️ {{ALERT_TITLE}}",
  "description": "Alert/warning notification template",
  "variables": ["ALERT_TITLE", "ALERT_MESSAGE", "ACTION_REQUIRED"],
  "category": "alerts"
}
EOF
}

# ============================================================================
# Template Variables Reference
# ============================================================================

create_template_variables_reference() {
  local ref_file="${TEMPLATES_DIR}/VARIABLES.md"

  cat >"$ref_file" <<'EOF'
# Email Template Variables Reference

## Global Variables (Available in all templates)

- `{{BRAND_NAME}}` - Brand/company name
- `{{LOGO_URL}}` - URL to brand logo
- `{{APP_URL}}` - Main application URL
- `{{CURRENT_YEAR}}` - Current year
- `{{COMPANY_ADDRESS}}` - Company address
- `{{SUPPORT_EMAIL}}` - Support email address
- `{{SUPPORT_URL}}` - Support page URL

## User Variables

- `{{USER_NAME}}` - User's display name
- `{{USER_EMAIL}}` - User's email address
- `{{USER_ID}}` - User's unique ID

## Authentication Variables

- `{{RESET_URL}}` - Password reset URL
- `{{VERIFY_URL}}` - Email verification URL
- `{{INVITE_URL}}` - Invitation acceptance URL
- `{{EXPIRY_TIME}}` - Link expiration time (e.g., "24 hours")

## Notification Variables

- `{{NOTIFICATION_TITLE}}` - Notification headline
- `{{NOTIFICATION_MESSAGE}}` - Notification body
- `{{ACTION_URL}}` - Call-to-action URL
- `{{ACTION_TEXT}}` - Call-to-action button text

## Alert Variables

- `{{ALERT_TITLE}}` - Alert headline
- `{{ALERT_MESSAGE}}` - Alert message
- `{{ACTION_REQUIRED}}` - Required action description

## Social Variables

- `{{SENDER_NAME}}` - Name of user who sent invite/message
- `{{RECIPIENT_NAME}}` - Name of recipient

## Date Variables

- `{{CHANGE_DATE}}` - Date of change/update
- `{{EVENT_DATE}}` - Event date/time

## Custom Variables

Add your own custom variables in the format `{{CUSTOM_VAR_NAME}}`
EOF
}

# ============================================================================
# Security - HTML Escaping & Sanitization
# ============================================================================

html_escape() {
  local input="$1"
  # Escape HTML special characters to prevent XSS
  printf "%s" "$input" | sed 's/&/\&amp;/g; s/</\&lt;/g; s/>/\&gt;/g; s/"/\&quot;/g; s/'\''/\&#39;/g'
}

sanitize_variable_name() {
  local var_name="$1"
  # Only allow uppercase alphanumeric and underscore (no command injection)
  # Remove ALL characters except A-Z, 0-9, and underscore
  printf "%s" "$var_name" | tr -cd 'A-Z0-9_'
}

validate_template_content() {
  local template_file="$1"

  # Check for dangerous patterns
  if grep -qE '\$\(|\`|eval|exec|source|bash|sh' "$template_file" 2>/dev/null; then
    printf "${RED}Error: Template contains potentially dangerous code${NC}\n" >&2
    return 1
  fi

  # Validate HTML structure (basic check)
  if grep -q '<!DOCTYPE html>' "$template_file"; then
    if ! grep -q '</html>' "$template_file"; then
      printf "${YELLOW}Warning: HTML template missing closing tag${NC}\n" >&2
    fi
  fi

  return 0
}

# ============================================================================
# Template Variable Substitution
# ============================================================================

substitute_template_variables() {
  local template_content="$1"
  shift
  local -a var_pairs=("$@")

  local result="$template_content"
  local i

  # Process variable pairs (NAME=value)
  for ((i = 0; i < ${#var_pairs[@]}; i++)); do
    local pair="${var_pairs[$i]}"
    if [[ "$pair" =~ ^([A-Z0-9_]+)=(.*)$ ]]; then
      local var_name="${BASH_REMATCH[1]}"
      local var_value="${BASH_REMATCH[2]}"

      # Validate variable name format
      if ! validate_variable_name "$var_name"; then
        printf "${YELLOW}Skipping invalid variable: %s${NC}\n" "$var_name" >&2
        continue
      fi

      # Validate variable value for injection
      if ! validate_variable_value "$var_value" "Variable $var_name"; then
        return 1
      fi

      # Sanitize variable name
      var_name=$(sanitize_variable_name "$var_name")

      # HTML escape the value for security
      local escaped_value
      escaped_value=$(html_escape "$var_value")

      # Replace all occurrences of {{VAR_NAME}}
      result="${result//\{\{${var_name}\}\}/${escaped_value}}"
    fi
  done

  printf "%s" "$result"
}

render_template() {
  local template_type="$1"
  local language="${2:-$DEFAULT_LANGUAGE}"
  local format="${3:-html}" # html or txt
  shift 3
  local -a variables=("$@")

  # Validate template type
  validate_template_type "$template_type" || return 1

  # Validate language code
  validate_language_code "$language" || return 1

  # Validate format
  if ! [[ "$format" =~ ^(html|txt|text)$ ]]; then
    printf "${RED}Error: Invalid format: %s. Must be html or txt${NC}\n" "$format" >&2
    return 1
  fi

  local template_file="${TEMPLATES_LANG_DIR}/${language}/${template_type}.${format}"

  if [[ ! -f "$template_file" ]]; then
    printf "${RED}Error: Template not found: %s (%s)${NC}\n" "$template_type" "$format" >&2
    return 1
  fi

  # Read template content
  local template_content
  template_content=$(cat "$template_file")

  # Validate template content for dangerous code
  if ! validate_template_content "$template_file"; then
    printf "${RED}Error: Template security validation failed${NC}\n" >&2
    return 1
  fi

  # Add default variables if not provided
  local default_vars=(
    "CURRENT_YEAR=$(date +%Y)"
    "BRAND_NAME=${BRAND_NAME:-${PROJECT_NAME:-nself}}"
    "APP_URL=${APP_URL:-${BASE_DOMAIN:-localhost}}"
    "LOGO_URL=${LOGO_URL:-}"
    "COMPANY_ADDRESS=${COMPANY_ADDRESS:-}"
    "SUPPORT_EMAIL=${SUPPORT_EMAIL:-support@${BASE_DOMAIN:-localhost}}"
    "SUPPORT_URL=${SUPPORT_URL:-${APP_URL:-}/support}"
  )

  # Combine default and provided variables
  local all_vars=("${default_vars[@]}" "${variables[@]}")

  # Substitute variables
  local rendered
  rendered=$(substitute_template_variables "$template_content" "${all_vars[@]}")

  printf "%s" "$rendered"
}

get_template_subject() {
  local template_type="$1"
  local language="${2:-$DEFAULT_LANGUAGE}"
  shift 2
  local -a variables=("$@")

  local meta_file="${TEMPLATES_LANG_DIR}/${language}/${template_type}.json"

  if [[ ! -f "$meta_file" ]] || ! command -v jq >/dev/null 2>&1; then
    printf "Email from %s" "${BRAND_NAME:-nself}"
    return 0
  fi

  local subject
  subject=$(jq -r '.subject' "$meta_file" 2>/dev/null || echo "Email from ${BRAND_NAME:-nself}")

  # Substitute variables in subject line
  local rendered_subject
  rendered_subject=$(substitute_template_variables "$subject" "${variables[@]}")

  printf "%s" "$rendered_subject"
}

# ============================================================================
# Template Management
# ============================================================================

list_email_templates() {
  local language="${1:-$DEFAULT_LANGUAGE}"

  printf "${CYAN}Email Templates (Language: %s)${NC}\n" "$language"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  local template_dir="${TEMPLATES_LANG_DIR}/${language}"
  if [[ ! -d "$template_dir" ]]; then
    printf "${YELLOW}No templates found for language: %s${NC}\n" "$language"
    return 0
  fi

  for template_type in $TEMPLATE_TYPES; do
    local meta_file="${template_dir}/${template_type}.json"
    if [[ -f "$meta_file" ]] && command -v jq >/dev/null 2>&1; then
      local subject
      subject=$(jq -r '.subject' "$meta_file")
      local description
      description=$(jq -r '.description' "$meta_file")
      local category
      category=$(jq -r '.category' "$meta_file")

      printf "${BLUE}%-20s${NC} %s\n" "$template_type" "$description"
      printf "  Subject: %s\n" "$subject"
      printf "  Category: %s\n\n" "$category"
    else
      printf "${BLUE}%-20s${NC} (Available)\n\n" "$template_type"
    fi
  done

  return 0
}

list_template_variables() {
  local template_type="$1"
  local language="${2:-$DEFAULT_LANGUAGE}"

  local meta_file="${TEMPLATES_LANG_DIR}/${language}/${template_type}.json"

  if [[ ! -f "$meta_file" ]]; then
    printf "${RED}Error: Template not found: %s${NC}\n" "$template_type" >&2
    return 1
  fi

  if ! command -v jq >/dev/null 2>&1; then
    printf "${YELLOW}Warning: jq not installed, cannot parse template metadata${NC}\n" >&2
    return 1
  fi

  printf "${CYAN}Variables for template: %s${NC}\n" "$template_type"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  local variables
  variables=$(jq -r '.variables[]' "$meta_file" 2>/dev/null)

  if [[ -n "$variables" ]]; then
    while IFS= read -r var; do
      printf "  • {{%s}}\n" "$var"
    done <<<"$variables"
  else
    printf "  ${YELLOW}No variables defined${NC}\n"
  fi

  printf "\n"
  return 0
}

edit_email_template() {
  local template_name="$1"
  local language="${2:-$DEFAULT_LANGUAGE}"
  local format="${3:-html}"

  local template_file="${TEMPLATES_LANG_DIR}/${language}/${template_name}.${format}"

  if [[ ! -f "$template_file" ]]; then
    printf "${RED}Error: Template not found: %s.%s${NC}\n" "$template_name" "$format" >&2
    return 1
  fi

  # Backup original template
  cp "$template_file" "${template_file}.backup-$(date +%Y%m%d_%H%M%S)"

  # Open in default editor
  local editor="${EDITOR:-vi}"
  "$editor" "$template_file"

  # Validate after edit
  if ! validate_template_content "$template_file"; then
    printf "${RED}Template validation failed. Restoring backup...${NC}\n" >&2
    mv "${template_file}.backup-"* "$template_file" 2>/dev/null
    return 1
  fi

  printf "${GREEN}✓${NC} Template updated: %s.%s\n" "$template_name" "$format"
  printf "  Backup saved: %s.backup-*\n" "$template_file"

  return 0
}

preview_email_template() {
  local template_name="$1"
  local language="${2:-$DEFAULT_LANGUAGE}"
  local format="${3:-html}"

  local template_file="${TEMPLATES_LANG_DIR}/${language}/${template_name}.${format}"

  if [[ ! -f "$template_file" ]]; then
    printf "${RED}Error: Template not found: %s.%s${NC}\n" "$template_name" "$format" >&2
    return 1
  fi

  printf "${CYAN}Preview: %s.%s (Language: %s)${NC}\n" "$template_name" "$format" "$language"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  # Sample data for preview
  local sample_vars=(
    "BRAND_NAME=nself"
    "USER_NAME=John Doe"
    "USER_EMAIL=john@example.com"
    "APP_URL=https://app.example.com"
    "RESET_URL=https://app.example.com/reset?token=abc123"
    "VERIFY_URL=https://app.example.com/verify?token=xyz789"
    "INVITE_URL=https://app.example.com/invite?code=invite123"
    "EXPIRY_TIME=24 hours"
    "CHANGE_DATE=$(date '+%B %d, %Y')"
    "NOTIFICATION_TITLE=New Feature Available"
    "NOTIFICATION_MESSAGE=We've added exciting new features to your account"
    "ACTION_URL=https://app.example.com/features"
    "ACTION_TEXT=View Features"
    "ALERT_TITLE=Security Alert"
    "ALERT_MESSAGE=Unusual login activity detected"
    "ACTION_REQUIRED=Review your recent login history"
    "SENDER_NAME=Jane Smith"
    "RECIPIENT_NAME=John Doe"
    "UPDATE_DESCRIPTION=Email address changed"
  )

  # Render with sample data
  local rendered
  rendered=$(render_template "$template_name" "$language" "$format" "${sample_vars[@]}")

  printf "%s\n\n" "$rendered"

  # Generate preview file
  local preview_file="${TEMPLATES_PREVIEW_DIR}/${template_name}-${language}.${format}"
  printf "%s" "$rendered" >"$preview_file"

  printf "${GREEN}✓${NC} Preview saved: %s\n" "$preview_file"

  return 0
}

export_template_html() {
  local template_name="$1"
  local language="${2:-$DEFAULT_LANGUAGE}"
  local output_file="${3:-${template_name}.html}"

  if ! preview_email_template "$template_name" "$language" "html" >"$output_file" 2>/dev/null; then
    printf "${RED}Error: Failed to export template${NC}\n" >&2
    return 1
  fi

  printf "${GREEN}✓${NC} Template exported: %s\n" "$output_file"
  return 0
}

# ============================================================================
# Email Sending Integration
# ============================================================================

send_email_from_template() {
  local template_type="$1"
  local recipient_email="$2"
  local language="${3:-$DEFAULT_LANGUAGE}"
  shift 3
  local -a variables=("$@")

  # Validate recipient email
  if [[ ! "$recipient_email" =~ ^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$ ]]; then
    printf "${RED}Error: Invalid email address: %s${NC}\n" "$recipient_email" >&2
    return 1
  fi

  # Check SMTP configuration
  local smtp_host="${AUTH_SMTP_HOST:-}"
  local smtp_port="${AUTH_SMTP_PORT:-587}"
  local smtp_user="${AUTH_SMTP_USER:-}"
  local smtp_pass="${AUTH_SMTP_PASS:-}"
  local smtp_sender="${AUTH_SMTP_SENDER:-noreply@${BASE_DOMAIN:-localhost}}"

  if [[ -z "$smtp_host" ]]; then
    printf "${YELLOW}Warning: SMTP not configured. Email will be queued but not sent.${NC}\n" >&2
    printf "${CYAN}Configure SMTP with: nself email setup${NC}\n" >&2
    return 1
  fi

  # Get subject line
  local subject
  subject=$(get_template_subject "$template_type" "$language" "${variables[@]}")

  # Render HTML version
  local html_body
  html_body=$(render_template "$template_type" "$language" "html" "${variables[@]}")

  # Render plain text version
  local txt_body
  txt_body=$(render_template "$template_type" "$language" "txt" "${variables[@]}")

  # Create temporary files for email content
  local tmp_html
  tmp_html=$(mktemp)
  local tmp_txt
  tmp_txt=$(mktemp)

  printf "%s" "$html_body" >"$tmp_html"
  printf "%s" "$txt_body" >"$tmp_txt"

  # Send email using swaks (in Docker)
  printf "${CYAN}Sending email...${NC}\n"

  local result=0
  if command -v docker >/dev/null 2>&1; then
    docker run --rm \
      --network host \
      -v "$tmp_html:/tmp/body.html:ro" \
      -v "$tmp_txt:/tmp/body.txt:ro" \
      boky/swaks \
      --to "$recipient_email" \
      --from "$smtp_sender" \
      --server "$smtp_host:$smtp_port" \
      --auth-user "$smtp_user" \
      --auth-password "$smtp_pass" \
      --tls \
      --header "Subject: $subject" \
      --body /tmp/body.txt \
      --attach-body /tmp/body.html \
      --timeout 30 2>&1 || result=$?
  else
    printf "${YELLOW}Warning: Docker not available, cannot send email${NC}\n" >&2
    result=1
  fi

  # Cleanup
  rm -f "$tmp_html" "$tmp_txt"

  if [[ $result -eq 0 ]]; then
    printf "${GREEN}✓${NC} Email sent to: %s\n" "$recipient_email"
    printf "  Template: %s\n" "$template_type"
    printf "  Subject: %s\n" "$subject"
  else
    printf "${RED}✗${NC} Failed to send email\n" >&2
  fi

  return $result
}

test_email_template() {
  local template_name="$1"
  local recipient_email="$2"
  local language="${3:-$DEFAULT_LANGUAGE}"

  printf "${CYAN}Sending test email to: %s${NC}\n" "$recipient_email"
  printf "  Template: %s\n" "$template_name"
  printf "  Language: %s\n\n" "$language"

  # Sample test variables
  local test_vars=(
    "USER_NAME=Test User"
    "USER_EMAIL=$recipient_email"
    "RESET_URL=https://example.com/reset?test=true"
    "VERIFY_URL=https://example.com/verify?test=true"
    "INVITE_URL=https://example.com/invite?test=true"
    "EXPIRY_TIME=24 hours"
    "NOTIFICATION_TITLE=Test Notification"
    "NOTIFICATION_MESSAGE=This is a test email from nself"
    "ACTION_URL=https://example.com/action"
    "ACTION_TEXT=Click Here"
    "ALERT_TITLE=Test Alert"
    "ALERT_MESSAGE=This is a test alert message"
    "ACTION_REQUIRED=No action required - this is a test"
    "SENDER_NAME=nself System"
    "RECIPIENT_NAME=Test User"
    "CHANGE_DATE=$(date '+%B %d, %Y')"
    "UPDATE_DESCRIPTION=Test update"
  )

  send_email_from_template "$template_name" "$recipient_email" "$language" "${test_vars[@]}"
}

# ============================================================================
# Multi-Language Support
# ============================================================================

list_available_languages() {
  printf "${CYAN}Available Email Languages${NC}\n"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  if [[ ! -d "$TEMPLATES_LANG_DIR" ]]; then
    printf "${YELLOW}No languages configured${NC}\n"
    return 0
  fi

  local lang_count=0
  for lang_dir in "$TEMPLATES_LANG_DIR"/*; do
    if [[ -d "$lang_dir" ]]; then
      local lang_code
      lang_code=$(basename "$lang_dir")
      local template_count
      template_count=$(find "$lang_dir" -name "*.html" -type f | wc -l | tr -d ' ')

      printf "  ${BLUE}%s${NC} - %s templates\n" "$lang_code" "$template_count"
      lang_count=$((lang_count + 1))
    fi
  done

  if [[ $lang_count -eq 0 ]]; then
    printf "${YELLOW}No languages configured${NC}\n"
  fi

  printf "\n"
  return 0
}

set_email_language() {
  local language="$1"

  printf "${CYAN}Setting email language to: %s${NC}\n" "$language"

  # Create language directory if it doesn't exist
  local lang_dir="${TEMPLATES_LANG_DIR}/${language}"
  mkdir -p "$lang_dir"

  # Initialize templates for this language
  for template_type in $TEMPLATE_TYPES; do
    create_default_template "$template_type" "$language"
  done

  printf "${GREEN}✓${NC} Email language set to: %s\n" "$language"
  printf "  Templates created: %s\n" "$TEMPLATE_TYPES"

  return 0
}

copy_templates_to_language() {
  local source_lang="$1"
  local target_lang="$2"

  local source_dir="${TEMPLATES_LANG_DIR}/${source_lang}"
  local target_dir="${TEMPLATES_LANG_DIR}/${target_lang}"

  if [[ ! -d "$source_dir" ]]; then
    printf "${RED}Error: Source language not found: %s${NC}\n" "$source_lang" >&2
    return 1
  fi

  mkdir -p "$target_dir"

  printf "${CYAN}Copying templates from %s to %s...${NC}\n" "$source_lang" "$target_lang"

  local count=0
  for template_file in "$source_dir"/*.{html,txt,json}; do
    if [[ -f "$template_file" ]]; then
      cp "$template_file" "$target_dir/"
      count=$((count + 1))
    fi
  done

  printf "${GREEN}✓${NC} Copied %s template files\n" "$count"
  printf "  ${YELLOW}Note: You should translate the content to %s${NC}\n" "$target_lang"

  return 0
}

# ============================================================================
# Custom Template Upload
# ============================================================================

upload_custom_template() {
  local template_type="$1"
  local html_file="$2"
  local txt_file="${3:-}"
  local language="${4:-$DEFAULT_LANGUAGE}"

  if [[ ! -f "$html_file" ]]; then
    printf "${RED}Error: HTML file not found: %s${NC}\n" "$html_file" >&2
    return 1
  fi

  # Validate template content
  if ! validate_template_content "$html_file"; then
    printf "${RED}Error: Template validation failed${NC}\n" >&2
    return 1
  fi

  local lang_dir="${TEMPLATES_LANG_DIR}/${language}"
  mkdir -p "$lang_dir"

  # Backup existing template if it exists
  local target_html="${lang_dir}/${template_type}.html"
  if [[ -f "$target_html" ]]; then
    cp "$target_html" "${target_html}.backup-$(date +%Y%m%d_%H%M%S)"
    printf "${YELLOW}Backed up existing template${NC}\n"
  fi

  # Copy new template
  cp "$html_file" "$target_html"
  printf "${GREEN}✓${NC} Uploaded HTML template: %s\n" "$template_type"

  # Copy text version if provided
  if [[ -n "$txt_file" ]] && [[ -f "$txt_file" ]]; then
    local target_txt="${lang_dir}/${template_type}.txt"
    cp "$txt_file" "$target_txt"
    printf "${GREEN}✓${NC} Uploaded text template: %s\n" "$template_type"
  fi

  # Create or update metadata
  local meta_file="${lang_dir}/${template_type}.json"
  if [[ ! -f "$meta_file" ]]; then
    cat >"$meta_file" <<EOF
{
  "name": "${template_type}",
  "subject": "Email from {{BRAND_NAME}}",
  "description": "Custom template: ${template_type}",
  "variables": [],
  "category": "custom"
}
EOF
    printf "${GREEN}✓${NC} Created metadata file (edit to customize)\n"
  fi

  return 0
}

delete_custom_template() {
  local template_type="$1"
  local language="${2:-$DEFAULT_LANGUAGE}"

  local lang_dir="${TEMPLATES_LANG_DIR}/${language}"

  # Prevent deletion of default templates
  if printf "%s" "$TEMPLATE_TYPES" | grep -qw "$template_type"; then
    printf "${RED}Error: Cannot delete default template: %s${NC}\n" "$template_type" >&2
    printf "  Use 'edit' to restore to original content\n" >&2
    return 1
  fi

  # Create backup before deleting
  local backup_dir="${TEMPLATES_DIR}/backups/$(date +%Y%m%d_%H%M%S)"
  mkdir -p "$backup_dir"

  local deleted_count=0
  for ext in html txt json; do
    local file="${lang_dir}/${template_type}.${ext}"
    if [[ -f "$file" ]]; then
      mv "$file" "$backup_dir/"
      deleted_count=$((deleted_count + 1))
    fi
  done

  if [[ $deleted_count -gt 0 ]]; then
    printf "${GREEN}✓${NC} Deleted template: %s (%s files)\n" "$template_type" "$deleted_count"
    printf "  Backup location: %s\n" "$backup_dir"
  else
    printf "${YELLOW}Template not found: %s${NC}\n" "$template_type"
  fi

  return 0
}

# ============================================================================
# Multi-Tenant Template Isolation
# ============================================================================

get_tenant_templates_dir() {
  local tenant_id="$1"

  if [[ -z "$tenant_id" ]]; then
    printf "${RED}Error: Tenant ID required${NC}\n" >&2
    return 1
  fi

  # Sanitize tenant ID (prevent directory traversal)
  local safe_tenant_id
  safe_tenant_id=$(printf "%s" "$tenant_id" | sed 's/[^a-zA-Z0-9_-]//g')

  printf "%s/tenants/%s/email-templates" "${PROJECT_ROOT}/branding" "$safe_tenant_id"
}

initialize_tenant_templates() {
  local tenant_id="$1"
  local source_language="${2:-$DEFAULT_LANGUAGE}"

  local tenant_dir
  tenant_dir=$(get_tenant_templates_dir "$tenant_id")

  if [[ -z "$tenant_dir" ]]; then
    return 1
  fi

  mkdir -p "$tenant_dir/languages/$source_language"
  mkdir -p "$tenant_dir/previews"

  printf "${CYAN}Initializing templates for tenant: %s${NC}\n" "$tenant_id"

  # Copy default templates to tenant directory
  local default_lang_dir="${TEMPLATES_LANG_DIR}/${source_language}"
  local tenant_lang_dir="$tenant_dir/languages/$source_language"

  if [[ -d "$default_lang_dir" ]]; then
    cp -r "$default_lang_dir"/* "$tenant_lang_dir/"
    printf "${GREEN}✓${NC} Templates copied from default\n"
  else
    # Create new templates
    local old_templates_dir="$TEMPLATES_DIR"
    TEMPLATES_DIR="$tenant_dir"
    TEMPLATES_LANG_DIR="$tenant_dir/languages"

    for template_type in $TEMPLATE_TYPES; do
      create_default_template "$template_type" "$source_language"
    done

    TEMPLATES_DIR="$old_templates_dir"
    TEMPLATES_LANG_DIR="$old_templates_dir/languages"

    printf "${GREEN}✓${NC} New templates created\n"
  fi

  # Create tenant-specific variables reference
  cat >"$tenant_dir/VARIABLES.md" <<EOF
# Email Template Variables - Tenant: $tenant_id

This tenant has isolated email templates. Changes here will not affect other tenants.

$(cat "${TEMPLATES_DIR}/VARIABLES.md")
EOF

  printf "${GREEN}✓${NC} Tenant templates initialized: %s\n" "$tenant_id"
  return 0
}

render_tenant_template() {
  local tenant_id="$1"
  local template_type="$2"
  local language="${3:-$DEFAULT_LANGUAGE}"
  local format="${4:-html}"
  shift 4
  local -a variables=("$@")

  local tenant_dir
  tenant_dir=$(get_tenant_templates_dir "$tenant_id")

  if [[ -z "$tenant_dir" ]]; then
    return 1
  fi

  local template_file="$tenant_dir/languages/${language}/${template_type}.${format}"

  # Fallback to default templates if tenant template doesn't exist
  if [[ ! -f "$template_file" ]]; then
    printf "${YELLOW}Tenant template not found, using default${NC}\n" >&2
    template_file="${TEMPLATES_LANG_DIR}/${language}/${template_type}.${format}"

    if [[ ! -f "$template_file" ]]; then
      printf "${RED}Error: Template not found: %s${NC}\n" "$template_type" >&2
      return 1
    fi
  fi

  # Read and render template
  local template_content
  template_content=$(cat "$template_file")

  # Add tenant-specific default variables
  local tenant_vars=(
    "TENANT_ID=$tenant_id"
    "CURRENT_YEAR=$(date +%Y)"
    "BRAND_NAME=${BRAND_NAME:-${PROJECT_NAME:-nself}}"
    "APP_URL=${APP_URL:-${BASE_DOMAIN:-localhost}}"
    "LOGO_URL=${LOGO_URL:-}"
    "COMPANY_ADDRESS=${COMPANY_ADDRESS:-}"
    "SUPPORT_EMAIL=${SUPPORT_EMAIL:-support@${BASE_DOMAIN:-localhost}}"
    "SUPPORT_URL=${SUPPORT_URL:-${APP_URL:-}/support}"
  )

  # Combine tenant and provided variables
  local all_vars=("${tenant_vars[@]}" "${variables[@]}")

  # Substitute variables
  local rendered
  rendered=$(substitute_template_variables "$template_content" "${all_vars[@]}")

  printf "%s" "$rendered"
}

send_tenant_email() {
  local tenant_id="$1"
  local template_type="$2"
  local recipient_email="$3"
  local language="${4:-$DEFAULT_LANGUAGE}"
  shift 4
  local -a variables=("$@")

  # Validate recipient email
  if [[ ! "$recipient_email" =~ ^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$ ]]; then
    printf "${RED}Error: Invalid email address: %s${NC}\n" "$recipient_email" >&2
    return 1
  fi

  # Get tenant-specific SMTP configuration (if available)
  local tenant_smtp_host="${AUTH_SMTP_HOST:-}"
  local tenant_smtp_port="${AUTH_SMTP_PORT:-587}"
  local tenant_smtp_user="${AUTH_SMTP_USER:-}"
  local tenant_smtp_pass="${AUTH_SMTP_PASS:-}"
  local tenant_smtp_sender="${AUTH_SMTP_SENDER:-noreply@${BASE_DOMAIN:-localhost}}"

  # TODO (v1.0): Load tenant-specific SMTP config from database
  # See: .ai/roadmap/v1.0/deferred-features.md (WHITELABEL-001)
  # For now, use global SMTP config

  if [[ -z "$tenant_smtp_host" ]]; then
    printf "${YELLOW}Warning: SMTP not configured for tenant: %s${NC}\n" "$tenant_id" >&2
    return 1
  fi

  # Render HTML version
  local html_body
  html_body=$(render_tenant_template "$tenant_id" "$template_type" "$language" "html" "${variables[@]}")

  # Render plain text version
  local txt_body
  txt_body=$(render_tenant_template "$tenant_id" "$template_type" "$language" "txt" "${variables[@]}")

  # Get subject (using tenant templates)
  local tenant_dir
  tenant_dir=$(get_tenant_templates_dir "$tenant_id")
  local meta_file="$tenant_dir/languages/${language}/${template_type}.json"

  if [[ ! -f "$meta_file" ]]; then
    meta_file="${TEMPLATES_LANG_DIR}/${language}/${template_type}.json"
  fi

  local subject
  if [[ -f "$meta_file" ]] && command -v jq >/dev/null 2>&1; then
    subject=$(jq -r '.subject' "$meta_file")
    subject=$(substitute_template_variables "$subject" "${variables[@]}")
  else
    subject="Email from ${BRAND_NAME:-nself}"
  fi

  # Create temporary files
  local tmp_html
  tmp_html=$(mktemp)
  local tmp_txt
  tmp_txt=$(mktemp)

  printf "%s" "$html_body" >"$tmp_html"
  printf "%s" "$txt_body" >"$tmp_txt"

  # Send email
  printf "${CYAN}Sending email (Tenant: %s)...${NC}\n" "$tenant_id"

  local result=0
  if command -v docker >/dev/null 2>&1; then
    docker run --rm \
      --network host \
      -v "$tmp_html:/tmp/body.html:ro" \
      -v "$tmp_txt:/tmp/body.txt:ro" \
      boky/swaks \
      --to "$recipient_email" \
      --from "$tenant_smtp_sender" \
      --server "$tenant_smtp_host:$tenant_smtp_port" \
      --auth-user "$tenant_smtp_user" \
      --auth-password "$tenant_smtp_pass" \
      --tls \
      --header "Subject: $subject" \
      --header "X-Tenant-ID: $tenant_id" \
      --body /tmp/body.txt \
      --attach-body /tmp/body.html \
      --timeout 30 2>&1 || result=$?
  else
    printf "${YELLOW}Warning: Docker not available${NC}\n" >&2
    result=1
  fi

  # Cleanup
  rm -f "$tmp_html" "$tmp_txt"

  if [[ $result -eq 0 ]]; then
    printf "${GREEN}✓${NC} Email sent to: %s (Tenant: %s)\n" "$recipient_email" "$tenant_id"
  else
    printf "${RED}✗${NC} Failed to send email\n" >&2
  fi

  return $result
}

list_tenant_templates() {
  local tenant_id="$1"
  local language="${2:-$DEFAULT_LANGUAGE}"

  local tenant_dir
  tenant_dir=$(get_tenant_templates_dir "$tenant_id")

  if [[ -z "$tenant_dir" ]]; then
    return 1
  fi

  printf "${CYAN}Email Templates - Tenant: %s (Language: %s)${NC}\n" "$tenant_id" "$language"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  local lang_dir="$tenant_dir/languages/$language"

  if [[ ! -d "$lang_dir" ]]; then
    printf "${YELLOW}No templates found for tenant: %s${NC}\n" "$tenant_id"
    printf "Initialize with: nself whitelabel email-templates init-tenant %s\n" "$tenant_id"
    return 0
  fi

  local template_count=0
  for template_file in "$lang_dir"/*.json; do
    if [[ -f "$template_file" ]] && command -v jq >/dev/null 2>&1; then
      local template_name
      template_name=$(basename "$template_file" .json)
      local subject
      subject=$(jq -r '.subject' "$template_file")
      local description
      description=$(jq -r '.description' "$template_file")

      printf "${BLUE}%-20s${NC} %s\n" "$template_name" "$description"
      printf "  Subject: %s\n\n" "$subject"
      template_count=$((template_count + 1))
    fi
  done

  printf "Total: %s templates\n" "$template_count"
  return 0
}

# ============================================================================
# Batch Operations
# ============================================================================

validate_all_templates() {
  local language="${1:-$DEFAULT_LANGUAGE}"

  printf "${CYAN}Validating all templates (Language: %s)...${NC}\n" "$language"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  local lang_dir="${TEMPLATES_LANG_DIR}/${language}"
  local errors=0
  local warnings=0
  local validated=0

  for template_type in $TEMPLATE_TYPES; do
    local html_file="${lang_dir}/${template_type}.html"
    local txt_file="${lang_dir}/${template_type}.txt"
    local meta_file="${lang_dir}/${template_type}.json"

    printf "Checking: %s\n" "$template_type"

    # Check HTML template exists
    if [[ ! -f "$html_file" ]]; then
      printf "  ${RED}✗${NC} HTML template missing\n"
      errors=$((errors + 1))
    else
      # Validate HTML content
      if validate_template_content "$html_file" 2>/dev/null; then
        printf "  ${GREEN}✓${NC} HTML template valid\n"
        validated=$((validated + 1))
      else
        printf "  ${RED}✗${NC} HTML template has errors\n"
        errors=$((errors + 1))
      fi
    fi

    # Check text template exists
    if [[ ! -f "$txt_file" ]]; then
      printf "  ${YELLOW}!${NC} Text template missing\n"
      warnings=$((warnings + 1))
    else
      printf "  ${GREEN}✓${NC} Text template present\n"
    fi

    # Check metadata exists
    if [[ ! -f "$meta_file" ]]; then
      printf "  ${YELLOW}!${NC} Metadata missing\n"
      warnings=$((warnings + 1))
    else
      # Validate JSON
      if command -v jq >/dev/null 2>&1 && jq empty "$meta_file" 2>/dev/null; then
        printf "  ${GREEN}✓${NC} Metadata valid\n"
      else
        printf "  ${RED}✗${NC} Metadata invalid JSON\n"
        errors=$((errors + 1))
      fi
    fi

    printf "\n"
  done

  printf "%s\n" "$(printf '%.s=' {1..60})"
  printf "Validation Summary:\n"
  printf "  ${GREEN}Validated: %s${NC}\n" "$validated"
  printf "  ${YELLOW}Warnings: %s${NC}\n" "$warnings"
  printf "  ${RED}Errors: %s${NC}\n" "$errors"

  if [[ $errors -gt 0 ]]; then
    return 1
  fi

  return 0
}

export_all_templates() {
  local language="${1:-$DEFAULT_LANGUAGE}"
  local output_dir="${2:-./email-templates-export}"

  mkdir -p "$output_dir"

  printf "${CYAN}Exporting all templates...${NC}\n"

  local lang_dir="${TEMPLATES_LANG_DIR}/${language}"
  local count=0

  for template_type in $TEMPLATE_TYPES; do
    for ext in html txt json; do
      local src_file="${lang_dir}/${template_type}.${ext}"
      local dst_file="${output_dir}/${template_type}.${ext}"

      if [[ -f "$src_file" ]]; then
        cp "$src_file" "$dst_file"
        count=$((count + 1))
      fi
    done
  done

  # Export variables reference
  cp "${TEMPLATES_DIR}/VARIABLES.md" "${output_dir}/"

  printf "${GREEN}✓${NC} Exported %s template files to: %s\n" "$count" "$output_dir"
  return 0
}

import_all_templates() {
  local source_dir="$1"
  local language="${2:-$DEFAULT_LANGUAGE}"

  if [[ ! -d "$source_dir" ]]; then
    printf "${RED}Error: Source directory not found: %s${NC}\n" "$source_dir" >&2
    return 1
  fi

  printf "${CYAN}Importing templates from: %s${NC}\n" "$source_dir"

  local lang_dir="${TEMPLATES_LANG_DIR}/${language}"
  mkdir -p "$lang_dir"

  # Backup existing templates
  local backup_dir="${TEMPLATES_DIR}/backups/import-$(date +%Y%m%d_%H%M%S)"
  mkdir -p "$backup_dir"

  if [[ -d "$lang_dir" ]]; then
    cp -r "$lang_dir"/* "$backup_dir/" 2>/dev/null || true
    printf "${YELLOW}Backed up existing templates to: %s${NC}\n" "$backup_dir"
  fi

  local count=0
  local errors=0

  for template_file in "$source_dir"/*.html; do
    if [[ -f "$template_file" ]]; then
      local template_name
      template_name=$(basename "$template_file" .html)

      # Validate before importing
      if validate_template_content "$template_file" 2>/dev/null; then
        cp "$template_file" "${lang_dir}/${template_name}.html"
        count=$((count + 1))

        # Copy text and metadata if they exist
        for ext in txt json; do
          local src="${source_dir}/${template_name}.${ext}"
          if [[ -f "$src" ]]; then
            cp "$src" "${lang_dir}/${template_name}.${ext}"
          fi
        done
      else
        printf "${RED}✗${NC} Skipped invalid template: %s\n" "$template_name"
        errors=$((errors + 1))
      fi
    fi
  done

  printf "${GREEN}✓${NC} Imported %s templates\n" "$count"

  if [[ $errors -gt 0 ]]; then
    printf "${YELLOW}Skipped %s invalid templates${NC}\n" "$errors"
  fi

  return 0
}

# ============================================================================
# Template Statistics & Reporting
# ============================================================================

show_template_stats() {
  printf "${CYAN}Email Template System Statistics${NC}\n"
  printf "%s\n\n" "$(printf '%.s=' {1..60})"

  # Count languages
  local lang_count=0
  if [[ -d "$TEMPLATES_LANG_DIR" ]]; then
    lang_count=$(find "$TEMPLATES_LANG_DIR" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')
  fi
  printf "Languages: %s\n" "$lang_count"

  # Count templates per language
  for lang_dir in "$TEMPLATES_LANG_DIR"/*; do
    if [[ -d "$lang_dir" ]]; then
      local lang_code
      lang_code=$(basename "$lang_dir")
      local template_count
      template_count=$(find "$lang_dir" -name "*.html" -type f | wc -l | tr -d ' ')
      printf "  • %s: %s templates\n" "$lang_code" "$template_count"
    fi
  done

  # Count tenant templates
  local tenant_dir="${PROJECT_ROOT}/branding/tenants"
  if [[ -d "$tenant_dir" ]]; then
    local tenant_count
    tenant_count=$(find "$tenant_dir" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')
    printf "\nTenant Templates: %s tenants\n" "$tenant_count"
  fi

  # Preview files
  local preview_count=0
  if [[ -d "$TEMPLATES_PREVIEW_DIR" ]]; then
    preview_count=$(find "$TEMPLATES_PREVIEW_DIR" -type f | wc -l | tr -d ' ')
  fi
  printf "\nPreview Files: %s\n" "$preview_count"

  # Backup files
  local backup_dir="${TEMPLATES_DIR}/backups"
  if [[ -d "$backup_dir" ]]; then
    local backup_count
    backup_count=$(find "$backup_dir" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')
    printf "Backups: %s\n" "$backup_count"
  fi

  printf "\n"
  return 0
}

# Export all functions
export -f html_escape
export -f sanitize_variable_name
export -f validate_template_content
export -f substitute_template_variables
export -f render_template
export -f get_template_subject
export -f send_email_from_template
export -f test_email_template
export -f list_available_languages
export -f copy_templates_to_language
export -f upload_custom_template
export -f delete_custom_template
export -f get_tenant_templates_dir
export -f initialize_tenant_templates
export -f render_tenant_template
export -f send_tenant_email
export -f list_tenant_templates
export -f validate_all_templates
export -f export_all_templates
export -f import_all_templates
export -f show_template_stats
