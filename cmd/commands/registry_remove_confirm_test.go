package commands

// Purpose: D-0331: `remove` and `bundle remove` declare confirm {flags: [yes]},
// so a machine caller can run them (and confirm --delete-data) through the
// invoke gate instead of being refused E421 or passing a raw yes flag.
// Inputs: the real command tree built into the v1.5 and v1.4 registries.
// Constraints: Gate only; no command body runs. The plugin name is synthetic.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
)

func TestInvokeRemoveDeleteDataConfirm(t *testing.T) {
	code := func(d invoke.Decision) string {
		var ce *errs.CLIError
		if errors.As(d.Refusal, &ce) {
			return ce.Code
		}
		return ""
	}
	// v1.5 exposes remove; the bundle remove shim carries no surface fields
	// there, so it is checked in the v1.4 registry where it is a real command.
	for _, tc := range []struct {
		v15  bool
		path string
	}{{true, "nself remove"}, {false, "nself remove"}, {false, "nself bundle remove"}} {
		t.Run(fmt.Sprintf("v15=%v %s", tc.v15, tc.path), func(t *testing.T) {
			compattest.Set(t, tc.v15)
			reattachRealTree()
			if tc.v15 {
				defer PrepareTreeForGeneration()()
			}
			reg, err := BuildRegistry(tc.v15)
			if err != nil {
				t.Fatalf("build registry: %v", err)
			}
			cmd, ok := reg.Lookup(tc.path)
			if !ok {
				t.Fatalf("%s is not in the registry", tc.path)
			}
			if cmd.Confirm == nil || !slices.Equal(cmd.Confirm.Flags, []string{"yes"}) {
				t.Fatalf("%s confirm = %+v, want flags [yes]", tc.path, cmd.Confirm)
			}
			opts := invoke.Options{Transport: invoke.TransportMCP, Guard: invoke.NewGuard("")}
			req := invoke.Request{Args: []string{"demo-plugin"}, Flags: map[string]any{"delete-data": true}}
			ctx := context.Background()

			// Unconfirmed: no run, a nonce round trip instead.
			d, err := invoke.Gate(ctx, reg, cmd, req, opts)
			if err != nil {
				t.Fatal(err)
			}
			if d.Kind != invoke.DecisionConfirm || d.Confirmation == nil || d.Confirmation.Confirm == "" {
				t.Fatalf("unconfirmed: kind %q refusal %v, want a confirmation", d.Kind, d.Refusal)
			}
			nonce := d.Confirmation.Confirm

			// The gate is per command, not per flag: a plain remove asks too.
			plain := invoke.Request{Args: req.Args}
			if d, _ := invoke.Gate(ctx, reg, cmd, plain, opts); d.Kind != invoke.DecisionConfirm {
				t.Errorf("plain remove: kind %q refusal %v, want a confirmation", d.Kind, d.Refusal)
			}

			// A wrong confirm is refused and runs nothing.
			bad := req
			bad.Confirm = "0000000000000000000000000000000000000000000000000000000000000000"
			if d, _ := invoke.Gate(ctx, reg, cmd, bad, opts); d.Kind != invoke.DecisionRefuse || code(d) != "E424" {
				t.Errorf("wrong confirm: kind %q code %q, want refuse E424", d.Kind, code(d))
			}

			// The caller cannot pass the consent flag itself.
			raw := invoke.Request{Args: req.Args, Flags: map[string]any{"delete-data": true, "yes": true}}
			if d, _ := invoke.Gate(ctx, reg, cmd, raw, opts); d.Kind != invoke.DecisionRefuse || code(d) != "E425" {
				t.Errorf("raw yes: kind %q code %q, want refuse E425", d.Kind, code(d))
			}

			// Confirmed: runs with the invoker-owned --yes.
			confirmed := req
			confirmed.Confirm = nonce
			d, err = invoke.Gate(ctx, reg, cmd, confirmed, opts)
			if err != nil {
				t.Fatal(err)
			}
			if d.Kind != invoke.DecisionRun || !slices.Equal(d.Flags, []string{"--yes=true"}) {
				t.Errorf("confirmed: kind %q flags %v refusal %v, want run with --yes=true", d.Kind, d.Flags, d.Refusal)
			}
		})
	}
}
