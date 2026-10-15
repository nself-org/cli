package imageswitch

import (
	"context"
	"slices"
	"testing"
)

func TestRestoreDropsDefaultCapabilities(t *testing.T) {
	f, s := newFake(t)
	rec := &specRecorder{fakeEnv: f}
	s.d.Runtime = rec
	if _, err := s.Run(context.Background(), RunOptions{}); err != nil {
		t.Fatal(err)
	}
	restore := rec.byName("-restore")
	if !slices.Equal(restore.CapDrop, []string{"ALL"}) {
		t.Fatalf("restore CapDrop = %v, want [ALL]", restore.CapDrop)
	}
	want := []string{"CHOWN", "SETUID", "SETGID", "DAC_OVERRIDE", "FOWNER"}
	if !slices.Equal(restore.CapAdd, want) {
		t.Errorf("restore CapAdd = %v, want %v", restore.CapAdd, want)
	}
}
