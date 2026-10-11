package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/spf13/cobra"
)

// v15Remote answers the remote commands of a v1.5 apply for a one-service stack
// and records them. Nothing runs for real.
type v15Remote struct {
	mu     sync.Mutex
	log    []string
	tag    string
	locked bool // the host's deploy lock is held by someone else
	label  string
	failOn string // a command containing this fails
	// stateful makes the model's web service keep a volume and the dry-run recreate it.
	stateful bool
	// shipped is the host's .nself/deploy-shipped.txt.
	shipped string
	// hang makes a command containing it print READY and block until ctx is done.
	hang string
}

func (r *v15Remote) Label() string {
	if r.label != "" {
		return r.label
	}
	return "deploy@app.example.test"
}

func (r *v15Remote) Rsync(_ context.Context, flags []string, _, _, dest string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, "rsync "+strings.Join(flags, " ")+" -> "+dest)
	return nil
}

var (
	quotedPath = regexp.MustCompile(`'(/[^']+)'`)
	tagArg     = regexp.MustCompile(`--tag '([^']+)'`)
	absentLoop = regexp.MustCompile(`for p in ((?:'[^']*' ?)+); do if \[ ! -e`)
	quotedArg  = regexp.MustCompile(`'([^']*)'`)
)

func (r *v15Remote) Run(ctx context.Context, cmd string) ([]byte, error) {
	s := cmd
	if rest, ok := strings.CutPrefix(cmd, "sh -c '"); ok {
		s = strings.ReplaceAll(strings.TrimSuffix(rest, "'"), `'\''`, `'`)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, s)
	if r.hang != "" && strings.Contains(s, r.hang) {
		r.mu.Unlock()
		fmt.Println("READY")
		<-ctx.Done()
		r.mu.Lock()
		return nil, ctx.Err()
	}
	if r.failOn != "" && strings.Contains(s, r.failOn) {
		return nil, errors.New("exit status 1: injected")
	}
	switch {
	case strings.HasPrefix(s, "if [ -d"):
		return []byte("yes\n"), nil // the build context exists on the host
	case strings.Contains(s, "if mkdir"):
		if r.locked {
			return []byte("LOCKED pid=7 host=other user=ops id=x\n"), nil
		}
		return []byte("OK\n"), nil
	case strings.Contains(s, "stat -c"):
		var b strings.Builder
		for _, m := range quotedPath.FindAllStringSubmatch(s, -1) {
			fmt.Fprintf(&b, "600 %s\n", m[1])
		}
		return []byte(b.String()), nil
	case strings.Contains(s, `"$HOME"`):
		return []byte("/home/deploy\n"), nil
	case strings.Contains(s, "config --format json") && r.stateful:
		return []byte(`{"name":"demo","services":{"web":{"image":"web:1","restart":"always","volumes":[{"type":"volume","source":"data","target":"/data"}]}}}`), nil
	case strings.Contains(s, "up -d --dry-run") && r.stateful:
		return []byte(" Container demo-web-1 Recreate\n"), nil
	case strings.Contains(s, "config --format json"):
		return []byte(`{"name":"demo","services":{"web":{"image":"web:1","restart":"always"}}}`), nil
	case strings.Contains(s, "config --services"):
		return []byte("web\n"), nil
	case strings.Contains(s, "nself backup create"):
		r.tag = tagArg.FindStringSubmatch(s)[1]
		return nil, nil
	case strings.Contains(s, "nself backup list"):
		return []byte(`[{"id":"demo_full_20261010_120000_` + r.tag + `","tag":"120000_` + r.tag + `","date":"2026-10-10T12:00:00Z"}]`), nil
	case strings.HasPrefix(s, "if [ -f") && strings.Contains(s, "deploy-shipped.txt"):
		return []byte(r.shipped), nil
	case strings.Contains(s, "ABSENT %s"):
		// The pre-deploy tar: the host has docker-compose.yml, none of the other shipped paths.
		m := absentLoop.FindStringSubmatch(s)
		var b strings.Builder
		for _, q := range quotedArg.FindAllStringSubmatch(m[1], -1) {
			if q[1] != "docker-compose.yml" {
				fmt.Fprintf(&b, "ABSENT %s\n", q[1])
			}
		}
		return []byte(b.String()), nil
	case strings.Contains(s, "tar -tzf") && !strings.Contains(s, "-czf"):
		return []byte("docker-compose.yml\n"), nil
	case strings.Contains(s, "compose-files.txt") && strings.HasPrefix(s, "if [ -f"):
		return []byte("/opt/nself/docker-compose.yml\n"), nil
	case strings.Contains(s, "compose-env-files.txt") && strings.HasPrefix(s, "if [ -f"):
		return []byte("/opt/nself/.env\n"), nil
	case strings.Contains(s, "docker inspect -f"):
		return []byte("2026-10-10T20:00:00Z\n"), nil
	case strings.Contains(s, "ps -a --format json"):
		return []byte(`[{"ID":"id-web","Name":"demo-web-1","State":"running"}]`), nil
	}
	return nil, nil
}

func (r *v15Remote) commands() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.log...)
}

type noClock struct{ now time.Time }

func (c *noClock) Now() time.Time { return c.now }
func (c *noClock) Sleep(ctx context.Context, d time.Duration) error {
	c.now = c.now.Add(d)
	return ctx.Err()
}

type okProber struct{}

func (okProber) SSHReachable(controlplane.Server) (bool, int, error) { return true, 1, nil }
func (okProber) DockerOK(controlplane.Server) (bool, error)          { return true, nil }
func (okProber) KeyState(controlplane.Server) (bool, string)         { return true, "/tmp/key" }
func (okProber) Architecture(context.Context, controlplane.Server) (string, error) {
	return "amd64", nil
}

// v15Project is a project plus the stubs a v1.5 deploy needs.
func v15Project(t *testing.T) (proj string, remote *v15Remote) {
	t.Helper()
	compattest.Set(t, true)
	proj = seedProject(t)
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	t.Setenv("NSELF_HASURA_METADATA_STRICT", "false")
	var ranIn string
	var remoteArg bool
	oldBuild, oldOpen, oldClock := deployScratchBuildFn, deployRemoteOpen, deployGateClock
	t.Cleanup(func() { deployScratchBuildFn, deployRemoteOpen, deployGateClock = oldBuild, oldOpen, oldClock })
	deployScratchBuildFn = fakeScratchBuild(t, &ranIn, &remoteArg, nil)
	remote = &v15Remote{}
	deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) { return remote, nil }
	deployGateClock = &noClock{now: time.Unix(0, 0)}
	return proj, remote
}

func firstIndex(cmds []string, sub string) int {
	for i, c := range cmds {
		if strings.Contains(c, sub) {
			return i
		}
	}
	return -1
}

func TestDeployApplyV15Pipeline(t *testing.T) {
	proj, remote := v15Project(t)
	before := hashProject(t, proj, ".nself")
	key := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(key, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_TEST_DEPLOY_KEY", key)
	inv := &controlplane.Inventory{SchemaVersion: 2, Environments: map[string]controlplane.Environment{
		"prod": {Name: "prod", Kind: "remote", Tier: controlplane.TierProd, Servers: []controlplane.Server{
			{Name: "prod-app", Role: controlplane.RoleApp, Host: "deploy@app.example.test", SSHKeyRef: "NSELF_TEST_DEPLOY_KEY", RemotePath: "/opt/nself", Primary: true},
		}},
	}}
	cmd := &cobra.Command{}
	cmd.SetContext(context.WithValue(context.Background(), deployYesKey{}, true))
	res, err := runDeployPipeline(cmd, proj, inv, "prod", okProber{}, filepath.Join(proj, "docker-compose.yml"), "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Servers) != 1 || res.Servers[0].Status != "ok" {
		t.Fatalf("result %+v", res.Servers)
	}
	cmds := remote.commands()
	order := []string{`"$HOME"`, "nself backup create", "nself backup verify", "if mkdir", "tar -czf", "rsync ", "stat -c", "config --format json", " pull", "up -d --dry-run", "up -d --no-deps -- 'web'", "hasura metadata apply"}
	prev := -1
	for _, want := range order {
		i := firstIndex(cmds, want)
		if i <= prev {
			t.Fatalf("%q at %d after %d in:\n%s", want, i, prev, strings.Join(cmds, "\n"))
		}
		prev = i
	}
	if after := hashProject(t, proj, ".nself"); after != before {
		t.Fatalf("the operator's project tree changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	st, _ := deploy.LoadDeployState(proj)
	if !st.Success || st.Target != "prod" || st.BackupHost != "deploy@app.example.test" || !strings.HasPrefix(st.BackupID, "demo_full_") || !strings.HasPrefix(st.PreDeployTar, "/opt/nself/.nself/backups/pre-deploy-") {
		t.Fatalf("deploy.json %+v", st)
	}

	// Rollback restores the files in place, then the database backup, then re-applies.
	remote.mu.Lock()
	remote.log = nil
	remote.mu.Unlock()
	handled, err := rollbackRemoteV15(cmd, proj, "prod")
	if !handled || err != nil {
		t.Fatalf("rollback handled=%v err=%v", handled, err)
	}
	cmds = remote.commands()
	prev = -1
	for _, want := range []string{"tar -xzf", "nself backup restore '" + st.BackupID + "' --yes", "config --format json", " pull", "up -d --no-deps -- 'web'"} {
		i := firstIndex(cmds, want)
		if i <= prev {
			t.Fatalf("rollback: %q at %d after %d in:\n%s", want, i, prev, strings.Join(cmds, "\n"))
		}
		prev = i
	}
	after, _ := deploy.LoadDeployState(proj)
	if !after.RolledBack || after.BackupID != "" {
		t.Errorf("deploy.json after the rollback: %+v", after)
	}
	// F7: a second rollback must not restore the database again.
	remote.mu.Lock()
	remote.log = nil
	remote.mu.Unlock()
	handled, err = rollbackRemoteV15(cmd, proj, "prod")
	if !handled || err == nil || !strings.Contains(err.Error(), "already rolled back") {
		t.Fatalf("second rollback: handled=%v err=%v", handled, err)
	}
	if got := remote.commands(); len(got) != 0 {
		t.Errorf("the second rollback touched the host: %v", got)
	}
	if handled, _ := rollbackRemoteV15(cmd, t.TempDir(), "prod"); handled {
		t.Error("a project with no v1.5 record must fall through to the old rollback")
	}
}

func TestDeployApplyV15Legacy(t *testing.T) {
	proj, remote := v15Project(t)
	t.Setenv("NSELF_DEPLOY_SSH_KEY", "/tmp/key")
	before := hashProject(t, proj, ".nself")
	ctx := context.WithValue(context.Background(), deployYesKey{}, true)
	if err := remoteDeployPushDispatch(ctx, proj, "deploy@app.example.test:/srv/nself", "staging", true); err != nil {
		t.Fatal(err)
	}
	cmds := remote.commands()
	if firstIndex(cmds, "nself backup create") >= 0 {
		t.Error("a non-production target takes no database backup")
	}
	if firstIndex(cmds, "tar -czf") < 0 || firstIndex(cmds, "up -d --no-deps -- 'web'") < 0 || firstIndex(cmds, "rsync ") < 0 {
		t.Fatalf("legacy v1.5 path skipped the shared apply:\n%s", strings.Join(cmds, "\n"))
	}
	if !strings.Contains(strings.Join(cmds, "\n"), "/srv/nself/docker-compose.yml") {
		t.Error("the compose manifest was not mapped to the legacy remote path")
	}
	if after := hashProject(t, proj, ".nself"); after != before {
		t.Fatal("the operator's project tree changed")
	}
	if st, _ := deploy.LoadDeployState(proj); st.BackupID != "" || !strings.HasPrefix(st.PreDeployTar, "/srv/nself/.nself/backups/") {
		t.Fatalf("deploy.json %+v", st)
	}
	_ = os.Remove(filepath.Join(proj, deploy.DeployStateFile))
}

// F6: a deploy refused before its sync never overwrites the last good record, and
// every synced server's tar is recorded so a rollback restores all of them.
func TestDeployRecordOnlyAfterSync(t *testing.T) {
	proj, remote := v15Project(t)
	key := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(key, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_TEST_DEPLOY_KEY", key)
	second := &v15Remote{label: "deploy@app2.example.test"}
	deployRemoteOpen = func(_ context.Context, srv controlplane.Server, _, _ string) (deploy.Remote, error) {
		if strings.Contains(srv.Host, "app2") {
			return second, nil
		}
		return remote, nil
	}
	servers := []controlplane.Server{
		{Name: "app-1", Role: controlplane.RoleApp, Host: "deploy@app.example.test", SSHKeyRef: "NSELF_TEST_DEPLOY_KEY", RemotePath: "/opt/nself", Primary: true},
		{Name: "app-2", Role: controlplane.RoleApp, Host: "deploy@app2.example.test", SSHKeyRef: "NSELF_TEST_DEPLOY_KEY", RemotePath: "/opt/nself"},
	}
	inv := &controlplane.Inventory{SchemaVersion: 2, Environments: map[string]controlplane.Environment{
		"staging": {Name: "staging", Kind: "remote", Tier: controlplane.TierLocalServers, Servers: servers}}}
	cmd := &cobra.Command{}
	cmd.SetContext(context.WithValue(context.Background(), deployYesKey{}, true))
	run := func() error {
		res, err := runDeployPipeline(cmd, proj, inv, "staging", okProber{}, filepath.Join(proj, "docker-compose.yml"), "", "", true)
		if err == nil {
			err = failedServersError(res)
		}
		return err
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	good, _ := deploy.LoadDeployState(proj)
	if len(good.PreDeployTars) != 2 || good.PreDeployTars["deploy@app.example.test"] == "" || good.PreDeployTars["deploy@app2.example.test"] == "" || good.BackupHost != "deploy@app.example.test" {
		t.Fatalf("both hosts' tars must be recorded, primary as backup_host: %+v", good)
	}
	// Rollback restores every server of the env.
	for _, r := range []*v15Remote{remote, second} {
		r.mu.Lock()
		r.log = nil
		r.mu.Unlock()
	}
	if handled, err := rollbackRemoteV15(cmd, proj, "staging"); !handled || err != nil {
		t.Fatalf("rollback: %v %v", handled, err)
	}
	for _, r := range []*v15Remote{remote, second} {
		if firstIndex(r.commands(), "tar -xzf") < 0 {
			t.Errorf("%s was not restored: %v", r.Label(), r.commands())
		}
	}
	// A deploy refused by the lock changes nothing in the record.
	good.RolledBack = false
	if err := deploy.SaveDeployState(proj, good); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := os.ReadFile(filepath.Join(proj, deploy.DeployStateFile))
	remote.locked, second.locked = true, true
	if err := run(); err == nil || !strings.Contains(err.Error(), "holds the lock") {
		t.Fatalf("locked deploy: %v", err)
	}
	if now, _ := os.ReadFile(filepath.Join(proj, deploy.DeployStateFile)); string(now) != string(snapshot) {
		t.Errorf("a refused deploy rewrote deploy.json:\n%s\nvs\n%s", snapshot, now)
	}
}

func prodInventory(servers ...controlplane.Server) *controlplane.Inventory {
	return &controlplane.Inventory{SchemaVersion: 2, Environments: map[string]controlplane.Environment{
		"prod": {Name: "prod", Kind: "remote", Tier: controlplane.TierProd, Servers: servers}}}
}

func prodServers(t *testing.T) []controlplane.Server {
	key := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(key, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_TEST_DEPLOY_KEY", key)
	return []controlplane.Server{
		{Name: "app-1", Role: controlplane.RoleApp, Host: "deploy@app.example.test", SSHKeyRef: "NSELF_TEST_DEPLOY_KEY", RemotePath: "/opt/nself", Primary: true},
		{Name: "app-2", Role: controlplane.RoleApp, Host: "deploy@app2.example.test", SSHKeyRef: "NSELF_TEST_DEPLOY_KEY", RemotePath: "/opt/nself"},
	}
}

func yesCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.WithValue(context.Background(), deployYesKey{}, true))
	return cmd
}

// G7: one verified database backup per env, on the primary, before any server is
// touched; the record names the primary for both backup_id and backup_host.
func TestOneVerifiedBackupPerEnv(t *testing.T) {
	proj, primary := v15Project(t)
	second := &v15Remote{label: "deploy@app2.example.test"}
	deployRemoteOpen = func(_ context.Context, srv controlplane.Server, _, _ string) (deploy.Remote, error) {
		if strings.Contains(srv.Host, "app2") {
			return second, nil
		}
		return primary, nil
	}
	res, err := runDeployPipeline(yesCmd(), proj, prodInventory(prodServers(t)...), "prod", okProber{}, filepath.Join(proj, "docker-compose.yml"), "", "", true)
	if err == nil {
		err = failedServersError(res)
	}
	if err != nil {
		t.Fatal(err)
	}
	if n := countCmds(primary, "nself backup create"); n != 1 {
		t.Errorf("primary ran backup create %d times, want 1", n)
	}
	if firstIndex(second.commands(), "nself backup") >= 0 {
		t.Errorf("the second server ran nself backup: %v", second.commands())
	}
	// The backup precedes the first sync on any server.
	if bi, ri := firstIndex(primary.commands(), "nself backup verify"), firstIndex(primary.commands(), "rsync "); bi < 0 || bi > ri {
		t.Errorf("backup verify (%d) must come before the first rsync (%d)", bi, ri)
	}
	st, _ := deploy.LoadDeployState(proj)
	if st.BackupHost != "deploy@app.example.test" || !strings.HasPrefix(st.BackupID, "demo_full_") || len(st.PreDeployTars) != 2 {
		t.Errorf("record %+v", st)
	}
}

func countCmds(r *v15Remote, sub string) int {
	n := 0
	for _, c := range r.commands() {
		if strings.Contains(c, sub) {
			n++
		}
	}
	return n
}

// G4: a deploy that failed before the first up and was restored leaves deploy.json
// byte-identical.
func TestRestoredFailureKeepsRecord(t *testing.T) {
	proj, remote := v15Project(t)
	inv := prodInventory(prodServers(t)[0])
	run := func() error {
		res, err := runDeployPipeline(yesCmd(), proj, inv, "prod", okProber{}, filepath.Join(proj, "docker-compose.yml"), "", "", true)
		if err == nil {
			err = failedServersError(res)
		}
		return err
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(proj, deploy.DeployStateFile))
	remote.failOn = " pull"
	if err := run(); err == nil {
		t.Fatal("want the pull failure")
	}
	if firstIndex(remote.commands(), "tar -xzf") < 0 {
		t.Fatal("the failed deploy was not restored")
	}
	if after, _ := os.ReadFile(filepath.Join(proj, deploy.DeployStateFile)); string(after) != string(before) {
		t.Errorf("a restored failure rewrote deploy.json:\n%s\nvs\n%s", before, after)
	}
}

// G6: the database restore runs at most once per record, even when the re-apply
// fails and the rollback is retried.
func TestDBRestoreRunsOnce(t *testing.T) {
	proj, remote := v15Project(t)
	inv := prodInventory(prodServers(t)[0])
	cmd := yesCmd()
	if res, err := runDeployPipeline(cmd, proj, inv, "prod", okProber{}, filepath.Join(proj, "docker-compose.yml"), "", "", true); err != nil || failedServersError(res) != nil {
		t.Fatal(err)
	}
	remote.mu.Lock()
	remote.log = nil
	remote.mu.Unlock()
	remote.failOn = " pull" // the re-apply fails after the database restore
	if handled, err := rollbackRemoteV15(cmd, proj, "prod"); !handled || err == nil {
		t.Fatalf("first rollback: handled=%v err=%v, want a failure", handled, err)
	}
	st, _ := deploy.LoadDeployState(proj)
	if st.DBRestoredAt == "" || st.RolledBack {
		t.Fatalf("the restore must be persisted before the re-apply, the record not yet rolled back: %+v", st)
	}
	remote.failOn = ""
	if handled, err := rollbackRemoteV15(cmd, proj, "prod"); !handled || err != nil {
		t.Fatalf("retry: handled=%v err=%v", handled, err)
	}
	if n := countCmds(remote, "nself backup restore"); n != 1 {
		t.Errorf("backup restore ran %d times, want exactly 1", n)
	}
	if st, _ := deploy.LoadDeployState(proj); !st.RolledBack || st.BackupID != "" {
		t.Errorf("record after the retry: %+v", st)
	}
}

// RR-M1: a stateful recreate refused at E403 (after the sync and the prod backup,
// cleanly restored) leaves deploy.json byte-identical.
func TestUnconfirmedRecreateKeepsRecord(t *testing.T) {
	proj, remote := v15Project(t)
	inv := prodInventory(prodServers(t)[0])
	run := func(cmd *cobra.Command) error {
		res, err := runDeployPipeline(cmd, proj, inv, "prod", okProber{}, filepath.Join(proj, "docker-compose.yml"), "", "", true)
		if err == nil {
			err = failedServersError(res)
		}
		return err
	}
	if err := run(yesCmd()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(proj, deploy.DeployStateFile))
	remote.stateful = true
	noYes := &cobra.Command{}
	noYes.SetContext(context.Background())
	err := run(noYes)
	if err == nil || !strings.Contains(err.Error(), "E403") {
		t.Fatalf("want E403, got %v", err)
	}
	if firstIndex(remote.commands(), "nself backup verify") < 0 || firstIndex(remote.commands(), "tar -xzf") < 0 {
		t.Fatal("the refusal must come after the prod backup and be restored")
	}
	if after, _ := os.ReadFile(filepath.Join(proj, deploy.DeployStateFile)); string(after) != string(before) {
		t.Errorf("an E403 refusal rewrote deploy.json:\n%s\nvs\n%s", before, after)
	}
}

// twoHostProd opens a primary and a second prod server on separate recorders.
func twoHostProd(t *testing.T) (proj string, primary, second *v15Remote, inv *controlplane.Inventory) {
	proj, primary = v15Project(t)
	second = &v15Remote{label: "deploy@app2.example.test"}
	deployRemoteOpen = func(_ context.Context, srv controlplane.Server, _, _ string) (deploy.Remote, error) {
		if strings.Contains(srv.Host, "app2") {
			return second, nil
		}
		return primary, nil
	}
	return proj, primary, second, prodInventory(prodServers(t)...)
}

// G12: the record's success reflects the whole env. A later server refused before
// its sync makes the env record success=false, with each host's outcome.
func TestRecordReflectsWholeEnv(t *testing.T) {
	proj, _, second, inv := twoHostProd(t)
	second.locked = true // app-2 is refused at the deploy lock, before its sync
	res, err := runDeployPipeline(yesCmd(), proj, inv, "prod", okProber{}, filepath.Join(proj, "docker-compose.yml"), "", "", true)
	if err == nil {
		err = failedServersError(res)
	}
	if err == nil {
		t.Fatal("want app-2 refused")
	}
	st, _ := deploy.LoadDeployState(proj)
	if st.Success || !strings.Contains(st.Error, "app-2") {
		t.Errorf("the env record must say it failed and name app-2: success=%v error=%q", st.Success, st.Error)
	}
	if st.HostOutcomes["app-1"] != "ok" || !strings.HasPrefix(st.HostOutcomes["app-2"], "not changed") || len(st.PreDeployTars) != 1 {
		t.Errorf("per-host outcomes: %v, tars %v", st.HostOutcomes, st.PreDeployTars)
	}
}

// G12: a multi-host rollback that fails on a later host and is retried never
// restores the database a second time.
func TestPartialRollbackRetryRestoresDBOnce(t *testing.T) {
	proj, primary, second, inv := twoHostProd(t)
	cmd := yesCmd()
	if res, err := runDeployPipeline(cmd, proj, inv, "prod", okProber{}, filepath.Join(proj, "docker-compose.yml"), "", "", true); err != nil || failedServersError(res) != nil {
		t.Fatalf("deploy: %v", err)
	}
	second.failOn = " pull" // the second host's re-apply fails
	if handled, err := rollbackRemoteV15(cmd, proj, "prod"); !handled || err == nil || !strings.Contains(err.Error(), "app2") {
		t.Fatalf("first rollback: handled=%v err=%v, want a failure on app2", handled, err)
	}
	if st, _ := deploy.LoadDeployState(proj); st.DBRestoredAt == "" || st.RolledBack {
		t.Fatalf("db_restored_at must be saved and the record not rolled back: %+v", st)
	}
	second.failOn = ""
	if handled, err := rollbackRemoteV15(cmd, proj, "prod"); !handled || err != nil {
		t.Fatalf("retry: handled=%v err=%v", handled, err)
	}
	if n := countCmds(primary, "nself backup restore") + countCmds(second, "nself backup restore"); n != 1 {
		t.Errorf("backup restore ran %d times across the env, want exactly 1", n)
	}
	if st, _ := deploy.LoadDeployState(proj); !st.RolledBack {
		t.Errorf("the retry should mark the record rolled back: %+v", st)
	}
}

// H3: a server that kept changes but never ran `up` (its restore failed) must not
// put the env's database backup into the record; one that ran `up` does.
func TestRecordBackupOnlyAfterUp(t *testing.T) {
	proj := t.TempDir()
	srv := controlplane.Server{Name: "app-1", Host: "deploy@app.example.test", Primary: true}
	rec := recordRemoteApply(proj, "prod", time.Now(), "demo_full_1", "deploy@app.example.test")
	res := deploy.ApplyResult{Synced: true, BackupHost: "deploy@app.example.test", PreDeployTar: "/opt/nself/.nself/backups/pre-deploy-x.tar.gz", Changed: []string{"docker-compose.yml"}}
	rec(srv, res, errors.New("restore also failed"))
	if st, _ := deploy.LoadDeployState(proj); st.BackupID != "" || len(st.PreDeployTars) != 1 {
		t.Fatalf("no container was touched, the record must not offer the database restore: %+v", st)
	}
	res.Touched = true
	rec(srv, res, errors.New("health gate"))
	if st, _ := deploy.LoadDeployState(proj); st.BackupID != "demo_full_1" || st.BackupHost != "deploy@app.example.test" {
		t.Fatalf("after `up` the record names the backup: %+v", st)
	}
}

func rollbackRecord(t *testing.T, proj, backupHost string, tarHosts ...string) {
	t.Helper()
	st := deploy.StrategyState{Strategy: deploy.StrategyRolling, Target: "prod", Success: true, BackupID: "demo_full_9", BackupHost: backupHost,
		PreDeployTars: map[string]string{}, PreDeployChanged: map[string][]string{}, PreDeployAbsent: map[string][]string{}}
	for _, h := range tarHosts {
		st.PreDeployTars[h] = "/opt/nself/.nself/backups/pre-deploy-20261011T010101Z-aaaaaa.tar.gz"
		st.PreDeployChanged[h], st.PreDeployAbsent[h] = []string{"docker-compose.yml"}, []string{}
	}
	if err := deploy.SaveDeployState(proj, st); err != nil {
		t.Fatal(err)
	}
}

// H8: the database host is rolled back first, whatever its name sorts as.
func TestRollbackDBHostFirst(t *testing.T) {
	proj, primary, second, _ := twoHostProd(t)
	var order []string
	deployRemoteOpen = func(_ context.Context, srv controlplane.Server, _, _ string) (deploy.Remote, error) {
		order = append(order, srv.Host)
		if strings.Contains(srv.Host, "app2") {
			return second, nil
		}
		return primary, nil
	}
	rollbackRecord(t, proj, "deploy@app2.example.test", "deploy@app.example.test", "deploy@app2.example.test")
	if handled, err := rollbackRemoteV15(yesCmd(), proj, "prod"); !handled || err != nil {
		t.Fatalf("rollback: %v %v", handled, err)
	}
	if len(order) != 2 || !strings.Contains(order[0], "app2") {
		t.Errorf("rollback order %v: the backup host must be first", order)
	}
	if countCmds(second, "nself backup restore") != 1 || countCmds(primary, "nself backup restore") != 0 {
		t.Error("the database must be restored on the backup host only")
	}
}

// H8: when the backup host kept no changes there is no database restore; the
// record keeps the backup id so it can be restored by hand.
func TestRollbackWithoutBackupHostTar(t *testing.T) {
	proj, primary, second, _ := twoHostProd(t)
	rollbackRecord(t, proj, "deploy@app.example.test", "deploy@app2.example.test")
	if handled, err := rollbackRemoteV15(yesCmd(), proj, "prod"); !handled || err != nil {
		t.Fatalf("rollback: %v %v", handled, err)
	}
	if countCmds(primary, "nself backup restore")+countCmds(second, "nself backup restore") != 0 {
		t.Error("the database was restored although the backup host kept no changes")
	}
	if st, _ := deploy.LoadDeployState(proj); st.BackupID != "demo_full_9" || !st.RolledBack {
		t.Errorf("the record must keep the backup id for a manual restore: %+v", st)
	}
}

const rollbackSignalEnv = "NSELF_DEPLOY_ROLLBACK_SIGNAL_HELPER"

// TestRollbackInterruptHelper is the child process; it does nothing unless re-executed.
func TestRollbackInterruptHelper(t *testing.T) {
	if os.Getenv(rollbackSignalEnv) == "" {
		t.Skip("helper process for TestRollbackInterrupt")
	}
	proj, remote := v15Project(t)
	inv := prodInventory(prodServers(t)[0])
	cmd := yesCmd()
	if res, err := runDeployPipeline(cmd, proj, inv, "prod", okProber{}, filepath.Join(proj, "docker-compose.yml"), "", "", true); err != nil || failedServersError(res) != nil {
		t.Fatalf("deploy: %v", err)
	}
	remote.mu.Lock()
	remote.log = nil
	remote.hang = " pull" // the rollback's re-apply is in flight when the signal arrives
	remote.mu.Unlock()
	_, err := rollbackRemoteV15(cmd, proj, "prod")
	st, _ := deploy.LoadDeployState(proj)
	fmt.Printf("RESULT err=%v restored=%v unlocked=%v rolledback=%v\n", err != nil, firstIndex(remote.commands(), "tar -xzf") >= 0,
		countCmds(remote, "/holder; rmdir") == 1, st.RolledBack)
}

// runSignalChild re-executes the test binary on helper, sends SIGINT when it prints
// READY and returns its RESULT line. A command entry that does not trap the signal
// dies from it and reports nothing, which fails the caller.
func runSignalChild(t *testing.T, helper string) string {
	t.Helper()
	c := exec.Command(os.Args[0], "-test.run=^"+helper+"$", "-test.count=1")
	c.Env = append(os.Environ(), rollbackSignalEnv+"=1")
	out, err := c.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Process.Kill(); _ = c.Wait() })
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	deadline := time.After(90 * time.Second)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatalf("%s exited without a result (the signal was not trapped?)", helper)
			}
			if l == "READY" {
				_ = c.Process.Signal(syscall.SIGINT)
			}
			if strings.HasPrefix(l, "RESULT ") {
				return l
			}
		case <-deadline:
			t.Fatal("timed out")
		}
	}
}

// H-rollback: a real SIGINT during a rollback stops it, leaves the lock released
// and the record not marked rolled back, so the rollback can be retried.
func TestRollbackInterrupt(t *testing.T) {
	if got, want := runSignalChild(t, "TestRollbackInterruptHelper"), "RESULT err=true restored=true unlocked=true rolledback=false"; got != want {
		t.Errorf("after SIGINT:\n got %s\nwant %s", got, want)
	}
}

func applyInterruptResult(remote *v15Remote, proj string, err error) {
	st, _ := deploy.LoadDeployState(proj)
	fmt.Printf("RESULT err=%v restored=%v unlocked=%v recorded=%v\n", err != nil, firstIndex(remote.commands(), "tar -xzf") >= 0,
		countCmds(remote, "/holder; rmdir") == 1, st.PreDeployTar != "")
}

const applyInterruptWant = "RESULT err=true restored=true unlocked=true recorded=false"

// TestPipelineInterruptHelper is the child process of TestPipelineInterrupt.
func TestPipelineInterruptHelper(t *testing.T) {
	if os.Getenv(rollbackSignalEnv) == "" {
		t.Skip("helper process")
	}
	proj, remote := v15Project(t)
	remote.hang = " pull"
	res, err := runDeployPipeline(yesCmd(), proj, prodInventory(prodServers(t)[0]), "prod", okProber{}, filepath.Join(proj, "docker-compose.yml"), "", "", true)
	if err == nil {
		err = failedServersError(res)
	}
	applyInterruptResult(remote, proj, err)
}

// J3: the pipeline's v1.5 branch traps a real SIGINT: the apply is restored, the
// lock released and the record untouched.
func TestPipelineInterrupt(t *testing.T) {
	if got := runSignalChild(t, "TestPipelineInterruptHelper"); got != applyInterruptWant {
		t.Errorf("after SIGINT:\n got %s\nwant %s", got, applyInterruptWant)
	}
}

// TestLegacyPushInterruptHelper is the child process of TestLegacyPushInterrupt.
func TestLegacyPushInterruptHelper(t *testing.T) {
	if os.Getenv(rollbackSignalEnv) == "" {
		t.Skip("helper process")
	}
	proj, remote := v15Project(t)
	t.Setenv("NSELF_DEPLOY_SSH_KEY", "/tmp/key")
	remote.hang = " pull"
	ctx := context.WithValue(context.Background(), deployYesKey{}, true)
	err := remoteDeployPushDispatch(ctx, proj, "deploy@app.example.test:/srv/nself", "staging", true)
	applyInterruptResult(remote, proj, err)
}

// J3: the legacy single-host push (v1.5) traps a real SIGINT the same way.
func TestLegacyPushInterrupt(t *testing.T) {
	if got := runSignalChild(t, "TestLegacyPushInterruptHelper"); got != applyInterruptWant {
		t.Errorf("after SIGINT:\n got %s\nwant %s", got, applyInterruptWant)
	}
}

// J1: Ctrl-C at the confirmation prompt answers no at once; the blocking read does
// not swallow it. An answer of yes still passes.
func TestConfirmReturnsOnInterrupt(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	var out strings.Builder
	go func() { done <- confirmFrom(ctx, pr, &out, "recreates db") }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case ok := <-done:
		if ok {
			t.Error("an interrupted prompt must answer no")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the prompt kept waiting after the interrupt")
	}
	if !confirmFrom(context.Background(), strings.NewReader("yes\n"), &out, "x") || confirmFrom(context.Background(), strings.NewReader("no\n"), &out, "x") {
		t.Error("yes must confirm and no must not")
	}
}

// J3: a record without the list of changed paths cannot be rolled back; nothing
// is sent to the host.
func TestRollbackNeedsRecordedChangeList(t *testing.T) {
	proj, primary, second, _ := twoHostProd(t)
	rollbackRecord(t, proj, "deploy@app.example.test", "deploy@app.example.test")
	st, _ := deploy.LoadDeployState(proj)
	st.PreDeployChanged = nil
	if err := deploy.SaveDeployState(proj, st); err != nil {
		t.Fatal(err)
	}
	handled, err := rollbackRemoteV15(yesCmd(), proj, "prod")
	if !handled || err == nil || !strings.Contains(err.Error(), "no list of changed paths") {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if len(primary.commands())+len(second.commands()) != 0 {
		t.Errorf("a refused rollback touched the host: %v", primary.commands())
	}
}

// J3: the recorded absent list reaches the rollback, which removes exactly those files.
func TestRecordedAbsentListDrivesRollbackRemoval(t *testing.T) {
	proj, primary, _, _ := twoHostProd(t)
	srv := controlplane.Server{Name: "app-1", Host: "deploy@app.example.test", Primary: true}
	host := "deploy@app.example.test"
	rec := recordRemoteApply(proj, "prod", time.Now(), "", "")
	rec(srv, deploy.ApplyResult{Synced: true, Touched: true, BackupHost: host, PreDeployTar: "/opt/nself/.nself/backups/pre-deploy-20261011T010101Z-aaaaaa.tar.gz",
		Changed: []string{"docker-compose.yml", "nginx/sites/new.conf"}, Absent: []string{"nginx/sites/new.conf"}}, nil)
	if st, _ := deploy.LoadDeployState(proj); len(st.PreDeployAbsent[host]) != 1 || st.PreDeployAbsent[host][0] != "nginx/sites/new.conf" {
		t.Fatalf("the absent list was not recorded: %+v", st.PreDeployAbsent)
	}
	if handled, err := rollbackRemoteV15(yesCmd(), proj, "prod"); !handled || err != nil {
		t.Fatalf("rollback: %v %v", handled, err)
	}
	removed := false
	for _, c := range primary.commands() {
		if strings.Contains(c, "'nginx/sites/new.conf'") && strings.Contains(c, `rm -f -- "$2"`) {
			removed = true
		}
	}
	if !removed {
		t.Errorf("the rollback did not remove the file the deploy created: %v", primary.commands())
	}
}

// K4: a bad record on a later host refuses the whole rollback before any host is
// opened, so the database host is not rolled back (nor its database restored).
func TestRollbackValidatesEveryHostFirst(t *testing.T) {
	proj, primary, second, _ := twoHostProd(t)
	opened := 0
	deployRemoteOpen = func(_ context.Context, srv controlplane.Server, _, _ string) (deploy.Remote, error) {
		opened++
		return primary, nil
	}
	rollbackRecord(t, proj, "deploy@app.example.test", "deploy@app.example.test", "deploy@app2.example.test")
	st, _ := deploy.LoadDeployState(proj)
	delete(st.PreDeployChanged, "deploy@app2.example.test") // a legacy or tampered record for the second host
	if err := deploy.SaveDeployState(proj, st); err != nil {
		t.Fatal(err)
	}
	handled, err := rollbackRemoteV15(yesCmd(), proj, "prod")
	if !handled || err == nil || !strings.Contains(err.Error(), "app2.example.test") || !strings.Contains(err.Error(), "no list of changed paths") {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if opened != 0 || len(primary.commands())+len(second.commands()) != 0 {
		t.Errorf("a refused rollback opened %d remotes and ran %v", opened, primary.commands())
	}
	if after, _ := deploy.LoadDeployState(proj); after.DBRestoredAt != "" || after.RolledBack {
		t.Errorf("the record changed: %+v", after)
	}
	// A malformed tar entry on the second host is refused the same way.
	st, _ = deploy.LoadDeployState(proj)
	st.PreDeployChanged["deploy@app2.example.test"] = []string{"docker-compose.yml"}
	st.PreDeployTars["deploy@app2.example.test"] = "/etc/passwd"
	_ = deploy.SaveDeployState(proj, st)
	if _, err := rollbackRemoteV15(yesCmd(), proj, "prod"); err == nil || !strings.Contains(err.Error(), "not a pre-deploy tar") || opened != 0 {
		t.Errorf("bad tar entry: err=%v opened=%d", err, opened)
	}
}

// N4: the prune is driven by the last recorded deploy of the same target: with
// the record it runs against the dropped file, with another target's record or none
// it does not. Silently switching the prune off, or feeding it the wrong record, fails.
func TestPruneFollowsTheRecordedDeploy(t *testing.T) {
	const old = "nginx/sites/old.conf"
	for name, tc := range map[string]struct {
		record string
		pruned bool
	}{"record of this target": {"prod", true}, "record of another target": {"staging", false}, "no record": {"", false}} {
		t.Run(name, func(t *testing.T) {
			proj, remote := v15Project(t)
			remote.shipped = old + "\n" // the host's own list names it too
			if tc.record != "" {
				st := deploy.StrategyState{Strategy: deploy.StrategyRolling, Target: tc.record, Success: true,
					PreDeployChanged: map[string][]string{"deploy@app.example.test": {old, "docker-compose.yml"}}}
				if err := deploy.SaveDeployState(proj, st); err != nil {
					t.Fatal(err)
				}
			}
			res, err := runDeployPipeline(yesCmd(), proj, prodInventory(prodServers(t)[0]), "prod", okProber{}, filepath.Join(proj, "docker-compose.yml"), "", "", true)
			if err == nil {
				err = failedServersError(res)
			}
			if err != nil {
				t.Fatal(err)
			}
			pruned := false
			for _, c := range remote.commands() {
				if strings.Contains(c, "head -n 5") && strings.Contains(c, "'"+old+"'") {
					pruned = true
				}
			}
			if pruned != tc.pruned {
				t.Errorf("prune of %s ran = %v, want %v", old, pruned, tc.pruned)
			}
		})
	}
}

// TestPromptInterruptHelper is the child process of TestPromptInterrupt.
func TestPromptInterruptHelper(t *testing.T) {
	if os.Getenv(rollbackSignalEnv) == "" {
		t.Skip("helper process")
	}
	proj, remote := v15Project(t)
	remote.stateful = true // the preview wants a confirmation
	old := deployConfirmPrompt
	defer func() { deployConfirmPrompt = old }()
	deployConfirmPrompt = func(ctx context.Context, _ string) bool {
		fmt.Println("READY")
		<-ctx.Done()
		return false
	}
	noYes := &cobra.Command{}
	noYes.SetContext(context.Background())
	res, err := runDeployPipeline(noYes, proj, prodInventory(prodServers(t)[0]), "prod", okProber{}, filepath.Join(proj, "docker-compose.yml"), "", "", true)
	if err == nil {
		err = failedServersError(res)
	}
	applyInterruptResult(remote, proj, err)
}

// N5: the command hands the interrupt-aware context to the confirmation prompt: a
// real SIGINT at the prompt ends the command, restored and unlocked.
func TestPromptInterrupt(t *testing.T) {
	if got := runSignalChild(t, "TestPromptInterruptHelper"); got != applyInterruptWant {
		t.Errorf("after SIGINT at the prompt:\n got %s\nwant %s", got, applyInterruptWant)
	}
}

// P1: the legacy push keeps the target's own name: for a prod-class env called
// "live" the record says "live", so `deploy rollback live` finds it.
func TestLegacyPushRecordKeepsTargetName(t *testing.T) {
	proj, _ := v15Project(t)
	t.Setenv("NSELF_DEPLOY_SSH_KEY", "/tmp/key")
	orig := prodClassFn
	prodClassFn = func(i *controlplane.Inventory, env string) bool { return env == "live" || orig(i, env) }
	t.Cleanup(func() { prodClassFn = orig })
	ctx := context.WithValue(context.Background(), deployYesKey{}, true)
	if err := remoteDeployPushDispatch(ctx, proj, "deploy@app.example.test:/srv/nself", "live", true); err != nil {
		t.Fatal(err)
	}
	if st, _ := deploy.LoadDeployState(proj); st.Target != "live" || st.BackupID == "" {
		t.Fatalf("record %+v: want target live with the prod backup", st)
	}
	if handled, err := rollbackRemoteV15(yesCmd(), proj, "live"); !handled || err != nil {
		t.Fatalf("rollback live: handled=%v err=%v", handled, err)
	}
}
