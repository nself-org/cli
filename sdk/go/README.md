# plugin-sdk-go

[![Go Reference](https://pkg.go.dev/badge/github.com/nself-org/cli/sdk/go.svg)](https://pkg.go.dev/github.com/nself-org/cli/sdk/go)
[![CI](https://github.com/nself-org/cli/sdk/go/actions/workflows/ci.yml/badge.svg)](https://github.com/nself-org/cli/sdk/go/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Shared Go infrastructure for building [nSelf](https://nself.org) plugin services.

Plugin authors import this module to get consistent lifecycle management, structured logging, config loading, HTTP server setup, Postgres pool helpers, Prometheus metrics, and a test harness — without reimplementing boilerplate in every plugin.

```bash
go get github.com/nself-org/cli/sdk/go@latest
```

```
import (
    sdkplugin  "github.com/nself-org/cli/sdk/go/plugin"
    sdklogger  "github.com/nself-org/cli/sdk/go/logger"
    sdkconfig  "github.com/nself-org/cli/sdk/go/config"
    sdkserver  "github.com/nself-org/cli/sdk/go/server"
    sdkmetrics "github.com/nself-org/cli/sdk/go/metrics"
    sdklicense "github.com/nself-org/cli/sdk/go/licensing"
    sdkhttpx   "github.com/nself-org/cli/sdk/go/httpx"
    sdkdb      "github.com/nself-org/cli/sdk/go/db"
    sdktracing "github.com/nself-org/cli/sdk/go/tracing"
    sdktest    "github.com/nself-org/cli/sdk/go/testing"
)
```

## What the SDK gives you

| Package | Responsibility |
| --- | --- |
| `plugin` | `Info`, `Base`, `Plugin` interface, lifecycle |
| `logger` | Standardized `slog.Logger` factory (JSON, `plugin`+`version` attrs) |
| `config` | Env-driven config loader with validation |
| `server` | chi router with `/healthz`, `/readyz`, `/metrics`, `/version` mounted |
| `metrics` | Shared Prometheus registry + universal counters (see [METRICS.md](METRICS.md)) |
| `licensing` | Offline license cache, grace period, skip-verify dev flag |
| `httpx` | HTTP client with retries, timeouts, propagated request-ID |
| `db` | `pgxpool` helpers (connect, health check, migrations hook) |
| `tracing` | OpenTelemetry tracer + request-ID middleware |
| `middleware` | Request-ID, validation helpers |
| `costmeter` | Shared cost accounting for AI plugins |
| `identity` | Ed25519 per-plugin keypair + request signing / verification |
| `remote` | The one SSH/scp/rsync exec funnel: remote path validation, pinned known_hosts, CI option set, `ssh -G` resolver (standard library only) |
| `simharness` | Integration-test fleets of Docker sshd containers: digest-pinned images, unique names, Exec, CopyTo, Pause, Netem, Partition (standard library only; `INTEGRATION=1`) |
| `testing` | Test harness (stub upstreams, metrics assertions, fixtures) |
| `devkit/cmd/new-plugin` | Scaffolding generator for new plugins |

## Quick start

Scaffold a new plugin:

```bash
go run github.com/nself-org/cli/sdk/go/devkit/cmd/new-plugin \
    --name mywidget --tier pro --bundle nClaw --dest paid/mywidget
cd paid/mywidget && go mod tidy && go test ./...
```

The generator writes `plugin.json`, `go.mod`, `cmd/main.go`, `internal/config`,
`internal/server`, a smoke test, `Dockerfile`, `docker-compose.plugin.yml`,
`.air.toml` for hot-reload, and a README.

## Remote execution (`remote`)

`remote` is the single place nSelf runs `ssh`, `scp`, `rsync` and `ssh-keyscan`.
Commands are argv slices (never a shell string built locally), operands follow
`--`, every remote path goes through `ValidateRemotePath`, and processes run with
`EnvAllowlist()`.

```go
opts := append(remote.CISSHFlags(), remote.CIOptions(nodeID, pinFile, ver)...)
out, err := remote.RunArgv(ctx, remote.Target{Dest: "ci@node1", Options: opts}, "nself-ci-agent", "--version")
```

- `CIOptions` is the `-o` set for ssh, scp and `rsync -e` (strict host key
  checking against a pinned file, no forwarding, no multiplexing, no local or
  remote command). `CISSHFlags` (`-T -a -x`) is ssh only, never scp.
- `PinnedHostKeys{Path}` pins keys by alias in a file you choose; it never
  touches `~/.ssh/known_hosts`. `ScanHostKeysSSH` captures a key through ssh
  itself so ProxyJump works; show the fingerprint and get confirmation before
  `Add`.
- `ResolveSSHHost` reads `ssh -G` for hostname, port, user, proxyjump and
  hostkeyalias.
- `nself deploy` keeps its historical argv inside `internal/deploy`; this package has no relaxed mode.
- `Rsync` caller options are an allowlist of self-contained flags (`-az`, `--delete`, `--exclude=x`; never `-e`, `--rsh`, `--files-from`, a bare `--`, or a flag whose value is a separate element). `Target.Options` accepts the D4 block plus `-o` keys ConnectTimeout, ServerAlive*, Port, User, IdentityFile, IdentitiesOnly, BatchMode, Compression, LogLevel, ConnectionAttempts, AddressFamily, PreferredAuthentications, and `-i`, `-p`, `-4`, `-6`, `-q`, `-v`. `RunArgv` needs a POSIX remote shell.

## Docker sshd fleets for integration tests (`simharness`)

`simharness` starts a few sshd containers on a private network and injects
faults. The cli control-plane simulation (`internal/controlplane/sim`) and the
ci plugin's node scenarios use it.

```go
f := simharness.Start(t, simharness.Config{Nodes: []simharness.NodeSpec{
    {Name: "app", Image: simharness.ImageDebian, CapAdd: []string{"NET_ADMIN"}},
    {Name: "lb", Image: simharness.ImageAlpine},
}})
f.Exec(t, "app", "uname", "-s")
f.Netem(t, "app", 80*time.Millisecond, 0)
f.Partition(t, "lb"); f.Heal(t, "lb")
// ssh -i f.KeyPath() -p <f.Node(t, "app").Port> nself@127.0.0.1
```

- **Opt-in.** `Start` skips unless `INTEGRATION=1`. With `INTEGRATION=1` and no
  reachable Docker daemon it fails the test; it never skips.
- **Linux Docker Engine** is the evidence platform. Docker Desktop and Colima run
  the same containers but are not release evidence. Needs the `docker` CLI and,
  for the test host, `ssh`.
- **Images:** `openssh` (default, linuxserver/openssh-server pinned by digest),
  and `debian`, `fedora`, `alpine` sshd images built on first use from the
  Dockerfiles in `simharness/testdata` (bases pinned by digest, tagged by content
  hash). The build installs openssh-server from the distro's package repository.
  A custom image must be a digest-pinned reference. Every node has a non-root
  user `nself`, key auth only, no root login.
- **Parallel safe.** Fleet id, network (`nself-sim-<id>`) and containers
  (`nself-sim-<id>-<name>`) are unique per `Start`, and every object carries the
  labels `org.nself.simharness=1` and `org.nself.simharness.fleet=<id>`.
- **Always removed.** `Close` runs from `t.Cleanup` (also after `t.Fatal`) and from
  a SIGINT/SIGTERM handler; it removes only its own fleet's objects.
- **Faults:** `Pause`/`Unpause` freeze the node's processes; `Netem` adds delay and
  loss on the node's egress (needs `NET_ADMIN` in `NodeSpec.CapAdd` and `tc` in
  the image; the debian, fedora and alpine images have it); `Partition`/`Heal`
  detach and re-attach the node's network. Nothing runs `--privileged`. Ports are
  published on 127.0.0.1 only.
- `Node.Banner` reads the SSH identification line, a readiness and reachability
  probe that a port forwarder cannot fake.

## Hot-reload during development

Each scaffolded plugin ships a `.air.toml`. Install [air](https://github.com/air-verse/air)
and run it from the plugin directory:

```bash
air        # rebuild + restart on file change
```

Or use the SDK helper which installs a default `.air.toml` and starts `air` for
you (falls back to `fswatch` + `go run` if `air` is missing):

```bash
./devkit/tools/dev-watch.sh
```

Pair with `nself dev` to reload plugin code without rebuilding the full
container image.

## Runtime contract

Every plugin built on the SDK satisfies these endpoints:

| Endpoint | Purpose |
| --- | --- |
| `GET /healthz` | Liveness (always 200 while the process is up) |
| `GET /readyz`  | Readiness (plugin-provided; 503 while deps are unhealthy) |
| `GET /metrics` | Prometheus metrics ([METRICS.md](METRICS.md)) |
| `GET /version` | `{"plugin","version","sdk"}` JSON |

See [SCOPE.md](SCOPE.md) for boundary rules (tables, routes, env vars, shared
state) and [COMPATIBILITY.md](COMPATIBILITY.md) for version ranges.

## Versioning

SemVer. `Version` constant lives in [`doc.go`](doc.go). Plugins declare the
minimum SDK they need via `sdk.CheckMinSDK("0.1.0")` at startup and
`minSdkVersion` in their `plugin.json`. See
[COMPATIBILITY.md](COMPATIBILITY.md) for the full guarantees and release
policy.

## Testing

```bash
go test ./...
```

The `testing` package provides:

- `StubUpstream(t, routes)` — canned JSON upstream
- `DoJSONRequest(t, h, ...)` — handler test helper
- `FetchMetrics(t, h)` + `AssertMetricPresent(t, expo, metric)` — metrics assertions
- `AssertHealthEndpoints(t, h)` — SDK contract smoke test

## Requirements

- Go 1.23 or later
- nSelf CLI v1.0.9 or later (for plugin runtime)

## License

MIT. See [LICENSE](LICENSE).
