# UX Standards Quick Reference for nself Developers

Quick reference for implementing consistent UX across nself commands.

## 1-Minute Setup

```bash
# Source the UX library in your command file
source "$CLI_SCRIPT_DIR/../lib/utils/ux-standards.sh" 2>/dev/null || true
```

## Common Patterns (Copy & Paste)

### Pattern 1: Show Actionable Error

```bash
# Missing file
ux_error_file_not_found ".env" "Run 'nself init' to create it"

# Docker not running
ux_error_docker_not_running

# Config missing
ux_error_config_missing

# Port in use
ux_error_port_in_use 5432 "postgres"

# Service failed
ux_error_service_failed "postgres" "Port already in use"

# Invalid input
ux_error_invalid_input "$port" "valid port (1-65535)" "3000, 8080, 5432"
```

### Pattern 2: Validate Input

```bash
# At the start of your command function:
ux_validate_docker || return 1
ux_validate_file_exists "docker-compose.yml" "Run 'nself build'" || return 1

# Validate specific inputs
ux_validate_required "$arg" "argument name" "example value" || return 1
ux_validate_port "$port" || return 1
ux_validate_env "$environment" || return 1
```

### Pattern 3: Show Progress

```bash
# Multi-step operations
ux_progress_init
ux_progress_add "Validating configuration"
ux_progress_add "Building Docker images"
ux_progress_add "Starting services"

# Execute each step
ux_progress_update 0 "running"
# do validation
ux_progress_update 0 "done" "Config valid"

ux_progress_update 1 "running"
# build images
ux_progress_update 1 "done" "Images built"

# Or use spinner for single long operation
ux_spinner_start "Downloading large file"
# download operation
ux_spinner_stop "" "Download complete"
```

### Pattern 4: Standardized Help

```bash
show_help() {
  ux_help_header "nself mycommand" "Short description"

  ux_help_section "Usage"
  printf "  nself mycommand [OPTIONS]\n\n"

  ux_help_section "Options"
  ux_help_option "-f, --force" "Force operation"
  ux_help_option "-v, --verbose" "Show detailed output"
  printf "\n"

  ux_help_section "Examples"
  ux_help_example "Basic usage" "nself mycommand"
  ux_help_example "Force mode" "nself mycommand --force"

  ux_help_section "See Also"
  printf "  ${COLOR_DIM}nself help${COLOR_RESET}\n\n"
}
```

## Error Message Checklist

Every error should have:

✓ **Problem** - What went wrong (user-friendly, not technical)
✓ **Fix** - How to resolve it (actionable command or steps)
✓ **Context** - Relevant details (file paths, service names, etc.)

**Example:**
```
✗ Problem: Port 5432 is already in use
Context: Service: postgres
Fix: Stop conflicting process: lsof -ti:5432 | xargs kill -9
```

## Input Validation Checklist

Before doing ANY operation, validate:

- [ ] Required arguments provided
- [ ] Docker is running (if needed)
- [ ] Configuration files exist
- [ ] Port numbers are valid
- [ ] Environment names are valid
- [ ] File paths are accessible

## Progress Indicators Decision Tree

**Q: Do you know how many steps?**
- Yes → Use `ux_progress_*` functions
- No → Use `ux_spinner_*` functions

**Q: Can you show percentage?**
- Yes → Future: Use `ux_progress_bar` (not yet implemented)
- No → Use spinner or step-by-step

## Color Usage Guide

| Use Case | Color | Function |
|----------|-------|----------|
| Success message | Green | `${COLOR_GREEN}✓${COLOR_RESET} Success` |
| Error message | Red | `${COLOR_RED}✗${COLOR_RESET} Error` |
| Warning | Yellow | `${COLOR_YELLOW}⚠${COLOR_RESET} Warning` |
| Info | Blue | `${COLOR_BLUE}ℹ${COLOR_RESET} Info` |
| Hint/Suggestion | Cyan | `${COLOR_CYAN}→${COLOR_RESET} Tip` |
| Dimmed text | Gray | `${COLOR_DIM}secondary info${COLOR_RESET}` |

## Testing Your Changes

### Manual Test
```bash
# Run your command with invalid input - should show helpful error
nself mycommand --invalid

# Run with --help - should show standardized help
nself mycommand --help

# Run actual operation - should show progress
nself mycommand
```

### Check Output
- [ ] Errors are helpful and actionable
- [ ] Colors display correctly in terminal
- [ ] No ANSI codes when piped: `nself mycommand | cat`
- [ ] Progress indicators work
- [ ] Help text is clear and complete

## Common Mistakes to Avoid

❌ **Don't use echo for errors**
```bash
echo "Error: File not found"  # Bad
```

✅ **Use UX functions**
```bash
ux_error_file_not_found "$file" "suggestion"  # Good
```

---

❌ **Don't use generic errors**
```bash
if [[ $? -ne 0 ]]; then
  echo "Something went wrong"  # Bad - not actionable
fi
```

✅ **Provide context and fix**
```bash
if [[ $? -ne 0 ]]; then
  ux_error_service_failed "postgres" "Check logs: nself logs postgres"  # Good
fi
```

---

❌ **Don't skip validation**
```bash
docker compose up  # Bad - might fail with cryptic error
```

✅ **Validate first**
```bash
ux_validate_docker || return 1  # Good - clear error message
docker compose up
```

## Full Example Command

```bash
#!/usr/bin/env bash
set -euo pipefail

# Source utilities
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$CLI_SCRIPT_DIR/../lib/utils/ux-standards.sh" 2>/dev/null || true

cmd_mycommand() {
  local force=false
  local service=""

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -f|--force)
        force=true
        shift
        ;;
      -h|--help)
        show_help
        return 0
        ;;
      -*)
        ux_error_invalid_input "$1" "valid option" "--force, --help"
        return 1
        ;;
      *)
        service="$1"
        shift
        ;;
    esac
  done

  # Validate inputs
  ux_validate_required "$service" "service name" "postgres" || return 1
  ux_validate_docker || return 1

  # Show progress
  ux_progress_init
  ux_progress_add "Validating service exists"
  ux_progress_add "Performing operation"

  # Step 1: Validate
  ux_progress_update 0 "running"
  if ! docker ps --format "{{.Names}}" | grep -q "$service"; then
    ux_progress_update 0 "error" "Service not found"
    ux_error_service_not_running "$service"
    return 1
  fi
  ux_progress_update 0 "done"

  # Step 2: Operate
  ux_progress_update 1 "running"
  if docker restart "$service" >/dev/null 2>&1; then
    ux_progress_update 1 "done"
    ux_success "Service restarted" "Check status: nself status"
  else
    ux_progress_update 1 "error" "Restart failed"
    ux_error_service_failed "$service" "See logs: nself logs $service"
    return 1
  fi

  return 0
}

show_help() {
  ux_help_header "nself mycommand" "Perform operation on service"

  ux_help_section "Usage"
  printf "  nself mycommand [OPTIONS] <service>\n\n"

  ux_help_section "Options"
  ux_help_option "-f, --force" "Force operation"
  ux_help_option "-h, --help" "Show help"
  printf "\n"

  ux_help_section "Examples"
  ux_help_example "Basic usage" "nself mycommand postgres"
  ux_help_example "Force mode" "nself mycommand --force postgres"
}

export -f cmd_mycommand
cmd_mycommand "$@"
```

## Before & After Examples

### Example 1: File Not Found

**Before:**
```bash
if [[ ! -f ".env" ]]; then
  echo "Error: .env not found"
  exit 1
fi
```

**After:**
```bash
ux_validate_file_exists ".env" "Run 'nself init' to create it" || return 1
```

**Output:**
```
✗ Problem: File not found: .env
Context: Current directory: /path/to/project
Fix: Run 'nself init' to create it
```

### Example 2: Invalid Port

**Before:**
```bash
if [[ "$port" -lt 1 ]] || [[ "$port" -gt 65535 ]]; then
  echo "Invalid port: $port"
  exit 1
fi
```

**After:**
```bash
ux_validate_port "$port" || return 1
```

**Output:**
```
✗ Problem: Invalid input: '99999'
Context: Expected: port between 1 and 65535. Example: Common ports: 3000, 8080, 8000
Fix: Provide port between 1 and 65535
```

### Example 3: Long Operation

**Before:**
```bash
echo "Starting services..."
docker compose up -d
echo "Done"
```

**After:**
```bash
ux_spinner_start "Starting services"
docker compose up -d >/dev/null 2>&1
ux_spinner_stop "" "All services started"
```

**Output:**
```
  ⠋ Starting services...
  ✓ All services started
```

## FAQ

**Q: When should I use `return 1` vs `exit 1`?**
A: Use `return 1` in functions (so tests can catch it). Use `exit 1` only in the main script.

**Q: Do I need to handle NO_COLOR environment variable?**
A: No, the UX library handles it automatically.

**Q: What if the user pipes output?**
A: The library detects non-terminal output and adjusts. Always use the UX functions.

**Q: Can I customize error messages?**
A: Yes! Use `ux_error` for custom messages, or extend the pre-built functions.

**Q: How do I test progress indicators?**
A: Run in a real terminal. Automated tests should check for `✓` and step names in output.

## Resources

- Full documentation: `/docs/development/UX-IMPROVEMENTS-V0.9.8.md`
- Source code: `/src/lib/utils/ux-standards.sh`
- Examples: See `stop.sh`, `start.sh` for reference implementations
- Testing: `/src/tests/unit/test-ux-standards.sh`

---

**Remember:** Good UX = Clear errors + Progress feedback + Helpful examples

**When in doubt:** Ask yourself: "If I were a new user, would this error help me fix the problem?"
