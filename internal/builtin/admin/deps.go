package admin

import (
	"context"
	"os/exec"

	"github.com/nself-org/cli/internal/config"
	"github.com/spf13/cobra"
)

const Slug = "admin"

// Deps carries the command package helpers used by the admin family.
type Deps struct {
	LoadHealthConfig  func() (*config.Config, string, error)
	OpenBrowserCmd    func(context.Context, string) *exec.Cmd
	ResolveEnvFile    func(string) (string, error)
	SetEnvKeyInFile   func(string, string, string) error
	ShouldOpenBrowser func() bool
}

type runner struct{ d Deps }

// Command builds a fresh admin subtree for a runtime or generator mount.
func Command(d Deps) *cobra.Command {
	r := runner{d: d}
	root := rootCommand(r)
	root.AddCommand(startCommand(r), stopCommand(r), logsCommand(r), healthCommand(r), connectCommand(), projectsCommand())
	return root
}
