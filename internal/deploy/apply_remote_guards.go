package deploy

// Purpose: the host guards ApplyRemote runs before the lock and the tar (D24
//          steps 1-2 tail): symlink resolution of the shipped directories, the
//          hand-written nginx/conf.d files that stay out of the set (D-0424),
//          the refusal to overwrite an unmarked nginx file, and the front root's
//          guards (apply_remote_front.go). A failure is E485 and nothing was sent.
// Inputs:  the ApplyOptions (its Set is narrowed in place: kept conf.d files leave).
// Outputs: the front root to ship (nil when there is none), or the refusal.
// Constraints: read-only on the host; moved out of apply_remote.go unchanged.

import "context"

func shipGuards(ctx context.Context, o *ApplyOptions) (*frontApply, error) {
	r, rp := o.Remote, o.RemotePath
	// A symlink that leaves remote_path is refused before the lock and the tar.
	linked, err := ResolveDirs(ctx, r, rp, layoutDirs(append(o.Set.shippedPaths(), o.Previous...)))
	if err != nil {
		return nil, err
	}
	noticeLinks(o.Out, linked, o.Set.shippedPaths())
	// A hand-written nginx/conf.d file on the host is kept, not overwritten and not
	// an error (D-0424): it leaves the set before the lock, the tar and the sync.
	kept, err := KeptHostConfD(ctx, r, rp, o.Set.Files)
	if err != nil {
		return nil, err
	}
	keptPaths := make([]string, 0, len(kept))
	for _, k := range kept {
		keptPaths = append(keptPaths, k.Path)
		if k.DefaultServerIn != "" {
			emit(o.Out, "  [kept] %s (the host already declares a default_server in %s; not shipped)", k.Path, k.DefaultServerIn)
		} else {
			emit(o.Out, "  [kept] %s (hand-written on the host; not overwritten)", k.Path)
		}
	}
	o.Set = o.Set.Without(keptPaths)
	if err := CheckNotOverwritingForeign(ctx, r, rp, o.Set.Files, o.Previous); err != nil {
		return nil, err
	}
	return prepareFront(ctx, *o)
}
