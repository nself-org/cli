# pgvector and Postgres extension requirements

Some plugins need a Postgres extension. claw, for example, needs `vector`. The plugin says so in its manifest:

```json
"requires": { "postgres_extensions": ["vector"] }
```

Use the Postgres extension name (`vector`), not the config spelling `pgvector`.

## What `nself plugin install` checks

The check runs before anything is downloaded.

| Database state | What decides | Result when the extension is missing |
|---|---|---|
| Running | `pg_available_extensions` on the project's postgres container | Error E507 |
| Stopped or not created | The resolved image (`POSTGRES_IMAGE`, or the version) | Error E507 |
| Stopped, custom image | Unknown | Warning E508, install continues |
| Running but cannot be queried, or Docker is unreachable | Nothing | A plain error that says so. It is never reported as "missing" |

The check only reads. It never creates or enables an extension, and it never changes the image or any env value. It talks to Postgres through the container (`docker exec ... psql` over the local socket), so no password is placed on a command line. Extension names are validated and are never put into SQL.

## Which images have `vector`

| Image | `vector` | Contrib extensions (pgcrypto, uuid-ossp, pg_trgm, ...) |
|---|---|---|
| `pgvector/pgvector:pg16` and other `pgvector/pgvector:*` | yes | yes |
| `postgres:16-alpine` and other `postgres:*` | no | yes |
| anything else | unknown | unknown |

New projects default to the pgvector image. Older projects and production run `postgres:16-alpine`, which has no `vector`, so installing claw there is refused with E507.

## The fix

Move the project to the pgvector image:

```bash
nself db image switch --to pgvector
```

Then run the install again. For another extension, set `POSTGRES_IMAGE` to an image that ships it, run `nself build`, restart, and retry.

## Moving an existing project

Never change the image of a project that already has data. An alpine volume (uid 70, data at the volume root) cannot be started by a Debian image (uid 999, data in `pgdata`), and a swapped image on a populated volume has lost data before. `nself db image switch` does the move the safe way: it copies the data into a **new** volume and leaves the old one alone.

```bash
nself db image switch --to pgvector --plan   # preview, change nothing
nself db image switch --to pgvector --yes    # run it
```

`--plan` lists the checks, the old and new volume, an estimate of the downtime, and the compose and lock change the switch will make. A run that is not interactive needs `--yes`.

### What happens

| Step | What it does |
|---|---|
| Preflight | Postgres is running; there is a pgvector image of the same major version; the docker data root has twice the data size free and the stage directory has 1.2 times; the target image ships every extension and locale the databases use; no custom tablespace and no user-made template database; no database name that psql would read as a connection string. A failed check stops the switch with E509 and nothing changes. |
| Quiesce | Every service except postgres stops. Each database is set `default_transaction_read_only = on` and other sessions end. **This is the downtime.** |
| Dump | `pg_dumpall --globals-only` and one `pg_dump -Fc` per database, each inside one exported snapshot, together with a capture of rows, schema, sequences, roles and grants. The files live in `.nself/image-switch/stage` (mode 0700, files 0600) and are deleted at the end. |
| Restore | A one-shot server of the pgvector image restores into the new volume with `pg_restore --exit-on-error`. It has no network, no published port and none of the project's `postgres/init` scripts, so the `np_plugins` seed cannot collide with the restored data. The only statement it skips is `CREATE ROLE` of the bootstrap role, which `initdb` already made; each skipped statement is printed. |
| Verify | The restored cluster is captured again and compared: every table's row count and content hash, the schema-only dump of every database, sequence values, large objects (count and content hash), roles and their attributes, grants, owners, default privileges, policies and extension versions. A difference is E510 and nothing is switched. |
| Recheck | The old cluster is captured once more. If anything changed since the dump, the switch stops with E510: the data written in between would not be in the new volume. |
| Switch | Postgres stops. `POSTGRES_IMAGE` and `POSTGRES_DATA_VOLUME` (`<project>_postgres_data_pgvector_<time>`) are written to `.env`, compose and `nself.lock.yaml` are regenerated, and the stack starts on the new volume. |

If any step before the last fails, the switch undoes itself: the new volume is removed (it is the only thing the command created), the read-only setting is reset and the stopped services start again on the old volume.

### Resume and rollback

The progress is in `.nself/image-switch/state.json`. While a switch is incomplete, every other command that changes the project (`start`, `build`, `config set`, ...) refuses with E512, because a half-moved database must not be started by accident.

```bash
nself db image switch --resume           # finish an interrupted switch
nself db image switch --rollback         # undo; from any phase
nself db image switch --rollback --yes   # undo after a finished switch
```

`--rollback` writes the previous env values back (the two keys are left set to an empty value), regenerates compose and the lock, starts postgres on the old volume, resets the read-only setting and starts the stack. After a finished switch it needs `--yes`, because rows written since the switch exist only in the new volume. That volume is kept and named; it is never removed for you.

A second `nself db image switch --to pgvector` on a project that already runs a pgvector image prints that and changes nothing.

### The old volume

The old volume is never mounted read-write by the switch, never renamed and never removed. After a successful switch its databases stay read-only (`default_transaction_read_only`), so nothing can write to it by accident; `--rollback` resets that. When you are satisfied, remove it by hand:

```bash
docker volume rm <project>_postgres_data
```

`nself reset`, `nself uninstall` and `nself stop --volumes` remove only the volumes of the current compose file, which names the new volume. The kept old volume (a full copy of the data, with password hashes) survives them, as does the new volume after a rollback. Remove them by hand.

### Limits

- Image switch errors omit SQL commands and continuations after the error summary, and cut the summary before its first quote to avoid printing row values.
- The data is read about four times (dump, restore, verify, recheck), so the downtime grows with the size of the database. `--plan` prints the estimate.
- Row hashes use each row's text form. A column that stores an `oid`, `xid` or a similar server-local number differs between clusters and fails the verify.
- The read-only setting is a session default, not a lock: any session can turn it off for itself. Sessions are ended at the quiesce and again right before the recheck, and a write made before the recheck is caught (E510), but a write in the second between the recheck and the postgres stop is not. Keep people off the database while the switch runs.
- Materialized views are rebuilt by the restore (`REFRESH`) and compared: a stale view in the old cluster gives E510, so refresh it first. Subscriptions and replication slots are refused in the preflight.
- Settings made with `ALTER SYSTEM` (`postgresql.auto.conf`), a custom `pg_hba.conf` and changes to `template1` are not moved; the plan lists the `ALTER SYSTEM` settings it finds. Custom tablespaces, user-made template databases and the ICU locale provider are refused in the preflight.
- The pgvector image is Debian-based and the alpine image is not: text sorts by the C library of the new image. Check ordering-sensitive queries after the switch.
- `POSTGRES_DATA_VOLUME` is written to `.env`. If another env file in the cascade sets `POSTGRES_IMAGE` or `POSTGRES_DATA_VOLUME`, the switch stops and names the problem; the env file is put back.

See [[error-codes]] for E507 to E512.
