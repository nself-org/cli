// JSON data-type registration for `help capabilities` (P7-SURF-03).
//
// Purpose: the envelope data of the self-description search.
// Constraints: registers from init(); see registry_types.go for the rules.
// The command exists in both compat modes, so the envelope is not v1.5-only.

package commands

func init() {
	registerJSONType("help capabilities", CapabilitiesResult{})
}

// CapabilityMatch is one hit of `help capabilities`: a command (Path set) or an
// installable plugin (Slug set), with the exact text that runs it.
type CapabilityMatch struct {
	Kind    string `json:"kind"` // command | plugin
	Path    string `json:"path,omitempty"`
	Slug    string `json:"slug,omitempty"`
	Summary string `json:"summary"`
	Run     string `json:"run"`
	Score   int    `json:"score"`
}

// CapabilitiesResult is the envelope data of `help capabilities --json`.
type CapabilitiesResult struct {
	Query   string            `json:"query"`
	Results []CapabilityMatch `json:"results"`
}
