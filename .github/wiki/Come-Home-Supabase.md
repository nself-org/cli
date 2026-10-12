# Come home from Supabase

Start a fresh nSelf project before importing, with `AUTH_ENABLED=true` and, if the
project has storage objects, an object store (MinIO or SeaweedFS). Postgres and Hasura
must be healthy. The source must run PostgreSQL 12 or later, with a major no newer than
the target (set `POSTGRES_VERSION` in the target first if needed).

## What you need from Supabase

- The **direct database connection string** (Project Settings > Database > Connection
  string, "Direct connection"; use the pooler URL only if your plan has no direct
  connection, and note that a pooler cannot hold the repeatable-read snapshot the
  import needs). Put it in an environment variable. The URL must not appear in a
  command argument or shell history.
- For storage: the API URL (`https://<project-ref>.supabase.co`, or `--project-ref`) and
  the `service_role` key, again in an environment variable.

```bash
nself db import supabase --db-url-env SUPA_DB --project-ref abcdefghijkl --service-key-env SUPA_KEY --plan --json
nself db import supabase --db-url-env SUPA_DB --project-ref abcdefghijkl --service-key-env SUPA_KEY --json
```

In v1.4 mode the command is `nself migrate supabase`, which only writes migration
artifacts (see [[cmd-migrate-supabase]]). In v1.5 mode it imports and verifies. The plan
checks source and target and writes nothing. For a self-hosted or local Supabase use
`--api-url http://127.0.0.1:<port>` (plain http is accepted only for loopback hosts).

## What is moved

| Part | How | Notes |
|---|---|---|
| Your tables | One `pg_dump` snapshot, restored with `pg_restore` | Every schema except the Supabase-owned ones |
| Auth users | `auth.users` to nSelf `auth.users` | `encrypted_password` is bcrypt and is kept, so users sign in with the password they have today. Every user gets the allowed role `user` |
| Storage | Storage REST API with the service key | Each bucket and object; the object count must match `storage.objects` |
| Access rules (RLS) | Translated or listed | See below |

Excluded schemas: `auth`, `storage`, `realtime`, `supabase_functions`, `extensions`,
`graphql`, `graphql_public`, `pgbouncer`, `pgsodium`, `vault`, `net`,
`supabase_migrations`. The Supabase roles (`anon`, `authenticated`, `service_role`) are
never recreated, and ownership and grants are not restored.

## Row level security

Row level security policies and the `ENABLE ROW LEVEL SECURITY` switch are **not**
restored. A policy of the form `auth.uid() = <column>` (either order) on a permissive
policy for `authenticated` or `public` becomes a Hasura permission for the `user` role:
`select`, `update` and `delete` filter on `{"<column>": {"_eq": "X-Hasura-User-Id"}}`,
`insert` checks it. Several policies for one command are combined with `_or`.

A translated permission covers only the kinds that the role `authenticated` can use on the table
(it needs the privilege), and a read command needs a `USING` expression. A table with a
restrictive policy has none of its policies translated, because Hasura has no AND rule and
would grant more than Postgres did.

Every other policy (restrictive policies, other roles, any other expression) is listed
in the report under `untranslated_policies` with its text and the reason. Hasura denies
a role that has no permission, so an untranslated policy means that access is **closed**
for non-admin roles until you add a Hasura permission for it. Review that list before
you open the app to users. The admin secret bypasses permissions.

## Shims

The bundle carries `db/compat.sql`, listed in the report as `compat_shims`:

- `extensions.uuid_generate_v4()` and `extensions.gen_random_uuid()` become wrappers for
  Postgres `gen_random_uuid()`, so column defaults keep working.
- `auth.uid()` reads `x-hasura-user-id` from the session setting `hasura.user`, so a
  default of `auth.uid()` fills the owner column with the caller's id when a user inserts
  through Hasura.

Other `extensions.*` or `auth.*` functions that your schema calls have no shim. They are
listed as exemptions (`compat:<name>`); define them in the target before relying on them.

## Limits

- Partitioned tables are refused. Foreign keys into Supabase-owned tables other than
  `auth.users` (for example `storage.objects`) are not supported.
- A bucket name that the object store rejects (uppercase or underscore) stops the import
  with a clear message; rename the bucket in Supabase first.
- A user whose hash is not bcrypt is listed in `users.reset_required`.
- Bucket settings (public flag, size limit, allowed types) are not carried; set them in the
  target. `auth.identities` is not carried, so a user who only signed in through an OAuth
  provider has no password on the target (hash algorithm `none`).
- The snapshot and the storage listing are read at slightly different moments. If someone uploads
  or deletes an object during the import, the count check stops it ("storage listing differs from
  the storage catalog"); rerun on a quiet source.
- A schema that uses `extensions.*` functions or types without a shim (for example `crypt`,
  `digest`, `citext`, `vector`) lists them as exemptions and the restore stops where it needs them.
  Install the extension in the target first.
- Edge functions, realtime and cron jobs are not moved. Row level security stays off in the target
  (the `ENABLE ROW LEVEL SECURITY` switch is not restored); access is governed by Hasura permissions.

## After the import

The result appears in the JSON envelope and in `.nself/import/<run-id>/report.json`. A
pass means every non-exempt row count, content hash, user hash algorithm, object
checksum, tracking record, permission and independent source count matched. After a
failed import see [[Come-Home-Postgres]] for the reset sequence (`wrote` and `last_step`
in the report):

```bash
nself db reset --yes
```

← [[Leaving-nSelf]] | [[Home]] →
