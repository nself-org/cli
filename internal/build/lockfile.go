package build

// lockfile.go — the project lock nself.lock.yaml in the build pipeline
// (contract:cli.lockfile v1, P7-LIVE-09, EPIC D10).
//
// Purpose: build honours the lock's image pins while it renders docker-compose.yml
// and writes the lock (through the Sink, artifact kind "lock") when its content
// changes, so `build --plan` lists it and plan == apply covers it.
// Inputs: the project dir, the rendered compose bytes, the bundle ledger
// (plugin versions, checksums, tiers) and the CLI version (major.minor only).
// Outputs: compose.SetProjectPins for the run; nself.lock.yaml when it changed;
// CurrentLock, the state `start --frozen` compares with the file.
// Constraints: active only when nself.lock.yaml exists, `build --write-lock`
// was given (BuildOptions.WriteLock) or v1.5 is selected. The first lock of a
// project with a docker-compose.yml takes its pins from that file. The lock is
// rewritten only when its bytes change. It records the compose as rendered: no digest lookup
// after rendering, so the lock and the compose never disagree.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/version"
)

// localRepoDigest returns "sha256:<hex>" when the image for a tag reference is
// present locally with exactly one repo digest for its repository, through the
// docker funnel (docker.LocalRepoDigest). It is a variable so tests can fake it.
// "" means none: the caller falls back to the CLI image lock, then to a tag-only
// entry.
var localRepoDigest = func(ref string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return docker.LocalRepoDigest(ctx, ref)
}

var minorRE = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)`)

// cliMinor returns major.minor of a version string, or "" when it has none.
func cliMinor(v string) string {
	if m := minorRE.FindStringSubmatch(strings.TrimSpace(v)); m != nil {
		return m[1] + "." + m[2]
	}
	return ""
}

// lockActive reports whether this build reads and writes the project lock.
func (st *buildState) lockActive() bool {
	if st.opts.WriteLock {
		return true
	}
	// compat.V15(P7-LIVE-09): the project lock is written and honoured only when nself.lock.yaml exists or --write-lock is given -> always
	if compat.V15() {
		return true
	}
	_, err := st.sink.Stat(lock.Path(st.workdir))
	return err == nil
}

// installLockPins installs the image pins for this render and returns the
// function that restores the previous ones.
func (st *buildState) installLockPins() (func(), error) {
	if !st.lockActive() {
		return func() {}, nil
	}
	data, err := st.sink.ReadFile(lock.Path(st.workdir))
	if err == nil {
		l, perr := lock.Parse(data)
		if perr != nil {
			return nil, fmt.Errorf("reading %s: %w", lock.FileName, perr)
		}
		return compose.SetProjectPins(l.Pins()), nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading %s: %w", lock.FileName, err)
	}
	pins, err := st.bootstrapPins()
	if err != nil {
		return nil, err
	}
	return compose.SetProjectPins(pins), nil
}

// bootstrapPins takes the first lock's pins from the project's existing
// docker-compose.yml, for the services the CLI owns and that file has. A
// service the old compose lacks takes the CLI default.
func (st *buildState) bootstrapPins() (map[string]string, error) {
	data, err := st.sink.ReadFile(filepath.Join(st.workdir, "docker-compose.yml"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading docker-compose.yml: %w", err)
	}
	refs, err := cliOwnedImages(data)
	if err != nil {
		return nil, err
	}
	images, unpinned := lock.Pin(refs, lock.Resolver{LocalDigest: localRepoDigest, CLILocked: cliLockedRef})
	for k, v := range unpinned {
		images[k] = v
	}
	return images, nil
}

// cliOwnedImages lists the images of compose services the CLI image lock names.
func cliOwnedImages(composeYAML []byte) (map[string]string, error) {
	all, err := lock.ComposeImages(composeYAML)
	if err != nil {
		return nil, err
	}
	return filterCLIOwned(all), nil
}

// mergedComposeImages reads the image of every service across the compose files
// `nself start` runs (ReadComposeManifest: the generated base, plugin fragments,
// docker-compose.override.yml). Docker merges them in order and the last image
// wins, so an override file cannot move a pinned image unseen. base is the rendered docker-compose.yml bytes, used in
// place of the file on disk when the build has them (nil reads the file).
func mergedComposeImages(workdir string, base []byte) (map[string]string, error) {
	paths, err := ReadComposeManifest(workdir)
	if err != nil {
		return nil, err
	}
	basePath := filepath.Join(workdir, "docker-compose.yml")
	abs := func(p string) string {
		if !filepath.IsAbs(p) {
			p = filepath.Join(workdir, p)
		}
		return filepath.Clean(p)
	}
	files := []string{basePath}
	for _, p := range paths {
		// image-overrides.yml is written by start from the digests this very
		// lock pins, so it mirrors the lock and is left out of the comparison.
		if abs(p) != basePath && filepath.Base(p) != docker.ImageOverrideFile {
			files = append(files, abs(p))
		}
	}
	merged := map[string]string{}
	for _, f := range files {
		var data []byte
		if f == basePath && base != nil {
			data = base
		} else if data, err = os.ReadFile(f); err != nil {
			return nil, fmt.Errorf("reading %s: %w", filepath.Base(f), err)
		}
		imgs, err := lock.ComposeImages(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
		for svc, ref := range imgs {
			merged[svc] = ref
		}
	}
	return merged, nil
}

// filterCLIOwned keeps the services the CLI image lock names and whose image
// is a literal reference.
func filterCLIOwned(all map[string]string) map[string]string {
	out := map[string]string{}
	for svc, ref := range all {
		if _, owned := compose.LockedRef(svc); owned && !strings.Contains(ref, "${") {
			out[svc] = ref
		}
	}
	return out
}

// cliLockedRef finds the CLI image lock entry whose version a tag reference names.
func cliLockedRef(ref string) (string, bool) {
	for _, r := range compose.LockedImages() {
		if ref == r.LegacyRef || ref == r.LegacyRepository()+":"+r.Version || ref == r.Repository+":"+r.Version {
			return r.String(), true
		}
	}
	return "", false
}

// ledgerSources adapts internal/bundle and the plugin directory to the ledger's
// bootstrap input (the ledger imports neither).
func ledgerSources(pluginDir string) func() (ledger.Sources, error) {
	return func() (ledger.Sources, error) {
		var src ledger.Sources
		for _, b := range bundle.All() {
			src.Bundles = append(src.Bundles, ledger.BundleDef{Slug: b.Slug, Installable: b.IsInstallable(), Plugins: b.Plugins})
		}
		manifests, err := plugin.LoadManifestsFromDir(pluginDir)
		if err != nil {
			return ledger.Sources{}, err
		}
		for _, m := range manifests {
			tier := ledger.TierFree
			if m.Tier == "pro" || m.Tier == "paid" || m.Tier == ledger.TierLicensed || m.RequiresLicense || m.LicenseType == "pro" {
				tier = ledger.TierLicensed
			}
			src.Plugins = append(src.Plugins, ledger.InstalledPlugin{Name: m.Name, Version: m.Version, Tier: tier, Checksum: m.Checksum})
		}
		return src, nil
	}
}

// lockFromState assembles a Lock from a rendered compose, the bundle ledger and
// the CLI version. No digest is looked up: the lock records the compose as is.
func lockFromState(pluginDir, workdir string, composeYAML []byte) (lock.Lock, error) {
	all, err := mergedComposeImages(workdir, composeYAML)
	if err != nil {
		return lock.Lock{}, err
	}
	refs := filterCLIOwned(all)
	l := lock.New()
	l.CLI = cliMinor(version.GetVersion())
	l.Images, l.Unpinned = lock.Pin(refs, lock.Resolver{})
	store := ledger.NewStore(pluginDir)
	store.Sources = ledgerSources(pluginDir)
	led, _, err := store.Load()
	if err != nil {
		return lock.Lock{}, fmt.Errorf("reading the bundle ledger for %s: %w", lock.FileName, err)
	}
	for slug, p := range led.Plugins {
		l.Plugins[slug] = lock.PluginPin{Version: p.Version, SHA256: p.Checksum, Tier: p.Tier}
	}
	for slug := range led.Bundles {
		l.Bundles = append(l.Bundles, slug)
	}
	return l, nil
}

// writeProjectLock writes nself.lock.yaml through the sink when its content
// changed. A byte-identical lock is not touched (not even its mtime).
func (st *buildState) writeProjectLock(composeYAML []byte) error {
	if !st.lockActive() {
		return nil
	}
	l, err := lockFromState(st.pluginDir, st.workdir, composeYAML)
	if err != nil {
		return err
	}
	want, err := l.Marshal()
	if err != nil {
		return fmt.Errorf("rendering %s: %w", lock.FileName, err)
	}
	path := lock.Path(st.workdir)
	if have, rerr := st.sink.ReadFile(path); rerr == nil && bytes.Equal(have, want) {
		return nil
	}
	// Atomic (temp file + rename): a reader, a crash or a full disk never leaves
	// a half-written lock whose missing pins would silently move images.
	if err := st.sink.WriteAtomic(path, want, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", lock.FileName, err)
	}
	st.filesGenerated++
	return nil
}

// CurrentLock computes the lock the project's docker-compose.yml and installed
// plugins describe right now, for `start --frozen` to compare with the file.
func CurrentLock(workdir string) (lock.Lock, error) {
	if _, err := os.Stat(filepath.Join(workdir, "docker-compose.yml")); err != nil {
		return lock.Lock{}, fmt.Errorf("reading docker-compose.yml: %w", err)
	}
	return lockFromState(DefaultPluginDir(), workdir, nil)
}
