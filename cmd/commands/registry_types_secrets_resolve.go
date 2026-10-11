package commands

// Purpose: register moved secret documents for the v1.5 registry.
// Inputs: command results. Outputs: registry data schemas.
// Constraints: resolve remains CLI-only and emits values only through a FIFO.
// SPORT: P7-TRUST-03.
import "github.com/nself-org/cli/internal/secrets"

// secretResolveData carries the ordered v1 resolution document for an attempt.
type secretResolveData struct {
	Schema    string          `json:"schema"`
	AttemptID string          `json:"attempt_id"`
	Values    []secrets.Value `json:"values"`
}

// secretRecipientsData reports public recipients and whether the store changed.
type secretRecipientsData struct {
	File       string   `json:"file"`
	Recipients []string `json:"recipients"`
	Changed    bool     `json:"changed"`
}

func init() {
	registerJSONType("config secrets resolve", secretResolveData{})
	registerV15OnlyEnvelope("config secrets resolve")
	registerJSONType("config secrets recipients", dataHub{})
	registerV15OnlyEnvelope("config secrets recipients")
	for _, action := range []string{"add", "remove", "list"} {
		registerJSONType("config secrets recipients "+action, secretRecipientsData{})
		registerV15OnlyEnvelope("config secrets recipients " + action)
	}
}
