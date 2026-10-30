package security

import (
	"context"
	"testing"

	"github.com/nself-org/cli/internal/docker/dockertest"
)

func TestWAFAuditComposeCarriesEnvFiles(t *testing.T) {
	root, calls := dockertest.ComposeProject(t, "")
	if _, _, err := ReadWAFAuditLogFromContainer(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if len(calls()) != 1 {
		t.Fatal("expected one compose exec")
	}
}
