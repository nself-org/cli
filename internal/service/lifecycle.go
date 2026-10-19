// Package service provides lifecycle management for individual nSelf optional
// services (start, stop, restart, ps, update, scale). All operations delegate
// to `docker compose` via the internal/docker package — no raw docker invocations.
package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"gopkg.in/yaml.v3"
)

// knownServiceNames is the set of valid optional service names accepted by the
// lifecycle commands. It mirrors cmd/commands/service.go knownServices plus the
// four required core services that can be individually restarted.
var knownServiceNames = map[string]bool{
	// Required services (always present in the stack).
	"postgres": true,
	"hasura":   true,
	"auth":     true,
	"nginx":    true,
	// Optional services.
	"redis":      true,
	"minio":      true,
	"email":      true,
	"mailpit":    true,
	"functions":  true,
	"search":     true,
	"monitoring": true,
	"admin":      true,
	// Aliases resolved before the lookup.
	"storage":     true,
	"mail":        true,
	"meilisearch": true,
}

// serviceAlias maps alternate names to the canonical Docker Compose service name.
var serviceAlias = map[string]string{
	"mail":        "email",
	"mailpit":     "email",
	"meilisearch": "search",
}

// ComposeDeclaresService reports whether the project's docker-compose.yml in
// dir declares a service called name. An unreadable or unparsable file reports
// false, so the compose call that follows surfaces the real error.
func ComposeDeclaresService(dir, name string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml"))
	if err != nil {
		return false
	}
	var doc struct {
		Services map[string]yaml.Node `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return false
	}
	_, ok := doc.Services[name]
	return ok
}

// resolveService validates and resolves the service name to its canonical
// Docker Compose service label. Returns an error for unknown names. The name
// "storage" reaches the free storage plugin when the compose file in dir
// declares that service, and falls back to core MinIO otherwise (D-0451).
func resolveService(name, dir string) (string, error) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "storage" {
		if ComposeDeclaresService(dir, "storage") {
			return "storage", nil
		}
		lower = "minio"
	}
	if canon, ok := serviceAlias[lower]; ok {
		lower = canon
	}
	if !knownServiceNames[lower] {
		return "", fmt.Errorf("unknown service %q\nValid services: postgres, hasura, auth, nginx, redis, minio, email, functions, search, monitoring, admin", name)
	}
	return lower, nil
}

// projectDir resolves the nSelf project root (directory containing docker-compose.yml)
// starting from the current working directory.
func projectDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	dir, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return "", err
	}
	composePath := filepath.Join(dir, "docker-compose.yml")
	if _, err := os.Stat(composePath); err != nil {
		return "", fmt.Errorf("docker-compose.yml not found in %s\nRun `nself build` first", dir)
	}
	return dir, nil
}

// composeFor builds the Compose for the project in dir the way `nself start`
// and `nself restart` do: every file in .nself/compose-files.txt as -f (plugin
// fragments included) and build.ComposeEnvFiles as --env-file, so ${VAR}
// references resolve from the project's env files (D-0703). A project built
// before the manifest existed gets docker-compose.yml alone.
func composeFor(dir string) (*docker.Compose, error) {
	files, err := build.ReadComposeManifest(dir)
	if err != nil {
		return nil, fmt.Errorf("reading compose manifest: %w", err)
	}
	for i, f := range files {
		if !filepath.IsAbs(f) {
			files[i] = filepath.Join(dir, f)
		}
	}
	c := docker.NewCompose(files...)
	c.EnvFiles = build.ComposeEnvFiles(dir)
	return c, nil
}

// Start starts a named nSelf service via `docker compose up -d --no-deps <service>`.
// The service must be configured in the stack (docker-compose.yml must exist).
func Start(ctx context.Context, serviceName string) error {
	dir, err := projectDir()
	if err != nil {
		return err
	}
	canon, err := resolveService(serviceName, dir)
	if err != nil {
		return err
	}
	c, err := composeFor(dir)
	if err != nil {
		return err
	}
	return c.ComposeUp(ctx, dir, canon)
}

// Stop stops a named nSelf service via `docker compose stop <service>`.
// The container is stopped but not removed; its state is preserved.
func Stop(ctx context.Context, serviceName string) error {
	dir, err := projectDir()
	if err != nil {
		return err
	}
	canon, err := resolveService(serviceName, dir)
	if err != nil {
		return err
	}
	c, err := composeFor(dir)
	if err != nil {
		return err
	}
	return c.ComposeStop(ctx, dir, canon)
}

// Restart restarts a named nSelf service via `docker compose restart <service>`.
func Restart(ctx context.Context, serviceName string) error {
	dir, err := projectDir()
	if err != nil {
		return err
	}
	canon, err := resolveService(serviceName, dir)
	if err != nil {
		return err
	}
	c, err := composeFor(dir)
	if err != nil {
		return err
	}
	return c.ComposeRestart(ctx, dir, canon)
}

// PSEntry represents a single service row in the `nself service ps` output.
type PSEntry struct {
	Name    string
	Service string
	Status  string
	Health  string
	ID      string
	Uptime  string
}

// PS returns the current status of all nSelf stack services.
func PS(ctx context.Context) ([]PSEntry, error) {
	dir, err := projectDir()
	if err != nil {
		return nil, err
	}
	c, err := composeFor(dir)
	if err != nil {
		return nil, err
	}
	containers, err := c.ComposePs(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("service ps: %w", err)
	}

	entries := make([]PSEntry, 0, len(containers))
	for _, ct := range containers {
		entries = append(entries, PSEntry{
			Name:    ct.Name,
			Service: ct.Service,
			Status:  ct.State,
			Health:  ct.Health,
			ID:      shortID(ct.ID),
			Uptime:  ct.State, // State already carries human-readable status from compose ps
		})
	}
	return entries, nil
}

// Update pulls the latest image for a service and restarts it.
// This is equivalent to `docker compose pull <service> && docker compose up -d --no-deps <service>`.
func Update(ctx context.Context, serviceName string) error {
	dir, err := projectDir()
	if err != nil {
		return err
	}
	canon, err := resolveService(serviceName, dir)
	if err != nil {
		return err
	}
	c, err := composeFor(dir)
	if err != nil {
		return err
	}

	// Pull the latest image for the specific service.
	pullCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := c.ComposePullService(pullCtx, dir, canon); err != nil {
		return fmt.Errorf("pulling image for %s: %w", canon, err)
	}

	// Restart the service with the new image (--no-deps = don't restart dependents).
	return c.ComposeUp(ctx, dir, canon)
}

// Scale adjusts the replica count for a named service via
// `docker compose up -d --scale <service>=<n> <service>`.
// Replicas must be >= 1.
func Scale(ctx context.Context, serviceName string, replicas int) error {
	if replicas < 1 {
		return fmt.Errorf("replicas must be at least 1 (got %d)", replicas)
	}
	dir, err := projectDir()
	if err != nil {
		return err
	}
	canon, err := resolveService(serviceName, dir)
	if err != nil {
		return err
	}
	c, err := composeFor(dir)
	if err != nil {
		return err
	}
	return c.ComposeScale(ctx, dir, canon, replicas)
}

// shortID returns the first 12 characters of a container ID, matching the
// format used by `docker ps` output.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
