package build

import "testing"

// TestLastDockerHost: the guard reads only DOCKER_HOST, and the last assignment wins as in Compose.
func TestLastDockerHost(t *testing.T) {
	for _, tc := range []struct {
		name  string
		in    string
		raw   bool
		want  string
		found bool
	}{
		{"no assignment", "FOO=bar\nPASS=pa$$word\nBARE\n", false, "", false},
		{"bare key beside it", "BARE\nDOCKER_HOST=unix:///var/run/docker.sock\n", false, "unix:///var/run/docker.sock", true},
		{"later literal beats earlier variable", "DOCKER_HOST=tcp://${H:-x}\nDOCKER_HOST=unix:///var/run/docker.sock\n", false, "unix:///var/run/docker.sock", true},
		{"later variable beats earlier literal", "DOCKER_HOST=unix:///var/run/docker.sock\nDOCKER_HOST=${H}\n", false, "${H}", true},
		{"export, quotes and comment", "export DOCKER_HOST=\"unix:///var/run/docker.sock\" # c\n", false, "unix:///var/run/docker.sock", true},
		{"unquoted trailing comment", "DOCKER_HOST=unix:///run/docker.sock # note\n", false, "unix:///run/docker.sock", true},
		{"unterminated quote is unresolved", "DOCKER_HOST='private-value\n", false, "$", true},
		{"raw keeps the text", "DOCKER_HOST=\"${H}\"\n", true, "\"${H}\"", true},
	} {
		got, found := lastDockerHost([]byte(tc.in), tc.raw)
		if got != tc.want || found != tc.found {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, got, found, tc.want, tc.found)
		}
	}
}
