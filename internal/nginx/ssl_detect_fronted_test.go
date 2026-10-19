package nginx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/sharedproxy"
)

// ssl_detect_fronted_test.go — D-0572: a fronted project (NGINX_FRONTED_BY) is
// served by the fronting stack's nginx, so TLS blocks follow the certificate
// pair in the fronting stack's ssl/, not the project's own.

// frontedLayout returns <root>/nself-web/backend (the project) and the
// fronting stack dir <root>/nself-web.
func frontedLayout(t *testing.T) (project, stack string) {
	t.Helper()
	stack = filepath.Join(t.TempDir(), "nself-web")
	project = filepath.Join(stack, "backend")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	return project, stack
}

func TestSSLShouldEmitFronted(t *testing.T) {
	cases := []struct {
		name        string
		mode        string
		ownPair     bool // pair in the project's own ssl/
		servedPair  bool // pair in the fronting stack's ssl/
		frontedBy   string
		assumedOwn  bool
		sharedOwner bool // the fronting stack is a shared-proxy owner
		want        bool
	}{
		{name: "local, only own pair", mode: "local", ownPair: true, frontedBy: "nself-web", want: false},
		{name: "local, served pair", mode: "local", servedPair: true, frontedBy: "nself-web", want: true},
		{name: "letsencrypt, only own pair", mode: "letsencrypt", ownPair: true, frontedBy: "nself-web", want: false},
		{name: "letsencrypt, served pair", mode: "letsencrypt", servedPair: true, frontedBy: "nself-web", want: true},
		{name: "custom, assumed plan pair does not count", mode: "custom", frontedBy: "nself-web", assumedOwn: true, want: false},
		{name: "none stays off with served pair", mode: "none", servedPair: true, frontedBy: "nself-web", want: false},
		{name: "unconfirmed layout reads as absent", mode: "local", ownPair: true, frontedBy: "other-stack", want: false},
		{name: "shared owner, local keeps the copy-in flow", mode: "local", frontedBy: "nself-web", sharedOwner: true, want: true},
		{name: "shared owner, letsencrypt with own pair", mode: "letsencrypt", ownPair: true, frontedBy: "nself-web", sharedOwner: true, want: true},
		{name: "shared owner, letsencrypt without pair", mode: "letsencrypt", frontedBy: "nself-web", sharedOwner: true, want: false},
		{name: "unfronted local is unchanged", mode: "local", frontedBy: "", want: true},
		{name: "unfronted letsencrypt with own pair is unchanged", mode: "letsencrypt", ownPair: true, frontedBy: "", want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			project, stack := frontedLayout(t)
			cfg := newCfgForSSLDetect(t, c.mode)
			cfg.Nginx.FrontedBy = c.frontedBy
			if c.ownPair {
				writeCertPair(t, project, cfg.BaseDomain)
			}
			if c.servedPair {
				writeCertPair(t, stack, cfg.BaseDomain)
			}
			if c.sharedOwner {
				dir := sharedproxy.DirOf(stack)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, sharedproxy.OwnerFile), []byte("{}"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var assumed []string
			if c.assumedOwn {
				assumed = []string{cfg.BaseDomain}
			}
			if got := sslShouldEmit(cfg, project, c.mode, assumed); got != c.want {
				t.Fatalf("sslShouldEmit = %v, want %v", got, c.want)
			}
		})
	}
}
