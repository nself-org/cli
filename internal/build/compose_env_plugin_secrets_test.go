package build

// compose_env_plugin_secrets_test.go — D-0647: a project whose .env lacks
// PLUGIN_INTERNAL_SECRET / NOTIFY_INTERNAL_SECRET still hands plugins the
// generated value through .nself/compose.env, persisted once in .env.secrets
// and kept on every rebuild. Failure messages name keys, never values.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pluginSecretKeys are the internal secrets every plugin fragment reads as
// ${KEY} from compose.env.
var pluginSecretKeys = []string{"PLUGIN_INTERNAL_SECRET", "NOTIFY_INTERNAL_SECRET"}

// envFileValues returns the value of key on every KEY= line of an env file,
// with one layer of single or double quotes removed.
func envFileValues(text, key string) []string {
	var vals []string
	for _, line := range strings.Split(text, "\n") {
		v, ok := strings.CutPrefix(line, key+"=")
		if !ok {
			continue
		}
		if len(v) >= 2 && (v[0] == '\'' || v[0] == '"') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		vals = append(vals, v)
	}
	return vals
}

func readProjectFile(t *testing.T, f *planFixture, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.workdir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// pluginSecretsOnDisk checks that compose.env carries each key exactly once,
// non-empty and equal to its single .env.secrets line, and returns the values.
func pluginSecretsOnDisk(t *testing.T, f *planFixture) map[string]string {
	t.Helper()
	composeEnv := readProjectFile(t, f, composeEnvFile)
	secrets := readProjectFile(t, f, ".env.secrets")
	got := map[string]string{}
	for _, key := range pluginSecretKeys {
		inCompose := envFileValues(composeEnv, key)
		inSecrets := envFileValues(secrets, key)
		switch {
		case len(inCompose) != 1 || inCompose[0] == "":
			t.Errorf("%s: compose.env has %d lines (want exactly one, non-empty)", key, len(inCompose))
		case len(inSecrets) != 1:
			t.Errorf("%s: .env.secrets has %d lines, want 1", key, len(inSecrets))
		case inCompose[0] != inSecrets[0]:
			t.Errorf("%s: compose.env value differs from the persisted .env.secrets value", key)
		default:
			got[key] = inCompose[0]
		}
	}
	return got
}

// TestComposeEnvCarriesGeneratedPluginSecrets (D-0647): build a project whose
// .env holds no secret at all, then rebuild it. compose.env carries the
// generated value both times, the value never changes, .env.secrets is not
// appended to again, and the value is never in docker-compose.yml or the log.
func TestComposeEnvCarriesGeneratedPluginSecrets(t *testing.T) {
	for _, withPlugin := range []bool{false, true} {
		name := map[bool]string{false: "no-plugins", true: "with-plugin"}[withPlugin]
		t.Run(name, func(t *testing.T) {
			f := freshProject(t, "dev-minimal")
			if strings.Contains(readProjectFile(t, f, ".env"), "INTERNAL_SECRET=") {
				t.Fatal("fixture .env must not define the plugin secrets")
			}
			if withPlugin {
				writePlanPlugin(t, f.plugins, "nself-alpha", 3901, true)
			}
			logs := captureSlog(t)

			if _, err := Build(f.workdir, BuildOptions{}); err != nil {
				t.Fatalf("build 1: %v", err)
			}
			first := pluginSecretsOnDisk(t, f)
			secretsAfterFirst := readProjectFile(t, f, ".env.secrets")

			if _, err := Build(f.workdir, BuildOptions{}); err != nil {
				t.Fatalf("build 2: %v", err)
			}
			second := pluginSecretsOnDisk(t, f)
			if readProjectFile(t, f, ".env.secrets") != secretsAfterFirst {
				t.Error("rebuild rewrote .env.secrets")
			}

			compose := readProjectFile(t, f, "docker-compose.yml")
			for _, key := range pluginSecretKeys {
				val := first[key]
				if val == "" {
					continue
				}
				if second[key] != val {
					t.Errorf("%s changed between builds", key)
				}
				if strings.Contains(compose, val) {
					t.Errorf("%s value appears literally in docker-compose.yml", key)
				}
				if strings.Contains(logs.String(), val) {
					t.Errorf("%s value appears in the build log", key)
				}
			}
		})
	}
}

// TestComposeEnvPluginSecretsPlanEqualsApply (D-0647): on a fresh project the
// seeded plan's compose.env carries both keys and equals what apply writes,
// and a plan of the rebuild keeps the applied values (plan == apply, no
// regeneration).
func TestComposeEnvPluginSecretsPlanEqualsApply(t *testing.T) {
	f := freshProject(t, "dev-minimal")
	plan := planSeeded(t, f, newSeedStream("d-0647"))
	planned := plannedFile(t, plan, ".nself/compose.env")
	for _, key := range pluginSecretKeys {
		if v := envFileValues(planned, key); len(v) != 1 || v[0] == "" {
			t.Errorf("%s: planned compose.env has %d lines (want exactly one, non-empty)", key, len(v))
		}
	}

	if _, err := Build(f.workdir, BuildOptions{Rand: newSeedStream("d-0647")}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	applied := readProjectFile(t, f, composeEnvFile)
	if normalizeFixtureBytes("", applied, f.root) != normalizeFixtureBytes("", planned, f.root) {
		t.Error("applied compose.env differs from the plan")
	}
	values := pluginSecretsOnDisk(t, f)

	// A rebuild plan with a different random source must not mint new values:
	// the persisted .env.secrets owns them now.
	replan := planSeeded(t, f, newSeedStream("d-0647-rebuild"))
	replanned := plannedFile(t, replan, ".nself/compose.env")
	for _, key := range pluginSecretKeys {
		if v := envFileValues(replanned, key); len(v) != 1 || v[0] != values[key] {
			t.Errorf("%s: rebuild plan does not keep the applied value", key)
		}
	}
}

// TestSecretEnvMapCarriesPluginSecrets (D-0647): compose.env gets both keys
// from SecretEnvMap alone, with no plugin env vars at all, and a blank value
// is left out rather than written empty. Values are fake fixtures.
func TestSecretEnvMapCarriesPluginSecrets(t *testing.T) {
	cfg := secretTestConfig()
	cfg.PluginSystem.InternalSecret = "fake-plugin-internal-Tz4Kp8Wq"
	cfg.PluginConfig.NotifySecret = "fake-notify-internal-Rm7Yc2Hd"
	dir := t.TempDir()
	if err := WriteComposeEnv(dir, cfg, SecretEnvMap(cfg), nil); err != nil {
		t.Fatalf("WriteComposeEnv: %v", err)
	}
	env := readComposeEnvMap(t, dir)
	if env["PLUGIN_INTERNAL_SECRET"] != cfg.PluginSystem.InternalSecret {
		t.Error("compose.env lacks PLUGIN_INTERNAL_SECRET without plugin env vars")
	}
	if env["NOTIFY_INTERNAL_SECRET"] != cfg.PluginConfig.NotifySecret {
		t.Error("compose.env lacks NOTIFY_INTERNAL_SECRET without plugin env vars")
	}

	cfg.PluginSystem.InternalSecret = "  "
	cfg.PluginConfig.NotifySecret = ""
	m := SecretEnvMap(cfg)
	for _, key := range pluginSecretKeys {
		if _, ok := m[key]; ok {
			t.Errorf("SecretEnvMap kept a blank %s", key)
		}
	}
}

// TestCustomServicePassthroughPluginSecretsTemplated (D-0647): when the plugin
// secrets come from the process environment (a CI export) and a custom
// service forwards them with CS_1_ENV_PASSTHROUGH, docker-compose.yml holds
// ${KEY} and compose.env the value. Before the keys joined SecretEnvMap the
// literal stayed in docker-compose.yml. Values are fake fixtures.
func TestCustomServicePassthroughPluginSecretsTemplated(t *testing.T) {
	f := freshProject(t, "dev-minimal")
	env := readProjectFile(t, f, ".env")
	writeFixtureFile(t, filepath.Join(f.workdir, ".env"), env+"CS_1=worker:express-ts\nCS_1_ENV_PASSTHROUGH=PLUGIN_INTERNAL_SECRET,NOTIFY_INTERNAL_SECRET\n", 0o600)
	fakes := map[string]string{
		"PLUGIN_INTERNAL_SECRET": "fake-ci-plugin-internal-Wd3Np7Lx",
		"NOTIFY_INTERNAL_SECRET": "fake-ci-notify-internal-Gs5Qe9Vb",
	}
	for k, v := range fakes {
		t.Setenv(k, v)
	}
	if _, err := Build(f.workdir, BuildOptions{}); err != nil {
		t.Fatalf("build: %v", err)
	}
	compose := readProjectFile(t, f, "docker-compose.yml")
	composeEnv := readProjectFile(t, f, composeEnvFile)
	for key, val := range fakes {
		if strings.Contains(compose, val) {
			t.Errorf("%s value appears literally in docker-compose.yml", key)
		}
		if !strings.Contains(compose, "${"+key+"}") {
			t.Errorf("custom service did not receive ${%s}", key)
		}
		if v := envFileValues(composeEnv, key); len(v) != 1 || v[0] != val {
			t.Errorf("%s: compose.env does not carry the exported value", key)
		}
	}
}
