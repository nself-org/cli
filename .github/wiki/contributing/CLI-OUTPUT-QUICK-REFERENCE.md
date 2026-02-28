# CLI Output Library - Quick Reference

One-page cheat sheet for common CLI output functions.

## Setup

```bash
source "src/lib/utils/cli-output.sh"
```

## Basic Messages

| Function | Usage | Output |
|----------|-------|--------|
| `cli_success "msg"` | Success | ✓ msg (green) |
| `cli_error "msg"` | Error | ✗ msg (red, stderr) |
| `cli_warning "msg"` | Warning | ⚠ msg (yellow, stderr) |
| `cli_info "msg"` | Info | ℹ msg (blue) |
| `cli_debug "msg"` | Debug | [DEBUG] msg (if DEBUG=true) |

## Structure

| Function | Usage | Purpose |
|----------|-------|---------|
| `cli_header "Title"` | Major section | Double-line box header |
| `cli_section "Name"` | Subsection | → Name (bold) |
| `cli_step 1 5 "task"` | Multi-step | ⚙ Step 1/5 ─ task |

## Boxes

```bash
cli_box "message" [type]
# Types: info, success, error, warning

cli_box_detailed "Title" "Content"
# Longer content with word wrap
```

## Lists

```bash
cli_list_item "text"          # • text
cli_list_numbered 1 "text"    # 1. text
cli_list_checked "text"       # [✓] text
cli_list_unchecked "text"     # [ ] text
```

## Tables

```bash
cli_table_header "Col1" "Col2" "Col3"
cli_table_row "A" "B" "C"
cli_table_row "D" "E" "F"
cli_table_footer "Col1" "Col2" "Col3"
```

## Progress

```bash
# Progress bar
cli_progress "Task" 50 100    # [████████████░░░░░░] 50%

# Spinner
pid=$(cli_spinner_start "Loading...")
# ... do work ...
cli_spinner_stop $pid "Done"
```

## Special

```bash
cli_summary "Title" "Item1" "Item2" "Item3"
cli_banner "Title" "Subtitle"
cli_separator [width]
```

## Utilities

```bash
cli_blank [count]           # Blank lines
cli_center "text" width     # Center text
cli_indent "text" level     # Indent text
echo "text" | cli_strip_colors  # Remove colors
```

## Common Patterns

### Command Template
```bash
#!/usr/bin/env bash
source "src/lib/utils/cli-output.sh"

main() {
  cli_header "Command Name"

  cli_section "Phase 1"
  cli_info "Starting..."
  # ... work ...
  cli_success "Phase 1 complete"

  cli_summary "Complete" "Item 1" "Item 2"
}

main "$@"
```

### Error Handling
```bash
if ! command; then
  cli_error "Operation failed"
  cli_info "Try: nself help command"
  exit 1
fi
cli_success "Operation succeeded"
```

### Multi-Step Process
```bash
steps=5
cli_step 1 $steps "Step 1"
# ... work ...
cli_step 2 $steps "Step 2"
# ... work ...
cli_summary "Complete" "All steps done"
```

### Service Status
```bash
cli_table_header "Service" "Status" "Port"
cli_table_row "postgres" "running" "5432"
cli_table_row "hasura" "running" "8080"
cli_table_footer "Service" "Status" "Port"
```

## Environment Variables

```bash
NO_COLOR=1      # Disable colors
DEBUG=true      # Show debug messages
```

## Color Constants

```bash
CLI_RED CLI_GREEN CLI_YELLOW CLI_BLUE
CLI_BOLD CLI_DIM CLI_RESET
```

## Icons

```bash
CLI_ICON_SUCCESS CLI_ICON_ERROR CLI_ICON_WARNING
CLI_ICON_INFO CLI_ICON_ARROW CLI_ICON_BULLET
```

## Tips

1. **Always use printf** - Never `echo -e`
2. **stderr for errors/warnings** - cli_error and cli_warning use stderr
3. **60-char standard** - Most functions use 60-character width
4. **TTY detection** - Spinners auto-disable in non-interactive mode
5. **NO_COLOR** - Always respect user's NO_COLOR setting

## Full Documentation

See [CLI-OUTPUT-LIBRARY.md](CLI-OUTPUT-LIBRARY.md) for complete API reference.

## Examples

```bash
bash src/examples/cli-output-demo.sh        # Interactive demo
bash src/tests/unit/test-cli-output-quick.sh  # Quick test
```
