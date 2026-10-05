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

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/errs"

	"github.com/joho/godotenv"
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
	for _, k := range []string{"NSELF_DEPLOY_ENV", "NSELF_DEPLOY_REMOTE", "ENV", "STAGING_DEPLOY_HOST", "PROD_DEPLOY_HOST", "QA_DEPLOY_HOST"} {
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

// (remote envs below carry no .env.local: a remote deploy never ships it)
// TestEnvCascadePerEnv: the deploy cascade is config.EnvCascadeOrder; qa never
// loads .env.prod; a prod-class env always gets the prod cascade.
func TestEnvCascadePerEnv(t *testing.T) {
	dir := t.TempDir()
	base := func(n string) string { return filepath.Join(dir, n) }
	want := map[string][]string{
		"local":      {".env", ".env.dev", ".env.secrets", ".env.local"},
		"staging":    {".env", ".env.staging", ".env.secrets"},
		"prod":       {".env", ".env.prod", ".env.secrets"},
		"production": {".env", ".env.prod", ".env.secrets"},
		"PROD":       {".env", ".env.prod", ".env.secrets"},
		"qa":         {".env", ".env.qa", ".env.secrets"},
		"QA":         {".env", ".env.qa", ".env.secrets"},
		"qa-eu":      {".env", ".env.qa-eu", ".env.secrets"},
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
	if os.Getenv("M_QA") != "qa" || os.Getenv("M_SEC") != "sec" {
		t.Errorf("qa cascade did not load qa/secrets: %q %q", os.Getenv("M_QA"), os.Getenv("M_SEC"))
	}
	if v := os.Getenv("M_DEV"); v != "" {
		t.Errorf("qa cascade inherited .env.dev (M_DEV=%q)", v)
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

// ── Opus adversarial review tests (P7-DEPL-12 CRITICAL review) ──

func opusWriteInv(t *testing.T, dir string, keys ...string) {
	t.Helper()
	envs := map[string]controlplane.Environment{}
	for _, k := range keys {
		envs[k] = controlplane.Environment{Name: k, Kind: "remote", Servers: []controlplane.Server{
			{Name: "s" + strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
					return r
				}
				return 'x'
			}, k), Role: controlplane.RoleApp, Host: "u@" + strings.ToLower(strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
					return r
				}
				return 'x'
			}, k)) + ".example.test", SSHKeyRef: "NSELF_SSH_KEY_X", RemotePath: "/opt/nself", Primary: true},
		}}
	}
	if err := controlplane.Write(dir, &controlplane.Inventory{SchemaVersion: 1, Project: "t", Environments: envs}); err != nil {
		t.Fatalf("write inventory: %v", err)
	}
}

func opusStubBuildPush(t *testing.T) (builds, pushes *int) {
	t.Helper()
	b, p := 0, 0
	ob, op := deployBuildStepFn, remoteDeployPushFn
	deployBuildStepFn = func(_ context.Context, _ string, s []deployStep) ([]deployStep, error) { b++; return s, nil }
	remoteDeployPushFn = func(context.Context, string, string, string, bool) error { p++; return errors.New("stub push") }
	t.Cleanup(func() { deployBuildStepFn, remoteDeployPushFn = ob, op })
	return &b, &p
}

// A1: an inventory env keyed "PROD" is gated; but its cascade is .env.PROD, not .env.prod.
func TestOpusUpperProdKey(t *testing.T) {
	dir, p := scopeFixture(t, true)
	opusWriteInv(t, dir, "PROD", "qa")
	got, err := resolveTarget("prod")
	if err != nil || got != "PROD" {
		t.Fatalf("resolveTarget(prod) = %q, %v", got, err)
	}
	err = runDeployArgs(t, nil, "prod")
	if err == nil || !strings.Contains(err.Error(), "requires --force") || len(p.hosts) != 0 {
		t.Errorf("PROD key: gate err=%v probed=%v", err, p.hosts)
	}
	t.Run("cascade", func(t *testing.T) {
		files := deployEnvCascadeFiles(dir, got)
		joined := strings.Join(files, ",")
		if !strings.Contains(joined, string(filepath.Separator)+".env.prod,") {
			t.Errorf("prod-class env %q cascade = %v: never loads .env.prod (exact-case filesystems)", got, files)
		}
	})
}

// A2: an inventory env literally named production is prod-class but its cascade drops .env.prod.
func TestOpusProductionKeyCascade(t *testing.T) {
	dir, _ := scopeFixture(t, true)
	opusWriteInv(t, dir, "production", "qa")
	got, err := resolveTarget("production")
	if err != nil || got != "production" {
		t.Fatalf("resolveTarget(production) = %q, %v", got, err)
	}
	if !controlplane.IsProdClass(nil, got) {
		t.Fatal("production must be prod-class")
	}
	files := deployEnvCascadeFiles(dir, got)
	if !strings.Contains(strings.Join(files, ","), ".env.prod,") {
		t.Errorf("prod-class env production cascade = %v: .env.prod never loaded", files)
	}
}

// A3: synthesized env whose host var spelling differs is refused before any build.
func TestOpusHostVarMismatchRefusedBeforeBuild(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	isolateDeployEnv(t)
	builds, pushes := opusStubBuildPush(t)
	// A hyphen cannot be folded away by any OS: the variable NSELF_DEPLOY_HOST_QA-EU
	// synthesizes env "qa-eu", while the host lookup reads NSELF_DEPLOY_HOST_QA_EU.
	// (Setting the lowercase spelling of the same name would be the same variable
	// on Windows, whose env names are case-insensitive.)
	t.Setenv("NSELF_DEPLOY_HOST_QA-EU", "u@qa.example.test:/opt/nself")
	if got, err := resolveTarget("qa-eu"); err != nil || got != "qa-eu" {
		t.Fatalf("resolveTarget(qa-eu) = %q, %v (synthesized env expected)", got, err)
	}
	for _, flags := range []map[string]string{nil, {"dry-run": "true"}} {
		assertE483(t, runDeployArgs(t, flags, "qa-eu"))
	}
	if *builds != 0 || *pushes != 0 {
		t.Errorf("builds=%d pushes=%d, want none", *builds, *pushes)
	}
}

// A4: hostile names never resolve, and hostile inventory keys are unreachable.
func TestOpusHostileNames(t *testing.T) {
	dir, p := scopeFixture(t, false)
	opusWriteInv(t, dir, "qa", "staging", "prod", "../prod", "qa;id", "prod ", "x=y")
	for _, bad := range []string{"../prod", "qa;id", "qa$(id)", "prod ", " prod/", "x=y", ".env", "-prod", "prod\x00", "qa`id`", "QA/../prod", "qa,prod", "*"} {
		got, err := resolveTarget(bad)
		if strings.TrimSpace(bad) == "prod" {
			if got != "prod" {
				t.Errorf("%q -> %q, want prod (trim)", bad, got)
			}
			continue
		}
		if err == nil {
			t.Errorf("resolveTarget(%q) = %q, want E483", bad, got)
		}
	}
	p.hosts = nil
	_ = runDeployArgs(t, map[string]string{"dry-run": "true"}, "qa;id")
	if len(p.hosts) != 0 {
		t.Errorf("hostile name probed %v", p.hosts)
	}
}

// A5: legacy custom env dry-run neither builds nor pushes.
func TestOpusLegacyDryRunCustomEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	isolateDeployEnv(t)
	builds, pushes := opusStubBuildPush(t)
	t.Setenv("NSELF_DEPLOY_HOST_QA", "u@qa.example.test:/opt/nself")
	if err := runDeployArgs(t, map[string]string{"dry-run": "true"}, "qa"); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if *builds != 0 || *pushes != 0 {
		t.Errorf("dry-run builds=%d pushes=%d", *builds, *pushes)
	}
}

// A6: the DEPL-12 gate runs before blue/green, whose own gate only knows "prod".
func TestOpusGateBeforeBlueGreen(t *testing.T) {
	dir, _ := scopeFixture(t, true)
	opusWriteInv(t, dir, "production", "qa")
	t.Setenv("NSELF_FEATURE_BLUE_GREEN_DEPLOY", "true")
	err := runDeployArgs(t, map[string]string{"canary": "10"}, "production")
	if err == nil || !strings.HasPrefix(err.Error(), "production deploy requires --force") {
		t.Errorf("blue/green production without --force: %v, want the DEPL-12 gate first", err)
	}
}

// A7 (out of scope, loader): what the build subprocess sees for a custom env.
func TestOpusLoaderCustomEnvLayer(t *testing.T) {
	dir := t.TempDir()
	isolateDeployEnv(t)
	t.Setenv("BASE_DOMAIN", "")
	t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
	for n, b := range map[string]string{".env": "BASE_DOMAIN=base.example.test\n", ".env.qa": "BASE_DOMAIN=qa.example.test\n", ".env.prod": "BASE_DOMAIN=prod.example.test\n"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loadDeployEnvCascade(dir, "qa") // parent: sets ENV=qa and BASE_DOMAIN=qa
	if os.Getenv("BASE_DOMAIN") != "qa.example.test" {
		t.Fatalf("parent cascade BASE_DOMAIN=%q", os.Getenv("BASE_DOMAIN"))
	}
	cfg, err := config.Load(dir) // what `nself build` (runCLISelf child, inherited env) does
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseDomain == "prod.example.test" {
		t.Fatalf("qa build saw prod values")
	}
	if cfg.BaseDomain != "qa.example.test" {
		t.Errorf("qa build BASE_DOMAIN=%q, want qa.example.test (.env overrides .env.qa in the build)", cfg.BaseDomain)
	}
}

// TestDeployCascadeMatchesBuild: Codex scenario. .env sets API_URL=prod-base and
// .env.qa sets API_URL=qa. The parent's cascade, the child build's config.Load
// and the snapshot shipped to the host must all say qa.
func TestDeployCascadeMatchesBuild(t *testing.T) {
	dir := t.TempDir()
	isolateDeployEnv(t)
	t.Setenv("API_URL", "")
	t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
	for n, b := range map[string]string{".env": "API_URL=prod-base\nBASE_ONLY=1\n", ".env.qa": "API_URL=qa\n", ".env.prod": "API_URL=prod\n"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loadDeployEnvCascade(dir, "qa")
	if _, err := config.Load(dir); err != nil { // what the child `nself build` runs
		t.Fatal(err)
	}
	if got := os.Getenv("API_URL"); got != "qa" {
		t.Errorf("build saw API_URL=%q, want qa", got)
	}
	snap, cleanup, err := writeResolvedDeployEnv(dir, "qa")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	b, _ := os.ReadFile(snap)
	if !strings.Contains(string(b), "API_URL=qa\n") || strings.Contains(string(b), "prod") || !strings.Contains(string(b), "BASE_ONLY=1") {
		t.Errorf("shipped snapshot disagrees with the build:\n%s", b)
	}
}

// TestDeployNoHostRefusedBeforeBuild: staging and prod with no host (no
// inventory, no variable) are refused with E483 before any build, dry-run
// included; they never fall back to deploying on this machine.
func TestDeployNoHostRefusedBeforeBuild(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	isolateDeployEnv(t)
	builds, pushes := opusStubBuildPush(t)
	for _, env := range []string{"staging", "prod", "production"} {
		for _, flags := range []map[string]string{{"force": "true"}, {"dry-run": "true"}} {
			assertE483(t, runDeployArgs(t, flags, env))
		}
	}
	if *builds != 0 || *pushes != 0 {
		t.Errorf("builds=%d pushes=%d, want none", *builds, *pushes)
	}
}

// TestBlueGreenRemoteRefused: blue/green drives the local stack, so a remote
// env is refused before any side effect, whatever its name.
func TestBlueGreenRemoteRefused(t *testing.T) {
	scopeFixture(t, true)
	builds, pushes := opusStubBuildPush(t)
	t.Setenv("NSELF_FEATURE_BLUE_GREEN_DEPLOY", "true")
	for _, env := range []string{"qa", "staging"} {
		for _, flags := range []map[string]string{{"canary": "10"}, {"skip-canary": "true"}, {"canary": "10", "dry-run": "true"}} {
			err := runDeployArgs(t, flags, env)
			if err == nil || !strings.Contains(err.Error(), "local target only") {
				t.Errorf("deploy %s %v: err = %v, want the blue/green local-only refusal", env, flags, err)
			}
		}
	}
	if *builds != 0 || *pushes != 0 {
		t.Errorf("builds=%d pushes=%d, want none", *builds, *pushes)
	}
}

// TestPipelineFailedServerExitsNonZero: a server whose deploy fails makes the
// pipeline deploy fail, naming it. The ssh and rsync on PATH are fakes that
// exit 1 (and PATH holds nothing else), so no host is contacted.
func TestPipelineFailedServerExitsNonZero(t *testing.T) {
	dir, _ := scopeFixture(t, false)
	bin := t.TempDir()
	for _, n := range []string{"ssh", "rsync"} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	key := filepath.Join(bin, "key")
	if err := os.WriteFile(key, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("NSELF_SSH_KEY_X", key)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := runDeployArgs(t, nil, "qa")
	if err == nil || !strings.Contains(err.Error(), "qa/qa-app") || !strings.Contains(err.Error(), "failed") {
		t.Errorf("failed server must fail the deploy and be listed, got %v", err)
	}
}

// TestInventoryCaseCollisionRefused: two inventory keys that differ only by
// case make the name ambiguous; it is refused, never guessed.
func TestInventoryCaseCollisionRefused(t *testing.T) {
	dir, p := scopeFixture(t, false)
	opusWriteInv(t, dir, "qa", "QA", "staging")
	for _, in := range []string{"qa", "QA"} {
		if got, err := resolveTarget(in); err == nil {
			t.Errorf("resolveTarget(%q) = %q with colliding keys, want E483", in, got)
		}
	}
	assertE483(t, runDeployArgs(t, map[string]string{"dry-run": "true"}, "qa"))
	if len(p.hosts) != 0 {
		t.Errorf("ambiguous env probed %v", p.hosts)
	}
}

// TestProdClassCascadeIsProd: whatever makes an env prod-class (here the
// prodClassFn seam standing in for the tier lookup of P7-DEPL-13), it gets the
// prod cascade, never a layer named after itself.
func TestProdClassCascadeIsProd(t *testing.T) {
	dir := t.TempDir()
	orig := prodClassFn
	prodClassFn = func(_ *controlplane.Inventory, env string) bool { return env == "live" || orig(nil, env) }
	t.Cleanup(func() { prodClassFn = orig })
	got := strings.Join(deployEnvCascadeFiles(dir, "live"), ",")
	if !strings.Contains(got, ".env.prod,") || strings.Contains(got, ".env.live") {
		t.Errorf("prod-class live cascade = %s, want the prod cascade", got)
	}
}

// TestR2ProductionSnapshotRemoteName: a legacy `production` deploy pushes under
// the cascade env, so the host gets .env.prod (the file its nself reads), not
// .env.production.
func TestR2ProductionSnapshotRemoteName(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	isolateDeployEnv(t)
	t.Setenv("NSELF_DEPLOY_HOST_PRODUCTION", "u@p.example.test:/opt/nself")
	var gotTarget string
	ob, op := deployBuildStepFn, remoteDeployPushFn
	deployBuildStepFn = func(_ context.Context, _ string, s []deployStep) ([]deployStep, error) { return s, nil }
	remoteDeployPushFn = func(_ context.Context, _ string, _ string, target string, _ bool) error {
		gotTarget = target
		return errors.New("stub")
	}
	t.Cleanup(func() { deployBuildStepFn, remoteDeployPushFn = ob, op })
	_ = runDeployArgs(t, map[string]string{"force": "true"}, "production")
	if gotTarget != "prod" {
		t.Errorf("push target = %q, want prod (remote file .env.prod)", gotTarget)
	}
}

// TestSnapshotMatchesConfigLoad: the snapshot is read with the reader config.Load
// uses, and written so that reader returns the same values. Covers export
// prefixes, inline comments, single and double quotes, escapes, $ and spaces.
func TestSnapshotMatchesConfigLoad(t *testing.T) {
	dir := t.TempDir()
	isolateDeployEnv(t)
	t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
	keys := []string{"K_EXPORT", "K_INLINE", "K_DQ", "K_SQ", "K_ESC", "K_DOLLAR", "K_SPACE", "K_HASH", "K_OVERRIDE", "K_EMPTY", "K_NL"}
	for _, k := range keys {
		t.Setenv(k, "")
	}
	files := map[string]string{
		".env": "export K_EXPORT=exported\nK_INLINE=value # a comment\nK_OVERRIDE=base\n",
		".env.qa": "K_DQ=\"say \\\"hi\\\" now\"\nK_SQ='it is $HOME'\nK_ESC=\"tab\\there\"\nK_DOLLAR='a$b'\n" +
			"K_SPACE='two words'\nK_HASH='not # a comment'\nK_OVERRIDE=qa\nK_EMPTY=\nK_NL=\"line1\\nline2\"\n",
	}
	for n, b := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loadDeployEnvCascade(dir, "qa")
	if _, err := config.Load(dir); err != nil {
		t.Fatal(err)
	}
	built := map[string]string{}
	for _, k := range keys {
		built[k] = os.Getenv(k)
	}
	snap, cleanup, err := writeResolvedDeployEnv(dir, "qa")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	shipped, err := godotenv.Read(snap) // what the remote nself's loader does
	if err != nil {
		t.Fatalf("snapshot unreadable: %v", err)
	}
	for _, k := range keys {
		if shipped[k] != built[k] {
			t.Errorf("%s: shipped %q, built %q", k, shipped[k], built[k])
		}
	}
	if built["K_EXPORT"] != "exported" || built["K_INLINE"] != "value" || built["K_OVERRIDE"] != "qa" || built["K_DQ"] != `say "hi" now` {
		t.Errorf("fixture did not exercise the syntax: %v", built)
	}
}

// TestDeployEnvironmentsRefusesCaseCollision: `deploy environments` loads the
// inventory, so an inventory with qa and QA is refused there too.
func TestDeployEnvironmentsRefusesCaseCollision(t *testing.T) {
	dir, p := scopeFixture(t, false)
	opusWriteInv(t, dir, "qa", "QA")
	c := &cobra.Command{Use: "environments"}
	c.SetContext(context.Background())
	assertE483(t, runDeployEnvironments(c, nil))
	if len(p.hosts) != 0 {
		t.Errorf("refused inventory still probed %v", p.hosts)
	}
}

// TestEncodeEnvValueRoundTrip: every ordinary value is written so godotenv reads
// it back unchanged.
func TestEncodeEnvValueRoundTrip(t *testing.T) {
	for _, v := range []string{"", "plain", "a b", "it's", "a$b", "${X}", "x\"y", "l1\nl2", "cr\rx", "a\\b", "é ü", "  lead", "tab\there", "a # b", "k=v"} {
		enc, ok := encodeEnvValue(v)
		if !ok {
			t.Errorf("%q: no faithful encoding", v)
			continue
		}
		if m, err := godotenv.Unmarshal("K=" + enc + "\n"); err != nil || m["K"] != v {
			t.Errorf("%q -> %q -> %q, %v", v, enc, m["K"], err)
		}
	}
}

// TestInventoryEnvNameAmbiguous: the lookup itself refuses colliding keys (a
// second guard behind controlplane.Load), and matches case-insensitively
// otherwise.
func TestInventoryEnvNameAmbiguous(t *testing.T) {
	mk := func(keys ...string) *controlplane.Inventory {
		inv := &controlplane.Inventory{Environments: map[string]controlplane.Environment{}}
		for _, k := range keys {
			inv.Environments[k] = controlplane.Environment{Name: k}
		}
		return inv
	}
	if got, ok := inventoryEnvName(mk("qa", "QA"), "qa"); ok {
		t.Errorf("colliding keys resolved to %q", got)
	}
	if got, ok := inventoryEnvName(mk("QA", "prod"), "qa"); !ok || got != "QA" {
		t.Errorf("inventoryEnvName(QA) = %q, %v", got, ok)
	}
	if _, ok := inventoryEnvName(nil, "qa"); ok {
		t.Error("nil inventory must not resolve")
	}
}

// TestRemoteDeployNeverReadsEnvLocal: .env.local is a personal override. For a
// remote env it is absent from the child build's config and from the pushed
// snapshot; for local it is still read.
func TestRemoteDeployNeverReadsEnvLocal(t *testing.T) {
	for _, tc := range []struct {
		target    string
		wantLocal bool
	}{{"qa", false}, {"staging", false}, {"prod", false}, {"production", false}, {"local", true}} {
		dir := t.TempDir()
		isolateDeployEnv(t)
		t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
		t.Setenv("LAPTOP_ONLY", "")
		t.Setenv("API_URL", "")
		for n, b := range map[string]string{
			".env":         "API_URL=base\n",
			".env.local":   "LAPTOP_ONLY=secret\nAPI_URL=laptop\n",
			".env.qa":      "API_URL=qa\n",
			".env.staging": "API_URL=staging\n",
			".env.prod":    "API_URL=prod\n",
			".env.dev":     "API_URL=dev\n",
		} {
			if err := os.WriteFile(filepath.Join(dir, n), []byte(b), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		loadDeployEnvCascade(dir, tc.target)
		if tc.target == "qa" || tc.target == "local" { // the child build (staging/prod Load demands real secrets)
			if _, err := config.Load(dir); err != nil {
				t.Fatal(err)
			}
		}
		builtLocal := os.Getenv("LAPTOP_ONLY") == "secret"
		if builtLocal != tc.wantLocal {
			t.Errorf("%s: build read .env.local = %v, want %v", tc.target, builtLocal, tc.wantLocal)
		}
		if !tc.wantLocal {
			cascadeEnv := deployCascadeEnv(dir, tc.target)
			snap, cleanup, err := writeResolvedDeployEnv(dir, cascadeEnv)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(snap)
			cleanup()
			if strings.Contains(string(b), "LAPTOP_ONLY") || strings.Contains(string(b), "laptop") {
				t.Errorf("%s: snapshot carries .env.local values:\n%s", tc.target, b)
			}
		}
	}
}

// TestSnapshotLiteralDollarStaysLiteral: Codex's case. A=prod_secret and
// B='$A' (a literal dollar sign): the snapshot must read back with B == "$A",
// never the value of A. Plus a table of awkward values read back as a whole.
func TestSnapshotLiteralDollarStaysLiteral(t *testing.T) {
	dir := t.TempDir()
	isolateDeployEnv(t)
	vals := map[string]string{
		"A": "prod_secret", "B": "$A", "C": "${A}", "D": "pre$A", "E": "it's $A", "F": `a\b$A`,
		"G": "line1\n$A", "H": "x \"$A\" y", "I": "$", "J": "$$A", "K": "end\\", "L": "# not a comment",
		"M": "  padded  ", "N": "é$A", "O": "k=v$A",
	}
	var body strings.Builder
	for k, v := range vals {
		body.WriteString(k + "=" + func() string { e, _ := encodeEnvValue(v); return e }() + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.qa"), []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, cleanup, err := writeResolvedDeployEnv(dir, "qa")
	if err != nil {
		t.Fatalf("snapshot refused: %v", err)
	}
	defer cleanup()
	back, err := godotenv.Read(snap)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range vals {
		if back[k] != v {
			t.Errorf("%s: shipped %q, want %q", k, back[k], v)
		}
	}
	if enc, _ := encodeEnvValue("$A"); enc == "$A" {
		t.Errorf("a value with $ must never be written bare, got %q", enc)
	}
}
