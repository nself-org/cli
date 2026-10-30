package invoke

// Purpose: read the facts the confirmation gate needs from a command's plan
// document (contract:cli.change-plan v1 inside the v1 envelope).
// Inputs: the plan exec's stdout. Outputs: planFacts (plan_id,
// requires_confirmation, whether the plan generates secrets, the raw data), or
// whether the child failed, or a short problem.
// Constraints: the plan is data from the child; a missing or malformed member
// is a problem, never a default. A plan that declares a secrets-persist,
// certificates or trust-store step changes something its plan id binds only
// by name (generated secrets, certificate bytes, the host trust store:
// reconcile binding.go and trust_binding.go), so the gate always shows it to
// the caller for consent (P7-SHIP-02).

import (
	"bytes"
	"encoding/json"
)

// consentEffects are the change-plan effect kinds that always need the
// caller's confirmation (reconcile.EffectSecretsPersist, EffectCertificates,
// EffectTrustStore).
var consentEffects = map[string]bool{"secrets-persist": true, "certificates": true, "trust-store": true}

// planFacts is what the gate reads from the child's plan document.
type planFacts struct {
	id       string
	requires bool
	secrets  bool // the plan declares a consentEffects step
	data     json.RawMessage
}

// parsePlan reads the plan exec's envelope. childErr is true when it is the
// child's own error envelope; problem is a short cause when plan_id or
// requires_confirmation is missing or invalid.
func parsePlan(stdout []byte) (f planFacts, childErr bool, problem string) {
	var doc struct {
		Data  json.RawMessage `json:"data"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout), &doc); err != nil {
		return f, false, "the plan output is not a JSON envelope"
	}
	if len(doc.Error) > 0 && string(doc.Error) != "null" {
		return f, true, ""
	}
	if d := bytes.TrimSpace(doc.Data); len(d) == 0 || d[0] != '{' {
		return f, false, "the plan output has no data object"
	}
	var data struct {
		PlanID   *string `json:"plan_id"`
		Requires *bool   `json:"requires_confirmation"`
		Effects  []struct {
			Kind string `json:"kind"`
		} `json:"effects"`
	}
	if err := json.Unmarshal(doc.Data, &data); err != nil {
		return f, false, "the plan output has no data object"
	}
	switch {
	case data.PlanID == nil:
		return f, false, "the plan output has no data.plan_id"
	case !confirmPattern.MatchString(*data.PlanID):
		return f, false, "data.plan_id is not 64 lowercase hex characters"
	case data.Requires == nil:
		return f, false, "the plan output has no data.requires_confirmation boolean"
	}
	secrets := false
	for _, e := range data.Effects {
		secrets = secrets || consentEffects[e.Kind]
	}
	return planFacts{id: *data.PlanID, requires: *data.Requires, secrets: secrets, data: doc.Data}, false, ""
}
