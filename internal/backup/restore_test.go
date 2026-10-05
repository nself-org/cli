package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// fakeDocker stubs `docker exec ... pg_dump` (prints fixture bytes) and
// `docker exec -i ... pg_restore` (copies stdin to out).
func fakePgDocker(t *testing.T, out string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$*\" in\n*pg_restore*) /bin/cat > '" + out + "';;\n*pg_dump*) printf 'PGDMP-local-fixture';;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func localBackupCfg(dir string) *config.Config {
	cfg := streamTestConfig()
	cfg.ProjectName = "proj"
	cfg.Backup.Dir = dir
	cfg.Backup.Encryption = true
	return cfg
}

// v1.5: create auto-keys a local backup, and plain restore (no key flag) finds
// the identity and returns the same bytes. v1.4: refuses as before and
// restore keeps the age-key.txt default.
func TestAutoKeyLocalCreateThenRestoreNoKeyFlag(t *testing.T) {
	home, _, _ := autoKeyEnv(t)
	t.Setenv("PROJECT_NAME", "proj")
	t.Setenv("POSTGRES_DB", "db")
	out := filepath.Join(t.TempDir(), "restored")
	fakePgDocker(t, out)
	compattest.Both(t, func(t *testing.T) {
		_ = os.RemoveAll(filepath.Join(home, ".config"))
		_ = os.Remove(out)
		dir := t.TempDir()
		cfg := localBackupCfg(dir)
		err := Create(context.Background(), cfg, CreateOptions{Type: BackupTypeFull})
		if os.Getenv("NSELF_V15") != "1" {
			if err == nil || !strings.Contains(err.Error(), "encryption requested but no recipient configured") {
				t.Fatalf("v1.4 create must refuse as before, got %v", err)
			}
			// restore keeps the legacy default: age-key.txt, no E223.
			enc := filepath.Join(dir, "p_full_1.dump.age")
			_ = os.WriteFile(enc, []byte("x"), 0o600)
			err = Restore(context.Background(), cfg, RestoreOptions{BackupID: "p_full_1.dump.age"})
			var ce *errs.CLIError
			if errors.As(err, &ce) {
				t.Fatalf("v1.4 restore must not use the shared search: %v", err)
			}
			return
		}
		if err != nil {
			t.Fatalf("v1.5 create: %v", err)
		}
		files, _ := os.ReadDir(dir)
		var obj string
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".dump.age") {
				obj = f.Name()
			}
		}
		if obj == "" {
			t.Fatalf("no encrypted dump in %v", files)
		}
		if raw, _ := os.ReadFile(filepath.Join(dir, obj)); strings.Contains(string(raw), "PGDMP") {
			t.Fatal("local backup is not encrypted")
		}
		if err := Restore(context.Background(), cfg, RestoreOptions{BackupID: obj, Only: []string{"pg"}}); err != nil {
			t.Fatalf("restore with no key flag: %v", err)
		}
		if b, _ := os.ReadFile(out); string(b) != "PGDMP-local-fixture" {
			t.Fatalf("restored %q", b)
		}
		if left, _ := filepath.Glob(filepath.Join(dir, "*.dec")); len(left) != 0 {
			t.Errorf("decrypted copy left behind: %v", left)
		}
	})
}

// v1.5 restore with no identity anywhere is E223, not an age error.
func TestAutoKeyRestoreWithoutIdentityIsE223(t *testing.T) {
	autoKeyEnv(t)
	compattest.Set(t, true)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "p_full_1.dump.age"), []byte("x"), 0o600)
	err := Restore(context.Background(), localBackupCfg(dir), RestoreOptions{BackupID: "p_full_1.dump.age"})
	if err == nil || !strings.Contains(err.Error(), "[E223]") {
		t.Fatalf("want E223, got %v", err)
	}
}
