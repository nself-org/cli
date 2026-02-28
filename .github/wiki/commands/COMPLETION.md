# nself completion - Shell Completion

**Version 0.9.9** | Generate shell completion scripts

---

## Overview

The `nself completion` command generates shell completion scripts for bash, zsh, and fish. These enable tab-completion for nself commands, options, and arguments.

---

## Basic Usage

```bash
# Generate bash completion
nself completion bash

# Generate zsh completion
nself completion zsh

# Generate fish completion
nself completion fish
```

---

## Installation

### Bash

Add to `~/.bashrc`:

```bash
eval "$(nself completion bash)"
```

Or save to completions directory:

```bash
nself completion bash > /etc/bash_completion.d/nself
```

### Zsh

Add to `~/.zshrc`:

```bash
eval "$(nself completion zsh)"
```

Or save to completions:

```bash
nself completion zsh > "${fpath[1]}/_nself"
```

### Fish

Save to completions directory:

```bash
nself completion fish > ~/.config/fish/completions/nself.fish
```

---

## What Gets Completed

- All nself commands (`init`, `build`, `start`, etc.)
- Subcommands (`db migrate`, `plugin install`, etc.)
- Options (`--verbose`, `--help`, etc.)
- Service names for relevant commands
- Environment names

---

## Examples

```bash
nself <TAB>           # Shows all commands
nself db <TAB>        # Shows db subcommands
nself logs <TAB>      # Shows service names
nself --<TAB>         # Shows global options
```

---

## Verification

After installation, test completion:

```bash
# Reload shell
source ~/.bashrc  # or ~/.zshrc

# Test completion
nself <TAB><TAB>
```

---

## See Also

- [help](HELP.md) - Command help
- [version](VERSION.md) - Version info
