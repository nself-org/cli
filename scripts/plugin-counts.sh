#!/usr/bin/env bash
# Retired: this script used to compute free/pro/total plugin counts itself,
# with its own dual-registry-overlap rule. That rule diverged from the rule
# the website used and the rule docs used, so the same day could produce
# three different plugin counts depending which generator ran. The org
# standardised on ONE generated artifact, plugins/counts.json (locked
# schema, produced in the public nself-org/plugins repo by
# plugins/scripts/plugin-counts.sh), and ONE way to read it offline:
# `nself plugin count` (backed by internal/plugin/count, which embeds a
# vendored copy of that artifact — see that package's doc comment for the
# sync TODO).
#
# This script no longer counts anything itself. It only:
#   1. Delegates to the real generator when the plugins repo is checked out
#      as a sibling of this repo (the same sibling-checkout convention the
#      old version of this script used), or
#   2. Points at `nself plugin count`, which works with no sibling checkout
#      and no network call.
#
# It never falls back to re-deriving a count locally — that fallback is
# exactly the divergent-rule bug this retirement fixes.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
siblings="$(dirname "$root")"
generator="$siblings/plugins/scripts/plugin-counts.sh"

if [ -x "$generator" ]; then
  exec "$generator" "$@"
fi

cat >&2 <<'EOF'
plugin-counts.sh has been retired from this repo.

The authoritative generator now lives in nself-org/plugins:
  plugins/scripts/plugin-counts.sh

This shim delegates to it when that repo is checked out as a sibling of
this one ($HOME/Sites/nself/plugins or equivalent). No sibling checkout was
found here.

For plugin counts, run this instead — works fully offline, no checkout or
network required:

  nself plugin count
  nself plugin count --json

See internal/plugin/count/count.go for how that command's data is sourced.
EOF
exit 1
