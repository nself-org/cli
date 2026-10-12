# Come home from nhost

Moves a self-hosted nhost project (or any stack of Postgres, Hasura, hasura-auth and
hasura-storage) into a fresh nSelf project: the database, the Hasura metadata, the
auth users with their bcrypt password hashes, and the hasura-storage files. People
sign in on nSelf with the password they already use; nobody is asked to reset it.

## Before you start

- Start a fresh nSelf project with auth and an object store enabled. Postgres,
  Hasura and the store must be healthy. `auth.users` on the target must be empty.
- You need a direct Postgres connection string to the nhost database. A hosted nhost
  project that offers no direct connection string for its tier cannot be imported;
  ask for the connection string or move the data with `pg_dump` and use the
  `postgres` source.
- You need the Hasura base URL and admin secret, and S3 access to the bucket that
  hasura-storage writes to (endpoint, access key, secret key, bucket name). Add
  `--s3-region` when the bucket is not in `us-east-1`, and `--s3-root-folder` when
  hasura-storage was configured with `S3_ROOT_FOLDER`.
- Use a source on PostgreSQL 12 or later, with a major no newer than the target
  (set POSTGRES_VERSION on the target if needed).
- Stop writes to the source while you copy. The tables are dumped first and the file list
  is read afterwards; if a file was added or removed in between, the import stops with
  `source storage changed while it was read` instead of leaving a row with no file.
- The import is refused, before any work, for a source that has users with MFA enabled,
  OAuth provider links (`auth.user_providers`) or security keys (`auth.user_security_keys`),
  and for more than 100000 files. MFA, provider links and keys are not carried yet, and
  dropping them silently would let a user sign in with the password alone or not at all.
  Remove them on the source, or recreate those accounts on the target afterwards.

Every secret goes in an environment variable; the flags take the variable's name,
never its value. Nothing secret appears in a command line, a log, the report or an
error. The Hasura URL and the S3 endpoint must be `https`; plain `http` is accepted
for loopback only, because the admin secret and the S3 signature travel with them.

```bash
nself db import nhost --db-url-env SRC_URL --hasura-url https://hasura.example.org --admin-secret-env SRC_ADMIN_SECRET --storage-endpoint https://s3.example.org --s3-access-key-env SRC_S3_ACCESS --s3-secret-env SRC_S3_SECRET --bucket nhost --plan --json
nself db import nhost --db-url-env SRC_URL --hasura-url https://hasura.example.org --admin-secret-env SRC_ADMIN_SECRET --storage-endpoint https://s3.example.org --s3-access-key-env SRC_S3_ACCESS --s3-secret-env SRC_S3_SECRET --bucket nhost --json
```

In v1.4 mode the command is `nself migrate nhost`. In v1.5 mode it lives under
`db import`. The plan checks the source (database, Hasura admin secret, S3 flags
when the source holds files) and the target, and writes nothing.

## What moves

| Part | How |
|---|---|
| Tables | One repeatable-read snapshot of every schema except `auth`, `hdb_catalog` and nSelf's own, including `storage.files` and `storage.buckets` and the foreign keys your tables hold into `auth.users` and `storage.files`. |
| Users | Every row of `auth.users` with its roles, email, phone, display name, metadata and password hash. Hashes are bcrypt in both stacks and carry over unchanged. A hash in another format is listed in `users.reset_required` and the user resets the password. |
| Files | Every `storage.files` row with an uploaded object is fetched from the bucket, checked against the recorded size, hashed (SHA-256) and written to the target store under the same bucket name and key (the file id), so point the target's file service at a bucket with that name. A row whose object is missing stops the import. |
| Metadata | Tracked tables, relationships, computed fields and permissions for the tables that came across. Tracked functions are recorded in the bundle and the SQL functions come across with the schema, but the load does not track them in Hasura yet: track them again after the import. |

Not carried: event triggers, actions, remote schemas, cron triggers, inherited roles,
query collections and any Hasura database source other than `default`. Tracked views,
materialized views, foreign tables and tables of schemas that are not copied (including
the `auth` tables) are not carried either, so their relationships and permissions are
not recreated: re-create them on the target after the import. Every tracked table,
function and extra source that is left out is listed in the report's `exemptions` with
the reason `kind: metadata`, and the manifest counts the skipped actions and remote
schemas. Tracked functions are recorded in the bundle and the SQL functions come across
with the schema, but the load does not track them in Hasura yet: track them again after
the import. A user's custom default role is not carried yet: every user lands with the
`user` default role.

## Result

The JSON envelope and `.nself/import/<run-id>/report.json` give per-table row
counts, user and file counts, and the metadata comparison. A pass means every
non-exempt row count, content hash, user hash algorithm, object checksum, tracking
record, permission, relationship and independent source count matched. The
independent counts are `count(auth.users)` and the number of uploaded
`storage.files` rows, read in separate queries outside the dump. E523 means a
mismatch; E522 means verification could not run. Treat both as an incomplete import.

`--skip-storage` exempts each file at load (listed in the report) so you can move the
database first; the source side still needs the S3 flags and still downloads every file. The private stage is removed after a pass; `--keep-stage` keeps the
integrity-checked bundle. If the target object store already holds a key from the
bundle, the import is refused with E519 before anything is written and the target's
object is left untouched. A failed run keeps its private stage under `.nself/import/`
(a copy of your data, mode 0600): delete it when you are done.

## After a failed import

`report.json` records `wrote` and `last_step`, and the error hint reads the same
flag. If `wrote` is false nothing reached the target: fix the cause and rerun. If it
is true, reset the fresh target and rerun (`db reset` drops the project database, so
use it only on the target you were importing into). `db reset` does not touch an object
store outside the project: if the failed attempt already wrote files there, delete them
from the target bucket (or recreate the bucket) first, or the rerun is refused with E519:

```bash
nself db reset --yes
nself stop
nself start
```

Only one import runs on a target at a time. The same recovery rules as
[Come home from PostgreSQL](Come-Home-Postgres) apply.
