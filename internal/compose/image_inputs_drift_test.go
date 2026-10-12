package compose

// image_inputs_drift_test.go — the table in image_inputs.go says exactly which
// env variables change the generated images (P7-LIVE-09, round 8).
//
// Purpose: the table cannot drift from the generator. Every candidate
// *_VERSION / *_IMAGE variable is set in turn, the stack is rendered through the
// real path (config.Load, then the generator), and the services whose image
// changed must be exactly the services whose row lists the variable.
// Inputs: candidate names from the config struct tags, from every image lock
// entry (<NAME>_VERSION, <NAME>_IMAGE) and from the table itself.
// Outputs: test failures naming the variable and the service.
// Constraints: hermetic; every candidate is blanked first, config.Load reads no
// project files (an empty directory) and every render restores the environment.

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

// enableAll switches every optional service the generator can render on.
var enableAll = map[string]string{
	"ENV": "dev", "REDIS_ENABLED": "true", "MINIO_ENABLED": "true", "MAILPIT_ENABLED": "true",
	"NSELF_ADMIN_ENABLED": "true", "FUNCTIONS_ENABLED": "true", "SEARCH_ENABLED": "true",
}

// renderVariants together reach every row of the table: the functions runtime,
// the search engine, the cache engine and the postgres flavour each change the rendered image.
var renderVariants = map[string]map[string]string{
	"node-meili-plain":   {"FUNCTIONS_RUNTIME": "node", "SEARCH_ENGINE": "meilisearch", "POSTGRES_EXTENSIONS": "uuid-ossp", "REDIS_ENGINE": "redis"},
	"deno-typesense-pgv": {"FUNCTIONS_RUNTIME": "deno", "SEARCH_ENGINE": "typesense", "POSTGRES_EXTENSIONS": "uuid-ossp,pgvector", "REDIS_ENGINE": "valkey"},
	"python-meili-plain": {"FUNCTIONS_RUNTIME": "python", "SEARCH_ENGINE": "meilisearch", "POSTGRES_EXTENSIONS": "uuid-ossp", "REDIS_ENGINE": "redis"},
}

// envCandidates lists every variable that could plausibly name an image.
func envCandidates() []string {
	set := map[string]bool{}
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if name := f.Tag.Get("env"); strings.HasSuffix(name, "_VERSION") || strings.HasSuffix(name, "_IMAGE") {
				set[name] = true
			}
			walk(f.Type)
		}
	}
	walk(reflect.TypeOf(config.Config{}))
	for _, r := range LockedImages() {
		base := strings.ToUpper(strings.ReplaceAll(r.Name, "-", "_"))
		set[base+"_VERSION"], set[base+"_IMAGE"] = true, true
	}
	for _, in := range ImageInputs() {
		for _, v := range in.Vars {
			set[v] = true
		}
	}
	for _, v := range []string{"ADMIN_VERSION", "ADMIN_IMAGE", "NSELF_ADMIN_IMAGE", "STORAGE_IMAGE", "DENO_VERSION", "PYTHON_VERSION"} {
		set[v] = true
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// sentinel is a value that cannot equal any default image or tag.
func sentinel(name string) string {
	if strings.HasSuffix(name, "_IMAGE") {
		return "registry.example.com/sentinel/img:9.9.9"
	}
	return "9.9.9-sentinel"
}

// renderImages loads the config from the process environment (config.Load, no
// project files) and returns service -> image of the generated compose.
func renderImages(t *testing.T, env map[string]string) map[string]string {
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
	cfg, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatalf("config.Load with %v: %v", env, err)
	}
	// config.Load never reads FUNCTIONS_RUNTIME (the env tag on the field is
	// not wired), so the runtime is applied to the loaded config here.
	if rt := env["FUNCTIONS_RUNTIME"]; rt != "" {
		cfg.Functions.Runtime = rt
	}
	raw, err := NewGenerator(cfg).Generate()
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

func mergeEnv(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// TestImageInputsMatchTheGenerator: for each candidate variable, the services
// whose generated image changes are exactly the rows that list it.
func TestImageInputsMatchTheGenerator(t *testing.T) {
	t.Setenv(EnvImagePinning, PinningLegacy)
	cands := envCandidates()
	for _, c := range cands {
		t.Setenv(c, "")
	}
	reached := map[string]bool{}
	for vname, variant := range renderVariants {
		base := renderImages(t, mergeEnv(enableAll, variant))
		rows := map[string]ImageInput{}
		for svc, img := range base {
			in := inputFor(PinKey(svc), img) // the production mapping, no hand remap
			if r, ok := LockedRef(in.Entry); !ok || repoOf(r.Repository) != repoOf(img) {
				t.Fatalf("%s: service %s renders %q, which no row of image_inputs.go describes", vname, svc, img)
			}
			rows[svc] = in
			reached[in.Service+"/"+in.Entry] = true
		}
		for _, cand := range cands {
			got := renderImages(t, mergeEnv(enableAll, variant, map[string]string{cand: sentinel(cand)}))
			for svc, in := range rows {
				changed := got[svc] != base[svc]
				listed := false
				for _, v := range in.Vars {
					listed = listed || v == cand
				}
				switch {
				case changed && !listed:
					t.Errorf("%s: setting %s changes the %s image (%s -> %s) but the table row %s/%s does not list it", vname, cand, svc, base[svc], got[svc], in.Service, in.Entry)
				case !changed && listed:
					t.Errorf("%s: the table row %s/%s lists %s but setting it leaves the %s image at %s", vname, in.Service, in.Entry, cand, svc, base[svc])
				}
			}
		}
	}
	for _, in := range ImageInputs() {
		if !reached[in.Service+"/"+in.Entry] {
			t.Errorf("no render variant reaches row %s/%s", in.Service, in.Entry)
		}
	}
}
