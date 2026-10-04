package sdk

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/zip"
)

// zipFile is a module-zip input backed by a file on disk. data, when set,
// stands in for the file contents (used for the injected root LICENSE).
type zipFile struct {
	path string
	disk string
	data string
}

func (f zipFile) Path() string                { return f.path }
func (f zipFile) Lstat() (os.FileInfo, error) { return os.Lstat(f.disk) }
func (f zipFile) Open() (io.ReadCloser, error) {
	if f.data != "" {
		return io.NopCloser(strings.NewReader(f.data)), nil
	}
	return os.Open(f.disk)
}

// moduleFiles lists the files the module zip for dir would hold, with the
// exclusions the go command applies (VCS dirs, vendor, nested modules). When
// withRootLicense is true it adds a LICENSE file at the module root: for a
// module in a repository subdirectory without its own LICENSE, the go command
// copies the repository-root LICENSE into the zip, so the check must see it.
func moduleFiles(t *testing.T, dir string, withRootLicense bool) []zip.File {
	t.Helper()
	var files []zip.File
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".hg", ".bzr", ".svn", "vendor":
				return filepath.SkipDir
			}
			if rel != "." {
				if _, statErr := os.Stat(filepath.Join(p, "go.mod")); statErr == nil {
					return filepath.SkipDir // nested module
				}
			}
			return nil
		}
		if d.Type().IsRegular() {
			files = append(files, zipFile{path: filepath.ToSlash(rel), disk: p})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	if withRootLicense {
		files = append(files, zipFile{path: "LICENSE", disk: dir, data: "stand-in for the repo-root LICENSE"})
	}
	return files
}

// TestModuleZip builds the file set of the sdk/go module zip, including the
// repo-root LICENSE the go command injects, and fails on any path problem,
// notably two paths that collide case-insensitively (a package directory named
// license next to LICENSE made every version of the module unfetchable).
func TestModuleZip(t *testing.T) {
	cf, err := zip.CheckFiles(moduleFiles(t, ".", true))
	if err != nil {
		t.Fatalf("module zip would not build: %v", err)
	}
	if err := cf.Err(); err != nil {
		t.Fatalf("module zip would not build: %v", err)
	}
}

// TestModuleZipDetectsCollision proves the guard works: a license/ directory
// next to the injected LICENSE is reported as a collision.
func TestModuleZipDetectsCollision(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "license"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "license", "x.go"), []byte("package license\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// CheckFiles reports a path collision as its error, other problems in the
	// returned CheckedFiles; either one proves the guard fires.
	cf, err := zip.CheckFiles(moduleFiles(t, dir, true))
	if err == nil && cf.Err() == nil {
		t.Fatal("expected a case-insensitive collision between LICENSE and license/, got none")
	}
}
