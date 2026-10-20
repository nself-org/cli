package bundle

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/bundle/appready"
)

func TestReconcileFailureRollsBackFreshMembers(t *testing.T) {
	for _, refusal := range []string{"refused", "regeneration failed"} {
		for _, keep := range []bool{false, true} {
			t.Run(refusal+map[bool]string{false: "/fresh", true: "/existing"}[keep], func(t *testing.T) {
				dir := isolate(t)
				registry := mockNotificationsOnly()
				if keep {
					registry = mockTaskRegistry()
				}
				setupOfflineRegistry(t, registry)
				if keep {
					writeMember(t, dir, "jobs", 1, nil)
				}
				var out bytes.Buffer
				opts := freeOpts(t, dir, &out, func(name string) error { writeMember(t, dir, name, 1, nil); return nil })
				boom := errors.New(refusal)
				opts.Hooks = &appready.Hooks{Reconcile: func(context.Context) error { return boom }}
				res, err := Install(opCtx(t), "task", opts)
				if !errors.Is(err, boom) {
					t.Fatalf("error = %v", err)
				}
				_, statErr := os.Stat(filepath.Join(dir, "notifications"))
				if !os.IsNotExist(statErr) {
					t.Fatalf("fresh member remains: %v", statErr)
				}
				if keep {
					if _, err := os.Stat(filepath.Join(dir, "jobs")); err != nil {
						t.Fatal(err)
					}
				}
				l, _, err := newLedgerStore(dir).Load()
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := l.Plugins["notifications"]; ok {
					t.Fatalf("ledger member present = %v, want %v", ok, false)
				}
				if len(res.RolledBack) != 1 {
					t.Fatalf("rollback = %v", res.RolledBack)
				}
			})
		}
	}
}
