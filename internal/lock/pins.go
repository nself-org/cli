package lock

// pins.go — turn compose image references into lock entries.
//
// Purpose: the first lock of a project takes its pins from the existing
// docker-compose.yml (EPIC D10, review F10), and every later lock is computed
// from the compose the build just rendered. Both need the same rule: digest
// from the reference itself, else from a local image, else from the CLI image
// lock when the version matches, else a tag-only entry listed under unpinned.
// Inputs: a service -> reference map and a Resolver (both lookups injected, so
// this package stays pure and never touches Docker).
// Outputs: the images and unpinned maps of a Lock.
// Constraints: a digest that is not sha256:<64 hex> is ignored.

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var digestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Resolver supplies the two digest sources. A nil field finds nothing.
type Resolver struct {
	// LocalDigest returns "sha256:<hex>" for a tag reference whose image is
	// present locally with exactly one repo digest, else "".
	LocalDigest func(ref string) string
	// CLILocked returns the digest-pinned reference of the CLI image lock whose
	// version matches the tag reference, if any.
	CLILocked func(ref string) (string, bool)
}

// Pin sorts refs into digest-pinned images and tag-only unpinned entries.
func Pin(refs map[string]string, r Resolver) (images, unpinned map[string]string) {
	images, unpinned = map[string]string{}, map[string]string{}
	for svc, ref := range refs {
		ref = strings.TrimSpace(ref)
		switch {
		case ref == "":
		case pinnedRE.MatchString(ref):
			images[svc] = ref
		case strings.Contains(ref, "@"):
			// A digest of another algorithm, or a malformed one: keep the tag part.
			unpinned[svc] = ref[:strings.Index(ref, "@")]
		default:
			if r.LocalDigest != nil {
				if d := r.LocalDigest(ref); digestRE.MatchString(d) {
					images[svc] = ref + "@" + d
					continue
				}
			}
			if r.CLILocked != nil {
				if locked, ok := r.CLILocked(ref); ok && pinnedRE.MatchString(locked) {
					images[svc] = locked
					continue
				}
			}
			unpinned[svc] = ref
		}
	}
	return images, unpinned
}

// ComposeImages reads the image of every service of a compose document. A
// service without an image (build only) is left out.
func ComposeImages(data []byte) (map[string]string, error) {
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("lock: parse compose: %w", err)
	}
	out := map[string]string{}
	for name, s := range doc.Services {
		if s.Image != "" {
			out[name] = s.Image
		}
	}
	return out, nil
}
