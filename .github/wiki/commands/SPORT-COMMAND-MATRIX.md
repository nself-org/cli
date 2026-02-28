# nself CLI SPORT Command Matrix

Single Point of Reference and Truth (SPORT) matrix for the runtime CLI surface.

Source audited from:
- `src/cli/*.sh`
- command help output captured on 2026-02-10
- canonical consolidation map in `commands/COMMANDS.md`

---

## Canonical v1 Command Surface (32 Runtime)

These are the canonical top-level commands for v1 structure.

### Core (5)

- [`init`](INIT.md)
- [`build`](BUILD.md)
- [`start`](START.md)
- [`stop`](STOP.md)
- [`restart`](RESTART.md)

### Infrastructure Safety (1)

- [`destroy`](DESTROY.md)

### Utilities (15)

- [`status`](STATUS.md)
- [`logs`](LOGS.md)
- [`help`](HELP.md)
- [`admin`](ADMIN.md)
- [`urls`](URLS.md)
- [`exec`](EXEC.md)
- [`doctor`](DOCTOR.md)
- [`monitor`](MONITOR.md)
- [`health`](HEALTH.md)
- [`version`](VERSION.md)
- [`update`](UPDATE.md)
- [`completion`](COMPLETION.md)
- [`metrics`](METRICS.md)
- [`history`](HISTORY.md)
- [`audit`](AUDIT.md)

### Platform Domains (11)

- [`db`](DB.md)
- [`tenant`](TENANT.md)
- [`deploy`](DEPLOY.md)
- [`infra`](INFRA.md)
- [`service`](SERVICE.md)
- [`config`](CONFIG.md)
- [`auth`](AUTH.md)
- [`perf`](PERF.md)
- [`backup`](BACKUP.md)
- [`dev`](DEV.md)
- [`plugin`](PLUGIN.md)

---

## Canonical Subcommand Namespaces

This section lists the primary subcommand namespaces and syntax entrypoints.

### `db`
- `nself db migrate ...`
- `nself db schema ...`
- `nself db seed ...`
- `nself db mock ...`
- `nself db backup ...`
- `nself db restore ...`
- `nself db inspect ...`
- `nself db types ...`
- `nself db data ...`

### `tenant`
- `nself tenant ...`
- `nself tenant billing ...`
- `nself tenant org ...`
- `nself tenant branding ...`
- `nself tenant domains ...`
- `nself tenant email ...`
- `nself tenant themes ...`
- `nself tenant member ...`
- `nself tenant setting ...`

### `deploy`
- `nself deploy staging`
- `nself deploy production`
- `nself deploy upgrade ...`
- `nself deploy server ...`
- `nself deploy provision ...`
- `nself deploy sync ...`
- `nself deploy rollback`
- `nself deploy status`

### `infra`
- `nself infra provider ...`
- `nself infra k8s ...`
- `nself infra helm ...`

### `service`
- `nself service list|enable|disable|status|restart|logs|init`
- `nself service scaffold|list-templates|template-info|wizard`
- `nself service admin ...`
- `nself service storage ...`
- `nself service email ...`
- `nself service search ...`
- `nself service redis ...`
- `nself service functions ...`
- `nself service mlflow ...`
- `nself service realtime ...`

### `config`
- `nself config show|get|set|list|edit|export|import|sync`
- `nself config env ...`
- `nself config secrets ...`
- `nself config vault ...`
- `nself config validate ...`

### `auth`
- `nself auth login|logout|status`
- `nself auth mfa ...`
- `nself auth roles ...`
- `nself auth devices ...`
- `nself auth oauth ...`
- `nself auth security ...`
- `nself auth ssl ...`
- `nself auth rate-limit ...`
- `nself auth webhooks ...`

### `perf`
- `nself perf profile ...`
- `nself perf bench ...`
- `nself perf scale ...`
- `nself perf migrate ...`
- `nself perf optimize ...`

### `backup`
- `nself backup create|list|restore|verify|prune|clean|rollback|reset|schedule`

### `dev`
- `nself dev mode ...`
- `nself dev frontend ...`
- `nself dev ci ...`
- `nself dev docs ...`
- `nself dev whitelabel ...`
- `nself dev sdk ...`
- `nself dev test ...`

### `plugin`
- `nself plugin list|install|remove|update|updates|refresh|status`
- `nself plugin <plugin> <action>`

---

## Compatibility / Legacy Wrapper Commands

These commands still exist in `src/cli` but are compatibility or older surfaces.

| Legacy Command | Canonical Path | Wrapper Doc |
|---|---|---|
| `billing` | `tenant billing` | [BILLING.md](BILLING.md) |
| `org` | `tenant org` | [org.md](org.md) |
| `upgrade` | `deploy upgrade` | [upgrade.md](upgrade.md) |
| `staging` | `deploy staging` | [STAGING.md](STAGING.md) |
| `prod` | `deploy production` | [PROD.md](PROD.md) |
| `provision` | `deploy provision` | [PROVISION.md](PROVISION.md) |
| `server` / `servers` | `deploy server` | [server.md](server.md), [SERVERS.md](SERVERS.md) |
| `sync` | `deploy sync` or `config sync` | [SYNC.md](SYNC.md) |
| `provider` / `cloud` | `infra provider` | [PROVIDER.md](PROVIDER.md), [cloud.md](cloud.md) |
| `k8s` | `infra k8s` | [K8S.md](K8S.md) |
| `helm` | `infra helm` | [HELM.md](HELM.md) |
| `storage` | `service storage` | [storage.md](storage.md) |
| `email` | `service email` | [EMAIL.md](EMAIL.md) |
| `search` | `service search` | [SEARCH.md](SEARCH.md) |
| `redis` | `service redis` | [redis.md](redis.md) |
| `functions` | `service functions` | [FUNCTIONS.md](FUNCTIONS.md) |
| `mlflow` | `service mlflow` | [MLFLOW.md](MLFLOW.md) |
| `realtime` | `service realtime` | [REALTIME.md](REALTIME.md) |
| `env` | `config env` | [ENV.md](ENV.md) |
| `secrets` | `config secrets` | [secrets.md](secrets.md) |
| `vault` | `config vault` | [vault.md](vault.md) |
| `validate` | `config validate` | [validate.md](validate.md) |
| `mfa` | `auth mfa` | [MFA.md](MFA.md) |
| `roles` | `auth roles` | [roles.md](roles.md) |
| `devices` | `auth devices` | [DEVICES.md](DEVICES.md) |
| `oauth` | `auth oauth` | [OAUTH.md](OAUTH.md) |
| `security` | `auth security` | [security.md](security.md) |
| `ssl` / `trust` | `auth ssl` | [SSL.md](SSL.md), [TRUST.md](TRUST.md) |
| `rate-limit` | `auth rate-limit` | [rate-limit.md](rate-limit.md) |
| `webhooks` | `auth webhooks` | [webhooks.md](webhooks.md) |
| `bench` | `perf bench` | [BENCH.md](BENCH.md) |
| `scale` | `perf scale` | [SCALE.md](SCALE.md) |
| `migrate` | `perf migrate` | [MIGRATE.md](MIGRATE.md) |
| `rollback` | `backup rollback` | [ROLLBACK.md](ROLLBACK.md) |
| `reset` | `backup reset` | [RESET.md](RESET.md) |
| `clean` | `backup clean` | [CLEAN.md](CLEAN.md) |
| `frontend` | `dev frontend` | [FRONTEND.md](FRONTEND.md) |
| `ci` | `dev ci` | [CI.md](CI.md) |
| `docs` | `dev docs` | [docs.md](docs.md) |
| `whitelabel` | `dev whitelabel` | [WHITELABEL.md](WHITELABEL.md) |

---

## Command Coverage Status

Runtime command scripts discovered in `src/cli/*.sh`: `82`.

Breakdown:
- `1` router script: `nself.sh`
- `81` user-invocable commands (includes `help`)
- `80` non-router/non-help commands

Wiki command documentation status:
- Runtime commands (excluding router): `81`
- Matching command pages in `/.wiki/commands`: `81`
- Current coverage: `100%` (every runtime command has a page)

---

## How To Use This Matrix

1. Start with canonical commands above.
2. Use wrapper docs only when maintaining old scripts or migrating older usage.
3. Prefer updating user-facing docs/examples to canonical command paths.
4. If a runtime command changes, update this matrix and `commands/COMMANDS.md` in the same change.
