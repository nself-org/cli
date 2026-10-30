package migration

import (
	"context"
	"testing"

	"github.com/nself-org/cli/internal/docker/dockertest"
)

func TestMigrationComposeCarriesEnvFiles(t *testing.T) {
	root, calls := dockertest.ComposeProject(t, "")
	if err := composeDown(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if len(calls()) != 1 {
		t.Fatal("expected one compose down")
	}
}
