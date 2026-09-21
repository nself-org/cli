# nself deploy

<!-- BEGIN PROSE:summary -->
> Deploy the stack to a target environment (local, staging, production).
<!-- END PROSE:summary -->

## Synopsis

```
nself deploy [target] <subcommand> [flags]
```

## Description

<!-- BEGIN PROSE:description -->
`nself deploy` builds your stack and deploys it to a target environment using a per-service
sequenced rolling restart. It chains `nself build` then restarts services in dependency order,
waiting for each service to pass a health check before restarting the next.

The restart order is **resolved at deploy time from the project's own compose file**
(`docker compose config --services`), never from a fixed guess list — a project's actual
service names vary (e.g. object storage is always named `minio`, never `storage`; there is no
literal `plugins` service). See [Rolling Restart, Service Order and Downtime](#rolling-restart-service-order-and-downtime).

The target environment can be supplied as a positional argument or via `--env`. The flag takes
priority when both are given. The three supported values are `local`, `staging`, and `prod`
(also accepted as `production`).

When `NSELF_DEPLOY_HOST_STAGING` or `NSELF_DEPLOY_HOST_PROD` is set, the CLI rsyncs the compose
file and env to the remote host, pulls updated images, then runs the rolling restart via SSH.
When no host is configured, the deploy runs on the current host (single-region model).

When `.nself/control-plane.yaml` is present (or `--server` is passed), `nself deploy` routes
through the topology-aware pipeline (`controlplane.Run`). The pipeline reads each server's
role and SSH capability, skips `read-only` and `hidden` servers automatically, and returns a
non-zero exit code if the primary app server was skipped. Without that file and without
`--server`, behavior is byte-identical to the legacy single-host path.

Targets accept both short and long forms:

| Target | Accepts | Description |
|---|---|---|
| local | `local` | Build and rolling-restart on this machine |
| staging | `staging` | Staging environment (uses `NSELF_DEPLOY_HOST_STAGING` if set) |
| prod | `prod`, `production` | Production (uses `NSELF_DEPLOY_HOST_PROD` if set; requires `--force` or `--dry-run`) |

## Deploy Strategies

| Strategy | Status | Behavior |
|---|---|---|
| `rolling` | **Default (v1.1.1)** | Per-service sequenced restart with health-gating. Each service waits up to 60s for `service_healthy` before the next restarts. |
| `blue-green / canary` | **Available via `--canary N` (Y17)** | Zero-downtime: green containers run alongside blue, Nginx shifts N% traffic to green during soak, then promotes to 100%. Requires `NSELF_FEATURE_BLUE_GREEN_DEPLOY=true`. |
| `preview` | Not yet implemented | Falls back to rolling with an explicit warning. |

The `--strategy=blue-green` and `--strategy=canary` flags still fall back to rolling for backwards
compatibility with existing scripts. Use `--canary N` to activate the implemented blue/green path.

See [[cmd-deploy]] for the full per-strategy behavior spec and downtime expectations.

## Blue/Green Canary Deploy (Y17)

Enable with `NSELF_FEATURE_BLUE_GREEN_DEPLOY=true` (feature flag `blue_green_deploy` via `nself flag list`).

### How it works

```
1. Pull new images tagged as green
2. Start green containers (docker compose -p nself-green up -d)
3. Health check green (30s timeout)
4. Nginx upstream: route N% to green (via upstream weights)
5. Canary soak period (default 5 min)
   - Monitor error rate from green containers
   - If error_rate > NSELF_CANARY_ERROR_THRESHOLD (default 1%) → auto-rollback
6. Promote Nginx upstream to 100% green
7. Health check green at 100%
8. Stop and remove blue containers
9. Green becomes blue for the next deploy
```

### Port assignment

| Color | Port offset | Example (Hasura) |
|---|---|---|
| Blue | 0 (base ports) | 8080 |
| Green | +100 | 8180 |

### Examples

```bash
# Canary at 10% (requires NSELF_FEATURE_BLUE_GREEN_DEPLOY=true)
nself deploy local --canary 10

# Canary at 25%
nself deploy staging --canary 25 --force

# Skip canary — flip directly to 100% green
nself deploy local --skip-canary

# Promote a canary to 100% after manual review
nself deploy promote

# Rollback to blue in < 5 seconds
nself deploy rollback local
```

### Backward-incompatible migrations

If a pending migration is not safe to run during canary (DROP COLUMN, DROP TABLE, RENAME COLUMN,
ALTER COLUMN TYPE, NOT NULL without DEFAULT), the deploy is blocked:

```
ERROR: Migration 0042_drop_column_old_name.sql is not backward-compatible.
Run with --force-migration to apply (disables canary, full downtime deploy).
```

Use `--force-migration` to proceed with a full downtime deploy.

### State file

Blue/green state is persisted to `.nself/bluegreen/state.json`. Run `nself deploy status --blue-green`
to inspect the current active environment and canary traffic split.

## Rolling Restart, Service Order and Downtime

The rolling strategy restarts services in dependency order. Each service restart is health-gated
(max 60s wait). If a service does not become healthy within 60s, the deploy halts and reports
which service failed.

**The order is derived from the project's own resolved compose file, not a fixed list.** Before
each restart, `nself deploy` runs `docker compose config --services` (locally, or over SSH for a
remote push target) to get the set of services that actually exist, then orders them:

1. **Core services first**, in dependency order, but only the ones actually present:
   `postgres` → `hasura` → `auth`.
2. **Every other present service next**, in the order docker reported them (e.g. `minio`,
   `nginx`, `redis`, `mailpit`, `ping-api`, `auth-server`, `nself-admin` — whatever this
   project's compose actually defines).
3. **Plugin-contributed services last** — any service name that comes from an installed
   plugin's `docker-compose.plugin.yml` fragment restarts after everything else.

A service name is never invented. Restarting a name that doesn't exist in this project's
compose (the old fixed order included `storage` and `plugins`, neither of which is a real
compose service name — object storage is always `minio`) used to abort the deploy with
`no such service: storage` *after* earlier services had already been recreated, leaving the
stack half-restarted. This is why the order can no longer be hardcoded.

Example for a project whose compose defines `postgres hasura auth auth-server mailpit minio
ping-api nginx redis nself-admin` (no `storage`, no `plugins`):

| Service | Restart order | Expected downtime |
|---|---|---|
| postgres | 1st | 5–30s (WAL recovery time) |
| hasura | 2nd | 0–10s (waits for postgres) |
| auth | 3rd | 0–5s |
| auth-server, mailpit, minio, ping-api, nginx, redis, nself-admin | 4th onward, in compose order | 0–5s each |
| *(any installed plugin's service)* | last | 0–5s per plugin |

Total deploy time scales with the number of services actually present (roughly ~15s each).
Total user-visible downtime is lower than the deploy duration because each service continues
serving until its replacement becomes healthy.

`--dry-run` cannot query a live `docker compose config --services` (nothing has been built or
pulled yet), so it prints the resolution rule instead of a concrete list — the real order is
only known once the build for this deploy has run.

Use `--skip-health-check` to bypass the 60s gate for known-slow services like MeiliSearch or
Grafana. Set `HEALTHCHECK_TIMEOUT_<SERVICE>` (e.g. `HEALTHCHECK_TIMEOUT_GRAFANA=120s`) to
extend the per-service timeout.

### Custom service env vars in a dry-run

`--dry-run` also prints, for each declared custom service (`CS_1`..`CS_10`), the full list of
environment variable **names** it will receive: the fixed core set (project identity, Postgres,
Hasura, Auth connection info) plus whatever `CS_N_ENV_PASSTHROUGH` and `CS_N_ENV` add. Values
are never printed. This exists so a plain addition to `.env.prod` doesn't get mistaken for
something a custom service will automatically see — it only reaches the service's container if
that service's `CS_N_ENV_PASSTHROUGH` allowlist names it, or `CS_N_ENV` sets it explicitly. See
[Config: Custom Services](Config-Custom-Services.md) for the full precedence rules.

## Remote Push

Set `NSELF_DEPLOY_HOST_STAGING` or `NSELF_DEPLOY_HOST_PROD` to a `user@host:/remote/path`
value to enable remote deployments. The CLI:

1. rsyncs `docker-compose.yml` and `.env.<target>` to the remote
2. Runs `docker compose pull` on the remote to fetch updated images
3. Runs the rolling restart on the remote via SSH

```bash
export NSELF_DEPLOY_HOST_STAGING=ubuntu@167.235.233.65:/opt/nself-staging
nself deploy staging
```

SSH key defaults to `~/.ssh/id_ed25519`. Override with `NSELF_DEPLOY_SSH_KEY`.

Agent forwarding is disabled by default. The CLI uses
`-o ForwardAgent=no -o StrictHostKeyChecking=accept-new` for all SSH connections.

- `nself deploy environments`, list all environments and per-server SSH capabilities (JSON)
- `nself deploy status [--blue-green] [--server <name>]`, report current deploy state; `--server` filters to one server; `--blue-green` adds slot info
- `nself deploy rollback [target]`, roll back the last deployment (see below)
- `nself deploy promote`, flip Nginx to 100% green after a manual canary review
- `nself deploy logs [target] [--server <name>]`, tail Docker logs; `--server` streams logs from a specific server via SSH
- `nself deploy health [target] [--server <name>] [--json]`, run `nself doctor`; `--server` runs doctor over SSH on that server
- `nself deploy check-access` **(deprecated)**, verify `NSELF_DEPLOY_HOST_*` values resolve — use `nself env target probe` instead
### nself deploy environments
Lists every environment defined in `.nself/control-plane.yaml` (or synthesized from
`NSELF_DEPLOY_HOST_<TARGET>` env vars) and the resolved SSH capability of each server.
No key material appears in the output.
```bash
nself deploy environments
```
Output:
```json
{
  "environments": [
    {
      "name": "local",
      "kind": "local",
      "servers": [
        {"name": "local", "role": "app", "capability": "manage"}
      ]
    },
    {
      "name": "staging",
      "kind": "remote",
      "servers": [
        {"name": "staging-app", "role": "app", "capability": "manage"},
        {"name": "staging-lb",  "role": "lb",  "capability": "manage"}
      ]
    },
    {
      "name": "prod",
      "kind": "remote",
      "servers": [
        {"name": "prod-app", "role": "app", "capability": "manage"},
        {"name": "prod-lb",  "role": "lb",  "capability": "read-only", "reason": "ssh: permission denied"}
      ]
    }
  ]
}
```
Capability values:
The `reason` field appears only when capability is not `manage`.
This command fixes a latent Admin 500 error: the Admin UI calls this endpoint to populate the
server-selector before starting a deploy.
### nself deploy status --server
When `--server <name>` is passed and `.nself/control-plane.yaml` exists, the output includes
a per-server capability section alongside the existing state:
```json
{
  "target": "staging",
  "state": "running",
  "servers": [
    {
      "env": "staging",
      "server": "staging-app",
      "role": "app",
      "capability": "manage",
      "latency_ms": 42
    },
    {
      "env": "staging",
      "server": "staging-lb",
      "role": "lb",
      "capability": "manage",
      "latency_ms": 38
    }
  ]
}
```
### nself deploy logs --server
Streams logs from a specific server over SSH:
```bash
nself deploy logs staging --server staging-app
```
The SSH connection uses `BatchMode=yes -o ForwardAgent=no -o StrictHostKeyChecking=accept-new`.
Key material is never logged.
### nself deploy health --server
Runs `nself doctor` on a specific server over SSH and streams the output:
```bash
nself deploy health staging --server staging-app
nself deploy health staging --server staging-app --json
```
With `--json`, the remote `nself doctor --json` is called and the structured output is passed
through to stdout.
### nself deploy check-access (deprecated)
**Deprecated.** Use `nself env target probe` for per-server SSH capability resolution.
`check-access` continues to work for backward compatibility. When `.nself/control-plane.yaml`
is present, it probes all servers and emits a deprecation warning, then outputs:
```json
{
  "servers": [
    {"name": "staging-app", "env": "staging", "role": "app", "capability": "manage", "ok": true},
    {"name": "prod-app",    "env": "prod",    "role": "app", "capability": "manage", "ok": true}
  ],
  "all_ok": true
}
```
Without a `control-plane.yaml`, it falls back to the legacy `NSELF_DEPLOY_HOST_*` env-var check
with the same JSON schema.
## Rollback

`nself deploy rollback` is wired to the last `nself promote` tag. Every `nself promote <src> <target>`
call creates a backup snapshot tagged `pre-promote-<target>-<unix>` and writes a promotion record
to `.nself/promotions/<id>.json`. Rollback reads the most recent record and restores the env,
migration state, and Hasura metadata from that snapshot.

**Rollback does not roll back container images.** If you need an older image, pull it manually
(`docker pull nself/...:v1.0.8`) before running rollback.

```bash
# After a failed or unwanted production promotion:
nself deploy rollback prod
# Finds last pre-promote-prod-* backup tag
# Runs: nself backup restore pre-promote-prod-<ts>
# Prints: Rollback for prod completed — prior promote state restored
```

When no promote history exists:

```
Error: rollback failed: find backup: no promotion records found
Run 'nself promote staging' before a deploy to create a rollback point.
```

When the last promote tag is older than 7 days, rollback prompts for confirmation. A stale rollback
may re-apply migrations that no longer match the current code. Review the diff shown before confirming.

**Requires:** `nself promote` must have run at least once against the target environment to create
a rollback point. On a fresh production host, run `nself promote staging prod` before the first
`nself deploy production` to ensure a rollback baseline exists.

## Health Checks

After each deploy (unless `--skip-health` is set), the CLI calls `nself doctor` and checks the
output. If any service is `unhealthy` or has not started within 60s, the deploy result is `failed`
and the specific service name is reported:

```
  [running] Health checks (calling nself health)
  [failed] Health check failed (service: hasura). Run 'nself doctor --verbose' for details.
```

Use `--skip-health` as a break-glass escape hatch. A visible warning is always emitted when used.

## Output format

When `--json` is set, the command writes a structured result:

```json
{
  "target": "staging",
  "strategy": "rolling",
  "steps": [
    {"name": "Build images", "status": "done"},
    {"name": "Restart postgres", "status": "done"},
    {"name": "Restart hasura", "status": "done"},
    {"name": "Restart auth", "status": "done"},
    {"name": "Restart minio", "status": "done"},
    {"name": "Restart nginx", "status": "done"},
    {"name": "Health checks", "status": "done"}
  ],
  "durationMs": 78234,
  "success": true
}
```

The `"Restart <service>"` step names shown above are an example, not a fixed list — they mirror
whatever this project's own compose file actually defines, per [Rolling Restart, Service Order
and Downtime](#rolling-restart-service-order-and-downtime).

Human output uses the same step vocabulary (`done`, `failed`, `skipped`, `running`, `pending`,
`unhealthy`) inside `[...]` brackets so the Admin UI's deploy API route can parse it without
a separate protocol.

## Environment variables

| Var | Default | Purpose |
|---|---|---|
| `NSELF_DEPLOY_HOST_STAGING` | — | SSH/rsync target for staging: `user@host:/path` |
| `NSELF_DEPLOY_HOST_PROD` | — | SSH/rsync target for production: `user@host:/path` |
| `NSELF_DEPLOY_HOST` | — | Generic remote host used when target-specific vars are unset: `user@host:/path` |
| `NSELF_DEPLOY_USER` | — | SSH user for remote deployments (used when host is specified without a user prefix) |
| `NSELF_DEPLOY_KEY_PATH` | `~/.ssh/id_ed25519` | SSH private key path for remote deployments (alias: `NSELF_DEPLOY_SSH_KEY`) |
| `STAGING_DEPLOY_HOST` | — | Fallback alias for `NSELF_DEPLOY_HOST_STAGING` |
| `PROD_DEPLOY_HOST` | — | Fallback alias for `NSELF_DEPLOY_HOST_PROD` |
| `NSELF_DEPLOY_SSH_KEY` | `~/.ssh/id_ed25519` | SSH key path (superseded by `NSELF_DEPLOY_KEY_PATH`; both accepted) |
| `HEALTHCHECK_TIMEOUT_<SERVICE>` | `60s` | Per-service health-check timeout |
| `NSELF_FEATURE_BLUE_GREEN_DEPLOY` | `false` | Enable blue/green canary path (Y17). Set to `true` to activate. |
| `NSELF_DEPLOY_STRATEGY` | `canary` | Deploy strategy when blue/green is active: `canary`, `blue-green`, `direct` |
| `NSELF_CANARY_PERCENT` | `10` | Initial canary traffic % (used when `--canary` is not passed) |
| `NSELF_CANARY_SOAK_MINUTES` | `5` | Soak duration before auto-promote |
| `NSELF_CANARY_ERROR_THRESHOLD` | `1.0` | Error rate % that triggers auto-rollback |
| `NSELF_DEPLOY_HEALTH_TIMEOUT` | `30` | Seconds to wait for green container health |
| `NSELF_BLUE_PORT_OFFSET` | `0` | Port offset for blue containers |
| `NSELF_GREEN_PORT_OFFSET` | `100` | Port offset for green containers |
| `NSELF_DEPLOY_ENV` | `production` | Deploy target environment set by `nself deploy` after resolving `--env` / positional argument. Values: `local`, `staging`, `production`. Exposed for subprocesses and plugins. |

When no host is configured, the CLI deploys to the current host. This is the
single-region model. Multi-region orchestration is available via the dedicated `nself region` command (`list`, `add`, `status`, `promote`).

## Maintenance Banner

`--maintenance-banner` is deferred to a future release (DEP-07). To show a maintenance page manually,
configure an nginx static page via `nginx/conf.d/`.

## Safety

- Production deploys without `--force` are rejected with a clear message
- Use `--dry-run` first to preview steps on any target
- Agent forwarding is disabled by default for all SSH connections (`ForwardAgent=no`)
- SSH connections use `BatchMode=yes StrictHostKeyChecking=accept-new`
- SSH keys are never logged or included in command output
- `--rollback` validates the promote tag's env against current code before applying; drift
 causes an explicit error rather than a silent misapply
- Primary-server skip gate: when the topology pipeline detects that the primary app server
 has `read-only` capability and was skipped, the command exits non-zero even if other servers
 succeeded. Use `nself deploy environments` to diagnose capability issues before deploying.
- Inline secrets in `--host` or `--key-ref` flags are rejected with an error

## Cross-references

- [[cmd-build|nself build]], generates `docker-compose.yml` and nginx configs
- [[cmd-start|nself start]], boot the stack with health checks
- [[cmd-promote|nself promote]], promote env-to-env with rollback support
- [[cmd-env|nself env target]], probe SSH capability and manage per-server access (`nself deploy check-access` replacement)
- [[cmd-deploy]], full per-strategy spec and downtime expectations
- [[Home]]
<!-- END PROSE:description -->

## Flags

<!-- BEGIN GENERATED:flags -->
| Flag | Default | Description |
|------|---------|-------------|
| `--canary` | `0` | Start a canary deploy at N%% traffic to green (0 = full flip) |
| `--dry-run` | `false` | Preview the deploy without executing |
| `--env` | `""` | Target environment: local\|staging\|prod (overrides positional arg; required env vars: NSELF_DEPLOY_HOST, NSELF_DEPLOY_USER, NSELF_DEPLOY_KEY_PATH) |
| `--exclude-frontends` | `false` | Exclude frontend apps from the deploy |
| `--follow` | `false` | Stream container logs after deploy until Ctrl-C (staging/prod only) |
| `--force-migration` | `false` | Force deploy even with backward-incompatible migrations (disables canary) |
| `--force` | `false` | Skip confirmation prompts (prod requires --confirm or --force) |
| `--include-frontends` | `false` | Include frontend apps in the deploy |
| `--json` | `false` | Emit JSON output |
| `--rolling` | `false` | Alias for --strategy=rolling |
| `--server` | `""` | Deploy to a specific server only (name from control-plane inventory) |
| `--skip-canary` | `false` | Skip canary phase and flip directly to 100% green |
| `--skip-health` | `false` | Skip post-deploy health checks |
| `--strategy` | `rolling` | Deploy strategy: rolling\|blue-green\|canary\|preview |
| `--yes` | `false` | Skip prod confirmation prompt (alias for --force) |
| `--help`, `-h` | — | Show help |
<!-- END GENERATED:flags -->

## Subcommands

<!-- BEGIN GENERATED:subcommands -->
| Name | Description |
|------|-------------|
| `check-access` | Verify access to configured deploy targets (deprecated: use 'nself env target probe') |
| `environments` | List configured deploy environments and server capabilities |
| `health` | Run health checks against a target deployment |
| `logs` | Show deployment logs for a target |
| `promote` | Promote a canary deploy to 100% green traffic |
| `rollback` | Roll back the last deployment for a target |
| `status` | Show deployment status for a target |
| `web` | Build web apps locally and deploy prebuilt output to Vercel |
<!-- END GENERATED:subcommands -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
# Local build + rolling restart (positional form)
nself deploy local

# Local build + rolling restart (flag form)
nself deploy --env local

# List all environments and per-server SSH capabilities
nself deploy environments

# Staging dry-run — shows what would happen without executing
nself deploy staging --dry-run

# Staging dry-run with topology plan (when control-plane.yaml exists)
nself deploy staging --dry-run --server staging-app

# Staging dry-run using --env flag
nself deploy --env staging --dry-run

# Deploy to a single server only
nself deploy staging --server staging-app

# Staging deploy with JSON output
nself deploy staging --json

# Production deploy (rolling)
nself deploy production --force

# Production deploy using --env, skipping the confirmation prompt via --yes
nself deploy --env prod --yes

# Production deploy and stream logs until Ctrl-C
nself deploy --env prod --force --follow

# Production deploy with explicit strategy (blue-green falls back to rolling with warning)
nself deploy production --strategy=blue-green --force

# Roll back to the last promoted state
nself deploy rollback prod

# Per-server status (requires control-plane.yaml)
nself deploy status --server staging-app

# Stream logs from a specific server
nself deploy logs staging --server staging-app

# Run doctor on a specific server
nself deploy health staging --server staging-app --json

# Check SSH access (deprecated — use 'nself env target probe' instead)
nself deploy check-access
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[Commands]] — full command index
- [[Core-Services]] — what a stack is made of
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
