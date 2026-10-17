package sharedproxy

// Tests for attach.go, quarantine.go and leave.go. A fake `docker` shell
// script goes first on PATH. It keeps state in $FAKE_DIR: the networks that
// exist (existing.txt), the networks the owner nginx is on (nets.txt), the
// owner container's state (state; default running; `docker start` and `docker
// stop` rewrite it), the calls (calls.log) and a list of owner-relative files that make `nginx -t`
// fail while they exist (broken.txt).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

const fakeProxyDocker = `#!/bin/sh
echo "$@" >> "$FAKE_DIR/calls.log"
nets_json() { s=""; while read n; do [ -n "$n" ] && s="$s${s:+,}\"$n\":{}"; done < "$FAKE_DIR/nets.txt"; printf '{%s}' "$s"; }
case "$1" in
  ps) [ -f "$FAKE_DIR/stopped" ] || echo ownerid ;;
  inspect)
    case "$3" in
      '{{json .NetworkSettings.Networks}}') nets_json ;;
      *) st=$(cat "$FAKE_DIR/state" 2>/dev/null || echo running)
         printf '{"Id":"ownerid","Name":"/owner_nginx","State":{"Status":"%s"},"Config":{"Labels":{"com.docker.compose.service":"nginx","com.docker.compose.project.working_dir":"%s"}}}' "$st" "$OWNER_DIR" ;;
    esac ;;
  start) echo running > "$FAKE_DIR/state" ;;
  stop) echo exited > "$FAKE_DIR/state" ;;
  network)
    case "$2" in
      inspect) n=$(eval echo \$$#); if grep -qx "$n" "$FAKE_DIR/existing.txt"; then echo "$n"; else echo "Error: No such network: $n" >&2; exit 1; fi ;;
      connect) echo "$4" >> "$FAKE_DIR/nets.txt" ;;
      disconnect) grep -vx "$4" "$FAKE_DIR/nets.txt" > "$FAKE_DIR/nets.new"; mv "$FAKE_DIR/nets.new" "$FAKE_DIR/nets.txt" ;;
    esac ;;
  exec)
    if [ "$4" = "-t" ]; then
      while read f; do
        if [ -e "$OWNER_DIR/$f" ]; then echo "nginx: [emerg] unknown directive \"bogus\" in /etc/nginx/$(echo $f | sed 's#^nginx/##'):3" >&2; echo "nginx: configuration file /etc/nginx/nginx.conf test failed" >&2; exit 1; fi
      done < "$FAKE_DIR/broken.txt"
      echo "nginx: configuration file /etc/nginx/nginx.conf test is successful" >&2
    elif [ "$5" = reload ]; then echo reload >> "$FAKE_DIR/reloads.log"; fi ;;
esac
`

type proxyFixture struct {
	t     *testing.T
	owner string
	fake  string
}

func newProxyFixture(t *testing.T, existing ...string) *proxyFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	f := &proxyFixture{t: t, owner: t.TempDir(), fake: t.TempDir()}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(fakeProxyDocker), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DIR", f.fake)
	t.Setenv("OWNER_DIR", f.owner)
	f.write(filepath.Join(f.fake, "existing.txt"), strings.Join(existing, "\n")+"\n")
	f.write(filepath.Join(f.fake, "nets.txt"), "bridge\n")
	f.write(filepath.Join(f.fake, "broken.txt"), "")
	return f
}

func (f *proxyFixture) write(path, body string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// project registers project with one site file and one certificate pair and
// writes those files into the owner.
func (f *proxyFixture) project(name string) Registration {
	f.t.Helper()
	r := New(name, filepath.Join(f.owner, name))
	r.Files = []string{"nginx/sites/" + name + ".api.conf", "ssl/certificates/" + name + "-cert/fullchain.pem", "ssl/certificates/" + name + "-cert/privkey.pem"}
	r.ServerNames = []string{"api." + name + ".test"}
	if err := Write(DirOf(f.owner), FileName(name), r); err != nil {
		f.t.Fatal(err)
	}
	f.write(filepath.Join(f.owner, "nginx", "sites", name+".api.conf"), "server {}\n")
	f.write(filepath.Join(f.owner, "ssl", "certificates", name+"-cert", "fullchain.pem"), "cert\n")
	f.write(filepath.Join(f.owner, "ssl", "certificates", name+"-cert", "privkey.pem"), "key\n")
	f.write(filepath.Join(f.owner, "ssl", "certificates", name+"-cert", CertMarker), name+"\n")
	if err := os.Chmod(filepath.Join(f.owner, "ssl", "certificates", name+"-cert", "privkey.pem"), 0o600); err != nil {
		f.t.Fatal(err)
	}
	return r
}

func (f *proxyFixture) owner0() {
	f.t.Helper()
	o := New("owner", f.owner)
	if err := Write(DirOf(f.owner), OwnerFile, o); err != nil {
		f.t.Fatal(err)
	}
}

func (f *proxyFixture) calls() string {
	b, _ := os.ReadFile(filepath.Join(f.fake, "calls.log"))
	return string(b)
}

func (f *proxyFixture) reloads() int {
	b, _ := os.ReadFile(filepath.Join(f.fake, "reloads.log"))
	return strings.Count(string(b), "reload")
}

func (f *proxyFixture) nets() string {
	b, _ := os.ReadFile(filepath.Join(f.fake, "nets.txt"))
	return string(b)
}

func isE506(err error) bool {
	var ce *errs.CLIError
	return errors.As(err, &ce) && ce.Code == "E506"
}

func TestAttach(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")

	out, err := Attach(ctx, f.owner, "b", []Registration{b})
	if err != nil || len(out.Attached) != 1 || !out.Reloaded {
		t.Fatalf("first attach: %+v %v", out, err)
	}
	if !strings.Contains(f.nets(), "b_network") || strings.Count(f.calls(), "network connect") != 1 || f.reloads() != 1 {
		t.Errorf("first attach must connect once and reload once:\n%s", f.calls())
	}

	// Idempotent: no new connection, one more reload.
	if _, err := Attach(ctx, f.owner, "b", []Registration{b}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(f.calls(), "network connect") != 1 || f.reloads() != 2 {
		t.Errorf("a second attach must not connect again and must reload once:\n%s", f.calls())
	}

	// A registered project whose network is gone is skipped, with no reload.
	c := f.project("c")
	before := f.reloads()
	out, err = Attach(ctx, f.owner, "", []Registration{c})
	if err != nil || len(out.Missing) != 1 || out.Missing[0] != "c" || f.reloads() != before {
		t.Errorf("missing network: %+v %v reloads %d", out, err, f.reloads())
	}

	// A registration that names a foreign network is refused before any connect.
	bad := b
	bad.Network = "other_network"
	calls := strings.Count(f.calls(), "network connect")
	if _, err := Attach(ctx, f.owner, "b", []Registration{bad}); !isE506(err) || strings.Count(f.calls(), "network connect") != calls {
		t.Errorf("a foreign network must be E506 with no connect: %v", err)
	}

	// The owner's nginx is not running.
	f.write(filepath.Join(f.fake, "stopped"), "")
	if _, err := Attach(ctx, f.owner, "b", []Registration{b}); !errors.Is(err, ErrOwnerNotRunning) || isE506(err) {
		t.Errorf("a stopped owner is ErrOwnerNotRunning, not E506: %v", err)
	}
}

func TestAttachNginxTestFailureRollsBack(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	// The error names no registered file, and the owner attaches everyone.
	f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/unregistered.conf\n")
	f.write(filepath.Join(f.owner, "nginx", "sites", "unregistered.conf"), "bogus\n")

	out, err := Attach(ctx, f.owner, "", []Registration{b})
	if !isE506(err) || len(out.Attached) != 0 || len(out.Quarantined) != 0 || f.reloads() != 0 {
		t.Fatalf("an unattributed failure: %+v %v reloads %d", out, err, f.reloads())
	}
	if strings.Contains(f.nets(), "b_network") {
		t.Error("the failed attach must disconnect again")
	}
	if _, err := os.Stat(filepath.Join(f.owner, "nginx", "sites", "b.api.conf")); err != nil {
		t.Error("nothing may be quarantined without attribution")
	}
}

func TestDetach(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	if _, err := Attach(ctx, f.owner, "b", []Registration{b}); err != nil {
		t.Fatal(err)
	}
	if err := Detach(ctx, f.owner, b); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.nets(), "b_network") {
		t.Error("Detach must disconnect the network")
	}
	reloads := f.reloads()
	if err := Detach(ctx, f.owner, b); err != nil || f.reloads() != reloads {
		t.Errorf("a second Detach is a no-op without reload: %v", err)
	}
	f.write(filepath.Join(f.fake, "stopped"), "")
	if err := Detach(ctx, f.owner, b); !errors.Is(err, ErrOwnerNotRunning) {
		t.Errorf("a stopped owner: %v", err)
	}
}

func TestQuarantineAttribution(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network", "c_network")
	f.owner0()
	b := f.project("b")
	c := f.project("c")
	f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/c.api.conf\n")

	// R3: a start in B, a broken conf of C: C is quarantined, B attaches.
	out, err := Attach(ctx, f.owner, "b", []Registration{b})
	if !isE506(err) {
		t.Fatalf("want E506, got %v", err)
	}
	if len(out.Quarantined) != 1 || out.Quarantined[0] != "c" || len(out.Attached) != 1 || out.Attached[0] != "b" || !out.Reloaded {
		t.Fatalf("outcome %+v: C must be quarantined, B attached, nginx reloaded", out)
	}
	qdir := filepath.Join(DirOf(f.owner), QuarantineDir, "c")
	if _, err := os.Stat(filepath.Join(qdir, "nginx", "sites", "c.api.conf")); err != nil {
		t.Errorf("C's conf is not in the quarantine tree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.owner, "nginx", "sites", "c.api.conf")); err == nil {
		t.Error("C's conf must leave nginx/sites")
	}
	if _, err := os.Stat(filepath.Join(f.owner, "nginx", "sites", "b.api.conf")); err != nil {
		t.Errorf("B's conf must stay: %v", err)
	}
	for path, want := range map[string]os.FileMode{
		filepath.Join(DirOf(f.owner), QuarantineDir): 0o700,
		qdir: 0o700,
		filepath.Join(qdir, "ssl", "certificates", "c-cert"):                0o700,
		filepath.Join(qdir, "ssl", "certificates", "c-cert", "privkey.pem"): 0o600,
	} {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s mode = %v (%v), want %v", path, fi.Mode().Perm(), err, want)
		}
	}
	if _, err := Load(DirOf(f.owner), f.owner); err != nil {
		t.Errorf("the quarantine tree must not break Load: %v", err)
	}

	// Fixed and rebuilt: the next attach of C restores nothing over a new file
	// and drops the stale copy; with no new file it restores the old one.
	f.write(filepath.Join(f.fake, "broken.txt"), "")
	if _, err := Attach(ctx, f.owner, "c", []Registration{c}); err != nil {
		t.Fatalf("attach after the fix: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.owner, "nginx", "sites", "c.api.conf")); err != nil {
		t.Errorf("C's conf must be restored: %v", err)
	}
	if _, err := os.Stat(qdir); err == nil {
		t.Error("the emptied quarantine dir of C must be removed")
	}

	// Fallback: the output names no registered file, so the attaching project is
	// the suspect. Quarantining it does not help (the test still fails), so its
	// files are put back and the attach is undone.
	f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/other.conf\n")
	f.write(filepath.Join(f.owner, "nginx", "sites", "other.conf"), "bogus\n")
	reloads := f.reloads()
	out, err = Attach(ctx, f.owner, "b", []Registration{b})
	if !isE506(err) || len(out.Quarantined) != 0 || len(out.Attached) != 0 || f.reloads() != reloads {
		t.Errorf("a wrong attribution: %+v %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(f.owner, "nginx", "sites", "b.api.conf")); err != nil {
		t.Errorf("B's conf must be back after a failed quarantine: %v", err)
	}
	if strings.Contains(f.nets(), "b_network") {
		t.Error("the failed attach must disconnect B")
	}
	// The same failure, but the fallback culprit's removal fixes the test.
	f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/b.api.conf\n")
	out, err = Attach(ctx, f.owner, "b", []Registration{b})
	if !isE506(err) || len(out.Quarantined) != 1 || out.Quarantined[0] != "b" || len(out.Attached) != 0 {
		t.Errorf("the attaching project's own broken conf: %+v %v", out, err)
	}
}

func TestQuarantineRefusesSymlinks(t *testing.T) {
	f := newProxyFixture(t)
	f.owner0()
	b := f.project("b")
	site := filepath.Join(f.owner, "nginx", "sites", "b.api.conf")
	if err := os.Remove(site); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(f.owner, "elsewhere.conf")
	f.write(target, "x")
	if err := os.Symlink(target, site); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	set, _ := Load(DirOf(f.owner), f.owner)
	moved, err := Quarantine(f.owner, set, b)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range moved {
		if m == "nginx/sites/b.api.conf" {
			t.Error("a symlinked conf must never be moved")
		}
	}
	if _, err := os.Lstat(site); err != nil {
		t.Error("the symlink must stay where it was")
	}
}

func TestUnregisterKeepsACMEPairs(t *testing.T) {
	root := t.TempDir()
	dir := DirOf(root)
	gen := filepath.Join(root, "ssl", "certificates", ".b-acme.gen-3")
	if err := os.MkdirAll(gen, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"fullchain.pem", "privkey.pem"} {
		if err := os.WriteFile(filepath.Join(gen, n), []byte("acme"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(gen, filepath.Join(root, "ssl", "certificates", "b-acme")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	b := New("b", filepath.Join(root, "b"))
	b.Files = []string{"ssl/certificates/b-acme/fullchain.pem", "ssl/certificates/b-acme/privkey.pem"}
	if err := Write(dir, FileName("b"), b); err != nil {
		t.Fatal(err)
	}
	removed, err := Unregister(root, "b")
	if err != nil || len(removed) != 0 {
		t.Fatalf("removed %v err %v: an ACME pair must never be removed", removed, err)
	}
	for _, n := range []string{"fullchain.pem", "privkey.pem"} {
		if _, err := os.Stat(filepath.Join(gen, n)); err != nil {
			t.Errorf("%s was removed: %v", n, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "ssl", "certificates", "b-acme")); err != nil {
		t.Errorf("the ACME symlink was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName("b"))); err == nil {
		t.Error("the registration itself must go")
	}
}

func TestLeave(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	if _, err := Attach(ctx, f.owner, "b", []Registration{b}); err != nil {
		t.Fatal(err)
	}
	reloads := f.reloads()
	removed, err := Leave(ctx, f.owner, "b")
	if err != nil || len(removed) == 0 {
		t.Fatalf("leave: %v %v", removed, err)
	}
	if strings.Contains(f.nets(), "b_network") || f.reloads() != reloads+1 {
		t.Errorf("leave must detach and reload once:\n%s", f.calls())
	}
	if _, err := os.Stat(filepath.Join(f.owner, "nginx", "sites", "b.api.conf")); err == nil {
		t.Error("B's conf must be gone")
	}
	if removed, err := Leave(ctx, f.owner, "b"); err != nil || removed != nil {
		t.Errorf("a second leave is a no-op: %v %v", removed, err)
	}
}

func TestLocate(t *testing.T) {
	root := t.TempDir()
	owner := filepath.Join(root, "a")
	child := filepath.Join(owner, "b")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if s, err := Locate(child); err != nil || s.Role != RoleNone {
		t.Fatalf("no owner file: %+v %v", s, err)
	}
	if err := Write(DirOf(owner), OwnerFile, New("a", owner)); err != nil {
		t.Fatal(err)
	}
	if s, _ := Locate(child); s.Role != RoleNone {
		t.Errorf("an unregistered child is not part of the proxy: %+v", s)
	}
	if err := Write(DirOf(owner), FileName("b"), New("b", child)); err != nil {
		t.Fatal(err)
	}
	s, err := Locate(child)
	if err != nil || s.Role != RoleFronted || s.Reg == nil || s.Reg.Project != "b" || s.OwnerDir != owner {
		t.Errorf("fronted child: %+v %v", s, err)
	}
	if s, _ := Locate(owner); s.Role != RoleOwner || len(s.Set.Projects) != 1 {
		t.Errorf("owner: %+v", s)
	}
}
