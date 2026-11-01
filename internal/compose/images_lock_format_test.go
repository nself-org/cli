package compose

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarshalLockCompactEntriesPreserveEscapedHeaders(t *testing.T) {
	f := &LockFile{Generated: "quoted \"header\", with <markup>", SchemaVersion: "1"}
	ref, ok := LockedRef("golang")
	if !ok {
		t.Fatal("missing scaffold toolchain entry")
	}
	f.Images = []Ref{ref, ref}
	raw, err := MarshalLock(f)
	if err != nil {
		t.Fatal(err)
	}
	var got LockFile
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Generated != f.Generated || len(got.Images) != len(f.Images) {
		t.Fatalf("lock changed during marshal: %+v", got)
	}
	if strings.Contains(string(raw), `\u003c`) {
		t.Fatal("lock must preserve HTML characters")
	}
	if lines := strings.Count(string(raw), "\n"); lines != len(f.Images)+6 {
		t.Errorf("got %d lines, want one per entry plus envelope", lines)
	}
}
