package commands

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
)

// TestInvokePinExposureUpgrade keeps the path fixed while the child mounts a
// plugin whose exposure attributes differ from the parent's registry snapshot.
func TestInvokePinExposureUpgrade(t *testing.T) {
	cases := []struct {
		name, replacement, transport string
		allowed                      bool
	}{
		{"unchanged", `"side_effect": "read"`, "mcp", true},
		{"cli only", `"side_effect": "read", "surface": "cli-only"`, "mcp", false},
		{"no envelope", `"side_effect": "read", "json": "none"`, "http", false},
		{"interactive", `"side_effect": "read", "output": "interactive"`, "mcp", false},
		{"document became stream", `"side_effect": "read", "output": "stream"`, "http", false},
		{"stream transport", `"side_effect": "read", "output": "stream"`, "http-ndjson", true},
		{"flag became stream", `"side_effect": "read"`, "http", false},
		{"lost gate", `"side_effect": "destructive", "confirm": null`, "mcp", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := mountFixture(t)
			path := filepath.Join(dir, "demo", "plugin.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			parentJSON := strings.Replace(string(raw), `"side_effect": "write"`, `"json": "envelope", "output": "document", "side_effect": "read"`, 1)
			write := func(s string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(s), 0644); err != nil {
					t.Fatal(err)
				}
			}
			write(parentJSON)
			parent, _ := mountRoot(t, dir)
			c := &canon.File{SchemaVersion: 1, Verbs: []string{"status"}, Commands: map[string]canon.Entry{}}
			reg, err := cmdregistry.Build(parent, c, nil, cmdregistry.BuildOptions{V15: true})
			if err != nil {
				t.Fatal(err)
			}
			row, _ := reg.Lookup("demo sub")
			if ok, why := invoke.Exposure(row, invoke.TransportMCP, nil); !ok {
				t.Fatalf("parent fixture must be exposed: %s", why)
			}
			childJSON := strings.Replace(parentJSON, `"side_effect": "read"`, tc.replacement, 1)
			if tc.name == "flag became stream" {
				childJSON = strings.Replace(childJSON, `"type": "bool"`, `"type": "bool", "output": "stream"`, 1)
			}
			// Replace existing fields instead of leaving duplicate JSON keys.
			for _, key := range []string{"json", "output"} {
				if strings.Contains(tc.replacement, `"`+key+`"`) {
					value := map[string]string{"json": "envelope", "output": "document"}[key]
					childJSON = strings.Replace(childJSON, `"`+key+`": "`+value+`", `, "", 1)
				}
			}
			if tc.name == "lost gate" {
				var manifest map[string]any
				if err := json.Unmarshal([]byte(childJSON), &manifest); err != nil {
					t.Fatal(err)
				}
				sub := manifest["commands"].(map[string]any)["subcommands"].([]any)[0].(map[string]any)
				delete(sub, "confirm")
				b, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				childJSON = string(b)
			}
			write(childJSON)
			child, _ := mountRoot(t, dir)
			oldLoad := canonLoad
			oldTypes, oldEnvelopes := jsonDataTypes, jsonV15OnlyEnvelope
			jsonDataTypes, jsonV15OnlyEnvelope = nil, nil
			canonLoad = func(bool) (*canon.File, error) { return c, nil }
			t.Cleanup(func() {
				canonLoad = oldLoad
				jsonDataTypes, jsonV15OnlyEnvelope = oldTypes, oldEnvelopes
			})
			t.Setenv(invoke.PathEnv, "demo sub")
			t.Setenv(invoke.InvokedByEnv, strings.Split(tc.transport, "-")[0])
			t.Setenv("NSELF_INVOKE_TRANSPORT", tc.transport)
			err = checkInvokePin(child, []string{"demo", "sub", "--json", "--yes=false"})
			if _, exists := os.LookupEnv(invoke.TransportEnv); exists {
				t.Error("child transport marker must not reach grandchildren")
			}
			if tc.allowed {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var coded *errs.CLIError
				if !errors.As(err, &coded) || coded.Code != "E421" {
					t.Fatalf("child must refuse changed exposure with E421, got %v", err)
				}
			}
		})
	}
}

// TestInvokePinExposureCore proves the prepared production tree builds a valid
// child registry and still exposes an ordinary core command.
func TestInvokePinExposureCore(t *testing.T) {
	if !inFreshProcess(t, "TestInvokePinExposureCore") {
		return
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NSELF_V15", "1")
	t.Setenv(invoke.PathEnv, "status")
	t.Setenv(invoke.InvokedByEnv, "mcp")
	t.Setenv(invoke.TransportEnv, invoke.TransportMCP)
	defer PrepareTreeForGeneration()()
	installInvocationDecorator(RootCmd)
	if err := checkInvokePin(RootCmd, []string{"status", "--json"}); err != nil {
		t.Fatal(err)
	}
	t.Log("CHECKED-TestInvokePinExposureCore")
}
