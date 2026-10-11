package bundle

// remover_property_test.go — P7-PLUG-20: a refcount property test. Random
// sequences of AddBundle / RemoveBundle / MarkExplicit / forget-a-record run on
// a ledger.Ledger beside a plain model; after every operation the ledger must
// be valid, installed_by must be a sorted set of recorded bundles (so a count
// can never go negative or double), and RemoveBundle must release exactly the
// plugins no other bundle references and the owner did not install.

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle/ledger"
)

type modelRec struct {
	by       map[string]bool
	explicit bool
}

func TestRefcountProperty(t *testing.T) {
	bundleSlugs := []string{"b0", "b1", "b2", "b3"}
	plugins := []string{"p0", "p1", "p2", "p3", "p4", "p5"}
	at := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	for seed := int64(1); seed <= 8; seed++ {
		rng := rand.New(rand.NewSource(seed))
		members := map[string][]string{}
		for _, b := range bundleSlugs {
			for _, p := range plugins {
				if rng.Intn(2) == 0 {
					members[b] = append(members[b], p)
				}
			}
			if len(members[b]) == 0 {
				members[b] = []string{plugins[rng.Intn(len(plugins))]}
			}
		}
		l := ledger.New()
		installed := map[string]bool{}
		model := map[string]*modelRec{}

		for step := 0; step < 200; step++ {
			ctx := fmt.Sprintf("seed %d step %d", seed, step)
			b := bundleSlugs[rng.Intn(len(bundleSlugs))]
			p := plugins[rng.Intn(len(plugins))]
			switch rng.Intn(4) {
			case 0: // AddBundle (also a re-add)
				var in []ledger.InstalledPlugin
				for _, m := range members[b] {
					in = append(in, ledger.InstalledPlugin{Name: m, Version: "1.0.0", Tier: ledger.TierFree})
					r := model[m]
					if r == nil {
						r = &modelRec{by: map[string]bool{}}
						model[m] = r
					}
					r.by[b] = true
				}
				installed[b] = true
				l.AddBundle(b, in, at)
			case 1: // RemoveBundle
				before, merr := l.Marshal()
				if merr != nil {
					t.Fatalf("%s: ledger invalid before remove: %v", ctx, merr)
				}
				released, err := l.RemoveBundle(b)
				if !installed[b] {
					if err == nil {
						t.Fatalf("%s: RemoveBundle of unrecorded %s did not fail", ctx, b)
					}
					after, _ := l.Marshal()
					if !bytes.Equal(before, after) {
						t.Fatalf("%s: a failed RemoveBundle changed the ledger", ctx)
					}
					break
				}
				if err != nil {
					t.Fatalf("%s: %v", ctx, err)
				}
				delete(installed, b)
				var want []string
				for name, r := range model {
					delete(r.by, b)
					if len(r.by) == 0 && !r.explicit {
						want = append(want, name)
						delete(model, name)
					}
				}
				sort.Strings(want)
				if !sameStrings(released, want) {
					t.Fatalf("%s: released %v, model says %v", ctx, released, want)
				}
			case 2: // MarkExplicit
				l.MarkExplicit(ledger.InstalledPlugin{Name: p, Version: "1.0.0", Tier: ledger.TierFree})
				r := model[p]
				if r == nil {
					r = &modelRec{by: map[string]bool{}}
					model[p] = r
				}
				r.explicit = true
			case 3: // forget a record (plugin removed on its own)
				delete(l.Plugins, p)
				delete(model, p)
			}
			checkLedgerAgainstModel(t, ctx, l, installed, model)
		}
	}
}

func checkLedgerAgainstModel(t *testing.T, ctx string, l ledger.Ledger, installed map[string]bool, model map[string]*modelRec) {
	t.Helper()
	if err := l.Validate(); err != nil {
		t.Fatalf("%s: %v", ctx, err)
	}
	if len(l.Bundles) != len(installed) {
		t.Fatalf("%s: ledger bundles %d, model %d", ctx, len(l.Bundles), len(installed))
	}
	if len(l.Plugins) != len(model) {
		t.Fatalf("%s: ledger plugins %d, model %d", ctx, len(l.Plugins), len(model))
	}
	for name, rec := range l.Plugins {
		m := model[name]
		if m == nil {
			t.Fatalf("%s: ledger has unexpected plugin %s", ctx, name)
		}
		if !sort.StringsAreSorted(rec.InstalledBy) {
			t.Fatalf("%s: %s installed_by %v is not sorted", ctx, name, rec.InstalledBy)
		}
		if len(rec.InstalledBy) != len(m.by) || rec.Explicit != m.explicit {
			t.Fatalf("%s: %s = %+v, model by=%v explicit=%v", ctx, name, rec, m.by, m.explicit)
		}
		seen := map[string]bool{}
		for _, b := range rec.InstalledBy {
			if seen[b] || !m.by[b] || !installed[b] {
				t.Fatalf("%s: %s installed_by %v has a duplicate, a stale or an unrecorded bundle", ctx, name, rec.InstalledBy)
			}
			seen[b] = true
		}
	}
}
