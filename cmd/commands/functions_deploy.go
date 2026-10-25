package commands

// Purpose: `nself functions deploy` split out of functions.go (CLI-R12
// Batch B mechanical file-size split), plus its copyFile/copyDir helpers.
// Copies a function file or directory into ./functions/<name>/ and signals
// the functions container to reload.
// Inputs: cobra command flags (--name, --runtime, --env) and the
// positional file/dir arg.
// Outputs: a deployed function directory under ./functions/; stdout
// confirmation with the invoke URL.
// Constraints: pure move, no behavior change. functionNamePattern and
// functionsCmd (parent) remain in functions.go.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var functionsDeployFlags struct {
	name     string
	runtime  string
	envPairs []string
}

var functionsDeployCmd = &cobra.Command{
	Use:   "deploy <file|dir>",
	Short: "Deploy a function from a file or directory",
	Long: `Copy the given file or directory into ./functions/<name>/ and signal the
functions container to reload. The name defaults to the base filename (without
extension) unless --name is provided.

Examples:
  nself functions deploy hello-world.ts
  nself functions deploy ./my-fn/ --name my-fn
  nself functions deploy handler.py --runtime python`,
	Args: cobra.ExactArgs(1),
	RunE: runFunctionsDeploy,
}

func init() {
	functionsDeployCmd.Flags().StringVar(&functionsDeployFlags.name, "name", "", "Function name (defaults to filename without extension)")
	functionsDeployCmd.Flags().StringVar(&functionsDeployFlags.runtime, "runtime", "", "Override runtime: node, deno, python")
	functionsDeployCmd.Flags().StringArrayVar(&functionsDeployFlags.envPairs, "env", nil, "Environment variable KEY=VALUE (repeatable)")
}

func runFunctionsDeploy(cmd *cobra.Command, args []string) error {
	src := args[0]

	// Determine function name.
	name := functionsDeployFlags.name
	if name == "" {
		base := filepath.Base(src)
		// Strip extension for files.
		if ext := filepath.Ext(base); ext != "" {
			base = strings.TrimSuffix(base, ext)
		}
		name = base
	}

	// Validate name.
	if !functionNamePattern.MatchString(name) {
		return fmt.Errorf("invalid function name %q: use lowercase alphanumeric and hyphens only", name)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	destDir := filepath.Join(cwd, "functions", name)
	if err := os.MkdirAll(destDir, 0750); err != nil {
		return fmt.Errorf("creating function directory: %w", err)
	}

	// Copy source into dest.
	fi, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	if fi.IsDir() {
		if err := copyDir(src, destDir); err != nil {
			return fmt.Errorf("copying directory: %w", err)
		}
	} else {
		destFile := filepath.Join(destDir, filepath.Base(src))
		if err := copyFile(src, destFile); err != nil {
			return fmt.Errorf("copying file: %w", err)
		}
	}

	// Write .env vars into function dir if provided.
	if len(functionsDeployFlags.envPairs) > 0 {
		envFile := filepath.Join(destDir, ".env")
		var buf bytes.Buffer
		for _, pair := range functionsDeployFlags.envPairs {
			buf.WriteString(pair)
			buf.WriteByte('\n')
		}
		if err := os.WriteFile(envFile, buf.Bytes(), 0600); err != nil {
			return fmt.Errorf("writing function .env: %w", err)
		}
	}

	// Signal container reload.
	cfg, _, loadErr := loadHealthConfig()
	if loadErr == nil {
		containerID := cfg.ServiceContainerName("functions")
		reloadCtx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
		defer cancel()
		// nhost/functions watches for a .reload file touch.
		reloadCmd := exec.CommandContext(reloadCtx, "docker", "exec", containerID,
			"touch", "/opt/project/.reload")
		if out, err := reloadCmd.CombinedOutput(); err != nil {
			// Non-fatal — container may not be running yet.
			fmt.Fprintf(os.Stderr, "Warning: could not signal reload: %v (%s)\n", err, strings.TrimSpace(string(out)))
		}
	}

	fmt.Printf("Function %q deployed to ./functions/%s/\n", name, name)
	fmt.Printf("URL: http://localhost:3008/v1/%s\n", name)
	return nil
}

// copyFile copies a single file src → dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

// copyDir recursively copies src directory into dst directory.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0750)
		}
		return copyFile(path, target)
	})
}
