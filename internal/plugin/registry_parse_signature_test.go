package plugin

import (
	"encoding/json"
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
)

// TestRegistryParseReleaseSignature: object decode, an absent field (nil) and
// a wrong type (error), through parseReleaseSignature and a whole registry.
func TestRegistryParseReleaseSignature(t *testing.T) {
	rel, err := parseReleaseSignature(json.RawMessage(`{"key_id":"plugins-0123456789abcdef","alg":"ed25519","sig":"c2ln"}`))
	if err != nil || rel == nil || *rel != (signature.Release{KeyID: "plugins-0123456789abcdef", Alg: "ed25519", Sig: "c2ln"}) {
		t.Fatalf("object: rel=%+v err=%v", rel, err)
	}
	for name, raw := range map[string]json.RawMessage{"absent": nil, "empty": json.RawMessage(``), "blank": json.RawMessage(`  `), "null": json.RawMessage(`null`)} {
		if rel, err := parseReleaseSignature(raw); rel != nil || err != nil {
			t.Errorf("%s: rel=%+v err=%v, want nil, nil", name, rel, err)
		}
	}
	for name, raw := range map[string]string{"string": `"sig"`, "array": `[]`, "number": `7`, "bool": `false`, "field type": `{"key_id":1}`} {
		if rel, err := parseReleaseSignature(json.RawMessage(raw)); err == nil || rel != nil {
			t.Errorf("%s: rel=%+v err=%v, want an error", name, rel, err)
		}
	}

	body := `{"plugins":[
	  {"name":"a","version":"1.0.0","release_signature":{"key_id":"plugins-0123456789abcdef","alg":"ed25519","sig":"c2ln"}},
	  {"name":"b","version":"1.0.0"},
	  {"name":"c","version":"1.0.0","release_signature":null},
	  {"name":"d","version":"1.0.0","release_signature":"mangled"}]}`
	reg, err := parseRegistryJSON([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*signature.Release{}
	for i := range reg.Plugins {
		got[reg.Plugins[i].Name] = reg.Plugins[i].ReleaseSignature
	}
	if got["a"] == nil || got["a"].KeyID != "plugins-0123456789abcdef" {
		t.Errorf("a: %+v", got["a"])
	}
	if got["b"] != nil || got["c"] != nil {
		t.Errorf("absent and null must be nil: b=%+v c=%+v", got["b"], got["c"])
	}
	if got["d"] == nil || *got["d"] != (signature.Release{}) {
		t.Errorf("a mangled value must be a non-nil empty Release: %+v", got["d"])
	}
	// Cache round trip: absent stays absent, empty stays empty.
	data, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	again, err := parseRegistryJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	for i := range again.Plugins {
		name := again.Plugins[i].Name
		switch rel := again.Plugins[i].ReleaseSignature; {
		case got[name] == nil && rel != nil, got[name] != nil && (rel == nil || *rel != *got[name]):
			t.Errorf("%s changed in the cache round trip: before %+v after %+v", name, got[name], rel)
		}
	}
}
