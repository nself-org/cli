package deploy

// Fix round 3 (H5, H7, L1).

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// H5: the local absent list may only name paths this deploy shipped.
func TestAbsentMustBeShipped(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	seedDeployedHost(t, rp)
	bk, err := CreatePreDeployTar(context.Background(), f, rp, NewBackupID(time.Now()), []string{"docker-compose.yml"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, rp+"/nginx/sites/foreign.conf", "# keep\n", 0o644)
	err = RestorePreDeployTar(context.Background(), f, rp, bk.Tar, RestoreSpec{Changed: []string{"docker-compose.yml"}, Absent: []string{"nginx/sites/foreign.conf"}})
	if err == nil || !strings.Contains(err.Error(), "did not ship") || !exists(rp+"/nginx/sites/foreign.conf") {
		t.Fatalf("an absent entry outside the change list was accepted: %v", err)
	}
}

// L1: remote steps never prompt: a child in a background process group would stop
// on SIGTTIN and hang the deploy.
func TestSSHOptsAreNonInteractive(t *testing.T) {
	opts := strings.Join(sshOpts("/k/id", []string{"-o", "StrictHostKeyChecking=yes"}), " ")
	for _, want := range []string{"-i /k/id", "StrictHostKeyChecking=yes", "ForwardAgent=no", "BatchMode=yes", "ConnectTimeout=10"} {
		if !strings.Contains(opts, want) {
			t.Errorf("ssh opts %q lack %q", opts, want)
		}
	}
}

// H7: a cancelled step is not killed: the child finishes (bounded by WaitDelay) and
// only then does the call return, so the restore never races a running remote script.
func TestCancelledStepFinishesBeforeReturn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 1; echo finished")
	ownProcessGroup(cmd)
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	out, _ := cmd.Output()
	if time.Since(start) < 900*time.Millisecond || !strings.Contains(string(out), "finished") {
		t.Errorf("the cancelled step was killed after %v with output %q", time.Since(start), out)
	}
}

// J3: pruneable refuses the host's own .env (and the other unsafe shapes).
func TestPruneableRefusesEnvAndUnsafe(t *testing.T) {
	for _, p := range []string{".env", "", ".", "..", "../x", "/abs", "nginx/conf.d/x.conf", "a//b", "a/./b"} {
		if pruneable(p) {
			t.Errorf("pruneable(%q) = true", p)
		}
	}
	for _, p := range []string{".env.prod", "nginx/sites/api.conf", "docker-compose.yml"} {
		if !pruneable(p) {
			t.Errorf("pruneable(%q) = false", p)
		}
	}
}

// N5: "inside remote_path" has a trailing-slash boundary: a link to /x/app2 is
// outside /x/app, for the pre-flight and for the delete guard alike.
func TestSiblingPrefixLinkIsOutside(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	base := t.TempDir()
	rp, sibling := base+"/app", base+"/app2"
	writeFile(t, rp+"/docker-compose.yml", "# v1\n", 0o644)
	writeFile(t, sibling+"/sites/x.conf", markedConf, 0o644)
	if err := os.Symlink("../app2", rp+"/nginx"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ResolveDirs(context.Background(), f, rp, []string{"nginx", "nginx/sites"}); err == nil || !strings.Contains(err.Error(), "symlink outside remote_path") {
		t.Fatalf("pre-flight: %v", err)
	}
	err := removeCreated(context.Background(), f, rp, []string{"nginx/sites/x.conf"})
	if err == nil || !exists(sibling+"/sites/x.conf") {
		t.Fatalf("delete guard: err %v, file survives = %v", err, exists(sibling+"/sites/x.conf"))
	}
}

// R7: the sibling-prefix boundary also holds for the backup dir and for a file link
// inside a followed dir.
func TestSiblingPrefixOtherSinksAreOutside(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	for name, mk := range map[string]func(rp, sibling string) error{
		"backup dir": func(rp, sibling string) error {
			if err := os.MkdirAll(rp+"/.nself", 0o755); err != nil {
				return err
			}
			return os.Symlink("../../app2", rp+"/.nself/backups")
		},
		"file link in nginx": func(rp, sibling string) error {
			writeFile(t, sibling+"/file", "secret\n", 0o600)
			return os.Symlink("../../app2/file", rp+"/nginx/f")
		},
	} {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			rp, sibling := base+"/app", base+"/app2"
			writeFile(t, rp+"/nginx/sites/a.conf", markedConf, 0o644)
			if err := os.MkdirAll(sibling, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := mk(rp, sibling); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := ResolveDirs(context.Background(), f, rp, layoutDirs([]string{"docker-compose.yml"})); err == nil || !strings.Contains(err.Error(), "symlink outside remote_path") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// N5: a path that is itself a symlink is never deleted or pruned, even inside remote_path.
func TestLeafSymlinkIsNeverDeleted(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	writeFile(t, rp+"/nginx/sites/real.conf", markedConf, 0o644)
	if err := os.Symlink("real.conf", rp+"/nginx/sites/leaf.conf"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := removeCreated(context.Background(), f, rp, []string{"nginx/sites/leaf.conf"}); err == nil || !exists(rp+"/nginx/sites/real.conf") {
		t.Fatalf("removal of a leaf symlink: %v", err)
	}
	if _, err := PruneMarked(context.Background(), f, rp, []string{"nginx/sites/leaf.conf"}); err == nil || !exists(rp+"/nginx/sites/real.conf") {
		t.Fatalf("prune of a leaf symlink: %v", err)
	}
}

// N5: a rollback runs the layout check before it takes the lock.
func TestRollbackChecksLayoutBeforeLock(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp, outside := t.TempDir(), t.TempDir()
	seedDeployedHost(t, rp)
	bk, err := CreatePreDeployTar(context.Background(), f, rp, NewBackupID(time.Now()), []string{"hasura/metadata/version.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(rp + "/hasura"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, rp+"/hasura"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	f.mu.Lock()
	f.log = nil
	f.mu.Unlock()
	err = RollbackRemote(context.Background(), RollbackOptions{Remote: f, RemotePath: rp, Tar: bk.Tar, Changed: []string{"hasura/metadata/version.yaml"}, Clock: &fakeClock{now: time.Unix(0, 0)}})
	if err == nil || !strings.Contains(err.Error(), "symlink outside remote_path") {
		t.Fatalf("err = %v", err)
	}
	for _, c := range f.commands() {
		if strings.Contains(c, "deploy.lock") {
			t.Errorf("the lock was taken before the layout check: %s", c)
		}
	}
}

// P3: a relative compose source (the build's docker-compose.override.yml) is read
// from the scratch root, never from the cwd, even when the cwd has a decoy.
func TestStageReadsRelativeSourceFromScratchRoot(t *testing.T) {
	m, compose, envs, snap := buildScratchFor(t, "/opt/nself")
	writeFile(t, m.ScratchRoot+"/docker-compose.override.yml", "# real override\n", 0o644)
	cwd := t.TempDir()
	writeFile(t, cwd+"/docker-compose.override.yml", "# DECOY\n", 0o644)
	t.Chdir(cwd)
	stage := t.TempDir()
	set, err := Stage(stageIn(m, append(compose, "docker-compose.override.yml"), envs, snap, stage))
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, stage+"/docker-compose.override.yml"); got != "# real override\n" {
		t.Errorf("shipped override = %q", got)
	}
	if !contains(set.Compose, "/opt/nself/docker-compose.override.yml") {
		t.Errorf("compose list %v lacks the override", set.Compose)
	}
}
