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
  storage/objects/<sha256 hex>  object bytes (see "Object names")
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
| `db` | `schemas`, and `tables[]` with `schema`, `name`, `rows`, `hash`, `pk`; `(schema, name)` is unique and `hash` is canonical (`0`, or a decimal with no leading zero, no `+`, no `-0`) |
| `auth` | `users`, `hash_algorithms` (algorithm to count), `reset_required` |
| `storage` | `buckets[]` (`name`, `objects`, `bytes`) and `objects[]` (`bucket`, `key`, `member`, `sha256`, `bytes`); `(bucket, key)` is unique and every object names a listed file with the same `sha256` and `bytes` |
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

`t::text` depends on session settings, so `tablehash.Snapshot` sets these for the session before it hashes, and restores the caller's values afterwards: `TimeZone=UTC`, `DateStyle=ISO`, `IntervalStyle=postgres`, `extra_float_digits=3`, `bytea_output=hex`, `lc_monetary=C`, `statement_timeout=0`, `row_security=off` and `search_path=pg_catalog`. The same fixture gives the same count and hash on Postgres 14, 15, 16 and 17 and under any session default of those settings.

- `row_security=off` is what `pg_dump` runs. A role that cannot read every row of a table gets the error "query would be affected by row-level security policy"; it never gets a hash of the rows it can see. Run the hash as a role that owns or bypasses the tables.
- `search_path=pg_catalog` makes `regclass`, `regtype`, `regproc` and the other `reg*` columns print schema-qualified, so the text form does not depend on the session's `search_path`.
- The database must be UTF8. The hash is over the server-encoding bytes, so the same text hashes differently in a LATIN1 database; `Snapshot` refuses a non-UTF8 database with `ErrNotUTF8` before it queries anything.

Schema and table names must match `^[A-Za-z_][A-Za-z0-9_]{0,62}$`. A table outside that shape is never queried; it comes back as unverifiable and the verify report lists it. Names are quoted with `pgx.Identifier` after the check.

Limits of the definition: the hash covers the row text in column order, so a target table must have the same columns in the same order as the source. It does not cover indexes, constraints or sequence values.

## Reading a bundle (untrusted input)

`portable.Open(dir)` treats the directory as untrusted and refuses it unless every check passes:

| Failure | Code |
|---|---|
| no `manifest.json`, not JSON, wrong `_format`, `schema_version` not major 1 (for example `"2"`) | E515 |
| manifest does not match the v1 layout, has an unknown field (for `schema_version` `1` or `1.0`), has data after the JSON object, repeats a key, has two keys that differ only in case, or spells a field in another case | E516 |
| a table listed twice, an object listed twice, an object that names no listed file or a file with a different `sha256` or `bytes`, or a non-canonical table hash | E516 |
| a listed file is missing, truncated, longer than listed, or its SHA-256 differs (the error names the file) | E516 |
| a member path that is absolute, uses `..`, `.`, `//`, a backslash, a drive letter or `:`, a control character, a Windows device name, ends in a dot or space, or is not in Unicode NFC | E516 |
| two members with the same path (compared after NFC normalisation and Unicode case folding), or a path that is both a file and a directory | E516 |
| a symlink anywhere in a member's path, a symlink in the bundle, a hard link (counted from the file handle on Unix and on Windows), or a member that is not a regular file | E516 |
| a member that is not the file that was verified when it is opened later (different device and inode, or Windows file ID, size or modification time) | E516 |
| an entry in the directory that the manifest does not list | E516 |
| more entries than `MaxFiles`, counting directories as well as files | E516 |
| a declared size or count over the limits (defaults: manifest 64 MiB, 1,000,000 files, 1 TiB per file, 8 TiB total) | E516 |

Reading is streamed and capped at the declared length, so a file cannot inflate past what the manifest says. `Reader.Open(rel)` returns a reader that re-checks length and SHA-256 as it streams and returns E516 instead of `io.EOF` when the bytes changed since `Open`. `Reader.OpenFile(rel)` returns the open file for tools that need a descriptor (`pg_restore` can read standard input): it opens without following a symlink (`O_NOFOLLOW` where the platform has it) and refuses unless the file has the device and inode (Windows: volume and file index), size and modification time recorded while the bundle was verified. There is no path accessor, because a path can be swapped between the check and the use. Error messages name paths only; they never print bundle content.

Because the reader refuses unlisted entries, do not add files to a bundle directory (editor backups, `.DS_Store`). Copy the directory as is.

## Object names

Object keys are arbitrary and case- and Unicode-sensitive; file systems are not. `portable.StorageMember(bucket, key)` returns `storage/objects/<hex sha256(bucket + "\x00" + key)>`, so every `(bucket, key)` pair has its own member on every platform: `Readme.md` and `README.md`, or the NFC and NFD spellings of `café.jpg`, export on a case-insensitive or normalising file system as they do on S3. The manifest `objects[]` holds the real `bucket`, `key`, `member`, `sha256` and `bytes`; the importer maps key to file with `StorageMember` (or reads `member`) and never parses the file name. A key may not be empty or invalid UTF-8, and a bucket may not be empty or contain NUL. `portable.NewObject(bucket, key, file)` builds the entry from the `File` that `Writer.WriteFile` returned.

## Object store client

`internal/objstore` talks S3 in path style with stdlib SigV4: `EnsureBucket`, `Put`, `Get`, `List` (continuation-token pagination, at most `MaxListPages` pages and `MaxListObjects` objects), `ListPage`. Listing sends `encoding-type=url` and decodes the keys, so a key with a character XML cannot carry still lists. The canonical request is built from the escaped path and the raw query without form decoding (a literal `+` signs as `%2B`). The default HTTP client has dial, TLS and response-header timeouts and no deadline on the whole request, so a large object can take as long as it needs; bound a call with its context. `Client` and `Signer` print with the secret key redacted. It is tested against SeaweedFS (`chrislusf/seaweedfs`) and the MinIO compatibility image (`pgsty/minio`, ADR 0028), over a direct and a loopback endpoint, with 1,100 keys that contain spaces, Unicode, `+` and `%`. A PUT streams with `UNSIGNED-PAYLOAD` only over plain http to a loopback endpoint; every other PUT signs the payload hash. Errors never contain the secret key or a signature.

## Evolution

Additive changes (a new optional field, a new `producer.source`) stay v1 and are released as a minor, `schema_version` `1.<n>`. A reader refuses a field it does not know in a `1` or `1.0` manifest, and ignores one in a `1.<n>` (n > 0) manifest. A rename or removal is v2: readers refuse an unknown major with E515 and nothing is read.

---
[[Home]]
