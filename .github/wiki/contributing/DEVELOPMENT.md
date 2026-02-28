# Contributing to nself v0.3.9

Thank you for your interest in contributing to nself! This document provides guidelines and standards for contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Architecture Overview](#architecture-overview)
- [Coding Standards](#coding-standards)
- [Testing Guidelines](#testing-guidelines)
- [Submitting Changes](#submitting-changes)
- [Documentation](#documentation)

## Code of Conduct

Be respectful, inclusive, and professional. We're building infrastructure tools that people depend on.

## Getting Started

1. **Fork the repository**
   ```bash
   git clone https://github.com/nself-org/cli.git
   cd nself
   ```

2. **Create a test project**
   ```bash
   mkdir ~/test-nself && cd ~/test-nself
   nself init
   ```

3. **Never run nself commands in the nself repository itself**
   - The repository contains source code, not a project
   - Always test in a separate directory
   - Safety checks prevent accidental repository pollution

## Development Setup

### Required Tools
- Bash 4.0+
- Docker 20.10+
- Docker Compose v2
- Git
- curl

### Repository Structure
```
/
├── bin/                    # All executable scripts
│   ├── nself.sh           # Main command dispatcher
│   ├── *.sh              # Individual command files
│   ├── shared/           # Shared utilities and libraries
│   │   ├── utils/        # Utility functions
│   │   ├── config/       # Configuration defaults
│   │   ├── hooks/        # Pre/post command hooks
│   │   └── auto-fix/     # Auto-fix strategies
│   ├── services/         # Service-related scripts
│   ├── templates/        # Service templates
│   └── VERSION          # Version file
├── docs/                 # Documentation
└── install.sh           # Installation script
```

## Architecture Overview

### Core Principles

1. **Modular Commands**
   - One `.sh` file per command in `/bin/`
   - Commands export a `cmd_<name>` function
   - Wrapper dispatches to appropriate command

2. **Shared Utilities**
   - Common functions in `/bin/shared/utils/`
   - Always source utilities, never duplicate code
   - Utilities must be idempotent (safe to source multiple times)

3. **Docker Compose Wrapper**
   ```bash
   # Always use the wrapper, never direct docker compose
   compose() {
       local env_file="${COMPOSE_ENV_FILE:-.env.local}"
       local project="${PROJECT_NAME:-nself}"
       
       if [[ -f "$env_file" ]]; then
           docker compose --project-name "$project" --env-file "$env_file" "$@"
       else
           docker compose --project-name "$project" "$@"
       fi
   }
   ```

4. **Hooks System**
   - Every command calls `pre_command` at start
   - Every command calls `post_command` at end
   - Hooks handle validation, logging, cleanup

5. **Auto-Fix Philosophy**
   - Only 4 safe, predictable scenarios
   - Never attempt risky operations
   - Always log attempts
   - User can disable with `--no-autofix`

## Coding Standards

### Shell Script Guidelines

1. **File Headers**
   ```bash
   #!/bin/bash
   # <filename> - <brief description>
   
   # Source shared utilities
   SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
   source "$SCRIPT_DIR/shared/utils/display.sh"
   ```

2. **Error Handling**
   ```bash
   set -eo pipefail  # Use carefully, not always appropriate
   
   # For commands that might fail
   if ! some_command; then
       log_error "Command failed"
       return 1
   fi
   ```

3. **Function Naming**
   - Command functions: `cmd_<name>`
   - Utilities: `<verb>_<noun>` (e.g., `load_env`, `validate_port`)
   - Internal: prefix with underscore `_internal_function`

4. **Variable Naming**
   - Environment variables: `UPPERCASE_WITH_UNDERSCORES`
   - Local variables: `lowercase_with_underscores`
   - Constants: `readonly CONSTANT_NAME="value"`

5. **Logging Standards**
   ```bash
   log_info "Informational message"
   log_success "Operation succeeded"
   log_warning "Warning message"
   log_error "Error message"
   log_debug "Debug message (only if DEBUG=true)"
   ```

6. **Output Formatting**
   - Use structured output functions from `display.sh`
   - Never use raw `echo` for user-facing output
   - Errors go to stderr: `>&2`
   - Keep output concise and actionable

### Safety Checks

1. **Repository Protection**
   ```bash
   # Prevent running in nself repository
   if [[ -f "bin/nself.sh" ]] && [[ -d "bin/shared" ]]; then
       log_error "Cannot run in nself repository!"
       return 1
   fi
   ```

2. **Environment Validation**
   ```bash
   # Safe environment loading
   load_env_safe() {
       local env_file="${1:-.env.local}"
       # Validates and sources without executing
   }
   ```

3. **Path Handling**
   ```bash
   # Always use absolute paths for safety
   SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
   ```

## Testing Guidelines

### Manual Testing

1. **Command Testing**
   ```bash
   cd ~/test-project
   nself init
   nself build
   nself up
   nself status
   nself down
   ```

2. **Hook Verification**
   - Check logs in `logs/nself.log`
   - Verify pre/post hooks execute
   - Confirm error handling works

3. **Auto-Fix Testing**
   - Test port conflicts
   - Test Docker build failures
   - Test missing dependencies
   - Verify limited scope

### Test Directory Structure
```
~/test-nself/         # Manual testing
/tmp/nself-test-*/    # Automated test runs
```

## Submitting Changes

### Pull Request Process

1. **Branch Naming**
   - Features: `feature/description`
   - Fixes: `fix/description`
   - Docs: `docs/description`

2. **Commit Messages**
   ```
   type: Brief description
   
   Longer explanation if needed.
   
   Fixes #123
   ```
   
   Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

3. **PR Description Template**
   ```markdown
   ## Description
   Brief description of changes
   
   ## Type of Change
   - [ ] Bug fix
   - [ ] New feature
   - [ ] Breaking change
   - [ ] Documentation update
   
   ## Testing
   - [ ] Tested locally
   - [ ] Hooks verified
   - [ ] Auto-fix tested (if applicable)
   
   ## Checklist
   - [ ] Follows coding standards
   - [ ] Documentation updated
   - [ ] No duplicate code
   - [ ] Uses shared utilities
   ```

## Documentation

### Documentation Standards

1. **File Naming**
   - Use UPPERCASE with underscores: `ARCHITECTURE.MD`
   - Exception: `README.md` (GitHub convention)

2. **Documentation Types**
   - `README.md` - User-facing documentation
   - `ARCHITECTURE.MD` - System design
   - `CODE_STYLE.MD` - Detailed coding standards
   - `API.MD` - Command reference

3. **Inline Documentation**
   ```bash
   # Function: Brief description
   # Arguments:
   #   $1 - Description
   #   $2 - Description
   # Returns:
   #   0 - Success
   #   1 - Failure
   function_name() {
       local arg1="$1"
       local arg2="$2"
   }
   ```

### AI Agent Documentation

Special documentation in `/docs/` helps AI agents understand the codebase:

- **ARCHITECTURE.MD** - System design and principles
- **CODE_STYLE.MD** - Detailed coding patterns
- **DIRECTORY_STRUCTURE.MD** - File organization
- **OUTPUT_STANDARDS.MD** - User interface and output standards

AI agents should reference these when making changes.

## Version Management

1. **Version File**: `/bin/VERSION`
2. **Format**: Semantic versioning `MAJOR.MINOR.PATCH`
3. **Update Process**:
   - Update `/bin/VERSION`
   - Update `CHANGELOG.md`
   - Tag release: `git tag v0.3.0`

## Common Patterns

### Adding a New Command

1. Create `/bin/mycommand.sh`:
   ```bash
   #!/bin/bash
   # mycommand.sh - Brief description
   
   SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
   source "$SCRIPT_DIR/shared/utils/display.sh"
   source "$SCRIPT_DIR/shared/hooks/pre-command.sh"
   source "$SCRIPT_DIR/shared/hooks/post-command.sh"
   
   cmd_mycommand() {
       # Implementation
   }
   
   export -f cmd_mycommand
   
   # Execute if run directly
   if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
       pre_command "mycommand" || exit $?
       cmd_mycommand "$@"
       exit_code=$?
       post_command "mycommand" $exit_code
       exit $exit_code
   fi
   ```

2. Update help in `/bin/help.sh`
3. Add documentation
4. Test thoroughly

### Adding a Utility Function

1. Add to appropriate file in `/bin/shared/utils/`
2. Export the function
3. Add safety check for double-sourcing if needed
4. Document parameters and return values

## Support

- **Issues**: [GitHub Issues](https://github.com/nself-org/cli/issues)
- **Discussions**: [GitHub Discussions](https://github.com/nself-org/cli/discussions)
- **Support Development**: [nself.org/commercial](https://nself.org/commercial)

## License

By contributing, you agree that your contributions will be licensed under the same license as the project (see LICENSE file).