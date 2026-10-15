package deploy

import (
	"context"
	"github.com/nself-org/cli/internal/docker/dockertest"
	"testing"
)

func TestRollbackComposeCarriesEnvFiles(t *testing.T) {
	root, check := dockertest.ProjectEnv(t, "")
	if err := runRollback(context.Background(), root, "notify", "v1", "dev"); err != nil {
		t.Fatal(err)
	}
	check()
}
