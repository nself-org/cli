package commands

import (
	"bytes"
	"testing"
)

func TestCanonIdentityWarning(t *testing.T) {
	for _, name := range []string{"ci", "functions"} {
		t.Run(name, func(t *testing.T) {
			resetCanonWarning()
			var b bytes.Buffer
			emitCanonWarning(&b, nil, []canonNote{{Old: name, New: name}}, "", "")
			if b.Len() != 0 {
				t.Fatalf("identity warning: %s", b.String())
			}
		})
	}
}
