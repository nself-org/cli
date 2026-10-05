package output

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shared case file format, also read by the CLI's internal/repoqa test.
type caseFile struct {
	Input struct {
		Kind    string          `json:"kind"`
		GoType  string          `json:"go_type"`
		Command string          `json:"command"`
		Type    string          `json:"type"`
		Data    json.RawMessage `json:"data"`
		Fields  []struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		} `json:"fields"`
		Meta  *Meta        `json:"meta"`
		Error *ErrorDetail `json:"error"`
	} `json:"input"`
	Expected    []string `json:"expected"`
	ExpectError bool     `json:"expect_error"`
}

// typedSample is rendered as typed Go data (struct field order, nil versus
// empty slices and maps); the CLI's repoqa test defines the same type.
type typedSample struct {
	Zed        string         `json:"zed"`
	Alpha      int            `json:"alpha"`
	Note       string         `json:"note"`
	NilSlice   []string       `json:"nil_slice"`
	EmptySlice []string       `json:"empty_slice"`
	NilMap     map[string]int `json:"nil_map"`
	EmptyMap   map[string]int `json:"empty_map"`
	Skip       string         `json:"skip,omitempty"`
	Ptr        *int           `json:"ptr"`
	Inner      struct {
		B int `json:"b"`
		A int `json:"a"`
	} `json:"inner"`
}

func loadCases(t *testing.T) map[string]caseFile {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "cases", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no cases found (err=%v)", err)
	}
	out := map[string]caseFile{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var c caseFile
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out[filepath.Base(f)] = c
	}
	return out
}

func anyOf(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func render(t *testing.T, c caseFile) ([]byte, error) {
	t.Helper()
	in := c.Input
	var fields []Field
	for _, f := range in.Fields {
		fields = append(fields, Field{Key: f.Key, Value: anyOf(t, f.Value)})
	}
	var b []byte
	var err error
	switch in.Kind {
	case "data":
		var data any = anyOf(t, in.Data)
		switch in.GoType {
		case "":
		case "typedSample":
			var ts typedSample
			if err := json.Unmarshal(in.Data, &ts); err != nil {
				t.Fatal(err)
			}
			data = ts
		default:
			t.Fatalf("unknown go_type %q", in.GoType)
		}
		b, err = Data(in.Command, data, in.Meta)
	case "error":
		b, err = Error(in.Command, *in.Error, in.Meta)
	case "stream_record":
		b, err = StreamRecord(in.Command, in.Type, fields...)
	case "stream_end":
		b, err = StreamEnd(in.Command, in.Meta, fields...)
	case "stream_error":
		b, err = StreamError(in.Command, *in.Error, in.Meta)
	default:
		t.Fatalf("unknown case kind %q", in.Kind)
	}
	return b, err
}

func TestCases(t *testing.T) {
	for name, c := range loadCases(t) {
		got, err := render(t, c)
		if c.ExpectError {
			if err == nil || got != nil {
				t.Errorf("%s: want a refusal and no bytes, got %q err=%v", name, got, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		want := strings.Join(c.Expected, "\n") + "\n"
		if string(got) != want {
			t.Errorf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
		}
		if err := CheckEnvelope(got); err != nil {
			t.Errorf("%s: CheckEnvelope rejects its own output: %v", name, err)
		}
	}
}

func TestCheckEnvelopeFixtures(t *testing.T) {
	for dir, wantOK := range map[string]bool{"valid": true, "invalid": false} {
		files, _ := filepath.Glob(filepath.Join("testdata", dir, "*.json"))
		if len(files) == 0 {
			t.Fatalf("no %s fixtures", dir)
		}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if err := CheckEnvelope(b); (err == nil) != wantOK {
				t.Errorf("%s: CheckEnvelope = %v, want ok=%v", f, err, wantOK)
			}
		}
	}
}

func TestCheckEnvelopeRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "nope", `{"schema_version":"1","command":"x","data":1} {}`, `{"schema_version":"1","command":"x","data":1`} {
		if CheckEnvelope([]byte(s)) == nil {
			t.Errorf("accepted %q", s)
		}
	}
}

func TestExitClasses(t *testing.T) {
	for class, want := range map[string]int{ClassUser: 1, ClassInfra: 2, ClassAuth: 3, ClassDestructiveBlocked: 4, ClassOther: 1, "": 1, "weird": 1} {
		if got := ExitCodeFor(class); got != want {
			t.Errorf("ExitCodeFor(%q) = %d, want %d", class, got, want)
		}
	}
	for code, want := range map[int]string{1: "user", 2: "infra", 3: "auth", 4: "destructive_blocked", 5: "other", 70: "other"} {
		if got := ClassFor(code); got != want {
			t.Errorf("ClassFor(%d) = %q, want %q", code, got, want)
		}
	}
}

// TestExitClassTable pins ExitCodeFor and ClassFor to testdata/exit-classes.json,
// the table the CLI's repoqa test checks against internal/errs.
func TestExitClassTable(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "exit-classes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tab struct {
		Classes []struct {
			Class string `json:"class"`
			Exit  int    `json:"exit"`
		} `json:"classes"`
		Other struct {
			Class       string `json:"class"`
			ExitExample int    `json:"exit_example"`
		} `json:"other"`
	}
	if err := json.Unmarshal(b, &tab); err != nil || len(tab.Classes) != 4 {
		t.Fatalf("bad table: %v", err)
	}
	for _, r := range tab.Classes {
		if got := ExitCodeFor(r.Class); got != r.Exit {
			t.Errorf("ExitCodeFor(%q) = %d, table %d", r.Class, got, r.Exit)
		}
		if got := ClassFor(r.Exit); got != r.Class {
			t.Errorf("ClassFor(%d) = %q, table %q", r.Exit, got, r.Class)
		}
	}
	if got := ClassFor(tab.Other.ExitExample); got != tab.Other.Class {
		t.Errorf("ClassFor(%d) = %q, table %q", tab.Other.ExitExample, got, tab.Other.Class)
	}
}

func TestErrorDerivesExitAndClass(t *testing.T) {
	b, err := Error("x", ErrorDetail{Code: "E410", Message: "m", Class: ClassInfra}, nil)
	if err != nil || !strings.Contains(string(b), `"exit_code": 2`) {
		t.Fatalf("exit not derived from class: %v %s", err, b)
	}
	b, err = Error("x", ErrorDetail{Code: "E430", Message: "m", ExitCode: 4}, nil)
	if err != nil || !strings.Contains(string(b), `"class": "destructive_blocked"`) {
		t.Fatalf("class not derived from exit: %v %s", err, b)
	}
	if _, err = Error("x", ErrorDetail{Message: "m"}, nil); err == nil {
		t.Fatal("accepted a detail without a code")
	}
	if _, err = StreamError("x", ErrorDetail{Code: "E400", Message: "m", ExitCode: 300}, nil); err == nil {
		t.Fatal("StreamError accepted exit code 300")
	}
}

type ordered struct {
	Zed   string `json:"zed"`
	Alpha int    `json:"alpha"`
}

func TestStructDataKeepsFieldOrder(t *testing.T) {
	b, err := Data("x", ordered{Zed: "z", Alpha: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"schema_version\": \"1\",\n  \"command\": \"x\",\n  \"data\": {\n    \"zed\": \"z\",\n    \"alpha\": 1\n  }\n}\n"
	if string(b) != want {
		t.Fatalf("got\n%s", b)
	}
}

func TestStreamRejectsBadRecords(t *testing.T) {
	if _, err := StreamRecord("x", ""); err == nil {
		t.Error("empty type accepted")
	}
	if _, err := StreamRecord("x", "result"); err == nil {
		t.Error("result type accepted for a record")
	}
	if _, err := StreamRecord("x", "log", Field{"type", 1}); err == nil {
		t.Error("field named type accepted")
	}
	if _, err := StreamRecord("x", "log", Field{"a", 1}, Field{"a", 2}); err == nil {
		t.Error("repeated field accepted")
	}
	if _, err := StreamRecord("x", "log", Field{"", 1}); err == nil {
		t.Error("empty field key accepted")
	}
	if _, err := StreamRecord("x", "log", Field{"a", make(chan int)}); err == nil {
		t.Error("unmarshalable value accepted")
	}
}

func TestWriteHelpersWriteOnce(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteData(&buf, "x", 1, nil); err != nil {
		t.Fatal(err)
	}
	if err := WriteError(&buf, "x", ErrorDetail{Code: "E400", Message: "m", Class: ClassUser}, nil); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "\n}\n"); n != 2 {
		t.Fatalf("want two documents, got %d:\n%s", n, buf.String())
	}
}

// TestStdlibOnly: no non-test file imports outside the standard library.
func TestStdlibOnly(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	n := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		n++
		b, _ := os.ReadFile(f)
		for _, line := range strings.Split(string(b), "\n") {
			l := strings.TrimSpace(line)
			if strings.HasPrefix(l, `"`) && strings.Contains(l, ".") {
				t.Errorf("%s imports a non-stdlib path: %s", f, l)
			}
		}
	}
	if n == 0 {
		t.Fatal("no source files scanned")
	}
}
