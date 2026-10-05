package remote

import (
	"context"
	"reflect"
	"testing"
)

func TestCISSHFlags(t *testing.T) {
	if got := CISSHFlags(); !reflect.DeepEqual(got, []string{"-T", "-a", "-x"}) {
		t.Fatalf("CISSHFlags = %q", got)
	}
}

func TestCIOptions_ExactD4List(t *testing.T) {
	want := []string{
		"BatchMode=yes", "ForwardAgent=no", "ForwardX11=no", "ClearAllForwardings=yes",
		"PermitLocalCommand=no", "ControlMaster=no", "ControlPath=none", "RemoteCommand=none",
		"RequestTTY=no", "UpdateHostKeys=no", "CheckHostIP=no", "VerifyHostKeyDNS=no",
		"StrictHostKeyChecking=yes", "UserKnownHostsFile=/pin/kh", "GlobalKnownHostsFile=/dev/null",
		"HostKeyAlias=nself-ci-n1", "ConnectTimeout=10", "ServerAliveInterval=10", "ServerAliveCountMax=3",
	}
	flat := func(kv []string) []string {
		var o []string
		for _, k := range kv {
			o = append(o, "-o", k)
		}
		return o
	}
	if got := CIOptions("n1", "/pin/kh", Version{8, 4}); !reflect.DeepEqual(got, flat(want)) {
		t.Fatalf("CIOptions(8.4) = %q", got)
	}
	if got := CIOptions("n1", "/pin/kh", Version{8, 5}); !reflect.DeepEqual(got, flat(append(want, "KnownHostsCommand=none"))) {
		t.Fatalf("CIOptions(8.5) = %q", got)
	}
	if got := CIOptions("n1", "/pin/kh", Version{10, 0}); !hasArg(got, "KnownHostsCommand=none") {
		t.Fatal("KnownHostsCommand=none missing for 10.0")
	}
	for _, f := range []string{"-T", "-a", "-x"} {
		if hasArg(CIOptions("n1", "/pin/kh", Version{9, 6}), f) {
			t.Fatalf("CIOptions holds %s", f)
		}
	}
}

func TestParseSSHVersion(t *testing.T) {
	cases := map[string]Version{
		"OpenSSH_9.6p1 Ubuntu-3ubuntu13, OpenSSL 3.0.13": {9, 6},
		"OpenSSH_8.0p1, OpenSSL 1.1.1k":                  {8, 0},
		"OpenSSH_10.3p1, LibreSSL 3.3.6":                 {10, 3},
	}
	for in, want := range cases {
		got, err := ParseSSHVersion(in)
		if err != nil || got != want {
			t.Errorf("ParseSSHVersion(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseSSHVersion("Dropbear v2022"); err == nil {
		t.Error("garbage parsed")
	}
	if !(Version{8, 5}).AtLeast(8, 5) || (Version{8, 4}).AtLeast(8, 5) || !(Version{9, 0}).AtLeast(8, 5) {
		t.Error("AtLeast wrong")
	}
}

func TestSSHVersion_UsesStub(t *testing.T) {
	stubTools(t, map[string]string{"ssh": `echo "OpenSSH_9.6p1, test" >&2`})
	v, err := SSHVersion(context.Background())
	if err != nil || v != (Version{9, 6}) {
		t.Fatalf("SSHVersion = %v, %v", v, err)
	}
}
