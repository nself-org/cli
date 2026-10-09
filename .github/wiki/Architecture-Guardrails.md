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

Two ratchets freeze the call sites that bypass a shared funnel. Each is a shrink-only allowlist of `<path> <count>` lines, checked by `go test ./internal/repoqa/`.

| Rule | Funnel (exempt dir) | Test | Allowlist | Basis constant |
|---|---|---|---|---|
| D5 | `internal/httptimeout` | `TestHTTPFunnel` | `internal/repoqa/testdata/http-client-allowlist.txt` | `httpBasisTotal` |
| D4 | `internal/docker` | `TestDockerFunnel` | `internal/repoqa/testdata/docker-exec-allowlist.txt` | `dockerBasisTotal` |

**What counts.** The scan parses files with `go/parser` and never builds them.
- D5: a composite literal of type `http.Client` (with or without `&`) and `new(http.Client)`. The file's import name for `net/http` is resolved, so an aliased import counts.
- D4: `exec.Command` or `exec.CommandContext` whose command is the string literal `"docker"`, or an identifier that the same file declares as a `const` or `var` equal to `"docker"`. The `os/exec` import name is resolved.
- Not counted: comments, strings, pointer and parameter types such as a `*http.Client` parameter, other binaries. A zero-value `var c http.Client` is a construction but is not counted (see blind spots).

**Scope.** Every non-test `.go` file of the root module, whatever its build tags or GOOS suffix (a `_windows.go` file is parsed). Skipped: `vendor/`, `testdata/`, dot-directories and any directory with its own `go.mod` (for example `sdk/go`). Paths are normalised to `/` before listing and exempt checks.

**Known blind spots (see debt D-0256 for the same class of gap in the layering guard).**
- `_test.go` files are not scanned: a site in a test file is not counted. The ratchets protect production code only.
- A docker binary path from `exec.LookPath("docker")` is not seen, nor a helper outside `internal/docker` that takes the binary name as a parameter.
- http.Client built through a type alias (`type C = http.Client`), a zero-value `var c http.Client`, a conversion `http.Client(*a)`, or a dot import of `net/http` is not counted. An `http.Transport`-only client is outside this ratchet; `http.DefaultClient` and `http.Get` are left to forbidigo.
- docker named through a local variable (`bin := "docker"`), a const declared in another file or package, a computed string (`"docker"+""`), a `[]string{"docker", ...}` argument slice, an absolute path (`/usr/local/bin/docker`), a shell (`sh -c "docker ..."`), `os.StartProcess`, `syscall.Exec`, or a dot import of `os/exec` is not counted.
- The basis constants stop a silent loosening, not a deliberate one: a PR can raise a line and its constant together. The gate for that is review of any diff to the two allowlists or the `*_funnel_test.go` basis constants.
- A command held in a variable assigned at run time (not a `const`/`var` literal) is not seen.
- `forbidigo` in `.golangci.yml` separately bans `http.DefaultClient`, `http.Get`, `Post`, `PostForm` and `Head`; these ratchets cover only constructed clients.

**Shrink only.** The list may not grow and may not be loosened without an edit of the basis constant in the test, which a reviewer sees. A site that is not listed fails with the file name. A line above the measured count fails with "new sites are rejected". A line below it, or for a file with no sites, fails with "lower the line" or "delete the line". If the list total no longer equals the constant, the test says "lower ...BasisTotal to N". `go test ./internal/repoqa/ -run 'HTTPFunnel|DockerFunnel' -update` lowers lines and drops stale ones; it never adds a line or raises a count, and it prints the new constant to paste.

**Touch a listed file: move its sites onto the funnel and lower the line in the same PR.** Migration is opportunistic (debt D-0034); no Ticket migrates sites just to empty the list.

Before and after for D5:

```go
// before: listed in http-client-allowlist.txt as "internal/foo/client.go 1"
c := &http.Client{Timeout: 30 * time.Second}

// after: the line for this file is deleted
c := httptimeout.WithTimeout(30 * time.Second)
```

Before and after for D4:

```go
// before: listed in docker-exec-allowlist.txt as "internal/foo/ps.go 1"
out, err := exec.CommandContext(ctx, "docker", "logs", "--tail", "50", name).CombinedOutput()

// after: the line for this file is deleted
out, err := docker.GetContainerLogs(ctx, name, 50)
```

Pick the helper that fits the call (`httptimeout.NoProxy`, `docker.ExecCapture`, `docker.RunOneShot`, `docker.InspectContainer`); add one to the funnel package when none fits.

## Domain stdout writers

`TestStdoutWriters` scans non-test Go files under `internal/`, excluding `internal/ui/`, `internal/output/`, and `internal/repoqa/`. It counts `os.Stdout` selectors and `fmt.Print`, `fmt.Printf`, and `fmt.Println` calls by imported package name, including aliases. Build tags do not hide a file from the scan. The tight, bytewise-sorted `<path> <count>` list is `internal/repoqa/testdata/stdout-writers-allowlist.txt`; `stdoutBasisTotal` pins its total. New sites fail the test.

When moving a domain writer behind a shared output seam, run `go test ./internal/repoqa/ -run '^TestStdoutWriters$' -update`. The update lowers or removes existing lines only. Lower `stdoutBasisTotal` to the reported total in the same change. The scan does not follow helper calls or package aliases declared in another file.
