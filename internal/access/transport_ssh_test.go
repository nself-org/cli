package access

import (
	"strings"
	"testing"
)

func TestSSHTransportHostSpecPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, v15, host, want, reject string
	}{
		{"legacy", "0", "u@example.test:2222", "StrictHostKeyChecking=accept-new", "HostKeyAlias="},
		{"strict-port", "1", "u@example.test:2222", "HostKeyAlias=nself-example.test-2222", "StrictHostKeyChecking=accept-new"},
		{"strict-default", "1", "u@example.test", "StrictHostKeyChecking=yes", "HostKeyAlias="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NSELF_V15", tc.v15)
			transport := &SSHTransport{Host: tc.host, IdentityPath: "/tmp/test-key"}
			args, err := transport.sshArgs()
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, tc.want) || strings.Contains(joined, tc.reject) {
				t.Fatalf("SSH policy argv: %s", joined)
			}
			if tc.v15 == "1" && !strings.Contains(joined, "GlobalKnownHostsFile=") {
				t.Fatalf("strict policy omitted pin file: %s", joined)
			}
			if strings.Contains(tc.host, ":2222") && !strings.Contains(joined, "-p 2222 -- u@example.test") {
				t.Fatalf("non-22 host lost port or end-of-options: %s", joined)
			}
		})
	}
	t.Setenv("NSELF_V15", "1")
	if _, err := (&SSHTransport{Host: "-oProxyCommand=touch /tmp/pwn"}).sshArgs(); err == nil {
		t.Fatal("unsafe host accepted")
	}
}
