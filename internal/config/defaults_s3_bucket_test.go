package config

import (
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestS3BucketDefaultCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name     string
		v15      bool
		explicit string
		want     string
	}{
		{"v14-default", false, "", "nself"},
		{"v14-explicit", false, "archives", "archives"},
		{"v15-unset", true, "", ""},
		{"v15-explicit", true, "archives", "archives"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15)
			cfg := &Config{Minio: MinioConfig{S3Bucket: tc.explicit}}
			applyDefaultsMinioStorage(cfg)
			if cfg.Minio.S3Bucket != tc.want {
				t.Fatalf("S3Bucket=%q, want %q", cfg.Minio.S3Bucket, tc.want)
			}
		})
	}
}
