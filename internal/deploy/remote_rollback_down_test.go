package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRollbackFirstDeployStopsTheStack(t *testing.T) {
	needTools(t)
	for _, downFails := range []bool{false, true} {
		f, o, rp := applyFixture(t, "")
		for _, n := range []string{"docker-compose.yml", ".nself/compose-files.txt", ".nself/compose-env-files.txt", ShippedFile} { // no earlier deploy
			_ = os.Remove(filepath.Join(rp, n))
		}
		o.Previous = nil
		res, err := ApplyRemote(context.Background(), o) // the failed release's files are on the host
		if err != nil {
			t.Fatal(err)
		}
		if downFails {
			f.fail[" down "] = errors.New("daemon gone")
		}
		f.mu.Lock()
		f.log = nil
		f.mu.Unlock()
		err = RollbackRemote(context.Background(), RollbackOptions{Remote: f, RemotePath: rp, Tar: res.PreDeployTar, Changed: res.Changed, Absent: res.Absent, Clock: &fakeClock{now: time.Unix(0, 0)}})
		downs := 0
		for _, c := range f.commands() {
			if strings.Contains(c, " down ") {
				downs++
				if strings.Contains(c, " -v") || strings.Contains(c, "--volumes") {
					t.Errorf("down removes volumes: %q", c)
				}
			}
			if strings.Contains(c, "up -d") {
				t.Errorf("first-deploy rollback re-applied: %q", c)
			}
		}
		if downs != 1 {
			t.Errorf("fails=%v: compose down ran %d times, want 1", downFails, downs)
		}
		if downFails != (err != nil) {
			t.Errorf("fails=%v: err = %v", downFails, err)
		}
	}
}
