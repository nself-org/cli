package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestArchiveSnapshotSurvivesPathReplacement pins the D-0844 / D-1326 contract:
// openArchive hashes the downloaded file once, unlinks the path, and returns
// a handle rewound to the start. Checksum verification and extraction both
// read that handle, so replacing the path after the open cannot change what
// gets verified or extracted.
func TestArchiveSnapshotSurvivesPathReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Go opens files without FILE_SHARE_DELETE, so the path of an open
		// archive cannot be unlinked there and the replacement below would
		// rewrite the very file the handle reads. The contract is POSIX-only.
		t.Skip("an open file cannot be unlinked on Windows")
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "payload", Mode: 0600, Size: 7})
	_, _ = tw.Write([]byte("trusted"))
	_ = tw.Close()
	_ = gz.Close()
	// The gzip stream differs between Go releases, so derive the expected hash
	// from the bytes written to disk rather than from a literal.
	original := sha256.Sum256(archive.Bytes())
	want := hex.EncodeToString(original[:])
	path := filepath.Join(t.TempDir(), "archive.tgz")
	if err := os.WriteFile(path, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	f, sum, err := openArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	// The path is unlinked by openArchive; replacing it now must not affect
	// what we verify or extract.
	if err := os.WriteFile(path, []byte("replaced after verification"), 0600); err != nil {
		t.Fatal(err)
	}

	// want is hashed from the original archive bytes, so this is not a
	// comparison of sum with itself.
	if err := verifyChecksumSum(path, sum, want, "stable"); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if err := extractTarGzFrom(f, dest); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dest, "payload"))
	if string(got) != "trusted" {
		t.Fatalf("extracted %q", got)
	}
}
