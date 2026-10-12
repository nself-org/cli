package promote

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
)

func TestPromoteServicePlatformDrift(t *testing.T) {
	ctx := context.Background()
	a, b := "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64)
	lookup := func(context.Context, string) (map[string]string, error) {
		return map[string]string{"linux/amd64": a, "linux/arm64": b}, nil
	}
	// The merged fragment overrides the platform, even though the target host is amd64.
	run := func(context.Context, string) ([]byte, error) {
		return []byte(`{"services":{"api":{"image":"example.invalid/api:multi","platform":"linux/arm64"}}}`), nil
	}
	images, err := ResolvedComposeImages(ctx, run, []string{"/srv/fixture/docker-compose.yml", "/srv/fixture/fragment.yml"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := CompareDigests(ctx, []SourceImage{{Service: "api", Platform: "linux/amd64", Digest: a}}, images, "amd64", controlplane.TierProd, lookup, nil)
	if err == nil || !strings.Contains(err.Error(), "E490") || !strings.Contains(err.Error(), "api") {
		t.Fatalf("service platform override silently dropped: rows=%#v err=%v", rows, err)
	}
	if len(rows) != 1 || rows[0].Target != b {
		t.Fatalf("target did not select the arm64 manifest: %#v", rows)
	}
	rows, err = CompareDigests(ctx, []SourceImage{{Service: "api", Platform: "linux/arm64", Digest: b}}, images, "amd64", controlplane.TierProd, lookup, nil)
	if err != nil || rows[0].Status != "equal" {
		t.Fatalf("equal platform override rejected: %#v %v", rows, err)
	}
	// Even identical manifest digests cannot hide a different selected platform.
	_, err = CompareDigests(ctx, []SourceImage{{Service: "api", Platform: "linux/amd64", Digest: b}}, images, "amd64", controlplane.TierProd, lookup, nil)
	if err == nil || !strings.Contains(err.Error(), "target selects linux/arm64") {
		t.Fatalf("platform mismatch with equal digest accepted: %v", err)
	}
}

func TestCaptureManifestSourcePlatform(t *testing.T) {
	ctx := context.Background()
	a := "sha256:" + strings.Repeat("a", 64)
	index := "sha256:" + strings.Repeat("c", 64)
	platform := "linux/arm64"
	run := func(_ context.Context, command string) ([]byte, error) {
		if strings.Contains(command, " config --format json") {
			return []byte(`{"services":{"api":{"image":"example.invalid/api:multi","platform":"linux/arm64"}}}`), nil
		}
		if strings.HasPrefix(command, "docker image inspect") {
			return []byte(`["example.invalid/api@` + index + `"]|` + platform), nil
		}
		t.Fatalf("unexpected command %s", command)
		return nil, nil
	}
	lookup := func(context.Context, string) (map[string]string, error) {
		return map[string]string{"linux/arm64": a, "linux/amd64": a}, nil
	}
	raw := []byte(`[{"Service":"api","Repository":"example.invalid/api","Tag":"multi","ID":"sha256:fixture"}]`)
	images, err := CaptureManifestSource(ctx, run, raw, lookup)
	if err != nil || len(images) != 1 || images[0].Platform != platform {
		t.Fatalf("source platform capture: %#v %v", images, err)
	}
	platform = "linux/amd64"
	_, err = CaptureManifestSource(ctx, run, raw, lookup)
	if err == nil || !strings.Contains(err.Error(), "E490") || !strings.Contains(err.Error(), "api") {
		t.Fatalf("source declared platform discarded: %v", err)
	}
}

func TestTargetArchitectureConfiguredDoesNotProbe(t *testing.T) {
	arch, err := TargetArchitecture(context.Background(), " x86_64 ", nil)
	if err != nil || arch != "amd64" {
		t.Fatalf("configured architecture = %q, %v", arch, err)
	}
}
