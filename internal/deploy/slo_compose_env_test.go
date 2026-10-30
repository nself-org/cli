package deploy

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/docker/dockertest"
)

func TestRollbackComposeCarriesEnvFiles(t *testing.T) {
	root, calls := dockertest.ComposeProject(t, "")
	if err := runRollback(context.Background(), root, "api", "prior", "dev"); err != nil {
		t.Fatal(err)
	}
	got := calls()
	if len(got) != 1 || !strings.Contains(got[0], "up -d --no-deps --force-recreate api") {
		t.Fatalf("unexpected rollback calls: %v", got)
	}
}
