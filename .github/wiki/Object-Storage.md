# Object Storage

← [[Configuration]] | [[Config-Env-Vars]] →

---

SeaweedFS is the core S3-compatible object store in v1.5 mode (`NSELF_V15=1`). Object storage is optional. Enable it for a new project with:

```dotenv
OBJECT_STORAGE_ENABLED=true
OBJECT_STORAGE_BUCKETS=uploads,archives
```

Put `S3_ACCESS_KEY` and `S3_SECRET_KEY` in your untracked `.env.secrets`. Both must be nonempty and must not contain a double quote or backslash. Then run `nself build` and `nself start`.

The `seaweedfs` and `seaweedfs-init` services use the pinned image lock. `SEAWEEDFS_VERSION` overrides the version for both services; leave it empty to use the lock default. The store uses the `seaweedfs_data` volume. Its identity JSON is created inside the container on `/tmp` tmpfs from environment variables. It never becomes a project file or a persistent volume file. Generated compose contains credential references; the CLI supplies their values through the private `.nself/compose.env` file.

## Buckets

`OBJECT_STORAGE_BUCKETS` is a comma-separated list. The one-shot `seaweedfs-init` service waits for SeaweedFS health, creates missing buckets and leaves existing buckets intact. An empty list creates none. Names must follow S3 bucket naming: 3–63 characters, lowercase letters, digits, hyphens or dots.

## Endpoints and active store

Custom services receive `S3_ENDPOINT=http://seaweedfs:8333` and the S3 credential references when core object storage is enabled. If core object storage is disabled and the `minio` plugin is installed, they receive `S3_ENDPOINT=http://minio:9000` with the existing MinIO key mapping. With neither store, no endpoint is injected.

With both core object storage enabled and the MinIO plugin installed, custom services get the core SeaweedFS endpoint. Choose one store deliberately before moving an application to its endpoint.

The host endpoint is `http://127.0.0.1:8333`. `SEAWEEDFS_PORT` changes the host port only, for example `SEAWEEDFS_PORT=18333`. It must be an integer from 1 through 65535. Only the S3 port is published, always on loopback; master and filer ports stay inside the Docker network.

## Existing MinIO data

In v1.5 mode, explicit MinIO intent without the MinIO plugin fails a profile that includes object storage with **E138: MinIO moved to a plugin** before generated project files change. This includes `MINIO_ENABLED=true`, even when `OBJECT_STORAGE_ENABLED=true` is also set. A profile without object storage, such as `ops`, does not fail this check. S3 credentials alone with core object storage enabled do not request MinIO.

To keep your existing store, run:

```bash
nself add minio
```

The plugin keeps the `minio` service name and attaches the existing `minio_data` volume. Installing it is not a data migration.

To move to SeaweedFS, first back up and export the old store. Keep the old store available while copying objects with `mc mirror` or `aws s3 sync` between its endpoint and the new endpoint. SeaweedFS starts with a separate, empty `seaweedfs_data` volume. Verify objects and application access, recreate bucket policies and CORS settings as needed, then change application endpoints. Remove `MINIO_ENABLED` and set `OBJECT_STORAGE_ENABLED=true` for the new core store. Do not discard the backup or old volume until verification is complete.

MinIO community editions have no bucket CORS API; their `MINIO_API_CORS_ALLOW_ORIGIN` setting applies server-wide. The storage plugin's S3 coverage suite exercises SeaweedFS operations, including bucket CORS. The generated-store check is `bash scripts/e2e/object-storage.sh SUITE_DIR`, where `SUITE_DIR` is the storage plugin's `test/s3coverage` directory. It starts an isolated Docker daemon and checks boot, bucket creation and rerun, the suite, credential references and E138's unchanged project hash. Passing generator tests alone does not establish that this runtime suite passed.

## Compatibility

With v1.5 mode off, `MINIO_ENABLED` retains the existing core MinIO behavior. `OBJECT_STORAGE_ENABLED` does not change v1.4 compose output. See [[Compat-V15]].
