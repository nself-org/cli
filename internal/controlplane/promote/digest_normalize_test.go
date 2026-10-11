package promote

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/controlplane"
)

func TestPromoteArm64DefaultVariant(t *testing.T) {
	ctx := context.Background()
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, inspected := range []string{"linux/arm64", "linux/arm64/v8"} {
		for _, registry := range []string{"linux/arm64", "linux/arm64/v8"} {
			t.Run(inspected+"/"+registry, func(t *testing.T) {
				run := func(_ context.Context, command string) ([]byte, error) {
					if command == "docker compose images --format json" {
						return []byte(`[{"Service":"api","Repository":"example.invalid/api","Tag":"same","ID":"sha256:fixture"}]`), nil
					}
					return []byte(fmt.Sprintf(`["example.invalid/api@%s"]|%s`, digest, inspected)), nil
				}
				lookup := func(context.Context, string) (map[string]string, error) {
					return map[string]string{registry: digest}, nil
				}
				images := []Image{{Service: "api", Ref: "example.invalid/api:same", Platform: "linux/arm64/v8"}}
				source, err := CaptureSource(ctx, run, images, lookup)
				if err != nil {
					t.Fatalf("equal source variant rejected: %v", err)
				}
				if source[0].Platform != "linux/arm64" {
					t.Fatalf("source variant not normalized: %#v", source)
				}
				rows, err := CompareDigests(ctx, source, images, "arm64", controlplane.TierProd, lookup, nil)
				if err != nil || rows[0].Status != "equal" {
					t.Fatalf("equal target variant rejected: %#v %v", rows, err)
				}
				_, err = CompareDigests(ctx, []SourceImage{{Service: "api", Platform: "linux/arm64/v8", Digest: digest}}, []Image{{Service: "api", Ref: images[0].Ref, Platform: "linux/arm64"}}, "arm64", controlplane.TierProd, lookup, nil)
				if err != nil {
					t.Fatalf("explicit source variant rejected: %v", err)
				}
			})
		}
	}
	for _, locked := range compose.LockedImages() {
		if expected := locked.Platforms["linux/arm64"]; expected != "" {
			got, err := targetPlatformDigest(ctx, locked.String(), "linux/arm64/v8", nil)
			if err != nil || got != expected {
				t.Fatalf("locked target variant rejected: %q %v", got, err)
			}
			got, err = sourcePlatformDigest(ctx, locked.String(), locked.IndexDigest, "linux/arm64/v8", nil)
			if err != nil || got != expected {
				t.Fatalf("locked source variant rejected: %q %v", got, err)
			}
			return
		}
	}
	t.Fatal("fixture lock has no arm64 manifest")
}

func TestPromoteArmVariantsRemainDistinct(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	lookup := func(context.Context, string) (map[string]string, error) {
		return map[string]string{"linux/arm/v6": digest, "linux/arm/v7": digest}, nil
	}
	_, err := CompareDigests(context.Background(), []SourceImage{{Service: "api", Platform: "linux/arm/v6", Digest: digest}}, []Image{{Service: "api", Ref: "example.invalid/api:same", Platform: "linux/arm/v7"}}, "arm", controlplane.TierProd, lookup, nil)
	if err == nil || !strings.Contains(err.Error(), "E490") || !strings.Contains(err.Error(), "source runs linux/arm/v6; target selects linux/arm/v7") {
		t.Fatalf("distinct ARM variants accepted: %v", err)
	}
}
