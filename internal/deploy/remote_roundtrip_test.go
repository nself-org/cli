package deploy

// Fix round 5 (K1, K2, K3): the tar covers every shipped path, a restore fails
// closed, host-reported paths are never trusted raw, and a restore creates nothing
// outside remote_path.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

const pluginFragment = ".nself/plugins/foo/docker-compose.plugin.yml"

// K1: .nself/plugins is a symlink inside remote_path; a fragment under it is in the
// tar and a restore puts v1 back.
func TestPluginsDirSymlinkIsTarredAndRestored(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	writeFile(t, rp+"/.shared-plugins/foo/docker-compose.plugin.yml", "# v1\n", 0o644)
	if err := os.MkdirAll(rp+"/.nself", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.shared-plugins", rp+"/.nself/plugins"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	ctx := context.Background()
	bk, err := CreatePreDeployTar(ctx, f, rp, NewBackupID(time.Now()), []string{pluginFragment})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tarList(t, bk.Tar), pluginFragment) {
		t.Fatalf("the fragment behind the symlinked plugins dir is not in the tar:\n%s", tarList(t, bk.Tar))
	}
	writeFile(t, rp+"/.shared-plugins/foo/docker-compose.plugin.yml", "# v2\n", 0o644)
	if err := RestorePreDeployTar(ctx, f, rp, bk.Tar, RestoreSpec{Changed: []string{pluginFragment}, Absent: bk.Absent}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, rp+"/.shared-plugins/foo/docker-compose.plugin.yml"); got != "# v1\n" {
		t.Errorf("fragment after the restore = %q, want v1", got)
	}
}

// K1: a changed path that is neither in the tar nor recorded as created cannot be
// reverted: the restore fails instead of reporting success.
func TestRestoreFailsClosedOnUnrestorablePath(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	seedDeployedHost(t, rp)
	writeFile(t, rp+"/docs/readme.md", "# mine\n", 0o644) // exists, but the tar never takes docs/
	bk, err := CreatePreDeployTar(context.Background(), f, rp, NewBackupID(time.Now()), []string{"docker-compose.yml"})
	if err != nil {
		t.Fatal(err)
	}
	err = RestorePreDeployTar(context.Background(), f, rp, bk.Tar, RestoreSpec{Changed: []string{"docker-compose.yml", "docs/readme.md"}, Absent: bk.Absent})
	if err == nil || !strings.Contains(err.Error(), "docs/readme.md") || !strings.Contains(err.Error(), "nothing was restored") {
		t.Fatalf("err = %v", err)
	}
}

// K1: an nginx -t failure with a symlinked plugins dir leaves the fragment at v1.
func TestApplyRestoresPluginFragmentThroughSymlink(t *testing.T) {
	f, o, rp := applyFixture(t, "")
	if err := os.Rename(rp+"/.nself/plugins", rp+"/.shared-plugins"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.shared-plugins", rp+"/.nself/plugins"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	frag := ".nself/plugins/ai/docker-compose.plugin.yml"
	before := readFile(t, rp+"/"+frag)
	writeFile(t, o.Set.StageDir+"/"+frag, "services: {ai: {image: v2}}\n", 0o644)
	o.Set.Files = append(o.Set.Files, frag)
	f.fail["docker run --rm"] = errBoom
	res, err := ApplyRemote(context.Background(), o)
	if err == nil || strings.Contains(err.Error(), "also failed") || res.Synced {
		t.Fatalf("want a clean restored refusal: res %+v err %v", res, err)
	}
	if got := readFile(t, rp+"/"+frag); got != before {
		t.Errorf("fragment = %q after the restore, want %q", got, before)
	}
}

// K2: a host-reported real path with a control character (here a newline, which
// would cut the path short in the line protocol) is refused; notices print %q.
func TestHostRealPathWithControlCharsRefused(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	if err := os.MkdirAll(rp+"/x\ny", 0o755); err != nil {
		t.Skipf("odd names unavailable: %v", err)
	}
	if err := os.Symlink("x\ny", rp+"/nginx"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := ResolveDirs(context.Background(), f, rp, []string{"nginx", "nginx/sites"})
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E485" || !strings.Contains(err.Error(), "symlink outside remote_path") {
		t.Fatalf("err = %v", err)
	}
	var out strings.Builder
	noticeLinks(&out, map[string]string{"nginx/sites": "/p/\x1b[31mred"}, []string{"nginx/sites/a.conf"})
	if strings.Contains(out.String(), "\x1b") || !strings.Contains(out.String(), `"nginx/sites"`) {
		t.Errorf("notice must print the host path with %%q: %q", out.String())
	}
}

// K3: a link swapped to outside between the deploy and the restore: refused, and no
// directory is created outside remote_path.
func TestRestoreCreatesNothingOutside(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp, outside := t.TempDir(), t.TempDir()
	seedDeployedHost(t, rp)
	ctx := context.Background()
	changed := []string{"nginx/sites/api.conf"}
	bk, err := CreatePreDeployTar(ctx, f, rp, NewBackupID(time.Now()), changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(rp + "/nginx"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, rp+"/nginx"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	err = RestorePreDeployTar(ctx, f, rp, bk.Tar, RestoreSpec{Changed: changed, Absent: bk.Absent})
	if err == nil || !strings.Contains(err.Error(), "outside remote_path") {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("the restore created %d entries outside remote_path", len(entries))
	}
}
