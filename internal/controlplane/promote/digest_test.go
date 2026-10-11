package promote

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/controlplane"
)

func TestPromoteDigestIdentity(t *testing.T) {
	a := "sha256:" + strings.Repeat("a", 64)
	b := "sha256:" + strings.Repeat("b", 64)
	index := "sha256:" + strings.Repeat("c", 64)
	runner := func(_ context.Context, command string) ([]byte, error) {
		switch {
		case command == "docker compose images --format json":
			return []byte(`[{"Service":"api","Repository":"example.invalid/app","Tag":"same","ID":"sha256:running"}]`), nil
		case strings.HasPrefix(command, "docker image inspect"):
			if !strings.Contains(command, "'sha256:running'") {
				return nil, fmt.Errorf("inspected mutable tag instead of image ID: %s", command)
			}
			return []byte(fmt.Sprintf(`["example.invalid/app@%s"]|linux/amd64`, index)), nil
		default:
			return nil, fmt.Errorf("unexpected command %s", command)
		}
	}
	lookup := func(_ context.Context, ref string) (map[string]string, error) {
		switch ref {
		case "example.invalid/app@" + index, "example.invalid/app:same":
			return map[string]string{"linux/amd64": a, "linux/arm64": b}, nil
		case "example.invalid/app:rebuilt":
			return map[string]string{"linux/amd64": b}, nil
		default:
			return nil, fmt.Errorf("unknown ref %s", ref)
		}
	}
	source, err := CaptureSource(context.Background(), runner, []Image{{Service: "api", Ref: "example.invalid/app:same"}}, lookup)
	if err != nil || len(source) != 1 || source[0].Digest != a {
		t.Fatalf("CaptureSource() = %#v, %v", source, err)
	}
	rows, err := CompareDigests(context.Background(), source, []Image{{Service: "api", Ref: "example.invalid/app:same"}}, "amd64", controlplane.TierProd, lookup, nil)
	if err != nil || len(rows) != 1 || rows[0].Status != "equal" || rows[0].Target != a {
		t.Fatalf("same tag comparison = %#v, %v", rows, err)
	}
	_, err = CompareDigests(context.Background(), source, []Image{{Service: "api", Ref: "example.invalid/app:rebuilt"}}, "amd64", controlplane.TierProd, lookup, nil)
	if err == nil || !strings.Contains(err.Error(), "E490") || !strings.Contains(err.Error(), "api") {
		t.Fatalf("rebuilt comparison error = %v", err)
	}
	_, err = CompareDigests(context.Background(), nil, []Image{{Service: "worker", Build: true}}, "amd64", controlplane.TierProd, lookup, nil)
	if err == nil || !strings.Contains(err.Error(), "unverifiable") || !strings.Contains(err.Error(), "worker") {
		t.Fatalf("prod build-context error = %v", err)
	}
	var warning string
	rows, err = CompareDigests(context.Background(), nil, []Image{{Service: "worker", Build: true}}, "amd64", controlplane.TierLocalServers, lookup, func(s string) { warning = s })
	if err != nil || !strings.Contains(warning, "worker") || rows[0].Status != "unverifiable" {
		t.Fatalf("local-server build context = %#v, %q, %v", rows, warning, err)
	}
	locked := compose.LockedImages()[0]
	rebuilt, err := sourcePlatformDigest(context.Background(), locked.LegacyRef, b, "linux/amd64", func(_ context.Context, ref string) (map[string]string, error) {
		if !strings.HasSuffix(ref, "@"+b) {
			t.Fatalf("rebuilt locked tag looked up as %q, want immutable digest %s", ref, b)
		}
		return map[string]string{"linux/amd64": b}, nil
	})
	if err != nil || rebuilt != b {
		t.Fatalf("rebuilt locked tag resolved to %q, %v", rebuilt, err)
	}
	rows, err = CompareDigests(context.Background(), []SourceImage{{Service: "locked", Digest: a}}, []Image{{Service: "locked", Ref: locked.LegacyRef}}, "amd64", controlplane.TierProd, func(_ context.Context, ref string) (map[string]string, error) {
		if ref != locked.LegacyRef {
			t.Fatalf("target legacy lookup ref = %q, want %q", ref, locked.LegacyRef)
		}
		return map[string]string{"linux/amd64": b}, nil
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "E490") || rows[0].Target != b {
		t.Fatalf("rebuilt locked target tag = %#v, %v", rows, err)
	}
	rows, err = CompareDigests(context.Background(), []SourceImage{{Service: "api", Digest: "malformed"}}, []Image{{Service: "api", Ref: "example.invalid/app:bad"}}, "amd64", controlplane.TierProd, func(context.Context, string) (map[string]string, error) {
		return map[string]string{"linux/amd64": "malformed"}, nil
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "E490") || rows[0].Status != "drift" {
		t.Fatalf("malformed target digest = %#v, %v", rows, err)
	}
}

func TestPromoteDigestEmptyAndBuild(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if _, err := CompareDigests(context.Background(), nil, nil, "amd64", controlplane.TierProd, nil, nil); err == nil || !strings.Contains(err.Error(), "E490") {
			t.Fatalf("empty comparison = %v", err)
		}
	})
	t.Run("build", func(t *testing.T) {
		_, err := CompareDigests(context.Background(), []SourceImage{{Service: "worker", Digest: "old"}}, []Image{{Service: "worker", Build: true}}, "amd64", controlplane.TierLocalServers, nil, nil)
		if err != nil {
			t.Fatalf("build warning turned into drift: %v", err)
		}
	})
}

func TestPromoteDigestLookupCause(t *testing.T) {
	rows, err := CompareDigests(context.Background(), []SourceImage{{Service: "api", Digest: "old"}}, []Image{{Service: "api", Ref: "example.invalid/api:same"}}, "amd64", controlplane.TierProd, func(context.Context, string) (map[string]string, error) {
		return nil, fmt.Errorf("registry unavailable")
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "registry unavailable") {
		t.Fatalf("lookup cause lost: %#v %v", rows, err)
	}
}
