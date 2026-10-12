package compose

import (
	"github.com/nself-org/cli/internal/compat/compattest"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestProfileForName verifies the profile registry returns correct service sets
// and that the fallback to ProfileApp works for unknown names.
func TestProfileForName(t *testing.T) {
	t.Run("app profile has all services enabled", func(t *testing.T) {
		set, ok := ProfileForName(ProfileApp)
		if !ok {
			t.Fatal("ProfileApp should be a known profile")
		}
		checks := []struct {
			name string
			val  bool
		}{
			{"CoreDB", set.CoreDB},
			{"Hasura", set.Hasura},
			{"Auth", set.Auth},
			{"Nginx", set.Nginx},
			{"Redis", set.Redis},
			{"Minio", set.Minio},
			{"Mailpit", set.Mailpit},
			{"AdminUI", set.AdminUI},
			{"Functions", set.Functions},
			{"Search", set.Search},
			{"Monitoring", set.Monitoring},
		}
		for _, c := range checks {
			if !c.val {
				t.Errorf("ProfileApp: %s should be true", c.name)
			}
		}
	})

	t.Run("ops profile includes core and monitoring only", func(t *testing.T) {
		set, ok := ProfileForName(ProfileOps)
		if !ok {
			t.Fatal("ProfileOps should be a known profile")
		}

		// Must be included.
		included := []struct {
			name string
			val  bool
		}{
			{"CoreDB", set.CoreDB},
			{"Hasura", set.Hasura},
			{"Auth", set.Auth},
			{"Nginx", set.Nginx},
			{"Monitoring", set.Monitoring},
			{"OpsServices", set.OpsServices},
		}
		for _, c := range included {
			if !c.val {
				t.Errorf("ProfileOps: %s should be true", c.name)
			}
		}

		// Must be excluded.
		excluded := []struct {
			name string
			val  bool
		}{
			{"Redis", set.Redis},
			{"Minio", set.Minio},
			{"Mailpit", set.Mailpit},
			{"AdminUI", set.AdminUI},
			{"Functions", set.Functions},
			{"Search", set.Search},
		}
		for _, c := range excluded {
			if c.val {
				t.Errorf("ProfileOps: %s should be false (excluded from ops profile)", c.name)
			}
		}
	})

	t.Run("unknown profile falls back to app", func(t *testing.T) {
		set, ok := ProfileForName("nonexistent")
		if ok {
			t.Error("unknown profile should return ok=false")
		}
		// Fallback set must equal the app profile.
		appSet, _ := ProfileForName(ProfileApp)
		if set != appSet {
			t.Error("fallback for unknown profile should equal ProfileApp service set")
		}
	})

	t.Run("empty profile name falls back to app", func(t *testing.T) {
		set, ok := ProfileForName("")
		if !ok {
			t.Error("empty profile name should return ok=true (treated as default)")
		}
		appSet, _ := ProfileForName(ProfileApp)
		if set != appSet {
			t.Error("empty profile name should equal ProfileApp service set")
		}
	})
}

// TestValidProfiles verifies the helper returns at least app and ops.
func TestValidProfiles(t *testing.T) {
	profiles := ValidProfiles()
	want := map[string]bool{"app": false, "ops": false}
	for _, p := range profiles {
		want[p] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("ValidProfiles() missing %q", name)
		}
	}
}

// TestGeneratorProfileOpsExcludesAppServices verifies that a Generator built
// with ProfileOps omits minio/mailpit/functions/search services even when the
// config has them enabled, and includes core (postgres/hasura/auth/nginx).
func TestGeneratorProfileOpsExcludesAppServices(t *testing.T) {
	cfg := minimalConfig() // defined in core_services_test.go

	// Enable every optional service so we can confirm the profile gate fires.
	cfg.Redis = config.RedisConfig{Enabled: true, Port: 6379, Version: "7-alpine"}
	cfg.Minio = config.MinioConfig{Enabled: true}
	cfg.Mailpit = config.MailpitConfig{Enabled: true}
	cfg.Admin = config.AdminConfig{Enabled: true}
	cfg.Functions = config.FunctionsConfig{Enabled: true}
	cfg.Search = config.SearchConfig{Enabled: true, Engine: "typesense"}

	g := NewGeneratorWithProfile(cfg, ProfileOps)
	dc, err := g.buildDockerCompose()
	if err != nil {
		t.Fatalf("buildDockerCompose() with ProfileOps: %v", err)
	}

	// Core services must be present.
	for _, svc := range []string{"postgres", "hasura", "auth", "nginx"} {
		if _, ok := dc.Services[svc]; !ok {
			t.Errorf("ProfileOps: expected service %q to be present", svc)
		}
	}

	// App-specific services must be absent.
	for _, svc := range []string{"redis", "minio", "mailpit", "nself-admin", "functions", "typesense"} {
		if _, ok := dc.Services[svc]; ok {
			t.Errorf("ProfileOps: service %q should be excluded but was generated", svc)
		}
	}
}

// TestGeneratorProfileAppIsUnchanged verifies that ProfileApp (the default)
// includes optional services when they are enabled — confirming no regression
// from the pre-profile behaviour.
func TestGeneratorProfileAppIsUnchanged(t *testing.T) {
	compattest.Set(t, false) // core MinIO exists only in v1.4 (P7-PLUG-60)
	cfg := minimalConfig()
	cfg.Redis = config.RedisConfig{Enabled: true, Port: 6379, Version: "7-alpine"}
	cfg.Minio = config.MinioConfig{Enabled: true}
	cfg.Search = config.SearchConfig{Enabled: true, Engine: "typesense"}

	// Default generator (no explicit profile) must match NewGeneratorWithProfile(ProfileApp).
	gDefault := NewGenerator(cfg)
	gApp := NewGeneratorWithProfile(cfg, ProfileApp)

	dcDefault, err := gDefault.buildDockerCompose()
	if err != nil {
		t.Fatalf("default generator buildDockerCompose(): %v", err)
	}
	dcApp, err := gApp.buildDockerCompose()
	if err != nil {
		t.Fatalf("ProfileApp generator buildDockerCompose(): %v", err)
	}

	// Both must have the same service keys.
	for name := range dcDefault.Services {
		if _, ok := dcApp.Services[name]; !ok {
			t.Errorf("ProfileApp missing service %q that default has", name)
		}
	}
	for name := range dcApp.Services {
		if _, ok := dcDefault.Services[name]; !ok {
			t.Errorf("default generator missing service %q that ProfileApp has", name)
		}
	}

	// Spot-check: optional services present in both.
	for _, svc := range []string{"redis", "minio", "typesense", "postgres", "hasura", "auth", "nginx"} {
		if _, ok := dcDefault.Services[svc]; !ok {
			t.Errorf("default generator: expected service %q (with cfg enabled)", svc)
		}
	}
}
