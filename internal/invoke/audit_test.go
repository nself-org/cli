package invoke

// Purpose: tests of the machine audit log (audit.go) against
// contract:cli.machine-audit v1: line shape and order, redaction, file modes,
// rotation and locking.
// Constraints: time, duration and the project dir are normalised before a
// golden compare; permission asserts are skipped on Windows.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	timeRe     = regexp.MustCompile(`"time":"[^"]*"`)
	durationRe = regexp.MustCompile(`"duration_ms":\d+`)
)

// normalizeAudit makes an audit file comparable with a golden.
func normalizeAudit(raw, projectDir string) string {
	s := strings.ReplaceAll(raw, projectDir, "<DIR>")
	s = timeRe.ReplaceAllString(s, `"time":"<TIME>"`)
	return durationRe.ReplaceAllString(s, `"duration_ms":0`)
}

var auditKeys = []string{"time", "transport", "project", "command", "side_effect", "decision", "confirm_kind",
	"request_id", "request", "exit_code", "error_code", "duration_ms"}

func TestAuditLineGolden(t *testing.T) {
	e := newGateEnv(t)
	e.call("gx put", `{"args":["PROJECT_NAME","demo"],"flags":{"note":"first"}}`)
	e.call("gx push", `{"flags":{"target":"prod"}}`)
	e.call("gx wipe", `{"flags":{"yes":true}}`)
	e.call("gx nope", `{}`)
	e.call("gx put", `{"args":["k"]}`)
	raw, err := os.ReadFile(e.audit)
	if err != nil {
		t.Fatal(err)
	}
	// Key order is the contract's, on every line.
	for _, line := range bytes.Split(bytes.TrimRight(raw, "\n"), []byte("\n")) {
		dec := json.NewDecoder(bytes.NewReader(line))
		if _, err := dec.Token(); err != nil {
			t.Fatal(err)
		}
		var keys []string
		for dec.More() {
			tok, _ := dec.Token()
			keys = append(keys, tok.(string))
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				t.Fatal(err)
			}
		}
		if strings.Join(keys, ",") != strings.Join(auditKeys, ",") {
			t.Errorf("keys %v, want %v", keys, auditKeys)
		}
	}
	got := normalizeAudit(string(raw), e.dir)
	golden := filepath.Join("testdata", "gate", "audit.golden.jsonl")
	if os.Getenv("GATE_GOLDEN_UPDATE") == "1" {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("audit lines differ from %s:\n got: %s\nwant: %s", golden, got, want)
	}
	lines := e.auditLines()
	if len(lines) != 5 || lines[0]["time"] != "2026-10-10T12:00:00.000Z" || lines[0]["project"] != e.dir {
		t.Errorf("time comes from the Guard clock in RFC 3339 UTC ms, project is the absolute dir: %v", lines[0])
	}
}

func TestAuditSecretRedaction(t *testing.T) {
	const (
		value  = "S3cr3tVal-9xQ"
		dbPass = "Hunt3r2-pwZ"
		token  = "tok-must-not-appear-123"
	)
	for _, c := range []struct {
		name, transport, surface, wantSurface string
	}{
		{"mcp default", TransportMCP, "", "mcp-stdio"},
		{"mcp sse", TransportMCP, "mcp-sse", "mcp-sse"},
		{"http default", TransportHTTP, "", "http"},
		{"http mcp-http", TransportHTTP, "mcp-http", "mcp-http"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newGateEnv(t, "STUBCLI_MODE", "text", "STUBCLI_STDERR", "failed: POSTGRES_PASSWORD="+value+" and "+dbPass)
			t.Setenv(TokenEnv, token)
			doc := `{"args":["POSTGRES_PASSWORD","` + value + `"],"flags":{"db-password":"` + dbPass + `","note":"mail ali@example.com"}}`
			res, err := Invoke(context.Background(), e.reg, "gx put", req(t, doc),
				Options{Dir: e.dir, Guard: e.guard, Transport: c.transport, Surface: c.surface})
			if err != nil {
				t.Fatal(err)
			}
			x := errorOf(t, res)
			if x["code"] != "E422" {
				t.Fatalf("the text child must give E422, got %v", x)
			}
			raw, err := os.ReadFile(e.audit)
			if err != nil {
				t.Fatal(err)
			}
			all := string(raw) + string(res.Stdout) + res.RequestID
			for _, secret := range []string{value, dbPass, "ali@example.com", token} {
				if strings.Contains(all, secret) {
					t.Errorf("%q leaked: audit=%s cause=%v", secret, raw, x["cause"])
				}
			}
			line := e.auditLines()[0]
			request := line["request"].(map[string]any)
			args := request["args"].([]any)
			flags := request["flags"].(map[string]any)
			if args[0] != "POSTGRES_PASSWORD" || args[1] != Redacted || flags["db-password"] != Redacted {
				t.Errorf("request redaction: %v", request)
			}
			if line["transport"] != c.wantSurface || line["error_code"] != "E422" || line["exit_code"] != nil || line["decision"] != "run" {
				t.Errorf("line: %v", line)
			}
		})
	}
	t.Run("free-form argv items are redacted", func(t *testing.T) {
		r := auditRequestOf(nil, Request{Args: []string{"a"}, Argv: []string{"--token=abc123", "status"}})
		if len(r.Argv) != 2 || r.Argv[0] != Redacted || r.Argv[1] != Redacted || r.Args[0] != Redacted {
			t.Errorf("%+v", r)
		}
	})
}

func TestAuditRotate(t *testing.T) {
	old := auditMaxBytes
	defer func() { auditMaxBytes = old }()
	auditMaxBytes = 1200
	e := newGateEnv(t)
	for i := 0; i < 40; i++ {
		e.call("gx put", fmt.Sprintf(`{"args":["k%d","v"]}`, i))
	}
	for _, suffix := range []string{"", ".1", ".2", ".3"} {
		raw, err := os.ReadFile(e.audit + suffix)
		if err != nil || len(raw) == 0 {
			t.Fatalf("%s%s: %v", e.audit, suffix, err)
		}
		for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
			if !json.Valid([]byte(line)) {
				t.Fatalf("torn line in %s: %q", suffix, line)
			}
		}
		if runtime.GOOS != "windows" {
			if fi, _ := os.Stat(e.audit + suffix); fi.Mode().Perm() != 0o600 {
				t.Errorf("%s mode %v", suffix, fi.Mode().Perm())
			}
		}
	}
	if _, err := os.Stat(e.audit + ".4"); !os.IsNotExist(err) {
		t.Error("only .1 to .3 are kept")
	}
	newest := e.auditLines()
	if last := newest[len(newest)-1]["request"].(map[string]any)["args"].([]any)[0]; last != "k39" {
		t.Errorf("the newest line is in the live file, got %v", last)
	}
	if runtime.GOOS != "windows" {
		for _, d := range []string{filepath.Dir(e.audit), filepath.Dir(filepath.Dir(e.audit))} {
			if fi, err := os.Stat(d); err != nil || fi.Mode().Perm() != 0o700 {
				t.Errorf("dir %s: %v %v", d, fi, err)
			}
		}
	}

	t.Run("an existing dir keeps its mode and a wide file is narrowed", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("no unix permission bits")
		}
		dir := t.TempDir()
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "machine-audit.jsonl")
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		auditMaxBytes = 10 << 20
		if err := NewGuard(path).appendLine([]byte("{}\n")); err != nil {
			t.Fatal(err)
		}
		di, _ := os.Stat(dir)
		fi, _ := os.Stat(path)
		if di.Mode().Perm() != 0o755 || fi.Mode().Perm() != 0o600 {
			t.Errorf("dir %v file %v", di.Mode().Perm(), fi.Mode().Perm())
		}
	})

	t.Run("concurrent writers in two Guards lose no line", func(t *testing.T) {
		auditMaxBytes = 10 << 20
		path := filepath.Join(t.TempDir(), "logs", "machine-audit.jsonl")
		guards := []*Guard{NewGuard(path), NewGuard(path)}
		var wg sync.WaitGroup
		for w := 0; w < 8; w++ {
			wg.Add(1)
			go func(g *Guard) {
				defer wg.Done()
				for i := 0; i < 25; i++ {
					if err := g.appendLine([]byte(`{"n":1}` + "\n")); err != nil {
						t.Error(err)
						return
					}
				}
			}(guards[w%2])
		}
		wg.Wait()
		raw, _ := os.ReadFile(path)
		if n := bytes.Count(raw, []byte("{\"n\":1}\n")); n != 200 {
			t.Errorf("%d lines, want 200", n)
		}
	})

	t.Run("rotation under concurrent writers tears no line", func(t *testing.T) {
		auditMaxBytes = 300
		path := filepath.Join(t.TempDir(), "machine-audit.jsonl")
		guards := []*Guard{NewGuard(path), NewGuard(path)}
		var wg sync.WaitGroup
		for w := 0; w < 6; w++ {
			wg.Add(1)
			go func(g *Guard) {
				defer wg.Done()
				for i := 0; i < 30; i++ {
					if err := g.appendLine([]byte(`{"pad":"` + strings.Repeat("x", 80) + `"}` + "\n")); err != nil {
						t.Error(err)
						return
					}
				}
			}(guards[w%2])
		}
		wg.Wait()
		for _, suffix := range []string{"", ".1", ".2", ".3"} {
			raw, err := os.ReadFile(path + suffix)
			if err != nil {
				continue
			}
			for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
				if line != "" && !json.Valid([]byte(line)) {
					t.Fatalf("torn line in %q: %q", suffix, line)
				}
			}
		}
		if _, err := os.Stat(path + ".4"); !os.IsNotExist(err) {
			t.Error("only .1 to .3 are kept")
		}
	})

	t.Run("a failed audit write is reported without the request and never stops the call", func(t *testing.T) {
		auditMaxBytes = 10 << 20
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		var errOut bytes.Buffer
		prev := auditErrOut
		auditErrOut = &errOut
		defer func() { auditErrOut = prev }()
		bad := newGateEnv(t)
		bad.guard = NewGuard(filepath.Join(blocker, "logs", "machine-audit.jsonl"))
		res := bad.call("gx put", `{"args":["POSTGRES_PASSWORD","Sup3rS3cretValue"]}`)
		if res.ExitCode != 0 || res.ErrorCode != "" {
			t.Errorf("the mutation result stands: %+v", res)
		}
		if !strings.Contains(errOut.String(), "audit write failed") || strings.Contains(errOut.String(), "Sup3rS3cretValue") {
			t.Errorf("stderr notice %q", errOut.String())
		}
	})
}

func TestDefaultAuditPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got, err := DefaultAuditPath()
	if err != nil || got != filepath.Join(home, ".nself", "logs", "machine-audit.jsonl") {
		t.Errorf("%q %v", got, err)
	}
}
