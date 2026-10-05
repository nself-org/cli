# Contributing to ɳSelf CLI

Thank you for your interest in contributing to ɳSelf. This guide covers everything from filing a bug report to landing a complex feature.

## Before You Start

- Read the [Code of Conduct](Code-of-Conduct.md). All contributors are expected to follow it.
- Read the [Governance model](https://github.com/nself-org/cli/blob/main/.github/GOVERNANCE.md), it explains how decisions get made.
- Check [open issues](https://github.com/nself-org/cli/issues) and [Discussions](https://github.com/nself-org/cli/discussions) before opening a new one.

## Reporting Bugs

1. Search [existing issues](https://github.com/nself-org/cli/issues) first. Your bug may already be tracked.
2. Open a new issue using the **Bug Report** template.
3. Include: ɳSelf version (`nself version`), OS, Docker version, exact steps to reproduce, and actual vs expected behaviour.
4. Attach relevant logs: `nself logs` or `nself doctor`.

## Requesting Features

1. Check [existing feature requests](https://github.com/nself-org/cli/issues?q=label%3Afeature).
2. Open a new issue using the **Feature Request** template.
3. Describe the use case, not just the solution. This helps evaluate alternatives.
4. For significant changes, start a [Discussion](https://github.com/nself-org/cli/discussions) first.

## Development Setup

### Requirements

- Go 1.22 or later (`go version`)
- GNU Make
- Docker + Docker Compose (for integration tests)
- `golangci-lint` (optional but recommended: `brew install golangci-lint`)

### First-time setup

```bash
git clone https://github.com/nself-org/cli.git
cd cli
make build          # builds ./nself binary
make test           # runs all unit tests (no Docker required)
make install        # installs binary to /usr/local/bin/nself
```

### Running integration tests

Integration tests require a running Docker environment:

```bash
INTEGRATION=1 make test
```

### Vendored dependencies

This repo vendors all Go dependencies. After changing `go.mod`:

```bash
go mod vendor
git add vendor/
```

## Branching Model

| Branch | Purpose |
|---|---|
| `main` | Latest stable release |
| `feat/xxx` | New features |
| `fix/xxx` | Bug fixes |
| `chore/xxx` | Maintenance (deps, tooling) |
| `docs/xxx` | Documentation only |

Always branch from `main`. Target `main` in your PR.

## Pull Request Process

```bash
# 1. Fork and clone
git clone https://github.com/YOUR-USERNAME/cli.git
cd cli

# 2. Create a branch
git checkout -b feat/my-feature

# 3. Make changes, then verify
make test
make vet
gofmt -l .          # must produce no output

# 4. Commit with a conventional message
git commit -m "feat: add my feature"

# 5. Push and open a PR
git push origin feat/my-feature
```

- Fill in the PR template completely.
- All CI checks must pass before review.
- One review from a CODEOWNER is required to merge (see [CODEOWNERS](https://github.com/nself-org/cli/blob/main/.github/CODEOWNERS)).

## Code Style

- **Format:** `gofmt`, run via `make fmt`. CI fails on unformatted code.
- **Lint:** `golangci-lint run` must pass with zero warnings.
- **Tests:** all existing tests must pass. New features need matching tests.
- **Errors:** wrap with context, `fmt.Errorf("doing X: %w", err)`.
- **Context:** accept `context.Context` as first parameter in I/O functions.
- **No panics** in production code paths. No `os.Exit()` outside `main.go`.
- **User output:** use `internal/ui`, not `fmt.Println` directly.
- **Layering:** imports point down and orphan packages are rejected by `internal/repoqa` tests. See [[Architecture-Guardrails]] before adding a package or a cross-package import.

## Test Coverage

The `Coverage` workflow enforces two kinds of floor, and both fail closed.

- **Whole tree:** total statement coverage must stay at or above `COVERAGE_FLOOR` in `.github/workflows/coverage.yml`.
- **Per package:** every package must stay at or above its line in `.github/coverage-floors.txt`. `tools/covfloor` checks the single `coverage.out` the workflow produces. A package below its floor, a line for a package that has no statements any more, a package with no line, and an unreadable profile or floors file all fail the job.

The floors file is data and holds every package's floor, measured on the Linux runner. This page does not copy the numbers; read the file.

Check locally before pushing:

```bash
go test -mod=vendor -coverprofile=coverage.out ./...
go run -mod=vendor ./tools/covfloor -profile coverage.out -floors .github/coverage-floors.txt -strict
```

Local macOS numbers can differ from Linux (for example `internal/trust`), so the Linux workflow is the one that counts.

Rules:

- A new package adds its line with `go run -mod=vendor ./tools/covfloor -profile coverage.out -floors .github/coverage-floors.txt -write`. It appends the missing packages at the measured value minus 2 points and never changes an existing line.
- Lowering a floor needs a PR justification. Writing the test is preferred over lowering the floor.
- Delete a package's line in the same change that deletes the package.

## Commit Conventions

Use [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:`, new feature
- `fix:`, bug fix
- `chore:`, maintenance (deps, CI)
- `docs:`, documentation only
- `test:`, tests only
- `refactor:`, no behaviour change

Breaking changes: add `!` after type (`feat!:`) and document in the PR body.

## Plugin Development

The CLI loads plugins from `plugins/` (free) and `bundles/` (paid). To add or modify a plugin:

1. Read `.claude/docs/PLUGIN_SYSTEM_SPEC.md`, plugin manifest schema and loader pipeline.
2. Create or update the plugin directory with a valid `plugin.json` manifest.
3. Add integration tests that verify `nself plugin install <name>` succeeds.
4. File a PR to the `plugins` repo, not to `cli` (unless you are changing the plugin loader itself).

See [Plugin Dev Guide](Plugin-Dev-Guide.md) for the complete walkthrough.

## Security Disclosures

Do not open a public issue for security vulnerabilities. Follow the process in [SECURITY.md](https://github.com/nself-org/cli/blob/main/.github/SECURITY.md).

## Translations / Internationalisation

The CLI's internationalisation strategy is under review. Translation contributions are deferred until the strategy decision is documented. Watch [Discussions](https://github.com/nself-org/cli/discussions) for updates.

## Questions

- [GitHub Discussions](https://github.com/nself-org/cli/discussions), preferred for questions
- Community: [nself.org](https://nself.org)

## Related

- [GOVERNANCE.md](https://github.com/nself-org/cli/blob/main/.github/GOVERNANCE.md), decision model
- [ENFORCEMENT.md](https://github.com/nself-org/cli/blob/main/.github/ENFORCEMENT.md), code of conduct enforcement
- [CODEOWNERS](https://github.com/nself-org/cli/blob/main/.github/CODEOWNERS), who reviews what
- [[Dev-Setup]], local environment setup
- [[Plugin-Dev-Guide]], building a new plugin
- [[Release-Process]], release workflow

---
← [[Home]] | [[_Sidebar]]
