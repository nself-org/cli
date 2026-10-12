package commands

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
)

const (
	frontTestRP  = "/srv/nself-web/backend"
	frontTestCID = "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00"
)

// frontStub is a host that answers the docker and path commands of the fronting
// stack's rollback and the ssl listing, and records every command. Other
// commands succeed with no output.
type frontStub struct {
	mu  sync.Mutex
	log []string
}

func (s *frontStub) Label() string { return "deploy@app.example.test" }
func (s *frontStub) Rsync(context.Context, []string, string, string, string) error {
	return nil
}

func (s *frontStub) Run(_ context.Context, cmd string) ([]byte, error) {
	c := cmd
	if rest, ok := strings.CutPrefix(cmd, "sh -c '"); ok {
		c = strings.ReplaceAll(strings.TrimSuffix(rest, "'"), `'\''`, `'`)
	}
	s.mu.Lock()
	s.log = append(s.log, c)
	s.mu.Unlock()
	switch {
	case strings.Contains(c, "docker ps --no-trunc"):
		return []byte(frontTestCID + "\tnself-web-nginx-1\t/srv/nself-web\n"), nil
	case strings.Contains(c, "docker inspect --format '{{range .Mounts}}"):
		return []byte("bind|/srv/nself-web/nginx|/etc/nginx\n"), nil
	case strings.Contains(c, "{{.State.Running}}"):
		return []byte("true\n"), nil
	case strings.Contains(c, "FRONT\\t"):
		return []byte("FRONT\t/srv/nself-web\nRP\t" + frontTestRP + "\n"), nil
	case strings.Contains(c, "if mkdir"):
		return []byte("OK\n"), nil
	case strings.Contains(c, "ssl/certificates"):
		return []byte("/srv/nself-web/ssl/certificates/api-example-com/fullchain.pem\n/srv/nself-web/ssl/certificates/api-example-com/privkey.pem\n"), nil
	}
	return nil, nil
}

func (s *frontStub) idx(sub string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.log {
		if strings.Contains(c, sub) {
			return i
		}
	}
	return -1
}

func frontRecord(front bool) deploy.StrategyState {
	h := "deploy@app.example.test"
	st := deploy.StrategyState{
		Strategy: deploy.StrategyRolling, Target: "prod", Success: true,
		PreDeployTars:    map[string]string{h: frontTestRP + "/.nself/backups/pre-deploy-20261010T200000Z-abcdef.tar.gz"},
		PreDeployChanged: map[string][]string{h: {"docker-compose.yml"}},
		PreDeployAbsent:  map[string][]string{h: {"docker-compose.yml"}},
	}
	if front {
		st.FrontTars = map[string]string{h: frontTestRP + "/.nself/backups/pre-deploy-20261010T200000Z-abcdef-front.tar.gz"}
		st.FrontChanged = map[string][]string{h: {"nginx/sites/api.conf"}}
		st.FrontAbsent = map[string][]string{h: {"nginx/sites/api.conf"}}
	}
	return st
}

func openFrontStub(t *testing.T) *frontStub {
	t.Helper()
	compattest.Set(t, true)
	s := &frontStub{}
	old := deployRemoteOpen
	t.Cleanup(func() { deployRemoteOpen = old })
	deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) { return s, nil }
	return s
}

func TestRollbackPassesFrontRecord(t *testing.T) {
	s := openFrontStub(t)
	proj := t.TempDir()
	if err := deploy.SaveDeployState(proj, frontRecord(true)); err != nil {
		t.Fatal(err)
	}
	if handled, err := rollbackRemoteV15(yesCmd(), proj, "prod"); !handled || err != nil {
		t.Fatalf("rollback handled=%v err=%v", handled, err)
	}
	// The fronting container is resolved before the lock; the files come back, then nginx -t, then the reload.
	prev := -1
	for _, want := range []string{"docker ps --no-trunc", "if mkdir", "-front.tar.gz", "'" + frontTestCID + "' nginx -t", "'" + frontTestCID + "' nginx -s reload"} {
		i := s.idx(want)
		if i <= prev {
			t.Fatalf("%q at %d after %d in:\n%s", want, i, prev, strings.Join(s.log, "\n"))
		}
		prev = i
	}
	if st, _ := deploy.LoadDeployState(proj); !st.RolledBack {
		t.Error("the record was not marked rolled back")
	}
}

func TestRollbackWithoutFrontRecordIsUnchanged(t *testing.T) {
	s := openFrontStub(t)
	proj := t.TempDir()
	if err := deploy.SaveDeployState(proj, frontRecord(false)); err != nil {
		t.Fatal(err)
	}
	if handled, err := rollbackRemoteV15(yesCmd(), proj, "prod"); !handled || err != nil {
		t.Fatalf("rollback handled=%v err=%v", handled, err)
	}
	if s.idx("docker ps") >= 0 || s.idx("docker exec") >= 0 {
		t.Errorf("an old record touched docker:\n%s", strings.Join(s.log, "\n"))
	}
	// A front tar with an empty change list is no front part: the main root rolls back, docker is not asked.
	odd := frontRecord(true)
	odd.FrontChanged = nil
	s2 := openFrontStub(t)
	proj2 := t.TempDir()
	if err := deploy.SaveDeployState(proj2, odd); err != nil {
		t.Fatal(err)
	}
	if handled, err := rollbackRemoteV15(yesCmd(), proj2, "prod"); !handled || err != nil || s2.idx("docker ps") >= 0 || s2.idx("tar -tzf") < 0 {
		t.Errorf("handled=%v err=%v\n%s", handled, err, strings.Join(s2.log, "\n"))
	}
}

func TestRecordFrontAndPreviousRoundTrip(t *testing.T) {
	proj := t.TempDir()
	var st deploy.StrategyState
	recordFront(&st, deploy.ApplyResult{BackupHost: "h1"}) // no front tar: nothing recorded
	if st.FrontTars != nil {
		t.Fatalf("recorded a front without a tar: %+v", st)
	}
	var res deploy.ApplyResult
	res.BackupHost, res.FrontTar = "h1", "/srv/a/backend/.nself/backups/pre-deploy-x-front.tar.gz"
	res.FrontChanged, res.FrontAbsent = []string{"nginx/sites/a.conf", "nginx/sites/b.conf", "nginx/sites/pruned.conf"}, []string{"nginx/sites/b.conf"}
	res.FrontShipped = []string{"nginx/sites/a.conf", "nginx/sites/b.conf"} // pruned.conf was pruned, not shipped
	recordFront(&st, res)
	st.Target = "prod"
	if err := deploy.SaveDeployState(proj, st); err != nil {
		t.Fatal(err)
	}
	// A second host with a different record: each host sees only its own paths.
	res.BackupHost, res.FrontChanged, res.FrontAbsent, res.FrontShipped = "h2", []string{"nginx/sites/c.conf"}, nil, []string{"nginx/sites/c.conf"}
	recordFront(&st, res)
	// A host whose deploy changed nothing in the front root records no front part.
	recordFront(&st, deploy.ApplyResult{BackupHost: "h3", FrontRecord: deploy.FrontRecord{FrontTar: "/srv/a/backend/.nself/backups/pre-deploy-y-front.tar.gz"}})
	if _, ok := st.FrontTars["h3"]; ok {
		t.Error("a deploy with an empty front change list recorded a front part")
	}
	if err := deploy.SaveDeployState(proj, st); err != nil {
		t.Fatal(err)
	}
	set := deploy.ArtifactSet{Front: &deploy.FrontRoot{}}
	markFrontPrevious(&set, proj, "prod")
	if strings.Join(set.Front.Previous["h1"], ",") != "nginx/sites/a.conf,nginx/sites/b.conf" || strings.Join(set.Front.Previous["h2"], ",") != "nginx/sites/c.conf" || len(set.Front.Previous) != 2 {
		t.Errorf("previous = %v", set.Front.Previous)
	}
	// A record from before front_shipped existed (front_changed only): nothing is pruned.
	old := st
	old.FrontShipped = nil
	oldProj := t.TempDir()
	if err := deploy.SaveDeployState(oldProj, old); err != nil {
		t.Fatal(err)
	}
	oldSet := deploy.ArtifactSet{Front: &deploy.FrontRoot{}}
	markFrontPrevious(&oldSet, oldProj, "prod")
	if len(oldSet.Front.Previous) != 0 {
		t.Errorf("an old record without front_shipped fed the prune: %v", oldSet.Front.Previous)
	}
	other := deploy.ArtifactSet{Front: &deploy.FrontRoot{}}
	markFrontPrevious(&other, proj, "staging") // another target's record is not this deploy's
	markFrontPrevious(&deploy.ArtifactSet{}, proj, "prod")
	if len(other.Front.Previous) != 0 {
		t.Errorf("previous from another target: %v", other.Front.Previous)
	}
	raw, err := os.ReadFile(proj + "/" + deploy.DeployStateFile)
	if err != nil || !strings.Contains(string(raw), `"front_tars"`) {
		t.Errorf("deploy.json lacks front_tars: %v\n%s", err, raw)
	}
	var plain deploy.StrategyState
	plain.Target = "prod"
	if err := deploy.SaveDeployState(proj, plain); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(proj + "/" + deploy.DeployStateFile); strings.Contains(string(raw), "front_") {
		t.Errorf("a record without a front gained front fields:\n%s", raw)
	}
}

func TestSnapshotFrontSSLOnlyForFrontedLayout(t *testing.T) {
	s := &frontStub{}
	proj := frontedProject(t)
	p := v15Params{workdir: proj, target: "staging"}
	p.srv.RemotePath = "/srv/nself-web/backend"
	var snap deploy.TargetSnapshot
	if err := snapshotFrontSSL(context.Background(), s, p, &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.FrontSSL) != 1 || snap.FrontSSL[0] != (deploy.CertDir{Name: "api-example-com", Fullchain: true, Privkey: true}) {
		t.Errorf("front ssl = %+v", snap.FrontSSL)
	}
	// A layout that does not follow the convention reads nothing (Stage refuses it with E485).
	p.srv.RemotePath = "/srv/other/backend"
	var skipped deploy.TargetSnapshot
	n := len(s.log)
	if err := snapshotFrontSSL(context.Background(), s, p, &skipped); err != nil || skipped.FrontSSL != nil || len(s.log) != n {
		t.Errorf("layout mismatch: %v %+v (%d new commands)", err, skipped.FrontSSL, len(s.log)-n)
	}
	// A project that runs its own nginx reads nothing.
	plain := nginxProject(t, "")
	p2 := v15Params{workdir: plain, target: "staging"}
	p2.srv.RemotePath = "/srv/nself-web/backend"
	var none deploy.TargetSnapshot
	n = len(s.log)
	if err := snapshotFrontSSL(context.Background(), s, p2, &none); err != nil || none.FrontSSL != nil || len(s.log) != n {
		t.Errorf("unfronted project: %v %+v", err, none.FrontSSL)
	}
}
