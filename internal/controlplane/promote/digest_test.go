package promote

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
)

func TestPromoteDigestIdentity(t *testing.T) {
	a := "sha256:" + strings.Repeat("a", 64)
	b := "sha256:" + strings.Repeat("b", 64)
	index := "sha256:" + strings.Repeat("c", 64)
	runner := func(_ context.Context, command string) ([]byte, error) {
		switch {
		case command == "docker compose images --format json":
			return []byte(`[{"Service":"api","Repository":"example.invalid/app","Tag":"same"}]`), nil
		case strings.HasPrefix(command, "docker image inspect"):
			return []byte(fmt.Sprintf(`["example.invalid/app@%s"]|linux/amd64`, index)), nil
		default:
			return nil, fmt.Errorf("unexpected command %s", command)
		}
	}
	lookup := func(_ context.Context, ref string) (map[string]string, error) {
		switch ref {
		case index, "example.invalid/app:same":
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
}
