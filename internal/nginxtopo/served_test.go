package nginxtopo

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
			name:      "trailing slash and dot segments are cleaned",
			project:   backend + string(filepath.Separator) + ".",
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

// TestServedDirs_ExactMessage pins the readiness R13g error text. The path is
// built with FromSlash and the expectation from its cleaned form, so the test
// holds on Windows where ServedRoot's Clean rewrites separators.
func TestServedDirs_ExactMessage(t *testing.T) {
	project := filepath.FromSlash("/x/backend")
	_, err := ServedRoot(project, "nself-web")
	want := `cannot resolve the served nginx stack: NGINX_FRONTED_BY="nself-web" but ` + filepath.Clean(project) +
		` is not directly under a directory named "nself-web"; ` +
		`lay the project out as "backend" under nself-web's own directory, or unset NGINX_FRONTED_BY`
	if err == nil || err.Error() != want {
		t.Errorf("message mismatch:\n got %v\nwant %s", err, want)
	}
}

// TestSharedOwnerRegistered covers the three states of a served root: no
// nginx/shared directory, the directory without the owner file, and the owner
// file present.
func TestSharedOwnerRegistered(t *testing.T) {
	root := t.TempDir()
	if SharedOwnerRegistered(root) {
		t.Fatal("a root with no nginx/shared reported a shared-proxy owner")
	}
	shared := filepath.Join(root, "nginx", SharedDir)
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if SharedOwnerRegistered(root) {
		t.Fatal("nginx/shared without " + SharedOwnerFile + " reported a shared-proxy owner")
	}
	if err := os.WriteFile(filepath.Join(shared, SharedOwnerFile), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !SharedOwnerRegistered(root) {
		t.Fatal("nginx/shared/" + SharedOwnerFile + " present but no owner reported")
	}
}

func TestContained(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nginx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Contained(root, filepath.Join(root, "nginx", "sites")); err != nil {
		t.Fatalf("a missing directory inside the project: %v", err)
	}
	if err := Contained(root, filepath.Join(root, "x", "y", "z")); err != nil {
		t.Fatalf("a missing nested directory inside the project: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	if err := os.Symlink(outside, filepath.Join(root, "nginx", "sites")); err != nil {
		t.Fatal(err)
	}
	if err := Contained(root, filepath.Join(root, "nginx", "sites")); err == nil || !strings.Contains(err.Error(), "outside the project") {
		t.Fatalf("a link out of the project: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "nginx", "sites")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "nginx")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "nginx")); err != nil {
		t.Fatal(err)
	}
	if err := Contained(root, filepath.Join(root, "nginx", "sites")); err == nil {
		t.Fatal("a linked parent that leaves the project must be refused even when sites does not exist")
	}
}

// Contained fails closed: a project root, a path component, or a link it cannot
// resolve is an error, never a silent pass (D-0757).
func TestContained_UnresolvablePaths(t *testing.T) {
	root := t.TempDir()

	t.Run("project root missing", func(t *testing.T) {
		gone := filepath.Join(root, "no-such-project")
		err := Contained(gone, filepath.Join(gone, "nginx"))
		if err == nil || !strings.Contains(err.Error(), "resolving the project directory") {
			t.Fatalf("a missing project root: %v", err)
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("the cause must stay in the chain: %v", err)
		}
	})

	if runtime.GOOS == "windows" {
		t.Skip("symlinks and ENOTDIR semantics differ on Windows")
	}

	t.Run("parent is a regular file", func(t *testing.T) {
		file := filepath.Join(root, "plain-file")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(file, "sites")
		err := Contained(root, dir)
		if err == nil || !strings.Contains(err.Error(), "checking "+dir) {
			t.Fatalf("a directory below a regular file: %v", err)
		}
	})

	t.Run("symlink loop", func(t *testing.T) {
		loop := filepath.Join(root, "loop")
		if err := os.Symlink(loop, loop); err != nil {
			t.Fatal(err)
		}
		err := Contained(root, loop)
		if err == nil || !strings.Contains(err.Error(), "resolving "+loop) {
			t.Fatalf("a self-referencing link: %v", err)
		}
	})
}
