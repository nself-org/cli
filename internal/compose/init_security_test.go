package compose

import (
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// meilisearch-init chowns and chmods a data volume it may not own. Under the
// default CapDrop ALL profile it failed with "Operation not permitted", which
// aborted nself start before nginx started (nself-web prod, 2026-09-23).
func TestMeiliInitGetsOwnershipCaps(t *testing.T) {
	cfg, err := config.ApplyDefaults(&config.Config{
		ProjectName: "test",
		Search:      config.SearchConfig{Enabled: true, Engine: "meilisearch"},
	})
	if err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}
	dc, err := NewGenerator(cfg).buildDockerCompose()
	if err != nil {
		t.Fatalf("buildDockerCompose: %v", err)
	}
	svc, ok := dc.Services["meilisearch-init"]
	if !ok {
		t.Fatal("meilisearch-init not generated with meilisearch enabled")
	}
	if want := []string{"CHOWN", "FOWNER", "DAC_READ_SEARCH"}; !reflect.DeepEqual(svc.CapAdd, want) {
		t.Errorf("meilisearch-init CapAdd = %v, want %v", svc.CapAdd, want)
	}
	if len(svc.CapDrop) != 1 || svc.CapDrop[0] != "ALL" {
		t.Errorf("meilisearch-init CapDrop = %v, want [ALL]", svc.CapDrop)
	}
	if meili := dc.Services["meilisearch"]; len(meili.CapAdd) != 0 {
		t.Errorf("meilisearch itself must keep the default profile, got CapAdd %v", meili.CapAdd)
	}
}
