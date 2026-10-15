package model

import (
	"github.com/nself-org/cli/internal/docker/dockertest"
	"testing"
)

func TestDetectorComposeCarriesEnvFiles(t *testing.T) {
	root, check := dockertest.ProjectEnv(t, "plugin-notify")
	t.Chdir(root)
	if !defaultResolveOpts().dockerCheck("notify") {
		t.Fatal("plugin not detected")
	}
	check()
}
