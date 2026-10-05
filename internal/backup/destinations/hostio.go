// hostio.go — I/O helpers for the host:// destination.
//
// Purpose: stream a remote file through an ssh session and hash local files.
// Inputs: a started remote.Session; a local path.
// Outputs: an io.ReadCloser that reports the remote exit status on Close.
// Constraints: no process is started here; sdk/go/remote owns every exec.
package destinations

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

type sessionReader struct {
	s      *remote.Session
	errBuf *bytes.Buffer
	done   chan struct{}
	name   string
	eof    bool
}

func (r *sessionReader) Read(p []byte) (int, error) {
	n, err := r.s.Stdout.Read(p)
	if err == io.EOF {
		r.eof = true
	}
	return n, err
}

// Close reports a failed remote command. A reader closed before EOF kills the
// session and reports no error.
func (r *sessionReader) Close() error {
	if !r.eof {
		_ = r.s.Kill()
	}
	<-r.done
	err := r.s.Wait()
	if err != nil && r.eof {
		return fmt.Errorf("read %s: %s: %w", r.name, strings.TrimSpace(r.errBuf.String()), err)
	}
	return nil
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
