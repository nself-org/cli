package manifestv2_test

import (
	"testing"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

// The nginx exposure declaration (D-0777) decodes strictly: public is the
// only key, it is a boolean, and absent means closed.
func TestNginxPublicDeclaration(t *testing.T) {
	m, err := manifestv2.Parse(mutate(t, "v2/full.json", func(d map[string]any) { d["nginx"] = map[string]any{"public": true} }))
	if err != nil || m.Nginx == nil || !m.Nginx.Public {
		t.Fatalf("nginx.public true: %+v, %v", m, err)
	}
	m, err = manifestv2.Parse(readFixture(t, "v2/full.json"))
	if err != nil || m.Nginx != nil {
		t.Fatalf("absent nginx must stay nil (closed): %+v, %v", m.Nginx, err)
	}
	for name, v := range map[string]any{
		"unknown key": map[string]any{"public": true, "auth": "none"},
		"string":      map[string]any{"public": "true"},
	} {
		if _, err := manifestv2.Parse(mutate(t, "v2/full.json", func(d map[string]any) { d["nginx"] = v })); err == nil {
			t.Errorf("%s: accepted %v", name, v)
		}
	}
}
