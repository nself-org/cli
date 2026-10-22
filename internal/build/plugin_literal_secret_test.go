package build

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPluginComposeLiteralSecret(t *testing.T) {
	for _, password := range []string{"fake-long-fixture$password", `fake-long-fixture\password`} {
		t.Run("roundtrip", func(t *testing.T) {
			f := freshProject(t, "dev-minimal")
			// A non-core variable carrying the database password must be templated too.
			writePlanPlugin(t, f.plugins, "nself-alpha", 3901, true)
			fragment := map[string]any{"services": map[string]any{"alpha": map[string]any{"image": "example/alpha:1", "environment": map[string]string{"OTHER_PASSWORD": password}}}}
			var data bytes.Buffer
			encoder := yaml.NewEncoder(&data)
			encoder.SetIndent(2)
			if err := encoder.Encode(fragment); err != nil {
				t.Fatal(err)
			}
			if err := encoder.Close(); err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, filepath.Join(f.plugins, "nself-alpha", pluginComposeFilename), data.String(), 0600)
			// The project's resolved password is the one that TemplateSecrets must use.
			writeFixtureFile(t, filepath.Join(f.workdir, ".env.local"), "\nPOSTGRES_PASSWORD='"+strings.ReplaceAll(password, "'", `\'`)+"'\n", 0600)
			if _, err := Build(f.workdir, BuildOptions{}); err != nil {
				t.Fatal(err)
			}
			out, err := os.ReadFile(filepath.Join(f.workdir, ".nself/plugins/nself-alpha", pluginComposeFilename))
			if err != nil {
				t.Fatal(err)
			}
			if len(LiteralSecretLeaks(out, map[string]string{"POSTGRES_PASSWORD": password})) != 0 {
				t.Error("plugin fragment retains literal password")
			}
			if !strings.Contains(string(out), "${POSTGRES_PASSWORD}") {
				t.Error("missing password reference")
			}
			var doc struct {
				Services map[string]struct {
					Environment map[string]string `yaml:"environment"`
				} `yaml:"services"`
			}
			if err := yaml.Unmarshal(out, &doc); err != nil {
				t.Fatal(err)
			}
			if resolveSecrets(doc.Services["alpha"].Environment["OTHER_PASSWORD"], map[string]string{"POSTGRES_PASSWORD": password}) != password {
				t.Error("plugin password did not round trip")
			}
		})
	}
}

func TestPluginConfigComposePasswordReferences(t *testing.T) {
	for _, name := range []string{"docker-compose.plugin.yml", "compose.yaml", "client.conf"} {
		t.Run(name, func(t *testing.T) {
			source, dest := t.TempDir(), t.TempDir()
			input := "password=${POSTGRES_PASSWORD}\nurl=postgres://user:${POSTGRES_PASSWORD}@postgres/db\n"
			writeFixtureFile(t, filepath.Join(source, name), input, 0600)
			password := "fake-long-fixture$password"
			if _, err := renderPluginConfigDir(dest, source, map[string]string{"POSTGRES_PASSWORD": password}); err != nil {
				t.Fatal(err)
			}
			out, err := os.ReadFile(filepath.Join(dest, name))
			if err != nil {
				t.Fatal(err)
			}
			if name == "client.conf" {
				if !strings.Contains(string(out), password) {
					t.Error("runtime config lost its literal password")
				}
			} else {
				if strings.Contains(string(out), password) {
					t.Error("compose config contains literal password")
				}
				if !strings.Contains(string(out), "${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}") {
					t.Error("URL lacks encoded password reference")
				}
			}
		})
	}
}
