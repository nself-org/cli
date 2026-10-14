package build

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// Purpose: reject licensed fragments whose entitlement guard cannot be written.
// Inputs: a licensed compose fixture and a Sink returning a permission error.
// Outputs: the original error and no usable compose paths.
// Constraints: plan effects avoid credential refresh; the disk stays unchanged.
// SPORT: contract:plugin.entitlement-token.
func TestLicensedPluginFragmentWriteFailure(t *testing.T) {
	compattest.Set(t, true)
	dir, original := entitlementFixture(t, "licensed")
	path := filepath.Join(dir, "fixture", pluginComposeFilename)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	sink := &fragmentWriteFailureSink{Sink: newDiskSink(dir), err: fs.ErrPermission}
	paths, err := discoverPluginComposeFilesFx(&planEffects{}, sink, dir, dir)
	if sink.path != path || !bytes.Contains(sink.data, []byte(entitlementMount)) {
		t.Fatalf("guard write not attempted: path=%q data=%s", sink.path, sink.data)
	}
	if !errors.Is(err, fs.ErrPermission) || len(paths) != 0 {
		t.Fatalf("want permission error and no compose paths, got paths=%v err=%v", paths, err)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(onDisk, original) {
		t.Fatalf("failed write changed fragment: %s, err=%v", onDisk, err)
	}
}

type fragmentWriteFailureSink struct {
	Sink
	err  error
	path string
	data []byte
}

func (s *fragmentWriteFailureSink) WriteFile(path string, data []byte, _ fs.FileMode) error {
	s.path, s.data = path, append([]byte(nil), data...)
	return s.err
}
