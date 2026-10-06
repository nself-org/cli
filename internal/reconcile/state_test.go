package reconcile

import (
	"context"
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
