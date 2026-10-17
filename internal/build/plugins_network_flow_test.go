package build

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"gopkg.in/yaml.v3"
)

// TestNormalizeComposeNetworksFlowShapes pins D-0859: flow-style fragments
// (shared flow lines, flow services, flow networks, service aliases) keep their
// services, lose the legacy network definition, attach to the project network,
// and normalise idempotently. v1.4 mode must return the input untouched.
func TestNormalizeComposeNetworksFlowShapes(t *testing.T) {
	cases := map[string]string{
		"shared flow line": `{services: {web: {image: busybox, networks: [nself_network]}}, networks: {nself_network: {external: true}}}`,
		"flow service":     "services:\n  web: {image: busybox}\n",
		"flow services":    "services: {web: {image: busybox}}\n",
		"service alias":    "x-base: &base {image: busybox}\nservices:\n  web: *base\n",
		"shared networks":  "services: {web: {image: busybox, networks: [nself_network, backend]}}\nnetworks: {nself_network: {external: true}, backend: {driver: bridge}}\n",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			compattest.Set(t, false)
			old, err := normalizeComposeNetworks([]byte(input), "web")
			if err != nil || !bytes.Equal(old, []byte(input)) {
				t.Fatalf("v1.4 changed input: %s, %v", old, err)
			}
			compattest.Set(t, true)
			out, err := normalizeComposeNetworks([]byte(input), "web")
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Services map[string]struct {
					Image    string
					Networks []string
				}
				Networks map[string]any
			}
			if err := yaml.Unmarshal(out, &doc); err != nil {
				t.Fatal(err)
			}
			want := []string{projectNetworkName}
			if name == "shared networks" {
				want = append(want, "backend")
			}
			if doc.Services["web"].Image != "busybox" || !reflect.DeepEqual(doc.Services["web"].Networks, want) {
				t.Fatalf("service lost or not attached: %s", out)
			}
			if _, ok := doc.Networks["nself_network"]; ok {
				t.Fatalf("legacy definition remains: %s", out)
			}
			if name == "shared networks" && doc.Networks["backend"] == nil {
				t.Fatalf("backend lost: %s", out)
			}
			again, err := normalizeComposeNetworks(out, "web")
			if err != nil || !bytes.Equal(out, again) {
				t.Fatalf("not idempotent: %s, %v", again, err)
			}
		})
	}
}
