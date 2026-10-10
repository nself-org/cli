package commands

// bundle_appready_gate_test.go — P7-PLUG-19 fix round 1: the v1.5 gate on the
// start/metadata/health hooks and Start outside a project.

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestBundleHooksGatedByMode: reconcile is wired in both modes; start, Hasura
// metadata and service health only in v1.5 (v1.4 does not boot the stack).
func TestBundleHooksGatedByMode(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		h := newBundleAppReadyHooks(bundleInstallCmd, "task")
		if h.Reconcile == nil {
			t.Fatal("reconcile hook missing")
		}
		wired := h.Start != nil && h.ExportMetadata != nil && h.ServiceHealth != nil
		none := h.Start == nil && h.ExportMetadata == nil && h.ServiceHealth == nil
		if compat.V15() && !wired {
			t.Errorf("v1.5 must wire start, metadata and health: %+v", h)
		}
		if !compat.V15() && !none {
			t.Errorf("v1.4 must not wire start, metadata or health: %+v", h)
		}
	})
}

// TestBundleStartOutsideProject: Start reports ErrNoProject (not start's own
// error), so an unchanged-membership install outside a project is not a
// failure: the rest is skipped and the exit stays 0.
func TestBundleStartOutsideProject(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	if err := startStackInProcess(context.Background()); !errors.Is(err, appready.ErrNoProject) {
		t.Fatalf("startStackInProcess outside a project = %v, want ErrNoProject", err)
	}
	h := newBundleAppReadyHooks(bundleInstallCmd, "task")
	rn := appready.NewRunner("task")
	appready.Run(context.Background(), rn, *h, t.TempDir(), nil, false) // changed=false
	if rn.Failure() != nil {
		t.Fatalf("outside a project must not fail: %+v", rn.Failure())
	}
	rep := rn.Report()
	if rep.AppReady || rep.Note == "" {
		t.Errorf("report = %+v, want not app-ready with a note", rep)
	}
}
