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

// End to end with local fixtures: fake pg_dump, real age and rclone, a named local
// rclone remote from the environment. First run creates the identity; the object restores
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
	// A named remote configured through the environment (RCLONE_CONFIG_<NAME>_TYPE).
	t.Setenv("RCLONE_CONFIG_NSELFTEST_TYPE", "local")
	dest := t.TempDir()
	cfg := streamTestConfig()
	cfg.ProjectName = "proj"
	res, err := Stream(context.Background(), cfg, StreamOptions{To: "nselftest:" + dest})
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
	// No --key: restore finds the auto identity by itself.
	if err := RestoreFromRemote(context.Background(), cfg, "nselftest:"+obj, ""); err != nil {
		t.Fatalf("restore-remote with the auto identity: %v", err)
	}
	if b, _ := os.ReadFile(out); string(b) != "PGDMP-fixture-bytes" {
		t.Fatalf("round trip got %q", b)
	}
	// A second run reuses the same identity and stays silent.
	before, _ := os.ReadFile(keyPath)
	notice.Reset()
	if _, err := Stream(context.Background(), cfg, StreamOptions{To: "nselftest:" + dest}); err != nil {
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
	if e := NewIdentityMissing("/x/k", "--key"); !strings.Contains(e.Error(), "[E223]") {
		t.Errorf("E223: %v", e)
	}
}

// restore-remote and the drill search <project>-age.key, then
// <project>-backup-age.key, then age-key.txt, and answer E223 when none exists.
func TestAutoKeyDefaultIdentityOrderAndE223(t *testing.T) {
	home, _, _ := autoKeyEnv(t)
	compattest.Set(t, true)
	dir := filepath.Join(home, ".config", "nself")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var ce *errs.CLIError
	if _, err := DefaultIdentity("proj", "--key"); !errors.As(err, &ce) || ce.Code != "E223" || !strings.Contains(err.Error(), "proj-age.key") {
		t.Fatalf("none present: want E223 naming the default path, got %v", err)
	}
	if _, err := resolveIdentity("proj", ""); !errors.As(err, &ce) || ce.Code != "E223" {
		t.Fatalf("drill with no identity: want E223, got %v", err)
	}
	if err := RestoreFromRemote(context.Background(), streamTestConfig(), "path://"+t.TempDir()+"/x.dump.age", ""); err == nil || !strings.Contains(err.Error(), "[E223]") {
		t.Fatalf("restore-remote with no identity: want E223, got %v", err)
	}
	order := []string{"age-key.txt", "proj-backup-age.key", "proj-age.key"}
	for _, n := range order {
		newAgeKey(t, filepath.Join(dir, n))
		got, err := DefaultIdentity("proj", "--key")
		if err != nil || filepath.Base(got) != n {
			t.Fatalf("after adding %s: got %q %v", n, got, err)
		}
	}
	// an explicit --identity wins over every default
	flag := filepath.Join(home, "mine.key")
	_ = os.WriteFile(flag, []byte("x"), 0o600) // explicit path: used as given, even if not an identity
	if got, err := resolveIdentity("proj", flag); err != nil || got != flag {
		t.Fatalf("flag: %q %v", got, err)
	}
}

// A winner stalled between link and unlink leaves a 2-link key; a second run
// must accept it (the other link is the winner's own temp), not give up.
func TestAutoKeyStalledWinnerIsAccepted(t *testing.T) {
	autoKeyEnv(t)
	linked, release := make(chan struct{}), make(chan struct{})
	old := afterLinkHook
	afterLinkHook = func() { close(linked); <-release }
	t.Cleanup(func() { afterLinkHook = old })
	type res struct {
		id  Identity
		err error
	}
	winner := make(chan res, 1)
	go func() { id, err := EnsureIdentity("race"); winner <- res{id, err} }()
	<-linked
	path, _ := IdentityPath("race")
	if fi, _ := os.Lstat(path); linkCount(fi) != 2 {
		t.Fatalf("expected the stalled state (2 links), got %d", linkCount(fi))
	}
	afterLinkHook = func() {} // later runs do not stall
	id2, err := EnsureIdentity("race")
	if err != nil || id2.Created {
		t.Fatalf("second run during the stall: %+v %v", id2, err)
	}
	close(release)
	w := <-winner
	if w.err != nil || !w.id.Created || w.id.Recipient != id2.Recipient {
		t.Fatalf("winner %+v %v vs %s", w.id, w.err, id2.Recipient)
	}
	ents, _ := os.ReadDir(filepath.Dir(path))
	if len(ents) != 1 {
		t.Fatalf("temp not unlinked after the stall: %v", ents)
	}
}

// --dry-run reports a bad existing auto identity instead of a placeholder.
func TestAutoKeyDryRunSurfacesBadIdentity(t *testing.T) {
	home, _, _ := autoKeyEnv(t)
	compattest.Set(t, true)
	dir := filepath.Join(home, ".config", "nself")
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, "proj-age.key"), []byte("garbage\n"), 0o600)
	cfg := streamTestConfig()
	cfg.ProjectName = "proj"
	_, err := Stream(context.Background(), cfg, StreamOptions{To: "s3:b/p", DryRun: true})
	if errCode(err) != "E222" || !strings.Contains(err.Error(), "proj-age.key") {
		t.Fatalf("dry-run with a garbage identity: want E222, got %v", err)
	}
}
