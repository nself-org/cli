package controlplane

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
)

// TestRemoteApplyDispatch: in v1.5 a payload with an artifact set goes through
// deploy.ApplyRemote on every deployed server (obs, app, app); in v1.4 the same
// run keeps DeployViaSsh and never opens a Remote.
func TestRemoteApplyDispatch(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		composePath := fakeDeploy(t)
		oldDeploy, oldRemote := deployServerFn, newRemoteFn
		defer func() { deployServerFn, newRemoteFn = oldDeploy, oldRemote }()
		var viaSSH, opened []string
		deployServerFn = func(_ context.Context, cfg deploy.SSHConfig, _ string) error {
			viaSSH = append(viaSSH, cfg.ServerName)
			return nil
		}
		newRemoteFn = func(_ context.Context, srv Server, _, _ string) (deploy.Remote, error) {
			opened = append(opened, srv.Name)
			return &recRemote{}, nil
		}
		var recorded []string
		ra := &RemoteApply{
			Set: deploy.ArtifactSet{RemotePath: "/opt/nself", StageDir: t.TempDir()},
			Record: func(srv Server, res deploy.ApplyResult, err error) {
				recorded = append(recorded, srv.Name)
			},
		}
		result, err := RunWithApply(context.Background(), fourServerInventory(), "staging", &pipelineStubProber{}, composePath, ra)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(t.Name(), "v1.5") {
			if len(viaSSH) != 0 || strings.Join(opened, ",") != "obs-01,app-01,app-02" || strings.Join(recorded, ",") != "obs-01,app-01,app-02" {
				t.Fatalf("v1.5: DeployViaSsh %v, remotes opened %v, recorded %v", viaSSH, opened, recorded)
			}
			// The empty recorder fails the sync; every failure is reported per server.
			failed := 0
			for _, s := range result.Servers {
				if s.Status == "failed" {
					failed++
				}
			}
			if failed != 3 {
				t.Errorf("failed servers = %d, want 3 (rsync is refused by the recorder)", failed)
			}
			return
		}
		if len(opened) != 0 || strings.Join(viaSSH, ",") != "obs-01,app-01,app-02" {
			t.Fatalf("v1.4: DeployViaSsh %v, remotes opened %v", viaSSH, opened)
		}
	})
}

func TestRemoteApplyRefusesMismatchedRemotePath(t *testing.T) {
	compattest.Set(t, true)
	composePath := fakeDeploy(t)
	inv := fourServerInventory()
	env := inv.Environments["staging"]
	env.Servers[2].RemotePath = "/srv/other" // app-02
	inv.Environments["staging"] = env
	oldRemote := newRemoteFn
	defer func() { newRemoteFn = oldRemote }()
	newRemoteFn = func(_ context.Context, srv Server, _, _ string) (deploy.Remote, error) { return &recRemote{}, nil }
	ra := &RemoteApply{Set: deploy.ArtifactSet{RemotePath: "/opt/nself", StageDir: t.TempDir()}}
	result, err := RunWithApply(context.Background(), inv, "staging", &pipelineStubProber{}, composePath, ra)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range result.Servers {
		if s.Server != "app-02" {
			continue
		}
		var ce *errs.CLIError
		if s.Status != "failed" || !errors.As(s.Err, &ce) || ce.Code != "E485" || !strings.Contains(s.Err.Error(), "/srv/other") {
			t.Fatalf("app-02: %s %v, want E485 naming the path", s.Status, s.Err)
		}
	}
}
