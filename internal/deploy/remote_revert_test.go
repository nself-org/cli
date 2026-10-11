package deploy

// Fix round 2 (G1-G4, G8, G9): the restore is an exact revert driven by the local
// record, deletions never follow a symlink, a restored deploy keeps the last
// record, rollback validates nginx and applies hasura, host names are validated.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// G1: only what the deploy shipped is restored or removed; the host's list of
// absent files is not consulted at all; unsafe or inconsistent lists are refused.
func TestRestoreIsDrivenByTheLocalRecord(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	seedDeployedHost(t, rp)
	writeFile(t, rp+"/.env.secrets", "TOKEN=1\n", 0o600)
	writeFile(t, rp+"/hasura/metadata/other.yaml", "mine\n", 0o644)
	ctx := context.Background()
	changed := []string{"docker-compose.yml", "nginx/sites/new.conf"}
	bk, err := CreatePreDeployTar(ctx, f, rp, NewBackupID(time.Now()), changed)
	if err != nil {
		t.Fatal(err)
	}
	// A hostile or stale file on the host naming other files; the restore must not read it.
	writeFile(t, rp+"/.nself/backups/"+filepath.Base(strings.TrimSuffix(bk.Tar, ".tar.gz"))+".absent", "hasura/metadata/other.yaml\nnginx/sites/foreign.conf\n.env.secrets\n", 0o644)
	writeFile(t, rp+"/docker-compose.yml", "# broken\n", 0o644)
	writeFile(t, rp+"/nginx/sites/new.conf", "created by the deploy\n", 0o644)
	writeFile(t, rp+"/.env.secrets", "TOKEN=rotated\n", 0o600) // changed by someone else after the tar
	if err := RestorePreDeployTar(ctx, f, rp, bk.Tar, RestoreSpec{Changed: changed, Absent: bk.Absent}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, rp+"/docker-compose.yml"), "# v1") || exists(rp+"/nginx/sites/new.conf") {
		t.Error("the deploy's own changes were not reverted")
	}
	for p, want := range map[string]string{"hasura/metadata/other.yaml": "mine\n", "nginx/sites/foreign.conf": "# hand written\n", ".env.secrets": "TOKEN=rotated\n"} {
		if got := readFile(t, filepath.Join(rp, p)); got != want {
			t.Errorf("%s was touched by the restore: %q", p, got)
		}
	}
	for name, spec := range map[string]RestoreSpec{
		"dotdot":       {Changed: []string{"../outside"}},
		"dot":          {Absent: []string{"."}},
		"empty":        {Absent: []string{""}},
		"env":          {Absent: []string{".env"}},
		"conf.d":       {Absent: []string{"nginx/conf.d/x.conf"}},
		"absolute":     {Changed: []string{"/etc/passwd"}},
		"in the tar":   {Changed: []string{"hasura/metadata/version.yaml"}, Absent: []string{"hasura/metadata/version.yaml"}},
		"inner dotdot": {Changed: []string{"nginx/../.env.secrets"}},
	} {
		if err := RestorePreDeployTar(ctx, f, rp, bk.Tar, spec); err == nil {
			t.Errorf("%s: an unsafe or inconsistent list was accepted", name)
		}
	}
	if readFile(t, rp+"/.env.secrets") != "TOKEN=rotated\n" {
		t.Error("a refused restore still changed a file")
	}
}

// G2: prune and remove never delete through a symlinked directory; the batch is
// refused whole and the error names the path.
func TestDeletesRefuseSymlinkedDirs(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp, outside := t.TempDir(), t.TempDir()
	writeFile(t, outside+"/other.conf", markedConf, 0o644)
	writeFile(t, rp+"/nginx/own.conf", markedConf, 0o644)
	if err := os.Symlink(outside, rp+"/nginx/sites"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeFile(t, rp+"/"+ShippedFile, "nginx/own.conf\nnginx/sites/other.conf\n", 0o644)
	_, err := PrunePrevious(context.Background(), f, rp, map[string]bool{})
	if err == nil || !strings.Contains(err.Error(), "nginx/sites/other.conf") || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("prune through a symlink: %v", err)
	}
	if !exists(outside+"/other.conf") || !exists(rp+"/nginx/own.conf") {
		t.Error("a refused batch still deleted a file")
	}
	// Removal of created files goes through the same guard.
	bk, err := CreatePreDeployTar(context.Background(), f, rp, NewBackupID(time.Now()), []string{"nginx/sites/other.conf"})
	if err != nil {
		t.Fatal(err)
	}
	err = removeCreated(context.Background(), f, rp, []string{"nginx/sites/other.conf"})
	if err == nil || !exists(outside+"/other.conf") {
		t.Fatalf("removal through a symlink: %v", err)
	}
	_ = bk
	if pruneable("nginx/./a") || pruneable("nginx//a") || pruneable(".") || pruneable("") {
		t.Error("pruneable accepted a . or empty component")
	}
}

// G3: a symlinked directory on the host is followed by rsync, never replaced by a real dir.
func TestRestoreKeepsSymlinkedDir(t *testing.T) {
	needTools(t)
	argv, err := rsyncArgv(remote.HostSpec{Host: "h.example"}, nil, nil, "/l", "/s", "/opt/nself")
	if err != nil || !contains(argv, "--keep-dirlinks") {
		t.Fatalf("the sync must follow symlinked dirs: %v %v", argv, err)
	}
	if runtime.GOOS == "darwin" {
		t.Skip("macOS rsync cannot chmod through a symlinked implied dir (ELOOP); the sim covers a Linux host")
	}
	f := newFakeRemote(t)
	rp, shared := t.TempDir(), t.TempDir()
	writeFile(t, rp+"/nginx/sites/old.conf", markedConf+"# v1\n", 0o644)
	ctx := context.Background()
	bk, err := CreatePreDeployTar(ctx, f, rp, NewBackupID(time.Now()), []string{"nginx/sites/old.conf"})
	if err != nil {
		t.Fatal(err)
	}
	// The operator later moved nginx/ to a shared place and linked it.
	if err := os.Rename(rp+"/nginx", shared+"/nginx"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared+"/nginx", rp+"/nginx"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeFile(t, shared+"/nginx/sites/old.conf", markedConf+"# v2\n", 0o644)
	writeFile(t, shared+"/nginx/sites/foreign.conf", "# foreign\n", 0o644)
	if err := RestorePreDeployTar(ctx, f, rp, bk.Tar, RestoreSpec{Changed: []string{"nginx/sites/old.conf"}}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(rp + "/nginx"); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("nginx/ is no longer a symlink: %v %v", info, err)
	}
	if !strings.Contains(readFile(t, shared+"/nginx/sites/old.conf"), "# v1") || !exists(shared+"/nginx/sites/foreign.conf") {
		t.Error("the restore did not write through the link, or lost a foreign file")
	}
}

// G4: a deploy that failed and was restored leaves the last good record alone;
// if the restore itself failed, the record is written without the database backup.
func TestRestoredDeployReportsNotSynced(t *testing.T) {
	f, o, rp := applyFixture(t, "")
	f.fail["docker run --rm"] = errBoom // nginx -t refuses after the sync
	res, err := ApplyRemote(context.Background(), o)
	if err == nil {
		t.Fatal("want the nginx -t refusal")
	}
	if res.Synced {
		t.Errorf("a restored deploy must report Synced=false: %+v", res)
	}
	if !strings.Contains(readFile(t, rp+"/docker-compose.yml"), "# v1") {
		t.Error("not restored")
	}
	f2, o2, _ := applyFixture(t, "")
	f2.fail["docker run --rm"] = errBoom
	f2.fail["tar -xzf"] = errBoom // and the restore fails too
	res2, err := ApplyRemote(context.Background(), o2)
	if err == nil || !strings.Contains(err.Error(), "also failed") {
		t.Fatalf("err = %v", err)
	}
	if !res2.Synced || res2.BackupID != "" {
		t.Errorf("a failed restore must keep Synced and drop the database backup: %+v", res2)
	}
}

// G8: the rollback validates the restored nginx config before reloading, then
// applies the hasura metadata.
func TestRollbackChecksNginxAndAppliesHasura(t *testing.T) {
	needTools(t)
	f, o, rp := applyFixture(t, "")
	res, err := ApplyRemote(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	run := func(fail string) error {
		f.mu.Lock()
		f.log = nil
		f.mu.Unlock()
		delete(f.fail, "docker run --rm")
		if fail != "" {
			f.fail[fail] = errBoom
		}
		return RollbackRemote(context.Background(), RollbackOptions{Remote: f, RemotePath: rp, Tar: res.PreDeployTar, Changed: res.Changed, Absent: res.Absent, StrictHasura: true, Clock: &fakeClock{now: time.Unix(0, 0)}})
	}
	if err := run(""); err != nil {
		t.Fatal(err)
	}
	got := steps(f)
	prev := -1
	for _, want := range []string{"restore", "pull", "up:db", "nginx", "reload", "hasura"} {
		i := indexOf(got, want)
		if i <= prev {
			t.Fatalf("rollback step %q at %d after %d in %v", want, i, prev, got)
		}
		prev = i
	}
	err = run("docker run --rm")
	if err == nil || !strings.Contains(err.Error(), "nginx -t") || !strings.Contains(err.Error(), "not reloaded") {
		t.Fatalf("err = %v", err)
	}
	if got := steps(f); indexOf(got, "reload") >= 0 || indexOf(got, "hasura") >= 0 {
		t.Errorf("nginx was reloaded after a failed nginx -t: %v", got)
	}
}

// G9: names read from the host are validated before they reach a path or a message.
func TestHostNamesAreValidated(t *testing.T) {
	_, err := parsePEMNames("/x/ssl/certificates/a\\..\\..\\x/fullchain.pem\n")
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E485" || !strings.Contains(err.Error(), `"a\\..\\..\\x"`) {
		t.Fatalf("cert dir: %v", err)
	}
	if _, err := parsePEMNames("/x/ssl/certificates/api-example.com/fullchain.pem\n"); err != nil {
		t.Errorf("a normal cert dir was refused: %v", err)
	}
	_, err = parsePluginBlocks("@F /p/ai/plugin.json\n{\"name\":\"ai\\u001b[31m\",\"version\":\"1\"}\n")
	if !errors.As(err, &ce) || ce.Code != "E485" || strings.Contains(err.Error(), "\x1b") {
		t.Fatalf("plugin name with an escape: %v", err)
	}
	if _, err := parsePluginBlocks("@F /p/ai/plugin.json\n{\"name\":\"ai\",\"version\":\"1.2.0-rc.1+build5\"}\n"); err != nil {
		t.Errorf("a semver with build metadata was refused: %v", err)
	}
}
