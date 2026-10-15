package build

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFragmentPolicyD0490(t *testing.T) {
	cases := []struct {
		name, fragment, want string
	}{
		{"build host", "services: {worker: {build: {context: ., network: host}}}", "build.network"},
		{"build variable", "services: {worker: {build: {network: '${BUILD_NET}'}}}", "build.network"},
		{"merged build", "x-build: &b {network: host}\nservices: {worker: {build: {<<: *b, context: .}}}", "build.network"},
		{"foreign list", "services: {worker: {networks: [b_private]}}", "b_private"},
		{"foreign map", "services: {worker: {networks: {b_private: {}}}}", "b_private"},
		{"merged service", "x-svc: &s {networks: [b_private]}\nservices: {worker: {<<: *s}}", "b_private"},
		{"core collision", "services: {hasura: {image: attacker}}", "hasura"},
		{"extends", "services: {worker: {extends: {file: outside.yml, service: spy}}}", "extends"},
		{"include", "include: [outside.yml]\nservices: {}", "include"},
		{"local network", "services: {worker: {networks: [private]}}\nnetworks: {private: {}}", ""},
		{"project network", "services: {worker: {networks: ['${DOCKER_NETWORK}']}}", ""},
		{"safe build", "services: {worker: {build: {context: ., network: none}}}", ""},
		{"build context", "services: {worker: {build: .}}", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			plugins := filepath.Join(dir, "plugins")
			writeFile(t, filepath.Join(plugins, "worker", pluginComposeFilename), tc.fragment)
			env := func(k string) (string, bool) { return "host", k == "BUILD_NET" }
			report, err := AnalyzeSocket(dir, plugins, env, []byte("services: {hasura: {image: core}}"))
			if err == nil {
				err = report.Refuse()
			}
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want refusal naming %q, got %v", tc.want, err)
			}
		})
	}
}
