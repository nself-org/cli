package invoke

import (
	"strings"
	"testing"
)

// An overlong number in args must not be echoed back in the E420 message:
// without UseNumber the decoder parses it as a float64 and puts all of its
// digits in the parse error.
func TestCheckNoNullItemsBoundsNumberError(t *testing.T) {
	raw := []byte(`["a", ` + strings.Repeat("9", 5000) + `]`)
	err := checkNoNullItems(raw, "args")
	if err != nil && len(err.Error()) > 500 {
		t.Fatalf("E420 message echoes the number: %d bytes", len(err.Error()))
	}
}

func TestCheckNoNullItemsStillRefusesNull(t *testing.T) {
	for _, raw := range []string{`[null]`, `["a", null]`} {
		if err := checkNoNullItems([]byte(raw), "args"); err == nil {
			t.Fatalf("%s: want an error", raw)
		}
	}
	if err := checkNoNullItems([]byte(`["a", 1, [null], {"k": null}]`), "args"); err != nil {
		t.Fatalf("nested null must be left to the typed decode: %v", err)
	}
}
