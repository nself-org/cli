package controlplane

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/deploy"
)

// pipelineStubProber is a test Prober that always succeeds (full manage capability).
type pipelineStubProber struct {
	sshFn    func(s Server) (bool, int, error)
	dockerFn func(s Server) (bool, error)
	keyFn    func(s Server) (bool, string)
}

func (p *pipelineStubProber) SSHReachable(s Server) (bool, int, error) {
	if p.sshFn != nil {
		return p.sshFn(s)
	}
	return true, 5, nil
}
func (p *pipelineStubProber) DockerOK(s Server) (bool, error) {
	if p.dockerFn != nil {
		return p.dockerFn(s)
	}
	return true, nil
}
func (p *pipelineStubProber) KeyState(s Server) (bool, string) {
	if p.keyFn != nil {
		return p.keyFn(s)
	}
	return true, "/tmp/key"
}

// fakeDeploy installs fake SSH and rsync binaries that always exit 0, sets up
// a fake key, and returns the compose path. deploy.DeployViaSsh calls rsync
// (step 1) before SSH, so both must be faked for the deploy to succeed.
// On Windows, shebang scripts cannot be executed directly, so callers must
// skip the test before calling fakeDeploy.
func fakeDeploy(t *testing.T) (composePath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: requires Unix shell-script PATH override")
	}
	dir := t.TempDir()

	// Fake SSH binary exits 0.
	fakeSSH := filepath.Join(dir, "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}

	// Fake rsync binary exits 0 (deploy.DeployViaSsh calls rsync first).
	fakeRsync := filepath.Join(dir, "rsync")
	if err := os.WriteFile(fakeRsync, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write fake rsync: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPath)

	// Fake key file.
	keyFile := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyFile, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("write fake key: %v", err)
	}
	t.Setenv("NSELF_SSH_KEY_STAGING", keyFile)

	// Fake compose file.
	composePath = filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	return composePath
}

// fourServerInventory builds an inventory with one server per role, all in
// the "staging" environment (non-local).
func fourServerInventory() *Inventory {
	return &Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]Environment{
			"staging": {
				Kind: "remote",
				Servers: []Server{
					{Name: "obs-01", Role: RoleObservability, Host: "ubuntu@obs.example.com", SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself"},
					{Name: "app-01", Role: RoleApp, Host: "ubuntu@app.example.com", SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself", Primary: true},
					{Name: "app-02", Role: RoleApp, Host: "ubuntu@app2.example.com", SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself"},
					{Name: "lb-01", Role: RoleLB, Host: "ubuntu@lb.example.com", SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself"},
				},
			},
		},
	}
}

// TestRunReturnsAllServers verifies that Run returns one ServerResult per
// non-hidden server in the inventory.
func TestRunReturnsAllServers(t *testing.T) {
	composePath := fakeDeploy(t)

	inv := fourServerInventory()
	prober := &pipelineStubProber{}

	result, err := Run(context.Background(), inv, "staging", prober, composePath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Expect 4 results (obs-01, app-01, app-02, lb-01).
	if len(result.Servers) != 4 {
		t.Errorf("len(result.Servers) = %d, want 4", len(result.Servers))
	}
}

// TestRunDeployOrderObsBeforeApp verifies that observability servers are
// recorded before app servers in the results.
func TestRunDeployOrderObsBeforeApp(t *testing.T) {
	composePath := fakeDeploy(t)

	inv := fourServerInventory()
	prober := &pipelineStubProber{}

	result, err := Run(context.Background(), inv, "staging", prober, composePath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Find first obs and first app index.
	obsIdx, appIdx := -1, -1
	for i, sr := range result.Servers {
		if sr.Role == RoleObservability && obsIdx < 0 {
			obsIdx = i
		}
		if sr.Role == RoleApp && appIdx < 0 {
			appIdx = i
		}
	}
	if obsIdx < 0 {
		t.Fatal("no observability server in results")
	}
	if appIdx < 0 {
		t.Fatal("no app server in results")
	}
	if obsIdx > appIdx {
		t.Errorf("obs server at index %d is AFTER app server at index %d; want obs before app", obsIdx, appIdx)
	}
}

// TestRunLBAfterApp verifies that LB servers are recorded after app servers.
func TestRunLBAfterApp(t *testing.T) {
	composePath := fakeDeploy(t)

	inv := fourServerInventory()
	prober := &pipelineStubProber{}

	result, err := Run(context.Background(), inv, "staging", prober, composePath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	lastAppIdx := -1
	firstLBIdx := -1
	for i, sr := range result.Servers {
		if sr.Role == RoleApp {
			lastAppIdx = i
		}
		if sr.Role == RoleLB && firstLBIdx < 0 {
			firstLBIdx = i
		}
	}
	if lastAppIdx < 0 || firstLBIdx < 0 {
		t.Fatal("missing app or lb server in results")
	}
	if firstLBIdx <= lastAppIdx {
		t.Errorf("lb server at index %d is NOT after last app server at index %d; want lb last", firstLBIdx, lastAppIdx)
	}
}

// TestRunReadOnlyServerSkipped verifies that a read-only server gets Status
// "skipped" in the result.
func TestRunReadOnlyServerSkipped(t *testing.T) {
	composePath := fakeDeploy(t)

	inv := &Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]Environment{
			"staging": {
				Kind: "remote",
				Servers: []Server{
					{Name: "app-ro", Role: RoleApp, Host: "ubuntu@ro.example.com", SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself", Primary: true},
				},
			},
		},
	}

	// Prober returns SSH unreachable → read-only capability.
	prober := &pipelineStubProber{
		sshFn: func(s Server) (bool, int, error) {
			return false, 0, errors.New("connection refused")
		},
		keyFn: func(s Server) (bool, string) { return true, "/tmp/key" },
	}

	result, err := Run(context.Background(), inv, "staging", prober, composePath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(result.Servers) != 1 {
		t.Fatalf("expected 1 server result, got %d", len(result.Servers))
	}
	if result.Servers[0].Status != "skipped" {
		t.Errorf("Status = %q, want %q", result.Servers[0].Status, "skipped")
	}
}

// TestRunPrimaryAppSkippedSetsPrimarySkipped verifies that if a primary app
// server is skipped, DeployResult.PrimarySkipped is true.
func TestRunPrimaryAppSkippedSetsPrimarySkipped(t *testing.T) {
	composePath := fakeDeploy(t)

	inv := &Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]Environment{
			"staging": {
				Kind: "remote",
				Servers: []Server{
					{Name: "app-primary", Role: RoleApp, Host: "ubuntu@primary.example.com", SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself", Primary: true},
				},
			},
		},
	}

	// Make SSH fail → read-only → skipped.
	prober := &pipelineStubProber{
		sshFn: func(s Server) (bool, int, error) {
			return false, 0, errors.New("timeout")
		},
		keyFn: func(s Server) (bool, string) { return true, "/tmp/key" },
	}

	result, err := Run(context.Background(), inv, "staging", prober, composePath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.PrimarySkipped {
		t.Error("PrimarySkipped = false, want true when primary app server is skipped")
	}
}

// TestRunPrimarySkippedFalseWhenAllOK verifies PrimarySkipped is false on a
// successful full deploy.
func TestRunPrimarySkippedFalseWhenAllOK(t *testing.T) {
	composePath := fakeDeploy(t)

	inv := &Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]Environment{
			"staging": {
				Kind: "remote",
				Servers: []Server{
					{Name: "app-primary", Role: RoleApp, Host: "ubuntu@primary.example.com", SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself", Primary: true},
				},
			},
		},
	}

	prober := &pipelineStubProber{}

	result, err := Run(context.Background(), inv, "staging", prober, composePath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.PrimarySkipped {
		t.Error("PrimarySkipped = true, want false when primary app server deployed OK")
	}
}

// TestRunHiddenServerOmitted verifies that a hidden server (CapHidden) does not
// appear in the results.
func TestRunHiddenServerOmitted(t *testing.T) {
	composePath := fakeDeploy(t)

	inv := &Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]Environment{
			"staging": {
				Kind: "remote",
				Servers: []Server{
					// Empty host → resolver assigns CapHidden.
					{Name: "ghost", Role: RoleApp, Host: "", SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself"},
					// Normal app server.
					{Name: "app-visible", Role: RoleApp, Host: "ubuntu@app.example.com", SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself"},
				},
			},
		},
	}

	prober := &pipelineStubProber{}

	result, err := Run(context.Background(), inv, "staging", prober, composePath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, sr := range result.Servers {
		if sr.Server == "ghost" {
			t.Error("hidden server 'ghost' appeared in results; it should be silently omitted")
		}
	}
	if len(result.Servers) != 1 {
		t.Errorf("expected 1 result (visible app only), got %d", len(result.Servers))
	}
}

// TestRunLocalEnvironmentReturnsUnsupportedError verifies that Run returns a
// descriptive error when the inventory contains a local-kind environment.
// The pipeline cannot drive local Docker stacks — that responsibility belongs
// to `nself build` + `nself start`. Returning a clear error (rather than a
// silent no-op) prevents false-success exits from `nself deploy --target local`.
func TestRunLocalEnvironmentReturnsUnsupportedError(t *testing.T) {
	composePath := fakeDeploy(t)

	inv := &Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]Environment{
			"local": {
				Kind: "local",
				Servers: []Server{
					{Name: "local-app", Role: RoleApp, Host: "localhost", SSHKeyRef: "", RemotePath: "/opt/nself"},
				},
			},
		},
	}

	prober := &pipelineStubProber{}

	_, err := Run(context.Background(), inv, "local", prober, composePath)
	if err == nil {
		t.Fatal("Run: expected error for local-kind environment, got nil (local deploy is not supported by the pipeline)")
	}
	if !strings.Contains(err.Error(), "not yet supported") {
		t.Errorf("Run: error message should mention 'not yet supported'; got: %v", err)
	}
}

// TestRunAllStatusOKWhenHealthy verifies that every server result has status
// "ok" when all probes pass and SSH succeeds.
func TestRunAllStatusOKWhenHealthy(t *testing.T) {
	composePath := fakeDeploy(t)

	inv := &Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]Environment{
			"staging": {
				Kind: "remote",
				Servers: []Server{
					{Name: "obs-01", Role: RoleObservability, Host: "ubuntu@obs.example.com", SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself"},
					{Name: "app-01", Role: RoleApp, Host: "ubuntu@app.example.com", SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself", Primary: true},
				},
			},
		},
	}

	prober := &pipelineStubProber{}

	result, err := Run(context.Background(), inv, "staging", prober, composePath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, sr := range result.Servers {
		if sr.Status != "ok" {
			t.Errorf("server %q: status = %q, want %q", sr.Server, sr.Status, "ok")
		}
	}
}

// TestRunSkippedLineWrittenToStderr verifies that a skipped server emits a
// SKIPPED line (captured via auditWriter redirect of stderr-equivalent).
func TestRunSkippedLineWrittenToStderr(t *testing.T) {
	composePath := fakeDeploy(t)

	// Redirect stderr-equivalent via the logSkipped function's fmt.Fprintf(os.Stderr).
	// We capture by replacing os.Stderr with a pipe.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origStderr })

	inv := &Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]Environment{
			"staging": {
				Kind: "remote",
				Servers: []Server{
					{Name: "app-skip", Role: RoleApp, Host: "ubuntu@skip.example.com", SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself"},
				},
			},
		},
	}

	// Force read-only.
	prober := &pipelineStubProber{
		sshFn: func(s Server) (bool, int, error) {
			return false, 0, errors.New("refused")
		},
		keyFn: func(s Server) (bool, string) { return true, "/tmp/key" },
	}

	if _, runErr := Run(context.Background(), inv, "staging", prober, composePath); runErr != nil {
		_ = w.Close()
		t.Fatalf("Run: %v", runErr)
	}
	_ = w.Close()

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "SKIPPED") {
		t.Errorf("expected SKIPPED line in stderr output; got: %q", output)
	}
}

// TestCheckPrimarySkipped unit tests the checkPrimarySkipped helper directly.
func TestCheckPrimarySkipped(t *testing.T) {
	result := &DeployResult{}

	// Non-primary skipped — should not set PrimarySkipped.
	checkPrimarySkipped(result, []ServerResult{
		{Primary: false, Status: "skipped"},
	})
	if result.PrimarySkipped {
		t.Error("PrimarySkipped should be false for non-primary skipped server")
	}

	// Primary skipped — should set PrimarySkipped.
	checkPrimarySkipped(result, []ServerResult{
		{Primary: true, Status: "skipped"},
	})
	if !result.PrimarySkipped {
		t.Error("PrimarySkipped should be true for primary skipped server")
	}
}

// TestFilterEnv verifies that filterEnv returns only statuses for the named env.
func TestFilterEnv(t *testing.T) {
	statuses := []TargetStatus{
		{Env: "staging", Server: "app-01", ProbedAt: time.Now()},
		{Env: "prod", Server: "app-02", ProbedAt: time.Now()},
		{Env: "staging", Server: "obs-01", ProbedAt: time.Now()},
	}

	got := filterEnv(statuses, "staging")
	if len(got) != 2 {
		t.Errorf("filterEnv staging: got %d, want 2", len(got))
	}
	for _, s := range got {
		if s.Env != "staging" {
			t.Errorf("filterEnv returned status with Env=%q, want staging", s.Env)
		}
	}
}

// TestServersByRoleExcludesHidden verifies that serversByRole omits hidden servers.
func TestServersByRoleExcludesHidden(t *testing.T) {
	env := Environment{
		Kind: "remote",
		Servers: []Server{
			{Name: "app-visible", Role: RoleApp, Host: "ubuntu@app.example.com"},
			{Name: "app-hidden", Role: RoleApp, Host: ""},
		},
	}
	statuses := []TargetStatus{
		{Server: "app-visible", Env: "test", Capability: CapManage},
		{Server: "app-hidden", Env: "test", Capability: CapHidden},
	}

	pairs := serversByRole(env, statuses, RoleApp)
	if len(pairs) != 1 {
		t.Errorf("serversByRole: got %d pairs, want 1 (hidden excluded)", len(pairs))
	}
	if len(pairs) > 0 && pairs[0].srv.Name != "app-visible" {
		t.Errorf("serversByRole: got %q, want %q", pairs[0].srv.Name, "app-visible")
	}
}

// recordingProber records every host the pipeline probes, so a test can prove
// that a cross-environment host was never contacted.
type recordingProber struct {
	pipelineStubProber
	hosts []string
}

func (p *recordingProber) SSHReachable(s Server) (bool, int, error) {
	p.hosts = append(p.hosts, s.Host)
	return p.pipelineStubProber.SSHReachable(s)
}

// threeEnvInventory has qa, staging and prod, each with one app server on a
// distinct host.
func threeEnvInventory() *Inventory {
	mk := func(env, host string) Environment {
		return Environment{Name: env, Kind: "remote", Servers: []Server{
			{Name: env + "-app", Role: RoleApp, Host: host, SSHKeyRef: "NSELF_SSH_KEY_STAGING", RemotePath: "/opt/nself", Primary: true},
		}}
	}
	return &Inventory{SchemaVersion: 1, Project: "test", Environments: map[string]Environment{
		"qa":      mk("qa", "u@qa.example.test"),
		"staging": mk("staging", "u@staging.example.test"),
		"prod":    mk("prod", "u@prod.example.test"),
	}}
}

// stubDeployServer swaps the per-server SSH deploy for a recorder; the
// returned slice pointer holds the hosts it was called with.
func stubDeployServer(t *testing.T) *[]string {
	t.Helper()
	var got []string
	orig := deployServerFn
	deployServerFn = func(_ context.Context, cfg deploy.SSHConfig, _ string) error {
		got = append(got, cfg.Host)
		return nil
	}
	t.Cleanup(func() { deployServerFn = orig })
	return &got
}

func TestPipelinePortHostSeparatePath(t *testing.T) {
	orig := deployServerFn
	defer func() { deployServerFn = orig }()
	called := false
	deployServerFn = func(_ context.Context, cfg deploy.SSHConfig, _ string) error {
		called = true
		if cfg.Host != "u@port.example:2222" || cfg.RemotePath != "/opt/nself" || cfg.ServerName != "web" {
			t.Fatalf("bad deploy destination: %+v", cfg)
		}
		return nil
	}
	srv := Server{Name: "web", Role: RoleApp, Host: "u@port.example:2222", RemotePath: "/opt/nself"}
	result := deployApp(context.Background(), "qa", srv, TargetStatus{}, payload{}, false, nil)
	if !called || result.Status != "ok" {
		t.Fatalf("pipeline deploy: called=%v result=%+v", called, result)
	}
	called = false
	srv.Role = RoleDB
	result = deployOne(context.Background(), "qa", srv, TargetStatus{}, payload{})
	if !called || result.Status != "ok" {
		t.Fatalf("pipeline non-app deploy: called=%v result=%+v", called, result)
	}
}

func TestPipelineE487InventoryEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX command stubs")
	}
	for _, envName := range []string{"live", "production"} {
		for _, deployKind := range []string{"app", "one"} {
			t.Run(envName+"/"+deployKind, func(t *testing.T) {
				root := t.TempDir()
				t.Setenv("HOME", t.TempDir())
				t.Setenv("NSELF_V15", "1")
				host := "u@host.example:2222"
				srv := Server{Name: "web", Role: RoleApp, Host: host, RemotePath: "/opt/nself", Primary: true}
				inv := &Inventory{SchemaVersion: 2, Environments: map[string]Environment{
					"local": {Name: "local", Kind: "local", Tier: TierLocal},
					envName: {Name: envName, Kind: "remote", Tier: TierProd, Servers: []Server{srv}},
				}}
				if err := Write(root, inv); err != nil {
					t.Fatal(err)
				}
				child := filepath.Join(root, "nested")
				if err := os.Mkdir(child, 0o700); err != nil {
					t.Fatal(err)
				}
				t.Chdir(child)
				bin := t.TempDir()
				marker := filepath.Join(bin, "rsync-ran")
				for name, body := range map[string]string{
					"ssh-keyscan": "#!/bin/sh\nprintf 'host.example ssh-ed25519 AQID\\n'\n",
					"ssh-keygen":  "#!/bin/sh\nif [ -f \"$4\" ]; then /bin/cat \"$4\"; fi\n",
					"ssh":         "#!/bin/sh\nexit 0\n",
					"rsync":       "#!/bin/sh\ntouch '" + marker + "'\n",
				} {
					if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700); err != nil {
						t.Fatal(err)
					}
				}
				t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
				compose := filepath.Join(root, "compose.yml")
				if err := os.WriteFile(compose, []byte("services: {}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				pl := payload{compose: compose, envName: "prod"}
				deployFn := func() ServerResult {
					if deployKind == "one" {
						return deployOne(context.Background(), envName, srv, TargetStatus{}, pl)
					}
					return deployApp(context.Background(), envName, srv, TargetStatus{}, pl, false, nil)
				}
				result := deployFn()
				if result.Err == nil || !strings.Contains(result.Err.Error(), "E487") ||
					!strings.Contains(result.Err.Error(), "env target add "+envName+" web --trust-host-key SHA256:") {
					t.Fatalf("pipeline hint: %+v", result)
				}
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatal("rsync ran before enrolment")
				}
				fingerprint := strings.SplitN(result.Err.Error(), "--trust-host-key ", 2)[1]
				fingerprint = strings.Fields(fingerprint)[0]
				if err := TrustHostKey(context.Background(), host, fingerprint); err != nil {
					t.Fatal(err)
				}
				if result = deployFn(); result.Status != "ok" {
					t.Fatalf("deploy after enrolment: %+v", result)
				}
				if _, err := os.Stat(marker); err != nil {
					t.Fatalf("rsync did not run: %v", err)
				}
			})
		}
	}
}

// TestEnvScopeRun: Run deploys and probes only the named env; an unknown,
// empty or differently-cased env is refused and nothing is contacted.
func TestEnvScopeRun(t *testing.T) {
	deployed := stubDeployServer(t)
	t.Setenv("NSELF_SSH_KEY_STAGING", filepath.Join(t.TempDir(), "k"))

	prober := &recordingProber{}
	compose := filepath.Join(t.TempDir(), "compose.yml")
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), threeEnvInventory(), "qa", prober, compose)
	if err != nil {
		t.Fatalf("Run qa: %v", err)
	}
	if len(res.Servers) != 1 || res.Servers[0].Env != "qa" {
		t.Fatalf("result servers = %+v, want exactly the qa server", res.Servers)
	}
	if len(*deployed) != 1 || !strings.HasPrefix((*deployed)[0], "u@qa.example.test") {
		t.Fatalf("deployed hosts = %v, want only the qa host", *deployed)
	}
	for _, h := range prober.hosts {
		if h != "u@qa.example.test" {
			t.Errorf("prober contacted %q, a host outside env qa", h)
		}
	}
	if len(prober.hosts) == 0 {
		t.Error("prober was never called for qa: the cross-env assertion above is vacuous")
	}

	for _, bad := range []string{"missing", "", "QA", "production"} {
		*deployed = nil
		prober := &recordingProber{}
		res, err := Run(context.Background(), threeEnvInventory(), bad, prober, "/tmp/compose.yml")
		if err == nil || res != nil {
			t.Errorf("Run(%q) = (%v, %v), want an error and no result", bad, res, err)
		}
		if len(*deployed) != 0 || len(prober.hosts) != 0 {
			t.Errorf("Run(%q) contacted hosts: deployed=%v probed=%v", bad, *deployed, prober.hosts)
		}
	}

	if _, err := Run(context.Background(), nil, "qa", &recordingProber{}, "/tmp/compose.yml"); err == nil {
		t.Error("Run with a nil inventory must error")
	}
}
