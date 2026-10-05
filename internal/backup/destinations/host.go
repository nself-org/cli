// host.go — the host:// destination kind (a controlplane inventory server).
//
// Purpose: store backups on a server we own over SSH, without rclone.
// Inputs: host://<server>/<absolute dir>; the server comes from the inventory.
// Outputs: objects under the directory on that server.
// Constraints: every ssh and scp process is started by sdk/go/remote (Run,
// Start, CopyTo); no argv is built here. Server name and paths are validated
// before any exec (remote.ValidateCopyPath allowlist, no ".."), remote paths
// are also single-quoted. Host keys are pinned: the D4 option set sets
// StrictHostKeyChecking=yes against a known_hosts file holding a key for the
// alias "nself-ci-<server>"; an unpinned server is refused before ssh runs.
package destinations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// KnownHostsEnv overrides the pinned known_hosts file host:// uses.
const KnownHostsEnv = "NSELF_BACKUP_KNOWN_HOSTS"

// KnownHostsPath is the pinned known_hosts file host:// reads.
func KnownHostsPath() string {
	if p := os.Getenv(KnownHostsEnv); p != "" {
		return p
	}
	return filepath.Join(userHome(), ".config", "nself", "backup_known_hosts")
}

type hostDest struct {
	server, dest, keyRef, dir string

	extra  []string // extra ssh options appended after the D4 set (tests: -p, -i)
	once   sync.Once
	target remote.Target
	terr   error
}

func newHostDest(rest string, inv *Inventory) (*hostDest, error) {
	name, dir, ok := strings.Cut(rest, "/")
	if !ok || dir == "" {
		return nil, fmt.Errorf("host:// needs <server>/<absolute dir> (got %q)", "host://"+rest)
	}
	dir = "/" + strings.Trim(dir, "/")
	if err := controlplane.ValidateServerName(name); err != nil {
		return nil, err
	}
	if err := remote.ValidateCopyPath(dir); err != nil {
		return nil, err
	}
	srv, err := findServer(inv, name)
	if err != nil {
		return nil, err
	}
	if err := remote.ValidateDest(srv.Host); err != nil {
		return nil, fmt.Errorf("server %q: %w", name, err)
	}
	return &hostDest{server: name, dest: srv.Host, keyRef: srv.SSHKeyRef, dir: dir}, nil
}

// findServer returns the inventory server called name. A name used by two
// environments with different hosts is ambiguous and refused.
func findServer(inv *Inventory, name string) (controlplane.Server, error) {
	var found *controlplane.Server
	if inv != nil {
		for _, env := range inv.Environments {
			for i := range env.Servers {
				s := env.Servers[i]
				if s.Name != name {
					continue
				}
				if found != nil && found.Host != s.Host {
					return s, fmt.Errorf("server %q is defined with different hosts in several environments", name)
				}
				found = &s
			}
		}
	}
	if found == nil {
		return controlplane.Server{}, fmt.Errorf("host:// server %q is not in the inventory (.nself/control-plane.yaml)", name)
	}
	if found.Host == "" {
		return *found, fmt.Errorf("host:// server %q has no SSH host", name)
	}
	return *found, nil
}

func (d *hostDest) Kind() string { return KindHost }

// tgt builds the pinned-host-key Target once.
func (d *hostDest) tgt(ctx context.Context) (remote.Target, error) {
	d.once.Do(func() {
		alias := "nself-ci-" + d.server
		keys, err := remote.PinnedHostKeys{Path: KnownHostsPath()}.Lookup(alias)
		if err != nil {
			d.terr = err
			return
		}
		if len(keys) == 0 {
			d.terr = fmt.Errorf("no pinned host key for %q in %s; pin it before using host://%s", alias, KnownHostsPath(), d.server)
			return
		}
		ver, err := remote.SSHVersion(ctx)
		if err != nil {
			d.terr = err
			return
		}
		opts := append(remote.CISSHFlags(), remote.CIOptions(d.server, KnownHostsPath(), ver)...)
		if d.keyRef != "" {
			if kp := os.Getenv(d.keyRef); kp != "" {
				opts = append(opts, "-i", kp)
			}
		}
		d.target = remote.Target{Dest: d.dest, Options: append(opts, d.extra...)}
	})
	return d.target, d.terr
}

func (d *hostDest) full(key string) (string, error) {
	if err := validKey(key); err != nil {
		return "", err
	}
	p := d.dir + "/" + key
	return p, remote.ValidateCopyPath(p)
}

func q(s string) string { return remote.ShellQuote(s) }

// Put copies localPath to <dir>/<key>: scp to <key>.tmp, sha256sum compared
// with the local hash, then chmod 600 and mv.
func (d *hostDest) Put(ctx context.Context, localPath, key string) error {
	full, err := d.full(key)
	if err != nil {
		return err
	}
	t, err := d.tgt(ctx)
	if err != nil {
		return err
	}
	sum, err := fileSHA256(localPath)
	if err != nil {
		return err
	}
	tmp := full + tmpSuffix
	prep := "umask 077 && mkdir -p -- " + q(path.Dir(full)) + " && rm -f -- " + q(tmp)
	if _, err := remote.Run(ctx, t, prep); err != nil {
		return err
	}
	if err := remote.CopyTo(ctx, t, localPath, tmp); err != nil {
		return err
	}
	got, err := remote.Run(ctx, t, "sha256sum -- "+q(tmp))
	if err == nil && !strings.HasPrefix(got, sum+" ") {
		err = fmt.Errorf("sha256 mismatch for %s on %s", key, d.server)
	}
	if err != nil {
		_, _ = remote.Run(ctx, t, "rm -f -- "+q(tmp))
		return err
	}
	_, err = remote.Run(ctx, t, "! test -d "+q(full)+" && chmod 600 "+q(tmp)+" && mv -f -- "+q(tmp)+" "+q(full))
	return err
}

// Open streams a regular, non-symlink remote file through `cat`.
func (d *hostDest) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	full, err := d.full(key)
	if err != nil {
		return nil, err
	}
	t, err := d.tgt(ctx)
	if err != nil {
		return nil, err
	}
	s, err := remote.Start(ctx, t, "test -f "+q(full)+" && ! test -L "+q(full)+" && exec cat -- "+q(full))
	if err != nil {
		return nil, err
	}
	_ = s.Stdin.Close()
	errBuf := &bytes.Buffer{}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(errBuf, io.LimitReader(s.Stderr, 4096))
		_, _ = io.Copy(io.Discard, s.Stderr)
		close(done)
	}()
	return &sessionReader{s: s, errBuf: errBuf, done: done, name: d.server + ":" + key}, nil
}

// Get downloads <dir>/<key> to localPath and checks its sha256 against the
// remote file's.
func (d *hostDest) Get(ctx context.Context, key, localPath string) error {
	full, err := d.full(key)
	if err != nil {
		return err
	}
	t, err := d.tgt(ctx)
	if err != nil {
		return err
	}
	want, err := remote.Run(ctx, t, "sha256sum -- "+q(full))
	if err != nil {
		return err
	}
	in, err := d.Open(ctx, key)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(localPath), ".get-*")
	if err != nil {
		_ = in.Close()
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmp, h), &ctxReader{ctx, in})
	if cerr := in.Close(); err == nil {
		err = cerr
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil && !strings.HasPrefix(want, hex.EncodeToString(h.Sum(nil))+" ") {
		err = fmt.Errorf("sha256 mismatch downloading %s from %s", key, d.server)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), localPath)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

// List returns the regular files under <dir> whose key starts with prefix.
func (d *hostDest) List(ctx context.Context, prefix string) ([]Object, error) {
	t, err := d.tgt(ctx)
	if err != nil {
		return nil, err
	}
	out, err := remote.Run(ctx, t, "if [ -d "+q(d.dir)+" ]; then find "+q(d.dir)+" -type f -exec stat -c '%s %Y %n' {} +; fi")
	if err != nil {
		return nil, err
	}
	var objs []Object
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(strings.TrimSpace(line), " ", 3)
		if len(f) != 3 || !strings.HasPrefix(f[2], d.dir+"/") {
			continue
		}
		key := strings.TrimPrefix(f[2], d.dir+"/")
		size, e1 := strconv.ParseInt(f[0], 10, 64)
		mod, e2 := strconv.ParseInt(f[1], 10, 64)
		if e1 != nil || e2 != nil || strings.HasSuffix(key, tmpSuffix) || !strings.HasPrefix(key, prefix) {
			continue
		}
		objs = append(objs, Object{Key: key, Size: size, ModTime: time.Unix(mod, 0).UTC()})
	}
	sortObjects(objs)
	return objs, nil
}
