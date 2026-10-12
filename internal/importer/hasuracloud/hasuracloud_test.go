package hasuracloud

import (
	"testing"

	"github.com/nself-org/cli/internal/importer"
)

func TestRegisteredWithEnvNameFlags(t *testing.T) {
	var found importer.Source
	for _, s := range importer.Sources() {
		if s.Name() == "hasura-cloud" {
			found = s
		}
	}
	if found == nil {
		t.Fatal("hasura-cloud not registered")
	}
	want := map[string]bool{"db-url-env": true, "hasura-url": false, "admin-secret-env": true}
	if len(found.Flags()) != len(want) {
		t.Fatalf("flags %+v", found.Flags())
	}
	for _, f := range found.Flags() {
		if env, ok := want[f.Name]; !ok || env != f.EnvVarName {
			t.Errorf("flag %+v", f)
		}
	}
	if got := pgInput(importer.Input{Values: map[string]string{"db-url-env": "SRC"}}); got.Values["from-env"] != "SRC" {
		t.Fatalf("pgInput %+v", got)
	}
}
