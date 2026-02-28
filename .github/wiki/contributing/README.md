# Contributing to nself

Thank you for your interest in contributing to nself! We welcome contributions from developers of all skill levels.

---

## Quick Links

| Resource | Description |
|----------|-------------|
| **[Development Setup](DEVELOPMENT.md)** | Environment setup and coding standards |
| **[Cross-Platform Compatibility](CROSS-PLATFORM-COMPATIBILITY.md)** | **CRITICAL** - Bash 3.2+, POSIX compliance rules |
| **[Code of Conduct](CODE_OF_CONDUCT.md)** | Community standards |

---

## Quick Start for Contributors

### 1. Fork and Clone

```bash
git clone https://github.com/YOUR_USERNAME/nself.git
cd nself
```

### 2. Create Test Directory

**IMPORTANT:** Never run nself commands in the nself repository itself!

```bash
mkdir ~/nself-test
cd ~/nself-test
```

### 3. Link Development CLI

```bash
# Create symlink to your development version
ln -s ~/path/to/nself/src/cli/nself.sh /usr/local/bin/nself-dev

# Use nself-dev for testing
nself-dev init
nself-dev build
nself-dev start
```

### 4. Make Changes

```bash
cd ~/path/to/nself
git checkout -b feature/my-contribution

# Make your changes
# Edit files in src/

# Test in ~/nself-test
cd ~/nself-test
nself-dev build && nself-dev start
```

### 5. Run Tests

```bash
cd ~/path/to/nself

# Run unit tests
bash src/tests/unit/test-init.sh

# Run integration tests
bash src/tests/integration/test-init-integration.sh

# Run all tests
bash src/tests/run-all.sh
```

### 6. Submit Pull Request

```bash
git add .
git commit -m "feat: Add awesome new feature"
git push origin feature/my-contribution

# Open PR on GitHub
```

---

## Critical: Cross-Platform Compatibility

**READ THIS FIRST:** [Cross-Platform Compatibility Guide](CROSS-PLATFORM-COMPATIBILITY.md)

nself must work on:
- macOS (Bash 3.2)
- All Linux distributions (Ubuntu, Debian, RHEL, Alpine, etc.)
- WSL (Windows Subsystem for Linux)

### Mandatory Rules

**NEVER use:**
- `echo -e` (use `printf` instead)
- `${var,,}` or `${var^^}` (use `tr` command)
- `declare -A` (use parallel arrays)
- `mapfile` or `readarray` (use `while read` loops)
- `stat -c` or `stat -f` without platform detection

**ALWAYS:**
- Use `printf` for formatted output
- Check command availability before using
- Use platform wrappers from `src/lib/utils/platform-compat.sh`
- Test on both macOS and Linux if possible

**[Read Full Compatibility Guide](CROSS-PLATFORM-COMPATIBILITY.md)**

---

## Development Workflow

### Project Structure

```
nself/
├── src/
│   ├── cli/              # CLI entry points
│   ├── lib/              # Core libraries
│   │   ├── init/        # Initialization logic
│   │   ├── build/       # Build system
│   │   ├── deploy/      # Deployment
│   │   ├── db/          # Database tools
│   │   └── utils/       # Utilities
│   ├── templates/        # Service templates
│   └── tests/           # Test suite
├── docs/                # Documentation
└── .releases/          # Release packaging
```

### Coding Standards

**Shell Scripts:**
- Use Bash 3.2+ compatible syntax
- Follow POSIX standards where possible
- Use meaningful variable names
- Add comments for complex logic
- Run `shellcheck` before committing

**Documentation:**
- Update relevant docs for code changes
- Use clear, concise language
- Include code examples
- Add screenshots where helpful

**Commit Messages:**
- Follow conventional commits format
- Use descriptive messages
- Reference issues when applicable

```
feat: Add new command
fix: Resolve database connection issue
docs: Update installation guide
test: Add tests for init command
```

---

## What to Contribute

### High Priority

- **Bug Fixes** - Fix issues reported on GitHub
- **Documentation** - Improve clarity and coverage
- **Tests** - Increase test coverage
- **Service Templates** - Add new language/framework templates
- **Plugin Development** - Create new integrations

### Medium Priority

- **Performance Improvements** - Optimize slow operations
- **CLI Enhancements** - Better user experience
- **Error Messages** - More helpful error messages
- **Examples** - Real-world configuration examples

### Low Priority

- **Code Refactoring** - Improve code quality
- **Minor Features** - Small improvements
- **Translations** - Internationalization (future)

---

## Testing Guidelines

### Before Submitting

1. Run all tests and ensure they pass
2. Test on macOS if you developed on Linux (or vice versa)
3. Check for `echo -e` usage (portability issue)
4. Verify no Bash 4+ features used
5. Run `shellcheck` on modified shell scripts

### Writing Tests

**Unit Tests:**
```bash
# src/tests/unit/test-my-feature.sh
test_my_feature() {
  local result
  result=$(my_function "input")
  assert_equals "expected" "$result"
}
```

**Integration Tests:**
```bash
# src/tests/integration/test-my-feature.sh
test_my_feature_integration() {
  cd "$TEST_DIR"
  nself init --simple
  nself build
  assert_equals "0" "$?"
}
```

**[View Development Guide](DEVELOPMENT.md)**

---

## Pull Request Process

### 1. Create Issue First

For major changes, create an issue first to discuss:
- What problem are you solving?
- How will you solve it?
- Are there alternative approaches?

### 2. Fork and Branch

```bash
git clone https://github.com/YOUR_USERNAME/nself.git
git checkout -b feature/descriptive-name
```

### 3. Make Changes

- Write clean, readable code
- Follow coding standards
- Add/update tests
- Update documentation

### 4. Test Thoroughly

```bash
# Run all tests
bash src/tests/run-all.sh

# Test on different platforms if possible
# macOS, Ubuntu, Debian, etc.
```

### 5. Commit and Push

```bash
git add .
git commit -m "feat: Add descriptive commit message"
git push origin feature/descriptive-name
```

### 6. Open Pull Request

- Fill out PR template completely
- Link related issues
- Describe what changed and why
- Add screenshots/examples if applicable
- Request review from maintainers

### 7. Address Feedback

- Respond to review comments
- Make requested changes
- Push updates to same branch
- Re-request review when ready

---

## Community Guidelines

### Code of Conduct

We follow a code of conduct to ensure a welcoming community:

- **Be respectful** - Treat everyone with respect
- **Be constructive** - Provide helpful feedback
- **Be patient** - Remember everyone is learning
- **Be inclusive** - Welcome diverse perspectives

**[Full Code of Conduct](CODE_OF_CONDUCT.md)**

### Getting Help

**Documentation Issues:**
- Open issue: [GitHub Issues](https://github.com/nself-org/cli/issues)
- Tag: `documentation`

**Code Questions:**
- GitHub Discussions: [Discussions](https://github.com/nself-org/cli/discussions)
- Tag: `question`

**Bug Reports:**
- GitHub Issues: [Issues](https://github.com/nself-org/cli/issues)
- Tag: `bug`
- Include: nself version, OS, steps to reproduce

---

## Recognition

Contributors are recognized in:
- Project README
- Release notes
- Contributors page

Thank you for making nself better!

---

## Related Documentation

- **[Development Setup](DEVELOPMENT.md)** - Environment and standards
- **[Cross-Platform Guide](CROSS-PLATFORM-COMPATIBILITY.md)** - Compatibility requirements
- **[Architecture](../architecture/ARCHITECTURE.md)** - System design
- **[Commands](../commands/README.md)** - CLI reference

---

**[Back to Documentation Home](../README.md)**
