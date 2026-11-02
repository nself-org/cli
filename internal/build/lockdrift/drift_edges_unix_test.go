//go:build unix

package lockdrift

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/bundle/ledger"
)

// Reading a regular file as a directory fails on unix; Windows reports no error for it, so this
// error path is exercised here only.
func TestSourcesDirectoryReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "regular-file")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := Sources(path)()
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || pathErr.Path != path {
		t.Fatalf("Sources error = %v, want directory read error for %s", err, path)
	}
	if !reflect.DeepEqual(src, ledger.Sources{}) {
		t.Fatalf("failed Sources returned partial data: %#v", src)
	}
}
