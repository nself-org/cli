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
	"strings"
	"testing"
)

// TestArchiveSnapshotSurvivesPathReplacement pins the D-0844 / D-1326 contract:
// openArchive hashes the downloaded file once and returns a rewound handle.
// POSIX unlinks the path, allowing a replacement; Windows retains the name
// because the handle denies deletion. Windows checks handle identity; both
// variants extract the independently hashed original bytes (D-1500).
func TestArchiveSnapshotSurvivesPathReplacement(t *testing.T) {
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

	if runtime.GOOS == "windows" {
		// Go opens without FILE_SHARE_DELETE. Check that the returned handle
		// pins the named file and prevents its removal instead of truncating
		// that same file with WriteFile. Same-inode writes are outside the
		// snapshot contract; checksum and extraction below still run here.
		held, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		named, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(held, named) {
			t.Fatal("archive handle does not refer to the downloaded file")
		}
		if err := os.Remove(path); err == nil {
			t.Fatal("archive handle unexpectedly allowed path removal")
		}
	} else {
		// The path is unlinked; replacing it must not affect the handle.
		if err := os.WriteFile(path, []byte("replaced after verification"), 0600); err != nil {
			t.Fatal(err)
		}
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

// Keep the archive snapshot contract covered on every supported platform.
func TestArchiveSnapshotHasNoPlatformSkip(t *testing.T) {
	source := readFile(t, "archive_snapshot_test.go")
	body := source[strings.Index(source, "func TestArchiveSnapshotSurvivesPathReplacement("):]
	body = body[:strings.Index(body, "\n// Keep the archive snapshot")]
	if strings.Contains(body, "t.Skip(") {
		t.Fatal("archive snapshot coverage must run on every supported platform (D-1500)")
	}
}
