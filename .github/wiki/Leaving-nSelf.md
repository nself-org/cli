# Leaving nSelf

Export a started project to a new or empty directory:

```bash
nself db export --out ./portable-bundle --json
```

Choose schemas with `--schemas public,app`. Omit object files with `--no-storage`.
The output's parent directory must exist. Export reads the stack without stopping
services or changing source data. It checks available disk before writing output.
The estimate is 1.2 times the selected tables' relation sizes plus object bytes;
leave additional room for large metadata, auth records and the SQL scan.

The bundle contains a custom-format PostgreSQL archive, a readable schema dump,
table counts and content hashes, portable auth users with bcrypt password hashes,
S3 objects, Hasura metadata for selected tables, and configuration key **names**.
The metadata keeps table configuration, relationships, computed fields and permissions.
Event triggers (webhook URLs and header values), remote relationships, remote schemas,
actions and source connection settings are not exported; recreate them at the destination.
Database reads and both dumps share one repeatable-read snapshot. S3 and Hasura
have no cross-service transaction: avoid changing objects or metadata during export.
An object's size changing between enumeration and download fails the export.

Restore the database with ordinary PostgreSQL tools of the source major or newer:

```bash
createdb destination
pg_restore --exit-on-error --no-owner --no-privileges --dbname destination portable-bundle/db/data.dump
```

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
of at least six bytes, including URL userinfo passwords. E526 rejects the entire
bundle and names only the affected member. Bcrypt password hashes in users.jsonl
are permitted. **User object files are not scanned:** inspect them before sharing.
E525 means insufficient disk; E527 means the output is occupied. Failed exports
leave no new bundle. Keep the bundle private: it contains application data and
password hashes even after the secret scan passes.

See [[Import-Export-Format]] for the manifest and integrity rules.
