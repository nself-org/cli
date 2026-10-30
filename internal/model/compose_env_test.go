package model

import (
	"testing"

	"github.com/nself-org/cli/internal/docker/dockertest"
)

func TestModelComposeCarriesEnvFiles(t *testing.T) {
	root, calls := dockertest.ComposeProject(t, "plugin-notify\n")
	t.Chdir(root)
	if !defaultResolveOpts().dockerCheck("notify") {
		t.Fatal("compose plugin service was not detected")
	}
	if len(calls()) != 1 {
		t.Fatal("expected one compose ps")
	}
}
