# Come home from PostgreSQL

Start a fresh nSelf project before importing. Postgres and Hasura must be healthy.
Use a source running PostgreSQL 12 or later, with a major no newer than the target.
Set POSTGRES_VERSION to the source major if needed before starting the target.

Put the source PostgreSQL URL in an environment variable using your secret manager.
The URL must not appear in a command argument or shell history.

```bash
nself db import postgres --from-env SRC_URL --plan --json
nself db import postgres --from-env SRC_URL --json
```

In v1.4 mode the command is `nself migrate postgres`. In v1.5 mode it lives under
`db import`. The plan checks the source and target and writes no stage or report.
The command takes the project's operation lock, including during a plan.

The importer holds a repeatable-read source snapshot during extraction. Restore
uses the target image, loads pre-data and data, then auth users and storage, then
post-data constraints. Imported tables are tracked with additive Hasura metadata.
Existing target user tables cause E519 before restore. Plugin tables named np_*
and nSelf-owned schemas do not make a fresh target non-empty.

The result appears in the JSON envelope and in
`.nself/import/<run-id>/report.json`. A pass means every non-exempt row count,
content hash, user hash algorithm, object checksum, tracking record, permission,
relationship and independent source count matched. E523 means a mismatch; E522
means verification could not run. Treat either outcome as an incomplete import.

The private stage is removed after a pass or when nothing was written to the
target. Use `--keep-stage` to retain the portable bundle for inspection. Failed
imports that wrote to the target retain their stage; every staged run retains its
report. The error hint (E521, E522 and E523) names the report path,
`.nself/import/<run-id>/report.json`, which lists every mismatch.

## After a failed import

`report.json` records `wrote` (true once the target may have changed) and
`last_step` (the last finished load step). The error hint reads the same flag.

If `wrote` is false, nothing was written to the target. Fix the cause named in the
error and rerun; no reset is needed. This covers every refusal before the first
write: an unreachable or too-old source (E517, E518), extraction failures
(E520, including a partitioned source table, which is not supported yet), a
target that is not empty or whose `auth.users` already holds users (E519), a
missing object store (E529), another import already running on the target (E519),
a function or type in the bundle that the target already owns (E519), an archive
that does not match the bundle, and a metadata conflict found in the preflight
(E521). Each restore section runs in one transaction, so a failure inside a
section undoes that section; a failure in the first section (pre-data) when no
compat step ran therefore also leaves `wrote` false. `--plan` runs the same
bundle-free target checks (emptiness and, when the source has users, `auth.users`),
so a plan does not pass where the real run would be refused with E519.

If `wrote` is true, the load has no rollback beyond that: the target holds partial
data and a rerun is refused with E519. Reset the target project database and start
the stack again, then rerun the import:

```bash
nself db reset --yes
nself stop
nself start
nself db import postgres --from-env SRC_URL --json
```

`db reset` drops and recreates the project database, so run it only on the fresh
target you were importing into.

Only one import runs on a target at a time; a second run is refused with E519
instead of waiting. A source database must be Postgres 12 or newer; the importer
reads `server_version_num`, so beta versions count by their major.
