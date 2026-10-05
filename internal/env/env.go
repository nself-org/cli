// Package env implements multi-environment management for nSelf projects.
// Each environment (dev, staging, prod) has its own .env file, Docker project
// name, network, and volume namespace to enable side-by-side operation.
package env

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// stateDir is the directory within .nself/ that stores env state.
const stateDir = ".nself"

// stateFile is the file that records the active environment.
const stateFile = "active_env"

// DefaultEnv is the environment used when none is explicitly set.
const DefaultEnv = "dev"

// knownEnvs are the standard environment names.
var knownEnvs = []string{"dev", "staging", "prod"}

// ActiveEnv reads the currently active environment from .nself/active_env.
// Returns DefaultEnv if the file does not exist.
func ActiveEnv(projectDir string) string {
	data, err := os.ReadFile(filepath.Join(projectDir, stateDir, stateFile))
	if err != nil {
		return DefaultEnv
	}
	env := strings.TrimSpace(string(data))
	if env == "" {
		return DefaultEnv
	}
	return env
}

// SetActiveEnv writes the active environment to .nself/active_env.
func SetActiveEnv(projectDir, env string) error {
	dir := filepath.Join(projectDir, stateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, stateFile), []byte(env+"\n"), 0o600)
}

// List returns all available environments for a project by scanning for
// .env.{name} files plus the active env.
func List(projectDir string) []EnvInfo {
	active := ActiveEnv(projectDir)
	seen := make(map[string]bool)
	var envs []EnvInfo

	// Always include known envs that have .env files.
	for _, name := range knownEnvs {
		envFile := filepath.Join(projectDir, fmt.Sprintf(".env.%s", name))
		if _, err := os.Stat(envFile); err == nil {
			seen[name] = true
			envs = append(envs, EnvInfo{
				Name:   name,
				Active: name == active,
				File:   envFile,
			})
		}
	}

	// Scan for additional .env.{name} files.
	entries, _ := os.ReadDir(projectDir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".env.") {
			name := strings.TrimPrefix(e.Name(), ".env.")
			// Skip known suffixes that aren't environments.
			if name == "example" || name == "local" || name == "secrets" || name == "computed" || name == "ai" {
				continue
			}
			if !seen[name] {
				seen[name] = true
				envs = append(envs, EnvInfo{
					Name:   name,
					Active: name == active,
					File:   filepath.Join(projectDir, e.Name()),
				})
			}
		}
	}

	// Ensure the active environment is always included, even if no .env file exists yet.
	if !seen[active] {
		envs = append(envs, EnvInfo{
			Name:   active,
			Active: true,
			File:   filepath.Join(projectDir, fmt.Sprintf(".env.%s", active)),
		})
	}

	sort.Slice(envs, func(i, j int) bool {
		return envs[i].Name < envs[j].Name
	})
	return envs
}

// EnvInfo describes an available environment.
type EnvInfo struct {
	Name   string
	Active bool
	File   string
}

// DockerProjectName returns the Docker Compose project name for a given
// project and environment: nself-{project}-{env}.
func DockerProjectName(projectName, env string) string {
	return fmt.Sprintf("nself-%s-%s", projectName, env)
}

// Show returns key-value pairs for the given environment with secrets redacted.
func Show(projectDir, env string) ([]KeyValue, error) {
	envFile := filepath.Join(projectDir, fmt.Sprintf(".env.%s", env))
	data, err := os.ReadFile(envFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", envFile, err)
	}

	var pairs []KeyValue
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		pairs = append(pairs, KeyValue{
			Key:   key,
			Value: redactIfSecret(key, val),
		})
	}
	return pairs, nil
}

// KeyValue is a key-value pair from an env file.
type KeyValue struct {
	Key   string
	Value string
}

// Diff compares two environment configs and returns differences.
func Diff(projectDir, envA, envB string) ([]DiffEntry, error) {
	pairsA, err := loadEnvMap(projectDir, envA)
	if err != nil {
		return nil, err
	}
	pairsB, err := loadEnvMap(projectDir, envB)
	if err != nil {
		return nil, err
	}

	allKeys := make(map[string]bool)
	for k := range pairsA {
		allKeys[k] = true
	}
	for k := range pairsB {
		allKeys[k] = true
	}

	var keys []string
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var diffs []DiffEntry
	for _, k := range keys {
		valA := pairsA[k]
		valB := pairsB[k]
		if valA != valB {
			diffs = append(diffs, DiffEntry{
				Key:    k,
				ValueA: redactIfSecret(k, valA),
				ValueB: redactIfSecret(k, valB),
			})
		}
	}
	return diffs, nil
}

// DiffEntry describes a difference between two environments.
type DiffEntry struct {
	Key    string
	ValueA string // value in env A (empty = not present)
	ValueB string // value in env B (empty = not present)
}

// Copy scaffolds a new environment by copying an existing one.
func Copy(projectDir, fromEnv, toEnv string) error {
	src := filepath.Join(projectDir, fmt.Sprintf(".env.%s", fromEnv))
	dst := filepath.Join(projectDir, fmt.Sprintf(".env.%s", toEnv))

	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf(".env.%s already exists", toEnv)
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read .env.%s: %w", fromEnv, err)
	}

	return os.WriteFile(dst, data, 0o600)
}

// loadEnvMap reads an .env.{name} file into a map.
func loadEnvMap(projectDir, env string) (map[string]string, error) {
	envFile := filepath.Join(projectDir, fmt.Sprintf(".env.%s", env))
	data, err := os.ReadFile(envFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", envFile, err)
	}

	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result, nil
}

// secretPatterns are key substrings that indicate a value should be redacted.
var secretPatterns = []string{
	"PASSWORD", "SECRET", "KEY", "TOKEN", "CREDENTIAL",
}

// redactIfSecret replaces the value with "****" if the key looks like a secret.
func redactIfSecret(key, val string) string {
	if val == "" {
		return val
	}
	upper := strings.ToUpper(key)
	for _, pat := range secretPatterns {
		if strings.Contains(upper, pat) {
			return "****"
		}
	}
	return val
}
