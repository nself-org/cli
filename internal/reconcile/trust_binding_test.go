package reconcile

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// localTrustSteps are the certificates and trust-store steps of a first local
// build, as plan mode records them; fp names the mkcert CA.
func localTrustSteps(project, fp string) []Effect {
	return []Effect{
		{Kind: EffectCertificates, Target: filepath.Join(project, "ssl", "certificates", "local-nself-org"),
			Detail: "generate a certificate for localhost, 127.0.0.1, ::1, local.nself.org, *.local.nself.org"},
		{Kind: EffectTrustStore, Target: "mkcert CA",
			Detail: "install the mkcert CA sha256:" + strings.Repeat(fp, 64) + " into the system trust store"},
	}
}

// TestPlanIDBindsLocalTrustSteps (P7-SHIP-02): on a dev env a plan id binds
// the local certificate and the named mkcert CA install; the apply passes the
// E454 gate and reaches the write.
func TestPlanIDBindsLocalTrustSteps(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed"), Extra: localTrustSteps(f.project, "a")}
	p, err := Compute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	reached := errors.New("write reached")
	_, err = Apply(context.Background(), req, ApplyOptions{PlanID: p.PlanID, Yes: true, BeforeWrite: func() error { return reached }})
	if !errors.Is(err, reached) {
		t.Fatalf("the bound local trust steps must reach the write, got %v", err)
	}
}

// TestPlanIDRefusesChangedTrust: a trust step that differs from the plan (the
// CA was replaced after the plan) is refused (E450) and nothing runs.
func TestPlanIDRefusesChangedTrust(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed"), Extra: localTrustSteps(f.project, "a")}
	p, err := Compute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	req.Extra = localTrustSteps(f.project, "b")
	before := treeHash(t, f.root)
	started := false
	_, err = Apply(context.Background(), req, ApplyOptions{PlanID: p.PlanID, Yes: true, BeforeWrite: func() error { started = true; return nil }})
	if d := errs.Describe(err); d == nil || d.Code != "E450" {
		t.Fatalf("want E450 for a changed trust step, got %v", err)
	}
	if started || treeHash(t, f.root) != before {
		t.Fatal("the refused apply ran effects or wrote project files")
	}
}

// TestPlanIDRefusesTrustOutsideLocalDev: the same steps on a prod-class env,
// a certificate for a public domain, or an unnamed CA install stay E454.
func TestPlanIDRefusesTrustOutsideLocalDev(t *testing.T) {
	for name, tc := range map[string]struct {
		fixture string
		mut     func(project string, e []Effect)
	}{
		"prod env":      {"prod-ssl", func(string, []Effect) {}},
		"public domain": {"dev-minimal", func(_ string, e []Effect) { e[0].Detail += ", example.com" }},
		"unnamed CA":    {"dev-minimal", func(_ string, e []Effect) { e[1].Detail = "install the mkcert CA when it is not yet trusted" }},
	} {
		t.Run(name, func(t *testing.T) {
			f := loadFixture(t, tc.fixture)
			steps := localTrustSteps(f.project, "a")
			tc.mut(f.project, steps)
			req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed"), Extra: steps}
			p, err := Compute(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			started := false
			_, err = Apply(context.Background(), req, ApplyOptions{PlanID: p.PlanID, Yes: true, BeforeWrite: func() error { started = true; return nil }})
			if d := errs.Describe(err); d == nil || d.Code != "E454" || started {
				t.Fatalf("want E454 before any write, got %v (write started %v)", err, started)
			}
		})
	}
}
