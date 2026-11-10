// Package main implements the regen tool that runs every generator in order.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// GeneratorOrder lists the generators in the required execution order:
// canongen, then cmd-inventory steps (cmdinventory, openapigen, facts),
// then wikigen -report, parity, schemagen, then core-services.
var GeneratorOrder = []string{
	"canongen",
	"cmdinventory",
	"openapigen",
	"facts",
	"wikigen-report",
	"parity",
	"schemagen",
	"core-services",
}

// Step represents a single generator step.
type Step struct {
	Name string
	Run  func(tmpDir string) error
}

func defaultSteps() []Step {
	return []Step{
		{Name: "canongen", Run: buildAndExec("canongen", nil)},
		{Name: "cmdinventory", Run: execBashScript("scripts/sport/generate-f02.sh")},
		{Name: "openapigen", Run: noopStep()},
		{Name: "facts", Run: noopStep()},
		{Name: "wikigen-report", Run: buildAndExec("wikigen", []string{"-report"})},
		{Name: "parity", Run: buildAndExec("parity", nil)},
		{Name: "schemagen", Run: buildAndExec("schemagen", nil)},
		{Name: "core-services", Run: execBashScript("scripts/sport/generate-core-services.sh")},
	}
}

func noopStep() func(string) error {
	return func(tmpDir string) error { return nil }
}

func buildAndExec(toolName string, args []string) func(string) error {
	return func(tmpDir string) error {
		binPath := filepath.Join(tmpDir, toolName)
		buildCmd := exec.Command("go", "build", "-mod=vendor", "-o", binPath, "./tools/"+toolName)
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("build %s: %w", toolName, err)
		}
		runCmd := exec.Command(binPath, args...)
		runCmd.Stdout = os.Stdout
		runCmd.Stderr = os.Stderr
		runCmd.Env = append(os.Environ(), "NSELF_SPORT_DIR=")
		return runCmd.Run()
	}
}

func execBashScript(scriptPath string) func(string) error {
	return func(tmpDir string) error {
		absScript, err := filepath.Abs(scriptPath)
		if err != nil {
			return err
		}
		cmd := exec.Command("/bin/bash", absScript)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// The scripts call go: keep the running toolchain (GOROOT/bin) on the narrowed PATH.
		safePath := filepath.Join(runtime.GOROOT(), "bin") + ":/usr/bin:/bin:" + tmpDir
		env := os.Environ()
		for i, e := range env {
			if len(e) > 5 && e[:5] == "PATH=" {
				env[i] = "PATH=" + safePath
			}
			if len(e) > 16 && e[:16] == "NSELF_SPORT_DIR=" {
				env[i] = "NSELF_SPORT_DIR="
			}
		}
		cmd.Env = env
		return cmd.Run()
	}
}

// Run executes all steps in order. Exported for testing.
func Run(steps []Step) error {
	tmpDir, err := os.MkdirTemp("", "regen-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	for _, s := range steps {
		if err := s.Run(tmpDir); err != nil {
			return fmt.Errorf("generator %q failed: %w", s.Name, err)
		}
	}

	checkCmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	if err := checkCmd.Run(); err == nil {
		cmd := exec.Command("git", "status", "--porcelain")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git status: %w", err)
		}
	}
	return nil
}

func main() {
	if err := Run(defaultSteps()); err != nil {
		fmt.Fprintf(os.Stderr, "regen: %v\n", err)
		os.Exit(1)
	}
}
