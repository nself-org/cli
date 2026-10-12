package deploy

// Tests for shipping the fronting stack's nginx/sites (D-0425 apply side): the
// container rule, the guards before the lock, the front tar, the restore, the
// prune and the rollback. A fake host runs the file system parts with sh against
// temp dirs; docker is answered by frontDocker.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

const (
	frontCID  = "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00"
	otherCID  = "0ddba11"
	front1    = markedConf + "# front v1\n"
	frontNew  = markedConf + "# front v2\n"
	frontOwn  = "FRONT_SECRET=1\n"
	frontComp = "services: {nginx: {}}\n"
)

// frontDocker answers the docker commands of a fake host: the nginx containers
// `docker ps` lists, the mounts and running state `docker inspect` reports, and
// `docker exec` (recorded).
type frontDocker struct {
	mu      sync.Mutex
	ps      string // rows of `docker ps`: id TAB name TAB working_dir
	mounts  string
	running string
	execs   []string
}

func psRows(rows ...[3]string) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r[0] + "\t" + r[1] + "\t" + r[2] + "\n")
	}
	return b.String()
}

func (d *frontDocker) handler(base func(string) (string, bool)) func(string) (string, bool) {
	return func(s string) (string, bool) {
		d.mu.Lock()
		defer d.mu.Unlock()
		switch {
		case strings.Contains(s, "docker ps --no-trunc"):
			return d.ps, true
		case strings.Contains(s, "docker inspect --format '{{range .Mounts}}"):
			return d.mounts, true
		case strings.Contains(s, "docker inspect --format '{{.State.Running}}'"):
			return d.running + "\n", true
		case strings.Contains(s, "docker exec "):
			d.execs = append(d.execs, s)
			return "", true
		case strings.Contains(s, "config --format json"):
			return `{"name":"proj","services":{"db":{"image":"postgres:16","restart":"unless-stopped","healthcheck":{"test":["CMD","pg_isready"]}},"web":{"image":"web:1","restart":"always"}}}`, true
		case strings.Contains(s, "config --services"):
			return "db\nweb\n", true
		}
		return base(s)
	}
}

func (d *frontDocker) execLog() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string{}, d.execs...)
}

type frontFx struct {
	f     *fakeRemote
	o     ApplyOptions
	d     *frontDocker
	front string
	rp    string
}

// frontFixture is a deployed fronted project: the fronting stack at front holds
// its own .env, compose file, ssl dir and another project's site; the project sits
// at front/backend. The staged front root ships api.conf and new.conf.
func frontFixture(t *testing.T) *frontFx {
	t.Helper()
	needTools(t)
	front := t.TempDir()
	rp := filepath.Join(front, "backend")
	f := newFakeRemote(t)
	seedDeployedHost(t, rp)
	writeFile(t, filepath.Join(front, ".env"), frontOwn, 0o600)
	writeFile(t, filepath.Join(front, "docker-compose.yml"), frontComp, 0o644)
	writeFile(t, filepath.Join(front, "ssl", "certificates", "api-example-com", "fullchain.pem"), "CERT", 0o644)
	writeFile(t, filepath.Join(front, "nginx", "nginx.conf"), markedConf+"# stack\n", 0o644)
	writeFile(t, filepath.Join(front, "nginx", "sites", "api.conf"), front1, 0o644)
	writeFile(t, filepath.Join(front, "nginx", "sites", "sibling.conf"), markedConf+"# another project\n", 0o644)
	m, compose, envs, snap := buildScratchFor(t, rp)
	set, err := Stage(stageIn(m, compose, envs, snap, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	fstage := t.TempDir()
	writeFile(t, filepath.Join(fstage, "nginx", "sites", "api.conf"), frontNew, 0o644)
	writeFile(t, filepath.Join(fstage, "nginx", "sites", "new.conf"), markedConf+"# new\n", 0o644)
	set.Front = &FrontRoot{StageDir: fstage, RemotePath: front, Files: []string{"nginx/sites/api.conf", "nginx/sites/new.conf"}}
	d := &frontDocker{
		ps:      psRows([3]string{otherCID, "other-stack-nginx", "/srv/other"}, [3]string{frontCID, "nself-web-nginx-1", front}),
		mounts:  "bind|" + front + "/nginx|/etc/nginx\nbind|" + front + "/ssl|/etc/nginx/ssl\n",
		running: "true",
	}
	f.handler = d.handler(stackHandler(rp, ""))
	o := ApplyOptions{
		Remote: f, RemotePath: rp, Set: set, Yes: true, StrictHasura: true,
		Clock: &fakeClock{now: time.Unix(0, 0)},
		Now:   func() time.Time { return time.Date(2026, 10, 10, 20, 0, 0, 0, time.UTC) },
	}
	return &frontFx{f: f, o: o, d: d, front: front, rp: rp}
}

func (x *frontFx) index(sub string) int {
	for i, c := range x.f.commands() {
		if strings.Contains(c, sub) {
			return i
		}
	}
	return -1
}

func e485(err error) bool {
	var ce *errs.CLIError
	return errors.As(err, &ce) && ce.Code == "E485"
}

func TestApplyShipsFrontRoot(t *testing.T) {
	x := frontFixture(t)
	res, err := ApplyRemote(context.Background(), x.o)
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(x.front, "nginx", "sites", "api.conf")); got != frontNew {
		t.Errorf("api.conf on the host = %q", got)
	}
	if got := readFile(t, filepath.Join(x.front, "nginx", "sites", "new.conf")); !strings.Contains(got, "# new") {
		t.Errorf("new.conf on the host = %q", got)
	}
	if got := readFile(t, filepath.Join(x.front, "nginx", "sites", "sibling.conf")); !strings.Contains(got, "another project") {
		t.Errorf("another project's site changed: %q", got)
	}
	for f, want := range map[string]string{".env": frontOwn, "docker-compose.yml": frontComp} {
		if got := readFile(t, filepath.Join(x.front, f)); got != want {
			t.Errorf("fronting stack's %s changed: %q", f, got)
		}
	}
	// The exec ran on the label-resolved container ID, nginx -t before the reload.
	ex := x.d.execLog()
	if len(ex) != 2 || !strings.Contains(ex[0], "docker exec '"+frontCID+"' nginx -t") || !strings.Contains(ex[1], "docker exec '"+frontCID+"' nginx -s reload") {
		t.Fatalf("exec log: %v", ex)
	}
	for _, c := range ex {
		if strings.Contains(c, otherCID) || strings.Contains(c, "nself-web-nginx") {
			t.Errorf("exec used another container or a name: %q", c)
		}
	}
	tIdx, rIdx, syncIdx := x.index("docker exec '"+frontCID+"' nginx -t"), x.index("nginx -s reload"), x.index("rsync  -> "+filepath.Join(x.front, "nginx", "sites"))
	if syncIdx < 0 || !(syncIdx < tIdx && tIdx < x.index("up -d --no-deps -- 'db'") && x.index("up -d --no-deps -- 'web'") < rIdx) {
		t.Errorf("order: front rsync %d, nginx -t %d, reload %d", syncIdx, tIdx, rIdx)
	}
	// The project model's nginx is not touched: no throwaway container, no compose exec.
	for _, c := range x.f.commands() {
		if strings.Contains(c, "docker run --rm") || strings.Contains(c, "exec -T") {
			t.Errorf("the project model's nginx was used: %q", c)
		}
	}
	wantTar := filepath.Join(x.rp, ".nself", "backups", "pre-deploy-20261010T200000Z-")
	if !strings.HasPrefix(res.FrontTar, wantTar) || !strings.HasSuffix(res.FrontTar, "-front.tar.gz") {
		t.Errorf("front tar %q", res.FrontTar)
	}
	if strings.Join(res.FrontChanged, ",") != "nginx/sites/api.conf,nginx/sites/new.conf" || strings.Join(res.FrontAbsent, ",") != "nginx/sites/new.conf" {
		t.Errorf("front record: changed %v absent %v", res.FrontChanged, res.FrontAbsent)
	}
	// The fronting stack's own files appear in no command, tar or rsync argument.
	for _, c := range x.f.commands() {
		if strings.Contains(c, x.front+"/.env") || strings.Contains(c, x.front+"/docker-compose") || strings.Contains(c, x.front+"/ssl") {
			t.Errorf("a command names the fronting stack's own files: %q", c)
		}
		if strings.Contains(c, "-front.tar.gz") && strings.Contains(c, "tar -hczf") {
			for _, bad := range []string{".env", "compose", "ssl"} {
				if strings.Contains(c, bad) {
					t.Errorf("the front tar script names %q: %q", bad, c)
				}
			}
		}
		if strings.Contains(c, "--delete") {
			t.Errorf("recorded %q", c)
		}
	}
}

func TestFrontNginxContainerByLabels(t *testing.T) {
	front := "/srv/nself-web"
	real := "/private/srv/nself-web"
	row := func(id, name, wd string) [3]string { return [3]string{id, name, wd} }
	mount := func(src string) string { return "bind|" + src + "|/etc/nginx\n" }
	cases := []struct {
		name    string
		ps      string
		mounts  string
		want    string // the container ID, "" when E485 is expected
		errPart string
	}{
		{"one by the given path", psRows(row("aaa1", "n1", front), row("bbb2", "other", "/srv/other")), mount(front + "/nginx"), "aaa1", ""},
		{"one by the real path", psRows(row("aaa1", "n1", real)), mount(front + "/nginx/sites"), "aaa1", ""},
		{"mounted by the real path", psRows(row("aaa1", "n1", front)), mount(real + "/nginx"), "aaa1", ""},
		{"none running", psRows(row("bbb2", "other-nginx", "/srv/other")), mount(front + "/nginx"), "", "found 0"},
		{"no nginx at all", "", mount(front + "/nginx"), "", "none"},
		{"two match", psRows(row("aaa1", "n1", front), row("ccc3", "n2", real)), mount(front + "/nginx"), "", "found 2"},
		{"not a plain id", psRows(row("a;rm -rf x", "n1", front)), mount(front + "/nginx"), "", "not a plain container id"},
		{"no mount of nginx", psRows(row("aaa1", "n1", front)), mount(front+"/ssl") + mount(front), "", "does not bind-mount"},
		{"a volume is not a bind mount", psRows(row("aaa1", "n1", front)), "volume|" + front + "/nginx|/etc/nginx\n", "", "does not bind-mount"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeRemote(t)
			d := &frontDocker{ps: c.ps, mounts: c.mounts, running: "true"}
			f.handler = d.handler(func(string) (string, bool) { return "", false })
			ctr, err := resolveFrontContainer(context.Background(), f, front, real)
			if err == nil {
				err = ctr.checkMounts(context.Background(), f, front, real)
			}
			if c.want != "" {
				if err != nil || ctr.ID != c.want {
					t.Fatalf("container %+v err %v, want %s", ctr, err, c.want)
				}
				return
			}
			if !e485(err) || !strings.Contains(err.Error(), c.errPart) {
				t.Fatalf("err = %v, want E485 with %q", err, c.errPart)
			}
		})
	}
	// Several containers: the message names every running nginx-service container with its label.
	f := newFakeRemote(t)
	d := &frontDocker{ps: psRows(row("aaa1", "n1", front), row("ccc3", "n2", front), row("bbb2", "other", "/srv/other")), running: "true"}
	f.handler = d.handler(func(string) (string, bool) { return "", false })
	_, err := resolveFrontContainer(context.Background(), f, front, "")
	for _, want := range []string{"n1 (com.docker.compose.project.working_dir=/srv/nself-web)", "n2 (", "other (com.docker.compose.project.working_dir=/srv/other)"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error %v lacks %q", err, want)
		}
	}
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Fix == "" || !strings.Contains(ce.Fix, front) {
		t.Errorf("no fix naming the front root: %v", err)
	}
}

// nothingSent fails the test when the host saw the lock, a tar or a sync.
func (x *frontFx) nothingSent(t *testing.T) {
	t.Helper()
	for _, c := range x.f.commands() {
		if strings.Contains(c, "deploy.lock") || strings.HasPrefix(c, "rsync ") || strings.Contains(c, "tar -hczf") || strings.Contains(c, "docker exec") {
			t.Errorf("a refused deploy sent %q", c)
		}
	}
	if exists(filepath.Join(x.rp, ".nself", "deploy.lock")) || exists(filepath.Join(x.front, "nginx", "sites", "new.conf")) {
		t.Error("a refused deploy left the lock or a shipped file behind")
	}
}

func TestFrontRefusalsBeforeLock(t *testing.T) {
	cases := []struct {
		name  string
		setup func(x *frontFx)
		want  string
	}{
		{"no running nginx", func(x *frontFx) { x.d.ps = "" }, "found 0"},
		{"two fronting nginx", func(x *frontFx) {
			x.d.ps = psRows([3]string{"aaa1", "n1", x.front}, [3]string{"bbb2", "n2", x.front})
		}, "found 2"},
		{"no mount of the nginx dir", func(x *frontFx) { x.d.mounts = "bind|/elsewhere|/etc/nginx\n" }, "does not bind-mount"},
		{"unmarked file at a shipped front path", func(x *frontFx) {
			writeFile(t, filepath.Join(x.front, "nginx", "sites", "api.conf"), "# hand written\n", 0o644)
		}, "unmarked file would be overwritten"},
		{"front root is not the parent of the real remote_path", func(x *frontFx) {
			real := filepath.Join(t.TempDir(), "backend")
			if err := os.Rename(x.rp, real); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, x.rp); err != nil {
				t.Fatal(err)
			}
		}, "must be the parent of remote_path"},
		{"front paths other than nginx/sites", func(x *frontFx) { x.o.Set.Front.Files = []string{"nginx/sites/api.conf", ".env"} }, "unsafe path"},
		{"front root is not the directory of remote_path", func(x *frontFx) { x.o.Set.Front.RemotePath = filepath.Dir(x.front) }, "is not the directory of remote_path"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x := frontFixture(t)
			c.setup(x)
			res, err := ApplyRemote(context.Background(), x.o)
			if !e485(err) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want E485 with %q", err, c.want)
			}
			if res.Synced || res.PreDeployTar != "" {
				t.Errorf("result %+v", res)
			}
			x.nothingSent(t)
		})
	}
}

func TestFrontPreDeployTarOnlyShippedPaths(t *testing.T) {
	x := frontFixture(t)
	// An earlier deploy shipped old.conf; it is a prune candidate now, so the tar holds it too.
	writeFile(t, filepath.Join(x.front, "nginx", "sites", "old.conf"), markedConf+"# old\n", 0o644)
	x.o.Set.Front.Previous = []string{"nginx/sites/old.conf"}
	res, err := ApplyRemote(context.Background(), x.o)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, l := range strings.Split(tarList(t, res.FrontTar), "\n") {
		if l = strings.TrimPrefix(strings.TrimSpace(l), "./"); l != "" && !strings.HasSuffix(l, "/") {
			files = append(files, l)
		}
	}
	sort.Strings(files)
	if got := strings.Join(files, ","); got != "nginx/sites/api.conf,nginx/sites/old.conf" {
		t.Errorf("front tar holds %s, want only the previously present shipped paths", got)
	}
	if strings.Join(res.FrontAbsent, ",") != "nginx/sites/new.conf" {
		t.Errorf("absent %v", res.FrontAbsent)
	}
}

func TestFrontNginxTestFailureRestores(t *testing.T) {
	check := func(t *testing.T, x *frontFx, res ApplyResult, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("the failure was swallowed")
		}
		if got := readFile(t, filepath.Join(x.front, "nginx", "sites", "api.conf")); got != front1 {
			t.Errorf("api.conf after the failure = %q, want the old bytes", got)
		}
		if exists(filepath.Join(x.front, "nginx", "sites", "new.conf")) {
			t.Error("the created front file survived the restore")
		}
		if got := readFile(t, filepath.Join(x.front, "nginx", "sites", "sibling.conf")); !strings.Contains(got, "another project") {
			t.Errorf("another project's site = %q", got)
		}
		if got := readFile(t, filepath.Join(x.rp, "docker-compose.yml")); !strings.Contains(got, "# v1") {
			t.Errorf("the main root was not restored: %q", got)
		}
		if res.Synced {
			t.Error("Synced stays true after both roots were restored: the last good record would be replaced")
		}
		for _, e := range x.d.execLog() {
			if strings.Contains(e, "reload") {
				t.Errorf("nginx was reloaded: %q", e)
			}
		}
		if x.index("up -d --no-deps") >= 0 {
			t.Error("a container was touched")
		}
	}
	t.Run("nginx -t fails", func(t *testing.T) {
		x := frontFixture(t)
		x.f.fail["nginx -t"] = errBoom
		res, err := ApplyRemote(context.Background(), x.o)
		check(t, x, res, err)
		if !strings.Contains(err.Error(), "nginx -t failed in the fronting container") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("the container stopped before step 8", func(t *testing.T) {
		x := frontFixture(t)
		x.d.running = "false"
		res, err := ApplyRemote(context.Background(), x.o)
		check(t, x, res, err)
		if !e485(err) || !strings.Contains(err.Error(), "no longer running") {
			t.Errorf("err = %v, want E485 at the re-check", err)
		}
		if len(x.d.execLog()) != 0 {
			t.Errorf("exec ran on a stopped container: %v", x.d.execLog())
		}
	})
}

func TestFrontPruneOnlyOwnShipped(t *testing.T) {
	x := frontFixture(t)
	sites := filepath.Join(x.front, "nginx", "sites")
	writeFile(t, filepath.Join(sites, "old.conf"), markedConf+"# shipped by the previous deploy\n", 0o644)
	writeFile(t, filepath.Join(sites, "mine-but-edited.conf"), "# hand edited, marker removed\n", 0o644)
	x.o.Set.Front.Previous = []string{"nginx/sites/old.conf", "nginx/sites/mine-but-edited.conf", "nginx/sites/api.conf", ".env", "nginx/sites/../../.env"}
	res, err := ApplyRemote(context.Background(), x.o)
	if err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(sites, "old.conf")) {
		t.Error("the file the previous deploy shipped and the new set dropped was not pruned")
	}
	for name, want := range map[string]string{"mine-but-edited.conf": "# hand edited", "sibling.conf": "another project"} {
		if got := readFile(t, filepath.Join(sites, name)); !strings.Contains(got, want) {
			t.Errorf("%s was removed or changed: %q", name, got)
		}
	}
	if got := readFile(t, filepath.Join(x.front, ".env")); got != frontOwn {
		t.Errorf("the fronting stack's .env = %q", got)
	}
	if !strings.Contains(strings.Join(res.FrontChanged, ","), "nginx/sites/old.conf") {
		t.Errorf("the prune is not in the record: %v", res.FrontChanged)
	}
	// The sibling's marker-carrying file was never a candidate: it is not in Previous.
	for _, c := range x.f.commands() {
		if strings.Contains(c, "sibling.conf") && strings.Contains(c, "rm -f") {
			t.Errorf("a prune script names the sibling's file: %q", c)
		}
	}
}

func TestDeployRollbackRestoresFront(t *testing.T) {
	x := frontFixture(t)
	res, err := ApplyRemote(context.Background(), x.o)
	if err != nil {
		t.Fatal(err)
	}
	sites := filepath.Join(x.front, "nginx", "sites")
	if got := readFile(t, filepath.Join(sites, "api.conf")); got != frontNew {
		t.Fatalf("the deploy did not ship: %q", got)
	}
	x.f.mu.Lock()
	x.f.log = nil
	x.f.mu.Unlock()
	x.d.mu.Lock()
	x.d.execs = nil
	x.d.mu.Unlock()
	ro := RollbackOptions{
		Remote: x.f, RemotePath: x.rp, Tar: res.PreDeployTar, Changed: res.Changed, Absent: res.Absent,
		FrontTar: res.FrontTar, FrontChanged: res.FrontChanged, FrontAbsent: res.FrontAbsent,
		Clock: &fakeClock{now: time.Unix(0, 0)},
	}
	if err := RollbackRemote(context.Background(), ro); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(sites, "api.conf")); got != front1 {
		t.Errorf("api.conf after the rollback = %q", got)
	}
	if exists(filepath.Join(sites, "new.conf")) {
		t.Error("the file the deploy created was not removed")
	}
	if got := readFile(t, filepath.Join(x.front, ".env")); got != frontOwn {
		t.Errorf("the fronting stack's .env = %q", got)
	}
	ex := x.d.execLog()
	if len(ex) != 2 || !strings.Contains(ex[0], "'"+frontCID+"' nginx -t") || !strings.Contains(ex[1], "'"+frontCID+"' nginx -s reload") {
		t.Errorf("exec log after the rollback: %v", ex)
	}
	if x.index("tar -xzf") < 0 || x.index("tar -xzf") > x.index("nginx -t") {
		t.Error("the files were not restored before nginx -t")
	}

	// A record without front fields (a deploy before P7-DEPL-30) rolls back as before:
	// no docker ps, no exec, the front root untouched.
	y := frontFixture(t)
	res2, err := ApplyRemote(context.Background(), y.o)
	if err != nil {
		t.Fatal(err)
	}
	y.f.mu.Lock()
	y.f.log = nil
	y.f.mu.Unlock()
	y.d.mu.Lock()
	y.d.execs = nil
	y.d.mu.Unlock()
	if err := RollbackRemote(context.Background(), RollbackOptions{Remote: y.f, RemotePath: y.rp, Tar: res2.PreDeployTar, Changed: res2.Changed, Absent: res2.Absent, Clock: &fakeClock{now: time.Unix(0, 0)}}); err != nil {
		t.Fatal(err)
	}
	if y.index("docker ps") >= 0 || len(y.d.execLog()) != 0 {
		t.Errorf("an old record touched the front: %v", y.d.execLog())
	}
	if got := readFile(t, filepath.Join(y.front, "nginx", "sites", "api.conf")); got != frontNew {
		t.Errorf("the front root changed without a front record: %q", got)
	}
	// A tampered front record is refused before anything is changed.
	z := frontFixture(t)
	bad := ro
	bad.Remote, bad.RemotePath, bad.Tar = z.f, z.rp, res.PreDeployTar
	bad.FrontTar = filepath.Join(z.rp, BackupDir, "pre-deploy-20261010T200000Z-abcdef-front.tar.gz")
	bad.FrontChanged = []string{".env"}
	if err := RollbackRemote(context.Background(), bad); err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Errorf("tampered record: %v", err)
	}
}
