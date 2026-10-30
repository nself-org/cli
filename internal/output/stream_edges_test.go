package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// unmarshalable cannot be encoded to JSON (channels have no representation).
type unmarshalable struct {
	Type string   `json:"type"`
	Bad  chan int `json:"bad"`
}

func TestRedactTextExported(t *testing.T) {
	got := RedactText("dial " + fixtureDSN + " failed")
	if strings.Contains(got, fixturePassword) {
		t.Fatalf("RedactText leaks the DSN password: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("RedactText left no redaction marker: %q", got)
	}
	if plain := RedactText("nothing secret here"); plain != "nothing secret here" {
		t.Fatalf("RedactText changed plain text: %q", plain)
	}
}

func TestStreamRecordRejectsBadInput(t *testing.T) {
	reset(t)
	cases := []struct {
		name string
		rec  any
		want string
	}{
		{"unencodable", unmarshalable{Type: "x"}, "json marshal"},
		{"not an object", []int{1, 2}, "stream record"},
		{"empty object", struct{}{}, `first member must be "type"`},
		{"nil record", nil, `first member must be "type"`},
		{"type not first", map[string]any{"a": 1, "type": "x"}, `first member must be "type"`},
		{"type not a string", map[string]any{"type": 7}, "not a string"},
		{"type empty", map[string]any{"type": ""}, "empty"},
		{"type reserved", map[string]any{"type": ResultType}, "reserved"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := StreamRecord(&buf, "x", tc.rec)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error containing %q", err, tc.want)
			}
			if buf.Len() != 0 {
				t.Fatalf("wrote %q despite the error", buf.String())
			}
		})
	}
}

func TestStreamRecordWriteFailures(t *testing.T) {
	reset(t)
	rec := recFixture{Type: "progress"}
	if err := StreamRecord(nil, "x", rec); !errors.Is(err, errNoOut) {
		t.Fatalf("nil writer: got %v, want errNoOut", err)
	}
	fw := &failWriter{}
	if err := StreamRecord(fw, "x", rec); !errors.Is(err, errDiskFull) {
		t.Fatalf("failing writer: got %v, want the writer's error", err)
	}
	if fw.calls != 1 {
		t.Fatalf("line written in %d calls, want 1", fw.calls)
	}
}

func TestStreamEndRejectsBadResult(t *testing.T) {
	reset(t)
	cases := []struct {
		name   string
		result any
		want   string
	}{
		{"unencodable", unmarshalable{}, "stream end"},
		{"not an object", "text", "stream end"},
		{"carries type", map[string]any{"type": "x"}, `must not carry a "type"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := StreamEnd(&buf, "x", tc.result, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error containing %q", err, tc.want)
			}
			if buf.Len() != 0 {
				t.Fatalf("wrote %q despite the error", buf.String())
			}
		})
	}
}

func TestStreamEndBareAndFailing(t *testing.T) {
	reset(t)
	var buf bytes.Buffer
	if err := StreamEnd(&buf, "x", nil, nil); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("bare result is not JSON: %v\n%s", err, buf.String())
	}
	if len(doc.Data) != 1 || doc.Data["type"] != ResultType {
		t.Fatalf("bare result data = %v, want only type=result", doc.Data)
	}
	if err := StreamEnd(&failWriter{}, "x", map[string]any{"ok": true}, nil); !errors.Is(err, errDiskFull) {
		t.Fatalf("success line: got %v, want the writer's error", err)
	}
	if err := StreamEnd(&failWriter{}, "x", nil, errors.New("boom")); !errors.Is(err, errDiskFull) {
		t.Fatalf("error line: got %v, want the writer's error", err)
	}
	if err := StreamEnd(nil, "x", nil, errors.New("boom")); !errors.Is(err, errNoOut) {
		t.Fatalf("error line, nil writer: got %v, want errNoOut", err)
	}
}

func TestObjectOfNullPointer(t *testing.T) {
	var p *struct{ A int }
	raw, keys, err := objectOf(p)
	if err != nil || raw != nil || len(keys) != 0 {
		t.Fatalf("typed nil: raw=%q keys=%v err=%v, want an empty description", raw, keys, err)
	}
	raw, keys, err = objectOf(map[string]any{"z": 1})
	if err != nil || string(raw) != `{"z":1}` || len(keys) != 1 || keys[0] != "z" {
		t.Fatalf("object: raw=%q keys=%v err=%v", raw, keys, err)
	}
}

func TestWriteBareFailures(t *testing.T) {
	if err := writeBare(Writer{}, 1); !errors.Is(err, errNoOut) {
		t.Fatalf("nil Out: got %v, want errNoOut", err)
	}
	var buf bytes.Buffer
	if err := writeBare(Writer{Out: &buf}, make(chan int)); err == nil || !strings.Contains(err.Error(), "json marshal") {
		t.Fatalf("unencodable value: got %v, want a json marshal error", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %q despite the error", buf.String())
	}
	if err := writeBare(Writer{Out: &buf}, map[string]string{"k": "<v>"}); err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"k\": \"\\u003cv\\u003e\"\n}\n"; buf.String() != want {
		t.Fatalf("bare output = %q, want %q (HTML escaping on, indent 2)", buf.String(), want)
	}
}
