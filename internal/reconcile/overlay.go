package reconcile

// overlay.go — reads project files as the planned build would leave them
// (EPIC P7-LIVE D7): planned bytes first, a planned removal as absent, else
// the file on disk. Split from containers.go for the file-size cap.

import (
	"os"
	"path/filepath"
	"strings"

	nbuild "github.com/nself-org/cli/internal/build"
)

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

// display is the plan path of a canonical key: project-relative and
// "@fronting/" keys as they are, an absolute key under the plugin directory as
// "@plugins/<rel>", any other absolute key as "@abs/<path>".
func (o overlay) display(key string) string {
	if !filepath.IsAbs(key) {
		return key
	}
	if rel, err := filepath.Rel(nbuild.DefaultPluginDir(), key); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return PluginsPrefix + filepath.ToSlash(rel)
	}
	return AbsPrefix + strings.TrimPrefix(filepath.ToSlash(key), "/")
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
