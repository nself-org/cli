package service

// Purpose: keep individual service lifecycle calls on the build's compose inputs.
// Inputs: project root and ordered compose/env manifests written by nself build.
// Outputs: a Compose client, or a manifest read error before Docker runs.
// Constraints: legacy projects retain base/override and computed-env discovery;
// no internal/build import (service is L1, build is L2).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/docker"
)

func lifecycleCompose(dir string) (*docker.Compose, error) {
	files, err := lifecycleManifest(dir, "compose-files.txt")
	if err != nil {
		return nil, err
	}
	// Match build.ReadComposeManifest: uninstalled plugin fragments are stale.
	available := files[:0]
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			available = append(available, file)
		}
	}
	files = available
	if len(files) == 0 {
		files = []string{filepath.Join(dir, "docker-compose.yml")}
	}
	// Explicit -f disables Docker's automatic user override discovery.
	for _, rel := range []string{"docker-compose.override.yml", docker.ImageOverrideFile} {
		path := filepath.Join(dir, rel)
		if _, err := os.Stat(path); err == nil && !containsComposeFile(files, path) {
			files = append(files, path)
		}
	}
	envs, err := lifecycleManifest(dir, "compose-env-files.txt")
	if err != nil {
		return nil, err
	}
	if len(envs) == 0 {
		envs = docker.ProjectEnvFiles(dir)
	}
	c := docker.NewCompose(files...)
	c.EnvFiles = envs
	return c, nil
}

func lifecycleManifest(dir, name string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".nself", name))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading %s: %w", name, err)
	}
	var files []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !filepath.IsAbs(line) {
			line = filepath.Join(dir, line)
		}
		files = append(files, line)
	}
	return files, nil
}

func containsComposeFile(files []string, path string) bool {
	for _, file := range files {
		if filepath.Clean(file) == filepath.Clean(path) {
			return true
		}
	}
	return false
}
