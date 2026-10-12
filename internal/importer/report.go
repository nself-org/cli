package importer

// Purpose: credential-free import outcome matching contract:cli.importer v1.
// Inputs: verifier observations. Outputs: JSON on disk and the command envelope.
import (
	"github.com/nself-org/cli/internal/importer/verify"
	"github.com/nself-org/cli/internal/portable"
)

type TableReport struct {
	Schema     string `json:"schema"`
	Name       string `json:"name"`
	SourceRows int64  `json:"source_rows"`
	TargetRows int64  `json:"target_rows"`
	Equal      bool   `json:"equal"`
}
type UsersReport struct {
	Source        int         `json:"source"`
	Target        int         `json:"target"`
	Equal         bool        `json:"equal"`
	ResetRequired []ResetUser `json:"reset_required"`
}
type ResetUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}
type StorageReport struct {
	SourceObjects int  `json:"source_objects"`
	TargetObjects int  `json:"target_objects"`
	Equal         bool `json:"equal"`
}
type HasuraReport struct {
	Equal bool              `json:"equal"`
	Diffs []verify.Mismatch `json:"diffs"`
}
type Policy struct {
	Table  string `json:"table"`
	Policy string `json:"policy"`
	Reason string `json:"reason"`
}
type Report struct {
	Source               string               `json:"source"`
	RunID                string               `json:"run_id"`
	Result               string               `json:"result"`
	Tables               []TableReport        `json:"tables"`
	Users                UsersReport          `json:"users"`
	Storage              StorageReport        `json:"storage"`
	Hasura               HasuraReport         `json:"hasura"`
	UntranslatedPolicies []Policy             `json:"untranslated_policies"`
	Exemptions           []portable.Exemption `json:"exemptions"`
	Mismatches           []verify.Mismatch    `json:"mismatches"`
	Steps                []string             `json:"steps,omitempty"`
	// Wrote is true once the target may have changed; LastStep is the last finished load step.
	Wrote     bool       `json:"wrote"`
	LastStep  string     `json:"last_step,omitempty"`
	Detection *Detection `json:"detection,omitempty"`
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
