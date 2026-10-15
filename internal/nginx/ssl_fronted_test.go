package nginx

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestD0609FrontedCertificateGate(t *testing.T) {
	for _, mode := range []string{"letsencrypt", "custom"} {
		t.Run(mode, func(t *testing.T) {
			front := filepath.Join(t.TempDir(), "front")
			cfg := newCfgForSSLDetect(t, mode)
			cfg.Nginx.FrontedBy = "front"
			writeCertPair(t, front, cfg.BaseDomain)
			gen := NewGenerator(cfg, filepath.Join(front, "backend"))
			if !gen.HasSSL() {
				t.Error("parent-only certificate must enable the core and plugin TLS gate")
			}
			out, err := gen.RenderServiceRoute(ServiceRouteData{Route: "api", BaseDomain: cfg.BaseDomain, Upstream: "api:8080", SSLDir: sslDirName(cfg.BaseDomain)})
			if err != nil || !strings.Contains(out, "listen 443 ssl;") {
				t.Errorf("fronted core TLS route missing: %v\n%s", err, out)
			}
			cfg.Nginx.FrontedBy = "unresolved"
			if NewGenerator(cfg, front).HasSSL() {
				t.Error("unresolved fronting layout must fail closed despite local certificates")
			}
		})
	}
}
