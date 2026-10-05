package commands

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/spf13/pflag"
)

// TestStripRootPersistentFlags pins the fix for a regression that affected
// every command moved out of core.
//
// Before a command became a plugin, cobra consumed nself's persistent flags at
// the root and the subcommand never saw them. Once proxied, they were handed
// straight to a plugin binary that has no reason to know about them, so a
// script that had always run
//
//	nself soak --no-deprecation-warnings run
//
// started failing with "unknown flag: --no-deprecation-warnings".
func TestStripRootPersistentFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "the flag that broke real scripts",
			args: []string{"--no-deprecation-warnings", "status"},
			want: []string{"status"},
		},
		{
			name: "flag after the subcommand is stripped too",
			args: []string{"status", "--no-monorepo"},
			want: []string{"status"},
		},
		{
			name: "inline value form",
			args: []string{"--no-monorepo=true", "run"},
			want: []string{"run"},
		},
		{
			name: "plugin flags are left alone",
			args: []string{"run", "--provider", "hetzner", "--force"},
			want: []string{"run", "--provider", "hetzner", "--force"},
		},
		{
			name: "a plugin flag whose name merely starts the same is kept",
			args: []string{"--no-monorepo-really", "run"},
			want: []string{"--no-monorepo-really", "run"},
		},
		{
			name: "everything after -- is positional and passes verbatim",
			args: []string{"exec", "--", "--no-monorepo", "arg"},
			want: []string{"exec", "--", "--no-monorepo", "arg"},
		},
		{
			name: "empty",
			args: []string{},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripRootPersistentFlags(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("stripRootPersistentFlags(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

// TestStripRootPersistentFlagsKeepsJSON proves `nself sentry status --json`
// reaches the plugin with --json in its argv (contract:cli.json-envelope:
// plugin-proxied commands receive --json unchanged) while the CLI's own
// flags are still dropped.
func TestStripRootPersistentFlagsKeepsJSON(t *testing.T) {
	tests := []struct {
		args, want []string
	}{
		{[]string{"status", "--json"}, []string{"status", "--json"}},
		{[]string{"status", "--json=true"}, []string{"status", "--json=true"}},
		{[]string{"--json=false", "status"}, []string{"--json=false", "status"}},
		{[]string{"status", "--no-monorepo", "--json"}, []string{"status", "--json"}},
		{[]string{"x", "--", "--json"}, []string{"x", "--", "--json"}},
	}
	for _, tt := range tests {
		if got := stripRootPersistentFlags(tt.args); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("stripRootPersistentFlags(%q) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

// TestStripRootPersistentFlagsCoversEveryRootFlag guards the derivation itself.
// The list is read from RootCmd rather than hardcoded precisely so that adding
// a persistent flag to the CLI does not quietly start breaking plugins; this
// asserts that property rather than trusting it.
func TestStripRootPersistentFlagsCoversEveryRootFlag(t *testing.T) {
	RootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		args := []string{"sub", "--" + f.Name}
		if f.Value.Type() != "bool" {
			args = append(args, "value")
		}
		got := stripRootPersistentFlags(args)
		if keepsForPlugin(f.Name) {
			// Forwarded flags (--json) must survive: the plugin owns its output.
			if !reflect.DeepEqual(got, args) {
				t.Errorf("forwarded flag --%s was stripped: %q", f.Name, got)
			}
			return
		}
		if len(got) != 1 || got[0] != "sub" {
			t.Errorf("persistent flag --%s (%s) survived stripping: %q", f.Name, f.Value.Type(), got)
		}
	})
}

// TestReportProxyFailure pins which plugin failures the CLI speaks about.
//
// A plugin that ran and exited non-zero already printed its own error to the
// inherited stderr. The CLI adding "Plugin error: plugin exited with code 1"
// underneath was pure noise on every extracted command, and contradicted
// ExitCodeError.Silent(), which had promised the opposite since it was written.
func TestReportProxyFailure(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantPrint string
		wantCode  int
	}{
		{
			name:      "plugin ran and failed: say nothing, mirror the code",
			err:       &plugin.ExitCodeError{Code: 1},
			wantPrint: "",
			wantCode:  1,
		},
		{
			name:      "a plugin's own exit code is preserved, not flattened to 1",
			err:       &plugin.ExitCodeError{Code: 42},
			wantPrint: "",
			wantCode:  42,
		},
		{
			name:      "proxy could not run anything: the user has seen nothing yet",
			err:       errors.New("unknown command \"frobnicate\""),
			wantPrint: "Plugin error: unknown command \"frobnicate\"\n",
			wantCode:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			got := reportProxyFailure(&buf, tt.err)

			if buf.String() != tt.wantPrint {
				t.Errorf("printed %q, want %q", buf.String(), tt.wantPrint)
			}

			var coder errs.ExitCoder
			if !errors.As(got, &coder) {
				t.Fatalf("returned %v, which carries no exit code", got)
			}
			if coder.ExitCode() != tt.wantCode {
				t.Errorf("exit code = %d, want %d", coder.ExitCode(), tt.wantCode)
			}
		})
	}
}

// The two tests below came from sentry_cloud_test.go, which moved to the sentry
// plugin with the rest of the command family under CLI-R11. They stayed because
// what they cover did not move: the `nsentry` argv shim and the `sentry` init
// preset are both core, and `nself sentry` still resolves — through the plugin
// proxy now rather than a registered command.
func TestNormalizeInvokedBinary_NSentryAlias(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"/usr/local/bin/nsentry", "monitors", "list"}
	normalizeInvokedBinary()
	want := []string{"/usr/local/bin/nsentry", "sentry", "monitors", "list"}
	if strings.Join(os.Args, " ") != strings.Join(want, " ") {
		t.Errorf("nsentry shim: got %v, want %v", os.Args, want)
	}

	// Idempotent: running again must not double-insert.
	normalizeInvokedBinary()
	if strings.Join(os.Args, " ") != strings.Join(want, " ") {
		t.Errorf("nsentry shim not idempotent: got %v", os.Args)
	}

	// Plain nself invocation untouched.
	os.Args = []string{"/usr/local/bin/nself", "status"}
	normalizeInvokedBinary()
	if strings.Join(os.Args, " ") != "/usr/local/bin/nself status" {
		t.Errorf("nself args mutated: %v", os.Args)
	}
}

func TestSentryPresetRegistered(t *testing.T) {
	p, ok := initPresets["sentry"]
	if !ok {
		t.Fatal("sentry preset missing from initPresets")
	}
	if len(p.SuggestedPlugins) != 13 {
		t.Errorf("sentry preset should list the 13 bundle plugins, got %d", len(p.SuggestedPlugins))
	}
	joined := strings.Join(p.Notes, "\n")
	if !strings.Contains(joined, "nsk_dev_local") || !strings.Contains(joined, "3848") {
		t.Errorf("sentry preset notes must document the dev API key and local API port:\n%s", joined)
	}
}
