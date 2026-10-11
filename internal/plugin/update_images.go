package plugin

// update_images.go — pulls the images a plugin update points at.
//
// Purpose: `nself plugin update` replaced the plugin directory but never
//          fetched an image whose reference the new fragment changed, so the
//          next `nself start` either pulled it late (slow, mid-start) or ran
//          the old tag. The update now pulls every image reference the new
//          docker-compose.plugin.yml has that the old one did not.
// Inputs:  the plugin directory before and after the update.
// Outputs: the references pulled; an error joining every pull that failed
//          (the caller reports it as a warning, the update itself is done).
//          Every changed reference is tried, one failure does not hide the rest.
// Constraints: services that build locally are not touched (image_invalidate.go
//              handles them), and neither is an `nself/...` image line, written
//              the way the build's rule matches it, in a plugin that ships a
//              Dockerfile (nself build turns it into a local build, so the
//              ref is not on a registry); a reference with a ${VAR} or $VAR
//              cannot be resolved here and is left for `nself start`, and one
//              starting with '-' is never handed to docker;
//              docker runs only through the internal/docker funnel.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/docker"
)

// pullImage pulls one image reference; a variable so tests can stub it.
var pullImage = func(ctx context.Context, ref string) error {
	return (&docker.Compose{}).Run(ctx, "", "pull", ref)
}

// composeImageNselfRE is the build's rule for an `image: nself/...` line,
// copied from composeImageNselfRE in internal/build/plugins_image_to_build.go
// (internal/plugin must not import internal/build). It matches the raw line,
// so a quoted value or a trailing comment is not converted to a build there
// and must still be pulled here.
var composeImageNselfRE = regexp.MustCompile(`^(\s*)image:\s*nself/\S+\s*$`)

// builtImages returns the references of the fragment's `image: nself/...`
// lines that `nself build` turns into a local build: the line matches the
// build's rule and the plugin directory holds a Dockerfile (the build's
// canonicalDockerfile check; the service-has-build: half of the rule is
// already handled by fragmentImages, which drops services that declare it).
func builtImages(pluginDir string) map[string]bool {
	if _, err := os.Stat(filepath.Join(pluginDir, "Dockerfile")); err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(pluginDir, "docker-compose.plugin.yml"))
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if composeImageNselfRE.MatchString(line) {
			out[strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "image:"))] = true
		}
	}
	return out
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
// the fragment counts as changed). References with a variable (${VAR} or
// $VAR) are skipped, and so is one starting with '-': `docker pull` would
// read it as a flag, not an image.
func changedImages(prev, next map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for svc, ref := range next {
		if prev[svc] == ref || strings.Contains(ref, "$") || strings.HasPrefix(ref, "-") || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

// pullChangedImages pulls every image the fragment in pluginDir references
// that prev (the fragment's images before the update) did not. It tries every
// changed reference and returns those pulled plus the joined failures.
func pullChangedImages(ctx context.Context, prev map[string]string, pluginDir string) ([]string, error) {
	next, err := fragmentImages(pluginDir)
	if err != nil {
		return nil, err
	}
	// Refs the build turns into a local build are not on a registry.
	built := builtImages(pluginDir)
	for svc, ref := range next {
		if built[ref] {
			delete(next, svc)
		}
	}
	var pulled []string
	var errs []error
	for _, ref := range changedImages(prev, next) {
		if err := pullImage(ctx, ref); err != nil {
			errs = append(errs, fmt.Errorf("pulling %s: %w", ref, err))
			continue
		}
		pulled = append(pulled, ref)
	}
	return pulled, errors.Join(errs...)
}
