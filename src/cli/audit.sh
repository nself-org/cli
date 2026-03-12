#!/usr/bin/env bash
# audit.sh - Audit log management CLI
# Provides secure audit trail for compliance and security tracking

set -euo pipefail

# Source dependencies
CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$CLI_SCRIPT_DIR/../lib/utils/audit-logging.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true

# Main command handler
cmd_audit() {
  local subcommand="${1:-help}"

  case "$subcommand" in
    init)
      # Initialize audit logging system
      audit_init
      log_success "Audit logging initialized"
      log_info "Audit logs location: ${NSELF_AUDIT_DIR}/${NSELF_AUDIT_FILE}"
      ;;

    query)
      # Query audit logs
      shift
      local category="${1:-}"
      local action="${2:-}"
      local user="${3:-}"
      local since="${4:-}"

      audit_query "$category" "$action" "$user" "$since"
      ;;

    stats)
      # Show audit statistics
      audit_stats
      ;;

    export)
      # Export audit logs
      shift
      local output_file="${1:-audit-export.txt}"
      local format="${2:-txt}"

      audit_export "$output_file" "$format"
      ;;

    verify)
      # Verify audit log integrity
      audit_verify
      ;;

    enable)
      # Enable audit logging
      export NSELF_AUDIT_ENABLED=true
      log_success "Audit logging enabled"
      log_info "Set NSELF_AUDIT_ENABLED=true in your .env file to persist"
      ;;

    disable)
      # Disable audit logging (with warning)
      log_warning "Disabling audit logging"
      log_warning "This may impact compliance requirements"
      read -p "Are you sure? (yes/no): " -r confirm
      if [[ "$confirm" == "yes" ]]; then
        export NSELF_AUDIT_ENABLED=false
        log_info "Audit logging disabled"
        log_info "Set NSELF_AUDIT_ENABLED=false in your .env file to persist"
      else
        log_info "Audit logging remains enabled"
      fi
      ;;

    security)
      # Source security audit functions
      if [[ -f "$CLI_SCRIPT_DIR/../lib/security/audit-checks.sh" ]]; then
        source "$CLI_SCRIPT_DIR/../lib/security/audit-checks.sh"
      else
        log_error "Security audit module not found"
        exit 1
      fi

      shift || true
      local check_type="${1:-all}"

      case "$check_type" in
        all)
          run_security_audit
          ;;
        secrets)
          check_weak_secrets
          ;;
        cors)
          check_cors_security
          ;;
        ports)
          check_exposed_ports
          ;;
        containers)
          check_container_users
          ;;
        *)
          log_error "Unknown security check: $check_type"
          printf "Options: all, secrets, cors, ports, containers\n"
          exit 1
          ;;
      esac
      ;;

    help | --help | -h)
      show_help
      ;;

    *)
      log_error "Unknown command: $subcommand"
      log_info "Use 'nself audit help' for usage information"
      return 1
      ;;
  esac
}

# Show help
show_help() {
  cat <<'HELP'
nself audit - Audit log management and compliance tracking

USAGE:
  nself audit <command> [options]

COMMANDS:
  init                          Initialize audit logging system
  query [category] [action]     Query audit logs with filters
  stats                         Show audit trail statistics
  export <file> [format]        Export audit logs (formats: txt, csv, json)
  verify                        Verify audit log integrity
  security [check]              Run security audits (checks: all, secrets, cors, ports, containers)
  enable                        Enable audit logging
  disable                       Disable audit logging (requires confirmation)
  help                          Show this help message

AUDIT CATEGORIES:
  AUTH        - Authentication and authorization events
  CONFIG      - Configuration changes
  DEPLOY      - Deployment actions
  SECRET      - Secret operations (access, rotation)
  SECURITY    - Security events and violations
  DATA        - Data operations
  ADMIN       - Administrative actions
  SYSTEM      - System events

EXAMPLES:
  # Initialize audit logging
  nself audit init

  # View all audit events
  nself audit query

  # View authentication events
  nself audit query AUTH

  # View deployment actions
  nself audit query DEPLOY

  # Show statistics
  nself audit stats

  # Run security audit
  nself audit security                 # All checks
  nself audit security secrets         # Check weak secrets
  nself audit security cors            # Check CORS config
  nself audit security ports           # Check exposed ports
  nself audit security containers      # Check container users

  # Export to CSV
  nself audit export audit-trail.csv csv

  # Export to JSON
  nself audit export audit-trail.json json

  # Verify integrity
  nself audit verify

CONFIGURATION:
  Environment variables:
    NSELF_AUDIT_DIR        - Audit log directory (default: ~/.nself/audit)
    NSELF_AUDIT_FILE       - Audit log filename (default: audit.log)
    NSELF_AUDIT_ENABLED    - Enable/disable audit logging (default: true)

SECURITY NOTES:
  • Audit logs are append-only and tamper-resistant
  • Each entry includes integrity checksum
  • File permissions are restricted to owner only
  • Sensitive data is never logged
  • On Linux, append-only attribute is set when possible

COMPLIANCE:
  Audit logs support compliance requirements for:
    - SOC 2 Type II
    - HIPAA
    - PCI-DSS
    - GDPR (with proper data handling)
    - ISO 27001

For more information: https://docs.nself.org/security/audit-logging

HELP
}

export -f cmd_audit
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then cmd_audit "$@"; fi
