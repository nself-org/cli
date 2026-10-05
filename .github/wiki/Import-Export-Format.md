# Import / Export Format (portable bundle v1)

The portable bundle is the vendor-neutral directory that `nself db export` writes and every importer reads. It is contract `cli.portable-export` v1. The manifest schema is generated: `schemas/portable-export.v1.schema.json`.

Library: `internal/portable` (Writer, Reader, Manifest), `internal/postgres/tablehash` (count and hash per table), `internal/objstore` (S3 client for bucket contents).

## Layout

```
<bundle>/
  manifest.json                 schema: schemas/portable-export.v1.schema.json
  db/schema.sql                 pg_dump --schema-only --no-owner --no-privileges (for humans)
  db/data.dump                  pg_dump -Fc --no-owner --no-privileges (schemas in manifest db.schemas)
  auth/users.jsonl              one JSON object per user, sorted by id
  storage/<bucket>/<key>        object bytes (see "Object names")
  hasura/metadata.json          export_metadata v3 subset for db.schemas
  config/keys.txt               sorted config key NAMES, never values (nself sources only)
  db/compat.sql                 optional source shims applied before pre-data
```

The bundle directory is mode 0700 and its files 0600. A bundle never holds a secret value: `config/keys.txt` lists key names only, and the exporter scrubs the text files before it finishes.

## manifest.json

| Field | Meaning |
|---|---|
| `_format` | always `nself-portable-export` |
| `schema_version` | `"1"`; readers accept `1` and `1.<n>` |
| `created_at` | RFC 3339; the only time-dependent field |
| `producer` | `tool`, `version`, `source` (`nself`, `postgres`, `nhost`, `hasura-cloud`, `supabase`, `firebase`, `appwrite`) |
| `postgres` | `server_version`, `major` of the source |
| `db` | `schemas`, and `tables[]` with `schema`, `name`, `rows`, `hash`, `pk` |
| `auth` | `users`, `hash_algorithms` (algorithm to count), `reset_required` |
| `storage` | `buckets[]` (`name`, `objects`, `bytes`) and `objects[]` (`bucket`, `key`, `sha256`, `bytes`) |
| `hasura` | counts of tables, relationships, permissions, functions, and `skipped` kinds |
| `exemptions` | items deliberately left out: `kind` (`table`, `user`, `object`, `metadata`), `id`, `reason` |
| `source_counts` | the source's own counts with the `method` used |
| `compat` | source shims the bundle needs |
| `files` | every member except `manifest.json`: `path`, `sha256` (lowercase hex of the exact bytes), `bytes` |

The manifest is deterministic: keys are sorted (Go marshals maps in key order), arrays are sorted by their natural id (`db.tables` by schema and name, `files` by path, and so on; a table's `pk` keeps the key's column order), two-space indent, one trailing newline. Two runs over the same input write the same bytes. Only `created_at` changes with time.

## Table hash

`rows` is `count(*)`. `hash` is, as a decimal string:

```sql
SELECT count(*), coalesce(sum(hashtextextended(t::text, 0)::numeric), 0)::text FROM "schema"."table" AS t
```

The sum makes it independent of row order and of how the rows were inserted. An empty table hashes to `"0"`. A changed value, an extra row or a duplicated row changes it.

`t::text` depends on session settings, so `tablehash.Snapshot` sets these for the session before it hashes, and restores the caller's values afterwards: `TimeZone=UTC`, `DateStyle=ISO`, `IntervalStyle=postgres`, `extra_float_digits=3`, `bytea_output=hex`, `lc_monetary=C`, `statement_timeout=0`. The same fixture gives the same count and hash on Postgres 14, 15, 16 and 17 and under any server or session default.

Schema and table names must match `^[A-Za-z_][A-Za-z0-9_]{0,62}$`. A table outside that shape is never queried; it comes back as unverifiable and the verify report lists it. Names are quoted with `pgx.Identifier` after the check.

Limits of the definition: the hash covers the row text in column order, so a target table must have the same columns in the same order as the source. It does not cover indexes, constraints or sequence values.

## Reading a bundle (untrusted input)

`portable.Open(dir)` treats the directory as untrusted and refuses it unless every check passes:

| Failure | Code |
|---|---|
| no `manifest.json`, not JSON, wrong `_format`, `schema_version` not major 1 (for example `"2"`) | E515 |
| manifest does not match the v1 layout, or has data after the JSON object | E516 |
| a listed file is missing, truncated, longer than listed, or its SHA-256 differs (the error names the file) | E516 |
| a member path that is absolute, uses `..`, `.`, `//`, a backslash, a drive letter or `:`, a control character, a Windows device name, or ends in a dot or space | E516 |
| two members with the same path (also compared case-insensitively), or a path that is both a file and a directory | E516 |
| a symlink anywhere in a member's path, a symlink in the bundle, a hard link, or a member that is not a regular file | E516 |
| an entry in the directory that the manifest does not list | E516 |
| a declared size or count over the limits (defaults: manifest 64 MiB, 1,000,000 files, 1 TiB per file, 8 TiB total) | E516 |

Reading is streamed and capped at the declared length, so a file cannot inflate past what the manifest says. `Reader.Open(rel)` returns a reader that re-checks length and SHA-256 as it streams and returns E516 instead of `io.EOF` when the bytes changed since `Open`. Error messages name paths only; they never print bundle content.

Because the reader refuses unlisted entries, do not add files to a bundle directory (editor backups, `.DS_Store`). Copy the directory as is.

## Object names

Object keys are arbitrary; member paths are not. `portable.StorageMember(bucket, key)` returns `storage/<bucket>/<key>` with each key segment escaped as `%XX`: control characters, `\ : % * ? " < > |`, a trailing dot or space, `.` and `..` segments, and Windows device names. An empty segment (from `a//b` or a trailing `/`) becomes a lone `%`. Unicode, spaces and `+` stay readable. The manifest `objects[]` always holds the real key; the importer maps key to file with `StorageMember` and never parses the file name. A key that is a directory prefix of another key (`a` and `a/b`) cannot both be files; the Writer reports that clash and the exporter records an exemption.

## Object store client

`internal/objstore` talks S3 in path style with stdlib SigV4: `EnsureBucket`, `Put`, `Get`, `List` (continuation-token pagination), `ListPage`. It is tested against SeaweedFS (`chrislusf/seaweedfs`) and the MinIO compatibility image (`pgsty/minio`, ADR 0028), over a direct and a loopback endpoint, with 1,100 keys that contain spaces, Unicode, `+` and `%`. A PUT streams with `UNSIGNED-PAYLOAD` only over plain http to a loopback endpoint; every other PUT signs the payload hash. Errors never contain the secret key or a signature.

## Evolution

Additive changes (a new optional field, a new `producer.source`) stay v1; readers ignore fields they do not know. A rename or removal is v2: readers refuse an unknown major with E515 and nothing is read.
