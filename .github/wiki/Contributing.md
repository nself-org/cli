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

## Test Coverage

The CLI has package-level coverage floors enforced by the
`Coverage` workflow. New code in these packages must keep the package above
its floor or CI will fail.

| Package | Floor | Notes |
|---|---|---|
| Whole tree | 25% | Anti-regression gate; total coverage uplift to 75% is a structural follow-up |
| `internal/license` | 60% | Security-critical |
| `internal/auth` | 80% | Security-critical |
| `internal/trust` | 75% | Security-critical (added P97 G0-T11) |
| `internal/ui` | 75% | Added P97 G0-T11 |
| `internal/watchdog` | 75% | Added P97 G0-T11 |

Run coverage locally before pushing:

```bash
go test -mod=vendor -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1
go tool cover -func=coverage.out | grep "internal/trust"   # per-package
```

Floor changes need a PR with a justification line in the workflow comment.
Path A (write tests) is preferred over Path B (lower the floor) per the
CI/CD 100% Green Hard Rule.

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
