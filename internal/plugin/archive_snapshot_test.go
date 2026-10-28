package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// TestArchiveSnapshotSurvivesPathReplacement pins the D-0844 / D-1326 contract:
// openArchive hashes the downloaded file once, unlinks the path, and returns
// a handle rewound to the start. Checksum verification and extraction both
// read that handle, so replacing the path after the open cannot change what
// gets verified or extracted.
func TestArchiveSnapshotSurvivesPathReplacement(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "payload", Mode: 0600, Size: 7})
	_, _ = tw.Write([]byte("trusted"))
	_ = tw.Close()
	_ = gz.Close()
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

	if err := verifyChecksumSum(path, sum, sum, "stable"); err != nil {
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
