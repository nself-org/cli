#!/usr/bin/env bash
# Synthetic probe gate v1: no Go toolchain or downloaded tools required.
set -euo pipefail
if [ "$(cat fixture.go)" != 'package fixture' ]; then
  echo 'probe fixture: unexpected source'
  exit 2
fi
echo 'probe fixture: expected gate failure'
exit 1
