# Come home from Appwrite

Start a fresh nSelf project before importing. Postgres and Hasura must be healthy,
and the project needs the hasura-auth schema (`AUTH_ENABLED=true`) if the Appwrite
project has users, and an object store if it has files.

Appwrite has no Postgres you can dump. The importer reads your project through the
Appwrite REST API with an API key, builds a portable bundle (the same bundle
`nself db export` writes), loads it, and verifies the result.

## API key

Create a key in the Appwrite console (project settings, API keys) with read scopes
only: `databases.read`, `collections.read`, `attributes.read`, `documents.read`,
`users.read`, `buckets.read` and `files.read`. The server version line shows `unknown`
with an API key (Appwrite hides it from keys). Put the key in an environment variable using your
secret manager. The key must not appear in a command argument or shell history.

```bash
nself db import appwrite --endpoint https://cloud.appwrite.io/v1 --project <project-id> --api-key-env APPWRITE_KEY --plan --json
nself db import appwrite --endpoint https://cloud.appwrite.io/v1 --project <project-id> --api-key-env APPWRITE_KEY --json
```

`--api-key-env` takes the variable name, never the key. The endpoint must be `https`
(plain `http` is accepted only for `localhost` and loopback addresses, so the key never
crosses a network in clear text). `--endpoint` is your Appwrite
API URL (a self-hosted server looks like `https://appwrite.example.com/v1`; the `/v1`
suffix is added when missing). In v1.4 mode the command is `nself migrate appwrite`.
The plan checks the source and target and writes nothing.

## What maps to what

| Appwrite | nSelf |
|---|---|
| database | Postgres schema (id with other characters replaced by `_`) |
| collection | table in that schema |
| document `$id` | `id text PRIMARY KEY` |
| `$createdAt`, `$updatedAt` | `created_at`, `updated_at` as `timestamptz` |
| string, email, url, ip, enum | `text` |
| integer | `bigint` |
| float (`double`) | `double precision` |
| boolean | `boolean` |
| datetime | `timestamptz` |
| array attribute | `jsonb` |
| relationship (single-valued) | `text` column holding the related id, plus a Hasura object relationship named `<column>_rel` |
| relationship (a list, such as many-to-many) | `jsonb` list of ids; no Hasura relationship (listed in the report) |
| `$permissions` | not mapped; reported (see below) |
| user | `auth.users` row; the id becomes a stable UUID, the Appwrite id is kept in `metadata.appwrite_id` |
| bucket | object-store bucket (lowercased, `_` and `.` become `-`) |
| file | object `<file id>/<file name>` in that bucket |

An attribute that is not `available` in Appwrite (still processing or failed) is not imported
and is listed in the report's `exemptions`. A database with a reserved id (`auth`, `storage`,
`public`, `pg_*`, `np_*`) becomes the schema `aw_<id>`.

Columns in your own tables that hold Appwrite user ids keep the Appwrite id; the imported
users get a stable UUID instead, with the Appwrite id in `metadata.appwrite_id`. Map them
with a one-off `UPDATE ... FROM auth.users WHERE metadata->>'appwrite_id' = ...`.

An attribute whose name collides with `id`, `created_at` or `updated_at` is stored as
`attr_<name>`. Two attributes that map to the same column name stop the import.

The import is tested against Appwrite 1.5.x. If your server leaves an attribute out of the
document JSON (some newer versions do not return relationship values unless asked), the
column is NULL and the report lists the attribute and the number of documents under
`exemptions` (id ending `#missing:<attribute>`); check those before you trust the data.
Buckets are created in nSelf only for the objects they hold, so an empty bucket is listed in
the report instead. A file name containing `/` or `\` has it replaced by `_`, and the file is
listed in the report. Each downloaded file must match Appwrite's `sizeOriginal`
and its MD5 `signature` when the server supplies one.

List pages start at 100 items and shrink on responses exceeding 64 MiB, down to
one item. A single item larger than this limit stops the import. The plan uses a
one-user count query; when its total reaches Appwrite's 5000-user cap, it walks
the user list to count users, so planning large projects requires more requests.

## Passwords

Appwrite stores one hash per user and the algorithm beside it. Users whose hash is
bcrypt keep their password. Argon2, scrypt, MD5, SHA, PHPass and every other
algorithm cannot be verified by nSelf auth: those users are imported with no
password and listed in the report's `users.reset_required` with their email. Send
them through the hasura-auth password reset (`/user/password/reset`). The count is
never silent. Users without a password (OAuth, anonymous, phone-only) import as they
are. MFA does not transfer: users who had it enabled are counted in the report
(`exemptions`, id `users#mfa`) and must enrol again. Phone verification is kept as
`metadata.phone_verified`.

## Permissions

Appwrite collection, document, bucket and file permissions have no Hasura or storage
equivalent. They are not translated. Every collection or bucket that has permissions, or
documents or files with their own permissions, appears in the report's `exemptions` with the
counts.
Review them and write Hasura permissions for the roles you use.

## Verification

The result appears in the JSON envelope and in `.nself/import/<run-id>/report.json`.
A pass means every row count, content hash, user hash algorithm, object checksum,
relationship and independent source count matched. Source counts come from the
`total` field of each Appwrite list call (documents per collection, users, files per
bucket), so a page lost on the way shows as a mismatch (E523). Appwrite caps list totals at 5000;
for a list of 5000 or more the count of items read is used instead and the report says
that count is not independent. Treat E522 and E523 as
an incomplete import. After a failed import follow the recovery steps in
[Come home from PostgreSQL](Come-Home-Postgres).

Only one import runs on a target at a time. The key is read once from the environment and never
written to the bundle, the report or the logs.
