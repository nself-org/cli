#!/usr/bin/env bash
# Purpose: catch a documented or scripted `nself <path> --flag` invocation
#          whose flag was never registered on the cobra command it names.
#          PR #258 fixed exactly this for `nself doctor --quick`: the flag
#          was documented and relied on by scripts/golden-path.sh but never
#          added to the doctor command, so every invocation died at the flag
#          parser before RunE ran — unnoticed for over a month because
#          nothing checked docs/scripts against the binary's real flags.
# Inputs:  none. Scans .github/wiki/*.md and scripts/**/*.sh; resolves flags
#          against the live cobra tree via cmd/commands (tools/flagaudit).
# Outputs: exit 0 when every scanned invocation's flags are all registered
#          on the command it names; exit 1 naming the offenders otherwise.
#          Commands the core binary doesn't register at all (plugin-provided,
#          e.g. region/alerts/dr per CLI-R11) are printed as an informational
#          skip list and never fail this gate.
# Constraints: CGO_ENABLED=0, same as every other tools/* generator in this
#              repo, so it needs no cgo toolchain in CI.
set -euo pipefail

cd "$(dirname "$0")/../.."

# Build once and run the binary: `go run` leaves a linked executable in GOCACHE.
CGO_ENABLED=0 go build -mod=vendor -o bin/flagaudit ./tools/flagaudit
bin/flagaudit
