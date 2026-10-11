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
	// A new set of pins is a new run: its drop notices print again.
	pinNoticedMu.Lock()
	pinNoticed = map[string]bool{}
	pinNoticedMu.Unlock()
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
// drops a pin. Tests replace it; the build leaves it on stderr.
var (
	pinNoticeW   io.Writer = os.Stderr
	pinNoticed             = map[string]bool{}
	pinNoticedMu sync.Mutex
)

// projectPin returns the pin for service. An explicit env value drops the pin
// only when it names a different image: nself init writes *_VERSION defaults
// into .env, and a value that equals the pin's reference (the part before
// "@") keeps ref@digest. A different explicit value drops the pin with a
// one-line notice on stderr.
func projectPin(service string) (string, bool) {
	pinsMu.RLock()
	pin, ok := projectPins[service]
	pinsMu.RUnlock()
	if !ok {
		return "", false
	}
	if name, differs := explicitDiffers(service, pin); differs {
		noticePinDropped(service, name, pin)
		return "", false
	}
	return pin, true
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

// explicitDiffers returns the first env variable of service that is set to
// something other than the pin's reference.
func explicitDiffers(service, pin string) (string, bool) {
	names, ok := explicitVars[service]
	if !ok {
		base := strings.ToUpper(strings.ReplaceAll(service, "-", "_"))
		names = []string{base + "_VERSION", base + "_IMAGE"}
	}
	want, _, _ := strings.Cut(pin, "@")
	for _, n := range names {
		v := strings.TrimSpace(os.Getenv(n))
		if v == "" {
			continue
		}
		switch {
		case strings.HasSuffix(n, "_VERSION") && refMatches(service, v, want):
			continue
		case strings.HasSuffix(n, "_IMAGE") && imageBase(v) == want:
			continue
		}
		return n, true
	}
	return "", false
}

func imageBase(ref string) string {
	base, _, _ := strings.Cut(ref, "@")
	return base
}

// refMatches reports whether version v of service resolves to the reference want.
func refMatches(service, v, want string) bool {
	if b := imageBase(ImageRef(service, v)); b != "" && b == want {
		return true
	}
	r, ok := LockedRef(service)
	return ok && r.LegacyRepository()+":"+v == want
}
