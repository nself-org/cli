package compose

import (
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

func TestCustomServiceS3Buckets(t *testing.T) {
	for _, v15 := range []bool{false, true} {
		for _, store := range []string{"seaweedfs", "minio"} {
			for _, tc := range []struct{ name, buckets, explicit, want string }{
				{"empty", "", "", ""},
				{"one", "uploads", "", "uploads"},
				{"two", "uploads,archives", "", "uploads"},
				{"explicit", "uploads,archives", "archives", "archives"},
				{"explicit-empty-list", "", "uploads", "uploads"},
			} {
				mode := "v14"
				if v15 {
					mode = "v15"
				}
				t.Run(mode+"/"+store+"/"+tc.name, func(t *testing.T) {
					compattest.Set(t, v15)
					storagePluginSeam(t, true)
					cfg := storageFixture(t, true, store == "seaweedfs", false)
					t.Setenv("OBJECT_STORAGE_BUCKETS", tc.buckets)
					cfg.Minio.DefaultBuckets = tc.buckets
					cfg.Minio.S3Bucket = tc.explicit
					env := coreEnvVars(cfg, config.CustomService{Name: "worker"}, nil)
					want := tc.want
					if !v15 {
						want = tc.buckets
					}
					if env["S3_BUCKET"] != want {
						t.Errorf("S3_BUCKET=%q, want %q", env["S3_BUCKET"], want)
					}
					buckets, present := env["S3_BUCKETS"]
					if present != v15 || (v15 && buckets != tc.buckets) {
						t.Errorf("S3_BUCKETS=%q present=%t, want %q present=%t", buckets, present, tc.buckets, v15)
					}
					if v15 {
						filtered := coreEnvVarsFor(cfg, config.CustomService{}, nil, false)
						if _, ok := filtered["S3_BUCKETS"]; ok {
							t.Error("S3_BUCKETS injected into profile without object storage")
						}
					}
				})
			}
		}
	}
}
