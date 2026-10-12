package stack

// Purpose: the non-production rollback adapters: image preflight, stopping the
// new-image app services, and restarting the pinned snapshot.
// Inputs: the journal's pinned snapshot and the project Compose manifest.
// Outputs: errors that stop the rollback before data is touched when images are gone.
// Constraints: never `down -v`, never a volume or backup removal; postgres stays up.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/reconcile"
)

// Docker seams, defaulting to the shared funnel, let tests observe rollback order.
var (
	runtimeImageDigests = docker.ImageRepoDigests
	runtimeServiceSpecs = docker.ComposeServiceSpecs
)

// databaseService is the one service that stays up during a rollback restore.
const databaseService = "postgres"

// refuseProd stops automatic recovery for production-class environments.
func (r *Runtime) refuseProd(j *Journal, what string) error {
	if j.Prod || reconcile.EnvClass(r.cfg.Env) == reconcile.ClassProd {
		return fmt.Errorf("automatic %s is forbidden on production-class environments", what)
	}
	return nil
}

// CheckPreviousImages proves every pinned previous image still exists locally.
// Inputs: journal snapshot. Outputs: error naming the first missing image.
// Constraints: read only (`docker image inspect`); runs before anything is stopped.
func (r *Runtime) CheckPreviousImages(ctx context.Context, j *Journal) error {
	if err := r.refuseProd(j, "image check"); err != nil {
		return err
	}
	if err := r.verifySnapshot(j); err != nil {
		return err
	}
	data, err := os.ReadFile(j.PreviousCompose)
	if err != nil {
		return fmt.Errorf("read recovery snapshot: %w", err)
	}
	var doc struct {
		Services map[string]struct {
			Image string `json:"image"`
		} `json:"services"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("decode recovery snapshot: %w", err)
	}
	names := make([]string, 0, len(doc.Services))
	for name := range doc.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, err := runtimeImageDigests(ctx, doc.Services[name].Image); err != nil {
			if errors.Is(err, docker.ErrImageNotFound) {
				return fmt.Errorf("service %s: pinned image %s is missing: %w", name, doc.Services[name].Image, err)
			}
			return fmt.Errorf("service %s: cannot check pinned image %s: %w", name, doc.Services[name].Image, err)
		}
	}
	return nil
}

// StopServices stops every current app service so no new-image process writes during restore.
// Inputs: the current project Compose manifest. Outputs: stop error.
// Constraints: `compose stop` only; postgres stays up, no container or volume is removed.
func (r *Runtime) StopServices(ctx context.Context, j *Journal) error {
	if err := r.refuseProd(j, "service stop"); err != nil {
		return err
	}
	c, err := r.compose()
	if err != nil {
		return err
	}
	specs, err := runtimeServiceSpecs(ctx, c, r.dir)
	if err != nil {
		return fmt.Errorf("list services to stop: %w", err)
	}
	var names []string
	for _, spec := range specs {
		if spec.Name != databaseService {
			names = append(names, spec.Name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	if err := c.ComposeStop(ctx, r.dir, names...); err != nil {
		return fmt.Errorf("stop new-image services: %w", err)
	}
	return nil
}

// RestartPrevious starts only the immutable snapshot and removes services it lacks.
// Inputs: verified snapshot. Outputs: restart or health error.
// Constraints: `up -d --remove-orphans` removes containers, never volumes.
func (r *Runtime) RestartPrevious(ctx context.Context, j *Journal) error {
	if err := r.refuseProd(j, "restart recovery"); err != nil {
		return err
	}
	if err := r.verifySnapshot(j); err != nil {
		return err
	}
	c := docker.NewCompose(j.PreviousCompose)
	if err := c.Run(ctx, r.dir, "compose", "-f", j.PreviousCompose, "up", "-d", "--remove-orphans"); err != nil {
		return fmt.Errorf("restart previous images: %w", err)
	}
	return r.healthCompose(ctx, c)
}
