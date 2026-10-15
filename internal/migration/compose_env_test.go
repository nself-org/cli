package migration

import (
	"context"
	"github.com/nself-org/cli/internal/docker/dockertest"
	"testing"
)

func TestMigrationComposeCarriesEnvFiles(t *testing.T) {
	root, check := dockertest.ProjectEnv(t, "")
	if err := composeDown(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	check()
}
