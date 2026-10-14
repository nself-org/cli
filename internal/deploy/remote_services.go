package deploy

// Purpose: the container-touching tail of a remote apply (D24 step 9): bring each
//          service up in compose order behind the P7-DEPL-16 health gate, re-read
//          them once, reload a running nginx, apply hasura metadata.
// Inputs:  ApplyOptions and the resolved ComposeModel.
// Outputs: errors classed like the local rolling restart (E250/E251).
// Constraints: every service name follows `--` in the remote argv (a name can
//              never be read as a flag). The nginx reload runs only after the
//              throwaway nginx -t passed (apply) or the restored config was the
//              live one (rollback).

import (
	"context"
	"errors"
	"fmt"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

// applyServices brings each service up in compose order and gates it, then
// re-reads every gated service once. Callers reload nginx afterwards.
func applyServices(ctx context.Context, o ApplyOptions, model ComposeModel) error {
	r, rp, files, envs := o.Remote, o.RemotePath, o.Set.Compose, o.Set.EnvManifest
	type gated struct {
		name string
		seen []docker.ContainerState
	}
	var done []gated
	for _, svc := range model.Order {
		spec, _ := model.Specs(svc)
		marks, err := RemoteStartMarks(ctx, r, rp, files, envs, svc)
		if err != nil {
			return gateError(svc, fmt.Errorf("%w: %w", docker.ErrStateRead, err))
		}
		emit(o.Out, "  [running] Restart %s (sequenced rolling)", svc)
		up := fmt.Sprintf("cd %s && %s up -d --no-deps -- %s", q(rp), composeBase(files, envs), q(svc))
		if o.rollbackImages != nil {
			up = fmt.Sprintf("cd %s && %s up -d --no-deps --pull never --no-build -- %s", q(rp), composeBase(files, envs), q(svc))
		}
		if _, err := r.Run(ctx, shScript(up)); err != nil {
			return fmt.Errorf("remote restart of %s failed: %w\nRun 'nself logs %s' on the host for details", svc, err, svc)
		}
		var seen []docker.ContainerState
		states := func(c context.Context) ([]docker.ContainerState, error) {
			cs, err := RemoteStates(c, r, rp, files, envs, svc, marks)
			if err == nil {
				seen = cs
			}
			return cs, err
		}
		if err := docker.HealthGate(ctx, spec, states, o.Clock); err != nil {
			return gateError(svc, err)
		}
		done = append(done, gated{svc, seen})
	}
	for _, g := range done {
		cs, err := RemoteStates(ctx, r, rp, files, envs, g.name, nil)
		if err == nil {
			err = docker.Regressed(g.seen, cs)
		} else {
			err = fmt.Errorf("%w: %w", docker.ErrStateRead, err)
		}
		if err != nil {
			return gateError(g.name, err)
		}
	}
	return nil
}

// reloadNginx reloads a running nginx service so it reads the config now on disk.
func reloadNginx(ctx context.Context, o ApplyOptions, model ComposeModel) error {
	if _, ok := model.Services[NginxServiceName]; !ok {
		return nil
	}
	r, rp, files, envs := o.Remote, o.RemotePath, o.Set.Compose, o.Set.EnvManifest
	cs, err := RemoteStates(ctx, r, rp, files, envs, NginxServiceName, nil)
	if err != nil {
		return gateError(NginxServiceName, fmt.Errorf("%w: %w", docker.ErrStateRead, err))
	}
	running := false
	for _, c := range cs {
		running = running || c.State == "running"
	}
	if !running {
		return nil
	}
	emit(o.Out, "  [running] nginx -s reload")
	cmd := fmt.Sprintf("cd %s && %s exec -T -- %s nginx -s reload 2>&1", q(rp), composeBase(files, envs), q(NginxServiceName))
	if out, err := r.Run(ctx, shScript(cmd)); err != nil {
		return fmt.Errorf("nginx reload failed on %s: %w\n%s", r.Label(), err, string(out))
	}
	return nil
}

// gateError classes a gate failure like the local rolling restart: E250 when
// the service is broken, E251 when it was too slow, no code when docker could
// not be read or the user interrupted.
func gateError(svc string, err error) error {
	hint := fmt.Sprintf("Run 'nself logs %s' on the host for details", svc)
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("remote apply interrupted at service %s: %w", svc, err)
	case errors.Is(err, docker.ErrStateRead):
		return fmt.Errorf("remote apply: cannot read the state of service %s, so it was not judged: %w", svc, err)
	case errors.Is(err, docker.ErrOneShotFailed), errors.Is(err, docker.ErrServiceCrashed):
		return errs.Wrap("E250", fmt.Sprintf("remote apply: %v. %s", err, hint), err)
	case errors.Is(err, docker.ErrGateTimeout):
		return errs.Wrap("E251", fmt.Sprintf("remote apply: %v. %s", err, hint), err)
	}
	return fmt.Errorf("remote apply: service %s: %w", svc, err)
}

// applyHasuraMetadata runs the legacy metadata apply on the host (cleanly a
// no-op when hasura/ is absent): strict fails the deploy, otherwise a warning.
func applyHasuraMetadata(ctx context.Context, o ApplyOptions) error {
	emit(o.Out, "  [running] hasura metadata apply on %s", o.Remote.Label())
	if out, err := o.Remote.Run(ctx, HasuraMetadataApplyCommand(o.RemotePath)); err != nil {
		msg := fmt.Sprintf("remote hasura metadata apply failed: %v\n%s", err, string(out))
		if o.StrictHasura {
			return fmt.Errorf("%s", msg)
		}
		emit(o.Out, "  [warn] %s (non-fatal; set NSELF_HASURA_METADATA_STRICT=true to fail deploys on this)", msg)
	}
	return nil
}

// HasuraMetadataApplyCommand is the remote metadata apply with a short retry
// loop, wrapped in sh -c: the for-loop is POSIX syntax that a fish or csh login
// shell would reject (D-0542). The legacy single-host push uses it too.
func HasuraMetadataApplyCommand(remotePath string) string {
	return shScript(fmt.Sprintf("cd %s && for i in 1 2 3 4 5; do nself db hasura metadata apply && exit 0; sleep 3; done; exit 1", q(remotePath)))
}
