package deploy

// D-0700: a rollback puts the image ids the previous deploy ran back under
// their tags and does not pull those tags again; a recorded image that is gone
// refuses the rollback (E489).

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// rollbackFixture applies a deploy on the stack fixture and returns the host,
// the apply result and a function that rolls it back and reports the commands.
func rollbackFixture(t *testing.T) (f *fakeRemote, res ApplyResult, rp string, roll func() error) {
	t.Helper()
	needTools(t)
	f, o, rp := applyFixture(t, "")
	res, err := ApplyRemote(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	roll = func() error {
		f.mu.Lock()
		f.log = nil
		f.mu.Unlock()
		return RollbackRemote(context.Background(), RollbackOptions{Remote: f, RemotePath: rp, Tar: res.PreDeployTar, Changed: res.Changed, Absent: res.Absent, Clock: &fakeClock{now: time.Unix(0, 0)}})
	}
	return f, res, rp, roll
}

func commandsWith(f *fakeRemote, sub string) []string {
	var out []string
	for _, c := range f.commands() {
		if strings.Contains(c, sub) {
			out = append(out, c)
		}
	}
	return out
}

func TestApplyRecordsTheRunningImageIDs(t *testing.T) {
	_, res, _, _ := rollbackFixture(t)
	got := readFile(t, imageRecordPath(res.PreDeployTar))
	want := fakeImageID("a") + " postgres:16\n" + fakeImageID("b") + " web:1\n"
	if got != want {
		t.Fatalf("image record = %q, want %q", got, want)
	}
	if info, err := os.Stat(imageRecordPath(res.PreDeployTar)); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("image record mode: %v %v", info, err)
	}
}

func TestRollbackRestoresRecordedImagesInsteadOfPulling(t *testing.T) {
	f, res, _, roll := rollbackFixture(t)
	if err := roll(); err != nil {
		t.Fatal(err)
	}
	got := steps(f)
	tag, pull := indexOf(got, "imgrestore"), indexOf(got, "pull")
	if tag < 0 || pull < 0 || tag > pull || indexOf(got, "up:db") < pull {
		t.Fatalf("want the tags put back, then a pull, then the services: %v", got)
	}
	tags := strings.Join(commandsWith(f, "docker tag"), "\n")
	for _, w := range []string{"docker tag '" + fakeImageID("a") + "' 'postgres:16'", "docker tag '" + fakeImageID("b") + "' 'web:1'"} {
		if !strings.Contains(tags, w) {
			t.Errorf("missing %q in %s", w, tags)
		}
	}
	pulls := commandsWith(f, " pull")
	if len(pulls) != 1 || !strings.HasSuffix(pulls[0], "pull -- 'files' 'job' 'nginx'") {
		t.Errorf("the rollback may pull only services without a record: %v", pulls)
	}
	_ = res
}

func TestRollbackPullsNothingWhenEveryImageIsRecorded(t *testing.T) {
	f, res, _, roll := rollbackFixture(t)
	// Record every ref the fixture model names (the digest-pinned nginx cannot move and has none).
	all := fakeImageID("a") + " postgres:16\n" + fakeImageID("a") + " web:1\n" + fakeImageID("b") + " job:1\n" + fakeImageID("b") + " f:1\n"
	writeFile(t, imageRecordPath(res.PreDeployTar), all, 0o600)
	if err := roll(); err != nil {
		t.Fatal(err)
	}
	if got := commandsWith(f, " pull"); len(got) != 1 || !strings.HasSuffix(got[0], "pull -- 'nginx'") {
		t.Errorf("only the digest-pinned service may still be pulled: %v", got)
	}
}

func TestRollbackRefusesWhenARecordedImageIsGone(t *testing.T) {
	f, res, _, roll := rollbackFixture(t)
	inner := f.handler
	f.handler = func(s string) (string, bool) {
		if strings.Contains(s, `printf 'OK`) {
			return "OK " + fakeImageID("a") + "\n", true // the web image is gone
		}
		return inner(s)
	}
	err := roll()
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E489" || !strings.Contains(err.Error(), "web:1") || strings.Contains(err.Error(), "postgres:16") {
		t.Fatalf("err = %v, want E489 naming web:1 only", err)
	}
	got := steps(f)
	for _, late := range []string{"dbbackup", "imgrestore", "pull", "up:db"} {
		if indexOf(got, late) >= 0 {
			t.Errorf("step %q ran after the refusal: %v", late, got)
		}
	}
	_ = res
}

func TestRollbackWithoutOrWithTamperedRecord(t *testing.T) {
	t.Run("no record keeps the full pull", func(t *testing.T) {
		f, res, _, roll := rollbackFixture(t)
		if err := os.Remove(imageRecordPath(res.PreDeployTar)); err != nil {
			t.Fatal(err)
		}
		if err := roll(); err != nil {
			t.Fatal(err)
		}
		pulls := commandsWith(f, " pull")
		if len(pulls) != 1 || strings.Contains(pulls[0], "pull --") || len(commandsWith(f, "docker tag")) != 0 {
			t.Errorf("a deploy with no record rolls back with the plain pull: pulls=%v", pulls)
		}
	})
	t.Run("a ref the restored model does not name is ignored", func(t *testing.T) {
		f, res, _, roll := rollbackFixture(t)
		writeFile(t, imageRecordPath(res.PreDeployTar), fakeImageID("c")+" evil:1\nnot-an-id web:1\n"+fakeImageID("b")+" web:1\n", 0o600)
		if err := roll(); err != nil {
			t.Fatal(err)
		}
		tags := commandsWith(f, "docker tag")
		if len(tags) != 1 || !strings.Contains(tags[0], "'web:1'") || strings.Contains(tags[0], "evil") {
			t.Errorf("tags put back: %v", tags)
		}
	})
}
