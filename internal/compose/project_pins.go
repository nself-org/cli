package compose

// project_pins.go — the image pins of a project lock (nself.lock.yaml,
// contract:cli.lockfile v1, P7-LIVE-09, EPIC D10) as ResolveImage sees them.
//
// Purpose: a CLI upgrade must never silently move an existing project's images.
// ResolveImage consults the pins after an explicit override and before the CLI
// image lock (precedence: an env variable the generator reads for that image >
// lock pin > CLI image lock). Which variables the generator reads is decided by
// one table, image_inputs.go; any other variable is not an override.
// Inputs: SetProjectPins (the build computes the pins, also for a project's
// first lock, from its existing docker-compose.yml) or LoadProjectPins (the
// lock file of a project directory).
// Outputs: ProjectPins (a copy) and the pin lookup ResolveImage uses.
// Constraints: process-wide state like ImageDigests, guarded by a mutex; the
// build installs the pins for one run and restores the previous ones.
// internal/compose cannot import internal/lock (both are L1, no declared edge),
// so LoadProjectPins reads only the two image maps of the file and the build
// validates the whole file through lock.Load first.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ProjectLockFile is the lock's file name at the project root (lock.FileName).
const ProjectLockFile = "nself.lock.yaml"

var (
	pinsMu      sync.RWMutex
	projectPins = map[string]string{}
)

// SetProjectPins installs pins (service -> reference) and returns a function
// that restores the previous set. A nil or empty map clears the pins.
func SetProjectPins(pins map[string]string) (restore func()) {
	next := make(map[string]string, len(pins))
	for k, v := range pins {
		if v != "" {
			next[k] = v
		}
	}
	pinsMu.Lock()
	prev := projectPins
	projectPins = next
	pinsMu.Unlock()
	return func() {
		pinsMu.Lock()
		projectPins = prev
		pinsMu.Unlock()
	}
}

// ProjectPins returns a copy of the installed pins.
func ProjectPins() map[string]string {
	pinsMu.RLock()
	defer pinsMu.RUnlock()
	out := make(map[string]string, len(projectPins))
	for k, v := range projectPins {
		out[k] = v
	}
	return out
}

// LoadProjectPins installs the pins of the lock file in projectDir (digest-pinned
// images and tag-only unpinned entries). A project without a lock has no pins:
// that is not an error, and any earlier pins are cleared.
func LoadProjectPins(projectDir string) error {
	data, err := os.ReadFile(filepath.Join(projectDir, ProjectLockFile))
	if errors.Is(err, os.ErrNotExist) {
		SetProjectPins(nil)
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", ProjectLockFile, err)
	}
	var doc struct {
		Images   map[string]string `yaml:"images"`
		Unpinned map[string]string `yaml:"unpinned"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing %s: %w", ProjectLockFile, err)
	}
	pins := map[string]string{}
	for k, v := range doc.Unpinned {
		pins[k] = v
	}
	for k, v := range doc.Images {
		pins[k] = v
	}
	SetProjectPins(pins)
	return nil
}

// pinNoticeW receives the one-line notice printed when an explicit env value
// drops a pin. Tests replace it; the build leaves it on stderr. A notice prints
// once per service, variable and pin for the whole process (a build installs
// the pins several times), and only when the pin really is not used.
var (
	pinNoticeW   io.Writer = os.Stderr
	pinNoticed             = map[string]bool{}
	pinNoticedMu sync.Mutex
)

// PinNoticeSink routes the drop notices to w (nil silences them), forgets which
// notices were already printed, and returns the function that restores the
// previous writer. Tests use it to count notices.
func PinNoticeSink(w io.Writer) (restore func()) {
	pinNoticedMu.Lock()
	prev := pinNoticeW
	pinNoticeW = w
	pinNoticed = map[string]bool{}
	pinNoticedMu.Unlock()
	return func() {
		pinNoticedMu.Lock()
		pinNoticeW = prev
		pinNoticedMu.Unlock()
	}
}

// projectPin returns the pin for service, given the reference the caller
// rendered from the config (image; "" means the CLI default). The pin applies
// only when its repository (tag and digest stripped, docker.io/library
// canonicalised) is the rendered image's repository: a runtime switch
// (deno to node) or pgvector turned off renders another image, and the pin
// must not hide that. With a variable the generator reads for this image set
// in the env (the table in image_inputs.go; no other variable counts) the pin is
// also kept only when the rendered reference equals the pin's reference: nself
// init writes *_VERSION defaults into .env, and for postgres 16-alpine renders
// to the pgvector entry only when the extension list asks for it, so the
// comparison is on the rendered reference, never on the raw env text. Dropping
// a pin prints a one-line notice on stderr.
func projectPin(service, image string) (string, bool) {
	return projectPinWithVersion(service, image, "")
}

// projectPinWithVersion is like projectPin but treats a nonblank version as an
// explicit override of the env variable for this service, without mutating the
// process environment. Hasura uses this so an explicit HASURA_VERSION (including
// v2.44.0 written by `nself service upgrade`) overrides a project pin; other
// services keep reading the env directly.
func projectPinWithVersion(service, image, version string) (string, bool) {
	pinsMu.RLock()
	pin, ok := projectPins[service]
	pinsMu.RUnlock()
	if !ok {
		return "", false
	}
	if image == "" {
		image = DefaultImage(service)
	}
	in := inputFor(service, image)
	var name string
	if version != "" {
		// Treat the caller-supplied version as if the env var were set to it.
		// A non-default value is explicit and drops the pin with a notice; a
		// default value keeps the pin silently.
		if !in.isDefault(version) {
			for _, v := range in.Vars {
				name = v
				break
			}
		}
	} else {
		name = in.Set(os.Getenv)
	}
	switch {
	case repoOf(image) != repoOf(pin):
		if name == "" {
			name = "the rendered image"
		}
	case name == "" || CanonicalRef(image) == CanonicalRef(pin):
		// Canonical references carry no digest. An *_IMAGE value that names its
		// own digest must name the pin's, or the user's digest wins. Every
		// *_IMAGE the generator reads is checked, not only the first variable
		// set: nself init writes POSTGRES_VERSION next to POSTGRES_IMAGE.
		over := envDigestDiffers(in, pin)
		if over == "" {
			return pin, true
		}
		name = over
	}
	noticePinDropped(service, name, pin)
	return "", false
}

// envDigestDiffers returns the name of the first *_IMAGE variable of the row
// (the ones the generator reads) that carries a digest which is not the pin's,
// or "" when there is none.
func envDigestDiffers(in ImageInput, pin string) string {
	_, pd, _ := strings.Cut(pin, "@")
	for _, name := range in.ImageVars() {
		_, d, ok := strings.Cut(strings.TrimSpace(os.Getenv(name)), "@")
		if ok && d != pd {
			return name
		}
	}
	return ""
}

// CLILockedRef finds the CLI image lock entry whose version a tag reference
// names, in the short or the docker.io/ spelling. The pre-rename
// nhost/hasura-auth spelling names nhost/auth (same digests, P7-LIVE-20).
func CLILockedRef(ref string) (string, bool) {
	c := CanonicalRef(ref)
	if rest, ok := strings.CutPrefix(c, "nhost/hasura-auth:"); ok {
		c = "nhost/auth:" + rest
	}
	for _, r := range LockedImages() {
		for _, cand := range []string{r.LegacyRef, r.LegacyRepository() + ":" + r.Version, r.Repository + ":" + r.Version} {
			if c == CanonicalRef(cand) {
				return r.String(), true
			}
		}
	}
	return "", false
}

// IsCLIOwned reports whether a compose service is one the CLI image lock names,
// looked up under its pin key (nself-admin is pinned as "admin").
func IsCLIOwned(service string) bool {
	_, owned := LockedRef(PinKey(service))
	return owned
}

// catalogPinPriority ranks the compose service that owns a pin key above a
// non-catalog service with the same name. An exact catalog name wins over an
// alias; otherwise ties use the service name so selection stays deterministic.
func catalogPinPriority(service, key string) int {
	for _, e := range serviceCatalog {
		if e.Name != service {
			continue
		}
		if service == key {
			return 2
		}
		return 1
	}
	return 0
}

// CLIOwned keeps the services the CLI image lock names and whose image is a
// literal reference (no variable). The result is keyed by pin key (PinKey), the
// key the project lock and ResolveImage use, not by compose service name. When
// names collapse onto one pin key, the catalog service wins deterministically.
func CLIOwned(all map[string]string) map[string]string {
	out := map[string]string{}
	owners := map[string]string{}
	for svc, ref := range all {
		if IsCLIOwned(svc) && !strings.Contains(ref, "$") {
			key := PinKey(svc)
			owner, exists := owners[key]
			priority, ownerPriority := catalogPinPriority(svc, key), catalogPinPriority(owner, key)
			if exists && (priority < ownerPriority || priority == ownerPriority && svc >= owner) {
				continue
			}
			out[key] = ref
			owners[key] = svc
		}
	}
	return out
}
