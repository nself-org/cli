package admin

import (
	"context"
	"os/exec"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestAdminCommandTree(t *testing.T) {
	root := Command(Deps{})
	for _, path := range [][]string{{"start"}, {"stop"}, {"logs"}, {"health"}, {"connect"}, {"projects"}, {"projects", "list"}, {"projects", "add"}, {"projects", "remove"}} {
		cmd, rest, err := root.Find(path)
		if err != nil || len(rest) != 0 || cmd == root {
			t.Fatalf("path %v: cmd=%v rest=%v err=%v", path, cmd, rest, err)
		}
	}
}

func TestAdminDeps(t *testing.T) {
	d := Deps{
		LoadHealthConfig:  func() (*config.Config, string, error) { return &config.Config{ProjectName: "fixture"}, "", nil },
		OpenBrowserCmd:    func(context.Context, string) *exec.Cmd { return nil },
		ResolveEnvFile:    func(string) (string, error) { return "", nil },
		SetEnvKeyInFile:   func(string, string, string) error { return nil },
		ShouldOpenBrowser: func() bool { return false },
	}
	r := runner{d: d}
	if got := r.adminContainerID(); got != "fixture_admin" {
		t.Fatalf("container: %s", got)
	}
	if Command(d) == nil {
		t.Fatal("nil command")
	}
}
