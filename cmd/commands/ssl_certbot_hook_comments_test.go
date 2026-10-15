package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestCertbotHookComments distinguishes shell comments from executable paths.
// Inputs: config values and executable hook lines containing a protected path.
// Outputs: a refusal only when the path cannot safely be discarded.
// Constraints: ambiguous shell syntax and multiline config stay fail closed.
func TestCertbotHookComments(t *testing.T) {
	const sslDir = "/srv/project/ssl"
	cases := []struct {
		name, body string
		conf, hit  bool
	}{
		{"double config", `post_hook = "true # old /srv/project/ssl/"`, true, false},
		{"single config", `post_hook = 'true # old /srv/project/ssl/'`, true, false},
		{"triple config", `post_hook = """true # old /srv/project/ssl/"""`, true, false},
		{"semicolon", `true;# old /srv/project/ssl/`, false, false},
		{"and", `true &&# old /srv/project/ssl/`, false, false},
		{"comment substitution", "true # $(echo /srv/project/ssl/)", false, false},
		{"comment backtick", "true # `echo /srv/project/ssl/`", false, false},
		{"comment ansi quote", "true # $' /srv/project/ssl/'", false, false},
		{"quoted hash", `echo '# keep /srv/project/ssl/'`, false, true},
		{"quoted config hash", `post_hook = "echo '# keep /srv/project/ssl/'"`, true, true},
		{"escaped hash", `echo \# keep /srv/project/ssl/`, false, true},
		{"escaped semicolon", `echo x\;# keep /srv/project/ssl/`, false, true},
		{"escaped space", `echo x\ # keep /srv/project/ssl/`, false, true},
		{"substitution before comment", `echo "$(echo " # ")"; cp x /srv/project/ssl/`, false, true},
		{"ansi quote before comment", `echo $'it\'s # x'; cp x /srv/project/ssl/`, false, true},
		{"multiline config", "post_hook = \"\"\"true # comment\ncp x /srv/project/ssl/\n\"\"\"", true, true},
		{"unclosed config", `post_hook = "true # cp x /srv/project/ssl/`, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			rel, mode := "renewal-hooks/deploy/check", os.FileMode(0o755)
			if tc.conf {
				rel, mode = "cli.ini", 0o644
			}
			p := filepath.Join(dir, rel)
			acmeWriteFile(t, p, []byte(tc.body+"\n"))
			if err := os.Chmod(p, mode); err != nil {
				t.Fatal(err)
			}
			hits, err := certbotHooks(dir, sslDir)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if tc.hit {
				want = 1
			}
			if len(hits) != want || (want == 1 && hits[0] != fmt.Sprintf("%s:1", p)) {
				t.Fatalf("hits = %v, want %d at %s:1", hits, want, p)
			}
		})
	}
}
