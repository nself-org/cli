package commands

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/deploy/childerr"
)

// TestScratchBuildFailureIncludesStderr: the deploy error names why the build failed.
func TestScratchBuildFailureIncludesStderr(t *testing.T) {
	old := deployScratchBuildFn
	t.Cleanup(func() { deployScratchBuildFn = old })
	deployScratchBuildFn = func(context.Context, string, bool, []deployStep) ([]deployStep, error) {
		return nil, &childerr.Failure{Err: errors.New("exit status 1"), Tail: "no such file: docker-compose"}
	}
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("PROJECT_NAME=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runScratchBuild(context.Background(), dir, "qa", deploy.TargetSnapshot{})
	if err == nil || !strings.Contains(err.Error(), "scratch build failed") || !strings.Contains(err.Error(), "no such file: docker-compose") {
		t.Fatalf("got %v", err)
	}
}
