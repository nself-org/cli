package database

// Purpose: P7-PLUG-20 fix round 1: --plugin must be a plain plugin name before
// any migration directory is scanned. The checks run before any database
// access, so no Postgres is needed.

import (
	"context"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

func TestPluginArgRefusedBeforeAnyScan(t *testing.T) {
	cfg := &config.Config{}
	for _, name := range []string{"../victim", "./x", "a/b", "..", "/etc"} {
		if err := validatePluginArg(name); err == nil {
			t.Errorf("validatePluginArg(%q) = nil", name)
		}
		_, err := MigrateUp(context.Background(), cfg, name)
		if d := errs.Describe(err); d == nil || d.Code != "E401" {
			t.Errorf("MigrateUp(%q) = %v, want E401", name, err)
		}
		if _, err := PendingMigrations(context.Background(), cfg, name); errs.Describe(err) == nil {
			t.Errorf("PendingMigrations(%q) = %v, want E401", name, err)
		}
		if _, err := VerifyChecksums(context.Background(), cfg, name); errs.Describe(err) == nil {
			t.Errorf("VerifyChecksums(%q) = %v, want E401", name, err)
		}
	}
	if err := validatePluginArg(""); err != nil {
		t.Errorf("empty --plugin must stay valid: %v", err)
	}
	if err := validatePluginArg("notify"); err != nil {
		t.Errorf("notify: %v", err)
	}
}
