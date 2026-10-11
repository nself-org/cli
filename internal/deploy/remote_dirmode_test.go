package deploy

// G11 (RR-S1): a restore never changes the mode of remote_path or of any
// directory that already exists, whatever the tar holds.

import (
	"context"
	"os"
	"testing"
	"time"
)

func modeOf(t *testing.T, p string) os.FileMode {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestRestoreKeepsDirModes(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	if err := os.Chmod(rp, 0o755); err != nil {
		t.Fatal(err)
	}
	seedDeployedHost(t, rp)
	ctx := context.Background()
	changed := []string{"docker-compose.yml", "nginx/sites/api.conf", "nginx/sites/new.conf"}
	bk, err := CreatePreDeployTar(ctx, f, rp, NewBackupID(time.Now()), changed)
	if err != nil {
		t.Fatal(err)
	}
	// The operator tightened a directory after the tar was taken.
	for p, m := range map[string]os.FileMode{rp + "/nginx": 0o711, rp + "/nginx/sites": 0o750} {
		if err := os.Chmod(p, m); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, rp+"/docker-compose.yml", "# v2\n", 0o644)
	writeFile(t, rp+"/nginx/sites/api.conf", "# v2\n", 0o644)
	if err := RestorePreDeployTar(ctx, f, rp, bk.Tar, RestoreSpec{Changed: changed, Absent: bk.Absent}); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{rp: 0o755, rp + "/nginx": 0o711, rp + "/nginx/sites": 0o750} {
		if got := modeOf(t, p); got != want {
			t.Errorf("%s mode %o after the restore, want %o", p, got, want)
		}
	}
	// An empty tar (a fresh host) with only absent paths: remote_path is untouched too.
	g := newFakeRemote(t)
	fresh := t.TempDir()
	if err := os.Chmod(fresh, 0o755); err != nil {
		t.Fatal(err)
	}
	ebk, err := CreatePreDeployTar(ctx, g, fresh, NewBackupID(time.Now()), []string{"docker-compose.yml"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, fresh+"/docker-compose.yml", "new\n", 0o644)
	if err := RestorePreDeployTar(ctx, g, fresh, ebk.Tar, RestoreSpec{Changed: []string{"docker-compose.yml"}, Absent: ebk.Absent}); err != nil {
		t.Fatal(err)
	}
	if exists(fresh+"/docker-compose.yml") || modeOf(t, fresh) != 0o755 {
		t.Errorf("fresh host: file removed=%v mode %o", !exists(fresh+"/docker-compose.yml"), modeOf(t, fresh))
	}
}

// G3: the sync writes a symlinked directory's files to its real path (rsync >= 3.4
// will not traverse the link), so the link survives and nothing else is touched.
func TestSyncWritesThroughSymlinkedDir(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp, shared, stage := t.TempDir(), t.TempDir(), t.TempDir()
	writeFile(t, shared+"/sites/api.conf", "# old\n", 0o644)
	writeFile(t, shared+"/sites/foreign.conf", "# foreign\n", 0o644)
	if err := os.Symlink(shared, rp+"/nginx"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeFile(t, stage+"/nginx/sites/api.conf", "# new\n", 0o644)
	writeFile(t, stage+"/docker-compose.yml", "# new\n", 0o644)
	writeFile(t, stage+"/hasura/metadata/version.yaml", "version: 3\n", 0o644)
	set := ArtifactSet{StageDir: stage, RemotePath: rp, Files: []string{"docker-compose.yml", "nginx/sites/api.conf", "hasura/metadata/version.yaml"}}
	if err := SyncArtifacts(context.Background(), f, set); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(rp + "/nginx"); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("nginx/ is no longer a symlink: %v %v", info, err)
	}
	if readFile(t, shared+"/sites/api.conf") != "# new\n" || readFile(t, shared+"/sites/foreign.conf") != "# foreign\n" {
		t.Error("the file was not written through the link, or a foreign file changed")
	}
	if readFile(t, rp+"/docker-compose.yml") != "# new\n" || readFile(t, rp+"/hasura/metadata/version.yaml") != "version: 3\n" {
		t.Error("files outside the link were not synced")
	}
}
