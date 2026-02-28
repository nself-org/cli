# nself QA Documentation

This directory contains quality assurance documentation, test reports, and verification records for nself releases.

---

## v1.0 QA Documentation

### Quick Links

- **[QA Summary](V1-QA-SUMMARY.md)** - Executive summary of v1.0 testing
- **[Full QA Report](V1-COMMAND-STRUCTURE-QA-REPORT.md)** - Comprehensive test results and analysis

### Test Coverage

| Area | Status | Pass Rate | Details |
|------|--------|-----------|---------|
| Command Structure | ✅ Pass | 100% | All 31 top-level commands verified |
| Command Routing | ✅ Pass | 100% | All routes functional |
| Help System | ✅ Pass | 100% | Help/version working |
| Error Handling | ✅ Pass | 100% | Proper error messages |
| Output Formatting | ⚠️ Warning | 62% | 3 by design |
| Subcommand Support | ⚠️ Warning | 75% | 2 by design |
| **Overall** | ✅ **Pass** | **96%** | **137/142 tests** |

### Test Scripts

Located in `/src/tests/`:
- `v1-command-structure-test.sh` - Basic command verification
- `v1-comprehensive-qa.sh` - Full test suite (142 tests)

### Test Execution

```bash
# Run comprehensive QA suite
bash src/tests/v1-comprehensive-qa.sh

# Run specific unit tests
bash src/tests/unit/test-init.sh
bash src/tests/unit/test-cli-output-quick.sh

# Run all tests
bash src/tests/run-tests.sh
```

---

## QA Process

### 1. Unit Testing
- Individual component verification
- Module-level functionality
- Isolated function testing

### 2. Integration Testing
- Command routing verification
- Inter-component communication
- End-to-end workflows

### 3. Compatibility Testing
- Bash 3.2+ compatibility (macOS)
- Bash 4.x/5.x compatibility (Linux)
- Cross-platform verification
- Docker/Compose version testing

### 4. Performance Testing
- Command execution speed
- Memory usage profiling
- Resource consumption analysis

### 5. Security Testing
- Input validation
- Command injection prevention
- File permission checks
- Repository protection

---

## Test Environment

### Development
- Platform: macOS/Linux
- Bash: 3.2+ / 4.x / 5.x
- Docker: 20.x+
- Docker Compose: 2.x+

### CI/CD
- GitHub Actions
- Multiple OS matrix (Ubuntu, macOS)
- Multiple Bash versions
- Automated on push/PR

---

## Quality Standards

### Pass Criteria
- ✅ Zero critical failures
- ✅ 90%+ overall pass rate
- ✅ All production commands functional
- ✅ Help/version systems working
- ✅ Error handling proper
- ✅ Backward compatibility maintained

### Warning Criteria
- ⚠️ Non-critical warnings acceptable if by design
- ⚠️ Performance within 2x expected
- ⚠️ Memory usage within expected range

### Fail Criteria
- ❌ Any critical command broken
- ❌ Command routing failures
- ❌ Security vulnerabilities
- ❌ Data loss potential
- ❌ Backward compatibility broken

---

## Release Checklist

Before releasing a new version:

- [ ] Run comprehensive QA suite
- [ ] Verify all critical commands work
- [ ] Test help and version systems
- [ ] Check backward compatibility
- [ ] Run on multiple platforms
- [ ] Test with multiple Bash versions
- [ ] Verify security protections
- [ ] Update QA documentation
- [ ] Create QA report
- [ ] Get QA sign-off

---

## Historical QA Reports

### v1.0.0 (2026-01-30)
- **Status:** ✅ Approved
- **Pass Rate:** 96% (137/142)
- **Critical Issues:** 0
- **Warnings:** 5 (non-critical)
- **Report:** [V1-COMMAND-STRUCTURE-QA-REPORT.md](V1-COMMAND-STRUCTURE-QA-REPORT.md)

### v0.9.5 (Previous)
- **Status:** ✅ Approved
- **Focus:** Core functionality
- **Report:** N/A (pre-formal QA)

---

## Contact

For QA questions or to report test failures:
- Open an issue on GitHub
- Check existing test documentation
- Review test scripts in `/src/tests/`

---

## Contributing to QA

### Adding New Tests

1. Create test file in appropriate directory:
   - `/src/tests/unit/` - Unit tests
   - `/src/tests/integration/` - Integration tests
   - `/src/tests/` - Other tests

2. Follow naming convention:
   - `test-<feature>.sh` for specific features
   - `test-<command>.sh` for command tests

3. Use test framework:
   ```bash
   source "$TEST_DIR/test_framework.sh"
   ```

4. Add to test runner:
   - Update `run-all-tests.sh` if needed (invoked via `run-tests.sh`)

### Test Documentation

- Document test purpose at top of file
- Include usage examples
- Specify expected outcomes
- Note any dependencies

---

**Last Updated:** 2026-01-30
**Version:** v1.0.0
**Maintained By:** nself Core Team
