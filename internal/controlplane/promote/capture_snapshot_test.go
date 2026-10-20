package promote

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestCaptureSourceManifestReplaysContainerListing(t *testing.T) {
	for _, failPS := range []bool{false, true} {
		t.Run(fmt.Sprint(failPS), func(t *testing.T) {
			calls := 0
			run := func(_ context.Context, command string) ([]byte, error) {
				switch {
				case strings.Contains(command, "compose-files.txt"), strings.Contains(command, "compose-env-files.txt"):
					return []byte("__NSELF_ABSENT__"), nil
				case strings.Contains(command, " images --format json"):
					return []byte(`[{"ContainerName":"old-api","ID":"fixture"}]`), nil
				case strings.Contains(command, " ps --all --format json"):
					calls++
					if failPS {
						return nil, fmt.Errorf("listing failed")
					}
					if calls > 1 {
						return nil, fmt.Errorf("live listing changed")
					}
					return []byte(`[{"Name":"old-api","Service":"api"}]`), nil
				}
				return nil, fmt.Errorf("unexpected command")
			}
			replay, _, err := CaptureSourceManifest(context.Background(), run, "/srv/fixture")
			if failPS {
				if err == nil {
					t.Fatal("capture ignored ps failure")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("capture ps calls=%d, want 1", calls)
			}
			for i := 0; i < 2; i++ {
				raw, err := replay(context.Background(), "docker compose ps --all --format json")
				if err != nil || !strings.Contains(string(raw), "old-api") {
					t.Fatalf("snapshot=%s error=%v", raw, err)
				}
			}
			if calls != 1 {
				t.Fatalf("compare consulted live ps %d times", calls)
			}
		})
	}
}
