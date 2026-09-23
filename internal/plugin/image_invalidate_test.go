package plugin

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// stubDocker records docker invocations and answers `image ls` per service.
func stubDocker(t *testing.T, images map[string]string) *[][]string {
	t.Helper()
	var calls [][]string
	orig := dockerRun
	dockerRun = func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args)
		if len(args) > 1 && args[0] == "image" && args[1] == "ls" {
			for _, a := range args {
				if svc, ok := strings.CutPrefix(a, "label=com.docker.compose.service="); ok {
					return []byte(images[svc]), nil
				}
			}
		}
		return nil, nil
	}
	t.Cleanup(func() { dockerRun = orig })
	return &calls
}

func TestInvalidatePluginImagesUntagsOnlyBuiltServices(t *testing.T) {
	t.Setenv("COMPOSE_PROJECT_NAME", "")
	dir := t.TempDir()
	fragment := `services:
  plugin-cron:
    build:
      context: ${NSELF_PLUGIN_DIR}/cron
  cron-sidecar:
    image: busybox:1.36
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.plugin.yml"), []byte(fragment), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := stubDocker(t, map[string]string{
		"plugin-cron":  "nself-web-plugin-cron:latest\n<none>:<none>\n",
		"cron-sidecar": "busybox:1.36\n",
	})

	removed, err := invalidatePluginImages(context.Background(), &config.Config{ProjectName: "nself-web"}, dir)
	if err != nil {
		t.Fatalf("invalidatePluginImages: %v", err)
	}
	if want := []string{"nself-web-plugin-cron:latest"}; !reflect.DeepEqual(removed, want) {
		t.Errorf("removed = %v, want %v", removed, want)
	}
	for _, c := range *calls {
		joined := strings.Join(c, " ")
		if strings.Contains(joined, "cron-sidecar") || strings.Contains(joined, "busybox") {
			t.Errorf("touched a pulled (non-built) service image: %q", joined)
		}
		if c[1] == "ls" && !strings.Contains(joined, "label=com.docker.compose.project=nself-web") {
			t.Errorf("image ls not scoped to the project: %q", joined)
		}
	}
}

func TestInvalidatePluginImagesWithoutFragmentIsNoop(t *testing.T) {
	calls := stubDocker(t, nil)
	removed, err := invalidatePluginImages(context.Background(), &config.Config{ProjectName: "p"}, t.TempDir())
	if err != nil || len(removed) != 0 || len(*calls) != 0 {
		t.Fatalf("got removed=%v err=%v calls=%v, want no docker calls", removed, err, *calls)
	}
}
