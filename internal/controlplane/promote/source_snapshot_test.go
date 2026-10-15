package promote

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCaptureSourceManifestReturnsContainerListingError(t *testing.T) {
	want := errors.New("source ps unavailable")
	run := func(_ context.Context, command string) ([]byte, error) {
		switch {
		case strings.Contains(command, "compose-files.txt"), strings.Contains(command, "compose-env-files.txt"):
			return []byte("__NSELF_ABSENT__"), nil
		case strings.Contains(command, " images --format json"):
			return []byte(`[{"ContainerName":"custom-name"}]`), nil
		case strings.Contains(command, " ps --all --format json"):
			return nil, want
		default:
			return nil, fmt.Errorf("unexpected command %s", command)
		}
	}
	_, _, err := CaptureSourceManifest(context.Background(), run, "/srv/fixture")
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "list source compose containers") {
		t.Fatalf("container listing error not returned during capture: %v", err)
	}
}

func TestCaptureSourceManifestFreezesContainerServices(t *testing.T) {
	for _, later := range []string{`[]`, `[{"Name":"custom-name","Service":"replacement"}]`} {
		t.Run(later, func(t *testing.T) {
			capturing, psCalls := true, 0
			run := func(ctx context.Context, command string) ([]byte, error) {
				switch {
				case strings.Contains(command, "compose-files.txt"), strings.Contains(command, "compose-env-files.txt"):
					return []byte("__NSELF_ABSENT__"), nil
				case strings.Contains(command, " images --format json"):
					return []byte(`[{"ID":"sha256:running","ContainerName":"custom-name","Repository":"example.invalid/app","Tag":"same"}]`), nil
				case strings.Contains(command, " ps --all --format json"):
					psCalls++
					if capturing {
						return []byte(`[{"Name":"custom-name","Service":"api"}]`), nil
					}
					return []byte(later), nil
				case strings.Contains(command, " config --format json"):
					return []byte(`{"services":{"api":{"image":"example.invalid/app:same"}}}`), nil
				case strings.HasPrefix(command, "docker image inspect"):
					return serviceRunner("", "")(ctx, command)
				default:
					return nil, fmt.Errorf("unexpected command %s", command)
				}
			}
			ctx := context.Background()
			captured, raw, err := CaptureSourceManifest(ctx, run, "/srv/fixture")
			if err != nil {
				t.Fatal(err)
			}
			capturing = false
			images, err := CaptureManifestSource(ctx, captured, raw, serviceLookup)
			if err != nil || len(images) != 1 || images[0].Service != "api" || psCalls != 1 {
				t.Fatalf("captured service mapping changed: images=%#v err=%v ps calls=%d", images, err, psCalls)
			}
		})
	}
}
