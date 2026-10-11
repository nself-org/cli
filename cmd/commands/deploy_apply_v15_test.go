package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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
	mu  sync.Mutex
	log []string
	tag string
}

func (r *v15Remote) Label() string { return "deploy@app.example.test" }

func (r *v15Remote) Rsync(_ context.Context, flags []string, _, _, dest string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, "rsync "+strings.Join(flags, " ")+" -> "+dest)
	return nil
}

var (
	quotedPath = regexp.MustCompile(`'(/[^']+)'`)
	tagArg     = regexp.MustCompile(`--tag '([^']+)'`)
)

func (r *v15Remote) Run(_ context.Context, cmd string) ([]byte, error) {
	s := cmd
	if rest, ok := strings.CutPrefix(cmd, "sh -c '"); ok {
		s = strings.ReplaceAll(strings.TrimSuffix(rest, "'"), `'\''`, `'`)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, s)
	switch {
	case strings.Contains(s, "stat -c"):
		var b strings.Builder
		for _, m := range quotedPath.FindAllStringSubmatch(s, -1) {
			fmt.Fprintf(&b, "600 %s\n", m[1])
		}
		return []byte(b.String()), nil
	case strings.Contains(s, `"$HOME"`):
		return []byte("/home/deploy\n"), nil
	case strings.Contains(s, "config --format json"):
		return []byte(`{"name":"demo","services":{"web":{"image":"web:1","restart":"always"}}}`), nil
	case strings.Contains(s, "config --services"):
		return []byte("web\n"), nil
	case strings.Contains(s, "nself backup create"):
		r.tag = tagArg.FindStringSubmatch(s)[1]
		return nil, nil
	case strings.Contains(s, "nself backup list"):
		return []byte(`[{"id":"demo_full_20261010_120000_` + r.tag + `","tag":"120000_` + r.tag + `","date":"2026-10-10T12:00:00Z"}]`), nil
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
	order := []string{`"$HOME"`, "tar -czf", "rsync ", "stat -c", "config --format json", "up -d --dry-run", "nself backup create", "nself backup verify", " pull", "up -d --no-deps 'web'", "hasura metadata apply"}
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
	for _, want := range []string{"tar -xzf", "nself backup restore '" + st.BackupID + "' --yes", "config --format json", " pull", "up -d --no-deps 'web'"} {
		i := firstIndex(cmds, want)
		if i <= prev {
			t.Fatalf("rollback: %q at %d after %d in:\n%s", want, i, prev, strings.Join(cmds, "\n"))
		}
		prev = i
	}
	if after, _ := deploy.LoadDeployState(proj); !after.RolledBack {
		t.Error("deploy.json does not say the deploy was rolled back")
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
	if firstIndex(cmds, "tar -czf") < 0 || firstIndex(cmds, "up -d --no-deps 'web'") < 0 || firstIndex(cmds, "rsync ") < 0 {
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
