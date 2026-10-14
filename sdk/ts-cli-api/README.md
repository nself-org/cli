# @nself/cli-api

Typed TypeScript client for the nSelf CLI control API (`nself admin serve`). One method per command, typed requests and `data`, typed errors, a confirmation helper and an NDJSON stream reader. Zero runtime dependencies. Node 20 or newer (it uses the global `fetch`; pass `fetch` for anything else).

The methods and types in `src/generated/` and the operation table below are generated from the committed OpenAPI document (`.github/openapi.json`) by `tools/sdkgen`. Do not edit them; run `go run ./tools/sdkgen -lang ts` from the repository root after the contract changes. `go run ./tools/sdkgen -lang ts -check` fails on drift and runs in `go test ./...`.

## Install

The package is not published yet (publishing is a separate, owner-named step). From a checkout of `nself-org/cli`:

```sh
cd sdk/ts-cli-api && pnpm install && pnpm run build
```

## Start the API and get a token

```sh
nself admin serve            # loopback only; prints the token file path
cat ~/.nself/mcp-token       # the bearer token (mode 0600)
```

The server answers on `http://127.0.0.1:3857` by default. Keep the token out of URLs, logs and source control.

## Use it

```ts
import { createClient, NselfError, ConfirmationRequired } from "@nself/cli-api";

const api = createClient({ token, baseUrl: "http://127.0.0.1:3857" });

// Typed data: ConfigGetData { key, value, masked, file }
const { value } = await api.configGet({ args: ["BASE_DOMAIN"] });
```

Every method takes `{ args?, flags? }` (plus `confirm?` on the operations that declare it) and an optional `{ signal, onResponse }`. `onResponse` receives the HTTP status, `X-Nself-Exit-Code` (state exits 10 to 12 return `data` with a non-zero exit code), the request id and the envelope warnings.

### Errors

A failure envelope or an undecodable response throws `NselfError` with `code` (for example `E426`), `class` (`user | infra | auth | destructive_blocked | other`), `exitCode`, `httpStatus`, `message`, and `remediation` / `docsUrl` / `command` / `requestId` when the server sent them. A response that is not a v1 envelope (a proxy error page) has `code: "CLIENT_PROTOCOL"`.

```ts
try {
  await api.configGet({ args: ["NO_SUCH_KEY"] });
} catch (e) {
  if (e instanceof NselfError && e.class === "user") console.error(e.code, e.remediation);
}
```

### Confirmation

A state-changing command answers HTTP 428 until it is confirmed. The client throws `ConfirmationRequired`; `confirm()` re-sends the identical request with the server's single-use value (valid for 10 minutes) and resolves to the command data. Review `confirmation.reason` and `confirmation.side_effect` before you call it.

```ts
try {
  await api.configSecretsSet({ args: ["API_KEY", secretValue] });
} catch (e) {
  if (e instanceof ConfirmationRequired) {
    console.log(e.confirmation.side_effect, e.confirmation.reason);
    await e.confirm();
  } else throw e;
}
```

### Streams

Commands whose output is a stream (for example `logs` with `follow`) are served as NDJSON, but the OpenAPI contract is document-only, so they have no generated method. Use `stream(path, request)`: it yields one item per line, every line a v1 envelope, and the last one is `final` (`data.type === "result"`). An error envelope that ends the stream throws `NselfError`; a stream that ends without a final line throws `CLIENT_PROTOCOL`. Breaking out of the loop cancels the request. Once streaming has started the HTTP status is already 200, so an error line that ends a stream has `httpStatus: 200`; read `code`, `class` and `exitCode`. The `X-Nself-Exit-Code` trailer is not readable through `fetch`.

```ts
for await (const { envelope, final } of api.stream("/v1/commands/logs", { flags: { follow: true } })) {
  if (!final) console.log(envelope.data?.message);
}
```

`openapi()` returns the served contract document.

## Develop

```sh
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm test -- --testPathIgnorePatterns live                 # unit tests, mock fetch
NSELF_BIN=/path/to/nself pnpm test -- live                 # live contract test against nself admin serve
```

The version always equals `.github/VERSION` (`sdk-version-sync.yml` fails otherwise).

## Operations

<!-- BEGIN GENERATED: operations (tools/sdkgen, DO NOT HAND EDIT) -->

211 operations. Every method takes `{ args, flags }` (plus `confirm` where the operation declares it) and resolves to the typed `data` member.

| Method | Route | Summary |
| --- | --- | --- |
| `backup` | `POST /v1/commands/backup` | Backup operations: create, list, restore, verify, prune, config, status, init-key |
| `backupConfig` | `POST /v1/commands/backup/config` | View backup configuration |
| `backupCreate` | `POST /v1/commands/backup/create` | Create a new backup |
| `backupDrill` | `POST /v1/commands/backup/drill` | Run a DR drill: restore latest backup into scratch DB and measure RTO |
| `backupInitKey` | `POST /v1/commands/backup/init-key` | Generate age encryption keypair for backups |
| `backupList` | `POST /v1/commands/backup/list` | List available backups |
| `backupPitr` | `POST /v1/commands/backup/pitr` | Point-in-time recovery: enable, disable, status, base-backup, restore |
| `backupPitrBaseBackup` | `POST /v1/commands/backup/pitr/base-backup` | Take a manual pg_basebackup snapshot now |
| `backupPitrDisable` | `POST /v1/commands/backup/pitr/disable` | Stop WAL archiving (removes PITR config snippet) |
| `backupPitrEnable` | `POST /v1/commands/backup/pitr/enable` | Configure WAL archiving and enable PITR |
| `backupPitrRestore` | `POST /v1/commands/backup/pitr/restore` | Restore database to a specific point in time |
| `backupPitrStatus` | `POST /v1/commands/backup/pitr/status` | Show PITR retention window, segment count, and latest WAL timestamp |
| `backupPrune` | `POST /v1/commands/backup/prune` | Remove old backups by retention policy |
| `backupRestore` | `POST /v1/commands/backup/restore` | Restore from a backup |
| `backupRestoreRemote` | `POST /v1/commands/backup/restore-remote` | Restore a backup directly from a remote URL |
| `backupResume` | `POST /v1/commands/backup/resume` | Resume an interrupted streaming backup |
| `backupSchedule` | `POST /v1/commands/backup/schedule` | Schedule recurring streaming backups via systemd timers |
| `backupStatus` | `POST /v1/commands/backup/status` | Show backup subsystem status |
| `backupVerify` | `POST /v1/commands/backup/verify` | Verify backup integrity |
| `build` | `POST /v1/commands/build` | Compose your infrastructure from .env |
| `clean` | `POST /v1/commands/clean` | Remove generated artifacts (docker-compose.yml, nginx configs, build cache) |
| `config` | `POST /v1/commands/config` | Manage project configuration |
| `configEnv` | `POST /v1/commands/config/env` | Multi-environment management: switch, list, diff, copy |
| `configEnvCopy` | `POST /v1/commands/config/env/copy` | Scaffold a new environment from an existing one |
| `configEnvDiff` | `POST /v1/commands/config/env/diff` | Compare two environment configs side-by-side |
| `configEnvExplain` | `POST /v1/commands/config/env/explain` | Show the effective .env cascade and which file wins |
| `configEnvList` | `POST /v1/commands/config/env/list` | List available environments |
| `configEnvShow` | `POST /v1/commands/config/env/show` | Show current environment config (secrets redacted) |
| `configEnvTarget` | `POST /v1/commands/config/env/target` | Manage control-plane server targets |
| `configEnvTargetAdd` | `POST /v1/commands/config/env/target/add` | Add a server to an environment |
| `configEnvTargetList` | `POST /v1/commands/config/env/target/list` | List servers in the control-plane inventory |
| `configEnvTargetMigrate` | `POST /v1/commands/config/env/target/migrate` | Migrate legacy NSELF_DEPLOY_HOST_* env vars to control-plane.yaml |
| `configEnvTargetProbe` | `POST /v1/commands/config/env/target/probe` | Resolve runtime capability for one or more targets |
| `configEnvTargetRemove` | `POST /v1/commands/config/env/target/remove` | Remove a server from an environment |
| `configEnvUse` | `POST /v1/commands/config/env/use` | Switch active environment |
| `configExplain` | `POST /v1/commands/config/explain` | Explain where a configuration key comes from |
| `configFeatures` | `POST /v1/commands/config/features` | Manage CLI-built-in feature flags |
| `configFeaturesDisable` | `POST /v1/commands/config/features/disable` | Disable a feature flag (records an override) |
| `configFeaturesEnable` | `POST /v1/commands/config/features/enable` | Enable a feature flag (records an override) |
| `configFeaturesList` | `POST /v1/commands/config/features/list` | List all registered feature flags with effective state |
| `configFeaturesStatus` | `POST /v1/commands/config/features/status` | Show the effective state for a single flag |
| `configGet` | `POST /v1/commands/config/get` | Get a single configuration value |
| `configImport` | `POST /v1/commands/config/import` | Import config from a file into .env |
| `configList` | `POST /v1/commands/config/list` | List all known config keys with current values |
| `configOauth` | `POST /v1/commands/config/oauth` | Manage OAuth provider tokens |
| `configOauthRefresh` | `POST /v1/commands/config/oauth/refresh` | Trigger immediate OAuth token refresh for a provider |
| `configSecrets` | `POST /v1/commands/config/secrets` | Manage encrypted project secrets (age encryption) |
| `configSecretsAudit` | `POST /v1/commands/config/secrets/audit` | Report secrets that haven't been rotated in >90 days |
| `configSecretsInit` | `POST /v1/commands/config/secrets/init` | Generate age key and set up .secrets/ directory |
| `configSecretsLint` | `POST /v1/commands/config/secrets/lint` | Check for plaintext secrets in git-tracked files |
| `configSecretsList` | `POST /v1/commands/config/secrets/list` | List all secrets for an environment |
| `configSecretsListSchedules` | `POST /v1/commands/config/secrets/list-schedules` | List all rotation schedule statuses (alias for: secrets schedule) |
| `configSecretsRecipients` | `POST /v1/commands/config/secrets/recipients` | Manage age recipients of one secret store |
| `configSecretsRekey` | `POST /v1/commands/config/secrets/rekey` | Re-encrypt all secrets, removing a team member's key |
| `configSecretsRetire` | `POST /v1/commands/config/secrets/retire` | Retire the _PREVIOUS variant of a dual-window rotated secret |
| `configSecretsRotate` | `POST /v1/commands/config/secrets/rotate` | Rotate a secret to a new value |
| `configSecretsRotationLog` | `POST /v1/commands/config/secrets/rotation-log` | Show rotation event log |
| `configSecretsSchedule` | `POST /v1/commands/config/secrets/schedule` | Add a rotation schedule or show all schedule statuses |
| `configSecretsSet` | `POST /v1/commands/config/secrets/set` | Set a secret value (prompts if value not provided) |
| `configSecretsVerify` | `POST /v1/commands/config/secrets/verify` | Verify that a named secret exists in the store |
| `configSet` | `POST /v1/commands/config/set` | Update a configuration value (writes to .env) |
| `configShow` | `POST /v1/commands/config/show` | Show all config key=value pairs (masked by default) |
| `configTelemetry` | `POST /v1/commands/config/telemetry` | Manage CLI telemetry preferences |
| `configTelemetryOff` | `POST /v1/commands/config/telemetry/off` | Disable telemetry (writes to ~/.nself/config.toml) |
| `configTelemetryOn` | `POST /v1/commands/config/telemetry/on` | Enable telemetry (writes to ~/.nself/config.toml) |
| `configTelemetryStatus` | `POST /v1/commands/config/telemetry/status` | Show current telemetry state and anonymous install-ID |
| `configTrustSsl` | `POST /v1/commands/config/trust/ssl` | Manage SSL certificates |
| `configTrustSslAdd` | `POST /v1/commands/config/trust/ssl/add` | Provision an SSL certificate for a single domain |
| `configTrustSslRenew` | `POST /v1/commands/config/trust/ssl/renew` | Renew SSL certificates and reload the serving proxy |
| `configTrustSslSetup` | `POST /v1/commands/config/trust/ssl/setup` | Set up SSL certificates via DNS-01 challenge |
| `configTrustSslStatus` | `POST /v1/commands/config/trust/ssl/status` | Show SSL certificate status |
| `configTrustStatus` | `POST /v1/commands/config/trust/status` | Show trusted cert CAs, last rotation date, and expiry warnings |
| `configValidate` | `POST /v1/commands/config/validate` | Validate configuration against all registered rules |
| `db` | `POST /v1/commands/db` | Database operations: migrations, backups, restore, seed, shell |
| `dbBackup` | `POST /v1/commands/db/backup` | Create pg_dump backup |
| `dbBackupSync` | `POST /v1/commands/db/backup-sync` | Sync local backups to remote storage (cross-region) |
| `dbBackupSyncStatus` | `POST /v1/commands/db/backup-sync-status` | Show cross-region backup sync status |
| `dbBackupList` | `POST /v1/commands/db/backup/list` | List available backups with size and date |
| `dbDrift` | `POST /v1/commands/db/drift` | Schema drift detection: scan np_* tables for Theme 25 column compliance |
| `dbDriftFix` | `POST /v1/commands/db/drift/fix` | Generate migration SQL to fix Theme 25 column drift |
| `dbDriftScan` | `POST /v1/commands/db/drift/scan` | Scan np_* tables for missing Theme 25 columns |
| `dbDrop` | `POST /v1/commands/db/drop` | Drop the project database (DESTRUCTIVE) |
| `dbExport` | `POST /v1/commands/db/export` | Export a portable bundle from a started project |
| `dbFkIndex` | `POST /v1/commands/db/fk-index` | Audit and create indexes for foreign key columns |
| `dbFkIndexApply` | `POST /v1/commands/db/fk-index/apply` | Create missing FK indexes |
| `dbFkIndexAudit` | `POST /v1/commands/db/fk-index/audit` | Find FK columns without supporting indexes |
| `dbGenerate` | `POST /v1/commands/db/generate` | Generate type-safe client SDK types from the live Hasura schema |
| `dbGenerateVectorSearch` | `POST /v1/commands/db/generate/vector-search` | Generate a k-NN search function for a vector column and track it in Hasura |
| `dbHasura` | `POST /v1/commands/db/hasura` | Hasura metadata operations |
| `dbHasuraApplyRef` | `POST /v1/commands/db/hasura/apply-ref` | Apply Hasura metadata from a specific git ref |
| `dbHasuraConsole` | `POST /v1/commands/db/hasura/console` | Open Hasura Console |
| `dbHasuraDiff` | `POST /v1/commands/db/hasura/diff` | Compare live Hasura metadata against on-disk files |
| `dbHasuraGitStatus` | `POST /v1/commands/db/hasura/git-status` | Show git status of the hasura/metadata/ directory |
| `dbHasuraMetadata` | `POST /v1/commands/db/hasura/metadata` | Manage Hasura metadata |
| `dbHasuraMetadataDrift` | `POST /v1/commands/db/hasura/metadata-drift` | Detect drift between live Hasura metadata and committed files |
| `dbHasuraMetadataApply` | `POST /v1/commands/db/hasura/metadata/apply` | Apply Hasura metadata |
| `dbHasuraMetadataExport` | `POST /v1/commands/db/hasura/metadata/export` | Export Hasura metadata to git-friendly sorted YAML |
| `dbHasuraMetadataReload` | `POST /v1/commands/db/hasura/metadata/reload` | Reload metadata cache |
| `dbHasuraPermissions` | `POST /v1/commands/db/hasura/permissions` | Read Hasura table permissions |
| `dbHasuraSchema` | `POST /v1/commands/db/hasura/schema` | Read the Hasura GraphQL schema |
| `dbHasuraSnapshot` | `POST /v1/commands/db/hasura/snapshot` | Capture or compare Hasura metadata snapshots |
| `dbHasuraSync` | `POST /v1/commands/db/hasura/sync` | Export live Hasura metadata and commit to git |
| `dbHasuraValidate` | `POST /v1/commands/db/hasura/validate` | Validate Hasura metadata consistency and permission coverage |
| `dbImageSwitch` | `POST /v1/commands/db/image/switch` | Move the database to the pgvector Postgres image (the old volume is kept) |
| `dbImportAppwrite` | `POST /v1/commands/db/import/appwrite` | Import and verify data from appwrite |
| `dbImportFirebase` | `POST /v1/commands/db/import/firebase` | Generate nSelf migration artifacts from a Firebase export |
| `dbImportHasuraCloud` | `POST /v1/commands/db/import/hasura-cloud` | Import and verify data from hasura-cloud |
| `dbImportNhost` | `POST /v1/commands/db/import/nhost` | Import and verify data from nhost |
| `dbImportNself` | `POST /v1/commands/db/import/nself` | Import and verify a portable bundle written by `nself db export` |
| `dbImportPostgres` | `POST /v1/commands/db/import/postgres` | Import and verify data from postgres |
| `dbImportSupabase` | `POST /v1/commands/db/import/supabase` | Migrate a Supabase project to nSelf |
| `dbLint` | `POST /v1/commands/db/lint` | Check RLS policies on tenant-scoped tables |
| `dbList` | `POST /v1/commands/db/list` | List databases in the project Postgres instance |
| `dbMigrate` | `POST /v1/commands/db/migrate` | Manage database migrations |
| `dbMigrateApply` | `POST /v1/commands/db/migrate/apply` | Apply a specific migration file (G-008) |
| `dbMigrateAudit` | `POST /v1/commands/db/migrate/audit` | Audit migrations for idempotency, rollback files, and checksum drift |
| `dbMigrateBaseline` | `POST /v1/commands/db/migrate/baseline` | Record migration(s) as applied WITHOUT running their SQL |
| `dbMigrateCreate` | `POST /v1/commands/db/migrate/create` | Create new migration file |
| `dbMigrateDown` | `POST /v1/commands/db/migrate/down` | Revert last migration |
| `dbMigrateGenerate` | `POST /v1/commands/db/migrate/generate` | Generate a SQL migration from a natural-language description |
| `dbMigrateIdempotent` | `POST /v1/commands/db/migrate/idempotent` | Check and optionally rewrite a migration file to be idempotent |
| `dbMigrateLint` | `POST /v1/commands/db/migrate/lint` | Refuse migrations with many unguarded CREATE TABLE statements |
| `dbMigrateStatus` | `POST /v1/commands/db/migrate/status` | Show migration status |
| `dbMigrateUp` | `POST /v1/commands/db/migrate/up` | Apply pending migrations |
| `dbPgbouncer` | `POST /v1/commands/db/pgbouncer` | PgBouncer connection pooler operations |
| `dbPgbouncerGenerate` | `POST /v1/commands/db/pgbouncer/generate` | Write PgBouncer config files to .nself/pgbouncer/ |
| `dbPgbouncerStatus` | `POST /v1/commands/db/pgbouncer/status` | Show PgBouncer runtime status (SHOW STATS via psql) |
| `dbPitr` | `POST /v1/commands/db/pitr` | Point-in-time recovery: status, enable, test, restore |
| `dbPitrEnable` | `POST /v1/commands/db/pitr/enable` | Write PITR configuration to .nself/pitr/ |
| `dbPitrRestore` | `POST /v1/commands/db/pitr/restore` | Restore database to a point in time (--target RFC3339) |
| `dbPitrStatus` | `POST /v1/commands/db/pitr/status` | Show PITR/WAL archiving status |
| `dbPitrTest` | `POST /v1/commands/db/pitr/test` | Verify WAL archiving is working |
| `dbReconcile` | `POST /v1/commands/db/reconcile` | Push repo-declared Hasura metadata (permissions, relationships, table tracking) to a live instance |
| `dbReset` | `POST /v1/commands/db/reset` | Drop and recreate database (DESTRUCTIVE) |
| `dbResetChecksum` | `POST /v1/commands/db/reset-checksum` | Reset stored checksum for a migration (dangerous) |
| `dbRestore` | `POST /v1/commands/db/restore` | Restore from backup |
| `dbRestoreDrill` | `POST /v1/commands/db/restore-drill` | Run a non-destructive restore drill |
| `dbRestoreDrillList` | `POST /v1/commands/db/restore-drill-list` | List past restore drill results |
| `dbRls` | `POST /v1/commands/db/rls` | Row-Level Security management: audit, apply, rollback |
| `dbRlsApply` | `POST /v1/commands/db/rls/apply` | Enable RLS and apply default policies on tables that are missing them |
| `dbRlsApplyTable` | `POST /v1/commands/db/rls/apply-table` | Enable RLS and apply default policies on a specific table |
| `dbRlsAudit` | `POST /v1/commands/db/rls/audit` | List tables with their RLS and policy state |
| `dbRlsRollback` | `POST /v1/commands/db/rls/rollback` | Print rollback SQL that removes RLS policies and disables RLS |
| `dbSeed` | `POST /v1/commands/db/seed` | Database seeding: run, list, verify, graph |
| `dbSeedFixtures` | `POST /v1/commands/db/seed/fixtures` | Manage seed fixtures |
| `dbSeedFixturesList` | `POST /v1/commands/db/seed/fixtures/list` | List available fixtures |
| `dbSeedFixturesManifest` | `POST /v1/commands/db/seed/fixtures/manifest` | Show or store a fixture manifest |
| `dbSeedFixturesRun` | `POST /v1/commands/db/seed/fixtures/run` | Run a specific fixture |
| `dbSeedFixturesVerify` | `POST /v1/commands/db/seed/fixtures/verify` | Verify fixture checksums against a stored manifest |
| `dbSeedGraph` | `POST /v1/commands/db/seed/graph` | Show seed dependency graph |
| `dbSeedList` | `POST /v1/commands/db/seed/list` | List available seeds and fixtures |
| `dbSeedMatrix` | `POST /v1/commands/db/seed/matrix` | Show the seed environment/file matrix |
| `dbSeedRun` | `POST /v1/commands/db/seed/run` | Execute seeds for current environment |
| `dbSeedVerify` | `POST /v1/commands/db/seed/verify` | Verify a fixture is deterministic (dry-run replay check) |
| `dbSoftDelete` | `POST /v1/commands/db/soft-delete` | Manage soft-delete (deleted_at) patterns across tables |
| `dbSoftDeleteApply` | `POST /v1/commands/db/soft-delete/apply` | Add deleted_at column, index, and active view to a table |
| `dbSoftDeleteAudit` | `POST /v1/commands/db/soft-delete/audit` | Audit tables for soft-delete status |
| `dbSoftDeleteGenerate` | `POST /v1/commands/db/soft-delete/generate` | Print soft-delete migration SQL to stdout |
| `dbVerify` | `POST /v1/commands/db/verify` | Verify what a Hasura role can actually reach via role-scoped introspection |
| `dbVerifyChecksums` | `POST /v1/commands/db/verify-checksums` | Verify migration file checksums against stored values |
| `deploy` | `POST /v1/commands/deploy` | Deploy the stack to a target environment |
| `deployAccess` | `POST /v1/commands/deploy/access` | Manage SSH key access on an already-deployed server |
| `deployAccessGrant` | `POST /v1/commands/deploy/access/grant` | Grant a person SSH key access to a host |
| `deployAccessList` | `POST /v1/commands/deploy/access/list` | List who has SSH key access to a host |
| `deployAccessRevoke` | `POST /v1/commands/deploy/access/revoke` | Revoke a person's SSH key access to a host |
| `deployCheckAccess` | `POST /v1/commands/deploy/check-access` | Verify access to configured deploy targets (deprecated: use 'nself env target probe') |
| `deployEnvironments` | `POST /v1/commands/deploy/environments` | List configured deploy environments and server capabilities |
| `deployHealth` | `POST /v1/commands/deploy/health` | Run health checks against a target deployment |
| `deployOps` | `POST /v1/commands/deploy/ops` | Deploy with the ops profile to the ops server |
| `deployPromote` | `POST /v1/commands/deploy/promote` | Promote a canary or an environment |
| `deployPromoteEnv` | `POST /v1/commands/deploy/promote-env` | Promote one environment to another (e.g. staging to prod) |
| `deployPromoteEnvRollback` | `POST /v1/commands/deploy/promote-env/rollback` | Rollback to pre-promotion backup |
| `deployRollback` | `POST /v1/commands/deploy/rollback` | Roll back the last deployment for a target |
| `deploySecuritySetup` | `POST /v1/commands/deploy/security/setup` | Apply baseline server hardening |
| `deployStatus` | `POST /v1/commands/deploy/status` | Show deployment status for a target |
| `deployTargets` | `POST /v1/commands/deploy/targets` | List deploy targets without connecting to them |
| `deployWeb` | `POST /v1/commands/deploy/web` | Build web apps locally and deploy prebuilt output to Vercel |
| `doctor` | `POST /v1/commands/doctor` | Run comprehensive system diagnostics |
| `doctorHeal` | `POST /v1/commands/doctor/heal` | Run targeted self-healing routines for nSelf components |
| `doctorImages` | `POST /v1/commands/doctor/images` | Check availability of every locked image |
| `doctorReport` | `POST /v1/commands/doctor/report` | Write a redacted support bundle (tar.gz) to share in an issue |
| `doctorSecurityAudit` | `POST /v1/commands/doctor/security/audit` | Run a read-only security audit |
| `doctorSecurityStatus` | `POST /v1/commands/doctor/security/status` | Show current security posture |
| `helpCapabilities` | `POST /v1/commands/help/capabilities` | Search commands and plugins by what you want to do |
| `helpTopics` | `POST /v1/commands/help/topics` | Browse help topics (quickstart, plugins, license, envs, doctor, errors) |
| `initTemplate` | `POST /v1/commands/init/template` | Browse and publish full-stack app templates |
| `initTemplateInfo` | `POST /v1/commands/init/template/info` | Show detail for a single template |
| `initTemplateList` | `POST /v1/commands/init/template/list` | List available app templates |
| `initTemplatePublish` | `POST /v1/commands/init/template/publish` | Submit a template to the nSelf registry |
| `initTemplateUpdate` | `POST /v1/commands/init/template/update` | Apply incremental migrations for the installed template |
| `logs` | `POST /v1/commands/logs` | View and filter service logs |
| `mcpSetup` | `POST /v1/commands/mcp/setup` | Print or write the MCP client config and AGENTS.md for this project |
| `reset` | `POST /v1/commands/reset` | Stop containers, remove all data volumes, and clean generated files |
| `status` | `POST /v1/commands/status` | Show health status of all services |
| `statusHealth` | `POST /v1/commands/status/health` | Health check management with continuous monitoring |
| `statusHealthCheck` | `POST /v1/commands/status/health/check` | Run all health checks |
| `statusHealthConfig` | `POST /v1/commands/status/health/config` | Show health check settings |
| `statusHealthEndpoint` | `POST /v1/commands/status/health/endpoint` | Check an HTTP endpoint |
| `statusHealthHistory` | `POST /v1/commands/status/health/history` | Show last 20 health checks |
| `statusHealthService` | `POST /v1/commands/status/health/service` | Check a single service |
| `statusUrls` | `POST /v1/commands/status/urls` | Display all service URLs with route conflict detection |
| `update` | `POST /v1/commands/update` | Update the nSelf CLI and admin UI |
| `updateImages` | `POST /v1/commands/update/images` | Refresh Docker image digests from registries |
| `updateProjectDetect` | `POST /v1/commands/update/project/detect` | Detect v1 artifacts in the current project |
| `updateProjectFromBash` | `POST /v1/commands/update/project/from-bash` | Migrate a v0.9.9 Bash-era project to the current nSelf CLI |
| `updateProjectFromV099` | `POST /v1/commands/update/project/from-v099` | Migrate v0.9.9 home-level state (license key, channel, ssh keys) to v1.x layout |
| `updateProjectRollback` | `POST /v1/commands/update/project/rollback` | Restore a v1 backup created by migrate run |
| `updateProjectRun` | `POST /v1/commands/update/project/run` | Migrate v1 project to v2 |
| `updateStack` | `POST /v1/commands/update/stack` | Upgrade the stack with a verified backup and resumable steps |
| `updateUpgrade` | `POST /v1/commands/update/upgrade` | Upgrade the nSelf CLI (detects install method) |
| `updateVerifySbom` | `POST /v1/commands/update/verify-sbom` | Verify the SBOM signature for a CLI release |

<!-- END GENERATED: operations -->
