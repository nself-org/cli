# Contributing Documentation

Guide for contributing to ɳSelf development.

## Overview

- **[Contributing Guide](CONTRIBUTING.md)** - How to contribute to ɳSelf
- **[Code of Conduct](CODE_OF_CONDUCT.md)** - Community guidelines
- **[Development Setup](DEVELOPMENT.md)** - Setting up dev environment
- **[Development Guide](README.md)** - Complete development guide

## Development

### Environment Setup

```bash
# Clone repository
git clone https://github.com/nself-org/cli.git
cd nself

# Install dependencies
./install.sh --dev

# Run tests
./test.sh
```

### Development Workflow

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests and linting
5. Submit a pull request

See **[Development Guide](DEVELOPMENT.md)** for complete details.

## Code Standards

### Shell Scripting

- **[Cross-Platform Compatibility](CROSS-PLATFORM-COMPATIBILITY.md)** - Bash 3.2+ requirements
  - No Bash 4+ features
  - POSIX-compliant where possible
  - Works on macOS, Linux, WSL

### Output Formatting

- **[CLI Output Library](CLI-OUTPUT-LIBRARY.md)** - Output formatting standards
- **[CLI Output Quick Reference](CLI-OUTPUT-QUICK-REFERENCE.md)** - Quick formatting guide

### Key Requirements

1. **No `echo -e`** - Use `printf` instead
2. **No Bash 4+ features** - Support Bash 3.2+
3. **Always validate input** - Security first
4. **Use platform-compat wrappers** - Cross-platform compatibility
5. **Write tests** - All features must have tests

## Cross-Platform Development

### Critical Rules

**NEVER use:**
- `echo -e` (use `printf`)
- `${var,,}` or `${var^^}` (use `tr`)
- `declare -A` (use parallel arrays)
- `mapfile`/`readarray`
- Unguarded `stat -c` or `stat -f`

**ALWAYS use:**
- `printf` for formatted output
- `safe_stat_perms()` for file permissions
- `safe_sed_inline()` for in-place edits
- `command -v` to check command availability

See **[Cross-Platform Compatibility](CROSS-PLATFORM-COMPATIBILITY.md)** for complete guide.

## Testing

### Running Tests

```bash
# Unit tests
./src/tests/unit/test-*.sh

# Integration tests
./src/tests/integration/test-*.sh

# All tests
./test.sh --all

# Specific test
./src/tests/unit/test-init.sh
```

### Writing Tests

```bash
# Test function format
test_my_feature() {
  # Setup
  local result

  # Execute
  result=$(my_function "input")

  # Assert
  assert_equals "expected" "$result"
}
```

### CI/CD

All tests must pass on:
- Ubuntu Latest (Bash 5.x)
- Ubuntu with Bash 3.2
- macOS Latest (Bash 3.2)

## CLI Output Standards

### Using the Output Library

```bash
# Source the library
source "$(dirname "${BASH_SOURCE[0]}")/../../lib/output/output.sh"

# Use output functions
output_success "Operation completed"
output_error "Something failed"
output_info "For your information"
output_warning "Be careful"
output_step "Step 1" "Doing something"
```

### Formatting Guidelines

**Good:**
```bash
printf "✓ %s\n" "Task completed"
output_success "Database migrated"
```

**Bad:**
```bash
echo -e "\e[32m✓\e[0m Task completed"  # Not portable
echo "Database migrated"  # No formatting
```

See **[CLI Output Library](CLI-OUTPUT-LIBRARY.md)** for complete API.

## Documentation

### Writing Documentation

1. Use clear, concise language
2. Include code examples
3. Add cross-references
4. Update navigation (INDEX.md files)
5. Test all commands

### Documentation Structure

```
docs/
├── getting-started/    # New user docs
├── guides/            # How-to guides
├── tutorials/         # Step-by-step tutorials
├── commands/          # Command reference
├── architecture/      # System design
└── reference/         # API reference
```

## Pull Request Process

### Before Submitting

1. **Run all tests**
   ```bash
   ./test.sh --all
   ```

2. **Check for portability issues**
   ```bash
   grep -r 'echo -e' src/
   grep -r '\${[^}]*,,[^}]*}' src/
   ```

3. **Run shellcheck** (if available)
   ```bash
   shellcheck -S error src/**/*.sh
   ```

4. **Update documentation** as needed

### PR Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Documentation update
- [ ] Performance improvement

## Testing
- [ ] All tests pass
- [ ] Added new tests
- [ ] Tested on macOS
- [ ] Tested on Linux

## Checklist
- [ ] No `echo -e` usage
- [ ] No Bash 4+ features
- [ ] Documentation updated
- [ ] Tests added/updated
```

## Code Review Process

1. **Automated Checks**
   - ShellCheck linting
   - Portability check
   - Unit tests (3 platforms)
   - Integration tests

2. **Manual Review**
   - Code quality
   - Documentation
   - Test coverage
   - Security considerations

3. **Approval**
   - At least 1 maintainer approval
   - All checks passing
   - No unresolved comments

## Community

### Getting Help

- [GitHub Discussions](https://github.com/nself-org/cli/discussions)
- [Discord Community](https://discord.gg/nself)
- [Issue Tracker](https://github.com/nself-org/cli/issues)

### Contributing Areas

**Code:**
- Bug fixes
- New features
- Performance improvements
- Test coverage

**Documentation:**
- Guides and tutorials
- API documentation
- Examples
- Translations

**Community:**
- Answer questions
- Write blog posts
- Create videos
- Share examples

## Security

### Reporting Vulnerabilities

**DO NOT** create public issues for security vulnerabilities.

Email: security@nself.org

Include:
- Description of vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### Security Guidelines

1. Never commit secrets
2. Always validate input
3. Use parameterized queries
4. Check file permissions
5. Sanitize user input

See **[Security Documentation](../security/README.md)** for details.

## License

ɳSelf is MIT licensed. By contributing, you agree to license your contributions under the MIT license.

---

**[← Back to Documentation Home](../README.md)**
