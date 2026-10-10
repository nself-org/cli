package plugin

// update_images.go — pulls the images a plugin update points at.
//
// Purpose: `nself plugin update` replaced the plugin directory but never
//          fetched an image whose reference the new fragment changed, so the
//          next `nself start` either pulled it late (slow, mid-start) or ran
//          the old tag. The update now pulls every image reference the new
//          docker-compose.plugin.yml has that the old one did not.
// Inputs:  the plugin directory before and after the update.
// Outputs: the references pulled; an error naming the first one that failed
//          (the caller reports it as a warning, the update itself is done).
// Constraints: services that build locally are not touched (image_invalidate.go
//              handles them); a reference with a ${VAR} cannot be resolved
//              here and is left for `nself start`; docker runs only through
//              the internal/docker funnel.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/docker"
)

// pullImage pulls one image reference; a variable so tests can stub it.
var pullImage = func(ctx context.Context, ref string) error {
	return (&docker.Compose{}).Run(ctx, "", "pull", ref)
}

// fragmentImages returns service -> image reference for every service in the
// plugin's compose fragment that names an image and does not build one. A
// missing fragment yields none.
func fragmentImages(pluginDir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(pluginDir, "docker-compose.plugin.yml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
			Build any    `yaml:"build"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing plugin compose fragment: %w", err)
	}
	out := map[string]string{}
	for svc, s := range doc.Services {
		if ref := strings.TrimSpace(s.Image); ref != "" && s.Build == nil {
			out[svc] = ref
		}
	}
	return out, nil
}

// changedImages returns the sorted, de-duplicated references in next that
// differ from the reference the same service had in prev (a service new to
// the fragment counts as changed). References with a ${VAR} are skipped.
func changedImages(prev, next map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for svc, ref := range next {
		if prev[svc] == ref || strings.Contains(ref, "${") || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

// pullChangedImages pulls every image the fragment in pluginDir references
// that prev (the fragment's images before the update) did not.
func pullChangedImages(ctx context.Context, prev map[string]string, pluginDir string) ([]string, error) {
	next, err := fragmentImages(pluginDir)
	if err != nil {
		return nil, err
	}
	var pulled []string
	for _, ref := range changedImages(prev, next) {
		if err := pullImage(ctx, ref); err != nil {
			return pulled, fmt.Errorf("pulling %s: %w", ref, err)
		}
		pulled = append(pulled, ref)
	}
	return pulled, nil
}
