// Package testkit provides isolated source fixtures and target projects.
// Purpose: shared source-ticket support. Inputs: compose fixture or fresh directory.
// Outputs: started fixtures and targets. Constraints: no daemon restart or production access.
package testkit

import (
	"context"
	"github.com/nself-org/cli/internal/docker"
	"os/exec"
	"path/filepath"
)

func FixtureUp(ctx context.Context, composeFile string) error {
	return docker.NewCompose(composeFile).ComposeUp(ctx, filepath.Dir(composeFile))
}
func Down(ctx context.Context, composeFile string) error {
	return docker.NewCompose(composeFile).ComposeDown(ctx, filepath.Dir(composeFile), docker.DownOptions{RemoveVolumes: true})
}

// runNself runs the nself binary in dir; tests replace it so no shell shim or real binary is needed.
var runNself = func(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "nself", args...)
	cmd.Dir = dir
	return cmd.Run()
}

func StartTarget(ctx context.Context, dir string) error {
	for _, args := range [][]string{{"init", "--non-interactive"}, {"start"}} {
		if err := runNself(ctx, dir, args...); err != nil {
			return err
		}
	}
	return nil
}
