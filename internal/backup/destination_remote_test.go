package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
)

func TestUploadToRemotePathDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell and unix paths")
	}
	dir := filepath.Join(t.TempDir(), "dest")
	src := filepath.Join(t.TempDir(), "p_full_20261005.dump")
	if err := os.WriteFile(src, []byte("dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := uploadToRemote(t.Context(), src, "path://"+dir, &config.Config{}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "p_full_20261005.dump")); err != nil || string(b) != "dump" {
		t.Fatalf("%q %v", b, err)
	}
	if err := uploadToRemote(t.Context(), src, "host://nosuch/srv", &config.Config{}); err == nil {
		t.Fatal("host:// with an unknown server accepted")
	}
}

func TestListRemotePathDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell and unix paths")
	}
	dir := t.TempDir()
	old := filepath.Join(dir, "p_full_1.dump.age")
	fresh := filepath.Join(dir, "p_metadata_2.tar.gz")
	for _, f := range []string{old, fresh} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	got, err := List(&config.Config{}, ListOptions{Remote: "path://" + dir})
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if got[0].Type != "metadata" || got[1].Type != "full" || !got[1].Encrypted {
		t.Fatalf("entries %+v", got)
	}
	got, _ = List(&config.Config{}, ListOptions{Remote: "path://" + dir, Since: 24 * time.Hour})
	if len(got) != 1 || got[0].Type != "metadata" {
		t.Fatalf("since filter: %+v", got)
	}
	if _, err := List(&config.Config{}, ListOptions{Remote: "path://relative"}); err == nil {
		t.Fatal("relative path:// accepted")
	}
}

// With rclone absent from PATH, restore-remote streams a path:// object into
// pg_restore (here a fake docker that records its stdin).
func TestRestoreFromRemotePathWithoutRclone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell and unix paths")
	}
	bin := t.TempDir()
	out := filepath.Join(t.TempDir(), "restored")
	script := map[string]string{
		"pg_restore": "#!/bin/sh\nexit 0\n",
		"docker":     "#!/bin/sh\n/bin/cat > '" + out + "'\n",
	}
	for n, body := range script {
		if err := os.WriteFile(filepath.Join(bin, n), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p_full_1.dump"), []byte("pg-custom-format-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ProjectName: "p"}
	if err := RestoreFromRemote(t.Context(), cfg, "path://"+dir+"/p_full_1.dump", ""); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if b, _ := os.ReadFile(out); string(b) != "pg-custom-format-bytes" {
		t.Fatalf("restored %q", b)
	}
	if err := RestoreFromRemote(t.Context(), cfg, "path://"+dir+"/missing.dump", ""); err == nil {
		t.Fatal("missing object restored")
	}
}
