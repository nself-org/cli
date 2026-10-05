package remote

// Purpose: tests for the Target.Options allowlist. Nothing here execs ssh.

import (
	"context"
	"fmt"
	"testing"
)

func TestCIOptionsPassTheirOwnCheck(t *testing.T) {
	for _, v := range []Version{{8, 4}, {8, 5}, {9, 6}, {99, 0}} {
		opts := CIOptions("n1", "/pin/known_hosts", v)
		if err := checkOptions(opts); err != nil {
			t.Errorf("CIOptions %v refused: %v", v, err)
		}
		if err := checkOptions(append(CISSHFlags(), opts...)); err != nil {
			t.Errorf("CISSHFlags+CIOptions %v refused: %v", v, err)
		}
	}
}

func TestOptionsAllowlistAccepts(t *testing.T) {
	d4 := CIOptions("n1", "/pin/known_hosts", Version{9, 6})
	tails := [][]string{
		{"-o", "ConnectTimeout=5"}, {"-oConnectTimeout=5"}, {"-o", "connecttimeout 5"},
		{"-o", "ConnectTimeout = 5"}, {"-oServerAliveInterval=5"}, {"-o", "ServerAliveCountMax=2"},
		{"-o", "Port=2222"}, {"-o", "User=deploy"}, {"-o", "IdentityFile=/k/id"},
		{"-o", "IdentitiesOnly=yes"}, {"-o", "BatchMode=yes"}, {"-o", "Compression=yes"},
		{"-o", "LogLevel=ERROR"}, {"-o", "ConnectionAttempts=2"}, {"-o", "AddressFamily=inet"},
		{"-o", "PreferredAuthentications=publickey"}, {"-o", "PORT=22"},
		{"-i", "/k/id"}, {"-p", "2222"}, {"-4"}, {"-6"}, {"-q"}, {"-v"},
		{"-v", "-o", "Port=22", "-i", "/k/id", "-4"},
	}
	for _, tail := range tails {
		if err := checkOptions(tail); err != nil {
			t.Errorf("bare %q refused: %v", tail, err)
		}
		if err := checkOptions(append(append([]string{}, d4...), tail...)); err != nil {
			t.Errorf("after D4 %q refused: %v", tail, err)
		}
	}
	if err := checkOptions([]string{}); err != nil {
		t.Error(err)
	}
}

func TestOptionsAllowlistRefuses(t *testing.T) {
	keys := []string{"ProxyCommand", "ProxyJump", "LocalCommand", "PermitLocalCommand", "KnownHostsCommand",
		"RemoteCommand", "ControlMaster", "ControlPath", "UserKnownHostsFile", "GlobalKnownHostsFile",
		"StrictHostKeyChecking", "HostKeyAlias", "SetEnv", "SendEnv", "Include", "Match", "ForwardAgent",
		"Tunnel", "PKCS11Provider", "SecurityKeyProvider", "CertificateFile", "XAuthLocation"}
	var bad [][]string
	for _, k := range keys {
		for _, kk := range []string{k, "x" + k[1:], upperLower(k)} {
			if kk == k || kk[0] == 'x' {
				bad = append(bad,
					[]string{"-o" + kk + "=x"}, []string{"-o", kk + "=x"}, []string{"-o", kk + " x"},
					[]string{"-o", kk + "=none"}, []string{"-o" + kk + " x"}, []string{"-o", kk + " = x"})
			}
			bad = append(bad, []string{"-o", kk + "=x"}, []string{"-o", mixed(kk) + "=x"})
		}
	}
	bad = append(bad,
		[]string{"-F", "/tmp/evil"}, []string{"-F/tmp/evil"}, []string{"-J", "h"}, []string{"-S", "/x"},
		[]string{"-E", "/x"}, []string{"-D", "1080"}, []string{"-L", "1:h:1"}, []string{"-R", "1:h:1"},
		[]string{"-W", "h:1"}, []string{"-N"}, []string{"-t"}, []string{"-T"}, []string{"-a"}, []string{"-x"},
		[]string{"-A"}, []string{"-X"}, []string{"-Y"}, []string{"-w", "1"}, []string{"-I", "x"},
		[]string{"-o"}, []string{"-i"}, []string{"-p"}, []string{"-p", "22x"}, []string{"-i", "-F"},
		[]string{"-o", "-F"}, []string{"-o", "ConnectTimeout=5 ProxyCommand=id"}, []string{"-o", "Port=22 -o"},
		[]string{"-o", "ConnectTimeout="}, []string{"-o", "ConnectTimeout=x"}, []string{"-o", "ConnectTimeout"},
		[]string{"-o", "User=a b"}, []string{"-o", "User=-oProxyCommand=x"}, []string{"-o", "IdentityFile=/a'b"},
		[]string{"-o", "IdentityFile=/a\\b"}, []string{"-o", "LogLevel=\"x\""}, []string{"-o", ""}, []string{"-o", "="},
		[]string{"ProxyCommand=x"}, []string{"-oUser=a\nProxyCommand=id"}, []string{"-o", "User=a\x00b"},
		[]string{"--", "x"}, []string{"-o", "ConnectTimeout=5", "-F", "x"}, []string{"-o", "Port=22", "-oProxyJump=x"},
	)
	d4 := CIOptions("n1", "/pin/known_hosts", Version{9, 6})
	for _, b := range bad {
		if err := checkOptions(b); err == nil {
			t.Errorf("bare %q accepted", b)
		}
		if err := checkOptions(append(append([]string{}, d4...), b...)); err == nil {
			t.Errorf("after D4 %q accepted", b)
		}
		if err := checkOptions(append(append(CISSHFlags(), d4...), b...)); err == nil {
			t.Errorf("after flags+D4 %q accepted", b)
		}
	}
}

func upperLower(s string) string { return fmt.Sprintf("%s", mixed(s)) }

func mixed(s string) string {
	b := []byte(s)
	for i := range b {
		if i%2 == 1 && b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		} else if i%2 == 0 && b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 'a' - 'A'
		}
	}
	return string(b)
}

func TestOptionsD4MustBeExactBlock(t *testing.T) {
	d4 := CIOptions("n1", "/pin/known_hosts", Version{9, 6})
	cases := map[string][]string{
		"partial block":     d4[:10],
		"flags only":        CISSHFlags(),
		"loose pin":         CIOptions("n1", "/dev/null", Version{9, 6}),
		"bad alias":         CIOptions("n 1", "/pin/known_hosts", Version{9, 6}),
		"reordered":         append(append([]string{}, d4[2:4]...), append(d4[:2], d4[4:]...)...),
		"weakened strict":   replaceOpt(d4, "StrictHostKeyChecking=yes", "StrictHostKeyChecking=no"),
		"weakened agent":    replaceOpt(d4, "ForwardAgent=no", "ForwardAgent=yes"),
		"pin with space":    CIOptions("n1", "/pin/a b", Version{9, 6}),
		"duplicate D4 pair": append(append([]string{}, d4...), "-o", "ControlPath=/tmp/x"),
	}
	for name, o := range cases {
		if err := checkOptions(o); err == nil {
			t.Errorf("%s accepted: %q", name, o)
		}
	}
}

func replaceOpt(opts []string, from, to string) []string {
	out := append([]string{}, opts...)
	for i, o := range out {
		if o == from {
			out[i] = to
		}
	}
	return out
}

func TestTargetOptionsRefusedBeforeExecByEveryEntryPoint(t *testing.T) {
	n := countExecs(t)
	ctx := context.Background()
	tg := Target{Dest: "h1", Options: []string{"-o", "ProxyCommand=touch /tmp/pwned"}}
	if _, err := Run(ctx, tg, "true"); err == nil {
		t.Error("Run accepted ProxyCommand")
	}
	if _, err := RunArgv(ctx, tg, "true"); err == nil {
		t.Error("RunArgv accepted ProxyCommand")
	}
	if _, err := Start(ctx, tg, "true"); err == nil {
		t.Error("Start accepted ProxyCommand")
	}
	if CopyTo(ctx, tg, "./a", "/opt/a") == nil || Rsync(ctx, tg, nil, "./a", "/opt/a") == nil {
		t.Error("CopyTo or Rsync accepted ProxyCommand")
	}
	if *n != 0 {
		t.Fatalf("%d execs before refusal", *n)
	}
}
