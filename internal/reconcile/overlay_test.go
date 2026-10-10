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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
		{"quoted value", "", false, "A", `p a"s$`, `A="p a\"s$"` + "\n", false},
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
	if got, err := pendingEnv(dir, nil); got != nil || err != nil {
		t.Fatalf("no overlay: %v %v", got, err)
	}
	writeFile(t, filepath.Join(dir, ".env"), "A=0\n", 0o600)
	got, err := pendingEnv(dir, map[string]map[string]string{
		".env":         {"B": "2", "A": "1"},
		".env.secrets": {"S": "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got[".env"]) != "A=1\nB=2\n" || string(got[".env.secrets"]) != "S=x\n" {
		t.Fatalf("pending bytes: %q", got)
	}
	for _, bad := range []string{"env", "../.env", "sub/.env", ".envrc", `.env.a\b`} {
		if _, err := pendingEnv(dir, map[string]map[string]string{bad: {"A": "1"}}); err == nil {
			t.Errorf("%q accepted as an env file", bad)
		}
	}
	if runtime.GOOS != "windows" {
		if err := os.Mkdir(filepath.Join(dir, ".env.dir"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := pendingEnv(dir, map[string]map[string]string{".env.dir": {"A": "1"}}); err == nil {
			t.Error("an unreadable env file must fail the plan")
		}
	}
	writeFile(t, filepath.Join(dir, ".env.long"), "B="+strings.Repeat("x", 70*1024)+"\n", 0o600)
	if _, err := pendingEnv(dir, map[string]map[string]string{".env.long": {"A": "1"}}); err == nil {
		t.Error("an unscannable env file must fail the plan")
	}
}

func TestPendingHandEdited(t *testing.T) {
	next := func(p string) bool { return p == "other" }
	if f := pendingHandEdited(next, nil, nil); !f("other") {
		t.Fatal("no overlay must return next unchanged")
	}
	state := &GeneratedState{Files: map[string]string{".env.secrets": sha256Hex([]byte("S=x\n"))}}
	f := pendingHandEdited(next, state, map[string][]byte{".env.secrets": []byte("S=x\n")})
	if f(".env.secrets") || !f("other") || f("nope") {
		t.Fatal("pending bytes equal to the record are not hand-edited; others go to next")
	}
	f = pendingHandEdited(nil, state, map[string][]byte{".env.secrets": []byte("S=y\n")})
	if !f(".env.secrets") || f("other") {
		t.Fatal("pending bytes that differ from the record are hand-edited; nil next is never")
	}
}

// overlayCase is one pending change and how to set the fixture up for it.
type overlayCase struct {
	name, fixture, file, key, value string
	prep                            func(t *testing.T, f *fixture)
	wantKinds                       []ArtifactKind
	fresh                           bool // plan a never-built project (no apply first)
}

var overlayCases = []overlayCase{
	{name: "base domain", fixture: "dev-minimal", file: ".env", key: "BASE_DOMAIN", value: "other.test",
		wantKinds: []ArtifactKind{KindCompose, KindNginx}},
	{name: "env to staging", fixture: "dev-minimal", file: ".env", key: "ENV", value: "staging"},
	// The build would persist a generated PLUGIN_INTERNAL_SECRET into
	// .env.secrets; the pending line supplies it, so the plan must not.
	{name: "secret not yet persisted", fixture: "dev-minimal", file: ".env.secrets", key: "PLUGIN_INTERNAL_SECRET", value: "Vt4pN8sQ2xL6mK9rB3wY7cF1hJ5dG0aZu",
		prep: func(t *testing.T, f *fixture) { dropEnvKey(t, f, "PLUGIN_INTERNAL_SECRET") }, fresh: true},
	{name: "rotate persisted secret", fixture: "dev-minimal", file: ".env.secrets", key: "PLUGIN_INTERNAL_SECRET", value: "Vt4pN8sQ2xL6mK9rB3wY7cF1hJ5dG0aZu",
		prep: func(t *testing.T, f *fixture) { dropEnvKey(t, f, "PLUGIN_INTERNAL_SECRET") }},
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
	if again := planOf(t, f, nil); !again.Empty {
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
