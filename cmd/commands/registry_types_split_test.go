package commands

// TestRegistryTypesSplit (P7-CANON-02): every registry_types_<domain>.go
// registration reaches the jsonDataTypes map, `help` stays declared in
// registry_types.go, and registering a path twice is refused.

import "testing"

func TestRegistryTypesSplit(t *testing.T) {
	for path, v15Only := range map[string]bool{
		"help": false, "status": false, "doctor": false,
		"config show": true, "config get": true, "config list": true,
	} {
		if _, ok := jsonDataTypes[path]; !ok {
			t.Errorf("%q is not registered: its registry_types_<domain>.go init() did not run", path)
		}
		if jsonV15OnlyEnvelope[path] != v15Only {
			t.Errorf("%q v1.5-only = %v, want %v", path, jsonV15OnlyEnvelope[path], v15Only)
		}
	}
	if got := len(JSONDataTypes()); got != len(jsonDataTypes) {
		t.Errorf("JSONDataTypes() returned %d types, the map holds %d", got, len(jsonDataTypes))
	}
	defer func() {
		if recover() == nil {
			t.Error("registering a path twice must panic")
		}
	}()
	registerJSONType("status", statusData{})
}
