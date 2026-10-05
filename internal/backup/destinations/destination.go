// Package destinations — destination.go: the Destination interface, Parse and
// the kind registry.
//
// Purpose: one interface for every place a backup can be written to or read
// from, so callers stop switching on URI schemes.
// Inputs: a destination URI and, for host://, the controlplane inventory.
// Outputs: a Destination of one of three kinds: rclone (s3 r2 minio b2 gcs az
// and bare rclone remotes), path (path://<abs dir>) and host
// (host://<inventory server>/<abs dir>).
// Constraints: Parse never touches the network or starts a process.
package destinations

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
)

// Kind names.
const (
	KindRclone = "rclone"
	KindPath   = "path"
	KindHost   = "host"
)

const (
	pathScheme = "path://"
	hostScheme = "host://"
)

// Object is one stored backup object.
type Object struct {
	Key     string
	Size    int64
	ModTime time.Time
}

// Destination is a place backups are stored. key is a relative, slash
// separated name inside the destination.
type Destination interface {
	Kind() string
	Put(ctx context.Context, localPath, key string) error
	Get(ctx context.Context, key, localPath string) error
	List(ctx context.Context, prefix string) ([]Object, error)
}

// Opener is implemented by destinations that can stream an object without a
// local copy (restore-remote pipes it into age and pg_restore).
type Opener interface {
	Open(ctx context.Context, key string) (io.ReadCloser, error)
}

// Inventory is the controlplane inventory host:// resolves servers from.
type Inventory = controlplane.Inventory

// KindInfo describes one destination kind for `nself backup config`.
type KindInfo struct {
	Kind    string `json:"kind"`
	Example string `json:"example"`
}

// Kinds lists every supported destination kind.
func Kinds() []KindInfo {
	return []KindInfo{
		{KindRclone, "s3://bucket/prefix (also r2 minio b2 gcs az, or a configured rclone remote)"},
		{KindPath, "path:///mnt/backups (local or mounted disk)"},
		{KindHost, "host://<inventory-server>/srv/backups (over SSH)"},
	}
}

// KindOf reports the kind a destination URI selects. An empty URI is "".
func KindOf(uri string) string {
	switch {
	case uri == "":
		return ""
	case strings.HasPrefix(uri, pathScheme):
		return KindPath
	case strings.HasPrefix(uri, hostScheme):
		return KindHost
	}
	return KindRclone
}

// IsNativeKey reports whether uri is a path:// or host:// destination.
func IsNativeKey(uri string) bool {
	k := KindOf(uri)
	return k == KindPath || k == KindHost
}

// Parse returns the Destination uri names. rcloneEnv is appended to the
// environment of every rclone process (nil: none); inv is used by host://
// only and may be nil for the other kinds.
func Parse(uri string, inv *Inventory, rcloneEnv ...string) (Destination, error) {
	switch KindOf(uri) {
	case "":
		return nil, fmt.Errorf("destination is empty")
	case KindPath:
		d, err := newPathDest(strings.TrimPrefix(uri, pathScheme))
		if err != nil {
			// Explicit nil: returning d would wrap a nil *pathDest in a
			// non-nil interface.
			return nil, err
		}
		return d, nil
	case KindHost:
		d, err := newHostDest(strings.TrimPrefix(uri, hostScheme), inv)
		if err != nil {
			return nil, err
		}
		return d, nil
	}
	return &rcloneDest{remote: uri, env: rcloneEnv}, nil
}

// ParseObject parses a URI that names one object (restore-remote --from).
// For path:// and host:// the last path element is the key and the rest is
// the destination directory. Other URIs are returned whole with an empty key.
func ParseObject(uri string, inv *Inventory) (Destination, string, error) {
	if !IsNativeKey(uri) {
		d, err := Parse(uri, inv)
		return d, "", err
	}
	scheme := pathScheme
	if KindOf(uri) == KindHost {
		scheme = hostScheme
	}
	rest := strings.TrimPrefix(uri, scheme)
	// Refuse ".." before path.Dir cleans it away.
	for _, seg := range strings.Split(rest, "/") {
		if seg == ".." {
			return nil, "", fmt.Errorf("destination %q must not contain '..'", uri)
		}
	}
	dir, key := path.Dir(rest), path.Base(rest)
	if key == "." || key == "/" || key == ".." || strings.HasSuffix(rest, "/") {
		return nil, "", fmt.Errorf("destination %q does not name an object", uri)
	}
	d, err := Parse(scheme+dir, inv)
	return d, key, err
}

// validKey checks a destination key: relative, no empty, "." or ".." element,
// no NUL or backslash.
func validKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") {
		return fmt.Errorf("invalid destination key %q", key)
	}
	if strings.ContainsAny(key, "\x00\\") {
		return fmt.Errorf("invalid destination key %q", key)
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("invalid destination key %q: element %q not allowed", key, seg)
		}
	}
	return nil
}

// tmpSuffix marks an in-progress write; List hides such objects.
const tmpSuffix = ".tmp"

func userHome() string {
	h, _ := os.UserHomeDir()
	return h
}
