# nself migrate firebase

> Generate nSelf migration artifacts from a Firebase Firestore and Auth export.

## Synopsis

```
nself migrate firebase --export-dir <path> [flags]
```

## Description

`nself migrate firebase` converts a Firebase Firestore JSON export into the migration artifacts needed to run the same data on nSelf: a `migration.sql` schema file, a `hasura-metadata.yaml` file, an optional `auth-import.sql` for migrating Auth users, and a `MIGRATION_SUMMARY.md` guide.

No live Firebase connection is required. The command operates entirely on locally exported JSON files, so it is safe to run in air-gapped or CI environments.

The migration process does not apply any changes to your nSelf project automatically. All generated files are written to the output directory for review before you apply them.

## Workflow

1. Export your Firestore data:
   ```bash
   firebase firestore:export --format json ./firebase-export
   ```
2. Optionally export your Auth users:
   ```bash
   firebase auth:export --format json ./firebase-export/auth-users.json
   ```
3. Run the migration command:
   ```bash
   nself migrate firebase --export-dir ./firebase-export
   ```
4. Review the generated files in `<export-dir>/nself-migration/`.
5. Apply the schema and metadata to your nSelf project.

## v1.5 mode: import pipeline

With `NSELF_V15=1` the command (also `nself db import firebase`) imports the live project instead of
writing artifacts: Firestore through the REST API, users from an `auth:export` file, objects from
Cloud Storage. It loads into a started nSelf project and verifies every count. See [[Come-Home-Firebase]].

```bash
nself db import firebase --project my-project --credentials-file ./key.json --auth-export ./auth-users.json --plan
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--export-dir` | — | Directory produced by `firebase firestore:export`. **Required in v1.4 mode.** |
| `--auth-export` | `""` | Path to Firebase Auth JSON export (optional) |
| `--output-dir` | `<export-dir>/nself-migration` | Directory for generated artifacts |
| `--project-name` | `firebase_import` | Project name used in generated SQL schema names and file prefixes |
| `--project` | `""` | v1.5 import: Firebase project id |
| `--credentials-file` | `""` | v1.5 import: service-account key file (not needed with `FIRESTORE_EMULATOR_HOST`) |
| `--bucket` | `""` | v1.5 import: Cloud Storage bucket to copy |
| `--plan` | `false` | v1.5 import: check the source and target, write nothing |
| `--keep-stage` | `false` | v1.5 import: keep the private portable bundle after a pass |
| `--skip-storage` | `false` | v1.5 import: list each storage object as an exemption instead of loading it |
| `--json` | `false` | v1.5 import: print the structured import report |

## Generated artifacts

| File | Contents |
|------|----------|
| `migration.sql` | `CREATE TABLE` statements with RLS scaffolding for each inferred collection |
| `hasura-metadata.yaml` | Hasura table tracking and permissions scaffold |
| `auth-import.sql` | `INSERT` statements for Auth users (only when `--auth-export` is provided) |
| `MIGRATION_SUMMARY.md` | Step-by-step guide for applying the artifacts to nSelf |

## Examples

```bash
# Basic: export dir only
nself migrate firebase --export-dir ./firebase-export

# With Auth users
nself migrate firebase \
  --export-dir ./firebase-export \
  --auth-export ./firebase-export/auth-users.json

# Custom output directory and project name
nself migrate firebase \
  --export-dir ./firebase-export \
  --output-dir ./nself-migration \
  --project-name myapp
```

## Applying the artifacts

After reviewing the generated files:

```bash
# Apply the schema SQL to your nSelf Postgres instance
nself db migrate apply --file ./nself-migration/migration.sql

# Import Hasura metadata (via Hasura console or CLI)
hasura metadata apply --directory ./nself-migration/hasura-metadata.yaml

# Import auth users (if generated)
nself exec psql < ./nself-migration/auth-import.sql
```

See `MIGRATION_SUMMARY.md` in the output directory for the full step-by-step instructions specific to your export.

## Limitations

- Collection naming follows Firestore conventions. Nested sub-collections become separate tables with a `_parent_id` foreign key.
- Firestore types are mapped to Postgres types on a best-effort basis. Review `migration.sql` before applying.
- Firestore Security Rules are not converted. Apply your Hasura row-level security policies manually.

## See Also

- [[cmd-migrate]] — v0.9 to v1 project migration
- [[cmd-migrate-supabase]] — migrate from Supabase
- [[cmd-db]] — run schema migrations on a live project

← [[Commands]] | [[Home]] →
