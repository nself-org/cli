package commands

// Purpose: register every import report's machine contract.
// Inputs: source registry. Outputs: v1.5-only envelope schemas for canonical import paths.
// Constraints: db import <name> exists only in v1.5 (move row from migrate <name>), so every
// path is also marked v1.5-only; the v1.4 tree has no node for it.
import "github.com/nself-org/cli/internal/importer"

func init() {
	names := []string{"supabase", "firebase"}
	for _, source := range importer.Sources() {
		names = append(names, source.Name())
	}
	for _, name := range names {
		key := "db import " + name
		if _, exists := jsonDataTypes[key]; !exists {
			registerJSONType(key, importer.Report{})
			registerV15OnlyEnvelope(key)
		}
	}
}
