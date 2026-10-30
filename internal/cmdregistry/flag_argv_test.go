package cmdregistry

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestFlagInsertionIndex(t *testing.T) {
	local := pflag.NewFlagSet("local", pflag.ContinueOnError)
	local.String("name", "", "takes a value")
	inherited := pflag.NewFlagSet("inherited", pflag.ContinueOnError)
	inherited.String("env", "", "takes a value")
	inherited.Bool("verbose", false, "bool")
	inherited.Lookup("verbose").NoOptDefVal = "true"

	for _, tc := range []struct {
		desc      string
		args      []string
		wantIndex int
		wantFound bool
	}{
		{"no args", nil, 0, false},
		{"positional only", []string{"a", "b"}, 2, false},
		{"separator stops the scan", []string{"a", "--", "--json"}, 1, false},
		{"flag already forwarded", []string{"a", "--json"}, 1, true},
		{"forwarded inline", []string{"--json=false"}, 0, true},
		{"local value flag skips its value", []string{"--name", "--json"}, 2, false},
		{"inherited value flag skips its value", []string{"--env", "--json", "x"}, 3, false},
		{"inline value is not skipped", []string{"--name=x", "--json"}, 1, true},
		{"no-value flag is not skipped", []string{"--verbose", "--json"}, 1, true},
		{"unknown flag is not skipped", []string{"--other", "--json"}, 1, true},
		{"single dash is positional", []string{"-x", "--"}, 1, false},
	} {
		got, found := FlagInsertionIndex(tc.args, "json", local, inherited)
		if got != tc.wantIndex || found != tc.wantFound {
			t.Errorf("%s: FlagInsertionIndex(%q) = (%d, %t), want (%d, %t)", tc.desc, tc.args, got, found, tc.wantIndex, tc.wantFound)
		}
	}
}

func TestNoopBoolValueIgnoresWrites(t *testing.T) {
	var v NoopBoolValue
	if err := v.Set("true"); err != nil {
		t.Fatalf("Set returned %v, want nil", err)
	}
	if v.String() != "false" || v.Type() != "bool" {
		t.Fatalf("NoopBoolValue = (%q, %q), want (false, bool)", v.String(), v.Type())
	}
}
