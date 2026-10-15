package reconcile

// overlay_test.go — the pending env line of `config set` (P7-LIVE-05).
//
// Purpose: prove the plan of a pending env line is the plan the project has
// once the line is written (same plan_id), that computing it writes nothing,
// and that EnvFileWithLine is the exact, idempotent rewrite config set uses.
// Inputs: the hermetic reconcile fixtures (planner_test.go). Constraints: no
// docker daemon (fakeRuntime), no real project.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	nbuild "github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/oplock"
)

func TestEnvFileWithLine(t *testing.T) {
	cases := []struct {
		name, old  string
		exists     bool
		key, value string
		want       string
		found      bool
	}{
		{"no file", "", false, "A", "1", "A=1\n", false},
		{"empty file", "", true, "A", "1", "A=1\n", false},
		{"append keeps comments", "# c\n\nB=2", true, "A", "1", "# c\n\nB=2\nA=1\n", false},
		{"replace every copy", "A=0\nB=2\n A =9\n", true, "A", "1", "A=1\nB=2\nA=1\n", true},
		{"crlf normalised", "A=0\r\nB=2\r\n", true, "A", "1", "A=1\nB=2\n", true},
		{"commented key untouched", "#A=0\n", true, "A", "1", "#A=0\nA=1\n", false},
		{"quoted value", "", false, "A", `p a"s$`, `A="p a\"s\$"` + "\n", false},
		{"empty value", "", false, "A", "", "A=\"\"\n", false},
		{"prefix key not matched", "AB=0\n", true, "A", "1", "AB=0\nA=1\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, found, err := EnvFileWithLine([]byte(tc.old), tc.exists, tc.key, tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want || found != tc.found {
				t.Fatalf("got %q found=%v, want %q found=%v", got, found, tc.want, tc.found)
			}
			again, found2, err := EnvFileWithLine(got, true, tc.key, tc.value)
			if err != nil || string(again) != string(got) || !found2 {
				t.Fatalf("not idempotent: %q -> %q (found=%v, err=%v)", got, again, found2, err)
			}
		})
	}
}

func TestEnvFileWithLineLongLine(t *testing.T) {
	long := "B=" + strings.Repeat("x", 70*1024) + "\n"
	if _, _, err := EnvFileWithLine([]byte(long), true, "A", "1"); err == nil {
		t.Fatal("a line over the scanner limit must fail, as config set always did")
	}
}

func TestPendingEnv(t *testing.T) {
	dir := t.TempDir()
	if got, changed, err := pendingEnv(dir, nil); got != nil || changed || err != nil {
		t.Fatalf("no overlay: %v %v", got, err)
	}
	writeFile(t, filepath.Join(dir, ".env"), "A=0\n", 0o600)
	got, changed, err := pendingEnv(dir, map[string]map[string]string{
		".env":         {"B": "2", "A": "1"},
		".env.secrets": {"S": "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got[".env"]) != "A=1\nB=2\n" || string(got[".env.secrets"]) != "S=x\n" {
		t.Fatalf("pending bytes: %q", got)
	}
	if !changed {
		t.Fatal("new lines must report a change")
	}
	writeFile(t, filepath.Join(dir, ".env"), "A=1\nB=2\n", 0o600)
	if _, changed, _ := pendingEnv(dir, map[string]map[string]string{".env": {"A": "1"}}); changed {
		t.Fatal("a line the file already holds is no change")
	}
	for _, bad := range []string{"env", "../.env", "sub/.env", ".envrc", `.env.a\b`} {
		if _, _, err := pendingEnv(dir, map[string]map[string]string{bad: {"A": "1"}}); err == nil {
			t.Errorf("%q accepted as an env file", bad)
		}
	}
	if runtime.GOOS != "windows" {
		if err := os.Mkdir(filepath.Join(dir, ".env.dir"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, _, err := pendingEnv(dir, map[string]map[string]string{".env.dir": {"A": "1"}}); err == nil {
			t.Error("an unreadable env file must fail the plan")
		}
	}
	writeFile(t, filepath.Join(dir, ".env.long"), "B="+strings.Repeat("x", 70*1024)+"\n", 0o600)
	if _, _, err := pendingEnv(dir, map[string]map[string]string{".env.long": {"A": "1"}}); err == nil {
		t.Error("an unscannable env file must fail the plan")
	}
}

func TestPendingHandEdited(t *testing.T) {
	dir := t.TempDir()
	next := func(p string) bool { return p == "other" }
	if f := pendingHandEdited(next, nil, nil, dir, ""); !f("other") {
		t.Fatal("no overlay must return next unchanged")
	}
	state := &GeneratedState{Files: map[string]string{".env.secrets": sha256Hex([]byte("S=x\n"))}}
	f := pendingHandEdited(next, state, map[string][]byte{".env.secrets": []byte("S=x\n")}, dir, "")
	if f(".env.secrets") || !f("other") || f("nope") {
		t.Fatal("pending bytes equal to the record are not hand-edited; others go to next")
	}
	f = pendingHandEdited(next, state, map[string][]byte{".env.secrets": []byte("S=y\n")}, dir, "")
	if !f(".env.secrets") {
		t.Fatal("pending bytes that differ from the record are hand-edited")
	}
	if pendingHandEdited(nil, state, map[string][]byte{".env": nil}, dir, "") != nil {
		t.Fatal("no detection asked for stays no detection")
	}
	// D-0607: a "@plugins/" path resolves under the build's plugin directory,
	// with no pending env and whatever the process env says.
	plugins := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	writeFile(t, filepath.Join(plugins, "p", "f.yml"), "edited by hand\n", 0o644)
	ps := &GeneratedState{Files: map[string]string{PluginsPrefix + "p/f.yml": sha256Hex([]byte("rendered\n"))}}
	if !pendingHandEdited(ps.HandEditedFn(dir), ps, nil, dir, plugins)(PluginsPrefix + "p/f.yml") {
		t.Fatal("a hand-edited fragment under the build's plugin directory must be seen")
	}
}

// TestPendingHandEditedFronting: a pending NGINX_FRONTED_BY moves the fronting
// stack the hand-edit check reads; the disk's value is not used.
func TestPendingHandEditedFronting(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("NGINX_FRONTED_BY", "")
	root := t.TempDir()
	project := filepath.Join(root, "front", "backend")
	writeFile(t, filepath.Join(project, ".env"), "PROJECT_NAME=p\n", 0o600)
	conf := "server {}\n"
	writeFile(t, filepath.Join(root, "front", "nginx", "f.conf"), "edited by hand\n", 0o644)
	state := &GeneratedState{Files: map[string]string{FrontingPrefix + "nginx/f.conf": sha256Hex([]byte(conf))}}
	next := state.HandEditedFn(project)
	if next(FrontingPrefix + "nginx/f.conf") {
		t.Fatal("without NGINX_FRONTED_BY on disk the fronting path must not resolve")
	}
	f := pendingHandEdited(next, state, map[string][]byte{".env": []byte("PROJECT_NAME=p\nNGINX_FRONTED_BY=front\n")}, project, "")
	if !f(FrontingPrefix + "nginx/f.conf") {
		t.Fatal("the pending NGINX_FRONTED_BY must resolve the fronting stack and see the hand edit")
	}
	// A pending ENV picks the layer that names the stack.
	writeFile(t, filepath.Join(project, ".env.staging"), "NGINX_FRONTED_BY=front\n", 0o600)
	f = pendingHandEdited(next, state, map[string][]byte{".env": []byte("PROJECT_NAME=p\nENV=staging\n")}, project, "")
	if !f(FrontingPrefix + "nginx/f.conf") {
		t.Fatal("the pending ENV must select .env.staging for NGINX_FRONTED_BY")
	}
}

// overlayCase is one pending change and how to set the fixture up for it.
type overlayCase struct {
	name, fixture, file, key, value string
	prep                            func(t *testing.T, f *fixture)
	wantKinds                       []ArtifactKind
	fresh                           bool                           // plan a never-built project (no apply first)
	after                           func(t *testing.T, f *fixture) // runs after the baseline apply
}

var overlayCases = []overlayCase{
	{name: "base domain", fixture: "dev-minimal", file: ".env", key: "BASE_DOMAIN", value: "other.test",
		wantKinds: []ArtifactKind{KindCompose, KindNginx}},
	// ENV picks the cascade layer: the overlay's ENV must select .env.staging.
	{name: "env to staging", fixture: "dev-minimal", file: ".env", key: "ENV", value: "staging",
		prep: func(t *testing.T, f *fixture) {
			writeFile(t, filepath.Join(f.project, ".env.staging"), "BASE_DOMAIN=staging.test\n", 0o600)
		}, wantKinds: []ArtifactKind{KindCompose, KindNginx}},
	// config set leaves the file 0600, so the plan must not chmod it.
	{name: "loose env mode", fixture: "dev-minimal", file: ".env", key: "BASE_DOMAIN", value: "other.test",
		after: func(t *testing.T, f *fixture) {
			if err := os.Chmod(filepath.Join(f.project, ".env"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	// The build appends a generated secret to the pending .env.secrets bytes.
	{name: "persist appends to pending secrets", fixture: "dev-minimal", file: ".env.secrets", key: "SMTP_PASSWORD", value: "Vt4pN8sQ2xL6mK9rB3wY7cF1hJ5dG0aZu",
		prep: func(t *testing.T, f *fixture) { dropEnvKey(t, f, "PLUGIN_INTERNAL_SECRET") }, fresh: true,
		wantKinds: []ArtifactKind{KindEnv}},
	// The build would persist a generated PLUGIN_INTERNAL_SECRET into
	// .env.secrets; the pending line supplies it, so the plan must not.
	{name: "secret not yet persisted", fixture: "dev-minimal", file: ".env.secrets", key: "PLUGIN_INTERNAL_SECRET", value: "Vt4pN8sQ2xL6mK9rB3wY7cF1hJ5dG0aZu",
		prep: func(t *testing.T, f *fixture) { dropEnvKey(t, f, "PLUGIN_INTERNAL_SECRET") }, fresh: true},
	{name: "rotate persisted secret", fixture: "dev-minimal", file: ".env.secrets", key: "PLUGIN_INTERNAL_SECRET", value: "Vt4pN8sQ2xL6mK9rB3wY7cF1hJ5dG0aZu",
		prep: func(t *testing.T, f *fixture) { dropEnvKey(t, f, "PLUGIN_INTERNAL_SECRET") }},
	// A custom service reads .env.secrets through CS_N_ENV_FILE: its values
	// are inlined into compose, so the plan must read the pending bytes too.
	{name: "custom service env file", fixture: "dev-minimal", file: ".env.secrets", key: "CS_TOKEN", value: "newvalue222",
		prep: func(t *testing.T, f *fixture) {
			appendEnv(t, f, "CS_1=fn:node:9500\nCS_1_ENV_FILE=.env.secrets\n")
			writeFile(t, filepath.Join(f.project, ".env.secrets"), "CS_TOKEN=oldvalue111\n", 0o600)
		}, wantKinds: []ArtifactKind{KindCompose}},
	{name: "prod layer", fixture: "prod-ssl", file: ".env.prod", key: "HASURA_GRAPHQL_ADMIN_SECRET", value: "Zq8vX3kLm9Pw2Rt7Yb4Nc6Hd1Jf5Gs0Qe"},
}

// TestPlanOfPendingLineEqualsPlanAfterWrite: for each case the plan of the
// pending line (nothing written) has the plan_id of the project after the
// line is written, the overlay plan leaves the tree untouched, and the
// overlay is idempotent (a plan with the overlay after the write is the same).
func TestPlanOfPendingLineEqualsPlanAfterWrite(t *testing.T) {
	for _, tc := range overlayCases {
		t.Run(tc.name, func(t *testing.T) {
			f := loadFixture(t, tc.fixture)
			if tc.prep != nil {
				tc.prep(t, f)
			}
			if !tc.fresh {
				applyFixture(t, f)
			}
			if tc.after != nil {
				tc.after(t, f)
			}
			overlay := func(r *Request) {
				r.EnvOverlay = map[string]map[string]string{tc.file: {tc.key: tc.value}}
			}
			before := treeHash(t, f.root)
			pending := planOf(t, f, overlay)
			if after := treeHash(t, f.root); after != before {
				t.Fatalf("planning the pending line changed the tree:\n%s", lineDiff(before, after))
			}
			if pending.Empty && len(tc.wantKinds) > 0 {
				t.Fatal("the pending change plans nothing")
			}
			for _, k := range tc.wantKinds {
				if !hasKind(pending, k) {
					t.Errorf("plan lacks a %s artifact: %+v", k, pending.Artifacts)
				}
			}
			writeLine(t, f, tc.file, tc.key, tc.value)
			written := planOf(t, f, nil)
			if written.PlanID != pending.PlanID {
				t.Fatalf("plan of the pending line %s != plan after the write %s\npending: %+v\nwritten: %+v",
					pending.PlanID, written.PlanID, pending.Artifacts, written.Artifacts)
			}
			if again := planOf(t, f, overlay); again.PlanID != pending.PlanID {
				t.Fatalf("overlay over the written line moved the plan: %s != %s", again.PlanID, pending.PlanID)
			}
		})
	}
}

// TestApplyWithPendingLine: Apply with the overlay and the line written in
// BeforeWrite regenerates exactly the plan; a plan afterwards is empty.
func TestApplyWithPendingLine(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	applyFixture(t, f)
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed"),
		EnvOverlay: map[string]map[string]string{".env": {"BASE_DOMAIN": "other.test"}}}
	wrote := false
	p, err := Apply(context.Background(), req, ApplyOptions{BeforeWrite: func() error {
		wrote = true
		writeLine(t, f, ".env", "BASE_DOMAIN", "other.test")
		return nil
	}})
	if err != nil || !wrote {
		t.Fatalf("Apply: %v (wrote=%v)", err, wrote)
	}
	if p.Empty || !hasKind(p, KindCompose) {
		t.Fatalf("applied plan: %+v", p.Artifacts)
	}
	if again := planOf(t, f, nil); !settledPlan(again) {
		t.Fatalf("plan after the apply is not empty: %+v", again.Artifacts)
	}
}

// TestApplyPendingLineEditedMeanwhile: a value changed on disk between the
// plan and the write (here: inside BeforeWrite, after the line) refuses with
// E450 before the build writes anything.
func TestApplyPendingLineEditedMeanwhile(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	applyFixture(t, f)
	compose := filepath.Join(f.project, "docker-compose.yml")
	was, err := os.ReadFile(compose)
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed"),
		EnvOverlay: map[string]map[string]string{".env": {"BASE_DOMAIN": "other.test"}}}
	_, err = Apply(context.Background(), req, ApplyOptions{BeforeWrite: func() error {
		writeLine(t, f, ".env", "BASE_DOMAIN", "other.test")
		writeLine(t, f, ".env", "PROJECT_NAME", "moved")
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("want the E450 refusal, got %v", err)
	}
	if now, _ := os.ReadFile(compose); string(now) != string(was) {
		t.Fatal("the build wrote after the refusal")
	}
}

func TestComputeRejectsBadOverlay(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	_, err := Compute(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project,
		EnvOverlay: map[string]map[string]string{"../.env": {"A": "1"}}})
	if err == nil {
		t.Fatal("an overlay outside the project root must be refused")
	}
}

func hasKind(p *Plan, k ArtifactKind) bool {
	for _, a := range p.Artifacts {
		if a.Kind == k {
			return true
		}
	}
	return false
}

// writeLine writes KEY=VALUE into a project env file with EnvFileWithLine, as
// config set does (0600).
func writeLine(t *testing.T, f *fixture, file, key, value string) {
	t.Helper()
	path := filepath.Join(f.project, file)
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	data, _, err := EnvFileWithLine(old, err == nil, key, value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
}

// dropEnvKey removes KEY=... from the project's .env.
func dropEnvKey(t *testing.T, f *fixture, key string) {
	t.Helper()
	path := filepath.Join(f.project, ".env")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var keep []string
	for _, l := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(l, key+"=") {
			keep = append(keep, l)
		}
	}
	writeFile(t, path, strings.Join(keep, "\n"), 0o600)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// envCapture is a fakeRuntime that keeps the bytes of the project .env the
// hash invocation was given, so a test can see the pending line reached it.
type envCapture struct {
	fakeRuntime
	dotenv string
}

func (e *envCapture) ConfigHashes(ctx context.Context, files, envFiles []string, dir string) (map[string]string, error) {
	for _, f := range envFiles {
		if filepath.Base(f) == ".env" {
			b, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			e.dotenv = string(b)
		}
	}
	return e.fakeRuntime.ConfigHashes(ctx, files, envFiles, dir)
}

// TestContainerImpactSeesPendingLine: the compose invocation container impact
// hashes reads the pending .env, not the file on disk.
func TestContainerImpactSeesPendingLine(t *testing.T) {
	f := loadFixture(t, "prod-plugins")
	applyFixture(t, f)
	rt := &envCapture{fakeRuntime: fakeRuntime{running: runningFromDisk(t, f)}}
	planOf(t, f, func(r *Request) {
		r.Containers, r.Runtime = true, rt
		r.EnvOverlay = map[string]map[string]string{".env": {"NSELF_LIVE05_PROBE": "pending-value"}}
	})
	if !strings.Contains(rt.dotenv, "NSELF_LIVE05_PROBE=pending-value\n") {
		t.Fatalf("the hashed .env lacks the pending line:\n%s", rt.dotenv)
	}
	if strings.Contains(readFixtureEnv(t, f), "NSELF_LIVE05_PROBE") {
		t.Fatal("the plan wrote the pending line to disk")
	}
}

func readFixtureEnv(t *testing.T, f *fixture) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.project, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestApplyRollbackUnderLock: a failure after BeforeWrite runs Rollback while
// the project lock is still held (its token is exported), joins a rollback
// error, and a failure before BeforeWrite never rolls back.
func TestApplyRollbackUnderLock(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	applyFixture(t, f)
	_ = os.Unsetenv(oplock.EnvToken)
	oldBuild := buildBuild
	t.Cleanup(func() { buildBuild = oldBuild })
	writeFailed := errors.New("write build failed")
	buildBuild = func(string, nbuild.BuildOptions) (*nbuild.BuildResult, error) { return nil, writeFailed }
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed"),
		EnvOverlay: map[string]map[string]string{".env": {"BASE_DOMAIN": "other.test"}}}
	var underLock, rolled bool
	_, err := Apply(context.Background(), req, ApplyOptions{
		BeforeWrite: func() error { writeLine(t, f, ".env", "BASE_DOMAIN", "other.test"); return nil },
		Rollback: func() error {
			rolled, underLock = true, os.Getenv(oplock.EnvToken) != ""
			return errors.New("rollback failed")
		},
	})
	if !errors.Is(err, writeFailed) || !strings.Contains(fmt.Sprint(err), "rollback failed") {
		t.Fatalf("err = %v, want the write error joined with the rollback error", err)
	}
	if !rolled || !underLock {
		t.Fatalf("rollback ran=%v under the lock=%v", rolled, underLock)
	}
	// A refusal (prod-class env, no --yes, v1.5) fails before BeforeWrite.
	compattest.Set(t, true)
	rolled = false
	req.EnvOverlay = map[string]map[string]string{".env": {"ENV": "prod"}}
	_, err = Apply(context.Background(), req, ApplyOptions{
		Rollback: func() error { rolled = true; return nil },
	})
	if err == nil || rolled {
		t.Fatalf("a refusal must not roll back: err=%v rolled=%v", err, rolled)
	}
}

// TestPlanEnvLeavingProdNeedsConfirmation: a pending ENV change away from a
// prod-class env requires confirmation and names both envs in the header.
func TestPlanEnvLeavingProdNeedsConfirmation(t *testing.T) {
	f := loadFixture(t, "prod-ssl")
	applyFixture(t, f)
	p := planOf(t, f, func(r *Request) { r.EnvOverlay = map[string]map[string]string{".env": {"ENV": "dev"}} })
	if p.Empty || p.EnvClass != ClassDev || !p.RequiresConfirmation {
		t.Fatalf("empty=%v class=%v requires_confirmation=%v", p.Empty, p.EnvClass, p.RequiresConfirmation)
	}
	var sb strings.Builder
	if err := RenderHuman(&sb, *p); err != nil || !strings.Contains(sb.String(), "env prod → dev/dev") {
		t.Fatalf("header: %q %v", sb.String(), err)
	}
	same := planOf(t, f, func(r *Request) {
		r.EnvOverlay = map[string]map[string]string{".env": {"BASE_DOMAIN": "x.example.org"}}
		r.EnvBefore = "dev" // an explicit EnvBefore is used as given
	})
	if same.EnvClass != ClassProd || !same.RequiresConfirmation {
		t.Fatalf("a prod plan still needs confirmation: %+v", same)
	}
}

// TestDiskEnvLikeConfigLoad: the ENV a pending change starts from is the one
// config.Load ends with: a file value wins over the process environment, the
// layer picked by ENV counts, and nothing set means dev.
func TestDiskEnvLikeConfigLoad(t *testing.T) {
	t.Setenv("ENV", "")
	dir := t.TempDir()
	if got := diskEnv(dir); got != "dev" {
		t.Fatalf("empty project: %q", got)
	}
	writeFile(t, filepath.Join(dir, ".env"), "ENV=dev\n", 0o600)
	t.Setenv("ENV", "prod")
	if got := diskEnv(dir); got != "dev" {
		t.Fatalf("the .env value must win over the process env, as Overload does: %q", got)
	}
	t.Setenv("ENV", "")
	writeFile(t, filepath.Join(dir, ".env.secrets"), "ENV=staging\n", 0o600)
	if got := diskEnv(dir); got != "staging" {
		t.Fatalf("a later layer wins: %q", got)
	}
}

// appendEnv appends lines to the project's .env.
func appendEnv(t *testing.T, f *fixture, lines string) {
	t.Helper()
	path := filepath.Join(f.project, ".env")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(raw)+"\n"+lines, 0o600)
}

// TestApplyRollbackOnPanic: a panic after the write rolls back, then the
// panic continues to the caller.
func TestApplyRollbackOnPanic(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	applyFixture(t, f)
	oldBuild := buildBuild
	t.Cleanup(func() { buildBuild = oldBuild })
	buildBuild = func(string, nbuild.BuildOptions) (*nbuild.BuildResult, error) { panic("boom in write build") }
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed"),
		EnvOverlay: map[string]map[string]string{".env": {"BASE_DOMAIN": "other.test"}}}
	rolled := false
	recovered := func() (r any) {
		defer func() { r = recover() }()
		_, _ = Apply(context.Background(), req, ApplyOptions{
			BeforeWrite: func() error { writeLine(t, f, ".env", "BASE_DOMAIN", "other.test"); return nil },
			Rollback:    func() error { rolled = true; return nil },
		})
		return nil
	}()
	if recovered == nil || !rolled {
		t.Fatalf("panic propagated=%v rolled back=%v", recovered, rolled)
	}
}

// TestPlanProdEnvLineOnlyNeedsConfirmation: on a prod-class env a pending
// line that regenerates nothing is still a change that needs confirmation;
// the confirmation and plan_id hold across the write (Apply with --yes).
func TestPlanProdEnvLineOnlyNeedsConfirmation(t *testing.T) {
	f := loadFixture(t, "prod-plugins")
	compattest.Set(t, true) // one mode throughout, so the baseline apply leaves nothing to do
	applyFixture(t, f)
	rt := &fakeRuntime{running: runningFromDisk(t, f)}
	overlay := map[string]map[string]string{".env": {"NSELF_LIVE05_PROBE": "x"}}
	p := planOf(t, f, func(r *Request) { r.Containers, r.Runtime, r.EnvOverlay = true, rt, overlay })
	if !settledPlan(p) || !p.PendingEnv || !p.RequiresConfirmation {
		t.Fatalf("empty=%v pending_env=%v requires_confirmation=%v", p.Empty, p.PendingEnv, p.RequiresConfirmation)
	}
	if err := Confirm(*p, ApplyOptions{}, true, nil); err == nil {
		t.Fatal("v1.5 non-interactive must refuse an env-line-only prod change")
	}
	name := fixtureProjectName(t, f)
	writeLine(t, f, ".env", "PROJECT_NAME", name) // normalise the file the way config set writes it
	same := planOf(t, f, func(r *Request) {
		r.Containers, r.Runtime = true, rt
		r.EnvOverlay = map[string]map[string]string{".env": {"PROJECT_NAME": name}}
	})
	if same.PendingEnv || !settledPlan(same) || same.Empty || !same.RequiresConfirmation {
		t.Fatalf("an equal line leaves only the confirmed prod snapshot: %+v", same)
	}
	req := Request{Runtime: rt, Containers: true, ProjectDir: f.project, Seed: []byte("fixture-seed"), EnvOverlay: overlay}
	applied, err := Apply(context.Background(), req, ApplyOptions{Yes: true,
		BeforeWrite: func() error { writeLine(t, f, ".env", "NSELF_LIVE05_PROBE", "x"); return nil }})
	if err != nil {
		t.Fatalf("--yes must apply an env-line-only prod change (stable plan across the write): %v", err)
	}
	if !settledPlan(applied) || applied.Empty || !applied.RequiresConfirmation {
		t.Fatalf("the applied plan must be the env-line-only one: empty=%v rc=%v %v", applied.Empty, applied.RequiresConfirmation, applied.Artifacts)
	}
}

// fixtureProjectName returns PROJECT_NAME from the fixture's .env.
func fixtureProjectName(t *testing.T, f *fixture) string {
	t.Helper()
	for _, l := range strings.Split(readFixtureEnv(t, f), "\n") {
		if v, ok := strings.CutPrefix(l, "PROJECT_NAME="); ok {
			return v
		}
	}
	t.Fatal("no PROJECT_NAME in the fixture")
	return ""
}
