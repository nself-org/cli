# Architecture Guardrails

The CLI's layering rules are enforced by tests in `internal/repoqa/`, so they run wherever `go test ./...` runs: every local run and every leg of the `ci.yml` "Vet, Build & Test" job. Nothing here changes CLI behaviour.

The rules mirror the dependency table in the project's architecture overview (section 4.2, rules D1 to D3 and D9).

## Layers

Every package is classified by path-glob rules in `internal/repoqa/testdata/layers.yaml`. The first matching rule wins, `x/**` matches `x` and everything below it, and any `internal/**` package no rule matches is L1.

| Layer | What it holds | Roots |
|---|---|---|
| L0 | Shared kernel and the contract packages. Imports only L0, the standard library and vendor code | `config`, `errs`, `httptimeout`, `ui`, `ux`, `version`, `ports`, `recover`, `confirm`, `cmdlog`, `observability`, `telemetry`, `featureflags`, `security`, `nginxtopo`, `metrics`, `docker`, `compat`, `output`, `canon`, `cmdregistry`, module-root `schemas/` |
| L1 | Domain capabilities (the default for `internal/**`) | everything not listed under L0 or L2 |
| L2 | Domain orchestrators | `build`, `plugin`, `bundle`, `doctor`, `health`, `deploy`, `controlplane`, `runner`, `backup`, `database`, `hasura`, `migration`, `tenant`, `invoke`, `httpapi`, `mcpgen`, `reconcile`, `builtin`, `importer`, `exporter`, `internal/cmd/ci` |
| L3 | Composition roots. May import anything | `cmd/**`, `tools/**`, `test/**` |
| test-only | `internal/repoqa`. Nothing may import it; exempt from the orphan rule | |

Subpackages inherit: `internal/deploy/bluegreen` is L2, `internal/plugin/verify` is L2.

## Rules

| Rule | Check | Message |
|---|---|---|
| D1 | L0 imports only L0 | `D1: <a> imports <b> (L0 -> L2): L0 may import only L0; ...` |
| D2 | Nothing under `internal/` imports `cmd/` | `D2: <a> imports <b> (...): internal/ must not import cmd/; ...` |
| D3 | Imports point down. An upward edge fails unless it is on the wrong-way ratchet | `D3: <a> imports <b> (L1 -> L2): imports point down; ...` |
| D3 | A same-layer edge (L1 to L1, L2 to L2) between two different domains must be listed | `D3: <a> imports <b> (L2 -> L2): unlisted same-layer edge; ...` |
| D3 | A forbidden edge never exists | `D3: ...: <from> -> <to> is a forbidden edge in layers.yaml` |
| D9 | No orphan packages | `D9: <pkg> has no importer; wire it, delete it, or add "<pkg> pending:<ticket-id>"` |

A **domain** is the first two path segments (`internal/deploy/bluegreen` belongs to `internal/deploy`; `internal/cmd/<x>` uses three). Edges inside one domain are always allowed. `edges`, `ratchet` and `forbidden` entries name domains on both sides, so one entry covers every package pair.

The graph comes from one `go list` call per test binary, with `GOOS=linux GOARCH=amd64 CGO_ENABLED=0` and vendor mode, so the result does not depend on the machine. A package that fails to load fails the test.

An **orphan** is an `internal/**` package, not `main`, not test-only, with no non-test importer and no importer in another package's tests (a package used only by another package's tests is test support).

## Adding a package

Usually nothing. A new `internal/<name>` package is L1 by default and may import L0 and L1 packages, and L1 packages in other domains only through listed edges. You need a `layers.yaml` change only when:

- the package belongs in L0 or L2: add one `internal/<root>/**` rule;
- it creates a same-layer edge between two domains: add `{from: ..., to: ...}` under `edges:` and the same line in the architecture overview in the same change.

Never list `bundle -> reconcile`: it closes a cycle with `build -> bundle`. Call reconcile from `cmd/` instead.

## Orphans and `pending:`

`internal/repoqa/testdata/orphans.txt` records the orphans that exist today. A new orphan fails the test. To create a package ahead of its first consumer, add `<pkg> pending:<ticket-id>` (for example `internal/foo pending:P7-LIVE-03`). A pending line may name a package that does not exist yet, and it goes stale as soon as the package has an importer: delete it then. Pending edges in `layers.yaml` work the same way.

## Ratchets

The lists are tight and shrink-only:

- A line whose package or edge is gone is stale: `D9: <pkg> is listed in orphans.txt but has importers; delete the line (stale)`.
- The plain orphan lines and the wrong-way `ratchet:` edges must equal hard-coded sets in the tests (`orphanBasis` in `orphans_test.go`, `ratchetBasis` in `layering_test.go`). A new orphan or wrong-way edge is rejected; a removed one fails with `drop <X> from orphanBasis` (or `ratchetBasis`). Every shrink edits the constant in the same change, so a list can never refill.
- All list files are sorted bytewise (`LC_ALL=C sort`).

`go test ./internal/repoqa/ -run 'Layering|Orphan' -update` rewrites downward only: it drops stale plain orphan lines and prints the new `orphanBasis`. It never adds a line, raises a count, or rewrites `layers.yaml`, which is edited by hand.

## Repository hygiene

A tracked file must not be a compiled binary: `TestTrackedBinaries` rejects an ELF, Mach-O or PE header, and a `100755` file with NUL bytes. Generate binary fixtures at test time instead. A repoqa test that needs `go` or `git` skips locally when it is missing and fails when `CI` is set.

## Funnels

Funnel ratchets for direct `http.Client` construction and direct `docker` process execution are documented here once they land.
