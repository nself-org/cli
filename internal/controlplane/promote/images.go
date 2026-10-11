package promote

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// ReadComposeImages reads and merges service image declarations from compose files.
//
// Purpose: give capture and compare one deterministic view of compose fragments.
// Inputs: compose files in merge order; later service declarations override earlier ones.
// Outputs: service/image/build rows sorted by service name.
// Constraints: files are read only; an empty image is retained for build-context checks.
func ReadComposeImages(files []string) ([]Image, error) {
	merged := map[string]Image{}
	for _, file := range files {
		raw, err := os.ReadFile(file) //nolint:gosec // caller-selected project compose file
		if err != nil {
			return nil, fmt.Errorf("reading promotion compose %s: %w", file, err)
		}
		var doc struct {
			Services map[string]struct {
				Image string    `yaml:"image"`
				Build yaml.Node `yaml:"build"`
			} `yaml:"services"`
		}
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("parsing promotion compose %s: %w", file, err)
		}
		for name, service := range doc.Services {
			if name == "" {
				return nil, fmt.Errorf("promotion compose %s has an empty service name", file)
			}
			image := merged[name]
			image.Service = name
			if service.Image != "" {
				image.Ref = service.Image
			}
			if service.Build.Kind != 0 {
				image.Build = true
			}
			merged[name] = image
		}
	}
	images := make([]Image, 0, len(merged))
	for _, image := range merged {
		images = append(images, image)
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Service < images[j].Service })
	return images, nil
}
