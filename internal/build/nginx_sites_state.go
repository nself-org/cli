package build

import (
	"encoding/json"
	"fmt"
	"os"
)

// A marker proves provenance, not ownership: copied marked files must survive
// orphan pruning unless this project's generated-state record owns their path.
func recordedSiteFiles(sink Sink) (map[string]string, error) {
	data, err := sink.ReadFile(".nself/state/generated.json")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading generated-state ownership: %w", err)
	}
	var state struct {
		SchemaVersion string            `json:"schema_version"`
		Files         map[string]string `json:"files"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("reading generated-state ownership: %w", err)
	}
	if state.SchemaVersion != "1" {
		return nil, fmt.Errorf("unsupported generated-state ownership schema %q", state.SchemaVersion)
	}
	return state.Files, nil
}
