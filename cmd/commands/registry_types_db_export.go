package commands

import "github.com/nself-org/cli/internal/exporter"

// Purpose: register portable export's envelope schema. Inputs: none.
// Outputs: db export data schema. Constraints: available in both mode trees.
func init() { registerJSONType("db export", exporter.Summary{}) }
