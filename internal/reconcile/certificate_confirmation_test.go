package reconcile

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// Certificate and trust effects are runtime operations: an artifact plan id
// must never claim to confirm their random bytes or mutable host state.
func TestPlanIDRefusedWithCertificateEffects(t *testing.T) {
	for _, kind := range []EffectKind{EffectCertificates, EffectTrustStore} {
		t.Run(string(kind), func(t *testing.T) {
			f := loadFixture(t, "dev-minimal")
			req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed"),
				Extra: []Effect{{Kind: kind, Target: "runtime target"}}}
			p, err := Compute(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			before := treeHash(t, f.root)
			started := false
			_, err = Apply(context.Background(), req, ApplyOptions{PlanID: p.PlanID, Yes: true,
				BeforeWrite: func() error { started = true; return errors.New("write reached") }})
			if d := errs.Describe(err); d == nil || d.Code != "E454" {
				t.Fatalf("want E454, got %v", err)
			}
			if started || treeHash(t, f.root) != before {
				t.Fatal("refusal ran effects or wrote project files")
			}
		})
	}
}

func TestCertificateRuntimeNotice(t *testing.T) {
	for _, kind := range []EffectKind{EffectCertificates, EffectTrustStore} {
		t.Run(string(kind), func(t *testing.T) {
			f := loadFixture(t, "dev-minimal")
			var out bytes.Buffer
			req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Stderr: &out,
				Extra: []Effect{{Kind: kind, Target: "runtime target"}}}
			p, err := Compute(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if err := RenderHuman(&out, *p); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "certificate bytes and trust state are runtime effects outside artifact byte confirmation") {
				t.Fatalf("plan lacks runtime notice: %s", &out)
			}
			out.Reset()
			stop := errors.New("stop before effects")
			_, err = Apply(context.Background(), req, ApplyOptions{Yes: true, BeforeWrite: func() error { return stop }})
			if !errors.Is(err, stop) || !strings.Contains(out.String(), "certificate bytes and trust state are runtime effects outside artifact byte confirmation") {
				t.Fatalf("apply must disclose exclusions before effects: err %v, stderr %s", err, &out)
			}
		})
	}
}
