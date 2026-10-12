# nself migrate supabase

> Pull schema, data, auth users, and storage buckets from a Supabase project and generate nSelf migration artifacts.

## Synopsis

```
nself migrate supabase --project-ref <ref> --supabase-key <key> [flags]
```

## Description

`nself migrate supabase` connects to a live Supabase project via the PostgREST and Admin APIs, pulls your public schema, auth users, and storage buckets, then generates three migration artifacts: a Drizzle-compatible SQL migration file, a Hasura metadata YAML, and a shell script to import your auth users.

Unlike `nself migrate firebase`, this command requires live access to your Supabase project. Use `NSELF_SUPABASE_SKIP=1` to skip live connectivity in CI environments.

## v1.5 mode

With `NSELF_V15=1` this command is `nself db import supabase`: it imports the project into a started nSelf project through a direct database connection, moves the auth users with their bcrypt hashes and the storage objects, translates simple row level security policies to Hasura permissions, and verifies the result. The flags below marked v1.5 apply only there; `--project-ref` and `--supabase-key` are then optional and `--supabase-key` and `--out` are unused. See [[Come-Home-Supabase]].

| Flag (v1.5) | Description |
|---|---|
| `--db-url-env` | Environment variable holding the direct database URL |
| `--api-url` | Storage API URL (default `https://<project-ref>.supabase.co`) |
| `--service-key-env` | Environment variable holding the `service_role` key |
| `--plan`, `--keep-stage`, `--skip-storage` | As for `nself db import` |

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--project-ref` | — | Supabase project reference ID (the ID in your project URL). **Required.** |
| `--supabase-key` | — | `service_role` key from Supabase > Project Settings > API. **Required.** |
| `--out` | `.` | Output directory for generated files |

## Steps performed

1. Connect to Supabase and pull public schema (tables, columns, RLS policies where accessible)
2. Pull auth users via the Supabase Admin API
3. Pull storage buckets via the Supabase Storage API
4. Generate a Drizzle-compatible SQL migration file
5. Generate Hasura metadata YAML
6. Generate an auth user import script
7. Write all files to the output directory

## Generated artifacts

| File | Contents |
|------|----------|
| `supabase-migration.sql` | `CREATE TABLE` statements compatible with Drizzle and nSelf's Postgres instance |
| `hasura-metadata.yaml` | Hasura table tracking and permissions scaffold |
| `import-auth-users.sh` | Shell script to import Supabase auth users into nSelf Auth |

## Environment variables

| Variable | Description |
|----------|-------------|
| `NSELF_SUPABASE_SKIP` | Set to `1` to skip live Supabase connectivity (CI mode) |

## Examples

```bash
# Basic migration
nself migrate supabase \
  --project-ref abcdefghijkl \
  --supabase-key eyJhbGci...

# Write artifacts to a named output directory
nself migrate supabase \
  --project-ref abcdefghijkl \
  --supabase-key eyJhbGci... \
  --out ./migration

# CI environment: skip live connectivity
NSELF_SUPABASE_SKIP=1 nself migrate supabase \
  --project-ref ci-project-ref \
  --supabase-key ci-service-key
```

## Finding your project ref and key

- **Project ref:** the alphanumeric ID in your Supabase project URL: `https://app.supabase.com/project/<ref>`
- **Service role key:** Supabase Dashboard > Project Settings > API > `service_role` (keep this secret)

## Applying the artifacts

After the command completes, apply the generated files to your nSelf project:

```bash
# Apply the schema migration
nself db migrate apply --file ./migration/supabase-migration.sql

# Apply Hasura metadata (via Hasura console or CLI)
hasura metadata apply --directory ./migration/hasura-metadata.yaml

# Import auth users
bash ./migration/import-auth-users.sh
```

## Limitations

- Only the `public` schema is pulled. Tables in other schemas require manual migration.
- Supabase-specific features (Edge Functions, Realtime subscriptions) are not converted.
- RLS policies are pulled where the `service_role` key has access; review `supabase-migration.sql` before applying.

## See Also

- [[cmd-migrate]] — v0.9 to v1 project migration
- [[cmd-migrate-firebase]] — migrate from Firebase
- [[cmd-db]] — run schema migrations on a live project

← [[Commands]] | [[Home]] →
