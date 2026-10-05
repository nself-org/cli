# nself status

<!-- BEGIN PROSE:summary -->
> Show health status of all services.
<!-- END PROSE:summary -->

## Synopsis

```
nself status [SERVICE] [flags]
```

## Description

<!-- BEGIN PROSE:description -->
`nself status` displays the running/stopped state of every service in the ɳSelf stack. For each service it shows the health status (healthy, unhealthy, starting), container name, and response time from the health endpoint. A summary line at the bottom shows the overall healthy count.

You can pass a single service name to show detailed status for that service only. Use `--verbose` to include resource usage (CPU, memory) and uptime. Use `--json` to get machine-readable output suitable for monitoring scripts or dashboards.

Exit codes are meaningful: `0` means all services are healthy, `1` means an error occurred running the checks, and `2` means one or more services are unhealthy.


A service that declares no Docker healthcheck reports as `running` rather than `healthy`, and is counted in the healthy total rather than as unhealthy.

### JSON output

`nself status --json` (or `-j`) writes one JSON document to stdout; everything human goes to stderr.

In v1.4 mode (the default before v1.5.0) it is the bare report and the exit status is `0` whatever the state. With `NSELF_V15=1` (the default from v1.5.0) it is the v1 envelope (see [[JSON-Output]]), `data` is the same report plus `state`, and the exit status carries the state:

```json
{
  "schema_version": "1",
  "command": "status",
  "data": {
    "timestamp": "2026-10-05T12:00:00Z",
    "services": [
      { "name": "postgres", "status": "healthy", "duration": "12ms", "details": "accepting connections" },
      { "name": "hasura", "status": "starting", "duration": "3ms", "details": "health check starting" }
    ],
    "summary": { "total": 2, "healthy": 1, "unhealthy": 1 },
    "state": "transitional"
  }
}
```

| `data.state` | Meaning | Exit status (v1.5 mode) |
|--------------|---------|-------------------------|
| `ok` | every service is healthy | 0 |
| `transitional` | the only services not healthy are still starting | 11 |
| `unhealthy` | at least one service is unhealthy | 10 |

In v1.4 mode the human exit codes stay `2` (unhealthy) and `1` (starting). `NSELF_V15=1 NSELF_JSON_LEGACY=1` prints the bare report (without `state`) for one more minor release; the variable is removed in v1.6.0. The schema is `schemas/commands/status.v1.schema.json`. See [[Exit-Codes]].
<!-- END PROSE:description -->

## Flags

<!-- BEGIN GENERATED:flags -->
| Flag | Default | Description |
|------|---------|-------------|
| `--deep` | `false` | Deep health aggregator (verbose + metrics + resource usage) |
| `--health-only` | `false` | Show only health status |
| `--json`, `-j` | `false` | JSON output |
| `--metrics` | `false` | Show performance metrics |
| `--verbose` | `false` | Show resource usage, uptime |
| `--help`, `-h` | — | Show help |
<!-- END GENERATED:flags -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
# Show status of all services
nself status

# Show status for a specific service
nself status postgres

# JSON output for scripting
nself status --json

# Include resource usage and uptime
nself status --verbose

# Show performance metrics
nself status --metrics
```

**Sample output:**

```
Service         Status      Details
postgres        ✓ healthy   pg_isready (10ms)
hasura          ✓ healthy   /healthz 200 (45ms)
auth            ✓ healthy   /healthz 200 (52ms)
nginx           ✓ healthy   /health 200 (8ms)
redis           ✓ healthy   PONG (3ms)

Summary: 5/5 services healthy
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[Commands]] — full command index
- [[Core-Services]] — what a stack is made of
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
