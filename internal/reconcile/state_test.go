package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
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

	// Next plan reports it as changed, not hand-edited
	req.HandEdited = func(path string) bool { return false } // Default when state is nil
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
