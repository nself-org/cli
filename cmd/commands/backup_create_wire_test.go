package commands

// Purpose: pin P7-WIRE-19 through the real cobra root: `backup create
// --encrypt` with a path:// remote leaves the encrypted object at the remote,
// and a failing configured remote exits non-zero naming the kept local file.
// Inputs: fake docker (pg_dump) and age stubs on PATH, a temp project dir.
// Outputs: test assertions only.

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// wire19Project chdirs into a temp project and puts docker/age stubs on PATH.
// It returns the local backup dir.
func wire19Project(t *testing.T, remote string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stubs are shell scripts")
	}
	bin := t.TempDir()
	for name, body := range map[string]string{
		"docker": "#!/bin/sh\nprintf 'pg-custom-dump'\n",
		"age":    "#!/bin/sh\ncp \"$5\" \"$4\"\n", // age -r R -o OUT IN
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	proj := t.TempDir()
	local := filepath.Join(proj, "backups")
	t.Setenv("PROJECT_NAME", "wire19")
	t.Setenv("POSTGRES_DB", "nself")
	t.Setenv("BACKUP_DIR", local)
	t.Setenv("BACKUP_REMOTE", remote)
	t.Setenv("BACKUP_AGE_RECIPIENTS", "age1testrecipient")
	t.Setenv("NSELF_METRICS_DIR", t.TempDir())
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	resetBackupFlagSet(t, backupCreateCmd.Flags())
	return local
}

func runWire19(t *testing.T, args ...string) (string, error) {
	t.Helper()
	buf := &bytes.Buffer{}
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs(append([]string{"backup", "create"}, args...))
	t.Cleanup(func() { RootCmd.SetArgs(nil) })
	err := RootCmd.Execute()
	return buf.String(), err
}

func TestWire19EncryptedCreateUploads(t *testing.T) {
	dest := t.TempDir()
	local := wire19Project(t, "path://"+dest)
	if out, err := runWire19(t, "--type", "full", "--encrypt"); err != nil {
		t.Fatalf("backup create --encrypt: %v\n%s", err, out)
	}
	entries, err := os.ReadDir(dest)
	if err != nil || len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".dump.age") {
		t.Fatalf("remote must hold exactly one .dump.age object, got %v (err %v)", entries, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, entries[0].Name())); string(b) != "pg-custom-dump" {
		t.Fatalf("remote object content = %q", b)
	}
	if _, err := os.Stat(filepath.Join(local, entries[0].Name())); err != nil {
		t.Fatalf("local encrypted copy missing: %v", err)
	}
}

func TestWire19FailedUploadExitsNonZero(t *testing.T) {
	// A relative path:// destination is rejected by the destination parser.
	local := wire19Project(t, "path://relative/dir")
	out, err := runWire19(t, "--type", "full", "--encrypt")
	if err == nil {
		t.Fatalf("a failed configured upload must exit non-zero; output:\n%s", out)
	}
	entries, _ := os.ReadDir(local)
	if len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".dump.age") {
		t.Fatalf("local encrypted backup must be kept, got %v", entries)
	}
	if !strings.Contains(err.Error(), filepath.Join(local, entries[0].Name())) {
		t.Fatalf("error must name the local file %s, got: %v", entries[0].Name(), err)
	}
}
