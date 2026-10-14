package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRollbackRetainsRunningImages(t *testing.T) {
	f, o, rp := applyFixture(t, "")
	base := f.handler
	f.handler = func(s string) (string, bool) {
		if strings.Contains(s, "ps --status running --quiet") {
			if strings.HasSuffix(s, "'db'") {
				return "running-db\n", true
			}
			return "running-web\n", true
		}
		if strings.Contains(s, "docker inspect --type container --format '{{.Image}}'") {
			if strings.Contains(s, "'running-db'") {
				return fakeImageID("d") + "\n", true
			}
			return fakeImageID("c") + "\n", true
		}
		if strings.Contains(s, "config --format json") {
			// Two services can share a mutable tag while their running images differ.
			return strings.NewReplacer("postgres:16", "shared:latest", "web:1", "shared:latest").Replace(modelJSON(rp)), true
		}
		return base(s)
	}
	res, err := ApplyRemote(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.log = nil
	f.mu.Unlock()
	err = RollbackRemote(context.Background(), RollbackOptions{Remote: f, RemotePath: rp, Tar: res.PreDeployTar, Changed: res.Changed, Absent: res.Absent, Clock: &fakeClock{now: time.Unix(0, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range f.commands() {
		if strings.Contains(s, " pull") {
			t.Fatalf("rollback pulled a mutable image: %s", s)
		}
	}
	joined := strings.Join(f.commands(), "\n")
	if !strings.Contains(joined, fakeImageID("c")) || !strings.Contains(joined, "--pull never") {
		t.Fatal("rollback did not pin the running image with pulls disabled")
	}
	if !strings.Contains(joined, `"db":{"image":"`+fakeImageID("d")) || !strings.Contains(joined, `"web":{"image":"`+fakeImageID("c")) {
		t.Fatal("services sharing a mutable tag did not retain their own running IDs")
	}
}

func TestRollbackRejectsInvalidImageEvidence(t *testing.T) {
	for _, images := range []map[string]string{{"web": "latest"}, {"../web": fakeImageID("a")}} {
		f := newFakeRemote(t)
		rp := t.TempDir()
		bk, err := CreatePreDeployTar(context.Background(), f, rp, NewBackupID(time.Now()), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Run(context.Background(), shScript("set -e; "+archiveImagesScript(bk.Tar, images))); err != nil {
			t.Fatal(err)
		}
		_, err = readRollbackImages(context.Background(), f, rp, bk.Tar)
		if err == nil || !strings.Contains(err.Error(), "E489") {
			t.Fatalf("want E489 for %v, got %v", images, err)
		}
	}
}

func TestImageArchiveUpdateFailureKeepsOriginalBackup(t *testing.T) {
	f := newFakeRemote(t)
	rp := t.TempDir()
	bk, err := CreatePreDeployTar(context.Background(), f, rp, NewBackupID(time.Now()), nil)
	if err != nil {
		t.Fatal(err)
	}
	before := readFile(t, bk.Tar)
	script := strings.Replace(archiveImagesScript(bk.Tar, map[string]string{"web": fakeImageID("a")}), "gzip -n -c", "failing_gzip", 1)
	_, err = f.Run(context.Background(), shScript("set -e; failing_gzip() { printf incomplete; return 1; }; "+script))
	if err == nil {
		t.Fatal("archive update unexpectedly succeeded")
	}
	if after := readFile(t, bk.Tar); after != before {
		t.Fatal("failed archive update corrupted the recorded backup")
	}
	if got := tarList(t, bk.Tar); strings.Contains(got, rollbackImagesFile) {
		t.Fatal("failed archive update replaced the original backup")
	}
}

func TestRollbackMissingImageRefusesBeforeRestore(t *testing.T) {
	f, o, rp := applyFixture(t, "")
	base := f.handler
	f.handler = func(s string) (string, bool) {
		if strings.Contains(s, "ps --status running --quiet") {
			return "running-web\n", true
		}
		if strings.Contains(s, "docker inspect --type container --format '{{.Image}}'") {
			return fakeImageID("c") + "\n", true
		}
		return base(s)
	}
	res, err := ApplyRemote(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	f.fail["docker image inspect "+q(fakeImageID("c"))] = errors.New("image deleted")
	f.mu.Lock()
	f.log = nil
	f.mu.Unlock()
	err = RollbackRemote(context.Background(), RollbackOptions{Remote: f, RemotePath: rp, Tar: res.PreDeployTar, BackupID: "backup-1", Changed: res.Changed, Absent: res.Absent})
	if err == nil || !strings.Contains(err.Error(), "E489") {
		t.Fatalf("want E489, got %v", err)
	}
	for _, s := range f.commands() {
		if strings.Contains(s, "tar -xzf") || strings.Contains(s, "nself backup restore") || strings.Contains(s, "up -d") {
			t.Fatalf("missing image changed host: %s", s)
		}
	}
}
