# Leaving nSelf

Export a started project to a new or empty directory:

```bash
nself db export --out ./portable-bundle --json
```

Choose schemas with `--schemas public,app`. Omit object files with `--no-storage`.
The output's parent directory must exist. Export runs on Linux and macOS only (its
tool containers run as your numeric user id); other platforms are refused before any
read. It reads the stack without stopping services or changing source data. It checks
available disk before writing output. The estimate is 3.7 times the selected tables'
relation sizes (1.2 for the archive, 2.5 for the temporary plain-SQL rendering the
secret scan needs) plus object bytes; leave additional room for large metadata and
auth records.

The bundle contains a custom-format PostgreSQL archive, a readable schema dump,
table counts and content hashes, portable auth users with bcrypt password hashes,
S3 objects, Hasura metadata for selected tables, and configuration key **names**.
The metadata keeps table configuration, relationships, computed fields and permissions.
Event triggers (webhook URLs and header values), remote relationships, remote schemas,
actions and source connection settings are not exported; recreate them at the destination.
Database reads and both dumps share one repeatable-read snapshot, which ends as soon
as the dumps and the auth read finish, before objects download. S3 and Hasura
have no cross-service transaction: avoid changing objects or metadata during export.
An object's size changing between enumeration and download fails the export.

Restore the database with ordinary PostgreSQL tools of the source major or newer.
The archive keeps some objects that point at data export leaves out, and it carries
no `CREATE EXTENSION` statements (the dump is scoped to schemas). A plain
`pg_restore` into a vanilla PostgreSQL therefore stops on those items. Use this
recipe instead.

1. Create the database and the extensions the schema uses. List what your schema
   needs (the source's `SELECT extname FROM pg_extension` is the authority):

   ```bash
   createdb destination
   grep -o -w -E 'citext|vector|hstore|ltree|gin_trgm_ops|uuid_generate_v[0-9]+|crypt|gen_salt|digest' \
     portable-bundle/db/schema.sql | sort -u
   # pgcrypto: crypt, gen_salt, digest. uuid-ossp: uuid_generate_v*. pg_trgm: gin_trgm_ops.
   psql destination -c 'CREATE EXTENSION IF NOT EXISTS pgcrypto'
   psql destination -c 'CREATE EXTENSION IF NOT EXISTS citext'
   psql destination -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp"'
   psql destination -c 'CREATE EXTENSION IF NOT EXISTS vector'   # only when the schema uses vector
   ```

2. Skip the items that reference excluded data: foreign keys into `np_*` tables or
   the `auth` schema (unless your destination already has a compatible `auth.users`),
   and Hasura event-trigger functions in `hdb_catalog`, which is not exported.

   ```bash
   cd portable-bundle
   # Names of foreign keys whose REFERENCES target is auth or an np_* table.
   grep -E 'REFERENCES +(auth\.|[A-Za-z0-9_]+\.np_|np_)' db/schema.sql \
     | grep -o 'ADD CONSTRAINT [^ ]*' | awk '{print $3}' > skip-names.txt
   pg_restore -l db/data.dump > full.toc
   { grep ' FK CONSTRAINT ' full.toc | grep -F -w -f skip-names.txt
     grep -E ' TRIGGER .*notify_hasura_' full.toc; } > skip.toc
   grep -v -F -x -f skip.toc full.toc > restore.toc
   pg_restore --exit-on-error --no-owner --no-privileges -L restore.toc --dbname destination db/data.dump
   ```

   Review `skip.toc` before restoring: every line in it is something the destination
   will not have. Recreate those foreign keys once the referenced tables exist.

Auth users are in `auth/users.jsonl`. Load these through your destination's supported
user import process, preserving bcrypt hashes where supported. Auth sessions,
refresh tokens, provider tokens and signing keys are not exported. Database dumps
exclude the platform auth schema; application foreign keys into it need a compatible
destination auth schema and users before the post-data section is restored.

Objects are content-addressed files. Use each manifest object's `bucket`, `key` and
`member` to upload it with any S3 client. For example, with the destination's AWS
credentials already configured:

```bash
jq -c '.storage.objects[]' portable-bundle/manifest.json | while IFS= read -r object; do
  bucket=$(printf '%s' "$object" | jq -r .bucket)
  key=$(printf '%s' "$object" | jq -r .key)
  member=$(printf '%s' "$object" | jq -r .member)
  aws s3 cp "portable-bundle/$member" "s3://$bucket/$key"
done
```

The file mapping preserves distinct S3 keys on case-insensitive filesystems.
Only object bytes are exported, not S3 ACLs, version history or HTTP metadata.

Plugin tables named `np_*`, schemas named `np_*`, `nself_ops`, `hdb_catalog`, and
PostgreSQL system schemas are excluded. Export never carries environment values,
database connection settings, remote Hasura sources, remote schemas or actions.
Hasura permissions, relationships and selected functions remain in the metadata
file for review and application to the destination's configured database source.

Output directories are 0700; files are 0600. Export scans every text member, the
manifest and a plain-SQL rendering of the custom archive for resolved secret values
of at least six bytes: values of secret-named keys (from the project cascade,
`.nself/compose.env` and the process environment), URL userinfo passwords and tokens,
credential-named query parameters, and string fields inside JSON-valued secrets. E526 rejects the entire
bundle and names only the affected member. Bcrypt password hashes in users.jsonl
are permitted. **User object files are not scanned:** inspect them before sharing.
E525 means insufficient disk; E527 means the output is occupied. Failed exports
leave no new bundle. The scan also looks for each secret as it is printed inside
json, jsonb, array, hstore and composite values, in COPY data and in SQL literals,
and in the spellings an application picks when it writes JSON itself and Postgres
stores it as text (`/` written as `\/`, punctuation written as `\u00xx` in lower- or
upper-case hex). **Not detected:** XML entity encoding (`&amp;`, `&lt;`), base64 or
any other application-level encoding, encrypted values, and values shorter than the
six-byte minimum. If an app stores a secret in one of those forms, the scan cannot
see it: inspect those columns before sharing.
Interrupting the command (Ctrl-C, SIGTERM, closing the terminal) removes the private
stage directory. A process killed outright (SIGKILL, power loss) can leave an
unscrubbed `.nself-export-*` directory next to the output; the next export warns
about it on stderr. Remove stale ones with `rm -rf <parent>/.nself-export-*` when no
export is running. Keep the bundle private: it contains application data and
password hashes even after the secret scan passes.

See [[Import-Export-Format]] for the manifest and integrity rules.
