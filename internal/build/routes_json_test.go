package build

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx/routemodel"
)

func TestRoutesJSONDeterministic(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "schemas", "proxy-routes.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	var first []byte
	for i := 0; i < 2; i++ {
		if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(f.workdir, ".nself", "generated", "routes.json"))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if !bytes.HasPrefix(b, []byte("{\n  \"_generated\":")) {
				t.Fatal("generated marker is not first")
			}
			first = append([]byte(nil), b...)
		} else if !bytes.Equal(first, b) {
			t.Fatal("consecutive builds changed routes.json bytes")
		}
		var m routemodel.Model
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		var instance any
		if err := json.Unmarshal(b, &instance); err != nil {
			t.Fatal(err)
		}
		if err := resolved.Validate(instance); err != nil {
			t.Fatalf("routes.json violates schema: %v", err)
		}
		if m.SchemaVersion != "1" || len(m.Routes) < 2 {
			t.Fatal("incomplete model")
		}
	}
	var bad map[string]any
	if err := json.Unmarshal(first, &bad); err != nil {
		t.Fatal(err)
	}
	delete(bad, "_generated")
	if err := resolved.Validate(bad); err == nil {
		t.Fatal("schema accepted missing generation marker")
	}
}

func TestRoutesJSONDefaults(t *testing.T) {
	cfg, err := config.ApplyDefaults(&config.Config{ProjectName: "defaults", BaseDomain: "example.test", SSLMode: "local"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := routemodel.Build(cfg, t.TempDir(), true, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if m.Defaults.MaxBodyBytes != 100*1024*1024 || !m.Defaults.Gzip.Enabled || len(m.Defaults.Gzip.Types) != 12 {
		t.Fatalf("wrong nginx.conf defaults: %+v", m.Defaults)
	}
	if m.Defaults.TLS.Ciphers == nil || !strings.Contains(*m.Defaults.TLS.Ciphers, "ECDHE-ECDSA") {
		t.Fatal("missing TLS ciphers")
	}
	if m.Defaults.ServerTokens || m.DefaultServer.HTTP.ACMEWebroot == nil || *m.DefaultServer.HTTP.ACMEWebroot != "/etc/nginx/ssl/.acme-webroot" || !m.DefaultServer.HTTP.RedirectHTTPS || m.DefaultServer.HTTPS.Action != "close" {
		t.Fatalf("wrong default server: %+v", m.DefaultServer)
	}
	if len(m.Zones) != 16 {
		t.Fatalf("want all rate zones, got %d", len(m.Zones))
	}
	for _, r := range m.Routes {
		if len(r.SecurityHeaders) == 0 || len(r.BlockedPaths) != 2 || r.TLS == nil || !r.TLS.HasTrustedChain {
			t.Fatalf("incomplete route %s", r.ID)
		}
	}
}

func TestDuplicateRouteRefusal(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		mode := compat.V15()
		f := newPlanFixture(t, "dev-minimal")
		if mode {
			t.Setenv(compat.EnvVar, "1")
		} else {
			t.Setenv(compat.EnvVar, "0")
		}
		envPath := filepath.Join(f.workdir, ".env")
		file, err := os.OpenFile(envPath, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = file.WriteString("CS_1=worker:express-ts\nCS_1_ROUTE=same\nFRONTEND_APP_1_DISPLAY_NAME=web\nFRONTEND_APP_1_SYSTEM_NAME=web\nFRONTEND_APP_1_ROUTE=same\n")
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		_, buildErr := Build(f.workdir, BuildOptions{Check: true})
		if buildErr == nil {
			t.Fatal("build accepted CS/frontend collision")
		}
		if mode {
			if !strings.Contains(buildErr.Error(), "E055") || !strings.Contains(buildErr.Error(), "cs:1") || !strings.Contains(buildErr.Error(), "frontend:1") {
				t.Fatal(buildErr)
			}
		} else if !strings.Contains(buildErr.Error(), "duplicate route \"same\" in FRONTEND_APP_1 and CS_1") {
			t.Fatal(buildErr)
		}
		cfg, err := config.ApplyDefaults(&config.Config{ProjectName: "duplicate", BaseDomain: "example.test", SSLMode: "none"})
		if err != nil {
			t.Fatal(err)
		}
		cfg.CustomServices = []config.CustomService{{Index: 1, Name: "worker", Route: "same", Port: 9000}}
		cfg.FrontendApps = []config.FrontendApp{{Index: 1, SystemName: "web", Route: "same", Port: 3000}}
		m, err := routemodel.Build(cfg, t.TempDir(), false, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = routemodel.Validate(m, compat.V15())
		if err == nil {
			t.Fatal("CS/frontend duplicate accepted")
		}
		if compat.V15() {
			if !strings.Contains(err.Error(), "E055") || !strings.Contains(err.Error(), "cs:1") || !strings.Contains(err.Error(), "frontend:1") {
				t.Fatal(err)
			}
		} else if !strings.Contains(err.Error(), "nginx domain conflict detected:") {
			t.Fatal(err)
		}
		cfg.FrontendApps = nil
		cfg.AppName = "app"
		cfg.CustomServices[0].Route = "api.app"
		m, err = routemodel.Build(cfg, t.TempDir(), false, nil)
		if err != nil {
			t.Fatal(err)
		}
		warnings, err := routemodel.Validate(m, compat.V15())
		if compat.V15() {
			if err == nil || !strings.Contains(err.Error(), "E055") {
				t.Fatalf("new duplicate not refused: %v", err)
			}
		} else {
			if err != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "core:hasura") {
				t.Fatalf("new duplicate: %v %v", warnings, err)
			}
		}
		t.Run("build-new-claim", func(t *testing.T) {
			f := newPlanFixture(t, "dev-minimal")
			if mode {
				t.Setenv(compat.EnvVar, "1")
			} else {
				t.Setenv(compat.EnvVar, "0")
			}
			file, err := os.OpenFile(filepath.Join(f.workdir, ".env"), os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			_, err = file.WriteString("APP_NAME=app\nCS_1=worker:express-ts\nCS_1_ROUTE=api.app\n")
			if err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			_, err = Build(f.workdir, BuildOptions{Check: true})
			if mode {
				if err == nil || !strings.Contains(err.Error(), "E055") {
					t.Fatalf("build missed E055: %v", err)
				}
			} else if err != nil {
				t.Fatalf("v1.4 refused newly seen claim: %v", err)
			}
		})
	})
}
