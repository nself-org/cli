package deploy

// G5, H6: a real SIGINT, SIGHUP or SIGTERM in the middle of a remote apply. The test re-executes the
// test binary as a child (the helper runs ApplyRemote against a fake host with
// a signal-aware context), waits until the child is inside the pull step, sends
// SIGINT to its pid, and reads back what the child reports: the old files are
// back, what the deploy created is gone, and the deploy lock is released.

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

const signalHelperEnv = "NSELF_DEPLOY_SIGNAL_HELPER"

// blockingPull makes the pull step wait for the context, like a slow ssh does.
type blockingPull struct {
	*fakeRemote
	ctx context.Context
}

func (b blockingPull) Run(ctx context.Context, cmd string) ([]byte, error) {
	if strings.Contains(unshell(cmd), " pull") {
		fmt.Println("READY")
		<-b.ctx.Done()
		return nil, b.ctx.Err()
	}
	return b.fakeRemote.Run(ctx, cmd)
}

// TestInterruptHelper is the child process; it does nothing unless re-executed.
func TestInterruptHelper(t *testing.T) {
	if os.Getenv(signalHelperEnv) == "" {
		t.Skip("helper process for TestInterruptRestoresAndUnlocks")
	}
	f, o, rp := applyFixture(t, "")
	ctx, stop := WithInterrupt(context.Background())
	defer stop()
	o.Remote = blockingPull{fakeRemote: f, ctx: ctx}
	res, err := ApplyRemote(ctx, o)
	restored := strings.Contains(readFile(t, rp+"/docker-compose.yml"), "# v1")
	fmt.Printf("RESULT err=%v synced=%v restored=%v lock=%v restoreRan=%v\n", err != nil && strings.Contains(err.Error(), "interrupted"),
		res.Synced, restored, exists(rp+"/.nself/deploy.lock"), indexOf(steps(f), "restore") >= 0)
}

func TestInterruptRestoresAndUnlocks(t *testing.T) {
	for name, sig := range map[string]syscall.Signal{"SIGINT": syscall.SIGINT, "SIGHUP": syscall.SIGHUP, "SIGTERM": syscall.SIGTERM} {
		t.Run(name, func(t *testing.T) { interruptWith(t, sig, false) })
	}
}

// interruptWith sends sig to a child that is inside the pull step.
func interruptWith(t *testing.T, sig syscall.Signal, twice bool) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no SIGINT delivery to a child on windows")
	}
	needTools(t)
	c := exec.Command(os.Args[0], "-test.run=^TestInterruptHelper$", "-test.count=1")
	c.Env = append(os.Environ(), signalHelperEnv+"=1")
	out, err := c.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	c.Stderr = &stderr
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Process.Kill(); _ = c.Wait() })
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	deadline := time.After(90 * time.Second)
	sent, result := false, ""
	for result == "" {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatal("the helper exited without reporting a result")
			}
			if l == "READY" && !sent {
				sent = true
				if err := c.Process.Signal(sig); err != nil {
					t.Fatal(err)
				}
				if twice {
					time.Sleep(300 * time.Millisecond) // identical signals sent back to back coalesce
					_ = c.Process.Signal(sig)
				}
			}
			if strings.HasPrefix(l, "RESULT ") {
				result = l
			}
		case <-deadline:
			t.Fatal("timed out waiting for the helper")
		}
	}
	_ = c.Wait()
	if !strings.Contains(stderr.String(), "interrupted: finishing the current step (up to 2m0s)") {
		t.Errorf("N3: the first signal printed no notice; stderr:\n%s", stderr.String())
	}
	if got := strings.Contains(stderr.String(), "still finishing the step or restoring"); got != twice {
		t.Errorf("second-signal notice printed = %v, want %v; stderr:\n%s", got, twice, stderr.String())
	}
	want := "RESULT err=true synced=false restored=true lock=false restoreRan=true"
	if result != want {
		t.Errorf("after SIGINT:\n got %s\nwant %s", result, want)
	}
}

// J2: a second signal while the restore runs is absorbed with one line saying the
// that it is still finishing the step or restoring, not silently ignored.
func TestSecondSignalIsAnnounced(t *testing.T) { interruptWith(t, syscall.SIGINT, true) }
