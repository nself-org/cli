package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nbuild "github.com/nself-org/cli/internal/build"
)

func TestStateFaultInjection(t *testing.T) {
	f := loadFixture(t, "dev-minimal")

	// Ensure no state to start
	os.RemoveAll(filepath.Join(f.project, ".nself", "state"))

	oldBuild := buildBuild
	t.Cleanup(func() { buildBuild = oldBuild })

	buildBuild = func(projectDir string, opts nbuild.BuildOptions) (*nbuild.BuildResult, error) {
		// Simulate half-writing a file that nself generates
		os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("half-written"), 0644)
		return nil, errors.New("injected failing sink after N writes")
	}

	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}
	_, err := Apply(context.Background(), req, ApplyOptions{Yes: true})
	if err == nil {
		t.Fatal("expected apply to fail")
	}

	state, err := LoadGeneratedState(f.project)
	if err != nil {
		t.Fatal(err)
	}
	if state != nil {
		t.Fatal("expected generated-state record to be intact (nil/missing)")
	}

	// Next plan reports it as changed, not hand-edited: the callback is the
	// real one Apply builds from the (absent) record.
	req.HandEdited = state.HandEditedFn(f.project)
	p, err := Compute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	hand := HandEditedPaths(*p)
	if len(hand) > 0 {
		t.Fatalf("expected no hand-edited files, got %v", hand)
	}
}

func TestStateFirstRun(t *testing.T) {
	f := loadFixture(t, "dev-minimal")

	// Ensure no state
	os.RemoveAll(filepath.Join(f.project, ".nself", "state"))

	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}
	_, err := Apply(context.Background(), req, ApplyOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}

	state, err := LoadGeneratedState(f.project)
	if err != nil || state == nil {
		t.Fatal("expected state to be recorded on first run")
	}
}

// TestStateRefusesRecordWithoutSchema: a record with no schema_version was
// not written by nself (every Save pins it), so loading it fails closed
// instead of trusting an unversioned map of digests.
func TestStateRefusesRecordWithoutSchema(t *testing.T) {
	dir := t.TempDir()
	p := GeneratedStatePath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"files":{"docker-compose.yml":"00"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s, err := LoadGeneratedState(dir); err == nil {
		t.Fatalf("a record without schema_version must be refused, got %+v", s)
	}
}

// TestHandEditedFnReadFailures: a deleted recorded file is not a hand-edit
// (no user bytes to lose, the build recreates it); any other read failure,
// here a directory in the file's place, counts as hand-edited (fail closed).
func TestHandEditedFnReadFailures(t *testing.T) {
	dir := t.TempDir()
	orig := []byte("services: {}\n")
	sum := sha256.Sum256(orig)
	s := &GeneratedState{Files: map[string]string{"docker-compose.yml": hex.EncodeToString(sum[:])}}
	fn := s.HandEditedFn(dir)
	target := filepath.Join(dir, "docker-compose.yml")

	if fn("docker-compose.yml") {
		t.Fatal("a deleted recorded file must not count as hand-edited")
	}
	if err := os.WriteFile(target, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	if fn("docker-compose.yml") {
		t.Fatal("unchanged bytes must not count as hand-edited")
	}
	if err := os.WriteFile(target, []byte("services: {x: 1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !fn("docker-compose.yml") {
		t.Fatal("changed bytes must count as hand-edited")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if !fn("docker-compose.yml") {
		t.Fatal("an unreadable recorded path (a directory) must fail closed as hand-edited")
	}
}

// TestStateFaultInjectionWithRecord: with a record already present, an apply
// killed after its file writes but before the record leaves generated.json
// byte-identical, and the next plan (real callback over that record) reports
// no hand-edits: files nself wrote equal what the plan would write, so they
// are not write targets. A file torn mid-write is a different case: its bytes
// match neither side, and it is reported hand-edited (fail closed, --force).
func TestStateFaultInjectionWithRecord(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}
	if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	rec0, err := os.ReadFile(GeneratedStatePath(f.project))
	if err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(f.project, ".env")
	env, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(env), "BASE_DOMAIN=example.test", "BASE_DOMAIN=changed.test", 1)
	if changed == string(env) {
		t.Fatal("fixture .env has no BASE_DOMAIN=example.test line to change")
	}
	if err := os.WriteFile(envPath, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}

	oldBuild := buildBuild
	t.Cleanup(func() { buildBuild = oldBuild })
	buildBuild = func(projectDir string, opts nbuild.BuildOptions) (*nbuild.BuildResult, error) {
		if _, err := oldBuild(projectDir, opts); err != nil {
			return nil, err
		}
		return nil, errors.New("injected kill after the writes, before the record")
	}
	if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err == nil {
		t.Fatal("expected the injected apply to fail")
	}
	buildBuild = oldBuild

	rec1, err := os.ReadFile(GeneratedStatePath(f.project))
	if err != nil {
		t.Fatal(err)
	}
	if string(rec1) != string(rec0) {
		t.Fatal("an interrupted apply must leave the record byte-identical")
	}
	state, err := LoadGeneratedState(f.project)
	if err != nil || state == nil {
		t.Fatalf("record must load after the interrupted apply: %v", err)
	}
	req.HandEdited = state.HandEditedFn(f.project)
	p, err := Compute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if hand := HandEditedPaths(*p); len(hand) > 0 {
		t.Fatalf("files nself itself wrote must not read as hand-edited, got %v", hand)
	}
}

// TestStateDanglingLinks: a dangling symlink in place of the record is an
// error, not a first run; in place of a recorded file it counts as
// hand-edited.
func TestStateDanglingLinks(t *testing.T) {
	dir := t.TempDir()
	p := GeneratedStatePath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "nowhere.json"), p); err != nil {
		t.Fatal(err)
	}
	if s, err := LoadGeneratedState(dir); err == nil {
		t.Fatalf("a dangling record link must be refused, got %+v", s)
	}
	s := &GeneratedState{Files: map[string]string{"docker-compose.yml": strings.Repeat("0", 64)}}
	if err := os.Symlink(filepath.Join(dir, "gone.yml"), filepath.Join(dir, "docker-compose.yml")); err != nil {
		t.Fatal(err)
	}
	if !s.HandEditedFn(dir)("docker-compose.yml") {
		t.Fatal("a dangling link in place of a recorded file must count as hand-edited")
	}
	var none *GeneratedState
	if none.HandEditedFn(dir)("docker-compose.yml") {
		t.Fatal("no record means nothing is hand-edited")
	}
}
