package destinations

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Purpose: tests for Streamer, Remover and the inline-credential refusal. All
// credential-looking values are obviously fake.

const (
	fakeKeyID  = "FAKEKEYID000000000000"
	fakeSecret = "fake-secret-not-real"
)

// failReader yields data, then fails.
type failReader struct {
	data string
	sent bool
}

func (f *failReader) Read(p []byte) (int, error) {
	if !f.sent {
		f.sent = true
		return copy(p, f.data), nil
	}
	return 0, errors.New("source went away")
}

func TestHasInlineCredentialsAndParseRefusals(t *testing.T) {
	cases := []struct {
		uri    string
		inline bool
		ok     bool
	}{
		{"-x", false, false},
		{"--config=/tmp/x", false, false},
		{":s3:b", false, false},
		{":s3," + "access_key_id=" + fakeKeyID + ",secret_access_key=" + fakeSecret + ":bucket", true, false},
		{"myr,pass=" + fakeSecret + ":p", true, false},
		{`:s3,secret_access_key="` + fakeSecret + `:x":b`, true, false},
		{"s3://" + fakeKeyID + ":" + fakeSecret + "@bucket/p", true, false},
		{"file:///etc/x", false, false},
		{"http://h/x", false, false},
		{"s3://" + fakeKeyID + ":fake/secret-not-real@bucket/p", true, false},
		{"minio://http://u:" + fakeSecret + "@h/b", true, false},
		{"az://acct:fake/key@c", true, false},
		{"s3:" + fakeKeyID + ":" + fakeSecret + "@b", true, false},
		{"s3://user@bucket", true, false},
		{"S3://k:" + fakeSecret + "@b", true, false},
		{" s3://k:" + fakeSecret + "@b", true, false},
		{"s3://bucket/p@x", false, true},
		{"s3://bucket/a:b@c", false, true},
		{"name:path", false, true},
		{"s3://bucket/p", false, true},
		{"path:///abs", false, true},
		{"path:///abs,with,commas@x", false, true},
	}
	for _, c := range cases {
		if got := HasInlineCredentials(c.uri); got != c.inline {
			t.Errorf("HasInlineCredentials(%q) = %v, want %v", c.uri, got, c.inline)
		}
		d, err := Parse(c.uri, nil)
		if (err == nil) != c.ok {
			t.Errorf("Parse(%q) err = %v, want ok=%v", c.uri, err, c.ok)
		}
		if err != nil {
			if strings.Contains(err.Error(), "secret-not-real") || strings.Contains(err.Error(), fakeKeyID) {
				t.Errorf("Parse(%q) error echoes the value: %v", c.uri, err)
			}
			if c.inline && !strings.Contains(err.Error(), "inline") {
				t.Errorf("Parse(%q) error should say inline: %v", c.uri, err)
			}
			continue
		}
		if _, ok := d.(Streamer); !ok {
			t.Errorf("%q: not a Streamer", c.uri)
		}
		if _, ok := d.(Remover); !ok {
			t.Errorf("%q: not a Remover", c.uri)
		}
	}
	// host:// carries no credentials and is still a Streamer and Remover.
	skipWindows(t)
	d, err := Parse("host://bk1/srv/b", hostFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.(Streamer); !ok {
		t.Error("host: not a Streamer")
	}
	if _, ok := d.(Remover); !ok {
		t.Error("host: not a Remover")
	}
}

func TestPathPutStream(t *testing.T) {
	old := syscallUmask(0)
	defer syscallUmask(old)
	d, dir := newPath(t)
	st := d.(Streamer)
	if err := st.PutStream(t.Context(), strings.NewReader("streamed bytes"), "p/a.bin"); err != nil {
		t.Fatal(err)
	}
	obj := filepath.Join(dir, "p", "a.bin")
	if b, err := os.ReadFile(obj); err != nil || string(b) != "streamed bytes" {
		t.Fatalf("object %q %v", b, err)
	}
	if fi, _ := os.Stat(obj); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	if _, err := os.Stat(obj + ".tmp"); err == nil {
		t.Error("tmp left behind")
	}
	if err := st.PutStream(t.Context(), strings.NewReader("x"), "../escape"); err == nil {
		t.Error("an escaping key was accepted")
	}
}

func TestPathPutStreamFailureLeavesNothing(t *testing.T) {
	d, dir := newPath(t)
	st := d.(Streamer)
	if err := st.PutStream(t.Context(), &failReader{data: "partial"}, "a.bin"); err == nil {
		t.Fatal("a failing reader succeeded")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := st.PutStream(ctx, strings.NewReader("never"), "b.bin"); err == nil {
		t.Fatal("a cancelled context succeeded")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("leftovers: %v", entries)
	}
}

func TestPathRemove(t *testing.T) {
	d, dir := newPath(t)
	rm := d.(Remover)
	if err := rm.Remove(t.Context(), "gone.bin"); err != nil {
		t.Fatalf("missing object (no dir yet): %v", err)
	}
	if err := d.(Streamer).PutStream(t.Context(), strings.NewReader("data"), "a.bin"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "a.bin.tmp", "partial")
	if err := rm.Remove(t.Context(), "a.bin"); err != nil {
		t.Fatal(err)
	}
	if err := rm.Remove(t.Context(), "a.bin"); err != nil {
		t.Fatalf("second remove: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("leftovers: %v", entries)
	}
	if err := rm.Remove(t.Context(), "/abs"); err == nil {
		t.Error("an invalid key was accepted")
	}
}

// fakeRcloneStream logs argv and stdin of rcat; deletefile exits $FAKE_DEL_EXIT.
func fakeRcloneStream(t *testing.T, delExit string) string {
	t.Helper()
	skipWindows(t)
	dir := t.TempDir()
	body := "#!/bin/sh\nd=$(dirname \"$0\")\necho \"$*\" >> \"$d/argv\"\n" +
		"case \"$1\" in rcat) cat > \"$d/stdin\";; deletefile) echo gone >&2; exit " + delExit + ";; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "rclone"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func TestRclonePutStreamArgv(t *testing.T) {
	dir := fakeRcloneStream(t, "0")
	for _, remote := range []string{"s3:b/p", "s3:b/p/"} {
		d, err := Parse(remote, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := d.(Streamer).PutStream(t.Context(), strings.NewReader("body"), "k.sql.age"); err != nil {
			t.Fatal(err)
		}
	}
	want := "rcat s3:b/p/k.sql.age\nrcat s3:b/p/k.sql.age"
	if g := readLog(t, filepath.Join(dir, "argv")); g != want {
		t.Fatalf("argv\n got: %s\nwant: %s", g, want)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "stdin")); string(b) != "body" {
		t.Errorf("stdin %q", b)
	}
}

func TestRcloneRemoveAndFailure(t *testing.T) {
	dir := fakeRcloneStream(t, "4")
	d, _ := Parse("r2:bkt/x", nil)
	if err := d.(Remover).Remove(t.Context(), "k"); err != nil {
		t.Fatalf("exit 4 should count as removed: %v", err)
	}
	if g := readLog(t, filepath.Join(dir, "argv")); g != "deletefile r2:bkt/x/k" {
		t.Errorf("argv %q", g)
	}
	fakeRcloneStream(t, "1")
	if err := d.(Remover).Remove(t.Context(), "k"); err == nil {
		t.Error("exit 1 should fail")
	}
	// A failing rcat carries the trimmed stderr.
	bad := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\necho '  boom from rclone  ' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bad, "rclone"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bad+string(os.PathListSeparator)+os.Getenv("PATH"))
	err := d.(Streamer).PutStream(t.Context(), strings.NewReader("x"), "k")
	if err == nil || !strings.Contains(err.Error(), "boom from rclone") {
		t.Errorf("PutStream error %v", err)
	}
}

func TestHostPutStreamFakeSSH(t *testing.T) {
	fakeBin(t)
	inv := hostFixture(t)
	root := filepath.Join(t.TempDir(), "store")
	d, err := Parse("host://bk1"+root, inv)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.(Streamer).PutStream(t.Context(), strings.NewReader("host streamed"), "sub/a.bin"); err != nil {
		t.Fatalf("PutStream: %v", err)
	}
	obj := filepath.Join(root, "sub", "a.bin")
	if b, err := os.ReadFile(obj); err != nil || string(b) != "host streamed" {
		t.Fatalf("object %q %v", b, err)
	}
	if fi, _ := os.Stat(obj); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	if _, err := os.Stat(obj + ".tmp"); err == nil {
		t.Error("tmp left behind")
	}
	rc, err := d.(Opener).Open(t.Context(), "sub/a.bin")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(rc); string(b) != "host streamed" {
		t.Errorf("round trip %q", b)
	}
	_ = rc.Close()
	// A reader that fails mid-way leaves no object and no tmp.
	if err := d.(Streamer).PutStream(t.Context(), &failReader{data: "half"}, "sub/b.bin"); err == nil {
		t.Fatal("a failing reader succeeded")
	}
	for _, n := range []string{"b.bin", "b.bin.tmp"} {
		if _, err := os.Stat(filepath.Join(root, "sub", n)); err == nil {
			t.Errorf("%s left behind", n)
		}
	}
	if err := d.(Remover).Remove(t.Context(), "sub/a.bin"); err != nil {
		t.Fatal(err)
	}
	if err := d.(Remover).Remove(t.Context(), "sub/a.bin"); err != nil {
		t.Fatalf("second remove: %v", err)
	}
	if _, err := os.Stat(obj); err == nil {
		t.Error("object still there")
	}
}

// TestHostPutStreamErrorHasNoEmptyGap: with no ssh stderr the error text has
// no empty "key on server: : " part.
func TestHostPutStreamErrorHasNoEmptyGap(t *testing.T) {
	fakeBin(t)
	inv := hostFixture(t)
	// A regular file where the directory should be: mkdir fails with stderr.
	blocker := writeFile(t, t.TempDir(), "file", "x")
	d, err := Parse("host://bk1"+blocker+"/sub", inv)
	if err != nil {
		t.Fatal(err)
	}
	err = d.(Streamer).PutStream(t.Context(), strings.NewReader("x"), "k.bin")
	if err == nil {
		t.Fatal("expected a failure")
	}
	if strings.Contains(err.Error(), ": : ") {
		t.Errorf("empty gap in %q", err)
	}
}
