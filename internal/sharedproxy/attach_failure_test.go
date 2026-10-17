package sharedproxy

// Tests for the failure exits of attach_flow.go: a call that fails leaves the
// files it restored in quarantine, and quarantines a second broken project.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// failingConnectDocker makes `network connect` fail for $FAIL_NET.
func failingConnectDocker(t *testing.T, network string) {
	t.Helper()
	script := strings.Replace(fakeProxyDocker, `connect) echo "$4" >> "$FAKE_DIR/nets.txt" ;;`,
		`connect) [ "$4" = "$FAIL_NET" ] && { echo "boom" >&2; exit 1; }; echo "$4" >> "$FAKE_DIR/nets.txt" ;;`, 1)
	if script == fakeProxyDocker {
		t.Fatal("the fake docker script changed: update failingConnectDocker")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAIL_NET", network)
}

func (f *proxyFixture) live(project string) bool {
	_, err := os.Stat(filepath.Join(f.owner, "nginx", "sites", project+".api.conf"))
	return err == nil
}

func (f *proxyFixture) quarantined(project string) bool {
	_, err := os.Stat(filepath.Join(DirOf(f.owner), QuarantineDir, project, "nginx", "sites", project+".api.conf"))
	return err == nil
}

// quarantineAll puts every project's files into quarantine, as an earlier
// failed start would have.
func (f *proxyFixture) quarantineAll(regs ...Registration) {
	f.t.Helper()
	set, err := Load(DirOf(f.owner), f.owner)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, r := range regs {
		if _, err := Quarantine(f.owner, set, r); err != nil {
			f.t.Fatal(err)
		}
		if f.live(r.Project) || !f.quarantined(r.Project) {
			f.t.Fatalf("setup: %s is not quarantined", r.Project)
		}
	}
}

func TestAttachFailureAfterRestoreKeepsTheQuarantine(t *testing.T) {
	ctx := context.Background()

	t.Run("connect fails", func(t *testing.T) {
		f := newProxyFixture(t, "b_network", "c_network")
		f.owner0()
		b, c := f.project("b"), f.project("c")
		f.quarantineAll(b, c)
		failingConnectDocker(t, "c_network")
		_, err := Attach(ctx, f.owner, "", []Registration{b, c})
		if !isE506(err) {
			t.Fatalf("want E506, got %v", err)
		}
		for _, p := range []string{"b", "c"} {
			if f.live(p) || !f.quarantined(p) {
				t.Errorf("%s: a failed attach must leave the restored files in quarantine (live=%v quarantined=%v)", p, f.live(p), f.quarantined(p))
			}
		}
	})

	t.Run("nginx -t names no registered file", func(t *testing.T) {
		f := newProxyFixture(t, "b_network")
		f.owner0()
		b := f.project("b")
		f.quarantineAll(b)
		f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/other.conf\n")
		f.write(filepath.Join(f.owner, "nginx", "sites", "other.conf"), "bogus\n")
		// self=b is the fallback culprit; quarantining it does not help.
		_, err := Attach(ctx, f.owner, "b", []Registration{b})
		if !isE506(err) {
			t.Fatalf("want E506, got %v", err)
		}
		if f.live(b.Project) || !f.quarantined(b.Project) {
			t.Errorf("restored files must be back in quarantine (live=%v quarantined=%v)", f.live("b"), f.quarantined("b"))
		}
		if strings.Contains(f.nets(), "b_network") {
			t.Error("the failed attach must disconnect B")
		}
	})

	t.Run("owner start attributes nothing", func(t *testing.T) {
		f := newProxyFixture(t, "b_network")
		f.owner0()
		b := f.project("b")
		f.quarantineAll(b)
		f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/other.conf\n")
		f.write(filepath.Join(f.owner, "nginx", "sites", "other.conf"), "bogus\n")
		if _, err := Attach(ctx, f.owner, "", []Registration{b}); !isE506(err) {
			t.Fatalf("want E506, got %v", err)
		}
		if f.live("b") || !f.quarantined("b") {
			t.Errorf("restored files must be back in quarantine (live=%v quarantined=%v)", f.live("b"), f.quarantined("b"))
		}
	})

	t.Run("a pre-existing live file stays live", func(t *testing.T) {
		f := newProxyFixture(t, "b_network", "c_network")
		f.owner0()
		b := f.project("b")
		c := f.project("c") // live before the call and not restored by it
		f.quarantineAll(b)
		failingConnectDocker(t, "b_network")
		if _, err := Attach(ctx, f.owner, "", []Registration{c, b}); !isE506(err) {
			t.Fatalf("want E506, got %v", err)
		}
		if !f.live("c") {
			t.Error("a file this call did not restore must not be moved")
		}
	})
}

func TestAttachQuarantinesASecondBrokenProject(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	f.project("c")
	f.project("d")
	// nginx -t names the first broken file only, so c is found first, d second.
	f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/c.api.conf\nnginx/sites/d.api.conf\n")
	out, err := Attach(ctx, f.owner, "b", []Registration{b})
	if !isE506(err) {
		t.Fatalf("want E506, got %v", err)
	}
	if strings.Join(out.Quarantined, ",") != "c,d" || len(out.Attached) != 1 || out.Attached[0] != "b" || !out.Reloaded {
		t.Fatalf("outcome %+v: c and d quarantined, b attached, nginx reloaded", out)
	}
	if f.live("c") || f.live("d") || !f.live("b") || !f.quarantined("c") || !f.quarantined("d") {
		t.Errorf("c and d must leave nginx/sites, b must stay")
	}
}

func TestAttachRefusesTheOwnersOwnNetwork(t *testing.T) {
	f := newProxyFixture(t, "owner_network")
	f.owner0()
	self := New("owner", filepath.Join(f.owner, "x"))
	if _, err := Attach(context.Background(), f.owner, "", []Registration{self}); !isE506(err) {
		t.Fatalf("want E506, got %v", err)
	}
	if strings.Contains(f.calls(), "network connect") {
		t.Errorf("the owner's own network must never be connected:\n%s", f.calls())
	}
}
