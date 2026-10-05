# Changelog

All notable changes to plugin-sdk-go are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added

- `remote` package (P7-NODE-04): one exec funnel for ssh, scp, rsync and
  ssh-keyscan (`Command`, `Run`, `RunArgv`, `Start`, `CopyTo`, `Rsync`,
  `EnvAllowlist`); `RemotePathRe`, `ValidateRemotePath` (charset and no leading
  `-`) and `ValidateCopyPath` (also no `..` segment; used by `CopyTo` and
  `Rsync`); `ValidateAlias`, `ValidateDest` (a `:` only inside an IPv6 literal),
  `ValidateNodeID`; `Rsync` accepts only single-element options starting with
  `-`; `RunArgv` refuses backslashes and needs a POSIX remote shell;
  `PinnedHostKeys` (atomic, 0600, keyed by alias); `ScanHostKeys`,
  `ScanHostKeysSSH`, `Fingerprint`; `CISSHFlags`, `CIOptions`, `SSHVersion`;
  `ResolveSSHHost` over `ssh -G`. Standard library only. The CLI's
  `internal/deploy` delegates to it with a byte-identical argv.

### Changed

- **Breaking:** package `license` moved to `licensing`
  (`github.com/nself-org/cli/sdk/go/v2/licensing`). The module zip includes the
  repository-root `LICENSE`, which collides case-insensitively with a `license/`
  package directory, so no version of the module could be fetched. No repository
  imported the old path. `modzip_test.go` now builds the module zip file set and
  fails on any such collision.

## [2.0.0] - 2026-05-13

### Changed

- **Breaking:** module path updated to `github.com/nself-org/cli/sdk/go/v2` per Go modules major-version convention.
  Update import paths in all plugins from `github.com/nself-org/cli/sdk/go` to `github.com/nself-org/cli/sdk/go/v2`.
- `doc.go` `Version` constant bumped to `"2.0.0"`.

### Added

- Publish workflow (`.github/workflows/sdk-go-publish.yml`): triggers on `sdk-go/v*` tag, runs `go mod tidy`, `go vet ./...`, `go test ./...`, verifies module path vs tag major version, triggers proxy index, and creates a GitHub release.

---

## [0.1.0] - 2026-04-23

### Added

Initial public release. Extracted from nSelf private plugin infrastructure.

- `plugin` — `Plugin` interface, `Info` struct, `Base` embed, `Validate()`
- `logger` — JSON `slog.Logger` factory with `plugin` + `version` attrs
- `config` — env-driven config loader with validation helpers
- `server` — chi router with `/healthz`, `/readyz`, `/metrics`, `/version`
- `metrics` — shared Prometheus registry + per-plugin request/error counters
- `license` — offline license cache, grace period, skip-verify dev flag
- `httpx` — HTTP client with retries, timeouts, propagated request-ID header
- `db` — `pgxpool` connect/health/migration helpers
- `tracing` — OpenTelemetry tracer + request-ID middleware
- `middleware` — request-ID injection, common validation helpers
- `costmeter` — shared cost accounting for AI/inference plugins
- `identity` — Ed25519 per-plugin keypair + request signing/verification
- `testing` — test harness: `StubUpstream`, `DoJSONRequest`, `FetchMetrics`, `AssertHealthEndpoints`
- `devkit/cmd/new-plugin` — scaffolding generator for new plugin projects
- `compatibility.go` — `CheckMinSDK` and `CheckCLICompat` runtime guards
- `doc.go` — `Version` constant (`"0.1.0"`)
- CI workflow: test (Go 1.23/1.24), govulncheck, private-import-path security check

### Notes

- Requires Go 1.23 or later.
- Targets nSelf CLI v1.0.9 and newer.
- Zero dependencies on `plugins-pro` private code.

[0.1.0]: https://github.com/nself-org/cli/sdk/go/releases/tag/v0.1.0
