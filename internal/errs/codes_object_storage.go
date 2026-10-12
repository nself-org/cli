// Registry fragment: core object storage (E138 MinIO moved, E142 credentials), owned by P7-PLUG-60.
package errs

func init() {
	Register(CodeEntry{
		Code: "E138", Category: "plugin", Summary: "MinIO moved to a plugin", Exit: 1,
		DefaultWhy: "In v1.5 core object storage is SeaweedFS and MinIO is no longer a core service.",
		DefaultFix: "Run nself add minio to keep the minio_data volume and minio service name with unchanged data, or set OBJECT_STORAGE_ENABLED=true and MINIO_ENABLED=false for a new empty SeaweedFS store, or set MINIO_ENABLED=false if this project does not use MinIO. Back up and move existing data dump-first.",
		DocsPath:   "reference/error-codes#e138",
	})
	Register(CodeEntry{
		Code: "E142", Category: "plugin", Summary: "Object storage credentials missing or unusable", Exit: 1,
		DefaultWhy: "OBJECT_STORAGE_ENABLED=true needs S3_ACCESS_KEY and S3_SECRET_KEY: they are the SeaweedFS S3 identity, and the store refuses to start without usable ones.",
		DefaultFix: "Set both in .env.secrets. Each must be nonempty and contain no double quote, backslash or control character.",
		DocsPath:   "reference/error-codes#e142",
	})
}
