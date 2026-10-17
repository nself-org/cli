package sharedproxy

// attach.go — connect the owner's nginx to fronted project networks and reload.
//
// Purpose: the runtime half of EPIC ADOPT D2 (P7-ADOPT-21). Compose gives each
// project its own network; the owner's nginx joins each registered project's
// `<project>_network` (docker network connect, idempotent), tests the combined
// configuration and reloads. Joining per project, not through one shared
// network, keeps project B's services unreachable from project C.
// Inputs: the owner dir, the registrations to attach, the attaching project
// (empty when the owner attaches everyone).
// Outputs: Outcome; an E506 error when the attach failed; ErrOwnerNotRunning
// (not an E506) when the owner's nginx container is not running.
// A cold owner (stopped or crash-looping nginx) is attached by the owner's own
// start only, then started, so a hand-written snippet with a static upstream
// can resolve it. A fronted project's start leaves a stopped owner alone.
// Constraints: only a network named exactly `<project>_network` for a valid
// project is ever touched, and only the owner's nginx container is ever
// connected; networks are never created or removed here; `nginx -s reload`
// runs only after a passing `nginx -t`. A failed test leaves the running
// configuration alone: the project(s) the output names are quarantined, the
// test is repeated (once more per further project the output names), and when
// it still fails every connection made by this call is undone. On every failed
// exit the files this call restored from quarantine go back into it, so a
// conf known to fail `nginx -t` never stays in nginx/sites. The public entry
// points (Attach, Detach, ...) hold the owner lock (lock.go); the bodies are
// lower case. All docker calls go through internal/docker.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

// ErrOwnerNotRunning is returned (wrapped) when the owner's nginx container is
// not running. Match with errors.Is.
var ErrOwnerNotRunning = errors.New("the shared proxy owner's nginx is not running")

// Outcome describes one Attach call.
type Outcome struct {
	Attached    []string // projects the owner nginx is connected to after the call
	Missing     []string // projects skipped because their network does not exist
	Quarantined []string // projects whose files were moved out of the owner
	Reloaded    bool
}

// OwnerNginx returns the name of the owner's running nginx container.
func OwnerNginx(ctx context.Context, ownerDir string) (string, error) {
	name, state, err := ownerNginxAny(ctx, ownerDir)
	if err == nil && state != "running" {
		return "", fmt.Errorf("%w (project dir %s, container %s is %s)", ErrOwnerNotRunning, ownerDir, name, state)
	}
	return name, err
}

// ownerNginxAny returns the owner's nginx container in any state, with its
// Docker state. A stopped container still holds network attachments that
// block compose from removing a fronted network, and a cold owner start has to
// attach before nginx can resolve a fronted upstream.
func ownerNginxAny(ctx context.Context, ownerDir string) (name, state string, err error) {
	dirs := []string{ownerDir}
	if real, err := filepath.EvalSymlinks(ownerDir); err == nil && real != ownerDir {
		dirs = append(dirs, real)
	}
	name, err = docker.FindServiceContainer(ctx, docker.ServiceMatch{Service: "nginx", WorkingDirs: dirs, AnyState: true})
	if errors.Is(err, docker.ErrServiceContainerNotFound) {
		return "", "", fmt.Errorf("%w (project dir %s)", ErrOwnerNotRunning, ownerDir)
	}
	if err != nil {
		return "", "", err
	}
	state, err = docker.InspectState(ctx, name)
	return name, state, err
}

// networkOK reports whether r names a network this package may touch: a valid
// project's own `<project>_network`, and never the owner's own project or
// network (the build refuses that registration; the runtime does too).
func networkOK(set Set, r Registration) bool {
	if !ValidProject(r.Project) || r.Network != r.Project+"_network" {
		return false
	}
	if o := set.Owner; o != nil && (r.Project == o.Project || (o.Network != "" && r.Network == o.Network)) {
		return false
	}
	return true
}

// ownerErr turns a failure to find the owner's nginx into an E506 error; the
// not-running sentinel stays matchable with errors.Is.
func ownerErr(err error) error {
	if err == nil || errors.Is(err, ErrOwnerNotRunning) {
		return err
	}
	return e506("cannot find the owner's nginx container", err, "")
}

func e506(msg string, cause error, text string) error {
	e := errs.New("E506", msg)
	switch {
	case text != "":
		e = e.WithWhy(strings.TrimSpace(text))
	case cause != nil:
		e = e.WithWhy(cause.Error())
	}
	return e
}

// startPoll and startWait bound how long startAndWait waits for the container
// to report running; variables so tests do not sleep.
var (
	startPoll = 250 * time.Millisecond
	startWait = 10 * time.Second
)

// startAndWait starts the container and waits until it stays running for one
// poll interval.
func startAndWait(ctx context.Context, name string) error {
	if err := docker.StartContainer(ctx, name); err != nil {
		return err
	}
	deadline := time.Now().Add(startWait)
	for {
		time.Sleep(startPoll)
		st, err := docker.InspectState(ctx, name)
		if err != nil {
			return err
		}
		if st == "running" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("container %s is %s after %s", name, st, startWait)
		}
	}
}

func nginxTest(ctx context.Context, nginx string) (string, string, error) {
	return docker.ExecCapture(ctx, nginx, []string{"nginx", "-t"})
}

func reload(ctx context.Context, nginx string, out *Outcome, undo func()) error {
	if _, stderr, err := docker.ExecCapture(ctx, nginx, []string{"nginx", "-s", "reload"}); err != nil {
		undo()
		out.Attached = nil
		return e506("nginx -s reload failed after a passing nginx -t", err, stderr)
	}
	out.Reloaded = true
	return nil
}

// detach disconnects the owner nginx (running or stopped) from the network of
// every registration in regs. A network that is absent or not connected is
// success. It does not reload: nginx resolves upstreams per request. Detach is
// the locked entry.
func detach(ctx context.Context, ownerDir string, regs ...Registration) error {
	nginx, _, err := ownerNginxAny(ctx, ownerDir)
	if err != nil {
		return ownerErr(err)
	}
	set, err := Load(DirOf(ownerDir), ownerDir)
	if err != nil {
		return e506("the shared proxy registrations are not readable", err, "")
	}
	var errList []error
	for _, r := range regs {
		if !networkOK(set, r) {
			continue
		}
		if err := docker.NetworkDisconnect(ctx, r.Network, nginx); err != nil {
			errList = append(errList, e506(fmt.Sprintf("cannot disconnect %s from %s", nginx, r.Network), err, ""))
		}
	}
	return errors.Join(errList...)
}
