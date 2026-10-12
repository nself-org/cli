package output

// Tests for contract:cli.json-stream v1 (P7-SURF-27). Every stream-*.json case
// shared with sdk/go/output (P7-SURF-28) must render to the same bytes here.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

const streamCasesGlob = "../../sdk/go/output/testdata/cases/stream-*.json"

// streamCase is the subset of a shared case that the stream writers read.
type streamCase struct {
	Name  string
	Input struct {
		Kind    string `json:"kind"`
		Command string `json:"command"`
		Type    string `json:"type"`
		Fields  []struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		} `json:"fields"`
		Meta  *Meta `json:"meta"`
		Error *struct {
			Code        string `json:"code"`
			Message     string `json:"message"`
			Cause       string `json:"cause"`
			Remediation string `json:"remediation"`
			DocsURL     string `json:"docs_url"`
			ExitCode    int    `json:"exit_code"`
			Class       string `json:"class"`
		} `json:"error"`
	} `json:"input"`
	Expected []string `json:"expected"`
}

func loadStreamCases(t *testing.T) []streamCase {
	t.Helper()
	files, err := filepath.Glob(streamCasesGlob)
	if err != nil || len(files) == 0 {
		t.Fatalf("no stream cases at %s (err=%v)", streamCasesGlob, err)
	}
	var out []streamCase
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var c streamCase
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		c.Name = strings.TrimSuffix(filepath.Base(f), ".json")
		out = append(out, c)
	}
	return out
}

// orderedObject marshals to a JSON object whose members keep the given order,
// the way a typed Go record keeps its field order.
type orderedObject struct {
	typ    string // leading "type" member, omitted when empty
	fields []struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	}
}

func (o orderedObject) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	sep := ""
	if o.typ != "" {
		k, _ := json.Marshal("type")
		v, _ := json.Marshal(o.typ)
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
		sep = ","
	}
	for _, f := range o.fields {
		k, err := json.Marshal(f.Key)
		if err != nil {
			return nil, err
		}
		b.WriteString(sep)
		b.Write(k)
		b.WriteByte(':')
		b.Write(f.Value)
		sep = ","
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func (c streamCase) object(withType bool) orderedObject {
	o := orderedObject{fields: c.Input.Fields}
	if withType {
		o.typ = c.Input.Type
	}
	return o
}

// sdkExit maps a case exit class to the internal/errs status (the status decides).
func sdkExit(class string) int {
	switch class {
	case "infra":
		return errs.ExitInfraError
	case "auth":
		return errs.ExitAuthError
	case "destructive_blocked":
		return errs.ExitDestructiveBlocked
	}
	return errs.ExitUserError
}

func (c streamCase) cliError() error {
	e := c.Input.Error
	code := e.ExitCode
	if code == 0 {
		code = sdkExit(e.Class)
	}
	cli := &errs.CLIError{Code: e.Code, What: e.Message, Why: e.Cause, Fix: e.Remediation,
		DocsPath: strings.TrimPrefix(e.DocsURL, "https://nself.org/docs/")}
	return &errs.ExitError{Code: code, Err: cli}
}

func addMeta(m *Meta) {
	if m == nil {
		return
	}
	for _, d := range m.Deprecations {
		AddDeprecation(d.Old, d.New, d.RemovalAt)
	}
	for _, w := range m.Warnings {
		AddWarning(w)
	}
}

func assertLines(t *testing.T, name string, got string, want []string) {
	t.Helper()
	if exp := strings.Join(want, "\n") + "\n"; got != exp {
		t.Fatalf("%s: stream bytes differ\n--- got ---\n%s--- want ---\n%s", name, got, exp)
	}
}

func TestStreamRecord(t *testing.T) {
	seen := 0
	for _, c := range loadStreamCases(t) {
		if c.Input.Kind != "stream_record" {
			continue
		}
		seen++
		c := c
		t.Run(c.Name, func(t *testing.T) {
			reset(t)
			var buf bytes.Buffer
			if err := StreamRecord(&buf, c.Input.Command, c.object(true)); err != nil {
				t.Fatal(err)
			}
			assertLines(t, c.Name, buf.String(), c.Expected)
		})
	}
	if seen < 2 {
		t.Fatalf("only %d stream_record cases found", seen)
	}
}

func TestStreamEnd(t *testing.T) {
	var ends, errsSeen int
	for _, c := range loadStreamCases(t) {
		c := c
		switch c.Input.Kind {
		case "stream_end":
			ends++
			t.Run(c.Name, func(t *testing.T) {
				reset(t)
				addMeta(c.Input.Meta)
				var buf bytes.Buffer
				if err := StreamEnd(&buf, c.Input.Command, c.object(false), nil); err != nil {
					t.Fatal(err)
				}
				assertLines(t, c.Name, buf.String(), c.Expected)
			})
		case "stream_error":
			errsSeen++
			t.Run(c.Name, func(t *testing.T) {
				reset(t)
				var buf bytes.Buffer
				// A result passed with an error is ignored.
				if err := StreamEnd(&buf, c.Input.Command, map[string]int{"ignored": 1}, c.cliError()); err != nil {
					t.Fatal(err)
				}
				assertLines(t, c.Name, buf.String(), c.Expected)
			})
		}
	}
	if ends < 2 || errsSeen < 2 {
		t.Fatalf("stream_end=%d stream_error=%d cases found", ends, errsSeen)
	}
}

type recFixture struct {
	Type string `json:"type"`
	N    int    `json:"n"`
}

type endFixture struct {
	Lines int `json:"lines"`
}

func TestStreamRecordTypedAndRefusals(t *testing.T) {
	reset(t)
	var buf bytes.Buffer
	if err := StreamRecord(&buf, "logs", recFixture{Type: "log", N: 2}); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), `{"schema_version":"1","command":"logs","data":{"type":"log","n":2}}`+"\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	bad := map[string]any{
		"empty type":    recFixture{Type: ""},
		"result type":   recFixture{Type: "result"},
		"not an object": []int{1},
		"nil":           nil,
		"number type": struct {
			Type int `json:"type"`
		}{Type: 3},
		"type not first":   struct{ N, Type any }{N: 1, Type: "log"},
		"unmarshalable":    map[string]any{"type": make(chan int)},
		"scalar":           "log",
		"empty object":     struct{}{},
		"null pointer rec": (*recFixture)(nil),
	}
	for name, rec := range bad {
		var b bytes.Buffer
		if err := StreamRecord(&b, "logs", rec); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if b.Len() != 0 {
			t.Errorf("%s: wrote %q on error", name, b.String())
		}
	}
	if err := StreamRecord(nil, "logs", recFixture{Type: "log"}); !errors.Is(err, errNoOut) {
		t.Errorf("nil writer: %v", err)
	}
}

func TestStreamEndTypedAndRefusals(t *testing.T) {
	reset(t)
	var buf bytes.Buffer
	if err := StreamEnd(&buf, "logs", endFixture{Lines: 5}, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), `{"schema_version":"1","command":"logs","data":{"type":"result","lines":5}}`+"\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	buf.Reset()
	if err := StreamEnd(&buf, "logs", (*endFixture)(nil), nil); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), `{"schema_version":"1","command":"logs","data":{"type":"result"}}`+"\n"; got != want {
		t.Fatalf("nil pointer result: got %q want %q", got, want)
	}
	for name, res := range map[string]any{"carries type": recFixture{Type: "x"}, "array": []int{1}, "scalar": 7} {
		var b bytes.Buffer
		if err := StreamEnd(&b, "logs", res, nil); err == nil || b.Len() != 0 {
			t.Errorf("%s: err=%v wrote=%q", name, err, b.String())
		}
	}
	if err := StreamEnd(nil, "logs", nil, nil); !errors.Is(err, errNoOut) {
		t.Errorf("nil writer: %v", err)
	}
}

type streamFailWriter struct{}

func (streamFailWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestStreamWriteFailureIsReturned(t *testing.T) {
	reset(t)
	if err := StreamRecord(streamFailWriter{}, "logs", recFixture{Type: "log"}); err == nil {
		t.Error("record: write failure swallowed")
	}
	if err := StreamEnd(streamFailWriter{}, "logs", nil, nil); err == nil {
		t.Error("end: write failure swallowed")
	}
	if err := StreamEnd(streamFailWriter{}, "logs", nil, errors.New("boom")); err == nil {
		t.Error("error line: write failure swallowed")
	}
}

func TestStreamEndErrorRedactsAndCompacts(t *testing.T) {
	reset(t)
	var buf bytes.Buffer
	if err := StreamEnd(&buf, "logs", nil, errors.New("plain failure")); err != nil {
		t.Fatal(err)
	}
	line := buf.String()
	if strings.Count(line, "\n") != 1 || strings.Contains(line, "  ") || !strings.Contains(line, `"error":{`) || strings.Contains(line, `"data"`) {
		t.Fatalf("not one compact error envelope: %q", line)
	}
	if err := StreamEnd(&buf, "logs", nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(buf.String(), "\n"); got != 2 {
		t.Fatalf("lines = %d, want 2", got)
	}
}
