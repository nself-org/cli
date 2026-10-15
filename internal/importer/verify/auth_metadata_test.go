package verify

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAuthMetadataNumbers(t *testing.T) {
	for _, tc := range []struct {
		name, want, got string
		equal           bool
	}{
		{"adjacent large integers", "9007199254740992", "9007199254740993", false},
		{"decimal exponent", "1e3", "1000.00", true},
		{"decimal fraction", "0.00100", "1e-3", true},
		{"negative zero", "-0.000e999999999", "0", true},
		{"huge exponent", "1e999999999999999999999", "10e999999999999999999998", true},
		{"huge exponent drift", "1e999999999999999999999", "1e999999999999999999998", false},
		{"string versus number", `"1000"`, "1000", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := map[string]any{"nested": []any{json.Number(tc.want)}}
			got := map[string]any{"nested": []any{json.Number(tc.got)}}
			if strings.HasPrefix(tc.want, `"`) {
				want["nested"] = []any{strings.Trim(tc.want, `"`)}
			}
			if sameMetadata(want, got) != tc.equal {
				t.Fatal("unexpected metadata equality")
			}
		})
	}
}

func TestAuthMetadataCanonicalMaps(t *testing.T) {
	if !sameMetadata(nil, map[string]any{}) {
		t.Fatal("legacy empty metadata differs")
	}
	if !sameMetadata(map[string]any{"b": 2, "a": 1}, map[string]any{"a": float64(1), "b": json.Number("2.00")}) {
		t.Fatal("map order or Go number type changed equality")
	}
	if sameMetadata(map[string]any{"unsupported": make(chan int)}, nil) {
		t.Fatal("invalid metadata accepted")
	}
}
