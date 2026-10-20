package deploy

// Tests for the front root's own host-file rules (D-0425): another project's
// same-named site, the strict marker, and what counts as "shipped before".

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// shipSite adds hasura.conf with the given server_name to the staged front files.
func (x *frontFx) shipSite(t *testing.T, name, serverName string) {
	t.Helper()
	writeFile(t, filepath.Join(x.o.Set.Front.StageDir, "nginx", "sites", name), markedConf+"server { server_name "+serverName+"; }\n", 0o644)
	x.o.Set.Front.Files = append(x.o.Set.Front.Files, "nginx/sites/"+name)
}

func (x *frontFx) hostSite(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(x.front, "nginx", "sites", name)
	writeFile(t, p, body, 0o644)
	return p
}

func TestFrontSiblingCollision(t *testing.T) {
	// A sibling project fronted by the same stack owns hasura.conf.
	x := frontFixture(t)
	x.shipSite(t, "hasura.conf", "api.chat.example.org")
	sib := x.hostSite(t, "hasura.conf", markedConf+"server { server_name api.backend.example.org; }\n")
	_, err := ApplyRemote(context.Background(), x.o)
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E485" || !strings.Contains(err.Error(), "hasura.conf") || !strings.Contains(err.Error(), "api.backend.example.org") || !strings.Contains(err.Error(), "api.chat.example.org") || ce.Fix == "" {
		t.Fatalf("err = %v, want E485 naming the file and both server_name sets with a fix", err)
	}
	x.nothingSent(t)
	if !strings.Contains(readFile(t, sib), "api.backend.example.org") {
		t.Error("the sibling's file was replaced")
	}

	// The same server_name set (order-insensitive) is a file the fronting stack generated for this project: it ships.
	y := frontFixture(t)
	y.shipSite(t, "hasura.conf", "b.example.org a.example.org")
	y.hostSite(t, "hasura.conf", markedConf+"server { server_name a.example.org b.example.org; }\n# old\n")
	if _, err := ApplyRemote(context.Background(), y.o); err != nil {
		t.Fatalf("same server_name must ship: %v", err)
	}
	if got := readFile(t, filepath.Join(y.front, "nginx", "sites", "hasura.conf")); strings.Contains(got, "# old") {
		t.Errorf("hasura.conf was not replaced: %q", got)
	}

	// An empty server_name set on either side is a mismatch: a sibling's snippet with only upstream/map blocks stays.
	for _, c := range []struct{ name, host, ship string }{
		{"host file without server_name", "upstream pool { server 10.0.0.1:80; }\n", "api.chat.example.org"},
		{"shipped file without server_name", "server { server_name api.backend.example.org; }\n", ""},
	} {
		e := frontFixture(t)
		if c.ship != "" {
			e.shipSite(t, "pool.conf", c.ship)
		} else {
			writeFile(t, filepath.Join(e.o.Set.Front.StageDir, "nginx", "sites", "pool.conf"), markedConf, 0o644)
			e.o.Set.Front.Files = append(e.o.Set.Front.Files, "nginx/sites/pool.conf")
		}
		host := e.hostSite(t, "pool.conf", markedConf+c.host)
		if _, err := ApplyRemote(context.Background(), e.o); !e485(err) || !strings.Contains(err.Error(), "pool.conf") {
			t.Errorf("%s: err = %v, want E485", c.name, err)
		}
		if !strings.Contains(readFile(t, host), c.host) {
			t.Errorf("%s: the sibling's file was replaced", c.name)
		}
		e.nothingSent(t)
	}

	// A path this host's own record lists ships whatever the host file says (marked).
	z := frontFixture(t)
	z.shipSite(t, "hasura.conf", "api.new.example.org")
	z.hostSite(t, "hasura.conf", markedConf+"server { server_name api.old.example.org; }\n")
	z.o.Set.Front.Previous = map[string][]string{"fake-host": {"nginx/sites/hasura.conf"}}
	z.o.Set.Front.PreviousNames = map[string]map[string][]string{"fake-host": {"nginx/sites/hasura.conf": {"api.old.example.org"}}}
	if _, err := ApplyRemote(context.Background(), z.o); err != nil {
		t.Fatalf("a file this project shipped before must be replaced: %v", err)
	}
	// A recorded path is no pass by itself: if another project's marked file with other
	// names now sits there, the record does not cover it (E485).
	b := frontFixture(t)
	b.shipSite(t, "hasura.conf", "api.new.example.org")
	b.hostSite(t, "hasura.conf", markedConf+"server { server_name api.other-project.example.org; }\n")
	b.o.Set.Front.Previous = map[string][]string{"fake-host": {"nginx/sites/hasura.conf"}}
	b.o.Set.Front.PreviousNames = map[string]map[string][]string{"fake-host": {"nginx/sites/hasura.conf": {"api.old.example.org"}}}
	if _, err := ApplyRemote(context.Background(), b.o); !e485(err) || !strings.Contains(err.Error(), "api.other-project.example.org") {
		t.Fatalf("a replaced recorded path must be refused: %v", err)
	}
	// An old record (the path, no names) is treated like an unknown path.
	o := frontFixture(t)
	o.shipSite(t, "hasura.conf", "api.new.example.org")
	o.hostSite(t, "hasura.conf", markedConf+"server { server_name api.old.example.org; }\n")
	o.o.Set.Front.Previous = map[string][]string{"fake-host": {"nginx/sites/hasura.conf"}}
	if _, err := ApplyRemote(context.Background(), o.o); !e485(err) {
		t.Fatalf("a recorded path without names must follow the unknown-path rule: %v", err)
	}
	// ... but the record of another host grants nothing here.
	w := frontFixture(t)
	w.shipSite(t, "hasura.conf", "api.new.example.org")
	w.hostSite(t, "hasura.conf", markedConf+"server { server_name api.old.example.org; }\n")
	w.o.Set.Front.Previous = map[string][]string{"other-host": {"nginx/sites/hasura.conf"}}
	if _, err := ApplyRemote(context.Background(), w.o); !e485(err) {
		t.Fatalf("another host's record must not grant the overwrite: %v", err)
	}
}

func TestFrontStrictMarkerAndNoShippedListRead(t *testing.T) {
	const loose = "# hand-written, NOT generated by nself build\nserver { server_name x.example.org; }\n"
	// A comment that mentions the generator is not the marker: refused on overwrite.
	x := frontFixture(t)
	x.hostSite(t, "api.conf", loose)
	if _, err := ApplyRemote(context.Background(), x.o); !e485(err) || !strings.Contains(err.Error(), "unmarked") {
		t.Fatalf("err = %v, want the unmarked refusal", err)
	}
	// ... even when the fronting stack's own shipped list names it: that list is never read.
	y := frontFixture(t)
	y.hostSite(t, "api.conf", loose)
	writeFile(t, filepath.Join(y.front, ".nself", "deploy-shipped.txt"), "nginx/sites/api.conf\n", 0o644)
	if _, err := ApplyRemote(context.Background(), y.o); !e485(err) {
		t.Fatalf("the fronting stack's shipped list granted an overwrite: %v", err)
	}
	for _, c := range y.f.commands() {
		if strings.Contains(c, "deploy-shipped") && strings.Contains(c, "cd '"+y.front+"'") {
			t.Errorf("a front script reads the fronting stack's shipped list: %q", c)
		}
	}
	// ... and the strict rule decides the prune: a loose file in the record stays.
	z := frontFixture(t)
	z.hostSite(t, "old.conf", loose)
	z.o.Set.Front.Previous = map[string][]string{"fake-host": {"nginx/sites/old.conf"}}
	if _, err := ApplyRemote(context.Background(), z.o); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(z.front, "nginx", "sites", "old.conf")); got != loose {
		t.Errorf("a file without the strict marker was pruned or changed: %q", got)
	}
}

func TestFrontPrunedIsNotShippedBefore(t *testing.T) {
	// Deploy N prunes search.conf (shipped by deploy N-1).
	x := frontFixture(t)
	search := x.hostSite(t, "search.conf", markedConf+"server { server_name search.example.org; }\n")
	x.o.Set.Front.Previous = map[string][]string{"fake-host": {"nginx/sites/search.conf"}}
	x.o.Set.Front.PreviousNames = map[string]map[string][]string{"fake-host": {"nginx/sites/search.conf": {"search.example.org"}}}
	res, err := ApplyRemote(context.Background(), x.o)
	if err != nil {
		t.Fatal(err)
	}
	if exists(search) || !strings.Contains(strings.Join(res.FrontChanged, ","), "nginx/sites/search.conf") {
		t.Fatalf("search.conf not pruned or not in the change list: %v", res.FrontChanged)
	}
	if strings.Join(res.FrontShipped, ",") != "nginx/sites/api.conf,nginx/sites/new.conf" {
		t.Fatalf("shipped = %v: the pruned file must not count as shipped", res.FrontShipped)
	}
	// An outside party puts its own search.conf there. Deploy N+1 gets Previous from the shipped list only.
	other := markedConf + "server { server_name search.other.example.org; }\n"
	writeFile(t, search, other, 0o644)
	next := func() *frontFx {
		n := frontFixture(t)
		writeFile(t, filepath.Join(n.front, "nginx", "sites", "search.conf"), other, 0o644)
		n.o.Set.Front.Previous = map[string][]string{"fake-host": res.FrontShipped}
		n.o.Set.Front.PreviousNames = map[string]map[string][]string{"fake-host": res.FrontNames}
		return n
	}
	n1 := next()
	if _, err := ApplyRemote(context.Background(), n1.o); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(n1.front, "nginx", "sites", "search.conf")); got != other {
		t.Errorf("deploy N+1 pruned or changed the outside file: %q", got)
	}
	// If N+1 ships search.conf again, the sibling check applies (the name was not shipped before).
	n2 := next()
	n2.shipSite(t, "search.conf", "search.example.org")
	if _, err := ApplyRemote(context.Background(), n2.o); !e485(err) || !strings.Contains(err.Error(), "search.conf") {
		t.Fatalf("err = %v, want E485 for the foreign search.conf", err)
	}
}

func TestFrontRollbackReloadsEvenWhenReapplyFails(t *testing.T) {
	roll := func(x *frontFx, res ApplyResult) error {
		return RollbackRemote(context.Background(), RollbackOptions{Remote: x.f, RemotePath: x.rp, Tar: res.PreDeployTar, Changed: res.Changed, Absent: res.Absent,
			FrontTar: res.FrontTar, FrontChanged: res.FrontChanged, FrontAbsent: res.FrontAbsent, Clock: &fakeClock{now: time.Unix(0, 0)}})
	}
	x := frontFixture(t)
	res, err := ApplyRemote(context.Background(), x.o)
	if err != nil {
		t.Fatal(err)
	}
	x.d.mu.Lock()
	x.d.execs = nil
	x.d.mu.Unlock()
	x.f.fail["up -d --no-deps -- 'db'"] = errBoom // the rollback pulls nothing: every image is put back from the record
	err = roll(x, res)
	if err == nil || !strings.Contains(err.Error(), "fronting nginx was tested and reloaded") {
		t.Fatalf("err = %v, want the re-apply error naming the front reload", err)
	}
	if ex := x.d.execLog(); len(ex) != 2 || !strings.Contains(ex[0], "nginx -t") || !strings.Contains(ex[1], "nginx -s reload") {
		t.Errorf("the front was not tested and reloaded after the failed re-apply: %v", ex)
	}
	if got := readFile(t, filepath.Join(x.front, "nginx", "sites", "api.conf")); got != front1 {
		t.Errorf("api.conf = %q, want the restored bytes", got)
	}

	// Both fail: the errors are joined.
	y := frontFixture(t)
	res2, err := ApplyRemote(context.Background(), y.o)
	if err != nil {
		t.Fatal(err)
	}
	y.f.fail["up -d --no-deps -- 'db'"] = errBoom
	y.d.runSeq = []string{"false"}
	err = roll(y, res2)
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "fronting nginx reload also failed") {
		t.Errorf("err = %v, want both errors", err)
	}
}

func TestFrontMountCoversLinkedDirs(t *testing.T) {
	front, real := "/srv/web", "/srv/web"
	cases := []struct {
		name   string
		linked map[string]string
		mounts string
		ok     bool
	}{
		{"no links", nil, "bind|/srv/web/nginx|/etc/nginx\n", true},
		{"sites link inside nginx, covered by the nginx mount", map[string]string{"nginx/sites": "/srv/web/nginx/sites-v2"}, "bind|/srv/web/nginx|/etc/nginx\n", true},
		{"sites link inside nginx, mounted as a path", map[string]string{"nginx/sites": "/srv/web/nginx/sites-v2"}, "bind|/srv/web/nginx/sites|/etc/nginx/sites\n", true},
		{"sites link the mount does not cover", map[string]string{"nginx/sites": "/srv/web/sites-v2"}, "bind|/srv/web/nginx|/etc/nginx\n", false},
		{"sites link covered by its own mount", map[string]string{"nginx/sites": "/srv/web/sites-v2"}, "bind|/srv/web/nginx|/etc/nginx\nbind|/srv/web/sites-v2|/etc/nginx/sites\n", true},
		{"nginx link mounted as a path", map[string]string{"nginx": "/srv/web/nginx-real", "nginx/sites": "/srv/web/nginx-real/sites"}, "bind|/srv/web/nginx|/etc/nginx\n", true},
		{"nginx link, mount of a parent real path", map[string]string{"nginx": "/srv/web/nginx-real"}, "bind|/srv/web/nginx-real|/etc/nginx\nbind|/srv/web/nginx|/etc/x\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeRemote(t)
			d := &frontDocker{mounts: c.mounts, running: "true"}
			f.handler = d.handler(func(string) (string, bool) { return "", false })
			err := frontContainer{ID: "aaa1", Name: "n"}.checkMounts(context.Background(), f, front, real, c.linked)
			if c.ok && err != nil || !c.ok && (!e485(err) || !strings.Contains(err.Error(), "symlink")) {
				t.Errorf("err = %v, want ok=%v", err, c.ok)
			}
		})
	}

}

func TestFrontRecordedPathContentDecidesPrune(t *testing.T) {
	// A shipped hasura.conf; later project B puts its own marked hasura.conf there.
	const bConf = markedConf + "server { server_name api.b.example.org; }\n"
	run := func(prev map[string]map[string][]string) (*frontFx, *bytes.Buffer) {
		x := frontFixture(t)
		host := x.hostSite(t, "hasura.conf", bConf)
		_ = host
		x.o.Set.Front.Previous = map[string][]string{"fake-host": {"nginx/sites/hasura.conf"}} // not shipped now: a prune candidate
		x.o.Set.Front.PreviousNames = prev
		var out bytes.Buffer
		x.o.Out = &out
		if _, err := ApplyRemote(context.Background(), x.o); err != nil {
			t.Fatal(err)
		}
		return x, &out
	}
	// Names changed since A shipped it: kept, with a line saying why.
	x, out := run(map[string]map[string][]string{"fake-host": {"nginx/sites/hasura.conf": {"api.a.example.org"}}})
	if got := readFile(t, filepath.Join(x.front, "nginx", "sites", "hasura.conf")); got != bConf {
		t.Errorf("B's file was pruned or changed: %q", got)
	}
	if !strings.Contains(out.String(), "[kept] ") || !strings.Contains(out.String(), "server_name changed since this project shipped it") {
		t.Errorf("no kept line:\n%s", out.String())
	}
	// An old record without names prunes nothing.
	y, out2 := run(nil)
	if !exists(filepath.Join(y.front, "nginx", "sites", "hasura.conf")) || !strings.Contains(out2.String(), "no server_name set recorded") {
		t.Errorf("a path without a recorded set was pruned:\n%s", out2.String())
	}
	// Names as recorded: pruned.
	z, _ := run(map[string]map[string][]string{"fake-host": {"nginx/sites/hasura.conf": {"api.b.example.org"}}})
	if exists(filepath.Join(z.front, "nginx", "sites", "hasura.conf")) {
		t.Error("a file that still matches the record was not pruned")
	}
}

func TestFrontHostFileChangedBetweenGuardAndSync(t *testing.T) {
	x := frontFixture(t)
	const sibling = markedConf + "server { server_name api.sibling.example.org; }\n# sibling\n"
	host := filepath.Join(x.front, "nginx", "sites", "api.conf")
	base, reads := x.f.handler, 0
	x.f.handler = func(s string) (string, bool) {
		if strings.Contains(s, "head -c") && strings.Contains(s, "@@") {
			if reads++; reads == 2 { // the re-check inside the sync, after the tar
				writeFile(t, host, sibling, 0o644)
			}
		}
		return base(s)
	}
	res, err := ApplyRemote(context.Background(), x.o)
	if !e485(err) || !strings.Contains(err.Error(), "api.sibling.example.org") {
		t.Fatalf("err = %v, want E485 from the re-check", err)
	}
	if got := readFile(t, host); got != sibling {
		t.Errorf("the sibling's file was overwritten or restored over: %q", got)
	}
	if exists(filepath.Join(x.front, "nginx", "sites", "new.conf")) {
		t.Error("a front file was shipped after the failed re-check")
	}
	if got := readFile(t, filepath.Join(x.rp, "docker-compose.yml")); !strings.Contains(got, "# v1") || res.Synced {
		t.Errorf("the main root was not restored (synced=%v): %q", res.Synced, got)
	}
	if len(x.d.execLog()) != 0 {
		t.Errorf("exec ran: %v", x.d.execLog())
	}
	if res.FrontSynced {
		t.Error("FrontSynced is true although the re-check refused before the first front write")
	}

	// The main restore then fails and keeps the record (Synced true): the front fields
	// must still be marked as never synced, so the record carries no front part.
	y := frontFixture(t)
	base2, reads2 := y.f.handler, 0
	hostY := filepath.Join(y.front, "nginx", "sites", "api.conf")
	y.f.handler = func(s string) (string, bool) {
		if strings.Contains(s, "head -c") && strings.Contains(s, "@@") {
			if reads2++; reads2 == 2 {
				writeFile(t, hostY, sibling, 0o644)
			}
		}
		return base2(s)
	}
	y.f.fail["tar -xzf"] = errBoom
	res2, err := ApplyRemote(context.Background(), y.o)
	if err == nil || !res2.Synced {
		t.Fatalf("err = %v synced = %v, want the failed restore to keep the record", err, res2.Synced)
	}
	if res2.FrontSynced || res2.FrontTar == "" {
		t.Errorf("front record %+v: want a tar taken but FrontSynced false", res2.FrontRecord)
	}
	if got := readFile(t, hostY); got != sibling {
		t.Errorf("the sibling's file changed: %q", got)
	}
}

func TestFrontSyncCreatesOnlyAMissingSitesLeaf(t *testing.T) {
	x := frontFixture(t)
	if err := os.RemoveAll(filepath.Join(x.front, "nginx", "sites")); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRemote(context.Background(), x.o); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(x.front, "nginx", "sites", "api.conf")); got != frontNew {
		t.Errorf("api.conf = %q", got)
	}
	resolve, mk := x.index("okdir()"), x.index("mkdir 'sites'")
	if mk < 0 || resolve < 0 || resolve > mk {
		t.Fatalf("no plain mkdir of the leaf (%d) or resolve (%d)", mk, resolve)
	}
}
