package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// buildNself builds ./cmd/nself of the module in srcDir into a fresh temp dir
// and returns the binary path and a cleanup func. The build flags match
// perf.yml's size gate so the measured binary is the shipped shape.
func buildNself(ctx context.Context, srcDir string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "perfbench-bin-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	bin := filepath.Join(dir, "nself")
	cmd := exec.CommandContext(ctx, "go", "build", "-mod=vendor", "-trimpath", "-ldflags=-s -w", "-o", bin, "./cmd/nself")
	cmd.Dir = srcDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("go build ./cmd/nself in %s: %v\n%s", srcDir, err, out)
	}
	return bin, cleanup, nil
}
