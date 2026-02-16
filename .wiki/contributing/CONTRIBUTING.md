# Contributing to nself

Thank you for your interest in contributing to nself! We welcome contributions from developers of all skill levels.

## 📚 Complete Documentation

**Essential Reading:**
- **[Development Setup](DEVELOPMENT.md)** - Environment setup and coding standards
- **[Cross-Platform Compatibility](CROSS-PLATFORM-COMPATIBILITY.md)** - **CRITICAL** - Bash 3.2+, POSIX compliance rules
- **[Code of Conduct](CODE_OF_CONDUCT.md)** - Community standards

---

## Quick Start for Contributors

### 1. Fork and Clone

```bash
git clone https://github.com/YOUR_USERNAME/nself.git
cd nself
```

### 2. Create Test Directory

⚠️ **IMPORTANT**: Never run nself commands in the nself repository itself!

```bash
# Create separate test directory
mkdir ~/test-nself && cd ~/test-nself

# Initialize test project
/path/to/nself/bin/nself init --demo
```

### 3. Make Your Changes

```bash
cd /path/to/nself
git checkout -b feature/my-contribution

# Edit files in src/, docs/, etc.
```

### 4. Test Your Changes

```bash
# Build and test in your test directory
cd ~/test-nself
/path/to/nself/bin/nself build --force
/path/to/nself/bin/nself start
/path/to/nself/bin/nself status
```

### 5. Pre-Commit Checks

**CRITICAL**: Run these compatibility checks before committing:

```bash
# Check for echo -e (not POSIX compliant)
grep -r "echo -e" src/ && echo "❌ FAIL" || echo "✅ PASS"

# Check for Bash 4+ features
grep -r '\${[^}]*,,}' src/ && echo "❌ FAIL" || echo "✅ PASS"
grep -r '\${[^}]*\^\^}' src/ && echo "❌ FAIL" || echo "✅ PASS"
grep -r "declare -A" src/ && echo "❌ FAIL" || echo "✅ PASS"

# All must PASS before committing
```

### 6. Submit Pull Request

```bash
git add .
git commit -m "feat: Brief description of changes"
git push origin feature/my-contribution

# Open PR on GitHub
```

---

## Development Requirements

### Platform Support

nself supports:
- ✅ macOS (Bash 3.2, BSD tools)
- ✅ Linux - all distros (Bash 3.2+, GNU tools)
- ✅ WSL (Windows Subsystem for Linux)

### Required Tools

- Bash 3.2+ (NOT Bash 4+ features)
- Docker 20.10+
- Docker Compose v2
- Git

---

## Compatibility Rules (CRITICAL)

### ❌ NEVER Use

```bash
# Bash 4+ features
${var,,}              # Use: tr '[:upper:]' '[:lower:]'
${var^^}              # Use: tr '[:lower:]' '[:upper:]'
declare -A            # Use: alternative data structures
mapfile / readarray   # Use: while read loops
&>>                   # Use: 2>&1

# Non-POSIX
echo -e "text"        # Use: printf "%s\n" "text"
```

### ✅ ALWAYS Use

```bash
# POSIX-compliant alternatives
printf "%s\n" "text"                    # Instead of echo -e
tr '[:upper:]' '[:lower:]'              # Instead of ${var,,}
safe_stat_perms() { ... }               # Platform-safe wrappers
```

**Read [CROSS-PLATFORM-COMPATIBILITY.md](CROSS-PLATFORM-COMPATIBILITY.md) for complete rules.**

---

## What to Contribute

### We Welcome

- 🐛 **Bug fixes** - Fix issues, improve stability
- ✨ **New features** - Add capabilities (discuss first in issues)
- 📚 **Documentation** - Improve guides, fix typos, add examples
- 🧪 **Tests** - Add test coverage, improve CI
- 🎨 **Templates** - New service templates for custom services
- 🔧 **Improvements** - Performance, code quality, refactoring

### Before Starting

1. **Check existing issues** - Avoid duplicate work
2. **Open an issue** - Discuss major changes first
3. **Read documentation** - Understand architecture and standards
4. **Review recent PRs** - See what's been accepted/rejected

---

## Pull Request Guidelines

### PR Checklist

- [ ] All CI tests pass (12 test jobs must pass)
- [ ] Pre-commit compatibility checks pass
- [ ] Code follows existing patterns
- [ ] Documentation updated (if needed)
- [ ] No hallucinated features (only real implementations)
- [ ] Tested on macOS or Linux (or both)
- [ ] Branch up to date with main
- [ ] Commit messages clear and descriptive

### Commit Message Format

```
type: Brief description

Longer explanation if needed.

Fixes #issue_number
```

**Types**: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

---

## Testing

### CI Pipeline

All PRs must pass 12 automated tests:
1. ShellCheck Linting
2. Unit Tests (Ubuntu - Bash 5.x)
3. Unit Tests (Ubuntu - Bash 3.2)
4. Unit Tests (macOS - Bash 3.2, BSD tools)
5. Portability Check
6. Integration Tests
7. File Permissions Test
8. Init Command Test
9. Build Command Test
10. Service Generation Test
11. Documentation Check
12. Security Scan

### Manual Testing

```bash
# Test complete workflow
cd ~/test-nself
nself init --demo
nself build
nself start
nself status
nself doctor
nself logs
nself stop --volumes
```

---

## Getting Help

### Resources

- **[FAQ](../getting-started/FAQ.md)** - Common questions
- **[Troubleshooting](../guides/TROUBLESHOOTING.md)** - Common issues
- **[Architecture Docs](../architecture/README.md)** - System design

### Communication

- **Issues**: [Bug reports and features](https://github.com/acamarata/nself/issues)
- **Discussions**: [Questions and ideas](https://github.com/acamarata/nself/discussions)

---

## Code of Conduct

We are committed to providing a welcoming and inclusive environment. Please read our [Code of Conduct](CODE_OF_CONDUCT.md).

Key principles:
- Be respectful and professional
- Be inclusive and welcoming
- Focus on what's best for the community
- We're building infrastructure people depend on - maintain high standards

---

## Recognition

Contributors are recognized in:
- Git commit history
- Release notes
- Project documentation

---

**Thank you for contributing to nself!** Your contributions help make backend infrastructure accessible to everyone.
