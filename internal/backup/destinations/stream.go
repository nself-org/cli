// stream.go — the Streamer and Remover interfaces and their rclone and path
// implementations.
//
// Purpose: let backup stream and the heartbeat write an object from a reader
// (no local copy) and delete a failed one, through the same destination
// grammar create and restore use.
// Inputs: a reader and a relative key; for HasInlineCredentials, a destination
// URI.
// Outputs: an object under the destination; errors that never echo a URI.
// Constraints: rclone argv is the old rcat/deletefile argv, byte for byte; the
// remote string is used verbatim. HasInlineCredentials only reports, it never
// returns the value it inspected.
package destinations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"
)

// Streamer writes an object from a reader without a local copy (backup
// stream, heartbeat). The object appears only when the whole reader was
// stored; a failed write leaves no object under key.
type Streamer interface {
	PutStream(ctx context.Context, r io.Reader, key string) error
}

// Remover deletes one object. An object that is not there counts as removed.
type Remover interface {
	Remove(ctx context.Context, key string) error
}

// HasInlineCredentials reports whether uri carries rclone options or
// credentials in the remote itself: a comma in the remote-name segment
// (`:s3,access_key_id=X:bucket`, `name,secret_access_key=Y:path`, quoted
// values included), userinfo in a scheme URI (`s3://KEY:SECRET@bucket/p`), or
// the same shape in the bare form (`s3:KEY:SECRET@bucket`). Such a value would
// be written to logs and unit files. path:// and host:// never carry any.
//
// For scheme URIs the bucket segment (text after "://" up to the first "/")
// is checked, not the authority: a secret may itself contain "/", which would
// hide the "@". No bucket or container name can hold "@" or ":".
func HasInlineCredentials(uri string) bool {
	if IsNativeKey(uri) {
		return false
	}
	rest := strings.TrimPrefix(uri, ":")
	name, path, _ := strings.Cut(rest, ":")
	if strings.Contains(name, ",") {
		return true
	}
	if IsRemoteKey(uri) {
		_, after, _ := strings.Cut(uri, "://")
		bucket, _, _ := strings.Cut(after, "/")
		return strings.ContainsAny(bucket, "@:")
	}
	// Bare form name:path. A ":" followed later by "@" in the path part is a
	// KEY:SECRET@host shape (also reached by odd-cased or padded schemes).
	if i := strings.Index(path, ":"); i >= 0 {
		return strings.Contains(path[i:], "@")
	}
	return false
}

// rcloneJoin is <remote>[/]<key>: "/" is added only when the remote does not
// already end in one. The remote is used verbatim, as the pre-interface
// helpers did.
func (d *rcloneDest) rcloneJoin(key string) string {
	remote := d.remote
	if !strings.HasSuffix(remote, "/") {
		remote += "/"
	}
	return remote + key
}

// PutStream runs `rclone rcat <remote>[/]<key>` with r on stdin. rclone does
// multipart uploads itself. A failure carries rclone's trimmed stderr.
func (d *rcloneDest) PutStream(ctx context.Context, r io.Reader, key string) error {
	if err := validKey(key); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "rclone", "rcat", d.rcloneJoin(key))
	cmd.Env = append(os.Environ(), d.env...)
	cmd.Stdin = r
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s: %w", msg, err)
		}
		return fmt.Errorf("rclone rcat: %w", err)
	}
	return nil
}

// Remove runs `rclone deletefile <remote>[/]<key>`. Exit code 3 or 4 (object
// or directory not found) counts as removed.
func (d *rcloneDest) Remove(ctx context.Context, key string) error {
	if err := validKey(key); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "rclone", "deletefile", d.rcloneJoin(key))
	cmd.Env = append(os.Environ(), d.env...)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) && (ee.ExitCode() == 3 || ee.ExitCode() == 4) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("rclone deletefile: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// PutStream writes r to <dir>/<key> with the guarantees of Put.
func (d *pathDest) PutStream(ctx context.Context, r io.Reader, key string) error {
	return d.putReader(ctx, r, key)
}

// Remove deletes <dir>/<key> and a leftover <key>.tmp. A missing object is not
// an error. A symlink on the way is refused, never followed.
func (d *pathDest) Remove(_ context.Context, key string) error {
	if err := validKey(key); err != nil {
		return err
	}
	r, err := d.root(false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if err := noSymlinks(r, key); err != nil {
		return err
	}
	for _, k := range []string{key, key + tmpSuffix} {
		if err := r.Remove(k); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
