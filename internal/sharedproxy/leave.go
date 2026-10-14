package sharedproxy

// leave.go — a fronted project leaves the shared host proxy for good.
//
// Purpose: review R2b of EPIC ADOPT D2. `nself uninstall` in a fronted project
// detaches the owner nginx, removes the project's files and registration from
// the owner (Unregister), and reloads the owner so it stops serving them.
// Inputs: the owner dir and the project name.
// Outputs: the owner-relative files removed.
// Constraints: the reload happens only after a passing `nginx -t`; an owner
// whose nginx is not running is not an error (the files are gone either way).

import (
	"context"
	"errors"
	"os"
)

// Leave detaches, unregisters and reloads. A project with no registration is a
// no-op.
func Leave(ctx context.Context, ownerDir, project string) ([]string, error) {
	set, err := Load(DirOf(ownerDir), ownerDir)
	if err != nil {
		return nil, err
	}
	reg := set.Find(project)
	if reg == nil {
		return nil, nil
	}
	detachErr := Detach(ctx, ownerDir, *reg)
	if errors.Is(detachErr, ErrOwnerNotRunning) {
		detachErr = nil
	}
	removed, err := Unregister(ownerDir, project)
	if err != nil {
		return removed, err
	}
	_ = os.RemoveAll(quarantineBase(ownerDir, project))
	if nginx, nerr := OwnerNginx(ctx, ownerDir); nerr == nil {
		if _, stderr, terr := nginxTest(ctx, nginx); terr != nil {
			return removed, e506("the owner's nginx -t fails after removing "+project+"; it was not reloaded", terr, stderr)
		}
		var o Outcome
		if err := reload(ctx, nginx, &o, func() {}); err != nil {
			return removed, err
		}
	}
	return removed, detachErr
}
