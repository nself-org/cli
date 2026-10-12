package stack

// Purpose: prove boot migration readiness and every declared service's health.
// Inputs: local manifests and Docker service state. Outputs: bounded check errors.
// Constraints: the CLI never applies plugin SQL or silently ignores missing ports.

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
	"github.com/nself-org/cli/internal/plugin/readiness"
)

// health gates every service, including ones with Docker healthchecks and one-shots.
func (r *Runtime) health(ctx context.Context) error {
	c, err := r.compose()
	if err != nil {
		return err
	}
	return r.healthCompose(ctx, c)
}

// healthCompose checks the supplied specification against actual container state.
func (r *Runtime) healthCompose(ctx context.Context, c *docker.Compose) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	specs, err := docker.ComposeServiceSpecs(ctx, c, r.dir)
	if err != nil {
		return fmt.Errorf("read health specifications: %w", err)
	}
	if len(specs) == 0 {
		return fmt.Errorf("stack has no declared health services")
	}
	for _, spec := range specs {
		state := func(ctx context.Context) ([]docker.ContainerState, error) {
			return c.ComposeServiceStates(ctx, r.dir, spec.Name)
		}
		if err := docker.HealthGate(ctx, spec, state, nil); err != nil {
			return fmt.Errorf("service %s health: %w", spec.Name, err)
		}
	}
	return nil
}

// readiness checks active installed migration-bearing plugins through their host ports.
func (r *Runtime) readiness(ctx context.Context) error {
	timeout, err := readiness.TimeoutFromEnv()
	if err != nil {
		return fmt.Errorf("plugin readiness timeout: %w", err)
	}
	dir, entries, err := r.pluginEntries()
	if err != nil {
		return err
	}
	if entries == nil {
		return nil
	}
	c, err := r.compose()
	if err != nil {
		return err
	}
	specs, err := docker.ComposeServiceSpecs(ctx, c, r.dir)
	if err != nil {
		return fmt.Errorf("resolve active plugin services: %w", err)
	}
	active := map[string]bool{}
	for _, spec := range specs {
		active[spec.Name] = true
	}
	containers, err := c.ComposePs(ctx, r.dir)
	if err != nil {
		return fmt.Errorf("resolve plugin containers: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() || (!active[e.Name()] && !active["plugin_"+e.Name()]) {
			continue
		}
		m, err := loadReadinessManifest(dir, e.Name())
		if err != nil {
			return err
		}
		if m == nil || m.Migrations == nil {
			continue
		}
		if err := waitPluginReadiness(ctx, m, containers, timeout); err != nil {
			return err
		}
	}
	return nil
}

// pluginEntries resolves the configured installed-plugin directory.
// Inputs: project configuration. Outputs: path and entries. Constraints: no writes.
func (r *Runtime) pluginEntries() (string, []os.DirEntry, error) {
	dir := r.cfg.PluginSystem.Dir
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", nil, fmt.Errorf("resolve plugin home: %w", err)
		}
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~/"))
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(r.dir, dir)
	}
	// Stat first: Windows reports reading a file as a directory as not-exist.
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return dir, nil, nil
	}
	if err == nil && !info.IsDir() {
		return "", nil, fmt.Errorf("read installed plugins: %s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, fmt.Errorf("read installed plugins: %w", err)
	}
	return dir, entries, nil
}

// loadReadinessManifest reads one active plugin's optional manifest.
// Inputs: installed directory and slug. Outputs: manifest. Constraints: invalid manifests fail.
func loadReadinessManifest(dir, name string) (*manifestv2.Manifest, error) {
	p := filepath.Join(dir, name, "plugin.json")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("inspect plugin manifest: %w", err)
	}
	m, err := manifestv2.Load(p)
	if err != nil {
		return nil, fmt.Errorf("load installed plugin %s: %w", name, err)
	}
	return m, nil
}

// waitPluginReadiness checks the active plugin's boot migration endpoint.
// Inputs: manifest, containers and timeout. Outputs: ready/error. Constraints: bounded HTTP.
func waitPluginReadiness(ctx context.Context, m *manifestv2.Manifest, containers []docker.ContainerInfo, timeout time.Duration) error {
	if m.Migrations.Apply != "boot" {
		return fmt.Errorf("plugin %s migrations must apply at boot", m.Name)
	}
	port := 0
	for _, container := range containers {
		service := container.Service
		if service == "" {
			service = container.Labels["com.docker.compose.service"]
		}
		if service != m.Name && service != "plugin_"+m.Name {
			continue
		}
		var err error
		port, err = pluginHostPort(container.Ports, m)
		if err != nil {
			return err
		}
		break
	}
	if port == 0 {
		return fmt.Errorf("plugin %s has no published health port", m.Name)
	}
	url := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/health"
	if err := readiness.Wait(ctx, m.Name, url, timeout); err != nil {
		return fmt.Errorf("plugin %s readiness: %w", m.Name, err)
	}
	return nil
}

// pluginHostPort selects only the published mapping for the manifest service port.
func pluginHostPort(ports []string, m *manifestv2.Manifest) (int, error) {
	if m.Service == nil || m.Service.Port == nil {
		return 0, fmt.Errorf("plugin %s has no declared health port", m.Name)
	}
	target := strconv.Itoa(*m.Service.Port)
	for _, p := range ports {
		halves := strings.Split(p, "->")
		if len(halves) != 2 || strings.Split(halves[1], "/")[0] != target {
			continue
		}
		// Docker's inspection formatter emits IPv6 addresses without brackets.
		colon := strings.LastIndex(halves[0], ":")
		if colon < 0 {
			return 0, fmt.Errorf("plugin %s published port is malformed", m.Name)
		}
		v := halves[0][colon+1:]
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			return 0, fmt.Errorf("plugin %s has invalid published port", m.Name)
		}
		return n, nil
	}
	return 0, fmt.Errorf("plugin %s health port is not published", m.Name)
}
