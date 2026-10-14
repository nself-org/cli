package hasurameta

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/portable"
)

func TestFilterRetainsComputedFields(t *testing.T) {
	raw := []byte(`{"sources":[{"name":"default","kind":"postgres","tables":[{"table":{"schema":"public","name":"posts"},"computed_fields":[{"name":"word_count","definition":{"function":{"schema":"public","name":"post_word_count"},"table_argument":"post"},"comment":"Post word count"}]}]}]}`)
	db := portable.DB{Schemas: []string{"public"}, Tables: []portable.Table{{Schema: "public", Name: "posts"}}}
	out, _, exempt, err := Filter(raw, db)
	if err != nil || len(exempt) != 0 {
		t.Fatalf("filter: %v, exemptions: %v", err, exempt)
	}
	fields := func(b []byte) any {
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatal(err)
		}
		return doc["sources"].([]any)[0].(map[string]any)["tables"].([]any)[0].(map[string]any)["computed_fields"]
	}
	if !reflect.DeepEqual(fields(raw), fields(out)) {
		t.Fatalf("computed field definition lost: %s", out)
	}
}
