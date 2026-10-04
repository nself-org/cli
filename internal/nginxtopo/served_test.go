package nginxtopo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestServedDirs covers the two supported topologies (own, fronted) and the
// refusal path, against real directory trees under t.TempDir().
func TestServedDirs(t *testing.T) {
	tmp := t.TempDir()

	// Prod layout (D-0121, P7-PROD-29): nself-web/ owns .env, nginx/, ssl/ and
	// the compose project; the project under test is nself-web/backend.
	web := filepath.Join(tmp, "nself-web")
	backend := filepath.Join(web, "backend")
	for _, d := range []string{filepath.Join(web, "nginx"), filepath.Join(web, "ssl"), backend} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(web, ".env"), []byte("ENV=prod\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	standalone := filepath.Join(tmp, "standalone")
	stray := filepath.Join(tmp, "elsewhere", "backend")

	tests := []struct {
		name       string
		project    string
		frontedBy  string
		wantRoot   string
		wantNginx  string
		wantSSL    string
		wantErr    bool
		errContain []string
	}{
		{
			name:      "own stack when frontedBy is empty",
			project:   standalone,
			wantRoot:  standalone,
			wantNginx: filepath.Join(standalone, "nginx"),
			wantSSL:   filepath.Join(standalone, "ssl"),
		},
		{
			name:      "confirmed fronted layout uses the fronting stack",
			project:   backend,
			frontedBy: "nself-web",
			wantRoot:  web,
			wantNginx: filepath.Join(web, "nginx"),
			wantSSL:   filepath.Join(web, "ssl"),
		},
		{
			name:       "unconfirmed layout is refused naming project and stack",
			project:    stray,
			frontedBy:  "nself-web",
			wantErr:    true,
			errContain: []string{stray, "NGINX_FRONTED_BY", "nself-web", "unset NGINX_FRONTED_BY"},
		},
		{
			name:       "fronting name that does not match the parent is refused",
			project:    backend,
			frontedBy:  "other-stack",
			wantErr:    true,
			errContain: []string{backend, "other-stack"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root, rerr := ServedRoot(tc.project, tc.frontedBy)
			nginx, nerr := ServedNginxDir(tc.project, tc.frontedBy)
			ssl, serr := ServedSSLDir(tc.project, tc.frontedBy)
			if tc.wantErr {
				for _, err := range []error{rerr, nerr, serr} {
					if !errors.Is(err, ErrFrontingUnresolved) {
						t.Fatalf("err = %v, want wrapping ErrFrontingUnresolved", err)
					}
					for _, s := range tc.errContain {
						if !strings.Contains(err.Error(), s) {
							t.Errorf("error %q does not contain %q", err, s)
						}
					}
				}
				if root != "" || nginx != "" || ssl != "" {
					t.Errorf("paths must be empty on error, got %q %q %q", root, nginx, ssl)
				}
				return
			}
			if rerr != nil || nerr != nil || serr != nil {
				t.Fatalf("unexpected errors: %v %v %v", rerr, nerr, serr)
			}
			if root != tc.wantRoot || nginx != tc.wantNginx || ssl != tc.wantSSL {
				t.Errorf("got root=%q nginx=%q ssl=%q; want %q %q %q",
					root, nginx, ssl, tc.wantRoot, tc.wantNginx, tc.wantSSL)
			}
		})
	}
}

// TestServedDirs_ExactMessage pins the readiness R13g error text.
func TestServedDirs_ExactMessage(t *testing.T) {
	_, err := ServedRoot("/x/backend", "nself-web")
	want := `cannot resolve the served nginx stack: NGINX_FRONTED_BY="nself-web" but /x/backend is not directly under a directory named "nself-web"; ` +
		`lay the project out as "backend" under nself-web's own directory, or unset NGINX_FRONTED_BY`
	if err == nil || err.Error() != want {
		t.Errorf("message mismatch:\n got %v\nwant %s", err, want)
	}
}
