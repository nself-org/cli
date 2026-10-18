package commands

// Regression test for D-0931: `nself restart nginx` recreates the owner's nginx
// container, so the registered fronted projects must be attached again.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/sharedproxy"
)

func TestRestartServicesReattachesTheSharedProxy(t *testing.T) {
	owner, _, _ := proxyLayout(t)
	fake := filepath.Join(t.TempDir(), "docker")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	comp := docker.NewCompose()
	comp.DockerPath = fake
	for _, tc := range []struct {
		services []string
		want     []attachCall
	}{
		{[]string{"nginx"}, []attachCall{{owner: owner, projects: []string{"b", "c"}}}},
		{[]string{"hasura"}, nil},
	} {
		calls := stubAttach(t, sharedproxy.Outcome{}, nil)
		if err := restartServices(context.Background(), comp, owner, &config.Config{}, tc.services, false); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(*calls, tc.want) && (len(*calls) != 0 || len(tc.want) != 0) {
			t.Fatalf("restart %v: attach calls = %+v, want %+v", tc.services, *calls, tc.want)
		}
	}
}
