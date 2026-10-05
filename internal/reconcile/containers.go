package reconcile

// containers.go — container impact of a plan (EPIC P7-LIVE D7).
//
// Purpose: say which running services the next `nself start` would recreate
// and which of those hold a named volume, without changing anything.
// Inputs: the planned build (bytes it would write) and the project on disk.
// Outputs: Containers items (action recreate or unknown, stateful, applied by
// the next start) and, with RemoveOrphans, orphan-remove effects.
// Constraints: the invocation is the one `nself start` builds: the files of
// build.ReadComposeManifest and the env files of build.ComposeEnvFiles, run
// over a temporary "shadow" copy of the project's small inputs with the planned
// bytes substituted, so the answer reflects the project after the change and
// the project itself is never written. Docker unreachable or compose failing
// reports known=false and a stderr notice, never an error and never a guess.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	nbuild "github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/docker"
)

const dockerTimeout = 30 * time.Second

// containerImpact computes the Containers section and the orphan effects.
func containerImpact(ctx context.Context, req Request, dir string, res *nbuild.BuildResult, ov overlay) (Containers, []Effect) {
	rt := req.Runtime
	if rt == nil {
		rt = dockerRuntime{}
	}
	notice := func(format string, a ...any) {
		if req.Stderr != nil {
			_, _ = fmt.Fprintf(req.Stderr, "nself: "+format+"\n", a...)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, dockerTimeout)
	defer cancel()

	running, err := rt.Running(ctx, res.ProjectName)
	if err != nil {
		notice("container impact unknown: docker is not reachable (%v)", err)
		return Containers{Known: false, Items: []ContainerItem{}}, nil
	}
	shadow, err := os.MkdirTemp("", "nself-plan-")
	if err != nil {
		notice("container impact unknown: %v", err)
		return Containers{Known: false, Items: []ContainerItem{}}, nil
	}
	defer func() { _ = os.RemoveAll(shadow) }()

	files, envFiles, err := shadowInvocation(shadow, dir, ov)
	if err != nil {
		notice("container impact unknown: %v", err)
		return Containers{Known: false, Items: []ContainerItem{}}, nil
	}
	hashes, err := rt.ConfigHashes(ctx, files, envFiles, dir)
	if err != nil {
		notice("container impact unknown: %v", err)
		return Containers{Known: false, Items: []ContainerItem{}}, nil
	}
	stateful, err := docker.StatefulServices(files)
	if err != nil {
		notice("container impact unknown: %v", err)
		return Containers{Known: false, Items: []ContainerItem{}}, nil
	}
	defined, err := docker.ComposeServiceNames(files)
	if err != nil {
		notice("container impact unknown: %v", err)
		return Containers{Known: false, Items: []ContainerItem{}}, nil
	}
	items, orphans := compareRunning(running, hashes, defined, stateful)
	var effects []Effect
	if req.RemoveOrphans {
		for _, o := range orphans {
			effects = append(effects, Effect{Kind: EffectOrphanRemove, Target: o.Name,
				Detail: fmt.Sprintf("service %q is not in the planned compose", o.Service)})
		}
	}
	return Containers{Known: true, Items: items}, effects
}

// compareRunning matches running containers with the planned hashes. A service
// is recreate when any of its containers carries a different hash, unknown
// when one carries none (treated as a recreate), none otherwise. A container
// whose service is defined nowhere in the planned compose is an orphan.
func compareRunning(running []RunningContainer, hashes map[string]string, defined map[string]struct{}, stateful map[string]bool) ([]ContainerItem, []RunningContainer) {
	action := map[string]ContainerAction{}
	var orphans []RunningContainer
	for _, c := range running {
		if _, ok := defined[c.Service]; !ok {
			orphans = append(orphans, c)
			continue
		}
		want, ok := hashes[c.Service]
		if !ok {
			continue // defined but not in the active configuration (profile off)
		}
		a := ContainerNone
		switch {
		case c.ConfigHash == "":
			a = ContainerUnknown
		case c.ConfigHash != want:
			a = ContainerRecreate
		}
		if rank(a) > rank(action[c.Service]) {
			action[c.Service] = a
		}
	}
	items := []ContainerItem{}
	for svc, a := range action {
		if a == ContainerNone {
			continue
		}
		items = append(items, ContainerItem{Service: svc, Action: a, Stateful: stateful[svc], AppliedBy: AppliedNextStart})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Service < items[j].Service })
	sort.Slice(orphans, func(i, j int) bool { return orphans[i].Name < orphans[j].Name })
	return items, orphans
}

// rank orders actions by severity: none < unknown < recreate.
func rank(a ContainerAction) int {
	switch a {
	case ContainerRecreate:
		return 3
	case ContainerUnknown:
		return 2
	case ContainerNone:
		return 1
	}
	return 0
}

// shadowInvocation copies the project inputs `start` reads into shadow, with
// planned bytes substituted, and returns the -f and --env-file lists computed
// by the build package's own functions over that copy.
func shadowInvocation(shadow, dir string, ov overlay) (files, envFiles []string, err error) {
	for _, rel := range []string{".nself/compose-files.txt", "docker-compose.yml", "docker-compose.override.yml", ".env", ".nself/compose.env"} {
		data, ok, rerr := ov.read(rel)
		if rerr != nil {
			return nil, nil, rerr
		}
		if ok {
			if err := writeShadow(filepath.Join(shadow, filepath.FromSlash(rel)), data); err != nil {
				return nil, nil, err
			}
		}
	}
	if err := shadowManifestLines(shadow, dir, ov); err != nil {
		return nil, nil, err
	}
	list, err := nbuild.ReadComposeManifest(shadow)
	if err != nil {
		return nil, nil, err
	}
	for _, f := range list {
		if !filepath.IsAbs(f) {
			f = filepath.Join(shadow, f)
		}
		files = append(files, f)
	}
	return files, nbuild.ComposeEnvFiles(shadow), nil
}

// shadowManifestLines rewrites the shadow manifest so every listed file is an
// absolute path into the shadow, holding the planned bytes (a fragment is
// rewritten in place under the plugin dir, so its planned bytes differ from
// disk). Lines whose file exists neither planned nor on disk are dropped, as
// ReadComposeManifest drops them.
func shadowManifestLines(shadow, dir string, ov overlay) error {
	raw, ok, err := ov.read(".nself/compose-files.txt")
	if err != nil || !ok {
		return err
	}
	var out []string
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		path := line
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		data, found, err := ov.readAbs(path)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		dst := filepath.Join(shadow, "frag", fmt.Sprintf("%d", i), filepath.Base(path))
		if err := writeShadow(dst, data); err != nil {
			return err
		}
		out = append(out, dst)
	}
	return writeShadow(filepath.Join(shadow, ".nself", "compose-files.txt"), []byte(strings.Join(out, "\n")+"\n"))
}

// writeShadow writes one shadow file, creating its directory.
func writeShadow(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// dockerRuntime is the default ContainerRuntime: the docker CLI.
type dockerRuntime struct{}

func (dockerRuntime) Running(ctx context.Context, project string) ([]RunningContainer, error) {
	rs, err := docker.RunningConfigHashes(ctx, project)
	if err != nil {
		return nil, err
	}
	out := make([]RunningContainer, 0, len(rs))
	for _, r := range rs {
		out = append(out, RunningContainer{Name: r.Name, Service: r.Service, State: r.State, ConfigHash: r.ConfigHash})
	}
	return out, nil
}

func (dockerRuntime) ConfigHashes(ctx context.Context, files, envFiles []string, projectDir string) (map[string]string, error) {
	return docker.ComposeConfigHashes(ctx, files, envFiles, projectDir)
}

// overlay reads project files as the planned build would leave them: planned
// bytes first, a planned removal as absent, else the file on disk.
type overlay struct {
	dir      string
	fronting string
	files    map[string]nbuild.PlannedFile
	removed  map[string]bool
}

// newOverlay indexes a planned build.
func newOverlay(dir, fronting string, pb *nbuild.PlannedBuild) overlay {
	ov := overlay{dir: dir, fronting: fronting, files: pb.Files, removed: map[string]bool{}}
	for _, k := range pb.Removed {
		ov.removed[k] = true
	}
	return ov
}

// diskPath resolves a canonical key to its location on disk.
func (o overlay) diskPath(key string) string {
	switch {
	case strings.HasPrefix(key, FrontingPrefix):
		return filepath.Join(o.fronting, filepath.FromSlash(strings.TrimPrefix(key, FrontingPrefix)))
	case filepath.IsAbs(key):
		return key
	}
	return filepath.Join(o.dir, filepath.FromSlash(key))
}

// read returns the bytes of a canonical key as planned. ok is false when the
// file would not exist; an error is a read failure other than not-exist.
func (o overlay) read(key string) ([]byte, bool, error) {
	if f, ok := o.files[key]; ok {
		return f.Data, true, nil
	}
	if o.removed[key] {
		return nil, false, nil
	}
	data, err := os.ReadFile(o.diskPath(key))
	switch {
	case err == nil:
		return data, true, nil
	case os.IsNotExist(err):
		return nil, false, nil
	}
	return nil, false, err
}

// readAbs is read for an absolute path: it is looked up under the key the
// build uses (project-relative inside the project, absolute outside).
func (o overlay) readAbs(path string) ([]byte, bool, error) {
	if rel, err := filepath.Rel(o.dir, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return o.read(filepath.ToSlash(rel))
	}
	return o.read(filepath.ToSlash(path))
}
