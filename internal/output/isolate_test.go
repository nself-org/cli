package output

import (
	"os"
	"testing"
)

// TestIsolateStdout verifies the document seam, nested no-op, and restoration.
func TestIsolateStdout(t *testing.T) {
	real := os.Stdout
	errStream := os.Stderr
	restore := IsolateStdout()
	defer restore()
	if os.Stdout != errStream || Default().Out != real {
		t.Fatal("human output was not redirected while document output stayed on real stdout")
	}
	nested := IsolateStdout()
	nested()
	if os.Stdout != errStream || Default().Out != real {
		t.Fatal("nested isolation changed streams")
	}
	restore()
	restore()
	if os.Stdout != real || Default().Out != real {
		t.Fatal("restore did not reset both streams")
	}
}
