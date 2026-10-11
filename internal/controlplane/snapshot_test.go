package controlplane

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// recRemote records commands and answers the three backup commands.
type recRemote struct {
	log     []string
	failOn  string
	listOut string
}

func (r *recRemote) Label() string { return "host-a" }
func (r *recRemote) Rsync(context.Context, []string, string, string, string) error {
	return errors.New("rsync is not part of a snapshot")
}
func (r *recRemote) Run(_ context.Context, c string) ([]byte, error) {
	r.log = append(r.log, c)
	if r.failOn != "" && strings.Contains(c, r.failOn) {
		return nil, errors.New("exit status 1: boom")
	}
	if strings.Contains(c, "backup list") {
		return []byte(r.listOut), nil
	}
	return nil, nil
}

const listJSON = `[
 {"id":"proj_full_20261010_120000_pre-deploy-OTHER","date":"2026-10-10T12:00:00Z","tag":"120000_pre-deploy-OTHER"},
 {"id":"proj_full_20261010_130000_pre-deploy-ID1","date":"2026-10-10T13:00:00Z","tag":"130000_pre-deploy-ID1"}]`

func TestVerifiedSnapshot(t *testing.T) {
	ctx := context.Background()
	t.Run("create, list by tag, verify, in that order", func(t *testing.T) {
		r := &recRemote{listOut: listJSON}
		id, err := VerifiedSnapshot(ctx, r, "/opt/nself", "pre-deploy-ID1")
		if err != nil || id != "proj_full_20261010_130000_pre-deploy-ID1" {
			t.Fatalf("id %q err %v", id, err)
		}
		want := []string{"backup create --tag 'pre-deploy-ID1'", "backup list --format json", "backup verify 'proj_full_20261010_130000_pre-deploy-ID1'"}
		if len(r.log) != 3 {
			t.Fatalf("log %v", r.log)
		}
		for i, w := range want {
			if !strings.HasPrefix(r.log[i], "cd '/opt/nself' && nself ") || !strings.HasSuffix(r.log[i], w) {
				t.Errorf("command %d = %q, want ...%s", i, r.log[i], w)
			}
		}
	})
	for _, c := range []struct{ name, failOn, list string }{
		{"create fails", "backup create", listJSON},
		{"list fails", "backup list", listJSON},
		{"verify fails", "backup verify", listJSON},
		{"no entry carries the tag", "", `[{"id":"x","tag":"y"}]`},
		{"list is not json", "", `No backups found.`},
	} {
		t.Run(c.name+" gives E206 and no compose command ran", func(t *testing.T) {
			r := &recRemote{listOut: c.list, failOn: c.failOn}
			id, err := VerifiedSnapshot(ctx, r, "/opt/nself", "pre-deploy-ID1")
			var ce *errs.CLIError
			if id != "" || !errors.As(err, &ce) || ce.Code != "E206" {
				t.Fatalf("id %q err %v, want E206", id, err)
			}
			for _, cmd := range r.log {
				if strings.Contains(cmd, "docker") {
					t.Errorf("a compose/docker command ran: %q", cmd)
				}
			}
		})
	}
	t.Run("the --json envelope is read too", func(t *testing.T) {
		got, err := backupIDByTag([]byte(`{"ok":true,"data":[{"id":"a_b_c_d_T1","tag":"d_T1","date":"1"}]}`), "T1")
		if err != nil || got != "a_b_c_d_T1" {
			t.Fatalf("%q %v", got, err)
		}
	})
}
