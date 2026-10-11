package deploy

// Fix round 3 (H5, H7, L1).

import (
	"context"
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
