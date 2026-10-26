# Architecture guard coverage

`TestLayering` enforces D1 (L0 imports only L0) and D2 (internal never imports
cmd) on imports parsed from every first-party Go source file. This includes
Windows, Darwin, Linux, other platforms and architectures, arbitrary build tags
(including integration), cgo files, and internal and external test files.
Imports are checked without evaluating build constraints or compiling packages.
Diagnostics identify the source file. Vendor, testdata, hidden and underscore
directories/files, and nested modules are excluded, following Go package scope.

`sourceBasis` pins three existing test violations by exact file/import pair:
config/functions_runtime_test and docker/pull_fallback_test import compose;
mcpgen/main_test imports cmd/commands. Removing or fixing one fails until its
exemption is removed. A new file or import cannot inherit these exemptions.
The baseline keeps this guard change within repoqa while preventing growth.

D3, stale-edge checks, and D9 orphan detection use the existing `go list` graph:
Linux/amd64, cgo disabled, default build tags, vendor mode, production imports.
Imports from other packages' tests also prevent D9 orphan classification.
This graph does not claim D3 or orphan coverage for other build variants.

`layers.yaml` accepted edges must exactly match `acceptedBasis`, including
pending state. Additions, substitutions, duplicates, deletions and pending-tag
changes require a visible test-code change. Architecture review must check that
change against HQ overview section 4.2. Upward edges separately match the
shrink-only `ratchetBasis`.

Plain orphan entries must match `orphanBasis`. No pending orphan exceptions
are approved, so all pending markers fail regardless of ticket status or package
existence. Enabling any future pending exception requires a live ticket-status
check that fails closed in CI; a syntactically valid ticket ID is insufficient.

HQ overview section 4.2 is maintained separately. Its accepted-edge count and
pending description must be reconciled with `acceptedBasis` by the HQ owner;
the CLI tests do not depend on a private sibling checkout in public CI.

## Package size and fan-in

`TestPackageSizeBudget` and `TestPackageFanInBudget` (D-0033) cover what the
300-line file cap cannot see. A package with more than 6000 non-test Go lines,
or one imported by more than 20 other packages of the module, needs a line in
`testdata/package-size-allowlist.txt` or `testdata/package-fanin-allowlist.txt`
and may not grow past it. A new package over either threshold fails until it is
split along domain lines. The lists are ceilings: a shrinking package passes,
and `go test ./internal/repoqa -update` lowers the line or drops it once the
package is under the threshold. The update never adds a line or raises a number.
Fan-in counts distinct importing packages from non-test files only.
