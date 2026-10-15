# Plugin counts

`nself plugin count` reads the vendored `internal/plugin/count/counts.json`
artifact offline. Use `nself plugin count --json` for the original JSON.

The legacy `scripts/plugin-counts.sh` shim reads a sibling plugins checkout:
no arguments defaults to `--json`. Explicit `--json` and
`--check [--free-only]` stay read-only; writing options are rejected.

Counts Vendor Sync compares the vendored artifact against plugins main.
It ignores `generated_from` provenance and the legacy `generated_at` stamp,
while checking every count and plugin classification. Refresh the snapshot
from a known plugins main commit and update the pinned assertions in
`internal/plugin/count/count_test.go` when the measured counts change.
