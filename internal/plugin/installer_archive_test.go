package plugin

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestOpenArchive_ExtractsWhatWasHashed: a file put at the archive path after the open is never
// extracted; the checked handle is (D-1326).
func TestOpenArchive_ExtractsWhatWasHashed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the archive name is only unlinked on POSIX")
	}
	good := buildTarGz(t, []struct{ name, content string }{{"plugin.json", `{"name":"good"}`}})
	evil := buildTarGz(t, []struct{ name, content string }{{"plugin.json", `{"name":"evil"}`}, {"evil.sh", "x"}})
	path := filepath.Join(t.TempDir(), "pkg.tar.gz")
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatal(err)
	}
	f, sum, err := openArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if want := sha256Hex(good); sum != want {
		t.Fatalf("sum = %s, want %s", sum, want)
	}
	// The attacker swaps the path between the checks and the extraction.
	if err := os.WriteFile(path, evil, 0o600); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := extractTarGzFrom(f, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "plugin.json"))
	if err != nil || !strings.Contains(string(got), "good") {
		t.Fatalf("extracted %q, %v: want the bytes that were hashed", got, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "evil.sh")); err == nil {
		t.Fatal("the swapped archive was extracted")
	}
}

func TestOpenArchive_Missing(t *testing.T) {
	if _, _, err := openArchive(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("a missing archive must fail")
	}
}

func TestVerifyChecksumSum(t *testing.T) {
	sum := sha256Hex([]byte("x"))
	if err := verifyChecksumSum("a", sum, strings.ToUpper(sum), "stable"); err != nil {
		t.Fatalf("matching sum (case-insensitive): %v", err)
	}
	if codeOf(verifyChecksumSum("a", sum, sha256Hex([]byte("y")), "stable")) != "E124" {
		t.Fatal("a wrong checksum must be E124")
	}
	if err := verifyChecksumSum("a", sum, "", "stable"); err != nil {
		t.Fatalf("no checksum warns and proceeds by default: %v", err)
	}
	t.Setenv(pluginRequireChecksumEnv, "1")
	if err := verifyChecksumSum("a", sum, "", "stable"); err == nil {
		t.Fatal("NSELF_PLUGIN_REQUIRE_CHECKSUM=1 must refuse a missing checksum")
	}
}

// TestInstallLocked_ReadsTheArchiveThroughOneHandle pins that the install sequence does not go back
// to the path after the single open.
func TestInstallLocked_ReadsTheArchiveThroughOneHandle(t *testing.T) {
	src := readFile(t, "installer_locked.go")
	body := src[strings.Index(src, "func installLocked("):]
	for _, bad := range []string{"verifyChecksum(archivePath", "extractTarGz(archivePath", "archiveSHA256("} {
		if strings.Contains(body, bad) {
			t.Errorf("installLocked reopens the archive by path: %s", bad)
		}
	}
	if strings.Count(body, "openArchive(") != 1 {
		t.Error("installLocked must open the archive exactly once")
	}
}
