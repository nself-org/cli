package build

import (
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestProjectScopedFor (D-0335): a named non-external network must carry the
// plugin's own namespace after the project name.
func TestProjectScopedFor(t *testing.T) {
	cases := []struct {
		plugin, name string
		want         bool
	}{
		{"w", "${COMPOSE_PROJECT_NAME}_w_net", true},
		{"w", "${COMPOSE_PROJECT_NAME:-nself}_w_net", true},
		{"w", "${PROJECT_NAME}_w_x", true},
		{"w", "${COMPOSE_PROJECT_NAME}_x_network", false}, // a sibling project's network
		{"w", "${COMPOSE_PROJECT_NAME}_w", false},         // no suffix
		{"w", "${COMPOSE_PROJECT_NAME}_wx_net", false},
		{"w", "${COMPOSE_PROJECT_NAME}_w-evil_net", false}, // another plugin's namespace
		{"", "${COMPOSE_PROJECT_NAME}_w_net", false},       // fail closed
		{"", "${COMPOSE_PROJECT_NAME}__net", false},
		{"w", "plain_w_net", false},
	}
	for _, c := range cases {
		if got := projectScopedFor(c.plugin, c.name); got != c.want {
			t.Errorf("projectScopedFor(%q, %q) = %v, want %v", c.plugin, c.name, got, c.want)
		}
	}
}

// TestNetworkPolicyScopedToPlugin runs the same names through the full policy:
// E129 in v1.5, a warning only in v1.4.
func TestNetworkPolicyScopedToPlugin(t *testing.T) {
	frag := func(name string) string {
		return "services:\n  w:\n    image: x\n    networks: [p]\nnetworks:\n  p:\n    name: " + name + "\n"
	}
	compattest.Both(t, func(t *testing.T) {
		for _, name := range []string{"${COMPOSE_PROJECT_NAME}_w_net", "${COMPOSE_PROJECT_NAME:-nself}_w_net", "${PROJECT_NAME}_w_x"} {
			if _, err := normalizeComposeNetworks([]byte(frag(name)), "w"); err != nil {
				t.Errorf("%s is scoped to w, got %v", name, err)
			}
		}
		for _, name := range []string{"${COMPOSE_PROJECT_NAME}_x_network", "${COMPOSE_PROJECT_NAME}_w", "${COMPOSE_PROJECT_NAME}_wx_net", "${COMPOSE_PROJECT_NAME}_w-evil_net"} {
			wantE129(t, frag(name), name)
		}
	})
	compattest.Set(t, true)
	{
		// An empty plugin name accepts no project-scoped name.
		if _, err := normalizeComposeNetworks([]byte(frag("${COMPOSE_PROJECT_NAME}_w_net")), ""); err == nil {
			t.Errorf("empty plugin name must fail closed")
		}
	}
}

func TestDocumentCount(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"# c\n", 0},
		{"a: 1\n", 1},
		{"---\na: 1\n", 1},
		{"---\n---\na: 1\n", 2},
		{"a: 1\n---\nb: 2\n", 2},
		{"a: 1\n---\n", 1}, // a trailing null document hides nothing
	}
	for _, c := range cases {
		if got := documentCount([]byte(c.in)); got != c.want {
			t.Errorf("documentCount(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestNormalizeNetworksEmptyLeadingDocument(t *testing.T) {
	compattest.Set(t, true)
	frag := "---\n---\nservices:\n  w:\n    image: x\n    network_mode: host\n"
	wantE129(t, frag, "more than one YAML document")
}
