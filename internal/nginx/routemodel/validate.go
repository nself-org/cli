package routemodel

import (
	"fmt"
	"strings"
)

type legacyConflict string

func (e legacyConflict) Error() string { return string(e) }

// Validate reports duplicate generated routes. Legacy preflight claims keep
// their original refusal text; newly discovered claims warn in v1.4.
func Validate(m *Model, v15 bool) ([]string, error) {
	type claim struct{ id, name string }
	seen := map[string]claim{}
	legacy := map[string]claim{}
	var warnings, oldConflicts, newConflicts []string
	for _, r := range m.Routes {
		if len(r.ServerNames) == 0 {
			continue
		}
		name := r.ServerNames[0]
		if prev, ok := seen[name]; ok {
			newConflicts = append(newConflicts, fmt.Sprintf("%s and %s claim %s", prev.id, r.ID, name))
			if !v15 {
				warnings = append(warnings, fmt.Sprintf("duplicate nginx route %s: %s and %s; first route wins", name, prev.id, r.ID))
			}
		} else {
			seen[name] = claim{r.ID, name}
		}
		key := r.LegacyServerName
		if key == "" {
			key = name
		}
		if prev, ok := legacy[key]; ok {
			oldConflicts = append(oldConflicts, fmt.Sprintf("%s conflicts with %s (same server_name+location: %s/)", key, prev.name, key))
		} else {
			legacy[key] = claim{r.ID, key}
		}
	}
	if v15 && len(newConflicts) > 0 {
		return nil, fmt.Errorf("E055 duplicate nginx route detected: %s", strings.Join(newConflicts, "; "))
	}
	if !v15 && len(oldConflicts) > 0 {
		return nil, legacyConflict(fmt.Sprintf("nginx domain conflict detected:\n  %s\n", strings.Join(oldConflicts, "\n  ")))
	}
	if !v15 {
		// Legacy claims can disagree with emitted claims when APP_NAME is set.
		// One warning per actual duplicate is enough for operators.
		return warnings, nil
	}
	return nil, nil
}
