// stream_host.go — Streamer and Remover for the host:// destination kind.
//
// Purpose: write an object to a server we own from a reader (backup stream,
// heartbeat) and delete a failed one, without a local copy.
// Inputs: a reader and a relative key; the pinned-host-key Target of hostDest.
// Outputs: <dir>/<key> on the server, mode 600, written via <key>.tmp.
// Constraints: every process is started by sdk/go/remote (Start, Run); remote
// paths are validated and single-quoted. The object appears only after its
// sha256 matched the bytes read from the reader (same tail as Put).
package destinations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// PutStream pipes r into `cat > <key>.tmp` over ssh, hashing the bytes on the
// way, then verifies the remote sha256 and moves the file into place.
func (d *hostDest) PutStream(ctx context.Context, r io.Reader, key string) error {
	full, err := d.full(key)
	if err != nil {
		return err
	}
	t, err := d.tgt(ctx)
	if err != nil {
		return err
	}
	tmp := full + tmpSuffix
	cmd := "umask 077 && mkdir -p -- " + q(path.Dir(full)) + " && rm -f -- " + q(tmp) + " && exec cat > " + q(tmp)
	s, err := remote.Start(ctx, t, cmd)
	if err != nil {
		return err
	}
	_ = s.Stdout.Close()
	errBuf := &bytes.Buffer{}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(errBuf, io.LimitReader(s.Stderr, 4096))
		_, _ = io.Copy(io.Discard, s.Stderr)
		close(done)
	}()
	h := sha256.New()
	_, cerr := io.Copy(s.Stdin, io.TeeReader(&ctxReader{ctx, r}, h))
	if xerr := s.Stdin.Close(); cerr == nil {
		cerr = xerr
	}
	if cerr != nil {
		_ = s.Kill()
	}
	<-done
	werr := s.Wait()
	if cerr == nil && werr != nil {
		cerr = fmt.Errorf("write %s on %s: %s: %w", key, d.server, errBuf.String(), werr)
	}
	if cerr != nil {
		// The cleanup must run even when ctx is the reason for the failure.
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer cancel()
		_, _ = remote.Run(cctx, t, "rm -f -- "+q(tmp))
		return cerr
	}
	return d.verifyAndMove(ctx, t, tmp, full, key, hex.EncodeToString(h.Sum(nil)))
}

// Remove deletes <dir>/<key> and a leftover <key>.tmp; missing is not an
// error (rm -f).
func (d *hostDest) Remove(ctx context.Context, key string) error {
	full, err := d.full(key)
	if err != nil {
		return err
	}
	t, err := d.tgt(ctx)
	if err != nil {
		return err
	}
	_, err = remote.Run(ctx, t, "rm -f -- "+q(full)+" "+q(full+tmpSuffix))
	return err
}
