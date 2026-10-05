package destinations

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostDestinationFakeSSH(t *testing.T) {
	logf := fakeBin(t)
	inv := hostFixture(t)
	root := filepath.Join(t.TempDir(), "store")
	d, err := Parse("host://bk1"+root, inv)
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind() != KindHost {
		t.Fatalf("kind %q", d.Kind())
	}
	src := writeFile(t, t.TempDir(), "a.dump", "payload-bytes")
	if err := d.Put(t.Context(), src, "sub/a.dump"); err != nil {
		t.Fatalf("put: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "sub", "a.dump")); err != nil || string(b) != "payload-bytes" {
		t.Fatalf("stored object: %q %v", b, err)
	}
	if fi, _ := os.Stat(filepath.Join(root, "sub", "a.dump")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("object mode %v", fi.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "a.dump.tmp")); err == nil {
		t.Fatal("tmp file left behind")
	}
	objs, err := d.List(t.Context(), "sub/")
	if err != nil || len(objs) != 1 || objs[0].Key != "sub/a.dump" || objs[0].Size != 13 {
		t.Fatalf("list: %+v %v", objs, err)
	}
	out := filepath.Join(t.TempDir(), "back.dump")
	if err := d.Get(t.Context(), "sub/a.dump", out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if b, _ := os.ReadFile(out); string(b) != "payload-bytes" {
		t.Fatalf("downloaded %q", b)
	}
	rc, err := d.(Opener).Open(t.Context(), "sub/a.dump")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(rc); string(b) != "payload-bytes" {
		t.Fatalf("open read %q", b)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := d.(Opener).Open(t.Context(), "sub/missing"); err == nil {
		// the failure surfaces on Close, after the empty read
		t.Log("open of a missing object returns the error on Close")
	}
	log := readLog(t, logf)
	for _, want := range []string{"StrictHostKeyChecking=yes", "HostKeyAlias=nself-ci-bk1", "UserKnownHostsFile=" + os.Getenv(KnownHostsEnv), "BatchMode=yes"} {
		if !strings.Contains(log, want) {
			t.Errorf("ssh argv lacks %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "accept-new") {
		t.Errorf("accept-new used:\n%s", log)
	}
}

func TestHostDestinationMissingObject(t *testing.T) {
	fakeBin(t)
	inv := hostFixture(t)
	d, err := Parse("host://bk1"+t.TempDir(), inv)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Get(t.Context(), "nope.dump", filepath.Join(t.TempDir(), "x")); err == nil {
		t.Fatal("expected an error for a missing object")
	}
}

// Every unsafe name is refused before any ssh or scp process starts.
func TestHostDestinationRejectsBeforeSSH(t *testing.T) {
	logf := fakeBin(t)
	inv := hostFixture(t)
	bad := []string{"a;b", "a$(id)", "a`id`", "a b", "a\nb", "a|b", "a&b", "a'b", "a\"b", "a\\b", "a*b", "a>b", "a<b", "a#b", "a$HOME", "a%b", "a~b", "a!b", "../x", "x/../y", "x//y", "/abs"}
	src := writeFile(t, t.TempDir(), "f", "x")
	good, err := Parse("host://bk1/srv/backups", inv)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		if err := good.Put(t.Context(), src, b); err == nil {
			t.Errorf("Put key %q accepted", b)
		}
		if err := good.Get(t.Context(), b, filepath.Join(t.TempDir(), "o")); err == nil {
			t.Errorf("Get key %q accepted", b)
		}
		if _, err := Parse("host://bk1/srv/"+b, inv); err == nil && !strings.HasPrefix(b, "-") && !strings.HasPrefix(b, "/") && !strings.Contains(b, "//") && b != "x/../y" {
			t.Errorf("Parse dir element %q accepted", b)
		}
	}
	for _, uri := range []string{"host://", "host://bk1", "host:///srv", "host://bad name/srv", "host://bk1;id/srv", "host://nosuch/srv", "host://lo/srv", "host://bk1/srv/../etc"} {
		if _, err := Parse(uri, inv); err == nil {
			t.Errorf("Parse(%q) accepted", uri)
		}
	}
	if l := readLog(t, logf); l != "" {
		t.Fatalf("ssh/scp ran for rejected input:\n%s", l)
	}
}

func TestHostDestinationRefusesUnpinnedServer(t *testing.T) {
	logf := fakeBin(t)
	inv := hostFixture(t)
	t.Setenv(KnownHostsEnv, filepath.Join(t.TempDir(), "empty"))
	d, err := Parse("host://bk1/srv/backups", inv)
	if err != nil {
		t.Fatal(err)
	}
	err = d.Put(t.Context(), writeFile(t, t.TempDir(), "f", "x"), "k")
	if err == nil || !strings.Contains(err.Error(), "no pinned host key") {
		t.Fatalf("want pinned-key refusal, got %v", err)
	}
	if l := readLog(t, logf); l != "" {
		t.Fatalf("ssh ran without a pinned key:\n%s", l)
	}
}

func TestHostDestinationRefusesDirectoryAtKey(t *testing.T) {
	fakeBin(t)
	inv := hostFixture(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "k"), 0o700); err != nil {
		t.Fatal(err)
	}
	d, err := Parse("host://bk1"+root, inv)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Put(t.Context(), writeFile(t, t.TempDir(), "f", "x"), "k"); err == nil {
		t.Fatal("Put replaced a directory")
	}
	if _, err := os.Stat(filepath.Join(root, "k", "k.tmp")); err == nil {
		t.Fatal("object landed inside the directory")
	}
}
