package commands

// Purpose: TestDeployStreamLinesUnchanged pins the exact bytes of the deploy
// progress stream, so moving the line writer onto output.StreamRecord and
// output.StreamEnd (P7-SURF-15) cannot change a single byte on the wire.
// The expected lines were captured from the writer before the move.

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	dprog "github.com/nself-org/cli/internal/deploy/progress"
)

func TestDeployStreamLinesUnchanged(t *testing.T) {
	clock := time.Unix(1700000000, 0)
	now := func() time.Time { clock = clock.Add(5 * time.Millisecond); return clock }
	var buf bytes.Buffer
	w := dprog.NewWriterClock(&buf, "deploy", now)
	w.At("prod", "").Event("Build images", dprog.PhaseStart, "")
	w.At("prod", "").Status("Build images", "done", "")
	w.At("prod", "web-1").Status("Deploy web-1", "failed", "ssh: token=abc123 refused <&>\nsecond line")
	w.Status("Deploy pipeline", "pending", "planned, not run (dry-run)")
	if err := w.Finish(dprog.ResultFailed, nil); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{"schema_version":"1","command":"deploy","data":{"type":"progress","seq":1,"step":"Build images","phase":"start","env":"prod","server":null,"elapsed_ms":5,"detail":null}}`,
		`{"schema_version":"1","command":"deploy","data":{"type":"progress","seq":2,"step":"Build images","phase":"done","env":"prod","server":null,"elapsed_ms":10,"detail":null}}`,
		`{"schema_version":"1","command":"deploy","data":{"type":"progress","seq":3,"step":"Deploy web-1","phase":"failed","env":"prod","server":"web-1","elapsed_ms":15,"detail":"ssh: token=[REDACTED] refused <&>"}}`,
		`{"schema_version":"1","command":"deploy","data":{"type":"progress","seq":4,"step":"Deploy pipeline","phase":"skipped","env":null,"server":null,"elapsed_ms":20,"detail":"planned, not run (dry-run)"}}`,
		`{"schema_version":"1","command":"deploy","data":{"type":"result","result":"failed","steps":[{"step":"Build images","status":"done"},{"step":"Deploy web-1","status":"failed"},{"step":"Deploy pipeline","status":"pending"}]}}`,
	}
	got := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), buf.String())
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d changed:\n got %s\nwant %s", i, got[i], want[i])
		}
	}

	// A broken sink stops the stream after the first failed write.
	bad := dprog.NewWriterClock(failWriter{}, "deploy", now)
	bad.Event("x", dprog.PhaseStart, "")
	if bad.Err() == nil {
		t.Fatal("write error was not recorded")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }
