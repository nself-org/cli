package docker

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/docker/dockertest"
)

func TestProjectStreamKeepsExplicitInterpolationScope(t *testing.T) {
	stub := installRecordingDocker(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".nself"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".nself", "compose.env"), []byte("SCOPE=project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, flag := range [][]string{{"--env-file", "custom.env"}, {"--env-file=custom.env"}} {
		args := append(append([]string{"compose"}, flag...), "logs")
		if err := Stream(context.Background(), root, args, io.Discard, io.Discard); err != nil {
			t.Fatal(err)
		}
		argv, err := os.ReadFile(filepath.Join(stub, "argv"))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := strings.TrimSpace(string(argv)), strings.Join(args, " "); got != want {
			t.Fatalf("explicit interpolation scope changed: got %q, want %q", got, want)
		}
		if _, err := ComposeOutput(context.Background(), root, args[1:]...); err != nil {
			t.Fatal(err)
		}
		argv, err = os.ReadFile(filepath.Join(stub, "argv"))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := strings.TrimSpace(string(argv)), strings.Join(args, " "); got != want {
			t.Fatalf("captured interpolation scope changed: got %q, want %q", got, want)
		}
	}
}

func TestProjectStreamChildEnvFileDoesNotReplaceProjectScope(t *testing.T) {
	root, calls := dockertest.ComposeProject(t, "")
	args := []string{"compose", "exec", "api", "program", "--env-file", "child.env"}
	if err := Stream(context.Background(), root, args, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := ComposeOutput(context.Background(), root, args[1:]...); err != nil {
		t.Fatal(err)
	}
	if len(calls()) != 2 {
		t.Fatal("expected captured and streamed calls")
	}
}
