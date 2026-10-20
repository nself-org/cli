package deploy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func needTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"sh", "tar", "rsync", "find", "grep"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
}

func seedDeployedHost(t *testing.T, rp string) {
	t.Helper()
	writeFile(t, filepath.Join(rp, "docker-compose.yml"), "services: {a: {}}\n# v1\n", 0o644)
	writeFile(t, filepath.Join(rp, ".env"), "SECRET=1\n", 0o600)
	writeFile(t, filepath.Join(rp, ".env.prod"), "ENV=prod\n", 0o600)
	writeFile(t, filepath.Join(rp, ".nself", "compose-files.txt"), rp+"/docker-compose.yml\n", 0o644)
	writeFile(t, filepath.Join(rp, ".nself", "plugins", "ai", "docker-compose.plugin.yml"), "services: {ai: {}}\n", 0o644)
	writeFile(t, filepath.Join(rp, ".nself", "plugins", "ai", "Dockerfile"), "FROM scratch\n", 0o644)
	writeFile(t, filepath.Join(rp, "nginx", "nginx.conf"), markedConf+"# v1\n", 0o644)
	writeFile(t, filepath.Join(rp, "nginx", "sites", "api.conf"), markedConf+"# v1\n", 0o644)
	writeFile(t, filepath.Join(rp, "nginx", "sites", "foreign.conf"), "# hand written\n", 0o644)
	writeFile(t, filepath.Join(rp, "nginx", "conf.d", "block.conf"), "# hand block\n", 0o644)
	writeFile(t, filepath.Join(rp, "hasura", "metadata", "version.yaml"), "version: 3\n", 0o644)
	writeFile(t, filepath.Join(rp, "ssl", "certificates", "x", "fullchain.pem"), "CERT", 0o644)
}

func tarList(t *testing.T, tar string) string {
	t.Helper()
	out, err := exec.Command("tar", "-tzf", tar).CombinedOutput()
	if err != nil {
		t.Fatalf("tar -tzf: %v\n%s", err, out)
	}
	return string(out)
}

func TestRemoteBackupTar(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	seedDeployedHost(t, rp)
	id := NewBackupID(time.Date(2026, 10, 10, 20, 45, 16, 0, time.UTC))
	if !strings.HasPrefix(id, "20261010T204516Z-") || !backupIDRe.MatchString(id) {
		t.Fatalf("id %q", id)
	}
	bk, err := CreatePreDeployTar(context.Background(), f, rp, id, []string{"docker-compose.yml", ".nself/compose-env-files.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(rp, ".nself", "backups", "pre-deploy-"+id+".tar.gz"); bk.Tar != want || !exists(want) {
		t.Fatalf("tar path %q (exists %v), want %q", bk.Tar, exists(want), want)
	}
	list := tarList(t, bk.Tar)
	for _, want := range []string{"docker-compose.yml", ".env", ".env.prod", ".nself/compose-files.txt", ".nself/plugins/ai/docker-compose.plugin.yml", "nginx/nginx.conf", "nginx/sites/api.conf", "hasura/metadata/version.yaml"} {
		if !strings.Contains(list, want) {
			t.Errorf("tar lacks %s:\n%s", want, list)
		}
	}
	for _, bad := range []string{"conf.d", "ssl/", ".nself/backups"} {
		if strings.Contains(list, bad) {
			t.Errorf("tar must not hold %s:\n%s", bad, list)
		}
	}
	if info, _ := os.Stat(bk.Tar); info.Mode().Perm()&0o077 != 0 {
		t.Errorf("tar mode %v: it holds env files and must be owner-only", info.Mode().Perm())
	}
	if _, err := CreatePreDeployTar(context.Background(), f, rp, "../evil", nil); err == nil {
		t.Error("an id with .. was accepted")
	}

	// A first deploy: the project dir does not exist yet, nothing to save.
	empty := filepath.Join(t.TempDir(), "fresh")
	bk2, err := CreatePreDeployTar(context.Background(), f, empty, NewBackupID(time.Now()), []string{"docker-compose.yml"})
	if err != nil {
		t.Fatalf("first deploy with nothing to back up: %v", err)
	}
	if !exists(bk2.Tar) {
		t.Fatal("no tar for the empty first deploy")
	}
	_ = tarList(t, bk2.Tar) // a valid, readable archive
	if got := strings.Join(bk2.Absent, ","); got != "docker-compose.yml" {
		t.Errorf("absent list on a first deploy = %q", got)
	}
	if got := strings.Join(bk.Absent, ","); got != ".nself/compose-env-files.txt" {
		t.Errorf("absent list = %q: only artifact paths the host did not have", got)
	}
}

func TestDeployRollbackInPlace(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	seedDeployedHost(t, rp)
	ctx := context.Background()
	changed := []string{"docker-compose.yml", ".nself/compose-env-files.txt", ".env.prod", "nginx/sites/api.conf", "nginx/sites/new-generated.conf"}
	bk, err := CreatePreDeployTar(ctx, f, rp, NewBackupID(time.Now()), changed)
	if err != nil {
		t.Fatal(err)
	}
	_ = err
	if err != nil {
		t.Fatal(err)
	}
	spec := RestoreSpec{Changed: changed, Absent: bk.Absent}
	inode := func(p string) uint64 { return statIno(t, p) }
	nginxIno, sitesIno := inode(filepath.Join(rp, "nginx")), inode(filepath.Join(rp, "nginx", "sites"))
	foreign := readFile(t, filepath.Join(rp, "nginx", "sites", "foreign.conf"))
	block := readFile(t, filepath.Join(rp, "nginx", "conf.d", "block.conf"))

	// The deploy: new content, a new generated conf, a new unmarked file.
	writeFile(t, filepath.Join(rp, "docker-compose.yml"), "# v2 broken\n", 0o644)
	writeFile(t, filepath.Join(rp, "nginx", "sites", "api.conf"), markedConf+"# v2\n", 0o644)
	writeFile(t, filepath.Join(rp, "nginx", "sites", "new-generated.conf"), markedConf, 0o644)
	writeFile(t, filepath.Join(rp, "nginx", "sites", "new-hand.conf"), "# somebody's\n", 0o644)
	writeFile(t, filepath.Join(rp, ".env"), "SECRET=2\n", 0o600)
	writeFile(t, filepath.Join(rp, ".nself", "compose-env-files.txt"), "created by the deploy\n", 0o644)

	if err := RestorePreDeployTar(ctx, f, rp, bk.Tar, spec); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(rp, "docker-compose.yml")); !strings.Contains(got, "# v1") {
		t.Errorf("compose not restored: %q", got)
	}
	if got := readFile(t, filepath.Join(rp, "nginx", "sites", "api.conf")); !strings.Contains(got, "# v1") {
		t.Errorf("api.conf not restored: %q", got)
	}
	if got := readFile(t, filepath.Join(rp, ".env")); got != "SECRET=2\n" {
		t.Errorf("the host's own .env was reverted: %q (the deploy never ships it)", got)
	}
	if exists(filepath.Join(rp, ".nself", "compose-env-files.txt")) {
		t.Error("a manifest the deploy created survived the restore")
	}
	if exists(filepath.Join(rp, "nginx", "sites", "new-generated.conf")) || !exists(filepath.Join(rp, "nginx", "sites", "new-hand.conf")) {
		t.Error("the restore must remove the marked new conf and keep the unmarked one")
	}
	if readFile(t, filepath.Join(rp, "nginx", "sites", "foreign.conf")) != foreign || readFile(t, filepath.Join(rp, "nginx", "conf.d", "block.conf")) != block {
		t.Error("a foreign conf or conf.d block changed")
	}
	if inode(filepath.Join(rp, "nginx")) != nginxIno || inode(filepath.Join(rp, "nginx", "sites")) != sitesIno {
		t.Error("a bind-mounted directory was replaced: its inode changed")
	}
	if exists(filepath.Join(rp, ".nself", "backups", "restore-pre-deploy-"+strings.TrimSuffix(strings.TrimPrefix(filepath.Base(bk.Tar), "pre-deploy-"), ".tar.gz"))) {
		t.Error("the temp extract dir was left behind")
	}
	for _, c := range f.commands() {
		if strings.Contains(c, "--delete") || strings.Contains(c, "mv ") {
			t.Errorf("restore ran %q", c)
		}
	}
	if err := RestorePreDeployTar(ctx, f, rp, "/etc/passwd", spec); err == nil {
		t.Error("restore accepted a tar outside the backup dir")
	}
}

func TestRollbackRemoteOrder(t *testing.T) {
	needTools(t)
	f, o, rp := applyFixture(t, "")
	// Deployed once already, so the host has a set to come back to.
	if _, err := ApplyRemote(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	prev := readFile(t, rp+"/.nself/compose-files.txt")
	res, err := ApplyRemote(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.log = nil
	f.mu.Unlock()
	writeFile(t, rp+"/.nself/compose-files.txt", "# broken by the second deploy\n", 0o644)
	err = RollbackRemote(context.Background(), RollbackOptions{Remote: f, RemotePath: rp, Tar: res.PreDeployTar, BackupID: "backup-1", Changed: res.Changed, Absent: res.Absent, Clock: &fakeClock{now: time.Unix(0, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, rp+"/.nself/compose-files.txt"); got != prev {
		t.Errorf("manifest not restored: %q", got)
	}
	got := steps(f)
	order := []string{"restore", "model", "imgcheck", "dbbackup", "imgrestore", "up:db", "up:web", "up:nginx"}
	prevAt := -1
	for _, want := range order {
		i := indexOf(got, want)
		if i < 0 || i <= prevAt {
			t.Fatalf("step %q at %d (previous %d) in %v", want, i, prevAt, got)
		}
		prevAt = i
	}
	// Off prod-class there is no database restore, and a first deploy is not re-applied.
	empty := newFakeRemote(t)
	empty.handler = stackHandler(t.TempDir(), "")
	fresh := t.TempDir()
	bk, err := CreatePreDeployTar(context.Background(), empty, fresh, NewBackupID(time.Now()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := RollbackRemote(context.Background(), RollbackOptions{Remote: empty, RemotePath: fresh, Tar: bk.Tar}); err != nil {
		t.Fatal(err)
	}
	for _, c := range empty.commands() {
		if strings.Contains(c, "nself backup restore") || strings.Contains(c, "docker compose") {
			t.Errorf("first-deploy rollback ran %q", c)
		}
	}
}
