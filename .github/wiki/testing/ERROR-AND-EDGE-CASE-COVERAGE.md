# Error and Edge Case Test Coverage

This document describes the comprehensive error scenario and edge case test coverage for nself.

## Testing Philosophy

**Test what users actually encounter, not impossible edge cases.**

### What We Test ✅

- **User Errors** - Invalid input, misconfigurations, missing files
- **System Errors** - Resource limits, permissions, network failures
- **State Errors** - Invalid state transitions, race conditions
- **Boundary Values** - Min/max valid inputs, edge cases
- **Error Messages** - Clarity, actionability, platform compatibility

### What We Don't Test ❌

- Impossible scenarios (defensive programming)
- Third-party library bugs
- Hardware failures
- Cosmic ray bit flips

### Goals

1. **100% coverage of realistic user-facing errors**
2. **Every error has clear, actionable message**
3. **Cross-platform compatibility (macOS, Linux, WSL)**
4. **Fast test execution (<5 minutes total)**
5. **Reliable tests (no flakiness)**

---

## Error Scenario Tests

Location: `/Users/admin/Sites/nself/src/tests/errors/`

### Installation Errors

**File:** `test-installation-errors.sh`

| Scenario | Test | Error Message Quality |
|----------|------|----------------------|
| Docker not installed | ✅ | Provides install links for macOS/Linux |
| Docker daemon not running | ✅ | Platform-specific start commands |
| Insufficient permissions | ✅ | sudo and usermod solutions |
| Disk space too low | ✅ | Shows required vs available, cleanup commands |
| Incompatible Docker version | ✅ | Version requirements, update instructions |
| Port conflicts | ✅ | lsof/netstat commands, alternative ports |
| Missing dependencies | ✅ | apt-get/brew install commands |

**Coverage:** 12 tests

### Configuration Errors

**File:** `test-configuration-errors.sh`

| Scenario | Test | Error Message Quality |
|----------|------|----------------------|
| Missing .env file | ✅ | Suggests `nself init` |
| Invalid env variable format | ✅ | Shows correct format with examples |
| Port out of range | ✅ | Explains 1-65535 range |
| Invalid domain name | ✅ | Domain name rules and examples |
| Conflicting settings | ✅ | Shows conflict, multiple solutions |
| Missing required variables | ✅ | Lists all missing vars |
| Encrypted .env corruption | ✅ | 3 recovery options with commands |
| Invalid boolean value | ✅ | Explains true/false requirement |
| Invalid email format | ✅ | Email regex, valid/invalid examples |
| Invalid URL format | ✅ | Protocol requirement, examples |
| Example values not changed | ✅ | Security warning, password generation |
| Production without SSL | ✅ | Security warning, SSL setup steps |

**Coverage:** 12 tests

### Service Failures

**File:** `test-service-failures.sh`

| Scenario | Test | Error Message Quality |
|----------|------|----------------------|
| Port already in use | ✅ | Find process, kill or change port |
| Multiple port conflicts | ✅ | Lists all conflicts, solutions |
| Container fails to start | ✅ | View logs, troubleshoot steps |
| Dependency not ready | ✅ | Explains temporary, auto-recovery |
| Health check timeout | ✅ | Logs and restart commands |
| Docker image not found | ✅ | Pull manually, auth instructions |
| Build failure | ✅ | Failed step, cache clear commands |
| Out of memory | ✅ | Required vs available, free memory |
| Disk full | ✅ | Docker cleanup commands, disk check |
| Network connection failed | ✅ | Network diagnostics |
| DNS resolution failure | ✅ | Network inspect, troubleshoot |
| Missing env var at runtime | ✅ | Add var, rebuild steps |
| Volume permission denied | ✅ | chown/chmod, SELinux fix |

**Coverage:** 13 tests

**Total Error Tests:** 37 tests across 3 files

---

## Edge Case Tests

Location: `/Users/admin/Sites/nself/src/tests/edge-cases/`

### Boundary Values

**File:** `test-boundary-values.sh`

| Category | Tests | Coverage |
|----------|-------|----------|
| Port numbers | 7 | 0, 1, 1023, 1024, 65535, 65536, negative |
| String lengths | 3 | Empty, single char, very long (>1000) |
| Domain names | 5 | Single char, max 253, over 253, label max 63, over 63 |
| Special characters | 2 | Unicode, control characters |
| Numeric boundaries | 2 | Integer overflow, negative zero |
| Boolean values | 1 | true/false vs TRUE/yes/1/0 |
| Email lengths | 2 | Minimum (a@b.c), maximum (320 chars) |
| URL lengths | 2 | Minimum, IE limit (2083 chars) |

**Coverage:** 24 tests

### State Transitions

**File:** `test-state-transitions.sh`

| Category | Tests | Coverage |
|----------|-------|----------|
| Idempotency | 4 | Start running, stop stopped, restart stopped, multiple starts |
| Invalid transitions | 3 | Build without init, start without build, deploy without build |
| Rapid state changes | 2 | Rapid start/stop, multiple restarts |
| Error recovery | 2 | Start after crash, interrupted build |
| Concurrent operations | 1 | Build lock prevents simultaneous builds |
| Runtime changes | 2 | Env change while running, compose change detection |
| Partial failures | 1 | Some services start, others fail |

**Coverage:** 15 tests

### Concurrency (Planned)

**File:** `test-concurrency.sh` (to be created)

| Scenario | Test | Expected Behavior |
|----------|------|------------------|
| Multiple starts | ⏳ | Lock prevents, first wins |
| Simultaneous builds | ⏳ | Queue or error |
| Concurrent migrations | ⏳ | Database lock prevents |
| Parallel deployments | ⏳ | Safety checks prevent |
| File write races | ⏳ | Atomic operations |

**Planned Coverage:** 5 tests

### Data Integrity (Planned)

**File:** `test-data-integrity.sh` (to be created)

| Scenario | Test | Expected Behavior |
|----------|------|------------------|
| Corrupted compose file | ⏳ | Validation, regeneration |
| Corrupted .env | ⏳ | Validation, example |
| Corrupted migration | ⏳ | Checksum verification |
| Corrupted backup | ⏳ | Integrity check fails gracefully |
| Invalid config syntax | ⏳ | Parse error with line number |

**Planned Coverage:** 5 tests

**Total Edge Case Tests:** 39 tests (current) + 10 planned = 49 tests

---

## Test Organization

```
src/tests/
├── errors/                         # Error scenario tests
│   ├── README.md                  # Error testing documentation
│   ├── run-error-tests.sh         # Master error test runner
│   ├── test-installation-errors.sh (12 tests)
│   ├── test-configuration-errors.sh (12 tests)
│   └── test-service-failures.sh   (13 tests)
│
├── edge-cases/                    # Edge case tests
│   ├── README.md                  # Edge case documentation
│   ├── run-edge-case-tests.sh     # Master edge case runner
│   ├── test-boundary-values.sh    (24 tests)
│   ├── test-state-transitions.sh  (15 tests)
│   ├── test-concurrency.sh        (planned - 5 tests)
│   └── test-data-integrity.sh     (planned - 5 tests)
│
└── unit/
    └── test-error-messages.sh     # Unit tests for error library
```

---

## Running Tests

### Run All Error Tests
```bash
./src/tests/errors/run-error-tests.sh
```

Output:
```
========================================
  nself Error Scenario Tests
========================================

Running: test-installation-errors
✓ 12/12 tests passed

Running: test-configuration-errors
✓ 12/12 tests passed

Running: test-service-failures
✓ 13/13 tests passed

========================================
  ✓ All error tests passed!
========================================
```

### Run All Edge Case Tests
```bash
./src/tests/edge-cases/run-edge-case-tests.sh
```

### Run Individual Test Files
```bash
./src/tests/errors/test-installation-errors.sh
./src/tests/edge-cases/test-boundary-values.sh
```

---

## Error Message Quality Standards

Every error message MUST have:

### 1. Clear Title
```
Docker is not running
```
Not: `Error code 0x4E2`

### 2. Problem Description
```
Problem:
  The 'docker' command could not connect to the Docker daemon.
```

### 3. Fix Instructions (Numbered)
```
Fix:
  1. Start Docker Desktop:
     macOS: open -a Docker
     Linux: sudo systemctl start docker

  2. Verify Docker is running:
     docker ps
```

### 4. Platform-Specific Commands
```
macOS:
  open -a Docker

Linux:
  sudo systemctl start docker
```

### 5. Verification Step
```
Verify Docker is running:
  docker ps
```

---

## Test Patterns

### Pattern 1: Error Message Verification

```bash
test_error_message_quality() {
  local test_name="Error has all required sections"

  local output=$(generate_error_message)

  # Verify structure
  assert_contains "$output" "Problem:" "$test_name: Has problem section"
  assert_contains "$output" "Fix:" "$test_name: Has fix section"
  assert_contains "$output" "command" "$test_name: Has actionable command"

  # Verify no bad patterns
  assert_not_contains "$output" "Error code" "$test_name: No error codes"
  assert_not_contains "$output" "contact support" "$test_name: No support punt"
}
```

### Pattern 2: Boundary Value Testing

```bash
test_boundary_value() {
  local test_name="Port 65535 (max valid)"

  local port=65535
  if validate_port "$port"; then
    local result="valid"
  else
    local result="invalid"
  fi

  assert_equals "$result" "valid" "$test_name"
}
```

### Pattern 3: Idempotency Testing

```bash
test_idempotent_operation() {
  local test_name="Start is idempotent"

  start_service
  local state1="$STATE"

  start_service  # Run again
  local state2="$STATE"

  assert_equals "$state1" "$state2" "$test_name: State unchanged"
}
```

---

## Coverage Metrics

### Current Coverage

| Category | Tests | Files | Status |
|----------|-------|-------|--------|
| Error Scenarios | 37 | 3 | ✅ Complete |
| Boundary Values | 24 | 1 | ✅ Complete |
| State Transitions | 15 | 1 | ✅ Complete |
| Concurrency | 0 | 0 | ⏳ Planned |
| Data Integrity | 0 | 0 | ⏳ Planned |
| **Total** | **76** | **5** | **80% Complete** |

### Target Coverage

| Category | Goal | Current | Status |
|----------|------|---------|--------|
| Installation Errors | 100% | 100% | ✅ |
| Configuration Errors | 100% | 100% | ✅ |
| Service Failures | 100% | 100% | ✅ |
| Boundary Values | 100% | 100% | ✅ |
| State Transitions | 100% | 100% | ✅ |
| Concurrency | 100% | 0% | ⏳ |
| Data Integrity | 100% | 0% | ⏳ |

---

## CI Integration

Tests run in GitHub Actions on:
- Ubuntu (latest) - Bash 5.x, GNU tools
- macOS (latest) - Bash 3.2, BSD tools

### CI Requirements

All tests must:
- ✅ Pass on both Ubuntu and macOS
- ✅ Use `printf` (not `echo -e`)
- ✅ Avoid Bash 4+ features
- ✅ Complete in <30 seconds per file
- ✅ Clean up temporary files
- ✅ Not depend on external services
- ✅ Be deterministic (no flaky tests)

### CI Workflow

```yaml
name: Error & Edge Case Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]

    steps:
      - uses: actions/checkout@v2

      - name: Run error tests
        run: ./src/tests/errors/run-error-tests.sh

      - name: Run edge case tests
        run: ./src/tests/edge-cases/run-edge-case-tests.sh
```

---

## Next Steps

### High Priority (v0.9.8)
1. ✅ Installation error tests
2. ✅ Configuration error tests
3. ✅ Service failure tests
4. ✅ Boundary value tests
5. ✅ State transition tests

### Medium Priority (v0.9.9)
1. ⏳ Concurrency tests (`test-concurrency.sh`)
2. ⏳ Data integrity tests (`test-data-integrity.sh`)
3. ⏳ Cleanup/recovery tests (`test-cleanup-recovery.sh`)

### Low Priority (v1.0)
1. ⏳ Performance degradation tests
2. ⏳ Network failure simulation tests
3. ⏳ Resource exhaustion recovery tests

---

## Related Documentation

- `/docs/ERROR-HANDLING.md` - Error handling guidelines
- `/docs/TESTING.md` - General testing guidelines
- `/src/tests/errors/README.md` - Error test documentation
- `/src/tests/edge-cases/README.md` - Edge case documentation
- `/src/lib/utils/error-messages.sh` - Error message library

---

**Last Updated:** January 31, 2026
**Version:** 0.9.8
**Status:** 80% Complete (76 of 96 planned tests)
**Test Execution Time:** ~2 minutes total
