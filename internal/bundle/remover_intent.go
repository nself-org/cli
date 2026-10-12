// remover_intent.go — the durable steps of a real `nself bundle remove` (D-0368).
//
// Purpose: before this, the plan, every plugin removal and the ledger write ran
// inside one ledger Update that wrote the file only at the end. With no ledger
// file yet, a kill between members (or a failed final write) left nothing on
// disk, so the next run re-bootstrapped and, because bootstrap records a bundle
// only when every member is installed, lost the half-removed bundle (E134).
// Inputs: the held ledger Tx and ledger, the bundle, the plugin directory and
// the Remove options.
// Outputs: members removed; the ledger and the remove intent kept in step.
// Constraints: all of it runs inside ONE Store.Locked. Order: read the intent,
// plan, checkpoint the ledger, record the intent, remove members (intent shrinks
// after each), on full success clear the intent, then write the final ledger
// (a kill between the last two is harmless: see locked). If either checkpoint
// write fails nothing is removed. A failed intent shrink is only a warning:
// the intent then over-approximates and a gone member plans as not installed.
// RemoveOpts.noIntent (tests only) skips the intent and the checkpoint to
// reproduce the old behaviour.
package bundle

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/bundle/ledger"
)

// removeRun carries what the locked part of Remove needs.
type removeRun struct {
	b          Bundle
	opts       RemoveOpts
	out        io.Writer
	pluginDir  string
	deleteData bool
	result     *RemoveResult
	removeErr  error // set when a member failed; Remove returns it after the summary
}

// planWithKeeps is computePlan plus the dependency keeps (D-0325). Dry run and
// real runs both use it, so a dry run shows the same keeps.
func planWithKeeps(b Bundle, l ledger.Ledger, pluginDir string, opts RemoveOpts) (removePlan, error) {
	plan, err := computePlan(b, l, pluginDir)
	if err != nil {
		return plan, err
	}
	if err := keepDependedOn(&plan, l, pluginDir, opts.reverseDeps); err != nil {
		return removePlan{}, err
	}
	return plan, nil
}

// locked is the Store.Locked callback of a real remove.
func (r *removeRun) locked(ctx context.Context, tx *ledger.Tx, l ledger.Ledger) error {
	slug := strings.ToLower(r.b.Slug)
	started := time.Now().UTC()
	if !r.opts.noIntent {
		in, had, err := recoverIntent(tx, l, r.b, r.out)
		if err != nil {
			return err
		}
		if had {
			started = in.StartedAt
		}
	}
	plan, perr := planWithKeeps(r.b, l, r.pluginDir, r.opts)
	if perr != nil {
		return perr // nothing is written
	}
	r.result.adopt(plan)
	printRemovePlan(r.out, r.b, r.result, r.opts)
	cfg := r.opts.Config
	if cfg == nil && len(r.result.Planned) > 0 {
		loaded, lerr := loadConfigOrDefault()
		if lerr != nil {
			return fmt.Errorf("loading project config: %w", lerr)
		}
		cfg = loaded
	}
	if len(r.result.Planned) > 0 && !r.opts.noIntent {
		if err := tx.Write(l); err != nil {
			return fmt.Errorf("recording the bundle ledger before removing anything failed, nothing was removed: %w", err)
		}
		in := ledger.RemoveIntent{StartedAt: started, Remaining: lowerAll(r.result.Planned)}
		if err := tx.SetRemoveIntent(slug, in); err != nil {
			return fmt.Errorf("recording the remove intent failed, nothing was removed: %w", err)
		}
	}
	remaining := lowerAll(r.result.Planned)
	afterEach := func(name string) {
		if r.opts.noIntent {
			return
		}
		remaining = without(remaining, strings.ToLower(name))
		if err := tx.SetRemoveIntent(slug, ledger.RemoveIntent{StartedAt: started, Remaining: remaining}); err != nil {
			_, _ = fmt.Fprintf(r.out, "warning: could not update the remove intent after %s: %v\n", name, err)
		}
	}
	r.removeErr = removeMembers(ctx, r.out, r.result, cfg, r.pluginDir, r.opts.remover, r.deleteData, afterEach)
	settleLedger(&l, plan.work, r.result.Removed, r.removeErr != nil)
	// On full success the intent goes BEFORE the final write: a kill between the
	// two leaves the checkpoint ledger recording the bundle and nothing on disk,
	// so a rerun plans nothing and finishes cleanly. After a member failure the
	// intent stays.
	if r.removeErr == nil && !r.opts.noIntent {
		if err := tx.ClearRemoveIntent(slug); err != nil {
			_, _ = fmt.Fprintf(r.out, "warning: the remove finished but the remove intent could not be cleared: %v\n", err)
		}
	}
	return tx.Write(l)
}

func lowerAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, strings.ToLower(v))
	}
	return out
}

func without(list []string, drop string) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		if v != drop {
			out = append(out, v)
		}
	}
	return out
}
