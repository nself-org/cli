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
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	nbuild "github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/oplock"
)

// mkPlan finalises a plan for the confirm tests.
func mkPlan(t *testing.T, env string, arts []Artifact, effs []Effect) Plan {
	t.Helper()
	p := Plan{Command: CmdBuild, Env: env, Artifacts: arts, Effects: effs}
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
	_, err := Apply(context.Background(), Request{ProjectDir: f.project}, ApplyOptions{BeforeWrite: func() error { hooked = true; return nil }})
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
	req := Request{ProjectDir: f.project, Seed: []byte("seed")}
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
		if _, err := Apply(context.Background(), Request{ProjectDir: f.project, Seed: []byte(seed)}, ApplyOptions{Yes: true}); err != nil {
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
	_, err = Apply(context.Background(), Request{ProjectDir: f.project}, ApplyOptions{Yes: true})
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
		if _, err := Apply(context.Background(), Request{ProjectDir: f.project}, ApplyOptions{Yes: true}); err != nil {
			t.Fatalf("build %d after the holder was killed: %v", i, err)
		}
	}
}

// TestApplyShortPlanID: a malformed id is E450 too (and the message abbreviates
// only ids longer than the header prefix).
func TestApplyShortPlanID(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	_, err := Apply(context.Background(), Request{ProjectDir: f.project}, ApplyOptions{PlanID: "abc"})
	if d := errs.Describe(err); d == nil || d.Code != "E450" || !strings.Contains(err.Error()+d.Cause, "abc") {
		t.Fatalf("want E450 naming the id, got %v", err)
	}
}

// TestApplyBeforeWriteError: a failing hook stops the apply before any write.
func TestApplyBeforeWriteError(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	before := treeHash(t, f.root)
	boom := errors.New("hook failed")
	if _, err := Apply(context.Background(), Request{ProjectDir: f.project}, ApplyOptions{BeforeWrite: func() error { return boom }}); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if after := treeHash(t, f.root); after != before {
		t.Fatalf("a failed hook still wrote:\n%s", lineDiff(before, after))
	}
}
