# Come home from Hasura Cloud

Moves a Hasura Cloud project (or any Hasura with its own Postgres and no platform
auth) into a fresh nSelf project: the database through a repeatable-read snapshot
and the Hasura metadata through `export_metadata`. This source carries no users: a
Hasura project has no built-in accounts. If the database has an `auth.users` table
it is an nhost project; the command says so and stops. Use
[`nself db import nhost`](Come-Home-nhost) instead.

## Before you start

- Start a fresh nSelf project. Postgres and Hasura must be healthy.
- You need a direct Postgres connection string to the database that Hasura is
  connected to (Hasura Cloud does not host the database itself; it is the one you
  attached, for example on Neon, RDS or Heroku), the project's Hasura base URL, and
  the admin secret.
- Use a source on PostgreSQL 12 or later, with a major no newer than the target.
- Stop writes to the source while you copy.

Every secret goes in an environment variable; the flags take the variable's name.
The Hasura URL must be `https` (plain `http` only for loopback). A console URL ending
in `/v1/graphql` is accepted.

```bash
nself db import hasura-cloud --db-url-env SRC_URL --hasura-url https://my-project.hasura.app --admin-secret-env SRC_ADMIN_SECRET --plan --json
nself db import hasura-cloud --db-url-env SRC_URL --hasura-url https://my-project.hasura.app --admin-secret-env SRC_ADMIN_SECRET --json
```

In v1.4 mode the command is `nself migrate hasura-cloud`. In v1.5 mode it lives
under `db import`. The plan proves the admin secret and the database connection and
writes nothing.

## What moves

| Part | How |
|---|---|
| Tables | Every user schema's tables with their data, constraints and indexes (not `hdb_catalog`, `auth` or nSelf's own). |
| Metadata | Tracked tables, relationships, computed fields and per-role permissions for the tables that came across, and tracked functions in those schemas. Connection strings are never copied. |

Not carried: event triggers, actions, remote schemas, cron triggers, inherited roles
and query collections. The manifest counts the skipped actions and remote schemas.

## Result

The JSON envelope and `.nself/import/<run-id>/report.json` give per-table counts and
the metadata comparison (tracking, relationships, permissions). A pass means every
non-exempt row count, content hash and metadata record matched. E523 means a
mismatch; E522 means verification could not run. A second import into the same
target is refused with E519.

## After a failed import

`report.json` records `wrote` and `last_step`. If `wrote` is false nothing reached
the target: fix the cause and rerun. If it is true, reset the fresh target and rerun
(`db reset` drops the project database, so use it only on the target you were
importing into):

```bash
nself db reset --yes
nself stop
nself start
```

The same recovery rules as [Come home from PostgreSQL](Come-Home-Postgres) apply.
