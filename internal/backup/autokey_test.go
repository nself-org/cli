package backup

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

const secretMarker = "AGE-SECRET-KEY-"

// autoKeyEnv gives the test its own HOME and a captured notice and log.
func autoKeyEnv(t *testing.T) (home string, notice, logs *bytes.Buffer) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs unix file modes")
	}
	if _, err := exec.LookPath("age-keygen"); err != nil {
		t.Skip("age-keygen not on PATH")
	}
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(NoAutoKeyEnv, "")
	notice, logs = &bytes.Buffer{}, &bytes.Buffer{}
	oldN, oldL := autoKeyNotice, slog.Default()
	autoKeyNotice = notice
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { autoKeyNotice = oldN; slog.SetDefault(oldL) })
	return home, notice, logs
}

func mode(t *testing.T, p string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestAutoKeyCreatesIdentityWithModes(t *testing.T) {
	home, notice, logs := autoKeyEnv(t)
	id, err := EnsureIdentity("proj")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "nself", "proj-age.key")
	if id.Path != want || !id.Created || !strings.HasPrefix(id.Recipient, "age1") {
		t.Fatalf("identity %+v", id)
	}
	if m := mode(t, id.Path); m != 0o600 {
		t.Errorf("key mode %04o", m)
	}
	if m := mode(t, filepath.Dir(id.Path)); m != 0o700 {
		t.Errorf("dir mode %04o", m)
	}
	body, _ := os.ReadFile(id.Path)
	if !bytes.Contains(body, []byte(secretMarker)) {
		t.Fatal("identity file holds no secret key")
	}
	if strings.Contains(notice.String()+logs.String()+id.Recipient+id.Path, secretMarker) {
		t.Error("secret material leaked into output or logs")
	}
}

func TestAutoKeyExistingIdentityUntouched(t *testing.T) {
	home, _, _ := autoKeyEnv(t)
	first, err := EnsureIdentity("proj")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(first.Path)
	fiBefore, _ := os.Stat(first.Path)
	second, err := EnsureIdentity("proj")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(first.Path)
	fiAfter, _ := os.Stat(first.Path)
	if second.Created || second.Recipient != first.Recipient || !bytes.Equal(before, after) || !os.SameFile(fiBefore, fiAfter) {
		t.Fatalf("existing identity was replaced: %+v vs %+v", first, second)
	}
	// A garbage file at the path is refused and left byte-identical.
	bad := filepath.Join(home, ".config", "nself", "bad-age.key")
	if err := os.WriteFile(bad, []byte("not an age key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = EnsureIdentity("bad")
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E222" {
		t.Fatalf("garbage identity: want E222, got %v", err)
	}
	if b, _ := os.ReadFile(bad); string(b) != "not an age key\n" {
		t.Error("garbage identity was modified")
	}
}

func TestAutoKeyWrongPermsAreTightened(t *testing.T) {
	home, _, logs := autoKeyEnv(t)
	id, _ := EnsureIdentity("proj")
	dir := filepath.Dir(id.Path)
	if err := os.Chmod(id.Path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(id.Path)
	if _, err := EnsureIdentity("proj"); err != nil {
		t.Fatal(err)
	}
	if mode(t, id.Path) != 0o600 || mode(t, dir) != 0o700 {
		t.Errorf("modes not tightened: %04o %04o", mode(t, id.Path), mode(t, dir))
	}
	if after, _ := os.ReadFile(id.Path); !bytes.Equal(before, after) {
		t.Error("tightening changed the key")
	}
	if !strings.Contains(logs.String(), "tightening") {
		t.Error("tightening was silent")
	}
	// A symlink at the identity path is refused; its target is untouched.
	victim := filepath.Join(home, "victim")
	if err := os.WriteFile(victim, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dir, "link-age.key")); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureIdentity("link"); err == nil {
		t.Fatal("symlinked identity accepted")
	}
	if b, _ := os.ReadFile(victim); string(b) != "v" || mode(t, victim) != 0o644 {
		t.Error("symlink target modified")
	}
}

func TestAutoKeyRejectsPathyProjectNames(t *testing.T) {
	autoKeyEnv(t)
	for _, p := range []string{"", "..", "a/b", "../x", ".hidden"} {
		if _, err := EnsureIdentity(p); err == nil {
			t.Errorf("project %q accepted", p)
		}
	}
}

func TestAutoKeyConcurrentFirstRunsYieldOneKey(t *testing.T) {
	_, notice, _ := autoKeyEnv(t)
	const n = 12
	var wg sync.WaitGroup
	ids := make([]Identity, n)
	errsOut := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ids[i], errsOut[i] = EnsureIdentity("race")
		}(i)
	}
	close(start)
	wg.Wait()
	created := 0
	for i := range ids {
		if errsOut[i] != nil {
			t.Fatalf("run %d: %v", i, errsOut[i])
		}
		if ids[i].Recipient != ids[0].Recipient {
			t.Fatalf("run %d used a different recipient", i)
		}
		if ids[i].Created {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("%d runs claim to have created the key, want 1", created)
	}
	entries, _ := os.ReadDir(filepath.Dir(ids[0].Path))
	if len(entries) != 1 || entries[0].Name() != "race-age.key" {
		t.Fatalf("stray files after the race: %v", entries)
	}
	if want, _ := exec.Command("age-keygen", "-y", ids[0].Path).Output(); strings.TrimSpace(string(want)) != ids[0].Recipient {
		t.Error("the surviving key does not match the shared recipient")
	}
	if notice.Len() != 0 {
		t.Error("EnsureIdentity must not print; only the caller's notice does")
	}
}

func TestAutoKeyNoticeIsHonestAndSecretFree(t *testing.T) {
	_, notice, _ := autoKeyEnv(t)
	compattest.Set(t, true)
	rec, err := resolveAutoRecipients("proj", nil, false, false)
	if err != nil || len(rec) != 1 {
		t.Fatalf("%v %v", rec, err)
	}
	out := notice.String()
	for _, want := range []string{"proj-age.key", "unrecoverable", "off this machine", rec[0], BackedUpMarkerSuffix} {
		if !strings.Contains(out, want) {
			t.Errorf("notice lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, secretMarker) {
		t.Error("notice contains the secret")
	}
	notice.Reset()
	if _, err := resolveAutoRecipients("proj", nil, false, false); err != nil || notice.Len() != 0 {
		t.Errorf("second run must be silent: %v %q", err, notice.String())
	}
}

func TestAutoKeyModeAndSwitches(t *testing.T) {
	home, notice, _ := autoKeyEnv(t)
	keyFile := filepath.Join(home, ".config", "nself", "proj-age.key")
	cfg := streamTestConfig()
	cfg.ProjectName = "proj"
	compattest.Both(t, func(t *testing.T) {
		_ = os.RemoveAll(filepath.Join(home, ".config"))
		notice.Reset()
		v15 := os.Getenv("NSELF_V15") == "1"
		// no recipient, no switches
		_, err := Stream(context.Background(), cfg, StreamOptions{To: "s3:b/p", DryRun: true})
		if v15 {
			if err != nil {
				t.Fatalf("v1.5 dry-run: %v", err)
			}
		} else if err == nil || !strings.Contains(err.Error(), "refusing to stream an unencrypted backup") {
			t.Fatalf("v1.4 must refuse as before, got %v", err)
		}
		if _, e := os.Stat(keyFile); e == nil {
			t.Fatal("dry-run or v1.4 created an identity")
		}
		// NSELF_BACKUP_NO_AUTO_KEY=1 refuses in both modes; E224 in v1.5.
		t.Setenv(NoAutoKeyEnv, "1")
		_, err = Stream(context.Background(), cfg, StreamOptions{To: "s3:b/p", DryRun: true})
		if err == nil {
			t.Fatal("NO_AUTO_KEY=1 must refuse")
		}
		if v15 && !strings.Contains(err.Error(), "[E224]") {
			t.Errorf("v1.5 refusal lacks E224: %v", err)
		}
		if _, e := os.Stat(keyFile); e == nil {
			t.Fatal("NO_AUTO_KEY=1 created an identity")
		}
		// --no-encrypt keeps its semantics and a configured recipient wins.
		if _, err = Stream(context.Background(), cfg, StreamOptions{To: "s3:b/p", DryRun: true, AllowUnencrypted: true}); err != nil {
			t.Fatalf("--no-encrypt: %v", err)
		}
		t.Setenv(NoAutoKeyEnv, "")
		cfg.Backup.AgeRecipients = "age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqs3290gq"
		if _, err = Stream(context.Background(), cfg, StreamOptions{To: "s3:b/p", DryRun: true}); err != nil {
			t.Fatalf("configured recipient: %v", err)
		}
		cfg.Backup.AgeRecipients = ""
		if _, e := os.Stat(keyFile); e == nil {
			t.Fatal("an identity appeared although none was needed")
		}
	})
}

func TestAutoKeyConfiguredRecipientWinsAndCreatesNothing(t *testing.T) {
	home, notice, _ := autoKeyEnv(t)
	compattest.Set(t, true)
	got, err := resolveAutoRecipients("proj", []string{"age1configured"}, false, false)
	if err != nil || len(got) != 1 || got[0] != "age1configured" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config")); err == nil || notice.Len() != 0 {
		t.Error("a configured recipient must create no file and print nothing")
	}
	t.Setenv(NoAutoKeyEnv, "1")
	if got, err := resolveAutoRecipients("proj", []string{"age1configured"}, false, false); err != nil || len(got) != 1 {
		t.Errorf("a configured recipient must pass even with NO_AUTO_KEY=1: %v %v", got, err)
	}
}

func TestAutoKeyCreateFillsRecipientWithoutMutatingConfig(t *testing.T) {
	autoKeyEnv(t)
	cfg := streamTestConfig()
	cfg.ProjectName = "proj"
	cfg.Backup.Encryption = true
	compattest.Set(t, false)
	if got, err := withAutoRecipient(cfg, CreateOptions{}); err != nil || got.Backup.AgeRecipients != "" {
		t.Fatalf("v1.4 must not fill a recipient: %v %v", got.Backup.AgeRecipients, err)
	}
	compattest.Set(t, true)
	got, err := withAutoRecipient(cfg, CreateOptions{})
	if err != nil || !strings.HasPrefix(got.Backup.AgeRecipients, "age1") {
		t.Fatalf("v1.5 recipient: %v %v", got.Backup.AgeRecipients, err)
	}
	if cfg.Backup.AgeRecipients != "" {
		t.Error("caller config was mutated")
	}
	if got, _ := withAutoRecipient(cfg, CreateOptions{NoEncrypt: true}); got.Backup.AgeRecipients != "" {
		t.Error("--no-encrypt must not create a key")
	}
	t.Setenv(NoAutoKeyEnv, "1")
	if _, err := withAutoRecipient(cfg, CreateOptions{}); err == nil || !strings.Contains(err.Error(), "[E224]") {
		t.Errorf("create with NO_AUTO_KEY=1: %v", err)
	}
}

// End to end with local fixtures: fake pg_dump, real age and rclone, a local
// rclone destination. First run creates the identity; the object restores
// with it.
func TestAutoKeyStreamRoundTrip(t *testing.T) {
	home, notice, logs := autoKeyEnv(t)
	for _, b := range []string{"age", "rclone"} {
		if _, err := exec.LookPath(b); err != nil {
			t.Skipf("%s not on PATH", b)
		}
	}
	compattest.Set(t, true)
	bin := t.TempDir()
	out := filepath.Join(t.TempDir(), "restored")
	stubs := map[string]string{
		"pg_dump":    "#!/bin/sh\nprintf 'PGDMP-fixture-bytes'\n",
		"pg_restore": "#!/bin/sh\nexit 0\n",
		"docker":     "#!/bin/sh\n/bin/cat > '" + out + "'\n",
	}
	for n, body := range stubs {
		if err := os.WriteFile(filepath.Join(bin, n), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RCLONE_CONFIG", filepath.Join(t.TempDir(), "none.conf"))
	dest := t.TempDir()
	cfg := streamTestConfig()
	cfg.ProjectName = "proj"
	res, err := Stream(context.Background(), cfg, StreamOptions{To: ":local:" + dest})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	keyPath := filepath.Join(home, ".config", "nself", "proj-age.key")
	if !res.Encrypted || !strings.HasSuffix(res.BackupID, ".age") || mode(t, keyPath) != 0o600 {
		t.Fatalf("result %+v", res)
	}
	if !strings.Contains(notice.String(), keyPath) {
		t.Error("notice does not name the identity path")
	}
	if strings.Contains(notice.String()+logs.String(), secretMarker) {
		t.Error("secret leaked")
	}
	obj := filepath.Join(dest, res.BackupID)
	raw, _ := os.ReadFile(obj)
	if bytes.Contains(raw, []byte("PGDMP")) {
		t.Fatal("the object is not encrypted")
	}
	if err := RestoreFromRemote(context.Background(), cfg, ":local:"+obj, keyPath); err != nil {
		t.Fatalf("restore-remote with the auto identity: %v", err)
	}
	if b, _ := os.ReadFile(out); string(b) != "PGDMP-fixture-bytes" {
		t.Fatalf("round trip got %q", b)
	}
	// A second run reuses the same identity and stays silent.
	before, _ := os.ReadFile(keyPath)
	notice.Reset()
	if _, err := Stream(context.Background(), cfg, StreamOptions{To: ":local:" + dest}); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(keyPath); !bytes.Equal(before, after) || notice.Len() != 0 {
		t.Error("second run changed the key or printed the creation notice")
	}
}

func TestAutoKeyErrorCodesRegistered(t *testing.T) {
	for _, c := range []string{"E222", "E223", "E224"} {
		if _, ok := errs.Registry[c]; !ok {
			t.Errorf("%s not registered", c)
		}
	}
	if e := NewIdentityMissing("/x/k"); !strings.Contains(e.Error(), "[E223]") {
		t.Errorf("E223: %v", e)
	}
}
