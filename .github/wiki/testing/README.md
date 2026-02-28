# Testing & Quality Assurance

Complete testing and QA documentation for nself.

## Testing Strategy

nself uses a comprehensive testing approach:

1. **Unit Tests** - Test individual functions and modules
2. **Integration Tests** - Test complete workflows
3. **Portability Tests** - Ensure cross-platform compatibility
4. **Security Tests** - Validate security measures
5. **Manual QA** - Feature validation and regression testing

## Running Tests

### Quick Test Suite

```bash
# Run all unit tests
bash src/tests/run-unit-tests.sh

# Run all integration tests
bash src/tests/run-integration-tests.sh

# Run portability checks
bash src/tests/portability-check.sh
```

### Individual Tests

```bash
# Test initialization
bash src/tests/unit/test-init.sh

# Test database commands
bash src/tests/unit/test-db.sh

# Test build system
bash src/tests/integration/test-build.sh
```

## CI/CD Pipeline

GitHub Actions runs automated tests on:

- **Every Push** - Unit tests and linting
- **Pull Requests** - Full test suite
- **Releases** - Complete validation including integration tests

### CI Test Matrix

| Platform | Bash Version | Test Coverage |
|----------|--------------|---------------|
| Ubuntu Latest | 5.1+ | Full suite |
| Ubuntu with Bash 3.2 | 3.2 | Compatibility tests |
| macOS Latest | 3.2 | BSD compatibility |

## Quality Assurance

### QA Reports

All QA reports are in the **[qa/](../qa/)** directory:

- Overall status and summaries
- Version-specific validation
- Issue tracking
- Regression testing results

### QA Process

1. **Feature Development** - Unit tests written alongside code
2. **Integration** - Integration tests validate workflows
3. **Manual Testing** - QA team validates features
4. **Regression Testing** - Existing features remain functional
5. **Release Validation** - Final checks before release

## Testing Requirements for Contributors

When contributing code:

1. Add unit tests for new functions
2. Add integration tests for new features
3. Ensure cross-platform compatibility
4. Run shellcheck on all shell scripts
5. Test on both macOS and Linux if possible

See **[Contributing Guide](../contributing/CONTRIBUTING.md)** for details.

## Test Coverage

Current test coverage by component:

| Component | Unit Tests | Integration Tests | Status |
|-----------|-----------|-------------------|--------|
| CLI Commands | High | High | ✅ Good |
| Database | High | Medium | ✅ Good |
| Build System | Medium | High | ✅ Good |
| Multi-tenancy | Medium | Medium | ⚠️ Improving |
| Plugins | Low | Low | ⚠️ In Progress |

## Known Issues

See **[qa/ISSUES-TO-FIX.md](../qa/ISSUES-TO-FIX.md)** for tracked issues.

## Related Documentation

- **[QA Reports](../qa/README.md)** - All QA documentation
- **[Contributing](../contributing/README.md)** - Contribution guidelines
- **[Development](../contributing/DEVELOPMENT.md)** - Dev environment setup
- **[Cross-Platform Compatibility](../contributing/CROSS-PLATFORM-COMPATIBILITY.md)** - Platform requirements

---

**Last Updated**: January 31, 2026
**Version**: v0.9.6
