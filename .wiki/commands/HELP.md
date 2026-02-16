# nself help - Command Help

**Version 0.9.9** | Display help information

---

## Overview

The `nself help` command displays help information for nself and its commands. It provides usage instructions, available options, and examples.

---

## Basic Usage

```bash
# General help
nself help
nself --help
nself -h

# Command-specific help
nself help build
nself build --help

# Subcommand help
nself help db migrate
nself db migrate --help
```

---

## Help Topics

### Commands

```bash
nself help commands     # List all commands
nself help init         # Help for init command
nself help db           # Help for db command group
```

### Guides

```bash
nself help quickstart   # Quick start guide
nself help deployment   # Deployment guide
```

---

## Output Format

Help output includes:

1. **Usage**: Command syntax
2. **Description**: What the command does
3. **Options**: Available flags
4. **Examples**: Common use cases
5. **See Also**: Related commands

---

## Online Documentation

For detailed documentation:

- **Wiki**: https://github.com/acamarata/nself/wiki
- **Commands**: https://github.com/acamarata/nself/wiki/commands/COMMANDS

---

## See Also

- [completion](COMPLETION.md) - Shell completion
- [version](VERSION.md) - Version info
- [doctor](DOCTOR.md) - System diagnostics
