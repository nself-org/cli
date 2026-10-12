// Registry fragment: core object storage migration (E138), owned by P7-PLUG-60.
package errs

func init() {
	Register(CodeEntry{
		Code: "E138", Category: "plugin", Summary: "MinIO moved to a plugin", Exit: 1,
		DefaultWhy: "In v1.5 core object storage is SeaweedFS and MinIO is no longer a core service.",
		DefaultFix: "Run nself add minio to keep the minio_data volume and minio service name with unchanged data, or set OBJECT_STORAGE_ENABLED=true and remove MINIO_ENABLED for a new empty SeaweedFS store. Back up and move existing data dump-first.",
		DocsPath:   "reference/error-codes#e138",
	})
}
