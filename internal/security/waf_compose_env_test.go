package security

import (
	"context"
	"github.com/nself-org/cli/internal/docker/dockertest"
	"testing"
)

func TestWAFComposeCarriesEnvFiles(t *testing.T) {
	root, check := dockertest.ProjectEnv(t, "")
	if _, _, err := ReadWAFAuditLogFromContainer(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	check()
}
