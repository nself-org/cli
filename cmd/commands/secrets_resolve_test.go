package commands

// Purpose: verify FIFO enforcement, audit refusals and moved registry contracts.
// Inputs: fake secrets and ephemeral age identities.
// Outputs: assertions on envelopes and transport refusals.
// Constraints: no committed identities or real secret values.
// SPORT: P7-TRUST-03.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/secrets"
	"github.com/spf13/cobra"
)

const resolveFakeValue = "s3cr3t-value-0042"

// resolveCommandFixture supports isolated secret contract checks.
func resolveCommandFixture(t *testing.T) (root, home, key string) {
	t.Helper()
	for _, tool := range []string{"age", "age-keygen"} {
		if _, err := exec.LookPath(tool); err != nil {
			if os.Getenv("NSELF_REQUIRE_AGE") == "1" {
				t.Fatal("NSELF_REQUIRE_AGE=1: age required")
			}
			t.Skip("age and age-keygen required")
		}
	}
	root, home = t.TempDir(), t.TempDir()
	key = filepath.Join(home, "operator.key")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SECRETS_AGE_KEY_PATH", key)
	if err := secrets.Init(root); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Set(root, "ci", "FOO", resolveFakeValue); err != nil {
		t.Fatal(err)
	}
	return
}

// resolveErrorCode supports isolated secret contract checks.
func resolveErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != code {
		t.Fatalf("error %v, want %s", err, code)
	}
	if strings.Contains(err.Error(), resolveFakeValue) {
		t.Fatal("error leaked value")
	}
}

// resolveLeaf supports isolated secret contract checks.
func resolveLeaf(t *testing.T, project string) *cobra.Command {
	t.Helper()
	c := newSecretsResolveCmd()
	if err := c.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	c.Flags().String("env", "dev", "")
	if err := c.Flags().Set("project", project); err != nil {
		t.Fatal(err)
	}
	if err := c.Flags().Set("for-attempt", "a1"); err != nil {
		t.Fatal(err)
	}
	c.SetContext(context.Background())
	return c
}

// TestSecretsResolveFIFO proves FIFO refusal precedes key/store access and isolation uses real stdout.
func TestSecretsResolveFIFO(t *testing.T) {
	root, home, _ := resolveCommandFixture(t)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = r.Close()
		_ = w.Close()
	}()
	f, err := os.CreateTemp(t.TempDir(), "regular")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if !realStdoutIsFIFO(w) || realStdoutIsFIFO(f) || realStdoutIsFIFO(&bytes.Buffer{}) {
		t.Fatal("FIFO detector accepted wrong type")
	}
	if null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
		defer func() { _ = null.Close() }()
		if realStdoutIsFIFO(null) {
			t.Fatal("accepted null")
		}
	}
	old := stdoutIsFIFO
	t.Cleanup(func() {
		stdoutIsFIFO = old
		output.ResetState()
	})
	stdoutIsFIFO = func(io.Writer) bool { return false }
	c := resolveLeaf(t, root)
	resolveErrorCode(t, runSecretsResolve(c, []string{"project/FOO"}), "E542")
	b, err := os.ReadFile(filepath.Join(home, ".nself", "audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), resolveFakeValue) || strings.Count(string(b), `"event":"secret-resolve"`) != 1 || !strings.Contains(string(b), `"outcome":"E542"`) {
		t.Fatal("refusal audit invalid")
	}
	t.Run("no store and missing identity", func(t *testing.T) {
		t.Setenv("SECRETS_AGE_KEY_PATH", filepath.Join(t.TempDir(), "missing.key"))
		resolveErrorCode(t, runSecretsResolve(resolveLeaf(t, t.TempDir()), []string{"project/FOO"}), "E542")
	})
	// The real stream retained by IsolateStdout must be used, not os.Stdout.
	oldOut := os.Stdout
	os.Stdout = w
	restore := output.IsolateStdout()
	stdoutIsFIFO = realStdoutIsFIFO
	err = runSecretsResolve(c, []string{"project/FOO"})
	restore()
	os.Stdout = oldOut
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	b, err = io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Data secretResolveData `json:"data"`
	}
	if err = json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Data.Schema != "cli.secret-resolve/v1" || doc.Data.AttemptID != "a1" || len(doc.Data.Values) != 1 || doc.Data.Values[0].Value != resolveFakeValue || doc.Data.Values[0].ExpiresAt != nil {
		t.Fatal("bad FIFO envelope")
	}
}

// TestSecretsResolvePreflightAudit proves command-body refusals each write one audit event.
func TestSecretsResolvePreflightAudit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	old := stdoutIsFIFO
	stdoutIsFIFO = func(io.Writer) bool { return true }
	t.Cleanup(func() { stdoutIsFIFO = old })
	for _, tc := range []struct{ flag, value, code string }{{"env", "staging", "E540"}, {"json", "false", "E401"}, {"for-attempt", "bad/id", "E401"}, {"project", filepath.Join(home, "absent"), "E401"}} {
		c := resolveLeaf(t, home)
		if err := c.Flags().Set(tc.flag, tc.value); err != nil {
			t.Fatal(err)
		}
		resolveErrorCode(t, runSecretsResolve(c, []string{"project/FOO"}), tc.code)
	}
	b, err := os.ReadFile(filepath.Join(home, ".nself", "audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(b), `"event":"secret-resolve"`) != 4 {
		t.Fatal("preflight audit count")
	}
}

// TestSecretsResolveRegistryGolden proves both registries refuse machine access and retain wire shape.
func TestSecretsResolveRegistryGolden(t *testing.T) {
	for _, v15 := range []bool{false, true} {
		reg, err := buildRegistry(v15)
		if err != nil {
			t.Fatal(err)
		}
		path := "secrets resolve"
		if v15 {
			path = "config secrets resolve"
		}
		row, ok := reg.Lookup("nself " + path)
		if !ok || row.SideEffect != "read" || row.Surface != "cli-only" {
			t.Fatalf("registry %s: %+v", path, row)
		}
		if v15 && (row.JSON != "envelope" || row.DataSchema == nil) {
			t.Fatalf("v1.5 envelope missing: %+v", row)
		}
		if !v15 && (row.JSON == "envelope" || row.DataSchema != nil) {
			t.Fatalf("v1.4 registry advertises moved envelope: %+v", row)
		}
		prefix := "secrets "
		if v15 {
			prefix = "config " + prefix
		}
		for _, leaf := range []string{"resolve", "recipients add", "recipients remove"} {
			for _, transport := range []string{invoke.TransportMCP, invoke.TransportHTTP} {
				res, err := invoke.Invoke(context.Background(), reg, prefix+leaf, invoke.Request{}, invoke.Options{Transport: transport})
				if err != nil || res.ErrorCode != "E421" {
					t.Fatalf("v15=%v %s exposed on %s: %s %v", v15, leaf, transport, res.ErrorCode, err)
				}
			}
		}
	}
	// Compare the full wire shape to a static golden, without fixture values.
	var b bytes.Buffer
	output.ResetState()
	if err := output.EmitData(output.Writer{Out: &b}, "config secrets resolve", secretResolveData{Schema: "cli.secret-resolve/v1", AttemptID: "a1", Values: []secrets.Value{{Ref: "project/FOO", EnvName: "FOO", Value: "fake-wire-trust03", Source: "age:ci.age"}}}); err != nil {
		t.Fatal(err)
	}
	const golden = `{"schema_version":"1","command":"config secrets resolve","data":{"schema":"cli.secret-resolve/v1","attempt_id":"a1","values":[{"ref":"project/FOO","env_name":"FOO","value":"fake-wire-trust03","source":"age:ci.age","expires_at":null}]}}`
	var got, want any
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(golden), &want); err != nil {
		t.Fatal(err)
	}
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	if !bytes.Equal(gb, wb) {
		t.Fatalf("golden mismatch: %s", gb)
	}
}

// TestSecretsResolveRealCommand proves both CLI spellings emit values only through pipes.
func TestSecretsResolveRealCommand(t *testing.T) {
	root, home, key := resolveCommandFixture(t)
	bin := pilotBinary(t)
	for _, v15 := range []bool{false, true} {
		args := []string{"secrets", "resolve"}
		mode := "0"
		if v15 {
			args = append([]string{"config"}, args...)
			mode = "1"
		}
		env := append(os.Environ(), "NSELF_V15="+mode, "HOME="+home, "SECRETS_AGE_KEY_PATH="+key, "NSELF_NONINTERACTIVE=1", "NSELF_NO_UPDATE_CHECK=1")
		cmd := exec.Command(bin, append(args, "--help")...)
		cmd.Dir = root
		cmd.Env = env
		if b, err := cmd.CombinedOutput(); err != nil || !bytes.Contains(b, []byte("--for-attempt")) {
			t.Fatalf("help: %v %s", err, b)
		}
		cmd = exec.Command(bin, append(args, "--for-attempt", "a1", "--json", "project/FOO")...)
		cmd.Dir = root
		cmd.Env = env
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("FIFO command: %v %s %s", err, out.String(), stderr.String())
		}
		if !strings.Contains(out.String(), resolveFakeValue) || strings.Contains(stderr.String(), resolveFakeValue) {
			t.Fatal("wrong value routing")
		}
		file, err := os.CreateTemp(t.TempDir(), "regular")
		if err != nil {
			t.Fatal(err)
		}
		cmd = exec.Command(bin, append(args, "--for-attempt", "a1", "--json", "project/FOO")...)
		cmd.Dir = root
		cmd.Env = env
		cmd.Stdout = file
		stderr.Reset()
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			t.Fatal("regular-file stdout allowed")
		}
		_ = file.Close()
		b, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(b, []byte("E542")) || bytes.Contains(b, []byte(resolveFakeValue)) || strings.Contains(stderr.String(), resolveFakeValue) {
			t.Fatal("regular-file refusal invalid")
		}
	}
}

// TestSecretsResolveFlagErrorAudit proves parser errors audit once without any secret output.
func TestSecretsResolveFlagErrorAudit(t *testing.T) {
	root, home, key := resolveCommandFixture(t)
	bin := pilotBinary(t)
	for _, mode := range []string{"0", "1"} {
		for _, flag := range []string{"--json=bogus", "--unknown-resolve-flag"} {
			t.Run(mode+flag, func(t *testing.T) {
				auditPath := filepath.Join(home, ".nself", "audit.log")
				before, err := os.ReadFile(auditPath)
				if err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				args := []string{"secrets", "resolve", "--project", root, "--for-attempt", "a1", flag, "project/FOO"}
				if mode == "1" {
					args = append([]string{"config"}, args...)
				}
				cmd := exec.Command(bin, args...)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "NSELF_V15="+mode, "HOME="+home, "SECRETS_AGE_KEY_PATH="+key, "NSELF_NONINTERACTIVE=1", "NSELF_NO_UPDATE_CHECK=1")
				b, runErr := cmd.CombinedOutput()
				if runErr == nil || !bytes.Contains(b, []byte("E401")) || bytes.Contains(b, []byte(resolveFakeValue)) {
					t.Fatal("parser refusal emitted value or wrong error", runErr)
				}
				after, err := os.ReadFile(auditPath)
				if err != nil {
					t.Fatal(err)
				}
				newEvents := after[len(before):]
				if bytes.Count(newEvents, []byte(`"event":"secret-resolve"`)) != 1 || !bytes.Contains(newEvents, []byte(`"outcome":"E401"`)) || bytes.Contains(newEvents, []byte(resolveFakeValue)) {
					t.Fatal("parser audit must be exactly one value-free E401 event")
				}
			})
		}
	}
}
