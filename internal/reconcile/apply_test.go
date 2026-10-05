package reconcile

// apply_test.go — tests for Apply and Confirm (P7-LIVE-03).
//
// Purpose: prove an apply writes only after the plan id and the confirmation
// pass, refuses a stale plan (E450) and a non-confirmed prod-class change
// (E403) with the tree untouched, and that the project lock is a flock a
// SIGKILL cannot leave stale.
// Constraints: the lock test re-executes the test binary as the lock holder;
// no real project or host is involved.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	nbuild "github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/oplock"
)

// mkPlan finalises a plan for the confirm tests.
func mkPlan(t *testing.T, env string, arts []Artifact, effs []Effect) Plan {
	t.Helper()
	p := Plan{Command: CmdBuild, Env: env, Artifacts: arts, Effects: effs, Containers: Containers{Known: true}}
	if err := p.Finalize(); err != nil {
		t.Fatal(err)
	}
	return p
}

func wantE403(t *testing.T, err error) {
	t.Helper()
	if d := errs.Describe(err); d == nil || d.Code != "E403" || errs.ExitCodeFor(err) != 4 {
		t.Fatalf("want E403 exit 4, got %v", err)
	}
}

// TestConfirmRules walks EPIC D4 in both compat modes.
func TestConfirmRules(t *testing.T) {
	change := []Artifact{{Kind: KindCompose, Path: "docker-compose.yml", Action: ActionChange, Generated: true, DiffLines: 2}}
	hand := []Artifact{{Kind: KindNginx, Path: "nginx/sites/a.conf", Action: ActionChange, Generated: true, HandEdited: true, DiffLines: 2}}
	yes := func(string) bool { return true }
	no := func(string) bool { return false }
	compattest.Both(t, func(t *testing.T) {
		v15 := compattestV15()
		var notice strings.Builder
		// refused: v1.5 fails with E403. notices: v1.4 proceeds with the notice
		// (it never prompts, so a prompt answer does not matter there).
		check := func(name string, p Plan, opt ApplyOptions, refused bool) {
			t.Helper()
			notice.Reset()
			err := Confirm(p, opt, v15, &notice)
			notices := refused || (opt.Interactive != nil && !p.Empty && (p.RequiresConfirmation || len(HandEditedPaths(p)) > 0))
			switch {
			case refused && v15:
				wantE403(t, err)
			case notices && !v15:
				if err != nil || !strings.Contains(notice.String(), "v1.5 will require") {
					t.Fatalf("%s: v1.4 must proceed with a notice, got err=%v notice=%q", name, err, notice.String())
				}
			default:
				if err != nil || notice.Len() != 0 {
					t.Fatalf("%s: want silent proceed, got err=%v notice=%q", name, err, notice.String())
				}
			}
		}
		check("empty prod plan", mkPlan(t, "prod", nil, nil), ApplyOptions{}, false)
		check("dev change", mkPlan(t, "dev", change, nil), ApplyOptions{}, false)
		check("prod change, no yes, not interactive", mkPlan(t, "prod", change, nil), ApplyOptions{}, true)
		check("staging is prod-class", mkPlan(t, "staging", change, nil), ApplyOptions{}, true)
		check("prod change with --yes", mkPlan(t, "prod", change, nil), ApplyOptions{Yes: true}, false)
		check("prod change, prompt yes", mkPlan(t, "prod", change, nil), ApplyOptions{Interactive: yes}, false)
		check("prod change, prompt no", mkPlan(t, "prod", change, nil), ApplyOptions{Interactive: no}, true)
		check("hand-edited in dev, no force", mkPlan(t, "dev", hand, nil), ApplyOptions{}, true)
		check("hand-edited in dev, force", mkPlan(t, "dev", hand, nil), ApplyOptions{Force: true}, false)
		check("hand-edited in prod, yes but no force", mkPlan(t, "prod", hand, nil), ApplyOptions{Yes: true}, true)
		check("hand-edited in prod, yes and force", mkPlan(t, "prod", hand, nil), ApplyOptions{Yes: true, Force: true}, false)
		check("hand-edited in dev, prompt yes", mkPlan(t, "dev", hand, nil), ApplyOptions{Interactive: yes}, false)
	})
}

// compattestV15 reports the mode compattest.Both put the test in.
func compattestV15() bool {
	v := os.Getenv("NSELF_V15")
	return v == "1" || strings.EqualFold(v, "true")
}

// TestConfirmPromptNamesDestructive: a destructive plan's prompt lists the
// reasons, so the operator is asked about the removal, not about "a change".
func TestConfirmPromptNamesDestructive(t *testing.T) {
	compattest.Set(t, true)
	p := mkPlan(t, "prod", nil, []Effect{{Kind: EffectOrphanRemove, Target: "fx_ghost"}})
	var asked string
	err := Confirm(p, ApplyOptions{Interactive: func(q string) bool { asked = q; return true }}, true, nil)
	if err != nil || !strings.Contains(asked, "DESTRUCTIVE") || !strings.Contains(asked, "orphan removed: fx_ghost") {
		t.Fatalf("err=%v prompt=%q", err, asked)
	}
}

// TestApplyRefusesBeforeWriting: a prod-class apply with nothing confirming it
// fails with E403 in v1.5 mode and leaves the whole root untouched, and the
// BeforeWrite hook (the plugin removal) never runs.
func TestApplyRefusesBeforeWriting(t *testing.T) {
	compattest.Set(t, true)
	f := loadFixture(t, "prod-ssl")
	before := treeHash(t, f.root)
	hooked := false
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project}, ApplyOptions{BeforeWrite: func() error { hooked = true; return nil }})
	wantE403(t, err)
	if hooked {
		t.Fatal("BeforeWrite ran for a refused apply")
	}
	if after := treeHash(t, f.root); after != before {
		t.Fatalf("a refused apply changed the tree:\n%s", lineDiff(before, after))
	}
}

// TestApplyPlanIDMismatch: a plan id from another state is refused with E450 and
// writes nothing; the id of the current plan applies.
func TestApplyPlanIDMismatch(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}
	shown := planOf(t, f, nil)
	// A new key adds a line to the generated env files, so the plan changes.
	writeFile(t, f.project+"/.env", readFile(t, f.project+"/.env")+"MONITORING_ENABLED=true\n", 0o600)
	before := treeHash(t, f.root)
	_, err := Apply(context.Background(), req, ApplyOptions{PlanID: shown.PlanID})
	if d := errs.Describe(err); d == nil || d.Code != "E450" || errs.ExitCodeFor(err) != 1 {
		t.Fatalf("want E450 exit 1, got %v", err)
	}
	if after := treeHash(t, f.root); after != before {
		t.Fatalf("E450 apply wrote:\n%s", lineDiff(before, after))
	}
	if _, err := Apply(context.Background(), req, ApplyOptions{PlanID: planOf(t, f, nil).PlanID}); err != nil {
		t.Fatalf("the current plan id must apply: %v", err)
	}
}

// TestApplyWritesWhatWasPlanned: the secrets an apply generates come from the
// seed its plan used, so two applies of one fresh project with one seed write
// identical bytes, and a different seed writes different secrets.
func TestApplyWritesWhatWasPlanned(t *testing.T) {
	files := func(seed string) map[string]string {
		f := loadFixture(t, "dev-minimal")
		root := f.root
		env := strings.SplitN(readFile(t, f.project+"/.env"), "POSTGRES_PASSWORD=", 2)[0]
		writeFile(t, f.project+"/.env", env, 0o600)
		if _, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte(seed)}, ApplyOptions{Yes: true}); err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for k := range fileHashes(t, f.project) {
			if !strings.HasPrefix(k, ".nself/backups/") && k != ".nself/op.lock" {
				out[k] = strings.ReplaceAll(readFile(t, f.project+"/"+k), root, "<ROOT>")
			}
		}
		return out
	}
	a, b, c := files("seed-one"), files("seed-one"), files("seed-two")
	if len(a) == 0 || strings.Join(sortedKeys(toSet(a)), ",") != strings.Join(sortedKeys(toSet(b)), ",") {
		t.Fatal("applies produced different file sets")
	}
	for k, v := range a {
		if b[k] != v {
			t.Errorf("%s differs between two applies with one seed", k)
		}
	}
	same := true
	for k, v := range a {
		same = same && c[k] == v
	}
	if same {
		t.Error("a different seed produced identical files: the seed is not reaching the generated secrets")
	}
}

func toSet(m map[string]string) map[string]bool {
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	return out
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSeededRand(t *testing.T) {
	read := func(seed string, n int) []byte {
		b := make([]byte, n)
		if _, err := newSeededRand([]byte(seed)).Read(b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	if string(read("a", 100)) != string(read("a", 100)) || string(read("a", 100)) == string(read("b", 100)) {
		t.Fatal("same seed must give the same stream and different seeds different ones")
	}
	r := newSeededRand([]byte("a"))
	x, y := make([]byte, 7), make([]byte, 93)
	_, _ = r.Read(x)
	_, _ = r.Read(y)
	if string(append(x, y...)) != string(read("a", 100)) {
		t.Fatal("the stream depends on how it is read")
	}
}

// TestBuildLockHelper is the lock holder: when NSELF_LOCK_HELPER names a
// project it takes the build lock, prints "locked" and waits to be killed.
func TestBuildLockHelper(t *testing.T) {
	dir := os.Getenv("NSELF_LOCK_HELPER")
	if dir == "" {
		t.Skip("helper process only")
	}
	if _, err := nbuild.AcquireBuildLock(context.Background(), dir); err != nil {
		os.Stdout.WriteString("error: " + err.Error() + "\n")
		os.Exit(3)
	}
	os.Stdout.WriteString("locked\n")
	select {}
}

// TestBuildLockAfterKill: a build refuses to run while another process holds
// the project lock (naming the holder), and after that holder is SIGKILLed two
// builds in a row succeed: a flock leaves no stale lock behind.
func TestBuildLockAfterKill(t *testing.T) {
	compattest.Set(t, true) // v1.5 fails at once; v1.4 would wait 30 s
	f := loadFixture(t, "dev-minimal")
	// The holder needs the project dir to exist with .nself; it is created by the lock.
	cmd := exec.Command(os.Args[0], "-test.run=^TestBuildLockHelper$")
	cmd.Env = append(os.Environ(), "NSELF_LOCK_HELPER="+f.project)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("lock helper said %q (%v)", line, err)
	}
	os.Unsetenv(oplock.EnvToken)
	before := treeHash(t, f.root)
	_, err = Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project}, ApplyOptions{Yes: true})
	if !errors.Is(err, oplock.ErrHeld) {
		t.Fatalf("a build under a held lock must fail with ErrHeld, got %v", err)
	}
	var held *oplock.HeldError
	if !errors.As(err, &held) || !held.Known || held.Holder.PID != cmd.Process.Pid {
		t.Fatalf("the error does not name the holder (pid %d): %v", cmd.Process.Pid, err)
	}
	if after := treeHash(t, f.root); after != before {
		t.Fatalf("a refused build wrote:\n%s", lineDiff(before, after))
	}
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	for i := 1; i <= 2; i++ {
		if _, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project}, ApplyOptions{Yes: true}); err != nil {
			t.Fatalf("build %d after the holder was killed: %v", i, err)
		}
	}
}

// TestApplyShortPlanID: a malformed id is E450 too (and the message abbreviates
// only ids longer than the header prefix).
func TestApplyShortPlanID(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project}, ApplyOptions{PlanID: "abc"})
	if d := errs.Describe(err); d == nil || d.Code != "E450" || !strings.Contains(err.Error()+d.Cause, "abc") {
		t.Fatalf("want E450 naming the id, got %v", err)
	}
}

// TestApplyBeforeWriteError: a failing hook stops the apply before any write.
func TestApplyBeforeWriteError(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	before := treeHash(t, f.root)
	boom := errors.New("hook failed")
	if _, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project}, ApplyOptions{BeforeWrite: func() error { return boom }}); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if after := treeHash(t, f.root); after != before {
		t.Fatalf("a failed hook still wrote:\n%s", lineDiff(before, after))
	}
}

// TestPlanIDBindsContent: a change that keeps every shown number the same (one
// env value swapped for another of the same length) still changes the plan id,
// the old id is refused with E450 and nothing is written, and unchanged inputs
// give the same id on every run.
func TestPlanIDBindsContent(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	a, again := planOf(t, f, nil), planOf(t, f, nil)
	if a.PlanID != again.PlanID {
		t.Fatal("identical inputs gave different plan ids")
	}
	setEnvValue(t, f, "BASE_DOMAIN", "swapped.test") // same line count and length class
	b := planOf(t, f, nil)
	if len(a.Artifacts) != len(b.Artifacts) {
		t.Fatal("the test change must keep the plan shape")
	}
	for i := range a.Artifacts {
		if a.Artifacts[i] != b.Artifacts[i] {
			t.Fatalf("shape differs at %s; the swap must be shape-neutral", a.Artifacts[i].Path)
		}
	}
	if a.PlanID == b.PlanID {
		t.Fatal("a same-shape content change did not change the plan id")
	}
	before := treeHash(t, f.root)
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed")}, ApplyOptions{PlanID: a.PlanID})
	if d := errs.Describe(err); d == nil || d.Code != "E450" {
		t.Fatalf("want E450, got %v", err)
	}
	if treeHash(t, f.root) != before {
		t.Fatal("E450 apply wrote")
	}
	if _, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed")}, ApplyOptions{PlanID: b.PlanID}); err != nil {
		t.Fatalf("the current id must apply: %v", err)
	}
}

// dropSecrets removes the pre-seeded secrets from a fixture so the build must
// generate them (the first build of a project).
func dropSecrets(t *testing.T, f *fixture) {
	t.Helper()
	env := strings.SplitN(readFile(t, f.project+"/.env"), "POSTGRES_PASSWORD=", 2)[0]
	writeFile(t, f.project+"/.env", env+"POSTGRES_PASSWORD="+fixtureValue(t, "POSTGRES_PASSWORD")+"\n"+
		"HASURA_GRAPHQL_ADMIN_SECRET="+fixtureValue(t, "HASURA_GRAPHQL_ADMIN_SECRET")+"\n", 0o600)
}

// TestPlanCoversGeneratedSecrets (review M2a, Codex 2): a build that generates
// secrets lists .env.secrets as an artifact with planned bytes, refuses
// --plan-id with E451 (random values cannot be bound), and an apply without
// --plan-id writes exactly the confirmed render.
func TestPlanCoversGeneratedSecrets(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	dropSecrets(t, f)
	p := planOf(t, f, nil)
	var listed bool
	for _, a := range p.Artifacts {
		listed = listed || (a.Path == ".env.secrets" && a.Action == ActionAdd && a.DiffLines > 0)
	}
	if !listed {
		t.Fatalf(".env.secrets is not a planned artifact: %+v", p.Artifacts)
	}
	before := treeHash(t, f.root)
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project}, ApplyOptions{PlanID: p.PlanID, Yes: true})
	if d := errs.Describe(err); d == nil || d.Code != "E451" {
		t.Fatalf("want E451, got %v", err)
	}
	if treeHash(t, f.root) != before {
		t.Fatal("E451 apply wrote")
	}
	if _, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("s")}, ApplyOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.project + "/.env.secrets"); err != nil {
		t.Fatal("apply did not persist the generated secrets")
	}
}

// TestPlanCoversPluginFragmentAndModes: a fragment rewritten in place is a plan
// artifact bound by the id (a port edited after the plan is E450), and a
// permission-only change (a 0644 .env) is shown as a change with diff_lines 0
// and applied.
func TestPlanCoversPluginFragmentAndModes(t *testing.T) {
	f := loadFixture(t, "dev-plugin")
	p := planOf(t, f, nil)
	var frag bool
	for _, a := range p.Artifacts {
		frag = frag || (a.Path == PluginsPrefix+"nself-alpha/docker-compose.plugin.yml" && a.Action == ActionChange)
	}
	if !frag {
		t.Fatalf("the rewritten plugin fragment is not a plan artifact: %+v", p.Artifacts)
	}
	path := filepath.Join(f.plugins, "nself-alpha", "docker-compose.plugin.yml")
	raw := readFile(t, path)
	writeFile(t, path, strings.Replace(raw, `"3901:3901"`, `"0.0.0.0:22:3901"`, 1), 0o644)
	before := treeHash(t, f.root)
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed")}, ApplyOptions{PlanID: p.PlanID})
	if d := errs.Describe(err); d == nil || d.Code != "E450" {
		t.Fatalf("a fragment edited after the plan must be E450, got %v", err)
	}
	if treeHash(t, f.root) != before {
		t.Fatal("E450 wrote")
	}

	g := loadFixture(t, "dev-minimal")
	if err := os.Chmod(g.project+"/.env", 0o644); err != nil {
		t.Fatal(err)
	}
	mp := planOf(t, g, nil)
	var mode bool
	for _, a := range mp.Artifacts {
		mode = mode || (a.Path == ".env" && a.Action == ActionChange && a.DiffLines == 0 && a.Redacted)
	}
	if !mode {
		t.Fatalf("a 0644 .env must be a mode change in the plan: %+v", mp.Artifacts)
	}
	applyFixture(t, g)
	if info, _ := os.Stat(g.project + "/.env"); info.Mode().Perm() != 0o600 {
		t.Fatalf(".env is %v after apply", info.Mode().Perm())
	}
	if !planOf(t, g, nil).Empty {
		t.Fatal("plan after apply is not empty")
	}
}

// TestPlanNeverMutatesProcessEnv (review M1a): planning leaves os.Environ as it
// found it, and an ENV set in a cascade file other than .env resolves the same
// in the plan and the apply, so the re-plan is empty.
func TestPlanNeverMutatesProcessEnv(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	writeFile(t, f.project+"/.env.local", "ENV=prod\n", 0o600)
	writeFile(t, f.project+"/.env.prod", "BASE_DOMAIN=prodonly.example.org\nSSL_MODE=none\n", 0o600)
	snap := strings.Join(os.Environ(), "\n")
	p := planOf(t, f, nil)
	if strings.Join(os.Environ(), "\n") != snap {
		t.Fatal("Compute changed the process environment")
	}
	var diff strings.Builder
	planOf(t, f, func(r *Request) { r.DiffOut = &diff })
	applyFixture(t, f)
	if !planOf(t, f, nil).Empty {
		t.Fatalf("plan (env %s) differs from the apply: the plan run leaked its env into the write", p.Env)
	}
}

// TestComposedGateNotToggledByProjectFiles (review M1b): with the operator in
// v1.5, NSELF_V15=0 in project files must not turn the E403 gate off.
func TestComposedGateNotToggledByProjectFiles(t *testing.T) {
	compattest.Set(t, true)
	f := loadFixture(t, "prod-ssl")
	l03 := readFile(t, f.project+"/.env") + "NSELF_V15=0\n"
	writeFile(t, f.project+"/.env", l03, 0o600)
	writeFile(t, f.project+"/.env.local", "NSELF_V15=false\n", 0o600)
	before := treeHash(t, f.root)
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project}, ApplyOptions{})
	wantE403(t, err)
	if treeHash(t, f.root) != before {
		t.Fatal("the refused apply wrote")
	}
	if os.Getenv("NSELF_V15") != "1" {
		t.Fatalf("NSELF_V15 is %q after the command", os.Getenv("NSELF_V15"))
	}
}

// TestApplyEditDuringPrompt (review M3): a value edited while the confirmation
// prompt waits cannot reach the disk: the apply is refused (E450) with nothing
// written.
func TestApplyEditDuringPrompt(t *testing.T) {
	compattest.Set(t, true)
	f := loadFixture(t, "prod-ssl")
	before := treeHash(t, f.root)
	asked := false
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project}, ApplyOptions{Interactive: func(string) bool {
		asked = true
		setEnvValue(t, f, "POSTGRES_PASSWORD", "Zq7Wx3Tn9Rk5Hb2Vc8Lm4Pd6Sa1FgJu0Ye8")
		return true
	}})
	if !asked {
		t.Fatal("the prompt was never shown")
	}
	if d := errs.Describe(err); d == nil || d.Code != "E450" {
		t.Fatalf("want E450, got %v", err)
	}
	if after := treeHash(t, f.root); strings.Contains(after, "docker-compose.yml") || strings.Count(after, "\n") != strings.Count(before, "\n") {
		// the only difference allowed is the edited .env itself
		if strings.Contains(lineDiff(before, after), ".nself/compose.env") {
			t.Fatal("the edited value reached the generated files")
		}
	}
	if _, err := os.Stat(f.project + "/docker-compose.yml"); err == nil {
		t.Fatal("a refused apply wrote docker-compose.yml")
	}
}

// TestApplyHeldToConfirmedRender: an edit after the re-check (here injected
// between the re-check and the write) makes the write fail instead of writing
// different bytes.
func TestApplyHeldToConfirmedRender(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	afterRecheck = func() { setEnvValue(t, f, "POSTGRES_PASSWORD", "Zq7Wx3Tn9Rk5Hb2Vc8Lm4Pd6Sa1FgJu0Ye8") }
	t.Cleanup(func() { afterRecheck = nil })
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project}, ApplyOptions{})
	if err == nil || !strings.Contains(err.Error(), "changed after the plan") {
		t.Fatalf("want the held-to-plan error, got %v", err)
	}
	if b, err := os.ReadFile(f.project + "/.nself/compose.env"); err == nil && strings.Contains(string(b), "Zq7Wx3Tn9Rk5Hb2Vc8Lm4Pd6Sa1FgJu0Ye8") {
		t.Fatal("the value edited after the plan was written")
	}
}

// TestUnknownContainersNeverEmpty (review M5): a plan that could not ask Docker
// is not empty, says so in the human output, and on a prod-class env asks.
func TestUnknownContainersNeverEmpty(t *testing.T) {
	for _, name := range []string{"dev-minimal", "prod-ssl"} {
		f := loadFixture(t, name)
		applyFixture(t, f)
		p := planOf(t, f, func(r *Request) { r.Containers, r.Runtime = true, &fakeRuntime{runErr: errors.New("daemon down")} })
		var sb strings.Builder
		_ = RenderHuman(&sb, *p)
		if p.Empty || p.Containers.Known || strings.Contains(sb.String(), "(no changes)") || !strings.Contains(sb.String(), "state unknown") {
			t.Fatalf("%s: empty=%v known=%v\n%s", name, p.Empty, p.Containers.Known, sb.String())
		}
		if want := name == "prod-ssl"; p.RequiresConfirmation != want {
			t.Fatalf("%s: requires_confirmation=%v", name, p.RequiresConfirmation)
		}
	}
}

// fakePlugin stands in for the registry download and the schema step: it lays a
// plugin down in pluginDir the way an install would (manifest, compose
// fragment, nginx route).
func fakePlugin(t *testing.T, pluginDir, name string) {
	t.Helper()
	dir := filepath.Join(pluginDir, name)
	writeFile(t, filepath.Join(dir, "plugin.json"), fmt.Sprintf(`{"name":%q,"port":3911,"language":"go"}`, name), 0o644)
	writeFile(t, filepath.Join(dir, "docker-compose.plugin.yml"), fmt.Sprintf("services:\n  %s:\n    image: nself/%s:latest\n    ports:\n      - \"3911:3911\"\n", name, name), 0o644)
	writeFile(t, filepath.Join(dir, "Dockerfile"), "FROM scratch\n", 0o644)
	writeFile(t, filepath.Join(dir, "nginx", "route.conf"), "server {\n  listen 80;\n  server_name ${PLUGIN_NAME}.example.test;\n}\n", 0o644)
}

// TestApplyInstallsDeclaredPluginInOnePass (review R1): a project whose
// nself.yaml declares a plugin that is not installed builds in one pass: the
// install runs as a confirmed effect inside the held apply, the plugin's nginx
// and compose artifacts are rendered, checked and held, and the plan afterwards
// is empty.
func TestApplyInstallsDeclaredPluginInOnePass(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	writeFile(t, f.project+"/nself.yaml", "plugins:\n  - fakeplug\n", 0o644)
	installs := 0
	old := nbuild.PluginInstall
	nbuild.PluginInstall = func(_ context.Context, _ *config.Config, name, pluginDir string) error {
		installs++
		fakePlugin(t, pluginDir, name)
		return nil
	}
	t.Cleanup(func() { nbuild.PluginInstall = old })

	p := planOf(t, f, nil)
	if !hasEffect(p, EffectPluginInstall) || installs != 0 {
		t.Fatalf("the plan must list the install and not run it (installs=%d, effects %+v)", installs, p.Effects)
	}
	if _, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("s")}, ApplyOptions{Yes: true}); err != nil {
		t.Fatalf("one-pass build with a declared plugin: %v", err)
	}
	if installs != 1 {
		t.Fatalf("plugin installed %d times, want 1", installs)
	}
	manifest := readFile(t, f.project+"/.nself/compose-files.txt")
	if !strings.Contains(manifest, "fakeplug") {
		t.Fatalf("the plugin fragment is not wired into the stack:\n%s", manifest)
	}
	if left := planOf(t, f, nil); !left.Empty {
		var sb strings.Builder
		_ = RenderHuman(&sb, *left)
		t.Fatalf("plan after the apply is not empty:\n%s", sb.String())
	}
}

// TestApplyEditDuringPromptWithPluginRemoval (review R2): with an expired
// plugin due for removal, an edit made while the prompt waits is refused (E450)
// before the removal runs: no exemption from the hold.
func TestApplyEditDuringPromptWithPluginRemoval(t *testing.T) {
	compattest.Set(t, true)
	f := loadFixture(t, "prod-ssl")
	before := treeHash(t, f.root)
	removed := false
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project,
		Extra: []Effect{{Kind: EffectPluginRemove, Target: "oldplug", Detail: "license grace period exhausted"}}},
		ApplyOptions{
			Interactive: func(string) bool {
				setEnvValue(t, f, "POSTGRES_PASSWORD", "Zq7Wx3Tn9Rk5Hb2Vc8Lm4Pd6Sa1FgJu0Ye8")
				return true
			},
			BeforeWrite: func() error { removed = true; return nil },
		})
	if d := errs.Describe(err); d == nil || d.Code != "E450" {
		t.Fatalf("want E450, got %v", err)
	}
	if removed {
		t.Fatal("the removal ran although the project changed under the prompt")
	}
	if _, err := os.Stat(f.project + "/docker-compose.yml"); err == nil {
		t.Fatal("a refused apply wrote")
	}
	_ = before
}

// TestApplyHoldAfterPluginRemoval: after a removal the write is still held to the
// final render: an edit made after the removal is caught.
func TestApplyHoldAfterPluginRemoval(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	afterRecheck = func() { setEnvValue(t, f, "POSTGRES_PASSWORD", "Zq7Wx3Tn9Rk5Hb2Vc8Lm4Pd6Sa1FgJu0Ye8") }
	t.Cleanup(func() { afterRecheck = nil })
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project,
		Extra: []Effect{{Kind: EffectPluginRemove, Target: "oldplug"}}}, ApplyOptions{BeforeWrite: func() error { return nil }})
	if err == nil || !strings.Contains(err.Error(), "changed after the plan") {
		t.Fatalf("a plan with a removal must still be held, got %v", err)
	}
}

// TestApplyAsksAgainAfterPluginInstall: on a prod-class env the render that
// exists after a plugin install is shown and asked about again, and both answers
// are needed.
func TestApplyAsksAgainAfterPluginInstall(t *testing.T) {
	compattest.Set(t, true)
	f := loadFixture(t, "prod-ssl")
	writeFile(t, f.project+"/nself.yaml", "plugins:\n  - fakeplug\n", 0o644)
	old := nbuild.PluginInstall
	nbuild.PluginInstall = func(_ context.Context, _ *config.Config, name, pluginDir string) error {
		fakePlugin(t, pluginDir, name)
		return nil
	}
	t.Cleanup(func() { nbuild.PluginInstall = old })
	var asked []string
	var notes strings.Builder
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Stderr: &notes}, ApplyOptions{
		Interactive: func(q string) bool { asked = append(asked, q); return len(asked) == 1 }})
	wantE403(t, err)
	if len(asked) != 2 || !strings.Contains(notes.String(), "after the plugin changes the build will write") {
		t.Fatalf("asked %d times, notes %q", len(asked), notes.String())
	}
	if _, err := os.Stat(f.project + "/docker-compose.yml"); err == nil {
		t.Fatal("a declined second confirmation still wrote")
	}
}

// TestOverlayDisplayAbs: a file outside the project and the plugin dir is shown
// as @abs/<path>.
func TestOverlayDisplayAbs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("absolute keys are unix paths")
	}
	ov := overlay{dir: t.TempDir()}
	if got := ov.display("/opt/somewhere/x.yml"); got != AbsPrefix+"opt/somewhere/x.yml" {
		t.Fatalf("display = %q", got)
	}
	if got := ov.display("nginx/a.conf"); got != "nginx/a.conf" {
		t.Fatalf("display = %q", got)
	}
}

// builtFixture builds the fixture and ages .env so the build's freshness cache
// (a comparison of .env's mtime with docker-compose.yml) says "fresh".
func builtFixture(t *testing.T, name string) *fixture {
	t.Helper()
	f := loadFixture(t, name)
	applyFixture(t, f)
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(f.project+"/.env", old, old); err != nil {
		t.Fatal(err)
	}
	return f
}

// TestApplyWritesWhenOnlySecretsFileChanged (review F1): a change outside .env
// is planned, so it must be written even though the freshness cache looks fresh.
func TestApplyWritesWhenOnlySecretsFileChanged(t *testing.T) {
	f := builtFixture(t, "dev-minimal")
	const pw = "Zq7Wx3Tn9Rk5Hb2Vc8Lm4Pd6Sa1FgJu0Ye8"
	writeFile(t, f.project+"/.env.secrets", "POSTGRES_PASSWORD="+pw+"\n", 0o600)
	p := planOf(t, f, nil)
	if p.Empty {
		t.Fatal("the secrets change is not planned")
	}
	if _, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed")}, ApplyOptions{PlanID: p.PlanID}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(readFile(t, f.project+"/.nself/compose.env"), pw) {
		t.Fatal("the planned password change was reported applied but not written (cache skipped the build)")
	}
	if !planOf(t, f, nil).Empty {
		t.Fatal("plan after the apply is not empty")
	}
}

// TestApplyAddsPluginToBuiltProject (review F1): adding a plugin to nself.yaml of
// a built project installs it and wires it in the same apply.
func TestApplyAddsPluginToBuiltProject(t *testing.T) {
	f := builtFixture(t, "dev-minimal")
	writeFile(t, f.project+"/nself.yaml", "plugins:\n  - fakeplug\n", 0o644)
	old := nbuild.PluginInstall
	nbuild.PluginInstall = func(_ context.Context, _ *config.Config, name, pluginDir string) error {
		fakePlugin(t, pluginDir, name)
		return nil
	}
	t.Cleanup(func() { nbuild.PluginInstall = old })
	if _, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("s")}, ApplyOptions{Yes: true}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(readFile(t, f.project+"/.nself/compose-files.txt"), "fakeplug") {
		t.Fatal("the plugin was installed but not wired (cache skipped the build)")
	}
}

// TestPlanIDRefusedWithPluginEffects (review F2): the id cannot bind what a
// plugin install brings, so --plan-id is refused (E453) before anything runs.
func TestPlanIDRefusedWithPluginEffects(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	writeFile(t, f.project+"/nself.yaml", "plugins:\n  - fakeplug\n", 0o644)
	installs := 0
	old := nbuild.PluginInstall
	nbuild.PluginInstall = func(context.Context, *config.Config, string, string) error { installs++; return nil }
	t.Cleanup(func() { nbuild.PluginInstall = old })
	p := planOf(t, f, nil)
	before := treeHash(t, f.root)
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed")}, ApplyOptions{PlanID: p.PlanID, Yes: true})
	if d := errs.Describe(err); d == nil || d.Code != "E453" {
		t.Fatalf("want E453, got %v", err)
	}
	if installs != 0 || treeHash(t, f.root) != before {
		t.Fatal("E453 ran effects or wrote")
	}
}

// TestSecondRenderNeedsForceForHandEdits (review F2): a hand-edited file that
// only the render after the plugin install overwrites needs --force even with
// --yes and no terminal.
func TestSecondRenderNeedsForceForHandEdits(t *testing.T) {
	compattest.Set(t, true)
	f := builtFixture(t, "dev-minimal")
	writeFile(t, f.project+"/nself.yaml", "plugins:\n  - fakeplug\n", 0o644)
	old := nbuild.PluginInstall
	nbuild.PluginInstall = func(_ context.Context, _ *config.Config, name, pluginDir string) error {
		fakePlugin(t, pluginDir, name)
		return nil
	}
	t.Cleanup(func() { nbuild.PluginInstall = old })
	hand := func(path string) bool { return path == ".nself/compose-files.txt" }
	_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, HandEdited: hand}, ApplyOptions{Yes: true})
	wantE403(t, err)
	if strings.Contains(readFile(t, f.project+"/.nself/compose-files.txt"), "fakeplug") {
		t.Fatal("a hand-edited file was overwritten without --force")
	}
	if _, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, HandEdited: hand}, ApplyOptions{Yes: true, Force: true}); err != nil {
		t.Fatalf("with --force: %v", err)
	}
}
