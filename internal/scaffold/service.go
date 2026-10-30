// Package scaffold provides CS_N custom service scaffolding for nself projects.
// It handles slot assignment, template rendering, and .env.dev updates.
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// supportedLangs is the ordered list of language templates available for
// custom service scaffolding.
var supportedLangs = []string{"go", "node", "python", "static", "rust", "other"}

// IsValidLang reports whether lang is a supported template language.
func IsValidLang(lang string) bool {
	for _, l := range supportedLangs {
		if l == lang {
			return true
		}
	}
	return false
}

// SupportedLangs returns the list of supported language template names.
func SupportedLangs() []string {
	cp := make([]string, len(supportedLangs))
	copy(cp, supportedLangs)
	return cp
}

// Options configures a custom service scaffold run.
type Options struct {
	// ImageRef resolves Dockerfile base images from the caller's image lock.
	// Required for runs that write files.
	ImageRef func(name, tag string) string
	// Name is the service name (e.g. "my-api"). Validated against customServiceNameRe.
	Name string
	// Lang is one of: go, node, python, rust, other. Default "go".
	Lang string
	// ProjectDir is the nSelf project root. Defaults to the current working directory.
	ProjectDir string
	// Force allows overwriting an existing service directory.
	Force bool
	// DryRun prints what would be done without writing any files.
	DryRun bool
}

// Result describes what was emitted.
type Result struct {
	// Slot is the assigned CS_N slot number (1-10).
	Slot int
	// EnvKey is the env var name that was written (e.g. "CS_3").
	EnvKey string
	// EnvValue is the value that was written (e.g. "my-api:go:8003").
	EnvValue string
	// ServiceDir is the directory that was created.
	ServiceDir string
	// EnvFile is the .env file that was updated.
	EnvFile string
	// Files lists the relative (to ServiceDir) paths of emitted files.
	Files []string
}

// Run scaffolds a custom service into the nSelf project at opts.ProjectDir.
// It finds the next free CS_N slot (1-10), creates the service directory,
// and writes CS_N=<name>:<lang>:<port> into .env.dev.
func Run(opts Options) (*Result, error) {
	if opts.Lang == "" {
		opts.Lang = "go"
	}
	if !IsValidLang(opts.Lang) {
		return nil, fmt.Errorf("unsupported language %q; choose one of: %s",
			opts.Lang, strings.Join(SupportedLangs(), ", "))
	}

	projectDir := opts.ProjectDir
	if projectDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting working directory: %w", err)
		}
		projectDir = cwd
	}

	// Find the next free CS_N slot.
	slot, err := nextFreeSlot(projectDir)
	if err != nil {
		return nil, err
	}

	// Determine the port: 8000 + N auto-assignment (matching config.ParseCustomServices).
	port := 8000 + slot

	envKey := fmt.Sprintf("CS_%d", slot)
	// Format: name:template[:port] — matches config.ParseCustomServices expectations.
	envValue := fmt.Sprintf("%s:%s:%d", opts.Name, opts.Lang, port)

	envFile := filepath.Join(projectDir, ".env.dev")

	serviceDir := filepath.Join(projectDir, "services", opts.Name)

	if opts.DryRun {
		// Report what would happen without writing.
		return &Result{
			Slot:       slot,
			EnvKey:     envKey,
			EnvValue:   envValue,
			ServiceDir: serviceDir,
			EnvFile:    envFile,
			Files:      scaffoldFileList(opts.Name, opts.Lang),
		}, nil
	}
	if opts.ImageRef == nil {
		return nil, fmt.Errorf("image resolver is required to scaffold service files")
	}

	// Create service directory.
	if !opts.Force {
		if entries, readErr := os.ReadDir(serviceDir); readErr == nil && len(entries) > 0 {
			return nil, fmt.Errorf("service directory %s already exists and is not empty (use --force to overwrite)", serviceDir)
		}
	}
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return nil, fmt.Errorf("creating service directory: %w", err)
	}

	// Write scaffold files.
	files, err := writeScaffold(serviceDir, opts.Name, opts.Lang, port, opts.ImageRef)
	if err != nil {
		return nil, fmt.Errorf("writing scaffold files: %w", err)
	}

	// Update .env.dev.
	if err := setEnvKey(envFile, envKey, envValue); err != nil {
		return nil, fmt.Errorf("updating %s: %w", envFile, err)
	}
	// Also write port env var: <UPPER_NAME>_PORT=<port>
	portKey := fmt.Sprintf("%s_PORT", strings.ToUpper(strings.ReplaceAll(opts.Name, "-", "_")))
	if err := setEnvKey(envFile, portKey, fmt.Sprintf("%d", port)); err != nil {
		return nil, fmt.Errorf("updating %s for port: %w", envFile, err)
	}

	return &Result{
		Slot:       slot,
		EnvKey:     envKey,
		EnvValue:   envValue,
		ServiceDir: serviceDir,
		EnvFile:    envFile,
		Files:      files,
	}, nil
}

// NextCSSlot returns the next CS_N slot number (1-10) not already set in .env.dev.
// Returns an error if all 10 slots are in use.
// This is the exported variant of the internal nextFreeSlot helper (G-006).
func NextCSSlot(projectRoot string) (int, error) {
	return nextFreeSlot(projectRoot)
}

// nextFreeSlot returns the next CS_N slot (1-10) not already set in .env.dev.
// Returns an error if all 10 slots are in use.
func nextFreeSlot(projectDir string) (int, error) {
	envFile := filepath.Join(projectDir, ".env.dev")
	values := readEnvFile(envFile)

	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("CS_%d", i)
		if _, exists := values[key]; !exists || values[key] == "" {
			return i, nil
		}
	}
	return 0, fmt.Errorf("all 10 CS_N slots are in use; remove an existing custom service to add a new one")
}

// readEnvFile reads KEY=VALUE pairs from an env file.
// Returns an empty map if the file does not exist.
func readEnvFile(filename string) map[string]string {
	out := make(map[string]string)
	data, err := os.ReadFile(filename)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		out[line[:idx]] = line[idx+1:]
	}
	return out
}

// setEnvKey reads filename, replaces or appends KEY=value, and writes it back.
// Safe if the file does not exist yet.
func setEnvKey(filename, key, value string) error {
	var lines []string
	if data, err := os.ReadFile(filename); err == nil {
		lines = strings.Split(string(data), "\n")
		// Remove trailing empty element from Split on trailing newline.
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}

	prefix := key + "="
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			lines[i] = key + "=" + value
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, key+"="+value)
	}

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
		return fmt.Errorf("writing %s: %w", filename, err)
	}
	return nil
}

// scaffoldFileList returns the list of files that would be scaffolded.
func scaffoldFileList(name, lang string) []string {
	base := []string{"README.md", "Dockerfile", ".dockerignore"}
	switch lang {
	case "go":
		return append(base, "go.mod", "main.go")
	case "node":
		return append(base, "package.json", "src/index.ts", "tsconfig.json")
	case "python":
		return append(base, "requirements.txt", "main.py")
	case "static":
		return append(base, "index.html", "nginx.conf")
	case "rust":
		return append(base, "Cargo.toml", "src/main.rs")
	default:
		return append(base, "main.sh")
	}
}
