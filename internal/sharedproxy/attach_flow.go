package sharedproxy

// attach_flow.go — the body of Attach (call it through Attach, which holds the
// owner lock): connect, restore, test, quarantine, reload, and undo on failure.
//
// Purpose: see attach.go. The state one call needs is attachRun.
// Constraints: the before-snapshot of the owner's networks is taken inside the
// lock, so a network a concurrent start connected is never recorded as
// connected by this call. Every error exit re-quarantines exactly the files
// this call restored (attachRun.restored), unless the call ended settled:
// nginx reloaded with the broken project(s) quarantined. A cold owner is never
// started before its merged configuration passes nginx -t, run in a throwaway
// container (D-0928); a failure there is attributed and quarantined like a
// failure in the running owner.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nself-org/cli/internal/docker"
)

// attachRun is the state of one attach call.
type attachRun struct {
	ctx       context.Context
	ownerDir  string
	self      string
	nginx     string
	set       Set
	out       Outcome
	connected []Registration // connected by this call, for the rollback
	restored  []Registration // Files = what this call took out of quarantine
	// test runs nginx -t on the owner's configuration, finish puts a passing
	// configuration live. Running owner: exec in it, reload. Cold owner: a
	// throwaway container, then start and reload.
	test   func() (stdout, stderr string, err error)
	finish func() error
}

// undo disconnects every network this call connected.
func (a *attachRun) undo() {
	for _, r := range a.connected {
		_ = docker.NetworkDisconnect(a.ctx, r.Network, a.nginx)
	}
}

// requarantine puts the files this call restored back into quarantine.
func (a *attachRun) requarantine() error {
	var errList []error
	for _, r := range a.restored {
		if _, err := quarantine(a.ownerDir, a.set, r); err != nil {
			errList = append(errList, e506("cannot put the restored files of "+r.Project+" back into quarantine", err, ""))
		}
	}
	return errors.Join(errList...)
}

// stillRegistered keeps the incoming registrations that the set loaded under
// the lock still holds with the same project, network and project dir, and
// returns the loaded copy of each. A project that left (Leave) since the
// caller read its registration is not attached. A registration that names a
// network this package may not touch is kept as it is: run refuses it with an
// E506, so tampered input stays an error and is never a silent drop.
func stillRegistered(set Set, regs []Registration) []Registration {
	var out []Registration
	for _, r := range regs {
		if !networkOK(set, r) {
			out = append(out, r)
			continue
		}
		cur := set.Find(r.Project)
		if cur == nil || cur.Network != r.Network || !sameDir(cur.ProjectDir, r.ProjectDir) {
			continue
		}
		out = append(out, *cur)
	}
	return out
}

// attach is Attach without the lock.
func attach(ctx context.Context, ownerDir, self string, regs []Registration) (Outcome, error) {
	if len(regs) == 0 {
		return Outcome{}, nil
	}
	a := &attachRun{ctx: ctx, ownerDir: ownerDir, self: self}
	a.test, a.finish = a.execTest, a.execFinish
	settled, err := a.run(regs)
	if err != nil && !settled {
		if qerr := a.requarantine(); qerr != nil {
			err = errors.Join(err, qerr)
		}
	}
	return a.out, err
}

// run does the work. settled is true when the error it returns leaves a
// consistent disk (the broken project(s) are quarantined and nginx reloaded).
func (a *attachRun) run(regs []Registration) (settled bool, err error) {
	nginx, state, err := ownerNginxAny(a.ctx, a.ownerDir)
	if err != nil {
		return false, ownerErr(err)
	}
	a.nginx = nginx
	// A fronted project's start never touches a stopped owner. The owner's own
	// start does: a cold nginx cannot resolve a fronted upstream until it
	// joins that network, so it is stopped (crash loop), attached, started.
	cold := state != "running"
	if cold && a.self != "" {
		return false, fmt.Errorf("%w (project dir %s, container %s is %s)", ErrOwnerNotRunning, a.ownerDir, nginx, state)
	}
	if a.set, err = Load(DirOf(a.ownerDir), a.ownerDir); err != nil {
		return false, e506("the shared proxy registrations are not readable", err, "")
	}
	if regs = stillRegistered(a.set, regs); len(regs) == 0 {
		return false, nil // every project left between the caller's read and the lock
	}
	if state == "restarting" {
		if err := docker.StopContainer(a.ctx, nginx); err != nil {
			return false, e506("cannot stop the restarting owner nginx "+nginx, err, "")
		}
	}
	before, err := docker.ContainerNetworks(a.ctx, nginx)
	if err != nil {
		return false, e506("cannot read the owner nginx's networks", err, "")
	}
	for _, r := range regs {
		if !networkOK(a.set, r) {
			a.undo()
			return false, e506(fmt.Sprintf("registration %q names the network %q, not %s_network (or it is the owner's own)", r.Project, r.Network, r.Project), nil, "")
		}
		exists, err := docker.NetworkExists(a.ctx, r.Network)
		if err != nil {
			a.undo()
			return false, e506("cannot check network "+r.Network, err, "")
		}
		if !exists {
			a.out.Missing = append(a.out.Missing, r.Project)
			continue
		}
		back, err := restore(a.ownerDir, r)
		if len(back) > 0 {
			rr := r
			rr.Files = back
			a.restored = append(a.restored, rr)
		}
		if err != nil {
			a.undo()
			return false, e506("cannot restore the quarantined files of "+r.Project, err, "")
		}
		if err := docker.NetworkConnect(a.ctx, r.Network, nginx); err != nil {
			a.undo()
			return false, e506(fmt.Sprintf("cannot connect %s to %s", nginx, r.Network), err, "")
		}
		if !slices.Contains(before, r.Network) {
			a.connected = append(a.connected, r)
		}
		a.out.Attached = append(a.out.Attached, r.Project)
	}
	if cold && len(a.out.Attached) > 0 {
		a.test, a.finish = a.coldTest, a.finishCold
		_, stderr, terr := a.test()
		var exit *docker.RunLikeExit
		switch {
		case terr != nil && !errors.As(terr, &exit):
			a.undo()
			a.out.Attached = nil
			return false, e506("cannot test the configuration before the owner's nginx starts", terr, stderr)
		case terr != nil:
			return a.quarantineBad(terr, stderr)
		}
		if err := a.startCold(); err != nil {
			return false, err
		}
		a.test, a.finish = a.execTest, a.execFinish
	}
	if len(a.out.Attached) == 0 {
		return false, nil
	}
	_, stderr, terr := a.test()
	if terr == nil {
		return false, a.finish()
	}
	return a.quarantineBad(terr, stderr)
}

// execTest is nginx -t in the running owner; execFinish reloads it.
func (a *attachRun) execTest() (string, string, error) { return nginxTest(a.ctx, a.nginx) }
func (a *attachRun) execFinish() error                 { return reload(a.ctx, a.nginx, &a.out, a.undo) }

// coldTest is nginx -t for a stopped owner: a throwaway container with the
// owner's image and mounts, on the networks the owner is attached to now.
func (a *attachRun) coldTest() (string, string, error) {
	nets, err := docker.ContainerNetworks(a.ctx, a.nginx)
	if err != nil {
		return "", "", err
	}
	nets = slices.DeleteFunc(nets, func(n string) bool { return n == "host" || n == "none" })
	return docker.RunLike(a.ctx, docker.LikeSpec{Container: a.nginx, Networks: nets, Args: []string{"nginx", "-t"}})
}

// startCold starts the cold owner and waits for it to stay up. A failure
// leaves nothing attached.
func (a *attachRun) startCold() error {
	if err := startAndWait(a.ctx, a.nginx); err != nil {
		a.undo()
		a.out.Attached = nil
		return e506("the owner's nginx did not start after it was attached", err, "")
	}
	return nil
}

// finishCold starts the cold owner whose configuration passed and reloads it.
func (a *attachRun) finishCold() error {
	if err := a.startCold(); err != nil {
		return err
	}
	return reload(a.ctx, a.nginx, &a.out, a.undo)
}

// quarantineBad is R3: quarantine the project(s) whose file nginx -t names (the
// attaching project only when it names none), test again, and repeat while the
// output names a further registered project, at most once per project. When the
// test never passes, the files this step moved go back and every connection is
// undone.
func (a *attachRun) quarantineBad(terr error, stderr string) (settled bool, err error) {
	done := map[string]bool{}
	moved := map[string][]string{}
	var order []Registration
	curText, curErr := stderr, terr
	for round := 0; round <= len(a.set.Projects); round++ {
		var bad []Registration
		for _, r := range Implicated(a.set, curText) {
			if !done[r.Project] {
				bad = append(bad, r)
			}
		}
		if len(bad) == 0 && round == 0 && a.self != "" {
			if r := a.set.Find(a.self); r != nil {
				bad = []Registration{*r}
			}
		}
		if len(bad) == 0 {
			if round == 0 {
				a.undo()
				a.out.Attached = nil
				return false, e506("nginx -t rejected the shared proxy configuration; nothing was changed", terr, stderr)
			}
			break
		}
		for _, r := range bad {
			done[r.Project] = true
			mv, qerr := quarantine(a.ownerDir, a.set, r)
			if len(mv) > 0 {
				moved[r.Project] = mv
				order = append(order, r)
			}
			if qerr != nil {
				a.undo()
				a.out.Attached = nil
				return false, e506("cannot quarantine the files of "+r.Project, qerr, stderr)
			}
			a.out.Quarantined = append(a.out.Quarantined, r.Project)
			// Only a connection this call made is undone: a project that was
			// already attached keeps working, and a registration that was not
			// part of this call is never disconnected.
			if i := slices.IndexFunc(a.connected, func(c Registration) bool { return c.Project == r.Project }); i >= 0 && networkOK(a.set, r) {
				_ = docker.NetworkDisconnect(a.ctx, r.Network, a.nginx)
				a.connected = slices.Delete(a.connected, i, i+1)
			}
			a.out.Attached = slices.DeleteFunc(a.out.Attached, func(p string) bool { return p == r.Project })
		}
		_, text2, err2 := a.test()
		if err2 == nil {
			if err := a.finish(); err != nil {
				return false, err
			}
			return true, e506(fmt.Sprintf("nginx -t rejected the files of %s; they were moved to nginx/shared/quarantine/ and the owner keeps serving", strings.Join(a.out.Quarantined, ", ")), terr, stderr)
		}
		curText, curErr = text2, err2
	}
	// Never passed: the attribution was wrong. Put back what this step moved
	// (the state before the call; restored files are re-quarantined by the
	// caller) and undo the connections.
	var restoreErrs []error
	for _, r := range order {
		rr := r
		rr.Files = moved[r.Project]
		if _, rerr := restore(a.ownerDir, rr); rerr != nil {
			restoreErrs = append(restoreErrs, e506("cannot put the files of "+r.Project+" back", rerr, ""))
		}
	}
	a.undo()
	msg := "nginx -t still fails after quarantining " + strings.Join(a.out.Quarantined, ", ") + "; the files were put back"
	a.out.Attached, a.out.Quarantined = nil, nil
	return false, errors.Join(append([]error{e506(msg, curErr, curText)}, restoreErrs...)...)
}
