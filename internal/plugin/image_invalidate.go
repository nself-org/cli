package plugin

// image_invalidate.go — makes `nself plugin update` take effect on the next
// start by removing the images compose built from the previous plugin source.
//
// Purpose: plugin fragments build their image locally (build.context =
//          ${NSELF_PLUGIN_DIR}/<name>). `docker compose up` reuses an image
//          that already carries the tag, so after an update the container
//          kept running the old code: on nself-web prod (2026-09-23) mux
//          was updated to bundles v1.2.9 and still crash-looped on the gate
//          that release removed, until its image was untagged by hand.
// Inputs:  the project config and the freshly installed plugin directory.
// Outputs: the image references removed; error only when docker itself
//          cannot be queried (the caller reports it as a warning).
// Constraints: touches only images compose labelled with this project and
//              one of this plugin's build services; `image rm -f` on a tag
//              in use only untags it, so running containers keep serving.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/config"
)

// dockerRun runs a docker CLI command; a variable so tests can stub it.
var dockerRun = func(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "docker", args...).CombinedOutput()
}

// pluginBuildServices returns the services in a plugin compose fragment that
// build their image locally, sorted. A missing fragment yields none.
func pluginBuildServices(pluginDir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(pluginDir, "docker-compose.plugin.yml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Services map[string]struct {
			Build any `yaml:"build"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing plugin compose fragment: %w", err)
	}
	var out []string
	for name, svc := range doc.Services {
		if svc.Build != nil {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// composeProjectName is the project label compose puts on the images it
// builds: COMPOSE_PROJECT_NAME when set, otherwise the nSelf project name.
func composeProjectName(cfg *config.Config) string {
	if p := strings.TrimSpace(os.Getenv("COMPOSE_PROJECT_NAME")); p != "" {
		return p
	}
	return cfg.ProjectName
}

// invalidatePluginImages removes the tags of every image compose built for
// the plugin's build services, so the next `nself start` rebuilds them from
// the updated source.
func invalidatePluginImages(ctx context.Context, cfg *config.Config, pluginDir string) ([]string, error) {
	services, err := pluginBuildServices(pluginDir)
	if err != nil || len(services) == 0 {
		return nil, err
	}
	project := composeProjectName(cfg)
	var removed []string
	for _, svc := range services {
		out, err := dockerRun(ctx, "image", "ls", "--format", "{{.Repository}}:{{.Tag}}",
			"--filter", "label=com.docker.compose.project="+project,
			"--filter", "label=com.docker.compose.service="+svc)
		if err != nil {
			return removed, fmt.Errorf("listing images for %s: %s: %w", svc, strings.TrimSpace(string(out)), err)
		}
		for _, ref := range strings.Fields(string(out)) {
			if strings.Contains(ref, "<none>") {
				continue
			}
			if out, err := dockerRun(ctx, "image", "rm", "-f", ref); err != nil {
				return removed, fmt.Errorf("removing image %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
			}
			removed = append(removed, ref)
		}
	}
	return removed, nil
}
