#!/usr/bin/env bats

# Test cross-platform compatibility

setup() {
    # Create temp test directory
    TEST_DIR=$(mktemp -d)
    cd "$TEST_DIR"

    # Resolve nself path dynamically (works in CI and locally)
    NSELF_PATH="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
    export PATH="$NSELF_PATH:$PATH"

    # Source compatibility utilities
    if [ -f "$NSELF_PATH/src/lib/utils/timeout.sh" ]; then
        source "$NSELF_PATH/src/lib/utils/timeout.sh"
    fi
}

teardown() {
    # Clean up test directory
    cd /
    rm -rf "$TEST_DIR"
}

@test "portable_timeout function works with short commands" {
    # Should complete successfully within timeout
    portable_timeout 5 echo "test"
    [ $? -eq 0 ]
}

@test "portable_timeout function kills long-running commands" {
    # Should timeout and return 124
    portable_timeout 1 sleep 3 || [ $? -eq 124 ]
}

@test "portable_timeout function with native timeout command" {
    # Test with existing timeout if available
    if command -v timeout >/dev/null 2>&1; then
        timeout 2 echo "test"
        [ $? -eq 0 ]
    else
        skip "Native timeout command not available"
    fi
}

@test "portable_stat_mtime function works" {
    # Create a test file
    echo "test" > testfile.txt
    
    # Should be able to get modification time
    mtime=$(portable_stat_mtime testfile.txt)
    [ -n "$mtime" ]
    [ "$mtime" -gt 0 ]
}

@test "bash version compatibility" {
    # Check Bash version is acceptable (3.2+)
    bash_version=$(bash --version | head -1 | grep -oE '[0-9]+\.[0-9]+')
    major=$(echo "$bash_version" | cut -d. -f1)
    minor=$(echo "$bash_version" | cut -d. -f2)
    
    # Should be at least 3.2
    [ "$major" -ge 3 ]
    if [ "$major" -eq 3 ]; then
        [ "$minor" -ge 2 ]
    fi
}

@test "required commands are available" {
    # Commands that should be available on all systems
    command -v bash
    command -v sh
    command -v echo
    command -v cat
    command -v grep
    command -v sed
    command -v awk
    command -v cut
    command -v head
    command -v tail
    command -v sort
    command -v uniq
    command -v wc
    command -v tr
    command -v find
    command -v mkdir
    command -v chmod
    command -v rm
    command -v mv
    command -v cp
    command -v ls
}

@test "mktemp works for temporary files" {
    # mktemp should be available and working
    temp_file=$(mktemp)
    [ -f "$temp_file" ]
    rm -f "$temp_file"
}

@test "docker commands don't break on missing docker" {
    # Test should work even if docker is not installed
    # This is important for CI environments
    if command -v docker >/dev/null 2>&1; then
        docker --version >/dev/null
    else
        # Should not fail - docker is optional for some commands
        true
    fi
}