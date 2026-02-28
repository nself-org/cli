# Testing Documentation

Testing, quality assurance, and validation documentation for nself.

## Overview

This directory contains testing guides, QA reports, and validation documentation for ensuring nself quality and reliability.

## Quality Assurance Reports

The QA reports are located in the **[qa/](../qa/)** directory:

- **[QA Summary](../qa/QA-SUMMARY.md)** - Overall QA status and results
- **[V1 QA Report](../qa/V1-QA-REPORT.md)** - Version 1.0 QA validation
- **[V1 Command Structure QA](../qa/V1-COMMAND-STRUCTURE-QA-REPORT.md)** - Command consolidation validation
- **[Final Validation Report](../qa/v1.0-final-validation-report.md)** - v1.0 final validation

## Testing Guides

### Unit Testing
```bash
# Run unit tests
bash src/tests/unit/test-init.sh

# Run specific test
bash src/tests/unit/test-commands.sh
```

### Integration Testing
```bash
# Run integration tests
bash src/tests/integration/test-init-integration.sh

# Full integration suite
bash src/tests/integration/run-all.sh
```

### CI/CD Testing

nself uses GitHub Actions for continuous integration:

- **ShellCheck Linting** - Code quality validation
- **Portability Checks** - Bash 3.2+ compatibility
- **Unit Tests** - Multiple platforms (Ubuntu, macOS)
- **Integration Tests** - Full workflow validation
- **Security Scanning** - Dependency and code security

See **[.github/workflows/](../../.github/workflows/)** for CI configuration.

## Cross-Platform Testing

nself must work across all platforms:

- **macOS** - Bash 3.2, BSD tools
- **Linux** - All distributions (Ubuntu, Debian, RHEL, Alpine)
- **WSL** - Windows Subsystem for Linux

See **[Cross-Platform Compatibility](../contributing/CROSS-PLATFORM-COMPATIBILITY.md)** for requirements.

## Development Testing

For contributors testing local changes:

```bash
# Check shell script quality
shellcheck src/lib/**/*.sh src/cli/*.sh

# Test portability
bash src/tests/portability-check.sh

# Run CI locally (requires act)
act -j test
```

## Related Documentation

- **[Contributing Guide](../contributing/CONTRIBUTING.md)** - How to contribute
- **[Development Setup](../contributing/DEVELOPMENT.md)** - Dev environment
- **[QA Directory](../qa/README.md)** - QA reports and tracking

---

**Last Updated**: January 31, 2026
**Version**: v0.9.6
