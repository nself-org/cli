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
	name := in.Set(os.Getenv)
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

// repoOf is CanonicalRef without the tag.
func repoOf(ref string) string {
	ref = CanonicalRef(ref)
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	return ref
}

// noticePinDropped prints the drop notice once per service, variable and pin.
func noticePinDropped(service, envName, pin string) {
	key := service + "/" + envName + "/" + pin
	pinNoticedMu.Lock()
	seen := pinNoticed[key]
	pinNoticed[key] = true
	w := pinNoticeW
	pinNoticedMu.Unlock()
	if seen || w == nil {
		return
	}
	if envName == "the rendered image" {
		_, _ = fmt.Fprintf(w, "notice: the %s image is now a different repository than its pin, so the pin in %s is not used\n", service, ProjectLockFile)
		return
	}
	_, _ = fmt.Fprintf(w, "notice: %s is set, so the %s pin in %s is not used\n", envName, service, ProjectLockFile)
}

func imageBase(ref string) string {
	base, _, _ := strings.Cut(ref, "@")
	return base
}

// CanonicalRef reduces an image reference to one spelling for comparison: no
// digest, no docker.io/, index.docker.io/ or library/ prefix, host and path in
// lower case, the tag as written (tags are case-sensitive). The bootstrap pins
// a reference as docker.io/<repo>:<tag>@sha256 while the legacy image strings
// omit the host, so every comparison of two spellings goes through here
// (internal/docker cannot be imported: its tests import this package).
func CanonicalRef(ref string) string {
	ref = strings.TrimSpace(imageBase(ref))
	tag := ""
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref, tag = ref[:i], ref[i:]
	}
	ref = strings.ToLower(ref)
	for _, host := range []string{"registry-1.docker.io/", "index.docker.io/", "docker.io/"} {
		ref = strings.TrimPrefix(ref, host)
	}
	return strings.TrimPrefix(ref, "library/") + tag
}

// CLILockedRef finds the CLI image lock entry whose version a tag reference
// names, in the short or the docker.io/ spelling.
func CLILockedRef(ref string) (string, bool) {
	c := CanonicalRef(ref)
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

// CLIOwned keeps the services the CLI image lock names and whose image is a
// literal reference (no variable). The result is keyed by pin key (PinKey), the
// key the project lock and ResolveImage use, not by compose service name.
func CLIOwned(all map[string]string) map[string]string {
	out := map[string]string{}
	for svc, ref := range all {
		if IsCLIOwned(svc) && !strings.Contains(ref, "$") {
			out[PinKey(svc)] = ref
		}
	}
	return out
}
