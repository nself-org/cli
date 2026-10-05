# Changelog

All notable changes to plugin-sdk-go are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added

- `signing` package (P7-CACHE-06): Ed25519 verification pinned to one purpose
  (plugins, agent, ci-release, ci-node, ci-audit) and optionally one scope, with
  key ids (`KeyID`), revocations, key validity windows and sentinel errors;
  `NewVerifier` over a fixed key set and `NewLookupVerifier` over a caller
  `KeyLookup`; `Signer`, `NewEd25519Signer`, `ParsePKCS8PEM`; strict
  `EncodeSig`/`DecodeSig`; `ParseKeysFile`/`ParseRevokedFile` for
  `.nself/trust/<purpose>.keys` and `.revoked`; DSSE v1 (`PAE`, `Envelope`,
  `SignEnvelope`, `VerifyEnvelope`); `signingtest.NewKey`. Standard library
  only. Signatures match `openssl pkeyutl -sign -rawin` output in base64.
  Key rules from the CRITICAL review: key ids are always `KeyID(purpose, key)`
  (no opt-out; `RequireDerivedID` does not exist), a public key may appear once
  per key set, small-order public keys are refused as `ErrMalformed` at every
  entry point, and `VerifyEnvelope` returns `(payloadType, payload, keyIDs, err)`
  and fails when any signature naming a known key fails (unknown key ids are
  ignored; the most specific error wins).
- `remote` package (P7-NODE-04): one exec funnel for ssh, scp, rsync and
  ssh-keyscan (`Command`, `Run`, `RunArgv`, `Start`, `CopyTo`, `Rsync`,
  `EnvAllowlist`); `RemotePathRe`, `ValidateRemotePath` (charset and no leading
  `-`) and `ValidateCopyPath` (also no `..` segment; used by `CopyTo` and
  `Rsync`); `ValidateAlias`, `ValidateDest` (a `:` only inside an IPv6 literal),
  `ValidateNodeID`; `Rsync` emits its `-e` transport first and accepts only an
  allowlist of self-contained flags; `Target.Options` accepts only the D4 block
  plus an allowlist of `-o` keys, `-i`, `-p`, `-4`, `-6`, `-q`, `-v`; `RunArgv` refuses backslashes and needs a POSIX remote shell;
  `PinnedHostKeys` (atomic, 0600, keyed by alias); `ScanHostKeys`,
  `ScanHostKeysSSH`, `Fingerprint`; `CISSHFlags`, `CIOptions`, `SSHVersion`;
  `ResolveSSHHost` over `ssh -G`. Standard library only. The CLI's
  `internal/deploy` delegates to it with a byte-identical argv.
- `remote.ParseHostSpec` (P7-DEPL-23): the one deploy host grammar,
  `[user@]host[:port]` plus the legacy `[user@]host:/abs/path`, with
  `HostSpec{User, Host, Port, LegacyPath}`, the typed error `*HostSpecError`
  (cli maps it to E484) and the methods `String` (canonical, IPv6 in brackets),
  `Dest`, `SSHArgs` (`[-p N] -- dest`), `SSHOptions` (the port only),
  `Target` (refuses options that carry a different port) and `Validate`. user `[a-z_][a-z0-9_.-]{0,31}`, host a dotted name
  (labels `[A-Za-z0-9_-]`, at most 253 bytes, no empty label, never a leading
  `-`) or a bracketed IPv6 literal, port 1-65535 without sign or leading zero.
  A second `@` or `:`, a bare IPv6 literal (the error says to use brackets), whitespace, control or non-ASCII
  bytes and shell metacharacters are refused. Hand-written scanner, no regular
  expression.

- `simharness` package (P7-NODE-21): Docker sshd fleets for integration tests.
  `Start(t, Config{Nodes []NodeSpec})` returns a `*Fleet` with a unique id,
  network (`nself-sim-<id>`) and containers per call, labelled
  `org.nself.simharness=1` and `org.nself.simharness.fleet=<id>`; one ED25519
  key per fleet (OpenSSH format, `KeyPath`); `Exec`, `CopyTo`, `Restart`,
  `Pause`, `Unpause`, `Netem` (delay and loss, needs `NET_ADMIN`), `Partition`,
  `Heal`, `Close` (from `t.Cleanup` and a SIGINT/SIGTERM handler; removes only
  its own fleet) and `Node.Banner`. Images `openssh` (linuxserver/openssh-server
  by digest), `debian`, `fedora`, `alpine` (embedded Dockerfiles on
  digest-pinned bases, tagged by content hash); an image reference without a
  sha256 digest is refused. Skips without `INTEGRATION=1`; fails when
  `INTEGRATION=1` and Docker is unreachable. `TB` is the testing subset it needs.
  Standard library only. The CLI's `internal/controlplane/sim` delegates to it
  with an unchanged exported API.

### Changed

- `ResolveSSHHost` checks its argument with `ParseHostSpec` instead of
  `ValidateAlias` and runs `ssh -G [-p N] -- [user@]host`: a bare IPv6 literal
  now needs brackets, `host:port` is read as a port, and a legacy `:/path`
  suffix never reaches ssh.
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
