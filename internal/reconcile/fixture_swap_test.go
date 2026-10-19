package reconcile

import (
	"context"
	"os"
	"regexp"
	"testing"
)

// TestLiveSafetyFixtureSwapRendersInProdPlugins (D-1018): the same-shape case of
// scripts/ci/live-safety-fixture.sh swaps one .env value after an apply and
// expects the old plan id to be refused (E450). That only proves the guard when
// the swapped key reaches a rendered artifact. MINIO_ROOT_USER does not in the
// prod-plugins project (MINIO_ENABLED=false): the plan stays empty with the same
// id, so applying with it exits 0, correctly, and the scheduled e2e-golden-path
// run went red. Pin that every key the case swaps changes the plan.
func TestLiveSafetyFixtureSwapRendersInProdPlugins(t *testing.T) {
	script, err := os.ReadFile("../../scripts/ci/live-safety-fixture.sh")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)# A same-shape content change.*?\n# ----`).Find(script)
	if block == nil {
		t.Fatal("same-shape block not found in live-safety-fixture.sh")
	}
	keys := regexp.MustCompile(`sed -i\.bak 's/\^([A-Z_]+)=`).FindAllSubmatch(block, -1)
	if len(keys) == 0 {
		t.Fatal("the same-shape block swaps no .env key")
	}
	for _, m := range keys {
		key := string(m[1])
		t.Run(key, func(t *testing.T) {
			f := loadFixture(t, "prod-plugins")
			req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed")}
			if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err != nil {
				t.Fatal(err)
			}
			if !planOf(t, f, nil).Empty {
				t.Fatal("plan after apply is not empty")
			}
			setEnvValue(t, f, key, "swapped-value-Zz9")
			if planOf(t, f, nil).Empty {
				t.Fatalf("swapping %s changes no artifact: the fixture's old plan id would still be current", key)
			}
		})
	}
}
