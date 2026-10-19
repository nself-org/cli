# Come home to nSelf

Bring a project from another platform onto nSelf, or take it back out. Every path is one
command under `nself db import <source>` or `nself db export`. Start a fresh nSelf
project first (`nself init`, then `nself start`) and import into it.

## What an import does

An import reads the source, builds a portable bundle in a private stage, loads the
bundle into your fresh project, then verifies the result. The verify report compares
row counts, content hashes, auth users, objects, Hasura tracking, permissions and
relationships against the source. A pass means **zero mismatches**. Any mismatch (E523)
or a check that could not run (E522) leaves the import incomplete; the report at
`.nself/import/<run-id>/report.json` lists each one. Run the command with `--plan`
first to check the source and target without writing anything.

Each guide below ends with the failure and reset path for that source.

## Guides

| Source | Guide | Command | Passwords | Fixture |
|---|---|---|---|---|
| PostgreSQL | [Come-Home-Postgres](Come-Home-Postgres) | `nself db import postgres --from-env SRC_URL` | No platform users. App tables only. When the source holds hasura-auth `auth.users`, the plan notes it looks like nhost; use the nhost command then | `test/fixtures/import/postgres` |
| nhost | [Come-Home-nhost](Come-Home-nhost) | `nself db import nhost --db-url-env SRC_URL --hasura-url <url> --admin-secret-env SRC_ADMIN_SECRET` | Preserved (bcrypt). A hash in another format is listed for reset | `test/fixtures/import/nhost` |
| Hasura Cloud | [Come-Home-Hasura-Cloud](Come-Home-Hasura-Cloud) | `nself db import hasura-cloud --db-url-env SRC_URL --hasura-url <url> --admin-secret-env SRC_ADMIN_SECRET` | Not applicable (no platform auth). Users tables are ordinary tables | `test/fixtures/import/hasura-cloud` |
| Supabase | [Come-Home-Supabase](Come-Home-Supabase) | `nself db import supabase --db-url-env SUPA_DB --project-ref <ref> --service-key-env SUPA_KEY` | Preserved (bcrypt `encrypted_password`). A non-bcrypt hash is listed for reset | `test/fixtures/import/supabase` |
| Firebase | [Come-Home-Firebase](Come-Home-Firebase) | `nself db import firebase --project <id> --credentials-file <key.json> --auth-export <users.json>` | Reset required. Firebase hashes (modified scrypt) cannot be verified, so users arrive without a password | `test/fixtures/import/firebase` |
| Appwrite | [Come-Home-Appwrite](Come-Home-Appwrite) | `nself db import appwrite --endpoint <url> --project <id> --api-key-env APPWRITE_KEY` | bcrypt preserved. Argon2, scrypt and every other hash need a reset | `test/fixtures/import/appwrite` |
| nSelf (leave and return) | [Leaving-nSelf](Leaving-nSelf) | `nself db export --out <dir>`, then `nself db import nself <dir>` | Preserved (bcrypt hashes travel in the bundle) | the round-trip script `scripts/e2e/export-round-trip.sh` |

## Commands in one place

Run each with `--plan` first. The guides give the full flags and the failure path.
`scripts/docs/check-guide-commands.sh` checks that every command below is exercised by a
test or drill leg.

```bash
nself db import postgres --from-env SRC_URL --plan
nself db import nhost --db-url-env SRC_URL --hasura-url <url> --admin-secret-env SRC_ADMIN_SECRET --plan
nself db import hasura-cloud --db-url-env SRC_URL --hasura-url <url> --admin-secret-env SRC_ADMIN_SECRET --plan
nself db import supabase --db-url-env SUPA_DB --project-ref <ref> --service-key-env SUPA_KEY --plan
nself db import firebase --project <id> --credentials-file <key.json> --auth-export <users.json> --plan
nself db import appwrite --endpoint <url> --project <id> --api-key-env APPWRITE_KEY --plan
nself db export --out <dir>
nself db import nself <dir> --plan
```

## Bringing a bundle back

[Leaving-nSelf](Leaving-nSelf) covers the export and the restore into plain PostgreSQL.
To load the same bundle into a fresh nSelf project, run `nself db import nself <dir>`.
It checks the whole bundle first and ends with the same verify report. In v1.4 mode the
command is `nself migrate nself <dir>`.

## Password resets

Where the table says reset, the report lists every affected user in
`users.reset_required` with the user id and email. The count is never silent. Send those
users through the hasura-auth password reset at `/user/password/reset`.

## Proof

The [importer drill](Drill-Importers) reruns every source and the export round trip
against pinned local fixtures each night. The drill evidence, not this page, is the
record of what currently passes.

## Related

- [cmd-migrate](cmd-migrate): the older `nself migrate <source>` spelling.
- [Compat-V15](Compat-V15): v1.4 and v1.5 command behaviour.

← [[Commands]] | [[Home]] →
