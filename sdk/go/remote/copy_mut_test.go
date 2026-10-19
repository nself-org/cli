package remote

// Purpose: mutation-killing tests for copy.go: the exact scpOptions mapping and
// the colon-at-index-0 boundary of checkLocalOperand. No network; no exec.

import (
	"reflect"
	"strings"
	"testing"
)

func TestScpOptions_ExactMapping(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"drop -T", []string{"-T"}, []string{}},
		{"drop -a", []string{"-a"}, []string{}},
		{"drop -x", []string{"-x"}, []string{}},
		{"keep -4", []string{"-4"}, []string{"-4"}},
		{"keep -6", []string{"-6"}, []string{"-6"}},
		{"keep -q", []string{"-q"}, []string{"-q"}},
		{"keep -v", []string{"-v"}, []string{"-v"}},
		{"-o pair", []string{"-o", "Port=22"}, []string{"-o", "Port=22"}},
		{"-i pair", []string{"-i", "/k"}, []string{"-i", "/k"}},
		{"-p becomes -P", []string{"-p", "2222"}, []string{"-P", "2222"}},
		{"attached -o", []string{"-oPort=22"}, []string{"-oPort=22"}},
		{"mixed", []string{"-T", "-a", "-x", "-4", "-p", "9", "-oUser=u", "-v"}, []string{"-4", "-P", "9", "-oUser=u", "-v"}},
	}
	for _, tc := range tests {
		got, err := scpOptions(tc.in)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: scpOptions(%q) = %q, %v; want %q", tc.name, tc.in, got, err, tc.want)
		}
	}
}

// OpenSSH 9+ scp runs ssh with -s (sftp subsystem); ssh refuses -s together
// with SessionType ("Cannot specify -s with -N/SessionType"), so scp must never
// be handed the D4 SessionType pin (D-1262).
func TestScpOptions_DropsSessionType(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"pair", []string{"-o", "SessionType=default"}, []string{}},
		{"attached", []string{"-oSessionType=default"}, []string{}},
		{"neighbours kept", []string{"-o", "StdinNull=no", "-o", "SessionType=default", "-o", "ForkAfterAuthentication=no"},
			[]string{"-o", "StdinNull=no", "-o", "ForkAfterAuthentication=no"}},
		{"other SessionType values kept for the allowlist", []string{"-o", "SessionType=none"}, []string{"-o", "SessionType=none"}},
	}
	for _, tc := range tests {
		got, err := scpOptions(tc.in)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: scpOptions(%q) = %q, %v; want %q", tc.name, tc.in, got, err, tc.want)
		}
	}
}

func TestScpOptions_RefusesUnknownAndValueless(t *testing.T) {
	for _, in := range [][]string{
		{"-Z"}, {"-Z", "x"}, {"-Z", "Port=22"}, {"-Z", "/k"}, {"-Z", "22"},
		{"-o"}, {"-i"}, {"-p"}, {"-4", "-p"}, {"-"}, {"o"}, {"-t"}, {"-q", "-Z"},
	} {
		if got, err := scpOptions(in); err == nil {
			t.Errorf("scpOptions(%q) = %q, want an error", in, got)
		}
	}
}

func TestCheckLocalOperand_ColonBoundary(t *testing.T) {
	for _, bad := range []string{":x", ":", "h:x", "a:b/c", "-x", "", "a\nb", "a\x00b"} {
		if err := checkLocalOperand(bad); err == nil {
			t.Errorf("checkLocalOperand(%q) accepted", bad)
		}
	}
	for _, ok := range []string{"./a:b", "/a:b", "dir/a:b", "plain", "a/b:c", "./:x"} {
		if err := checkLocalOperand(ok); err != nil {
			t.Errorf("checkLocalOperand(%q) refused: %v", ok, err)
		}
	}
	if err := checkLocalOperand(":x"); err == nil || !strings.Contains(err.Error(), "remote operand") {
		t.Errorf("colon at index 0: err = %v, want the remote-operand refusal", err)
	}
}
