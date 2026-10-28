package commands

import (
	"path/filepath"
	"testing"
)

// TestCertbotHookComments permits known shell comments while retaining ambiguous code.
func TestCertbotHookComments(t *testing.T) {
	ssl := filepath.Join(t.TempDir(), "ssl")
	for _, tc := range []struct {
		name, body string
		wantHit    bool
	}{
		{"config double quotes", `post_hook = "true # old copy ` + ssl + `/"`, false},
		{"config single quotes", `post_hook = 'true # old copy ` + ssl + `/'`, false},
		{"semicolon comment", "post_hook = true;# old copy " + ssl + "/", false},
		{"and comment", "post_hook = true &&# old copy " + ssl + "/", false},
		{"quoted shell hash", `post_hook = "echo 'still # code'; cp x ` + ssl + `/"`, true},
		{"escaped semicolon", `post_hook = cp x\;# ` + ssl + `/`, true},
		{"escaped hash", `post_hook = echo \#; cp x ` + ssl + `/`, true},
		{"quoted semicolon", `post_hook = cp 'x;#' ` + ssl + `/`, true},
		{"quoted and", `post_hook = cp 'x&&#' ` + ssl + `/`, true},
		{"substitution", `post_hook = echo "$(printf " # ")"; cp x ` + ssl + `/`, true},
		{"backtick", "post_hook = echo `printf ' # '`; cp x " + ssl + "/", true},
		{"ANSI quote", `post_hook = echo $'it\'s # x'; cp x ` + ssl + `/`, true},
		{"multiline", "post_hook = \"\"\"true # ignored\ncp x " + ssl + "/\n\"\"\"", true},
		{"unclosed", "post_hook = \"true # unknown " + ssl + "/", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			le := t.TempDir()
			path := filepath.Join(le, "renewal", "x.conf")
			acmeWriteFile(t, path, []byte(tc.body+"\n"))
			hits, err := certbotHooks(le, ssl)
			if err != nil || (len(hits) != 0) != tc.wantHit {
				t.Fatalf("hits=%v err=%v, want hit=%v", hits, err, tc.wantHit)
			}
		})
	}
}
