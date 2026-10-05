package destinations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRclone logs its argv (one line per call) and creates the destination of
// a copyto, so the argv rclone receives can be compared with origin/main's.
func fakeRclone(t *testing.T) string {
	t.Helper()
	skipWindows(t)
	dir := t.TempDir()
	body := "#!/bin/sh\nd=$(dirname \"$0\")\necho \"$*\" >> \"$d/argv\"\necho \"AWS=$AWS_ACCESS_KEY_ID\" >> \"$d/argv\"\n" +
		"if [ \"$1\" = copyto ]; then case \"$3\" in /*) : > \"$3\";; esac; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "rclone"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return filepath.Join(dir, "argv")
}

func TestRcloneArgvGolden(t *testing.T) {
	logf := fakeRclone(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	d, err := Parse("s3:bucket/prefix", nil, "AWS_ACCESS_KEY_ID=AKIDTEST")
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind() != KindRclone {
		t.Fatalf("kind %q", d.Kind())
	}
	if err := d.Put(t.Context(), "/data/p_full_1.dump", "p_full_1.dump"); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	got, err := FetchRemote(t.Context(), "r2://bkt/arch/base.tar.gz", dest)
	if err != nil || got != filepath.Join(dest, "base.tar.gz") {
		t.Fatalf("FetchRemote: %q %v", got, err)
	}
	// Recorded from origin/main 2a8337c4 (uploadToRemote and FetchRemote).
	want := strings.Join([]string{
		"copyto /data/p_full_1.dump s3:bucket/prefix/p_full_1.dump",
		"AWS=AKIDTEST",
		"copyto r2:bkt/arch/base.tar.gz " + filepath.Join(dest, "base.tar.gz"),
		"AWS=",
	}, "\n")
	if g := readLog(t, logf); g != want {
		t.Fatalf("rclone argv changed\n got: %s\nwant: %s", g, want)
	}
}

func TestRcloneGetAndKinds(t *testing.T) {
	logf := fakeRclone(t)
	d, _ := Parse("minio://b/p", nil)
	out := filepath.Join(t.TempDir(), "o")
	if err := d.Get(t.Context(), "k.dump", out); err != nil {
		t.Fatal(err)
	}
	if g := readLog(t, logf); !strings.HasPrefix(g, "copyto minio:b/p/k.dump "+out) {
		t.Fatalf("argv %q", g)
	}
	for uri, k := range map[string]string{"": "", "s3://b/k": KindRclone, "r2:b/k": KindRclone, "path:///x": KindPath, "host://s/x": KindHost} {
		if KindOf(uri) != k {
			t.Errorf("KindOf(%q) = %q want %q", uri, KindOf(uri), k)
		}
	}
	if len(Kinds()) != 3 {
		t.Fatal("Kinds must list three kinds")
	}
}

func TestParseObject(t *testing.T) {
	skipWindows(t)
	d, key, err := ParseObject("path:///mnt/b/sub/x.dump.age", nil)
	if err != nil || key != "x.dump.age" || d.(*pathDest).dir != "/mnt/b/sub" {
		t.Fatalf("%v %q %v", d, key, err)
	}
	for _, u := range []string{"path:///mnt/b/", "path://", "path:///.."} {
		if _, _, err := ParseObject(u, nil); err == nil {
			t.Errorf("ParseObject(%q) accepted", u)
		}
	}
	if _, key, err := ParseObject("s3://b/k", nil); err != nil || key != "" {
		t.Fatalf("rclone: %q %v", key, err)
	}
}
