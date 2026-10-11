package remote

// Purpose: mutation-killing tests for ssh_allow.go: every branch of the caller
// option allowlist, and the D4 block followed by a lone "-o". No exec.

import "testing"

func TestCheckCallerOptions_AcceptsEachForm(t *testing.T) {
	for _, in := range [][]string{
		{"-4"}, {"-6"}, {"-q"}, {"-v"},
		{"-o", "Port=22"}, {"-oPort=22"}, {"-o", "Compression=yes"},
		{"-i", "/k"}, {"-p", "22"}, {"-4", "-p", "1", "-i", "/k", "-oUser=u"},
	} {
		if err := checkCallerOptions(in); err != nil {
			t.Errorf("checkCallerOptions(%q) refused: %v", in, err)
		}
	}
}

func TestCheckCallerOptions_RefusesEachForm(t *testing.T) {
	for _, in := range [][]string{
		{"-T"}, {"-a"}, {"-x"}, {"-Z"}, {"-"}, {"x"}, {""},
		{"-Z", "Port=22"}, {"-Z", "/k"}, {"-Z", "22"}, {"-Z", "-q"},
		{"-o"}, {"-i"}, {"-p"}, {"-4", "-o"}, {"-q", "-i"}, {"-v", "-p"},
		{"-p", "abc"}, {"-p", "123456"}, {"-p", "-1"}, {"-i", "-x"}, {"-i", "a b"},
		{"-oBogus=1"}, {"-o", "Bogus=1"}, {"-o", "ProxyCommand=x"}, {"-oPort=abc"},
	} {
		if err := checkCallerOptions(in); err == nil {
			t.Errorf("checkCallerOptions(%q) accepted", in)
		}
	}
}

// A D4 block followed by a lone "-o" must be refused cleanly: the lookahead for
// KnownHostsCommand=none may not read past the slice.
func TestCheckOptions_D4BlockThenLoneO(t *testing.T) {
	// OpenSSH 8.4 has no KnownHostsCommand, so the D4 block ends the slice.
	d4 := append(CISSHFlags(), CIOptions("n1", "/h/known_hosts", Version{Major: 8, Minor: 4})...)
	for _, tail := range [][]string{{"-o"}, {"-q", "-o"}} {
		in := append(append([]string(nil), d4...), tail...)
		if err := checkOptions(in); err == nil {
			t.Errorf("checkOptions(D4 + %q) accepted", tail)
		}
	}
	if err := checkOptions(append(append([]string(nil), d4...), "-q")); err != nil {
		t.Errorf("checkOptions(D4 + -q) refused: %v", err)
	}
}
