package bluegreen

import (
	"context"
	"github.com/nself-org/cli/internal/docker/dockertest"
	"testing"
)

func TestTrafficComposeCarriesEnvFiles(t *testing.T) {
	root, check := dockertest.ProjectEnv(t, "nginx\trunning\thealthy")
	if err := reloadNginx(root); err != nil {
		t.Fatal(err)
	}
	if _, err := measureErrorRate(context.Background(), DeployConfig{ProjectRoot: root}); err != nil {
		t.Fatal(err)
	}
	check()
}
