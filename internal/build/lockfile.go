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
	"log/slog"
	"os"
	"path/filepath"
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

// installImagePins installs the lock's image pins (or the bootstrap pins).
func (st *buildState) installImagePins() (func(), error) {
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

// projectPostgresVolumeExists reports whether the project's postgres data
// volume exists, through the docker funnel. It is a variable so tests can fake
// it. A docker error counts as "no": the notice it gates is advisory.
var projectPostgresVolumeExists = func(name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, err := docker.VolumeExists(ctx, name)
	return err == nil && ok
}

// noticeUnpinnedHasura warns, once per build, that a project with data but no
// pin source (no lock, no docker-compose.yml: nself clean keeps data and
// removes both) takes the CLI default hasura, which in v1.5 is a newer
// graphql-engine than the v1.4 default (D-0719). It names the move and the
// backup step. A project without a postgres volume is new and gets no notice.
func (st *buildState) noticeUnpinnedHasura() {
	if st.cfg == nil || compose.HasuraEntry() == "hasura-v14" {
		return
	}
	vol := strings.TrimSpace(st.cfg.Postgres.DataVolume)
	if vol == "" {
		vol = st.cfg.ProjectName + "_postgres_data"
	}
	if !projectPostgresVolumeExists(vol) {
		return
	}
	from, _ := compose.LockedRef("hasura-v14")
	to, _ := compose.LockedRef("hasura")
	st.warnOnce("no "+lock.FileName+" and no docker-compose.yml, but this project's database exists: hasura takes the v1.5 default graphql-engine "+to.Version+" (v1.4 ran "+from.Version+"); run `nself backup create` first (going back is restore-then-pin-back, see the Image-Lock wiki page, Roll back)", "volume", vol)
}

// bootstrapPins takes the first lock's pins from the project's existing
// docker-compose.yml, for the services the CLI owns and that file has. A
// service the old compose lacks takes the CLI default. With no compose file
// there is no pin source: the project takes the CLI defaults, and an existing
// project is told so (noticeUnpinnedHasura).
func (st *buildState) bootstrapPins() (map[string]string, error) {
	data, err := st.sink.ReadFile(filepath.Join(st.workdir, "docker-compose.yml"))
	if errors.Is(err, fs.ErrNotExist) {
		st.noticeUnpinnedHasura()
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading docker-compose.yml: %w", err)
	}
	all, err := lock.ComposeImages(data)
	if err != nil {
		return nil, err
	}
	refs := compose.CLIOwned(all)
	images, unpinned := lock.Pin(refs, lock.Resolver{LocalDigest: localRepoDigest, CLILocked: compose.CLILockedRef})
	for k, v := range unpinned {
		images[k] = v
	}
	return images, nil
}

// mergedComposeImages reads the compose files `nself start` runs (lock.MergeDetailed
// has the rules) and returns the merged image of each service plus what could
// not be verified. base is the rendered docker-compose.yml bytes, used in place
// of the file on disk when the build has them (nil reads the file). paths is the
// file list (nil reads .nself/compose-files.txt): the build passes the list it
// has just discovered, so the lock and the manifest agree. forCompare is true
// for start --frozen.
func mergedComposeImages(workdir string, base []byte, paths []string, forCompare bool) (lock.Merged, error) {
	if paths == nil {
		var err error
		if paths, err = ReadComposeManifest(workdir); err != nil {
			return lock.Merged{}, err
		}
	}
	files, err := lock.LoadSources(workdir, base, paths, docker.ImageOverrideFile)
	if err != nil {
		return lock.Merged{}, err
	}
	return lock.MergeDetailed(files, compose.IsCLIOwned, forCompare)
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
// prev is the lock on disk (nil for none): a CLI-owned service whose image cannot
// be verified (a variable, extends, build) keeps its previous entry instead of
// losing its pin.
func lockFromState(pluginDir, workdir string, composeYAML []byte, files []string, forCompare bool, prev *lock.Lock) (lock.Lock, lock.Merged, error) {
	m, err := mergedComposeImages(workdir, composeYAML, files, forCompare)
	if err != nil {
		return lock.Lock{}, m, err
	}
	l := lock.New()
	l.CLI = lock.CLIMinor(version.GetVersion())
	l.Images, l.Unpinned = lock.Pin(compose.CLIOwned(m.Images), lock.Resolver{})
	if prev != nil {
		for _, name := range m.Services {
			svc := compose.PinKey(name) // the lock is keyed by pin key
			if ref, ok := prev.Images[svc]; ok {
				l.Images[svc] = ref
				delete(l.Unpinned, svc)
			} else if ref, ok := prev.Unpinned[svc]; ok {
				l.Unpinned[svc] = ref
				delete(l.Images, svc)
			}
		}
	}
	store := ledger.NewStore(pluginDir)
	store.Sources = ledgerSources(pluginDir)
	led, _, err := store.Load()
	if err != nil {
		return lock.Lock{}, m, fmt.Errorf("reading the bundle ledger for %s: %w", lock.FileName, err)
	}
	for slug, p := range led.Plugins {
		l.Plugins[slug] = lock.PluginPin{Version: p.Version, SHA256: p.Checksum, Tier: p.Tier}
	}
	for slug := range led.Bundles {
		l.Bundles = append(l.Bundles, slug)
	}
	return l, m, nil
}

// warnOnce logs a warning once per build, however often the lock is written.
func (st *buildState) warnOnce(msg, key, val string) {
	if k := msg + "\x00" + val; !st.lockWarned[k] {
		if st.lockWarned == nil {
			st.lockWarned = map[string]bool{}
		}
		st.lockWarned[k] = true
		slog.Warn(msg, key, val)
	}
}

// pruneMirror drops the entries of the registry-mirror override file whose
// digest is not the digest of the image the compose files render without it
// (see lock.PruneMirror). A file that cannot be read as YAML is left alone.
func (st *buildState) pruneMirror(rendered map[string]string) error {
	path := filepath.Join(st.workdir, docker.ImageOverrideFile)
	data, err := st.sink.ReadFile(path)
	if err != nil {
		return nil
	}
	out, dropped, err := lock.PruneMirror(data, rendered)
	if err != nil {
		st.warnOnce("cannot prune "+docker.ImageOverrideFile, "reason", err.Error())
		return nil
	}
	if len(dropped) == 0 {
		return nil
	}
	if out == nil {
		err = st.sink.Remove(path)
	} else {
		err = st.sink.WriteAtomic(path, out, 0o600)
	}
	if err != nil {
		return fmt.Errorf("updating %s: %w", docker.ImageOverrideFile, err)
	}
	st.warnOnce("dropped stale registry-mirror entries from "+docker.ImageOverrideFile, "services", strings.Join(dropped, ","))
	st.filesGenerated++
	return nil
}

// writeProjectLock writes nself.lock.yaml through the sink when its content
// changed. A byte-identical lock is not touched (not even its mtime).
func (st *buildState) writeProjectLock(composeYAML []byte, files []string) error {
	if !st.lockActive() {
		return nil
	}
	path := lock.Path(st.workdir)
	have, rerr := st.sink.ReadFile(path)
	var prev *lock.Lock
	if pl, perr := lock.Parse(have); rerr == nil && perr == nil {
		prev = &pl
	}
	l, m, err := lockFromState(st.pluginDir, st.workdir, composeYAML, files, false, prev)
	if err != nil {
		return err
	}
	for _, b := range m.Unverifiable {
		st.warnOnce("not verified in "+lock.FileName+" (an existing pin is kept)", "reason", b)
	}
	if err := st.pruneMirror(m.Images); err != nil {
		return err
	}
	want, err := l.Marshal()
	if err != nil {
		return fmt.Errorf("rendering %s: %w", lock.FileName, err)
	}
	if rerr == nil && bytes.Equal(have, want) {
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

// UnverifiableImages lists why the images of the compose files start runs cannot
// all be verified (a variable image, extends, build or include on a CLI-owned
// service): `start --frozen` fails on any.
func UnverifiableImages(workdir string) ([]string, error) {
	m, err := mergedComposeImages(workdir, nil, nil, true)
	return m.Unverifiable, err
}

// CurrentLock computes the lock the project's docker-compose.yml and installed
// plugins describe right now, for `start --frozen` to compare with the file.
func CurrentLock(workdir string) (lock.Lock, error) {
	if _, err := os.Stat(filepath.Join(workdir, "docker-compose.yml")); err != nil {
		return lock.Lock{}, fmt.Errorf("reading docker-compose.yml: %w", err)
	}
	l, _, err := lockFromState(DefaultPluginDir(), workdir, nil, nil, true, nil)
	return l, err
}
