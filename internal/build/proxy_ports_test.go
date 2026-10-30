package build

import (
	"github.com/nself-org/cli/internal/config"
	"testing"
)

func TestTraefikRejectsCustomPorts(t *testing.T) {
	if err := validateTraefikPorts(nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		http, https int
		refuse      bool
	}{{80, 443, false}, {0, 0, false}, {8080, 443, true}, {80, 8443, true}, {8080, 8443, true}} {
		cfg := &config.Config{}
		cfg.Nginx.HTTPPort, cfg.Nginx.SSLPort = tc.http, tc.https
		f := freshProxyFixture(t)
		// Nginx projects keep their custom ports; only an actual swap refuses.
		if _, err := ProxyProvider(cfg, f.workdir, f.plugins); err != nil {
			t.Fatal(err)
		}
		installProxy(t, f.plugins)
		if _, err := ProxyProvider(cfg, f.workdir, f.plugins); (err != nil) != tc.refuse {
			t.Fatalf("ports %d/%d: %v", tc.http, tc.https, err)
		}
	}
}
