package commands

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
)

func TestDoctorImagesStatus(t *testing.T) {
	mirror := "mirror.example/test"
	ref := compose.Ref{Name: "test", Repository: "upstream.example/test", IndexDigest: "sha256:index", Platforms: map[string]string{"linux/amd64": "sha256:amd", "linux/arm64": "sha256:arm"}, Mirror: &mirror}
	for _, tc := range []struct {
		name, missing, want string
	}{
		{"all present", "", "pass"},
		{"mirror missing", "mirror", "warn"},
		{"upstream missing", "upstream", "warn"},
		{"both missing", "both", "fail"},
		{"network unavailable", "network", "warn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			row := probeLockedImage(context.Background(), ref, func(_ context.Context, image string, _ bool) ([]byte, error) {
				calls++
				if tc.missing == "network" {
					return nil, errors.New("dial tcp: network is unreachable")
				}
				if (strings.HasPrefix(image, "mirror") && (tc.missing == "mirror" || tc.missing == "both")) || (strings.HasPrefix(image, "upstream") && (tc.missing == "upstream" || tc.missing == "both")) {
					return nil, errors.New("manifest unknown: 404")
				}
				return []byte("{}"), nil
			})
			if row.Status != tc.want || calls == 0 {
				t.Fatalf("row=%+v, calls=%d, want %s", row, calls, tc.want)
			}
			if tc.missing == "" && calls != 6 {
				t.Fatalf("did not probe index and both platforms at both registries: %d", calls)
			}
		})
	}
}
