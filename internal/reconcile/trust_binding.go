package reconcile

// trust_binding.go — which certificates and trust-store steps a plan id may
// bind (P7-SHIP-02; E454 keeps refusing the rest).
//
// Purpose: a first local build issues a development certificate and installs
// the mkcert CA. Both are named in the plan (domains, CA fingerprint), so the
// plan id binds what the user consented to: a CA replaced or a domain added
// after the plan changes the id (E450). Everything else stays refused.
// Inputs: the computed plan and the project dir. Outputs: true when every
// certificates and trust-store effect is a local development step.
// Constraints: a prod-class env is never bindable; the step test itself is
// trustplan.LocalDevStep, next to the code that writes the details.

import "github.com/nself-org/cli/internal/build/trustplan"

// bindableTrust reports whether a plan id may bind p's certificate and trust
// effects: none, or all local development steps on a dev-class env.
func bindableTrust(p *Plan, projectDir string) bool {
	if !certificateEffects(p) {
		return true
	}
	if EnvClass(p.Env) != ClassDev {
		return false
	}
	for _, e := range p.Effects {
		if e.Kind != EffectCertificates && e.Kind != EffectTrustStore {
			continue
		}
		if !trustplan.LocalDevStep(string(e.Kind), e.Target, e.Detail, projectDir) {
			return false
		}
	}
	return true
}
