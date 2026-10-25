package nginxtopo

import "testing"

func TestCertDirName(t *testing.T) {
	for _, c := range []struct{ in, dir, orLocalhost string }{
		{"local.nself.org", "local-nself-org", "local-nself-org"},
		{"example.test", "example-test", "example-test"},
		{"localhost", "localhost", "localhost"},
		{"", "", "localhost"},
	} {
		if got := CertDirName(c.in); got != c.dir {
			t.Errorf("CertDirName(%q) = %q, want %q", c.in, got, c.dir)
		}
		if got := CertDirNameOrLocalhost(c.in); got != c.orLocalhost {
			t.Errorf("CertDirNameOrLocalhost(%q) = %q, want %q", c.in, got, c.orLocalhost)
		}
	}
}
