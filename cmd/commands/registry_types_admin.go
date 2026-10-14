// JSON data-type registration for the admin canon fragment (the builtin
// `admin` family: admin, start, stop, health, logs, projects ...).
//
// Purpose: every runnable document command of the family answers with the v1
// envelope in v1.5 mode (P7-SURF-13). The builtin admin package owns the
// emission and exports the data types; the registry, schemas and goldens live
// here with the other fragments. `admin logs --follow` is a stream
// (contract:cli.json-stream v1) of admin.LogRecord lines; `admin connect` is
// interactive (an SSH tunnel that stays up) and `admin serve` is a stream; neither
// registers anything. `admin projects list` printed
// a JSON array before the contract: it stays in v1.4 mode and in the v1.4
// registry view (`json: legacy` in admin.yaml); the v1.5 envelope carries the
// same array.
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

import "github.com/nself-org/cli/internal/builtin/admin"

// adminEnvelopeTypes maps each path of the family to its envelope data.
var adminEnvelopeTypes = map[string]any{
	"admin":                 admin.OpenData{},
	"admin health":          admin.HealthData{},
	"admin logs":            admin.LogsData{},
	"admin projects":        admin.Hub{},
	"admin projects add":    admin.ProjectChange{},
	"admin projects list":   admin.ProjectList{},
	"admin projects remove": admin.ProjectChange{},
	"admin start":           admin.StartData{},
	"admin stop":            admin.StopData{},
}

func init() {
	for path, zero := range adminEnvelopeTypes {
		registerJSONType(path, zero)
		registerV15OnlyEnvelope(path)
	}
	registerJSONRecordType("admin logs", admin.LogRecord{})
}
