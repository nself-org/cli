package deploy

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// shRecorder records every command string exactly as SSHRemote would hand it
// to ssh, before the fake host unwraps it.
type shRecorder struct {
	Remote
	mu  sync.Mutex
	raw []string
}

func (s *shRecorder) Run(ctx context.Context, cmd string) ([]byte, error) {
	s.mu.Lock()
	s.raw = append(s.raw, cmd)
	s.mu.Unlock()
	return s.Remote.Run(ctx, cmd)
}

// portableUnwrapped are the only commands allowed past the wrapper, matched
// exactly: one simple command every login shell parses the same way
// (target_snapshot.go reads $HOME with it).
var portableUnwrapped = map[string]bool{`printf '%s\n' "$HOME"`: true}

// unwrapped returns the recorded commands a non-POSIX login shell would parse.
func (s *shRecorder) unwrapped() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var bad []string
	for _, c := range s.raw {
		if !strings.HasPrefix(c, "sh -c '") && !portableUnwrapped[c] {
			bad = append(bad, c)
		}
	}
	return bad
}

// D-0542: every remote step of a v1.5 apply and rollback runs under sh -c, so a
// fish or csh login shell never parses the scripts.
func TestEveryRemoteRunIsShWrapped(t *testing.T) {
	t.Run("positive control sees an unwrapped command", func(t *testing.T) {
		rec := &shRecorder{Remote: newFakeRemote(t)}
		_, _ = rec.Run(context.Background(), "cd /srv && for i in 1 2; do true; done")
		_, _ = rec.Run(context.Background(), shScript("true"))
		if bad := rec.unwrapped(); len(bad) != 1 || !strings.HasPrefix(bad[0], "cd /srv") {
			t.Fatalf("unwrapped = %q", bad)
		}
	})
	t.Run("apply with hasura metadata", func(t *testing.T) {
		f, o, _ := applyFixture(t, "")
		rec := &shRecorder{Remote: f}
		o.Remote = rec
		if _, err := ApplyRemote(context.Background(), o); err != nil {
			t.Fatal(err)
		}
		if indexOf(steps(f), "hasura") < 0 {
			t.Fatalf("the metadata apply did not run: %v", steps(f))
		}
		if bad := rec.unwrapped(); len(bad) > 0 {
			t.Errorf("unwrapped remote commands: %q", bad)
		}
	})
	t.Run("apply refused at E403 puts image tags back", func(t *testing.T) {
		f, o, _ := applyFixture(t, "")
		rec := &shRecorder{Remote: f}
		o.Remote, o.Yes = rec, false
		f.handler = func(s string) (string, bool) {
			if strings.Contains(s, "up -d --dry-run") {
				return " Container proj-db-1 Recreate\n", true
			}
			return stackHandler(o.RemotePath, "")(s)
		}
		if _, err := ApplyRemote(context.Background(), o); err == nil {
			t.Fatal("want E403")
		}
		if indexOf(steps(f), "imgrestore") < 0 {
			t.Fatalf("the tag restore did not run: %v", steps(f))
		}
		if bad := rec.unwrapped(); len(bad) > 0 {
			t.Errorf("unwrapped remote commands: %q", bad)
		}
	})
	t.Run("rollback with a database restore", func(t *testing.T) {
		f, o, rp := applyFixture(t, "")
		res, err := ApplyRemote(context.Background(), o)
		if err != nil {
			t.Fatal(err)
		}
		rec := &shRecorder{Remote: f}
		err = RollbackRemote(context.Background(), RollbackOptions{Remote: rec, RemotePath: rp, Tar: res.PreDeployTar, BackupID: "backup-1", Changed: res.Changed, Absent: res.Absent, StrictHasura: true, Clock: &fakeClock{now: time.Unix(0, 0)}})
		if err != nil {
			t.Fatal(err)
		}
		var restore bool
		for _, c := range rec.raw {
			restore = restore || strings.Contains(c, "nself backup restore")
		}
		if !restore {
			t.Fatalf("the database restore did not run: %q", rec.raw)
		}
		if bad := rec.unwrapped(); len(bad) > 0 {
			t.Errorf("unwrapped remote commands: %q", bad)
		}
	})
	t.Run("the legacy push shares the wrapped metadata command", func(t *testing.T) {
		got := HasuraMetadataApplyCommand("/srv/my app")
		if !strings.HasPrefix(got, "sh -c '") || !strings.Contains(unshell(got), "cd '/srv/my app' && for i in 1 2 3 4 5;") {
			t.Errorf("got %q", got)
		}
	})
}
