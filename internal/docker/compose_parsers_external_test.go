package docker_test

import (
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker"
)

func TestSharedComposeParsers(t *testing.T) {
	for _, raw := range []string{
		`[{"ID":"a","State":"exited","ExitCode":2}]`,
		"\r\n{\"ID\":\"a\",\"State\":\"exited\",\"ExitCode\":2}\r\n\n",
	} {
		states, err := docker.ParseContainerStates([]byte(raw))
		if err != nil || len(states) != 1 {
			t.Fatalf("states = %+v, err = %v", states, err)
		}
		if states[0].ID != "a" || states[0].State != "exited" || states[0].ExitCode != 2 {
			t.Fatalf("state = %+v", states[0])
		}
	}
	if states, err := docker.ParseContainerStates(nil); err != nil || len(states) != 0 {
		t.Fatalf("empty states = %+v, err = %v", states, err)
	}
	if _, err := docker.ParseContainerStates([]byte("{}\ninvalid")); err == nil {
		t.Fatal("malformed container state accepted")
	}
	specs, err := docker.ParseServiceSpecs([]byte(`{"services":{
		"z":{"healthcheck":{"test":["NONE"]}},
		"a":{"restart":"always","healthcheck":{"test":["CMD","true"],"interval":"1s","timeout":"2s","start_period":"3s","retries":2}}
	}}`))
	if err != nil || len(specs) != 2 {
		t.Fatalf("specs = %+v, err = %v", specs, err)
	}
	if specs[0].Name != "a" || specs[0].Restart != "always" || !specs[0].HasHealthcheck || specs[0].HealthTimeout != 9*time.Second || specs[1].HasHealthcheck {
		t.Fatalf("specs = %+v", specs)
	}
	if _, err := docker.ParseServiceSpecs([]byte(`{"services":{"a":{"healthcheck":{"interval":"bad"}}}}`)); err == nil {
		t.Fatal("malformed healthcheck accepted")
	}
}
