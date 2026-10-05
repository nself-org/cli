package commands

// Tests for P7-DEPL-12: a deploy touches only the environment it names.
// Nothing here opens a network connection: the SSH prober, the remote push
// and the build step are replaced by recording seams, and the per-server SSH
// deploy is only reachable through a prober that reports every host unreachable.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/errs"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// scopeProber records every host any probe method was called with. When
// unreachable is true every SSH probe fails, so the pipeline skips the server
// and never reaches the SSH deploy.
type scopeProber struct {
	mu          sync.Mutex
	hosts       []string
	unreachable bool
}

func (p *scopeProber) rec(s controlplane.Server) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hosts = append(p.hosts, s.Host)
}
func (p *scopeProber) SSHReachable(s controlplane.Server) (bool, int, error) {
	p.rec(s)
	if p.unreachable {
		return false, 0, errors.New("stub: unreachable")
	}
	return true, 1, nil
}
func (p *scopeProber) DockerOK(s controlplane.Server) (bool, error) { p.rec(s); return true, nil }
func (p *scopeProber) KeyState(s controlplane.Server) (bool, string) {
	p.rec(s)
	return true, "/nonexistent/key"
}

// scopeFixture creates a project dir with a three-env inventory (qa, staging,
// prod, one host each), makes it the cwd, isolates deploy env vars, and
// installs the recording prober. It returns the dir and the prober.
func scopeFixture(t *testing.T, unreachable bool) (string, *scopeProber) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	isolateDeployEnv(t)
	mk := func(env string) controlplane.Environment {
		return controlplane.Environment{Name: env, Kind: "remote", Servers: []controlplane.Server{
			{Name: env + "-app", Role: controlplane.RoleApp, Host: "u@" + env + ".example.test", SSHKeyRef: "NSELF_SSH_KEY_X", RemotePath: "/opt/nself", Primary: true},
		}}
	}
	inv := &controlplane.Inventory{SchemaVersion: 1, Project: "t", Environments: map[string]controlplane.Environment{
		"qa": mk("qa"), "staging": mk("staging"), "prod": mk("prod"),
	}}
	if err := controlplane.Write(dir, inv); err != nil {
		t.Fatalf("write inventory: %v", err)
	}
	p := &scopeProber{unreachable: unreachable}
	orig := newDeployProber
	newDeployProber = func(string) controlplane.Prober { return p }
	t.Cleanup(func() { newDeployProber = orig })
	return dir, p
}

// isolateDeployEnv blanks every deploy-related variable for the test and
// restores them afterwards, so an operator's real NSELF_DEPLOY_HOST_* never
// leaks into a fixture and a cascade load never leaks out.
func isolateDeployEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if k, _, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(k, "NSELF_DEPLOY_HOST_") {
			t.Setenv(k, "")
		}
	}
	for _, k := range []string{"NSELF_DEPLOY_ENV", "ENV", "STAGING_DEPLOY_HOST", "PROD_DEPLOY_HOST", "QA_DEPLOY_HOST"} {
		t.Setenv(k, "")
	}
}

// runDeployArgs runs `nself deploy` through runDeploy with the given flags
// (key=value, or key for a bool) and positional args, on a fresh flag set
// shared with deployCmd so defaults match production.
func runDeployArgs(t *testing.T, flags map[string]string, args ...string) error {
	t.Helper()
	c := &cobra.Command{Use: "deploy"}
	c.Flags().AddFlagSet(deployCmd.Flags())
	c.SetContext(context.Background())
	// The flag set is shared with deployCmd: reset it before and after each
	// call so one run's flags never leak into the next.
	reset := func() {
		deployCmd.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
	}
	reset()
	t.Cleanup(reset)
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s: %v", k, err)
		}
	}
	return runDeploy(c, args)
}

func assertE483(t *testing.T, err error) {
	t.Helper()
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E483" {
		t.Fatalf("want an E483 error, got %v", err)
	}
	if got := errs.ExitCodeFor(err); got != 1 {
		t.Errorf("E483 exit code = %d, want 1", got)
	}
}

// TestEnvScopeDryRunProbe: `deploy qa --dry-run` probes only qa's host; an
// unknown env, or a --server that lives in another env, probes nothing.
func TestEnvScopeDryRunProbe(t *testing.T) {
	_, p := scopeFixture(t, false)
	if err := runDeployArgs(t, map[string]string{"dry-run": "true"}, "qa"); err != nil {
		t.Fatalf("deploy qa --dry-run: %v", err)
	}
	if len(p.hosts) == 0 {
		t.Fatal("prober never called: the cross-env assertion would be vacuous")
	}
	for _, h := range p.hosts {
		if h != "u@qa.example.test" {
			t.Errorf("dry-run for qa probed %q, a host outside qa", h)
		}
	}

	p.hosts = nil
	if err := runDeployArgs(t, map[string]string{"dry-run": "true", "env": "qa"}); err != nil {
		t.Fatalf("deploy --env qa --dry-run: %v", err)
	}
	for _, h := range p.hosts {
		if h != "u@qa.example.test" {
			t.Errorf("--env qa probed %q", h)
		}
	}

	p.hosts = nil
	assertE483(t, runDeployArgs(t, map[string]string{"dry-run": "true"}, "nope"))
	if err := runDeployArgs(t, map[string]string{"dry-run": "true", "server": "staging-app"}, "qa"); err == nil {
		t.Error("--server naming a staging server must be refused when deploying qa")
	}
	if len(p.hosts) != 0 {
		t.Errorf("refused deploys still probed hosts: %v", p.hosts)
	}
}

// TestProdClassGate: prod, production and any name prodClassFn marks prod-class
// refuse without --force; --dry-run and --force pass the gate; qa never needs it.
func TestProdClassGate(t *testing.T) {
	dir, p := scopeFixture(t, true) // unreachable: the pipeline skips, nothing deploys
	inv, err := controlplane.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	inv.Environments["production"] = controlplane.Environment{Name: "production", Kind: "remote", Servers: []controlplane.Server{
		{Name: "production-app", Role: controlplane.RoleApp, Host: "u@production.example.test", SSHKeyRef: "NSELF_SSH_KEY_X", RemotePath: "/opt/nself", Primary: true}}}
	inv.Environments["live"] = controlplane.Environment{Name: "live", Kind: "remote", Servers: []controlplane.Server{
		{Name: "live-app", Role: controlplane.RoleApp, Host: "u@live.example.test", SSHKeyRef: "NSELF_SSH_KEY_X", RemotePath: "/opt/nself", Primary: true}}}
	if err := controlplane.Write(dir, inv); err != nil {
		t.Fatal(err)
	}
	orig := prodClassFn
	prodClassFn = func(i *controlplane.Inventory, env string) bool { return env == "live" || orig(i, env) }
	t.Cleanup(func() { prodClassFn = orig })

	for _, env := range []string{"prod", "production", "live"} {
		p.hosts = nil
		err := runDeployArgs(t, nil, env)
		if err == nil || !strings.Contains(err.Error(), "requires --force") {
			t.Errorf("deploy %s without --force: err = %v, want the production gate", env, err)
		}
		if len(p.hosts) != 0 {
			t.Errorf("deploy %s was refused but probed %v", env, p.hosts)
		}
		if err := runDeployArgs(t, map[string]string{"dry-run": "true"}, env); err != nil {
			t.Errorf("deploy %s --dry-run must pass the gate: %v", env, err)
		}
		err = runDeployArgs(t, map[string]string{"force": "true"}, env)
		if err == nil || strings.Contains(err.Error(), "requires --force") {
			t.Errorf("deploy %s --force: err = %v, want to pass the gate (and fail later on the unreachable stub)", env, err)
		}
		if err := runDeployArgs(t, map[string]string{"yes": "true"}, env); err != nil && strings.Contains(err.Error(), "requires --force") {
			t.Errorf("deploy %s --yes must count as --force: %v", env, err)
		}
	}
	for _, env := range []string{"qa", "staging"} {
		err := runDeployArgs(t, nil, env)
		if err != nil && strings.Contains(err.Error(), "requires --force") {
			t.Errorf("deploy %s must not hit the production gate: %v", env, err)
		}
	}
}

// TestDeployLegacyNoSilentExit: with only NSELF_DEPLOY_HOST_QA set (no
// inventory file) `deploy qa` goes to the legacy remote push for qa, or fails;
// it never builds-and-returns-nil. An env with no host is refused before build.
func TestDeployLegacyNoSilentExit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	isolateDeployEnv(t)

	builds := 0
	origBuild := deployBuildStepFn
	deployBuildStepFn = func(_ context.Context, _ string, steps []deployStep) ([]deployStep, error) {
		builds++
		return steps, nil
	}
	t.Cleanup(func() { deployBuildStepFn = origBuild })
	var pushHost, pushTarget string
	pushes := 0
	origPush := remoteDeployPushFn
	remoteDeployPushFn = func(_ context.Context, _ string, host, target string, _ bool) error {
		pushes++
		pushHost, pushTarget = host, target
		return errors.New("stub push stops here")
	}
	t.Cleanup(func() { remoteDeployPushFn = origPush })

	assertE483(t, runDeployArgs(t, nil, "qa"))
	if builds != 0 || pushes != 0 {
		t.Fatalf("unknown env qa built %d times and pushed %d times", builds, pushes)
	}

	t.Setenv("NSELF_DEPLOY_HOST_QA", "deploy@qa.example.test:/opt/nself")
	err := runDeployArgs(t, nil, "qa")
	if err == nil {
		t.Fatal("deploy qa returned nil with a stubbed failing push: a silent exit 0")
	}
	if pushes != 1 || pushTarget != "qa" || pushHost != "deploy@qa.example.test:/opt/nself" {
		t.Errorf("push calls=%d host=%q target=%q, want one push of qa's own host", pushes, pushHost, pushTarget)
	}
	if !strings.Contains(err.Error(), "stub push") {
		t.Errorf("error %v does not carry the push failure", err)
	}
}

// TestEnvCascadePerEnv: qa loads .env.dev, .env.qa, .env.secrets and never
// .env.prod; prod and staging keep theirs; local is unchanged.
func TestEnvCascadePerEnv(t *testing.T) {
	dir := t.TempDir()
	base := func(n string) string { return filepath.Join(dir, n) }
	want := map[string][]string{
		"local":   {".env.dev", ".env.local"},
		"staging": {".env.dev", ".env.staging", ".env.secrets"},
		"prod":    {".env.dev", ".env.prod", ".env.secrets"},
		"qa":      {".env.dev", ".env.qa", ".env.secrets"},
		"qa-eu":   {".env.dev", ".env.qa-eu", ".env.secrets"},
	}
	for target, names := range want {
		got := deployEnvCascadeFiles(dir, target)
		if len(got) != len(names) {
			t.Fatalf("%s cascade = %v, want %v", target, got, names)
		}
		for i, n := range names {
			if got[i] != base(n) {
				t.Errorf("%s cascade[%d] = %s, want %s", target, i, got[i], base(n))
			}
		}
	}

	isolateDeployEnv(t)
	for _, k := range []string{"M_DEV", "M_QA", "M_SEC", "M_PROD"} {
		t.Setenv(k, "")
	}
	for name, body := range map[string]string{
		".env.dev": "M_DEV=dev\n", ".env.qa": "M_QA=qa\n", ".env.secrets": "M_SEC=sec\n", ".env.prod": "M_PROD=prod\n"} {
		if err := os.WriteFile(base(name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loadDeployEnvCascade(dir, "qa")
	if os.Getenv("M_DEV") != "dev" || os.Getenv("M_QA") != "qa" || os.Getenv("M_SEC") != "sec" {
		t.Errorf("qa cascade did not load dev/qa/secrets: %q %q %q", os.Getenv("M_DEV"), os.Getenv("M_QA"), os.Getenv("M_SEC"))
	}
	if v := os.Getenv("M_PROD"); v != "" {
		t.Errorf("qa cascade read .env.prod (M_PROD=%q)", v)
	}
	if os.Getenv("NSELF_DEPLOY_ENV") != "qa" || os.Getenv("ENV") != "qa" {
		t.Errorf("deploy env not exported as qa: %q %q", os.Getenv("NSELF_DEPLOY_ENV"), os.Getenv("ENV"))
	}
	snap, cleanup, err := writeResolvedDeployEnv(dir, "qa")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	b, _ := os.ReadFile(snap)
	if strings.Contains(string(b), "M_PROD") || !strings.Contains(string(b), "M_QA=qa") {
		t.Errorf("resolved qa snapshot wrong:\n%s", b)
	}
	loadDeployEnvCascade(dir, "prod")
	if os.Getenv("M_PROD") != "prod" {
		t.Error("prod cascade must still load .env.prod")
	}
}

// TestResolveTargetGolden: the legacy names resolve exactly as before; any
// inventory env resolves; anything else is E483 listing the known envs.
func TestResolveTargetGolden(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	isolateDeployEnv(t)

	golden := map[string]string{"local": "local", "staging": "staging", "prod": "prod", "production": "prod",
		"PRODUCTION": "prod", "  staging  ": "staging", "Local": "local"}
	for in, want := range golden {
		if got, err := resolveTarget(in); err != nil || got != want {
			t.Errorf("resolveTarget(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "  ", "nope", "all", "*", "../prod", "qa", "prod/..", "a b"} {
		got, err := resolveTarget(bad)
		if got != "" {
			t.Errorf("resolveTarget(%q) returned %q, want no environment", bad, got)
		}
		assertE483(t, err)
		if !strings.Contains(err.Error(), "invalid target") || !strings.Contains(err.Error(), "known:") || !strings.Contains(err.Error(), "staging") {
			t.Errorf("E483 message must start with invalid target and list known envs: %v", err)
		}
	}

	t.Setenv("NSELF_DEPLOY_HOST_QA", "u@qa.example.test:/opt/nself")
	if got, err := resolveTarget("QA"); err != nil || got != "qa" {
		t.Errorf("env-var env: resolveTarget(QA) = %q, %v; want qa", got, err)
	}
	if _, err := resolveTarget("nope"); err == nil || !strings.Contains(err.Error(), "qa") {
		t.Errorf("known envs must include qa: %v", err)
	}

	// An env literally named production wins over the alias.
	inv := &controlplane.Inventory{SchemaVersion: 1, Project: "t", Environments: map[string]controlplane.Environment{
		"production": {Name: "production", Kind: "remote"}, "prod": {Name: "prod", Kind: "remote"}}}
	if err := controlplane.Write(dir, inv); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveTarget("production"); err != nil || got != "production" {
		t.Errorf("resolveTarget(production) with a production env = %q, %v; want production", got, err)
	}
}

// TestDbRemoteResolve: db --env resolves through the same inventory and
// alias rules as deploy; an unknown env is E483, never local.
func TestDbRemoteResolve(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	isolateDeployEnv(t)
	inv := &controlplane.Inventory{SchemaVersion: 1, Project: "t", Environments: map[string]controlplane.Environment{
		"qa": {Name: "qa", Kind: "remote", Servers: []controlplane.Server{
			{Name: "qa-app", Role: controlplane.RoleApp, Host: "u@qa.example.test", RemotePath: "/opt/qa", Primary: true}}},
		"prod": {Name: "prod", Kind: "remote", Servers: []controlplane.Server{
			{Name: "prod-app", Role: controlplane.RoleApp, Host: "u@prod.example.test", RemotePath: "/opt/prod", Primary: true}}},
	}}
	if err := controlplane.Write(dir, inv); err != nil {
		t.Fatal(err)
	}
	for env, want := range map[string]string{"qa": "u@qa.example.test", "QA": "u@qa.example.test",
		"prod": "u@prod.example.test", "production": "u@prod.example.test"} {
		cmd := newDBRemoteTestCmd()
		if err := cmd.Flags().Set("env", env); err != nil {
			t.Fatal(err)
		}
		tg, err := resolveDBRemoteTarget(cmd)
		if err != nil || tg.Local || !strings.Contains(tg.SSHTarget, want) {
			t.Errorf("--env %s -> %+v, %v; want host %s", env, tg, err, want)
		}
	}
	cmd := newDBRemoteTestCmd()
	_ = cmd.Flags().Set("env", "nope")
	tg, err := resolveDBRemoteTarget(cmd)
	assertE483(t, err)
	if tg.Local {
		t.Error("an unknown env must never resolve to local")
	}
}
