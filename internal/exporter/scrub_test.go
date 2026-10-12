package exporter

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("fake read failure") }

func TestScrubStreaming(t *testing.T) {
	secret := "FAKE-EXPORT-SECRET-ONLY"
	for _, offset := range []int{0, 32760, 32767, 65530} {
		hit, err := scan(strings.NewReader(strings.Repeat("x", offset)+secret+"suffix"), secretPatterns([]string{secret}))
		if !hit || err != nil {
			t.Fatalf("miss at %d: %v", offset, err)
		}
	}
	if hit, err := scan(strings.NewReader("innocent text"), secretPatterns([]string{secret})); hit || err != nil {
		t.Fatal("false positive", err)
	}
	if _, err := scan(brokenReader{}, secretPatterns([]string{secret})); err == nil {
		t.Fatal("read error accepted")
	}
	// The minimum is a contract boundary, not a silent whole-scan bypass.
	if len(secretPatterns([]string{"short"})) != 0 {
		t.Fatal("short value included")
	}
}

func TestScrubEncodedSQL(t *testing.T) {
	for _, tc := range []struct{ secret, text string }{
		{"FAKE'QUOTE", "COMMENT ON TABLE t IS 'FAKE''QUOTE';"},
		{"FAKE\nLINE", "COPY t FROM stdin;\nFAKE\\nLINE\n"},
		{"FAKE@URL", "postgres://fake:FAKE%40URL@localhost/fake"},
		{"FAKEBYTES", "\\x46414b454259544553"},
	} {
		hit, err := scan(strings.NewReader(tc.text), secretPatterns([]string{tc.secret}))
		if !hit || err != nil {
			t.Fatal("encoded secret missed", err)
		}
	}
}

func TestScrubUsers(t *testing.T) {
	secret := "FAKE-EXPORT-SECRET-ONLY"
	for _, doc := range []string{`{"password_hash":"$2b$FAKE-EXPORT-SECRET-ONLY","metadata":{"private":"FAKE-EXPORT-SECRET-ONLY"}}`, `{"password_hash":null,"email":"FAKE-EXPORT-SECRET-ONLY"}`} {
		hit, err := scanUsers(strings.NewReader(doc), secretPatterns([]string{secret}))
		if !hit || err != nil {
			t.Fatal("metadata secret missed", err)
		}
	}
	hit, err := scanUsers(strings.NewReader(`{"password_hash":"$2b$FAKE-EXPORT-SECRET-ONLY","email":"fake@example.invalid"}`), secretPatterns([]string{secret}))
	if hit || err != nil {
		t.Fatal("bcrypt exemption failed", err)
	}
	if _, err = scanUsers(strings.NewReader(`{"broken":`), nil); err == nil {
		t.Fatal("malformed auth accepted")
	}
	for _, secret := range []string{"FAKE\"QUOTE", "FAKE\nNEWLINE", "FAKE\\SLASH", "FAKE<ANGLE"} {
		var b bytes.Buffer
		writeJSON(t, &b, map[string]any{"metadata": secret})
		hit, err := scanUsers(&b, secretPatterns([]string{secret}))
		if !hit || err != nil {
			t.Fatalf("escaped secret missed: %v", err)
		}
	}
}

func TestPreflightRefusals(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "bundle")
	if err := emptyOutput(out); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := emptyOutput(out); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "keep"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertCode(t, emptyOutput(out), "E527")
	assertCode(t, diskCheck(out, 100, 40, func(string) (uint64, error) { return 159, nil }), "E525")
	if err := diskCheck(out, 100, 40, func(string) (uint64, error) { return 160, nil }); err != nil {
		t.Fatal(err)
	}
	if err := diskCheck(out, 100, 0, func(string) (uint64, error) { return 0, io.ErrUnexpectedEOF }); err == nil {
		t.Fatal("probe error accepted")
	}
	assertCode(t, diskCheck(out, ^uint64(0), 0, nil), "E525")
	if b, err := os.ReadFile(filepath.Join(out, "keep")); err != nil || string(b) != "fixture" {
		t.Fatal("preflight wrote output")
	}
	if err := os.Symlink(dir, filepath.Join(dir, "link")); err == nil {
		assertCode(t, emptyOutput(filepath.Join(dir, "link")), "E527")
	}
	err := secretFound("db/data.dump")
	assertCode(t, err, "E526")
	if strings.Contains(err.Error(), "FAKE-EXPORT") {
		t.Fatal("error leaks")
	}
}

func assertCode(t *testing.T, err error, want string) {
	t.Helper()
	var e *errs.CLIError
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("want %s got %v", want, err)
	}
}
