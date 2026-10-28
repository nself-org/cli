# Object Storage

← [[Configuration]] | [[Config-Env-Vars]] →

---

SeaweedFS is the core S3-compatible object store in v1.5 mode (`NSELF_V15=1`). Object storage is optional. Enable it for a new project with:

```dotenv
OBJECT_STORAGE_ENABLED=true
OBJECT_STORAGE_BUCKETS=uploads,archives
```

Put `S3_ACCESS_KEY` and `S3_SECRET_KEY` in your untracked `.env.secrets`. Both must be nonempty and must not contain a double quote, a backslash or a control character. `nself build` checks them first and stops with **E142: Object storage credentials missing or unusable**, naming the variable (never its value). Then run `nself build` and `nself start`.

The `seaweedfs` and `seaweedfs-init` services use the pinned image lock. `SEAWEEDFS_VERSION` overrides the version for both services; leave it empty to use the lock default. The store uses the `seaweedfs_data` volume. Its identity JSON is created inside the container on `/tmp` tmpfs from environment variables. It never becomes a project file or a persistent volume file. Generated compose contains credential references; the CLI supplies their values through the private `.nself/compose.env` file.

## Buckets

`OBJECT_STORAGE_BUCKETS` is a comma-separated list. The one-shot `seaweedfs-init` service waits for SeaweedFS health, creates missing buckets and leaves existing buckets intact. An empty list creates none. Names must follow S3 bucket naming: 3–63 characters, lowercase letters, digits, hyphens or dots.

Custom services receive two bucket variables in v1.5. `S3_BUCKETS` is the full comma-separated list (`uploads,archives`). `S3_BUCKET` is a single bucket name that any S3 client accepts: an explicit `S3_BUCKET` from your `.env` when set, otherwise the first bucket in the list. With an empty list both are empty. The same two variables apply when the `minio` plugin is the active store, built from `MINIO_DEFAULT_BUCKETS`. v1.4 keeps passing the raw `MINIO_DEFAULT_BUCKETS` list as `S3_BUCKET`.

## Endpoints and active store

Custom services receive `S3_ENDPOINT=http://seaweedfs:8333` and the S3 credential references when core object storage is enabled. If core object storage is disabled, the `minio` plugin is installed and the project still declares MinIO (`MINIO_ENABLED=true`, or `MINIO_ROOT_*` credentials), they receive `S3_ENDPOINT=http://minio:9000` with the existing MinIO key mapping. An installed plugin alone does nothing for a project that does not use MinIO, because the plugin is machine-wide. With neither store, no `S3_*` variable is injected. Profiles without object storage, such as `ops`, never inject them.

`nself build` also writes the active store's internal `S3_ENDPOINT` and configured `MINIO_REGION` as `S3_REGION` to `.nself/compose.env`. Plugin compose fragments therefore resolve the same store as custom services. The computed endpoint wins over a stale project-level `S3_ENDPOINT` while a store is active. With no active store, the CLI writes neither value, so a plugin's explicit project configuration or built-in fallback keeps working.

With both core object storage enabled and the MinIO plugin installed, custom services get the core SeaweedFS endpoint. Choose one store deliberately before moving an application to its endpoint.

The host endpoint is `http://127.0.0.1:8333`. `SEAWEEDFS_PORT` changes the host port only, for example `SEAWEEDFS_PORT=18333`. It must be an integer from 1 through 65535. Only the S3 port is published, always on loopback. Inside the Docker network only S3 on port 8333 is reachable from other containers: the SeaweedFS master, volume and filer listen on loopback inside the store container, and the bucket bootstrap runs in the store's own network namespace. They carry no authentication, so they are never exposed to other services.

## Existing MinIO data

In v1.5 mode, explicit MinIO intent without the MinIO plugin fails a profile that includes object storage with **E138: MinIO moved to a plugin** before generated project files change. This includes `MINIO_ENABLED=true`, even when `OBJECT_STORAGE_ENABLED=true` is also set. A profile without object storage, such as `ops`, does not fail this check. S3 credentials alone with core object storage enabled do not request MinIO.

E138 names three ways out: run `nself add minio` to keep the `minio_data` volume; set `OBJECT_STORAGE_ENABLED=true` and `MINIO_ENABLED=false` for a new empty SeaweedFS store; or set `MINIO_ENABLED=false` when this project does not use MinIO.

If this project does not use MinIO at all, set `MINIO_ENABLED=false` in `.env`. MinIO credentials such as `MINIO_ROOT_USER` and `MINIO_ROOT_PASSWORD` also count as MinIO intent when `MINIO_ENABLED` is not set, which is why `nself init` writes `MINIO_ENABLED=false`.

To keep your existing store, run (this needs the minio plugin, coming with the v1.5 release):

```bash
nself add minio
```

The plugin keeps the `minio` service name and attaches the existing `minio_data` volume. Installing it is not a data migration. Keep `MINIO_ENABLED=true` in `.env` after `nself add minio`: it is what tells the project to use the plugin's store.

To move your data into SeaweedFS, back up the old store first, then run both stores at once in v1.5 mode: keep the minio plugin installed with `MINIO_ENABLED=true`, and add `OBJECT_STORAGE_ENABLED=true` with the S3 keys. (v1.4 mode cannot generate SeaweedFS, so the copy happens in v1.5.) Copy with `mc mirror` or `aws s3 sync` from `minio:9000` to `seaweedfs:8333`, either from a one-off container on the project network or from the host through `127.0.0.1:<MINIO_PORT>` and `127.0.0.1:<SEAWEEDFS_PORT>`. SeaweedFS starts with a separate, empty `seaweedfs_data` volume. Verify objects and application access, recreate bucket policies and CORS settings as needed, and point applications at the new endpoint. Then remove the plugin and set `MINIO_ENABLED=false` (the `MINIO_ROOT_*` keys alone still read as MinIO intent). The `minio_data` volume is no longer declared in compose after that, but nself leaves it on disk and never deletes it. Once you have verified the copy, remove it yourself with `docker volume rm <project>_minio_data`.

MinIO community editions have no bucket CORS API; their `MINIO_API_CORS_ALLOW_ORIGIN` setting applies server-wide. The storage plugin's S3 coverage suite exercises SeaweedFS operations, including bucket CORS. The generated-store check is `bash scripts/e2e/object-storage.sh SUITE_DIR`, where `SUITE_DIR` is the storage plugin's `test/s3coverage` directory. It starts an isolated Docker daemon and checks boot, bucket creation and rerun, the suite, credential references and E138's unchanged project hash. Passing generator tests alone does not establish that this runtime suite passed.

## Compatibility

With v1.5 mode off, `MINIO_ENABLED` retains the existing core MinIO behavior. `OBJECT_STORAGE_ENABLED` does not change v1.4 compose output. See [[Compat-V15]].
