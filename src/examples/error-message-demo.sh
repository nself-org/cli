#!/usr/bin/env bash
set -euo pipefail

# error-message-demo.sh - Demonstration of improved error messages
# Shows before/after comparison of error handling

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source the error messages library
source "$SCRIPT_DIR/../lib/utils/error-messages.sh"

printf "\n"
printf "========================================\n"
printf "  nself Error Messages - Demonstration\n"
printf "========================================\n"
printf "\n"

printf "This demo shows the new improved error messages.\n"
printf "\n"

# Demo 1: Port Conflict
printf "\n"
printf "=== Example 1: Port Conflict ===\n"
printf "\n"
show_port_conflict_error 5432 "postgres" "PostgreSQL 14.1"

# Demo 2: Container Failed
printf "\n"
printf "=== Example 2: Container Startup Failure ===\n"
printf "\n"
show_container_failed_error "hasura" "Connection to database failed" "Error: connection refused at localhost:5432"

# Demo 3: Missing Config
printf "\n"
printf "=== Example 3: Missing Configuration ===\n"
printf "\n"
show_config_missing_error ".env" "PROJECT_NAME POSTGRES_PASSWORD HASURA_GRAPHQL_ADMIN_SECRET"

# Demo 4: Error Reference
printf "\n"
printf "=== Example 4: Quick Reference Guide ===\n"
printf "\n"
show_error_reference

printf "\n"
printf "========================================\n"
printf "  Key Features\n"
printf "========================================\n"
printf "\n"
printf "✓ Clear problem statements\n"
printf "✓ Specific reasons for failure\n"
printf "✓ Numbered, actionable solutions\n"
printf "✓ Copy-paste ready commands\n"
printf "✓ Platform-specific guidance\n"
printf "✓ Links to further help\n"
printf "\n"
