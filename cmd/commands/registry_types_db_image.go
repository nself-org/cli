package commands

import "github.com/nself-org/cli/internal/postgres/imageswitch"

// Purpose: register the image switch's envelope schema. Inputs: none.
// Outputs: db image switch data schema. Constraints: available in both mode trees.
func init() { registerJSONType("db image switch", imageswitch.Report{}) }
