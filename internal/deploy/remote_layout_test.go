package deploy

// Fix round 3 (H1, H2, H4): the symlink rule and the locally driven prune.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// H1: a link that leaves remote_path is refused with E485 before the lock, the
// tar and any sync; nothing is sent.
func TestOutsideSymlinkRefusedBeforeLock(t *testing.T) {
	for name, mk := range map[string]func(rp, outside string) error{
		"dir": func(rp, outside string) error {
			if err := os.RemoveAll(rp + "/hasura"); err != nil {
				return err
			}
			return os.Symlink(outside, rp+"/hasura")
		},
		"dir inside nginx": func(rp, outside string) error { return os.Symlink(outside, rp+"/nginx/sites/ext") },
		"env file": func(rp, outside string) error {
			writeFile(t, outside+"/secret", "S=1\n", 0o600)
			return os.Symlink(outside+"/secret", rp+"/.env.prod")
		},
	} {
		t.Run(name, func(t *testing.T) {
			f, o, rp := applyFixture(t, "")
			outside := t.TempDir()
			os.Remove(rp + "/.env.prod")
			if err := mk(rp, outside); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			_, err := ApplyRemote(context.Background(), o)
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E485" || !strings.Contains(err.Error(), "symlink outside remote_path") {
				t.Fatalf("err = %v", err)
			}
			for _, s := range steps(f) {
				if s == "lock" || s == "tar" || s == "rsync" || s == "dbbackup" {
					t.Errorf("%s ran before the refusal: %v", s, steps(f))
				}
			}
			if exists(outside + "/metadata/version.yaml") {
				t.Error("a file was written outside remote_path")
			}
		})
	}
}

// H2: with nginx/ a link that stays inside remote_path, a deploy that fails after
// the sync removes the file it created through the link and leaves the record
// alone; a later prune through the link works.
func TestSymlinkedNginxInsideRemotePath(t *testing.T) {
	f, o, rp := applyFixture(t, "")
	if err := os.Rename(rp+"/nginx", rp+"/.shared-nginx"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".shared-nginx", rp+"/nginx"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	f.fail["docker run --rm"] = errBoom // nginx -t refuses after the sync
	res, err := ApplyRemote(context.Background(), o)
	if err == nil || strings.Contains(err.Error(), "also failed") {
		t.Fatalf("want a clean refusal, got %v", err)
	}
	if res.Synced {
		t.Errorf("a restored deploy must report Synced=false: %+v", res)
	}
	if len(res.Absent) == 0 {
		t.Fatal("the fixture should create at least one file through the link")
	}
	for _, a := range res.Absent {
		if exists(rp + "/" + a) {
			t.Errorf("%s created through the link survived the restore", a)
		}
	}
	if info, err := os.Lstat(rp + "/nginx"); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("nginx/ is no longer a symlink")
	}
	// A later prune through the link.
	writeFile(t, rp+"/.shared-nginx/sites/dropped.conf", markedConf, 0o644)
	writeFile(t, rp+"/"+ShippedFile, "nginx/sites/dropped.conf\n", 0o644)
	removed, err := PrunePrevious(context.Background(), f, rp, map[string]bool{}, []string{"nginx/sites/dropped.conf"})
	if err != nil || len(removed) != 1 || exists(rp+"/.shared-nginx/sites/dropped.conf") {
		t.Fatalf("prune through an inside link: removed %v err %v", removed, err)
	}
}

// H4: without a local record nothing is pruned, even when the host's list names
// a marked file.
func TestPruneNeedsLocalRecord(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	writeFile(t, rp+"/nginx/sites/foreign.conf", markedConf, 0o644)
	writeFile(t, rp+"/"+ShippedFile, "nginx/sites/foreign.conf\n", 0o644)
	removed, err := PrunePrevious(context.Background(), f, rp, map[string]bool{}, nil)
	if err != nil || len(removed) != 0 || !exists(rp+"/nginx/sites/foreign.conf") {
		t.Fatalf("pruned without a record: %v %v", removed, err)
	}
}
