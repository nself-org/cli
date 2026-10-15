package imageswitch

import (
	"context"
	"testing"
)

func TestRollbackPersistsAbsentAndExplicitEmptyEnv(t *testing.T) {
	_, s := newFake(t)
	s.cfg.PrevEnv = map[string]string{EnvImage: ""}
	if _, err := s.Run(context.Background(), RunOptions{}); err != nil {
		t.Fatal(err)
	}
	st, err := LoadState(s.cfg.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	if value, present := st.PrevEnv[EnvImage]; !present || value != "" {
		t.Fatalf("explicit empty image was lost: %v", st.PrevEnv)
	}
	if _, present := st.PrevEnv[EnvDataVolume]; present {
		t.Fatalf("absent volume became an empty assignment: %v", st.PrevEnv)
	}
	if _, err := New(s.cfg, s.d).Rollback(context.Background(), true); err != nil {
		t.Fatal(err)
	}
}
