# CLI Output Library

Standardized output formatting for nself CLI commands.

## Overview

The `cli-output.sh` library provides consistent, cross-platform CLI output formatting with:

- **Bash 3.2+ compatibility** - Works on macOS, Linux, WSL
- **NO_COLOR support** - Respects user preferences
- **CI/TTY detection** - Adapts to terminal vs. non-interactive environments
- **Comprehensive API** - 40+ functions for all output needs
- **Zero dependencies** - Pure Bash, no external tools required

## Quick Start

```bash
# Source the library
source "src/lib/utils/cli-output.sh"

# Basic usage
cli_success "Operation completed successfully"
cli_error "Failed to connect to database"
cli_warning "Port 8080 is already in use"
cli_info "Loading configuration..."
```

## Installation

The library is automatically available in all nself CLI commands:

```bash
# In any CLI command file
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../lib/utils/cli-output.sh"
```

## API Reference

### Basic Messages

#### `cli_success "message"`
Print a success message with checkmark icon.

```bash
cli_success "Database connection established"
# Output: ✓ Database connection established (in green)
```

#### `cli_error "message"`
Print an error message with cross icon. Outputs to stderr.

```bash
cli_error "Failed to read configuration file"
# Output: ✗ Failed to read configuration file (in red, to stderr)
```

#### `cli_warning "message"`
Print a warning message with warning icon. Outputs to stderr.

```bash
cli_warning "Configuration file not found, using defaults"
# Output: ⚠ Configuration file not found, using defaults (in yellow, to stderr)
```

#### `cli_info "message"`
Print an informational message with info icon.

```bash
cli_info "Scanning for services..."
# Output: ℹ Scanning for services... (in blue)
```

#### `cli_debug "message"`
Print a debug message (only when `DEBUG=true`).

```bash
DEBUG=true cli_debug "Variable value: $var"
# Output: [DEBUG] Variable value: example (in magenta)
```

#### `cli_message "message"`
Print a plain message without icons or colors.

```bash
cli_message "Hello, world"
# Output: Hello, world
```

#### `cli_bold "message"`
Print a bold message.

```bash
cli_bold "Important Notice"
# Output: Important Notice (bold)
```

#### `cli_dim "message"`
Print a dimmed/subtle message.

```bash
cli_dim "Additional context information"
# Output: Additional context information (dimmed)
```

---

### Sections and Headers

#### `cli_section "title"`
Print a section header with arrow.

```bash
cli_section "Database Configuration"
# Output:
# → Database Configuration (bold, with spacing)
```

#### `cli_header "title"`
Print a major section header with double-line box.

```bash
cli_header "Build Process"
# Output:
# ════════════════════════════════════════════════════════════
#                         Build Process
# ════════════════════════════════════════════════════════════
```

#### `cli_step current total "message"`
Print a step indicator for multi-step processes.

```bash
cli_step 1 5 "Installing dependencies"
cli_step 2 5 "Running tests"
# Output:
# ⚙ Step 1/5 ─ Installing dependencies
# ⚙ Step 2/5 ─ Running tests
```

---

### Boxes

#### `cli_box "message" [type]`
Draw a simple box around text.

**Types:** `info` (default), `success`, `error`, `warning`

```bash
cli_box "Build completed successfully" "success"
# Output:
# ┌──────────────────────────────────┐
# │  Build completed successfully  │
# └──────────────────────────────────┘
```

#### `cli_box_detailed "title" "content"`
Draw an enhanced box with title and word-wrapped content.

```bash
cli_box_detailed "Important Notice" "This is a longer message that will be wrapped properly within the box boundaries to fit the standard 60-character width."
# Output:
# ╔══════════════════════════════════════════════════════════╗
# ║                    Important Notice                      ║
# ╟──────────────────────────────────────────────────────────╢
# ║ This is a longer message that will be wrapped properly   ║
# ║ within the box boundaries to fit the standard            ║
# ║ 60-character width.                                      ║
# ╚══════════════════════════════════════════════════════════╝
```

---

### Tables

Tables automatically calculate column widths based on headers.

#### `cli_table_header "Col1" "Col2" "Col3"`
Print table header with column names.

#### `cli_table_row "val1" "val2" "val3"`
Print table row with values.

#### `cli_table_footer "Col1" "Col2" "Col3"`
Print table footer (closes the table).

**Example:**

```bash
cli_table_header "Service" "Status" "Port"
cli_table_row "postgres" "running" "5432"
cli_table_row "hasura" "running" "8080"
cli_table_row "auth" "stopped" "4000"
cli_table_footer "Service" "Status" "Port"

# Output:
# ┌─────────┬─────────┬──────┐
# │ Service │ Status  │ Port │
# ├─────────┼─────────┼──────┤
# │ postgres│ running │ 5432 │
# │ hasura  │ running │ 8080 │
# │ auth    │ stopped │ 4000 │
# └─────────┴─────────┴──────┘
```

---

### Lists

#### `cli_list_item "text"`
Print a bullet list item.

```bash
cli_list_item "First item"
cli_list_item "Second item"
# Output:
#   • First item
#   • Second item
```

#### `cli_list_numbered number "text"`
Print a numbered list item.

```bash
cli_list_numbered 1 "First task"
cli_list_numbered 2 "Second task"
# Output:
#   1. First task
#   2. Second task
```

#### `cli_list_checked "text"`
Print a checked checklist item.

```bash
cli_list_checked "Completed task"
# Output:
#   [✓] Completed task (in green)
```

#### `cli_list_unchecked "text"`
Print an unchecked checklist item.

```bash
cli_list_unchecked "Pending task"
# Output:
#   [ ] Pending task (dimmed)
```

---

### Progress Indicators

#### `cli_progress "task" current total`
Show a progress bar for a task.

```bash
cli_progress "Building project" 45 100
# Output:
# ⚙ Building project [██████████████████░░░░░░░░░░░░░░░░░░░░]  45%

cli_progress "Building project" 100 100
# Output:
# ⚙ Building project [████████████████████████████████████████] 100% ✓
```

#### `cli_spinner_start "message"`
Start an animated spinner (returns PID).

```bash
spinner_pid=$(cli_spinner_start "Loading configuration")
# Do work...
cli_spinner_stop "$spinner_pid" "Configuration loaded"

# Output (animated):
# ⠋ Loading configuration...
# (becomes)
# ✓ Configuration loaded
```

**Note:** Spinners only work in interactive terminals. In CI/non-TTY, a simple message is printed instead.

---

### Special Output

#### `cli_summary "title" "item1" "item2" ...`
Print a summary box with multiple items.

```bash
cli_summary "Build Complete" \
  "5 services started" \
  "Database initialized" \
  "Nginx configured"

# Output:
# ╔══════════════════════════════════════════════════════════╗
# ║                  ★ Build Complete ★                      ║
# ╟──────────────────────────────────────────────────────────╢
# ║  • 5 services started                                    ║
# ║  • Database initialized                                  ║
# ║  • Nginx configured                                      ║
# ╚══════════════════════════════════════════════════════════╝
```

#### `cli_banner "title" ["subtitle"]`
Print a banner for major events.

```bash
cli_banner "nself v1.0.0" "Modern Full-Stack Platform"

# Output:
# ╔══════════════════════════════════════════════════════════╗
# ║                                                          ║
# ║                      nself v1.0.0                        ║
# ║               Modern Full-Stack Platform                 ║
# ║                                                          ║
# ╚══════════════════════════════════════════════════════════╝
```

#### `cli_separator [width]`
Print a horizontal separator line.

```bash
cli_separator     # 60 characters (default)
cli_separator 40  # 40 characters

# Output:
# ────────────────────────────────────────────────────────────
```

---

### Utilities

#### `cli_strip_colors`
Remove ANSI color codes from text (useful for logging).

```bash
colored_output=$(cli_success "Done")
plain_output=$(echo "$colored_output" | cli_strip_colors)
echo "$plain_output" >> logfile.txt
```

#### `cli_blank [count]`
Print blank line(s).

```bash
cli_blank     # 1 blank line
cli_blank 3   # 3 blank lines
```

#### `cli_center "text" width`
Center text within a given width.

```bash
cli_center "Centered Text" 60
# Output (centered within 60 characters):
#                      Centered Text
```

#### `cli_indent "message" [level]`
Print an indented message.

```bash
cli_indent "Level 1 indent" 1
cli_indent "Level 2 indent" 2
cli_indent "Level 3 indent" 3
# Output:
#   Level 1 indent
#     Level 2 indent
#       Level 3 indent
```

---

## Environment Variables

### NO_COLOR

The library respects the `NO_COLOR` environment variable ([no-color.org](https://no-color.org/)):

```bash
# Disable all colors
export NO_COLOR=1
nself build

# Colors will be disabled for all output
```

### DEBUG

Enable debug messages:

```bash
# Show debug messages
DEBUG=true nself build

# Debug messages will be shown
```

---

## Platform Compatibility

### Bash 3.2+ Support

The library is fully compatible with Bash 3.2 (default on macOS):

- ✅ Uses `printf` (not `echo -e`)
- ✅ No Bash 4+ features (associative arrays, lowercase expansion)
- ✅ No external dependencies
- ✅ Tested on macOS, Linux, WSL

### Terminal Detection

The library automatically detects terminal capabilities:

```bash
# Interactive terminal
if [[ -t 1 ]]; then
  # Colors enabled, animations work
fi

# CI/non-TTY
if [[ ! -t 1 ]]; then
  # Colors may be disabled, spinners become simple messages
fi
```

---

## Usage Patterns

### Command Structure

Standard pattern for nself CLI commands:

```bash
#!/usr/bin/env bash
# command.sh - Description

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../lib/utils/cli-output.sh"

main() {
  cli_header "Command Name"

  cli_section "Phase 1"
  cli_info "Starting phase 1..."
  # Do work
  cli_success "Phase 1 complete"

  cli_section "Phase 2"
  cli_info "Starting phase 2..."
  # Do work
  cli_success "Phase 2 complete"

  cli_summary "Operation Complete" \
    "Phase 1: Done" \
    "Phase 2: Done"
}

main "$@"
```

### Error Handling

```bash
perform_operation() {
  cli_info "Starting operation..."

  if ! some_command; then
    cli_error "Operation failed: some_command returned error"
    cli_warning "Check logs for more details"
    return 1
  fi

  cli_success "Operation completed successfully"
  return 0
}
```

### Multi-Step Processes

```bash
build_project() {
  local steps=5

  cli_header "Build Process"

  cli_step 1 $steps "Installing dependencies"
  npm install || { cli_error "npm install failed"; return 1; }

  cli_step 2 $steps "Running linter"
  npm run lint || { cli_warning "Linting found issues"; }

  cli_step 3 $steps "Running tests"
  npm test || { cli_error "Tests failed"; return 1; }

  cli_step 4 $steps "Building application"
  npm run build || { cli_error "Build failed"; return 1; }

  cli_step 5 $steps "Cleanup"
  npm run clean

  cli_summary "Build Complete" \
    "Dependencies: Installed" \
    "Tests: Passed" \
    "Build: Successful"
}
```

### Progress Tracking

```bash
process_files() {
  local files=("file1.txt" "file2.txt" "file3.txt")
  local total=${#files[@]}
  local current=0

  for file in "${files[@]}"; do
    ((current++))
    cli_progress "Processing files" $current $total

    # Process file
    process_file "$file"
  done
}
```

### Long-Running Operations

```bash
load_configuration() {
  local spinner_pid
  spinner_pid=$(cli_spinner_start "Loading configuration")

  # Simulate long operation
  sleep 3

  cli_spinner_stop "$spinner_pid" "Configuration loaded successfully"
}
```

### Service Status Display

```bash
show_status() {
  cli_header "Service Status"

  cli_table_header "Service" "Status" "Port" "Health"
  cli_table_row "postgres" "running" "5432" "healthy"
  cli_table_row "hasura" "running" "8080" "healthy"
  cli_table_row "auth" "stopped" "4000" "n/a"
  cli_table_row "nginx" "running" "443" "healthy"
  cli_table_footer "Service" "Status" "Port" "Health"
}
```

---

## Testing

Run the test suite:

```bash
bash src/tests/unit/test-cli-output.sh
```

Tests include:
- ✅ All message types
- ✅ Sections and headers
- ✅ Boxes and borders
- ✅ Lists (bullet, numbered, checkbox)
- ✅ Tables
- ✅ Progress bars
- ✅ Spinners
- ✅ Summaries and banners
- ✅ NO_COLOR support
- ✅ Non-TTY output
- ✅ Bash 3.2 compatibility

---

## Migration Guide

### From `display.sh`

```bash
# Old
log_info "message"
log_success "message"
log_error "message"
log_warning "message"

# New
cli_info "message"
cli_success "message"
cli_error "message"
cli_warning "message"
```

### From `output-formatter-v2.sh`

```bash
# Old
format_success "message"
format_error "message"
format_warning "message"
format_info "message"

# New
cli_success "message"
cli_error "message"
cli_warning "message"
cli_info "message"
```

### From Raw printf/echo

```bash
# Old
echo -e "\033[32m✓\033[0m Success"
printf "\033[31m✗\033[0m Error\n"

# New
cli_success "Success"
cli_error "Error"
```

---

## Color Reference

### Available Colors

All colors are exported as constants:

```bash
CLI_RESET       # Reset all formatting
CLI_BOLD        # Bold text
CLI_DIM         # Dimmed text
CLI_UNDERLINE   # Underlined text

# Standard colors
CLI_RED         CLI_GREEN       CLI_YELLOW
CLI_BLUE        CLI_MAGENTA     CLI_CYAN
CLI_WHITE       CLI_BLACK

# Bright colors
CLI_BRIGHT_RED          CLI_BRIGHT_GREEN
CLI_BRIGHT_YELLOW       CLI_BRIGHT_BLUE
CLI_BRIGHT_MAGENTA      CLI_BRIGHT_CYAN
CLI_BRIGHT_WHITE
```

### Custom Colored Output

```bash
# Manual color usage
printf "%b%s%b\n" "${CLI_GREEN}" "Custom green text" "${CLI_RESET}"

# Combined formatting
printf "%b%b%s%b\n" "${CLI_BOLD}" "${CLI_RED}" "Bold red text" "${CLI_RESET}"
```

---

## Icon Reference

### Available Icons

```bash
CLI_ICON_SUCCESS    # ✓
CLI_ICON_ERROR      # ✗
CLI_ICON_WARNING    # ⚠
CLI_ICON_INFO       # ℹ
CLI_ICON_ARROW      # →
CLI_ICON_BULLET     # •
CLI_ICON_CHECK      # ✓
CLI_ICON_CROSS      # ✗
CLI_ICON_STAR       # ★
CLI_ICON_GEAR       # ⚙
CLI_ICON_ROCKET     # 🚀
CLI_ICON_PACKAGE    # 📦
CLI_ICON_FIRE       # 🔥
CLI_ICON_SPARKLES   # ✨
```

### Custom Icon Usage

```bash
printf "%b%s%b %s\n" \
  "${CLI_BLUE}" "${CLI_ICON_ROCKET}" "${CLI_RESET}" \
  "Launching application"
```

---

## Best Practices

### 1. Consistent Hierarchy

```bash
cli_header "Top-level operation"      # Major sections
cli_section "Sub-operation"           # Sub-sections
cli_info "Detailed step"              # Individual actions
```

### 2. Meaningful Icons

- Use `cli_success` for completed operations
- Use `cli_error` for failures that stop execution
- Use `cli_warning` for issues that don't stop execution
- Use `cli_info` for informational messages

### 3. Progress Feedback

Always provide feedback for long operations:

```bash
# Bad
perform_long_operation  # User sees nothing

# Good
cli_info "Starting long operation..."
perform_long_operation
cli_success "Operation completed"

# Better
spinner_pid=$(cli_spinner_start "Performing operation")
perform_long_operation
cli_spinner_stop "$spinner_pid" "Operation completed"
```

### 4. Error Context

Provide actionable information with errors:

```bash
# Bad
cli_error "Failed"

# Good
cli_error "Failed to connect to database"
cli_info "Check that PostgreSQL is running: nself status"
```

### 5. Summaries

End complex operations with summaries:

```bash
cli_summary "Build Complete" \
  "Duration: 2m 34s" \
  "Warnings: 0" \
  "Errors: 0" \
  "Services started: 5"
```

---

## Troubleshooting

### Colors Not Showing

1. Check `NO_COLOR` environment variable: `echo $NO_COLOR`
2. Verify terminal supports colors: `echo -e "\033[31mRed\033[0m"`
3. Test with: `cli_success "Test"` (should be green)

### Spinners Not Animating

Spinners only work in interactive terminals. In CI or when piped, they become simple messages (by design).

### Box Characters Not Displaying

Ensure terminal supports UTF-8:
```bash
echo $LANG  # Should contain UTF-8
```

### Wide Output Wrapping

The library uses 60-character width by default. For narrower terminals:
```bash
# Most functions respect this width
cli_box "Text"           # 60 chars
cli_separator 40         # 40 chars (custom)
```

---

## Performance

The library is optimized for performance:

- **No external commands** - Pure Bash, no `sed`/`awk`/etc
- **Minimal string operations** - Direct printf usage
- **Lazy evaluation** - Colors only loaded when needed
- **TTY detection** - Skips animations in non-interactive mode

Typical overhead: **<1ms per function call**

---

## Contributing

When adding new functions:

1. Use `printf` (never `echo -e`)
2. Support NO_COLOR
3. Test in Bash 3.2
4. Add to test suite
5. Document in this file
6. Export the function

---

## License

Part of nself - MIT License

---

## See Also

- [Platform Compatibility Guide](CROSS-PLATFORM-COMPATIBILITY.md)
- [Testing Guide](../testing/README.md)
