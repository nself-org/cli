package plugin

// installArchive binds verification and extraction to one open file, even if
// the download pathname is replaced while an install waits for external checks.
import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

type installArchive struct {
	*os.File
	digest string
	size   int64
}

func openInstallArchive(path string) (*installArchive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &installArchive{File: f, digest: hex.EncodeToString(h.Sum(nil)), size: stat.Size()}, nil
}

func (a *installArchive) checksum(expected, status string) error {
	if expected == "" {
		return verifyChecksum(a.Name(), expected, status)
	}
	if !strings.EqualFold(a.digest, expected) {
		return errs.Newf("E124", "checksum mismatch: expected %s, got %s", expected, a.digest)
	}
	return nil
}

func (a *installArchive) signature(m *PluginManifest, paid bool, kind string, bypass bool) error {
	return verifyReleaseSignatureDigest(m, paid, false, kind, func() (string, error) { return a.digest, nil }, bypass)
}

func (a *installArchive) extract(dest string) error {
	if err := extractTarGzReader(io.NewSectionReader(a.File, 0, a.size), dest); err != nil {
		return fmt.Errorf("extracting open archive: %w", err)
	}
	return nil
}
