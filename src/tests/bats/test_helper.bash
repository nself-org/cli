#!/usr/bin/env bash
# test_helper.bash — bats/ subdirectory shim
# Sources the canonical test_helper.bash one level up.
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/../test_helper.bash"
