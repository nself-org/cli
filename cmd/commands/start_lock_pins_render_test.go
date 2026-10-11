package commands

// start_lock_pins_render_test.go — fixtures of the project-lock pin matrix
// (P7-LIVE-09, round 8): render the real stack from the process environment.
//
// Purpose: give TestProjectLockPinMatrix one honest rendering path (config.Load,
// then the generator, with the pins installed), the way nself build does it.
// Inputs: env maps applied with t.Setenv.
// Outputs: service -> image of the generated compose.
// Constraints: hermetic; config.Load reads an empty directory, so no project
// file or process-wide state outlives a subtest.

import (
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"

	"gopkg.in/yaml.v3"
)

// rowEnv switches on what a table row needs to be rendered at all.
var rowEnv = map[string]map[string]string{
	"postgres/postgres":                 {"POSTGRES_EXTENSIONS": "uuid-ossp"},
	"postgres/pgvector":                 {"POSTGRES_EXTENSIONS": "uuid-ossp,pgvector"},
	"hasura/hasura":                     {},
	"auth/auth":                         {},
	"nginx/nginx":                       {},
	"redis/redis":                       {"REDIS_ENABLED": "true"},
	"minio/minio":                       {"MINIO_ENABLED": "true"},
	"mailpit/mailpit":                   {"MAILPIT_ENABLED": "true"},
	"admin/admin":                       {"NSELF_ADMIN_ENABLED": "true"},
	"functions/functions":               {"FUNCTIONS_ENABLED": "true", "FUNCTIONS_RUNTIME": "node"},
	"functions/deno":                    {"FUNCTIONS_ENABLED": "true", "FUNCTIONS_RUNTIME": "deno"},
	"functions/python":                  {"FUNCTIONS_ENABLED": "true", "FUNCTIONS_RUNTIME": "python"},
	"meilisearch/meilisearch":           {"SEARCH_ENABLED": "true", "SEARCH_ENGINE": "meilisearch"},
	"meilisearch-init/meilisearch-init": {"SEARCH_ENABLED": "true", "SEARCH_ENGINE": "meilisearch"},
	"typesense/typesense":               {"SEARCH_ENABLED": "true", "SEARCH_ENGINE": "typesense"},
}

// renderStack renders the compose of env (on top of a clean environment) and
// returns service -> image. Pins are whatever the caller installed.
func renderStack(t *testing.T, env map[string]string) map[string]string {
	t.Helper()
	return renderStackIn(t, t.TempDir(), env)
}

// renderStackIn is renderStack for a project directory: config.Load reads the
// .env files in dir (for example the ones nself init wrote).
func renderStackIn(t *testing.T, dir string, env map[string]string) map[string]string {
	t.Helper()
	for k, v := range env {
		prev, had := os.LookupEnv(k)
		_ = os.Setenv(k, v)
		defer func(k, prev string, had bool) { // each render sees only its own env
			if had {
				_ = os.Setenv(k, prev)
			} else {
				_ = os.Unsetenv(k)
			}
		}(k, prev, had)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load with %v: %v", env, err)
	}
	// config.Load never reads FUNCTIONS_RUNTIME (the env tag on the field is
	// not wired), so the runtime is applied to the loaded config here.
	if rt := env["FUNCTIONS_RUNTIME"]; rt != "" {
		cfg.Functions.Runtime = rt
	}
	raw, err := compose.NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatalf("Generate with %v: %v", env, err)
	}
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for name, s := range doc.Services {
		out[name] = s.Image
	}
	return out
}

// envName turns a service or lock entry name into its variable prefix.
func envName(s string) string { return strings.ToUpper(strings.ReplaceAll(s, "-", "_")) }

// blankImageVars blanks every variable that could name an image, so an ambient
// value never reaches a subtest.
func blankImageVars(t *testing.T) {
	t.Helper()
	t.Setenv("ENV", "dev")
	t.Setenv("IMAGE_PINNING", "")
	for _, in := range compose.ImageInputs() {
		for _, p := range []string{envName(in.Service), envName(in.Entry)} {
			t.Setenv(p+"_VERSION", "")
			t.Setenv(p+"_IMAGE", "")
		}
		for _, v := range in.Vars {
			t.Setenv(v, "")
		}
	}
	t.Setenv("ADMIN_VERSION", "")
	t.Setenv("ADMIN_IMAGE", "")
}

// imageOfPin finds, in a rendered stack, the service the pin key names and
// returns its image and its compose service name. The lookup goes through the
// production mapping (compose.PinKey): the admin image is pinned as "admin" but
// runs as the service "nself-admin", and no test may remap that by hand.
func imageOfPin(stack map[string]string, pinKey string) (image, service string) {
	for name, img := range stack {
		if compose.PinKey(name) == pinKey {
			return img, name
		}
	}
	return "", ""
}
