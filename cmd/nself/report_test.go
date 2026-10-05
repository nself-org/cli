package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
)

// silentErr is an error that asks main not to print it again.
type silentErr struct{ code int }

func (s silentErr) Error() string { return "already reported" }
func (s silentErr) ExitCode() int { return s.code }
func (s silentErr) Silent() bool  { return true }
func newReportEnv(t *testing.T)   { t.Helper(); output.ResetState(); t.Cleanup(output.ResetState) }

// run calls report with fresh buffers.
func run(err error, args ...string) (code int, stdout, stderr string) {
	var o, e bytes.Buffer
	code = report(err, &o, &e, args)
	return code, o.String(), e.String()
}

func TestReportPlainErrorBothModes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		newReportEnv(t)
		code, out, errOut := run(errors.New("boom"))
		if code != 1 || out != "" || errOut != "Error: boom\n" {
			t.Fatalf("got code=%d stdout=%q stderr=%q", code, out, errOut)
		}
	})
}

func TestReportSilentAndExplicitCodes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		newReportEnv(t)
		code, out, errOut := run(silentErr{code: 9})
		if code != 9 || out != "" || errOut != "" {
			t.Fatalf("silent: code=%d stdout=%q stderr=%q", code, out, errOut)
		}
		// A silent error prints nothing even in JSON mode (D9).
		output.SetInvocation("start", true)
		code, out, errOut = run(silentErr{code: 9})
		if code != 9 || out != "" || errOut != "" {
			t.Fatalf("silent json: code=%d stdout=%q stderr=%q", code, out, errOut)
		}
		output.ResetState()
		code, _, errOut = run(errs.ExitWith(7, errors.New("custom")))
		if code != 7 || errOut != "Error: custom\n" {
			t.Fatalf("exit with: code=%d stderr=%q", code, errOut)
		}
	})
}

func TestReportCodedErrorByMode(t *testing.T) {
	cli := errs.New("E101", "licence key rejected")
	wrapped := fmt.Errorf("docker info failed: %w", errs.ErrDockerNotRunning)
	compattest.Both(t, func(t *testing.T) {
		newReportEnv(t)
		for _, c := range []struct {
			name         string
			err          error
			v14, v15     int
			tag15, plain string
		}{
			{"cli error", cli, 1, 3, "[E101]", ""},
			{"wrapped sentinel", wrapped, 1, 2, "[E002]", ""},
		} {
			code, out, errOut := run(c.err)
			want := c.v14
			if compat.V15() {
				want = c.v15
			}
			if code != want || out != "" {
				t.Fatalf("%s: code=%d (want %d) stdout=%q", c.name, code, want, out)
			}
			if compat.V15() {
				if !strings.HasPrefix(errOut, "Error: "+c.tag15) || !strings.Contains(errOut, "\n  Fix: ") {
					t.Fatalf("%s: v1.5 stderr is not the coded block: %q", c.name, errOut)
				}
			} else if errOut != "Error: "+c.err.Error()+"\n" {
				t.Fatalf("%s: v1.4 stderr changed: %q", c.name, errOut)
			}
		}
	})
}

// decodeOne decodes exactly one JSON document from s and fails on trailing data.
func decodeOne(t *testing.T, s string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, s)
	}
	if dec.More() {
		t.Fatalf("stdout holds more than one document:\n%s", s)
	}
	return doc
}

func TestReportJSONEnvelopeV15(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		newReportEnv(t)
		output.SetInvocation("start", true)
		err := fmt.Errorf("docker info failed: %w", errs.ErrDockerNotRunning)
		code, out, errOut := run(err)
		if !compat.V15() {
			if code != 1 || out != "" || !strings.HasPrefix(errOut, "Error: docker info failed") {
				t.Fatalf("v1.4 json: code=%d stdout=%q stderr=%q", code, out, errOut)
			}
			return
		}
		doc := decodeOne(t, out)
		e, _ := doc["error"].(map[string]any)
		if code != 2 || doc["command"] != "start" || doc["schema_version"] != "1" ||
			e["code"] != "E002" || e["class"] != "infra" || e["exit_code"] != float64(2) {
			t.Fatalf("envelope mismatch: code=%d doc=%v", code, doc)
		}
		if _, has := doc["data"]; has {
			t.Fatalf("error envelope carries data: %v", doc)
		}
		if !strings.Contains(errOut, "[E002]") {
			t.Fatalf("human block missing on stderr: %q", errOut)
		}
	})
}

func TestReportJSONFromArgsBeforeResolution(t *testing.T) {
	compattest.Set(t, true)
	newReportEnv(t)
	code, out, errOut := run(errors.New("unknown command"), "--json", "nosuch")
	doc := decodeOne(t, out)
	e, _ := doc["error"].(map[string]any)
	if code != 1 || doc["command"] != "" || e["code"] != "E400" || e["exit_code"] != float64(1) || e["class"] != "user" {
		t.Fatalf("code=%d doc=%v", code, doc)
	}
	if errOut != "Error: unknown command\n" {
		t.Fatalf("stderr = %q", errOut)
	}
	// --json=false and no flag keep stdout empty.
	for _, args := range [][]string{{"--json=false", "x"}, {"x"}, nil} {
		if _, out, _ := run(errors.New("e"), args...); out != "" {
			t.Fatalf("args %v wrote stdout %q", args, out)
		}
	}
}

func TestReportLegacyJSONEnvVarDoesNotSuppressErrorEnvelope(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv(output.LegacyEnvVar, "1")
	newReportEnv(t)
	output.SetInvocation("status", true)
	_, out, _ := run(errs.New("E200", "database down"))
	doc := decodeOne(t, out)
	if e, _ := doc["error"].(map[string]any); e["code"] != "E200" || e["exit_code"] != float64(2) {
		t.Fatalf("doc=%v", doc)
	}
}

func TestReportLeaksNeitherSecretNorArgv(t *testing.T) {
	compattest.Set(t, true)
	newReportEnv(t)
	secret := "pw" + "Sup3rS3cret"
	err := fmt.Errorf("connect postgres://admin:%s@db:5432/app: %w", secret, errs.ErrDatabaseNotRunning)
	argvValue := "--token=argv" + "ValueXyz123"
	code, out, errOut := run(err, "--json", argvValue, "db", "shell")
	if code != 2 {
		t.Fatalf("code = %d", code)
	}
	for name, s := range map[string]string{"stdout": out, "stderr": errOut} {
		if strings.Contains(s, secret) || strings.Contains(s, "argvValueXyz123") {
			t.Fatalf("%s leaks a secret or argv value:\n%s", name, s)
		}
	}
	decodeOne(t, out)
}
