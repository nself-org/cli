// path.go — the path:// destination kind (local or mounted disk).
//
// Purpose: store backups under an absolute directory without rclone.
// Inputs: path://<absolute dir>, local files and keys.
// Outputs: objects under the directory, files 0600, directories 0700.
// Constraints: every access goes through os.Root, so a ".." or a symlink
// cannot leave the directory; the directory itself and every component below
// it must not be a symlink; writes go to <key>.tmp, are fsynced, renamed and re-read to compare
// sha256. A symlink at the final component is refused, never followed.
package destinations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type pathDest struct{ dir string }

func newPathDest(dir string) (*pathDest, error) {
	if dir == "" || !filepath.IsAbs(dir) || strings.ContainsRune(dir, 0) {
		return nil, fmt.Errorf("path:// needs an absolute directory (got %q)", dir)
	}
	for _, seg := range strings.Split(filepath.ToSlash(dir), "/") {
		if seg == ".." {
			return nil, fmt.Errorf("path:// directory must not contain '..' (got %q)", dir)
		}
	}
	return &pathDest{dir: filepath.Clean(dir)}, nil
}

func (d *pathDest) Kind() string { return KindPath }

// root opens the destination directory, creating it 0700 when asked.
func (d *pathDest) root(create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(d.dir, 0o700); err != nil {
			return nil, fmt.Errorf("create %s: %w", d.dir, err)
		}
	}
	// The destination root itself is never followed when it is a symlink: a
	// link can be retargeted, so its target is not the directory the operator
	// named. Ancestors of the root are resolved normally.
	if fi, err := os.Lstat(d.dir); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is a symlink; use the real directory path", d.dir)
	}
	r, err := os.OpenRoot(d.dir)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", d.dir, err)
	}
	return r, nil
}

// noSymlinks refuses a symlink at any existing component of key.
func noSymlinks(r *os.Root, key string) error {
	cur := ""
	for _, seg := range strings.Split(key, "/") {
		cur = path.Join(cur, seg)
		fi, err := r.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink; refusing to follow it", cur)
		}
	}
	return nil
}

// Put writes localPath to <dir>/<key> atomically and verifies it by sha256.
func (d *pathDest) Put(ctx context.Context, localPath, key string) (err error) {
	if err := validKey(key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	src, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	r, err := d.root(true)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if err := noSymlinks(r, key); err != nil {
		return err
	}
	if sub := path.Dir(key); sub != "." {
		if err := r.MkdirAll(sub, 0o700); err != nil {
			return err
		}
	}
	tmp := key + tmpSuffix
	_ = r.Remove(tmp)
	f, err := r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = r.Remove(tmp)
		}
	}()
	h := sha256.New()
	if _, err = io.Copy(io.MultiWriter(f, h), &ctxReader{ctx, src}); err == nil {
		if err = f.Chmod(0o600); err == nil {
			err = f.Sync()
		}
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("write %s: %w", key, err)
	}
	if err = r.Rename(tmp, key); err != nil {
		return err
	}
	syncDir(r, path.Dir(key))
	return verifyHash(r, key, h.Sum(nil))
}

// verifyHash re-reads key and compares its sha256 with want; a mismatch
// removes the object.
func verifyHash(r *os.Root, key string, want []byte) error {
	f, err := r.Open(key)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if !bytes.Equal(h.Sum(nil), want) {
		_ = r.Remove(key)
		return fmt.Errorf("sha256 mismatch after writing %s", key)
	}
	return nil
}

func syncDir(r *os.Root, dir string) {
	if f, err := r.Open(dir); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}
}

// Open opens a regular, non-symlink object for reading.
func (d *pathDest) Open(_ context.Context, key string) (io.ReadCloser, error) {
	if err := validKey(key); err != nil {
		return nil, err
	}
	r, err := d.root(false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	if err := noSymlinks(r, key); err != nil {
		return nil, err
	}
	li, err := r.Lstat(key)
	if err != nil {
		return nil, err
	}
	if !li.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", key)
	}
	f, err := r.Open(key)
	if err != nil {
		return nil, err
	}
	if fi, err := f.Stat(); err != nil || !os.SameFile(li, fi) {
		_ = f.Close()
		return nil, fmt.Errorf("%s changed while opening", key)
	}
	return f, nil
}

// Get copies <dir>/<key> to localPath (temp file in the same directory, then
// rename; 0600).
func (d *pathDest) Get(ctx context.Context, key, localPath string) error {
	in, err := d.Open(ctx, key)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	tmp, err := os.CreateTemp(filepath.Dir(localPath), ".get-*")
	if err != nil {
		return err
	}
	_, err = io.Copy(tmp, &ctxReader{ctx, in})
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), localPath)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

// List returns the regular files whose key starts with prefix, sorted by key.
// Symlinks and in-progress ".tmp" files are skipped.
func (d *pathDest) List(_ context.Context, prefix string) ([]Object, error) {
	r, err := d.root(false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	var objs []Object
	err = fs.WalkDir(r.FS(), ".", func(p string, e fs.DirEntry, werr error) error {
		if werr != nil || e.IsDir() || !e.Type().IsRegular() ||
			strings.HasSuffix(p, tmpSuffix) || !strings.HasPrefix(p, prefix) {
			return werr
		}
		fi, ierr := e.Info()
		if ierr != nil {
			return ierr
		}
		objs = append(objs, Object{Key: p, Size: fi.Size(), ModTime: fi.ModTime()})
		return nil
	})
	sortObjects(objs)
	return objs, err
}

// ctxReader stops a copy when ctx is cancelled.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

func sortObjects(objs []Object) {
	sort.Slice(objs, func(i, j int) bool { return objs[i].Key < objs[j].Key })
}
