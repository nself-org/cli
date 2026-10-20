package plugin

// archive_pin_test.go — D-1326: the registry install opens the downloaded archive once.
//
// Purpose: prove the checksum, the signature hash and the extraction all read the one pinned handle, so swapping the archive path after the checks cannot change what is extracted.
// Inputs: two tarballs written to one temp path in turn.
// Outputs: test failures on any step that reads the path again.
// Constraints: the swap is a rename over the path, the move an attacker with the same user id can make.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedArchive_SwapAfterPinChangesNothing(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "pkg.tar.gz")
	good := binTarball(t, "good")
	if err := os.WriteFile(archivePath, good, 0o600); err != nil {
		t.Fatal(err)
	}
	pinned := pinForTest(t, archivePath)
	want := sha256Hex(good)

	// An attacker replaces the path with different bytes after the pin.
	evil := filepath.Join(dir, "evil.tar.gz")
	if err := os.WriteFile(evil, binTarball(t, "evil-different-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(evil, archivePath); err != nil {
		t.Fatal(err)
	}

	// Checksum and signature hash read the pinned bytes, twice (the handle is rewound each time).
	for i := 0; i < 2; i++ {
		if got, err := archiveSHA256(pinned); err != nil || got != want {
			t.Fatalf("pass %d: archiveSHA256 = %q, %v; want the pinned digest %q", i, got, err, want)
		}
	}
	if err := verifyChecksumFile(pinned, archivePath, want, "stable"); err != nil {
		t.Fatalf("checksum of the pinned bytes must pass after the swap: %v", err)
	}

	// Extraction reads the same pinned bytes, not the swapped file.
	reference := t.TempDir()
	if err := extractTarGz(writeCopy(t, good), reference); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := extractTarGzFile(pinned, dest); err != nil {
		t.Fatalf("extract from the pinned handle: %v", err)
	}
	if a, b := listTree(t, dest), listTree(t, reference); a != b {
		t.Fatalf("extracted the swapped archive: got %q, want %q", a, b)
	}
}

// TestInstallLocked_ReadsTheArchiveOnlyThroughThePin pins the wiring: after
// pinArchive, installLocked must not name archivePath in a reader again.
func TestInstallLocked_ReadsTheArchiveOnlyThroughThePin(t *testing.T) {
	src := readFile(t, "installer_locked.go")
	start := strings.Index(src, "func installLocked(")
	if start < 0 {
		t.Fatal("installLocked not found")
	}
	body := src[start:]
	if end := strings.Index(body[1:], "\nfunc "); end > 0 {
		body = body[:end+1]
	}
	for _, banned := range []string{"verifyChecksum(archivePath", "extractTarGz(archivePath", "archiveSHA256(archivePath", "os.Open(archivePath"} {
		if strings.Contains(body, banned) {
			t.Errorf("installLocked reopens the archive by path: %s", banned)
		}
	}
	if !strings.Contains(body, "pinArchive(archivePath)") {
		t.Error("installLocked must open the archive once with pinArchive")
	}
}

func writeCopy(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ref.tar.gz")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func listTree(t *testing.T, root string) string {
	t.Helper()
	var out []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(root, p)
			data, _ := os.ReadFile(p)
			out = append(out, rel+"="+string(data))
		}
		return nil
	})
	return strings.Join(out, "|")
}
