package simharness

import (
	"strings"
	"sync"
	"testing"
)

func TestStartSkipsWithoutIntegration(t *testing.T) {
	t.Setenv(EnvIntegration, "")
	tb := newFakeTB(t)
	tb.run(func() { Start(tb, Config{Nodes: []NodeSpec{{Name: "a"}}}) })
	if !tb.skipped || tb.fatal != "" {
		t.Fatalf("skipped=%v fatal=%q, want a skip", tb.skipped, tb.fatal)
	}
}

// With INTEGRATION=1 and no daemon, Start must fail the test, never skip it (review F12).
func TestStartFailsWhenDockerUnreachable(t *testing.T) {
	t.Setenv(EnvIntegration, "1")
	t.Setenv("DOCKER_HOST", "unix:///nonexistent/simharness.sock")
	tb := newFakeTB(t)
	tb.run(func() { Start(tb, Config{Nodes: []NodeSpec{{Name: "a"}}}) })
	if tb.skipped {
		t.Fatal("Start skipped with INTEGRATION=1 and no Docker daemon")
	}
	if !strings.Contains(tb.fatal, "Docker is unreachable") {
		t.Fatalf("fatal = %q, want the unreachable message", tb.fatal)
	}
}

func TestStartRejectsBadConfig(t *testing.T) {
	cases := map[string]Config{
		"empty":    {},
		"bad name": {Nodes: []NodeSpec{{Name: "a b"}}},
		"dup name": {Nodes: []NodeSpec{{Name: "a"}, {Name: "a"}}},
		"unpinned": {Nodes: []NodeSpec{{Name: "a", Image: "alpine:3.20"}}},
		"tagged":   {Nodes: []NodeSpec{{Name: "a", Image: "debian"}, {Name: "b", Image: "x/y:1.2.3"}}},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			d := installFakeDocker(t)
			tb := newFakeTB(t)
			tb.run(func() { Start(tb, cfg) })
			if tb.fatal == "" {
				t.Fatal("Start accepted the config")
			}
			if n := len(d.find("run")) + len(d.find("network", "create")); n != 0 {
				t.Fatalf("%d containers or networks created for a rejected config", n)
			}
		})
	}
}

func TestStartRunArgv(t *testing.T) {
	d := installFakeDocker(t)
	tb := newFakeTB(t)
	var f *Fleet
	tb.run(func() {
		f = Start(tb, Config{Nodes: []NodeSpec{
			{Name: "a"},
			{Name: "b", Image: ImageDebian, Platform: "linux/arm64", CapAdd: []string{"NET_ADMIN"}, Env: map[string]string{"X": "1"}},
		}})
	})
	if tb.fatal != "" {
		t.Fatal(tb.fatal)
	}
	runs := d.find("run")
	if len(runs) != 2 {
		t.Fatalf("%d run calls, want 2", len(runs))
	}
	a, b := runs[0], runs[1]
	if got := flagValues(a, "--name"); len(got) != 1 || got[0] != "nself-sim-"+f.ID+"-a" {
		t.Fatalf("container name = %v", got)
	}
	if got := flagValues(a, "--network"); len(got) != 1 || got[0] != "nself-sim-"+f.ID {
		t.Fatalf("network = %v", got)
	}
	if got := flagValues(a, "-p"); len(got) != 1 || got[0] != "127.0.0.1::2222" {
		t.Fatalf("publish = %v", got)
	}
	if a[len(a)-1] != OpenSSHRef || !Pinned(a[len(a)-1]) {
		t.Fatalf("default image %q is not the pinned openssh ref", a[len(a)-1])
	}
	if len(flagValues(a, "--cap-add")) != 0 {
		t.Fatalf("node a got capabilities %v", flagValues(a, "--cap-add"))
	}
	if got := flagValues(b, "--cap-add"); len(got) != 1 || got[0] != "NET_ADMIN" {
		t.Fatalf("node b cap-add = %v", got)
	}
	if got := flagValues(b, "--platform"); len(got) != 1 || got[0] != "linux/arm64" {
		t.Fatalf("node b platform = %v", got)
	}
	for _, c := range runs {
		for _, arg := range c {
			if arg == "--privileged" {
				t.Fatal("a container was started --privileged")
			}
		}
		if len(flagValues(c, "--label")) != 2 {
			t.Fatalf("labels = %v", flagValues(c, "--label"))
		}
	}
	if !strings.HasPrefix(b[len(b)-1], "nself-simharness-debian:") {
		t.Fatalf("debian image = %q, want a content-hash tag", b[len(b)-1])
	}
	if n := f.Node(tb, "a"); n.User != "nself" || n.Port != d.port || n.Host != "127.0.0.1" {
		t.Fatalf("node a = %+v", n)
	}
}

func TestParallelStartsGetDistinctNames(t *testing.T) {
	d := installFakeDocker(t)
	var wg sync.WaitGroup
	fleets := make([]*Fleet, 2)
	tbs := []*fakeTB{newFakeTB(t), newFakeTB(t)}
	for i := range fleets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tbs[i].run(func() { fleets[i] = Start(tbs[i], Config{Nodes: []NodeSpec{{Name: "a"}, {Name: "b"}}}) })
		}()
	}
	wg.Wait()
	for _, tb := range tbs {
		if tb.fatal != "" {
			t.Fatal(tb.fatal)
		}
	}
	names := map[string]bool{}
	for _, c := range d.find("run") {
		names[flagValues(c, "--name")[0]] = true
	}
	nets := map[string]bool{}
	for _, c := range d.find("network", "create") {
		nets[c[len(c)-1]] = true
	}
	if len(names) != 4 || len(nets) != 2 {
		t.Fatalf("containers %v networks %v, want 4 and 2 distinct", names, nets)
	}
}

// Close must remove every container and the network, also after t.Fatal, and
// only this fleet's own ones.
func TestCloseAfterFatalRemovesOnlyOwnObjects(t *testing.T) {
	d := installFakeDocker(t)
	other := newFakeTB(t)
	other.keep = true
	var o *Fleet
	other.run(func() { o = Start(other, Config{Nodes: []NodeSpec{{Name: "x"}}}) })
	if other.fatal != "" {
		t.Fatal(other.fatal)
	}
	t.Cleanup(o.Close)

	tb := newFakeTB(t)
	var f *Fleet
	tb.run(func() {
		f = Start(tb, Config{Nodes: []NodeSpec{{Name: "a"}, {Name: "b"}}})
		tb.Fatalf("test failed on purpose")
	})
	if tb.fatal == "" || f == nil {
		t.Fatal("scenario did not reach the Fatalf")
	}
	rms := d.find("rm")
	// the first fleet is still open: exactly one rm, for the second fleet
	if len(rms) != 1 {
		t.Fatalf("rm calls = %v", rms)
	}
	got := rms[0][3:]
	want := d.byFleet[f.ID]
	if strings.Join(got, ",") != strings.Join(want, ",") || len(want) != 2 {
		t.Fatalf("rm ids %v, want exactly fleet %s containers %v", got, f.ID, want)
	}
	nets := d.find("network", "rm")
	if len(nets) != 1 || nets[0][2] != f.Network {
		t.Fatalf("network rm = %v, want only %s", nets, f.Network)
	}
	f.Close() // idempotent
	if len(d.find("rm")) != 1 {
		t.Fatal("second Close removed again")
	}
}

// A failure halfway through Start still cleans up what exists.
func TestStartFailureStillCleansUp(t *testing.T) {
	d := installFakeDocker(t)
	d.failOn = "port "
	tb := newFakeTB(t)
	tb.run(func() { Start(tb, Config{Nodes: []NodeSpec{{Name: "a"}}}) })
	if tb.fatal == "" {
		t.Fatal("expected a failure")
	}
	if len(d.find("network", "rm")) != 1 || len(d.find("rm")) != 1 {
		t.Fatalf("calls: rm=%v network rm=%v", d.find("rm"), d.find("network", "rm"))
	}
}

func TestCloseSkipsRmWithoutContainers(t *testing.T) {
	d := installFakeDocker(t)
	d.failOn = "network create"
	tb := newFakeTB(t)
	tb.run(func() { Start(tb, Config{Nodes: []NodeSpec{{Name: "a"}}}) })
	if len(d.find("rm")) != 0 {
		t.Fatalf("rm called with no containers: %v", d.find("rm"))
	}
	if len(d.find("network", "rm")) != 1 {
		t.Fatal("network rm not attempted")
	}
}
