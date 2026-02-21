# nself Help System

Comprehensive help text files for all major nself commands.

## Overview

This directory contains detailed help documentation for nself commands, formatted for 80-column terminal display with proper structure and examples.

## Available Help Files

| Command | File | Lines | Description |
|---------|------|-------|-------------|
| `nself billing` | billing.help.txt | 309 | ⚠️  DEPRECATED - Use `nself tenant billing` |
| `nself whitelabel` | whitelabel.help.txt | 388 | ⚠️  DEPRECATED - Use `nself tenant branding/domains/email/themes` |
| `nself tenant` | tenant.help.txt | 359 | Multi-tenant management, isolation, member management |
| `nself tenant billing` | tenant-billing.help.txt | 120 | Tenant-scoped billing and usage tracking |
| `nself tenant branding` | tenant-branding.help.txt | 90 | Tenant-scoped branding and customization |
| `nself tenant domains` | tenant-domains.help.txt | 85 | Tenant-scoped domain management and SSL |
| `nself tenant email` | tenant-email.help.txt | 85 | Tenant-scoped email template management |
| `nself tenant themes` | tenant-themes.help.txt | 100 | Tenant-scoped theme management |
| `nself org` | org.help.txt | 419 | Organizations, teams, RBAC, permissions |
| `nself plugin` | plugin.help.txt | 434 | Plugin system, installation, registry, custom plugins |
| `nself realtime` | realtime.help.txt | 440 | WebSocket server, channels, presence, real-time features |
| `nself security` | security.help.txt | 447 | Security scanning, MFA, incidents, device management |
| `nself perf` | perf.help.txt | 412 | Performance optimization, slow queries, database tuning |
| `nself dev` | dev.help.txt | 448 | SDK generation, docs, test helpers, mock data |
| `nself migrate` | migrate.help.txt | 493 | Environment & vendor migration, Firebase, Supabase |

**Total: 4,629 lines of comprehensive documentation**

### Command Structure Changes (v0.5.0)

As of v0.5.0, billing and white-label features have been reorganized under the `tenant` command:

**Old Structure (Deprecated):**
```bash
nself billing <command>
nself whitelabel <command>
```

**New Structure (Current):**
```bash
nself tenant billing <command>
nself tenant branding <command>
nself tenant domains <command>
nself tenant email <command>
nself tenant themes <command>
```

## Usage

### Display Help in Terminal

```bash
# Show help for a command
cat src/lib/help/billing.help.txt

# Page through help (recommended)
less src/lib/help/billing.help.txt

# Search within help
grep -i "stripe" src/lib/help/billing.help.txt
```

### Integration with CLI

Help files can be integrated into the CLI commands:

```bash
# In a CLI script (e.g., src/cli/billing.sh)
show_help() {
  if [[ -f "$NSELF_ROOT/src/lib/help/billing.help.txt" ]]; then
    cat "$NSELF_ROOT/src/lib/help/billing.help.txt"
  else
    # Fallback to inline help
    echo "nself billing - Billing Management"
    # ... inline help content
  fi
}
```

### Search Across All Help Files

```bash
# Find which commands support a feature
grep -r "webhook" src/lib/help/*.help.txt

# Find examples
grep -r "EXAMPLES" src/lib/help/*.help.txt -A 20
```

## Help File Structure

Each help file follows a consistent structure:

```
================================================================================
Command Title - Brief Description
================================================================================

OVERVIEW:
  Detailed overview of the command and its purpose

USAGE:
  Command syntax and basic usage patterns

================================================================================
COMMANDS
================================================================================

  Organized list of subcommands with descriptions

================================================================================
OPTIONS
================================================================================

  Available flags and options

================================================================================
[COMMAND-SPECIFIC SECTIONS]
================================================================================

  Sections relevant to the specific command, such as:
  - Permission System
  - API Integration
  - Database Schema
  - Security Features
  - etc.

================================================================================
EXAMPLES
================================================================================

  3-5 practical examples with real-world use cases
  Organized from simple to complex

================================================================================
COMMON WORKFLOWS
================================================================================

  Step-by-step guides for common tasks
  Multi-step procedures

================================================================================
RELATED COMMANDS
================================================================================

  Links to related nself commands

================================================================================
TIPS & BEST PRACTICES
================================================================================

  10 actionable tips for effective usage

================================================================================
TROUBLESHOOTING
================================================================================

  Common issues and solutions
  Error messages and fixes

================================================================================
MORE INFORMATION
================================================================================

  Links to:
  - Documentation
  - Examples
  - Support
```

## Content Guidelines

### Formatting

1. **80-Column Width**: All text wrapped at 80 characters for terminal display
2. **Section Headers**: Double-line separators (80 equals signs)
3. **Indentation**: 2 spaces for nested content
4. **Examples**: Properly formatted with command prompts ($)
5. **Code Blocks**: Indented and syntax-highlighted where possible

### Content Requirements

Each help file must include:

1. **Overview** - Clear explanation of purpose
2. **Commands** - All subcommands with descriptions
3. **Options** - All flags and their effects
4. **Examples** - At least 3-5 practical examples
5. **Workflows** - Common multi-step procedures
6. **Troubleshooting** - Common issues and solutions
7. **Related Commands** - Cross-references
8. **Tips** - Best practices (target: 10 tips)

### Writing Style

- **Imperative mood** for commands ("Run", "Create", "Update")
- **Clear, concise** descriptions
- **No assumptions** - explain everything
- **Practical examples** - real-world scenarios
- **Progressive complexity** - simple to advanced

## Examples Section Guidelines

Good examples should:

1. **Be self-contained** - Include all necessary context
2. **Show real output** - When helpful
3. **Demonstrate progression** - Start simple, add complexity
4. **Cover common use cases** - Not edge cases
5. **Include explanations** - When command is complex

Example structure:

```
Simple task:
  $ nself command simple-option

Intermediate task with options:
  $ nself command complex --option value --flag

Advanced multi-step workflow:
  $ nself command prepare
  $ nself command execute --advanced-option
  $ nself command verify
```

## Troubleshooting Section Guidelines

Format:

```
"Error message or symptom"
  - First troubleshooting step
  - Second step
  - Command to diagnose: nself command --debug
  - Resolution command
```

Always include:
- Clear symptom description
- Diagnostic steps
- Resolution commands
- Prevention tips

## Maintenance

### Adding New Help Files

1. Create file: `src/lib/help/<command>.help.txt`
2. Follow structure template above
3. Include all required sections
4. Test formatting: `cat <file> | head -100`
5. Verify 80-column width: No lines > 80 chars
6. Update this README with new entry

### Updating Existing Help

1. Preserve section structure
2. Update examples with current syntax
3. Add new features to relevant sections
4. Update troubleshooting as issues discovered
5. Increment version in file header if needed

### Quality Checklist

- [ ] 80-column width maintained
- [ ] All sections present
- [ ] At least 3 practical examples
- [ ] Common workflows documented
- [ ] Troubleshooting section complete
- [ ] Related commands listed
- [ ] Tips section has 10 items
- [ ] No spelling errors
- [ ] Commands tested and verified
- [ ] Cross-references accurate

## Version Control

Help files are version controlled with the codebase:

- **Track changes** to reflect feature updates
- **Tag releases** to match nself versions
- **Review changes** during PR process
- **Test examples** before committing

## Integration Points

### CLI Integration

```bash
# In CLI script
NSELF_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HELP_FILE="$NSELF_ROOT/src/lib/help/command.help.txt"

show_help() {
  if [[ -f "$HELP_FILE" ]]; then
    cat "$HELP_FILE" | less
  else
    echo "Help file not found"
    exit 1
  fi
}
```

### Web Documentation

Help files can be converted to web docs:

```bash
# Convert to HTML
pandoc -f markdown -t html billing.help.txt > billing.html

# Convert to PDF
pandoc -f markdown -t pdf billing.help.txt > billing.pdf

# Convert to Markdown
# Already in markdown-compatible format
```

### Man Pages

Help files can be converted to man pages:

```bash
# Generate man page
help2man ./nself-billing --include=billing.help.txt > nself-billing.1

# Install man page
sudo cp nself-billing.1 /usr/local/share/man/man1/
sudo mandb

# View man page
man nself-billing
```

## Statistics

- **Total Files**: 10
- **Total Lines**: 4,149
- **Average Lines per File**: 415
- **Total Size**: ~145 KB
- **Coverage**: All major nself commands

## Future Additions

Planned help files for additional commands:

- `nself init.help.txt` - Initialization and setup
- `nself build.help.txt` - Build system
- `nself deploy.help.txt` - Deployment
- `nself backup.help.txt` - Backup and restore
- `nself monitor.help.txt` - Monitoring and metrics

## Support

For questions or suggestions about help documentation:

- Create issue: https://github.com/nself-org/cli/issues
- Email: docs@nself.org
- Slack: #documentation channel

## License

Help files are part of nself and distributed under the same license.

---

Last Updated: 2026-01-30
Version: 0.9.0
