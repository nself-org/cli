// rclone.go — the rclone destination kind and the pre-interface helpers.
//
// Purpose: drive rclone for s3, r2, minio, b2, gcs and az remotes. IsRemoteKey,
// FetchRemote and toRclonePath predate the Destination interface and keep
// their exact behaviour and argv for existing callers (pitr restore).
// Inputs: remote URIs such as s3://bucket/path/to/object.
// Outputs: files downloaded or uploaded through rclone.
// Constraints: rclone argv is frozen (see rclone_golden_test.go).
//
// Remote keys follow rclone's URI scheme:
//
//	s3://bucket/path/to/object
//	r2://bucket/path/to/object
//	minio://bucket/path/to/object
//	b2://bucket/path/to/object
//	gcs://bucket/path/to/object
//	az://container/path/to/object
//
// The minio:// scheme is an nSelf convention: it is rewritten to a
// "minio" rclone remote automatically (rclone remote name "minio").
// All other schemes are passed through to rclone verbatim (scheme
// becomes the rclone remote name).
//
// rclone must be installed and configured (rclone.conf or environment
// variables) before FetchRemote is called.
package destinations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// IsRemoteKey reports whether key is a supported remote destination URI
// (i.e. not a local filesystem path).
func IsRemoteKey(key string) bool {
	for _, scheme := range []string{"s3://", "r2://", "minio://", "b2://", "gcs://", "az://"} {
		if strings.HasPrefix(key, scheme) {
			return true
		}
	}
	return false
}

// FetchRemote downloads the object at remoteKey to a local file in destDir.
// It returns the local file path on success.
//
// remoteKey must be a URI understood by rclone (or a minio:// URI — see
// package-level documentation for the rewrite rules).
//
// If remoteKey ends in ".age" the caller is responsible for decrypting
// the returned file; FetchRemote copies the raw (still encrypted) bytes.
func FetchRemote(ctx context.Context, remoteKey, destDir string) (string, error) {
	if remoteKey == "" {
		return "", fmt.Errorf("remote key is empty")
	}
	if destDir == "" {
		return "", fmt.Errorf("destination directory is required")
	}

	// Verify rclone is available before attempting the download.
	if _, err := exec.LookPath("rclone"); err != nil {
		return "", fmt.Errorf("rclone not found on PATH: install rclone to use remote backup destinations")
	}

	// Translate URI scheme to an rclone remote path.
	rclonePath, err := toRclonePath(remoteKey)
	if err != nil {
		return "", err
	}

	// Derive a local destination filename from the last path component.
	objectName := filepath.Base(strings.TrimSuffix(remoteKey, "/"))
	if objectName == "" || objectName == "." {
		objectName = "base.tar.gz"
	}
	localPath := filepath.Join(destDir, objectName)

	slog.Info("pitr_fetch_remote",
		"remote_key", remoteKey,
		"rclone_path", rclonePath,
		"local_path", localPath,
	)

	// Use "rclone copyto <remote> <local>" for a single-file download.
	// "copyto" preserves the exact object without rclone adding directories.
	cmd := exec.CommandContext(ctx, "rclone", "copyto", rclonePath, localPath)

	// Let rclone inherit the environment so that rclone.conf, AWS_* variables,
	// and provider-specific env vars (RCLONE_CONFIG_*) are all visible.
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return "", fmt.Errorf("rclone copyto %s: %s: %w", rclonePath, msg, err)
		}
		return "", fmt.Errorf("rclone copyto %s: %w", rclonePath, err)
	}

	// Verify the file actually arrived.
	if _, statErr := os.Stat(localPath); statErr != nil {
		return "", fmt.Errorf("rclone copyto succeeded but %s not found: %w", localPath, statErr)
	}

	slog.Info("pitr_fetch_remote_done", "local_path", localPath)
	return localPath, nil
}

// toRclonePath converts an nSelf remote key URI to an rclone path.
//
// rclone's path format is "<remote>:<bucket>/<key>" or "<remote>:<path>".
// The URI schemes map as follows:
//
//	s3://bucket/key    → "s3:bucket/key"   (standard AWS S3 / compatible)
//	r2://bucket/key    → "r2:bucket/key"   (Cloudflare R2)
//	minio://bucket/key → "minio:bucket/key" (rclone remote named "minio")
//	b2://bucket/key    → "b2:bucket/key"   (Backblaze B2)
//	gcs://bucket/key   → "gcs:bucket/key"  (Google Cloud Storage)
//	az://container/key → "az:container/key" (Azure Blob Storage)
func toRclonePath(uri string) (string, error) {
	schemes := map[string]string{
		"s3://":    "s3:",
		"r2://":    "r2:",
		"minio://": "minio:",
		"b2://":    "b2:",
		"gcs://":   "gcs:",
		"az://":    "az:",
	}

	for scheme, rclonePrefix := range schemes {
		if strings.HasPrefix(uri, scheme) {
			rest := strings.TrimPrefix(uri, scheme)
			if rest == "" {
				return "", fmt.Errorf("remote key %q has no bucket/path after scheme", uri)
			}
			return rclonePrefix + rest, nil
		}
	}

	return "", fmt.Errorf("unsupported remote key scheme in %q: supported schemes are s3://, r2://, minio://, b2://, gcs://, az://", uri)
}

// rcloneDest is the rclone kind. remote is the destination exactly as the
// caller gave it ("s3:bucket/prefix", "r2://bucket/prefix", ...).
type rcloneDest struct {
	remote string
	env    []string
}

func (d *rcloneDest) Kind() string { return KindRclone }

// run starts rclone with the inherited environment plus d.env and returns the
// combined output.
func (d *rcloneDest) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "rclone", args...)
	cmd.Env = append(os.Environ(), d.env...)
	return cmd.CombinedOutput()
}

// Put runs `rclone copyto <localPath> <remote>/<key>`, the argv backup create
// has always used.
func (d *rcloneDest) Put(ctx context.Context, localPath, key string) error {
	out, err := d.run(ctx, "copyto", localPath, d.remote+"/"+key)
	if err != nil {
		if len(out) > 0 {
			return errors.New(string(out))
		}
		return err
	}
	return nil
}

// objectPath is the rclone path of key. A scheme URI is rewritten as
// FetchRemote does; anything else is used verbatim.
func (d *rcloneDest) objectPath(key string) (string, error) {
	base := d.remote
	if IsRemoteKey(base) {
		var err error
		if base, err = toRclonePath(base); err != nil {
			return "", err
		}
	}
	if key == "" {
		return base, nil
	}
	return strings.TrimSuffix(base, "/") + "/" + key, nil
}

// Get runs `rclone copyto <remote>/<key> <localPath>`.
func (d *rcloneDest) Get(ctx context.Context, key, localPath string) error {
	src, err := d.objectPath(key)
	if err != nil {
		return err
	}
	if out, err := d.run(ctx, "copyto", src, localPath); err != nil {
		return fmt.Errorf("rclone copyto %s: %s: %w", src, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Open streams `rclone cat <remote>/<key>`.
func (d *rcloneDest) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	src, err := d.objectPath(key)
	if err != nil {
		return nil, err
	}
	return startCat(exec.CommandContext(ctx, "rclone", "cat", src), d.env)
}

type lsjsonEntry struct {
	Path    string    `json:"Path"`
	Size    int64     `json:"Size"`
	ModTime time.Time `json:"ModTime"`
	IsDir   bool      `json:"IsDir"`
}

// List runs `rclone lsjson -R <remote>` and returns the files whose key starts
// with prefix.
func (d *rcloneDest) List(ctx context.Context, prefix string) ([]Object, error) {
	src, err := d.objectPath("")
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "rclone", "lsjson", "-R", "--files-only", src)
	cmd.Env = append(os.Environ(), d.env...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("rclone lsjson %s: %w", src, err)
	}
	var entries []lsjsonEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parse rclone lsjson output: %w", err)
	}
	objs := make([]Object, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir && strings.HasPrefix(e.Path, prefix) {
			objs = append(objs, Object{Key: e.Path, Size: e.Size, ModTime: e.ModTime})
		}
	}
	return objs, nil
}

// startCat starts cmd with its stdout exposed as a reader. Close waits for the
// process and reports a non-zero exit with its stderr text.
func startCat(cmd *exec.Cmd, env []string) (io.ReadCloser, error) {
	cmd.Env = append(os.Environ(), env...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", cmd.Args[0], err)
	}
	return &catReader{ReadCloser: out, cmd: cmd, stderr: &stderr}, nil
}

type catReader struct {
	io.ReadCloser
	cmd    *exec.Cmd
	stderr *strings.Builder
}

func (c *catReader) Close() error {
	_ = c.ReadCloser.Close()
	if err := c.cmd.Wait(); err != nil {
		if msg := strings.TrimSpace(c.stderr.String()); msg != "" {
			return fmt.Errorf("%s: %s: %w", c.cmd.Args[0], msg, err)
		}
		return fmt.Errorf("%s: %w", c.cmd.Args[0], err)
	}
	return nil
}
