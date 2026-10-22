package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutoInstallConfigPreservesBootstrapEnvironment(t *testing.T) {
	for _, state := range []string{"set", "empty", "unset"} {
		t.Run(state, func(t *testing.T) {
			t.Chdir(t.TempDir())
			keys := []string{"HOME", "NSELF_PLUGIN_DIR", "NSELF_PLUGIN_REGISTRY"}
			for _, key := range keys {
				value := ""
				if state == "set" {
					value = filepath.Join(t.TempDir(), key)
				}
				t.Setenv(key, value)
				if state == "unset" {
					_ = os.Unsetenv(key)
				}
			}
			if err := os.WriteFile(".env", []byte("PROJECT_NAME=fixture\nHOME=/project-home\nNSELF_PLUGIN_DIR=/project-plugins\nNSELF_PLUGIN_REGISTRY=https://project.invalid\n"), 0600); err != nil {
				t.Fatal(err)
			}
			_, _ = loadAutoInstallConfig()
			for _, key := range keys {
				got, present := os.LookupEnv(key)
				if present != (state != "unset") || (state != "set" && got != "") || (state == "set" && filepath.Base(got) != key) {
					t.Errorf("%s was overwritten: present=%v", key, present)
				}
			}
		})
	}
}
