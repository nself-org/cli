package commands

import (
	"github.com/nself-org/cli/internal/templates/clone"
	"reflect"
	"testing"
)

func TestTemplateNextUsesManifest(t *testing.T) {
	m, err := clone.GetManifest("multi-app")
	if err != nil {
		t.Fatal(err)
	}
	if got := templateNext(m.Name); !reflect.DeepEqual(got, m.NextSteps) {
		t.Fatalf("got %v, want manifest steps %v", got, m.NextSteps)
	}
}
