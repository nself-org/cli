package load

import (
	"github.com/nself-org/cli/internal/portable"
	"testing"
)

func TestArchiveCannotBypassTargetGuard(t *testing.T) {
	m := portable.Manifest{DB: portable.DB{Schemas: []string{"app"}, Tables: []portable.Table{{Schema: "app", Name: "notes"}}}}
	good := "1; 1259 123 TABLE app notes postgres\n2; 0 123 TABLE DATA app notes postgres\n"
	if err := validateArchive(good, m); err != nil {
		t.Fatal(err)
	}
	for _, toc := range []string{"2; 0 123 TABLE DATA public existing postgres", "1; 1259 123 TABLE app np_plugins postgres", "1; 1259 123 TABLE app unsafe;name postgres"} {
		if err := validateArchive(toc, m); err == nil {
			t.Fatalf("unsafe archive accepted: %s", toc)
		}
	}
	m.DB.Schemas = nil
	if err := validateArchive(good, m); err == nil {
		t.Fatal("undeclared schema accepted")
	}
}
