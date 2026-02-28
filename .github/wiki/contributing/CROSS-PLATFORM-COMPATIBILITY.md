# Cross-Platform Compatibility Guide

**Last Updated**: January 2026
**CI Status**: ✅ All 12 tests passing
**Platforms Supported**: Ubuntu, Debian, RHEL, Alpine, macOS (Sonoma/Sequoia/Tahoe), WSL

> **Note**: macOS continues to ship Bash 3.2 (last GPLv2 version from 2007) even in macOS Tahoe 26. Apple avoids GPLv3 software due to licensing concerns. We must continue targeting Bash 3.2 for macOS compatibility.

---

## Overview

nself is designed for **maximum compatibility** across all major platforms and shell environments. This guide documents the mandatory requirements and best practices for maintaining this compatibility.

### Compatibility Requirements

- ✅ **Bash 3.2+** (macOS default since 2007)
- ✅ **POSIX-compliant** where possible
- ✅ **All major Linux distributions** (Ubuntu, Debian, RHEL, Fedora, Alpine, etc.)
- ✅ **macOS** with BSD tools and Bash 3.2
- ✅ **WSL** (Windows Subsystem for Linux)

---

## Mandatory Shell Scripting Rules

### 🚫 NEVER Use These Bash 4+ Features

#### 1. Associative Arrays
```bash
# ❌ WRONG - Bash 4+ only
declare -A config
config["key"]="value"

# ✅ RIGHT - Use parallel arrays or case statements
keys=("key1" "key2")
values=("value1" "value2")
```

#### 2. Uppercase/Lowercase Parameter Expansion
```bash
# ❌ WRONG - Bash 4+ only
response="${input,,}"  # lowercase
response="${input^^}"  # uppercase

# ✅ RIGHT - Use tr command
response=$(echo "$input" | tr '[:upper:]' '[:lower:]')
response=$(echo "$input" | tr '[:lower:]' '[:upper:]')
```

**Real Example from Code**:
```bash
# wizard-simple.sh (FIXED)
read -r response
response=$(echo "$response" | tr '[:upper:]' '[:lower:]')
[[ "$response" == "y" ]] || [[ "$response" == "yes" ]]
```

#### 3. mapfile/readarray
```bash
# ❌ WRONG - Bash 4+ only
mapfile -t lines < file.txt

# ✅ RIGHT - Use while read loop
lines=()
while IFS= read -r line; do
  lines+=("$line")
done < file.txt
```

#### 4. coproc (Coprocesses)
```bash
# ❌ WRONG - Bash 4+ only
coproc mycoproc { command; }

# ✅ RIGHT - Use named pipes or process substitution
```

---

## Output Commands: echo vs printf

### ✅ ALWAYS Use printf for Formatted Output

The `echo -e` flag is **not portable** and behaves differently across shells.

```bash
# ❌ WRONG - Not portable!
echo -e "\033[32m✓\033[0m $message"
echo -e "Line 1\nLine 2"

# ✅ RIGHT - Always use printf
printf "\033[32m✓\033[0m %s\n" "$message"
printf "Line 1\nLine 2\n"
```

### When to Use echo

Only use `echo` for **simple, unformatted strings**:

```bash
# ✅ OK - Simple strings
echo "Starting process..."
echo ""
echo "Done"

# ❌ NOT OK - Escape sequences
echo -e "Done\n"  # Use printf instead
```

### Real Examples from Fixes

**Before (demo.sh)**:
```bash
log_success() {
  echo -e "\033[32m✓\033[0m $1"
}
```

**After**:
```bash
log_success() {
  printf "\033[32m✓\033[0m %s\n" "$1"
}
```

---

## Platform-Specific Commands

### 1. stat Command (BSD vs GNU)

The `stat` command has **completely different syntax** on macOS/BSD vs Linux.

```bash
# ❌ WRONG - Will fail on macOS
perms=$(stat -c "%a" "$file")

# ❌ WRONG - Will fail on Linux
perms=$(stat -f "%OLp" "$file")

# ✅ RIGHT - Use safe wrapper
perms=$(safe_stat_perms "$file")
```

**Implementation** (in `src/lib/utils/platform-compat.sh`):
```bash
safe_stat_perms() {
  local file="$1"
  if stat --version 2>/dev/null | grep -q GNU; then
    stat -c "%a" "$file"  # GNU stat (Linux)
  else
    stat -f "%OLp" "$file"  # BSD stat (macOS)
  fi
}

safe_stat_mtime() {
  local file="$1"
  if stat --version 2>/dev/null | grep -q GNU; then
    stat -c %Y "$file"  # GNU stat
  else
    stat -f %m "$file"  # BSD stat
  fi
}
```

### 2. date Command (BSD vs GNU)

Date parsing differs significantly:

```bash
# ❌ WRONG - GNU only
epoch=$(date -d "2023-01-01" +%s)

# ✅ RIGHT - Platform detection
if [[ "$(uname)" == "Darwin" ]]; then
  epoch=$(date -j -f "%Y-%m-%d" "2023-01-01" +%s)  # macOS
else
  epoch=$(date -d "2023-01-01" +%s)  # Linux
fi
```

### 3. timeout Command

**CRITICAL**: `timeout` doesn't exist on macOS by default!

```bash
# ❌ WRONG - Fails on macOS with exit code 127
timeout 5 some_command

# ✅ RIGHT - Check availability first
if command -v timeout >/dev/null 2>&1; then
  timeout 5 some_command
elif command -v gtimeout >/dev/null 2>&1; then
  gtimeout 5 some_command  # macOS with coreutils installed
else
  # Run without timeout or skip test gracefully
  some_command
fi
```

**Real Example from test-init.sh**:
```bash
test_check_dependencies() {
  local result

  if command -v timeout >/dev/null 2>&1; then
    timeout 2 bash -c "$test_cmd" && result=0 || result=$?
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout 2 bash -c "$test_cmd" && result=0 || result=$?
  else
    # No timeout available - run directly
    bash -c "$test_cmd" && result=0 || result=$?
  fi

  # Handle result...
}
```

### 4. sed -i (In-place Editing)

```bash
# ❌ WRONG - Different syntax on macOS vs Linux
sed -i 's/foo/bar/' file.txt

# ✅ RIGHT - Use safe wrapper
safe_sed_inline "$file" 's/foo/bar/'
```

**Implementation**:
```bash
safe_sed_inline() {
  local file="$1"
  shift
  if [[ "$OSTYPE" == "darwin"* ]]; then
    sed -i '' "$@" "$file"  # macOS needs empty string
  else
    sed -i "$@" "$file"     # Linux doesn't
  fi
}
```

### 5. readlink -f (Canonical Path)

```bash
# ❌ WRONG - macOS doesn't have readlink -f
realpath=$(readlink -f "$path")

# ✅ RIGHT - Use safe wrapper or manual resolution
realpath=$(cd "$(dirname "$path")" && pwd)/$(basename "$path")

# OR use the safe wrapper:
realpath=$(safe_readlink "$path")
```

---

## Pre-Commit Checklist

Before committing **any** shell script changes:

### 1. Check for echo -e
```bash
grep -r 'echo -e' src/lib/init/ src/cli/init.sh src/tests/
# Should return NOTHING (or only comments)
```

### 2. Check for Bash 4+ Features
```bash
# Associative arrays
grep -r "declare -A" src/lib/init/ src/cli/init.sh

# Uppercase/lowercase expansion
grep -r '\${[^}]*\^\^[^}]*}' src/lib/init/
grep -r '\${[^}]*,,[^}]*}' src/lib/init/

# mapfile/readarray
grep -rE '\b(mapfile|readarray)\b' src/lib/init/

# All should return NOTHING
```

### 3. Check for Platform-Specific Commands
```bash
# Unguarded stat usage
grep -r 'stat -c' src/lib/init/
grep -r 'stat -f' src/lib/init/

# Should use safe_stat_perms() or safe_stat_mtime()
```

### 4. Run ShellCheck (if available)
```bash
shellcheck -S error src/lib/init/**/*.sh src/cli/init.sh
```

### 5. Test Locally
```bash
# Run unit tests
bash src/tests/unit/test-init.sh

# Check for errors
echo $?  # Should be 0
```

---

## CI/CD Requirements

### Workflow Trigger Configuration

**CRITICAL**: Include test files in workflow paths!

```yaml
# .github/workflows/test-init.yml
on:
  push:
    paths:
      - 'src/cli/init.sh'
      - 'src/lib/init/**'
      - 'src/tests/unit/test-init.sh'  # ← MUST include!
      - '.github/workflows/test-init.yml'
```

**Why**: Without this, changes to tests won't trigger CI runs!

### Writing Portable Tests

Tests must be **environment-tolerant**:

```bash
# ❌ BAD - Fails on environment differences
test_something() {
  result=$(some_command)
  assert_equals "expected" "$result"  # Strict assertion
}

# ✅ GOOD - Handles environment quirks
test_something() {
  local result
  result=$(some_command 2>/dev/null) || result=$?

  if [[ $result -eq 127 ]]; then
    return 0  # Command not found - skip gracefully
  fi

  if [[ -n "$result" ]]; then
    assert_equals "expected" "$result"
  else
    return 0  # Environment issue - skip
  fi
}
```

### CI Test Matrix

All code must pass on:

1. ✅ **Ubuntu Latest** (Bash 5.x, GNU tools)
2. ✅ **Ubuntu with Bash 3.2** (Legacy compatibility)
3. ✅ **macOS Latest** (Bash 3.2, BSD tools)

**Integration tests** are more critical than unit tests:
- Unit tests can skip on environment issues
- Integration tests must validate actual functionality
- If integration tests pass, the code works

---

## Common CI Failure Patterns

### Pattern 1: echo -e Usage

**Symptom**: Portability Check fails
**Error**: `WARNING: Found echo -e usage`

**Fix**:
```bash
# Find all instances
grep -rn 'echo -e' src/lib/init/ src/cli/init.sh src/tests/

# Replace with printf
# Before: echo -e "Message\nLine 2"
# After:  printf "Message\nLine 2\n"
```

### Pattern 2: Bash 4+ Features

**Symptom**: Portability Check fails
**Error**: `ERROR: Found lowercase expansion (Bash 4+)`

**Fix**:
```bash
# Before
[[ "${response,,}" == "y" ]]

# After
response=$(echo "$response" | tr '[:upper:]' '[:lower:]')
[[ "$response" == "y" ]]
```

### Pattern 3: macOS stat Failure

**Symptom**: Unit Tests (macOS) fail
**Error**: `stat: illegal option -- c`

**Fix**:
```bash
# Before
perms=$(stat -c "%a" "$file")

# After
source "$(dirname "${BASH_SOURCE[0]}")/../utils/platform-compat.sh"
perms=$(safe_stat_perms "$file")
```

### Pattern 4: Missing timeout

**Symptom**: Unit Tests (macOS) fail with exit code 127
**Error**: `bash: timeout: command not found`

**Fix**: See timeout section above - always check command availability

### Pattern 5: Workflow Not Triggering

**Symptom**: CI doesn't run after push
**Cause**: Changed file not in workflow `paths:` filter

**Fix**: Add file path to `.github/workflows/test-init.yml`

---

## Platform Compatibility Utilities

### Available Functions

**File**: `src/lib/utils/platform-compat.sh`

```bash
# Source at top of file
source "$(dirname "${BASH_SOURCE[0]}")/../utils/platform-compat.sh"

# Then use:
safe_sed_inline()      # Cross-platform sed -i
safe_readlink()        # Cross-platform realpath
safe_mktemp()          # Cross-platform temp files
safe_date()            # Cross-platform date formatting
safe_stat_mtime()      # File modification time (BSD/GNU)
safe_stat_perms()      # File permissions (BSD/GNU) ← Added Oct 2025
safe_grep_extended()   # Extended regex grep
is_macos()             # Platform detection
is_linux()             # Platform detection
is_wsl()               # WSL detection
```

### Usage Example

```bash
#!/usr/bin/env bash

# Source compatibility utilities
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$SCRIPT_DIR/../utils/platform-compat.sh"

# Now use platform-safe functions
perms=$(safe_stat_perms ".env")
mtime=$(safe_stat_mtime "config.yml")

if is_macos; then
  echo "Running on macOS with BSD tools"
elif is_linux; then
  echo "Running on Linux with GNU tools"
fi
```

---

## Success Metrics

A successful cross-platform implementation has:

### ✅ All 12 CI Tests Passing

1. ShellCheck Linting (error-level only)
2. Portability Check (no Bash 4+ features, no echo -e)
3. Unit Tests (Ubuntu latest)
4. Unit Tests (Ubuntu Bash 3.2)
5. Unit Tests (macOS latest)
6. Integration Tests (Ubuntu basic)
7. Integration Tests (Ubuntu force)
8. Integration Tests (Ubuntu wizard)
9. Integration Tests (macOS basic)
10. Integration Tests (macOS force)
11. Integration Tests (macOS wizard)
12. File Permissions Test

### ✅ No Platform-Specific Failures

- Works on macOS with Bash 3.2 and BSD tools
- Works on all major Linux distributions
- Works in WSL environments
- No hardcoded GNU-specific flags
- No hardcoded BSD-specific flags

### ✅ Portable Code Patterns

- All formatted output uses `printf`
- All stat commands use safe wrappers
- All date commands have platform detection
- All external commands check availability before use
- No Bash 4+ features anywhere in codebase

### ✅ Resilient Tests

- Unit tests handle missing commands gracefully
- Tests skip on environment quirks instead of failing
- Integration tests validate actual functionality
- Tests pass on all three CI platforms

---

## Quick Reference

### DO ✅

- Use `printf` for all formatted output
- Check command availability before use
- Use `safe_*` wrappers from platform-compat.sh
- Test on both macOS and Linux
- Handle environment differences in tests
- Run shellcheck before committing
- Verify workflow triggers include changed files

### DON'T ❌

- Use `echo -e` (not portable)
- Use Bash 4+ features (`${var,,}`, `declare -A`, etc.)
- Assume commands exist (`timeout`, `readlink -f`, etc.)
- Use GNU-specific flags without platform checks
- Use BSD-specific flags without platform checks
- Write tests that fail on environment quirks
- Forget to source platform-compat.sh when needed

---

## Additional Resources

- [platform-compat.sh](https://github.com/nself-org/cli/blob/main/src/lib/utils/platform-compat.sh) - Compatibility utilities (source code)
- [test-init.yml](https://github.com/nself-org/cli/blob/main/.github/workflows/test-init.yml) - CI workflow configuration (source code)
- [CONTRIBUTING.md](CONTRIBUTING.md) - General contribution guidelines
- [Bash 3.2 Documentation](https://www.gnu.org/software/bash/manual/bash-3.2.0.html)

---

**Questions?** Open an issue at https://github.com/nself-org/cli/issues
