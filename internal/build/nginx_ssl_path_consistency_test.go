package build

// nginx_ssl_path_consistency_test.go — proves the fix for the SSL path
// mismatch found on a production box: the generated nginx confs'
// ssl_certificate directives and the compose nginx service's SSL volume
// mount must always agree on the in-container path
// (nginxtopo.NginxSSLContainerPath), and the certificate subdirectory name
// must be derived from the resolved BASE_DOMAIN via the one shared
// internal/ssl.DomainToDirName implementation — not two independently
// hand-typed literals that can drift.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/ssl"
)

// TestGeneratedSSLCertPath_MatchesComposeMountTarget generates both the
// nginx confs and the docker-compose.yml for a custom BASE_DOMAIN and
// asserts: (1) nginx's ssl_certificate path is rooted at
// nginxtopo.NginxSSLContainerPath and named after the resolved domain, and
// (2) the compose nginx service mounts "./ssl" at that exact same
// in-container path — the two emitters can no longer independently drift.
func TestGeneratedSSLCertPath_MatchesComposeMountTarget(t *testing.T) {
	cfg := &config.Config{
		ProjectName: "sslpathtest",
		BaseDomain:  "custom.example.org",
	}
	cfg, err := config.ApplyDefaults(cfg)
	if err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}

	// nginx side.
	nginxGen := nginx.NewGenerator(cfg, t.TempDir())
	files, err := nginxGen.Generate()
	if err != nil {
		t.Fatalf("nginx Generate: %v", err)
	}
	defaultConf, ok := files["nginx/conf.d/default.conf"]
	if !ok {
		t.Fatal("nginx/conf.d/default.conf not generated")
	}

	wantCertDir := ssl.DomainToDirName(cfg.BaseDomain)
	if wantCertDir != "custom-example-org" {
		t.Fatalf("sanity: DomainToDirName(%q) = %q, want custom-example-org", cfg.BaseDomain, wantCertDir)
	}
	wantCertPath := nginxtopo.NginxSSLContainerPath + "/certificates/" + wantCertDir + "/fullchain.pem"
	if !strings.Contains(defaultConf, wantCertPath) {
		t.Errorf("default.conf missing ssl_certificate path %q\ngot:\n%s", wantCertPath, defaultConf)
	}

	// compose side.
	composeYAML, err := compose.NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatalf("compose Generate: %v", err)
	}
	wantMount := "./ssl:" + nginxtopo.NginxSSLContainerPath + ":ro"
	if !strings.Contains(string(composeYAML), wantMount) {
		t.Errorf("docker-compose.yml missing nginx ssl volume mount %q", wantMount)
	}
}
