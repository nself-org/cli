package verify

// Purpose: compare function configuration and grants independent of export ordering.
// Inputs: decoded metadata fields. Outputs: semantic equality.
// Constraints: omitted configuration and permissions equal their empty forms.
import (
	"encoding/json"
	"reflect"
	"sort"
)

func equalFunctionField(key string, want, got any) bool {
	if key == "configuration" {
		empty := func(v any) any {
			if m, ok := v.(map[string]any); ok && len(m) == 0 {
				return nil
			}
			return v
		}
		return reflect.DeepEqual(empty(want), empty(got))
	}
	canonical := func(v any) any {
		if v == nil {
			return []string{}
		}
		a, ok := v.([]any)
		if !ok {
			return v
		}
		out := make([]string, 0, len(a))
		for _, item := range a {
			b, err := json.Marshal(item)
			if err != nil {
				return v
			}
			out = append(out, string(b))
		}
		sort.Strings(out)
		return out
	}
	return reflect.DeepEqual(canonical(want), canonical(got))
}
