package invoke

// Purpose: an overlong number in args or argv is refused with a bounded E420.
// Inputs: requests whose array holds a 5000-digit number.
// Outputs: test failures when the refusal echoes the digits.
// Constraints: encoding/json's Token() without UseNumber fails on a number
// that overflows float64 and puts the whole literal in the error text
// (regression for D-1113).

import (
	"strings"
	"testing"
)

func TestOverlongNumberItemRefusedBounded(t *testing.T) {
	digits := strings.Repeat("9", 5000)
	for _, member := range []string{"args", "argv"} {
		body := `{"` + member + `":[` + digits + `]}`
		_, err := decodeFast(t, body)
		wantE420(t, err, member+" with a 5000-digit number")
		if err != nil && strings.Contains(err.Error(), digits[:100]) {
			t.Errorf("%s: the refusal echoes the number", member)
		}
	}
}
