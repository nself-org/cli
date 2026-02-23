#!/usr/bin/env bash
# mfa.sh - MFA CLI commands (MFA-007)
# Part of nself v0.6.0 - Phase 1 Sprint 2
#
# Command-line interface for multi-factor authentication management

set -euo pipefail

# Get script directory and nself root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Source required libraries
if [[ -z "${EXIT_SUCCESS:-}" ]]; then
  source "$NSELF_ROOT/src/lib/config/constants.sh" 2>/dev/null || true
fi
source "$NSELF_ROOT/src/lib/utils/display.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/auth/mfa/totp.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/auth/mfa/sms.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/auth/mfa/email.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/auth/mfa/backup-codes.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/auth/mfa/policies.sh" 2>/dev/null || true

# ============================================================================
# MFA CLI Commands
# ============================================================================

# Main MFA command handler
cmd_mfa() {
  local subcommand="${1:-}"

  if [[ -z "$subcommand" ]]; then
    cmd_mfa_help
    return 0
  fi

  shift

  case "$subcommand" in
    enable)
      cmd_mfa_enable "$@"
      ;;
    disable)
      cmd_mfa_disable "$@"
      ;;
    verify)
      cmd_mfa_verify "$@"
      ;;
    status)
      cmd_mfa_status "$@"
      ;;
    backup-codes)
      cmd_mfa_backup_codes "$@"
      ;;
    policy)
      cmd_mfa_policy "$@"
      ;;
    methods)
      cmd_mfa_methods "$@"
      ;;
    help|--help|-h)
      cmd_mfa_help
      ;;
    *)
      echo "ERROR: Unknown MFA command: $subcommand" >&2
      cmd_mfa_help
      return 1
      ;;
  esac
}

# ============================================================================
# Enable MFA
# ============================================================================

cmd_mfa_enable() {
  local method=""
  local user_id=""
  local email=""
  local phone=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --method=*)
        method="${1#*=}"
        shift
        ;;
      --user=*|--user-id=*)
        user_id="${1#*=}"
        shift
        ;;
      --email=*)
        email="${1#*=}"
        shift
        ;;
      --phone=*)
        phone="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  # Validate inputs
  if [[ -z "$method" ]]; then
    echo "ERROR: MFA method required. Use --method=totp|sms|email" >&2
    return 1
  fi

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required. Use --user=<user_id>" >&2
    return 1
  fi

  # Enable MFA method
  case "$method" in
    totp)
      cmd_mfa_enable_totp "$user_id" "$email"
      ;;
    sms)
      if [[ -z "$phone" ]]; then
        echo "ERROR: Phone number required for SMS MFA. Use --phone=<phone>" >&2
        return 1
      fi
      cmd_mfa_enable_sms "$user_id" "$phone"
      ;;
    email)
      if [[ -z "$email" ]]; then
        echo "ERROR: Email required for email MFA. Use --email=<email>" >&2
        return 1
      fi
      cmd_mfa_enable_email "$user_id" "$email"
      ;;
    *)
      echo "ERROR: Unknown MFA method: $method" >&2
      echo "Available methods: totp, sms, email" >&2
      return 1
      ;;
  esac
}

# Enable TOTP MFA
cmd_mfa_enable_totp() {
  local user_id="$1"
  local email="${2:-user@example.com}"

  print_info "Enrolling TOTP MFA for user: $user_id"

  # Enroll user in TOTP
  local enrollment_data
  enrollment_data=$(totp_enroll "$user_id" "$email" 2>&1)

  if [[ $? -ne 0 ]]; then
    print_error "Failed to enroll TOTP MFA"
    echo "$enrollment_data" >&2
    return 1
  fi

  # Parse enrollment data
  local secret uri qr_command
  secret=$(echo "$enrollment_data" | jq -r '.secret')
  uri=$(echo "$enrollment_data" | jq -r '.uri')
  qr_command=$(echo "$enrollment_data" | jq -r '.qr_command')

  print_success "TOTP MFA enrolled successfully"
  echo ""
  print_info "Secret (for manual entry): $secret"
  echo ""
  print_info "Scan this QR code with your authenticator app:"
  echo "  $qr_command"
  echo ""
  print_warning "Save your secret in a safe place!"
  echo ""
  print_info "To complete enrollment, verify with a code:"
  echo "  nself mfa verify --method=totp --user=$user_id --code=<6-digit-code>"

  return 0
}

# Enable SMS MFA
cmd_mfa_enable_sms() {
  local user_id="$1"
  local phone="$2"

  print_info "Enrolling SMS MFA for user: $user_id"

  # Enroll phone number
  if ! sms_enroll "$user_id" "$phone"; then
    print_error "Failed to enroll SMS MFA"
    return 1
  fi

  print_success "SMS MFA enrolled successfully"
  echo ""
  print_info "To complete enrollment, request a verification code:"
  echo "  nself mfa verify --method=sms --user=$user_id --send"

  return 0
}

# Enable email MFA
cmd_mfa_enable_email() {
  local user_id="$1"
  local email="$2"

  print_info "Enrolling email MFA for user: $user_id"

  # Enroll email
  if ! email_mfa_enroll "$user_id" "$email"; then
    print_error "Failed to enroll email MFA"
    return 1
  fi

  print_success "Email MFA enrolled successfully"
  echo ""
  print_info "To complete enrollment, request a verification code:"
  echo "  nself mfa verify --method=email --user=$user_id --send"

  return 0
}

# ============================================================================
# Verify MFA
# ============================================================================

cmd_mfa_verify() {
  local method=""
  local user_id=""
  local code=""
  local send=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --method=*)
        method="${1#*=}"
        shift
        ;;
      --user=*|--user-id=*)
        user_id="${1#*=}"
        shift
        ;;
      --code=*)
        code="${1#*=}"
        shift
        ;;
      --send)
        send=true
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  # Validate inputs
  if [[ -z "$method" ]]; then
    echo "ERROR: MFA method required. Use --method=totp|sms|email" >&2
    return 1
  fi

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required. Use --user=<user_id>" >&2
    return 1
  fi

  # Verify MFA method
  case "$method" in
    totp)
      cmd_mfa_verify_totp "$user_id" "$code"
      ;;
    sms)
      if $send; then
        cmd_mfa_send_sms "$user_id"
      else
        cmd_mfa_verify_sms "$user_id" "$code"
      fi
      ;;
    email)
      if $send; then
        cmd_mfa_send_email "$user_id"
      else
        cmd_mfa_verify_email "$user_id" "$code"
      fi
      ;;
    backup)
      cmd_mfa_verify_backup "$user_id" "$code"
      ;;
    *)
      echo "ERROR: Unknown MFA method: $method" >&2
      return 1
      ;;
  esac
}

# Verify TOTP code
cmd_mfa_verify_totp() {
  local user_id="$1"
  local code="$2"

  if [[ -z "$code" ]]; then
    echo "ERROR: Code required. Use --code=<6-digit-code>" >&2
    return 1
  fi

  print_info "Verifying TOTP code..."

  if totp_verify_enrollment "$user_id" "$code"; then
    print_success "TOTP MFA enabled successfully"
    return 0
  else
    print_error "Invalid TOTP code"
    return 1
  fi
}

# Send SMS verification code
cmd_mfa_send_sms() {
  local user_id="$1"

  print_info "Sending SMS verification code..."

  if sms_send_verification "$user_id"; then
    print_success "Verification code sent"
    echo ""
    print_info "Verify with: nself mfa verify --method=sms --user=$user_id --code=<6-digit-code>"
    return 0
  else
    print_error "Failed to send SMS"
    return 1
  fi
}

# Verify SMS code
cmd_mfa_verify_sms() {
  local user_id="$1"
  local code="$2"

  if [[ -z "$code" ]]; then
    echo "ERROR: Code required. Use --code=<6-digit-code>" >&2
    return 1
  fi

  print_info "Verifying SMS code..."

  if sms_verify "$user_id" "$code"; then
    print_success "SMS MFA enabled successfully"
    return 0
  else
    print_error "Invalid SMS code"
    return 1
  fi
}

# Send email verification code
cmd_mfa_send_email() {
  local user_id="$1"

  print_info "Sending email verification code..."

  if email_send_verification "$user_id"; then
    print_success "Verification code sent"
    echo ""
    print_info "Verify with: nself mfa verify --method=email --user=$user_id --code=<6-digit-code>"
    return 0
  else
    print_error "Failed to send email"
    return 1
  fi
}

# Verify email code
cmd_mfa_verify_email() {
  local user_id="$1"
  local code="$2"

  if [[ -z "$code" ]]; then
    echo "ERROR: Code required. Use --code=<6-digit-code>" >&2
    return 1
  fi

  print_info "Verifying email code..."

  if email_verify "$user_id" "$code"; then
    print_success "Email MFA enabled successfully"
    return 0
  else
    print_error "Invalid email code"
    return 1
  fi
}

# Verify backup code
cmd_mfa_verify_backup() {
  local user_id="$1"
  local code="$2"

  if [[ -z "$code" ]]; then
    echo "ERROR: Backup code required. Use --code=<backup-code>" >&2
    return 1
  fi

  print_info "Verifying backup code..."

  if backup_code_verify "$user_id" "$code"; then
    print_success "Backup code verified"
    return 0
  else
    print_error "Invalid or used backup code"
    return 1
  fi
}

# ============================================================================
# Disable MFA
# ============================================================================

cmd_mfa_disable() {
  local method=""
  local user_id=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --method=*)
        method="${1#*=}"
        shift
        ;;
      --user=*|--user-id=*)
        user_id="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  # Validate inputs
  if [[ -z "$method" ]]; then
    echo "ERROR: MFA method required. Use --method=totp|sms|email" >&2
    return 1
  fi

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required. Use --user=<user_id>" >&2
    return 1
  fi

  # Disable MFA method
  case "$method" in
    totp)
      totp_disable "$user_id"
      ;;
    sms)
      sms_disable "$user_id"
      ;;
    email)
      email_mfa_disable "$user_id"
      ;;
    *)
      echo "ERROR: Unknown MFA method: $method" >&2
      return 1
      ;;
  esac

  print_success "MFA method '$method' disabled"
  return 0
}

# ============================================================================
# MFA Status
# ============================================================================

cmd_mfa_status() {
  local user_id=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --user=*|--user-id=*)
        user_id="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required. Use --user=<user_id>" >&2
    return 1
  fi

  print_info "MFA status for user: $user_id"
  echo ""

  # Get MFA status
  local status_json
  status_json=$(mfa_get_user_status "$user_id")

  if [[ $? -ne 0 ]] || [[ -z "$status_json" ]]; then
    print_error "Failed to get MFA status"
    return 1
  fi

  # Parse status
  local enabled required exempt
  local totp sms email backup_codes

  enabled=$(echo "$status_json" | jq -r '.enabled')
  required=$(echo "$status_json" | jq -r '.required')
  exempt=$(echo "$status_json" | jq -r '.exempt')
  totp=$(echo "$status_json" | jq -r '.methods.totp')
  sms=$(echo "$status_json" | jq -r '.methods.sms')
  email=$(echo "$status_json" | jq -r '.methods.email')
  backup_codes=$(echo "$status_json" | jq -r '.backup_codes_remaining')

  # Display status
  echo "  MFA Enabled: $enabled"
  echo "  MFA Required: $required"
  echo "  Exempt: $exempt"
  echo ""
  echo "  Enabled Methods:"
  echo "    TOTP: $totp"
  echo "    SMS: $sms"
  echo "    Email: $email"
  echo ""
  echo "  Backup Codes Remaining: $backup_codes"

  return 0
}

# ============================================================================
# Backup Codes
# ============================================================================

cmd_mfa_backup_codes() {
  local action="${1:-}"
  local user_id=""

  if [[ -z "$action" ]]; then
    echo "ERROR: Action required. Use: generate, list, status" >&2
    return 1
  fi

  shift

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --user=*|--user-id=*)
        user_id="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required. Use --user=<user_id>" >&2
    return 1
  fi

  case "$action" in
    generate)
      cmd_mfa_backup_codes_generate "$user_id"
      ;;
    list)
      cmd_mfa_backup_codes_list "$user_id"
      ;;
    status)
      cmd_mfa_backup_codes_status "$user_id"
      ;;
    *)
      echo "ERROR: Unknown action: $action" >&2
      return 1
      ;;
  esac
}

cmd_mfa_backup_codes_generate() {
  local user_id="$1"

  print_info "Generating backup codes for user: $user_id"

  local codes_json
  codes_json=$(backup_codes_create "$user_id" 2>&1)

  if [[ $? -ne 0 ]]; then
    print_error "Failed to generate backup codes"
    echo "$codes_json" >&2
    return 1
  fi

  print_success "Backup codes generated"
  echo ""
  print_warning "Store these codes in a safe place. Each code can only be used once."
  echo ""

  # Display codes
  local codes
  codes=$(echo "$codes_json" | jq -r '.codes[]')

  while IFS= read -r code; do
    echo "  $code"
  done <<< "$codes"

  return 0
}

cmd_mfa_backup_codes_list() {
  local user_id="$1"

  print_info "Backup codes for user: $user_id"

  local list_json
  list_json=$(backup_codes_list "$user_id")

  echo "$list_json" | jq '.'

  return 0
}

cmd_mfa_backup_codes_status() {
  local user_id="$1"

  local status_json
  status_json=$(backup_codes_status "$user_id")

  local unused used total
  unused=$(echo "$status_json" | jq -r '.unused')
  used=$(echo "$status_json" | jq -r '.used')
  total=$(echo "$status_json" | jq -r '.total')

  print_info "Backup codes status:"
  echo "  Unused: $unused"
  echo "  Used: $used"
  echo "  Total: $total"

  return 0
}

# ============================================================================
# MFA Policy Management
# ============================================================================

cmd_mfa_policy() {
  local action="${1:-}"

  if [[ -z "$action" ]]; then
    echo "ERROR: Action required. Use: set, get" >&2
    return 1
  fi

  shift

  case "$action" in
    set)
      cmd_mfa_policy_set "$@"
      ;;
    get)
      cmd_mfa_policy_get "$@"
      ;;
    *)
      echo "ERROR: Unknown action: $action" >&2
      return 1
      ;;
  esac
}

cmd_mfa_policy_set() {
  local policy_type="optional"

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --type=*)
        policy_type="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  print_info "Setting global MFA policy to: $policy_type"

  if mfa_policy_set_global "$policy_type"; then
    print_success "MFA policy updated"
    return 0
  else
    print_error "Failed to update MFA policy"
    return 1
  fi
}

cmd_mfa_policy_get() {
  local policy_json
  policy_json=$(mfa_policy_get_global)

  echo "$policy_json" | jq '.'

  return 0
}

# ============================================================================
# List MFA Methods
# ============================================================================

cmd_mfa_methods() {
  print_info "Available MFA methods:"
  echo ""
  echo "  totp     - Time-based One-Time Password (Google Authenticator, Authy, etc.)"
  echo "  sms      - SMS verification code"
  echo "  email    - Email verification code"
  echo "  backup   - One-time backup codes"
  echo ""
  print_info "Usage examples:"
  echo "  nself mfa enable --method=totp --user=<user_id> --email=<email>"
  echo "  nself mfa enable --method=sms --user=<user_id> --phone=<phone>"
  echo "  nself mfa verify --method=totp --user=<user_id> --code=<code>"
  echo "  nself mfa status --user=<user_id>"
  echo "  nself mfa backup-codes generate --user=<user_id>"

  return 0
}

# ============================================================================
# Help
# ============================================================================

cmd_mfa_help() {
  cat <<EOF
nself mfa - Multi-Factor Authentication management

USAGE:
  nself mfa <command> [options]

COMMANDS:
  enable          Enable MFA for a user
  disable         Disable MFA for a user
  verify          Verify MFA code
  status          Show MFA status for a user
  backup-codes    Manage backup codes
  policy          Manage MFA policies
  methods         List available MFA methods
  help            Show this help message

EXAMPLES:
  # Enable TOTP MFA
  nself mfa enable --method=totp --user=<user_id> --email=<email>

  # Verify TOTP enrollment
  nself mfa verify --method=totp --user=<user_id> --code=<6-digit-code>

  # Enable SMS MFA
  nself mfa enable --method=sms --user=<user_id> --phone=+1234567890

  # Send SMS verification code
  nself mfa verify --method=sms --user=<user_id> --send

  # Verify SMS code
  nself mfa verify --method=sms --user=<user_id> --code=<6-digit-code>

  # Check MFA status
  nself mfa status --user=<user_id>

  # Generate backup codes
  nself mfa backup-codes generate --user=<user_id>

  # Set MFA policy
  nself mfa policy set --type=required

For more information, visit: https://docs.nself.org/auth/mfa
EOF
}

# Export main command
export -f cmd_mfa

# Run command if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_mfa "$@"
fi
