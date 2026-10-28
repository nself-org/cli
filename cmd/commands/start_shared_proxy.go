package commands

// start_shared_proxy.go — the runtime half of the shared host proxy (EPIC ADOPT
// D2, P7-ADOPT-21): attach the owner's nginx to fronted project networks after
// `nself start`, and detach it before any compose down of a fronted project.
//
// Purpose: a fronted project publishes no 80/443; the owner's nginx serves its
// domains by joining its `<project>_network`. Compose cannot remove that
// network while the owner's nginx is attached, so every compose-down path
// detaches first (a pre-down hook in internal/docker registered here, which
// covers stop, restart, `start --fresh/--clean-start`, reset and uninstall).
// Inputs: the loaded config and the project directory; for the hook only the
// directory (the registration under the parent says what the project is).
// Outputs: warnings on stderr; an E506 error when the attach of the starting
// fronted project fails, when its registrations are unreadable, or when
// uninstall cannot leave the proxy.
// Constraints: nothing happens without nginx/shared/_owner.json, so a project
// outside a shared proxy is untouched; the owner's start never fails because a
// fronted project is down or broken (it warns and continues); a missing owner
// container or network is one warning, not an error. The sharedproxy calls are
// package variables so tests run without Docker.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/sharedproxy"
	"github.com/nself-org/cli/internal/ui"
)

var (
	spLocate = sharedproxy.Locate
	spAttach = sharedproxy.Attach
	spDetach = sharedproxy.DetachHolding
	spLeave  = sharedproxy.Leave
	spFlush  = sharedproxy.ReloadIfPending
)

func init() {
	docker.RegisterPreDownHook(beforeStopSharedProxy)
}

// afterStartSharedProxy runs once the stack is healthy. In an owner it attaches
// every registered project whose network exists; in a fronted project it
// attaches the owner's nginx to this project's network.
func afterStartSharedProxy(ctx context.Context, cfg *config.Config, workdir string) error {
	site, err := spLocate(workdir)
	if err != nil {
		// A fronted start that cannot read the registrations is not attached:
		// its domains would not be served, so it fails. The owner's start (and
		// any other project) only warns: it never fails for a fronted project.
		if cfg != nil && cfg.Nginx.FrontedBy != "" {
			return errs.New("E506", "the shared proxy registrations are not readable, so this project was not attached").WithWhy(err.Error())
		}
		ui.Warn(fmt.Sprintf("Shared proxy: registrations are not readable, nothing attached: %v", err))
		return nil
	}
	switch site.Role {
	case sharedproxy.RoleOwner:
		flushPendingReload(ctx, workdir)
		attachAllRegistered(ctx, workdir, site)
		return nil
	case sharedproxy.RoleFronted:
		if cfg == nil || cfg.Nginx.FrontedBy == "" {
			return nil // not configured as fronted any more; the next build unregisters it
		}
		return attachSelf(ctx, site)
	}
	return nil
}

// flushPendingReload pays the owner reload that a fronted project's failed
// uninstall left owed (D-0931). A failure is a warning: the owner still starts.
func flushPendingReload(ctx context.Context, ownerDir string) {
	if _, err := spFlush(ctx, ownerDir); err != nil {
		ui.Warn(fmt.Sprintf("Shared proxy: a removed project's files may still be served; the owner's nginx could not be reloaded: %v", err))
	}
}

func attachAllRegistered(ctx context.Context, ownerDir string, site sharedproxy.Site) {
	if len(site.Set.Projects) == 0 {
		return
	}
	out, err := spAttach(ctx, ownerDir, "", site.Set.Projects)
	warnOutcome(out, err)
}

func attachSelf(ctx context.Context, site sharedproxy.Site) error {
	reg := *site.Reg
	out, err := spAttach(ctx, site.OwnerDir, reg.Project, []sharedproxy.Registration{reg})
	if err == nil || errors.Is(err, sharedproxy.ErrOwnerNotRunning) {
		warnOutcome(out, err)
		return nil
	}
	// A broken conf of another project was quarantined and this project is
	// attached: the start succeeded, so only warn.
	if slices.Contains(out.Attached, reg.Project) && !slices.Contains(out.Quarantined, reg.Project) {
		warnOutcome(out, err)
		return nil
	}
	return err
}

func warnOutcome(out sharedproxy.Outcome, err error) {
	for _, p := range out.Missing {
		ui.Warn(fmt.Sprintf("Shared proxy: project %s is registered but its network %s_network does not exist (stopped?); skipped", p, p))
	}
	switch {
	case err == nil:
	case errors.Is(err, sharedproxy.ErrOwnerNotRunning):
		ui.Warn(fmt.Sprintf("Shared proxy: %v; start the owner project first. Nothing was attached.", err))
	default:
		ui.Warn(fmt.Sprintf("Shared proxy: %v", err))
	}
}

// beforeStopSharedProxy is the compose pre-down hook: a fronted project
// disconnects the owner's nginx from its network before its network is removed.
// The owner lock is held until the returned function runs after the down, so a
// concurrent owner start cannot reattach the network in between (D-0929).
func beforeStopSharedProxy(ctx context.Context, workdir string) func() {
	site, err := spLocate(workdir)
	if err != nil {
		ui.Warn(fmt.Sprintf("Shared proxy: registrations are not readable, so the owner's nginx was not disconnected and the network may stay in use: %v", err))
		return nil
	}
	if site.Role != sharedproxy.RoleFronted {
		return nil
	}
	release, err := spDetach(ctx, site.OwnerDir, *site.Reg)
	switch {
	case err == nil:
	case errors.Is(err, sharedproxy.ErrOwnerNotRunning):
		ui.Warn("Shared proxy: the owner's nginx is not running; nothing to disconnect")
	default:
		ui.Warn(fmt.Sprintf("Shared proxy: %v", err))
	}
	return release
}

// leaveSharedProxy is `nself uninstall` in a fronted project (review R2b): it
// removes the project's files and registration from the owner and reloads the
// owner's nginx. A project outside a shared proxy is untouched. A failure is
// returned so uninstall does not report the project as fully removed.
func leaveSharedProxy(ctx context.Context, workdir string) error {
	site, err := spLocate(workdir)
	if err != nil {
		ui.Warn(fmt.Sprintf("Shared proxy: registrations are not readable: %v", err))
		return errs.New("E506", "cannot leave the shared proxy: its registrations are not readable").WithWhy(err.Error())
	}
	if site.Role != sharedproxy.RoleFronted {
		// A retry after a failed reload: the registration is already gone, but
		// the parent owner may still owe a reload (D-0931).
		if parent := filepath.Dir(workdir); sharedproxy.ReloadPending(parent) {
			if _, err := spFlush(ctx, parent); err != nil {
				ui.Warn(fmt.Sprintf("Shared proxy: reloading %s: %v", parent, err))
				return fmt.Errorf("reloading the shared proxy at %s: %w", parent, err)
			}
		}
		return nil
	}
	if _, err := spLeave(ctx, site.OwnerDir, site.Reg.Project); err != nil {
		ui.Warn(fmt.Sprintf("Shared proxy: leaving %s: %v", site.OwnerDir, err))
		return fmt.Errorf("leaving the shared proxy at %s: %w", site.OwnerDir, err)
	}
	return nil
}
