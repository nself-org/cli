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
	"bytes"
	"errors"
	"fmt"
	"io"
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
					// Prefer the CLI spelling when it identifies the same bytes.
					if r.CLILocked != nil {
						if locked, ok := r.CLILocked(ref); ok && pinnedRE.MatchString(locked) && RefDigest(locked) == d {
							images[svc] = locked
						}
					}
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

// ServiceSource is what a compose service says about where its image comes
// from: the image key, and whether it also carries extends or build, either of
// which can change the image without an image key.
type ServiceSource struct {
	Image   string
	Extends bool
	Build   bool
	// Reset is set when the service or its image carries Compose's !reset tag:
	// the key is cleared, so an earlier file's image no longer applies. A
	// service-level !override sets Override: it replaces the earlier service
	// whole, so the earlier image does not carry over either.
	Reset    bool
	Override bool
}

// ComposeSources reads every service of the first document of a compose file,
// whether the file has a top-level include (which pulls in services this reader
// cannot see), and whether it has a second YAML document (Compose merges every
// document, and this reader sees only the first, so such a file is opaque).
func ComposeSources(data []byte) (services map[string]ServiceSource, include, multiDoc bool, err error) {
	var doc struct {
		Include  yaml.Node            `yaml:"include"`
		Services map[string]yaml.Node `yaml:"services"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, false, false, fmt.Errorf("lock: parse compose: %w", err)
	}
	var extra yaml.Node
	multiDoc = !errors.Is(dec.Decode(&extra), io.EOF)
	services = map[string]ServiceSource{}
	for name, node := range doc.Services {
		var s struct {
			Image   yaml.Node `yaml:"image"`
			Extends yaml.Node `yaml:"extends"`
			Build   yaml.Node `yaml:"build"`
		}
		if node.Tag != "!reset" {
			if err := node.Decode(&s); err != nil {
				return nil, false, false, fmt.Errorf("lock: parse compose service %s: %w", name, err)
			}
		}
		// An anchored image (image: *img) is resolved to its target.
		for s.Image.Kind == yaml.AliasNode && s.Image.Alias != nil {
			s.Image = *s.Image.Alias
		}
		src := ServiceSource{Extends: s.Extends.Kind != 0, Build: s.Build.Kind != 0,
			Reset: node.Tag == "!reset" || s.Image.Tag == "!reset", Override: node.Tag == "!override"}
		if !src.Reset && s.Image.Kind == yaml.ScalarNode {
			src.Image = s.Image.Value
		}
		services[name] = src
	}
	return services, doc.Include.Kind != 0, multiDoc, nil
}

// ComposeImages reads the image of every service of a compose document. A
// service without an image (build only) is left out.
func ComposeImages(data []byte) (map[string]string, error) {
	svcs, _, _, err := ComposeSources(data)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for name, s := range svcs {
		if s.Image != "" {
			out[name] = s.Image
		}
	}
	return out, nil
}

// RefDigest returns the "sha256:<hex>" of a reference of the form repo[:tag]@digest,
// or "" when it has none or the digest is malformed.
func RefDigest(ref string) string {
	if i := strings.LastIndex(ref, "@"); i >= 0 && digestRE.MatchString(ref[i+1:]) {
		return ref[i+1:]
	}
	return ""
}

var minorRE = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)`)

// CLIMinor returns major.minor of a version string, or "" when it has none.
func CLIMinor(v string) string {
	if m := minorRE.FindStringSubmatch(strings.TrimSpace(v)); m != nil {
		return m[1] + "." + m[2]
	}
	return ""
}
