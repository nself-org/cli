package plugin

// installer_archive.go — one open handle for the downloaded plugin archive (D-1326).
//
// Purpose: the checksum check, the platform signature check and the extraction all read the same bytes. Each used to reopen the archive by path, so a file swapped in between passed the check on the old bytes and was extracted as the new ones.
// Inputs: the path of the downloaded archive (a 0600 temp file).
// Outputs: an open handle, the sha256 of its bytes, and a reader positioned at its start for extraction.
// Constraints: the sum is computed once from the handle and the same handle is extracted. The name is unlinked after the open on a best-effort basis, so no later open by path can reach the inode (Windows keeps the name until the handle closes). A write to the same inode by the same user is not covered.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// openArchive opens path once, hashes it and returns the handle rewound to the start together with the
// lowercase hex sha256. The caller closes the handle.
func openArchive(path string) (*os.File, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("opening archive: %w", err)
	}
	_ = os.Remove(path)
	sum, err := hashHandle(f)
	if err != nil {
		_ = f.Close()
		return nil, "", err
	}
	return f, sum, nil
}

// hashHandle returns the lowercase hex sha256 of everything in f and leaves f at the start.
func hashHandle(f *os.File) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("computing checksum: %w", err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("computing checksum: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("computing checksum: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// verifyChecksumSum is verifyChecksum for a sum that was already computed: it compares actual with
// expectedHash and applies the same missing-checksum policy. label names the file in the refusal.
func verifyChecksumSum(label, actual, expectedHash, publishStatus string) error {
	if expectedHash == "" {
		return missingChecksum(label, publishStatus)
	}
	if !strings.EqualFold(actual, expectedHash) {
		return errs.Newf("E124", "checksum mismatch: expected %s, got %s", expectedHash, actual)
	}
	return nil
}
