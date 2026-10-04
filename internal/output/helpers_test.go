package output

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata golden files")

// golden compares got with testdata/<name>, or rewrites it under -update.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", name, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// bufs returns a Writer over two fresh buffers.
func bufs() (Writer, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return Writer{Out: &out, Err: &errb}, &out, &errb
}

// reset clears process state before and after a test.
func reset(t *testing.T) {
	t.Helper()
	ResetState()
	t.Cleanup(ResetState)
}

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// stripANSI removes colour codes so goldens do not depend on TTY detection.
func stripANSI(b []byte) []byte { return ansi.ReplaceAll(b, nil) }

// sample is the data value used by the golden tests; Note holds HTML-sensitive
// characters on purpose.
type sample struct {
	Name  string   `json:"name"`
	Note  string   `json:"note"`
	Items []string `json:"items"`
}

var sampleValue = sample{Name: "demo", Note: "<key> & value", Items: []string{"a", "b"}}
