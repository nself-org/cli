package promote

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// composeImagesV56 is `docker compose images --format json` as Compose 5.6
// prints it: ContainerName and no Service. Two replicas of api share one image.
const composeImagesV56 = `[{"ID":"sha256:running","ContainerName":"custom-name","Repository":"example.invalid/app","Tag":"same","Platform":"linux/amd64","Size":1},` +
	`{"ID":"sha256:running","ContainerName":"proj-api-1","Repository":"example.invalid/app","Tag":"same","Platform":"linux/amd64","Size":1},` +
	`{"ID":"sha256:running","ContainerName":"proj-api-2","Repository":"example.invalid/app","Tag":"same","Platform":"linux/amd64","Size":1}]`

func serviceRunner(images, ps string) Runner {
	return func(_ context.Context, command string) ([]byte, error) {
		switch {
		case command == "docker compose images --format json":
			return []byte(images), nil
		case command == "docker compose ps --all --format json":
			return []byte(ps), nil
		case strings.HasPrefix(command, "docker image inspect"):
			return []byte(`["example.invalid/app@sha256:` + strings.Repeat("c", 64) + `"]|linux/amd64`), nil
		default:
			return nil, fmt.Errorf("unexpected command %s", command)
		}
	}
}

func serviceLookup(context.Context, string) (map[string]string, error) {
	return map[string]string{"linux/amd64": "sha256:" + strings.Repeat("a", 64)}, nil
}

// Real Compose output names containers, not services: capture maps them through ps,
// in both the NDJSON (Compose >= 2.21) and the array form.
func TestCaptureSourceComposeContainerNames(t *testing.T) {
	declared := []Image{{Service: "api", Ref: "example.invalid/app:same"}, {Service: "worker", Ref: "example.invalid/app:same"}}
	for name, ps := range map[string]string{
		"ndjson": `{"Name":"custom-name","Service":"worker"}` + "\n" + `{"Name":"proj-api-1","Service":"api"}` + "\n" + `{"Name":"proj-api-2","Service":"api"}` + "\n",
		"array":  `[{"Name":"custom-name","Service":"worker"},{"Name":"proj-api-1","Service":"api"},{"Name":"proj-api-2","Service":"api"}]`,
	} {
		got, err := CaptureSource(context.Background(), serviceRunner(composeImagesV56, ps), declared, serviceLookup)
		if err != nil || len(got) != 2 || got[0].Service != "api" || got[1].Service != "worker" {
			t.Fatalf("%s: CaptureSource() = %#v, %v", name, got, err)
		}
	}
}

// A container ps does not name, or replicas running different images, fail closed.
func TestCaptureSourceComposeContainerNamesFailClosed(t *testing.T) {
	declared := []Image{{Service: "api", Ref: "example.invalid/app:same"}}
	if _, err := CaptureSource(context.Background(), serviceRunner(composeImagesV56, `{"Name":"proj-api-1","Service":"api"}`), declared, serviceLookup); err == nil || !strings.Contains(err.Error(), "no service name") {
		t.Fatalf("unmapped container accepted: %v", err)
	}
	mixed := strings.Replace(composeImagesV56, `"sha256:running","ContainerName":"proj-api-2"`, `"sha256:other","ContainerName":"proj-api-2"`, 1)
	ps := `[{"Name":"custom-name","Service":"api"},{"Name":"proj-api-1","Service":"api"},{"Name":"proj-api-2","Service":"api"}]`
	if _, err := CaptureSource(context.Background(), serviceRunner(mixed, ps), declared, serviceLookup); err == nil || !strings.Contains(err.Error(), "repeats service api") {
		t.Fatalf("replicas on different images accepted: %v", err)
	}
	failing := func(_ context.Context, command string) ([]byte, error) {
		if strings.Contains(command, " ps ") {
			return nil, fmt.Errorf("injected ps failure")
		}
		return serviceRunner(composeImagesV56, "")(context.Background(), command)
	}
	if _, err := CaptureSource(context.Background(), failing, declared, serviceLookup); err == nil || !strings.Contains(err.Error(), "list source compose containers") {
		t.Fatalf("ps failure not surfaced: %v", err)
	}
}
