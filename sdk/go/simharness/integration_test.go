//go:build integration

package simharness

// Purpose: the Docker-backed acceptance tests for Start, Exec, CopyTo, faults and per-distro images.
// Run: INTEGRATION=1 go test -tags integration -count=1 -timeout 20m ./simharness/...

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Two Starts in parallel with the same logical names get distinct names, both
// work, and nothing is left behind afterwards.
func TestIntegrationParallelStartsAreIsolated(t *testing.T) {
	var mu sync.Mutex
	ids := map[string]string{}
	t.Run("group", func(t *testing.T) {
		for _, name := range []string{"one", "two"} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				f := Start(t, Config{Nodes: []NodeSpec{{Name: "app", Image: ImageAlpine}, {Name: "lb", Image: ImageAlpine}}})
				mu.Lock()
				ids[name] = f.ID
				mu.Unlock()
				if got := mustSSH(t, f, f.Node(t, "app"), "echo", name); got != name {
					t.Fatalf("ssh echo = %q", got)
				}
				if got := strings.Join(fleetObjects(t, f.ID), ","); !strings.Contains(got, "nself-sim-"+f.ID+"-app") || !strings.Contains(got, "nself-sim-"+f.ID+"-lb") {
					t.Fatalf("objects = %s", got)
				}
			})
		}
	})
	if len(ids) != 2 || ids["one"] == ids["two"] {
		t.Fatalf("fleet ids %v are not distinct", ids)
	}
	for name, id := range ids {
		if left := fleetObjects(t, id); len(left) != 0 {
			t.Fatalf("%s: left behind after Close: %v", name, left)
		}
	}
}

// Close runs from t.Cleanup also after a t.Fatal.
func TestIntegrationCloseAfterFatal(t *testing.T) {
	tb := newFakeTB(t)
	var id string
	tb.run(func() {
		f := Start(tb, Config{Nodes: []NodeSpec{{Name: "a", Image: ImageAlpine}}})
		id = f.ID
		if len(fleetObjects(t, id)) != 2 {
			t.Errorf("objects before Fatal = %v", fleetObjects(t, id))
		}
		tb.Fatalf("failing on purpose")
	})
	if tb.fatal == "" {
		t.Fatal("scenario did not fail")
	}
	if left := fleetObjects(t, id); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

// SIGINT removes a live fleet when t.Cleanup cannot run.
func TestIntegrationSigintRemovesFleet(t *testing.T) {
	if os.Getenv("SIMHARNESS_SIGINT_CHILD") == "1" {
		f := Start(t, Config{Nodes: []NodeSpec{{Name: "a", Image: ImageAlpine}}})
		os.Stdout.WriteString("SIMHARNESS_READY " + f.ID + "\n")
		time.Sleep(2 * time.Minute)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestIntegrationSigintRemovesFleet$", "-test.timeout=5m")
	cmd.Env = append(os.Environ(), "SIMHARNESS_SIGINT_CHILD=1", "INTEGRATION=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	id := ""
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "SIMHARNESS_READY "); ok {
			id = strings.TrimSpace(rest)
			break
		}
	}
	if id == "" {
		t.Fatal("child never reported a fleet")
	}
	if n := len(fleetObjects(t, id)); n != 2 {
		t.Fatalf("objects while running = %d, want 2", n)
	}
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 130 {
		t.Fatalf("child exit = %v, want status 130", err)
	}
	if left := fleetObjects(t, id); len(left) != 0 {
		t.Fatalf("left behind after SIGINT: %v", left)
	}
}

// Every image accepts the fleet key for the login user, refuses root and passwords.
func TestIntegrationDistrosAcceptFleetKey(t *testing.T) {
	var specs []NodeSpec
	for _, img := range []string{ImageOpenSSH, ImageDebian, ImageFedora, ImageAlpine} {
		specs = append(specs, NodeSpec{Name: img, Image: img})
	}
	f := Start(t, Config{Nodes: specs})
	for _, n := range f.Nodes {
		t.Run(n.Name, func(t *testing.T) {
			if got := mustSSH(t, f, n, "id", "-un"); got != "nself" {
				t.Fatalf("id -un = %q", got)
			}
			if _, err := hostSSH(f, n, "root", 10*time.Second, "true"); err == nil {
				t.Fatal("root login was accepted")
			}
		})
	}
}

func TestIntegrationExecAndCopy(t *testing.T) {
	f := Start(t, Config{Nodes: []NodeSpec{{Name: "a", Image: ImageDebian}}})
	if got := f.Exec(t, "a", "echo", "hello"); got != "hello" {
		t.Fatalf("Exec = %q", got)
	}
	if got := f.Exec(t, "a", "sh", "-c", "uname -s"); got != "Linux" {
		t.Fatalf("uname = %q", got)
	}
	src := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(src, []byte("simharness payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.CopyTo(t, "a", src, "/tmp/payload")
	if got := mustSSH(t, f, f.Node(t, "a"), "cat", "/tmp/payload"); got != "simharness payload" {
		t.Fatalf("file over ssh = %q", got)
	}
	f.Restart(t, "a")
	if got := mustSSH(t, f, f.Node(t, "a"), "id", "-un"); got != "nself" {
		t.Fatalf("after Restart id -un = %q", got)
	}
}

func TestIntegrationPauseStopsSSH(t *testing.T) {
	f := Start(t, Config{Nodes: []NodeSpec{{Name: "a", Image: ImageDebian}}})
	n := f.Node(t, "a")
	mustSSH(t, f, n, "true")
	f.Pause(t, "a")
	if _, err := n.Banner(2 * time.Second); err == nil {
		t.Fatal("paused node still answers with an SSH banner")
	}
	if _, err := hostSSH(f, n, n.User, 3*time.Second, "true"); err == nil {
		t.Fatal("ssh to a paused node succeeded")
	}
	f.Unpause(t, "a")
	mustSSH(t, f, n, "true")
}

func TestIntegrationNetemAddsDelay(t *testing.T) {
	f := Start(t, Config{Nodes: []NodeSpec{{Name: "a", Image: ImageDebian, CapAdd: []string{"NET_ADMIN"}}}})
	n := f.Node(t, "a")
	base := medianBanner(t, n, 5)
	f.Netem(t, "a", 80*time.Millisecond, 0)
	slow := medianBanner(t, n, 5)
	if slow-base < 60*time.Millisecond {
		t.Fatalf("netem(80ms): median %v -> %v, want an increase of at least 60ms", base, slow)
	}
	f.Netem(t, "a", 0, 0)
	if back := medianBanner(t, n, 5); back-base > 40*time.Millisecond {
		t.Fatalf("after clearing netem: median %v, baseline %v", back, base)
	}
}

func TestIntegrationPartitionAndHeal(t *testing.T) {
	f := Start(t, Config{Nodes: []NodeSpec{{Name: "a", Image: ImageDebian}, {Name: "b", Image: ImageDebian}}})
	n := f.Node(t, "a")
	reach := func() error { // b reaches a by alias over the fleet network
		_, err := dockerOut(f, "b", "timeout", "3", "bash", "-c", "exec 3<>/dev/tcp/a/22")
		return err
	}
	if err := reach(); err != nil {
		t.Fatalf("b cannot reach a before the partition: %v", err)
	}
	f.Partition(t, "a")
	if err := reach(); err == nil {
		t.Fatal("b still reaches a during the partition")
	}
	if _, err := n.Banner(2 * time.Second); err == nil {
		t.Fatal("the host still reaches a during the partition")
	}
	f.Heal(t, "a")
	if err := reach(); err != nil {
		t.Fatalf("b cannot reach a after Heal: %v", err)
	}
	mustSSH(t, f, f.Node(t, "a"), "true")
}

// dockerOut runs argv in a node without failing the test, for negative checks.
func dockerOut(f *Fleet, node string, argv ...string) (string, error) {
	var c string
	for _, n := range f.Nodes {
		if n.Name == node {
			c = n.Container
		}
	}
	return docker(testCtx(), append([]string{"exec", c}, argv...)...)
}
