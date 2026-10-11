package compose

// project_pins.go — the image pins of a project lock (nself.lock.yaml,
// contract:cli.lockfile v1, P7-LIVE-09, EPIC D10) as ResolveImage sees them.
//
// Purpose: a CLI upgrade must never silently move an existing project's images.
// ResolveImage consults the pins after an explicit override and before the CLI
// image lock (precedence: explicit *_IMAGE/*_VERSION in the env > lock pin >
// CLI image lock).
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
	pinsMu       sync.RWMutex
	projectPins  = map[string]string{}
	explicitVars = map[string][]string{
		"admin":    {"NSELF_ADMIN_VERSION", "ADMIN_VERSION", "ADMIN_IMAGE"},
		"postgres": {"POSTGRES_VERSION", "POSTGRES_IMAGE", "POSTGRES_EXTENSIONS"},
		"storage":  {"STORAGE_VERSION", "STORAGE_IMAGE"},
	}
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

// projectPin returns the pin for service, given the reference the caller
// rendered from the config (image; "" means the CLI default). Without an
// explicit *_IMAGE/*_VERSION in the env the pin always wins. With one, the pin
// is kept only when that value renders to the pin's own reference: nself init
// writes *_VERSION defaults into .env, and for postgres 16-alpine renders to the
// pgvector entry, so the comparison is on the rendered reference (what the
// generator would emit), never on the raw env text. A value that renders to a
// different image drops the pin, with a one-line notice on stderr.
func projectPin(service, image string) (string, bool) {
	pinsMu.RLock()
	pin, ok := projectPins[service]
	pinsMu.RUnlock()
	if !ok {
		return "", false
	}
	name := explicitEnvName(service)
	if name == "" {
		return pin, true
	}
	if image == "" {
		image = DefaultImage(service)
	}
	want := CanonicalRef(pin)
	if CanonicalRef(image) == want {
		return pin, true
	}
	// nself init writes POSTGRES_VERSION=16-alpine; a project that runs the
	// pgvector image gets it from the extension list, which may be set elsewhere.
	if v := strings.TrimSpace(os.Getenv("POSTGRES_VERSION")); service == "postgres" && v != "" && CanonicalRef(pgvectorImageForVersion(v)) == want {
		return pin, true
	}
	noticePinDropped(service, name, pin)
	return "", false
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
	_, _ = fmt.Fprintf(w, "notice: %s is set, so the %s pin in %s is not used\n", envName, service, ProjectLockFile)
}

// explicitEnvName returns the first env variable of service that is set, or "".
func explicitEnvName(service string) string {
	names, ok := explicitVars[service]
	if !ok {
		base := strings.ToUpper(strings.ReplaceAll(service, "-", "_"))
		names = []string{base + "_VERSION", base + "_IMAGE"}
	}
	for _, n := range names {
		if strings.TrimSpace(os.Getenv(n)) != "" {
			return n
		}
	}
	return ""
}

func imageBase(ref string) string {
	base, _, _ := strings.Cut(ref, "@")
	return base
}

// CanonicalRef reduces an image reference to one spelling for comparison: no
// digest, no docker.io/, index.docker.io/ or library/ prefix, lower case. The
// bootstrap pins a reference as docker.io/<repo>:<tag>@sha256 while the legacy
// image strings omit the host, so every comparison of two spellings goes through
// here (internal/docker cannot be imported: its tests import this package).
func CanonicalRef(ref string) string {
	ref = strings.ToLower(strings.TrimSpace(imageBase(ref)))
	for _, host := range []string{"registry-1.docker.io/", "index.docker.io/", "docker.io/"} {
		ref = strings.TrimPrefix(ref, host)
	}
	return strings.TrimPrefix(ref, "library/")
}
