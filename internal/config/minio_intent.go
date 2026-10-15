package config

import (
	"os"

	"github.com/nself-org/cli/internal/compat"
)

// MinioEnabledFromEnv shares the loader's MinIO intent rule with plugin discovery.
// Purpose: keep service generation and machine-wide fragment selection consistent.
// Inputs: process environment after the caller resolves the project env cascade.
// Outputs: whether the project requests MinIO.
// Constraints: read-only; preserves legacy toggles and credential inference.
func MinioEnabledFromEnv() bool {
	_, minioEnabledSet := os.LookupEnv("MINIO_ENABLED")
	_, storageEnabledSet := os.LookupEnv("STORAGE_ENABLED")
	minioCredentialVars := []string{
		"MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD",
		"MINIO_ACCESS_KEY", "MINIO_SECRET_KEY",
	}
	coreS3KeyVars := []string{"S3_ACCESS_KEY", "S3_SECRET_KEY"}
	minioIntentVars := append(append([]string{}, minioCredentialVars...), coreS3KeyVars...)
	// compat.V15(P7-PLUG-60): S3 keys infer MinIO intent -> core object storage keys do not enable MinIO
	if compat.V15() && getEnvBool("OBJECT_STORAGE_ENABLED", false) {
		minioIntentVars = minioCredentialVars
	}
	hasMinioIntent := false
	for _, k := range minioIntentVars {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			hasMinioIntent = true
			break
		}
	}
	minioEnabled := getEnvBool("MINIO_ENABLED", false) || getEnvBool("STORAGE_ENABLED", false)
	if !minioEnabled && !minioEnabledSet && !storageEnabledSet && hasMinioIntent {
		minioEnabled = true
	}
	return minioEnabled
}
