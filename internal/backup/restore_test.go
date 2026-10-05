package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
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

func errCode(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func ageEncrypt(t *testing.T, keyFile, plain, out string) {
	t.Helper()
	pub, err := exec.Command("age-keygen", "-y", keyFile).Output()
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "p")
	_ = os.WriteFile(src, []byte(plain), 0o600)
	if o, err := exec.Command("age", "-r", strings.TrimSpace(string(pub)), "-o", out, src).CombinedOutput(); err != nil {
		t.Fatalf("age: %v %s", err, o)
	}
}

func newAgeKey(t *testing.T, p string) {
	t.Helper()
	if o, err := exec.Command("age-keygen", "-o", p).CombinedOutput(); err != nil {
		t.Fatalf("age-keygen: %v %s", err, o)
	}
}

// Opus review: in v1.4 restore-remote still uses age-key.txt only, even when
// an unrelated <project>-age.key exists; v1.5 uses the shared search.
func TestAutoKeyV14RestoreRemoteKeepsLegacyDefault(t *testing.T) {
	home, _, _ := autoKeyEnv(t)
	for _, b := range []string{"age", "rclone"} {
		if _, err := exec.LookPath(b); err != nil {
			t.Skipf("%s missing", b)
		}
	}
	dir := filepath.Join(home, ".config", "nself")
	_ = os.MkdirAll(dir, 0o700)
	newAgeKey(t, filepath.Join(dir, "age-key.txt"))
	newAgeKey(t, filepath.Join(dir, "proj-age.key"))
	out := filepath.Join(t.TempDir(), "restored")
	fakePgDocker(t, out)
	stub := t.TempDir()
	_ = os.WriteFile(filepath.Join(stub, "pg_restore"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RCLONE_CONFIG", filepath.Join(t.TempDir(), "none.conf"))
	t.Setenv("RCLONE_CONFIG_NSELFTEST_TYPE", "local")
	dest := t.TempDir()
	obj := filepath.Join(dest, "x.dump.age")
	ageEncrypt(t, filepath.Join(dir, "age-key.txt"), "PGDMP-opus", obj)
	cfg := streamTestConfig()
	cfg.ProjectName = "proj"
	compattest.Both(t, func(t *testing.T) {
		_ = os.Remove(out)
		err := RestoreFromRemote(context.Background(), cfg, "nselftest:"+obj, "")
		b, _ := os.ReadFile(out)
		if os.Getenv("NSELF_V15") != "1" {
			if err != nil || string(b) != "PGDMP-opus" {
				t.Fatalf("v1.4 must keep age-key.txt: %v %q", err, b)
			}
			return
		}
		if err == nil && string(b) == "PGDMP-opus" {
			t.Fatal("v1.5 should have preferred proj-age.key (a different key)")
		}
	})
}

func TestAutoKeySymlinkedConfigDirRefused(t *testing.T) {
	home, _, _ := autoKeyEnv(t)
	elsewhere := filepath.Join(home, "elsewhere")
	_ = os.MkdirAll(elsewhere, 0o755)
	_ = os.MkdirAll(filepath.Join(home, ".config"), 0o755)
	if err := os.Symlink(elsewhere, filepath.Join(home, ".config", "nself")); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureIdentity("proj"); errCode(err) != "E222" {
		t.Fatalf("want E222, got %v", err)
	}
	if ents, _ := os.ReadDir(elsewhere); len(ents) != 0 {
		t.Fatalf("secret material was written through the symlink: %v", ents)
	}
	if fi, _ := os.Stat(elsewhere); fi.Mode().Perm() != 0o755 {
		t.Error("the symlink target was chmodded")
	}
}

func TestAutoKeyDefaultLookupValidatesIdentities(t *testing.T) {
	home, _, _ := autoKeyEnv(t)
	dir := filepath.Join(home, ".config", "nself")
	_ = os.MkdirAll(dir, 0o700)
	real := filepath.Join(home, "real.key")
	newAgeKey(t, real)
	link := filepath.Join(dir, "proj-age.key")
	_ = os.Symlink(real, link)
	if _, err := DefaultIdentity("proj", "--key"); errCode(err) != "E222" || !strings.Contains(err.Error(), link) {
		t.Fatalf("symlinked default key: want E222 naming %s, got %v", link, err)
	}
	_ = os.Remove(link)
	_ = os.WriteFile(link, []byte("not an age identity\n"), 0o600)
	if _, err := DefaultIdentity("proj", "--key"); errCode(err) != "E222" || !strings.Contains(err.Error(), link) {
		t.Fatalf("garbage default key: want E222 naming %s, got %v", link, err)
	}
	// an explicit path is the user's choice and is used as given
	if got, err := resolveIdentity("proj", link); err != nil || got != link {
		t.Fatalf("explicit key: %q %v", got, err)
	}
}

func TestAutoKeyMissingAgeKeygenNeverCallsGoodKeyUnusable(t *testing.T) {
	autoKeyEnv(t)
	id, err := EnsureIdentity("proj")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(id.Path)
	t.Setenv("PATH", t.TempDir())
	for name, call := range map[string]func() error{
		"ensure":  func() error { _, e := EnsureIdentity("proj"); return e },
		"default": func() error { _, e := DefaultIdentity("proj", "--key"); return e },
	} {
		err := call()
		if errCode(err) != "E222" || !strings.Contains(err.Error(), "age-keygen is not installed") || strings.Contains(err.Error(), "not a usable") {
			t.Errorf("%s: want the missing-tool error, got %v", name, err)
		}
	}
	if after, _ := os.ReadFile(id.Path); !bytes.Equal(before, after) {
		t.Error("key changed")
	}
}

func TestAutoKeyHardLinkedKeyRefusedNotChmodded(t *testing.T) {
	home, _, _ := autoKeyEnv(t)
	id, _ := EnsureIdentity("proj")
	other := filepath.Join(home, "other")
	if err := os.Link(id.Path, other); err != nil {
		t.Skip("hard links unsupported")
	}
	if err := os.Chmod(other, 0o644); err != nil { // chmod via either name changes the shared inode
		t.Fatal(err)
	}
	if _, err := EnsureIdentity("proj"); errCode(err) != "E222" || !strings.Contains(err.Error(), "hard link") {
		t.Fatalf("want E222 hard link refusal, got %v", err)
	}
	if mode(t, other) != 0o644 {
		t.Error("the shared inode was chmodded")
	}
}

func TestAutoKeyE223NamesTheCommandsFlag(t *testing.T) {
	autoKeyEnv(t)
	compattest.Set(t, true)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "p_full_1.dump.age"), []byte("x"), 0o600)
	cfg := localBackupCfg(dir)
	cases := map[string]error{
		"--decrypt-key": Restore(context.Background(), cfg, RestoreOptions{BackupID: "p_full_1.dump.age"}),
		"--key":         RestoreFromRemote(context.Background(), cfg, "path://"+dir+"/p_full_1.dump.age", ""),
	}
	_, cases["--identity"] = resolveIdentity("proj", "")
	for flag, err := range cases {
		if errCode(err) != "E223" || !strings.Contains(err.Error(), "pass "+flag+" <file>") {
			t.Errorf("%s: %v", flag, err)
		}
	}
}

// The plaintext dump is 0600 while it exists and is removed on every path.
func TestAutoKeyDecryptedDumpIs0600AndRemovedOnFailure(t *testing.T) {
	home, _, _ := autoKeyEnv(t)
	key := filepath.Join(home, "k")
	newAgeKey(t, key)
	dir := t.TempDir()
	obj := filepath.Join(dir, "p_full_1.dump.age")
	ageEncrypt(t, key, "PGDMP-x", obj)
	dec, err := decryptFile(context.Background(), obj, key, "proj")
	if err != nil {
		t.Fatal(err)
	}
	if mode(t, dec) != 0o600 {
		t.Errorf("decrypted dump mode %04o", mode(t, dec))
	}
	_ = os.Remove(dec)
	other := filepath.Join(home, "other")
	newAgeKey(t, other)
	if _, err := decryptFile(context.Background(), obj, other, "proj"); err == nil {
		t.Fatal("wrong key decrypted")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.dec")); len(left) != 0 {
		t.Errorf("failed decrypt left %v", left)
	}
	// restore failing after the decrypt still removes the dump
	bin := t.TempDir()
	_ = os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\necho FATAL boom >&2\nexit 1\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cfg := localBackupCfg(dir)
	if err := Restore(context.Background(), cfg, RestoreOptions{BackupID: "p_full_1.dump.age", DecryptKey: key, Only: []string{"pg"}}); err == nil {
		t.Fatal("restore should fail")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.dec")); len(left) != 0 {
		t.Errorf("failed restore left %v", left)
	}
}

// An existing <backup>.dec is somebody's file: a failed decrypt must not touch it.
func TestAutoKeyExistingDecFileSurvives(t *testing.T) {
	home, _, _ := autoKeyEnv(t)
	key, wrong := filepath.Join(home, "k"), filepath.Join(home, "w")
	newAgeKey(t, key)
	newAgeKey(t, wrong)
	dir := t.TempDir()
	obj := filepath.Join(dir, "p_full_1.dump.age")
	ageEncrypt(t, key, "PGDMP-x", obj)
	mine := filepath.Join(dir, "p_full_1.dump.dec")
	if err := os.WriteFile(mine, []byte("precious"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := decryptFile(context.Background(), obj, wrong, "proj"); err == nil {
		t.Fatal("wrong key decrypted")
	}
	if b, _ := os.ReadFile(mine); string(b) != "precious" || mode(t, mine) != 0o640 {
		t.Fatal("the pre-existing .dec file was modified or removed after a failed decrypt")
	}
	dec, err := decryptFile(context.Background(), obj, key, "proj")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(dec) }()
	if dec == mine || !strings.HasSuffix(strings.TrimSuffix(dec, ".dec"), ".dump") {
		t.Fatalf("temp name %s", dec)
	}
	if b, _ := os.ReadFile(mine); string(b) != "precious" {
		t.Fatal("a successful decrypt overwrote the pre-existing .dec file")
	}
}
