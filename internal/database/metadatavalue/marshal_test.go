package metadatavalue

import (
	"gopkg.in/yaml.v3"
	"reflect"
	"strings"
	"testing"
)

func TestMarshalPreservesIncludeResolverValues(t *testing.T) {
	for _, value := range []any{
		"one line", "keep terminal newline\n", "two\nnewlines\n\n",
		"literal !include file.yaml", map[string]any{"a": true},
		map[string]any{"a": "first", "b": "second"}, []any{"first", "second"},
	} {
		body, err := Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "!include") {
			t.Fatalf("resolver would interpret literal as directive: %s", body)
		}
		var got any
		if err := yaml.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, value) {
			t.Fatalf("roundtrip %q = %#v, want %#v", body, got, value)
		}
	}
}
