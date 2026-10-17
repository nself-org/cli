package build

// shared_proxy_lock.go — the owner lock a shared-proxy build holds while it writes.
//
// Purpose: a build writes into the owner's nginx/sites, ssl and nginx/shared,
// the same files Attach tests, quarantines and reloads. The per-project build
// lock does not cover that, so the build takes sharedproxy.Lock(ownerRoot).
// Inputs: the sharedProxy plan (role, ownerRoot).
// Outputs: a lock held from sharedDetect until Build returns (released by
// sharedUnlock, deferred in Build), so it spans the render writes and
// sharedFinish.
// Constraints: write mode only. Plan mode and --check write nothing and never
// wait. The lock is taken before the registrations are read, so the checks
// see what is on disk. Taken once per build (modelRoutes may run twice).

import (
	"context"

	"github.com/nself-org/cli/internal/sharedproxy"
)

// lockShared takes the owner lock for a build that will write to the owner.
func (st *buildState) lockShared(sp *sharedProxy) error {
	if st.planning() || st.opts.Check || st.sharedRelease != nil {
		return nil
	}
	release, err := sharedproxy.Lock(context.Background(), sp.ownerRoot)
	if err != nil {
		return err
	}
	st.sharedRelease = release
	return nil
}

// sharedUnlock releases the owner lock, if this build holds it.
func (st *buildState) sharedUnlock() {
	if st.sharedRelease != nil {
		st.sharedRelease()
		st.sharedRelease = nil
	}
}
