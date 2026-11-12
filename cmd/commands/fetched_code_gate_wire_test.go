package commands

// Purpose: pin P7-WIRE-22. Every command that downloads or runs fetched code
// needs a confirmation over the machine surfaces, and `update upgrade
// --binary-url` (with its digest and signature flags) never crosses MCP or
// HTTP and is refused without a valid signature before anything is downloaded.
// Inputs: the real machine registry (machineRegistry) and the real
// cobra root driven by argv.
// Outputs: test assertions only.
// Constraints: no hand-built registry or command structs (epic rule W1).

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/spf13/pflag"
)

// fetchedCodeCommands are the registry paths whose run downloads or executes
// fetched code: a CLI binary, a plugin, a bundle of plugins.
var fetchedCodeCommands = []string{
	"add",
	"update upgrade",
	"config bundles install",
	"config plugins update",
}

func wire22Registry(t *testing.T) *cmdregistry.Registry {
	t.Helper()
	reg, err := machineRegistry()
	if err != nil {
		t.Fatalf("machine registry: %v", err)
	}
	return reg
}

func TestWire22FetchedCodeCommandsGated(t *testing.T) {
	reg := wire22Registry(t)
	guard := invoke.NewGuard(filepath.Join(t.TempDir(), "audit.jsonl"))
	for _, path := range fetchedCodeCommands {
		cmd, ok := reg.Lookup(path)
		if !ok {
			t.Fatalf("%s: not in the registry", path)
		}
		if cmd.SideEffect != canon.SideEffectRemote && cmd.SideEffect != canon.SideEffectDestructive {
			t.Errorf("%s: side effect %q, want remote or destructive", path, cmd.SideEffect)
			continue
		}
		if cmd.Confirm == nil {
			t.Errorf("%s: no confirm block", path)
			continue
		}
		req := invoke.Request{Args: []string{"x"}}
		if path == "update upgrade" {
			req.Args = nil
		}
		d, err := invoke.Gate(context.Background(), reg, cmd, req, invoke.Options{Guard: guard})
		if err != nil {
			t.Fatalf("%s: gate: %v", path, err)
		}
		if d.Kind != invoke.DecisionConfirm {
			t.Errorf("%s: decision %q (refusal %v), want %q", path, d.Kind, d.Refusal, invoke.DecisionConfirm)
		}
		if len(d.Flags) != 0 {
			t.Errorf("%s: ran with consent flags %v before a confirmation", path, d.Flags)
		}
		nd, err := invoke.Gate(context.Background(), reg, cmd, req, invoke.Options{})
		if err != nil || nd.Kind != invoke.DecisionRefuse {
			t.Errorf("%s: with no Guard want refusal, got %q (%v)", path, nd.Kind, err)
		}
	}
}

func TestWire22BinaryURLNotOnMCP(t *testing.T) {
	reg := wire22Registry(t)
	cmd, ok := reg.Lookup("update upgrade")
	if !ok {
		t.Fatal("update upgrade: not in the registry")
	}
	for _, name := range []string{"binary-url", "binary-sha256", "binary-signature"} {
		found := false
		for _, f := range cmd.Flags {
			if f.Name == name {
				found = true
				if !f.CLIOnly {
					t.Errorf("--%s is not cli_only in the registry", name)
				}
			}
		}
		if !found {
			t.Errorf("--%s missing from the registry row", name)
		}
		for _, transport := range []string{invoke.TransportMCP, invoke.TransportHTTP} {
			for _, f := range invoke.ExposedFlags(cmd, transport) {
				if f.Name == name {
					t.Errorf("--%s is exposed on %s", name, transport)
				}
			}
			req := invoke.Request{Flags: map[string]any{name: "https://github.com/x/nself.tar.gz"}}
			if _, err := invoke.BuildArgvFor(cmd, req, transport); err == nil {
				t.Errorf("a %s request carrying --%s was turned into argv", transport, name)
			}
		}
		props, _ := invoke.ParamsSchemaIn(reg, cmd)["properties"].(map[string]any)
		if flags, _ := props["flags"].(map[string]any); flags != nil {
			if inner, _ := flags["properties"].(map[string]any); inner != nil {
				if _, leaked := inner[name]; leaked {
					t.Errorf("--%s is in the generated params schema", name)
				}
			}
		}
	}
}

func TestWire22UnsignedBinaryRefused(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_BINARY_SIGNING_PUBKEY_HEX", hex.EncodeToString(pub))
	// A regression must not reach the real binary or its backup dir.
	t.Setenv("HOME", t.TempDir())

	const url = "https://github.com/nself-org/cli/releases/download/v9.9.9/nself-9.9.9-linux-amd64.tar.gz"
	sum := strings.Repeat("ab", 32)
	good := hex.EncodeToString(ed25519.Sign(priv, []byte(sum)))
	other := hex.EncodeToString(ed25519.Sign(priv, []byte(strings.Repeat("cd", 32))))
	_, wrongPriv, _ := ed25519.GenerateKey(nil)
	wrongKey := hex.EncodeToString(ed25519.Sign(wrongPriv, []byte(sum)))

	cases := []struct {
		name string
		args []string
	}{
		{"no signature", []string{"--binary-sha256", sum}},
		{"no digest", []string{"--binary-signature", good}},
		{"malformed signature", []string{"--binary-sha256", sum, "--binary-signature", "zz"}},
		{"signature over another digest", []string{"--binary-sha256", sum, "--binary-signature", other}},
		{"signature by an untrusted key", []string{"--binary-sha256", sum, "--binary-signature", wrongKey}},
	}
	for _, tc := range cases {
		out, err := wire22Upgrade(t, append([]string{"--binary-url", url}, tc.args...)...)
		if err == nil {
			t.Errorf("%s: accepted\n%s", tc.name, out)
			continue
		}
		if !strings.Contains(err.Error(), "signature") && !strings.Contains(err.Error(), "binary-sha256") {
			t.Errorf("%s: refusal does not name the signature: %v", tc.name, err)
		}
		if strings.Contains(out, "Downloading binary") || strings.Contains(out, "Backup saved") {
			t.Errorf("%s: reached the backup or download step\n%s", tc.name, out)
		}
	}
}

// wire22Upgrade runs `update upgrade --yes <args>` through the real cobra root.
func wire22Upgrade(t *testing.T, args ...string) (string, error) {
	t.Helper()
	// cobra keeps flag values between Execute calls: start each run clean.
	if c, _, err := RootCmd.Find([]string{"update", "upgrade"}); err == nil {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
	buf := &bytes.Buffer{}
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs(append([]string{"update", "upgrade", "--yes"}, args...))
	t.Cleanup(func() { RootCmd.SetArgs(nil) })
	err := RootCmd.Execute()
	return buf.String(), err
}
