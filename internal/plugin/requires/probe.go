package requires

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
)

// dockerProbe asks the project's postgres container through the docker
// funnel. psql connects over the container's local socket as the configured
// user, so no password is passed in argv or the environment.
type dockerProbe struct{ container, user, db string }

func newDockerProbe(cfg *config.Config) Probe {
	p := &dockerProbe{container: cfg.ProjectName + "_postgres", user: cfg.Postgres.User, db: cfg.Postgres.DB}
	if p.user == "" {
		p.user = "postgres"
	}
	if p.db == "" {
		p.db = "nself"
	}
	return p
}

// Running is true only for a container in state "running". A missing
// container is "not running"; any other docker failure is an error.
func (p *dockerProbe) Running(ctx context.Context) (bool, error) {
	info, err := docker.InspectContainer(ctx, p.container)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return info.State == "running", nil
}

// Available lists pg_available_extensions (installed ones are included).
func (p *dockerProbe) Available(ctx context.Context) (map[string]bool, error) {
	out, _, err := docker.ExecCapture(ctx, p.container, []string{"psql", "-X", "-At", "-v", "ON_ERROR_STOP=1",
		"-U", p.user, "-d", p.db, "-c", "SELECT name FROM pg_available_extensions"})
	if err != nil {
		return nil, fmt.Errorf("querying pg_available_extensions in %s: %w", p.container, err)
	}
	have := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			have[l] = true
		}
	}
	return have, nil
}

// DockerProbe returns the default probe for cfg; the integration test uses it
// against a container it started.
func DockerProbe(cfg *config.Config) Probe { return newDockerProbe(cfg) }
