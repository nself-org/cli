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

See [[error-codes]] for E507 and E508.
