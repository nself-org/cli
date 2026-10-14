package compose

// nginx_socket_test.go — P7-DEPL-21 (ADR 0027 §3): for every compose profile
// the generated nginx service has no Docker socket volume and no unix
// DOCKER_HOST, even when the admin service (which is excluded from the guard)
// is on the stack with its socket mount.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

type socketStack struct {
	Services map[string]struct {
		Volumes     []any `yaml:"volumes"`
		Environment any   `yaml:"environment"`
	} `yaml:"services"`
}

func socketStackOf(t *testing.T, cfg *config.Config, profile ProfileName, omitNginx bool) socketStack {
	t.Helper()
	g := NewGeneratorWithProfile(cfg, profile)
	if omitNginx {
		g.OmitNginx()
	}
	out, err := g.Generate()
	if err != nil {
		t.Fatal(err)
	}
	var s socketStack
	if err := yaml.Unmarshal(out, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNginxSocketGenerator(t *testing.T) {
	for _, withAdmin := range []bool{false, true} {
		for _, name := range ValidProfiles() {
			profile := ProfileName(name)
			cfg := minimalConfig()
			if withAdmin {
				cfg.Admin = config.AdminConfig{Enabled: true, Port: 3021, Version: "latest"}
			}
			t.Run(name+map[bool]string{false: "", true: "/admin on"}[withAdmin], func(t *testing.T) {
				s := socketStackOf(t, cfg, profile, false)
				nginx, ok := s.Services["nginx"]
				if !ok {
					t.Fatalf("profile %s generated no nginx service", name)
				}
				for _, v := range nginx.Volumes {
					if text := strings.ToLower(yamlText(t, v)); strings.Contains(text, "docker.sock") || strings.Contains(text, "/var/run:") {
						t.Errorf("nginx mounts the Docker socket: %v", v)
					}
				}
				if env := strings.ToLower(yamlText(t, nginx.Environment)); strings.Contains(env, "docker_host") || strings.Contains(env, "unix:") {
					t.Errorf("nginx env carries DOCKER_HOST or a unix socket: %v", nginx.Environment)
				}
				if withAdmin && name == string(ProfileApp) {
					// The exclusion is real: admin still has the socket, nginx does not.
					admin, ok := s.Services[AdminServiceName]
					if !ok || !strings.Contains(yamlText(t, admin.Volumes), "docker.sock") {
						t.Fatalf("expected the excluded admin service to keep its socket mount: %+v", admin)
					}
				}
			})
		}
	}

	t.Run("proxy swapped out leaves no nginx", func(t *testing.T) {
		s := socketStackOf(t, minimalConfig(), ProfileApp, true)
		if _, ok := s.Services["nginx"]; ok {
			t.Error("OmitNginx still generated nginx")
		}
	})
}

func yamlText(t *testing.T, v any) string {
	t.Helper()
	b, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
