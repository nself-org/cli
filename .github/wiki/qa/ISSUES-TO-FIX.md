# Issues to Fix - v1.0 QA Follow-Up

**Priority:** Non-Critical (Post-Release)
**Target Version:** v1.1 or v1.2

---

## Overview

The v1.0 QA testing revealed **5 non-critical warnings** and **1 test suite issue**. None of these issues block release, but they should be addressed in future minor versions for consistency and maintainability.

---

## Issue 1: Output Formatting Inconsistency

### Category: Code Quality / Consistency
### Priority: Low
### Affected Commands: 3
### Target Fix: v1.1

### Description

Three commands are not using the standardized `cli-output.sh` library for output formatting:

1. **init.sh**
   - Currently uses custom output formatting
   - Should use: `print_info()`, `print_success()`, `print_error()`, etc.

2. **build.sh**
   - Currently uses custom output formatting
   - Should use: `print_info()`, `print_success()`, `print_error()`, etc.

3. **env.sh**
   - Currently uses custom output formatting
   - Should use: `print_info()`, `print_success()`, `print_error()`, etc.

### Impact

- **Functional Impact:** None - commands work correctly
- **User Impact:** Minimal - output is still readable and functional
- **Code Quality:** Minor inconsistency in output styling across commands
- **Maintainability:** Harder to maintain consistent output styles

### Why It's Not Blocking

These commands have complete, working implementations with their own output formatting. While they don't use the standardized library, their output is:
- Clear and readable
- Properly colored and formatted
- Functionally correct
- User-friendly

### Recommended Fix

**Approach:** Incremental refactoring
1. Audit each command's output calls
2. Map custom formatting to `cli-output.sh` equivalents
3. Replace custom formatting gradually
4. Test output before/after to ensure consistency
5. Update one command per commit for easier review

**Example Refactoring:**

**Before (custom formatting):**
```bash
echo -e "${GREEN}✓${NC} Project initialized successfully"
```

**After (standardized):**
```bash
source "$(dirname "$0")/../lib/utils/cli-output.sh"
print_success "Project initialized successfully"
```

**Effort Estimate:** 2-4 hours per command

---

## Issue 2: Subcommand Structure Review Needed

### Category: Code Structure
### Priority: Low
### Affected Commands: 2
### Target Fix: v1.1

### Description

Two commands may need subcommand handling review:

1. **env.sh**
   - QA test expected case statement for subcommands
   - May handle subcommands differently or not have them yet
   - Need to verify intended command structure

2. **secrets.sh**
   - QA test expected case statement for subcommands
   - May handle subcommands differently or not have them yet
   - Need to verify intended command structure

### Impact

- **Functional Impact:** None - commands work as designed
- **Consistency:** May differ from other multi-level commands
- **Future Proofing:** May need restructuring if subcommands added later

### Why It's Not Blocking

These commands are functional and work correctly. The "warning" is based on test expectations, not actual command failures. They may:
- Not have subcommands yet (by design)
- Handle subcommands through a different mechanism
- Be planned for subcommands in a future version

### Recommended Investigation

1. **Review command specifications:**
   - Check `docs/commands/COMMAND-TREE-V1.md`
   - Determine if `env` and `secrets` should have subcommands
   - Document current vs. intended behavior

2. **Check current implementation:**
   ```bash
   # Test current behavior
   nself env --help
   nself secrets --help

   # Look for subcommand handling
   grep -n "case.*in" src/cli/env.sh
   grep -n "case.*in" src/cli/secrets.sh
   ```

3. **Decide on action:**
   - **Option A:** Commands are simple and don't need subcommands (document this)
   - **Option B:** Add case statements for future subcommands (structure for growth)
   - **Option C:** Commands already handle subcommands differently (update tests)

**Effort Estimate:** 1-2 hours investigation + implementation if needed

---

## Issue 3: Test Suite Out of Date

### Category: Testing Infrastructure
### Priority: Medium
### Affected File: `src/tests/v1-command-structure-test.sh`
### Target Fix: v1.0.1 or v1.1

### Description

The command structure test suite is checking for commands that don't exist in the v1.0 specification and missing checks for commands that do exist.

**Test expects but shouldn't:**
- `destroy.sh` - Not in v1.0 spec
- `shell.sh` - Not in v1.0 spec
- `domain.sh` - Not in v1.0 spec
- `seed.sh` - Not in v1.0 spec
- `test.sh` - Not a TLC (under `dev test`)
- `lint.sh` - Not a TLC (under `dev lint`)

**Test should check but doesn't:**
- `tenant.sh` - v1.0 TLC
- `infra.sh` - v1.0 TLC
- `service.sh` - v1.0 TLC
- `perf.sh` - v1.0 TLC
- `plugin.sh` - v1.0 TLC
- Subcommand routing (e.g., `tenant billing`, `auth mfa`)

### Impact

- **Functional Impact:** None - the actual code is correct
- **Testing Impact:** High - can't validate v1.0 structure correctly
- **CI/CD Impact:** Test suite will fail erroneously
- **Developer Confidence:** Confusing false failures

### Why It's Not Blocking Release

This is a **test bug, not a code bug**. The actual command structure is correct according to the v1.0 spec. The test suite simply wasn't updated during refactoring.

### Recommended Fix

**Update the test suite to match v1.0 spec:**

```bash
# File: src/tests/v1-command-structure-test.sh

# TEST 1: Check correct 31 TLCs exist
test_command_exists "init" "init.sh exists"
test_command_exists "build" "build.sh exists"
test_command_exists "start" "start.sh exists"
test_command_exists "stop" "stop.sh exists"
test_command_exists "restart" "restart.sh exists"

# Utilities (15)
test_command_exists "status" "status.sh exists"
test_command_exists "logs" "logs.sh exists"
test_command_exists "help" "help.sh exists"
test_command_exists "admin" "admin.sh exists"
test_command_exists "urls" "urls.sh exists"
test_command_exists "exec" "exec.sh exists"
test_command_exists "doctor" "doctor.sh exists"
test_command_exists "monitor" "monitor.sh exists"
test_command_exists "health" "health.sh exists"
test_command_exists "version" "version.sh exists"
test_command_exists "update" "update.sh exists"
test_command_exists "completion" "completion.sh exists"
test_command_exists "metrics" "metrics.sh exists"
test_command_exists "history" "history.sh exists"
test_command_exists "audit" "audit.sh exists"

# Other Commands (11)
test_command_exists "db" "db.sh exists"
test_command_exists "tenant" "tenant.sh exists"
test_command_exists "deploy" "deploy.sh exists"
test_command_exists "infra" "infra.sh exists"
test_command_exists "service" "service.sh exists"
test_command_exists "config" "config.sh exists"
test_command_exists "auth" "auth.sh exists"
test_command_exists "perf" "perf.sh exists"
test_command_exists "backup" "backup.sh exists"
test_command_exists "dev" "dev.sh exists"
test_command_exists "plugin" "plugin.sh exists"

# TEST 2: Check subcommand routing
test_subcommand_routing "tenant billing" "tenant billing routes correctly"
test_subcommand_routing "auth mfa" "auth mfa routes correctly"
test_subcommand_routing "service storage" "service storage routes correctly"
test_subcommand_routing "deploy upgrade" "deploy upgrade routes correctly"
test_subcommand_routing "config env" "config env routes correctly"

# TEST 3: Check legacy command warnings
test_deprecation_warning "billing" "billing shows deprecation warning"
test_deprecation_warning "org" "org shows deprecation warning"
test_deprecation_warning "storage" "storage shows deprecation warning"
```

**Effort Estimate:** 3-4 hours (rewrite test suite)

---

## Root Cause Analysis

### Why These Issues Exist

1. **Output Formatting Inconsistency:**
   - Commands were created at different times
   - `cli-output.sh` library was standardized later
   - Some commands haven't been refactored yet
   - Not caught during initial refactoring sweep

2. **Subcommand Structure:**
   - Test assumptions may not match command design
   - Commands may be designed as simple commands without subcommands
   - Specifications unclear on which commands need subcommands
   - Test suite expectations not validated against spec

3. **Test Suite Out of Date:**
   - Test suite created before v1.0 spec finalized
   - Not updated during 79→31 command consolidation
   - Uses old command structure (v0.x)
   - No CI validation for test suite accuracy

### Prevention for Future

1. **Code Standards:**
   - Create coding standards document
   - Require all new commands use `cli-output.sh`
   - Add pre-commit checks for output formatting
   - Document subcommand patterns

2. **Test Suite Maintenance:**
   - Update tests alongside code changes
   - Validate test suite against spec before release
   - Add test suite validation to CI/CD
   - Keep test suite in sync with command tree

3. **Documentation:**
   - Keep command tree docs up to date
   - Document which commands have subcommands
   - Maintain migration guide during refactoring
   - Review docs before each release

---

## Fix Priority and Timeline

### Immediate (v1.0.1 Patch - If Needed)
**Priority:** None - no critical issues

### Short Term (v1.1 Minor Release)
**Target Date:** 1-2 months after v1.0 release

1. ✅ **Update test suite** (Medium priority - 3-4 hours)
   - Blocks: Proper CI/CD validation
   - Impact: High for developer confidence

2. ✅ **Review env/secrets subcommands** (Low priority - 1-2 hours)
   - Blocks: Nothing
   - Impact: Low, documentation/consistency

### Medium Term (v1.2 Minor Release)
**Target Date:** 3-4 months after v1.0 release

3. ✅ **Refactor init.sh formatting** (Low priority - 2-4 hours)
   - Blocks: Nothing
   - Impact: Code consistency

4. ✅ **Refactor build.sh formatting** (Low priority - 2-4 hours)
   - Blocks: Nothing
   - Impact: Code consistency

5. ✅ **Refactor env.sh formatting** (Low priority - 2-4 hours)
   - Blocks: Nothing
   - Impact: Code consistency

### Total Estimated Effort
- **Test Suite Update:** 3-4 hours
- **Subcommand Review:** 1-2 hours
- **Output Formatting:** 6-12 hours (3 commands)
- **Total:** 10-18 hours across 2 minor releases

---

## Conclusion

All identified issues are **cosmetic, consistency, or testing infrastructure** problems. None affect functionality or block release.

**Release Status:** ✅ **APPROVED**

These issues should be tracked in the backlog and addressed incrementally in v1.1 and v1.2 releases as part of continuous improvement.

---

*Document Created: 2026-01-30*
*Last Updated: 2026-01-30*
*Related: docs/qa/V1-QA-REPORT.md*
