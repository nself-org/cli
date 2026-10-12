package docker

// Purpose: the init-container cleanup removes only finished, successful
//          containers (P7-PLUG-60): a running init container or a failed one
//          (kept for `docker logs`) must never be selected.

import (
	"reflect"
	"testing"
)

func TestInitContainerListArgsSelectExitedZeroOnly(t *testing.T) {
	want := []string{"ps", "-aq",
		"--filter", "label=nself.type=init-container",
		"--filter", "status=exited",
		"--filter", "exited=0",
	}
	if got := initContainerListArgs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("initContainerListArgs() = %v, want %v", got, want)
	}
}
