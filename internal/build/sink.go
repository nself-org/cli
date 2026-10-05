package build

// sink.go — the Sink seam: every file the build pipeline writes goes through
// a Sink, so one pipeline serves two modes (P7-LIVE-21).
//
// Purpose: ModeWrite uses diskSink (today's os calls); ModePlan uses memSink,
// which records writes and removals in memory and serves reads of planned
// paths (overlay) before falling back to disk.
// Inputs: paths are project-relative or absolute; absolute paths under the
// project root canonicalise to relative ones, and paths under the fronting
// stack's nginx/sites dir to "@fronting/<name>".
// Outputs: PlannedBuild (files, modes, removals, effects) from memSink.
// Constraints: memSink never writes or creates a directory; internal/build must
// not import internal/reconcile (P7-GUARD), so the store is a plain PlannedFile
// map that P7-LIVE-03 converts.

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// frontingPrefix roots the fronting stack's nginx/sites dir in canonical keys.
const frontingPrefix = "@fronting/"

// PlannedFile is one file a plan-mode build would write.
type PlannedFile struct {
	Data []byte
	Perm fs.FileMode
}

// PlannedBuild is what a plan-mode build would do, without having done it.
type PlannedBuild struct {
	// Files maps a canonical path to the bytes and mode the build would write.
	Files map[string]PlannedFile
	// Removed lists canonical paths the build would delete (sorted).
	Removed []string
	// Modes lists permission changes to files the build does not itself write.
	Modes map[string]fs.FileMode
	// Effects lists host effects the build would perform, in order.
	Effects []PlannedEffect
}

// Sink is the only way the build pipeline writes into a project.
type Sink interface {
	WriteFile(path string, data []byte, perm fs.FileMode) error
	// WriteAtomic writes via temp file + rename; the final mode is exactly perm.
	WriteAtomic(path string, data []byte, perm fs.FileMode) error
	Remove(path string) error
	Chmod(path string, perm fs.FileMode) error
	MkdirAll(path string, perm fs.FileMode) error
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]fs.DirEntry, error)
	Stat(path string) (fs.FileInfo, error)
}

// diskSink is write mode: each method is the os call the pipeline made before.
type diskSink struct{ root, fronting string }

func newDiskSink(root string) *diskSink { return &diskSink{root: root} }

// abs resolves a project-relative, absolute or "@fronting/" path to disk.
func (d *diskSink) abs(p string) string { return resolveSinkPath(d.root, d.fronting, p) }

func (d *diskSink) WriteFile(p string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(d.abs(p), data, perm)
}
func (d *diskSink) WriteAtomic(p string, data []byte, perm fs.FileMode) error {
	return atomicWrite(d.abs(p), data, perm)
}
func (d *diskSink) Remove(p string) error                     { return os.Remove(d.abs(p)) }
func (d *diskSink) Chmod(p string, perm fs.FileMode) error    { return os.Chmod(d.abs(p), perm) }
func (d *diskSink) MkdirAll(p string, perm fs.FileMode) error { return os.MkdirAll(d.abs(p), perm) }
func (d *diskSink) ReadFile(p string) ([]byte, error)         { return os.ReadFile(d.abs(p)) }
func (d *diskSink) ReadDir(p string) ([]fs.DirEntry, error)   { return os.ReadDir(d.abs(p)) }
func (d *diskSink) Stat(p string) (fs.FileInfo, error)        { return os.Stat(d.abs(p)) }

// resolveSinkPath maps a sink path to its absolute location on disk.
func resolveSinkPath(root, fronting, p string) string {
	if strings.HasPrefix(p, frontingPrefix) {
		return filepath.Join(fronting, filepath.FromSlash(strings.TrimPrefix(p, frontingPrefix)))
	}
	if filepath.IsAbs(p) || root == "" {
		return p
	}
	return filepath.Join(root, p)
}

// memSink is plan mode: writes land in maps, reads see them first.
type memSink struct {
	diskSink
	files   map[string]PlannedFile
	dirs    map[string]bool
	removed map[string]bool
	modes   map[string]fs.FileMode
}

func newMemSink(root, fronting string) *memSink {
	return &memSink{diskSink: diskSink{root: root, fronting: fronting}, files: map[string]PlannedFile{},
		dirs: map[string]bool{}, removed: map[string]bool{}, modes: map[string]fs.FileMode{}}
}

// key canonicalises p: project-relative (slash form), "@fronting/<name>", or
// the absolute path for anything else (for example a plugin directory).
func (m *memSink) key(p string) string {
	if strings.HasPrefix(p, frontingPrefix) {
		return p
	}
	if !filepath.IsAbs(p) {
		return filepath.ToSlash(filepath.Clean(p))
	}
	if rel, ok := under(m.root, p); ok {
		return rel
	}
	if m.fronting != "" {
		if rel, ok := under(m.fronting, p); ok {
			return frontingPrefix + rel
		}
	}
	return filepath.ToSlash(filepath.Clean(p))
}

// under returns p relative to dir (slash form) when p is inside dir.
func under(dir, p string) (string, bool) {
	if dir == "" {
		return "", false
	}
	rel, err := filepath.Rel(dir, p)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func (m *memSink) WriteFile(p string, data []byte, perm fs.FileMode) error {
	if _, planned := m.files[m.key(p)]; !planned {
		// os.WriteFile keeps the mode of a file that already exists.
		if info, err := os.Stat(m.abs(p)); err == nil && info.Mode().IsRegular() {
			perm = info.Mode().Perm()
		}
	}
	return m.WriteAtomic(p, data, perm)
}

func (m *memSink) WriteAtomic(p string, data []byte, perm fs.FileMode) error {
	k := m.key(p)
	m.files[k] = PlannedFile{Data: append([]byte(nil), data...), Perm: perm}
	delete(m.removed, k)
	return nil
}

func (m *memSink) Remove(p string) error {
	k := m.key(p)
	if _, planned := m.files[k]; planned {
		delete(m.files, k)
		// Removing a planned file also hides an older on-disk copy, if any.
		if _, err := os.Stat(m.abs(p)); err == nil {
			m.removed[k] = true
		}
		return nil
	}
	if m.removed[k] {
		return &fs.PathError{Op: "remove", Path: p, Err: fs.ErrNotExist}
	}
	if _, err := os.Stat(m.abs(p)); err != nil {
		return err
	}
	m.removed[k] = true
	return nil
}

func (m *memSink) Chmod(p string, perm fs.FileMode) error {
	k := m.key(p)
	if f, ok := m.files[k]; ok {
		m.files[k] = PlannedFile{Data: f.Data, Perm: perm}
	} else {
		m.modes[k] = perm
	}
	return nil
}

func (m *memSink) MkdirAll(p string, _ fs.FileMode) error {
	m.dirs[m.key(p)] = true
	return nil
}

func (m *memSink) ReadFile(p string) ([]byte, error) {
	k := m.key(p)
	if f, ok := m.files[k]; ok {
		return append([]byte(nil), f.Data...), nil
	}
	if m.removed[k] {
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
	return os.ReadFile(m.abs(p))
}

func (m *memSink) Stat(p string) (fs.FileInfo, error) {
	k := m.key(p)
	if f, ok := m.files[k]; ok {
		return memInfo{name: filepath.Base(p), size: int64(len(f.Data)), mode: f.Perm}, nil
	}
	if m.removed[k] {
		return nil, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
	}
	if info, err := os.Stat(m.abs(p)); err == nil {
		return info, nil
	} else if !errors.Is(err, fs.ErrNotExist) || !m.hasChildren(k) {
		return nil, err
	}
	return memInfo{name: filepath.Base(p), mode: fs.ModeDir | 0o755}, nil
}

// hasChildren reports whether a planned file or directory sits at or below k.
func (m *memSink) hasChildren(k string) bool {
	if m.dirs[k] {
		return true
	}
	for f := range m.files {
		if strings.HasPrefix(f, k+"/") {
			return true
		}
	}
	for d := range m.dirs {
		if strings.HasPrefix(d, k+"/") {
			return true
		}
	}
	return false
}

// ReadDir merges the on-disk listing with planned files directly below p and
// drops removed names, sorted by name like os.ReadDir.
func (m *memSink) ReadDir(p string) ([]fs.DirEntry, error) {
	k := m.key(p)
	entries, diskErr := os.ReadDir(m.abs(p))
	if diskErr != nil && !errors.Is(diskErr, fs.ErrNotExist) {
		return nil, diskErr
	}
	byName := map[string]fs.DirEntry{}
	for _, e := range entries {
		if !m.removed[joinKey(k, e.Name())] {
			byName[e.Name()] = e
		}
	}
	prefix := joinKey(k, "")
	for f, pf := range m.files {
		if rest := strings.TrimPrefix(f, prefix); rest != f && !strings.Contains(rest, "/") {
			byName[rest] = fs.FileInfoToDirEntry(memInfo{name: rest, size: int64(len(pf.Data)), mode: pf.Perm})
		}
	}
	if diskErr != nil && len(byName) == 0 && !m.dirs[k] {
		return nil, diskErr
	}
	var out []fs.DirEntry
	for _, e := range byName {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func joinKey(dir, name string) string {
	if dir == "" || dir == "." {
		return name
	}
	return dir + "/" + name
}

// snapshot returns the plan so far; effects are attached by the caller.
func (m *memSink) snapshot() *PlannedBuild {
	pb := &PlannedBuild{Files: map[string]PlannedFile{}, Modes: maps.Clone(m.modes)}
	for k, f := range m.files {
		pb.Files[k] = PlannedFile{Data: append([]byte(nil), f.Data...), Perm: f.Perm}
	}
	for k := range m.removed {
		pb.Removed = append(pb.Removed, k)
	}
	sort.Strings(pb.Removed)
	return pb
}

// memInfo is the fs.FileInfo of a planned file or directory.
type memInfo struct {
	name string
	size int64
	mode fs.FileMode
}

func (i memInfo) Name() string       { return i.name }
func (i memInfo) Size() int64        { return i.size }
func (i memInfo) Mode() fs.FileMode  { return i.mode }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return i.mode.IsDir() }
func (i memInfo) Sys() any           { return nil }
