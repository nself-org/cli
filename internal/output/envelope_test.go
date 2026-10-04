package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestEmitDataGolden(t *testing.T) {
	reset(t)
	w, out, errb := bufs()
	if err := EmitData(w, "status", sampleValue); err != nil {
		t.Fatal(err)
	}
	golden(t, "success.golden.json", out.Bytes())
	if errb.Len() != 0 {
		t.Fatalf("EmitData wrote to Err: %q", errb.String())
	}
}

func TestEmitDataExactShape(t *testing.T) {
	reset(t)
	w, out, _ := bufs()
	if err := EmitData(w, "status", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"schema_version\": \"1\",\n  \"command\": \"status\",\n  \"data\": {\n    \"n\": 1\n  }\n}\n"
	if out.String() != want {
		t.Fatalf("got %q want %q", out.String(), want)
	}
}

func TestEmitDataNilIsNull(t *testing.T) {
	reset(t)
	w, out, _ := bufs()
	if err := EmitData(w, "doctor", nil); err != nil {
		t.Fatal(err)
	}
	golden(t, "nil-data.golden.json", out.Bytes())
	if !strings.Contains(out.String(), `"data": null`) {
		t.Fatalf("data null missing: %s", out.String())
	}
}

func TestHTMLNotEscapedInEnvelope(t *testing.T) {
	reset(t)
	w, out, _ := bufs()
	if err := EmitData(w, "x", sampleValue); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "<key> & value") || strings.Contains(s, `\u003c`) {
		t.Fatalf("HTML escaped in envelope: %s", s)
	}
}

func TestEmitErrorSentinelGolden(t *testing.T) {
	reset(t)
	w, out, errb := bufs()
	err := fmt.Errorf("start: %w", errs.ErrDockerNotRunning)
	if e := EmitError(w, "start", err); e != nil {
		t.Fatal(e)
	}
	golden(t, "error-sentinel.golden.json", out.Bytes())
	if errb.Len() != 0 {
		t.Fatalf("EmitError wrote to Err: %q", errb.String())
	}
	var doc map[string]json.RawMessage
	if e := json.Unmarshal(out.Bytes(), &doc); e != nil {
		t.Fatal(e)
	}
	if _, has := doc["data"]; has {
		t.Fatal("error envelope has a data key")
	}
	var d errs.Detail
	if e := json.Unmarshal(doc["error"], &d); e != nil || d.Code != "E002" {
		t.Fatalf("error.code = %q (err %v), want E002", d.Code, e)
	}
}

func TestEmitErrorPlainErrorIsE400(t *testing.T) {
	reset(t)
	w, out, _ := bufs()
	if e := EmitError(w, "", errors.New("boom")); e != nil {
		t.Fatal(e)
	}
	golden(t, "error-plain.golden.json", out.Bytes())
}

func TestEmitErrorNilWritesNothing(t *testing.T) {
	reset(t)
	w, out, _ := bufs()
	if err := EmitError(w, "x", nil); err == nil {
		t.Fatal("EmitError(nil) returned nil error")
	}
	if out.Len() != 0 {
		t.Fatalf("wrote %q for nil error", out.String())
	}
}

func TestExactlyOneDocumentKeyOrder(t *testing.T) {
	reset(t)
	AddWarning("w1")
	w, out, _ := bufs()
	if err := EmitData(w, "x", sampleValue); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(out.Bytes()))
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	if err := dec.Decode(&v); err != io.EOF {
		t.Fatalf("second document or trailing data: %v", err)
	}
	s := out.String()
	order := []string{`"schema_version"`, `"command"`, `"data"`, `"meta"`}
	last := -1
	for _, k := range order {
		i := strings.Index(s, k)
		if i <= last {
			t.Fatalf("key %s out of order in %s", k, s)
		}
		last = i
	}
	if !strings.HasSuffix(s, "}\n") || strings.HasSuffix(s, "\n\n") {
		t.Fatalf("want exactly one trailing newline: %q", s[len(s)-4:])
	}
}

func TestMarshalFailureWritesNothing(t *testing.T) {
	reset(t)
	w, out, _ := bufs()
	if err := EmitData(w, "x", make(chan int)); err == nil {
		t.Fatal("unmarshalable data returned nil error")
	}
	if out.Len() != 0 {
		t.Fatalf("partial output %q", out.String())
	}
}

func TestNilOutIsAnError(t *testing.T) {
	reset(t)
	if err := EmitData(Writer{}, "x", 1); err == nil {
		t.Fatal("nil Out returned nil error")
	}
	if err := EmitError(Writer{}, "x", errors.New("e")); err == nil {
		t.Fatal("nil Out returned nil error")
	}
}
