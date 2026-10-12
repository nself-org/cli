package compose

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestObjectStoragePreflight: the build preflight raises E138 under the same rule
// as Generate (v1.5, MinIO intent, no plugin), never for a profile without object
// storage, and never in v1.4.
func TestObjectStoragePreflight(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		for _, tc := range []struct {
			name             string
			minio, installed bool
			profile          ProfileName
			wantE138         bool
		}{
			{"minio-no-plugin", true, false, "", true},
			{"minio-no-plugin-app", true, false, ProfileApp, true},
			{"minio-with-plugin", true, true, "", false},
			{"no-minio", false, false, "", false},
			{"ops-profile", true, false, ProfileOps, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				storagePluginSeam(t, tc.installed)
				cfg := storageFixture(t, tc.minio, false, false)
				err := ObjectStoragePreflight(cfg, tc.profile)
				want := tc.wantE138 && compat.V15()
				if got := err != nil && strings.Contains(err.Error(), "E138"); got != want {
					t.Fatalf("preflight error %v, want E138=%v", err, want)
				}
				if !want && err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			})
		}
	})
}

// TestActiveStoreNeedsMinioIntent: in v1.5 an installed minio plugin is the
// active store only for a project with MinIO intent.
func TestActiveStoreNeedsMinioIntent(t *testing.T) {
	compattest.Set(t, true)
	for _, tc := range []struct {
		minio, object, installed bool
		want                     string
	}{
		{false, false, true, "none"},
		{true, false, true, "minio-plugin"},
		{true, true, true, "seaweedfs"},
		{false, true, false, "seaweedfs"},
		{false, false, false, "none"},
	} {
		cfg := storageFixture(t, tc.minio, tc.object, false)
		if got := ActiveStore(cfg, tc.installed); got != tc.want {
			t.Fatalf("ActiveStore(minio=%v object=%v installed=%v) = %q, want %q", tc.minio, tc.object, tc.installed, got, tc.want)
		}
	}
}
