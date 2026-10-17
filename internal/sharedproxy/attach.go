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
// test is repeated, and when it still fails every connection made by this call
// is undone. All docker calls go through internal/docker.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
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

// networkOK reports whether r names a network this package may touch.
func networkOK(r Registration) bool {
	return ValidProject(r.Project) && r.Network == r.Project+"_network"
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

// Attach connects the owner nginx to the network of every registration in regs
// whose network exists, restores any quarantined files of those projects, then
// tests and reloads nginx once. self names the attaching project ("" when the
// owner attaches all of them); it is the fallback culprit when `nginx -t`
// names no registered file.
func Attach(ctx context.Context, ownerDir, self string, regs []Registration) (Outcome, error) {
	var out Outcome
	if len(regs) == 0 {
		return out, nil
	}
	nginx, state, err := ownerNginxAny(ctx, ownerDir)
	if err != nil {
		return out, err
	}
	// A fronted project's start never touches a stopped owner. The owner's own
	// start does: a cold nginx cannot resolve a fronted upstream until it
	// joins that network, so it is stopped (crash loop), attached, started.
	cold := state != "running"
	if cold && self != "" {
		return out, fmt.Errorf("%w (project dir %s, container %s is %s)", ErrOwnerNotRunning, ownerDir, nginx, state)
	}
	if state == "restarting" {
		if err := docker.StopContainer(ctx, nginx); err != nil {
			return out, e506("cannot stop the restarting owner nginx "+nginx, err, "")
		}
	}
	set, err := Load(DirOf(ownerDir), ownerDir)
	if err != nil {
		return out, e506("the shared proxy registrations are not readable", err, "")
	}
	before, err := docker.ContainerNetworks(ctx, nginx)
	if err != nil {
		return out, e506("cannot read the owner nginx's networks", err, "")
	}
	var connected []Registration // connected by this call, for the rollback
	undo := func() {
		for _, r := range connected {
			_ = docker.NetworkDisconnect(ctx, r.Network, nginx)
		}
	}
	for _, r := range regs {
		if !networkOK(r) {
			undo()
			return out, e506(fmt.Sprintf("registration %q names the network %q, not %s_network", r.Project, r.Network, r.Project), nil, "")
		}
		exists, err := docker.NetworkExists(ctx, r.Network)
		if err != nil {
			undo()
			return out, e506("cannot check network "+r.Network, err, "")
		}
		if !exists {
			out.Missing = append(out.Missing, r.Project)
			continue
		}
		if _, err := Restore(ownerDir, r); err != nil {
			undo()
			return out, e506("cannot restore the quarantined files of "+r.Project, err, "")
		}
		if err := docker.NetworkConnect(ctx, r.Network, nginx); err != nil {
			undo()
			return out, e506(fmt.Sprintf("cannot connect %s to %s", nginx, r.Network), err, "")
		}
		if !slices.Contains(before, r.Network) {
			connected = append(connected, r)
		}
		out.Attached = append(out.Attached, r.Project)
	}
	if cold && len(out.Attached) > 0 {
		if err := startAndWait(ctx, nginx); err != nil {
			undo()
			out.Attached = nil
			return out, e506("the owner's nginx did not start after it was attached", err, "")
		}
	}
	if len(out.Attached) == 0 {
		return out, nil
	}

	_, stderr, terr := nginxTest(ctx, nginx)
	if terr == nil {
		return out, reload(ctx, nginx, &out, undo)
	}
	// R3: quarantine the project(s) whose file the output names; the attaching
	// project only when it names none.
	bad := Implicated(set, stderr)
	if len(bad) == 0 && self != "" {
		if r := set.Find(self); r != nil {
			bad = []Registration{*r}
		}
	}
	if len(bad) == 0 {
		undo()
		out.Attached = nil
		return out, e506("nginx -t rejected the shared proxy configuration; nothing was changed", terr, stderr)
	}
	for _, r := range bad {
		if _, err := Quarantine(ownerDir, set, r); err != nil {
			undo()
			out.Attached = nil
			return out, e506("cannot quarantine the files of "+r.Project, err, stderr)
		}
		out.Quarantined = append(out.Quarantined, r.Project)
		// Only a connection this call made is undone: a project that was
		// already attached keeps working, and a registration that was not
		// part of this call is never disconnected.
		if i := slices.IndexFunc(connected, func(c Registration) bool { return c.Project == r.Project }); i >= 0 && networkOK(r) {
			_ = docker.NetworkDisconnect(ctx, r.Network, nginx)
			connected = slices.Delete(connected, i, i+1)
		}
		out.Attached = slices.DeleteFunc(out.Attached, func(p string) bool { return p == r.Project })
	}
	if _, stderr2, err2 := nginxTest(ctx, nginx); err2 != nil {
		// The attribution was wrong: put the files back, undo the connections.
		for _, r := range bad {
			_, _ = Restore(ownerDir, r)
		}
		undo()
		msg := "nginx -t still fails after quarantining " + strings.Join(out.Quarantined, ", ") + "; the files were put back"
		out.Attached, out.Quarantined = nil, nil
		return out, e506(msg, err2, stderr2)
	}
	if err := reload(ctx, nginx, &out, undo); err != nil {
		return out, err
	}
	return out, e506(fmt.Sprintf("nginx -t rejected the files of %s; they were moved to nginx/shared/quarantine/ and the owner keeps serving", strings.Join(out.Quarantined, ", ")), terr, stderr)
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

// Detach disconnects the owner nginx (running or stopped) from the network of
// every registration in regs. A network that is absent or not connected is
// success. It does not
// reload: nginx resolves upstreams per request.
func Detach(ctx context.Context, ownerDir string, regs ...Registration) error {
	nginx, _, err := ownerNginxAny(ctx, ownerDir)
	if err != nil {
		return err
	}
	var errList []error
	for _, r := range regs {
		if !networkOK(r) {
			continue
		}
		if err := docker.NetworkDisconnect(ctx, r.Network, nginx); err != nil {
			errList = append(errList, e506(fmt.Sprintf("cannot disconnect %s from %s", nginx, r.Network), err, ""))
		}
	}
	return errors.Join(errList...)
}
