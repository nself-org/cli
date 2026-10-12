# Come home from Firebase

Start a fresh nSelf project before importing. Postgres and Hasura must be healthy, and the
project must have no tables in the `firebase` schema and no users in `auth.users`.

You need three things from Firebase:

1. The project id (`--project`).
2. A service-account key file for a service account that can read Firestore and Cloud Storage
   (`--credentials-file`). The key stays on disk where you keep it. The importer reads it, signs a
   short-lived token with the standard library and holds both only in memory. Neither appears in
   output, reports or errors. With the Firebase emulator, set `FIRESTORE_EMULATOR_HOST` (and
   `STORAGE_EMULATOR_HOST` for a bucket) and no key file is needed. Giving both a key file and an
   emulator host is refused, so a stale host variable cannot read the wrong data.
3. A user export: `firebase auth:export auth-users.json --format=json` (`--auth-export`).

```bash
nself db import firebase --project my-project --credentials-file ./key.json --auth-export ./auth-users.json --bucket my-project.appspot.com --plan --json
nself db import firebase --project my-project --credentials-file ./key.json --auth-export ./auth-users.json --bucket my-project.appspot.com --json
```

In v1.4 mode `nself migrate firebase --export-dir <dir>` keeps writing migration artifacts from a
Firestore export. In v1.5 mode the same command, and `nself db import firebase`, run this import.
`--bucket` is optional; without it no storage is copied. `--plan` checks the inputs, the source
and the target and writes nothing; the old `--dry-run` flag means `--plan` in v1.5 mode.

## What is copied

**Documents.** Firestore is read through its REST API (`documents`, `listCollectionIds`, and the
`runAggregationQuery` COUNT used as the independent source count). Every top-level collection
becomes a table in the schema `firebase`:

| Part | Rule |
|---|---|
| Table name | the collection id, lower-cased, other characters replaced by `_` (a leading digit gets a `_`) |
| Name clash | the first collection (sorted) keeps the name, the next gets `_2`, `_3`; each rename is listed in the report's `exemptions` with kind `metadata` |
| Columns | `id text` (primary key), `data jsonb` (the whole document) |
| Typed columns | a top-level field becomes a column when every document that has it agrees on one scalar type: string `text`, integer `bigint`, double `double precision`, boolean `boolean`, timestamp `timestamptz` |
| Disagreement | a field whose documents disagree on type, or whose column name clashes after sanitising (`Title` and `title`, or `id`), gets no column; the value stays in `data`, and the report lists the field in `exemptions` (kind `metadata`) |
| Sub-collections | table `<parent>__<sub>` with `parent_path` (for example `users/u1`) and `parent_id`; the key is `(parent_path, id)` because document ids repeat across parents |
| Nested maps, arrays, geo points, references, bytes | stay in `data` only (bytes as base64, references as the resource name) |

No type is ever guessed. There is no data exemption: every document and every field is in a table.
A document that holds a NUL character in a string cannot be stored by Postgres and stops the import.

**Users.** Firebase hashes passwords with a modified scrypt that hasura-auth cannot verify, so
users arrive **without a password** and `users.reset_required` in the report lists each one with
its id and email. The hash is never copied into the bundle. Send each user through hasura-auth's
`/user/password/reset`. A user id is a UUIDv5 of the Firebase uid, so a re-run gives the same ids;
the uid stays in `auth.users.metadata.firebase.uid`, with providers and custom attributes.
Users that only sign in through a provider have no hash and are not in the reset list. A user
whose email or phone number repeats (hasura-auth keeps them unique) is left out and listed as an
exemption of kind `user`.

**Storage.** With `--bucket`, every object is read through the Cloud Storage JSON API and loaded
into the project's object store. Without an enabled store the import stops with E529 unless you
pass `--skip-storage`, which lists each object as an exemption.

## What the verifier compares

The report passes only when, for every table, the row count and a content hash match, and the
independent source counts match: documents per collection from the Firestore COUNT aggregation
(a different API than the document listing, so a missing page shows up), users from the export
file's element count, and objects from a second bucket listing. A mismatch is E523 and the exit
code is 1.

## Limits

- Firestore Security Rules and indexes are not translated. Add Hasura permissions after the import.
- A document that was deleted but still has sub-collections is not a row; its sub-collections are
  still imported (the listing asks Firestore for such parents).
- Each document costs one `listCollectionIds` read to find sub-collections.
- Object versions other than the live one are not copied.

## After a failed import

`report.json` under `.nself/import/<run-id>/` records `wrote` and `last_step`, exactly as for the
other sources. If `wrote` is false, nothing changed: fix the cause and rerun. If it is true, reset
the fresh target and rerun:

```bash
nself db reset --yes
nself stop
nself start
nself db import firebase --project my-project --credentials-file ./key.json --auth-export ./auth-users.json --json
```

`db reset` drops and recreates the project database, so run it only on the fresh target you were
importing into. See [[Come-Home-Postgres]] for the full recovery notes.

[[Home]] | [[Command-Canon]]
