package simharness

import (
	"net"
	"strings"
	"testing"
	"time"
)

func startOne(t *testing.T, spec NodeSpec) (*fakeDocker, *Fleet, *fakeTB) {
	t.Helper()
	d := installFakeDocker(t)
	tb := newFakeTB(t)
	var f *Fleet
	tb.run(func() { f = Start(tb, Config{Nodes: []NodeSpec{spec}}); tb.keep = true })
	if tb.fatal != "" {
		t.Fatal(tb.fatal)
	}
	t.Cleanup(f.Close)
	return d, f, tb
}

func TestPauseUnpauseArgv(t *testing.T) {
	d, f, tb := startOne(t, NodeSpec{Name: "a"})
	f.Pause(tb, "a")
	f.Unpause(tb, "a")
	c := "nself-sim-" + f.ID + "-a"
	if p := d.find("pause"); len(p) != 1 || p[0][1] != c {
		t.Fatalf("pause = %v", p)
	}
	if p := d.find("unpause"); len(p) != 1 || p[0][1] != c {
		t.Fatalf("unpause = %v", p)
	}
}

func TestNetemNeedsNetAdmin(t *testing.T) {
	d, f, tb := startOne(t, NodeSpec{Name: "a"})
	tb.fatal = ""
	tb.run(func() { f.Netem(tb, "a", 80*time.Millisecond, 0) })
	if !strings.Contains(tb.fatal, "NET_ADMIN") {
		t.Fatalf("fatal = %q", tb.fatal)
	}
	if len(d.find("exec")) != 0 {
		t.Fatal("netem ran without NET_ADMIN")
	}
}

func TestNetemArgv(t *testing.T) {
	d, f, tb := startOne(t, NodeSpec{Name: "a", CapAdd: []string{"CAP_NET_ADMIN"}})
	f.Netem(tb, "a", 80*time.Millisecond, 2.5)
	f.Netem(tb, "a", 0, 0)
	ex := d.find("exec")
	if len(ex) != 2 || ex[0][2] != "sh" || ex[0][3] != "-c" {
		t.Fatalf("exec = %v", ex)
	}
	if !strings.Contains(ex[0][4], "netem delay 80ms loss 2.50%") || strings.Contains(ex[0][4], "qdisc del") {
		t.Fatalf("apply script = %q", ex[0][4])
	}
	if !strings.Contains(ex[1][4], "qdisc del") || strings.Contains(ex[1][4], "netem") {
		t.Fatalf("clear script = %q", ex[1][4])
	}
	if !strings.Contains(ex[0][4], `[ "$n" = lo ] && continue`) {
		t.Fatal("loopback is not skipped")
	}
}

func TestNetemRange(t *testing.T) {
	_, f, tb := startOne(t, NodeSpec{Name: "a", CapAdd: []string{"NET_ADMIN"}})
	for _, c := range []struct {
		d time.Duration
		l float64
	}{{-1, 0}, {0, -1}, {0, 101}} {
		tb.fatal = ""
		tb.run(func() { f.Netem(tb, "a", c.d, c.l) })
		if !strings.Contains(tb.fatal, "out of range") {
			t.Errorf("Netem(%v,%v): fatal = %q", c.d, c.l, tb.fatal)
		}
	}
}

func TestPartitionHealArgv(t *testing.T) {
	d, f, tb := startOne(t, NodeSpec{Name: "a"})
	f.Partition(tb, "a")
	f.Heal(tb, "a")
	c := "nself-sim-" + f.ID + "-a"
	dis := d.find("network", "disconnect")
	if len(dis) != 1 || dis[0][2] != f.Network || dis[0][3] != c {
		t.Fatalf("disconnect = %v", dis)
	}
	con := d.find("network", "connect")
	if len(con) != 1 || strings.Join(con[0], " ") != "network connect --alias a "+f.Network+" "+c {
		t.Fatalf("connect = %v", con)
	}
}

func TestExecCopyRestartArgv(t *testing.T) {
	d, f, tb := startOne(t, NodeSpec{Name: "a"})
	f.Exec(tb, "a", "echo", "hi there")
	f.CopyTo(tb, "a", "/tmp/src", "/tmp/dst")
	f.Restart(tb, "a")
	c := "nself-sim-" + f.ID + "-a"
	ex := d.find("exec")
	if len(ex) != 2 || strings.Join(ex[0], "|") != "exec|"+c+"|echo|hi there" {
		t.Fatalf("exec = %v", ex)
	}
	if strings.Join(ex[1], "|") != "exec|-u|0|"+c+"|chmod|a+r|/tmp/dst" {
		t.Fatalf("chmod = %v", ex[1])
	}
	if cp := d.find("cp"); len(cp) != 1 || cp[0][2] != c+":/tmp/dst" {
		t.Fatalf("cp = %v", cp)
	}
	if r := d.find("restart"); len(r) != 1 || r[0][len(r[0])-1] != c {
		t.Fatalf("restart = %v", r)
	}
}

func TestUnknownNodeFails(t *testing.T) {
	_, f, tb := startOne(t, NodeSpec{Name: "a"})
	tb.run(func() { f.Pause(tb, "nope") })
	if !strings.Contains(tb.fatal, `no node "nope"`) {
		t.Fatalf("fatal = %q", tb.fatal)
	}
}

func TestBannerFailsWithoutSSH(t *testing.T) {
	_, f, tb := startOne(t, NodeSpec{Name: "a"})
	n := f.Node(tb, "a")
	if b, err := n.Banner(time.Second); err != nil || !strings.HasPrefix(b, "SSH-2.0") {
		t.Fatalf("banner %q, %v", b, err)
	}
	n.Port = 1 // nothing listens
	if _, err := n.Banner(300 * time.Millisecond); err == nil {
		t.Fatal("Banner succeeded against a closed port")
	}
}

// A listener that is not sshd is not a ready node.
func TestBannerRejectsOtherProtocols(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			_, _ = c.Write([]byte("HTTP/1.1 400 Bad Request\r\n"))
			_ = c.Close()
		}
	}()
	n := &Node{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port}
	if b, err := n.Banner(time.Second); err == nil {
		t.Fatalf("Banner accepted %q", b)
	}
}
